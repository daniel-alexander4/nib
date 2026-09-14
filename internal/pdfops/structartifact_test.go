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
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/contentstream"
)

// The artifact edit — `PLAN-accessibility.md` P09.S03.

func parsed(t *testing.T, pdf []byte) *model.Context {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

// markerFreeTokens is a content stream's tokens with every marked-content opener — a tag, its property
// list, and `BDC` or `BMC` — removed and whitespace dropped: what an artifact edit, which changes openers
// and nothing else, must leave identical. Read by tokenizing, independently of the writer's spans.
func markerFreeTokens(src []byte) []string {
	var out []string
	var kinds []contentstream.Kind
	drop := func(n int) {
		out, kinds = out[:len(out)-n], kinds[:len(kinds)-n]
	}
	for _, tk := range contentstream.Tokenize(src) {
		if tk.Kind == contentstream.Whitespace {
			continue
		}
		b := string(tk.Bytes(src))
		if tk.Kind == contentstream.Operator && (b == "BDC" || b == "BMC") {
			if b == "BDC" {
				if n := len(kinds); n > 0 && kinds[n-1] == contentstream.DictClose {
					i, depth := n-1, 0
					for ; i >= 0; i-- {
						if kinds[i] == contentstream.DictClose {
							depth++
						} else if kinds[i] == contentstream.DictOpen {
							depth--
						}
						if depth == 0 {
							break
						}
					}
					drop(n - i)
				} else {
					drop(1)
				}
			}
			drop(1) // the tag
			continue
		}
		out = append(out, b)
		kinds = append(kinds, tk.Kind)
	}
	return out
}

// subtreeOf is the ids of the element with this id and every element under it.
func subtreeOf(v structureView, id int) map[int]bool {
	gone := map[int]bool{}
	var mark func(i int)
	mark = func(i int) {
		gone[v.elements[i].id] = true
		for _, j := range v.elements[i].kids {
			mark(j)
		}
	}
	for i, e := range v.elements {
		if e.id == id {
			mark(i)
		}
	}
	return gone
}

// ownedMCIDs is every MCID the elements in ids own.
func ownedMCIDs(t *testing.T, pdf []byte, ids map[int]bool) []int {
	t.Helper()
	tree, _ := checkTree(t, pdf)
	var out []int
	for _, e := range tree.elems {
		if !ids[e.objNr] {
			continue
		}
		for _, k := range e.kids {
			if k.kind == kidMCID || k.kind == kidMCR {
				out = append(out, k.mcid)
			}
		}
	}
	return out
}

// TestArtifactingAnElementTakesItOutOfTheTreeAndLeavesItsContent — a paragraph and a whole list on
// LibreOffice's own tree: gone from the tree, their ParentTree slots empty, their text still drawn and now
// drawn as an artifact, and not one token of the page changed but the openers.
func TestArtifactingAnElementTakesItOutOfTheTreeAndLeavesItsContent(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice is not installed, so there is no producer's tree; the hand-built refusals still run")
	}
	lists := truthCorpus(t)[0].pdf
	v := viewOf(t, lists)
	top := kidIDs(v, v.elements[0])
	list, closing := top[4], top[5]
	ids := byID(v)
	gone := subtreeOf(v, list)
	gone[closing] = true
	mcids := ownedMCIDs(t, lists, gone)
	if len(mcids) < 3 {
		t.Fatalf("setup: the list and the closing paragraph own %d MCID(s)", len(mcids))
	}

	out := mustApply(t, lists, structEdit{kind: editArtifact, elem: closing}, structEdit{kind: editArtifact, elem: list})
	requireConsistent(t, "artifact", out)
	ov := viewOf(t, out)
	for _, e := range ov.elements {
		if gone[e.id] {
			t.Errorf("element %d (%s) is still in the tree", e.id, e.kind)
		}
	}
	if got := kidIDs(ov, ov.elements[0]); !equalInts(got, top[:4]) {
		t.Errorf("the root's kids read %v, want %v", got, top[:4])
	}
	if a, b := markerFreeTokens(pageContentOf(t, lists, 1)), markerFreeTokens(pageContentOf(t, out, 1)); strings.Join(a, " ") != strings.Join(b, " ") {
		t.Errorf("the page's tokens changed beyond its marked-content openers (%d tokens before, %d after)", len(a), len(b))
	}

	ctx := parsed(t, out)
	pr, err := readPageRuns(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var drawnAsArtifact strings.Builder
	for _, r := range pr.runs {
		if r.artifact {
			drawnAsArtifact.WriteString(r.text)
		}
	}
	for _, id := range []int{closing, list} {
		if !strings.Contains(squeeze(drawnAsArtifact.String()), squeeze(ids[id].text)) {
			t.Errorf("element %d's text %q is not drawn as an artifact", id, ids[id].text)
		}
	}
	for _, s := range pr.sequences {
		for _, m := range mcids {
			if s.mcid == m {
				t.Errorf("the page still opens a sequence with /MCID %d", m)
			}
		}
	}
	otree, err := readStructTree(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	arrays, _ := parentTreeEntries(ctx, otree)
	d, _, _, _ := ctx.PageDict(1, false)
	key := d["StructParents"].(types.Integer).Value()
	for _, m := range mcids {
		if slots := arrays[key]; m < len(slots) && slots[m] != 0 {
			t.Errorf("ParentTree slot %d still names object %d", m, slots[m])
		}
	}
}

// TestArtifactingAFigureNeedsNoText — an image has no runs, so its sequence is found by the sequence
// record and not by text.
func TestArtifactingAFigureNeedsNoText(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice is not installed, so the table-and-figure document cannot be generated")
	}
	table := tableAndFigureODT(t)
	v := viewOf(t, table)
	fig := 0
	for _, e := range v.elements {
		if e.standard == "Figure" {
			fig = e.id
		}
	}
	mcids := ownedMCIDs(t, table, map[int]bool{fig: true})
	if fig == 0 || len(mcids) != 1 {
		t.Fatalf("setup: figure %d owns %v", fig, mcids)
	}
	out := mustApply(t, table, structEdit{kind: editArtifact, elem: fig})
	requireConsistent(t, "a figure", out)
	for _, e := range viewOf(t, out).elements {
		if e.standard == "Figure" {
			t.Error("the figure is still in the tree")
		}
	}
	pr, err := readPageRuns(parsed(t, out), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range pr.sequences {
		if s.mcid == mcids[0] {
			t.Errorf("the page still opens the figure's sequence (/MCID %d)", s.mcid)
		}
	}
	if a, b := markerFreeTokens(pageContentOf(t, table, 1)), markerFreeTokens(pageContentOf(t, out, 1)); strings.Join(a, " ") != strings.Join(b, " ") {
		t.Error("the page's tokens changed beyond its openers — the image is no longer drawn as it was")
	}
}

// TestArtifactingAddsNoUA1Clause — a paragraph and the figure declared artifacts; veraPDF finds nothing
// the producer's document did not already fail.
func TestArtifactingAddsNoUA1Clause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so P09.S03's ua1 differential is UNCHECKED in this run.")
	}
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is absent, so there is no table-and-figure document.")
	}
	table := tableAndFigureODT(t)
	v := viewOf(t, table)
	top := kidIDs(v, v.elements[0])
	fig := 0
	for _, e := range v.elements {
		if e.standard == "Figure" {
			fig = e.id
		}
	}
	out := mustApply(t, table, structEdit{kind: editArtifact, elem: top[1]}, structEdit{kind: editArtifact, elem: fig})
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"original.pdf": table, "artifacted.pdf": out} {
		f := filepath.Join(dir, n)
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	before, after := cl["original.pdf"], cl["artifacted.pdf"]
	if before == nil || after == nil {
		t.Fatal("veraPDF could not validate one of the pair")
	}
	var added []string
	for c := range after {
		if !before[c] {
			added = append(added, c)
		}
	}
	sort.Strings(added)
	t.Logf("original fails %v; artifacted %v", sortedClauses(before), sortedClauses(after))
	if len(added) > 0 {
		t.Errorf("declaring content an artifact ADDED ua1 clause(s): %v", added)
	}
}

// artifactFixture is a one-page tagged document drawing content, with a form XObject available as Fm0
// (drawing form, when given, or a plain rectangle).
func artifactFixture(content string, elems map[int]string, form ...string) []byte {
	formBody := "0 0 1 1 re f"
	if len(form) > 0 {
		formBody = form[0]
	}
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm0 5 0 R >> >> /Contents 4 0 R >>",
		4: streamObject("", content),
		5: streamObject("/Type /XObject /Subtype /Form /BBox [0 0 10 10]", formBody),
	}
	for k, v := range elems {
		objs[k] = v
	}
	return assembleFixture(objs)
}

// TestArtifactingRefusesWhatItCannotRewriteHonestly — each refusal driven by the document shape it is for.
func TestArtifactingRefusesWhatItCannotRewriteHonestly(t *testing.T) {
	marked := "/P <</MCID 0>> BDC 0 0 1 1 re f EMC"
	sibling := "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R >>"
	for _, c := range []struct {
		name    string
		content string
		elems   map[int]string
		want    error
		says    string
		form    string
	}{
		{name: "content marked inside a form's own stream", content: "/Fm0 Do", elems: map[int]string{
			7: "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>",
			8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>", 9: sibling,
		}, want: errCommitInForm, form: "/P <</MCID 0>> BDC 0 0 1 1 re f EMC"},
		{name: "an unclosed sequence enclosing another element's", content: "/Div <</MCID 0>> BDC /P <</MCID 1>> BDC 0 0 1 1 re f EMC", elems: map[int]string{
			7: "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>",
			8: "<< /Type /StructElem /S /Div /P 7 0 R /Pg 3 0 R /K 0 >>",
			9: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 1 >>",
		}, want: ErrTagsReview},
		{"content drawn through a form", "/Figure <</MCID 0>> BDC /Fm0 Do EMC", map[int]string{
			7: "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>",
			8: "<< /Type /StructElem /S /Figure /P 7 0 R /Pg 3 0 R /K 0 >>", 9: sibling,
		}, errCommitInForm, "", ""},
		{"a marked-content reference into a form's stream", marked, map[int]string{
			7: "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>",
			8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K << /Type /MCR /Pg 3 0 R /MCID 0 /Stm 5 0 R >> >>", 9: sibling,
		}, errCommitInForm, "", ""},
		{"an annotation's element", marked, map[int]string{
			7:  "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>",
			8:  "<< /Type /StructElem /S /Link /P 7 0 R /Pg 3 0 R /K << /Type /OBJR /Obj 10 0 R >> >>",
			9:  sibling,
			10: "<< /Type /Annot /Subtype /Link /Rect [0 0 1 1] >>",
		}, ErrTagsReview, "", ""},
		{"content enclosing another element's", "/Div <</MCID 0>> BDC /P <</MCID 1>> BDC 0 0 1 1 re f EMC EMC", map[int]string{
			7: "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>",
			8: "<< /Type /StructElem /S /Div /P 7 0 R /Pg 3 0 R /K 0 >>",
			9: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 1 >>",
		}, ErrTagsReview, "", ""},
		{"the last element of the tree", marked, map[int]string{
			7: "<< /Type /StructTreeRoot /K [8 0 R] >>",
			8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
		}, ErrTagsReview, "", ""},
		{"an MCID the page does not draw", marked, map[int]string{
			7: "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>",
			8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 5 >>", 9: sibling,
		}, nil, "draws no sequence", ""},
	} {
		var src []byte
		if c.form != "" {
			src = artifactFixture(c.content, c.elems, c.form)
		} else {
			src = artifactFixture(c.content, c.elems)
		}
		_, err := applyStructEdits(src, []structEdit{{kind: editArtifact, elem: 8}})
		switch {
		case err == nil:
			t.Errorf("%s: the artifact edit was applied", c.name)
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		case c.says != "" && !strings.Contains(err.Error(), c.says):
			t.Errorf("%s: err = %v, want it to say %q", c.name, err, c.says)
		}
	}

	// And the positive twin of the fixtures above, so a refusal is not the only thing they can produce: a
	// sequence FOLLOWED by another element's, which follows it and is not inside it. The tree has no
	// ParentTree, and emptying a slot must not create one.
	ok := artifactFixture(marked+"\n/P <</MCID 1>> BDC 2 2 1 1 re f EMC", map[int]string{
		// The page names a ParentTree key the tree has no ParentTree for, so the slot-clearing path is
		// reached and has nothing to clear.
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << /XObject << /Fm0 5 0 R >> >> /Contents 4 0 R >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] >>",
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
		9: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 1 >>",
	})
	out, err := applyStructEdits(ok, []structEdit{{kind: editArtifact, elem: 8}})
	if err != nil {
		t.Fatalf("the plain case was refused: %v", err)
	}
	got := string(pageContentOf(t, out, 1))
	if !strings.Contains(got, "/Artifact BMC") || strings.Contains(got, "MCID 0") || !strings.Contains(got, "/P <</MCID 1>> BDC") {
		t.Errorf("the plain case's page reads %q — want the first sequence an artifact and the second untouched", got)
	}
	ctx := parsed(t, out)
	cat, _ := ctx.XRefTable.Catalog()
	if root, _ := ctx.DereferenceDict(cat["StructTreeRoot"]); root == nil {
		t.Fatal("the plain case lost its structure tree root")
	} else if _, made := root["ParentTree"]; made {
		t.Error("the artifact edit created a ParentTree to empty a slot in — an object nobody asked for")
	}
}
