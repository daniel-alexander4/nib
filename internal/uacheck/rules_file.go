package uacheck

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The file-level clauses — `PLAN-ua-coverage.md` P06.
//
// These read the FILE rather than the document: the bytes on disk, before pdfcpu normalised anything.
// Every other rule in this package reads `d.Ctx`, which is what `ReadValidateAndOptimize` made of the
// file, and that is deliberately not the same thing.

func init() {
	register(Rule{
		Clause:  "7.1 t4",
		Summary: "files shall have a Suspects value of false",
		Check:   checkSuspects,
	})
	register(Rule{
		Clause:  "7.11 t1",
		Summary: "the file specification dictionary for an embedded file shall contain the non-empty F and UF keys",
		Check:   checkEmbeddedFileNames,
	})
	register(Rule{
		Clause:  "7.15 t1",
		Summary: "dynamic XFA forms shall not be used",
		Check:   checkDynamicXFA,
	})
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

// checkSuspects evaluates ua1 7.1 t4 (P06.S02).
//
// The profile's test is `Suspects != true` on the `CosDocument`, one check per document, and both
// halves of that matter: an ABSENT `/MarkInfo`, and a `/MarkInfo` with no `/Suspects`, both pass —
// `null != true`. `/Suspects true` means a producer marked the tagging as possibly unreliable, which
// is a claim about the document nib must not silently carry forward.
func checkSuspects(d *Document) Result {
	mi := d.dict(d.Catalog["MarkInfo"])
	if mi == nil {
		return Result{Verdict: Pass}
	}
	v, has := mi["Suspects"]
	if !has {
		return Result{Verdict: Pass}
	}
	// Resolved, not cast: an indirect boolean is a boolean (`/pending 496`, the same correction the
	// catalog rules took).
	b, isBool := d.boolValue(v)
	if !isBool {
		// `Suspects != true` is satisfied by anything that is not the boolean true, and a non-boolean
		// is not the boolean true. Passing is what the profile's test does, not leniency.
		return Result{Verdict: Pass}
	}
	if b {
		return Result{
			Verdict: Fail,
			Why: "/MarkInfo /Suspects is true, so the producer itself marks this document's tagging " +
				"as possibly unreliable",
			Where: "catalog /MarkInfo /Suspects",
		}
	}
	return Result{Verdict: Pass}
}

// checkEmbeddedFileNames evaluates ua1 7.11 t1 (P06.S02) over the file-specification door.
//
// The profile's test is `containsEF == false || (F != null && F != ” && UF != null && UF != ”)`.
// **It is a conjunction of four conditions and each fails separately**: `F` absent, `F` empty, `UF`
// absent, `UF` empty. The two keys are a file's name in a system-dependent encoding and in Unicode,
// and a reader with neither has nothing to call the attachment.
func checkEmbeddedFileNames(d *Document) Result {
	// **A DEFINITE failure beats a refusal**, which is this package's convention and not a preference:
	// `checkMediaClipsNameTheirContentType` scans the population before reporting what it missed, and
	// `rules_annots_test.go` states it by name. Returning CannotCheck first threw away a defect already
	// in hand — a document with a nameless attachment AND sixty-four bookmarks reported "nib could not
	// look" about the very specification it had read.
	specs, serr := d.fileSpecs()
	if len(specs) == 0 {
		if serr != "" {
			return Result{Verdict: CannotCheck, Why: serr}
		}
		return Result{
			Verdict: NotApplicable,
			Why:     "the document embeds no files, so there is no file specification to check",
		}
	}
	for _, s := range specs {
		// A specification with no embedded file satisfies the profile's first disjunct
		// (`containsEF == false`) whatever else it carries — it is a passing CHECK, and it is in the
		// population so that a document holding only such specs answers Pass rather than NotApplicable.
		if _, hasEF := s.dict["EF"]; !hasEF {
			continue
		}
		for _, key := range []string{"F", "UF"} {
			v, has := s.dict[key]
			if !has {
				return Result{
					Verdict: Fail,
					Why: fmt.Sprintf("an embedded file's specification has no /%s, so a reader has no "+
						"name for the attachment in that encoding", key),
					Where: s.where,
				}
			}
			// Resolved, not cast: an indirect or hex string is a string.
			text, isStr := d.text(v)
			if !isStr {
				return Result{
					Verdict: Fail,
					Why: fmt.Sprintf("an embedded file's specification has a /%s that is not a string, "+
						"so it names nothing", key),
					Where: s.where,
				}
			}
			if text == "" {
				return Result{
					Verdict: Fail,
					Why: fmt.Sprintf("an embedded file's specification has an EMPTY /%s; the clause "+
						"requires the key to be present AND non-empty", key),
					Where: s.where,
				}
			}
		}
	}
	// Nothing in the population fails, so a short population is the difference between "every
	// specification is complete" and "every specification nib SAW is complete".
	if serr != "" {
		return Result{Verdict: CannotCheck, Why: serr}
	}
	return Result{Verdict: Pass}
}

// checkDynamicXFA evaluates ua1 7.15 t1 (P06.S02).
//
// The profile's test is `dynamicRender != 'required'` on the `PDAcroForm`, so the subject is the
// AcroForm and a document with none has none — measured, veraPDF reports 0 passed / 0 failed for
// every corpus document without one, which is most of them.
//
// **The value read is the XFA packet's own `dynamicRender`, not a PDF key**, so a STATIC XFA form
// passes: the clause refuses dynamic rendering, not XFA. veraPDF fails its own `7.15-t01-fail-a.pdf`
// with one check, which is the measurement this rule is written against.
func checkDynamicXFA(d *Document) Result {
	// **The validated catalog is not enough, and that is measured** — `rawCatalog` says why: pdfcpu
	// DELETES the `/AcroForm` key on veraPDF's own fixture for this clause, so reading only the
	// validated catalog reports "no form" for a document whose form is the defect.
	// **The validated catalog is not enough, and that is measured.** pdfcpu DELETES an `/AcroForm`
	// whose `/Fields` is empty or absent — an XFA-only form, which is exactly veraPDF's own fixture for
	// this clause — so reading only the validated catalog reports "no form" for a document whose form
	// IS the defect. `dynamicRenderFromRawFile` says what that costs and why nothing is pinned.
	if form := d.dict(d.Catalog["AcroForm"]); form != nil {
		xfa, has := form["XFA"]
		if !has {
			return Result{Verdict: Pass}
		}
		render, why := d.xfaDynamicRender(xfa)
		return xfaVerdict(render, why)
	}
	found, render, why := d.dynamicRenderFromRawFile()
	if why != "" && !found {
		// nib could not establish whether a form is there at all. That is a refusal, not an absence:
		// answering NotApplicable would be a pass over a question nib never settled.
		return Result{Verdict: CannotCheck, Why: why}
	}
	if !found {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document has no AcroForm, so there is no XFA form to be dynamic",
		}
	}
	return xfaVerdict(render, why)
}

// xfaVerdict turns a `dynamicRender` reading into this clause's answer, so the validated and the
// re-read paths cannot drift apart (ADR-009).
func xfaVerdict(render, why string) Result {
	if why != "" {
		return Result{Verdict: CannotCheck, Why: why, Where: "catalog /AcroForm /XFA"}
	}
	if render == "required" {
		return Result{
			Verdict: Fail,
			Why: "the XFA packet declares dynamicRender required, so this is a dynamic XFA form — a " +
				"shape assistive technology cannot read, because the visible document is generated by " +
				"the form engine rather than present in the PDF",
			Where: "catalog /AcroForm /XFA, config dynamicRender",
		}
	}
	return Result{Verdict: Pass}
}

// maxXFAPackets and maxXFABytes bound the XFA reading, because `/XFA` is an array the document
// chooses the length of and each entry is a compressed stream it chooses the expansion of.
//
// **Measured as a real amplification, not a hypothetical**: pdfcpu caps ONE stream's decode at 512 MiB
// and caches the decoded bytes per object, so `/XFA [(a) 5 0 R (a) 5 0 R …]` repeated twenty thousand
// times — a few hundred bytes in the file — has nib scan half a gigabyte twenty thousand times in one
// request. Every other walk in this package declares a ceiling; this one now does too.
const (
	maxXFAPackets = 64
	maxXFABytes   = 32 << 20
)

// xfaDynamicRender reads the XFA packet's `dynamicRender` setting (P06.S02). It returns the value and,
// separately, a reason nib could not read part of the form — never a silent empty string, which would
// read as "not dynamic".
//
// # Why this is parsed and not searched, with a concrete counterexample
//
// `xmp.go` states the general form of this rule; here is the specific trap. veraPDF's own fixture
// writes the element as:
//
//	<dynamicRender
//	>required</dynamicRender
//	>
//
// XFA serialisers routinely put the newline before the closing angle bracket, so
// `bytes.Contains(x, "<dynamicRender>")` finds nothing in the one document in the corpus that is
// supposed to fail this clause. The element name is a token, not a string.
//
// # The /XFA value has two shapes, and one bad packet must not refuse the form
//
// It is either a single stream holding the whole XDP packet, or an ARRAY of alternating name and
// stream pairs — veraPDF's fixture uses the array, with entries named `config`, `template`, `form` and
// so on, and `dynamicRender` lives in `config`.
//
// **A packet that will not parse is skipped, not fatal.** Go's `encoding/xml` rejects things a Java
// parser accepts and XFA templates routinely contain: `encoding="ISO-8859-1"` or `"UTF-16"` (no
// `CharsetReader` is set), `version="1.1"`, and XHTML rich text carrying `&nbsp;`. Refusing the whole
// clause because the `template` packet has a named entity would make nib report "could not check" for
// ordinary forms that veraPDF passes — so each packet is tried, the first value found wins, and a
// packet nib could not read is remembered and only reported if NO packet yielded a value.
func (d *Document) xfaDynamicRender(xfa types.Object) (string, string) {
	var packets [][]byte
	var unread string
	total := 0
	add := func(o types.Object) {
		if len(packets) >= maxXFAPackets {
			if unread == "" {
				unread = fmt.Sprintf("the XFA form holds more than %d packets and nib stopped reading "+
					"there", maxXFAPackets)
			}
			return
		}
		sd, _, err := d.Ctx.DereferenceStreamDict(o)
		if err != nil || sd == nil {
			// A name entry in the alternating array lands here and is not a packet; so does a stream
			// nib cannot resolve. Only the second is worth reporting, and the two are indistinguishable
			// at this point, so neither is — the packet COUNT is what the caller checks.
			return
		}
		if err := sd.Decode(); err != nil {
			// **Recorded, not dropped.** A `config` packet that will not decode while `template` does
			// would otherwise make nib answer Pass having never read the setting.
			if unread == "" {
				unread = "an XFA packet could not be decoded: " + err.Error()
			}
			return
		}
		if total+len(sd.Content) > maxXFABytes {
			if unread == "" {
				unread = fmt.Sprintf("the XFA form's packets exceed %d bytes decoded and nib stopped "+
					"reading there", maxXFABytes)
			}
			return
		}
		total += len(sd.Content)
		packets = append(packets, sd.Content)
	}
	switch v := d.resolve(xfa).(type) {
	case types.Array:
		for _, e := range v {
			add(e)
		}
	default:
		add(xfa)
	}
	for _, p := range packets {
		v, err := dynamicRenderIn(p)
		if err != "" {
			if unread == "" {
				unread = err
			}
			continue
		}
		if v != "" {
			return v, ""
		}
	}
	if unread != "" {
		return "", unread
	}
	// **No packet declared it — including the case of no packets at all.** `dynamicRender` is then
	// null, and the profile's test is `dynamicRender != 'required'`, which null satisfies. An empty
	// `/XFA []` reaches here and must PASS: pdfcpu's validator accepts an empty array, so it is a
	// document the checker really sees.
	return "", ""
}

// dynamicRenderIn returns the text of the first `dynamicRender` element in one XFA packet.
//
// `RawToken` is used rather than `Token` because an XFA packet is a FRAGMENT: veraPDF's own fixture
// splits `<xdp:xdp>` across packets and carries the closing tag as its own array entry, so no single
// packet is a well-formed document and a matching-tag parser refuses the whole thing.
//
// **The element's text is accumulated and the nesting is counted, not flagged.** A counter that was
// merely set to 1 on the opening tag collapsed on a nested element of the same name, and a single
// captured run lost text split by a comment or a processing instruction — `requi<!--x-->red` read as
// `requi`, which is not `required`, so a dynamic form passed.
func dynamicRenderIn(packet []byte) (string, string) {
	dec := xml.NewDecoder(bytes.NewReader(packet))
	depth, inside := 0, 0
	var text strings.Builder
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A packet nib cannot parse is reported to the caller, which decides whether any other
			// packet answered the question.
			return "", "an XFA packet is not well-formed XML: " + err.Error()
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if inside == 0 && t.Name.Local == "dynamicRender" {
				inside = depth
			}
		case xml.EndElement:
			if inside == depth {
				// The element nib was reading has closed; its text is complete.
				return strings.TrimSpace(text.String()), ""
			}
			depth--
		case xml.CharData:
			if inside > 0 {
				text.Write(t)
			}
		}
	}
	return strings.TrimSpace(text.String()), ""
}

// dynamicRenderFromRawFile re-reads the file UNVALIDATED and returns its AcroForm's `dynamicRender`.
//
// # Why a rule ever needs a second parse
//
// `open` reads with `ReadValidateAndOptimize`, and pdfcpu's validator does not merely report what it
// refuses — it DELETES it. The exact trigger is `validate/form.go`'s handling of an AcroForm whose
// `/Fields` is empty or absent: `rootDict.Delete("AcroForm")`. That is precisely an XFA-ONLY form, and
// it is veraPDF's own `7.15-t01-fail-a.pdf`, the single corpus document that exists to fail this
// clause: the file carries `/AcroForm 2 0 R` with the XFA packet, and after pdfcpu's read the catalog
// has no `/AcroForm` key at all while keeping `/NeedsRendering`, the marker of the very form it
// dropped. A rule reading only the validated catalog answers NotApplicable there — a false pass on the
// clause's own fixture, which the corpus guard caught.
//
// A HYBRID form (non-empty `/Fields` plus `/XFA`) keeps its validated dictionary and never comes here,
// because `validateFormXFA` errors rather than deleting. That is the load-bearing reason this fallback
// is narrow, and it is why it is stated.
//
// # Nothing is pinned
//
// The second context is dropped before returning: only the answer survives. An earlier version
// memoised a whole `*Document` over the raw parse and held two full contexts for the rest of `Check`.
// The parse itself is measured at 1–10 ms over 159 KB–2.6 MB (22%, 70% and 111% of the parse `open`
// already paid); it is NOT projected past that range, and it is paid on every document with no
// validated AcroForm, which is most of them.
//
// **`scanInlineType3` uses the same technique for a different omission and keeps its own parse**, so a
// document that needs both is parsed three times. That is a real duplication and it is NOT claimed to
// be one door here; consolidating them is `/pending 656`.
func (d *Document) dynamicRenderFromRawFile() (found bool, render string, why string) {
	if len(d.raw) == 0 {
		return false, "", "this document was assembled in memory rather than read from a file, so nib " +
			"cannot re-read what pdfcpu's validator may have dropped"
	}
	ctx, err := api.ReadContext(bytes.NewReader(d.raw), model.NewDefaultConfiguration())
	if err != nil {
		return false, "", "the file could not be re-read without validation: " + err.Error()
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return false, "", "the unvalidated re-read has no catalog: " + cerr.Error()
	}
	raw := &Document{Ctx: ctx, Catalog: cat}
	form := raw.dict(cat["AcroForm"])
	if form == nil {
		return false, "", ""
	}
	xfa, has := form["XFA"]
	if !has {
		return true, "", ""
	}
	render, why = raw.xfaDynamicRender(xfa)
	return true, render, why
}
