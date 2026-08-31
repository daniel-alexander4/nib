package server

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// convenedBytes builds a real convened document — the population the freeze is about.
func convenedBytes(t *testing.T) []byte {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	fp, ferr := sign.Fingerprint(cert)
	if ferr != nil {
		t.Fatal(ferr)
	}
	base, perr := testpdf.Text("the lease")
	if perr != nil {
		t.Fatal(perr)
	}
	out, cerr := ceremony.Convene(base, ceremony.ConveneRequest{
		Roster: []ceremony.Party{
			{Fingerprint: hex.EncodeToString(fp), Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("11", 32), Label: "A", Signs: true},
		},
		Intent:         "We agree to co-sign the lease",
		Expires:        time.Now().Add(48 * time.Hour),
		HopBudget:      ceremonyHopBudget(),
		DeliveryBudget: ceremonyDeliveryLegBudget(),
		ConvenerSigns:  true,
	}, cert, key, time.Now())
	if cerr != nil {
		t.Fatal(cerr)
	}
	return out.Document
}

// TestAConvenedDocumentRefusesMutation — D29's freeze, driven through a REAL edit rather
// than asserted on a flag.
//
// The client-side stopgap does not cover this and never did: `confirmSignatureLoss` is
// `!isSigned() || confirm(...)`, `isSigned()` reads `state !== 'unsigned'`, and a convened
// document IS unsigned — a record is an attachment, not a signature. So on exactly these
// documents the confirm short-circuits to true and no dialog appears. D29 already says a
// client confirm is not a freeze; measured, it is weaker than that.
func TestAConvenedDocumentRefusesMutation(t *testing.T) {
	doc := convenedBytes(t)
	// Stimulus: it really is convened, or "the commit was refused" below is equally true of
	// a guard that refuses everything.
	if _, err := ceremony.Extract(doc); err != nil {
		t.Fatalf("setup: the fixture carries no ceremony record (%v)", err)
	}
	plain, err := testpdf.Text("an ordinary page")
	if err != nil {
		t.Fatal(err)
	}
	if _, xerr := ceremony.Extract(plain); !errors.Is(xerr, ceremony.ErrNoRecord) {
		t.Fatalf("setup: the control document carries a record (%v)", xerr)
	}

	s := &Server{epoch: "test-epoch"}
	d := &document{data: doc}
	s.mu.Lock()
	s.registerLocked(d)
	s.mu.Unlock()

	// An ordinary mutation: append the readme, which is what any edit does structurally.
	edited, aerr := p2p.AppendReadme(doc)
	if aerr != nil {
		t.Fatal(aerr)
	}
	err = s.commitMutation(d, doc, edited)
	if !errors.Is(err, ErrCeremonyFrozen) {
		t.Fatalf("a mutation on a convened document reported %v, want ErrCeremonyFrozen — "+
			"every other party was invited to sign these exact bytes", err)
	}
	if !strings.Contains(err.Error(), "ceremony") {
		t.Errorf("the refusal does not name the ceremony: %v", err)
	}

	// The DESTRUCTIVE door too, and it matters more there: redaction on a convened document
	// would leave every other party's copy hashing to bytes that no longer exist.
	if err := s.commitBarrier(d, edited); !errors.Is(err, ErrCeremonyFrozen) {
		t.Errorf("a barrier operation on a convened document reported %v, want ErrCeremonyFrozen", err)
	}

	// And the control: an UNCONVENED document still commits, or the freeze has broken editing.
	p := &document{data: plain}
	s.mu.Lock()
	s.registerLocked(p)
	s.mu.Unlock()
	grown, gerr := p2p.AppendReadme(plain)
	if gerr != nil {
		t.Fatal(gerr)
	}
	if err := s.commitMutation(p, plain, grown); err != nil {
		t.Errorf("an ordinary document was refused (%v) — the freeze is refusing everything, "+
			"which would break every edit in the product", err)
	}
}

// TestConveneItselfIsNotFrozenOut — the guard tests the INPUT, and this is why.
//
// A freeze on the RESULT would refuse the one operation that is supposed to create a
// ceremony: convene's output carries a record by construction. The pre-op document is what
// distinguishes "a ceremony already exists" from "one is being created".
func TestConveneItselfIsNotFrozenOut(t *testing.T) {
	plain, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	conv := convenedBytes(t)
	s := &Server{epoch: "test-epoch"}
	d := &document{data: plain}
	s.mu.Lock()
	s.registerLocked(d)
	s.mu.Unlock()
	// The shape convene performs: an input with no record, a result with one.
	if err := s.commitBarrier(d, conv); err != nil {
		t.Fatalf("convene's own commit was refused (%v) — a freeze on the RESULT rather than "+
			"the input makes creating a ceremony impossible", err)
	}
}

// TestEveryMutatingRouteReachesTheCeremonyFreeze — the ROUTING guard, and it exists because
// its absence is what let a route through.
//
// The freeze tests above call commitMutation and commitBarrier in process. That proves the
// doors refuse; it cannot prove every mutating route USES one. `handleSave` does not — it
// writes the file itself and assigns doc.data under the registry lock — so it inherited
// nothing while sitting in tier 2's MUTATING inventory, and the freeze's own doc comment
// claimed the opposite.
//
// So this asserts the property ADR-009 actually asks for: every route the repo calls mutating
// reaches the rule. A source scan, because the alternative — driving eleven handlers over
// HTTP with eleven different request bodies — tests the bodies as much as the routing, and
// the thing that goes wrong is a handler that never calls the rule at all.
func TestEveryMutatingRouteReachesTheCeremonyFreeze(t *testing.T) {
	// The inventory is tier 2's, read from its file rather than restated — a second copy of
	// the list is the drift this guard exists to catch, one level up.
	raw, err := os.ReadFile(filepath.Join("..", "..", "test", "jsdom", "pinning.test.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	block := string(raw)
	start := strings.Index(block, "const MUTATING = [")
	if start < 0 {
		t.Fatal("cannot find the MUTATING inventory in pinning.test.mjs — this guard is " +
			"reading the wrong file and its pass means nothing")
	}
	end := strings.Index(block[start:], "];")
	routes := regexp.MustCompile(`'(/api/[^']+)'`).FindAllStringSubmatch(block[start:start+end], -1)
	if len(routes) < 10 {
		t.Fatalf("found %d routes in the MUTATING inventory; it holds a dozen — the scan is "+
			"not reading the list", len(routes))
	}

	mux, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	// Every .go file in the package, so a handler in any file is found.
	srcs := map[string]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			b, rerr := os.ReadFile(e.Name())
			if rerr != nil {
				t.Fatal(rerr)
			}
			srcs[e.Name()] = string(b)
		}
	}

	// **Deliberate exemptions, each named with why — ADR-009's own carve-out.** A route that
	// cannot change a convened document's bytes does not need the freeze; a route that is
	// merely believed not to is how one gets through.
	exempt := map[string]string{
		"/api/undo": "structurally a no-op on a convened document: convene commits through " +
			"commitBarrier, which clears the undo stack, and the freeze then refuses every " +
			"mutation that could push to it — so the stack is empty and handleUndo returns " +
			"early. **This exemption depends on convene using the BARRIER door**; if it ever " +
			"moves to commitMutation, undo becomes live again and this line is wrong.",
		"/api/redo": "same as /api/undo: commitBarrier clears the redo stack too, and nothing " +
			"can push to it while the freeze holds.",
		"/api/close-view": "closes a VIEW, not a document — it changes registry state and not " +
			"one byte of any document, so there is nothing for the freeze to protect.",
	}

	reached := 0
	exempted := 0
	for _, m := range routes {
		route := m[1]
		if why := exempt[route]; why != "" {
			exempted++
			continue
		}
		// The handler this route is registered with.
		reg := regexp.MustCompile(`"POST ` + regexp.QuoteMeta(route) + `",\s*(?:[a-zA-Z.]+\()*s\.(handle\w+)`)
		hm := reg.FindStringSubmatch(string(mux))
		if hm == nil {
			t.Errorf("%s is in the MUTATING inventory and has no POST registration naming a "+
				"handler in server.go", route)
			continue
		}
		name := hm[1]
		body := ""
		for _, src := range srcs {
			if i := strings.Index(src, "func (s *Server) "+name+"("); i >= 0 {
				body = funcBodyFrom(src, i)
				break
			}
		}
		if body == "" {
			t.Errorf("%s: cannot find the body of %s", route, name)
			continue
		}
		// **CODE only — comments are stripped, and this line is the finding.**
		//
		// The first draft substring-matched the raw body. `handleSave`'s body mentions both
		// commit doors ONLY in the comment explaining that it reaches neither — so the guard
		// read the sentence saying the route was uncovered as proof that it was covered, and
		// deleting handleSave's real freeze call left the suite green. Measured at the slice's
		// diff review. `registry_test.go` already strips `//` for exactly this reason and
		// cites a predecessor that hit it; this guard did not carry the lesson.
		code := stripLineComments(body)
		if strings.Contains(code, "ceremonyFreeze") ||
			strings.Contains(code, "commitMutation") || strings.Contains(code, "commitBarrier") {
			reached++
			continue
		}
		t.Errorf("%s (%s) is a MUTATING route and reaches neither a commit door nor "+
			"ceremonyFreeze, so a document under a live ceremony can be changed through "+
			"it. D29: mutating operations refuse and name the ceremony.", route, name)
	}
	// Stimulus: if NOTHING was found to reach it, the scan is matching nothing and every row
	// above passed for a reason that has nothing to do with the freeze.
	if reached == 0 {
		t.Fatal("no MUTATING route was found to reach the freeze — the scan matched nothing")
	}
	// An exemption for a route that has left the inventory silently covers the next route
	// given that name — the same hole `unreadKnown` polices one level out.
	for route := range exempt {
		found := false
		for _, m := range routes {
			if m[1] == route {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q is exempted from the ceremony freeze but is no longer in the MUTATING "+
				"inventory — the exemption now covers nothing and would silently cover the next "+
				"route of that name", route)
		}
	}
	t.Logf("%d MUTATING routes reach the freeze, %d exempt with a stated reason", reached, exempted)
}

// funcBodyFrom returns the source of the function starting at i, brace-matched.
func funcBodyFrom(src string, i int) string {
	rest := src[i:]
	open := strings.Index(rest, "{")
	if open < 0 {
		return ""
	}
	depth := 0
	for j := open; j < len(rest); j++ {
		switch rest[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[open : j+1]
			}
		}
	}
	return ""
}

// stripLineComments removes `//` comment tails so a scan for a call cannot be satisfied by
// prose that merely names it.
func stripLineComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
