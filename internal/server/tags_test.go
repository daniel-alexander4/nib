package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
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

// ruledTableContent draws a ruled grid of two rows and two columns with a word in each cell (ADR-121).
const ruledTableContent = "100 700 m 400 700 l S 100 670 m 400 670 l S 100 640 m 400 640 l S " +
	"100 640 m 100 700 l S 250 640 m 250 700 l S 400 640 m 400 700 l S " +
	"BT /F1 12 Tf 106 680 Td (Name) Tj ET BT /F1 12 Tf 256 680 Td (Qty) Tj ET " +
	"BT /F1 12 Tf 106 650 Td (Apple) Tj ET BT /F1 12 Tf 256 650 Td (3) Tj ET"

// TestTheRoutesCarryATablesNesting — ADR-121 at the routes: the proposal publishes each element's parent as
// the door answers it, the commit writes the table, and a review that parts the table is a 400 saying why.
func TestTheRoutesCarryATablesNesting(t *testing.T) {
	pdf := testpdf.WithContent(ruledTableContent, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	want, err := pdfops.ProposeTags(pdf)
	if err != nil {
		t.Fatal(err)
	}
	base, c, csrf := openTagsFixture(t, pdf)
	prop := proposeOpen(t, c, base)
	var roles, parents []string
	for i, e := range prop.Elements {
		roles = append(roles, e.Role)
		parents = append(parents, strconv.Itoa(e.Parent))
		if e.Parent != want.Elements[i].Parent {
			t.Errorf("element %d: the route says parent %d and the door %d", i, e.Parent, want.Elements[i].Parent)
		}
	}
	if got := strings.Join(roles, " "); got != "Table TR TH TH TR TD TD" {
		t.Fatalf("the route proposes %s", got)
	}
	if got := strings.Join(parents, " "); got != "-1 0 1 1 0 4 4" {
		t.Errorf("the route publishes parents %s", got)
	}
	code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, func(e []map[string]any) []map[string]any {
		e[2], e[3] = e[3], e[2] // the header row's two cells exchanged
		return e
	}))
	if code != http.StatusBadRequest || !strings.Contains(body, "move the whole table") {
		t.Errorf("a parted table: %d %s — want 400 and the way out", code, body)
	}
	code, body = postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, nil))
	if code != http.StatusOK {
		t.Fatalf("commit = %d: %s", code, body)
	}
	var tree tagTreeResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/tree"), &tree); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, e := range tree.Elements {
		k := e.Kind
		if e.Scope != "" {
			k += "/" + e.Scope
		}
		kinds = append(kinds, k)
	}
	if got := strings.Join(kinds, " "); got != "Table TR TH/Column TH/Column TR TD TD" {
		t.Errorf("the committed tree reads %s", got)
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
	// The signed refusal is tagwrite's, shared with the CLI (P10.S02); `tagdoor_test.go` holds that nothing
	// reaches the pdfops writers around it.
	for _, want := range []string{"tagwrite.Commit(", "s.commitMutation("} {
		if !strings.Contains(commit, want) {
			t.Errorf("handleTagsCommit does not call %s", want)
		}
	}
	edit := fn("handleTagsEdit")
	for _, want := range []string{"tagwrite.Edit(", "s.commitMutation("} {
		if !strings.Contains(edit, want) {
			t.Errorf("handleTagsEdit does not call %s", want)
		}
	}
	tree := fn("handleTagsTree")
	if !strings.Contains(tree, "pdfops.ReadStructure(") || strings.Contains(tree, "commitMutation") {
		t.Error("handleTagsTree must read through pdfops.ReadStructure and write nothing")
	}
	untagged := fn("handleTagsUntagged")
	if !strings.Contains(untagged, "pdfops.ReadUntagged(") || strings.Contains(untagged, "commitMutation") || strings.Contains(untagged, "tagwrite.") {
		t.Error("handleTagsUntagged must read through pdfops.ReadUntagged and write nothing")
	}
}

// TestTheCLIsJSONIsTheRoutesShape — P10.S01. `nib tag tree --json` and `nib tag propose --json` print the
// pdfops doors' values; the routes answer with this package's views of the same values. Two definitions of
// one shape drift unless something holds them equal, and this is it: every field's JSON tag, options
// included, the same on both sides.
func TestTheCLIsJSONIsTheRoutesShape(t *testing.T) {
	tags := func(v any) []string {
		rt := reflect.TypeOf(v)
		var out []string
		for i := 0; i < rt.NumField(); i++ {
			tag := rt.Field(i).Tag.Get("json")
			if tag == "" {
				t.Errorf("%s.%s carries no json tag, so its JSON name is the Go name and nothing holds it", rt.Name(), rt.Field(i).Name)
			}
			out = append(out, tag)
		}
		sort.Strings(out)
		return out
	}
	for _, c := range []struct{ door, route any }{
		{pdfops.StructureTree{}, tagTreeResponse{}},
		{pdfops.StructureElement{}, tagTreeElementView{}},
		{pdfops.RoleMapping{}, tagRoleView{}},
		{pdfops.TagProposal{}, tagProposalResponse{}},
		{pdfops.TagElement{}, tagElementView{}},
		{pdfops.TagPageNote{}, tagPageView{}},
		{pdfops.UntaggedContent{}, untaggedResponse{}},
		{pdfops.UntaggedPiece{}, untaggedPieceView{}},
	} {
		d, r := tags(c.door), tags(c.route)
		if strings.Join(d, " ") != strings.Join(r, " ") {
			t.Errorf("%T prints %v and the route's %T answers %v — the CLI and the panel would read different shapes",
				c.door, d, c.route, r)
		}
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
// tree. Read from the document itself, independently of the tree route, so the route's answer
// (`TestTheTreeRouteReadsTheTreeAndChangesNothing`) is compared with something that does not share its code.
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

// TestTheEditRouteCreatesATagFillsItAndDeletesItAgain — ADR-124 through the route's own JSON: a create
// places a new element among the root's, a move puts an element in it, and a delete takes the tag away and
// leaves what it held where it was. Each batch is one undo.
func TestTheEditRouteCreatesATagFillsItAndDeletesItAgain(t *testing.T) {
	base, c, csrf := committedTagsFixture(t)
	kinds := func(els []rootElement) string {
		var k []string
		for _, e := range els {
			k = append(k, e.kind)
		}
		return strings.Join(k, " ")
	}
	els := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	if len(els) < 3 || els[0].kind != "H1" || els[1].kind != "P" {
		t.Fatalf("setup: the committed tree's top level reads %s", kinds(els))
	}
	start := kinds(els)
	edit := func(what string, edits ...map[string]any) []rootElement {
		t.Helper()
		if code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": edits}); code != http.StatusOK {
			t.Fatalf("%s = %d: %s", what, code, body)
		}
		return rootElements(t, getBytes(t, c, base+"/api/pdf"))
	}

	made := edit("create", map[string]any{"kind": "create", "value": "Sect", "parent": 0, "index": 1})
	if len(made) != len(els)+1 || made[1].kind != "Sect" || made[0].obj != els[0].obj || made[2].obj != els[1].obj {
		t.Fatalf("after the create the top level reads %s — want a Sect second", kinds(made))
	}
	sect := made[1].obj
	filled := edit("move into it", map[string]any{"kind": "move", "element": els[1].obj, "parent": sect})
	if len(filled) != len(els) || filled[1].obj != sect || filled[2].obj != els[2].obj {
		t.Fatalf("after the move the top level reads %s — want the paragraph gone into the Sect", kinds(filled))
	}
	var tree tagTreeResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/tree"), &tree); err != nil {
		t.Fatal(err)
	}
	for i, e := range tree.Elements {
		if e.ID == sect && (len(e.Kids) != 1 || tree.Elements[e.Kids[0]].ID != els[1].obj || e.Text == "") {
			t.Errorf("the tree route reads the new Sect (element %d of the answer) as %+v — want the paragraph and its text under it", i, e)
		}
	}

	// A top-level element that holds content cannot be deleted, and the refusal is the sentence.
	before := getBytes(t, c, base+"/api/pdf")
	code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{{"kind": "delete", "element": els[0].obj}}})
	if code != http.StatusBadRequest || !strings.Contains(body, "top of the structure tree") || !strings.Contains(body, "change its type") {
		t.Errorf("deleting a top-level heading: edit = %d %q, want 400 saying why and what to do instead", code, body)
	}
	for _, bad := range []map[string]any{
		{"kind": "create", "value": "Bogus"},
		{"kind": "create", "value": "Div", "element": sect},
	} {
		if code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{bad}}); code != http.StatusBadRequest {
			t.Errorf("%v: edit = %d %q, want 400", bad, code, body)
		}
	}
	if code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{{"kind": "create", "value": "Div", "parent": 999999}}}); code != http.StatusConflict {
		t.Errorf("a create under an element the tree does not have: edit = %d %q, want 409", code, body)
	}
	if code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{{"kind": "delete", "element": 999999}}}); code != http.StatusConflict {
		t.Errorf("a delete of an element the tree does not have: edit = %d %q, want 409", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Error("a refused edit changed the document")
	}

	gone := edit("delete", map[string]any{"kind": "delete", "element": sect})
	if kinds(gone) != start || gone[1].obj != els[1].obj {
		t.Errorf("with the Sect deleted the top level reads %s — want %s, the paragraph back where the Sect was", kinds(gone), start)
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if undone := rootElements(t, getBytes(t, c, base+"/api/pdf")); len(undone) != len(els) || undone[1].obj != sect {
		t.Errorf("one undo of the delete left %s — want the Sect back", kinds(undone))
	}

	// A batch is one step: a create and a delete of something else together, taken back by one undo.
	sizeBefore := kinds(rootElements(t, getBytes(t, c, base+"/api/pdf")))
	batch := edit("a batch", map[string]any{"kind": "create", "value": "Div"}, map[string]any{"kind": "delete", "element": sect})
	if len(batch) != len(els)+1 || batch[len(batch)-1].kind != "Div" {
		t.Fatalf("after the batch the top level reads %s", kinds(batch))
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if got := kinds(rootElements(t, getBytes(t, c, base+"/api/pdf"))); got != sizeBefore {
		t.Errorf("one undo of a two-edit batch left %s, want %s", got, sizeBefore)
	}
}

// TestCreateAndDeleteAreRefusedOnASignedDocumentAtTheDoor — the same door as every other edit.
func TestCreateAndDeleteAreRefusedOnASignedDocumentAtTheDoor(t *testing.T) {
	base, c, csrf := openTagsFixture(t, threeSigned(t))
	before := getBytes(t, c, base+"/api/pdf")
	for _, e := range []map[string]any{
		{"kind": "create", "value": "Div"},
		{"kind": "delete", "element": 1},
	} {
		code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{e}})
		if code != http.StatusConflict || !strings.Contains(body, "signed") {
			t.Errorf("%v on a signed document: edit = %d %q, want 409 naming the signature", e, code, body)
		}
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

// TestTheEditRouteCarriesACellsSpansAndHeaders — the cell edits (ADR-119) reach the document through the
// route's own JSON, and the tree route answers them back: spans as numbers, headers as element indices.
func TestTheEditRouteCarriesACellsSpansAndHeaders(t *testing.T) {
	base, c, csrf := openTagsFixture(t, testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4:  "<< /Length 1 >>\nstream\n \nendstream",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8:  "<< /Type /StructElem /S /Table /P 7 0 R /K [10 0 R 11 0 R] >>",
		10: "<< /Type /StructElem /S /TR /P 8 0 R /K [12 0 R] >>",
		11: "<< /Type /StructElem /S /TR /P 8 0 R /K [14 0 R 15 0 R] >>",
		12: "<< /Type /StructElem /S /TH /P 10 0 R >>",
		14: "<< /Type /StructElem /S /TD /P 11 0 R >>",
		15: "<< /Type /StructElem /S /TD /P 11 0 R >>",
	}))
	code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{
		{"kind": "colspan", "element": 12, "value": "2"},
		{"kind": "rowspan", "element": 14, "value": "1"},
		{"kind": "headers", "element": 15, "headers": []int{12}},
	}})
	if code != http.StatusOK {
		t.Fatalf("edit = %d: %s", code, body)
	}
	var tree tagTreeResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/tree"), &tree); err != nil {
		t.Fatal(err)
	}
	at := map[int]int{}
	for i, e := range tree.Elements {
		at[e.ID] = i
	}
	th, td := tree.Elements[at[12]], tree.Elements[at[15]]
	if th.ColSpan != 2 || th.RowSpan != 1 || len(td.Headers) != 1 || td.Headers[0] != at[12] || tree.Elements[at[14]].Headers == nil {
		t.Errorf("the tree route answers the header %+v and the cell %+v", th, td)
	}
	for _, bad := range []map[string]any{
		{"kind": "colspan", "element": 12, "value": "none"},
		{"kind": "headers", "element": 15, "headers": []int{14}},
	} {
		if code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{bad}}); code != http.StatusBadRequest {
			t.Errorf("a malformed cell edit %v answered %d: %s", bad, code, body)
		}
	}
	if code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{{"kind": "headers", "element": 15, "headers": []int{99}}}}); code != http.StatusConflict {
		t.Errorf("a header the tree does not have answered %d: %s", code, body)
	}
}

// TestTheRemoveRouteTakesTheTreeAwayAndOneUndoBringsItBack — ADR-120: the document reads untagged, the
// proposal that was refused can be committed, and one undo of the removal restores the tree it had.
func TestTheRemoveRouteTakesTheTreeAwayAndOneUndoBringsItBack(t *testing.T) {
	base, c, csrf := committedTagsFixture(t)
	had := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	if len(had) == 0 {
		t.Fatal("setup: the committed fixture has no tree")
	}
	if code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(proposeOpen(t, c, base), nil)); code == http.StatusOK || !strings.Contains(body, "remove its tags first") {
		t.Fatalf("committing over the tree = %d %q, want a refusal naming the way out", code, body)
	}
	if code, body := postTags(t, c, csrf, base+"/api/tags/remove", map[string]any{}); code != http.StatusOK {
		t.Fatalf("remove = %d: %s", code, body)
	}
	var tree tagTreeResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/tree"), &tree); err != nil || tree.Tagged {
		t.Fatalf("after remove the tree route answers tagged %v (%v)", tree.Tagged, err)
	}
	if code, body := postTags(t, c, csrf, base+"/api/tags/remove", map[string]any{}); code != http.StatusConflict {
		t.Errorf("removing the tags of an untagged document = %d %q, want 409", code, body)
	}
	if code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(proposeOpen(t, c, base), nil)); code != http.StatusOK {
		t.Fatalf("committing after remove = %d: %s", code, body)
	}
	for i := 0; i < 2; i++ { // the second commit, then the removal
		if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
			t.Fatalf("undo %d = %d: %s", i+1, code, body)
		}
	}
	if back := rootElements(t, getBytes(t, c, base+"/api/pdf")); !reflect.DeepEqual(back, had) {
		t.Errorf("undoing the removal left %+v, and the tree was %+v", back, had)
	}
}

// TestTheRemoveRouteRefusesASignedDocumentAtTheDoor — and changes nothing.
func TestTheRemoveRouteRefusesASignedDocumentAtTheDoor(t *testing.T) {
	base, c, csrf := openTagsFixture(t, threeSigned(t))
	before := getBytes(t, c, base+"/api/pdf")
	if code, body := postTags(t, c, csrf, base+"/api/tags/remove", map[string]any{}); code != http.StatusConflict || !strings.Contains(body, "signed") {
		t.Errorf("a signed document: remove = %d %q, want 409 naming the signature", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Error("a refused removal changed the signed document")
	}
}

// picturedPDF is an untagged page with a line of text and one image XObject drawn 100 by 80 points (ADR-122).
func picturedPDF() []byte {
	content := "BT /F1 12 Tf 72 700 Td (Above the picture) Tj ET q 100 0 0 80 50 500 cm /Im0 Do Q"
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\nabcd\nendstream",
	})
}

// TestTheCommitRouteWritesAFigureOnlyWithItsDescription — ADR-122 at the route: the proposal names the picture
// as a Figure, a review that keeps it with no `alt` is a malformed review (400) answered with the reason, and
// with one the tree route reads the Figure and what was written for it.
func TestTheCommitRouteWritesAFigureOnlyWithItsDescription(t *testing.T) {
	base, c, csrf := openTagsFixture(t, picturedPDF())
	prop := proposeOpen(t, c, base)
	if len(prop.Elements) != 2 || prop.Elements[1].Role != "Figure" || prop.Elements[1].Text != "" || prop.Elements[1].Rect != [4]float64{50, 500, 150, 580} {
		t.Fatalf("proposed %+v, want a paragraph and a Figure at 50 500 150 580", prop.Elements)
	}
	before := getBytes(t, c, base+"/api/pdf")
	code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, nil))
	if code != http.StatusBadRequest || !strings.Contains(body, "a figure needs a description of what it shows, or must be ignored") {
		t.Errorf("a figure kept with no description: commit = %d %q, want 400 saying it needs one", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Error("a refused commit changed the document")
	}
	const alt = `A chart (2024) \ ü`
	code, body = postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, func(e []map[string]any) []map[string]any {
		e[1]["alt"] = alt
		return e
	}))
	if code != http.StatusOK {
		t.Fatalf("a described figure: commit = %d %q", code, body)
	}
	var tree tagTreeResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/tree"), &tree); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range tree.Elements {
		got = append(got, e.Kind+" "+e.Alt)
	}
	if !reflect.DeepEqual(got, []string{"P ", "Figure " + alt}) {
		t.Errorf("the tree reads %q, want the paragraph and the Figure with its description", got)
	}
}

// ignoredParagraphFixture opens the untagged fixture and commits its proposal with the opening paragraph
// ignored — written as decoration — and answers that paragraph's text (ADR-125).
func ignoredParagraphFixture(t *testing.T) (base string, c *http.Client, csrf, words string) {
	t.Helper()
	base, c, csrf = openTagsFixture(t, untaggedTagsFixture(t))
	prop := proposeOpen(t, c, base)
	at := -1
	for i, e := range prop.Elements {
		if e.Role == "P" && at < 0 {
			at = i
		}
	}
	if at < 0 {
		t.Fatal("setup: the proposal holds no paragraph")
	}
	code, body := postTags(t, c, csrf, base+"/api/tags/commit", reviewBody(prop, func(e []map[string]any) []map[string]any {
		e[at]["ignore"] = true
		return e
	}))
	if code != http.StatusOK {
		t.Fatalf("setup: commit = %d: %s", code, body)
	}
	return base, c, csrf, prop.Elements[at].Text
}

// getAnswer is a GET's status and body, for the answers that are not 200.
func getAnswer(t *testing.T, c *http.Client, url string) (int, string) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func squashed(s string) string { return strings.Join(strings.Fields(s), "") }

// TestTheUntaggedRouteListsWhatNoTagOwnsAndChangesNothing — ADR-125: the ignored paragraph is listed on its
// page as decoration, placed on the page as displayed; the read leaves the document byte-identical; and a
// page that is not a number, or not a page of the document, is a 400 with the reason.
func TestTheUntaggedRouteListsWhatNoTagOwnsAndChangesNothing(t *testing.T) {
	base, c, _, words := ignoredParagraphFixture(t)
	before := getBytes(t, c, base+"/api/pdf")
	var got untaggedResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/untagged?page=1"), &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Error("reading what is untagged changed the document")
	}
	if got.Pages != 1 || len(got.Pieces) != 1 {
		t.Fatalf("page 1 of %d lists %+v, want the one ignored paragraph", got.Pages, got.Pieces)
	}
	p := got.Pieces[0]
	if p.Kind != "text" || squashed(p.Text) != squashed(words) || !p.Decoration || p.InForm || p.Page != 1 {
		t.Errorf("the ignored paragraph is listed as %+v, want the text %q marked as decoration", p, words)
	}
	if r := p.Rect; !(0 <= r[0] && r[0] < r[2] && r[2] <= 1 && 0 <= r[1] && r[1] < r[3] && r[3] <= 1) {
		t.Errorf("the piece is placed at %v, which is not a rectangle on the displayed page", r)
	}
	for _, c2 := range []struct{ query, says string }{
		{"", "page must be a page number"},
		{"?page=one", "page must be a page number"},
		{"?page=0", "page must be a page number"},
		{"?page=-1", "page must be a page number"},
		{"?page=2", "page 2"},
	} {
		if code, body := getAnswer(t, c, base+"/api/tags/untagged"+c2.query); code != http.StatusBadRequest || !strings.Contains(body, c2.says) {
			t.Errorf("untagged%s = %d %q, want 400 saying %q", c2.query, code, body, c2.says)
		}
	}
	// A document with no tree at all: everything it draws is untagged, and the route still answers.
	ubase, uc, _ := openTagsFixture(t, untaggedTagsFixture(t))
	var bare untaggedResponse
	if err := json.Unmarshal(getBytes(t, uc, ubase+"/api/tags/untagged?page=1"), &bare); err != nil || len(bare.Pieces) < 4 {
		t.Errorf("an untagged document lists %d piece(s) (%v), want its headings, paragraphs and list", len(bare.Pieces), err)
	}
	for _, p := range bare.Pieces {
		if p.Decoration {
			t.Errorf("an untagged document's bare text is listed as decoration: %+v", p)
		}
	}
}

// TestARegionThroughTheEditRouteTagsWhatWasIgnoredAndOneUndoTakesItBack — ADR-125 through the route's own
// JSON: the pieces the untagged route lists are what a region edit takes; the tree gains one paragraph that
// reads the words, placed where the edit says; the page lists nothing afterwards; and one undo takes it back.
func TestARegionThroughTheEditRouteTagsWhatWasIgnoredAndOneUndoTakesItBack(t *testing.T) {
	base, c, csrf, words := ignoredParagraphFixture(t)
	had := rootElements(t, getBytes(t, c, base+"/api/pdf"))
	var listed untaggedResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/untagged?page=1"), &listed); err != nil || len(listed.Pieces) != 1 {
		t.Fatalf("setup: the page lists %+v (%v)", listed.Pieces, err)
	}
	rect := listed.Pieces[0].Rect
	region := func(over map[string]any) map[string]any {
		e := map[string]any{"kind": "region", "value": "P", "page": 1, "pieces": [][4]float64{rect}, "parent": -1, "index": 1}
		for k, v := range over {
			if v == nil {
				delete(e, k)
			} else {
				e[k] = v
			}
		}
		return map[string]any{"edits": []map[string]any{e}}
	}
	before := getBytes(t, c, base+"/api/pdf")
	for _, bad := range []struct {
		name string
		over map[string]any
		code int
		says string
	}{
		{"a figure with no description", map[string]any{"value": "Figure"}, http.StatusBadRequest, "only with a description of what it shows"},
		{"a type that is not standard", map[string]any{"value": "Paragraph"}, http.StatusBadRequest, "not a standard structure type"},
		{"a page the document does not have", map[string]any{"page": 7}, http.StatusBadRequest, "page 7 is not a page of this document"},
		{"no rectangle at all", map[string]any{"pieces": nil}, http.StatusBadRequest, "a region is a rectangle on the page"},
		{"a rectangle past the page", map[string]any{"pieces": nil, "rect": []float64{0, 0, 1.5, 1}}, http.StatusBadRequest, "a region is a rectangle on the page"},
		{"a rectangle of three numbers", map[string]any{"pieces": nil, "rect": []float64{0, 0, 1}}, http.StatusBadRequest, "could not read the edits"},
		{"an empty part of the page", map[string]any{"pieces": nil, "rect": []float64{0.9, 0.9, 0.99, 0.99}}, http.StatusBadRequest, "nothing untagged is in that region of page 1"},
		{"an element named", map[string]any{"element": 5}, http.StatusBadRequest, "leave the element out"},
		{"a parent the tree does not have", map[string]any{"parent": 9999}, http.StatusConflict, "element 9999"},
	} {
		if code, body := postTags(t, c, csrf, base+"/api/tags/edit", region(bad.over)); code != bad.code || !strings.Contains(body, bad.says) {
			t.Errorf("%s: edit = %d %q, want %d saying %q", bad.name, code, body, bad.code, bad.says)
		}
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("a refused region changed the document")
	}

	// The whole page as one rectangle — what the pointer would send — takes the same paragraph.
	if code, body := postTags(t, c, csrf, base+"/api/tags/edit", region(map[string]any{"pieces": nil, "rect": []float64{0, 0, 1, 1}})); code != http.StatusOK {
		t.Fatalf("a region over the whole page = %d: %s", code, body)
	}
	if got := rootElements(t, getBytes(t, c, base+"/api/pdf")); len(got) != len(had)+1 {
		t.Errorf("a region over the whole page left %d top-level elements, want one more than %d", len(got), len(had))
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("one undo did not take the region back")
	}

	if code, body := postTags(t, c, csrf, base+"/api/tags/edit", region(nil)); code != http.StatusOK {
		t.Fatalf("a region of the listed piece = %d: %s", code, body)
	}
	var tree tagTreeResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/tree"), &tree); err != nil {
		t.Fatal(err)
	}
	if got := rootElements(t, getBytes(t, c, base+"/api/pdf")); len(got) != len(had)+1 || tree.Elements[1].Kind != "P" || squashed(tree.Elements[1].Text) != squashed(words) || tree.Elements[1].Parent != -1 {
		t.Errorf("the tree's second element reads %+v among %d at the top, want the paragraph %q, second of %d", tree.Elements[1], len(got), words, len(had)+1)
	}
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/untagged?page=1"), &listed); err != nil || len(listed.Pieces) != 0 {
		t.Errorf("with the paragraph tagged the page still lists %+v (%v)", listed.Pieces, err)
	}
	var st struct {
		CanUndo bool `json:"canUndo"`
	}
	if err := json.Unmarshal(getBytes(t, c, base+"/api/doc"), &st); err != nil || !st.CanUndo {
		t.Errorf("after a region the document cannot be undone (%v)", err)
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if back := rootElements(t, getBytes(t, c, base+"/api/pdf")); !reflect.DeepEqual(back, had) {
		t.Errorf("undoing the region left %+v, and the tree was %+v", back, had)
	}
}

// TestARegionIsRefusedOnASignedDocumentAtTheDoor — and changes nothing; the read of what is untagged is still
// answered, because it writes nothing.
func TestARegionIsRefusedOnASignedDocumentAtTheDoor(t *testing.T) {
	base, c, csrf := openTagsFixture(t, threeSigned(t))
	before := getBytes(t, c, base+"/api/pdf")
	code, body := postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": []map[string]any{
		{"kind": "region", "value": "P", "page": 1, "rect": []float64{0, 0, 1, 1}},
	}})
	if code != http.StatusConflict || !strings.Contains(body, "signed") {
		t.Errorf("a signed document: region = %d %q, want 409 naming the signature", code, body)
	}
	if code, body := getAnswer(t, c, base+"/api/tags/untagged?page=1"); code != http.StatusOK {
		t.Errorf("a signed document: untagged = %d %q, want the read answered", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Error("a refused region changed the signed document")
	}
}
