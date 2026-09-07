package server

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P02.S03 — the signer sees where their block will land, on the surface they decide from.
//
// # The defect these drive
//
// `handleSessionQuote` answers with `p2p.NominalBlockRect()`, and its own doc says that is *"a size
// template, not a placement — the caller wants a rect of the right shape and must not care where it
// says it is"*; `web/app.js` consumes only its width and height. The real placement was computed
// server-side AFTER consent, at `p2p/session.go:1097`. So a party decided whether to sign, and only
// then did anything work out where their signature would go.

// TestTheBlockShownIsTheBlockStamped is the slice's second acceptance clause, and it is asserted as
// an EQUALITY between two calls rather than as a property of one.
//
// **The equality is the whole point and it is why this is not a comparison of two implementations.**
// `blockFor` and the stamp both call `p2p.PlacementFor`, ADR-009's one door for this question, and
// the inputs are the same values rather than equal ones: `Confirm` is handed `doc []byte`, which is
// `coSignExchange`'s `inbound`; the roster is `cer.l3Roster()` on the same `cer` the confirmer is
// built with (`p2p.Receive(` has one production call site and `sessionConfirmer{` one construction,
// and they are the same line). This test pins that the door is what `blockFor` reaches, so a future
// hand-copied placement fails here rather than drifting silently.
func TestTheBlockShownIsTheBlockStamped(t *testing.T) {
	doc, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := sign.GenerateIdentity("Signer")
	if err != nil {
		t.Fatal(err)
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}

	// The stamp side, called exactly as `coSignExchange` calls it: the received bytes, the arm's
	// roster, this machine's fingerprint. Outside a ceremony the roster is the zero value, which
	// is the manual co-sign path and which `PlacementFor` answers with `NextPlacement`.
	want, err := p2p.PlacementFor(doc, p2p.Roster{}, hex.EncodeToString(myFP))
	if err != nil {
		t.Fatal(err)
	}

	got := sessionConfirmer{}.blockFor(doc, myFP)
	if got == nil {
		t.Fatal("the consent view carries no block for a document whose placement computes fine, " +
			"so the signer decides without being shown where their signature lands")
	}
	if got.Page != want.Page || got.Rect != want.Rect {
		t.Errorf("the consent surface shows page %d %v and the signature is stamped at page %d %v "+
			"— a signer shown one location and given another is worse off than one shown none",
			got.Page, got.Rect, want.Page, want.Rect)
	}
	// SETUP, and it is the assertion that keeps the one above honest: a placement that was
	// hardcoded to the nominal rect would satisfy an equality against a nominal `want` too. The
	// nominal rect is a size template at index 0; a real placement on this document must not be it.
	if got.Rect == p2p.NominalBlockRect() && got.Page == 0 {
		t.Error("the reported block is the nominal size template rather than a placement — that " +
			"is the value this slice exists to stop sending")
	}
}

// TestAnUncomputableBlockLeavesTheRestOfTheSurface — STANDARDS §9, the rule `loadPendingPreview`
// already cites for the preview: a box that cannot be worked out degrades that box and not the
// screen where somebody decides whether to sign.
func TestAnUncomputableBlockLeavesTheRestOfTheSurface(t *testing.T) {
	myFP := []byte(strings.Repeat("\x01", 32))
	got := sessionConfirmer{}.blockFor([]byte("this is not a PDF"), myFP)
	if got != nil {
		t.Errorf("a document whose placement cannot be computed produced a block %v — a rect of "+
			"zeroes on page zero is a LOCATION, and this is the absence of one, so the field is "+
			"omitted rather than sent empty", got)
	}
}

// TestTheConsentViewSendsTheBlock is the routing half: the rule exists and the view calls it.
//
// It is a source scan because the behavioural reach needs a live session with a peer mid-handshake,
// which no tier below 4 arranges — the same limit `recitalFor`'s call-site scan records one field
// over. A scan proves the line is present; `TestTheBlockShownIsTheBlockStamped` proves it is right.
func TestTheConsentViewSendsTheBlock(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)
	body := funcBodyFrom(code, strings.Index(code, "func (sc sessionConfirmer) Confirm("))
	if !strings.Contains(body, "view.Block = sc.blockFor(") {
		t.Error("the consent view no longer carries the signer's own block. blockFor can be " +
			"perfectly correct and reach nobody: the field is `omitempty`, so a view that stops " +
			"setting it is indistinguishable at the client from a document with no placement")
	}
}
