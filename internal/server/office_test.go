package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
)

// importByPath is how the window converts a file since Import… folded into Open… (ADR-215): the file is
// written where a person keeps it and opened BY PATH, carrying the Open dialog's language field. These
// tests were /api/office's; that upload route is retired and every property it was tested for is asked
// of /api/open here.
func importByPath(t *testing.T, base string, c *http.Client, csrf, name string, data []byte, lang string, chosen bool) *http.Response {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(openRequest{Path: p, Lang: lang, LangChosen: chosen})
	return write(t, c, csrf, http.MethodPost, base+"/api/open", "application/json", bytes.NewReader(body))
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

// TestOfficeHandler: a DOCX opened by path is converted and installed as the working
// document; the response names it .pdf and /api/pdf then serves a real PDF. Skips
// when LibreOffice isn't installed.
func TestOfficeHandler(t *testing.T) {
	if !pdfops.LibreOfficeAvailable() {
		t.Skip("LibreOffice not installed; skipping office conversion handler")
	}
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp := importByPath(t, ts.URL, c, csrf, "report.docx", tinyDOCX(t), "", false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(resp.Body)
		t.Fatalf("open of a .docx status = %d: %s", resp.StatusCode, out)
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

// TestOfficeHandlerRejectsUnsupported: a file Nib has no route for is refused with 415 — the same status
// and sentence every open route gives it, because one function decides (D8) — before any conversion.
func TestOfficeHandlerRejectsUnsupported(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp := importByPath(t, ts.URL, c, csrf, "malware.exe", []byte("MZ..."), "", false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("open of a .exe status = %d, want 415", resp.StatusCode)
	}
}

// TestOfficeHandlerMarkdown: a Markdown file opened by path converts natively (no
// LibreOffice, so no skip) and is installed as the working document.
func TestOfficeHandlerMarkdown(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp := importByPath(t, ts.URL, c, csrf, "notes.md", []byte("# Notes\n\n- one\n- two\n"), "", false)
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
