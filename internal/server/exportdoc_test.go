package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"nib/internal/docmodel"
	"nib/internal/gate"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"nib/internal/docwrite/wtest"
	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// exportDocument posts to the route as the window does, pinned to the document id.
func exportDocument(t *testing.T, base string, c *http.Client, csrf, id, query string) (int, []byte, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, base+"/api/export/document?"+query, nil)
	req.Header.Set("X-Nib-Doc", id)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, resp.Header
}

func exportFixture(t *testing.T, name string) []byte {
	t.Helper()
	for _, f := range testpdf.ExportSet() {
		if f.Name == name {
			return f.PDF
		}
	}
	t.Fatalf("no export fixture %q", name)
	return nil
}

func TestTheExportRouteWritesTheThreeFormatsAndReportsWhatItLeftOut(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dr := uploadPDF(t, ts.URL, c, csrf, "doc.pdf", exportFixture(t, "covered"))

	for format, want := range map[string]string{"md": "visible heading", "html": "<title>Quarterly</title>", "txt": "visible heading"} {
		code, body, h := exportDocument(t, ts.URL, c, csrf, dr.ID, "format="+format+"&title=Quarterly&redactionsPending=2")
		if code != 200 || !strings.Contains(string(body), want) {
			t.Fatalf("%s: %d, want 200 holding %q:\n%s", format, code, want, body)
		}
		if strings.Contains(string(body), "secret under the box") {
			t.Errorf("%s carried text beneath an opaque box (D30):\n%s", format, body)
		}
		var f ExportFacts
		if err := json.Unmarshal([]byte(h.Get("X-Nib-Export")), &f); err != nil {
			t.Fatalf("%s: X-Nib-Export %q: %v", format, h.Get("X-Nib-Export"), err)
		}
		if f.Source != "proposal" || f.CoveredText == 0 || f.UnappliedRedactions != 2 || f.Signed {
			t.Errorf("%s: facts = %+v, want proposal source, covered text counted, 2 unapplied redactions, unsigned", format, f)
		}
	}
}

func TestTheExportRouteRefusesByCause(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dr := uploadPDF(t, ts.URL, c, csrf, "doc.pdf", exportFixture(t, "covered"))
	if code, _, _ := exportDocument(t, ts.URL, c, csrf, dr.ID, "format=rtf"); code != 400 {
		t.Errorf("an unknown format = %d, want 400", code)
	}
	if code, _, _ := exportDocument(t, ts.URL, c, csrf, "e1-9999", "format=md"); code != 409 {
		t.Errorf("a document the server does not hold = %d, want 409 (ADR-004)", code)
	}
	// A page of nothing: no text and no picture is a 422 naming "empty", never an empty file.
	blank := uploadPDF(t, ts.URL, c, csrf, "blank.pdf", mustBlankPDF(t))
	code, body, _ := exportDocument(t, ts.URL, c, csrf, blank.ID, "format=md")
	var e map[string]string
	_ = json.Unmarshal(body, &e)
	if code != 422 || e["cause"] != "empty" {
		t.Errorf("a blank document = %d %s, want 422 cause empty", code, body)
	}
}

func TestTheExportRouteTitleIsTheNameGivenAndNothingElse(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dr := uploadPDF(t, ts.URL, c, csrf, "SOURCE-MARKER.pdf", exportFixture(t, "covered"))
	_, body, _ := exportDocument(t, ts.URL, c, csrf, dr.ID, "format=html&title="+url.QueryEscape("a\x00b <i>x</i>\n"))
	if !strings.Contains(string(body), "<title>ab &lt;i&gt;x&lt;/i&gt;</title>") {
		t.Errorf("title not cleaned and escaped:\n%s", body)
	}
	_, body, _ = exportDocument(t, ts.URL, c, csrf, dr.ID, "format=html")
	if !strings.Contains(string(body), "<title>Document</title>") || strings.Contains(string(body), "SOURCE-MARKER") {
		t.Errorf("no title given must read the literal Document and never the source name:\n%s", body)
	}
}

// TestExportFactsAreIntegersAndEnumsOnly holds W3 over the struct: a header value is a number, a bool, an enum word or
// a page list — never a string a document could choose.
func TestExportFactsAreIntegersAndEnumsOnly(t *testing.T) {
	typ := reflect.TypeOf(ExportFacts{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		switch f.Type.Kind() {
		case reflect.Int, reflect.Bool:
		case reflect.String:
			if f.Name != "Source" {
				t.Errorf("ExportFacts.%s is a free string: only Source is an enum", f.Name)
			}
		case reflect.Slice:
			if f.Type.Elem().Kind() != reflect.Int && f.Type.Elem() != reflect.TypeOf(exportPage{}) {
				t.Errorf("ExportFacts.%s is a list of %s", f.Name, f.Type.Elem())
			}
		default:
			t.Errorf("ExportFacts.%s has kind %s", f.Name, f.Type.Kind())
		}
	}
	if p := reflect.TypeOf(exportPage{}); p.Field(1).Type.Kind() != reflect.String {
		t.Fatal("exportPage.Cause moved; this test reads it")
	}
}

func mustBlankPDF(t *testing.T) []byte {
	t.Helper()
	b, err := testpdf.Text("")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTheExportRouteWritesWordAndOpenDocumentPackages(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dr := uploadPDF(t, ts.URL, c, csrf, "doc.pdf", exportFixture(t, "covered"))
	for format, want := range map[string]struct{ mime, member string }{
		"docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "word/document.xml"},
		"odt":  {"application/vnd.oasis.opendocument.text", "content.xml"},
		"epub": {"application/epub+zip", "OEBPS/content.xhtml"},
	} {
		code, body, h := exportDocument(t, ts.URL, c, csrf, dr.ID, "format="+format+"&title=Quarterly")
		if code != 200 || h.Get("Content-Type") != want.mime {
			t.Fatalf("%s: %d %q", format, code, h.Get("Content-Type"))
		}
		pkg := wtest.Open(t, body)
		pkg.WellFormed(t)
		xmlText := string(pkg.Files[want.member])
		if !strings.Contains(xmlText, "visible heading") || strings.Contains(xmlText, "secret under the box") {
			t.Errorf("%s: the covered text rule (D30) does not hold in the package:\n%s", format, xmlText)
		}
		if h.Get("X-Nib-Export") == "" {
			t.Errorf("%s: no X-Nib-Export", format)
		}
	}
}

func TestTheExportFactsCarryTheTablesWrittenAsText(t *testing.T) {
	if f := factsOf(docmodel.Report{TablesAsText: 2}, false, 0); f.TablesAsText != 2 {
		t.Errorf("TablesAsText = %d, want 2", f.TablesAsText)
	}
}

func TestTheExportFactsCarryTheTextDirectionControlsRemoved(t *testing.T) {
	if f := factsOf(docmodel.Report{BidiControls: 3}, false, 0); f.BidiControls != 3 {
		t.Errorf("BidiControls = %d, want 3", f.BidiControls)
	}
}

func TestTheExportFactsCarryTheArtifactTextLeftOut(t *testing.T) {
	if f := factsOf(docmodel.Report{ArtifactText: 4}, false, 0); f.ArtifactText != 4 {
		t.Errorf("ArtifactText = %d, want 4", f.ArtifactText)
	}
}

func exportSVG(t *testing.T, base string, c *http.Client, csrf, id, query string) (int, []byte, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, base+"/api/export/svg?"+query, nil)
	req.Header.Set("X-Nib-Doc", id)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, resp.Header
}

// The SVG route (ADR-205): one page drawn by pdftocairo, refusals by cause, pinned to the document (ADR-004).
func TestTheSVGRouteDrawsOnePageAndRefusesByCause(t *testing.T) {
	if !pdfops.PdftocairoAvailable() {
		t.Skip("pdftocairo (poppler) not installed")
	}
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dr := uploadPDF(t, ts.URL, c, csrf, "doc.pdf", exportFixture(t, "covered"))
	code, body, h := exportSVG(t, ts.URL, c, csrf, dr.ID, "page=1")
	if code != 200 || h.Get("Content-Type") != "image/svg+xml" || !strings.Contains(string(body[:200]), "<svg") {
		t.Fatalf("page 1 = %d %q %.120s", code, h.Get("Content-Type"), body)
	}
	if code, _, _ := exportSVG(t, ts.URL, c, csrf, dr.ID, "page=0"); code != 400 {
		t.Errorf("page 0 = %d, want 400", code)
	}
	if code, _, _ := exportSVG(t, ts.URL, c, csrf, dr.ID, "page=x"); code != 400 {
		t.Errorf("page x = %d, want 400", code)
	}
	code, body, _ = exportSVG(t, ts.URL, c, csrf, dr.ID, "page=99")
	var e map[string]string
	_ = json.Unmarshal(body, &e)
	if code != 422 || e["cause"] != "page" {
		t.Errorf("page 99 = %d %s, want 422 cause page", code, body)
	}
	if code, _, _ := exportSVG(t, ts.URL, c, csrf, "e1-9999", "page=1"); code != 409 {
		t.Errorf("a document the server does not hold = %d, want 409 (ADR-004)", code)
	}
	// No session: the route is behind requireSession like the rest.
	resp, err := http.Post(ts.URL+"/api/export/svg?page=1", "", nil)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Error("answered without a session")
		}
	}
}

// The PowerPoint route (ADR-206): positioned, from the page reader, and the visibility rule still holds (D30).
func TestTheExportRouteWritesPowerPointAndHoldsTheCoveredTextRule(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dr := uploadPDF(t, ts.URL, c, csrf, "doc.pdf", exportFixture(t, "covered"))
	code, body, h := exportDocument(t, ts.URL, c, csrf, dr.ID, "format=pptx&title=Quarterly&redactionsPending=1")
	if code != 200 || h.Get("Content-Type") != "application/vnd.openxmlformats-officedocument.presentationml.presentation" {
		t.Fatalf("pptx: %d %q", code, h.Get("Content-Type"))
	}
	pkg := wtest.Open(t, body)
	pkg.WellFormed(t)
	slide := string(pkg.Files["ppt/slides/slide1.xml"])
	if !strings.Contains(slide, "visible heading") || strings.Contains(slide, "secret under the box") {
		t.Errorf("the covered text rule does not hold on the slide:\n%s", slide)
	}
	var f ExportFacts
	if err := json.Unmarshal([]byte(h.Get("X-Nib-Export")), &f); err != nil {
		t.Fatal(err)
	}
	if f.Source != "proposal" || f.CoveredText == 0 || f.UnappliedRedactions != 1 {
		t.Errorf("facts = %+v", f)
	}
}

func TestThePowerPointRouteRefusesADocumentWithNothingToPlace(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dr := uploadPDF(t, ts.URL, c, csrf, "doc.pdf", mustBlankPDF(t))
	code, body, _ := exportDocument(t, ts.URL, c, csrf, dr.ID, "format=pptx")
	if code != 422 || !strings.Contains(string(body), `"empty"`) {
		t.Errorf("blank page = %d %s, want 422 empty", code, body)
	}
	if code, _, _ := exportDocument(t, ts.URL, c, csrf, dr.ID, "format=ppt"); code != 400 {
		t.Errorf("unknown format = %d", code)
	}
}

// ── /pending 948 (b): the SVG route's 503 and its moved-revision 409 ──

func svgRouteDoc(t *testing.T) (*httptest.Server, *Server, *http.Client, string, string) {
	t.Helper()
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dr := uploadPDF(t, ts.URL, c, csrf, "doc.pdf", exportFixture(t, "covered"))
	return ts, srv, c, csrf, dr.ID
}

// A full queue at the converters' gate is a 503 `busy`, nothing started: the slot is held by the test, so the route's own
// request finds none and no queue to wait in. The control is the same request with the slot free.
func TestTheSVGRouteAnswersBusyWhenTheGateIsFull(t *testing.T) {
	if !pdfops.PdftocairoAvailable() {
		t.Skip("pdftocairo (poppler) not installed")
	}
	ts, _, c, csrf, id := svgRouteDoc(t)
	g := gate.New(1, 0)
	useGate(t, g)
	release, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	code, body, _ := exportSVG(t, ts.URL, c, csrf, id, "page=1")
	var e map[string]string
	_ = json.Unmarshal(body, &e)
	if code != 503 || e["cause"] != "busy" {
		t.Errorf("a full gate = %d %s, want 503 cause busy", code, body)
	}
	release()
	if code, _, _ := exportSVG(t, ts.URL, c, csrf, id, "page=1"); code != 200 {
		t.Errorf("control: the same request with the slot free = %d, want 200", code)
	}
}

// A document that moved while its page was being drawn is a 409 and the drawing is not sent. The request waits at the gate
// (the test holds the one slot), the document's count moves, the slot is released, and the draw finishes against a
// document that is no longer the one the request named.
func TestTheSVGRouteRefusesADocumentThatMovedDuringTheDraw(t *testing.T) {
	if !pdfops.PdftocairoAvailable() {
		t.Skip("pdftocairo (poppler) not installed")
	}
	ts, srv, c, csrf, id := svgRouteDoc(t)
	g := gate.New(1, 4)
	useGate(t, g)
	release, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	type answer struct {
		code int
		body []byte
	}
	done := make(chan answer, 1)
	go func() {
		code, body, _ := exportSVG(t, ts.URL, c, csrf, id, "page=1")
		done <- answer{code, body}
	}()
	// The request has read the document's revision before it reached the gate; wait until it is queued there.
	deadline := time.Now().Add(10 * time.Second)
	for g.Queued() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the request never reached the gate")
		}
		time.Sleep(5 * time.Millisecond)
	}
	srv.mu.Lock()
	for _, d := range srv.docs {
		d.rev++
	}
	srv.mu.Unlock()
	release()
	select {
	case a := <-done:
		if a.code != 409 || bytes.Contains(a.body, []byte("<svg")) {
			t.Errorf("moved during the draw = %d %.100s, want 409 and no drawing", a.code, a.body)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("no answer")
	}
}

// With no pdftocairo on PATH the route is a 503 `missing-tool` that names the tool. The tool is looked up once per
// process, so this runs in a child that starts with an empty PATH: `go test` in the parent has usually found it already.
func TestTheSVGRouteNamesAMissingTool(t *testing.T) {
	if os.Getenv("NIB_SVG_NOTOOL_CHILD") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTheSVGRouteNamesAMissingTool$", "-test.count=1")
		cmd.Env = append(os.Environ(), "NIB_SVG_NOTOOL_CHILD=1", "PATH="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("the child (empty PATH) failed: %v\n%s", err, out)
		}
		return
	}
	ts, _, c, csrf, id := svgRouteDoc(t)
	code, body, _ := exportSVG(t, ts.URL, c, csrf, id, "page=1")
	var e map[string]string
	_ = json.Unmarshal(body, &e)
	if code != 503 || e["cause"] != "missing-tool" || !strings.Contains(e["error"], "pdftocairo") {
		t.Errorf("no tool = %d %s, want 503 cause missing-tool naming pdftocairo", code, body)
	}
}
