package pdfops

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"nib/internal/ziputil"
	"nib/mdpdf"
)

const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

func docxWith(t *testing.T, body string, extra ...ziputil.Entry) []byte {
	t.Helper()
	es := []ziputil.Entry{
		{Name: "[Content_Types].xml", Data: []byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`)},
		{Name: "_rels/.rels", Data: []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`)},
		{Name: "word/document.xml", Data: []byte(`<?xml version="1.0" encoding="UTF-8"?><w:document ` + wNS + `><w:body>` + body + `</w:body></w:document>`)},
	}
	b, err := ziputil.Write(append(es, extra...))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func odtWith(t *testing.T, body string) []byte {
	t.Helper()
	b, err := ziputil.Write([]ziputil.Entry{
		{Name: "mimetype", Data: []byte("application/vnd.oasis.opendocument.text"), Store: true},
		{Name: "META-INF/manifest.xml", Data: []byte(`<?xml version="1.0"?><manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">` +
			`<manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/>` +
			`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`)},
		{Name: "content.xml", Data: []byte(`<?xml version="1.0"?><office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" office:version="1.2"><office:body><office:text>` + body + `</office:text></office:body></office:document-content>`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func partOf(t *testing.T, pkg []byte, name string) string {
	t.Helper()
	a, err := ziputil.Open(bytes.NewReader(pkg), int64(len(pkg)), packageLimits)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range a.Files() {
		if m.Name == name {
			b, err := a.Read(m, packagePartCap)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("no part %s", name)
	return ""
}

func fillDocx(t *testing.T, body string, vars map[string]string) (string, error) {
	t.Helper()
	out, err := FillOfficeTemplate(docxWith(t, body), ".docx", vars)
	if err != nil {
		return "", err
	}
	return partOf(t, out, "word/document.xml"), nil
}

func wRun(text string) string { return `<w:r><w:rPr><w:b/></w:rPr><w:t>` + text + `</w:t></w:r>` }

func TestAWholePlaceholderInOneRunIsFilled(t *testing.T) {
	got, err := fillDocx(t, `<w:p><w:r><w:t>Dear {{name}},</w:t></w:r></w:p>`, map[string]string{"name": "Ana"})
	if err != nil || !strings.Contains(got, `>Dear </w:t>`) && !strings.Contains(got, "Dear Ana,") {
		t.Fatalf("%v\n%s", err, got)
	}
	if !strings.Contains(got, "Dear Ana,") || strings.Contains(got, "{{") {
		t.Errorf("not filled: %s", got)
	}
}

// The case the item exists for: Word splits the key across runs, with proofErr and rsid-style boundaries between.
func TestAPlaceholderSplitAcrossRunsIsFilledIntoTheRunItBeginsIn(t *testing.T) {
	body := `<w:p>` + wRun("Hi {{na") + `<w:proofErr w:type="spellStart"/><w:r><w:rPr><w:i/></w:rPr><w:t>m</w:t></w:r>` + wRun("e}}, bye") + `</w:p>`
	got, err := fillDocx(t, body, map[string]string{"name": "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<w:t xml:space="preserve">Hi Ana</w:t>`, `<w:t xml:space="preserve"></w:t>`, `<w:t xml:space="preserve">, bye</w:t>`, `<w:rPr><w:i/></w:rPr>`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "{{") || strings.Contains(got, "}}") {
		t.Errorf("braces left: %s", got)
	}
}

func TestSplitAndWholePlaceholdersInOneParagraph(t *testing.T) {
	body := `<w:p>` + wRun("{{a}} and {{b") + wRun("}}") + `</w:p>`
	got, err := fillDocx(t, body, map[string]string{"a": "1", "b": "2"})
	if err != nil || !strings.Contains(got, "1 and 2") {
		t.Fatalf("%v %s", err, got)
	}
}

func TestEscapeAndTheMissingAndMalformedErrorsNameTheKeyNotTheValue(t *testing.T) {
	got, err := fillDocx(t, `<w:p><w:r><w:t>{{=name}} is {{name}}</w:t></w:r></w:p>`, map[string]string{"name": "SECRET"})
	if err != nil || !strings.Contains(got, "{{name}} is SECRET") {
		t.Fatalf("%v %s", err, got)
	}
	_, err = fillDocx(t, `<w:p><w:r><w:t>{{missing}}</w:t></w:r></w:p>`, map[string]string{"other": "SECRET"})
	var miss *mdpdf.MissingPlaceholderError
	if !errors.As(err, &miss) || miss.Key != "missing" || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("missing key: %v", err)
	}
	_, err = fillDocx(t, `<w:p><w:r><w:t>{{first name}}</w:t></w:r></w:p>`, map[string]string{"first": "x"})
	var bad *mdpdf.MalformedPlaceholderError
	if !errors.As(err, &bad) {
		t.Errorf("malformed: %v", err)
	}
}

// A tab, a break or a field between two runs is not text: braces cannot run across it.
func TestAPlaceholderDoesNotRunAcrossATabOrBreak(t *testing.T) {
	body := `<w:p>` + wRun("{{na") + `<w:tab/>` + wRun("me}}") + `</w:p>`
	_, err := fillDocx(t, body, map[string]string{"name": "x", "na": "y"})
	var bad *mdpdf.MalformedPlaceholderError
	if !errors.As(err, &bad) {
		t.Errorf("a tab inside the braces must be a refusal, got %v", err)
	}
}

func TestAPlaceholderDoesNotCrossAParagraph(t *testing.T) {
	got, err := fillDocx(t, `<w:p>`+wRun("a {{")+`</w:p><w:p>`+wRun("name}}")+`</w:p>`, map[string]string{"name": "x"})
	if err != nil || !strings.Contains(got, "a {{") || !strings.Contains(got, "name}}") {
		t.Errorf("%v %s", err, got)
	}
}

func TestTheValueIsTextAndTabsAndNewlinesBecomeWordElements(t *testing.T) {
	got, err := fillDocx(t, `<w:p><w:r><w:t>{{v}}</w:t></w:r></w:p>`, map[string]string{"v": " <b>&</b> a\nb\tc "})
	if err != nil {
		t.Fatal(err)
	}
	want := `<w:t xml:space="preserve"> &lt;b&gt;&amp;&lt;/b&gt; a</w:t><w:br/><w:t xml:space="preserve">b</w:t><w:tab/><w:t xml:space="preserve">c </w:t>`
	if !strings.Contains(got, want) {
		t.Errorf("want %s in %s", want, got)
	}
}

func TestAValueXMLCannotCarryIsRefusedByKey(t *testing.T) {
	_, err := fillDocx(t, `<w:p><w:r><w:t>{{v}}</w:t></w:r></w:p>`, map[string]string{"v": "a\x01b"})
	var u *UnwritableValueError
	if !errors.As(err, &u) || u.Key != "v" {
		t.Errorf("got %v", err)
	}
}

// Deleted text, instructions and everything outside a text node are not read, and bytes outside an edit are untouched.
func TestOnlyTextNodesAreReadAndEverythingElseIsByteIdentical(t *testing.T) {
	body := `<w:p w:rsidR="00A1"><w:del><w:r><w:delText>{{gone}}</w:delText></w:r></w:del><w:r><w:instrText> MERGEFIELD {{field}} </w:instrText></w:r>` + wRun("keep") + `</w:p>`
	in := docxWith(t, body)
	out, err := FillOfficeTemplate(in, ".docx", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if partOf(t, out, "word/document.xml") != partOf(t, in, "word/document.xml") {
		t.Error("a document with nothing to fill changed")
	}
}

func TestHeadersFootersAndNotesAreFilledAndOtherPartsAreCarried(t *testing.T) {
	hdr := ziputil.Entry{Name: "word/header1.xml", Data: []byte(`<w:hdr ` + wNS + `><w:p>` + wRun("{{co}}") + `</w:p></w:hdr>`)}
	other := ziputil.Entry{Name: "word/media/x.bin", Data: []byte("{{co}} binary")}
	out, err := FillOfficeTemplate(docxWith(t, `<w:p>`+wRun("x")+`</w:p>`, hdr, other), ".docx", map[string]string{"co": "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(partOf(t, out, "word/header1.xml"), "Acme") {
		t.Error("header not filled")
	}
	if partOf(t, out, "word/media/x.bin") != "{{co}} binary" {
		t.Error("a member that is not text was touched")
	}
}

func TestAFilledPartPastTheCapIsRefused(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<w:p><w:r><w:t>`)
	for i := 0; i < 100000; i++ {
		sb.WriteString("{{v}}")
	}
	sb.WriteString(`</w:t></w:r></w:p>`)
	_, err := fillDocx(t, sb.String(), map[string]string{"v": strings.Repeat("x", 64<<10)})
	if !errors.Is(err, ErrTemplateFilledTooLarge) {
		t.Errorf("got %v", err)
	}
}

func fillOdt(t *testing.T, body string, vars map[string]string) (string, error) {
	t.Helper()
	out, err := FillOfficeTemplate(odtWith(t, body), ".odt", vars)
	if err != nil {
		return "", err
	}
	return partOf(t, out, "content.xml"), nil
}

func TestAnODTPlaceholderSplitAcrossSpansIsFilledAndTheMimetypeStaysFirstAndStored(t *testing.T) {
	in := odtWith(t, `<text:p>Dear <text:span text:style-name="B">{{na</text:span><text:span text:style-name="I">me}}</text:span>,  x</text:p>`)
	out, err := FillOfficeTemplate(in, ".odt", map[string]string{"name": "Ana  B"})
	if err != nil {
		t.Fatal(err)
	}
	got := partOf(t, out, "content.xml")
	if !strings.Contains(got, `>Ana <text:s text:c="1"/>B</text:span><text:span text:style-name="I"></text:span>,  x`) {
		t.Errorf("%s", got)
	}
	a, _ := ziputil.Open(bytes.NewReader(out), int64(len(out)), packageLimits)
	if f := a.Files()[0]; f.Name != "mimetype" || f.Method != 0 {
		t.Errorf("mimetype is %s method %d", f.Name, f.Method)
	}
}

func TestAnODTTabLineBreakAndFrameKeepTheBracesApart(t *testing.T) {
	_, err := fillOdt(t, `<text:p>{{a<text:tab/>b}}</text:p>`, map[string]string{"ab": "x"})
	var bad *mdpdf.MalformedPlaceholderError
	if !errors.As(err, &bad) {
		t.Errorf("got %v", err)
	}
	got, err := fillOdt(t, `<text:p>{{v}}</text:p>`, map[string]string{"v": "a\nb"})
	if err != nil || !strings.Contains(got, `a<text:line-break/>b`) {
		t.Errorf("%v %s", err, got)
	}
}

func TestASplitAndEscapedKeyInAnODTHeading(t *testing.T) {
	got, err := fillOdt(t, `<text:h text:outline-level="1">{{=k}} {{k}}</text:h>`, map[string]string{"k": "v"})
	if err != nil || !strings.Contains(got, "{{k}} v") {
		t.Errorf("%v %s", err, got)
	}
}

// The option gate: placeholders are DOCX's and ODT's as well as Markdown's; every other option stays Markdown's.
func TestTheOptionGateAdmitsPlaceholdersOnADOCXAndNothingElseOnAnOfficeFile(t *testing.T) {
	vars := mdpdf.Options{Vars: map[string]string{"a": "b"}}
	for ext, want := range map[string]bool{".docx": false, ".odt": false, ".xlsx": true, ".pptx": true, ".ods": true} {
		if got := optionsNeedMarkdown(ext, vars); got != want {
			t.Errorf("placeholders on %s refused = %v, want %v", ext, got, want)
		}
	}
	for _, o := range []mdpdf.Options{{PageSize: "a4"}, {Margin: 10}, {ImageDir: "x"}} {
		if !optionsNeedMarkdown(".docx", o) {
			t.Errorf("%+v must still be refused on a docx", o)
		}
	}
	if _, err := ConvertDocToPDFWith(context.Background(), []byte("x"), ".xlsx", vars); !errors.Is(err, ErrOptionsNeedMarkdown) {
		t.Errorf("xlsx: %v", err)
	}
}

// The whole path: a filled DOCX goes through LibreOffice and the value is on the page. Skips without the tools.
func TestAFilledDOCXPrintsItsValuesThroughLibreOffice(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice not installed")
	}
	txt, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}
	in := docxWith(t, `<w:p>`+wRun("Dear {{na")+`<w:r><w:t>m</w:t></w:r>`+wRun("e}},")+`</w:p>`)
	pdf, err := ConvertDocToPDFWith(context.Background(), in, ".docx", mdpdf.Options{Vars: map[string]string{"name": "Ana Zed"}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(txt, "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	out, err := cmd.Output()
	if err != nil || !strings.Contains(string(out), "Dear Ana Zed,") {
		t.Errorf("%v: %q", err, out)
	}
}

func TestAFilledODTPrintsItsValuesThroughLibreOffice(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("LibreOffice not installed")
	}
	txt, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}
	in := odtWith(t, `<text:p>Dear <text:span text:style-name="B">{{na</text:span><text:span>me}}</text:span>, ok</text:p>`)
	pdf, err := ConvertDocToPDFWith(context.Background(), in, ".odt", mdpdf.Options{Vars: map[string]string{"name": "Ana Zed"}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(txt, "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	out, err := cmd.Output()
	if err != nil || !strings.Contains(string(out), "Dear Ana Zed, ok") {
		t.Errorf("%v: %q", err, out)
	}
}

// Markup that is not the prefix Word writes is a refusal, never a document handed back with its braces.
func TestAPartWhoseMarkupIsNotRecognisedButHoldsBracesIsRefused(t *testing.T) {
	in, err := ziputil.Write([]ziputil.Entry{
		{Name: "[Content_Types].xml", Data: []byte(`<Types/>`)},
		{Name: "word/document.xml", Data: []byte(`<document xmlns="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><body><p><r><t>{{a}}</t></r></p></body></document>`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FillOfficeTemplate(in, ".docx", map[string]string{"a": "x"}); !errors.Is(err, ErrCorruptDocument) {
		t.Errorf("got %v", err)
	}
}

// A picture bigger than a text part may be is carried through whole: the cap on a text part is not the package's.
func TestAMemberBiggerThanATextPartIsCarriedThrough(t *testing.T) {
	big := ziputil.Entry{Name: "word/media/big.bin", Data: make([]byte, packagePartCap+1<<20)}
	out, err := FillOfficeTemplate(docxWith(t, `<w:p>`+wRun("{{a}}")+`</w:p>`, big), ".docx", map[string]string{"a": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got := partOf2(t, out, "word/media/big.bin"); got != len(big.Data) {
		t.Errorf("the big member is %d bytes after, %d before", got, len(big.Data))
	}
}

func partOf2(t *testing.T, pkg []byte, name string) int {
	t.Helper()
	a, err := ziputil.Open(bytes.NewReader(pkg), int64(len(pkg)), packageLimits)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range a.Files() {
		if m.Name == name {
			b, err := a.Read(m, int64(importDecodedBytes))
			if err != nil {
				t.Fatal(err)
			}
			return len(b)
		}
	}
	t.Fatalf("no part %s", name)
	return 0
}
