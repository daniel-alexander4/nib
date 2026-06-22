package server

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// TestCombineCreatesDocument proves combine assembles several uploaded PDFs into
// one document — in the posted order — and makes it the open document even with
// nothing open beforehand (a fresh, path-less doc named combined.pdf).
func TestCombineCreatesDocument(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	// Deliberately do NOT open a document first — combine creates one.

	doc1, err := pdfops.ImagesToPDF([]pdfops.RasterPage{{Image: pageImage(t, 160, 220), W: 80, H: 110}})
	if err != nil {
		t.Fatal(err)
	}
	doc2, err := pdfops.ImagesToPDF([]pdfops.RasterPage{
		{Image: pageImage(t, 200, 240), W: 100, H: 120},
		{Image: pageImage(t, 120, 180), W: 60, H: 90},
	})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	f1, _ := mw.CreateFormFile("file", "a.pdf")
	f1.Write(doc1)
	f2, _ := mw.CreateFormFile("file", "b.pdf")
	f2.Write(doc2)
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/combine", mw.FormDataContentType(), &buf)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("combine status = %d, want 200", resp.StatusCode)
	}
	var meta map[string]any
	json.NewDecoder(resp.Body).Decode(&meta)
	resp.Body.Close()
	if meta["name"] != "combined.pdf" {
		t.Errorf("combined doc name = %v, want combined.pdf", meta["name"])
	}

	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	dims, err := api.PageDims(bytes.NewReader(got), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{80, 100, 60} // doc1's page, then doc2's two pages — in order
	if len(dims) != len(want) {
		t.Fatalf("combined page count = %d, want %d", len(dims), len(want))
	}
	for i, wv := range want {
		if math.Round(dims[i].Width) != wv {
			t.Errorf("page %d width = %.1f, want %.0f (combine order wrong)", i+1, dims[i].Width, wv)
		}
	}
}
