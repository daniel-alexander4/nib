package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// fillXFDFForm builds the multipart body the fill-xfdf endpoint expects: the form
// template as the "pdf" part plus the XFDF text as the "data" field.
func fillXFDFForm(t *testing.T, pdf []byte, xfdf string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	mw.WriteField("data", xfdf)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// TestFormFillXFDFHandler: POSTing a form template + an XFDF record returns one
// valid filled PDF (a single record fills one document, so no zip). It operates on
// the uploaded bytes, needing no open document.
func TestFormFillXFDFHandler(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	form, err := testpdf.Form() // fields: text "fullName", checkbox "agree"
	if err != nil {
		t.Fatal(err)
	}
	xfdf := `<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields>` +
		`<field name="fullName"><value>Alice</value></field>` +
		`<field name="agree"><value>Yes</value></field></fields></xfdf>`
	body, ct := fillXFDFForm(t, form, xfdf)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/form/fill-xfdf", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("fill-xfdf status = %d: %s", resp.StatusCode, out)
	}
	b, _ := io.ReadAll(resp.Body)
	if err := pdfops.Validate(b); err != nil {
		t.Errorf("filled PDF invalid: %v", err)
	}
}

// TestFormFillXFDFHandlerNoFields: a flat PDF with no form fields is rejected (400).
func TestFormFillXFDFHandlerNoFields(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	flat, _ := testpdf.Text("no fields here")
	xfdf := `<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields>` +
		`<field name="fullName"><value>Alice</value></field></fields></xfdf>`
	body, ct := fillXFDFForm(t, flat, xfdf)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/form/fill-xfdf", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("fill-xfdf on a fieldless PDF status = %d, want 400", resp.StatusCode)
	}
}

// TestFormFillNamesTheFieldsItCouldNotReach: a widget whose /Parent leads nowhere takes no value (pdfcpu sets it
// aside), so both fill routes say which in `X-Nib-Skipped` - ASCII, with the count whole - and say nothing for a
// form with no such widget (/pending 919 F14).
func TestFormFillNamesTheFieldsItCouldNotReach(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	orphans := testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /DR << /Font << /Helv 5 0 R >> >> /DA (/Helv 10 Tf 0 g) >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Annots [6 0 R 9 0 R 10 0 R] >>",
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /DA (/Helv 10 Tf 0 g) /Rect [10 10 150 30] /P 3 0 R >>",
		9:  "<< /Type /Annot /Subtype /Widget /FT /Tx /T (orphan) /Parent 99 0 R /Rect [10 40 70 60] /P 3 0 R >>",
		10: "<< /Type /Annot /Subtype /Widget /FT /Tx /T <FEFF00C9> /Parent 98 0 R /Rect [10 80 70 100] /P 3 0 R >>",
	})
	xfdf := `<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields><field name="name"><value>A</value></field></fields></xfdf>`
	body, ct := fillXFDFForm(t, orphans, xfdf)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/form/fill-xfdf", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, out)
	}
	got := resp.Header.Get("X-Nib-Skipped")
	if got != "{\"count\":2,\"names\":[\"orphan\",\"\\u00c9\"]}" {
		t.Errorf("X-Nib-Skipped = %q, want both orphans with the accent as \\u00c9", got)
	}

	// The mail-merge says the same, from the same door.
	csvBody, csvCT := fillCSVForm(t, orphans, "name\nA\n")
	merged := write(t, c, csrf, http.MethodPost, ts.URL+"/api/form/fill-csv", csvCT, csvBody)
	defer merged.Body.Close()
	if merged.StatusCode != http.StatusOK || merged.Header.Get("X-Nib-Skipped") != got {
		t.Errorf("fill-csv: status %d, X-Nib-Skipped = %q, want %q", merged.StatusCode, merged.Header.Get("X-Nib-Skipped"), got)
	}

	form, _ := testpdf.Form()
	body, ct = fillXFDFForm(t, form, `<xfdf xmlns="http://ns.adobe.com/xfdf/"><fields><field name="fullName"><value>A</value></field></fields></xfdf>`)
	clean := write(t, c, csrf, http.MethodPost, ts.URL+"/api/form/fill-xfdf", ct, body)
	defer clean.Body.Close()
	if clean.StatusCode != http.StatusOK {
		t.Fatalf("the clean form's fill answered %d, so its missing header says nothing", clean.StatusCode)
	}
	if h := clean.Header.Get("X-Nib-Skipped"); h != "" {
		t.Errorf("a form with no such widget carried X-Nib-Skipped = %q", h)
	}
}
