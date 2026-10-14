package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestAFormExportOfADocumentWithNoFormFieldsIsA422ThatSaysSo — `/pending 888`, ADR-072. Twenty-one of 193 real
// documents whose only field is a signature were answered 500 "could not export form" by a route that had
// nothing to export. It is a fact about the document: 422, cause `no-form-fields`, the sentence — in each of
// the three formats, and from the fill that reads the form first.
func TestAFormExportOfADocumentWithNoFormFieldsIsA422ThatSaysSo(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	sig := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /SigFlags 1 >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R] >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Sig /T (Signature1) /Rect [10 10 150 60] /P 3 0 R >>",
	})
	path := filepath.Join(t.TempDir(), "sig.pdf")
	if err := os.WriteFile(path, sig, 0o600); err != nil {
		t.Fatal(err)
	}
	openByPath(t, ts.URL, c, csrf, path)
	refused := func(what string, resp *http.Response) {
		t.Helper()
		defer resp.Body.Close()
		var got map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&got)
		if resp.StatusCode != http.StatusUnprocessableEntity || got["cause"] != "no-form-fields" || !strings.Contains(got["error"], pdfops.NoFormFieldsRefusal) {
			t.Errorf("%s: %d %v — want 422, cause no-form-fields, and the sentence", what, resp.StatusCode, got)
		}
	}
	for _, format := range []string{"", "csv", "xfdf"} {
		resp, err := c.Get(ts.URL + "/api/form-data?format=" + format + "&auth=" + csrf)
		if err != nil {
			t.Fatal(err)
		}
		refused("export "+format, resp)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "sig.pdf")
	fw.Write(sig)
	mw.WriteField("data", `<?xml version="1.0"?><xfdf xmlns="http://ns.adobe.com/xfdf/"><fields><field name="a"><value>b</value></field></fields></xfdf>`)
	mw.Close()
	refused("fill from XFDF", write(t, c, csrf, http.MethodPost, ts.URL+"/api/form/fill-xfdf", mw.FormDataContentType(), &buf))

	// Control: a form exports, 200, so the refusals above are about the document.
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	fpath := filepath.Join(t.TempDir(), "form.pdf")
	if err := os.WriteFile(fpath, form, 0o600); err != nil {
		t.Fatal(err)
	}
	openByPath(t, ts.URL, c, csrf, fpath)
	resp, err := c.Get(ts.URL + "/api/form-data?auth=" + csrf)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("control: a form's export answered %d", resp.StatusCode)
	}
}

// TestFormEntriesOnNoPageAreRemovedOnRequestAndNeverFromASignedDocument — `/pending 888`. The sanitize route's
// fourth method: the scan's finding is gone from the residual and from the open document; undo brings the
// document back; and a signed document is refused 422 and left byte for byte as it was.
func TestFormEntriesOnNoPageAreRemovedOnRequestAndNeverFromASignedDocument(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R 7 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R] >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (onpage) /V (kept) /DA (/Helv 10 Tf 0 g) /Rect [10 10 150 30] /P 3 0 R >>",
		7: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (gone) /V (typed on a page that is not here) /DA (/Helv 10 Tf 0 g) /Rect [10 40 150 60] >>",
	})
	path := filepath.Join(t.TempDir(), "cut.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	openByPath(t, ts.URL, c, csrf, path)
	offPage := func(rep pdfops.ScanReport) bool {
		for _, f := range rep.Findings {
			if strings.Contains(f.Detail, "on no page") {
				return true
			}
		}
		return false
	}
	// Stimulus: the scan route reports it before anything is removed.
	var rep pdfops.ScanReport
	sr, err := c.Get(ts.URL + "/api/scan?auth=" + csrf)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(sr.Body).Decode(&rep)
	sr.Body.Close()
	if !offPage(rep) {
		t.Fatalf("setup: the scan does not report the entry on no page: %+v", rep.Findings)
	}
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=fields", "", nil)
	var out sanitizeResponse
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !out.Ok || offPage(out.Residual) {
		t.Fatalf("the removal: %d ok=%v residual=%+v", resp.StatusCode, out.Ok, out.Residual.Findings)
	}
	// Read through the parsers, never the bytes: the written file keeps its objects in compressed streams.
	now := fetchPDF(t, ts, c)
	if left, err := pdfops.Scan(now); err != nil || offPage(left) {
		t.Errorf("the open document still holds an entry on no page (%v)", err)
	}
	if data, err := pdfops.ExportFormJSON(now); err != nil || !bytes.Contains(data, []byte(`"kept"`)) {
		t.Errorf("the field on the page lost its value: %s (%v)", data, err)
	}

	// A signed document: refused, and not a byte moved.
	fresp, signed := finalizeWith(t, c, csrf, ts.URL, pdf, `{"reason":"Finalized in Nib"}`)
	if fresp.StatusCode != http.StatusOK || len(signed) == 0 {
		t.Fatalf("setup: could not sign the fixture: %d", fresp.StatusCode)
	}
	spath := filepath.Join(t.TempDir(), "signed.pdf")
	if err := os.WriteFile(spath, signed, 0o600); err != nil {
		t.Fatal(err)
	}
	openByPath(t, ts.URL, c, csrf, spath)
	before := fetchPDF(t, ts, c)
	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/sanitize?method=fields", "", nil)
	var refused map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&refused)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(refused["error"], "signed") {
		t.Errorf("the removal on a signed document: %d %v — want 422 and the sentence", resp.StatusCode, refused)
	}
	if after := fetchPDF(t, ts, c); !bytes.Equal(before, after) {
		t.Error("the refused removal changed the signed document")
	}
}
