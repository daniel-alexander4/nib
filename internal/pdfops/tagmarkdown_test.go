package pdfops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nib/mdpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Tagging a Markdown document from its own AST — `PLAN-accessibility.md` P06.S02.

const s02Markdown = "# Heading one\n\nBody paragraph one.\n\nBody paragraph two.\n\n" +
	"- first item\n- second item\n\n## Heading two\n\n```\ncode a\ncode b\n```\n"

// elementKinds counts the structure types in a document's tree.
func elementKinds(t *testing.T, pdf []byte) map[string]int {
	t.Helper()
	tree, defects := checkTree(t, pdf)
	if len(defects) > 0 {
		t.Errorf("the tagged document is not self-consistent: %v", defects)
	}
	out := map[string]int{}
	for _, e := range tree.elems {
		out[e.kind]++
	}
	return out
}

// TestTheTreeShapeMatchesTheMarkdown — S02's second acceptance clause.
//
// **Checked against the SOURCE, not a golden file.** The Markdown says how many headings there are
// and at what level; a golden file would say whatever the code produced on the day it was written,
// which is the same assertion with the answer copied from the thing being tested.
func TestTheTreeShapeMatchesTheMarkdown(t *testing.T) {
	out, err := tagMarkdown([]byte(s02Markdown), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatalf("tagMarkdown: %v", err)
	}
	kinds := elementKinds(t, out)

	// Read straight off the source: one `#`, one `##`, two `-` items, two paragraphs, one fence.
	for _, c := range []struct {
		kind string
		want int
		why  string
	}{
		{"H1", 1, "the Markdown has one `# ` heading"},
		{"H2", 1, "the Markdown has one `## ` heading"},
		{"L", 1, "the Markdown has one list"},
		{"LI", 2, "the list has two items"},
		{"Lbl", 2, "each item has a bullet"},
		{"LBody", 2, "each item has text"},
		{"P", 3, "two paragraphs and one code block, which is a block of text"},
	} {
		if kinds[c.kind] != c.want {
			t.Errorf("%d /%s element(s), want %d — %s. Whole tree: %v",
				kinds[c.kind], c.kind, c.want, c.why, kinds)
		}
	}
	// And nothing generic survives: P05.S04's `/Div` was the honest claim when nothing was known,
	// and a `/Div` here means a block fell through to the default.
	if kinds["Div"] != 0 {
		t.Errorf("%d /Div element(s) — a block was not recognised and fell through", kinds["Div"])
	}
}

// TestAHeadingLevelSurvivesToTheTree: the level, not just the heading-ness. A tree that calls every
// heading `H1` gives a reader one flat level to navigate by and is a different document.
//
// **`######` under `###` is H4, not H6, since `/pending 487`.** This test used to pin H6 there, which is a
// skip from H3 — a tree that fails PDF/UA 7.4.2 t1. The hierarchy survives; the gap does not.
func TestAHeadingLevelSurvivesToTheTree(t *testing.T) {
	out, err := tagMarkdown([]byte("# One\n\n## Two\n\n### Three\n\n###### Six\n"),
		authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	kinds := elementKinds(t, out)
	for _, want := range []string{"H1", "H2", "H3", "H4"} {
		if kinds[want] != 1 {
			t.Errorf("%d /%s, want 1; whole tree: %v", kinds[want], want, kinds)
		}
	}
	if kinds["H6"] != 0 {
		t.Errorf("%d /H6 — the source's jump from ### to ###### survived as a skipped level; whole tree: %v", kinds["H6"], kinds)
	}
}

// TestHeadingsNestWithoutSkippingALevel — `/pending 487`, over sequences, read back in tree order.
func TestHeadingsNestWithoutSkippingALevel(t *testing.T) {
	for _, c := range []struct {
		md   string
		want []string
	}{
		{"# A\n\n### B\n", []string{"H1", "H2"}},
		{"## A\n\n### B\n", []string{"H1", "H2"}},
		{"# A\n\n### B\n\n# C\n\n## D\n", []string{"H1", "H2", "H1", "H2"}},
		{"# A\n\n## B\n\n#### C\n\n## D\n\n### E\n", []string{"H1", "H2", "H3", "H2", "H3"}},
		{"# A\n\n## B\n\n### C\n", []string{"H1", "H2", "H3"}},
	} {
		out, err := tagMarkdown([]byte(c.md), authoringFaces(), markdownFallbackFonts())
		if err != nil {
			t.Fatal(err)
		}
		tree, defects := checkTree(t, out)
		if len(defects) > 0 {
			t.Fatalf("%q: not self-consistent: %v", c.md, defects)
		}
		var got []string
		for _, e := range tree.elems {
			if len(e.kind) == 2 && e.kind[0] == 'H' {
				got = append(got, e.kind)
			}
		}
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("%q tags its headings %v, want %v", c.md, got, c.want)
		}
	}
}

// TestNestingIsReal — S02's third acceptance clause, read back from the written document.
//
// A flat tree of `L`, `LI` and `LBody` siblings would satisfy every count above and describe a
// document with no list in it. The parent-child edges are the list.
func TestNestingIsReal(t *testing.T) {
	out, err := tagMarkdown([]byte("- first\n- second\n"), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	tree, defects := checkTree(t, out)
	if len(defects) > 0 {
		t.Fatalf("not self-consistent: %v", defects)
	}
	var list *structElem
	for _, e := range tree.elems {
		if e.kind == "L" {
			list = e
		}
	}
	if list == nil {
		t.Fatal("no /L element")
	}
	items := 0
	for _, k := range list.kids {
		if k.kind != kidElement || k.elem.kind != "LI" {
			t.Errorf("/L has a child that is not an /LI: %v", k.kindName())
			continue
		}
		items++
		var lbl, body int
		for _, gk := range k.elem.kids {
			if gk.kind != kidElement {
				continue
			}
			switch gk.elem.kind {
			case "Lbl":
				lbl++
			case "LBody":
				body++
			default:
				t.Errorf("/LI has a child of type /%s", gk.elem.kind)
			}
		}
		if lbl != 1 || body != 1 {
			t.Errorf("an /LI has %d /Lbl and %d /LBody child(ren), want 1 and 1", lbl, body)
		}
	}
	if items != 2 {
		t.Errorf("/L has %d /LI child(ren), want 2", items)
	}
}

// TestAWrappedParagraphIsOneElementWithSeveralMCIDs.
//
// The `Block` ordinal P06.S01 gained exists for this: four runs of one wrapped paragraph are ONE
// element owning four marked-content ids, not four paragraphs. A tree that says otherwise tells a
// reader the document has four paragraphs where it has one.
func TestAWrappedParagraphIsOneElementWithSeveralMCIDs(t *testing.T) {
	long := strings.Repeat("word ", 200)
	out, err := tagMarkdown([]byte(long+"\n"), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	tree, defects := checkTree(t, out)
	if len(defects) > 0 {
		t.Fatalf("not self-consistent: %v", defects)
	}
	paras, mcids := 0, 0
	for _, e := range tree.elems {
		if e.kind != "P" {
			continue
		}
		paras++
		for _, k := range e.kids {
			if k.kind == kidMCID {
				mcids++
			}
		}
	}
	if paras != 1 {
		t.Errorf("a single wrapped paragraph produced %d /P element(s)", paras)
	}
	if mcids < 3 {
		t.Errorf("the paragraph owns %d marked-content id(s); it wrapped to several lines and "+
			"should own one per line", mcids)
	}
}

// TestTaggingRefusesWhenTheStructureAndTheStREAMDisagree — the correspondence, checked rather than
// assumed, which is what P06.S01's doc comment promised this slice would do.
//
// Tagging by POSITION is only correct while the Nth run is the Nth drawing group. If that ever
// stops being true — a pdfcpu change, a new construct that draws without a run — every element from
// the point of divergence attaches to the wrong content, and the document looks entirely correct.
func TestTaggingRefusesWhenTheStructureAndTheStREAMDisagree(t *testing.T) {
	// The floor: the correspondence holds today, or the refusal below is untestable.
	pdf, st, err := mdpdf.ConvertStructured([]byte(s02Markdown), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	runs := 0
	for _, page := range st.Pages {
		runs += len(page)
	}
	spans := 0
	for p := 1; p <= len(st.Pages); p++ {
		spans += len(textOperatorSpans(pageContentOf(t, pdf, p)))
	}
	if runs == 0 || spans != runs {
		t.Fatalf("the correspondence does not hold today: %d run(s) against %d drawing group(s) — "+
			"this is the finding, not the test being wrong", runs, spans)
	}

	// And the refusal fires when it is broken: hand `tagOnePage` one fewer role than the page draws.
	short := append([]mdpdf.Role(nil), st.Pages[0][:len(st.Pages[0])-1]...)
	_, err = writeMutated(pdf, func(ctx *model.Context) error {
		live := map[int]bool{}
		for p := 1; p <= ctx.PageCount; p++ {
			if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
				live[ir.ObjectNumber.Value()] = true
			}
		}
		tree, terr := ensureStructTree(ctx, live)
		if terr != nil {
			return terr
		}
		return tagOnePage(ctx, tree, 1, short)
	})
	if err == nil {
		t.Fatal("tagging accepted a structure describing one fewer run than the page draws — " +
			"every element from the divergence would attach to the wrong content")
	}
	if !strings.Contains(err.Error(), "correspond") {
		t.Errorf("the refusal does not explain itself: %v", err)
	}
}

// TestATaggedMarkdownDocumentPassesUA1 — S02's first acceptance clause.
func TestATaggedMarkdownDocumentPassesUA1(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so P06's exit criterion — a Markdown " +
			"document passes ua1 — is UNCHECKED in this run")
	}
	// **Through the PRODUCT door, not `tagMarkdown`** — `/pending 481`. This test used to call
	// `tagMarkdown` directly, and P06 closed with criterion 1 met while nothing a user could reach
	// called that function: `ConvertDocToPDF` returned untagged Markdown the whole time. A criterion's
	// reader reads what users get.
	tagged, err := ConvertDocToPDF([]byte(s02Markdown), ".md")
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus floor: the door must have TAGGED, or the ua1 result below is about an untagged
	// document and "fails only 5 t1" is unreachable for a reason unrelated to the criterion.
	if !ClaimsTagging(tagged) {
		t.Fatal("setup: ConvertDocToPDF returned an UNTAGGED document for Markdown — the product " +
			"door is not tagging, which is /pending 481's defect")
	}
	if src, ok := StructureSource(tagged); !ok || src != sourceExact {
		t.Fatalf("setup: the converted document records source %q (recorded=%v), want %q", src, ok, sourceExact)
	}
	titled, err := SetTitle(tagged, "P06.S02")
	if err != nil {
		t.Fatal(err)
	}
	final, err := SetLang(titled, "en")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "tagged.pdf")
	if err := os.WriteFile(f, final, 0o600); err != nil {
		t.Fatal(err)
	}
	clauses := ua1FailedClauses(t, vp, []string{f})
	got := clauses["tagged.pdf"]
	if got == nil {
		t.Fatal("veraPDF could not validate the tagged document")
	}
	// `5 t1` is the PDF/UA identification schema, which nib refuses to write until something can
	// say the document conforms — P03.S01's decision, and P07's job.
	delete(got, "5 t1")
	if len(got) > 0 {
		t.Errorf("a tagged Markdown document fails %v; only `5 t1` is expected", sortedClauses(got))
	}
	t.Logf("a tagged Markdown document fails only: 5 t1 (refused by decision)")
}

// TestADocumentWithNothingInItClaimsNothing — `/pending 508`, which reported that an empty Markdown
// file "still converts untagged" because `tagMarkdown` refuses a document with zero elements.
//
// **That refusal is correct, and this pins it rather than removing it.** A document that draws no
// text has nothing to describe, and `/MarkInfo /Marked true` over a tree with no elements is
// `orphaned()` by construction — ADR-031 law 1, the claim that stops a reader reaching for the
// fallbacks it would otherwise use. The honest product is the untagged render, and the two doors
// that could make a claim over nothing — `claimTagging` and ADR-033's `LabelUA` — must both refuse.
//
// This is NOT the text-less PAGE case (`tagblankpage_test.go`), which is a page with no runs inside
// a document that has content, and which IS tagged. Here the whole document is empty.
func TestADocumentWithNothingInItClaimsNothing(t *testing.T) {
	for _, src := range []string{"", "\n", "   \n\n", "<!-- a comment and nothing else -->\n"} {
		pdf, err := ConvertDocToPDF([]byte(src), ".md")
		if err != nil {
			t.Fatalf("%q: an empty document is still a document, and the conversion failed: %v", src, err)
		}
		// STIMULUS: the input really does reach the case, or every assertion below is about an
		// ordinary document.
		_, st, serr := mdpdf.ConvertStructured([]byte(src), authoringFaces(), markdownFallbackFonts())
		if serr != nil {
			t.Fatal(serr)
		}
		runs := 0
		for _, page := range st.Pages {
			runs += len(page)
		}
		if runs != 0 {
			t.Fatalf("setup: %q laid out %d run(s), so it is not the empty case", src, runs)
		}
		if s := inspectTags(pdf); s.orphaned() {
			t.Errorf("%q converted to a document that claims tagging over nothing: %+v", src, s)
		}
		if _, lerr := LabelUA(pdf, true); !errors.Is(lerr, ErrUAUntagged) {
			t.Errorf("%q: LabelUA answered %v — a PDF/UA identification over a document with no "+
				"structure is a claim nothing in it supports", src, lerr)
		}
	}
}

// sortedKinds renders a kind map for a message.
func sortedKinds(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%s:%d", k, m[k])
	}
	return b.String()
}
