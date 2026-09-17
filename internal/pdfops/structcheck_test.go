package pdfops

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure tree's consistency invariants and its one mutation — P05.S03.

// checkTree reads a document and returns its tree's defects.
// TestANullParentTreeSlotUnderAClaimedMCIDIsADefect — `/pending 503`. Invariant 4 was checked only where
// the slot named SOME object; a null slot under an MCID an element claims passed.
func TestANullParentTreeSlotUnderAClaimedMCIDIsADefect(t *testing.T) {
	if _, defects := checkTree(t, keyFixture("/ParentTreeNextKey 1", "", nil)); len(defects) != 0 {
		t.Fatalf("setup: the owned fixture already has defects %v, so a defect below would not be the null slot's", defects)
	}
	_, defects := checkTree(t, keyFixture("/ParentTreeNextKey 1", "", map[int]string{9: "<< /Nums [0 [null]] >>"}))
	for _, d := range defects {
		if strings.HasPrefix(d.key, "mcid-unowned ") {
			return
		}
	}
	t.Errorf("MCID 0 is claimed by object 8 and its ParentTree slot is null, and the check reports %v", defects)
}

func checkTree(t *testing.T, pdf []byte) (*structTree, []structDefect) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		t.Fatalf("tree: %v", terr)
	}
	return tree, checkStructConsistency(ctx, tree)
}

// TestEveryTreeThisRepoProducesIsSelfConsistent — P05.S03's first reader, pointed at documents that
// already exist rather than at ones this slice creates.
//
// **`NUp` is in the list on purpose.** P01.S06 built `carryTagsThroughNUp` to re-anchor a tree
// through an n-up, and its acceptance was measured with veraPDF clause counts. This asks a
// different question — is the result self-consistent — with an instrument P01 did not have.
func TestEveryTreeThisRepoProducesIsSelfConsistent(t *testing.T) {
	base := taggedFixture()
	nup, err := NUp(base, 2, false)
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	rot, err := Rotate(base, nil, 90)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	for name, pdf := range map[string][]byte{
		"the tagged corpus fixture":  base,
		"two identical elements":     twinElementFixture(),
		"a shared (DAG) element":     sharedElementFixture(),
		"the fixture through NUp(2)": nup,
		"the fixture through Rotate": rot,
	} {
		tree, defects := checkTree(t, pdf)
		if tree.elements() == 0 {
			t.Errorf("%s: no elements, so consistency is vacuous", name)
			continue
		}
		if len(defects) > 0 {
			var lines []string
			for _, d := range defects {
				lines = append(lines, "  - "+d.String())
			}
			t.Errorf("%s: %d structural defect(s)\n%s", name, len(defects), strings.Join(lines, "\n"))
		}
	}
}

// TestADanglingStructParentsKeyIsReported is the stimulus floor for the test above: a checker that
// reports nothing wrong with everything is indistinguishable from a checker that reports nothing.
func TestADanglingStructParentsKeyIsReported(t *testing.T) {
	_, defects := checkTree(t, danglingStructParentsFixture())
	if len(defects) == 0 {
		t.Fatal("a page declaring /StructParents 4 against a ParentTree that only has key 0 was " +
			"reported as consistent — the checker cannot see the invariant it exists for, and " +
			"every clean result above means nothing")
	}
	found := false
	for _, d := range defects {
		if strings.Contains(d.String(), "no entry 4") {
			found = true
		}
	}
	if !found {
		t.Errorf("the defects do not name the dangling key: %v", defects)
	}
}

// TestAddingAMarkedElementKeepsEveryInvariant — the write half, with the checker as its
// post-condition rather than as a separate opinion.
func TestAddingAMarkedElementKeepsEveryInvariant(t *testing.T) {
	out, err := writeMutatedTree(t, taggedFixture(), func(ctx *model.Context, tree *structTree) error {
		mcid, ref, aerr := addMarkedElementUnder(ctx, tree, 1, "Span", nil)
		if aerr != nil {
			return aerr
		}
		if ref == nil {
			return errNoStructTree
		}
		// The fixture's page already owns MCID 0, so the next free slot must be 1 — an
		// implementation that appended rather than indexing would also say 1 here, which is why
		// the invariant check below is what actually settles it.
		if mcid != 1 {
			t.Errorf("the new element got MCID %d; the page's ParentTree array had one slot, so "+
				"the next free index is 1", mcid)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	tree, defects := checkTree(t, out)
	if len(defects) > 0 {
		var lines []string
		for _, d := range defects {
			lines = append(lines, "  - "+d.String())
		}
		t.Fatalf("adding an element broke %d invariant(s)\n%s", len(defects), strings.Join(lines, "\n"))
	}
	if got := tree.elements(); got != 2 {
		t.Errorf("the document now has %d element(s), want 2 — the addition did not reach the tree", got)
	}
}

// TestMarkingARunCostsTheSameHoweverManyRunsCameBefore — the writers are linear in the runs they mark.
//
// `addMarkedElementUnder` and `addMCIDTo` once rebuilt every ParentTree slot list on each call, which
// made tagging quadratic: tier 4d's 20,000-clause Markdown fixture took over twenty minutes to convert
// (found at `PLAN-accessibility.md` P07's close). Four times the runs must cost well under the sixteen
// times a quadratic writer takes; the bound is eight, and the best of three runs is compared so a busy
// machine does not read as a regression.
//
// **Blind spot**: the fixture has one page, so the page-tree half of that defect (`tree.page`'s cache)
// is not exercised here — pdfcpu's `PageDict` is constant-time on a one-page document.
func TestMarkingARunCostsTheSameHoweverManyRunsCameBefore(t *testing.T) {
	// once measures one run of `elements` markings. The min of several is the caller's job, because
	// the two sizes are measured INTERLEAVED — see below.
	once := func(elements int) time.Duration {
		var took time.Duration
		{
			if _, err := writeMutatedTree(t, taggedFixture(), func(ctx *model.Context, tree *structTree) error {
				start := time.Now()
				for i := 0; i < elements; i++ {
					_, ref, aerr := addMarkedElementUnder(ctx, tree, 1, "P", nil)
					if aerr != nil {
						return aerr
					}
					for k := 0; k < 3; k++ {
						if _, merr := addMCIDTo(ctx, tree, 1, *ref); merr != nil {
							return merr
						}
					}
				}
				took = time.Since(start)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		return took
	}

	// **The MINIMUM RATIO over several interleaved rounds, not the ratio of the minimums.**
	//
	// This is a clock, and a clock on a shared machine measures the weather as well as the code. The
	// ratio is robust to the machine being uniformly slow — that is what a ratio is for — and not to
	// being slow for only ONE of its two terms. Measuring `cost(1000)` to completion and then
	// `cost(4000)` put minutes between them, so a load spike covering the second and not the first
	// inflated the ratio with nothing about the code having changed: **8.5× against a ceiling of 8**
	// during a full suite sharing the machine with another repo's `cargo test --workspace`, against
	// 4.3×, 3.2× and 1.8× on three solo runs immediately afterwards.
	//
	// Interleaving alone was measured and is NOT enough — 1 run in 8 still breached, at 10.6×, with
	// all 8 cores saturated. Taking each round's own ratio and keeping the smallest is, because a
	// regression breaches in EVERY round and survives the minimum while a load spike breaches in one
	// and does not. Measured, with the quadratic introduced as work proportional to the MCIDs already
	// on the page — per document, so it is quadratic identically in every round:
	//
	//   linear, quiet:            3.7
	//   linear, 8 cores saturated: 2.5  2.6  3.2  3.6  4.0  4.0  4.4  4.7   — 8 of 8 green
	//   dominant quadratic:       9.1  10.3  10.4                           — 3 of 3 RED
	//
	// **Declared limit: a MILD quadratic is caught by neither shape.** The same mutation doing cheap
	// integer adds rather than allocating measured 4.7, 7.9 and 5.5 and passed, because its quadratic
	// term does not dominate the insertion's own linear cost. This test catches a regression that has
	// become the dominant cost, which is the one that matters and is not the only one that exists.
	// The original shape had the same blind spot; the fix neither widens nor narrows it.
	//
	// **Two probes were rejected before these numbers, both for being inert**, which is the reason the
	// mutation above is described so precisely: disabling `allocParentTreeKey`'s cache moved nothing
	// (it is not on this path), and iterating `tree.elems` moved nothing (the model's slice does not
	// grow as the document does). A third, a package-level counter, was rejected for the opposite
	// reason — it leaked state across rounds, so later rounds started slow and the ratio flattened,
	// which would have condemned this shape for an artefact of the probe.
	best := 0.0
	var small, large time.Duration
	for round := 0; round < 3; round++ {
		s, l := once(1000), once(4000)
		if s <= 0 {
			t.Fatal("marking 1000 runs took no measurable time, so the ratio below compares nothing")
		}
		if r := float64(l) / float64(s); best == 0 || r < best {
			best, small, large = r, s, l
		}
	}
	ratio := best
	t.Logf("4× the runs cost %.1f× the time (%v → %v)", ratio, small, large)
	if ratio > 8 {
		t.Errorf("4× the runs cost %.1f× the time (%v → %v) — the writers have gone superlinear in the "+
			"runs already on the page", ratio, small, large)
	}
}

// TestAddingToAPageWithNoStructParentsCreatesTheKey: the page's side of the invariant, which is the
// half a caller would forget.
func TestAddingToAPageWithNoStructParentsCreatesTheKey(t *testing.T) {
	// A tagged document whose page carries NO /StructParents.
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4: "<< /Length 1 >>\nstream\n \nendstream",
		7: "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8: "<< /Type /StructElem /S /P /Pg 3 0 R >>",
	})
	out, err := writeMutatedTree(t, src, func(ctx *model.Context, tree *structTree) error {
		_, _, aerr := addMarkedElementUnder(ctx, tree, 1, "P", nil)
		return aerr
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatalf("re-read: %v", rerr)
	}
	d, _, _, _ := ctx.PageDict(1, false)
	if d == nil {
		t.Fatal("no page")
	}
	if _, ok := d["StructParents"].(types.Integer); !ok {
		t.Errorf("the page still has no /StructParents (%v) — the element that was just added is "+
			"unreachable from the page's side", d["StructParents"])
	}
	if _, defects := checkTree(t, out); len(defects) > 0 {
		t.Errorf("invariants broken: %v", defects)
	}
}

// writeMutatedTree reads a document, parses its tree, applies f, and writes it back. The test-side
// equivalent of what P05.S04 will do in production.
func writeMutatedTree(t *testing.T, pdf []byte, f func(*model.Context, *structTree) error) ([]byte, error) {
	t.Helper()
	return writeMutated(pdf, func(ctx *model.Context) error {
		live := map[int]bool{}
		for p := 1; p <= ctx.PageCount; p++ {
			if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
				live[ir.ObjectNumber.Value()] = true
			}
		}
		tree, err := readStructTree(ctx, live)
		if err != nil {
			return err
		}
		return f(ctx, tree)
	})
}

// TestAGapInTheParentTreeArrayIsFilledNotAppended drives `setParentTreeSlot`'s general contract,
// which its only production caller cannot reach.
//
// `addMarkedElementUnder` allocates `mcid` as the array's length, so filling and appending coincide on
// every call it makes — mutation proved it, by swapping one for the other and finding every test
// still green. The array is indexed BY MCID, so the distinction is the whole correctness of the
// structure: an element registered at the wrong index is an element a reader attributes to
// different content.
func TestAGapInTheParentTreeArrayIsFilledNotAppended(t *testing.T) {
	src := taggedFixture()
	out, err := writeMutatedTree(t, src, func(ctx *model.Context, tree *structTree) error {
		elem := types.Dict{"Type": types.Name("StructElem"), "S": types.Name("Span")}
		ref, rerr := ctx.IndRefForNewObject(elem)
		if rerr != nil {
			return rerr
		}
		// Key 0's array has one slot. Put the new element at MCID 5.
		return setParentTreeSlot(ctx, tree, 0, 5, *ref)
	})
	if err != nil {
		t.Fatalf("setParentTreeSlot: %v", err)
	}

	ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if rerr != nil {
		t.Fatalf("re-read: %v", rerr)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	tree, terr := readStructTree(ctx, live)
	if terr != nil {
		t.Fatalf("tree: %v", terr)
	}
	arrays, _ := parentTreeEntries(ctx, tree)
	arr, ok := arrays[0]
	if !ok {
		t.Fatal("key 0 has no array entry any more")
	}
	if len(arr) != 6 {
		t.Fatalf("the array for key 0 has %d slot(s); an element at MCID 5 needs 6 — an APPEND "+
			"would have produced %d and put the element at the wrong index", len(arr), 2)
	}
	if arr[5] == 0 {
		t.Error("slot 5 is empty — the element did not land at its MCID")
	}
	if arr[1] != 0 || arr[4] != 0 {
		t.Errorf("slots 1 and 4 should be empty fillers, got %d and %d", arr[1], arr[4])
	}
	if arr[0] == 0 {
		t.Error("slot 0 lost the element that was already there")
	}
}

// treeFingerprint is everything about a structure tree that a lossless round trip must preserve:
// every element's type and page, its MCIDs and object references in order, and the role map.
//
// It is deliberately NOT the document's bytes. `writeMutated` re-serialises the whole file — object
// numbers move, streams are recompressed — so a byte comparison would fail on every document and
// prove nothing about the tree. What must survive is the STRUCTURE, and this is it written down.
func treeFingerprint(t *testing.T, pdf []byte) string {
	t.Helper()
	tree, _ := checkTree(t, pdf)
	var b strings.Builder
	roles := make([]string, 0, len(tree.roleMap))
	for k, v := range tree.roleMap {
		roles = append(roles, k+"->"+v)
	}
	sort.Strings(roles)
	fmt.Fprintf(&b, "rolemap[%s]\n", strings.Join(roles, ","))
	for _, e := range tree.elems {
		fmt.Fprintf(&b, "elem /%s pgLive=%v kids=", e.kind, e.pgLive)
		for _, k := range e.kids {
			switch k.kind {
			case kidMCID:
				fmt.Fprintf(&b, "mcid:%d ", k.mcid)
			case kidMCR:
				fmt.Fprintf(&b, "mcr:%d ", k.mcid)
			case kidOBJR:
				fmt.Fprintf(&b, "objr ")
			case kidElement:
				fmt.Fprintf(&b, "elem:/%s ", k.elem.kind)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TestRoundTrippingAnExistingTaggedDocumentIsLossless — **P05's first exit criterion**, and it had
// no reader until the phase close asked for one.
//
// S02 proved the model PARSES a tree faithfully, by comparing its count against an independent
// object-keyed walk. That is not the same claim: parsing correctly and then writing back everything
// you parsed are two properties, and the second is the one every later mutation rests on. A model
// that quietly dropped `/A` attributes, or an `OBJR`, would pass every S02 assertion and lose part
// of a user's document the first time anything wrote through it.
//
// The population is the REAL tree — role-mapped names, an OBJR, integer MCIDs — because the
// generated fixtures contain none of the three and a round trip over a tree with one element kind
// cannot lose the other two.
func TestRoundTrippingAnExistingTaggedDocumentIsLossless(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is absent, so P05's exit criterion — a round trip " +
			"of an existing tagged document is lossless — is UNCHECKED in this run. The generated " +
			"fixtures have no RoleMap, no OBJR and no integer MCIDs, so they cannot stand in.")
	}
	pdf := richTaggedFixture(t)
	before := treeFingerprint(t, pdf)
	if strings.Count(before, "elem /") < 5 {
		t.Fatalf("the fixture has too few elements for this to mean anything:\n%s", before)
	}
	if !strings.Contains(before, "objr") || !strings.Contains(before, "mcid:") {
		t.Fatalf("the fixture lacks an OBJR or an MCID, so a round trip cannot show them "+
			"surviving:\n%s", before)
	}
	// **The fingerprint compares the model against itself, so a SYMMETRIC loss is invisible.**
	// Dropping the role map entirely leaves before and after identical and the test green — found
	// by mutation. Every field the fingerprint carries therefore needs a floor asserting it is
	// non-empty in the first place, or the comparison is over a smaller tree than the document has.
	if strings.HasPrefix(before, "rolemap[]") {
		t.Fatalf("the fingerprint records an EMPTY role map for a document that has one — the "+
			"model is not reading it, and a round trip comparing the model to itself cannot see "+
			"that:\n%s", before)
	}

	// A no-op mutation: read the tree, change nothing, write the document back.
	out, err := writeMutatedTree(t, pdf, func(ctx *model.Context, tree *structTree) error {
		if tree.elements() == 0 {
			return errNoStructTree
		}
		return nil
	})
	if err != nil {
		t.Fatalf("no-op round trip: %v", err)
	}

	after := treeFingerprint(t, out)
	if before != after {
		t.Errorf("the tree changed across a no-op round trip\n--- before\n%s\n--- after\n%s", before, after)
	}
	if _, defects := checkTree(t, out); len(defects) > 0 {
		t.Errorf("the round-tripped document is not self-consistent: %v", defects)
	}
}

// richTaggedFixture produces a real tagged PDF with a RoleMap, an OBJR and integer MCIDs, by
// converting HTML through LibreOffice at test time. Generated rather than committed, for the reason
// `corpus_test.go` gives about opaque binary fixtures.
func richTaggedFixture(t *testing.T) []byte {
	t.Helper()
	const html = `<html><body><h1>A heading</h1><p>Body with <b>bold</b>.</p>` +
		`<ul><li>one</li><li>two</li></ul>` +
		`<table border="1"><tr><th>H</th></tr><tr><td>c</td></tr></table>` +
		`<p>A <a href="https://example.com">link</a>.</p></body></html>`
	dir := t.TempDir()
	in := filepath.Join(dir, "rich.html")
	if err := os.WriteFile(in, []byte(html), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(libreOfficePath(), "-env:UserInstallation=file://"+filepath.Join(dir, "prof"),
		"--headless", "--convert-to", "pdf", in, "--outdir", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("SKIP (not a pass): LibreOffice could not build the fixture: %v\n%s", err, out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "rich.pdf"))
	if err != nil {
		t.Skipf("SKIP (not a pass): no converted fixture: %v", err)
	}
	return b
}
