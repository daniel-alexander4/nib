package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestBakeNotes posts a comment note and confirms the bake wires it through to
// pdfops.AddNotes (the /Text annotation content itself is covered by the pdfops
// test). The result must be a valid PDF.
func TestBakeNotes(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	pdf, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	notes, _ := json.Marshal([]pdfops.Note{{Page: 1, X: 72, Y: 700, Text: "Please review"}})
	mw.WriteField("notes", string(notes))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/bake", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("bake notes status = %d: %s", resp.StatusCode, body)
	}
	out, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Error("bake notes did not return a PDF")
	}
}

func TestBakeStampsFields(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	pdf, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	fields, _ := json.Marshal([]pdfops.Field{{Page: 1, Rect: [4]float64{72, 700, 300, 716}, Text: "Hello"}})
	mw.WriteField("fields", string(fields))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/bake", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("bake status = %d: %s", resp.StatusCode, body)
	}
	out, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(out, []byte("%PDF")) || len(out) <= len(pdf) {
		t.Error("bake did not return a stamped PDF larger than the input")
	}
}

// TestBakeCoverAndReplace posts a cover-and-replace edit: an opaque cover PNG
// plus a styled replacement field. Both must bake into a valid PDF, exercising
// the covers-before-text pre-pass.
func TestBakeCoverAndReplace(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// A tiny opaque white PNG used as the cover fill.
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))
	for x := 0; x < 16; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	var ib bytes.Buffer
	_ = png.Encode(&ib, img)
	cover := base64.StdEncoding.EncodeToString(ib.Bytes())

	pdf, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	covers, _ := json.Marshal([]stampReq{{Page: 1, Rect: [4]float64{72, 700, 200, 714}, PNG: cover}})
	fields, _ := json.Marshal([]pdfops.Field{{Page: 1, Rect: [4]float64{72, 700, 200, 714}, Text: "New text", Font: "Times-Roman", Size: 12, Color: "#003366"}})
	mw.WriteField("covers", string(covers))
	mw.WriteField("fields", string(fields))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/bake", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("bake status = %d: %s", resp.StatusCode, body)
	}
	out, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(out, []byte("%PDF")) || len(out) <= len(pdf) {
		t.Error("cover-and-replace bake did not return a larger PDF")
	}
	if n, _ := pdfops.PageCount(out); n != 1 {
		t.Errorf("page count = %d, want 1 (edit must stay vector, not rasterize)", n)
	}
}

// TestBakeImageStamp posts an inline base64 PNG stamp (the Y/N-circle path) and
// confirms the bake returns a valid, single-page PDF.
func TestBakeImageStamp(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// A tiny opaque red PNG.
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for x := 0; x < 24; x++ {
		for y := 0; y < 24; y++ {
			img.Set(x, y, color.RGBA{200, 0, 0, 255})
		}
	}
	var ib bytes.Buffer
	_ = png.Encode(&ib, img)
	b64 := base64.StdEncoding.EncodeToString(ib.Bytes())

	pdf, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	stamps, _ := json.Marshal([]stampReq{{Page: 1, Rect: [4]float64{72, 100, 120, 148}, PNG: b64}})
	mw.WriteField("stamps", string(stamps))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/bake", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("bake status = %d: %s", resp.StatusCode, body)
	}
	out, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Error("bake did not return a PDF")
	}
	if n, _ := pdfops.PageCount(out); n != 1 {
		t.Errorf("page count = %d, want 1", n)
	}
}
