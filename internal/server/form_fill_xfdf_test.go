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
