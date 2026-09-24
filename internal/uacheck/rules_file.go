package uacheck

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
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
		Clause:  "7.16 t1",
		Summary: "an encrypted file's encryption dictionary shall contain a P key whose 10th bit is true",
		Check:   checkEncryptionPermissions,
	})
	register(Rule{
		Clause:  "7.20 t1",
		Summary: "a conforming file shall not contain any reference XObjects",
		Check:   checkReferenceXObjects,
	})
	register(Rule{
		Clause:  "7.20 t2",
		Summary: "a form XObject whose content is incorporated into structure elements shall be referenced only once",
		Check:   checkUniqueSemanticParent,
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

// xfaDynamicRender reads the XFA form's `dynamicRender` setting exactly where veraPDF reads it (P06.S02,
// ported at the P06 phase close). It returns the value and, separately, a reason nib could not read it —
// never a silent empty string, which would read as "not dynamic".
//
// # veraPDF's predicate, transcribed (`GFPDAcroForm.java:87-125`)
//
//   - `/XFA` is a stream, or an ARRAY of alternating names and streams; from an array veraPDF takes the
//     entry right after the FIRST string `config`, and if there is none, nothing — the test then passes.
//   - That stream is parsed as a whole XML document by a non-namespace-aware `DocumentBuilder`; a document
//     that does not parse gives no value, and the test passes.
//   - From the root it walks `xdp:xdp` (optional) → `config` → `acrobat` → `acrobat7` → `dynamicRender`,
//     each the FIRST child whose qualified name — prefix included — is exactly that.
//   - The value is `dynamicRender`'s FIRST CHILD NODE's value, untrimmed: its text run, or nothing.
//
// **Until the phase close nib read every packet at any depth and trimmed**, which the review measured as
// four live divergences on variations of veraPDF's own fixture: the element moved into `template`, the
// value written ` required `, the element moved off the `acrobat7` path — all PASSED by veraPDF and FAILED
// by nib — and `&nbsp;` in the config packet, passed by veraPDF and refused by nib. The corpus's one
// `7.15` document fails the clause, so none of the four was visible to it.
//
// # Where Go and veraPDF's parser differ, nib refuses rather than guesses
//
// A syntax error — a mismatched tag, an undeclared entity like `&nbsp;` — is a document neither parser
// accepts, and passes. But Go's decoder also refuses what a Java parser reads: an `encoding` other than
// UTF-8 or ISO-8859-1, and `version="1.1"`. Those are `CannotCheck`: the packet may be dynamic, and nib
// did not read it.
//
// **And Go's decoder accepts shapes veraPDF's parser refuses**, which without a check are false FAILS. Three
// review rounds measured eleven and each is now refused as not-a-document: a DOCTYPE, an unbound prefix, a
// prefix bound to "", one attribute twice (by URI and local name), anything before the XML declaration, the
// reserved name `xml` in another case — and a UTF-8 byte-order mark, which veraPDF skips and Go does not.
// **This list is not proven complete**: it is the part of XML 1.0 + Namespaces well-formedness that Go
// omits and three passes found, and the remainder is `/pending 673`, not an assumption.
func (d *Document) xfaDynamicRender(xfa types.Object) (string, string) {
	target := xfa
	if arr, isArr := d.resolve(xfa).(types.Array); isArr {
		target = nil
		for i := 0; i+1 < len(arr); i++ {
			if str, ok := d.text(arr[i]); ok && str == "config" {
				target = arr[i+1]
				break
			}
		}
		if target == nil {
			// No `config` entry: veraPDF reads no stream, `dynamicRender` is null, and the test passes.
			return "", ""
		}
	}
	sd, _, err := d.Ctx.DereferenceStreamDict(target)
	if err != nil || sd == nil {
		// Not a stream (a name, a dictionary, null): veraPDF reads nothing from it either.
		return "", ""
	}
	if err := sd.Decode(); err != nil {
		// Recorded, not dropped: a config packet nib cannot decode may say `required`.
		return "", "the XFA form's config packet could not be decoded: " + err.Error()
	}
	if len(sd.Content) > maxXFABytes {
		return "", fmt.Sprintf("the XFA form's config packet exceeds %d bytes decoded and nib stopped there", maxXFABytes)
	}
	return dynamicRenderIn(sd.Content)
}

// xfaPath is the element path veraPDF walks to `dynamicRender`, by qualified name; the leading `xdp:xdp`
// is optional.
var xfaPath = []string{"xdp:xdp", "config", "acrobat", "acrobat7", "dynamicRender"}

// dynamicRenderIn returns the value of the first child node of the `dynamicRender` element at veraPDF's
// path in one XFA packet, and a reason when nib could not read the packet the way veraPDF does.
//
// `RawToken` keeps the prefix as written, which is what a non-namespace-aware DOM's node names are; the
// well-formedness `Token` would enforce is checked here by hand, because veraPDF parses the WHOLE document
// before walking it — a value found early in a document that is broken later is not a value.
func dynamicRenderIn(packet []byte) (string, string) {
	// A UTF-8 byte-order mark is skipped, as veraPDF's parser skips it; Go's decoder calls it a syntax error,
	// which the rule below would have turned into a Pass on a packet veraPDF FAILS (the R1 re-review, measured).
	packet = bytes.TrimPrefix(packet, []byte("\xef\xbb\xbf"))
	dec := xml.NewDecoder(bytes.NewReader(packet))
	dec.CharsetReader = latin1Reader
	// **Go's decoder accepts four shapes veraPDF's parser rejects**, each measured as a live false FAIL by the
	// R1 re-review: a DOCTYPE, a prefix no `xmlns:` binds, anything before the XML declaration, and one attribute
	// written twice. veraPDF's parse fails on all four and the test passes, so here they are what a syntax
	// error is — not a document.
	type frame struct {
		name string
		// bound is the prefixes this element's own `xmlns:` attributes declare, and the URIs they bind.
		bound map[string]string
		// at is how far along xfaPath this element is (-1 off the path); took is whether a child has
		// already been taken as the next path step, since veraPDF takes only the FIRST child of a name.
		at   int
		took bool
	}
	var stack []frame
	rootSeen := false
	var value strings.Builder
	capturing, captured := false, false
	// kind is what the first child node is: 0 not yet seen, 1 a text run, 2 a CDATA section.
	kind := 0
	end := func() { capturing, captured = false, true }
	for {
		off := dec.InputOffset()
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			var se *xml.SyntaxError
			if errors.As(err, &se) && !strings.Contains(se.Msg, "unsupported version") {
				// A document neither parser accepts: veraPDF catches the exception and the test passes.
				return "", ""
			}
			return "", "the XFA config packet is XML nib's reader cannot parse the way veraPDF's does: " + err.Error()
		}
		if len(stack) > maxXMPDepth {
			return "", fmt.Sprintf("the XFA config packet nests deeper than %d elements and nib stopped reading there", maxXMPDepth)
		}
		// uriOf resolves a prefix through the bindings in scope; "" with false when nothing binds it.
		uriOf := func(prefix string) (string, bool) {
			switch prefix {
			case "":
				return "", true
			case "xml":
				return "http://www.w3.org/XML/1998/namespace", true
			case "xmlns":
				return "http://www.w3.org/2000/xmlns/", true
			}
			for i := len(stack) - 1; i >= 0; i-- {
				if u, ok := stack[i].bound[prefix]; ok {
					return u, true
				}
			}
			return "", false
		}
		switch t := tok.(type) {
		case xml.Directive:
			return "", "" // a DOCTYPE (or any declaration): veraPDF's parser refuses the document
		case xml.StartElement:
			own := map[string]string{}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" {
					if a.Value == "" {
						return "", "" // `xmlns:foo=""` unbinds nothing in XML 1.0: not a document (R1 round 3)
					}
					own[a.Name.Local] = a.Value
				}
			}
			stack = append(stack, frame{bound: own}) // provisional, so this element's own bindings count
			_, ok := uriOf(t.Name.Space)
			// **Duplicate attributes are compared by (URI, local name)**, as a namespace-aware parser does —
			// two prefixes bound to one URI name the same attribute (R1 round 3, measured).
			seen := map[[2]string]bool{}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
					key := [2]string{"xmlns", a.Name.Local}
					if seen[key] {
						ok = false
					}
					seen[key] = true
					continue
				}
				uri, bound := uriOf(a.Name.Space)
				if a.Name.Space == "" {
					uri = "" // an unprefixed attribute is in no namespace, whatever the default is
				}
				key := [2]string{uri, a.Name.Local}
				if !bound || seen[key] {
					ok = false
				}
				seen[key] = true
			}
			stack = stack[:len(stack)-1]
			if !ok {
				return "", "" // an unbound prefix or one attribute twice: not a document
			}
			name := t.Name.Local
			if t.Name.Space != "" {
				name = t.Name.Space + ":" + name
			}
			if capturing {
				end() // an element ends the first child, or IS it, and an element's value is null
			}
			at := -1
			if len(stack) == 0 {
				if rootSeen {
					return "", "" // two root elements: not a document
				}
				rootSeen = true
				switch name {
				case "xdp:xdp":
					at = 0
				case "config":
					at = 1
				}
			} else if parent := &stack[len(stack)-1]; parent.at >= 0 && parent.at+1 < len(xfaPath) &&
				!parent.took && name == xfaPath[parent.at+1] {
				parent.took = true
				at = parent.at + 1
			}
			stack = append(stack, frame{name: name, at: at, bound: own})
			if at == len(xfaPath)-1 && !captured {
				capturing = true
			}
		case xml.EndElement:
			name := t.Name.Local
			if t.Name.Space != "" {
				name = t.Name.Space + ":" + name
			}
			if len(stack) == 0 || stack[len(stack)-1].name != name {
				return "", "" // a mismatched end tag: not a document
			}
			if capturing {
				end() // the element closed: its first child, if any, is complete
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			// **The first child node, as veraPDF's DOM builds it — measured, each case on veraPDF 1.30.2**:
			// a text run MERGES across comments (`requi<!--x-->red` and `<!--x-->required` both FAIL), a
			// CDATA section is a node of its own (`requi<![CDATA[red]]>` PASSES, `<![CDATA[required]]>`
			// FAILS), and a processing instruction or an element ends the run (`requi<?pi?>red` and
			// `<x/>required` PASS). Go reports CDATA as ordinary character data, so the input decides.
			if capturing {
				cdata := off < int64(len(packet)) && bytes.HasPrefix(packet[off:], []byte("<![CDATA["))
				switch {
				case kind == 0 && cdata:
					value.Write(t)
					end()
				case kind == 0:
					kind = 1
					value.Write(t)
				case kind == 1 && !cdata:
					value.Write(t)
				default:
					end()
				}
			} else if len(stack) == 0 && strings.TrimSpace(string(t)) != "" {
				return "", "" // text outside the root element: not a document
			}
		case xml.Comment:
			// Ignored: veraPDF's DOM carries no comment nodes, so text on either side is one run.
		case xml.ProcInst:
			if strings.EqualFold(t.Target, "xml") && (t.Target != "xml" || off != 0) {
				// The declaration is exactly `xml`, at the very start; any other case of the reserved name, or
				// the declaration anywhere else, is not a document (R1 re-review, rounds 2 and 3).
				return "", ""
			}
			if capturing {
				if kind == 0 {
					value.Write(t.Inst) // a processing instruction first: that is the first child's value
				}
				end()
			}
		}
	}
	if len(stack) != 0 || !rootSeen {
		return "", "" // unclosed or empty: not a document
	}
	return value.String(), ""
}

// latin1Reader lets the decoder read an ISO-8859-1 packet, which a Java parser reads natively; any other
// declared encoding stays an error, and `dynamicRenderIn` turns it into a refusal.
func latin1Reader(label string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "iso-8859-1", "latin1", "latin-1", "iso_8859-1", "us-ascii":
		b, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		return strings.NewReader(string(r)), nil
	}
	return nil, fmt.Errorf("encoding %q is not one nib's XML reader supports", label)
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
	ctx, err := api.ReadContext(bytes.NewReader(d.raw), checkerConfig())
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

// checkReferenceXObjects evaluates ua1 7.20 t1 (P06.S03) over the forms the document DRAWS.
//
// The profile's test is `containsRef == false` on every `PDXForm`. A reference XObject imports the
// content of ANOTHER document by reference, so what a reader shows depends on a file that may not be
// there — and nothing in the structure tree describes it.
//
// **The population is what the document DRAWS, not what it holds** — `formXObjects` says why, and the
// distinction was measured rather than reasoned. Four holders were measured to be subjects: a form the
// page's content draws, a form an outer form's content draws, and an annotation's `/AP` appearance in
// its `/N`, `/R` and `/D` states. A form sitting in a page's resources that nothing draws is NOT one.
func checkReferenceXObjects(d *Document) Result {
	forms, ferr := d.formXObjects()
	for _, f := range forms {
		if _, hasRef := f.dict["Ref"]; hasRef {
			return Result{
				Verdict: Fail,
				Why: "a form XObject carries /Ref, so it imports another document's content by " +
					"reference — what a reader shows depends on a file that may not be there, and " +
					"nothing in this document's structure describes it",
				Where: f.where,
			}
		}
	}
	// A definite failure beats a refusal, so the short-population check comes after the scan.
	if ferr != "" {
		return Result{Verdict: CannotCheck, Why: ferr}
	}
	if len(forms) == 0 {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document draws no form XObjects, so there is none to be a reference XObject",
		}
	}
	return Result{Verdict: Pass}
}

// checkUniqueSemanticParent evaluates ua1 7.20 t2 (P06.S05).
//
// # The predicate reads no MCID and no structure tree
//
// veraPDF's test is `isUniqueSemanticParent == true` on every `PDXForm`, and `GFPDXForm.java:163-176` is
// the whole of it: a form with no `/StructParents` KEY passes; a form with no object key passes; a form
// whose key was already seen FAILS; otherwise the key is recorded and it passes. The profile's message —
// *"Form XObject contains MCIDs and is referenced more than once"* — is prose about why the rule exists,
// not what it tests, and every half of that difference was measured: a form with MCIDs and no
// `/StructParents` drawn twice PASSES, one with `/StructParents` and no MCIDs drawn twice FAILS, and one
// whose MCIDs are claimed by elements under two different parents PASSES. `knownKey` is key PRESENCE: a
// `/StructParents` naming a `null` object, or naming nothing at all, is a key and fails twice-drawn,
// while a direct `null` is no key (pdfcpu drops it, and veraPDF passes it).
//
// # The count is veraPDF's traversal, not the document's draws
//
// `recordDrawnForm` tallies a reach only where veraPDF builds a `PDXForm`, which is every `Do` in a
// stream it TRAVERSES and every appearance entry of an annotation it visits — and it traverses a stream
// once per object key. `retraversal` has the rule and the measurements.
//
// # And the count may not be nib's to give
//
// pdfcpu fuses equal form XObjects when it opens the file, so the object numbers the walk sees are not
// always veraPDF's. `formTwins` asks pdfcpu's own predicate which reached forms could have been fused. A
// keyed form with no twin reached twice is exact. One reached more times than it has twins is a definite
// failure, since some one of them was reached twice. Anything else touched by a twin is a count nib knows
// is not veraPDF's, and the answer is a refusal.
//
// **The refusal is deliberately wider than the fusion.** A twin is any equal form in the xref table,
// including an unreferenced copy pdfcpu never fused — so one orphaned duplicate turns a real failure into
// `CannotCheck`. Telling a fused copy from an unfused one would need the file's references before the
// optimize pass rewrote them, and a refusal is never a wrong verdict. On veraPDF's corpus it costs nothing.
//
// # Two declared gaps
//
//   - A `/StructParents` that is neither an integer nor a reference — `(a)` — is FAILED twice-drawn by
//     veraPDF, and pdfcpu's validator refuses to open the file ("validateIntegerEntry … invalid type"),
//     so nib emits no report at all: the same declared class as `6.1 t1`'s `%PDF-1.9`.
//   - A stream is traversed once, so which forms a stream's `Do`s name is settled by the resources in
//     force at its FIRST traversal — measured: a form with no `/Resources` drawn on two pages that bind its
//     name differently is FAILED when the drawing binding is on page 1 and PASSED when it is on page 2.
//     nib cannot follow either, because pdfcpu's reader drops the page binding only the form uses, so both
//     are refusals (the XObject route in `doXObject`, the pattern route in `enterPattern`). That leaves
//     `retraversal`'s `underRepeat` and `enterLangOnly`'s inherited `repeat` unreachable through a file:
//     measured red-proof survivors, kept because they are veraPDF's semantics the day the binding survives.
//     Whether nib's walk order (page content, then appearances) is veraPDF's link order across the two
//     routes is still unmeasured.
func checkUniqueSemanticParent(d *Document) Result {
	forms, ferr := d.formXObjects()
	nrs := make([]int, 0, len(d.formReaches))
	for nr := range d.formReaches {
		nrs = append(nrs, nr)
	}
	sort.Ints(nrs)
	var unsure string
	for _, nr := range nrs {
		r := d.formReaches[nr]
		twins, ok := d.formTwins(nr)
		if !ok {
			if unsure == "" {
				unsure = fmt.Sprintf("the document holds more equal-length form XObjects than nib will compare (%d pairs), so "+
					"which of them pdfcpu fused when it opened the file — and so how often each was drawn — was never "+
					"established", maxTwinCompares)
			}
			continue
		}
		keyed := r.dict["StructParents"] != nil
		if keyed && r.count > twins+1 {
			return Result{
				Verdict: Fail,
				Why: fmt.Sprintf("form XObject (object %d) carries /StructParents and is drawn %d times, so its "+
					"content has more than one place in the structure tree and a reader cannot tell which is "+
					"its parent", nr, r.count),
				Where: r.first + "; and " + r.second,
			}
		}
		// **A twin moves nothing when neither the key nor a drawn form is involved**: fusing two forms that
		// carry no `/StructParents` and draw no other form changes how often an unkeyed form is counted and
		// how often content with no `Do` is traversed, and neither can fail the clause. Measured on veraPDF's
		// corpus, five annotation files hold exactly such twins — identical appearance streams — and were
		// refused before this line and are settled after it, agreeing with veraPDF on every one.
		if twins > 0 && (keyed || d.drawsForms[nr]) && unsure == "" {
			unsure = fmt.Sprintf("form XObject (object %d) is equal to %d other form XObject(s) in the file, which "+
				"pdfcpu fuses into one when it opens it, so how many times each of them is drawn — and so whether "+
				"one carrying /StructParents is drawn twice — is not something nib can count", nr, twins)
		}
	}
	// A definite failure beats a refusal, so both refusals come after the scan.
	if ferr != "" {
		return Result{Verdict: CannotCheck, Why: ferr}
	}
	if unsure != "" {
		return Result{Verdict: CannotCheck, Why: unsure}
	}
	if len(forms) == 0 {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document draws no form XObjects, so none can be drawn twice",
		}
	}
	return Result{Verdict: Pass}
}

// checkEncryptionPermissions evaluates ua1 7.16 t1 (P06.S03).
//
// The profile's test is `P != null && (P & 512) == 512`, and its object is the ENCRYPTION DICTIONARY —
// so an unencrypted document has no subject at all, which is most documents. Bit 10 (value 512) is
// *"extract text and graphics in support of accessibility to users with disabilities"*: a protected
// document that denies it cannot be read aloud.
//
// **nib's own `Encrypt` output fails this clause's PREDICATE**, measured: it never sets `conf.Permissions`,
// so the written `/P` is `-3901` = `0xF0C3`, and `0xF0C3 & 512 == 0`. **No product door reports it**, though
// (the P06 phase-close review): `Encrypt` sets a user password, and `Check`/`CheckForUA` read without one,
// so on that file they stop at the password and emit no report. The failure is therefore a fact about the
// writer that `/pending 640` owns — which permissions "Protect with a password" should grant is a product
// decision with no single right answer — and not something a user of `nib ua` is shown.
func checkEncryptionPermissions(d *Document) Result {
	// **`XRefTable.Encrypt` is a POINTER to an indirect reference**, not an object. Handing the pointer
	// to `dict` resolved nothing, so every encrypted document read as unencrypted — a false pass on
	// veraPDF's own fixture for this clause, caught by the corpus guard.
	ref := d.Ctx.XRefTable.Encrypt
	if ref == nil {
		return Result{
			Verdict: NotApplicable,
			Why:     "the document is not encrypted, so it has no permissions to withhold",
		}
	}
	enc := d.dict(*ref)
	if enc == nil {
		return Result{
			Verdict: CannotCheck,
			Why:     "the trailer names an encryption dictionary that does not resolve, so nib cannot read the permissions",
			Where:   "trailer /Encrypt",
		}
	}
	// **This branch is correct and unreachable through a file, which is declared rather than removed.**
	// pdfcpu reads `/P` once, with `IntEntry`, which matches a direct integer and does not dereference:
	// an encrypted document with no `/P`, an indirect `/P` or a real-valued `/P` fails to OPEN
	// ("unsupported encryption: required entry \"P\" missing"), so `Check` emits no report at all. The
	// verdict here agrees with veraPDF anyway — its `P != null` fails a missing key — and the branch is
	// kept because the day nib reads a document pdfcpu did not validate, it is the honest answer.
	p, isInt := d.intValue(enc["P"])
	if !isInt {
		return Result{
			Verdict: Fail,
			Why: "the encryption dictionary has no /P, so nothing states which permissions the " +
				"document grants",
			Where: "the encryption dictionary",
		}
	}
	if p&512 != 512 {
		return Result{
			Verdict: Fail,
			Why: fmt.Sprintf("the encryption dictionary's /P is %d, whose bit 10 is clear: this "+
				"document denies extracting text and graphics in support of accessibility, so a "+
				"screen reader may not read it", p),
			Where: "the encryption dictionary /P",
		}
	}
	return Result{Verdict: Pass}
}
