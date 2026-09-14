package pdfops

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The proposer — `PLAN-accessibility.md` P08.S05.

func roles(p proposal) []string {
	out := make([]string, len(p.elements))
	for i, e := range p.elements {
		out[i] = e.role
	}
	return out
}

func proposeRuns(runs ...textRun) proposal { return proposeFromLayouts([]pageLayout{groupRuns(runs)}) }

// TestHeadingLevelsFollowTheDistinctLargerSizes — larger than the body is a heading, the largest is H1,
// and a long passage set large is not a heading.
func TestHeadingLevelsFollowTheDistinctLargerSizes(t *testing.T) {
	body := "Body text that fills the measure of the page with enough characters to be the body size"
	p := proposeRuns(
		run("Title", 72, 760, 80, 20),
		run("Chapter", 72, 730, 80, 16),
		run(body, 72, 710, 450, 12),
		run(body, 72, 696, 450, 12),
		run("Section", 72, 670, 80, 16),
		run(body, 72, 650, 450, 12),
	)
	if got, want := strings.Join(roles(p), " "), "H1 H2 P H2 P"; got != want {
		t.Errorf("roles = %s, want %s (body size %.1f)", got, want, p.bodySize)
	}
	if p.bodySize != 12 {
		t.Errorf("body size %.1f, want 12 — the size carrying the most characters", p.bodySize)
	}

	// More heading RUNS than body runs, fewer heading CHARACTERS: the body is still the 12pt text.
	many := proposeRuns(
		run("One", 72, 780, 40, 16), run("Two", 72, 750, 40, 16), run("Three", 72, 720, 50, 16),
		run("A single long paragraph of body text that carries far more characters than all of the headings put together", 72, 690, 450, 12),
		run("Four", 72, 660, 40, 16),
	)
	if many.bodySize != 12 || strings.Join(roles(many), " ") != "H1 H1 H1 P H1" {
		t.Errorf("four short 16pt headings beside one long 12pt paragraph: body %.1f, roles %v — the body is the size with the most characters, not the most runs",
			many.bodySize, roles(many))
	}

	// Four lines at 16pt beside a 12pt body is large text, not a heading.
	large := proposeRuns(
		run("Large text line one that fills the whole measure", 72, 760, 450, 16),
		run("Large text line two that fills the whole measure", 72, 740, 450, 16),
		run("Large text line three that fills the whole measure", 72, 720, 450, 16),
		run("Large text line four that fills the whole measure", 72, 700, 450, 16),
		run(body+body, 72, 670, 450, 12), run(body+body, 72, 656, 450, 12), run(body+body, 72, 642, 450, 12),
	)
	if large.elements[0].role != "P" {
		t.Errorf("a four-line passage at 16pt was proposed as %s", large.elements[0].role)
	}
}

// TestAListLabelStartsAnItemInEveryFormTheCorpusDraws — and an uppercase initial does not.
func TestAListLabelStartsAnItemInEveryFormTheCorpusDraws(t *testing.T) {
	for _, c := range []struct {
		name  string
		label string
		want  string
	}{
		{"OpenSymbol's private-use bullet", "", "LI"},
		{"a bullet", "•", "LI"},
		{"a number", "1.", "LI"},
		{"a parenthesised letter", "a)", "LI"},
		{"a roman numeral", "iv.", "LI"},
		{"an uppercase initial", "A.", "P"},
		{"a word", "The", "P"},
	} {
		p := proposeRuns(run(c.label, 72, 700, 8, 12), run("item text here", 90, 700, 80, 12))
		if len(p.elements) != 1 || p.elements[0].role != c.want {
			t.Errorf("%s: %q proposed %v, want %s", c.name, c.label, roles(p), c.want)
		}
	}
	// A label drawn flush against its text, as LibreOffice's numbered list does: the line's first WORD
	// is `1.Open`, and only its first RUN is the label.
	flush := proposeRuns(run("1.", 72, 700, 9, 12), run("Open the document.", 81, 700, 100, 12))
	if len(flush.elements) != 1 || flush.elements[0].role != "LI" || flush.elements[0].marker != "1." {
		t.Errorf("a label flush against its text proposed %+v, want one LI labelled 1.", flush.elements)
	}
	// Two numbered items alone in a column are ONE paragraph to the grouping (their right edge is the
	// column's) — the labels must split them into two items of one list.
	merged := proposeRuns(
		run("1.", 72, 700, 8, 12), run("numbered one", 90, 700, 70, 12),
		run("2.", 72, 686, 8, 12), run("numbered two", 90, 686, 70, 12),
	)
	if got := strings.Join(roles(merged), " "); got != "LI LI" || merged.elements[0].list != merged.elements[1].list {
		t.Errorf("two merged numbered lines proposed %s in lists %d/%d, want two LIs in one list", got,
			merged.elements[0].list, merged.elements[len(merged.elements)-1].list)
	}
	// A paragraph between two lists ends the first.
	two := proposeRuns(
		run("•", 72, 760, 5, 12), run("first list item", 90, 760, 300, 12),
		run("A paragraph between the lists that fills the whole measure", 72, 730, 450, 12),
		run("•", 72, 700, 5, 12), run("second list item", 90, 700, 300, 12),
	)
	if len(two.elements) != 3 || two.elements[0].list == two.elements[2].list {
		t.Errorf("items either side of a paragraph share a list: %+v", two.elements)
	}
	// And a heading between them ends the first list just as a paragraph does.
	headed := proposeRuns(
		run("•", 72, 760, 5, 12), run("first list item that fills much of the measure here", 90, 760, 380, 12),
		run("A heading", 72, 730, 90, 18),
		run("•", 72, 700, 5, 12), run("second list item that fills much of the measure here", 90, 700, 380, 12),
	)
	if got := strings.Join(roles(headed), " "); got != "LI H1 LI" || headed.elements[0].list == headed.elements[2].list {
		t.Errorf("items either side of a heading: roles %s, lists %d and %d — a heading must end the list",
			got, headed.elements[0].list, headed.elements[len(headed.elements)-1].list)
	}
}

// proposalBlocks converts a proposal into scoring blocks.
func proposalBlocks(p proposal) []truthBlock {
	out := make([]truthBlock, 0, len(p.elements))
	for _, e := range p.elements {
		if text := squeeze(e.text); text != "" {
			out = append(out, truthBlock{heading: strings.HasPrefix(e.role, "H"), text: text})
		}
	}
	return out
}

// TestTheProposalClearsItsFloorsOnTheTruthCorpus — exit criterion 1 as a guard: on every document the
// proposal finds paragraph boundaries at least as well as the grouping it starts from, and finds
// headings, which the grouping cannot.
func TestTheProposalClearsItsFloorsOnTheTruthCorpus(t *testing.T) {
	corpus := truthCorpus(t)
	if corpus == nil {
		t.Skip("LibreOffice is absent, so no document carries a producer's own structure to score against")
	}
	// Counted from each document's source. The scores alone cannot see a list item called a paragraph —
	// the first cut of this proposer cleared every floor while proposing `H1 P P P P` for a document
	// with three numbered items.
	wantRoles := map[string]string{
		"headings, paragraphs and a list": "H1 P H2 P LI LI P",
		"two columns":                     "H1 P P P P",
		"a numbered list":                 "H1 P LI LI LI P",
	}
	for _, doc := range corpus {
		truth := readTruth(t, doc.pdf)
		stripped := stripStructTree(t, doc.pdf)
		ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(stripped), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		prop, err := proposeStructure(ctx)
		if err != nil {
			t.Fatalf("%s: %v", doc.name, err)
		}
		floorB, _, ferr := scoreBlocks(truth, layoutBlocks(t, stripped))
		pb, ph, perr := scoreBlocks(truth, proposalBlocks(prop))
		if ferr != nil || perr != nil {
			t.Fatalf("%s: cannot score (grouping err=%v, proposal err=%v)", doc.name, ferr, perr)
		}
		if pb.f1 < floorB.f1 {
			t.Errorf("%s: the proposal finds boundaries worse than the grouping it starts from: %v against %v", doc.name, pb, floorB)
		}
		if ph.f1 <= 0 {
			t.Errorf("%s: the proposal finds no heading the producer stated (%v)", doc.name, ph)
		}
		got := strings.Join(roles(prop), " ")
		if want, ok := wantRoles[doc.name]; ok && got != want {
			t.Errorf("%s: proposed %s, the source states %s", doc.name, got, want)
		}
		if doc.name == "a multi-page report" {
			// Its body paragraphs cannot be separated (S04's pin), but its seven headings can.
			if h := strings.Count(got, "H"); h != 7 || strings.Contains(got, "LI") {
				t.Errorf("%s: proposed %d heading(s) and roles %s, want 7 headings and no list", doc.name, h, got)
			}
		}
		t.Logf("%s: boundaries %v (grouping %v); headings %v; roles %v", doc.name, pb, floorB, ph, roles(prop))
	}
}

// TestTheProposerHasNoPathToAWriter — law 3 by routing: nothing in proposer.go calls a function that
// writes a document or a tree. It checks direct calls; the proposer's only reader is readPageLayout,
// which itself calls no writer.
func TestTheProposerHasNoPathToAWriter(t *testing.T) {
	writers := map[string]bool{
		"writeMutated": true, "setPageContent": true, "ensureStructTree": true, "addMarkedElement": true,
		"addMarkedElementUnder": true, "addGroupingElement": true, "addMCIDTo": true, "setParentTreeSlot": true,
		"setParentTreeSingle": true, "claimTagging": true, "setTagSource": true, "WriteContext": true,
		"tagOnePage": true, "tagMarkdown": true, "AuthorTaggedForm": true, "TagOCRLayer": true,
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "proposer.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		calls++
		name := ""
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			name = fn.Name
		case *ast.SelectorExpr:
			name = fn.Sel.Name
		}
		if writers[name] {
			t.Errorf("%s: the proposer calls %s — a proposal is shown and edited before anything is written (law 3, D5)", fset.Position(call.Pos()), name)
		}
		return true
	})
	if calls < 10 {
		t.Fatalf("proposer.go holds %d call(s) — the scan is not reading the proposer", calls)
	}
}
