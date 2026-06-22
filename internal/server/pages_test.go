package server

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func pageImage(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{0, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func threePagePDF(t *testing.T) []byte {
	t.Helper()
	var pages []pdfops.RasterPage
	for i := 0; i < 3; i++ {
		pages = append(pages, pdfops.RasterPage{Image: pageImage(t, 160, 220), W: 80, H: 110})
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

func TestInsertBlankUpdatesDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(threePagePDF(t))
	mw.WriteField("op", "insertblank")
	mw.WriteField("page", "2")
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("insert blank status = %d, want 200", resp.StatusCode)
	}

	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	if n, _ := pdfops.PageCount(got); n != 4 {
		t.Errorf("current document page count = %d, want 4 (blank inserted)", n)
	}
}

func TestExtractReturnsSubsetWithoutTouchingDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(threePagePDF(t))
	mw.WriteField("pages", "1,3")
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/extract", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("extract status = %d, want 200", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if n, _ := pdfops.PageCount(got); n != 2 {
		t.Errorf("extracted PDF page count = %d, want 2", n)
	}

	// Extract must NOT replace the open document: it operates on the posted bytes
	// and never assigns s.doc, so /api/pdf still returns the opened fixture (1 page)
	// rather than the 3-page payload or its 2-page subset.
	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	open, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	if n, _ := pdfops.PageCount(open); n != 1 {
		t.Errorf("open document page count = %d, want 1 (extract must not mutate it)", n)
	}
}

func TestCropUpdatesDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(threePagePDF(t)) // three 80×110pt pages
	mw.WriteField("op", "crop")
	mw.WriteField("rect", "[10,10,70,90]") // display-space points → a 60×80 window
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("crop status = %d, want 200", resp.StatusCode)
	}

	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	dims, err := api.PageDims(bytes.NewReader(got), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(dims) != 3 {
		t.Fatalf("page count = %d, want 3 (crop must not add or drop pages)", len(dims))
	}
	for i, d := range dims { // every page trimmed to the 60×80 crop window
		if math.Round(d.Width) != 60 || math.Round(d.Height) != 80 {
			t.Errorf("page %d is %.0f×%.0f pt, want 60×80 (crop window)", i+1, d.Width, d.Height)
		}
	}
}

func TestPageReorderUpdatesDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path) // ensures a current document exists

	// Three pages of distinct widths so the new order is observable, not just the
	// page count (identical pages couldn't prove a reorder happened).
	doc, err := pdfops.ImagesToPDF([]pdfops.RasterPage{
		{Image: pageImage(t, 160, 220), W: 80, H: 110},
		{Image: pageImage(t, 200, 240), W: 100, H: 120},
		{Image: pageImage(t, 120, 180), W: 60, H: 90},
	})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(doc)
	mw.WriteField("op", "reorder")
	mw.WriteField("pages", "3,1,2")
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page reorder status = %d, want 200", resp.StatusCode)
	}

	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	dims, err := api.PageDims(bytes.NewReader(got), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	// Original widths 80,100,60 in order 3,1,2 -> 60,80,100.
	want := []float64{60, 80, 100}
	if len(dims) != len(want) {
		t.Fatalf("page count = %d, want %d", len(dims), len(want))
	}
	for i, w := range want {
		if math.Round(dims[i].Width) != w {
			t.Errorf("page %d width = %.1f pt, want %.0f (reorder 3,1,2 not applied)", i+1, dims[i].Width, w)
		}
	}
}
