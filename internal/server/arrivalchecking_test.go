package server

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// watchChecking samples the session's published status until stopped, and reports whether
// `checking` was ever true — what a window polling `GET /api/session/status` could have seen.
func watchChecking(se *session) (seen *atomic.Bool, stop func()) {
	seen = &atomic.Bool{}
	quit, gone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(gone)
		for {
			select {
			case <-quit:
				return
			default:
			}
			if se.status().Checking {
				seen.Store(true)
			}
		}
	}()
	return seen, func() { close(quit); <-gone }
}

// TestTheStatusSaysTheDocumentIsBeingCheckedForTheAnchorAndNoLonger is /pending 359's second half.
//
// The arrival content anchor hashes the whole document against its record and stays on the request
// path by decision. It runs before either party has been shown anything — the initiating side has
// not dialled, the receiving side has no consent screen — so for its whole duration the window
// showed a wait with no words. Both windows poll the session status across that stretch; this is
// the field they read.
//
// **Four arms, because "cleared on every exit" is the half that is easy to get wrong**: the anchor
// that passes, the anchor that REFUSES (a different document under the same record), an arrival
// refused before the anchor is reached, and a signed arrival, for which there is no anchor and the
// status must never say there is.
func TestTheStatusSaysTheDocumentIsBeingCheckedForTheAnchorAndNoLonger(t *testing.T) {
	cert, key, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	fpb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	convFP, signer := hex.EncodeToString(fpb), strings.Repeat("2b", 32)
	// Many pages, so the anchor lasts long enough to be sampled on any machine.
	pages := make([]string, 400)
	for i := range pages {
		pages[i] = fmt.Sprintf("clause %d of the lease, set out in full", i)
	}
	base, err := testpdf.Text(pages...)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ceremony.Convene(base, ceremony.ConveneRequest{
		Roster: []ceremony.Party{
			{Fingerprint: convFP, Label: "Convener", Signs: false},
			{Fingerprint: signer, Label: "B", Signs: true},
		},
		Intent: "We agree", Expires: time.Now().Add(48 * time.Hour),
		HopBudget: ceremonyHopBudget(), DeliveryBudget: ceremonyDeliveryLegBudget(),
	}, cert, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cer := &ceremonyID{inv: ceremony.Invitation{
		Version: ceremony.InvitationVersion, ID: out.Record.ID, Roster: out.Record.Roster,
		ConvenerFingerprint: convFP, Intent: out.Record.Intent, RosterHash: rosterHashHex(out.Record),
	}}
	now := time.Now()

	other := append([]string{"a different lease, on worse terms"}, pages[1:]...)
	otherPDF, err := testpdf.Text(other...)
	if err != nil {
		t.Fatal(err)
	}
	decoy, err := ceremony.Embed(otherPDF, out.Record)
	if err != nil {
		t.Fatal(err)
	}
	place, err := p2p.NextPlacement(out.Document)
	if err != nil {
		t.Fatal(err)
	}
	sCert, sKey, err := sign.GenerateIdentity("B")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := p2p.Contribute(out.Document, sCert, sKey,
		p2p.Attestation{Signer: "B", When: now}, onePixelPNG(t), place)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := testpdf.Text("an unrelated page")
	if err != nil {
		t.Fatal(err)
	}

	for _, arm := range []struct {
		name    string
		doc     []byte
		refused bool
		anchor  bool
	}{
		{"the convened document", out.Document, false, true},
		{"a different document under the same record", decoy, true, true},
		{"a document with no record, refused before the anchor", plain, true, false},
		{"a signed arrival, which has no anchor", signed, false, false},
	} {
		se := &session{}
		if se.status().Checking {
			t.Fatalf("%s: setup: a fresh session already says a document is being checked", arm.name)
		}
		seen, stop := watchChecking(se)
		err := cer.checkArrival(arm.doc, now, se)
		stop()
		if (err != nil) != arm.refused {
			t.Fatalf("%s: setup: checkArrival answered %v, so this arm is not the exit it names", arm.name, err)
		}
		if seen.Load() != arm.anchor {
			if arm.anchor {
				t.Errorf("%s: the status never said the document was being checked while the "+
					"anchor ran — the window shows a wait with no words for the whole hash", arm.name)
			} else {
				t.Errorf("%s: the status said a document was being checked and no anchor runs "+
					"for this arrival", arm.name)
			}
		}
		if se.status().Checking {
			t.Errorf("%s: the status still says the document is being checked after the gate "+
				"answered (%v) — the window would say so for good", arm.name, err)
		}
	}
}

// TestTwoAnchorsAtOnceAreOneCheckingUntilBothEnd — the count, and the nil receiver.
//
// One process can run two anchors at once (a hop being dialled while a delivery arm receives). With
// a flag the first to finish clears the other's words; with a count the status is true until both
// are done.
func TestTwoAnchorsAtOnceAreOneCheckingUntilBothEnd(t *testing.T) {
	se := &session{}
	first := se.checkingDocument()
	second := se.checkingDocument()
	first()
	if !se.status().Checking {
		t.Error("one of two anchors finished and the status stopped saying a document is being " +
			"checked while the other is still running")
	}
	second()
	if se.status().Checking {
		t.Error("both anchors finished and the status still says a document is being checked")
	}
	var nobody *session
	nobody.checkingDocument()() // must not panic: a gate driven with no session tells nobody
}

// TestEveryArrivalCheckTellsTheSession holds the three production callers to the argument.
//
// `checkArrival` takes the session as a parameter and nil is legal — which is how a fourth caller
// would silently bring back the wordless wait. Every non-test call names a session.
func TestEveryArrivalCheckTellsTheSession(t *testing.T) {
	call := regexp.MustCompile(`\.checkArrival\(([^\n]*)\)`)
	calls := 0
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			m := call.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			calls++
			if !regexp.MustCompile(`\.sess\b`).MatchString(m[1]) {
				t.Errorf("%s: %s — this arrival check names no session, so the window says "+
					"nothing while the document is hashed", name, strings.TrimSpace(line))
			}
		}
	}
	if calls < 3 {
		t.Fatalf("setup: found %d calls to checkArrival, want the three doors (the dial, the "+
			"consent gate, the convener's own turn) — this guard walked nothing", calls)
	}
}
