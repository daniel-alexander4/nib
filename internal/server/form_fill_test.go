package server

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// fillCSVForm builds the multipart body the fill-csv endpoint expects: the form
// template as the "pdf" part plus the CSV text as the "data" field.
func fillCSVForm(t *testing.T, pdf []byte, csv string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	mw.WriteField("data", csv)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// TestFormFillCSVHandler: POSTing a form template + a CSV (header = field names,
// one row each) returns a ZIP holding one valid filled PDF per row. It operates
// on the uploaded bytes, so it needs no open document.
func TestFormFillCSVHandler(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	form, err := testpdf.Form() // fields: text "fullName", checkbox "agree"
	if err != nil {
		t.Fatal(err)
	}
	body, ct := fillCSVForm(t, form, "fullName,agree\nAlice,true\nBob,false\n")
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/form/fill-csv", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("fill-csv status = %d: %s", resp.StatusCode, out)
	}
	zipped, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		t.Fatalf("response is not a zip: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("zip holds %d files, want 2 (one per CSV row)", len(zr.File))
	}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if err := pdfops.Validate(b); err != nil {
			t.Errorf("%s is not a valid PDF: %v", f.Name, err)
		}
	}
}

// TestFormFillCSVHandlerNoFields: a flat PDF with no form fields is rejected
// (400), never a bogus empty zip.
func TestFormFillCSVHandlerNoFields(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	flat, _ := testpdf.Text("no fields here")
	body, ct := fillCSVForm(t, flat, "fullName\nAlice\n")
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/form/fill-csv", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("fill-csv on a fieldless PDF status = %d, want 400", resp.StatusCode)
	}
}
