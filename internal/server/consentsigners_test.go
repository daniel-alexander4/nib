package server

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P07.S07c: the consent screen is shown every party who has already signed (D27 item 3, C09).
//
// # What it showed instead, and why that is the wrong person
//
// `pendingView` named exactly one identity: `peer.Signer`, who the server saw connect. In an
// ordinary two-party co-sign that is the only other party and the screen is complete. In a
// ceremony it is neither complete nor necessarily a signer — under a carry route the connected
// peer is a NON-SIGNING convener, and at hop 6 it is whoever dialled rather than the five
// parties whose signatures the user is about to join.
//
// So a party asked to sign sixth was told about one person while holding a document bearing five
// signatures. D27's item 3 is that they are equipped to make the decision; that is the decision.

// threeSigned builds a document carrying three real signatures from three distinct identities —
// the count the acceptance clause names.
func threeSigned(t *testing.T) []byte {
	t.Helper()
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := p2p.PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		certPEM, keyPEM, err := sign.GenerateIdentity("Party Label " + string(rune('A'+i)))
		if err != nil {
			t.Fatal(err)
		}
		place, err := p2p.NextPlacement(doc)
		if err != nil {
			t.Fatal(err)
		}
		att := p2p.Attestation{Signer: "Party Label " + string(rune('A'+i)), When: time.Now()}
		doc, err = p2p.Contribute(doc, certPEM, keyPEM, att, nil, place)
		if err != nil {
			t.Fatalf("party %d could not contribute: %v", i, err)
		}
	}
	return doc
}

func TestTheConsentScreenIsToldEveryPartyThatHasAlreadySigned(t *testing.T) {
	doc := threeSigned(t)

	// SETUP: the document really carries three signatures, or "three signers were listed" is a
	// claim about a fixture that never signed.
	if n := len(sign.Verify(doc).Signers); n != 3 {
		t.Fatalf("setup: the fixture carries %d signature(s), want 3", n)
	}
	if _, err := pdfops.PageCount(doc); err != nil {
		t.Fatalf("setup: the three-signature fixture does not parse: %v", err)
	}

	got := signersSoFar(doc)
	if len(got) != 3 {
		t.Fatalf("the consent screen is told about %d signer(s) on a document carrying 3. A "+
			"party asked to sign is shown who they are joining, and a short list is a document "+
			"that looks less committed than it is", len(got))
	}
	names := map[string]bool{}
	for i, s := range got {
		if s.Fingerprint == "" {
			t.Errorf("signer %d is listed with no fingerprint, so the screen names a person and "+
				"offers nothing to check them against", i)
		}
		if !s.Valid {
			t.Errorf("signer %d is reported invalid on a fixture whose signatures all verify", i)
		}
		if !strings.HasPrefix(s.Signer, "Party Label ") {
			t.Errorf("signer %d is named %q; the signature says %q", i, s.Signer,
				"Party Label …")
		}
		names[s.Signer] = true
	}
	// DISTINCT, which is the assertion three copies of one name would fail. Before P07.S07a
	// every signature reported "Nib User", and a screen listing that three times tells the user
	// nothing they did not already know.
	if len(names) != 3 {
		t.Errorf("three signatures produced %d distinct name(s): %v — a consent screen listing "+
			"one name three times is not a list of the parties", len(names), names)
	}
}

// TestTheConsentGateRoutesThroughTheSignerList is the ADR-009 half, and the red proof is what
// made it necessary.
//
// The two tests above call `signersSoFar` directly. Deleting `Signers: signersSoFar(doc)` from
// the view `Confirm` builds left both of them GREEN — they prove the function works and say
// nothing about whether the consent screen is ever handed its result, which is the same shape as
// the L3 gate's own guard and the reason that one asserts routing rather than text.
func TestTheConsentGateRoutesThroughTheSignerList(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	i := strings.Index(code, "func (sc sessionConfirmer) Confirm(")
	if i < 0 {
		t.Fatal("cannot find sessionConfirmer.Confirm — this guard is reading the wrong thing")
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("could not brace-match Confirm's body")
	}
	if !strings.Contains(body, "signersSoFar(") {
		t.Error("Confirm builds the consent view without reading who has already signed. The " +
			"screen then names one identity — whoever connected — which under a carry route is " +
			"a non-signing convener and at hop 6 is not any of the five parties whose " +
			"signatures the user is about to join (D27 item 3).")
	}
}

// TestAnUnsignedDocumentSaysSoRatherThanListingNobody is the third state, and it is the one an
// absent element collapses into the second.
//
// "Nobody has signed this yet" and "Nib did not look" must not render the same. The first hop of
// every ceremony takes this branch, and so does every one-way transfer, so it is the ordinary
// case rather than an error — a screen that simply omits the section there teaches the user that
// its absence means nothing.
func TestAnUnsignedDocumentSaysSoRatherThanListingNobody(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	got := signersSoFar(base)
	if len(got) != 0 {
		t.Errorf("an unsigned document reports %d signer(s)", len(got))
	}
	// The field is present and empty rather than absent, so the client can tell the two states
	// apart. `omitempty` here would make an unsigned document and an old server look identical.
	b, err := json.Marshal(pendingView{Signers: got})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"signers"`) {
		t.Errorf("pendingView omits `signers` when there are none: %s\n\nThe consent screen "+
			"renders a sentence for the empty case, and it cannot tell an empty list from a "+
			"server that never sent one", b)
	}
}

// TestTheConsentViewPublishesNoUnreadPeerFields is /pending 252's decision, made enforceable.
//
// **The decision (2026-09-04): the connected peer's own signature validity is intentionally not
// shown on the consent card, because the card already answers the question better.** It lists
// every signature already on the document and marks an invalid one rather than dropping it
// (`renderConsentSigners`), and it renders "Nobody yet — you would be the first to sign this
// document." when there are none. That reports the DOCUMENT's state; `Valid` reported one boolean
// about whoever dialled, and under a carry route the dialler is not a signer at all.
//
// **So the fields were deleted rather than parked, and this is the guard that keeps them gone.**
// Parking them was tried and is wrong twice over: `published.test.mjs` already carries both in
// `COINCIDENTAL` and refuses a second park by assertion, and the Go scan
// (`TestEveryPublishedObservableHasANamedReader`) has no entry for `pendingView` at all — it
// matches on a bare name, which `pendingSigner.Valid` and `attestationView.acceptedPeer` in
// `web/app.js` satisfy. Both scans were laundering these two fields, and only deletion reaches
// both.
//
// The assertion is over the marshalled JSON rather than the struct, because the property is about
// what crosses the wire: a field re-added and left unread is exactly what neither scan can see.
func TestTheConsentViewPublishesNoUnreadPeerFields(t *testing.T) {
	b, err := json.Marshal(pendingView{Signer: "Alice", Fingerprint: "aa", Reason: "I agree"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"valid"`, `"acceptedPeer"`} {
		if strings.Contains(string(b), key) {
			t.Errorf("the consent view publishes %s again: %s\n\nNothing reads it — not `showConsent`, "+
				"not any Go caller — and both reader scans launder it on a coincidental name match, so "+
				"re-adding it re-opens /pending 252 silently. If the consent card should state the "+
				"connected peer's own signature validity, render it and delete this guard; do not "+
				"publish it unread.", key, b)
		}
	}
}

// --- P02.S01: the consent view carries the ceremony's recital ---------------

// TestTheConsentViewOmitsTheRecitalOutsideACeremony pins the shape, because the client branches on
// the field's PRESENCE. A plain two-party co-sign has no record and therefore no recital, and a
// `recital: ""` on the wire would make the client set the agreement box to an empty string — worse
// than the generic sentence it replaced.
func TestTheConsentViewOmitsTheRecitalOutsideACeremony(t *testing.T) {
	b, err := json.Marshal(pendingView{Signer: "Alice", Fingerprint: "aa", Reason: "I agree"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "recital") {
		t.Errorf("pendingView carries `recital` with no ceremony: %s\n\nThe consent screen branches "+
			"on the field being absent; an empty one blanks the signer's agreement statement.", b)
	}
	// The stimulus: the field must appear when it HAS a value, or the assertion above is satisfied
	// by a field that was never serialised at all.
	b2, err := json.Marshal(pendingView{Signer: "Alice", Recital: "We agree to the lease"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b2), "We agree to the lease") {
		t.Errorf("pendingView drops a recital it was given: %s", b2)
	}
}

// TestTheRecitalComesFromTheArmsCeremony drives the rule itself, which is a pure function of the
// arm — no session, no socket, no disk.
//
// **It replaced a source scan.** The rule was two lines at a call site reachable only with a live
// ceremony session in flight, so the only instrument available was a grep over the file. Extracting
// `recitalFor` turned it into something a test can call, which is strictly better: a scan proves a
// line exists, and this proves the line is right.
func TestTheRecitalComesFromTheArmsCeremony(t *testing.T) {
	// Outside a ceremony there is nothing to take a recital from, and the field must stay empty —
	// the client branches on its absence, and an empty string would blank the signer's box.
	if got := recitalFor(nil); got != "" {
		t.Errorf("recitalFor(nil) = %q, want \"\" — a plain co-sign has no ceremony", got)
	}
	// The stimulus: with a ceremony it must actually return the recital, or the assertion above is
	// satisfied by a function that always returns "".
	cer := &ceremonyID{inv: ceremony.Invitation{Intent: "We agree to the Fitzroy Street lease"}}
	if got := recitalFor(cer); got != "We agree to the Fitzroy Street lease" {
		t.Errorf("recitalFor(ceremony) = %q, want the record's recital — the signature carries "+
			"whatever this returns", got)
	}
}

// TestTheConsentViewCallsTheRecitalRule pins the WIRING that the rule test cannot see: a correct
// `recitalFor` that nothing calls leaves the field empty in every ceremony, and marshals exactly
// like one that works. Kept as a scan because the call site needs a live session to reach.
func TestTheConsentViewCallsTheRecitalRule(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "view.Recital = recitalFor(sc.cer)") {
		t.Error("the consent view does not call recitalFor — the field is published, the client " +
			"reads it, and it would be empty in every ceremony")
	}
}
