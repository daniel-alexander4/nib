package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
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
	dims, err := api.PageDims(context.Background(), bytes.NewReader(got), model.NewDefaultConfiguration())
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

// TestCombineTakesImagesThroughTheOpenersDoor — /pending 909. A picture posted to Combine becomes
// the page the opener would make of it (pixels ÷ 96 dpi × 72, `asOpenableDocument`), in the posted
// order beside a PDF; a file the opener refuses is still refused, and an office file is sent to the
// route that converts it. The widths are the assertion: they differ per input, so a build that
// dropped the images, reordered them or sized them another way cannot pass.
func TestCombineTakesImagesThroughTheOpenersDoor(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	doc, err := pdfops.ImagesToPDF([]pdfops.RasterPage{{Image: pageImage(t, 160, 220), W: 80, H: 110}})
	if err != nil {
		t.Fatal(err)
	}
	post := func(files ...[2]any) (*http.Response, string) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for _, f := range files {
			w, _ := mw.CreateFormFile("file", f[0].(string))
			w.Write(f[1].([]byte))
		}
		mw.Close()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/combine", mw.FormDataContentType(), &buf)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(body)
	}

	resp, body := post([2]any{"a.png", testPNG(t, 192, 96)}, [2]any{"b.pdf", doc}, [2]any{"c.png", testPNG(t, 96, 96)})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("combine status = %d (%s), want 200", resp.StatusCode, body)
	}
	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	dims, err := api.PageDims(context.Background(), bytes.NewReader(got), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{144, 80, 72}
	if len(dims) != len(want) {
		t.Fatalf("combined page count = %d, want %d", len(dims), len(want))
	}
	for i, wv := range want {
		if math.Round(dims[i].Width) != wv {
			t.Errorf("page %d width = %.1f, want %.0f", i+1, dims[i].Width, wv)
		}
	}

	if resp, _ := post([2]any{"a.png", testPNG(t, 8, 8)}, [2]any{"notes.txt", []byte("not a document")}); resp.StatusCode == http.StatusOK {
		t.Error("a text file was combined; the opener refuses it and Combine must too")
	}
	resp, body = post([2]any{"a.png", testPNG(t, 8, 8)}, [2]any{"report.docx", []byte("PK\x03\x04")})
	var refusal struct{ Error string }
	json.Unmarshal([]byte(body), &refusal)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(refusal.Error, "report.docx") || !strings.Contains(refusal.Error, "Open & convert to PDF") {
		t.Errorf("an office file = %d %q, want 400 naming it and the route that converts it", resp.StatusCode, refusal.Error)
	}
}
