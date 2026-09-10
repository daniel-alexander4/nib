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
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	// Keep-box as page fractions [fx, fy, fw, fh], top-left origin: on an 80×110pt
	// page this is a 60×80pt window (fw=60/80, fh=80/110).
	mw.WriteField("rect", "[0.125,0.18181818181818182,0.75,0.7272727272727273]")
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

func TestDuplicateUpdatesDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	// Distinct widths so the duplicated page is identifiable by dimension.
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
	mw.WriteField("op", "duplicate")
	mw.WriteField("page", "2") // duplicate the 100-wide page
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate status = %d, want 200", resp.StatusCode)
	}

	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	dims, err := api.PageDims(bytes.NewReader(got), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{80, 100, 100, 60} // page 2 (100) now appears twice, in place
	if len(dims) != len(want) {
		t.Fatalf("page count = %d, want %d", len(dims), len(want))
	}
	for i, w := range want {
		if math.Round(dims[i].Width) != w {
			t.Errorf("page %d width = %.1f, want %.0f (duplicate page 2 not applied)", i+1, dims[i].Width, w)
		}
	}
}

func TestInsertPDFUpdatesDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	doc, err := pdfops.ImagesToPDF([]pdfops.RasterPage{
		{Image: pageImage(t, 160, 220), W: 80, H: 110},
		{Image: pageImage(t, 200, 240), W: 100, H: 120},
		{Image: pageImage(t, 120, 180), W: 60, H: 90},
	})
	if err != nil {
		t.Fatal(err)
	}
	insert, err := pdfops.ImagesToPDF([]pdfops.RasterPage{
		{Image: pageImage(t, 80, 120), W: 40, H: 60},
	})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(doc)
	af, _ := mw.CreateFormFile("append", "insert.pdf") // secondary PDF reuses the append field
	af.Write(insert)
	mw.WriteField("op", "insertpdf")
	mw.WriteField("page", "2") // insert BEFORE page 2
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("insertpdf status = %d, want 200", resp.StatusCode)
	}

	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	dims, err := api.PageDims(bytes.NewReader(got), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{80, 40, 100, 60} // inserted 40-wide page lands before the 100-wide page 2
	if len(dims) != len(want) {
		t.Fatalf("page count = %d, want %d", len(dims), len(want))
	}
	for i, w := range want {
		if math.Round(dims[i].Width) != w {
			t.Errorf("page %d width = %.1f, want %.0f (insert before page 2 not applied)", i+1, dims[i].Width, w)
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

// /pending 448 — every field the pagenum case READS must be one `pageNumGo` SENDS.
//
// The defect this pins: `handlePages` has read `size` and `color` since it shipped,
// `pageNumGo` appended neither, and no control offered them — so every page-number stamp
// the product produced was 11pt black, silently, because StampPageNumbers clamps a size
// below 6 to 11 and a non-hex colour to #000000. Nothing was wrong with the output, which
// is why it survived: it is a shipped knob with no handle, and only a comparison of the
// two sides can see it.
//
// **This is the narrow instance of `/pending 447`'s general scan** — "nothing compares what
// a handler READS to what the client SENDS". That one is still open and would subsume this;
// until it exists, the route that actually lost two knobs gets its own.
//
// Scoped to the pagenum case deliberately: the other cases in this switch are reached by
// callers this test does not read, and a whole-file scan would report them as unsent.
func TestEveryPageNumberFieldTheServerReadsIsOneTheClientSends(t *testing.T) {
	srv, err := os.ReadFile("pages.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(srv)
	from := strings.Index(s, `case "pagenum":`)
	if from < 0 {
		t.Fatal(`the "pagenum" case is gone from pages.go — this scan would pass over nothing`)
	}
	to := strings.Index(s[from:], `case "pagelabels":`)
	if to < 0 {
		t.Fatal("the case after pagenum is gone — the scan has no end and would read the whole switch")
	}
	reads := map[string]bool{}
	for _, m := range regexp.MustCompile(`FormValue\("([a-zA-Z]+)"\)`).FindAllStringSubmatch(s[from:from+to], -1) {
		reads[m[1]] = true
	}
	if len(reads) < 4 {
		t.Fatalf("found %d FormValue reads in the pagenum case; it takes at least four. The scan "+
			"is broken, so a clean result means nothing.", len(reads))
	}

	cli, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(cli)
	// **`pageOp`, and NOT `pageNumGo` — this test was satisfied one function short of the wire.**
	//
	// The first cut read `pageNumGo`'s options object, and that object is not the request: it is
	// handed to `pageOp`, which builds the FormData by appending a NAMED LIST of keys and silently
	// drops anything not on it. So when /pending 448 added `size` and `color` to the object and to
	// the dialog, this test went green and **the two knobs still did nothing** — the request
	// carried neither, the server applied its defaults, and every stamp stayed 11pt black exactly
	// as before the fix. The mutation recorded with it ("client stops sending both knobs") removed
	// them from the object, which this test could see, and never touched the form.
	//
	// Reading the appends is what makes the assertion about the REQUEST. `pageNumGo` is still
	// checked, below, because a key that never reaches `pageOp` cannot be appended either — but it
	// is the weaker half and it is no longer the only one.
	oi := strings.Index(js, "async function pageOp(")
	if oi < 0 {
		t.Fatal("pageOp is gone or renamed — the function that actually builds the /api/pages " +
			"request has no source, so nothing below is about what the server receives")
	}
	opBody := js[oi : oi+strings.Index(js[oi:], "\n}")]
	sends := map[string]bool{}
	for _, m := range regexp.MustCompile(`form\.append\('([a-zA-Z]+)'`).FindAllStringSubmatch(opBody, -1) {
		sends[m[1]] = true
	}
	if len(sends) < 10 {
		t.Fatalf("parsed %d form.append names out of pageOp; it appends well over ten. The client "+
			"half of this comparison is broken, so a clean result means nothing.", len(sends))
	}

	gi := strings.Index(js, "async function pageNumGo()")
	if gi < 0 {
		t.Fatal("pageNumGo is gone or renamed — the client half of this comparison has no source")
	}
	body := js[gi : gi+strings.Index(js[gi:], "\n}")]
	passes := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*([a-zA-Z]+):`).FindAllStringSubmatch(body, -1) {
		passes[m[1]] = true
	}
	if len(passes) < 4 {
		t.Fatalf("parsed %d keys out of pageNumGo; it passes at least four", len(passes))
	}

	for name := range reads {
		if !sends[name] {
			t.Errorf("handlePages reads %q from the pagenum request and pageOp never appends it, "+
				"so the request does not carry it. The server then applies its default and the "+
				"user gets a value no control offered — which is how every page-number stamp came "+
				"out 11pt black, both before /pending 448 and after it.", name)
		}
		if !passes[name] {
			t.Errorf("pageNumGo never passes %q to pageOp, so however pageOp is written the value "+
				"cannot reach the request", name)
		}
	}
}
