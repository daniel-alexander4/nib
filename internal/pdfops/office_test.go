package pdfops

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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

// TestConvertDocMarkdown: Markdown converts natively — no LibreOffice, no skip.
func TestConvertDocMarkdown(t *testing.T) {
	out, err := ConvertDocToPDF([]byte("# Title\n\nBody with **bold** text.\n"), ".md")
	if err != nil {
		t.Fatalf("ConvertDocToPDF: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("output is not a PDF (no %%PDF header)")
	}
	if err := Validate(out); err != nil {
		t.Errorf("converted PDF invalid: %v", err)
	}
}

// TestSupportedDocExt: Markdown extensions are accepted alongside the office
// allowlist; everything else still refuses.
func TestSupportedDocExt(t *testing.T) {
	for _, ok := range []string{"md", ".md", ".MD", "markdown", ".docx", "csv"} {
		if !SupportedDocExt(ok) {
			t.Errorf("SupportedDocExt(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"exe", ".pdf", "", ".html"} {
		if SupportedDocExt(bad) {
			t.Errorf("SupportedDocExt(%q) = true, want false", bad)
		}
	}
	if SupportedOfficeExt(".md") {
		t.Error("SupportedOfficeExt(.md) = true — Markdown must not route to LibreOffice")
	}
}

// A Markdown file containing non-Latin text converts with that text PRESENT.
//
// The Base-14 core fonts mdpdf uses are WinAnsi, and pdfcpu maps anything outside that to
// a SPACE rather than erroring — so Cyrillic, Greek, CJK and the Indic scripts came out
// blank in a valid PDF that nothing could tell from a correct one. mdpdf gained a fallback
// pool; this asserts the wiring, which is the half that could be right in the library and
// missing in Nib.
//
// Driven through ConvertDocToPDF — the route the office surface actually calls — rather
// than through mdpdf directly, because "mdpdf can do it" and "Nib does it" are different
// claims and only the second one reaches a user.
func TestConvertDocToPDFPrintsNonLatinMarkdown(t *testing.T) {
	// One from each family the pool covers, so a face dropped from
	// markdownFallbackFonts fails here rather than in whichever script nobody tests.
	for _, tc := range []struct{ name, text, face string }{
		{"thai", "สวัสดีครับ", "NotoSansThai"},
		{"cjk", "文書の署名", "DroidSansFallback"},
		{"arabic", "مرحبا بالعالم", "NotoSansArabic"},
		{"hangul", "안녕하세요", "NanumGothic"},
		{"devanagari", "नमस्ते दुनिया", "NotoSansDevanagari"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ConvertDocToPDF([]byte("# "+tc.text+"\n\n"+tc.text+"\n"), ".md")
			if err != nil {
				t.Fatalf("ConvertDocToPDF: %v", err)
			}
			if err := Validate(out); err != nil {
				t.Fatalf("the converted PDF does not validate: %v", err)
			}
			ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("re-reading: %v", err)
			}
			var faces []string
			for _, e := range ctx.XRefTable.Table {
				if e == nil {
					continue
				}
				if d, ok := e.Object.(types.Dict); ok {
					if b := d.NameEntry("BaseFont"); b != nil {
						faces = append(faces, *b)
					}
				}
			}
			found := false
			for _, f := range faces {
				if strings.Contains(f, tc.face) {
					found = true
				}
			}
			if !found {
				t.Errorf("%s text converted without embedding %s (fonts present: %v) — it was set in a core font, which renders it as spaces", tc.name, tc.face, faces)
			}
		})
	}
}
