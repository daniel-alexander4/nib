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
	"net/http/httptest"
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

// bakeFields posts a field set and returns the response's fit report.
func bakeFields(t *testing.T, ts *httptest.Server, c *http.Client, csrf string, fields []pdfops.Field) (*http.Response, []pdfops.Fit) {
	t.Helper()
	pdf, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	b, _ := json.Marshal(fields)
	mw.WriteField("fields", string(b))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/bake", mw.FormDataContentType(), &buf)
	var fits []pdfops.Fit
	if h := resp.Header.Get(fitReportHeader); h != "" {
		if err := json.Unmarshal([]byte(h), &fits); err != nil {
			t.Fatalf("%s is not decodable JSON (%v): %s", fitReportHeader, err, h)
		}
	}
	return resp, fits
}

// A bake whose text cannot be made to fit still returns 200 and a whole PDF, and
// says on the response what happened and why.
//
// **The status assertion is the load-bearing one.** Every save, print, flatten,
// export and signature in the client runs through /api/bake, and the client's own
// rule is that a bake which is not OK aborts the whole operation — so refusing an
// over-long field here would make the document unsaveable rather than merely
// untidy. That is a worse failure than the one this slice exists to fix.
func TestBakeReportsAnOverrunAndStillReturnsTheDocument(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp, fits := bakeFields(t, ts, c, csrf, []pdfops.Field{
		{Page: 1, Rect: [4]float64{50, 400, 130, 420},
			Text: "alpha bravo charlie delta echo foxtrot", Font: "Helvetica", Size: 12},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("an over-long edit made the bake fail with %d: %s — every save runs through here",
			resp.StatusCode, body)
	}
	out, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("the bake did not return a PDF")
	}
	if len(fits) != 1 {
		t.Fatalf("got %d fit report(s), want 1: the overrun must be named on the response", len(fits))
	}
	if fits[0].Outcome != pdfops.FitOverran {
		t.Errorf("outcome %q, want %q", fits[0].Outcome, pdfops.FitOverran)
	}
	if fits[0].OverrunPt <= 0 {
		t.Errorf("outcome is %q but the reported overrun is %.2fpt", fits[0].Outcome, fits[0].OverrunPt)
	}
	if fits[0].Field != 0 {
		t.Errorf("the report names field %d, want 0 — a report that cannot be tied back to a "+
			"field cannot be shown against one", fits[0].Field)
	}
}

// A bake where everything fits carries NO header, and one where something was
// silently adjusted carries one. The pair is the point: a header that is always
// present is a header nobody reads.
func TestBakeOmitsTheFitReportWhenEverythingFits(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	roomy, fits := bakeFields(t, ts, c, csrf, []pdfops.Field{
		{Page: 1, Rect: [4]float64{50, 400, 400, 420}, Text: "Plain ASCII", Font: "Helvetica", Size: 12},
	})
	defer roomy.Body.Close()
	if roomy.StatusCode != http.StatusOK {
		t.Fatalf("status %d", roomy.StatusCode)
	}
	if h := roomy.Header.Get(fitReportHeader); h != "" {
		t.Errorf("a bake where every field fitted still sent %s: %s", fitReportHeader, h)
	}
	if len(fits) != 0 {
		t.Errorf("got %d fit report(s) for a document where nothing needed doing", len(fits))
	}

	// STIMULUS: the same route DOES send one when something happened, so the empty
	// result above is the rule working rather than the header being unwired.
	shrunk, sfits := bakeFields(t, ts, c, csrf, []pdfops.Field{
		{Page: 1, Rect: [4]float64{50, 400, 108, 420}, Text: "Plain ASCII", Font: "Helvetica", Size: 12},
	})
	defer shrunk.Body.Close()
	if len(sfits) != 1 || sfits[0].Outcome != pdfops.FitShrunk {
		t.Fatalf("the control did not shrink (got %v) — the assertion above proves nothing", sfits)
	}
	if sfits[0].StampedPt >= 12 {
		t.Errorf("reported StampedPt %d, want below the 12pt asked for", sfits[0].StampedPt)
	}
}
