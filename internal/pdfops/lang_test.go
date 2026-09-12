package pdfops

import (
	"archive/zip"
	"bytes"
	"hash/crc32"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestSetLang(t *testing.T) {
	out, err := SetLang(threePagePDF(t), "th")
	if err != nil {
		t.Fatalf("SetLang: %v", err)
	}
	// The /Lang must survive a validating re-read (every later op reads this way).
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := root["Lang"].(types.StringLiteral); string(s) != "th" {
		t.Fatalf("catalog /Lang = %q, want \"th\"", root["Lang"])
	}
}

func TestSetLangRejectsEmpty(t *testing.T) {
	if _, err := SetLang(threePagePDF(t), "   "); err == nil {
		t.Error("blank language tag should error")
	}
}

func TestOCRLangToBCP47(t *testing.T) {
	cases := map[string]string{
		"eng": "en", "tha": "th", "hin": "hi", "ell": "el", "ukr": "uk",
		"": "", "zzz": "",
	}
	for code, want := range cases {
		if got := OCRLangToBCP47(code); got != want {
			t.Errorf("OCRLangToBCP47(%q) = %q, want %q", code, got, want)
		}
	}
}

// odtWithLang builds a minimal ODT whose paragraph declares a language. ODT rather than DOCX
// because it is LibreOffice's own native format: if the converter ignores a language declaration
// *there*, it is not a parsing failure on a foreign format.
func odtWithLang(t *testing.T, text, lang, country string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// The mimetype entry must be first and STORED — that is how the format is sniffed.
	now := time.Now()
	// **`CreateRaw`, not `Create`.** ODF requires the mimetype entry first, STORED, and
	// LibreOffice reads it at a fixed offset. Go's zip writer streams an entry it was not given
	// sizes for, which sets the data-descriptor bit and moves everything after it — measured
	// 2026-09-11, LibreOffice answers "source file could not be loaded" while `unzip -l` reads the
	// archive without complaint, so the failure looks like a content error and is a container one.
	mime := []byte("application/vnd.oasis.opendocument.text")
	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store, Modified: now}
	mh.CRC32 = crc32.ChecksumIEEE(mime)
	mh.CompressedSize64, mh.UncompressedSize64 = uint64(len(mime)), uint64(len(mime))
	mt, err := zw.CreateRaw(mh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mt.Write(mime); err != nil {
		t.Fatal(err)
	}
	// Ordered, not a map: mimetype first is part of the format, and a deterministic order keeps a
	// failure reproducible. Each entry carries a real `Modified` — Go's `zip.Create` leaves it at
	// the zero value, which writes an invalid `1980-00-00` date that LibreOffice refuses to load
	// ("source file could not be loaded", measured 2026-09-11) while `unzip -l` reads it happily.
	parts := []struct{ name, body string }{
		{"content.xml", `<?xml version="1.0" encoding="UTF-8"?>` +
			`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"` +
			` xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"` +
			` xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"` +
			` xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" office:version="1.2">` +
			`<office:automatic-styles><style:style style:name="P1" style:family="paragraph">` +
			`<style:text-properties fo:language="` + lang + `" fo:country="` + country + `"/>` +
			`</style:style></office:automatic-styles>` +
			`<office:body><office:text><text:p text:style-name="P1">` + text + `</text:p></office:text></office:body>` +
			`</office:document-content>`},
		{"META-INF/manifest.xml", `<?xml version="1.0" encoding="UTF-8"?>` +
			`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">` +
			`<manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/>` +
			`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>` +
			`</manifest:manifest>`},
	}
	for _, part := range parts {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: part.name, Method: zip.Deflate, Modified: now})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(part.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func catalogLang(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	s, _ := root["Lang"].(types.StringLiteral)
	return string(s)
}

// requireLibreOffice skips loudly. A skip that says nothing is credited as a pass by whoever reads
// the run — `/pending 411`'s failure mode, where three seed tests reported SKIP on a condition that
// was silently always true.
func requireLibreOffice(t *testing.T) {
	t.Helper()
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is not installed, so the office conversion path " +
			"cannot be driven and its /Lang classification in langdoor_test.go is unverified here")
	}
}

// TestAConvertersLanguageDoesNotComeFromTheDocument — `PLAN-accessibility.md` P03.S02.
//
// This is the measurement that overturned the slice, kept as a standing reader. `langdoor_test.go`
// classifies the office conversion door `carries`, and the reason it gives turns on WHAT that
// carried value is: not a determination about the document, but the converting machine's locale.
//
// **Asserted as an equality between two documents rather than against a locale**, so it needs no
// particular language installed and no environment variable: two sources declaring *different*
// languages producing the *same* `/Lang` is proof the value did not come from either of them.
// Confirmed separately, by hand on 2026-09-11, in the direction this cannot portably test —
// re-running the same ODT under `LANG=de_DE.UTF-8` yields `/Lang=(de-DE)`.
func TestAConvertersLanguageDoesNotComeFromTheDocument(t *testing.T) {
	requireLibreOffice(t)
	de, err := ConvertOfficeToPDF(odtWithLang(t, "Guten Tag.", "de", "DE"), ".odt")
	if err != nil {
		t.Fatalf("ConvertOfficeToPDF(de): %v", err)
	}
	th, err := ConvertOfficeToPDF(odtWithLang(t, "Hello.", "th", "TH"), ".odt")
	if err != nil {
		t.Fatalf("ConvertOfficeToPDF(th): %v", err)
	}
	gotDE, gotTH := catalogLang(t, de), catalogLang(t, th)
	if gotDE == "" || gotTH == "" {
		t.Fatalf("the converter emitted no /Lang at all (de=%q th=%q) — the `carries` row in "+
			"langdoor_test.go says it always does, and that is now false", gotDE, gotTH)
	}
	if gotDE != gotTH {
		t.Errorf("two documents declaring different languages converted to /Lang %q and %q — the "+
			"converter now DOES derive it from the document, so langdoor_test.go's reason for the "+
			"office door is out of date and the user-declared-language gap may be narrower than "+
			"it says", gotDE, gotTH)
	}
}

// TestNibDoesNotReplaceAConvertersLanguage is the claim `langdoor_test.go`'s `carries` rows
// actually depend on: whatever the converter decided survives nib's authoring path intact.
//
// nib passes it through rather than stripping it, and that is a decision rather than an oversight.
// Stripping would take a correct declaration off every document whose author and machine share a
// language — the common case — to avoid a wrong one on the documents where they do not, and it
// would move the office path from one failing ua1 clause to three.
func TestNibDoesNotReplaceAConvertersLanguage(t *testing.T) {
	requireLibreOffice(t)
	src := odtWithLang(t, "Guten Tag.", "de", "DE")
	converted, err := ConvertDocToPDF(src, ".odt")
	if err != nil {
		t.Fatalf("ConvertDocToPDF: %v", err)
	}
	before := catalogLang(t, converted)
	if before == "" {
		t.Fatal("nothing to preserve: the conversion emitted no /Lang")
	}
	titled, err := SetTitle(converted, "A converted document")
	if err != nil {
		t.Fatalf("SetTitle: %v", err)
	}
	if after := catalogLang(t, titled); after != before {
		t.Errorf("nib's authoring path changed the converter's /Lang from %q to %q — nib is "+
			"substituting its own guess for a value it did not determine", before, after)
	}
}
