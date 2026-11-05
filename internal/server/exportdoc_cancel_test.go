package server

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"nib/internal/docmodel/read"
	"nib/internal/docwrite/wtest"
	"nib/internal/pptxexport"
	"nib/internal/testpdf"
)

// tripCtx is a context whose Err() says Canceled from its nth call on, and counts the calls: it finds every place the
// writers observe the context and proves each one is honoured, which a context cancelled before the call cannot.
type tripCtx struct {
	context.Context
	n     int64 // trip on this call; 0 never
	calls atomic.Int64
}

func (c *tripCtx) Err() error {
	k := c.calls.Add(1)
	if c.n > 0 && k >= c.n {
		return context.Canceled
	}
	return nil
}

// TestEveryWriterObservesTheContextAndHonoursIt — /pending 941 #1. A cancel landing at ANY of a writer's observation
// points ends it with the context's error and no bytes, and it observes at least once a block.
func TestEveryWriterObservesTheContextAndHonoursIt(t *testing.T) {
	d := wtest.Hostile()
	for name, f := range exportFormats {
		if f.slides {
			continue // no flowing writer: TestASlidesExportStopsOnACancel holds the PowerPoint path
		}
		t.Run(name, func(t *testing.T) {
			probe := &tripCtx{Context: context.Background()}
			out, err := f.write(probe, d)
			if err != nil || len(out) == 0 {
				t.Fatalf("a writer given a live context: %d bytes, %v", len(out), err)
			}
			total := probe.calls.Load()
			if total < int64(len(d.Blocks)) {
				t.Fatalf("the writer looked at the context %d times for %d blocks — a long document is not interruptible", total, len(d.Blocks))
			}
			for n := int64(1); n <= total; n++ {
				out, err := f.write(&tripCtx{Context: context.Background(), n: n}, d)
				if err != context.Canceled || out != nil {
					t.Fatalf("a cancel at observation %d of %d: %d bytes, err %v — want context.Canceled and no bytes", n, total, len(out), err)
				}
			}
		})
	}
}

// exportRequest is the route called directly with ctx as the request's context.
func exportRequest(s *Server, ctx context.Context, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/export/document?"+query, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleExportDocument(rec, req)
	return rec
}

// TestACancelledExportAnswersNothing — /pending 941 #1, at the route: the caller went away, so no 422 "unreadable" or
// 500 is written for it, whichever step the cancel reached.
func TestACancelledExportAnswersNothing(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportSet() {
		if f.Name == "covered" {
			pdf = f.PDF
		}
	}
	s := openTestServer(t, pdf)
	live := exportRequest(s, context.Background(), "format=md")
	if live.Code != 200 {
		t.Fatalf("the control export: %d %s", live.Code, live.Body)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for format := range exportFormats {
		rec := exportRequest(s, cancelled, "format="+format)
		if rec.Body.Len() != 0 || rec.Header().Get("X-Nib-Export") != "" {
			t.Errorf("%s: a cancelled export wrote %d bytes (%q) and header %q", format, rec.Body.Len(), rec.Body.String(), rec.Header().Get("X-Nib-Export"))
		}
	}
	// A cancel part-way: every observation point of the whole route, reader and writer, is honoured.
	probe := &tripCtx{Context: context.Background()}
	if rec := exportRequest(s, probe, "format=docx"); rec.Code != 200 {
		t.Fatalf("a counted export: %d", rec.Code)
	}
	// The route hands the request's context to the reader AND the writer: its observations are the two separately
	// counted, so a stage given another context is a count that comes up short.
	rd := &tripCtx{Context: context.Background()}
	model, err := read.Read(rd, pdf, read.Options{Title: "Document"})
	if err != nil {
		t.Fatal(err)
	}
	wr := &tripCtx{Context: context.Background()}
	if _, err := exportFormats["docx"].write(wr, model); err != nil {
		t.Fatal(err)
	}
	if want := rd.calls.Load() + wr.calls.Load(); probe.calls.Load() != want {
		t.Fatalf("the route observed the request's context %d times, the reader (%d) and the writer (%d) alone %d", probe.calls.Load(), rd.calls.Load(), wr.calls.Load(), want)
	}
	for n := int64(1); n <= probe.calls.Load(); n++ {
		rec := exportRequest(s, &tripCtx{Context: context.Background(), n: n}, "format=docx")
		if rec.Body.Len() != 0 {
			t.Fatalf("a cancel at observation %d of %d still wrote %d bytes: %s", n, probe.calls.Load(), rec.Body.Len(), rec.Body.String())
		}
	}
}

// TestTheSecondExportRequestNamesTheRevisionTheFirstWasReadAt — /pending 941 #5, ADR-146. The Save dialog asks again for
// the format the user picked; if the document moved since the press, the facts it showed are not that document's.
func TestTheSecondExportRequestNamesTheRevisionTheFirstWasReadAt(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportSet() {
		if f.Name == "covered" {
			pdf = f.PDF
		}
	}
	s := openTestServer(t, pdf)
	doc := s.activeDoc()
	first := exportRequest(s, context.Background(), "format=md")
	rev := first.Header().Get("X-Nib-Doc-Rev")
	if first.Code != 200 || rev != "0" {
		t.Fatalf("the first read: %d, X-Nib-Doc-Rev %q — it must say the count it was read at", first.Code, rev)
	}
	if rec := exportRequest(s, context.Background(), "format=html&baseRev="+rev); rec.Code != 200 {
		t.Fatalf("the second request at the same revision: %d %s", rec.Code, rec.Body)
	}
	s.mu.Lock()
	doc.install(append([]byte(nil), doc.data...), doc.sig)
	s.mu.Unlock()
	if rec := exportRequest(s, context.Background(), "format=html&baseRev="+rev); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), errDocChanged.Error()) {
		t.Errorf("the second request after the document moved: %d %s — want 409 and errDocChanged", rec.Code, rec.Body)
	}
	if rec := exportRequest(s, context.Background(), "format=html"); rec.Code != 200 {
		t.Errorf("a request naming no revision is last-writer-wins: %d", rec.Code)
	}
	if rec := exportRequest(s, context.Background(), "format=html&baseRev=abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("a revision that is not a number: %d, want 400 (a client that tried to say and failed is not one that said nothing)", rec.Code)
	}
	now := exportRequest(s, context.Background(), "format=html")
	if got := now.Header().Get("X-Nib-Doc-Rev"); got != strconv.FormatUint(doc.rev, 10) || got == rev {
		t.Errorf("a read after the install says revision %q, the document is at %d", got, doc.rev)
	}
}

// TestACancelledPageImageExportAnswersNothing — /pending 941 #6 at the route: the GIF, BMP and TIFF writers are given the
// request's context, so a caller who went away is not answered with a picture or a 500.
func TestACancelledPageImageExportAnswersNothing(t *testing.T) {
	s := openTestServer(t, threePagePDF(t))
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	post := func(ctx context.Context, format string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("image", "p.png")
		fw.Write(pngBytes.Bytes())
		mw.WriteField("format", format)
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/assemble", &body).WithContext(ctx)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		s.handleAssemble(rec, req)
		return rec
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, f := range []string{"gif", "bmp", "tiff"} {
		if rec := post(context.Background(), f); rec.Code != 200 || rec.Body.Len() == 0 {
			t.Fatalf("%s control: %d, %d bytes", f, rec.Code, rec.Body.Len())
		}
		if rec := post(cancelled, f); rec.Body.Len() != 0 {
			t.Errorf("%s of a cancelled request answered %d: %s", f, rec.Code, rec.Body.String())
		}
	}
}

// The slides path reads, builds and zips under the request's context: a cancel before the call, and one landing at
// the writer's own observation points, ends it with the context's error and no bytes.
func TestASlidesExportStopsOnACancel(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportSet() {
		if f.Name == "covered" {
			pdf = f.PDF
		}
	}
	if body, _, _, err := slidesOf(context.Background(), pdf, "t"); err != nil || len(body) == 0 {
		t.Fatalf("a live context: %d bytes, %v", len(body), err)
	}
	probe := &tripCtx{Context: context.Background()}
	if _, _, _, err := slidesOf(probe, pdf, "t"); err != nil {
		t.Fatal(err)
	}
	total := probe.calls.Load()
	for n := int64(1); n <= total; n++ {
		body, _, _, err := slidesOf(&tripCtx{Context: context.Background(), n: n}, pdf, "t")
		if !errors.Is(err, context.Canceled) || body != nil {
			t.Fatalf("a cancel at observation %d of %d: %d bytes, err %v", n, total, len(body), err)
		}
	}
}

// The page count is asked before the read: a document past the slide cap is refused without a page being laid out
// (the read observes the context first, so an unobserved context proves it never started).
func TestASlidesExportPastTheCapIsRefusedBeforeTheRead(t *testing.T) {
	pages := make([]string, pptxexport.MaxSlides+1)
	for i := range pages {
		pages[i] = "p"
	}
	pdf, err := testpdf.Text(pages...)
	if err != nil {
		t.Fatal(err)
	}
	probe := &tripCtx{Context: context.Background()}
	if _, _, _, err := slidesOf(probe, pdf, "t"); !errors.Is(err, pptxexport.ErrTooManySlides) {
		t.Fatalf("%v, want ErrTooManySlides", err)
	}
	if n := probe.calls.Load(); n != 0 {
		t.Errorf("the read began (%d context observations) before the page count was refused", n)
	}
}

// The slides path sums the reader's visibility counts itself (it does not call read.Read), so each count is carried there too:
// text the document marks as an artifact is named on a deck as on a document (/pending 949).
func TestASlidesExportNamesTheArtifactTextItLeftOut(t *testing.T) {
	var pdf []byte
	for _, f := range testpdf.ExportModelSet() {
		if f.Name == "vis-artifact" {
			pdf = f.PDF
		}
	}
	if _, rep, _, err := slidesOf(context.Background(), pdf, "t"); err != nil || rep.ArtifactText != 1 {
		t.Fatalf("slides: ArtifactText = %d, err %v, want 1", rep.ArtifactText, err)
	}
}
