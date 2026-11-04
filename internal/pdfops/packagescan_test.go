package pdfops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nib/internal/ziputil"
)

func pkg(t *testing.T, parts ...string) []byte {
	t.Helper()
	var es []ziputil.Entry
	for i := 0; i+1 < len(parts); i += 2 {
		es = append(es, ziputil.Entry{Name: parts[i], Data: []byte(parts[i+1])})
	}
	b, err := ziputil.Write(es)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rels(targets ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i, t := range targets {
		mode := ""
		if strings.HasPrefix(t, "ext:") {
			t, mode = strings.TrimPrefix(t, "ext:"), ` TargetMode="External"`
		}
		b.WriteString(`<Relationship Id="r` + string(rune('a'+i)) + `" Type="http://x/rel" Target="` + t + `"` + mode + `/>`)
	}
	return b.String() + `</Relationships>`
}

func odf(body string) string {
	return `<?xml version="1.0"?><office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:xlink="http://www.w3.org/1999/xlink"><office:body>` + body + `</office:body></office:document-content>`
}

// The refused references each reached LibreOffice's fetch paths in some version or product; the allowed ones are
// ordinary documents (a clicked hyperlink, a picture in the package, a fragment).
func TestAPackageThatNamesAFileOrAddressOutsideItselfIsRefusedBeforeAConverterSeesIt(t *testing.T) {
	docx := func(targets ...string) []byte {
		return pkg(t, "[Content_Types].xml", "<Types/>", "word/document.xml", "<d/>", "word/_rels/document.xml.rels", rels(targets...))
	}
	odt := func(body string) []byte {
		return pkg(t, "mimetype", "application/vnd.oasis.opendocument.text", "content.xml", odf(body),
			"Pictures/a.png", "x", "Object 1/content.xml", "<o/>", "Object 2/content.xml", "<o/>")
	}
	refused := map[string][]byte{
		"docx external file:":        docx("ext:file:///etc/hostname"),
		"docx external absolute":     docx("ext:/etc/hostname"),
		"docx external UNC":          docx(`ext:\\\\host\\share\\x`),
		"docx external // share":     docx("ext://host/share/x"),
		"docx external smb:":         docx("ext:smb://host/x"),
		"docx external drive":        docx(`ext:C:/Users/x/secret.txt`),
		"docx external ftp:":         docx("ext:ftp://host/x"),
		"docx internal climbs out":   docx("../../etc/hostname"),
		"docx internal absolute out": docx("/etc/hostname"),
		"docx internal file:":        docx("file:///etc/hostname"),
		"odt picture file:":          odt(`<draw:image xlink:href="file:///etc/hostname"/>`),
		"odt section source":         odt(`<text:section text:name="S"><text:section-source xlink:href="file:///etc/hostname"/></text:section>`),
		"odt table source":           odt(`<table:table-source xlink:href="/etc/hostname"/>`),
		"odt climbs out":             odt(`<draw:object xlink:href="../../x"/>`),
		"odt dangling relative":      odt(`<draw:object xlink:href="other.odt"/>`),
		"odt percent-encoded climb":  odt(`<draw:object xlink:href="%2e%2e/%2e%2e/etc/hostname"/>`),
		"odt percent-encoded scheme": odt(`<draw:image xlink:href="file%3A///etc/hostname"/>`),
		"odt no mimetype member":     pkg(t, "META-INF/manifest.xml", "<m/>", "content.xml", odf(`<draw:image xlink:href="file:///etc/hostname"/>`)),
		"docx external relative":     docx("ext:secret.txt"),
		"docx external sibling dir":  docx("ext:sub/x.docx"),
		"docx percent-encoded climb": docx("%2e%2e/%2e%2e/%2e%2e/etc/hostname"),
		"docx external encoded file": docx("ext:file%3A///etc/hostname"),
		"docx climbs past the root":  docx("../../../etc/hostname"),
		"docx upper-case .RELS":      pkg(t, "[Content_Types].xml", "<Types/>", "word/_rels/Document.XML.RELS", rels("ext:file:///etc/hostname")),
		"docx absolute not a member": pkg(t, "[Content_Types].xml", "<Types/>", "word/_rels/document.xml.rels", rels("/word/missing.xml")),
		"odt UNC":                    odt(`<draw:object xlink:href="\\\\host\\x"/>`),
		"odt embedded object":        pkg(t, "mimetype", "application/vnd.oasis.opendocument.text", "content.xml", odf(`<p/>`), "Object 1/content.xml", odf(`<draw:image xlink:href="file:///etc/hostname"/>`)),
	}
	for name, b := range refused {
		if err := scanPackageReferences(b); !errors.Is(err, ErrPackageNamesOutside) {
			t.Errorf("%s: err = %v, want ErrPackageNamesOutside", name, err)
		}
		// The one door every LibreOffice route reaches refuses it, before any tool is looked for.
		if _, err := ConvertOfficeToPDF(context.Background(), b, "docx"); !errors.Is(err, ErrPackageNamesOutside) {
			t.Errorf("%s: ConvertOfficeToPDF err = %v, want ErrPackageNamesOutside", name, err)
		}
	}
	allowed := map[string][]byte{
		"docx hyperlink":         docx("ext:https://example.com/a?b=c#d", "ext:mailto:a@b.example"),
		"docx internal relative": docx("media/image1.png", "styles.xml", "../customXml/item1.xml", "../docProps/core.xml", "my%20pic.png"),
		"docx package root rel":  pkg(t, "[Content_Types].xml", "<Types/>", "_rels/.rels", rels("word/document.xml", "docProps/core.xml"), "word/document.xml", "<d/>"),
		"docx internal absolute": pkg(t, "[Content_Types].xml", "<Types/>", "word/styles.xml", "<s/>", "word/_rels/document.xml.rels", rels("/word/styles.xml")),
		"odt fragment":           odt(`<text:a xlink:href="#bookmark"/>`),
		"odt package picture":    odt(`<draw:image xlink:href="Pictures/a.png"/><draw:object xlink:href="./Object 1"/>`),
		"odt web link":           odt(`<text:a xlink:href="https://example.com/"/><text:a xlink:href="mailto:a@b.example"/>`),
		"odt sibling subdir":     odt(`<draw:object xlink:href="Object 1/../Object 2"/>`),
		"no xlink at all":        odt(`<text:p>plain</text:p>`),
	}
	for name, b := range allowed {
		if err := scanPackageReferences(b); err != nil {
			t.Errorf("%s: err = %v, want nil", name, err)
		}
	}
}

func TestThePackageScanLeavesWhatItCannotReadAlone(t *testing.T) {
	for name, b := range map[string][]byte{
		"OLE":   append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...),
		"RTF":   []byte(`{\rtf1 \field{\*\fldinst INCLUDETEXT "/etc/hostname"}}`),
		"empty": nil,
	} {
		if err := scanPackageReferences(b); err != nil {
			t.Errorf("%s: err = %v, want nil (declared gap: not a zip package)", name, err)
		}
	}
	// A part that cannot be parsed is damaged, not passed.
	bad := pkg(t, "[Content_Types].xml", "<Types/>", "word/_rels/document.xml.rels", `<Relationships><Relationship Target="x" `)
	if err := scanPackageReferences(bad); !errors.Is(err, ErrCorruptDocument) {
		t.Errorf("an unparseable rels part = %v, want ErrCorruptDocument", err)
	}
	// A truncated zip is refused by the archive door, not scanned as empty.
	if err := scanPackageReferences([]byte("PK\x03\x04junk")); !errors.Is(err, ErrArchiveRefused) {
		t.Errorf("a truncated zip = %v, want ErrArchiveRefused", err)
	}
}

func TestThePackageRefusalIsCountedUnderItsOwnCause(t *testing.T) {
	if got := refusalCause(ErrPackageNamesOutside); got != "names-outside" {
		t.Errorf("cause = %q", got)
	}
	if !isNativeRefusal(ErrPackageNamesOutside) {
		t.Error("the refusal is not a native refusal, so a caller would call it a failed conversion")
	}
}

// The scan must not turn an ordinary document away: a Word file with a web link in it still converts.
func TestADocumentWithAWebLinkStillConverts(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice is not installed: the conversion half of this test is NOT RUN (the scan half ran above)")
	}
	const w = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	b := pkg(t,
		"[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml", `<?xml version="1.0"?><w:document `+w+`><w:body><w:p><w:hyperlink r:id="rId9"><w:r><w:t>a linked word</w:t></w:r></w:hyperlink></w:p></w:body></w:document>`,
		"word/_rels/document.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/" TargetMode="External"/></Relationships>`)
	out, err := ConvertOfficeToPDF(context.Background(), b, "docx")
	if err != nil {
		t.Fatalf("a document with a web link did not convert: %v", err)
	}
	if !strings.HasPrefix(string(out), "%PDF") {
		t.Errorf("not a PDF: %q", out[:min(len(out), 8)])
	}
}

// A zip with bytes in front of it is still a zip to a reader that finds the central directory from the end;
// the scan must not take the missing leading signature for "not a package" (P04 review).
func TestAPackageWithBytesInFrontOfItIsStillScanned(t *testing.T) {
	d := pkg(t, "[Content_Types].xml", "<Types/>", "word/document.xml", "<d/>", "word/_rels/document.xml.rels", rels("ext:file:///etc/passwd"))
	prefixed := append([]byte("JUNKJUNKJUNK"), d...)
	if err := scanPackageReferences(prefixed); err == nil {
		t.Error("a prefixed package naming a local file was passed")
	}
	if err := scanPackageReferences(d[:len(d)-30]); !errors.Is(err, ErrArchiveRefused) {
		t.Errorf("a package with its directory cut off: %v", err)
	}
	if err := scanPackageReferences([]byte("not a zip at all")); err != nil {
		t.Errorf("a non-zip was refused: %v", err)
	}
}
