package pdfops

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/contentstream"
)

// The commit writer — `PLAN-accessibility.md` P08.S06a.

const commitFixtureMarkdown = "# A committed document\n\n" +
	"An opening paragraph that is long enough to wrap onto a second line when it is rendered at the default measure of the converter.\n\n" +
	"## A second-level heading\n\n" +
	"- first item\n- second item\n\n" +
	"A closing paragraph.\n"

func proposeFor(t *testing.T, pdf []byte) proposal {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	p, err := proposeStructure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestACommittedProposalReadsBackAsItsOwnTruth — S06a's first clause: what is committed is what S04's
// truth reader finds, block for block, and the tree says it was inferred.
func TestACommittedProposalReadsBackAsItsOwnTruth(t *testing.T) {
	src, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	p := proposeFor(t, src)
	if got := roles(p); len(got) != 6 {
		t.Fatalf("setup: the proposal is %v — the round trip needs headings, paragraphs and a list", got)
	}
	out, err := commitProposal(src, p.elements)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	want, got := proposalBlocks(p), readTruth(t, out)
	if len(got) != len(want) {
		t.Fatalf("the committed tree reads %d block(s), the proposal had %d:\n  got  %+v\n  want %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("block %d reads %+v, committed %+v", i, got[i], want[i])
		}
	}
	if src, ok := StructureSource(out); !ok || src != sourceInferred {
		t.Errorf("the committed tree records source %q (recorded %v), want Inferred", src, ok)
	}
	// The tree's element types, counted from the fixture. The block comparison above cannot see a
	// list label folded into its body — label and body read back as the same text either way — so the
	// types are asserted directly. mdpdf draws each bullet as its own run, so each label is its own Lbl.
	tctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := readStructTree(tctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, e := range tree.elems {
		counts[e.kind]++
	}
	for kind, want := range map[string]int{"H1": 1, "H2": 1, "P": 2, "L": 1, "LI": 2, "Lbl": 2, "LBody": 2} {
		if counts[kind] != want {
			t.Errorf("the committed tree has %d %s element(s), want %d (all: %v)", counts[kind], kind, want, counts)
		}
	}
	// Nothing on the committed page is left neither tagged nor an artifact: every run is under an MCID,
	// or under /Artifact and so reads mcid -1 — and there is at least one of each only if the grouping
	// dropped something, so the count asserted is the tagged one.
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageRuns(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	tagged := 0
	for _, r := range pr.runs {
		if r.mcid >= 0 {
			tagged++
		}
	}
	if tagged == 0 {
		t.Error("no run on the committed page carries an MCID")
	}
}

// TestTheCommitRefusesWhatItCannotDescribeHonestly — the four refusals, each driven by a real document.
func TestTheCommitRefusesWhatItCannotDescribeHonestly(t *testing.T) {
	untagged, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	p := proposeFor(t, untagged)

	tagged, err := ConvertDocToPDF([]byte(commitFixtureMarkdown), ".md")
	if err != nil {
		t.Fatal(err)
	}
	if _, cerr := commitProposal(tagged, proposeFor(t, tagged).elements); !errors.Is(cerr, errCommitTagged) {
		t.Errorf("a document with a tree: err = %v, want errCommitTagged", cerr)
	}

	stripped := stripStructTree(t, tagged)
	if _, cerr := commitProposal(stripped, proposeFor(t, stripped).elements); !errors.Is(cerr, errCommitMarked) {
		t.Errorf("a page keeping its MCIDs with no tree: err = %v, want errCommitMarked", cerr)
	}

	stamped, err := StampWatermark(untagged, "DRAFT", WatermarkStyle{})
	if err != nil {
		t.Fatal(err)
	}
	// The watermark sits inside `/Artifact` on the page, so it is never proposed — and a watermarked
	// document commits. Until the run reader said so, the stamp was proposed as a paragraph and the
	// whole page was refused (S06a's finding).
	sp := proposeFor(t, stamped)
	for _, el := range sp.elements {
		if strings.Contains(el.text, "DRAFT") {
			t.Errorf("the watermark was proposed as content: %q", el.text)
		}
	}
	if _, cerr := commitProposal(stamped, sp.elements); cerr != nil {
		t.Errorf("a watermarked document could not be committed: %v", cerr)
	}

	// The same stamp with its artifact marker replaced: now the form's text IS content, drawn inside a
	// form XObject, and the commit must refuse to describe the `Do` in its place.
	bare, err := writeMutated(stamped, func(ctx *model.Context) error {
		for pg := 1; pg <= ctx.PageCount; pg++ {
			d, _, _, derr := ctx.PageDict(pg, false)
			if derr != nil {
				return derr
			}
			src, cerr := ctx.PageContent(d, pg)
			if cerr != nil {
				return cerr
			}
			edit := contentstream.NewEdit(src)
			for _, m := range watermarkArtifactSpans(src) {
				edit.Replace(m.start, m.end, []byte("/Span BMC"))
			}
			out, aerr := edit.Apply()
			if aerr != nil {
				return aerr
			}
			if serr := setPageContent(ctx, d, out); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	bp := proposeFor(t, bare)
	inForm := false
	for _, el := range bp.elements {
		for _, r := range elementRuns(el) {
			inForm = inForm || r.inForm
		}
	}
	if !inForm {
		t.Fatal("setup: with its artifact marker gone the stamp's text was not proposed as drawn inside a form, so the refusal below is not driven")
	}
	if _, cerr := commitProposal(bare, bp.elements); !errors.Is(cerr, errCommitInForm) {
		t.Errorf("text drawn inside a form XObject: err = %v, want errCommitInForm", cerr)
	}

	other, err := untaggedMarkdown([]byte("# Something else\n\nA different document entirely.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, cerr := commitProposal(other, p.elements); !errors.Is(cerr, errCommitStale) {
		t.Errorf("a proposal for a different document: err = %v, want errCommitStale", cerr)
	}
	if _, cerr := commitProposal(untagged, nil); cerr == nil {
		t.Error("an empty proposal committed")
	}

	// The same document with the last character changed: every show operator keeps its byte span —
	// the change is inside the last string on the last line — so only the TEXT tells the proposal
	// no longer describes it. A different document fails on its spans first and never reaches that.
	edited, err := untaggedMarkdown([]byte(strings.Replace(commitFixtureMarkdown, "A closing paragraph.", "A closing paragraph!", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, cerr := commitProposal(edited, p.elements); !errors.Is(cerr, errCommitStale) {
		t.Errorf("a proposal whose runs keep their spans but not their text: err = %v, want errCommitStale", cerr)
	}

	// An element listed twice would bracket the same show operator twice — content with two owners.
	twice := append(append([]proposedElement(nil), p.elements...), p.elements[1])
	if _, cerr := commitProposal(untagged, twice); !errors.Is(cerr, errCommitStale) {
		t.Errorf("a proposal listing an element twice: err = %v, want errCommitStale", cerr)
	}
}

// TestACommitAddsNoUA1ClauseTheUntaggedDocumentLacked — S06a's veraPDF clause, the same differential
// P06.S06 ran for the OCR layer: the committed document must fail a subset of what the untagged one
// fails.
func TestACommitAddsNoUA1ClauseTheUntaggedDocumentLacked(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so P08.S06a's ua1 clause is UNCHECKED in this run.")
	}
	src, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	prop := proposeFor(t, src)
	out, err := commitProposal(src, prop.elements)
	if err != nil {
		t.Fatal(err)
	}
	// A reviewer who ignores an element: its runs are covered by no element and must become artifacts,
	// or the page holds text that is neither tagged nor an artifact — 7.1 t3.
	var kept []proposedElement
	dropped := false
	for _, el := range prop.elements {
		if !dropped && el.role == "P" {
			dropped = true
			continue
		}
		kept = append(kept, el)
	}
	if !dropped {
		t.Fatal("setup: the proposal has no paragraph to ignore")
	}
	ignored, err := commitProposal(src, kept)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"untagged.pdf": src, "committed.pdf": out, "ignored.pdf": ignored} {
		f := filepath.Join(dir, n)
		if werr := os.WriteFile(f, b, 0o600); werr != nil {
			t.Fatal(werr)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	before, after := cl["untagged.pdf"], cl["committed.pdf"]
	if before == nil || after == nil {
		t.Fatal("veraPDF could not validate one of the pair, so there is no differential")
	}
	var added, removed []string
	for c := range after {
		if !before[c] {
			added = append(added, c)
		}
	}
	for c := range before {
		if !after[c] {
			removed = append(removed, c)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	t.Logf("untagged fails %d clause(s), committed %d; cleared %v; added %v", len(before), len(after), removed, added)
	if len(added) > 0 {
		t.Errorf("committing ADDED ua1 clause(s) the untagged document did not fail: %v", added)
	}
	if len(removed) == 0 {
		t.Error("committing a tree cleared no ua1 clause at all — the structure is not reaching the checker")
	}
	ig := cl["ignored.pdf"]
	if ig == nil {
		t.Fatal("veraPDF could not validate the document with an ignored element")
	}
	if ig["7.1 t3"] {
		t.Error("an ignored element's text is neither tagged nor an artifact: the committed page fails 7.1 t3")
	}
	for c := range ig {
		if !before[c] {
			t.Errorf("ignoring an element ADDED ua1 clause %s", c)
		}
	}
}
