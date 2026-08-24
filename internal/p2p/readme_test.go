package p2p

import (
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
	"nib/mdpdf"
)

// TestRenderReadmeOnePage asserts the shape of the generated document.
//
// **It cannot fail on overflow, and that is why the two tests below exist.**
// RenderReadme hardcodes `"pages": {"1": …}`, so the page count is a constant of
// the spec rather than a function of the text: a body twice too long still yields
// exactly one page. Kept because "CreateFromJSON returned a single-page document"
// is a real if small property; named here so nobody reads it as a fit check.
func TestRenderReadmeOnePage(t *testing.T) {
	pdf, err := RenderReadme()
	if err != nil {
		t.Fatal(err)
	}
	n, err := pdfops.PageCount(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("readme page count = %d, want 1", n)
	}
}

func TestAppendReadmeAddsTrailingPage(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	before, err := pdfops.PageCount(base)
	if err != nil {
		t.Fatal(err)
	}
	out, err := AppendReadme(base)
	if err != nil {
		t.Fatal(err)
	}
	after, err := pdfops.PageCount(out)
	if err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Errorf("page count %d -> %d, want +1", before, after)
	}
}

// The readme body must state every trust claim. This is the Go-source half; the
// rendered half is TestRenderedReadmeStatesEveryTrustClaim, and both are needed:
// this one cannot see the page, and that one cannot tell a missing claim from a
// claim the wrapper split.
func TestReadmeContainsTrustClaims(t *testing.T) {
	if len(trustClaims) < 6 {
		t.Fatalf("trustClaims has %d entries; an emptied list makes every loop over it "+
			"pass over nothing", len(trustClaims))
	}
	body := readmeBody()
	for _, c := range trustClaims {
		if !strings.Contains(body, c) {
			t.Errorf("readme missing trust claim %q", c)
		}
	}
}

// litRE matches one PDF string literal drawn with Tj. RenderReadme emits one per
// wrapped line, so these are the page's text runs in drawing order.
var litRE = regexp.MustCompile(`\(((?:[^()\\]|\\.)*)\) Tj`)

// renderedReadme returns the text actually drawn on the readme page, flattened to
// single-spaced words.
//
// **Extraction is via pdfcpu's content stream, not pdftotext and not
// digitorus/pdf**, and both alternatives were measured before this one was chosen:
//   - digitorus/pdf returns one run PER GLYPH with W=0 and spaces expressed as
//     positioning rather than glyphs, so a naive join yields "Aboutthisco-signed"
//     and every phrase assertion below would report false against a CORRECT page —
//     a negative clause that passes before the work is done.
//   - pdftotext is correct but is gated on exec.LookPath + t.Skip everywhere this
//     repo uses it, and a skip is a green. The clause this serves is the one the
//     plan calls load-bearing; it must not be dischargeable by not running.
//
// Flattening matters: the page is drawn one wrapped line per run, so a phrase that
// straddles a line break exists in neither run. Joining the runs and collapsing
// whitespace is what makes a claim assertion a statement about the PAGE rather
// than about where the wrapper happened to break.
func renderedReadme(t *testing.T) string {
	t.Helper()
	pdf, err := RenderReadme()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := api.ExtractContent(bytes.NewReader(pdf), []string{"1"},
		func(r io.Reader, _ int) error { _, e := io.Copy(&buf, r); return e },
		model.NewDefaultConfiguration()); err != nil {
		t.Fatal(err)
	}
	var runs []string
	for _, m := range litRE.FindAllStringSubmatch(buf.String(), -1) {
		// Undo the PDF string escaping and the WinAnsi single-byte encoding, or a
		// phrase containing a bracket arrives as `\(QES\)` and a phrase containing an
		// em dash arrives with a raw 0x97. Both would make a POSITIVE assertion fail
		// noisily — but they would make the load-bearing NEGATIVE assertion pass
		// quietly, which is the direction that matters.
		runs = append(runs, winAnsiToUTF8(pdfUnescape(m[1])))
	}
	flat := strings.Join(strings.Fields(strings.Join(runs, " ")), " ")
	// Setup assertion, and it is not ceremony: every assertion built on this is a
	// substring check, and an extractor that silently returned nothing would make
	// each of them pass. Pin something the page certainly says.
	if !strings.Contains(flat, readmeTitle) {
		t.Fatalf("extraction returned %d runs / %d chars and does not contain the page title %q — "+
			"the extractor is not reading the page, so nothing below this line means anything",
			len(runs), len(flat), readmeTitle)
	}
	return flat
}

// pdfUnescape undoes the PDF literal escaping pdfcpu applies to `(`, `)` and `\`.
func pdfUnescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// winAnsiToUTF8 maps the CP1252 bytes pdfcpu emits back to runes. Only the
// characters the readme actually uses are mapped; everything else is Latin-1,
// which is byte-identical.
func winAnsiToUTF8(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case 0x97:
			b.WriteRune('\u2014') // em dash
		case 0x96:
			b.WriteRune('\u2013') // en dash
		case 0x92:
			b.WriteRune('\u2019') // right single quote
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// The RENDERED page must no longer describe the ceremony as exactly two people.
//
// The negative half is the load-bearing one: the positive half below is a
// substring check over text this package wrote and could not realistically fail.
func TestRenderedReadmeNoLongerSaysTwoParty(t *testing.T) {
	flat := renderedReadme(t)
	for _, gone := range []string{"two people", "In two-party signing"} {
		if strings.Contains(flat, gone) {
			t.Errorf("the rendered readme still says %q — the page describes a ceremony of "+
				"exactly two, and P07 makes it N", gone)
		}
	}
}

func TestRenderedReadmeStatesEveryTrustClaim(t *testing.T) {
	flat := renderedReadme(t)
	for _, c := range trustClaims {
		if !strings.Contains(flat, c) {
			t.Errorf("the rendered readme does not carry trust claim %q — it is in the Go "+
				"source but not on the page", c)
		}
	}
}

// The body must stay clear of the signature blocks stacked on the same page.
//
// **This is the only check in any tier that can see the defect**, and the two
// instruments a reader would reach for first are both blind:
//   - The RENDERED position saturates. pdfcpu clamps what it emits — a requested y
//     of -50 and of -5000 both land at 421.0, A4's vertical centre — so an
//     overflowing page draws its surplus lines stacked on ONE baseline and forty
//     overflow lines look identical to four hundred.
//   - The extracted TEXT is unchanged. Every overflowing line is still in the
//     content stream, so a text assertion passes over text nobody can read.
//
// The appearance image is an opaque white fill (web/app.js renderAttestation), so a
// collision ERASES the trust text rather than merely overlapping it.
func TestReadmeBodyClearsTheAttestationStack(t *testing.T) {
	lines := readmeLines()
	if len(lines) == 0 {
		t.Fatal("readmeLines() is empty, so the clearance below is vacuous")
	}
	last, floor := readmeLastBaseline(lines), readmeFloor()
	if last <= floor {
		t.Errorf("the last body baseline is %.0f and the signature-block stack starts at %.0f: "+
			"%d lines is %.0fpt too many. The block appearance is an opaque fill, so it does not "+
			"overlap the trust text, it erases it",
			last, floor, len(lines), floor-last)
	}
}

// And the door refuses, so an unreadable page cannot be produced at all.
func TestRenderReadmeRefusesAnOverflowingBody(t *testing.T) {
	orig := readmeParagraphs
	t.Cleanup(func() { readmeParagraphs = orig })

	var filler strings.Builder
	for i := 0; i < 40; i++ {
		filler.WriteString("Padding sentence that occupies roughly one whole rendered line of the page. ")
	}
	readmeParagraphs = append(append([]string{}, orig...), filler.String())

	_, err := RenderReadme()
	if !errors.Is(err, ErrReadmeOverflow) {
		t.Fatalf("RenderReadme() error = %v, want ErrReadmeOverflow — a body past the page "+
			"renders without complaint, which is the silent failure this door exists to stop", err)
	}
}

// Every rendered line must fit the text column.
//
// **What this catches, and what it deliberately cannot.** It measures with the same
// function wrapText breaks on, so it is CIRCULAR with respect to the metric itself:
// it cannot tell a correct width table from a wrong one, and it was green against
// the hand-rolled estimate this slice deleted. What it does catch is a wrapper that
// packs past the column it was given — driven, a limit of maxW*1.10 fires it — and a
// line that cannot be broken at all. The metric's own correctness is guarded one
// level down, by mdpdf's coverage tests, which is where the encoded-form rule lives.
//
// Calling font.TextWidth on raw UTF-8 is the bug, not the fix: it iterates BYTES, so
// a line containing an em dash measures 15.90 where the glyph is 11.00.
func TestEveryReadmeLineFitsTheColumn(t *testing.T) {
	maxW := readmePageW - readmeLeft - readmeRight
	n := 0
	for _, ln := range readmeLines() {
		if ln == "" {
			continue
		}
		n++
		if w := mdpdf.CoreWidth(ln, readmeFont, readmeFontPt); w > maxW {
			t.Errorf("line runs %.2fpt past the %.0fpt column (nothing clips it, so it prints "+
				"off the sheet): %q", w-maxW, maxW, ln)
		}
	}
	if n == 0 {
		t.Fatal("no non-empty lines measured, so this test asserted nothing")
	}
}

// The About guard's stripping is exercised directly: the red proof for it deletes
// #aboutMain and stops at the "could not locate" fatal, so these branches are never
// reached by it.
func TestAboutScanIgnoresCommentsScriptsAndMarkup(t *testing.T) {
	strip := func(body string) string {
		body = htmlCommentRE.ReplaceAllString(body, " ")
		body = htmlScriptRE.ReplaceAllString(body, " ")
		body = htmlTagRE.ReplaceAllString(body, " ")
		return strings.Join(strings.Fields(body), " ")
	}
	for _, c := range []struct{ name, in, wantAbsent, wantPresent string }{
		{"comment", `<p>kept</p><!-- hidden claim -->`, "hidden claim", "kept"},
		{"script", `<p>kept</p><script>var s = "hidden claim";</script>`, "hidden claim", "kept"},
		{"attribute", `<p title="hidden claim">kept</p>`, "hidden claim", "kept"},
		{"tagsplit", `<p>one</p><p>two</p>`, "onetwo", "one two"},
	} {
		got := strip(c.in)
		if strings.Contains(got, c.wantAbsent) {
			t.Errorf("%s: %q survived stripping and would satisfy a claim check: %q", c.name, c.wantAbsent, got)
		}
		if !strings.Contains(got, c.wantPresent) {
			t.Errorf("%s: visible text %q was lost: %q", c.name, c.wantPresent, got)
		}
	}
}

// aboutMainRE isolates the About dialog's own copy: from its opening div to the
// licence pane that follows it.
var aboutMainRE = regexp.MustCompile(`(?s)<div id="aboutMain">(.*?)<pre id="aboutDocText"`)

var (
	htmlCommentRE = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlScriptRE  = regexp.MustCompile(`(?s)<script\b.*?</script>`)
	htmlTagRE     = regexp.MustCompile(`<[^>]*>`)
)

// Drift-guard: the in-app About dialog must make the same honest-trust claims as
// the appended readme. trustClaims is the single source.
//
// **It reads the About dialog's TEXT, not the file's bytes, and that is the whole
// point of the rewrite.** The previous form was strings.Contains over the whole of
// web/index.html, which is satisfied by an HTML comment, by a string inside a
// <script>, by a title= attribute, or by a leftover after #aboutModal is deleted
// outright. Measured: with #aboutMain deleted entirely and the six claims left in
// one HTML comment, the old form returned true for ALL SIX. docs/red-proofs.md's
// vacuous-green table records that shape as instances two, three and four; this was
// the fifth, and it is the SOLE discharge of P07's C08.
// internal/server/pinbehaviour_test.go is the model followed here: locate the named
// block, fail loudly when it is gone, and hold a population floor.
func TestAboutCopyContainsTrustClaims(t *testing.T) {
	html, err := os.ReadFile("../../web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	m := aboutMainRE.FindSubmatch(html)
	if m == nil {
		t.Fatal("could not locate the About dialog's #aboutMain block in web/index.html — " +
			"this scan would otherwise read the whole file and pass on a comment, so it " +
			"fails rather than reporting a drift check it did not perform")
	}
	body := string(m[1])
	body = htmlCommentRE.ReplaceAllString(body, " ")
	body = htmlScriptRE.ReplaceAllString(body, " ")
	body = htmlTagRE.ReplaceAllString(body, " ")
	body = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'", "&quot;", `"`).Replace(body)
	text := strings.Join(strings.Fields(body), " ")

	if len(trustClaims) < 6 {
		t.Fatalf("trustClaims has %d entries; an emptied list would make this loop pass "+
			"over nothing", len(trustClaims))
	}
	if len(text) < 400 {
		t.Fatalf("the About dialog's visible text is %d chars — too short to be the real copy, "+
			"so a green here would mean the extraction broke, not that the claims are present",
			len(text))
	}
	for _, c := range trustClaims {
		if !strings.Contains(text, c) {
			t.Errorf("the About dialog's visible text is missing trust claim %q — readme and "+
				"About have drifted", c)
		}
	}
}
