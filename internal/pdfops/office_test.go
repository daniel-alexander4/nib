package pdfops

import (
	"archive/zip"
	"bytes"
	"testing"
)

// minimalDOCX builds the smallest valid Word document carrying one line of text —
// enough for LibreOffice to convert. Avoids a binary test fixture.
func minimalDOCX(t *testing.T, text string) []byte {
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
			`<w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, body := range parts {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestConvertOfficeToPDF: a real DOCX converts to a valid PDF. Skips when
// LibreOffice isn't installed (the poppler/Ghostscript oracle pattern), so the
// suite stays green on a machine without it.
func TestConvertOfficeToPDF(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice not installed; skipping office conversion")
	}
	out, err := ConvertOfficeToPDF(minimalDOCX(t, "Hello from Nib."), ".docx")
	if err != nil {
		t.Fatalf("ConvertOfficeToPDF: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("output is not a PDF (no %%PDF header): % x", out[:min(8, len(out))])
	}
	if err := Validate(out); err != nil {
		t.Errorf("converted PDF invalid: %v", err)
	}
}

// TestConvertOfficeRejectsUnsupported: an extension outside the allowlist is
// refused with ErrUnsupportedOffice and never reaches LibreOffice.
func TestConvertOfficeRejectsUnsupported(t *testing.T) {
	if _, err := ConvertOfficeToPDF([]byte("whatever"), ".exe"); err != ErrUnsupportedOffice {
		t.Errorf("ConvertOfficeToPDF(.exe) error = %v, want ErrUnsupportedOffice", err)
	}
	if _, err := ConvertOfficeToPDF([]byte("whatever"), ""); err != ErrUnsupportedOffice {
		t.Errorf("ConvertOfficeToPDF(no ext) error = %v, want ErrUnsupportedOffice", err)
	}
}

// TestSupportedOfficeExt is case- and dot-insensitive over the allowlist.
func TestSupportedOfficeExt(t *testing.T) {
	for _, ok := range []string{"docx", ".docx", ".DOCX", "Xlsx", ".odt", "csv", ".pptx"} {
		if !SupportedOfficeExt(ok) {
			t.Errorf("SupportedOfficeExt(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"exe", ".pdf", "", ".sh", "docx.exe"} {
		if SupportedOfficeExt(bad) {
			t.Errorf("SupportedOfficeExt(%q) = true, want false", bad)
		}
	}
}
