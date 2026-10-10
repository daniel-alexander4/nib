package pdfops

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nib/internal/pdfread"
)

// Removing a structure tree — ADR-120.

// markState is what a document's pages say about tagging, read from the content: the text every run draws in
// drawing order, how many of those runs sit under an id, how many under an artifact, and how many sequences
// carry an id (a figure's has no run).
type markState struct {
	text      string
	underID   int
	artifact  int
	sequences int
	keys      int // pages, annotations and form XObjects still naming a /ParentTree row
}

func marksOf(t *testing.T, pdf []byte) markState {
	t.Helper()
	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var s markState
	var text []string
	walked := pdfread.Pages(ctx)
	budget := newFormWalkBudget(len(walked))
	for _, pg := range walked {
		pr, perr := readPageRuns(ctx, pg, budget)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, r := range pr.runs {
			text = append(text, r.text)
			if r.mcid >= 0 {
				s.underID++
			}
			if r.artifact {
				s.artifact++
			}
		}
		s.sequences += len(pr.sequences)
	}
	s.text = strings.Join(text, "|")
	for _, owners := range parentTreeOwners(ctx) {
		s.keys += len(owners)
	}
	return s
}

// TestRemovingTheTreeLeavesAnUntaggedDocumentThatDrawsTheSame — over a page's own marks, marks inside a form
// drawn twice, a producer's table and figure, and a form field's `/StructParent`: no tree, no claim, no id
// and no key is left; the text drawn and what was an artifact are as they were; the result validates.
func TestRemovingTheTreeLeavesAnUntaggedDocumentThatDrawsTheSame(t *testing.T) {
	cases := map[string][]byte{
		"a page's own marks":         taggedTwoPageFixture(),
		"marks in the page and form": pageAndFormFixture(),
		"a form drawn twice":         sharedFormFixture(),
		"an artifact beside a mark":  artifactAndNamedPropertiesFixture(),
	}
	if md, err := tagMarkdown([]byte("# A title\n\nA paragraph.\n\n- one\n- two\n\n---\n\nAnother.\n"), authoringFaces(), markdownFallbackFonts()); err != nil {
		t.Fatal(err)
	} else {
		cases["nib's own Markdown"] = md
		if two, nerr := NUp(md, 2, true); nerr != nil {
			t.Fatal(nerr)
		} else {
			cases["its 2-up, tags carried into forms"] = two
		}
	}
	if LibreOfficeAvailable() {
		cases["LibreOffice's table and figure"] = tableAndFigureODT(t)
	} else {
		t.Log("NOTE (not a pass): LibreOffice is absent, so a producer's table and figure are not among the documents stripped")
	}
	if form, tagged, err := AuthorTaggedForm(taggedFixture(), s07Fields()); err != nil || !tagged {
		t.Fatalf("setup: the tagged form could not be authored (tagged %v): %v", tagged, err)
	} else {
		cases["tagged form fields"] = form
	}
	for name, src := range cases {
		before := marksOf(t, src)
		if st := inspectTags(src); !st.tree || before.sequences == 0 && before.keys == 0 {
			t.Fatalf("%s: setup — the document is not tagged (tree %v, %d sequence(s), %d key(s)), so removing proves nothing", name, st.tree, before.sequences, before.keys)
		}
		out, err := RemoveStructure(src)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if st := inspectTags(out); st.tree || st.marked || st.pagesSP != 0 {
			t.Errorf("%s: after removal the catalog still claims tagging (tree %v, marked %v) or %d page(s) keep /StructParents", name, st.tree, st.marked, st.pagesSP)
		}
		after := marksOf(t, out)
		if after.sequences != 0 || after.underID != 0 || after.keys != 0 {
			t.Errorf("%s: %d sequence(s) still carry an id, %d run(s) sit under one, %d object(s) still name a /ParentTree row", name, after.sequences, after.underID, after.keys)
		}
		if after.text != before.text {
			t.Errorf("%s: the text drawn changed:\n before %q\n after  %q", name, before.text, after.text)
		}
		if after.artifact != before.artifact {
			t.Errorf("%s: %d run(s) were artifacts and %d are now — an artifact says nothing about a tree and stays", name, before.artifact, after.artifact)
		}
		if err := Validate(out); err != nil {
			t.Errorf("%s: the untagged document does not validate: %v", name, err)
		}
		if _, err := RemoveStructure(out); !errors.Is(err, ErrTagsStale) {
			t.Errorf("%s: removing the tags of a document that has none: err = %v, want ErrTagsStale", name, err)
		}
	}
}

// artifactAndNamedPropertiesFixture marks one run through a NAMED property list, one inline, leaves one under an
// artifact, and never closes its last sequence.
func artifactAndNamedPropertiesFixture() []byte {
	content := "/P /MC0 BDC\nBT /F1 12 Tf 72 700 Td (named) Tj ET\nEMC\n" +
		"/Artifact BMC\nBT /F1 12 Tf 72 680 Td (decoration) Tj ET\nEMC\n" +
		"/Span<</MCID 1>>BDC BT /F1 12 Tf 72 660 Td (inline) Tj ET EMC\n" +
		"/P <</MCID 2>> BDC\nBT /F1 12 Tf 72 640 Td (never closed) Tj ET\n"
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /Properties << /MC0 << /MCID 0 >> >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  "<< /Length " + itoaLen(content) + " >>\nstream\n" + content + "\nendstream",
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R 11 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R 10 0 R 11 0 R]] >>",
		10: "<< /Type /StructElem /S /Span /P 7 0 R /Pg 3 0 R /K [1] >>",
		11: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [2] >>",
	})
}

func itoaLen(s string) string {
	n, out := len(s), ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}

// TestRemovingTagsTakesOnlyTheBrackets — the page's bytes between an opener and its EMC are untouched, the
// artifact's own bracket stays, and a sequence the stream never closed loses only its opener.
func TestRemovingTagsTakesOnlyTheBrackets(t *testing.T) {
	out, err := RemoveStructure(artifactAndNamedPropertiesFixture())
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := inspectionRead(out)
	if err != nil {
		t.Fatal(err)
	}
	pg := pageAt(ctx, nil, 1)
	got, err := pdfread.PageContent(ctx, pg.Dict, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := "BT /F1 12 Tf 72 700 Td (named) Tj ET " +
		"/Artifact BMC BT /F1 12 Tf 72 680 Td (decoration) Tj ET EMC " +
		"BT /F1 12 Tf 72 660 Td (inline) Tj ET " +
		"BT /F1 12 Tf 72 640 Td (never closed) Tj ET"
	if g := strings.Join(strings.Fields(string(got)), " "); g != want {
		t.Errorf("the page now draws\n %s\nwant\n %s", g, want)
	}
}

// TestATaggedDocumentCanBeTaggedAgainOnceItsTagsAreRemoved — the commit writer refuses a document with a tree
// and names the way out; with the tags removed the ordinary proposal commits, and veraPDF finds nothing in the
// result that the same proposal committed to a never-tagged copy does not have.
func TestATaggedDocumentCanBeTaggedAgainOnceItsTagsAreRemoved(t *testing.T) {
	md := []byte("# A title\n\nAn opening paragraph.\n\n- first item\n- second item\n\n---\n\nA closing paragraph.\n")
	tagged, err := tagMarkdown(md, authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	never, err := untaggedMarkdown(md)
	if err != nil {
		t.Fatal(err)
	}
	asProposed := func(pdf []byte) ([]byte, error) {
		p, perr := ProposeTags(pdf)
		if perr != nil {
			return nil, perr
		}
		reviews := make([]TagReview, len(p.Elements))
		for i, e := range p.Elements {
			reviews[i] = TagReview{ID: e.ID, Role: e.Role, Text: e.Text}
		}
		return CommitTags(pdf, reviews)
	}
	if _, err := asProposed(tagged); !errors.Is(err, errCommitTagged) || !strings.Contains(err.Error(), "Remove all tags") {
		t.Fatalf("committing over a tagged document: err = %v, want the refusal that names Remove all tags", err)
	}
	bare, err := RemoveStructure(tagged)
	if err != nil {
		t.Fatal(err)
	}
	again, err := asProposed(bare)
	if err != nil {
		t.Fatalf("the document could not be tagged again once its tags were removed: %v", err)
	}
	fresh, err := asProposed(never)
	if err != nil {
		t.Fatal(err)
	}
	a, f := viewOf(t, again), viewOf(t, fresh)
	kinds := func(v structureView) string {
		var out []string
		for _, e := range v.elements {
			out = append(out, e.kind+":"+e.text)
		}
		return strings.Join(out, "|")
	}
	if kinds(a) != kinds(f) {
		t.Errorf("tagged again, the tree reads\n %s\nand the never-tagged copy's reads\n %s", kinds(a), kinds(f))
	}
	vp := verapdfPath()
	if vp == "" {
		t.Log("NOTE (not a pass): veraPDF is absent, so the re-tagged document is UNCHECKED against the oracle in this run")
		return
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"again.pdf": again, "fresh.pdf": fresh} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	cl := ua1FailedClauses(t, vp, files)
	if cl["again.pdf"] == nil || cl["fresh.pdf"] == nil {
		t.Fatal("veraPDF could not validate one of the two, so there is no differential")
	}
	var added []string
	for c := range cl["again.pdf"] {
		if !cl["fresh.pdf"][c] {
			added = append(added, c)
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		t.Errorf("tagged again, the document fails ua1 clause(s) the never-tagged copy does not: %v", added)
	}
}
