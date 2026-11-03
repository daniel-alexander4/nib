package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"nib/internal/docmodel"
	"reflect"
	"strings"
	"testing"

	"nib/internal/docwrite/wtest"
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
