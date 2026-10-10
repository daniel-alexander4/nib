package pdfops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The figure proposal — ADR-122: a figure is proposed for an image the page draws, and is written only with a
// description.

// figureFixture is an untagged one-page document drawing content in Helvetica, with an image XObject (`/Im0`,
// two samples square) and a form XObject that draws that image (`/Fm0`) in its resources.
func figureFixture(content string) []byte {
	form := "q 100 0 0 80 0 0 cm /Im0 Do Q"
	return assembleFixture(map[int]string{
		// The catalog names a language: a description is text, and with no language to read it in a Figure's
		// `/Alt` fails ua1 7.2 t22 as the page's own text fails 7.2 t34 (measured: ADR-122).
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R /Fm0 7 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream",
		7: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 100 80] /Resources << /XObject << /Im0 6 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
	})
}

// drawImage draws the image XObject w by h with its bottom-left corner at x, y; drawInline an inline image.
func drawImage(x, y, w, h float64) string {
	return fmt.Sprintf("q %g 0 0 %g %g %g cm /Im0 Do Q\n", w, h, x, y)
}

const inlineImage = "BI /W 2 /H 2 /CS /G /BPC 8 ID abcd\nEI"

func drawInline(x, y, w, h float64) string {
	return fmt.Sprintf("q %g 0 0 %g %g %g cm %s Q\n", w, h, x, y, inlineImage)
}

// figuresOf is the Figures a proposal holds; withoutFigures every other element, as `proposed` writes them.
func figuresOf(pr TagProposal) (out []TagElement) {
	for _, e := range pr.Elements {
		if e.Role == "Figure" {
			out = append(out, e)
		}
	}
	return out
}

func withoutFigures(pr TagProposal) (rows []string) {
	for _, e := range pr.Elements {
		if e.Role != "Figure" {
			rows = append(rows, fmt.Sprintf("%s %q list %d", e.Role, e.Text, e.List))
		}
	}
	return rows
}

// describing is a review that keeps every element as proposed and gives each Figure the description alt.
func describing(pr TagProposal, alt string) []TagReview {
	out := reviewAll(pr)
	for i, e := range pr.Elements {
		if e.Role == "Figure" {
			out[i].Alt = alt
		}
	}
	return out
}

// writtenContent is page 1's content as a written document holds it.
func writtenContent(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 1)
	src, err := pdfread.PageContent(ctx, pg.Dict, 1)
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// figuresRead is every Figure a written document's tree holds, as its description.
func figuresRead(t *testing.T, pdf []byte) (alts []string) {
	t.Helper()
	v, err := readStructureView(pdf)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range v.elements {
		if e.kind == "Figure" {
			if !e.hasAlt {
				t.Errorf("a Figure was written with no /Alt")
			}
			alts = append(alts, e.alt)
		}
	}
	return alts
}

// TestAnImageThePageDrawsIsProposedAsAFigure — decision 1 and 3: an image XObject and an inline image are each
// proposed as one Figure with no text, at the top level, whose rect is the image as drawn.
func TestAnImageThePageDrawsIsProposedAsAFigure(t *testing.T) {
	for _, c := range []struct {
		name, draw string
		want       [4]float64
	}{
		{"an image XObject", drawImage(50, 500, 100, 80), [4]float64{50, 500, 150, 580}},
		{"an inline image", drawInline(200, 300, 120, 90), [4]float64{200, 300, 320, 390}},
		// Drawn top to bottom, as most producers draw one: the box is the same box.
		{"an image drawn mirrored", "q 100 0 0 -80 50 580 cm /Im0 Do Q\n", [4]float64{50, 500, 150, 580}},
		// The least that is a figure.
		{"an image eight points square", drawImage(72, 500, 8, 8), [4]float64{72, 500, 80, 508}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rows, pr := proposed(t, figureFixture(drawText(72, 700, "Above the picture")+c.draw))
			if want := []string{`P^-1 "Above the picture"`, `Figure^-1 ""`}; !reflect.DeepEqual(rows, want) {
				t.Fatalf("proposed %v, want %v", rows, want)
			}
			fig := pr.Elements[1]
			if fig.Rect != c.want {
				t.Errorf("the figure's rect is %v, want the image as drawn, %v", fig.Rect, c.want)
			}
			if fig.Page != 1 || fig.List != -1 || fig.Marker != "" || fig.PageBox != [4]float64{0, 0, 612, 792} {
				t.Errorf("the figure is %+v", fig)
			}
			if len(pr.Unsupported) != 0 {
				t.Errorf("a figure was reported on its page: %v", pr.Unsupported)
			}
		})
	}
}

// TestWhatIsNotAPictureToDescribeIsNotAFigure — decision 1's refusals: each proposes no Figure and leaves every
// other element exactly as the page without the image proposes it.
func TestWhatIsNotAPictureToDescribeIsNotAFigure(t *testing.T) {
	text := drawText(72, 700, "Above") + drawText(72, 400, belowText)
	plain := withoutFigures(mustPropose(t, figureFixture(text)))
	if len(plain) != 2 {
		t.Fatalf("setup: the page with no image proposes %v", plain)
	}
	for _, c := range []struct{ name, draw, note string }{
		{"a bullet six points square", drawImage(50, 500, 6, 6), ""},
		{"narrower than a figure", drawImage(50, 500, 7.9, 80), ""},
		{"shorter than a figure", drawImage(50, 500, 100, 7.9), ""},
		{"drawn inside a form XObject", "q 1 0 0 1 50 500 cm /Fm0 Do Q\n", ""},
		{"inside an artifact", "/Artifact BMC\n" + drawImage(50, 500, 100, 80) + "EMC\n", ""},
		{"an inline image inside an artifact", "/Artifact <</Type /Layout>> BDC\n" + drawInline(50, 500, 100, 80) + "EMC\n", ""},
		{"inside marked content with an id", "/Figure <</MCID 0>> BDC\n" + drawImage(50, 500, 100, 80) + "EMC\n", ""},
		// Turned a sixth of a turn: the image fills a parallelogram, and what bounds it is not where it is.
		{"turned thirty degrees", "q 86.6 50 -40 69.28 300 500 cm /Im0 Do Q\n", figureTurned},
		{"slanted", "q 100 0 30 80 300 500 cm /Im0 Do Q\n", figureTurned},
	} {
		t.Run(c.name, func(t *testing.T) {
			pr := mustPropose(t, figureFixture(drawText(72, 700, "Above")+c.draw+drawText(72, 400, belowText)))
			if figs := figuresOf(pr); len(figs) != 0 {
				t.Errorf("proposed %d figure(s): %+v", len(figs), figs)
			}
			if got := withoutFigures(pr); !reflect.DeepEqual(got, plain) {
				t.Errorf("the rest of the page is proposed as %v, and without the image as %v", got, plain)
			}
			var notes []string
			for _, u := range pr.Unsupported {
				notes = append(notes, u.Reason)
			}
			if c.note == "" && len(notes) != 0 {
				t.Errorf("the page is reported: %v", notes)
			}
			if c.note != "" && !reflect.DeepEqual(notes, []string{c.note}) {
				t.Errorf("the page's notes are %v, want it to say %q", notes, c.note)
			}
		})
	}
}

// belowText is a line long enough that the short one above it is a paragraph of its own (`breaksBefore`).
const belowText = "Below the picture, in a line long enough to stand apart"

func mustPropose(t *testing.T, pdf []byte) TagProposal {
	t.Helper()
	pr, err := ProposeTags(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return pr
}

// TestAnImageTurnedAQuarterIsProposedWhereItIs — an image turned by a whole number of quarter turns still fills
// a rectangle along the axes, so its box is exact and it is proposed; any other turn is reported (above).
func TestAnImageTurnedAQuarterIsProposedWhereItIs(t *testing.T) {
	// The unit square under [0 80 -100 0 300 500]: x from 200 to 300, y from 500 to 580.
	pr := mustPropose(t, figureFixture(drawText(72, 700, "Above the picture")+"q 0 80 -100 0 300 500 cm /Im0 Do Q\n"))
	figs := figuresOf(pr)
	if len(figs) != 1 || figs[0].Rect != [4]float64{200, 500, 300, 580} {
		t.Fatalf("a quarter-turned image is proposed as %+v, want one figure at 200 500 300 580", figs)
	}
	if len(pr.Unsupported) != 0 {
		t.Errorf("a quarter turn was reported: %v", pr.Unsupported)
	}
	out, err := CommitTags(figureFixture(drawText(72, 700, "Above the picture")+"q 0 80 -100 0 300 500 cm /Im0 Do Q\n"), describing(pr, "Turned"))
	if err != nil {
		t.Fatal(err)
	}
	if got := figuresRead(t, out); !reflect.DeepEqual(got, []string{"Turned"}) {
		t.Errorf("the committed figures read %q", got)
	}
}

// TestFiguresSitInReadingOrder — decision 3: a figure sits where its top-left corner would be read, among the
// paragraphs and beside a table.
func TestFiguresSitInReadingOrder(t *testing.T) {
	t.Run("among paragraphs", func(t *testing.T) {
		// Drawn out of order: both images first, then the text.
		content := drawImage(72, 300, 100, 80) + drawInline(72, 520, 100, 80) +
			drawText(72, 700, "Above") + drawText(72, 450, "Between the two of them") + drawText(72, 200, belowText)
		rows, pr := proposed(t, figureFixture(content))
		want := []string{`P^-1 "Above"`, `Figure^-1 ""`, `P^-1 "Between the two of them"`, `Figure^-1 ""`, `P^-1 "` + belowText + `"`}
		if !reflect.DeepEqual(rows, want) {
			t.Fatalf("proposed %v, want %v", rows, want)
		}
		if a, b := pr.Elements[1].Rect, pr.Elements[3].Rect; a != [4]float64{72, 520, 172, 600} || b != [4]float64{72, 300, 172, 380} {
			t.Errorf("the figures are at %v and %v: the upper one is not first", a, b)
		}
	})
	t.Run("two figures with nothing between them", func(t *testing.T) {
		// Drawn lower one first, and right one first: top decides, then left.
		content := drawText(72, 700, "Above") + drawImage(72, 300, 100, 80) + drawImage(300, 500, 100, 80) + drawImage(72, 500, 100, 80)
		_, pr := proposed(t, figureFixture(content))
		var at [][4]float64
		for _, f := range figuresOf(pr) {
			at = append(at, f.Rect)
		}
		want := [][4]float64{{72, 500, 172, 580}, {300, 500, 400, 580}, {72, 300, 172, 380}}
		if !reflect.DeepEqual(at, want) {
			t.Errorf("the figures are proposed at %v, want %v", at, want)
		}
	})
	t.Run("beside a table", func(t *testing.T) {
		// One image whose top is above the table's, to its left; one below the table's top, to its left; and one
		// standing inside the grid, which is a Figure of its own after the table and not inside a cell.
		table := ruledLines() + cellTexts(sixWords...)
		content := table + drawImage(20, 660, 60, 60) + drawImage(20, 560, 60, 60) + drawImage(330, 615, 20, 20)
		rows, pr := proposed(t, figureFixture(content))
		var roles []string
		for _, e := range pr.Elements {
			if e.Parent == -1 {
				roles = append(roles, e.Role)
			}
		}
		if want := []string{"Figure", "Table", "Figure", "Figure"}; !reflect.DeepEqual(roles, want) {
			t.Fatalf("the top level is %v, want %v (%v)", roles, want, rows)
		}
		figs := figuresOf(pr)
		if figs[0].Rect != [4]float64{20, 660, 80, 720} || figs[1].Rect != [4]float64{330, 615, 350, 635} || figs[2].Rect != [4]float64{20, 560, 80, 620} {
			t.Errorf("the figures are at %v, %v, %v", figs[0].Rect, figs[1].Rect, figs[2].Rect)
		}
		// The table itself is what it was with no image on the page.
		plain, _ := proposed(t, figureFixture(table))
		var tableRows []string
		for _, r := range rows {
			if !strings.HasPrefix(r, "Figure") {
				// A table's elements name their parents by index, which the figure before the table moved along by one.
				tableRows = append(tableRows, r[strings.Index(r, " "):])
			}
		}
		for i, r := range plain {
			plain[i] = r[strings.Index(r, " "):]
		}
		if !reflect.DeepEqual(tableRows, plain) {
			t.Errorf("the table is proposed as %v, and with no image as %v", tableRows, plain)
		}
	})
}

// TestAFigureEndsNoList — decision 8: the items either side of a picture are proposed as the one list they were
// before a figure was ever proposed.
func TestAFigureEndsNoList(t *testing.T) {
	items := func(between string) string {
		return drawText(72, 700, "1. first item") + between + drawText(72, 500, "2. second item")
	}
	plain := withoutFigures(mustPropose(t, figureFixture(items(""))))
	pr := mustPropose(t, figureFixture(items(drawImage(72, 560, 100, 80))))
	if len(figuresOf(pr)) != 1 {
		t.Fatalf("setup: %d figure(s) proposed", len(figuresOf(pr)))
	}
	if got := withoutFigures(pr); !reflect.DeepEqual(got, plain) || !strings.Contains(strings.Join(got, "|"), "LI") {
		t.Errorf("with a picture between them the items are proposed as %v, and with none as %v", got, plain)
	}
}

// TestAPageOfPicturesAloneProposesNoFigure — the declared gap: a page with no text is a scan's page, reported
// as one, and nothing is proposed on it.
func TestAPageOfPicturesAloneProposesNoFigure(t *testing.T) {
	pr := mustPropose(t, figureFixture(drawImage(50, 500, 100, 80)))
	if len(pr.Elements) != 0 || !reflect.DeepEqual(pr.NoText, []int{1}) {
		t.Errorf("a page that draws only an image proposes %+v with no-text pages %v", pr.Elements, pr.NoText)
	}
}

// TestAFigureIsReviewedAsAFigureWithADescriptionOrIgnored — decision 4, each refusal by its own sentence.
func TestAFigureIsReviewedAsAFigureWithADescriptionOrIgnored(t *testing.T) {
	src := figureFixture(drawText(72, 700, "Above the picture") + drawImage(50, 500, 100, 80))
	pr := mustPropose(t, src)
	if rows, _ := proposed(t, src); !reflect.DeepEqual(rows, []string{`P^-1 "Above the picture"`, `Figure^-1 ""`}) {
		t.Fatalf("setup: proposed %v", rows)
	}
	for _, c := range []struct {
		name   string
		change func(r []TagReview)
		want   string
	}{
		{"kept with no description", func(r []TagReview) {}, "a figure needs a description of what it shows, or must be ignored"},
		{"a description of spaces", func(r []TagReview) { r[1].Alt = " \t\n " }, "a figure needs a description of what it shows, or must be ignored"},
		{"made a paragraph", func(r []TagReview) { r[1].Role, r[1].Alt = "P", "A chart" }, "a figure keeps its type"},
		{"made a table", func(r []TagReview) { r[1].Role, r[1].Alt = "Table", "A chart" }, "a figure keeps its type"},
		{"a paragraph made a figure", func(r []TagReview) { r[0].Role, r[1].Alt = "Figure", "A chart" }, "only a picture the page draws can be a Figure"},
		{"a description on a paragraph", func(r []TagReview) { r[0].Alt, r[1].Alt = "Words", "A chart" }, "only a figure takes a description"},
		{"a description on an ignored paragraph", func(r []TagReview) { r[0].Alt, r[0].Ignore, r[1].Alt = "Words", true, "A chart" }, "only a figure takes a description"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := reviewAll(pr)
			c.change(r)
			_, err := CommitTags(src, r)
			if !errors.Is(err, ErrTagsReview) || !strings.Contains(fmt.Sprint(err), c.want) {
				t.Errorf("the commit answered %v, want ErrTagsReview saying %q", err, c.want)
			}
		})
	}
	// What is allowed: described, ignored (with or without a description left in the field), and moved.
	for _, c := range []struct {
		name   string
		review func() []TagReview
		alts   []string
	}{
		{"described", func() []TagReview { return describing(pr, "A chart") }, []string{"A chart"}},
		{"ignored", func() []TagReview { r := reviewAll(pr); r[1].Ignore = true; return r }, nil},
		{"ignored with a description left behind", func() []TagReview { r := describing(pr, "A chart"); r[1].Ignore = true; return r }, nil},
		{"moved above the paragraph", func() []TagReview { r := describing(pr, "A chart"); return []TagReview{r[1], r[0]} }, []string{"A chart"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := CommitTags(src, c.review())
			if err != nil {
				t.Fatal(err)
			}
			if got := figuresRead(t, out); !reflect.DeepEqual(got, c.alts) {
				t.Errorf("the committed figures read %q, want %q", got, c.alts)
			}
			v, err := readStructureView(out)
			if err != nil {
				t.Fatal(err)
			}
			var kinds []string
			for _, e := range v.elements {
				kinds = append(kinds, e.kind)
			}
			want := []string{"P", "Figure"}
			switch c.name {
			case "ignored", "ignored with a description left behind":
				want = []string{"P"}
			case "moved above the paragraph":
				want = []string{"Figure", "P"}
			}
			if !reflect.DeepEqual(kinds, want) {
				t.Errorf("the tree holds %v, want %v", kinds, want)
			}
		})
	}
	// The only element on its page and ignored: there is nothing to commit, as for any element.
	if _, err := CommitTags(src, []TagReview{{ID: 0, Role: "P", Text: "Above the picture", Ignore: true}, {ID: 1, Role: "Figure", Ignore: true}}); !errors.Is(err, ErrTagsReview) {
		t.Errorf("ignoring everything answered %v", err)
	}
}

// TestAKeptFigureIsContentAndAnIgnoredOneAnArtifact — decision 5: a kept figure's operator sits inside the
// Figure's own marked-content sequence and in no artifact; an ignored one, and one too small to be proposed,
// inside an artifact. The description keeps every character.
func TestAKeptFigureIsContentAndAnIgnoredOneAnArtifact(t *testing.T) {
	const alt = `Ünïcode — 図 (a chart) with a back\slash and a ) alone`
	for _, c := range []struct{ name, draw, op string }{
		{"an image XObject", drawImage(50, 500, 100, 80), "/Im0 Do"},
		// An inline image is bracketed with the `q … cm` before it and the `Q` after it (`drawingBrackets`).
		{"an inline image", drawInline(50, 500, 100, 80), "q 100 0 0 80 50 500 cm " + inlineImage + " Q"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The tiny image is drawn FIRST, so its operator is not the one a careless match would find last.
			src := figureFixture("q 6 0 0 6 400 700 cm /Im0 Do Q\n" + drawText(72, 700, "Above the picture") + c.draw)
			pr := mustPropose(t, src)
			if len(figuresOf(pr)) != 1 {
				t.Fatalf("setup: %d figure(s) proposed", len(figuresOf(pr)))
			}
			kept, err := CommitTags(src, describing(pr, alt))
			if err != nil {
				t.Fatal(err)
			}
			if got := figuresRead(t, kept); !reflect.DeepEqual(got, []string{alt}) {
				t.Errorf("the description reads back as %q, want %q", got, alt)
			}
			content := writtenContent(t, kept)
			const tiny = "/Artifact BMC\n/Im0 Do\nEMC"
			if !strings.Contains(content, "/Figure <</MCID 1>> BDC\n"+c.op+"\nEMC") {
				t.Errorf("the kept figure's operator is not inside its own sequence:\n%s", content)
			}
			if strings.Count(content, "/Artifact") != 1 || !strings.Contains(content, tiny) {
				t.Errorf("want one artifact, the six-point image, and the kept figure in none:\n%s", content)
			}
			if n, err := UnmarkedTextRuns(kept); err != nil || n != 0 {
				t.Errorf("%d unmarked text run(s) after the commit (%v)", n, err)
			}

			r := reviewAll(pr)
			r[1].Ignore = true
			ignored, err := CommitTags(src, r)
			if err != nil {
				t.Fatal(err)
			}
			content = writtenContent(t, ignored)
			if !strings.Contains(content, "/Artifact BMC\n"+c.op+"\nEMC") || strings.Contains(content, "/Figure") || !strings.Contains(content, tiny) {
				t.Errorf("the ignored figure is not the artifact it would have been:\n%s", content)
			}
			if got := figuresRead(t, ignored); len(got) != 0 {
				t.Errorf("an ignored figure was written: %q", got)
			}
		})
	}
}

// TestAKeptFigureBetweenItemsPartsTheirList — lists follow the REVIEWED order: a figure that is kept is content
// between the two items, and one that is ignored is not there.
func TestAKeptFigureBetweenItemsPartsTheirList(t *testing.T) {
	src := figureFixture(drawText(72, 700, "1. first item") + drawImage(72, 560, 100, 80) + drawText(72, 500, "2. second item"))
	pr := mustPropose(t, src)
	lists := func(pdf []byte) (n int) {
		v, err := readStructureView(pdf)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range v.elements {
			if e.kind == "L" {
				n++
			}
		}
		return n
	}
	kept, err := CommitTags(src, describing(pr, "A chart"))
	if err != nil {
		t.Fatal(err)
	}
	r := reviewAll(pr)
	r[1].Ignore = true
	ignored, err := CommitTags(src, r)
	if err != nil {
		t.Fatal(err)
	}
	if k, i := lists(kept), lists(ignored); k != 2 || i != 1 {
		t.Errorf("%d list(s) with the figure kept and %d with it ignored, want 2 and 1", k, i)
	}
}

// TestAFigureThatIsNotWhereItWasProposedIsStale — decision 5: the commit matches a figure by the operator a
// fresh read of the page finds, and a proposal naming anything else is refused.
func TestAFigureThatIsNotWhereItWasProposedIsStale(t *testing.T) {
	src := figureFixture(drawText(72, 700, "Above the picture") + "50 600 m 300 600 l S\n" + drawImage(50, 500, 100, 80) +
		"/Artifact BMC q 90 0 0 70 300 300 cm /Im0 Do Q EMC\n")
	ctx, err := inspectionRead(src)
	if err != nil {
		t.Fatal(err)
	}
	p, err := proposeStructure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.elements) != 2 || p.elements[1].role != figureRole {
		t.Fatalf("setup: proposed %+v", p.elements)
	}
	content := writtenContent(t, src)
	fig := p.elements[1].figure
	if got := content[fig.start:fig.end]; got != "/Im0 Do" {
		t.Fatalf("setup: the figure's operator is %q", got)
	}
	commit := func(sp opSpan) error {
		els := append([]proposedElement(nil), p.elements...)
		els[1].figure, els[1].alt = sp, "A chart"
		_, err := commitProposal(src, els)
		return err
	}
	if err := commit(fig); err != nil {
		t.Fatalf("the proposal as proposed does not commit: %v", err)
	}
	covered := strings.LastIndex(content, "/Im0 Do")
	rule := strings.Index(content, "50 600 m")
	ruleEnd := strings.Index(content, "l S") + len("l S")
	for name, sp := range map[string]opSpan{
		"ending one byte on":                         {fig.start, fig.end + 1},
		"ending one byte short":                      {fig.start, fig.end - 1},
		"starting before the operator":               {fig.start - 1, fig.end},
		"naming the rule above it":                   {rule, ruleEnd},
		"naming an image an artifact already covers": {covered, covered + len("/Im0 Do")},
		"naming the text's own operator":             {p.elements[0].lines[0].runs[0].span.start, p.elements[0].lines[0].runs[0].span.end},
	} {
		if err := commit(sp); !errors.Is(err, errCommitStale) {
			t.Errorf("a figure %s answered %v, want the stale refusal", name, err)
		}
	}
}

// TestAProducersImageIsProposedAndCommitsAsAFigure — LibreOffice's own image: with its tags removed, exactly one
// Figure is proposed, on the image's page and where the page draws it; committed with a description it reads
// back as a Figure carrying it, and veraPDF fails neither 7.3 t1 nor 7.1 t3, and nothing the same document
// committed with the figure ignored does not fail.
func TestAProducersImageIsProposedAndCommitsAsAFigure(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is not installed, so no producer's image is proposed in this run; the hand-built images still run.")
	}
	original := tableAndFigureODT(t)
	truth, err := readStructureView(original)
	if err != nil {
		t.Fatal(err)
	}
	figurePage := 0
	for _, e := range truth.elements {
		if e.kind == "Figure" {
			if figurePage != 0 || e.alt != "A grey square" {
				t.Fatalf("setup: LibreOffice's figures are not the one described image (%+v)", e)
			}
			figurePage = e.page
		}
	}
	if figurePage == 0 {
		t.Fatal("setup: LibreOffice wrote no Figure")
	}
	src, err := RemoveStructure(original)
	if err != nil {
		t.Fatal(err)
	}
	rows, pr := proposed(t, src)
	figs := figuresOf(pr)
	if len(figs) != 1 || figs[0].Page != figurePage {
		t.Fatalf("%d figure(s) proposed, on %v; LibreOffice's own is one, on page %d (%v)", len(figs), figs, figurePage, rows)
	}
	// The frame was 2cm by 1.5cm — 56.7pt by 42.5pt — and the proposal's box is what the page draws.
	w, h := figs[0].Rect[2]-figs[0].Rect[0], figs[0].Rect[3]-figs[0].Rect[1]
	if w < 56 || w > 57.5 || h < 42 || h > 43 {
		t.Errorf("the figure's box is %.1f by %.1f pt, and the image was set 56.7 by 42.5", w, h)
	}
	ctx, err := inspectionRead(src)
	if err != nil {
		t.Fatal(err)
	}
	pr0, err := readPageShapes(ctx, pageAt(ctx, nil, figurePage))
	if err != nil {
		t.Fatal(err)
	}
	if len(pr0.images) != 1 || pr0.images[0].box != figs[0].Rect {
		t.Errorf("the page draws %+v and the figure is at %v", pr0.images, figs[0].Rect)
	}

	const alt = "A grey square, described again"
	kept, err := CommitTags(src, describing(pr, alt))
	if err != nil {
		t.Fatal(err)
	}
	if got := figuresRead(t, kept); !reflect.DeepEqual(got, []string{alt}) {
		t.Errorf("the committed figures read %q, want %q", got, []string{alt})
	}
	if n, err := UnmarkedTextRuns(kept); err != nil || n != 0 {
		t.Errorf("%d unmarked text run(s) after the commit (%v)", n, err)
	}
	r := reviewAll(pr)
	r[figs[0].ID].Ignore = true
	ignored, err := CommitTags(src, r)
	if err != nil {
		t.Fatal(err)
	}
	if got := figuresRead(t, ignored); len(got) != 0 {
		t.Errorf("an ignored figure was written: %q", got)
	}

	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the committed figure's ua1 clauses are UNCHECKED in this run.")
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"ignored.pdf": ignored, "figure.pdf": kept} {
		f := filepath.Join(dir, n)
		if werr := os.WriteFile(f, b, 0o600); werr != nil {
			t.Fatal(werr)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	before, after := cl["ignored.pdf"], cl["figure.pdf"]
	if before == nil || after == nil {
		t.Fatal("veraPDF could not validate one of the pair, so there is no differential")
	}
	t.Logf("committed with the figure ignored fails %v; with the figure %v", sortedClauses(before), sortedClauses(after))
	for _, c := range []string{"7.3 t1", "7.1 t3"} {
		if after[c] {
			t.Errorf("the committed figure fails %s", c)
		}
	}
	for c := range after {
		if !before[c] {
			t.Errorf("the committed figure fails %s, which the same document with the figure ignored does not", c)
		}
	}
}

// TestAHandBuiltFigureFailsNoFigureClause — the same differential over a hand-built page, for an image XObject
// and an inline image, so the clause is checked where LibreOffice is absent.
func TestAHandBuiltFigureFailsNoFigureClause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so a hand-built figure's ua1 clauses are UNCHECKED in this run.")
	}
	src := figureFixture(drawText(72, 700, "Above the pictures") + drawImage(50, 500, 100, 80) + drawInline(300, 500, 100, 80))
	pr := mustPropose(t, src)
	if len(figuresOf(pr)) != 2 {
		t.Fatalf("setup: %d figure(s) proposed", len(figuresOf(pr)))
	}
	kept, err := CommitTags(src, describing(pr, "A chart"))
	if err != nil {
		t.Fatal(err)
	}
	r := reviewAll(pr)
	r[1].Ignore, r[2].Ignore = true, true
	ignored, err := CommitTags(src, r)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"ignored.pdf": ignored, "figure.pdf": kept} {
		f := filepath.Join(dir, n)
		if werr := os.WriteFile(f, b, 0o600); werr != nil {
			t.Fatal(werr)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	before, after := cl["ignored.pdf"], cl["figure.pdf"]
	if before == nil || after == nil {
		t.Fatal("veraPDF could not validate one of the pair, so there is no differential")
	}
	t.Logf("hand-built, figures ignored fails %v; kept %v", sortedClauses(before), sortedClauses(after))
	for _, c := range []string{"7.3 t1", "7.1 t3"} {
		if after[c] {
			t.Errorf("the committed figures fail %s", c)
		}
	}
	for c := range after {
		if !before[c] {
			t.Errorf("the committed figures fail %s, which the same page with them ignored does not", c)
		}
	}
}

// TestTheWriterWritesNoFigureWithoutADescription — the writer's own half: whoever calls it, a Figure with
// nothing to say is not written.
func TestTheWriterWritesNoFigureWithoutADescription(t *testing.T) {
	src := figureFixture(drawText(72, 700, "Above the picture") + drawImage(50, 500, 100, 80))
	ctx, err := inspectionRead(src)
	if err != nil {
		t.Fatal(err)
	}
	p, err := proposeStructure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, alt := range []string{"", "  \t"} {
		p.elements[1].alt = alt
		if _, err := commitProposal(src, p.elements); err == nil || !strings.Contains(err.Error(), "only with a description") {
			t.Errorf("a figure described as %q was answered %v", alt, err)
		}
	}
}

// TestAnInlineImageIsBracketedWithItsOwnDrawing — `drawingBrackets`: pdfcpu ends an inline image only at an
// `EI` followed by `Q`, so a sequence about one opens before its `q … cm` and closes after its `Q`; every
// other drawing, and an inline image drawn any other way, keeps its own span.
func TestAnInlineImageIsBracketedWithItsOwnDrawing(t *testing.T) {
	for _, c := range []struct{ name, src, drawing, want string }{
		{"as a producer draws one", "0 g q 10 0 0 10 5 5 cm " + inlineImage + " Q 1 g", inlineImage, "q 10 0 0 10 5 5 cm " + inlineImage + " Q"},
		{"with new lines for spaces", "q\n10 0 0 10 5 5\ncm\n" + inlineImage + "\nQ\n", inlineImage, "q\n10 0 0 10 5 5\ncm\n" + inlineImage + "\nQ"},
		{"an image XObject", "q 10 0 0 10 5 5 cm /Im0 Do Q", "/Im0 Do", "/Im0 Do"},
		// Not an inline image, though it ends in the letters and stands where one would.
		{"a name that ends in the letters", "q 10 0 0 10 5 5 cm /EI Q", "/EI", "/EI"},
		{"no Q after it", "q 10 0 0 10 5 5 cm " + inlineImage + " 1 g Q", inlineImage, inlineImage},
		{"no cm before it", "q " + inlineImage + " Q", inlineImage, inlineImage},
		{"more than a cm before it", "q 10 0 0 10 5 5 cm 1 g " + inlineImage + " Q", inlineImage, inlineImage},
		{"six numbers and another operator before it", "q 1 2 3 4 5 6 c " + inlineImage + " Q", inlineImage, inlineImage},
		{"operators where the cm's numbers would be", "q 1 g 1 g 1 g cm " + inlineImage + " Q", inlineImage, inlineImage},
		{"five numbers before the cm", "q 0 0 10 5 5 cm " + inlineImage + " Q", inlineImage, inlineImage},
		{"no q before the cm", "1 g 10 0 0 10 5 5 cm " + inlineImage + " Q", inlineImage, inlineImage},
		{"nothing before it", inlineImage + " Q", inlineImage, inlineImage},
		{"nothing after it", "q 10 0 0 10 5 5 cm " + inlineImage, inlineImage, inlineImage},
	} {
		at := strings.Index(c.src, c.drawing)
		b := drawingBrackets{src: []byte(c.src)}
		got := b.around(opSpan{at, at + len(c.drawing)})
		if c.src[got.start:got.end] != c.want {
			t.Errorf("%s: bracketed %q, want %q", c.name, c.src[got.start:got.end], c.want)
		}
	}
}

// TestAnIgnoredInlineImageCommitsAsAnArtifact — the page an inline image is on could not be committed at all
// before ADR-122: the artifact written about it at its own span is one pdfcpu refuses.
func TestAnIgnoredInlineImageCommitsAsAnArtifact(t *testing.T) {
	src := figureFixture(drawText(72, 700, "Above the picture") + drawInline(50, 500, 100, 80) + drawText(72, 300, belowText))
	pr := mustPropose(t, src)
	r := reviewAll(pr)
	for i, e := range pr.Elements {
		r[i].Ignore = e.Role == "Figure"
	}
	out, err := CommitTags(src, r)
	if err != nil {
		t.Fatal(err)
	}
	if content := writtenContent(t, out); !strings.Contains(content, "/Artifact BMC\nq 100 0 0 80 50 500 cm "+inlineImage+" Q\nEMC") {
		t.Errorf("the inline image is not an artifact with its own drawing:\n%s", content)
	}
	if n, err := uncoveredDrawings(out); err != nil || n != 0 {
		t.Errorf("%d drawing(s) no sequence covers after the commit (%v)", n, err)
	}
}

// TestTheOCRDoorBracketsAnInlineImageWithItsOwnDrawing — the same door at its other writer: the OCR door's
// artifact about an inline image is one pdfcpu can read back.
func TestTheOCRDoorBracketsAnInlineImageWithItsOwnDrawing(t *testing.T) {
	src := figureFixture(drawText(72, 700, "Above the picture") + drawInline(50, 500, 100, 80))
	out, err := writeMutated(src, func(ctx *model.Context) error {
		return artifactUncoveredDrawings(ctx, pageAt(ctx, nil, 1))
	})
	if err != nil {
		t.Fatal(err)
	}
	if content := writtenContent(t, out); !strings.Contains(content, "/Artifact BMC\nq 100 0 0 80 50 500 cm "+inlineImage+" Q\nEMC") {
		t.Errorf("the inline image is not an artifact with its own drawing:\n%s", content)
	}
}
