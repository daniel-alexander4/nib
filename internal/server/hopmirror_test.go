package server

import (
	"os"
	"strings"
	"testing"
)

// TestTheHopRouteDoesNotAuthoriseFromTheMirror — /pending 438, closed.
//
// # What the item feared, and why it is not what shipped
//
// 438 was filed as a precondition on an UNBUILT route: `POST /api/ceremony/hop` "would be the first
// route to close a read-write loop on the mirror" — reading the convener's mirror, resolving whose
// turn it is from `ContributionProgress` over those bytes, dialling, and writing the result back.
// Since `WriteMirror` refuses nothing about descent, a one-off rewind (/pending 437) would have
// become a durable rollback.
//
// The route then shipped at v1.128.57–.60, and **its own header cites the deepdive that filed 438
// as the reason it does not do that**: "the hop reads the document from the OPEN TAB under
// `X-Nib-Doc`, exactly as `/api/session/initiate` does, and reads the mirror only for the roster and
// the invitation — which are covered by `record.json`'s signature, verified inside `ReadMirror`."
//
// So the loop is not closed: `hopTarget` DISCARDS `ReadMirror`'s document (`rec, _, err := …`) and
// returns `doc.data`, and no handler in that file writes a mirror.
//
// # Why the closure gets a guard rather than a sentence
//
// The item is closed on a property of the code, and a property nothing checks is one a later change
// undoes silently. Switching `ContributionProgress(doc.data, …)` to the mirror's bytes is a
// one-token edit that would make 438 live again with nothing to notice — and it would look like a
// simplification, because `ReadMirror` is already being called two lines above.
//
// **This does NOT assert the mirror is trustworthy.** It is not: /pending 437 measures a rewind
// reading back clean, and ADR-013 calls that a decided limitation. What it asserts is that this
// route does not depend on it.
func TestTheHopRouteDoesNotAuthoriseFromTheMirror(t *testing.T) {
	src, err := os.ReadFile("ceremonyhop.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)

	body := funcBodyFrom(s, strings.Index(s, "func (s *Server) hopTarget("))
	// The stimulus floor. An empty body contains neither the string this test wants nor the one it
	// refuses, so a scan that lost its function would report clean on both clauses.
	if !strings.Contains(body, "ceremony.ReadMirror(") {
		t.Fatal("hopTarget's body could not be read, or it no longer reads the mirror at all — " +
			"every clause below would pass over nothing")
	}

	// The turn is computed over the OPEN DOCUMENT. This is the clause 438 turns on.
	if !strings.Contains(body, "p2p.ContributionProgress(doc.data,") {
		t.Error("the hop's turn is no longer computed over the open document's bytes. If it now " +
			"reads the mirror's document instead, /pending 438 is live again: the mirror's " +
			"`DocHash` check runs only while a document is UNSIGNED and its sidecar is a damage " +
			"detector rather than an access control, so a rewind reads back clean (/pending 437) " +
			"— and this route would be the first to pick a party and dial them from it")
	}
	// And the mirror's document is DISCARDED, so it cannot be used by accident further down.
	if !strings.Contains(body, "rec, _, err := ceremony.ReadMirror(") {
		t.Error("hopTarget now keeps the mirror's document. It read the mirror for the roster and " +
			"the invitation only — both covered by record.json's signature, which ReadMirror " +
			"verifies — and discarded the bytes. Keeping them is how the bytes come to be used")
	}

	// Neither handler writes a mirror, so there is no read-write loop to close.
	for _, fn := range []string{"handleCeremonyHop", "handleCeremonyHopQuote"} {
		b := funcBodyFrom(s, strings.Index(s, "func (s *Server) "+fn+"("))
		if !strings.Contains(b, "hopTarget(") {
			t.Fatalf("%s's body could not be read — the clause below would pass over nothing", fn)
		}
		if strings.Contains(b, "WriteMirror(") {
			t.Errorf("%s now writes the mirror. Together with a read it would be the read-write "+
				"loop /pending 438 was filed about, and `WriteMirror` refuses nothing about "+
				"descent — so a rewind stops being a one-off and becomes canonical. 437's "+
				"write-door check is the precondition for that, and it has not landed", fn)
		}
	}

	// The GET sibling is read-only, and that is enforced elsewhere rather than here — said so the
	// next reader does not add a duplicate. `getreadonly_test.go` lists `WriteMirror` among the
	// writes no GET handler may reach, and `handleCeremonyHopQuote` is a GET.
	if !strings.Contains(s, "func (s *Server) handleCeremonyHopQuote(") {
		t.Error("handleCeremonyHopQuote is gone or renamed, so the GET half of this route is no " +
			"longer the one getreadonly_test.go polices")
	}
}
