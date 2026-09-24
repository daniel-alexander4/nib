package uacheck

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
)

// 6.1 t1 — the file header (P06.S01).
//
// Every case here is a length-preserving rewrite of a real document's header, because the clause is
// about the FILE's bytes: a fixture built by writing a document would have pdfcpu put a well-formed
// header back, and would measure the writer rather than the rule.

func TestTheFileHeaderIsReadFromTheFilesOwnBytes(t *testing.T) {
	doc, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// The control: unedited, the header is whatever pdfcpu wrote, and it passes.
	if got := verdictOf(t, doc, "6.1 t1"); got.Verdict != Pass {
		t.Fatalf("control: an unedited document reports %v for 6.1 t1 (%s), want Pass", got.Verdict, got.Why)
	}

	for _, c := range []struct {
		name   string
		header string
		want   Verdict
		why    string
	}{
		// The profile's test is `/^%PDF-1\.[0-7]$/`, anchored at both ends.
		{"a known minor version", "%PDF-1.4", Pass, ""},
		{"the lowest minor version", "%PDF-1.0", Pass, ""},
		{"the highest minor version", "%PDF-1.7", Pass, ""},
		// Reachable failures — pdfcpu opens both. `%PDF-1.9` is NOT here: pdfcpu refuses it before any
		// rule runs, which `checkFileHeader` records as the clause's unreachable half.
		{"a major version the clause does not name", "%PDF-2.0", Fail, "%PDF-2.0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := verdictOf(t, withFileHeader(t, doc, c.header), "6.1 t1")
			if got.Verdict != c.want {
				t.Fatalf("header %q reports %v (%s), want %v", c.header, got.Verdict, got.Why, c.want)
			}
			// A Fail has to name the header it read, or the user cannot tell which byte to change.
			if c.why != "" && !strings.Contains(got.Why, c.why) {
				t.Errorf("header %q is refused as %q, which does not quote the header it read", c.header, got.Why)
			}
		})
	}
}

// **Anything between the version and the EOL fails, and that is the only sense in which this clause
// tests the ISO sentence's "followed by a single EOL marker".** The profile's regex is anchored, so a
// longer line simply stops matching — measured on veraPDF, whose failure message reported the header
// back as `%PDF-1.6X%öäüß`, the rest of the line and its binary comment included. The header is
// lengthened by consuming the EOL byte rather than by inserting one, so every xref offset stays true.
func TestAByteAfterTheVersionIsPartOfTheHeader(t *testing.T) {
	doc, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	got := verdictOf(t, withHeaderEOL(t, doc, "X"), "6.1 t1")
	if got.Verdict != Fail {
		t.Fatalf("a header line continuing past the version reports %v (%s), want Fail", got.Verdict, got.Why)
	}
	// The header it read runs past the version — it is not truncated to the eight-byte match.
	if !strings.Contains(got.Why, "X") {
		t.Errorf("the refusal is %q, which does not show the bytes that follow the version", got.Why)
	}
}

// A CR terminates the header line, so `%PDF-1.n\r\n` passes — measured on veraPDF, where replacing a
// corpus file's EOL byte with a lone CR left the file passing. A reader that took the line to the LF
// would see a trailing CR, fail the anchored regex and disagree with the oracle on every CRLF file
// there is.
func TestACarriageReturnEndsTheHeaderLine(t *testing.T) {
	doc, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	crlf := withHeaderEOL(t, doc, "\r")
	if got := verdictOf(t, crlf, "6.1 t1"); got.Verdict != Pass {
		t.Errorf("a header ended by CR reports %v (%s), want Pass — a CR is an EOL marker", got.Verdict, got.Why)
	}
}

// **A document nib assembled rather than read has no bytes to check, and the clause says so.** It is
// never a Pass: the alternative is reporting conformance of a header nib was never given.
func TestAnInMemoryDocumentCannotAnswerTheHeaderClause(t *testing.T) {
	doc, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// `docWithViewerPref` builds a `*Document` straight from a parsed context, which is the shape a
	// document assembled rather than read from a file has: no `raw`. `openMutated` is NOT that shape —
	// it goes through `open`, which keeps the bytes it was handed.
	d := docWithViewerPref(t, doc, "DisplayDocTitle", types.Boolean(true))
	got := registry["6.1 t1"].Check(d)
	if got.Verdict != CannotCheck {
		t.Fatalf("a document with no raw bytes reports %v (%s) for 6.1 t1, want CannotCheck", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "assembled in memory") {
		t.Errorf("the refusal is %q, which does not say nib never held the bytes", got.Why)
	}
}

// **`header` is the whole LINE that first mentions `%PDF-`, not the match**, and these three cases
// are what separate the two readings. All three were measured against veraPDF 1.30.2 before being
// written down, and nib disagreed with it on two of them until this was fixed.
//
// Slicing at the `%PDF-` occurrence makes the profile's `^` anchor unfalsifiable — the result then
// begins with `%PDF-` by construction — so junk on the header's own line passed. A fixed search
// window made the opposite error, refusing a file whose header sits past it although pdfcpu reads it.
func TestTheHeaderIsTheWholeLineThatMentionsPDF(t *testing.T) {
	doc, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		prefix string
		want   Verdict
	}{
		// veraPDF: FAILED. The header line is `ZZZZ…%PDF-1.n`, which the anchored regex refuses.
		{"junk on the header's own line", "ZZZZZZZZZZZZZZZZ", Fail},
		// veraPDF: passed. Line 1 mentions no `%PDF-`, so the header is line 2, intact.
		{"junk on a line of its own", "ZZZZ leading\n", Pass},
		// veraPDF: passed. Same shape, far past any fixed search window — 2,144 bytes of preamble.
		{"a preamble longer than any window", strings.Repeat("%% leading junk\n", 134), Pass},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := verdictOf(t, append([]byte(c.prefix), doc...), "6.1 t1")
			if got.Verdict != c.want {
				t.Errorf("with %d bytes of preamble, 6.1 t1 reports %v (%s), want %v — measured on "+
					"veraPDF, which is the oracle here", len(c.prefix), got.Verdict, got.Why, c.want)
			}
		})
	}
}

// **The `[0-7]` digit class — the clause's namesake — cannot be reached through `Check`.**
//
// pdfcpu refuses `%PDF-1.8`, `%PDF-1.9` and `%PDF-0.0` at `headerVersion: unknown PDF Header Version`
// before any rule runs, which is why the oracle's failing fixture is `%PDF-2.0` instead. So the one
// condition the clause is actually about had no test at all: widening the pattern to `[0-9]` left
// every test in this file green. The rule is called directly here, against a `Document` carrying only
// the bytes — the same door `TestAnInMemoryDocumentCannotAnswerTheHeaderClause` uses.
func TestTheHeaderVersionDigitClassIsExactlyZeroToSeven(t *testing.T) {
	for _, c := range []struct {
		header string
		want   Verdict
	}{
		{"%PDF-1.0", Pass}, {"%PDF-1.7", Pass}, {"%PDF-1.4", Pass},
		{"%PDF-1.8", Fail}, {"%PDF-1.9", Fail}, {"%PDF-2.0", Fail}, {"%PDF-1.a", Fail},
	} {
		d := &Document{raw: []byte(c.header + "\n%\xe2\xe3\xcf\xd3\n")}
		if got := checkFileHeader(d); got.Verdict != c.want {
			t.Errorf("header %q reports %v (%s), want %v", c.header, got.Verdict, got.Why, c.want)
		}
	}
}

// A header line with no EOL after it runs to the end of the file, so the refusal bounds what it
// quotes back — an unbounded `%q` puts the whole remainder of the document into a user-facing
// message. Measured before the bound: a 200-byte trailer produced a 929-byte reason.
func TestTheHeaderRefusalDoesNotQuoteTheWholeFile(t *testing.T) {
	// **Asserted as independence from the file's size, not as an absolute length.** A fixed ceiling
	// would pass for any cap that happened to sit under it; two sizes four kilobytes apart can only
	// agree if the quote is bounded.
	small := checkFileHeader(&Document{raw: append([]byte("%PDF-1.7X"), bytes.Repeat([]byte{0xFF}, 256)...)})
	large := checkFileHeader(&Document{raw: append([]byte("%PDF-1.7X"), bytes.Repeat([]byte{0xFF}, 4096)...)})
	if small.Verdict != Fail || large.Verdict != Fail {
		t.Fatalf("a header line running to EOF reports %v / %v, want Fail", small.Verdict, large.Verdict)
	}
	if len(small.Why) != len(large.Why) {
		t.Errorf("the refusal is %d bytes for a 256-byte trailer and %d for a 4096-byte one, so it "+
			"grows with the file: an unbounded %%q puts the whole document into a user-facing message",
			len(small.Why), len(large.Why))
	}
	if !strings.Contains(large.Why, "\u2026") {
		t.Errorf("the refusal does not mark that it truncated what it quoted: %.120q", large.Why)
	}
}
