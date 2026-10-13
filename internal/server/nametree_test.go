package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/testpdf"
)

// TestAnUnreadNameTreeRefusalIsA422NamingItsCause walks ADR-164 to the route (`/pending 880`): a document whose
// list of embedded files pdfcpu's validator dropped is refused by the write door, and a mutation route answers
// that as 422 with cause `name-tree` and the sentence — not as its own handler's 400 or 500.
func TestAnUnreadNameTreeRefusalIsA422NamingItsCause(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)
	doc := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: 3, Loop: true})
	for _, tc := range []struct {
		route  string
		fields map[string]string
	}{
		{"/api/pages", map[string]string{"op": "rotate", "deg": "90", "pages": "1"}},
		{"/api/outline", map[string]string{"outline": `[{"title":"One","page":1,"level":0}]`}},
	} {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(doc)
		for k, v := range tc.fields {
			mw.WriteField(k, v)
		}
		mw.Close()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+tc.route, mw.FormDataContentType(), &buf)
		var got map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity || got["cause"] != "name-tree" || got["error"] == "" {
			t.Errorf("%s: status %d, body %v — want 422 with cause name-tree and the sentence", tc.route, resp.StatusCode, got)
		}
	}
}
