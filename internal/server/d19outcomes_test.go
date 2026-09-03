package server

import (
	"os"
	"strings"
	"testing"
)

// TestEveryD19OutcomeSaysItsOwnThing — P06.S08's criterion, at the door where the words live.
//
// **The criterion is "each produces its own message, distinct from each other AND from D19's
// network causes", and the two halves of that set live in different languages.** D28's end-state
// words are in `web/app.js`; D19's summaries are here. A jsdom test can compare five sentences it
// was handed, which is a fact about its own fixture — so the cross-set claim is made HERE, where
// one side is the product and the other is read off the file that owns it.
//
// # The count is MEASURED, not asserted
//
// The slice's first pin said nine outcomes: five causes plus four end states. Driving
// `classifyD19` says otherwise — `causePeerRecordUnusable` has two summaries (a record refused and
// a record with no address yet) and `causeMappingDependent` has two (with and without a directly
// reachable IPv6 endpoint), so **seven** summaries come out of five causes. Enumerating instead of
// counting is the point: a sixth cause added tomorrow joins the set without anybody remembering to
// bump a literal.
func TestEveryD19OutcomeSaysItsOwnThing(t *testing.T) {
	// Inputs chosen to reach each documented branch of classifyD19, which is pure over them.
	cases := []struct {
		what string
		in   d19Inputs
	}{
		{"no DHT reply ever", d19Inputs{}},
		{"a record the gate refused", d19Inputs{dhtResponded: true, recordRefused: true}},
		{"a record with no address yet", d19Inputs{dhtResponded: true, recordEmpty: true}},
		{"the directory held nothing for them", d19Inputs{dhtResponded: true}},
		{"endpoint-dependent, no mapping", d19Inputs{dhtResponded: true, peerSeen: true, mappingDependent: true}},
		{"endpoint-dependent but v6-reachable", d19Inputs{
			dhtResponded: true, peerSeen: true, mappingDependent: true, v6Independent: true}},
		{"published, and it still did not connect", d19Inputs{dhtResponded: true, peerSeen: true}},
	}

	summaries := map[string]string{} // summary -> the case that produced it
	causes := map[string]bool{}
	for _, c := range cases {
		d := classifyD19(c.in)
		if d.summary == "" {
			t.Errorf("%s: classifyD19 returned no summary at all — D19's presentation pin is plain "+
				"language FIRST, and an empty one leaves the user the blank wait the whole "+
				"diagnosis exists to replace", c.what)
			continue
		}
		if prior, dup := summaries[d.summary]; dup {
			t.Errorf("%q and %q both produce %q. Each outcome must say its own thing — a user who "+
				"reads one sentence for two different network conditions is given advice for a "+
				"situation they are not in.", c.what, prior, d.summary)
			continue
		}
		summaries[d.summary] = c.what
		causes[causeName(d.cause)] = true
	}

	// STIMULUS: the cases really did reach different causes. Without this the distinctness above is
	// satisfied by seven variants of one branch, which is the shape a fixture drifts into.
	if len(causes) < 5 {
		t.Errorf("these inputs reached %d of D19's causes (%v) — the summaries may be distinct and "+
			"the classification untested", len(causes), causes)
	}
	if _, ok := causes["unknown"]; ok {
		t.Error("a case fell through to the `unknown` tag, so its cause is unclassified")
	}
	t.Logf("%d distinct summaries across %d causes", len(summaries), len(causes))

	// ── The cross-set half, and the only place it can be made ───────────────────────────────────
	//
	// D28's end-state words are rendered by `renderEndedCeremonies` in web/app.js. Read from the
	// file rather than copied here: a copy would agree with itself on the day it was written, which
	// is the two-implementations shape ADR-009 refuses. If the ternary is ever restructured this
	// scan finds nothing and fails LOUDLY below rather than passing over an empty set.
	src, err := os.ReadFile("../../web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	marker := "what.textContent = r.state === 'declined'"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("cannot find renderEndedCeremonies' end-state ternary in web/app.js — the " +
			"cross-set check below would compare against nothing, which is the vacuous green this " +
			"whole test is about")
	}
	// Bounded at the statement's own end, not at a byte count: an 800-byte window swept up the
	// comment above the ternary and scraped nine "words" where there are five. An over-inclusive
	// scrape only makes the collision check stricter, so it passed — and a check that passes for
	// the wrong reason is the thing this file is about.
	stmt := body[i:]
	if end := strings.Index(stmt, ";\n"); end > 0 {
		stmt = stmt[:end]
	}
	// ODD indices are what is inside the quotes — the statement starts outside one — and of those
	// the RENDERED words are the ones that begin with a capital. The state keys they are compared
	// against (`declined`, `completed`, …) are lowercase by construction. A filter on "contains a
	// space" instead scraped eight, because splitting on a quote also yields the ` ? ` between two
	// of them.
	var endWords []string
	for i, part := range strings.Split(stmt, "'") {
		if i%2 == 1 && part != "" && part[0] >= 'A' && part[0] <= 'Z' {
			endWords = append(endWords, part)
		}
	}
	if len(endWords) < 3 {
		t.Fatalf("scraped %d end-state word(s) from web/app.js (%v) — too few to be the four D28 "+
			"names plus the unknown case, so this comparison is not being made", len(endWords), endWords)
	}
	for _, w := range endWords {
		if who, clash := summaries[w]; clash {
			t.Errorf("the end-state word %q is also the D19 summary for %q. The criterion's own "+
				"example of failure is a screen that folds \"they declined\" into \"couldn't "+
				"establish a connection\".", w, who)
		}
	}
	t.Logf("%d end-state word(s) read from web/app.js, none colliding", len(endWords))
}
