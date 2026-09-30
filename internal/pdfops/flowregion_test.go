package pdfops

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// TestEveryMarkKindIsBoxed — `PLAN-text-reflow.md` P07.S01: reflow's reader boxes every non-text mark a page draws, in
// user space, through the CTM and a form's matrix; a clip draws nothing; and no other reader pays for it.
func TestEveryMarkKindIsBoxed(t *testing.T) {
	const form = "q 30 0 0 40 5 5 cm /Im1 Do Q 0 0 10 10 re f" // an image and a path, inside a form
	content := strings.Join([]string{
		"q 2 0 0 2 10 20 cm 0 0 50 10 re f Q",                                                // a filled rectangle, scaled and moved
		"q 5 w 100 100 m 200 100 l S Q",                                                      // a stroke, grown by half its width
		"q 0 0 612 792 re W n Q",                                                             // a clip: nothing drawn
		"q 300 300 m 310 350 320 250 330 300 c f Q",                                          // a curve, bounded by its control points
		"q 30 0 0 40 300 400 cm /Im1 Do Q",                                                   // an image XObject
		"q 10 0 0 10 50 50 cm BI /W 1 /H 1 /CS /G /BPC 8 ID A EI Q",                          // an inline image
		"q 1 0 0 1 400 500 cm /Fm1 Do Q", "q 0 0 5 5 re B Q", "q 1 0 0 1 0 600 cm /Fm2 Do Q", // a form drawing a path
		"/Sh1 sh", // a shading
	}, "\n")
	pdf := pageWith(content, "/XObject << /Im1 6 0 R /Fm1 7 0 R /Fm2 9 0 R >> /Shading << /Sh1 8 0 R >>", "", "", nil,
		"<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\nA\nendstream",
		fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 100 100] /Matrix [1 0 0 1 5 5] /Length %d >>\nstream\n%s\nendstream", len(form), form),
		"<< /ShadingType 2 /ColorSpace /DeviceGray /Coords [0 0 1 0] /Function << /FunctionType 2 /Domain [0 1] /C0 [0] /C1 [1] /N 1 >> >>",
		// a form whose content cannot be decoded: the walk does not enter it, so its /BBox is its box
		"<< /Type /XObject /Subtype /Form /BBox [0 0 20 30] /Matrix [1 0 0 1 10 10] /Filter /FlateDecode /Length 4 >>\nstream\nnope\nendstream")
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageGlyphRuns(ctx, pageAt(ctx, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	inf := math.Inf(1)
	want := []pageMark{
		{markPath, [4]float64{10, 20, 110, 40}},
		{markPath, [4]float64{97.5, 97.5, 202.5, 102.5}},
		{markPath, [4]float64{300, 250, 330, 350}},
		{markImage, [4]float64{300, 400, 330, 440}},
		{markInlineImage, [4]float64{50, 50, 60, 60}},
		{markImage, [4]float64{410, 510, 440, 550}},
		{markPath, [4]float64{405, 505, 415, 515}},
		{markPath, [4]float64{-0.5, -0.5, 5.5, 5.5}},
		{markForm, [4]float64{10, 610, 30, 640}},
		{markShading, [4]float64{-inf, -inf, inf, inf}},
	}
	if fmt.Sprint(pr.marks) != fmt.Sprint(want) {
		t.Errorf("marks\n  got  %v\n  want %v", pr.marks, want)
	}
	plain, err := readPageRuns(ctx, pageAt(ctx, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.marks) != 0 {
		t.Errorf("a reader that did not ask kept %d marks", len(plain.marks))
	}
}

// flowPage is a Helvetica page of three twelve-point paragraphs down a column — lines 14 points apart, every line full
// so only the 24-point paragraph step breaks them — followed by extra.
func flowPage(extra string) []byte {
	return pageWith(flowPageContent+extra, "", "", "", helvetica)
}

var flowPageContent = func() string {
	line := "(Words run across the column here and wrap) Tj T*"
	para := func(y, n int) string {
		return fmt.Sprintf("BT /F1 12 Tf 14 TL 72 %d Td %s ET\n", y, strings.Repeat(line, n))
	}
	return para(700, 3) + para(648, 2) + para(610, 2)
}()

func regionFor(t *testing.T, pdf []byte, pi int) (flowRegion, pageLayout) {
	t.Helper()
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 1)
	l, err := readPageGlyphLayout(ctx, pg)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.paragraphs) <= pi {
		t.Fatalf("%d paragraphs, wanted paragraph %d", len(l.paragraphs), pi)
	}
	return regionOf(l, pi, visibleBoxOf(pg)), l
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// TestTheRoomBelowAParagraph — P07.S01: the region is the paragraphs below one in its column up to a wide gap; the room is
// bounded by what lies past that gap, or by the page's bottom margin when nothing does; and a mark drawn where the region
// will move — a rule, an artifact's text — is named.
func TestTheRoomBelowAParagraph(t *testing.T) {
	t.Run("a footer past a wide gap stays put and bounds the room", func(t *testing.T) {
		r, l := regionFor(t, flowPage("BT /F1 12 Tf 300 100 Td (Page 1) Tj ET"), 0)
		if len(l.paragraphs) != 4 {
			t.Fatalf("%d paragraphs, want 4", len(l.paragraphs))
		}
		if fmt.Sprint(r.paragraphs) != "[1 2]" {
			t.Fatalf("region %v, want [1 2]", r.paragraphs)
		}
		// The footer's top is 100 + 12, the floor an em above it — higher than the 80-point margin, so it binds.
		if r.bound != roomBelowContent || !near(r.floor, 124) || !near(r.room, 593-124) {
			t.Errorf("bound %s floor %v room %v, want content-below 124 %v", r.bound, r.floor, r.room, 593.0-124)
		}
		if len(r.marks) != 0 {
			t.Errorf("marks %v, want none", r.marks)
		}
	})
	t.Run("a footer inside the bottom margin leaves the margin binding", func(t *testing.T) {
		r, _ := regionFor(t, flowPage("BT /F1 12 Tf 300 40 Td (Page 1) Tj ET"), 0)
		if r.bound != roomBelowMargin || !near(r.floor, 80) {
			t.Errorf("bound %s floor %v, want page-margin 80", r.bound, r.floor)
		}
	})
	t.Run("a fill behind the whole band is a backdrop, not an obstacle", func(t *testing.T) {
		r, _ := regionFor(t, flowPage("q 0.9 g 0 0 612 792 re f Q"), 0)
		if len(r.marks) != 0 {
			t.Errorf("marks %v, want none", r.marks)
		}
	})
	t.Run("a fill behind the column alone is a backdrop too", func(t *testing.T) {
		r, _ := regionFor(t, flowPage("q 0.9 g 60 20 360 740 re f Q"), 0)
		if len(r.marks) != 0 {
			t.Errorf("marks %v, want none", r.marks)
		}
	})
	t.Run("a shading is never a backdrop", func(t *testing.T) {
		pdf := pageWith(flowPageContent+"/Sh1 sh", "/Shading << /Sh1 6 0 R >>", "", "", helvetica,
			"<< /ShadingType 2 /ColorSpace /DeviceGray /Coords [0 0 1 0] /Function << /FunctionType 2 /Domain [0 1] /C0 [0] /C1 [1] /N 1 >> >>")
		r, _ := regionFor(t, pdf, 0)
		if len(r.marks) != 1 || r.marks[0].kind != markShading {
			t.Errorf("marks %v, want one shading", r.marks)
		}
	})
	t.Run("with nothing below, the floor mirrors the top margin", func(t *testing.T) {
		r, _ := regionFor(t, flowPage(""), 0)
		// The highest line's top is 700 + 12, so the top margin is 80.
		if r.bound != roomBelowMargin || !near(r.floor, 80) || !near(r.room, 593-80) {
			t.Errorf("bound %s floor %v room %v, want page-margin 80 %v", r.bound, r.floor, r.room, 593.0-80)
		}
	})
	t.Run("the last paragraph's region is empty and its own bottom is the start", func(t *testing.T) {
		r, _ := regionFor(t, flowPage(""), 2)
		if len(r.paragraphs) != 0 || !near(r.bottom, 593) {
			t.Errorf("region %v bottom %v, want [] 593", r.paragraphs, r.bottom)
		}
	})
	t.Run("a rule between paragraphs is a mark in the band", func(t *testing.T) {
		r, _ := regionFor(t, flowPage("q 1 w 72 660 m 400 660 l S Q"), 0)
		if len(r.marks) != 1 || r.marks[0].kind != markPath {
			t.Errorf("marks %v, want one path", r.marks)
		}
	})
	t.Run("a rule below the region bounds the room and is not in the band", func(t *testing.T) {
		r, _ := regionFor(t, flowPage("q 1 w 72 500 m 400 500 l S Q"), 0)
		if r.bound != roomBelowContent || !near(r.floor, 500.5+12) || len(r.marks) != 0 {
			t.Errorf("bound %s floor %v marks %v, want content-below 512.5 and none", r.bound, r.floor, r.marks)
		}
	})
	t.Run("an artifact's text in the band is a mark", func(t *testing.T) {
		r, _ := regionFor(t, flowPage("/Artifact BMC BT /F1 12 Tf 100 655 Td (Draft) Tj ET EMC"), 0)
		if len(r.marks) != 1 || r.marks[0].kind != markText {
			t.Errorf("marks %v, want one text", r.marks)
		}
	})
	t.Run("a run that shows only a space draws no ink and is not a mark", func(t *testing.T) {
		r, _ := regionFor(t, flowPage("BT /F1 12 Tf 100 655 Td ( ) Tj ET"), 0)
		if len(r.marks) != 0 {
			t.Errorf("marks %v, want none", r.marks)
		}
	})
	t.Run("text in another column is not in the band", func(t *testing.T) {
		r, _ := regionFor(t, flowPage("BT /F1 12 Tf 470 640 Td (Aside) Tj ET"), 0)
		if len(r.marks) != 0 {
			t.Errorf("marks %v, want none", r.marks)
		}
	})
}

// linePitch is a paragraph's baseline-to-baseline step — its lines' median, or 1.2 em for a paragraph of one line.
func censusPitch(p textParagraph) float64 {
	if len(p.lines) < 2 {
		return 1.2 * p.lines[0].size
	}
	var steps []float64
	for i := 1; i < len(p.lines); i++ {
		steps = append(steps, p.lines[i-1].y-p.lines[i].y)
	}
	return median(steps)
}

// TestHowManyParagraphsCanGrow — P07.S01's census: over the generated and real-producer corpora, of the paragraphs P06
// can reflow, how many have room below them for one more line with nothing drawn in the way — and why the others do not.
// The figure is the phase's population, printed so later slices can state what they unlocked.
//
// **The stimulus is asserted**: both answers are reached, and the reader saw marks at all — a corpus it boxed nothing in
// would make "no mark in the band" vacuous.
func TestHowManyParagraphsCanGrow(t *testing.T) {
	corpora := []lawOneCorpus{
		// Hand-built pages that reach "cannot" — a rule in the band, a footer close below — so a fresh clone without the
		// external corpus still asks both questions.
		{name: "generated", docs: append(runCorpus(t),
			runCorpusDoc{"a rule between paragraphs", flowPage("q 1 w 72 660 m 400 660 l S Q")},
			runCorpusDoc{"a footer close below", flowPage("BT /F1 12 Tf 72 558 Td (Page 1) Tj ET")})},
		externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers")),
	}
	allCan, allCannot, allMarks := 0, 0, 0
	for _, corp := range corpora {
		if corp.absent != "" {
			t.Logf("NOTE (a narrower population, not a pass over it): %s: %s", corp.name, corp.absent)
			continue
		}
		can, marks := 0, 0
		why := map[string]int{}
		for _, doc := range corp.docs {
			ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
			if err != nil {
				continue
			}
			for p := 1; p <= ctx.PageCount && p <= 2; p++ {
				pg := pageAt(ctx, nil, p)
				l, err := readPageGlyphLayout(ctx, pg)
				if err != nil {
					continue
				}
				marks += len(l.marks)
				for pi, para := range l.paragraphs {
					if paragraphRefusal(ctx, pg, para) != "" {
						continue
					}
					r := regionOf(l, pi, visibleBoxOf(pg))
					switch {
					case len(r.marks) > 0:
						why["mark in the band: "+string(r.marks[0].kind)]++
					case r.room < censusPitch(para):
						why["no room below: "+r.bound]++
					default:
						can++
					}
				}
			}
		}
		var reasons []string
		cannot := 0
		for k, n := range why {
			reasons = append(reasons, fmt.Sprintf("%s %d", k, n))
			cannot += n
		}
		sort.Strings(reasons)
		t.Logf("%s: %d of %d reflowable paragraphs have room for one more line; the rest: %s; %d marks boxed",
			corp.name, can, can+cannot, strings.Join(reasons, ", "), marks)
		allCan, allCannot, allMarks = allCan+can, allCannot+cannot, allMarks+marks
	}
	if allCan == 0 || allCannot == 0 {
		t.Errorf("can %d, cannot %d — the census must reach both answers", allCan, allCannot)
	}
	if allMarks == 0 {
		t.Error("no mark was boxed anywhere in the corpora")
	}
}
