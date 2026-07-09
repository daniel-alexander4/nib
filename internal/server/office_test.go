package server

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/pdfops"
)

// officeForm builds the multipart body the office endpoint expects: the document
// as the "file" part, named so the handler can read its extension.
func officeForm(t *testing.T, name string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func tinyDOCX(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml": `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
			`<w:body><w:p><w:r><w:t>Office handler test.</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, body := range parts {
		fw, _ := zw.Create(name)
		fw.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

// TestOfficeHandler: a posted DOCX is converted and installed as the working
// document; the response names it .pdf and /api/pdf then serves a real PDF. Skips
// when LibreOffice isn't installed.
func TestOfficeHandler(t *testing.T) {
	if !pdfops.LibreOfficeAvailable() {
		t.Skip("LibreOffice not installed; skipping office conversion handler")
	}
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	body, ct := officeForm(t, "report.docx", tinyDOCX(t))
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/office", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("office status = %d: %s", resp.StatusCode, out)
	}
	out, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(out, []byte(`"name":"report.pdf"`)) {
		t.Errorf("response should name the converted file report.pdf, got: %s", out)
	}

	// The converted PDF is now the working document.
	pr, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Body.Close()
	pdf, _ := io.ReadAll(pr.Body)
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Errorf("working document is not a PDF after office conversion")
	}
}

// TestOfficeHandlerRejectsUnsupported: a non-office upload is refused with 400
// before any conversion is attempted.
func TestOfficeHandlerRejectsUnsupported(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	body, ct := officeForm(t, "malware.exe", []byte("MZ..."))
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/office", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("office on a .exe status = %d, want 400", resp.StatusCode)
	}
}

// TestOfficeHandlerMarkdown: a posted Markdown file converts natively (no
// LibreOffice, so no skip) and is installed as the working document.
func TestOfficeHandlerMarkdown(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	body, ct := officeForm(t, "notes.md", []byte("# Notes\n\n- one\n- two\n"))
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/office", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("markdown convert status = %d: %s", resp.StatusCode, out)
	}
	out, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(out, []byte(`"name":"notes.pdf"`)) {
		t.Errorf("response should name the converted file notes.pdf, got: %s", out)
	}

	pr, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Body.Close()
	pdf, _ := io.ReadAll(pr.Body)
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Errorf("working document is not a PDF after markdown conversion")
	}
}
