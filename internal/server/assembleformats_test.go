package server

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// TestAssemblePackagesPagesInEveryPictureFormat drives /api/assemble (ADR-189) with two rendered pages and reads
// each answer back as the format it claims to be: the route is the only door the window's GIF, BMP, TIFF, CBZ and
// ZIP exports go through, and a format that came back as the wrong bytes would be saved under the right name.
func TestAssemblePackagesPagesInEveryPictureFormat(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	var pg bytes.Buffer
	png.Encode(&pg, image.NewRGBA(image.Rect(0, 0, 30, 20)))
	send := func(format string, pages int) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for i := 0; i < pages; i++ {
			fw, _ := mw.CreateFormFile("image", "page.png")
			fw.Write(pg.Bytes())
			mw.WriteField("pageW", "15")
			mw.WriteField("pageH", "10")
		}
		mw.WriteField("format", format)
		mw.Close()
		return write(t, c, csrf, http.MethodPost, ts.URL+"/api/assemble", mw.FormDataContentType(), &buf)
	}
	for _, tc := range []struct {
		format, mime string
		pages        int
		magic        string
	}{
		{"tiff", "image/tiff", 2, "II*\x00"},
		{"gif", "image/gif", 1, "GIF8"},
		{"bmp", "image/bmp", 1, "BM"},
		{"cbz", "application/vnd.comicbook+zip", 2, "PK"},
		{"zip", "application/zip", 2, "PK"},
	} {
		resp := send(tc.format, tc.pages)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s: status %d: %s", tc.format, resp.StatusCode, body)
			continue
		}
		if got := resp.Header.Get("Content-Type"); got != tc.mime {
			t.Errorf("%s: content type %q", tc.format, got)
		}
		if !strings.HasPrefix(string(body), tc.magic) {
			t.Errorf("%s: the bytes begin %q", tc.format, body[:4])
		}
		if tc.magic == "PK" {
			zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
			if err != nil || len(zr.File) != 2 {
				t.Errorf("%s: %v, %d members", tc.format, err, len(zr.File))
			}
		}
	}
	// One picture only for GIF and BMP: two is a 400, never a silently dropped page.
	resp := send("gif", 2)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("two pages as a GIF answered %d, want 400", resp.StatusCode)
	}
	// A picture that is not a PNG is the caller's fault: 400 with a reason, not a 500.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("image", "page.png")
	fw.Write([]byte("not a picture"))
	mw.WriteField("format", "tiff")
	mw.Close()
	bad := write(t, c, csrf, http.MethodPost, ts.URL+"/api/assemble", mw.FormDataContentType(), &buf)
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("a non-PNG page answered %d, want 400", bad.StatusCode)
	}
}
