package server

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestAnArrivalCannotBePumped is what keeps `addDoc`'s arrival exemption honest (/pending 337).
//
// # Why a guard and not a comment
//
// `openArrival` deliberately takes the UNCAPPED door, past both of ADR-005's bounds, and ADR-009
// requires a deliberate exemption to be named at the site. It was — and the reason given was that
// the bytes "have no other home, since the session path installs it and nothing writes it to disk".
// P08.S02 made that half false: a ceremony arrival's bytes are written to the mirror before the
// frame. The exemption survived on a premise that had quietly stopped applying to one of its two
// classes, and nothing noticed for four slices.
//
// The reason that actually carries it is a STRUCTURAL property of the call sites: an arm admits at
// most one document, and only one the user accepted. That is a property code can lose, so it is
// asserted here rather than described. If this guard goes red, the choice is to restore the
// property or to cap the door — not to reword the comment.
//
// It is a source scan for the same reason `TestTheReceivedWriteHasOneDoor` is: the defect it
// prevents is a NEW call site added without the guard, and driving the two known ones says nothing
// about a third.
func TestAnArrivalCannotBePumped(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))

	// Every call site, found rather than listed.
	var sites []int
	for _, m := range regexp.MustCompile(`s\.openArrival\(`).FindAllStringIndex(code, -1) {
		sites = append(sites, m[0])
	}
	if len(sites) < 2 {
		t.Fatalf("found %d call site(s) for openArrival; there are at least two (the TCP arm loop "+
			"and the ceremony receive loop), so this scan is looking for a spelling that no "+
			"longer appears and a clean result would mean nothing", len(sites))
	}

	for _, at := range sites {
		// The guard is the statement the call sits inside. Read the 400 bytes before it, which
		// comfortably covers `if final != nil && !opened {`.
		lo := at - 400
		if lo < 0 {
			lo = 0
		}
		window := code[lo:at]
		if !strings.Contains(window, "!opened") {
			t.Error("an openArrival call site is not guarded by `!opened`, so one arm can install " +
				"MORE THAN ONE document. That is the property `addDoc`'s arrival exemption now " +
				"rests on: the door is uncapped because every increment costs a deliberate arm " +
				"and a deliberate consent, so a peer cannot drive it. Lose the guard and the " +
				"exemption is a hole past both of ADR-005's bounds.")
		}
		if !strings.Contains(window, "final != nil") {
			t.Error("an openArrival call site does not test `final != nil` first, so an arrival " +
				"is installed on a session that produced no co-signed document — which means " +
				"the consent gate was not the thing that admitted it")
		}
	}

	// AND the other half of the exemption's argument: the call really is downstream of consent.
	// `serveOneSession` is what returns `final`, and it is where Confirm runs.
	if !strings.Contains(code, "serveOneSession(") {
		t.Fatal("serveOneSession is gone; `final` no longer comes from the path that asks the " +
			"user, and the exemption's second clause is unverified")
	}

	// The exemption is NAMED at the site, per ADR-009 — asserted against the UNSTRIPPED source,
	// because it is a comment.
	whole := string(src)
	i := strings.Index(whole, "func (s *Server) openArrival(")
	if i < 0 {
		t.Fatal("cannot find openArrival")
	}
	body := funcBodyFrom(whole, i)
	if body == "" {
		t.Fatal("openArrival: the brace matcher read an empty body")
	}
	if !strings.Contains(body, "addDoc, not addDocCapped") {
		t.Error("openArrival no longer names its exemption from the open-document cap. ADR-009 " +
			"requires a deliberate exemption to be named AT THE SITE, and this one is four " +
			"slices' worth of argument that a reader arriving at `s.addDoc(` cannot reconstruct")
	}
}
