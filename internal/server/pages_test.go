package server

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
)

func threePagePDF(t *testing.T) []byte {
	t.Helper()
	var pages []pdfops.RasterPage
	for i := 0; i < 3; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 160, 220))
		img.Set(0, 0, color.RGBA{0, 0, 0, 255})
		var buf bytes.Buffer
		png.Encode(&buf, img)
		pages = append(pages, pdfops.RasterPage{Image: buf.Bytes(), W: 80, H: 110})
	}
	pdf, err := pdfops.ImagesToPDF(pages)
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func TestPageDeleteUpdatesDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path) // ensures a current document exists

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(threePagePDF(t))
	mw.WriteField("op", "delete")
	mw.WriteField("pages", "2")
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page delete status = %d, want 200", resp.StatusCode)
	}

	// The current document should now be the 2-page result.
	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	if n, _ := pdfops.PageCount(got); n != 2 {
		t.Errorf("current document page count = %d, want 2", n)
	}
}
