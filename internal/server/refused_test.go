package server

import (
	"context"
	"encoding/hex"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
)

// TestAnArmNamesWhoItTurnedAwayAndKeepsWaiting — /pending 669 #3, ADR-156.
//
// A listener that refused a peer at the pinned-peer check told nobody, on either transport, and the
// person at the armed screen watched a blank wait. Driven through the real routes over TCP and QUIC:
// a stranger connects to an armed session, and the status the window polls must say so — in words
// that do not accuse the pinned peer — while the arm stays armed. The stranger's own side must be
// able to name what happened to it too.
func TestAnArmNamesWhoItTurnedAwayAndKeepsWaiting(t *testing.T) {
	for _, transport := range []string{transportTCP, transportQUIC} {
		t.Run(transport, func(t *testing.T) {
			ts, _ := startServer(t)
			c, csrf := authedClient(t, ts)
			meFP, err := hex.DecodeString(myFingerprint(t, c, ts.URL))
			if err != nil {
				t.Fatal(err)
			}
			aCert, _, err := sign.GenerateIdentity("Alice")
			if err != nil {
				t.Fatal(err)
			}
			aFP, _ := sign.Fingerprint(aCert)
			pinPeer(t, c, csrf, ts.URL, hex.EncodeToString(aFP))

			var armed sessionStatus
			sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
				jsonBody(armRequest{Fingerprint: hex.EncodeToString(aFP), Bind: "127.0.0.1:0", Transport: transport})), &armed)
			if !armed.Armed || armed.Address == "" {
				t.Fatalf("setup: arm failed: %+v", armed)
			}
			defer func() {
				write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/disarm", "application/json", nil).Body.Close()
			}()
			// STIMULUS: nothing has been turned away yet, so the line below is the stranger's doing.
			var before sessionStatus
			sessGet(t, c, ts.URL+"/api/session/status", &before)
			if before.TurnedAway != nil {
				t.Fatalf("a fresh arm already reports %+v", before.TurnedAway)
			}

			// Mallory pins this machine correctly and is not who it armed for.
			mCert, mKey, err := sign.GenerateIdentity("Mallory")
			if err != nil {
				t.Fatal(err)
			}
			conn, derr := dialPeerWithin(context.Background(), transport, armed.Address, mCert, mKey, meFP, 10*time.Second, nil)
			if derr == nil {
				// TLS 1.3: the refusal can arrive at the first read instead of at the dial.
				_ = conn.Stream.SetDeadline(time.Now().Add(10 * time.Second))
				_, derr = conn.Stream.Read(make([]byte, 1))
				conn.Close()
			}
			said, named := dialRefusalSentence(derr)
			if !named || !strings.Contains(said, "turned this computer away") {
				t.Errorf("the dialler that was turned away is told %q (named=%v) for %v", said, named, derr)
			}
			if got := connectFailure(derr); strings.Contains(got, "could not connect") {
				t.Errorf("being turned away still reads as a connect failure: %q", got)
			}

			var st sessionStatus
			deadline := time.Now().Add(10 * time.Second)
			for {
				sessGet(t, c, ts.URL+"/api/session/status", &st)
				if st.TurnedAway != nil || time.Now().After(deadline) {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if st.TurnedAway == nil {
				t.Fatal("a stranger was refused at the handshake and the armed status says nothing — " +
					"the person waiting is shown a blank wait")
			}
			if !st.Armed {
				t.Error("the refusal ended the arm: one stranger reaching the port must not spend a wait")
			}
			sum := st.TurnedAway.Summary
			if !strings.HasPrefix(sum, "Still waiting. ") || !strings.Contains(sum, "Someone other than the pinned peer connected") {
				t.Errorf("the line reads %q", sum)
			}
			if strings.Contains(strings.ToLower(sum+st.TurnedAway.Detail), "identity is wrong") {
				t.Errorf("anyone who reaches the port can cause this, and the line accuses the pinned peer: %q", sum)
			}
			if st.TurnedAway.At.IsZero() {
				t.Error("the line carries no time")
			}
			if st.Notice != nil {
				t.Errorf("a refusal while the arm is alive wrote the sticky notice (%+v): a stranger "+
					"could overwrite a real failure with it", st.Notice)
			}
		})
	}
}

// TestTheClockOutranksAStrangerAndTheWaitsEndGivesTheReason holds the one wording: which cause is
// shown, that a stranger's count cannot displace the clock, and that the same cause is the reason a
// spent wait gives — and that a wait which turned nobody away gives none.
func TestTheClockOutranksAStrangerAndTheWaitsEndGivesTheReason(t *testing.T) {
	skew := &p2p.ClockSkewError{Behind: true, By: 2 * time.Hour}

	cause, _, ok := refusalCause(p2p.RefusalReport{})
	if ok || cause != "" {
		t.Fatalf("nothing refused reads as %q", cause)
	}
	// A thousand strangers and one clock refusal: the clock is the sentence.
	cause, detail, ok := refusalCause(p2p.RefusalReport{NotPinned: 1000, Clock: 1, Skew: skew})
	if !ok || !strings.Contains(cause, "date and time") || !strings.Contains(cause, "behind") {
		t.Errorf("with a clock refusal beside 1000 strangers the cause reads %q", cause)
	}
	if !strings.Contains(detail, "1000") {
		t.Errorf("the strangers are not counted in the detail: %q", detail)
	}
	// The peer turning us away outranks a stranger too, and says both things it can mean.
	cause, _, _ = refusalCause(p2p.RefusalReport{NotPinned: 5, RefusedUs: 1})
	if !strings.Contains(cause, "different identity pinned") || !strings.Contains(cause, "clock") {
		t.Errorf("a refusal by the other side reads %q — its alert cannot say which, so the sentence must say both", cause)
	}
	cause, _, _ = refusalCause(p2p.RefusalReport{NotPinned: 5, WrongAnswer: 1})
	if !strings.Contains(cause, "answered at an address") {
		t.Errorf("a dial answered by something else reads %q", cause)
	}

	// The deadline reason, through the door every arm calls.
	se := &session{}
	se.noteWaitEnded(armInteractive, &p2p.Refusals{})
	if n := se.status().Notice; n != nil {
		t.Fatalf("a wait that turned nobody away left a notice: %+v", n)
	}
	r := &p2p.Refusals{}
	r.NoteDial(skew)
	se.noteWaitEnded(armInteractive, r)
	n := se.status().Notice
	if n == nil || n.What != "wait-ended-turned-away" {
		t.Fatalf("a wait that ended after a clock refusal left %+v", n)
	}
	if !strings.HasPrefix(n.Summary, "The wait ended and nobody got through. ") || !strings.Contains(n.Summary, "date and time") {
		t.Errorf("the reason reads %q", n.Summary)
	}
	// The live line and the reason are one wording.
	live := turnedAwayOf(r)
	if live == nil || strings.TrimPrefix(live.Summary, "Still waiting. ") != strings.TrimPrefix(n.Summary, "The wait ended and nobody got through. ") {
		t.Errorf("the live line (%+v) and the deadline reason (%q) name different causes", live, n.Summary)
	}
}

// TestEveryArmNamesItsRefusals — /pending 766 (2), ADR-009 over a lifecycle rule.
//
// The arms are three goroutines in two files, and this package has added a rule to one and not the
// others at least five times (R4-1, R4-3, 750, 744, 289). So the count is taken from the source: every
// function that installs an arm is known here with the goroutine that runs it, and every one of those
// goroutines calls both doors.
func TestEveryArmNamesItsRefusals(t *testing.T) {
	// The function that installs an arm → where its arm goroutine's body is.
	runs := map[string]struct{ file, fn string }{
		"handleSessionArm": {"session.go", "func (s *Server) runSession("},
		"armCeremonyHop":   {"session.go", "func (s *Server) runCeremonyReceive("},
		"armForDelivery":   {"delivery.go", "func (s *Server) armForDelivery("},
	}
	door := regexp.MustCompile(`sess\.(arm|armCeremony|armCeremonyByPolicy|armDeliveryForCeremony)\b`)
	funcName := regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?(\w+)\(`)
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	installers := map[string]bool{}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		b, rerr := os.ReadFile(f.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		code := stripLineComments(string(b))
		for _, m := range door.FindAllStringIndex(code, -1) {
			names := funcName.FindAllStringSubmatch(code[:m[0]], -1)
			if len(names) == 0 {
				t.Fatalf("%s: an arm is installed outside any function", f.Name())
			}
			installers[names[len(names)-1][1]] = true
		}
	}
	// STIMULUS: the scan found the installers it knows, so "no unknown installer" is a fact.
	if len(installers) < len(runs) {
		t.Fatalf("found %d function(s) that install an arm (%v), want the %d known — the scan is not "+
			"reading the arming doors", len(installers), installers, len(runs))
	}
	for name := range installers {
		if _, ok := runs[name]; !ok {
			t.Errorf("%s installs an arm and is not in this guard's table. A new arm must call "+
				"watchRefusals and noteWaitEnded as the others do, and be entered here with the "+
				"function that runs it", name)
		}
	}
	for name, at := range runs {
		b, rerr := os.ReadFile(at.file)
		if rerr != nil {
			t.Fatal(rerr)
		}
		code := stripLineComments(string(b))
		i := strings.Index(code, at.fn)
		if i < 0 {
			t.Fatalf("cannot find %s in %s — the guard is pinned to a function that is gone", at.fn, at.file)
		}
		body := funcBodyFrom(code, i)
		for _, call := range []string{"watchRefusals(", "noteWaitEnded("} {
			if !strings.Contains(body, call) {
				t.Errorf("the arm %s installs (%s) never calls %s. Its refused peers are then "+
					"counted and never shown, or shown and never given as the reason the wait ended — "+
					"the rule reaching some arms and not others", name, at.fn, strings.TrimSuffix(call, "("))
			}
		}
	}
}
