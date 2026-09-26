package uacheck

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The P07 phase-close review's cost findings in the content and structure walks (R3-1, R3-2, R3-8) and the two
// info findings beside them (R3-10, R3-11). Every cost test asserts a WORK COUNTER, never a wall clock, and asserts
// the stimulus happened before it grades the response.

// refTaggedContent, refInheritedLangOf and refEvent are the full-stack scans `pushFrame`'s aggregates replaced,
// kept verbatim as the differential oracle.
func refTaggedContent(d *Document, stack []frame) (bool, string) {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].elem != nil {
			return d.reachesStructTreeRoot(stack[i].elem)
		}
		if stack[i].elemUnread != "" {
			return false, stack[i].elemUnread
		}
	}
	return false, ""
}

func refInheritedLangOf(d *Document, stack []frame, stream int, ownCounts bool) (bool, string) {
	last := len(stack) - 1
	for i := last; i >= 0 && stack[i].stream == stream; i-- {
		if (i < last || ownCounts) && stack[i].lang {
			return true, ""
		}
		switch {
		case stack[i].elem != nil:
			if d.declaresLang(stack[i].elem["Lang"]) {
				return true, ""
			}
			found, why := d.parentLang(stack[i].elem)
			if why != "" {
				return false, why
			}
			if found {
				return true, ""
			}
		case stack[i].elemUnread != "":
			return false, stack[i].elemUnread
		}
	}
	return false, ""
}

func refEvent(d *Document, stack []frame, stream int) contentEvent {
	ev := contentEvent{mcid: -1, spKey: -1}
	artifact := false
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].artifact {
			artifact = true
		}
		if ev.mcid < 0 && stack[i].mcid >= 0 && stack[i].stream == stream {
			ev.mcid, ev.spKey = stack[i].mcid, stack[i].spKey
		}
	}
	if artifact {
		ev.covered = true
	} else {
		ev.covered, ev.coverUnread = refTaggedContent(d, stack)
	}
	ev.langDetermined, ev.langUnread = refInheritedLangOf(d, stack, stream, true)
	return ev
}

// elementPool builds structure elements of every shape the three inheritances distinguish: reaching the root or
// not, a `/Lang` on the element, on an ancestor, on the root, a cycle, and a `/P` chain long enough to refuse.
func elementPool(rng *rand.Rand) []types.Dict {
	root := types.Dict{"Type": types.Name("StructTreeRoot")}
	if rng.Intn(2) == 0 {
		root["Lang"] = types.StringLiteral("en")
	}
	var pool []types.Dict
	chain := func(n int, parent types.Dict, langAt int) types.Dict {
		p := parent
		var e types.Dict
		for i := 0; i < n; i++ {
			e = types.Dict{"S": types.Name("Span")}
			if p != nil {
				e["P"] = p
			}
			if i == langAt {
				e["Lang"] = types.StringLiteral("de")
			}
			p = e
		}
		return e
	}
	for i := 0; i < 6; i++ {
		pool = append(pool, chain(1+rng.Intn(5), root, rng.Intn(8)-2)) // reaches the root
		pool = append(pool, chain(1+rng.Intn(5), nil, rng.Intn(8)-2))  // detached
	}
	pool = append(pool, chain(maxLangClimb+5, root, -1)) // refuses the climb
	cyc := types.Dict{"S": types.Name("Span")}
	mid := types.Dict{"S": types.Name("Span"), "P": cyc}
	cyc["P"] = mid
	pool = append(pool, cyc)
	return pool
}

// TestTheStackAggregatesAgreeWithTheFullScan — R3-2's guard. Every question the rules ask of the marked-content
// stack is now read off the top frame's aggregates; this compares each against the full scan it replaced, over
// random nestings that cross stream boundaries, push and pop, and share backing arrays the way nested walks do.
func TestTheStackAggregatesAgreeWithTheFullScan(t *testing.T) {
	d, err := open(markedDoc{content: "q Q"}.build())
	if err != nil {
		t.Fatal(err)
	}
	compared := 0
	for seed := int64(1); seed <= 300; seed++ {
		rng := rand.New(rand.NewSource(seed))
		pool := elementPool(rng)
		var stack []frame
		stream := 1
		for step := 0; step < 60; step++ {
			if len(stack) > 0 && rng.Intn(4) == 0 {
				stack = stack[:len(stack)-1]
			} else {
				if rng.Intn(5) == 0 {
					stream++ // a nested walk: frames above here are a newer stream's
				}
				f := frame{mcid: -1, spKey: -1, stream: stream}
				if rng.Intn(4) == 0 {
					f.tag, f.artifact = "Artifact", true
				}
				if rng.Intn(3) == 0 {
					f.mcid, f.spKey = rng.Intn(9), rng.Intn(3)
				}
				f.lang = rng.Intn(5) == 0
				switch rng.Intn(5) {
				case 0, 1:
					f.elem = pool[rng.Intn(len(pool))]
				case 2:
					f.elemUnread = fmt.Sprintf("unread %d/%d", seed, step)
				}
				stack = d.pushFrame(stack, f)
			}
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1].stream
			for _, s := range []int{top, top + 1} {
				w := walker{d: d, stream: s}
				got, want := w.event(stack, true, ""), refEvent(d, stack, s)
				if got.covered != want.covered || got.coverUnread != want.coverUnread ||
					got.langDetermined != want.langDetermined || got.langUnread != want.langUnread ||
					got.mcid != want.mcid || got.spKey != want.spKey {
					t.Fatalf("seed %d step %d stream %d: event %+v, full scan %+v", seed, step, s, got, want)
				}
				for _, own := range []bool{true, false} {
					gl, gw := d.inheritedLangOf(stack, s, own)
					wl, ww := refInheritedLangOf(d, stack, s, own)
					if gl != wl || gw != ww {
						t.Fatalf("seed %d step %d stream %d own %v: inheritedLang (%v, %q), full scan (%v, %q)",
							seed, step, s, own, gl, gw, wl, ww)
					}
				}
				compared++
			}
			sub := walker{d: d, stream: top}.subject(stack)
			wantArtifact := false
			for _, f := range stack {
				wantArtifact = wantArtifact || f.artifact
			}
			wl, ww := refInheritedLangOf(d, stack, top, false)
			wt, wtw := refTaggedContent(d, stack)
			if sub.insideArtifact != wantArtifact || sub.inheritedLang != wl || sub.langUnread != ww ||
				sub.tagged != wt || sub.taggedUnread != wtw {
				t.Fatalf("seed %d step %d: subject %+v, full scan artifact=%v lang=(%v,%q) tagged=(%v,%q)",
					seed, step, sub, wantArtifact, wl, ww, wt, wtw)
			}
		}
	}
	// The stimulus: the comparison must have run over stacks that reached every answer, not over empty ones.
	if compared < 10000 {
		t.Fatalf("only %d comparisons ran", compared)
	}
}

// type3Doc is a tagged page showing `/T3` `shows` times, whose font's `/CharProcs` holds `procs` entries naming
// glyph streams: each its own stream when distinct, else all one stream.
func type3Doc(shows, procs int, distinct bool) []byte {
	var cp strings.Builder
	extra := map[int]string{}
	glyph := "<< /Length 21 >>\nstream\n10 0 0 0 10 10 d0\n\nendstream"
	for i := 0; i < procs; i++ {
		obj := 11
		if distinct {
			obj = 11 + i
			extra[obj] = glyph
		}
		fmt.Fprintf(&cp, "/g%d %d 0 R ", i, obj)
	}
	extra[11] = glyph
	extra[10] = "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.1 0 0 0.1 0 0] " +
		"/CharProcs << " + cp.String() + ">> /Encoding << /Type /Encoding /Differences [97 /g0] >> " +
		"/FirstChar 97 /LastChar 97 /Widths [10] /Resources << >> >>"
	return markedDoc{
		content: "/P <</MCID 0>> BDC\nBT /T3 12 Tf 72 700 Td\n" + strings.Repeat("(a) Tj\n", shows) + "ET\nEMC",
		pageRes: "<< /Font << /F1 5 0 R /T3 10 0 R >> >>",
		extra:   extra,
	}.build()
}

// TestAType3FontIsEnumeratedOncePerCharProcsDictionary — R3-1. `enterType3` ran its whole per-glyph loop on every
// text-showing operator, ahead of the per-stream memo, so N shows of an N-glyph font cost N² (20,000 of each:
// 4m30s). Now the enumeration happens once: the count of re-enumerated procedures is zero however often it shows.
func TestAType3FontIsEnumeratedOncePerCharProcsDictionary(t *testing.T) {
	const shows, procs = 400, 30
	d, err := open(type3Doc(shows, procs, true))
	if err != nil {
		t.Fatal(err)
	}
	events, why := d.contentEvents()
	texts := 0
	for _, ev := range events {
		if ev.text {
			texts++
		}
	}
	if texts != shows || why != "" {
		t.Fatalf("stimulus: %d text events (want %d), contentErr %q", texts, shows, why)
	}
	if d.formWalks != procs {
		t.Fatalf("stimulus: %d glyph procedures walked, want every one of the %d", d.formWalks, procs)
	}
	if d.charProcAsks != 0 {
		t.Fatalf("the font's /CharProcs was re-enumerated: %d asks for procedures already read over %d shows", d.charProcAsks, shows)
	}
}

// TestTheCharProcAskBudgetHasAControlJustUnderIt — R3-1's ceiling. One `/CharProcs` dictionary naming ONE stream
// M times asks M-1 times for a procedure already read. At `maxCharProcAsks` asks the walk answers; one more and it
// refuses, naming the budget.
func TestTheCharProcAskBudgetHasAControlJustUnderIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		procs  int
		refuse bool
	}{
		{"control, at the ceiling", maxCharProcAsks + 1, false},
		{"one ask past it", maxCharProcAsks + 2, true},
	} {
		pdf := type3Doc(1, tc.procs, false)
		d, err := open(pdf)
		if err != nil {
			t.Fatal(err)
		}
		_, why := d.contentEvents()
		if want := tc.procs - 1; d.charProcAsks != want {
			t.Fatalf("%s: stimulus: %d asks, want %d", tc.name, d.charProcAsks, want)
		}
		if refused := strings.Contains(why, "Type 3 glyph procedures it has already read"); refused != tc.refuse {
			t.Fatalf("%s: contentErr %q, want the budget's refusal %v", tc.name, why, tc.refuse)
		}
		got := verdictOf(t, pdf, "7.1 t3")
		if (got.Verdict == CannotCheck) != tc.refuse {
			t.Fatalf("%s: 7.1 t3 %v (%s)", tc.name, got.Verdict, got.Why)
		}
	}
}

// sharedArrayTree is `st_probe`'s shape: L levels of indirect arrays, each holding two INLINE elements (or number
// tree nodes) that both name the next level's array — so a walk keyed on object numbers reads 2^L of them.
func sharedArrayTree(L int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 4 0 R /MarkInfo << /Marked true >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		4: fmt.Sprintf("<< /Type /StructTreeRoot /K 5 0 R /ParentTree << /Kids %d 0 R >> >>", 5+L),
	}
	for k := 0; k < L; k++ {
		if k+1 < L {
			e := fmt.Sprintf("<< /Type /StructElem /S /P /P 4 0 R /K %d 0 R >> ", 6+k)
			objs[5+k] = "[" + e + e + "]"
			n := fmt.Sprintf("<< /Kids %d 0 R >> ", 6+L+k)
			objs[5+L+k] = "[" + n + n + "]"
		} else {
			objs[5+k] = "[<< /Type /StructElem /S /P /P 4 0 R >>]"
			objs[5+L+k] = "[<< /Nums [0 3 0 R] >>]"
		}
	}
	return buildPDF(objs)
}

// TestTheParentTreeReadsASharedNodeOnce — R3-8's parent-tree half. A node inline in a shared `/Kids` array has no
// object number, so the walk read it once per path: 2^L. A lookup reads a node it has finished searching as a skip, so
// a key the tree lacks costs each shared array's entries once per parent — 4L-3 reads — and a key it holds one path.
func TestTheParentTreeReadsASharedNodeOnce(t *testing.T) {
	const L = 12
	d, err := open(sharedArrayTree(L))
	if err != nil {
		t.Fatal(err)
	}
	if _, found, why := d.parentTreeEntry(0); !found || why != "" {
		t.Fatalf("stimulus: the parent tree's one key was not found (why %q)", why)
	}
	if want := 1 + L; d.ptNodes != want {
		t.Fatalf("finding the key read %d nodes over %d shared levels, want %d — the first path", d.ptNodes, L, want)
	}
	before := d.ptNodes
	if _, found, why := d.parentTreeEntry(7); found || why != "" {
		t.Fatalf("an absent key: found %v, why %q — want definitely absent", found, why)
	}
	if want := 4*L - 3; d.ptNodes-before != want {
		t.Fatalf("an absent key read %d nodes over %d shared levels, want %d — a shared node read once per path", d.ptNodes-before, L, want)
	}
}

// TestTheStructureWalkBoundHasAControlJustUnderIt — R3-8's structure half. An inline element reached twice is
// visited twice (veraPDF gives a key-less object a fresh identity each reach), so the walk is BOUNDED rather than
// deduplicated: exactly at the ceiling it answers, one entry more and it refuses.
func TestTheStructureWalkBoundHasAControlJustUnderIt(t *testing.T) {
	const L = 10
	pdf := sharedArrayTree(L)
	d, err := open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	nodes, why := d.structNodes()
	// 2 + 4 + … + 2^(L-1) inline elements, and the last level's single element under each of 2^(L-1) paths.
	wantNodes := (1<<L - 2) + 1<<(L-1)
	if len(nodes) != wantNodes || why != "" {
		t.Fatalf("stimulus: %d nodes (want %d, repeats included), why %q", len(nodes), wantNodes, why)
	}
	entries := d.nodeEntries
	saved := maxStructEntries
	t.Cleanup(func() { maxStructEntries = saved })
	for _, tc := range []struct {
		ceiling int
		refuse  bool
	}{{entries, false}, {entries - 1, true}} {
		maxStructEntries = tc.ceiling
		d, err := open(pdf)
		if err != nil {
			t.Fatal(err)
		}
		nodes, why := d.structNodes()
		if refused := strings.Contains(why, "/K entries"); refused != tc.refuse {
			t.Fatalf("ceiling %d over %d entries: why %q, want refusal %v", tc.ceiling, entries, why, tc.refuse)
		}
		if !tc.refuse && len(nodes) != wantNodes {
			t.Fatalf("ceiling %d: the control read %d nodes, want %d", tc.ceiling, len(nodes), wantNodes)
		}
	}
}

// TestAPanicOpeningTheDocumentIsARefusalForEveryClause — R3-11. Opening ran outside `runOne`'s recover, so a pdfcpu
// panic there killed `nib ua`. It is every clause CannotCheck, naming the panic, and no error.
func TestAPanicOpeningTheDocumentIsARefusalForEveryClause(t *testing.T) {
	saved := openDocument
	t.Cleanup(func() { openDocument = saved })
	openDocument = func([]byte) (*Document, error) { panic(errors.New("pdfcpu: index out of range")) }
	rep, err := Check([]byte("%PDF-1.7"))
	if err != nil {
		t.Fatalf("a panic while opening is an error %v, want a report of refusals", err)
	}
	if len(rep.Results) != len(Clauses()) {
		t.Fatalf("%d results for %d clauses", len(rep.Results), len(Clauses()))
	}
	for _, r := range rep.Results {
		if r.Verdict != CannotCheck || !strings.Contains(r.Why, "index out of range") {
			t.Fatalf("%s: %v (%s), want CannotCheck naming the panic", r.Clause, r.Verdict, r.Why)
		}
	}
}

// TestObjectZeroIsNeverALinkInTheChain — R3-10. A form with no object number must not enter the chain, or the
// first key-less form would make every later one read as "drawing itself" and go unwalked in silence.
func TestObjectZeroIsNeverALinkInTheChain(t *testing.T) {
	next := withLink(map[int]bool{7: true}, 0)
	if next[0] || !next[7] || len(next) != 1 {
		t.Fatalf("withLink(chain{7}, 0) = %v, want {7}", next)
	}
	if next := withLink(next, 9); !next[9] || !next[7] {
		t.Fatalf("withLink(chain{7}, 9) = %v, want {7, 9}", next)
	}
}

// TestAProcedureSkippedOnTheChainIsReachedByALaterShow — R3-1's one exception to the memo. A stream that is both
// a form XObject and a Type 3 glyph procedure, showing that font from inside itself, is skipped there for being on
// the chain; the enumeration is then not memoised, so the page's own show still walks it lang-only, as every show
// always did before the memo existed.
func TestAProcedureSkippedOnTheChainIsReachedByALaterShow(t *testing.T) {
	body := "BT /T3 12 Tf (a) Tj ET"
	pdf := markedDoc{
		content: "/P <</MCID 0>> BDC\n/X0 Do\nBT /T3 12 Tf 72 700 Td (a) Tj ET\nEMC",
		pageRes: "<< /Font << /F1 5 0 R /T3 10 0 R >> /XObject << /X0 20 0 R >> >>",
		extra: map[int]string{
			10: "<< /Type /Font /Subtype /Type3 /FontBBox [0 0 10 10] /FontMatrix [0.1 0 0 0.1 0 0] " +
				"/CharProcs << /g0 20 0 R >> /Encoding << /Type /Encoding /Differences [97 /g0] >> " +
				"/FirstChar 97 /LastChar 97 /Widths [10] /Resources << /Font << /T3 10 0 R >> >> >>",
			20: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /Font << /T3 10 0 R >> >> "+
				"/Length %d >>\nstream\n%s\nendstream", len(body), body),
		},
	}.build()
	d, err := open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	d.contentEvents()
	// The stimulus: the form was walked AND met itself on the chain.
	if d.charProcAsks < 1 {
		t.Fatalf("stimulus: no procedure was skipped on the chain (asks %d)", d.charProcAsks)
	}
	if d.formWalks != 2 {
		t.Fatalf("%d nested walks, want 2 — the form, then the page's show reading it as a glyph procedure", d.formWalks)
	}
}
