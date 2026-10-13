package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/testpdf"
)

// TestAnAssociatedFileRefusalIsA422NamingItsCause walks `/pending 655` to the route: a document whose catalog
// `/AF` names a file nothing else reaches is refused by the write door, and each mutation route answers that as
// 422 with cause `associated-file` and the sentence — not as the 400 or 500 its own handler gives a failure.
func TestAnAssociatedFileRefusalIsA422NamingItsCause(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)
	doc := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AF [5 0 R] >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		5: "<< /Type /Filespec /F (a.csv) /UF (a.csv) /AFRelationship /Source /EF << /F 6 0 R >> >>",
		6: "<< /Type /EmbeddedFile /Length 5 >>\nstream\nBYTES\nendstream",
	})
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
		if resp.StatusCode != http.StatusUnprocessableEntity || got["cause"] != "associated-file" || got["error"] == "" {
			t.Errorf("%s: status %d, body %v — want 422 with cause associated-file and the sentence", tc.route, resp.StatusCode, got)
		}
	}
}
