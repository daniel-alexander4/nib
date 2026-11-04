package mdpdf

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func runTexts(l *layout) string {
	var b strings.Builder
	for _, page := range l.runs {
		for _, r := range page {
			b.WriteString(r.text)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func convertOpts(t *testing.T, md string, o Options) (string, *layout, error) {
	t.Helper()
	_, l, err := convert([]byte(md), nil, nil, o)
	if err != nil {
		return "", nil, err
	}
	return runTexts(l), l, nil
}

// TestAPlaceholderIsFilledWithTextAndNeverMarkup — RelaxTax B4. The value goes in after the document is parsed, so
// *, _, [ and # in a value are characters, not formatting.
func TestAPlaceholderIsFilledWithTextAndNeverMarkup(t *testing.T) {
	vars := map[string]string{"name": "*Ada* _L_ [x](y) # not a heading", "n": "7"}
	got, l, err := convertOpts(t, "Dear {{name}}, you are number {{ n }}.\n", Options{Vars: vars})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"*Ada*", "_L_", "[x](y)", "#", "number", "7"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is not in the page text:\n%s", want, got)
		}
	}
	for _, r := range l.runs[0] {
		if r.sty.font == l.f.bold || r.sty.font == l.f.italic {
			t.Errorf("run %q is bold or italic: the value was read as Markdown", r.text)
		}
	}
	// In a heading, in emphasis, in a list, in a link's text and in a code span.
	for _, md := range []string{"# Hi {{n}}\n", "**{{n}}**\n", "- item {{n}}\n", "[{{n}}](http://x)\n", "`{{n}}`\n"} {
		out, _, err := convertOpts(t, md, Options{Vars: vars})
		if err != nil || !strings.Contains(out, "7") || strings.Contains(out, "{{") {
			t.Errorf("%q: %q, %v, want the value and no braces", md, out, err)
		}
	}
}

func TestAMissingPlaceholderKeyIsRefusedNamingTheKeyAndNotAValue(t *testing.T) {
	_, _, err := convertOpts(t, "Hello {{who}} and {{other}}.\n", Options{Vars: map[string]string{"other": "SECRET-VALUE-9"}})
	var missing *MissingPlaceholderError
	if !errors.As(err, &missing) || missing.Key != "who" {
		t.Fatalf("err = %v, want a MissingPlaceholderError for who", err)
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Errorf("the refusal repeats a value: %q", err)
	}
}

func TestPlaceholdersAreOffUnlessAskedFor(t *testing.T) {
	got, _, err := convertOpts(t, "A {{ literal }} here.\n", Options{})
	if err != nil || !strings.Contains(got, "{{") {
		t.Errorf("without Vars braces must stay as they were: %q, %v", got, err)
	}
	// Code blocks are literal even with Vars, and {{=key}} is the escape.
	got, _, err = convertOpts(t, "```\n{{n}}\n```\n\nWrite {{=n}} to print it.\n", Options{Vars: map[string]string{"n": "7"}})
	if err != nil || !strings.Contains(got, "{{n}}") || strings.Contains(got, "7") {
		t.Errorf("code block / escape: %q, %v", got, err)
	}
}

func TestAPlaceholderBrokenByAnEscapeIsRefusedNotLeftLiteral(t *testing.T) {
	// `\_` is a Markdown escape: the parser splits the text there, so no single text node holds {{first_name}} even
	// though the page text reads as one. That must be an error naming the key, not a literal in the output.
	_, _, err := convertOpts(t, "Hello {{first\\_name}}.\n", Options{Vars: map[string]string{"first_name": "Ada"}})
	var split *SplitPlaceholderError
	var bad *MalformedPlaceholderError
	if !errors.As(err, &split) && !errors.As(err, &bad) {
		t.Fatalf("err = %v, want the placeholder refused as split or malformed, not left as literal text", err)
	}
	// And braces that merely look like a placeholder are refused with placeholders on.
	for _, md := range []string{"A {{ two words }} here\n", "B {{na*me*}} here\n", "C {{}} here\n"} {
		_, _, err := convertOpts(t, md, Options{Vars: map[string]string{"name": "x"}})
		// {{na*me*}} reads as {{name}} once the emphasis is parsed away, so it is the split refusal; the others malformed.
		if !errors.As(err, &bad) && !errors.As(err, &split) {
			t.Errorf("%q: err = %v, want a malformed or split placeholder refusal", md, err)
		}
	}
}

func TestOptionsChooseThePageAndTheMargin(t *testing.T) {
	pdf, l, err := convert([]byte("# Title\n\nSome text here.\n"), nil, nil, Options{PageSize: "A4", Margin: 36})
	if err != nil {
		t.Fatal(err)
	}
	dims, err := api.PageDims(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil || len(dims) == 0 || int(dims[0].Width) != 595 || int(dims[0].Height) != 842 {
		t.Fatalf("A4 dims = %v, %v, want 595x842", dims, err)
	}
	if got := l.runs[0][0].x; got != 36 {
		t.Errorf("first run starts at x=%v, want the 36pt margin", got)
	}
	if got := l.runs[0][0].y; got > 842-36 || got < 842-36-40 {
		t.Errorf("first run at y=%v, want just under the 36pt top margin (%v)", got, 842.0-36)
	}
	// The default is Letter with one-inch margins, unchanged.
	pdf, l, _ = convert([]byte("Hi\n"), nil, nil, Options{})
	dims, _ = api.PageDims(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if int(dims[0].Width) != 612 || int(dims[0].Height) != 792 || l.runs[0][0].x != 72 {
		t.Errorf("default page %v x=%v, want Letter and 72", dims, l.runs[0][0].x)
	}
	legal, _, _ := convert([]byte("Hi\n"), nil, nil, Options{PageSize: "legal"})
	dims, _ = api.PageDims(context.Background(), bytes.NewReader(legal), model.NewDefaultConfiguration())
	if int(dims[0].Height) != 1008 {
		t.Errorf("legal = %v", dims)
	}
}

func TestAnUnusableOptionIsRefused(t *testing.T) {
	for _, o := range []Options{{PageSize: "tabloid"}, {Margin: -1}, {Margin: 300}, {PageSize: "a4", Margin: 250}} {
		if _, _, err := convert([]byte("Hi\n"), nil, nil, o); err == nil {
			t.Errorf("%+v was accepted", o)
		}
		if o.Validate() == nil {
			t.Errorf("%+v validates", o)
		}
	}
}

// TestABackslashEscapeIsThePunctuationAndACodeSpanKeepsItsBackslash — P01.S03b. A CSV cell reaches the renderer as
// escaped Markdown, and the parser leaves the backslash in its text segment: only the renderer resolves it.
func TestABackslashEscapeIsThePunctuationAndACodeSpanKeepsItsBackslash(t *testing.T) {
	got, _, err := convertOpts(t, "a \\*star\\* and \\_under\\_ and \\[x\\] and \\| bar\n", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "\\") {
		t.Errorf("a CommonMark escape printed its backslash: %q", got)
	}
	for _, want := range []string{"*star*", "_under_", "[x]", "|"} {
		if !strings.Contains(strings.ReplaceAll(got, " ", ""), want) {
			t.Errorf("%q missing from %q", want, got)
		}
	}
	code, _, err := convertOpts(t, "`C:\\temp\\*`\n", Options{})
	if err != nil || !strings.Contains(code, "C:\\temp\\*") {
		t.Errorf("a code span's backslashes are literal: %q, %v", code, err)
	}
	// In a table cell too: the shape the CSV door writes.
	cell, _, err := convertOpts(t, "| a |\n| --- |\n| \\*x\\* \\| y |\n", Options{})
	if err != nil || strings.Contains(cell, "\\") {
		t.Errorf("a table cell printed a backslash: %q, %v", cell, err)
	}
}

// TestAConvertedDocumentEndsWithItsContext — /pending 940 finding 9. Options.Context ends the conversion with an
// error and no document: already done at the start, and done while the layout is inside a long code block or table.
func TestAConvertedDocumentEndsWithItsContext(t *testing.T) {
	done, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ConvertWithFacesAndOptions([]byte("# hi\n"), nil, nil, Options{Context: done}); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context = %v, want context.Canceled", err)
	}
	live, stop := context.WithCancel(context.Background())
	defer stop()
	pdf, err := ConvertWithFacesAndOptions([]byte("# hi\n"), nil, nil, Options{Context: live})
	if err != nil || len(pdf) == 0 {
		t.Fatalf("a live context = %v, want a document", err)
	}
	// Mid-layout: the context turns done after the layout has begun; the loop that lays out lines must notice.
	mid := &flipAfter{Context: context.Background(), n: 1000}
	code := "```\n" + strings.Repeat("a line of code\n", 4000) + "```\n"
	// The error is the context's own and unwrapped: the PDF writer wraps its ("render pdf: ..."), so a wrapped one
	// means the layout ran to the end and only the writer noticed.
	if _, err := ConvertWithFacesAndOptions([]byte(code), nil, nil, Options{Context: mid}); err != context.Canceled {
		t.Errorf("a context that ends inside a code block = %v, want context.Canceled", err)
	}
	if got := mid.asked; got > 1000+10 {
		t.Errorf("the code block asked its context %d times: it kept laying out lines after the context ended", got)
	}
	mid = &flipAfter{Context: context.Background(), n: 1000}
	table := "| a | b |\n| - | - |\n" + strings.Repeat("| x | y |\n", 3000)
	if _, err := ConvertWithFacesAndOptions([]byte(table), nil, nil, Options{Context: mid}); err != context.Canceled {
		t.Errorf("a context that ends inside a table = %v, want context.Canceled", err)
	}
	if got := mid.asked; got > 1000+10 {
		t.Errorf("the table asked its context %d times: it kept reading rows after the context ended", got)
	}
	mid = &flipAfter{Context: context.Background(), n: 1000}
	paras := strings.Repeat("A paragraph of words.\n\n", 3000)
	if _, err := ConvertWithFacesAndOptions([]byte(paras), nil, nil, Options{Context: mid}); err != context.Canceled {
		t.Errorf("a context that ends between paragraphs = %v, want context.Canceled", err)
	}
	if got := mid.asked; got > 1000+10 {
		t.Errorf("the walk asked its context %d times: it kept laying out blocks after the context ended", got)
	}
}

// flipAfter is a context that reports itself done after n questions and counts them.
type flipAfter struct {
	context.Context
	n     int
	asked int
}

func (c *flipAfter) Err() error {
	c.asked++
	if c.asked > c.n {
		return context.Canceled
	}
	return nil
}
