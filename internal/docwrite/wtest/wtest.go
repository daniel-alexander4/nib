// Package wtest holds the hand-built models the writers' tests share (PLAN-import-export W22): data and one oracle
// helper, no PDF. Committed code, never a binary fixture.
package wtest

import (
	"bytes"
	"context"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/docmodel"
	"nib/internal/ziputil"
)

// onePixelPNG is a PNG a decoder opens: the writers are shown to carry a picture a reader can draw, not four bytes of magic.
func onePixelPNG() []byte {
	var b bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	im.Set(0, 0, color.RGBA{R: 200, A: 255})
	if err := png.Encode(&b, im); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func sp(t string) []docmodel.Span { return []docmodel.Span{{Text: t}} }

// Hostile is a model whose every text value tries to be structure.
func Hostile() docmodel.Document {
	return docmodel.Document{
		Meta: docmodel.Meta{Title: `</title><script>alert(1)</script>`, Lang: `en"><script>`, Dir: "ltr"},
		Blocks: []docmodel.Block{
			{Kind: docmodel.Heading, Level: 1, Page: 1, Spans: sp("# Title with *stars* and <b>tags</b>")},
			{Kind: docmodel.Paragraph, Page: 1, Spans: []docmodel.Span{
				{Text: "plain "}, {Text: "bold", Bold: true}, {Text: " and "}, {Text: "italic", Italic: true},
				{Text: " then a "}, {Text: "link", Link: "https://example.com/a(b)c?x=1&y=2"},
				{Text: " and "}, {Text: "bad", Link: "javascript:alert(1)"},
				{Text: " <script>alert(2)</script> \"onerror=\"x\" [x](javascript:alert(3)) | pipe \\ back`tick"},
			}},
			{Kind: docmodel.Paragraph, Page: 1, Spans: sp("- not a bullet")},
			{Kind: docmodel.Paragraph, Page: 1, Spans: sp("1. not a list")},
			{Kind: docmodel.Paragraph, Page: 1, Spans: sp("ctl\x00\x01\x08chars\u0085 and ￾ gone")},
			{Kind: docmodel.ListItem, Level: 1, List: 1, Marker: "1.", Page: 2, Spans: sp("first")},
			{Kind: docmodel.ListItem, Level: 2, List: 1, Marker: "•", Page: 2, Spans: sp("nested")},
			{Kind: docmodel.ListItem, Level: 1, List: 1, Marker: "2.", Page: 2, Spans: sp("second")},
			{Kind: docmodel.Table, Page: 2, HeaderRows: 1, Rows: [][]docmodel.Cell{
				{{Paragraphs: [][]docmodel.Span{sp("Name")}, ColSpan: 1, RowSpan: 1, Header: true}, {Paragraphs: [][]docmodel.Span{sp("Value | v")}, ColSpan: 1, RowSpan: 1, Header: true}},
				{{Paragraphs: [][]docmodel.Span{sp("a"), sp("a2")}, ColSpan: 1, RowSpan: 1}, {Paragraphs: [][]docmodel.Span{sp("<img src=x onerror=1>")}, ColSpan: 1, RowSpan: 1}},
			}},
			{Kind: docmodel.Figure, Page: 3, Image: &docmodel.Image{Alt: `Scanned page 3 "quoted" <b>`, Placeholder: "mask"}},
			{Kind: docmodel.Figure, Page: 3, Image: &docmodel.Image{Alt: "a dot", Ext: "png", Data: onePixelPNG(), Width: 1, Height: 1}},
			{Kind: docmodel.Paragraph, Page: 3, Dir: "rtl", Spans: sp("שלום עולם")},
		},
	}
}

// Words is the model's text as the words a reader of the output should find, in order, from the model's own spans —
// the manifest the writers are judged against (D34: a writer that drops a block must not pass).
func Words(d docmodel.Document) []string {
	var w []string
	add := func(s string) { w = append(w, strings.Fields(docmodel.Clean(s))...) }
	for _, b := range d.Blocks {
		for _, s := range b.Spans {
			add(s.Text)
		}
		for _, r := range b.Rows {
			for _, c := range r {
				for _, p := range c.Paragraphs {
					for _, s := range p {
						add(s.Text)
					}
				}
			}
		}
	}
	return w
}

// Package is a written package opened for a test: its members in archive order, and the first bytes as written.
type Package struct {
	Order []string
	Files map[string][]byte
	Raw   []byte
}

// Open reads a written zip through the one door, with caps generous enough for a fixture.
func Open(t testing.TB, data []byte) Package {
	t.Helper()
	a, err := ziputil.Open(bytes.NewReader(data), int64(len(data)), ziputil.Limits{Entries: 1000, DecodedBytes: 64 << 20, RatioFloor: 1 << 20, RatioMax: 1000})
	if err != nil {
		t.Fatalf("the package does not open: %v", err)
	}
	p := Package{Files: map[string][]byte{}, Raw: data}
	for _, f := range a.Files() {
		b, err := a.Read(f, 64<<20)
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		p.Order = append(p.Order, f.Name)
		p.Files[f.Name] = b
	}
	return p
}

// WellFormed parses every .xml, .rels, .xhtml, .opf member to its end, strictly: a writer that emits a bare `&` or an unclosed tag fails here.
func (p Package) WellFormed(t testing.TB) {
	t.Helper()
	for _, n := range p.Order {
		if !strings.HasSuffix(n, ".xml") && !strings.HasSuffix(n, ".rels") && !strings.HasSuffix(n, ".xhtml") && !strings.HasSuffix(n, ".opf") {
			continue
		}
		d := xml.NewDecoder(bytes.NewReader(p.Files[n]))
		for {
			_, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("%s is not well-formed XML: %v", n, err)
				break
			}
		}
	}
}

// LibreOfficeText converts a written package to plain text with `soffice` and returns it. It skips, saying so by name,
// where soffice is absent: the confirmatory run is "not run", never the writer (D22). The profile and the output live in
// t.TempDir, and nothing is fetched (the package holds no external reference).
func LibreOfficeText(t *testing.T, name string, data []byte) string {
	t.Helper()
	bin, err := exec.LookPath("soffice")
	if err != nil {
		t.Skip("soffice is not installed: the LibreOffice confirmatory run is NOT RUN (the XML oracle ran)")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, name)
	if err := os.WriteFile(in, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-env:UserInstallation=file://"+filepath.Join(dir, "profile"), "--headless", "--convert-to", "txt:Text", "--outdir", filepath.Join(dir, "out"), in)
	// The environment is an allowlist, as pdfops.toolEnv's is (a writer's tests cannot import pdfops): no proxy, no
	// inherited secrets, a throwaway HOME.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "LANG=C.UTF-8", "TMPDIR=" + dir}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("soffice: %v\n%s", err, out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "out", strings.TrimSuffix(name, filepath.Ext(name))+".txt"))
	if err != nil {
		t.Fatalf("soffice wrote no text: %v", err)
	}
	return string(b)
}

// ReadBack checks that LibreOffice's text holds every word of the model in order, and reports how many words it holds
// beyond them: the rendered list labels and the placeholder line a picture-less figure becomes. The count is the caller's
// to pin, so a dropped or reordered word and an invented one both fail.
func ReadBack(got string, want []string) (extra int, ok bool) {
	fields := strings.Fields(strings.TrimPrefix(got, "\ufeff"))
	i := 0
	for _, g := range fields {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return len(fields) - len(want), i == len(want)
}

// EpubCheck runs the W3C validator over a written package and fails on any error it reports. It skips, saying so by
// name, where `epubcheck` is absent: the confirmatory run is "not run", never the writer (D22). The package is written
// to t.TempDir and nothing is fetched.
func EpubCheck(t *testing.T, data []byte) {
	t.Helper()
	bin, err := exec.LookPath("epubcheck")
	if err != nil {
		t.Skip("epubcheck is not installed: the EPUB validator run is NOT RUN (the XML oracle ran)")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "book.epub")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, in)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "LANG=C.UTF-8", "TMPDIR=" + dir}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("epubcheck: %v\n%s", err, out)
	}
}

// DecodesAsPNG fails unless the named member is a PNG a decoder opens: "the picture is kept" is shown with a picture.
func (p Package) DecodesAsPNG(t testing.TB, name string) {
	t.Helper()
	b, ok := p.Files[name]
	if !ok {
		t.Fatalf("no member %q in %v", name, p.Order)
	}
	if _, err := png.Decode(bytes.NewReader(b)); err != nil {
		t.Errorf("%s does not decode as a PNG: %v", name, err)
	}
}
