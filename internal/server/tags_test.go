package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/mdpdf"
)

// The tag review routes — `PLAN-accessibility.md` P08.S06b.

const tagsFixtureMarkdown = "# A document to tag\n\n" +
	"An opening paragraph that is long enough to wrap onto a second line when it is rendered at the default measure.\n\n" +
	"## A second heading\n\n" +
	"- first item\n- second item\n\n" +
	"A closing paragraph.\n"

// openTagsFixture opens an untagged document with headings, paragraphs and a list.
func openTagsFixture(t *testing.T, pdf []byte) (string, *http.Client, string) {
	t.Helper()
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	path := filepath.Join(t.TempDir(), "totag.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	openByPath(t, ts.URL, c, csrf, path)
	return ts.URL, c, csrf
}

func untaggedTagsFixture(t *testing.T) []byte {
	t.Helper()
	pdf, err := mdpdf.Convert([]byte(tagsFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func getBytes(t *testing.T, c *http.Client, url string) []byte {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", url, resp.StatusCode, b)
	}
	return b
}

func proposeOpen(t *testing.T, c *http.Client, base string) tagProposalResponse {
	t.Helper()
	var prop tagProposalResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/propose"), &prop); err != nil {
		t.Fatal(err)
	}
	return prop
}

// reviewBody is the review the Tags card sends: every element, as proposed unless edit changes it.
func reviewBody(prop tagProposalResponse, edit func([]map[string]any) []map[string]any) map[string]any {
	els := make([]map[string]any, len(prop.Elements))
	for i, e := range prop.Elements {
		els[i] = map[string]any{"id": e.ID, "role": e.Role, "ignore": false, "text": e.Text}
	}
	if edit != nil {
		els = edit(els)
	}
	return map[string]any{"elements": els}
}

func postTags(t *testing.T, c *http.Client, csrf, url string, body map[string]any) (int, string) {
	t.Helper()
	r := write(t, c, csrf, http.MethodPost, url, "application/json", jsonBody(body))
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

// TestTheProposeRouteReadsAndChangesNothing — law 3 at the route: a proposal leaves the document
// byte-identical.
func TestTheProposeRouteReadsAndChangesNothing(t *testing.T) {
	base, c, _ := openTagsFixture(t, untaggedTagsFixture(t))
	before := getBytes(t, c, base+"/api/pdf")
	prop := proposeOpen(t, c, base)
	after := getBytes(t, c, base+"/api/pdf")
	if !bytes.Equal(before, after) {
		t.Fatal("proposing a structure changed the open document")
	}
	var roles []string
	for _, e := range prop.Elements {
		roles = append(roles, e.Role)
		if e.PageBox[2]-e.PageBox[0] <= 0 || e.Page < 1 {
			t.Errorf("element %d does not say where it sits: page %d box %v", e.ID, e.Page, e.PageBox)
		}
	}
	joined := strings.Join(roles, " ")
	if !strings.Contains(joined, "H1") || !strings.Contains(joined, "LI") || !strings.Contains(joined, "P") {
		t.Errorf("proposed %s — expected headings, paragraphs and list items from the fixture", joined)
	}
	if prop.Unsupported == nil || prop.NoText == nil {
		t.Error("the response publishes null where the client expects a list")
	}
}

// TestTheCommitRouteWritesTheReviewAndUndoTakesItBack — the reviewed structure reaches the document,
// the report's provenance says Inferred, and the edit rides the undo ring like every other.
func TestTheCommitRouteWritesTheReviewAndUndoTakesItBack(t *testing.T) {
	base, c, csrf := openTagsFixture(t, untaggedTagsFixture(t))
	prop := proposeOpen(t, c, base)
	code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, func(e []map[string]any) []map[string]any {
		e[len(e)-1]["ignore"] = true // the reviewer ignores the closing paragraph
		return e
	}))
	if code != http.StatusOK {
		t.Fatalf("commit = %d: %s", code, body)
	}
	var rep uaReportResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/uacheck"), &rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rep.Structure, "inferred") {
		t.Errorf("after a commit the report says %q — the provenance must say the structure was inferred", rep.Structure)
	}
	if code, body = postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if err := json.Unmarshal(getBytes(t, c, base+"/api/uacheck"), &rep); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rep.Structure, "inferred") {
		t.Errorf("undo left the committed structure in place: %q", rep.Structure)
	}
}

// TestTheCommitRouteRefusesASignedDocumentAtTheDoor — S06's acceptance: the server refuses, whatever
// the client does.
func TestTheCommitRouteRefusesASignedDocumentAtTheDoor(t *testing.T) {
	signed := threeSigned(t)
	base, c, csrf := openTagsFixture(t, signed)
	prop := proposeOpen(t, c, base)
	before := getBytes(t, c, base+"/api/pdf")
	code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, nil))
	if code != http.StatusConflict || !strings.Contains(body, "signed") {
		t.Errorf("a signed document: commit = %d %q, want 409 naming the signature", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Error("a refused commit changed the signed document")
	}
}

// TestTheCommitRouteTellsAStaleReviewFromAMalformedOne — 409 for a review of a document that has
// changed, 400 for a review that cannot describe any document.
func TestTheCommitRouteTellsAStaleReviewFromAMalformedOne(t *testing.T) {
	base, c, csrf := openTagsFixture(t, untaggedTagsFixture(t))
	prop := proposeOpen(t, c, base)
	code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, func(e []map[string]any) []map[string]any {
		e[0]["text"] = "not what the document says"
		return e
	}))
	if code != http.StatusConflict {
		t.Errorf("a stale review: commit = %d %q, want 409", code, body)
	}
	code, body = postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, func(e []map[string]any) []map[string]any {
		e[0]["role"] = "Table"
		return e
	}))
	if code != http.StatusBadRequest {
		t.Errorf("a role nobody can choose: commit = %d %q, want 400", code, body)
	}
}

// TestTheTagRoutesReachTheirDoors — routing, per ADR-009 and S06's clause: propose reaches
// `pdfops.ProposeTags` (which the pdfops guard follows to `readPageLayout`), and commit reaches
// `pdfops.CommitTags` and installs through `commitMutation`.
func TestTheTagRoutesReachTheirDoors(t *testing.T) {
	src, err := os.ReadFile("tags.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	fn := func(name string) string {
		i := strings.Index(body, "func (s *Server) "+name+"(")
		if i < 0 {
			t.Fatalf("%s is gone", name)
		}
		j := strings.Index(body[i:], "\n}\n")
		return body[i : i+j]
	}
	if !strings.Contains(fn("handleTagsPropose"), "pdfops.ProposeTags(") {
		t.Error("handleTagsPropose does not reach pdfops.ProposeTags")
	}
	commit := fn("handleTagsCommit")
	for _, want := range []string{"pdfops.CommitTags(", "s.commitMutation(", "sign.HasSignatureBlob("} {
		if !strings.Contains(commit, want) {
			t.Errorf("handleTagsCommit does not call %s", want)
		}
	}
	edit := fn("handleTagsEdit")
	for _, want := range []string{"pdfops.EditStructure(", "s.commitMutation(", "sign.HasSignatureBlob("} {
		if !strings.Contains(edit, want) {
			t.Errorf("handleTagsEdit does not call %s", want)
		}
	}
	tree := fn("handleTagsTree")
	if !strings.Contains(tree, "pdfops.ReadStructure(") || strings.Contains(tree, "commitMutation") {
		t.Error("handleTagsTree must read through pdfops.ReadStructure and write nothing")
	}
}

// TestTheTreeRouteReadsTheTreeAndChangesNothing — P09.S06a: the Tags panel's read leaves the document
// byte-identical, publishes the tree the document holds, and answers a document with no tree as untagged.
func TestTheTreeRouteReadsTheTreeAndChangesNothing(t *testing.T) {
	base, c, _ := committedTagsFixture(t)
	before := getBytes(t, c, base+"/api/pdf")
	var tree tagTreeResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/tree"), &tree); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("reading the tree changed the open document")
	}
	if !tree.Tagged || len(tree.Elements) == 0 {
		t.Fatalf("a committed document reads tagged %v with %d element(s)", tree.Tagged, len(tree.Elements))
	}
	var top, want []string
	for _, e := range tree.Elements {
		if e.Parent == -1 {
			top = append(top, e.Standard)
		}
	}
	for _, r := range rootElements(t, before) {
		want = append(want, r.kind)
	}
	if strings.Join(top, " ") != strings.Join(want, " ") {
		t.Errorf("the tree's top level reads %v, the document holds %v", top, want)
	}
	for i, e := range tree.Elements {
		if e.Kids == nil {
			t.Errorf("element %d publishes null kids", i)
		}
		for _, k := range e.Kids {
			if tree.Elements[k].Parent != i {
				t.Errorf("element %d lists kid %d, whose parent reads %d", i, k, tree.Elements[k].Parent)
			}
		}
		if e.Text != "" {
			r, b := e.Rect, e.PageBox
			if !(r[0] < r[2] && r[1] < r[3] && r[0] >= b[0] && r[2] <= b[2] && r[1] >= b[1] && r[3] <= b[3]) {
				t.Errorf("element %d (%s %q) reads box %v on page box %v", i, e.Standard, e.Text, r, b)
			}
		}
	}

	untagged, uc, _ := openTagsFixture(t, untaggedTagsFixture(t))
	var none tagTreeResponse
	if err := json.Unmarshal(getBytes(t, uc, untagged+"/api/tags/tree"), &none); err != nil {
		t.Fatal(err)
	}
	if none.Tagged || none.Elements == nil || len(none.Elements) != 0 {
		t.Errorf("a document with no tree reads %+v — want untagged with an empty list", none)
	}
}

// The structure editor's route — `PLAN-accessibility.md` P09.S04.

// rootElement is one element directly under the structure tree root, as the document holds it.
type rootElement struct {
	obj    int
	kind   string
	hasAlt bool
}

// rootElements reads the elements directly under pdf's structure tree root; nil for a document with no
// tree. The server has no tree route until S06, so a test reads the document itself.
func rootElements(t *testing.T, pdf []byte) []rootElement {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	root, _ := ctx.DereferenceDict(cat["StructTreeRoot"])
	if root == nil {
		return nil
	}
	kids, _ := ctx.DereferenceArray(root["K"])
	var out []rootElement
	for _, k := range kids {
		ir, ok := k.(types.IndirectRef)
		if !ok {
			continue
		}
		d, _ := ctx.DereferenceDict(ir)
		if d == nil {
			continue
		}
		e := rootElement{obj: ir.ObjectNumber.Value()}
		if n := d.NameEntry("S"); n != nil {
			e.kind = *n
		}
		_, e.hasAlt = d["Alt"]
		out = append(out, e)
	}
	return out
}

// committedTagsFixture opens the untagged fixture and commits its proposal as proposed.
func committedTagsFixture(t *testing.T) (string, *http.Client, string) {
	t.Helper()
	base, c, csrf := openTagsFixture(t, untaggedTagsFixture(t))
	if code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(proposeOpen(t, c, base), nil)); code != http.StatusOK {
		t.Fatalf("setup: commit = %d: %s", code, body)
	}
	return base, c, csrf
}

// TestTheEditRouteCorrectsTheTreeAndOneUndoTakesTheBatchBack — a retype and an alt text in one batch
// reach the document, and ONE undo takes both back: a second undo is the commit's.
func TestTheEditRouteCorrectsTheTreeAndOneUndoTakesTheBatchBack(t *testing.T) {
	base, c, csrf := committedTagsFixture(t)
	els := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	if len(els) < 2 || els[0].kind != "H1" || els[1].kind != "P" || els[1].hasAlt {
		t.Fatalf("setup: the committed tree's top level reads %+v", els)
	}
	code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{
		{"kind": "retype", "element": els[0].obj, "value": "H2"},
		{"kind": "alt", "element": els[1].obj, "value": "An opening paragraph"},
	}})
	if code != http.StatusOK {
		t.Fatalf("edit = %d: %s", code, body)
	}
	edited := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	if len(edited) != len(els) || edited[0].kind != "H2" || !edited[1].hasAlt {
		t.Fatalf("after the edit the top level reads %+v", edited)
	}
	if code, body = postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	undone := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	if len(undone) != len(els) || undone[0].kind != "H1" || undone[1].hasAlt {
		t.Errorf("one undo left %+v — the batch is not one step", undone)
	}
	if code, body = postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("second undo = %d: %s", code, body)
	}
	if again := rootElements(t, getBytes(t, c, base+"/api/pdf")); again != nil {
		t.Errorf("a second undo should take back the commit, and the tree reads %+v", again)
	}
}

// TestTheEditRouteRefusesASignedDocumentAtTheDoor — whatever the client sends, and before any edit runs.
func TestTheEditRouteRefusesASignedDocumentAtTheDoor(t *testing.T) {
	base, c, csrf := openTagsFixture(t, threeSigned(t))
	before := getBytes(t, c, base+"/api/pdf")
	code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{
		{"kind": "alt", "element": 1, "value": "x"},
	}})
	if code != http.StatusConflict || !strings.Contains(body, "signed") {
		t.Errorf("a signed document: edit = %d %q, want 409 naming the signature", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Error("a refused edit changed the signed document")
	}
}

// TestTheEditRouteTellsAStaleEditFromAMalformedOne — 409 for an element or a tree the document no
// longer has; 400 for an edit no document could take. Nothing is written either way.
func TestTheEditRouteTellsAStaleEditFromAMalformedOne(t *testing.T) {
	base, c, csrf := committedTagsFixture(t)
	els := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	before := getBytes(t, c, base+"/api/pdf")
	for _, tc := range []struct {
		name string
		edit map[string]any
		want int
	}{
		{"an element the tree does not have", map[string]any{"kind": "alt", "element": 999999, "value": "x"}, http.StatusConflict},
		{"an edit that is not one", map[string]any{"kind": "paint", "element": els[0].obj}, http.StatusBadRequest},
		{"a type that is not standard", map[string]any{"kind": "retype", "element": els[0].obj, "value": "Bogus"}, http.StatusBadRequest},
	} {
		code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{tc.edit}})
		if code != tc.want {
			t.Errorf("%s: edit = %d %q, want %d", tc.name, code, body, tc.want)
		}
	}
	if code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{}}); code != http.StatusBadRequest {
		t.Errorf("no edits: edit = %d %q, want 400", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Error("a refused edit changed the document")
	}

	untagged, uc, ucsrf := openTagsFixture(t, untaggedTagsFixture(t))
	if code, body := postTags(t, uc, ucsrf, untagged+"/api/tags/edit", map[string]any{"edits": []map[string]any{
		{"kind": "alt", "element": 5, "value": "x"},
	}}); code != http.StatusConflict || strings.Contains(body, "propos") {
		t.Errorf("a document with no tree: edit = %d %q, want 409 in a tree's terms — the tree the reviewer read is gone", code, body)
	}
}

// TestAMoveWithNoIndexGoesToTheEnd — the route's one translation: a position the client leaves out
// appends, where Go's zero value would have put the element first.
func TestAMoveWithNoIndexGoesToTheEnd(t *testing.T) {
	base, c, csrf := committedTagsFixture(t)
	els := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	if code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{
		{"kind": "move", "element": els[0].obj},
	}}); code != http.StatusOK {
		t.Fatalf("edit = %d: %s", code, body)
	}
	moved := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	if len(moved) != len(els) || moved[len(moved)-1].kind != "H1" || moved[0].kind == "H1" {
		var kinds []string
		for _, e := range moved {
			kinds = append(kinds, e.kind)
		}
		t.Errorf("a move with no index reads %v — want the heading last", kinds)
	}
}
