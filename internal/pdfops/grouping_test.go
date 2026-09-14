package pdfops

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Lines and paragraphs — `PLAN-accessibility.md` P08.S03 (`PLAN-text-reflow.md` P04).

func run(text string, x, y, width, size float64) textRun {
	return textRun{text: text, x: x, y: y, width: width, size: size, decoded: true}
}

func paragraphTexts(l pageLayout) []string {
	out := make([]string, len(l.paragraphs))
	for i, p := range l.paragraphs {
		out[i] = p.text()
	}
	return out
}

// TestRunsJoinIntoLinesAndAColumnGapSplitsThem — the measured distances: a bullet 1.3 em from its text
// joins; a 3 em column gap on the same baseline does not.
func TestRunsJoinIntoLinesAndAColumnGapSplitsThem(t *testing.T) {
	segs := lineSegments([]textRun{
		run("second item with ", 90, 624.6, 86.17, 11),
		run("•", 72, 624.6, 3.7, 11),
		run("bold ", 176.17, 624.6, 24.24, 11),
		run(" ", 300, 624.6, 3, 11), // a whitespace run carries no text and joins nothing
	})
	if len(segs) != 1 || segs[0].text != "• second item with bold" {
		t.Fatalf("a bullet line read as %+v, want one line \"• second item with bold\"", segs)
	}
	// An artifact on the same baseline — a watermark crossing the line — is not part of it.
	stamped := lineSegments([]textRun{
		run("real text", 72, 500, 60, 12),
		{text: "DRAFT", x: 132, y: 500, width: 60, size: 12, artifact: true},
	})
	if len(stamped) != 1 || stamped[0].text != "real text" {
		t.Errorf("an artifact run joined a line: %+v", stamped)
	}
	cols := lineSegments([]textRun{
		run("left column", 56.8, 685.4, 229.4, 12),
		run("right column", 324.1, 685.4, 231.1, 12),
	})
	if len(cols) != 2 {
		t.Errorf("two columns on one baseline read as %d line(s), want 2 — a 3 em gap fused them", len(cols))
	}
}

// TestEachParagraphSignalBreaksAndARaggedLineDoesNot — the four signals, each alone, and the case the
// short-line signal exists to get right.
func TestEachParagraphSignalBreaksAndARaggedLineDoesNot(t *testing.T) {
	body := func(y, x, w float64, text string) textRun { return run(text, x, y, w, 12) }
	for _, c := range []struct {
		name string
		runs []textRun
		want int
	}{
		{"a size change", []textRun{run("Heading", 72, 700, 300, 20), body(680, 72, 300, "Body text that fills the line")}, 2},
		{"a wide vertical step", []textRun{
			body(700, 72, 300, "one line that fills the measure"), body(686, 72, 300, "two line that fills the measure"),
			body(672, 72, 300, "three line that fills the measure"), body(640, 72, 300, "four line that fills the measure")}, 2},
		{"an indent", []textRun{body(700, 72, 300, "a line that fills the measure"), body(686, 90, 282, "indented line that fills")}, 2},
		{"a short line the next word would have fitted", []textRun{
			body(700, 72, 300, "a line that fills the measure"), body(686, 72, 100, "ends here."),
			body(672, 72, 300, "Next paragraph that fills it")}, 2},
		{"a ragged line whose next word would NOT have fitted", []textRun{
			body(700, 72, 300, "a line that fills the measure"), body(686, 72, 276, "a line that stops a little"),
			body(672, 72, 300, "extraordinarily long word begins")}, 1},
	} {
		if got := len(groupRuns(c.runs).paragraphs); got != c.want {
			t.Errorf("%s: %d paragraph(s), want %d", c.name, got, c.want)
		}
	}
}

// TestALayoutTheRuleCannotReadIsReportedNotGuessed — exit criterion 3's other half: a full-width line
// spanning two columns fuses them into one, and the side-by-side lines that result are reported.
func TestALayoutTheRuleCannotReadIsReportedNotGuessed(t *testing.T) {
	l := groupRuns([]textRun{
		run("A title that spans the whole page width across both columns", 56, 720, 500, 12),
		run("left column line", 56.8, 685.4, 229.4, 12),
		run("right column line", 324.1, 685.4, 231.1, 12),
	})
	if l.unsupported == "" {
		t.Errorf("a spanning line fused two columns and nothing was reported: %+v", paragraphTexts(l))
	}
	clean := groupRuns([]textRun{run("left", 56.8, 685.4, 229.4, 12), run("right", 324.1, 685.4, 231.1, 12)})
	if clean.unsupported != "" || clean.columns != 2 {
		t.Errorf("two separable columns read as columns=%d unsupported=%q", clean.columns, clean.unsupported)
	}
}

// TestParagraphCountsMatchTheHandCheckedCorpus — P04's first exit criterion, on documents whose source
// states the paragraphs, with each paragraph's text checked against that source.
func TestParagraphCountsMatchTheHandCheckedCorpus(t *testing.T) {
	type page struct {
		columns int
		texts   []string
	}
	cases := []struct {
		name  string
		pdf   []byte
		pages []page
	}{}
	corpus := runCorpus(t)
	for _, doc := range corpus {
		switch doc.name {
		case "converted Markdown":
			cases = append(cases, struct {
				name  string
				pdf   []byte
				pages []page
			}{doc.name, doc.pdf, []page{{1, []string{
				"A heading",
				"A paragraph that is long enough to wrap onto a second line when it is rendered at the default measure of the converter.",
				"• first item",
				"• second item with bold and italic",
			}}}})
		case "core-font pages":
			cases = append(cases, struct {
				name  string
				pdf   []byte
				pages []page
			}{doc.name, doc.pdf, []page{{1, []string{"A core-font page"}}, {1, []string{"and a second one"}}}})
		case "LibreOffice text":
			cases = append(cases, struct {
				name  string
				pdf   []byte
				pages []page
			}{doc.name, doc.pdf, []page{{1, []string{
				"A heading line",
				"A paragraph of ordinary prose that wraps across the page width so the converter emits several lines.",
			}}}})
		}
	}
	if LibreOfficeAvailable() {
		body := strings.TrimSpace(strings.Repeat("Words that fill a narrow column and wrap several times before the paragraph ends. ", 3))
		p := func(s string) string { return "<text:p>" + s + "</text:p>" }
		odt := odtDocument(t,
			`<style:style style:name="Sect1" style:family="section"><style:section-properties>`+
				`<style:columns fo:column-count="2" fo:column-gap="0.5in"/></style:section-properties></style:style>`,
			`<text:h text:outline-level="1">Two columns</text:h><text:section text:style-name="Sect1" text:name="S1">`+
				p("First "+body)+p("Second "+body)+p("Third "+body)+p("Fourth "+body)+`</text:section>`)
		pdf, err := ConvertOfficeToPDF(odt, "odt")
		if err != nil {
			t.Fatalf("LibreOffice is present and could not convert the two-column fixture: %v", err)
		}
		cases = append(cases, struct {
			name  string
			pdf   []byte
			pages []page
		}{"LibreOffice two columns", pdf, []page{{2, []string{
			"Two columns", "First " + body, "Second " + body, "Third " + body, "Fourth " + body,
		}}}})
	} else {
		t.Log("NOTE (a narrower corpus, not a pass): LibreOffice is absent, so neither a third-party producer nor a two-column page is grouped")
	}

	for _, c := range cases {
		ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(c.pdf), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if ctx.PageCount != len(c.pages) {
			t.Fatalf("%s: %d page(s), the expectation has %d", c.name, ctx.PageCount, len(c.pages))
		}
		for i, want := range c.pages {
			l, lerr := readPageLayout(ctx, i+1)
			if lerr != nil {
				t.Fatalf("%s page %d: %v", c.name, i+1, lerr)
			}
			got := paragraphTexts(l)
			if len(got) != len(want.texts) {
				t.Errorf("%s page %d: %d paragraph(s), hand-checked %d:\n  got %q", c.name, i+1, len(got), len(want.texts), got)
				continue
			}
			for j := range got {
				if got[j] != want.texts[j] {
					t.Errorf("%s page %d paragraph %d:\n  got  %q\n  want %q", c.name, i+1, j, got[j], want.texts[j])
				}
			}
			if l.columns != want.columns || l.unsupported != "" {
				t.Errorf("%s page %d: columns=%d unsupported=%q, want columns=%d and supported", c.name, i+1, l.columns, l.unsupported, want.columns)
			}
		}
	}
}

// TestAnImageOnlyPageHasNoLayout — the layout door carries the run reader's structural answer through.
func TestAnImageOnlyPageHasNoLayout(t *testing.T) {
	l := groupRuns(nil)
	if !l.noText || len(l.paragraphs) != 0 {
		t.Errorf("no runs grouped into %+v, want noText", l)
	}
}

// TestOnlyTheGroupingDoorReadsRuns — P04's second exit criterion as ADR-009 states it: routing, not
// agreement. Nothing in the package outside the run reader and this door may name a run or call the
// reader, so a second opinion about where a line or paragraph begins cannot be written without this
// test naming it.
//
// `groupWords` (`tagocr.go`) is the named exemption and needs no entry: it groups tesseract's words by
// tesseract's own ids and never touches a run.
func TestOnlyTheGroupingDoorReadsRuns(t *testing.T) {
	owners := map[string]bool{"textrun.go": true, "grouping.go": true}
	idents := map[string]bool{"textRun": true, "pageRuns": true, "readPageRuns": true, "lineSegments": true, "groupRuns": true}
	// exempt names a file that reads runs for a purpose other than grouping them, and the identifiers
	// it may use — each with why. ADR-009: a deliberate exemption is named, and it is narrow: the
	// grouping identifiers stay forbidden to an exempt file, so a second grouping written there still
	// fails.
	exempt := map[string]map[string]string{
		"tagcommit.go": {
			"textRun":      "the commit matches a reviewed proposal's runs to the page's own by span and text before bracketing them (P08.S06a)",
			"readPageRuns": "the commit re-reads each page it writes, so a proposal that no longer matches is refused instead of tagging the wrong bytes",
		},
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	scanned, ownersSeen := 0, 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, rerr := os.ReadFile(f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		file, perr := parser.ParseFile(fset, f, src, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		scanned++
		ast.Inspect(file, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || !idents[id.Name] {
				return true
			}
			if owners[f] {
				ownersSeen++
				return true
			}
			if _, ok := exempt[f][id.Name]; ok {
				return true
			}
			t.Errorf("%s names %s — positioned runs are read in textrun.go and grouped in grouping.go only. "+
				"Call readPageLayout; a second grouping is a second answer to where a paragraph begins (reflow D10)", fset.Position(id.Pos()), id.Name)
			return true
		})
	}
	// 37 non-test files when this was written (measured — a first guess of 50 was wrong); a floor well
	// under it catches a glob that silently matched nothing without tracking every new file.
	if scanned < 30 || ownersSeen == 0 {
		t.Fatalf("scanned %d file(s) and saw the owners name the runs %d time(s) — the scan is not reading the package", scanned, ownersSeen)
	}
}
