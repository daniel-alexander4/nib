package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"testing"
)

// /pending 591 — a page-number field that is sent and is not a whole number is refused.
//
// The three parses discarded their error, so "abc" stamped as 0 — which StampPageNumbers reads as
// its default — and the request answered 200 for numbering nobody asked for. A field left OUT is
// still the default: that is the control, and without it this test would pass on a route that
// refused every page-number request.
func TestAMalformedPageNumberFieldIsRefused(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)
	pdf := threePagePDF(t)

	post := func(fields map[string]string) int {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(pdf)
		mw.WriteField("op", "pagenum")
		for k, v := range fields {
			mw.WriteField(k, v)
		}
		mw.Close()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
		resp.Body.Close()
		return resp.StatusCode
	}

	if got := post(map[string]string{"start": "3", "pad": "2", "size": "12"}); got != http.StatusOK {
		t.Fatalf("control: well-formed page numbering answered %d, want 200", got)
	}
	if got := post(nil); got != http.StatusOK {
		t.Fatalf("control: page numbering with the three fields left out answered %d, want 200 (the defaults)", got)
	}
	for _, field := range []string{"start", "pad", "size"} {
		for _, bad := range []string{"abc", "1.5"} {
			if got := post(map[string]string{field: bad}); got != http.StatusBadRequest {
				t.Errorf("page numbering with %s=%s answered %d, want 400 — the value was read as 0 and stamped",
					field, bad, got)
			}
		}
	}
}
