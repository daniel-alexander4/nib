package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
)

// TestOutlineWiring locks the read/write rail: POST a baked PDF + outline JSON,
// then GET the outline back. The per-operation behaviour is in the pdfops tests.
func TestOutlineWiring(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	doc, err := pdfops.ImagesToPDF([]pdfops.RasterPage{
		{Image: pageImage(t, 120, 160), W: 120, H: 160},
		{Image: pageImage(t, 120, 160), W: 120, H: 160},
		{Image: pageImage(t, 120, 160), W: 120, H: 160},
	})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(doc)
	mw.WriteField("outline", `[{"title":"Intro","page":1,"level":0},{"title":"Details","page":2,"level":1}]`)
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/outline", mw.FormDataContentType(), &buf)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set outline status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = c.Get(ts.URL + "/api/outline")
	var got outlineResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("outline response not JSON: %v", err)
	}
	resp.Body.Close()
	if len(got.Items) != 2 || got.Items[0].Title != "Intro" || got.Items[1].Level != 1 {
		t.Fatalf("read back %+v, want Intro(level 0) + Details(level 1)", got.Items)
	}
}
