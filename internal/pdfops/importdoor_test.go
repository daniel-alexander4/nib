package pdfops

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"nib/internal/gate"
)

// asReader is bytes as the ReaderAt and size Import takes.
func asReader(b []byte) (io.ReaderAt, int64) { return BytesSource(b) }

func importBytes(t *testing.T, b []byte, name string, o ImportOptions) (*ImportResult, error) {
	t.Helper()
	if o.Door == "" {
		o.Door = "test"
	}
	r, n := asReader(b)
	return Import(context.Background(), r, n, name, o)
}

func tinyPDF(t *testing.T) []byte {
	t.Helper()
	out, err := ImagesToPDF([]RasterPage{{Image: pngBytes(t, 8, 8), W: 72, H: 72}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAPDFIsReturnedAsItCameAndNeverWaitsForTheGate(t *testing.T) {
	pdf := tinyPDF(t)
	// A gate with no slots free and no queue: anything that waited would be refused.
	g := gate.New(1, 0)
	hold, _ := g.Acquire(context.Background())
	defer hold()
	res, err := importBytes(t, pdf, "report.pdf", ImportOptions{Gate: g})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.PDF, pdf) || res.Converted || res.Name != "report.pdf" {
		t.Fatalf("a PDF came back changed: converted=%v name=%q", res.Converted, res.Name)
	}
}

func TestAnImageBecomesAPathlessPDFNamedForItsSource(t *testing.T) {
	res, err := importBytes(t, pngBytes(t, 40, 30), "photo.png", ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Converted || res.Name != "photo.pdf" || !LooksLikePDF(res.PDF) {
		t.Fatalf("converted=%v name=%q pdf=%v", res.Converted, res.Name, LooksLikePDF(res.PDF))
	}
}

func TestAnImageWithNoNameOrAnUnknownOneStillOpens(t *testing.T) {
	for _, name := range []string{"", "download", "x.bin", "scan?id=3"} {
		if _, err := importBytes(t, pngBytes(t, 10, 10), name, ImportOptions{}); err != nil {
			t.Errorf("an image named %q was refused: %v", name, err)
		}
	}
}

func TestANameThatContradictsTheBytesIsARefusalThatSaysSo(t *testing.T) {
	png := pngBytes(t, 10, 10)
	for _, c := range []struct {
		why  string
		data []byte
		name string
	}{
		{"a PNG named .docx", png, "letter.docx"},
		{"a PNG named .md", png, "notes.md"},
		{"a zip named .png", zipOf(t, map[string]string{"a": "b"}), "pic.png"},
		{"a zip named .doc (a legacy name)", zipOf(t, map[string]string{"word/document.xml": "x", "[Content_Types].xml": "x"}), "old.doc"},
		{"text named .docx", []byte("hello"), "letter.docx"},
		{"text named .xlsx", []byte("a,b"), "sheet.xlsx"},
		{"a docx with no word/ part", zipOf(t, map[string]string{"[Content_Types].xml": "x", "other/x": "y"}), "a.docx"},
		{"a docx that is not a package", zipOf(t, map[string]string{"word/document.xml": "x"}), "a.docx"},
		{"an .odt declaring a spreadsheet", zipOf(t, map[string]string{"mimetype": mimeODF + "spreadsheet"}), "a.odt"},
		{"an .odt declaring nothing", zipOf(t, map[string]string{"content.xml": "x"}), "a.odt"},
		{"a packaged document with no name", zipOf(t, map[string]string{"word/document.xml": "x"}), "report"},
	} {
		_, err := importBytes(t, c.data, c.name, ImportOptions{})
		if !errors.Is(err, ErrExtensionDisagrees) && !errors.Is(err, ErrNotADocument) {
			t.Errorf("%s: err = %v, want a named refusal", c.why, err)
			continue
		}
		if c.why != "a zip named .png" && c.why != "a packaged document with no name" && !errors.Is(err, ErrExtensionDisagrees) {
			t.Errorf("%s: err = %v, want ErrExtensionDisagrees", c.why, err)
		}
	}
}

func TestNothingElseIsADocument(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"empty", nil}, {"mystery.xyz", []byte("hello")}, {"noext", []byte("hello")},
	} {
		if _, err := importBytes(t, c.data, c.name, ImportOptions{}); !errors.Is(err, ErrNotADocument) {
			t.Errorf("%q: err = %v, want ErrNotADocument", c.name, err)
		}
	}
	if _, err := importBytes(t, []byte("a\x00b"), "notes.md", ImportOptions{}); !errors.Is(err, ErrNotText) {
		t.Errorf("binary named .md: err = %v, want ErrNotText", err)
	}
}

func TestMarkdownConvertsNativelyWithAnnouncedFacts(t *testing.T) {
	res, err := importBytes(t, []byte("# Title\n\nHello world.\n"), "README.md", ImportOptions{Lang: "en-GB", LangChosen: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "README.pdf" || !res.Converted {
		t.Fatalf("name=%q converted=%v", res.Name, res.Converted)
	}
	if res.LabelRefused != nil {
		t.Errorf("a chosen language was refused the label: %v", res.LabelRefused)
	}
	// A pre-filled language earns no label.
	res2, err := importBytes(t, []byte("# T\n\nx\n"), "a.md", ImportOptions{Lang: "en-GB"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(res.PDF, res2.PDF) {
		t.Error("the same document with and without a chosen language came out identical; the label is not conditional")
	}
}

func TestTheTypeOverrideNamesStdinMarkdown(t *testing.T) {
	res, err := importBytes(t, []byte("# From stdin\n"), "-", ImportOptions{TypeExt: ".md"})
	if err != nil || !res.Converted {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestNoNativeProducerEverAcquiresTheGate(t *testing.T) {
	g := gate.New(1, 0)
	hold, _ := g.Acquire(context.Background())
	defer hold()
	for name, b := range map[string][]byte{"p.png": pngBytes(t, 8, 8), "n.md": []byte("# x\n")} {
		if _, err := importBytes(t, b, name, ImportOptions{Gate: g}); err != nil {
			t.Errorf("%s waited on or was refused by a full gate: %v", name, err)
		}
	}
	if g.Queued() != 0 || g.Refused() != 0 {
		t.Errorf("native producers touched the queue: queued %d refused %d", g.Queued(), g.Refused())
	}
}

type countingReader struct{ n int }

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) { c.n++; return 0, io.EOF }

func TestAnOversizedSourceIsRefusedBeforeAnyByteIsRead(t *testing.T) {
	cr := &countingReader{}
	_, err := Import(context.Background(), cr, MaxImportBytes+1, "x.pdf", ImportOptions{Door: "test"})
	if !errors.Is(err, ErrImportTooLarge) || cr.n != 0 {
		t.Fatalf("err=%v reads=%d, want ErrImportTooLarge with 0 reads", err, cr.n)
	}
}

// at cap: not refused for size (it fails later, for what it is); cap+1: refused by name.
type fillReader struct{ b byte }

func (f fillReader) ReadAt(p []byte, off int64) (int, error) {
	for i := range p {
		p[i] = f.b
	}
	return len(p), nil
}

func TestTheTextCapRefusesAtCapPlusOneAndAdmitsAtCap(t *testing.T) {
	admitted := errors.New("admitted")
	o := ImportOptions{Door: "test", Admit: func(context.Context) error { return admitted }}
	if _, err := Import(context.Background(), fillReader{'a'}, importTextBytes, "big.md", o); !errors.Is(err, admitted) {
		t.Fatalf("a text source exactly at the cap = %v, want it to reach Admit", err)
	}
	if _, err := Import(context.Background(), fillReader{'a'}, importTextBytes+1, "big.md", o); !errors.Is(err, ErrImportTooLarge) {
		t.Fatalf("a text source one past the cap = %v, want ErrImportTooLarge", err)
	}
}

func TestTheInputCapAdmitsExactlyMaxImportBytes(t *testing.T) {
	admitted := errors.New("admitted")
	o := ImportOptions{Door: "test", Admit: func(context.Context) error { return admitted }}
	// Size at the cap with a head that classifies as text named .md would trip the text cap first, so use
	// an image head: the cap is the input cap, and the sniff then accepts it.
	head := fillThen(pngBytes(t, 4, 4))
	if _, err := Import(context.Background(), head, MaxImportBytes, "x.png", o); errors.Is(err, ErrImportTooLarge) {
		t.Fatalf("exactly MaxImportBytes was refused for size: %v", err)
	}
}

type headReader struct{ head []byte }

func fillThen(head []byte) headReader { return headReader{head} }
func (h headReader) ReadAt(p []byte, off int64) (int, error) {
	for i := range p {
		if j := off + int64(i); j < int64(len(h.head)) {
			p[i] = h.head[j]
		} else {
			p[i] = 0
		}
	}
	return len(p), nil
}

// zipOf builds a stored zip of the given members, in map order made deterministic by sorting the keys.
func zipOf(t *testing.T, members map[string]string) []byte {
	t.Helper()
	var keys []string
	for k := range members {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, k := range keys {
		w, err := zw.Create(k)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(members[k]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func docxLike(extra map[string]string) map[string]string {
	m := map[string]string{"[Content_Types].xml": "<x/>", "word/document.xml": "<d/>"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func inspect(t *testing.T, data []byte, key string) error {
	t.Helper()
	f, ok := FormatForExt(key)
	if !ok {
		t.Fatalf("no row for %q", key)
	}
	r, n := asReader(data)
	return inspectPackage(r, n, f)
}

func TestAnArchiveWithTooManyPartsIsRefusedAtCapPlusOneAndAcceptedAtCap(t *testing.T) {
	build := func(n int) []byte {
		m := docxLike(nil)
		for i := 0; len(m) < n; i++ {
			m[fmt.Sprintf("word/media/%05d", i)] = "x"
		}
		return zipOf(t, m)
	}
	if err := inspect(t, build(importArchiveEntries), "docx"); err != nil {
		t.Fatalf("a package of exactly %d parts was refused: %v", importArchiveEntries, err)
	}
	if err := inspect(t, build(importArchiveEntries+1), "docx"); !errors.Is(err, ErrArchiveRefused) {
		t.Fatalf("a package of %d parts = %v, want ErrArchiveRefused", importArchiveEntries+1, err)
	}
}

// bomb builds a package whose DECLARED sizes are what the caps read; the bytes behind them are tiny.
func bomb(t *testing.T, declared, packed uint64) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"[Content_Types].xml", "word/document.xml"} {
		w, _ := zw.Create(n)
		w.Write([]byte("<x/>"))
	}
	hdr := &zip.FileHeader{Name: "word/big.bin", Method: zip.Store, CompressedSize64: packed, UncompressedSize64: declared}
	w, err := zw.CreateRaw(hdr)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(make([]byte, 16))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAnArchiveThatExpandsPastTheCapsIsRefusedByName(t *testing.T) {
	if err := inspect(t, bomb(t, importDecodedBytes-8, 16<<20), "docx"); errors.Is(err, ErrArchiveRefused) && strings.Contains(err.Error(), "MiB") {
		t.Fatalf("at the decoded cap: %v", err)
	}
	if err := inspect(t, bomb(t, importDecodedBytes+1, 16<<20), "docx"); !errors.Is(err, ErrArchiveRefused) {
		t.Fatalf("one byte past the decoded cap = %v, want ErrArchiveRefused", err)
	}
	// A big expansion with a modest ratio passes; the same size at 1000:1 does not.
	if err := inspect(t, bomb(t, 100<<20, 10<<20), "docx"); err != nil {
		t.Fatalf("100 MiB from 10 MiB (10:1) was refused: %v", err)
	}
	if err := inspect(t, bomb(t, 100<<20, 100<<10), "docx"); !errors.Is(err, ErrArchiveRefused) {
		t.Fatalf("100 MiB from 100 KiB (1000:1) = %v, want ErrArchiveRefused", err)
	}
	// A small package cannot be a bomb whatever its ratio.
	if err := inspect(t, bomb(t, 32<<20, 1<<10), "docx"); err != nil {
		t.Fatalf("a 32 MiB expansion was refused as a bomb: %v", err)
	}
}

func TestAnArchiveNamingAPartTwiceIsRefused(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"[Content_Types].xml", "word/document.xml", "word/document.xml"} {
		w, _ := zw.Create(n)
		w.Write([]byte("x"))
	}
	zw.Close()
	if err := inspect(t, buf.Bytes(), "docx"); !errors.Is(err, ErrArchiveRefused) {
		t.Fatalf("a duplicated part = %v, want ErrArchiveRefused", err)
	}
}

func TestACorruptPackageIsRefusedNotPassedToTheConverter(t *testing.T) {
	if err := inspect(t, []byte("PK\x03\x04 this is not a zip"), "docx"); !errors.Is(err, ErrCorruptDocument) {
		t.Fatalf("a corrupt zip = %v, want ErrCorruptDocument", err)
	}
}

func TestEveryImportIsCountedByDoorAndFormatAndEveryRefusalByCause(t *testing.T) {
	before := ImportCounts()
	door := "countdoor"
	importBytes(t, pngBytes(t, 8, 8), "a.png", ImportOptions{Door: door})
	importBytes(t, tinyPDF(t), "a.pdf", ImportOptions{Door: door})
	importBytes(t, []byte("hello"), "mystery.xyz", ImportOptions{Door: door})
	importBytes(t, pngBytes(t, 8, 8), "a.docx", ImportOptions{Door: door})
	after := ImportCounts()
	for _, k := range []string{
		"import.ok{door=countdoor,format=png}", "import.ok{door=countdoor,format=pdf}",
		"import.refused{door=countdoor,cause=unsupported}", "import.refused{door=countdoor,cause=extension-disagrees}",
	} {
		if after[k] != before[k]+1 {
			t.Errorf("%s moved %d → %d, want +1 (all counters: %v)", k, before[k], after[k], ImportCountKeys())
		}
	}
}

func TestAConversionFailureOfAnImageIsAConversionErrorNamingTheImage(t *testing.T) {
	// A header that sniffs as PNG and decodes as nothing.
	_, err := importBytes(t, []byte("\x89PNG\r\n\x1a\nnot really"), "bad.png", ImportOptions{})
	var ce *ConversionError
	if !errors.As(err, &ce) || !ce.Image {
		t.Fatalf("err = %v, want a ConversionError for an image", err)
	}
}

func TestPDFOnlyRefusesAnythingElse(t *testing.T) {
	if _, err := importBytes(t, pngBytes(t, 8, 8), "a.png", ImportOptions{PDFOnly: true}); !errors.Is(err, ErrNotAPDF) {
		t.Fatalf("a PNG under PDFOnly = %v, want ErrNotAPDF", err)
	}
	if _, err := importBytes(t, tinyPDF(t), "a.pdf", ImportOptions{PDFOnly: true}); err != nil {
		t.Fatalf("a PDF under PDFOnly was refused: %v", err)
	}
}

func TestImportNameIsBasePlusPDF(t *testing.T) {
	for in, want := range map[string]string{
		"a.docx": "a.pdf", "/x/y/report.final.md": "report.final.pdf", "noext": "noext.pdf", "": "document.pdf", ".": "document.pdf",
	} {
		if got := ImportName(in); got != want {
			t.Errorf("ImportName(%q) = %q, want %q", in, got, want)
		}
	}
}
