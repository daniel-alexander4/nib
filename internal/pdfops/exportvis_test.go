package pdfops

import "testing"

// opaqueImage covers only an image that nothing stops hiding what is below it: each condition alone turns it into a
// non-cover (PLAN-import-export X6).
func TestOnlyAnUnqualifiedImageCoversText(t *testing.T) {
	ok := drawnImage{upright: true}
	if !opaqueImage(ok) {
		t.Fatal("the control: an upright plain image must cover")
	}
	for name, im := range map[string]drawnImage{
		"translucent": {upright: true, translucent: true},
		"clipped":     {upright: true, clipped: true},
		"layer off":   {upright: true, layerOff: true},
		"turned":      {upright: false},
		"stencil":     {upright: true, cause: PlaceholderStencil},
		"masked":      {upright: true, cause: PlaceholderMasked},
		"soft-masked": {upright: true, cause: PlaceholderSoftMask},
	} {
		if opaqueImage(im) {
			t.Errorf("%s image covers text", name)
		}
	}
}

func visRun(seq int, x, y, w, size float64, tr int) textRun {
	r := textRun{text: "w", decoded: true, x: x, y: y, width: w, size: size, seq: seq}
	r.state.tr = tr
	return r
}

// Two visible boxes that overlap each other are one cover: summing each one's overlap with the hidden run counted the
// shared part twice (/pending 941 #2).
func TestOverlappingVisibleTextCountsOnceAgainstHiddenText(t *testing.T) {
	big := [4]float64{0, 0, 1000, 1000}
	hidden := visRun(3, 0, 0, 100, 10, 3)
	// Two visible runs on the same spot: each covers 60% of the hidden box's width, together 60%, not 120%.
	a, b := visRun(1, 0, 0, 60, 10, 0), visRun(2, 0, 0, 60, 10, 0)
	pr := pageRuns{runs: []textRun{a, b, hidden}}
	v := visibleRuns(&pr, big)
	if v.HiddenText != 1 || len(pr.runs) != 2 {
		t.Fatalf("60%% covered twice over: HiddenText=%d runs=%d, want the hidden run dropped (control)", v.HiddenText, len(pr.runs))
	}
	// Two visible runs on the same 40% of the hidden box: 80% summed, 40% real.
	a, b = visRun(1, 0, 0, 40, 10, 0), visRun(2, 0, 0, 40, 10, 0)
	pr = pageRuns{runs: []textRun{a, b, hidden}}
	v = visibleRuns(&pr, big)
	if v.HiddenText != 0 || len(pr.runs) != 3 {
		t.Fatalf("40%% covered twice over: HiddenText=%d runs=%d, want the hidden run kept", v.HiddenText, len(pr.runs))
	}
}

func TestUnionAreaCountsAnOverlapOnce(t *testing.T) {
	got := unionArea([][4]float64{{0, 0, 10, 10}, {5, 5, 15, 15}, {0, 0, 10, 10}})
	if got != 175 {
		t.Fatalf("union = %v, want 175", got)
	}
}

// The hidden-text pass is bounded like the cover test: past the budget the hidden run is kept and counted untested.
func TestHiddenTextPassHasABudget(t *testing.T) {
	defer func(b int) { coverBudget = b }(coverBudget)
	coverBudget = 0
	big := [4]float64{0, 0, 1000, 1000}
	pr := pageRuns{runs: []textRun{visRun(1, 0, 0, 100, 10, 0), visRun(2, 0, 0, 100, 10, 3)}}
	v := visibleRuns(&pr, big)
	if v.HiddenText != 0 || len(pr.runs) != 2 || v.CoverUntested == 0 {
		t.Fatalf("over budget: HiddenText=%d runs=%d untested=%d", v.HiddenText, len(pr.runs), v.CoverUntested)
	}
}

// A covering rectangle that holds the horizontal strip an upright run occupies does not hold a turned or vertical one:
// its box is not that strip (/pending 941 #7). The vertical run's width is a horizontal sum that says nothing of how
// far down its codes run.
func TestACoverBesideATurnedOrVerticalRunDoesNotDropIt(t *testing.T) {
	big := [4]float64{-1000, -1000, 1000, 1000}
	for name, c := range map[string]struct {
		mod     func(*textRun)
		cover   [4]float64
		dropped bool
	}{
		"upright":  {func(r *textRun) {}, [4]float64{90, 90, 125, 125}, true},
		"turned":   {func(r *textRun) { r.rotated = true }, [4]float64{90, 90, 125, 125}, false},
		"vertical": {func(r *textRun) { r.vertical = true; r.codes = 5 }, [4]float64{70, 70, 130, 130}, false},
	} {
		r := visRun(1, 100, 100, 10, 10, 0)
		c.mod(&r)
		cover := pageShape{rect: true, filled: true, seq: 2, box: c.cover}
		pr := pageRuns{runs: []textRun{r}, shapes: []pageShape{cover}}
		v := visibleRuns(&pr, big)
		if (v.Covered == 1) != c.dropped || (len(pr.runs) == 0) != c.dropped {
			t.Errorf("%s: Covered=%d runs=%d, dropped want %v", name, v.Covered, len(pr.runs), c.dropped)
		}
	}
}

// The unions of a page share one budget: past it the remaining hidden text is kept and counted untested, so a page of
// thousands of hidden words over hundreds of overlapping boxes cannot cost minutes (/pending 941 #2, the review).
func TestTheUnionsOfAPageShareABudget(t *testing.T) {
	defer func(old int) { unionBudget = old }(unionBudget)
	big := [4]float64{0, 0, 1000, 1000}
	build := func() pageRuns {
		runs := []textRun{visRun(1, 0, 0, 100, 10, 0), visRun(2, 0, 0, 100, 10, 0)}
		for i := 0; i < 4; i++ {
			runs = append(runs, visRun(3+i, 0, 0, 100, 10, 3))
		}
		return pageRuns{runs: runs}
	}
	unionBudget = 500_000_000
	pr := build()
	if v := visibleRuns(&pr, big); v.HiddenText != 4 || v.CoverUntested != 0 {
		t.Fatalf("within the budget: HiddenText=%d CoverUntested=%d, want 4 and 0", v.HiddenText, v.CoverUntested)
	}
	unionBudget = 0
	pr = build()
	v := visibleRuns(&pr, big)
	if v.HiddenText != 1 || v.CoverUntested != 3 || len(pr.runs) != 5 {
		t.Errorf("past the budget: HiddenText=%d CoverUntested=%d runs=%d, want 1, 3 and 5 (two visible, three kept)", v.HiddenText, v.CoverUntested, len(pr.runs))
	}
}

// A mirrored run advances backwards: its width is negative, and a reach taken from it must not turn the box inside out,
// or every box near the run would "hold" it and drop visible text.
func TestACoverNearAMirroredTurnedRunDoesNotDropIt(t *testing.T) {
	big := [4]float64{-1000, -1000, 1000, 1000}
	r := visRun(1, 100, 100, 10, 10, 0)
	r.rotated = true
	r.width = -500
	cover := pageShape{rect: true, filled: true, seq: 2, box: [4]float64{20, 20, 30, 30}}
	pr := pageRuns{runs: []textRun{r}, shapes: []pageShape{cover}}
	if v := visibleRuns(&pr, big); v.Covered != 0 || len(pr.runs) != 1 {
		t.Errorf("a small box far from the run dropped it: Covered=%d runs=%d", v.Covered, len(pr.runs))
	}
	if b := inkBox(r); b[0] > b[2] || b[1] > b[3] {
		t.Errorf("the box is inside out: %v", b)
	}
}

// A hidden run that more than maxCoverPieces visible boxes meet is kept and counted untested: the union is not worked out
// over an unbounded number of pieces.
func TestHiddenTextOverTooManyPiecesIsKeptAndCounted(t *testing.T) {
	big := [4]float64{0, 0, 1000, 1000}
	runs := []textRun{}
	for i := 0; i <= maxCoverPieces; i++ {
		runs = append(runs, visRun(i+1, 0, 0, 100, 10, 0))
	}
	runs = append(runs, visRun(maxCoverPieces+2, 0, 0, 100, 10, 3))
	pr := pageRuns{runs: runs}
	v := visibleRuns(&pr, big)
	if v.HiddenText != 0 || v.CoverUntested != 1 || len(pr.runs) != len(runs) {
		t.Fatalf("HiddenText=%d CoverUntested=%d runs=%d of %d", v.HiddenText, v.CoverUntested, len(pr.runs), len(runs))
	}
}

// A turned run is off the page only when the square that holds it however it points is (/pending 946 I11): one set upward
// from below the bottom edge has its origin off the page and 150 pt of its body on it. The controls: the same run kept
// upright is dropped there (its box is the strip below the page), and a turned run wholly past the edge is dropped.
func TestATurnedRunPartlyOnThePageIsNotOffPage(t *testing.T) {
	page := [4]float64{0, 0, 612, 792}
	run := func(x, y float64, rotated bool) textRun {
		r := visRun(1, x, y, 200, 10, 0)
		r.rotated = rotated
		return r
	}
	for name, c := range map[string]struct {
		r    textRun
		kept bool
	}{
		"turned, origin below the page, body on it": {run(100, -50, true), true},
		"upright at the same origin (control)":      {run(100, -50, false), false},
		"turned, wholly past the edge (control)":    {run(100, -500, true), false},
	} {
		pr := pageRuns{runs: []textRun{c.r}}
		v := visibleRuns(&pr, page)
		if kept := len(pr.runs) == 1; kept != c.kept || (v.OffPage == 1) == c.kept {
			t.Errorf("%s: kept=%v OffPage=%d, want kept=%v", name, kept, v.OffPage, c.kept)
		}
	}
}
