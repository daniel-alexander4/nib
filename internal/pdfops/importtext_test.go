package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"

	"nib/internal/gate"
	"nib/mdpdf"
)

func importText(t *testing.T, name string, data []byte, o ImportOptions) (*ImportResult, error) {
	t.Helper()
	r, n := BytesSource(data)
	return Import(context.Background(), r, n, name, o)
}

func csvOf(rows, cols int) []byte {
	var b strings.Builder
	for c := 0; c < cols; c++ {
		fmt.Fprintf(&b, "h%d,", c)
	}
	b.Reset()
	for c := 0; c < cols; c++ {
		if c > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "h%d", c)
	}
	b.WriteByte('\n')
	for r := 1; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if c > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "r%dc%d", r, c)
		}
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func TestTextAndCSVAreNativeAndNeedNoTool(t *testing.T) {
	for _, k := range []string{"txt", "csv"} {
		f, _ := FormatForExt(k)
		if f.Tool != ToolNone {
			t.Errorf("%s names tool %v, but D10 makes it native", k, f.Tool)
		}
	}
}

func TestPlainTextOpensAndReadsBack(t *testing.T) {
	src := "first line\n\tindented with a tab\nlast line has ```backticks``` in it\n"
	res, err := importText(t, "notes.txt", []byte(src), ImportOptions{Door: "path"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Converted || res.Name != "notes.pdf" {
		t.Fatalf("result = %+v", res)
	}
	txt := pdfTextOf(t, res.PDF)
	for _, want := range []string{"firstline", "indentedwithatab", "lastlinehas", "```backticks```"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the page lacks %q: %q", want, txt)
		}
	}
}

func TestACSVReadsBackAsATableAcrossDelimiters(t *testing.T) {
	for name, src := range map[string]string{
		"comma":     "name,qty\nbolt,4\nnut,9\n",
		"semicolon": "name;qty\nbolt;4\nnut;9\n",
		"tab":       "name\tqty\nbolt\t4\nnut\t9\n",
		"quoted":    "\"name\",\"qty\"\n\"bolt, large\",4\nnut,9\n",
	} {
		t.Run(name, func(t *testing.T) {
			res, err := importText(t, "parts.csv", []byte(src), ImportOptions{Door: "path"})
			if err != nil {
				t.Fatal(err)
			}
			txt := pdfTextOf(t, res.PDF)
			for _, want := range []string{"name", "qty", "bolt", "nut"} {
				if !strings.Contains(txt, want) {
					t.Errorf("%s: the page lacks %q: %q", name, want, txt)
				}
			}
		})
	}
}

func TestCSVCellsAreLiteralNotMarkdown(t *testing.T) {
	md, err := csvToMarkdown("a,b\n*stars* and _under_,[link](http://x) | pipe\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md, "*stars*") || strings.Contains(md, "[link]") {
		t.Fatalf("a cell's Markdown syntax went through unescaped: %q", md)
	}
	res, err := importText(t, "x.csv", []byte("a,b\n*stars* and _under_,[link](http://x) | pipe\n"), ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	txt := pdfTextOf(t, res.PDF)
	for _, want := range []string{"*stars*", "_under_", "[link](http://x)", "|pipe"} {
		if !strings.Contains(txt, want) {
			t.Errorf("literal %q lost from %q", want, txt)
		}
	}
}

func TestATextFileWhoseLinesLookLikeAFenceStaysText(t *testing.T) {
	md, err := textToMarkdown("```\nnot a fence end\n````\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(md, "`````\n") || !strings.HasSuffix(md, "`````\n") {
		t.Fatalf("the fence must outrun the longest backtick run: %q", md)
	}
}

func TestTextEncodings(t *testing.T) {
	u16 := func(s string, be bool) []byte {
		out := []byte{0xFF, 0xFE}
		if be {
			out = []byte{0xFE, 0xFF}
		}
		for _, c := range utf16.Encode([]rune(s)) {
			if be {
				out = append(out, byte(c>>8), byte(c))
			} else {
				out = append(out, byte(c), byte(c>>8))
			}
		}
		return out
	}
	for name, b := range map[string][]byte{
		"utf8":         []byte("café\n"),
		"utf8bom":      append([]byte{0xEF, 0xBB, 0xBF}, "café\n"...),
		"utf16le":      u16("café\n", false),
		"utf16be":      u16("café\n", true),
		"windows-1252": {'c', 'a', 'f', 0xE9, '\n'},
	} {
		if got := strings.TrimSpace(decodeText(b)); got != "café" {
			t.Errorf("%s decoded to %q", name, got)
		}
	}
}

func TestCSVRaggedRowsArePadded(t *testing.T) {
	md, err := csvToMarkdown("a,b,c\n1\n1,2,3,4\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimSpace(md), "\n") {
		if strings.Count(l, "|") != 5 {
			t.Errorf("a ragged row is not padded to the widest (4 columns): %q", l)
		}
	}
}

func TestEmptyCSVAndEmptyTextStillOpen(t *testing.T) {
	for _, n := range []string{"e.csv", "e.txt"} {
		if _, err := importText(t, n, nil, ImportOptions{}); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
}

// Each cap reads its named constant, accepts at the cap (the last row present) and refuses at cap + 1 by sentinel.
func TestTextCapsRefuseByName(t *testing.T) {
	t.Run("csv rows", func(t *testing.T) {
		if _, err := csvToMarkdown(string(csvOf(importCSVRows, 3))); err != nil {
			t.Fatalf("at the cap: %v", err)
		}
		_, err := csvToMarkdown(string(csvOf(importCSVRows+1, 3)))
		if !errors.Is(err, ErrCSVTooLong) {
			t.Fatalf("cap+1 = %v, want ErrCSVTooLong", err)
		}
	})
	t.Run("csv columns", func(t *testing.T) {
		if _, err := csvToMarkdown(string(csvOf(2, csvMaxColumns))); err != nil {
			t.Fatalf("at the cap: %v", err)
		}
		_, err := csvToMarkdown(string(csvOf(2, csvMaxColumns+1)))
		if !errors.Is(err, ErrCSVTooWide) {
			t.Fatalf("cap+1 = %v, want ErrCSVTooWide", err)
		}
	})
	t.Run("csv cells", func(t *testing.T) {
		atCap := importCSVCells / csvMaxColumns
		if _, err := csvToMarkdown(string(csvOf(atCap, csvMaxColumns))); err != nil {
			t.Fatalf("at the cap: %v", err)
		}
		_, err := csvToMarkdown(string(csvOf(atCap+1, csvMaxColumns)))
		if !errors.Is(err, ErrCSVTooManyCells) {
			t.Fatalf("cap+1 = %v, want ErrCSVTooManyCells", err)
		}
	})
	t.Run("text lines", func(t *testing.T) {
		if _, err := textToMarkdown(strings.Repeat("x\n", importTextLines)); err != nil {
			t.Fatalf("at the cap: %v", err)
		}
		_, err := textToMarkdown(strings.Repeat("x\n", importTextLines+1))
		if !errors.Is(err, ErrTextTooLong) {
			t.Fatalf("cap+1 = %v, want ErrTextTooLong", err)
		}
	})
	t.Run("text bytes", func(t *testing.T) {
		big := bytes.Repeat([]byte("x"), importTextBytes+1)
		_, err := importText(t, "big.txt", big, ImportOptions{})
		if !errors.Is(err, ErrImportTooLarge) {
			t.Fatalf("cap+1 = %v, want ErrImportTooLarge", err)
		}
	})
}

func TestTheTableLimitsAreTheRenderers(t *testing.T) {
	// csvMaxColumns restates mdpdf's maxTableCols: one more column is refused by the renderer too.
	md, _ := csvToMarkdown(string(csvOf(2, csvMaxColumns)))
	if _, err := mdpdf.Convert([]byte(md)); err != nil {
		t.Fatalf("a table at csvMaxColumns must lay out: %v", err)
	}
	wide := "|" + strings.Repeat(" a |", csvMaxColumns+1) + "\n|" + strings.Repeat(" --- |", csvMaxColumns+1) + "\n|" + strings.Repeat(" b |", csvMaxColumns+1) + "\n"
	if _, err := mdpdf.Convert([]byte(wide)); !errors.Is(err, mdpdf.ErrTableTooWide) {
		t.Fatalf("mdpdf's own limit moved: %v", err)
	}
	if csvRowsPerTable >= 5000 {
		t.Fatalf("csvRowsPerTable %d must stay under mdpdf's 5000-row table limit", csvRowsPerTable)
	}
}

func TestACSVLongerThanOneTableSplitsAndRepeatsTheHeader(t *testing.T) {
	md, err := csvToMarkdown(string(csvOf(csvRowsPerTable*2+10, 2)))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(md, "| h0 | h1 |\n"); n != 3 {
		t.Fatalf("header appears %d times, want 3 tables", n)
	}
	if !strings.Contains(md, fmt.Sprintf("| r%dc0 |", csvRowsPerTable*2+9)) {
		t.Fatal("the last row is missing")
	}
}

func TestTextImportHonoursAPageCap(t *testing.T) {
	// Lines that each take a whole page: the page cap, not the line cap, binds.
	long := strings.Repeat("word ", 20000)
	_, err := importText(t, "wide.txt", []byte(strings.Repeat(long+"\n", 3)), ImportOptions{})
	if err != nil && !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("unexpected refusal: %v", err)
	}
}

// TestMeasureTextImports prints wall time and peak heap for the text producers at the sizes D14's caps were tuned
// on. It is a measurement and not a test: it runs only with NIB_MEASURE=1.
func TestMeasureTextImports(t *testing.T) {
	if os.Getenv("NIB_MEASURE") == "" {
		t.Skip("set NIB_MEASURE=1 to measure")
	}
	run := func(label, name string, data []byte) {
		runtime.GC()
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		start := time.Now()
		res, err := importText(t, name, data, ImportOptions{})
		runtime.ReadMemStats(&m1)
		pages := -1
		if res != nil {
			pages, _ = PageCount(res.PDF)
		}
		t.Logf("%-28s %8d B in  %6.1fs  pages=%d  totalAlloc=%d MiB  err=%v", label, len(data), time.Since(start).Seconds(), pages, (m1.TotalAlloc-m0.TotalAlloc)>>20, err)
	}
	for _, n := range []int{10000} {
		run(fmt.Sprintf("csv %d rows x 6", n), "m.csv", csvOf(n, 6))
	}
	for _, n := range []int{40000} {
		run(fmt.Sprintf("txt %d lines", n), "m.txt", []byte(strings.Repeat("The quick brown fox jumps over the lazy dog, again and again.\n", n)))
	}
}

// pdfTextOf is every page's text, runs joined by a space — what the page prints, read the way the window reads it.
func pdfTextOf(t *testing.T, pdf []byte) string {
	return strings.Join(strings.Fields(pdfTextRaw(t, pdf)), "")
}

func pdfTextRaw(t *testing.T, pdf []byte) string {
	t.Helper()
	pages, err := DocumentText(pdf)
	if err != nil {
		t.Fatalf("read text: %v", err)
	}
	var b strings.Builder
	for _, p := range pages {
		for _, r := range p.Runs {
			b.WriteString(r.Text)
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func TestOneVeryLongLineIsCountedByTheRowsItWrapsTo(t *testing.T) {
	_, err := textToMarkdown(strings.Repeat("x", textRowRunes*importTextLines+1))
	if !errors.Is(err, ErrTextTooLong) {
		t.Fatalf("a single line that lays out as more than %d rows: %v, want ErrTextTooLong", importTextLines, err)
	}
	if _, err := textToMarkdown(strings.Repeat("x", textRowRunes*importTextLines)); err != nil {
		t.Fatalf("at the cap: %v", err)
	}
}

func TestTheDelimiterIsSniffedPastATitleLineWithNone(t *testing.T) {
	if d := sniffDelimiter("Quarterly report\nname\tqty\nbolt\t4\n"); d != '\t' {
		t.Fatalf("delimiter = %q, want tab", d)
	}
}

func TestASourceSmallerThanItsDeclaredSizeIsNamedNotPadded(t *testing.T) {
	r, _ := BytesSource([]byte("a,b\n1,2\n"))
	_, err := Import(context.Background(), r, 4096, "x.csv", ImportOptions{})
	if !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("err = %v, want ErrSourceChanged", err)
	}
	if _, err := Import(context.Background(), r, -1, "x.csv", ImportOptions{}); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("a negative size: %v", err)
	}
}

// errAfter is a context that reports itself cancelled after n questions and counts every one asked.
type errAfter struct {
	context.Context
	n     int64
	asked atomic.Int64
}

func (c *errAfter) Err() error {
	if c.asked.Add(1) > c.n {
		return context.Canceled
	}
	return nil
}

// TestTextImportEndsWithItsContextInsideTheLayout — /pending 940 finding 9. textImport used context.Background(), so a
// cancelled request laid out the whole document anyway. The context here turns cancelled after 1,000 questions, deep
// inside a 30,000-line code block; the layout must stop asking within a few more, which it cannot do if the loop
// that lays out the lines never asks.
func TestTextImportEndsWithItsContextInsideTheLayout(t *testing.T) {
	data := []byte(strings.Repeat("The quick brown fox jumps over the lazy dog.\n", 30000))
	ctx := &errAfter{Context: context.Background(), n: 1000}
	r, n := BytesSource(data)
	_, err := Import(ctx, r, n, "big.txt", ImportOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a text import whose context ended mid-layout = %v, want context.Canceled", err)
	}
	if got := ctx.asked.Load(); got > 1000+10 {
		t.Errorf("the layout asked its context %d times: it kept going for %d lines after the context ended", got, got-1000)
	}
}

// TestTextImportTakesNoGateSlot — /pending 940 finding 9, ADR-185. The gate bounds outside tools; text and CSV are laid
// out in-process and must not wait behind a full gate nor count towards its queue.
func TestTextImportTakesNoGateSlot(t *testing.T) {
	g := gate.New(1, 0)
	release, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, name := range []string{"a.txt", "a.csv"} {
		res, err := importText(t, name, []byte("a,b\n1,2\n"), ImportOptions{Gate: g})
		if err != nil || res == nil || len(res.PDF) == 0 {
			t.Fatalf("%s through a full gate = %v, want a PDF", name, err)
		}
	}
}
