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
}
