package pdfops

import (
	"fmt"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"nib/internal/testpdf"
)

// labelsOf reads a document's page labels back as one string per page, in output order, using the
// same resolution a reader performs: the last range starting at or before the page.
//
// **Read back through the number tree rather than compared as a dict**, because the assertion this
// test owes is about what a READER shows on page 3, and two different trees can say the same thing.
func labelsOf(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx := readCtx(t, pdf)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	d := derefDict(ctx.XRefTable, root["PageLabels"])
	if d == nil {
		return nil
	}
	nums, aerr := ctx.XRefTable.DereferenceArray(d["Nums"])
	if aerr != nil {
		t.Fatalf("/PageLabels /Nums does not dereference: %v", aerr)
	}
	ranges := readPageLabelRanges(ctx.XRefTable, nums)
	out := make([]string, ctx.PageCount)
	for i := range out {
		lab, ok := labelAt(ranges, i)
		// **"-" is what a READER shows, and both ways of getting there are it.** A page before the
		// first range has no entry; a page under an entry with neither /S nor /P has one that says
		// nothing (§12.4.2). Rendering those differently would make this helper assert the shape of
		// the tree rather than what the tree says, and the carry is entitled to choose the shape.
		// It still separates the case that matters: a page given NO entry inherits the range above
		// it and comes back reading "D8", which is `TestAPageTheSourceNeverLabelledIsNotGivenOne`.
		if !ok || (lab.style == "" && lab.prefix == "") {
			out[i] = "-"
			continue
		}
		out[i] = fmt.Sprintf("%s%s%d", lab.prefix, styleTag(lab.style), lab.number)
	}
	return out
}

func styleTag(s string) string {
	if s == "" {
		return "_"
	}
	return s
}

// rangeCount is how many entries the output's number tree actually holds — the figure the refused
// ceiling would have bounded.
func rangeCount(t *testing.T, pdf []byte) int {
	t.Helper()
	ctx := readCtx(t, pdf)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	d := derefDict(ctx.XRefTable, root["PageLabels"])
	if d == nil {
		return 0
	}
	nums, aerr := ctx.XRefTable.DereferenceArray(d["Nums"])
	if aerr != nil {
		return 0
	}
	return len(nums) / 2
}

// pageLabelledFixture is `n` pages labelled i, ii, iii then 1, 2, 3… — the front-matter-plus-body shape
// `PageLabelRange`'s own doc calls the two-range case.
func pageLabelledFixture(t *testing.T, n int) []byte {
	t.Helper()
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("page %d", i+1)
	}
	src, err := testpdf.Text(names...)
	if err != nil {
		t.Fatal(err)
	}
	out, err := SetPageLabels(src, []PageLabelRange{
		{Start: 1, Style: "roman-lower"},
		{Start: 4, Style: "decimal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAPageLabelTravelsWithItsPage — /pending 554.
//
// `/PageLabels` is keyed by page INDEX, so unlike the outline it cannot be pruned: every kept page's
// label has to be re-stated at the position that page now occupies. The failure this closes is not a
// missing label but a WRONG one — a page that says it is page 7 while it is page 3.
func TestAPageLabelTravelsWithItsPage(t *testing.T) {
	src := pageLabelledFixture(t, 6)

	// SETUP: the fixture really does label its pages the two ways, or every case below compares two
	// documents that agree because neither has any labels.
	if got := labelsOf(t, src); len(got) != 6 || got[0] != "r1" || got[3] != "D1" {
		t.Fatalf("setup: the fixture's labels are %v, want six pages reading r1..r3 then D1..D3", got)
	}

	for _, c := range []struct {
		name   string
		order  []string
		want   []string
		ranges int
	}{
		// An order-preserving extract: source pages 4,5,6 are body pages 1,2,3 and stay so. One
		// range out, because the run collapses — this is the case the refused ceiling would never
		// have fired on and the one a user hits most.
		{"an extract keeps the labels of the pages it took", []string{"4-6"},
			[]string{"D1", "D2", "D3"}, 1},
		// Across the boundary, so the carry has to notice the style changing mid-selection.
		{"a selection spanning both ranges keeps both", []string{"3-5"},
			[]string{"r3", "D1", "D2"}, 2},
		// The reorder, and the whole decision: the label follows the PAGE. A positional reading
		// would answer r1, r2, r3 here — this document's own answer for those three pages is
		// D3, r1, D1, and inventing the other is a claim the user never made.
		{"a reorder carries each page's own label, not the position's", []string{"6", "1", "4"},
			[]string{"D3", "r1", "D1"}, 3},
		// A page named twice is two output pages, and the source's answer for both is the same
		// label. A duplicate label is what the source says; a renumbered one is not.
		{"a repeated page repeats its label", []string{"5", "5"},
			[]string{"D2", "D2"}, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := Collect(src, c.order)
			if err != nil {
				t.Fatal(err)
			}
			got := labelsOf(t, out)
			if len(got) != len(c.want) {
				t.Fatalf("got %d labels %v, want %d %v", len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("output page %d reads %q, want %q (all: %v)", i+1, got[i], c.want[i], got)
				}
			}
			if n := rangeCount(t, out); n != c.ranges {
				t.Errorf("the output holds %d /Nums range(s), want %d — the collapse rule is what "+
					"keeps an order-preserving subset from emitting one entry per page", n, c.ranges)
			}
		})
	}
}

// TestAPageTheSourceNeverLabelledIsNotGivenOne — the half a carry gets wrong by omission.
//
// A page before the source's first range carries no label. Emitting nothing for it in the output
// leaves it inheriting whatever range precedes it THERE, which is a label this document never gave
// it — the wrong-statement failure again, arriving through a gap rather than through a remap.
func TestAPageTheSourceNeverLabelledIsNotGivenOne(t *testing.T) {
	src, err := testpdf.Text("one", "two", "three", "four")
	if err != nil {
		t.Fatal(err)
	}
	// Labels start at page 3, so source pages 1 and 2 have none.
	if src, err = SetPageLabels(src, []PageLabelRange{{Start: 3, Style: "decimal", First: 7}}); err != nil {
		t.Fatal(err)
	}
	if got := labelsOf(t, src); got[0] != "-" || got[2] != "D7" {
		t.Fatalf("setup: the fixture reads %v, want the first two pages unlabelled and page 3 at D7", got)
	}

	// Source page 3 (label 7) first, then source page 1 (no label). If the unlabelled page emits no
	// entry it inherits the D-range above it and comes out reading "8".
	out, err := Collect(src, []string{"3", "1"})
	if err != nil {
		t.Fatal(err)
	}
	got := labelsOf(t, out)
	if len(got) != 2 || got[0] != "D7" {
		t.Fatalf("the labelled page came out as %v, want D7 first", got)
	}
	if got[1] != "-" {
		t.Errorf("a page the source never labelled came out reading %q — it inherited the range "+
			"above it in the OUTPUT, which is a label this document has never made", got[1])
	}
}

// TestTheWorstCasePageLabelCarryIsAffordable is the measurement the refused range ceiling rests on,
// and it is here rather than in a comment because `CLAUDE.md` says a claim carrying a number is
// measured or it is unmeasured.
//
// The worst case is a full reversal: every run of one, so the collapse rule saves nothing and the
// tree holds one entry per page. The ceiling that was considered — drop the labels past N ranges —
// would produce a document with NO labels where this produces one with every label right, so it has
// to be justified by a cost. The assertion is that the cost is not there.
//
// **The two sides are the SAME document**, one with its `/PageLabels` key deleted. An earlier cut
// compared the labelled fixture against a separately built blank one and measured the difference in
// their page CONTENT as well: it reported +23 KB and a labelled run that was FASTER than the
// unlabelled one, which is how a contaminated control announces itself.
//
// **Bounds, not the measurement itself**: the numbers move with the machine, so the test asserts a
// ceiling generous enough not to flake and prints what it actually saw. Observed here, 200 pages
// fully reversed into 200 ranges: **+6,582 bytes, identical on every run.**
//
// **No time cost is measurable, and the evidence is that the SIGN is not stable.** Over nine runs
// the labelled `Collect` was the faster of the two six times and the slower three (30.9-60.0 ms
// labelled against 35.0-50.9 ms without), which is a spread wider than any difference between them.
// An earlier note here claimed "faster in 4 of 4" off the first four runs; the next five refuted the
// direction, which is what a run-to-run difference does and a real cost does not. So the honest
// statement is that the carry's time cost is under the noise of this comparison — not that it is
// zero, and not a figure.
func TestTheWorstCasePageLabelCarryIsAffordable(t *testing.T) {
	const n = 200
	src := pageLabelledFixture(t, n)
	plain, err := writeMutated(src, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		delete(root, "PageLabels")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	order := make([]string, n)
	for i := range order {
		order[i] = fmt.Sprintf("%d", n-i)
	}

	start := time.Now()
	withLabels, err := Collect(src, order)
	if err != nil {
		t.Fatal(err)
	}
	withDur := time.Since(start)

	start = time.Now()
	without, err := Collect(plain, order)
	if err != nil {
		t.Fatal(err)
	}
	withoutDur := time.Since(start)

	ranges := rangeCount(t, withLabels)
	extra := len(withLabels) - len(without)
	t.Logf("%d pages fully reversed: %d /Nums ranges, %+d bytes, %v vs %v",
		n, ranges, extra, withDur, withoutDur)

	// SETUP: this really is the shattered case. Without it the affordability claim could be made by
	// a carry that collapsed everything into one range and measured nothing.
	if ranges != n {
		t.Fatalf("setup: a full reversal produced %d ranges, not %d — this is not the worst case "+
			"the refused ceiling was about", ranges, n)
	}
	if extra > 64*1024 {
		t.Errorf("one label range per page costs %d bytes on a %d-page document, which is the "+
			"order of magnitude that would make a range ceiling worth its own failure mode",
			extra, n)
	}
	if withDur > withoutDur+500*time.Millisecond {
		t.Errorf("the labelled reversal took %v against %v unlabelled", withDur, withoutDur)
	}
}
