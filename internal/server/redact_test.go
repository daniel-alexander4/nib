package server

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
)

func TestRedactReplacesPage(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path) // current document must exist

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(threePagePDF(t))
	iw, _ := mw.CreateFormFile("page", "page-2.png")
	img := image.NewRGBA(image.Rect(0, 0, 80, 110))
	png.Encode(iw, img)
	mw.WriteField("pageNum", "2")
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/redact", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redact status = %d, want 200", resp.StatusCode)
	}
	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	if n, _ := pdfops.PageCount(got); n != 3 {
		t.Errorf("redacted document page count = %d, want 3", n)
	}
}
