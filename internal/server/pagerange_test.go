package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"

	"nib/internal/pdfops"
)

// TestPageRoutesRefuseAPageTheDocumentDoesNotHave walks /pending 823 to the observable: rotate, delete and crop of
// a page past the end used to come back 200 with the document unchanged — a delete reported done that removed
// nothing — and extract and the cover/stamp bake took a page 0 or past the end the same way. Each route now answers
// the one page-range refusal (422, cause page-not-in-document, the sentence in `error`) and the open document's
// bytes are exactly what they were.
func TestPageRoutesRefuseAPageTheDocumentDoesNotHave(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	served := func() []byte {
		t.Helper()
		resp, err := c.Get(ts.URL + "/api/pdf")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	before := served()
	n, err := pdfops.PageCount(before)
	if err != nil {
		t.Fatal(err)
	}
	past := strconv.Itoa(n + 1)

	post := func(route string, fields map[string]string) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(before)
		for k, v := range fields {
			mw.WriteField(k, v)
		}
		mw.Close()
		return write(t, c, csrf, http.MethodPost, ts.URL+route, mw.FormDataContentType(), &buf)
	}
	cover := `[{"page":0,"rect":[10,10,60,30],"png":"` + pngBase64(t) + `"}]`
	for _, tc := range []struct {
		name, route string
		fields      map[string]string
	}{
		{"rotate", "/api/pages", map[string]string{"op": "rotate", "deg": "90", "pages": past}},
		{"delete", "/api/pages", map[string]string{"op": "delete", "pages": past}},
		{"reorder", "/api/pages", map[string]string{"op": "reorder", "pages": "1," + past}},
		{"crop", "/api/pages", map[string]string{"op": "crop", "rect": "[0.1,0.1,0.5,0.5]", "pages": past}},
		{"extract", "/api/extract", map[string]string{"pages": past}},
		{"bake a cover on page 0", "/api/bake", map[string]string{"covers": cover}},
	} {
		resp := post(tc.route, tc.fields)
		var got pageRefusal
		decErr := json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", tc.name, resp.StatusCode)
			continue
		}
		if decErr != nil || got.Cause != "page-not-in-document" || got.Pages != n || got.Error == "" {
			t.Errorf("%s: body = %+v (%v), want cause page-not-in-document against %d page(s) with a sentence", tc.name, got, decErr, n)
		}
		if after := served(); !bytes.Equal(after, before) {
			t.Errorf("%s: the refused operation changed the open document", tc.name)
		}
	}
}

// pngBase64 is a small opaque PNG, base64'd as the bake's covers carry it.
func pngBase64(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 8))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}
