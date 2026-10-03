package server

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// noVerifier is a dialing peer whose user says the words do not match.
type noVerifier struct{}

func (noVerifier) ConfirmVerification(string) (bool, error) { return false, nil }

// TestTheOtherSidesNoToTheWordsReachesTheUserWhoConfirmed — /pending 802, the receiving arm's half.
//
// The armed user CONFIRMS the words; the dialing peer's user says they do not match. Before
// ADR-076 the dialer closed without a byte, this side's `Receive` read EOF, and the user who had
// confirmed was told nothing — a decided outcome filed as a dropped channel. It must now arrive as
// `p2p.ErrPeerDidNotConfirm` and put the sentence on the status notice, which is the only surface
// a background arm has.
func TestTheOtherSidesNoToTheWordsReachesTheUserWhoConfirmed(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	var me struct {
		Fingerprint string `json:"fingerprint"`
	}
	sessGet(t, c, ts.URL+"/api/peers", &me)
	bFPBytes, _ := hex.DecodeString(me.Fingerprint)
	aCert, aKey, err := sign.GenerateIdentity("Alice")
	if err != nil {
		t.Fatal(err)
	}
	aFPBytes, _ := sign.Fingerprint(aCert)
	aFP := hex.EncodeToString(aFPBytes)
	pinPeer(t, c, csrf, ts.URL, aFP)

	var armed sessionStatus
	sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
		jsonBody(armRequest{Fingerprint: aFP, Bind: "127.0.0.1:0"})), &armed)
	if !armed.Armed {
		t.Fatalf("setup: arm failed: %+v", armed)
	}

	errc := make(chan error, 1)
	go func() {
		base, e := testpdf.Form()
		if e != nil {
			errc <- e
			return
		}
		conn, e := p2p.Dial(context.Background(), armed.Address, aCert, aKey, bFPBytes, 10*time.Second)
		if e != nil {
			errc <- e
			return
		}
		defer conn.Close()
		_ = p2p.WriteRole(conn.Channel, p2p.RoleCoSign)
		// The document is never reached: the dialer's user rejects the words first.
		_, e = p2p.Initiate(conn.Channel, base, aFPBytes, noVerifier{}, p2p.Roster{})
		errc <- e
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("setup: the spoken check never reached this side's screen")
		}
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if st.Verify != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	vr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/verify", "application/json",
		jsonBody(map[string]any{"confirmed": true}))
	if vr.StatusCode != http.StatusOK {
		t.Fatalf("setup: verify(true) status = %d", vr.StatusCode)
	}
	vr.Body.Close()

	select {
	case e := <-errc:
		if !errors.Is(e, p2p.ErrVerificationDeclined) {
			t.Fatalf("setup: the dialer returned %v, want its own ErrVerificationDeclined", e)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the dialer never returned")
	}

	until := time.Now().Add(10 * time.Second)
	for {
		var st sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &st)
		if st.Notice != nil && st.Notice.What == "words-not-confirmed" {
			if !strings.Contains(st.Notice.Detail, "did not match") {
				t.Errorf("the notice's detail %q does not say the words may not have matched", st.Notice.Detail)
			}
			if st.Armed {
				t.Error("the arm is still live after the other side rejected the words — a decided " +
					"outcome must end it, not re-accept")
			}
			return
		}
		if time.Now().After(until) {
			t.Fatalf("the user who confirmed the words was never told the other side did not "+
				"(notice %+v, armed %t) — the rejection was filed as a dropped channel", st.Notice, st.Armed)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestTheDialingHandlersLiftThePeersNoBeforeTheNetworkFallthrough is the source-order half, in
// `mitmreport_test.go`'s shape: the error->response mapping is inline in each handler, and a
// `p2p.ErrPeerDidNotConfirm` that reaches the fallthrough is a 502 "could not connect" (initiate)
// or "send did not complete" (send) — a network sentence about the one verdict that may mean a
// machine is sitting between the two parties.
func TestTheDialingHandlersLiftThePeersNoBeforeTheNetworkFallthrough(t *testing.T) {
	b, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, tc := range []struct{ fn, fallthroughCall string }{
		{"func (s *Server) runHopDial(", "s.writeConnectDiagnosis(w,"},
		{"func (s *Server) handleSessionSend(", `"send did not complete: "`},
	} {
		start := strings.Index(src, tc.fn)
		if start < 0 {
			t.Fatalf("%s not found — this guard has gone blind", tc.fn)
		}
		rest := src[start+len(tc.fn):]
		if end := strings.Index(rest, "\nfunc "); end >= 0 {
			rest = rest[:end]
		}
		lift := strings.Index(rest, "errors.Is(err, p2p.ErrPeerDidNotConfirm)")
		fall := strings.Index(rest, tc.fallthroughCall)
		if fall < 0 {
			t.Fatalf("%s no longer contains %s — this guard has gone blind", tc.fn, tc.fallthroughCall)
		}
		if lift < 0 || lift > fall {
			t.Errorf("%s does not lift p2p.ErrPeerDidNotConfirm before %s (%d, %d) — the other "+
				"side's rejection of the words is reported as a network failure", tc.fn,
				tc.fallthroughCall, lift, fall)
		}
	}
}
