package uacheck

import (
	"bytes"
	"fmt"
	"regexp"
)

// The file-level clauses — `PLAN-ua-coverage.md` P06.
//
// These read the FILE rather than the document: the bytes on disk, before pdfcpu normalised anything.
// Every other rule in this package reads `d.Ctx`, which is what `ReadValidateAndOptimize` made of the
// file, and that is deliberately not the same thing.

func init() {
	register(Rule{
		Clause:  "6.1 t1",
		Summary: `the file header shall be "%PDF-1.n" for a single digit n between 0 and 7, followed by a single EOL marker`,
		Check:   checkFileHeader,
	})
}

// headerPattern is veraPDF's own test, transcribed: `/^%PDF-1\.[0-7]$/.test(header)`.
var headerPattern = regexp.MustCompile(`^%PDF-1\.[0-7]$`)

// maxQuotedHeader bounds how much of the header line the refusal repeats back. The line itself is
// whatever the file contains: a header with no EOL after it runs to the end of the file, so quoting
// it whole would put megabytes into a user-facing message. The clause needs exactly eight bytes, so
// anything past this is already a failure and the excess adds nothing a reader can act on.
const maxQuotedHeader = 64

// headerLine returns the first line of raw that contains `%PDF-`, whole, with the offset it starts
// at — veraPDF's `getLine`, which is what its `header` string actually holds.
//
// **From the LINE's start, not from the `%PDF-`, and that distinction is measured.** Slicing at the
// occurrence makes the profile's `^` anchor unfalsifiable, since the result then begins with `%PDF-`
// by construction. veraPDF does not do that: a file beginning `ZZZZZZZZZZZZZZZZ%PDF-1.6` — junk on
// the header's OWN line — is FAILED by veraPDF and was passed by nib until this was measured, while
// the same junk on a PRIOR line (`ZZZZ leading\n%PDF-1.6`) is PASSED by both. The rule that explains
// both is "the first line that mentions `%PDF-`, in full".
//
// **And the search is the whole file, not a window.** A 1 KiB cap was measured to refuse a file with
// 2,144 bytes of preamble that pdfcpu opens and veraPDF passes — nib's own reader looked further than
// its checker did. `bytes.Index` returns the FIRST occurrence, so a file with its header at offset 0
// is unaffected and only the case that used to refuse changes.
func headerLine(raw []byte) ([]byte, int, bool) {
	i := bytes.Index(raw, []byte("%PDF-"))
	if i < 0 {
		return nil, 0, false
	}
	// Back to the start of the line holding it: one past the previous EOL byte, or offset 0.
	start := bytes.LastIndexAny(raw[:i], "\r\n") + 1
	end := len(raw)
	if j := bytes.IndexAny(raw[i:], "\r\n"); j >= 0 {
		end = i + j
	}
	return raw[start:end], start, true
}

// checkFileHeader evaluates ua1 6.1 t1 (P06.S01).
//
// # What `header` actually is, measured rather than assumed
//
// veraPDF's test is a regex ANCHORED at both ends over a string called `header`, so everything turns on
// what that string holds. It is not the eight-byte version match: veraPDF's parser takes the first line
// from `%PDF-` to the end of the line, and the failure message prints it back. Four length-preserving
// mutations of a corpus file settle it, each read out of veraPDF's own `%1` argument:
//
//   - `%PDF-1.9` — fails, reported as `%PDF-1.9` (the digit class is real).
//   - `%PDF-2.6` — fails (the `1.` is literal).
//   - the EOL byte replaced by `X` — fails, reported as **`%PDF-1.6X%öäüß`**: the rest of the line,
//     binary comment included. Nothing is truncated to eight bytes.
//   - the EOL byte replaced by a lone `\r` — **passes**, so a CR terminates the line.
//
// # So the ISO sentence's EOL half is only half-tested, and nib matches veraPDF rather than the text
//
// "followed by a single EOL marker" is enforced only in the sense that any byte between the version
// digit and the EOL lengthens the string past the anchored regex. `%PDF-1.7\r\n` passes; `%PDF-1.7 `
// fails on the trailing space. nib deliberately reproduces that: law 5's guard compares nib to veraPDF,
// and a rule that were stricter than the oracle would report a failure no one else can reproduce.
//
// # What this clause cannot reach from a file, measured
//
// pdfcpu refuses to open a document whose header names a minor version it does not know: `%PDF-1.9`
// fails at `headerVersion: unknown PDF Header Version: 1.9` before any rule runs, while veraPDF
// validates the same bytes and fails this clause on them. So **the out-of-range minor digit — the most
// obvious way to fail this clause — is unreachable through `open`**, and a document that reaches the
// checker at all has already passed pdfcpu's own narrower test of the same field. What remains
// reachable is a major version pdfcpu knows but the profile refuses (`%PDF-2.0`), and anything trailing
// on the header line (`%PDF-1.6X…`), both of which pdfcpu opens. The clause is still worth checking:
// those two are real failures, and the alternative is a checker silent about the header entirely.
//
// # Where the bytes come from
//
// `d.raw`, the file exactly as handed to `open` — the second reader of it after `hasInlineType3Font`.
// A `*Document` assembled in memory rather than read from a file has no `raw`, and this clause is then
// `CannotCheck` saying so. It is never a Pass: nib cannot report on bytes it was never given.
func checkFileHeader(d *Document) Result {
	if len(d.raw) == 0 {
		return Result{
			Verdict: CannotCheck,
			Why: "this document was assembled in memory rather than read from a file, so nib does not " +
				"hold the bytes the header is made of",
		}
	}
	line, start, found := headerLine(d.raw)
	if !found {
		return Result{
			Verdict: CannotCheck,
			Why: "the file contains no %PDF- marker anywhere, so nib cannot say what its header is — " +
				"pdfcpu read the document from some other structure",
		}
	}
	if !headerPattern.Match(line) {
		quoted, over := line, ""
		if len(quoted) > maxQuotedHeader {
			quoted, over = quoted[:maxQuotedHeader], "…"
		}
		// **The refusal says what nib COMPARED, not what the clause says.** The registered `Summary`
		// carries veraPDF's own clause text, EOL marker and all, because this package quotes clauses
		// rather than paraphrasing them; repeating that wording here would claim nib had checked the
		// EOL separately, and it has not — `%PDF-1.7\n\n` passes. What nib checked is the header line.
		return Result{
			Verdict: Fail,
			Why: fmt.Sprintf("the file's header line is %q%s; PDF/UA-1 requires it to be exactly "+
				"%%PDF-1.n, where n is a single digit from 0 to 7", quoted, over),
			Where: fmt.Sprintf("file offset %d", start),
		}
	}
	return Result{Verdict: Pass}
}
