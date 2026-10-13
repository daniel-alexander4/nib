package p2p

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// refusedWithin waits for a listener's count to reach want. The refusal is recorded on the
// handshake's own goroutine, so the dialler can learn of it a moment before the count moves.
func refusedWithin(t *testing.T, r *Refusals, want int) RefusalReport {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		rep := r.Report()
		if rep.NotPinned >= want || time.Now().After(deadline) {
			return rep
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// dialOrFirstRead is where a dialler learns the far side refused it: the dial's own error, or — the
// client's TLS 1.3 handshake finishes before the server has read its certificate — the first read.
func dialOrFirstRead(conn *Conn, derr error) error {
	if derr != nil {
		return derr
	}
	defer conn.Close()
	_ = conn.Stream.SetDeadline(time.Now().Add(10 * time.Second))
	_, err := conn.Stream.Read(make([]byte, 1))
	return err
}

// TestARefusalIsNamedOnBothSidesOfBothTransports — /pending 669 #3, ADR-156.
//
// A listener that turns a peer away at the pinned-peer check said nothing to anybody on either
// transport, and the dialler it turned away saw a bare transport error. Three facts, each over TCP
// and QUIC: the LISTENER counts the refusal; the DIALLER it refused can tell it was refused; and a
// dialler whose OWN pin failed can tell that apart from being refused.
func TestARefusalIsNamedOnBothSidesOfBothTransports(t *testing.T) {
	eachTransport(t, func(t *testing.T, tr transport) {
		aCert, aKey := newIdentity(t)
		bCert, bKey := newIdentity(t)
		cCert, cKey := newIdentity(t)
		aFP, bFP, cFP := fingerprint(t, aCert), fingerprint(t, bCert), fingerprint(t, cCert)

		// B listens for A and nobody else.
		ln, err := tr.listen("127.0.0.1:0", bCert, bKey, aFP)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			for {
				c, e := ln.Accept()
				if e != nil {
					return
				}
				go c.Close()
			}
		}()
		if ln.Refusals() == nil {
			t.Fatal("the listener carries no refusal count — its constructor did not come through listenTLS")
		}
		// STIMULUS: the pinned peer gets in and is not counted, so the count below is the stranger's.
		warm, werr := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, bFP, 10*time.Second)
		if werr != nil {
			t.Fatalf("stimulus: the pinned peer cannot connect: %v", werr)
		}
		warm.Close()
		if rep := ln.Refusals().Report(); rep.Any() {
			t.Fatalf("the pinned peer's own handshake was counted as a refusal: %+v", rep)
		}

		// C — a stranger who pins B correctly — dials. B refuses it.
		conn, derr := tr.dial(context.Background(), ln.Addr().String(), cCert, cKey, bFP, 10*time.Second)
		derr = dialOrFirstRead(conn, derr)
		if !RefusedByPeer(derr) {
			t.Errorf("the dialler the listener refused cannot tell it was refused: %v", derr)
		}
		if errors.Is(derr, ErrPeerNotPinned) {
			t.Errorf("being refused by the far side reads as this side's own pin failing: %v", derr)
		}
		rep := refusedWithin(t, ln.Refusals(), 1)
		if rep.NotPinned != 1 || rep.Clock != 0 {
			t.Errorf("after one stranger the listener counts %d not-pinned and %d clock refusals, want 1 and 0",
				rep.NotPinned, rep.Clock)
		}
		if rep.Last.IsZero() {
			t.Error("the refusal carries no time")
		}

		// A dials B's address but has pinned C: A's OWN check refuses what answered.
		_, oerr := tr.dial(context.Background(), ln.Addr().String(), aCert, aKey, cFP, 10*time.Second)
		if !errors.Is(oerr, ErrPeerNotPinned) {
			t.Errorf("a dialler whose own pin failed cannot name it: %v", oerr)
		}
		if RefusedByPeer(oerr) {
			t.Errorf("this side's own pin failing reads as the far side refusing: %v", oerr)
		}
	})
}

// TestTheHandshakeListenerCountsItsRefusals is the same fact for the one listener that does not embed
// listenerCore — the ceremony arm's racing accept.
func TestTheHandshakeListenerCountsItsRefusals(t *testing.T) {
	aCert, _ := newIdentity(t)
	bCert, bKey := newIdentity(t)
	cCert, cKey := newIdentity(t)
	e, err := NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	hl, err := QUICListenHandshakeOn(e, bCert, bKey, fingerprint(t, aCert))
	if err != nil {
		t.Fatal(err)
	}
	defer hl.Close()
	conn, derr := QUICDial(context.Background(), hl.Addr().String(), cCert, cKey, fingerprint(t, bCert), 10*time.Second)
	derr = dialOrFirstRead(conn, derr)
	if !RefusedByPeer(derr) {
		t.Errorf("the refused dialler sees %v", derr)
	}
	if rep := refusedWithin(t, hl.Refusals(), 1); rep.NotPinned != 1 {
		t.Errorf("the racing accept counted %d refusals of a stranger, want 1", rep.NotPinned)
	}
}

// TestAClockRefusalIsCountedApartAndAStrangerCannotReplaceIt drives the door itself, because a
// listener only sees a skewed leaf from a peer whose own check passed — two clocks, which one
// process does not have.
func TestAClockRefusalIsCountedApartAndAStrangerCannotReplaceIt(t *testing.T) {
	realNow := time.Now
	aCert, aKey := newIdentity(t)
	bCert, bKey := newIdentity(t)
	cCert, cKey := newIdentity(t)
	cfg, r, err := listenTLS(bCert, bKey, fingerprint(t, aCert))
	if err != nil {
		t.Fatal(err)
	}
	pinned, stranger := rawChain(t, aCert, aKey), rawChain(t, cCert, cKey)

	// STIMULUS: at the true time the pinned chain passes and nothing is counted.
	if verr := cfg.VerifyPeerCertificate(pinned, nil); verr != nil {
		t.Fatalf("stimulus: the pinned chain is refused at the true time: %v", verr)
	}
	if r.Report().Any() {
		t.Fatal("a chain that passed was counted")
	}

	at := func(shift time.Duration, chain [][]byte) error {
		defer setClock(func() time.Time { return realNow().Add(shift) })()
		return cfg.VerifyPeerCertificate(chain, nil)
	}
	if verr := at(-2*time.Hour, pinned); verr == nil {
		t.Fatal("a certificate dated two hours ahead was accepted")
	}
	rep := r.Report()
	if rep.Clock != 1 || rep.NotPinned != 0 || rep.Skew == nil || !rep.Skew.Behind {
		t.Fatalf("a clock two hours behind reads %+v (skew %v), want one clock refusal reading behind", rep, rep.Skew)
	}
	// An EXPIRED leaf is one a stranger can replay; it is counted and does not replace the reading
	// only the pinned peer could have caused.
	if verr := at(48*time.Hour, pinned); verr == nil {
		t.Fatal("an expired certificate was accepted")
	}
	rep = r.Report()
	if rep.Clock != 2 || !rep.Skew.Behind {
		t.Errorf("after an expired leaf: %d clock refusals, reading behind=%v — want 2, still behind", rep.Clock, rep.Skew.Behind)
	}
	// A stranger is the other count, and leaves the clock reading alone.
	if verr := at(0, stranger); !errors.Is(verr, ErrPeerNotPinned) {
		t.Fatalf("a stranger's chain gave %v", verr)
	}
	rep = r.Report()
	if rep.NotPinned != 1 || rep.Clock != 2 || rep.Skew == nil {
		t.Errorf("after a stranger: %+v, want one not-pinned beside two clock refusals", rep)
	}
	// And a stranger cannot reach the clock count at all: the pin is checked first.
	if verr := at(48*time.Hour, stranger); !errors.Is(verr, ErrPeerNotPinned) {
		t.Errorf("a stranger under a wrong clock gave %v, want the pin refusal", verr)
	}
	if got := r.Report().Clock; got != 2 {
		t.Errorf("a stranger moved the clock count to %d", got)
	}
}

// TestEveryListenerCountsItsRefusals holds the door (ADR-009): a listening config is built by
// listenTLS and nowhere else, and every constructor that hands back a listener keeps the count.
func TestEveryListenerCountsItsRefusals(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	serverCfg := regexp.MustCompile(`SessionTLS\([^)]*,\s*true\)`)
	ctor := regexp.MustCompile(`(?m)^func ([A-Z]\w+)\([^)]*\) \((Listener|\*HandshakeListener), error\) \{`)
	constructors, doors := 0, 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		src := string(b)
		// Comments name the call too; only code counts.
		var code strings.Builder
		for _, line := range strings.Split(src, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "//") {
				code.WriteString(line + "\n")
			}
		}
		n := len(serverCfg.FindAllString(code.String(), -1))
		if f == "refusal.go" {
			doors += n
		} else if n > 0 {
			t.Errorf("%s builds a listening TLS config for itself (%d site(s)). A listener built past "+
				"listenTLS refuses peers and counts none of them, so its arm can never say why "+
				"nobody got in", f, n)
		}
		for _, m := range ctor.FindAllStringSubmatchIndex(src, -1) {
			constructors++
			name := src[m[2]:m[3]]
			body := src[m[1]:]
			if end := strings.Index(body, "\n}\n"); end >= 0 {
				body = body[:end]
			}
			if !strings.Contains(body, "listenTLS(") || !strings.Contains(body, "refused") {
				t.Errorf("%s hands back a listener without the refusal count listenTLS gave it", name)
			}
		}
	}
	// STIMULUS: the scan saw the door and the four constructors, so a clean result is about them.
	if doors != 1 {
		t.Errorf("refusal.go builds %d listening configs, want exactly the one in listenTLS", doors)
	}
	if constructors != 4 {
		t.Errorf("found %d listener constructors, want 4 (Listen, QUICListen, QUICListenOn, "+
			"QUICListenHandshakeOn) — a new one must come through listenTLS and be counted here", constructors)
	}
}
