package uacheck

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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

// 7.11 t1 — an embedded file's names (P06.S02).
//
// **The conjunction is split four ways, because it is four conditions.** The profile's test is
// `containsEF == false || (F != null && F != ” && UF != null && UF != ”)`, and a fixture that only
// removed `/UF` would leave three of the four unasserted — the shape `code-review` calls an assertion
// narrower than its claim.
func TestAnEmbeddedFilesNamesAreBothPresentAndNonEmpty(t *testing.T) {
	base, err := pdfops.AddAttachment(plainDoc(t), "schedule.csv", []byte("a,b\n1,2\n"))
	if err != nil {
		t.Fatal(err)
	}
	// Control: the product door writes both names, so the unmutated document passes. Without this the
	// four cases below could all be failing for a reason the mutation did not introduce.
	if got := verdictOf(t, base, "7.11 t1"); got.Verdict != Pass {
		t.Fatalf("control: AddAttachment's output reports %v for 7.11 t1 (%s), want Pass", got.Verdict, got.Why)
	}
	for _, c := range []struct {
		name string
		key  string
		to   types.Object
		why  string
	}{
		{"F absent", "F", nil, "no /F"},
		{"UF absent", "UF", nil, "no /UF"},
		{"F empty", "F", types.StringLiteral(""), "EMPTY /F"},
		{"UF empty", "UF", types.StringLiteral(""), "EMPTY /UF"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := verdictOf(t, withSpecKey(t, base, c.key, c.to), "7.11 t1")
			if got.Verdict != Fail {
				t.Fatalf("with %s, 7.11 t1 reports %v (%s), want Fail", c.name, got.Verdict, got.Why)
			}
			if !strings.Contains(got.Why, c.why) {
				t.Errorf("with %s, the refusal is %q, which does not name the condition that failed", c.name, got.Why)
			}
		})
	}
}

// **A file specification with NO embedded file is a passing CHECK, not an absent subject** — the
// profile's first disjunct. A document holding only such specs must answer Pass; answering
// NotApplicable is a strict disagreement with the oracle, and it is what nib did until law 5 caught it
// on three media-clip documents whose clip `/D` is exactly this shape.
func TestASpecificationWithNoEmbeddedFileIsAPassingCheck(t *testing.T) {
	doc := withBareFileSpec(t, plainDoc(t))
	if got := verdictOf(t, doc, "7.11 t1"); got.Verdict != Pass {
		t.Errorf("a document whose only file specification has no /EF reports %v (%s) for 7.11 t1, "+
			"want Pass — it is a check that succeeded, not a subject that is missing", got.Verdict, got.Why)
	}
	// And a document with no file specification at all genuinely has no subject.
	if got := verdictOf(t, plainDoc(t), "7.11 t1"); got.Verdict != NotApplicable {
		t.Errorf("a document with no file specification reports %v (%s), want NotApplicable", got.Verdict, got.Why)
	}
}

// 7.1 t4 — Suspects (P06.S02). `Suspects != true`, so both absences pass.
func TestSuspectsIsOnlyAFailureWhenItIsTrue(t *testing.T) {
	base := plainDoc(t)
	if got := verdictOf(t, base, "7.1 t4"); got.Verdict != Pass {
		t.Fatalf("control: a document with no /MarkInfo reports %v for 7.1 t4 (%s), want Pass", got.Verdict, got.Why)
	}
	if got := verdictOf(t, withSuspects(t, base, false), "7.1 t4"); got.Verdict != Pass {
		t.Errorf("/Suspects false reports %v (%s), want Pass", got.Verdict, got.Why)
	}
	got := verdictOf(t, withSuspects(t, base, true), "7.1 t4")
	if got.Verdict != Fail {
		t.Fatalf("/Suspects true reports %v (%s), want Fail", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "Suspects") {
		t.Errorf("the refusal is %q, which does not name the key", got.Why)
	}
}

// 7.15 t1 — dynamic XFA (P06.S02).
//
// **The fixture writes the element the way veraPDF's own does** — `<dynamicRender\n>` with the newline
// before the closing bracket — because that serialisation is what a `bytes.Contains` search misses,
// and the rule is parsed rather than searched for exactly that reason.
func TestADynamicXFAFormIsRefusedAndAStaticOneIsNot(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	if got := verdictOf(t, base, "7.15 t1"); got.Verdict != NotApplicable {
		t.Fatalf("control: a document with no AcroForm reports %v for 7.15 t1 (%s), want NotApplicable",
			got.Verdict, got.Why)
	}
	got := verdictOf(t, withDynamicXFA(t, base), "7.15 t1")
	if got.Verdict != Fail {
		t.Fatalf("a dynamicRender of required reports %v (%s), want Fail", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "dynamicRender") {
		t.Errorf("the refusal is %q, which does not name what it read", got.Why)
	}
	// **A STATIC XFA form PASSES** — the clause refuses dynamic rendering, not XFA.
	if got := verdictOf(t, withStaticXFA(t, base), "7.15 t1"); got.Verdict != Pass {
		t.Errorf("a dynamicRender of forbidden reports %v (%s), want Pass — the clause refuses "+
			"dynamic rendering, not XFA itself", got.Verdict, got.Why)
	}
}

// **pdfcpu DELETES an `/AcroForm` its validator refuses, and the rule re-reads the file to see it.**
//
// Measured on veraPDF's own `7.15-t01-fail-a.pdf`, the one corpus document that exists to fail this
// clause: the file carries `/AcroForm 2 0 R` with the XFA packet, and after `ReadValidateAndOptimize`
// the catalog has no `/AcroForm` key at all — it keeps `/NeedsRendering`, the marker of exactly the
// form it dropped. Reading only the validated catalog reported NotApplicable, a live false pass on the
// clause's own fixture, and the corpus guard is what caught it.
func TestAnAcroFormPDFCPUDroppedIsStillFound(t *testing.T) {
	path := corpusDir() + "/7.15 XFA/7.15-t01-fail-a.pdf"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("veraPDF's corpus is absent: %v", err)
	}
	d, oerr := open(raw)
	if oerr != nil {
		t.Fatalf("open: %v", oerr)
	}
	// The stimulus, asserted: this test is only meaningful while pdfcpu still drops the key. If a
	// pdfcpu bump starts keeping it, this goes red and the fallback can be reconsidered.
	if _, kept := d.Catalog["AcroForm"]; kept {
		t.Fatal("pdfcpu now KEEPS the /AcroForm on this document, so the unvalidated re-read is no " +
			"longer what makes this clause reachable — re-measure before trusting the fallback")
	}
	if got := registry["7.15 t1"].Check(d); got.Verdict != Fail {
		t.Errorf("7.15 t1 reports %v (%s) on veraPDF's own failing fixture, want Fail", got.Verdict, got.Why)
	}
}

// **An UNTYPED dictionary carrying an embedded file is still a file specification** (P06.S02).
//
// Measured against veraPDF on a hand-built document: a dictionary with `/EF` and no `/Type /Filespec`
// is graded exactly as a typed one is, and fails when a name is missing. Keying the population on
// `/Type` alone would pass it — and nothing else in this package reaches that case, because every
// fixture's specification is typed. Probed: dropping the `/EF` half of the population predicate left
// the whole package green until this test existed.
//
// **It is built in memory because pdfcpu refuses the state through a file**: its validator rejects the
// document outright with `dict=fileSpecDict required entry=Type missing`, so a fixture written to disk
// never reaches any rule. That is a fact about the DEPENDENCY rather than about nib — veraPDF reads
// the same bytes and grades them — and `openMutated` exists for exactly this shape.
func TestAnUntypedDictionaryCarryingAnEmbeddedFileIsASpecification(t *testing.T) {
	base, err := pdfops.AddAttachment(plainDoc(t), "schedule.csv", []byte("a,b\n1,2\n"))
	if err != nil {
		t.Fatal(err)
	}
	untype := func(d *Document) int {
		specs, serr := d.fileSpecs()
		if serr != "" {
			t.Fatalf("the specification walk was short: %s", serr)
		}
		n := 0
		for _, sp := range specs {
			if _, has := sp.dict["Type"]; has {
				delete(sp.dict, "Type")
				n++
			}
		}
		// Reset the memoised population so the rule re-walks and sees the untyped shape.
		d.specList, d.specsDone, d.specsErr = nil, false, ""
		return n
	}

	// Complete but untyped: still a subject, so it PASSES rather than vanishing.
	d := openMutated(t, base, func(_ *Document, _ types.Dict) {})
	if n := untype(d); n == 0 {
		t.Fatal("setup: no specification carried a /Type to remove, so the untyped case is never reached")
	}
	if got := registry["7.11 t1"].Check(d); got.Verdict != Pass {
		t.Fatalf("an untyped specification with both names reports %v (%s), want Pass", got.Verdict, got.Why)
	}

	// Untyped AND missing a name: still a subject, and it FAILS — which is what makes it a subject
	// rather than something the walk skipped.
	d2 := openMutated(t, base, func(_ *Document, _ types.Dict) {})
	untype(d2)
	specs, _ := d2.fileSpecs()
	for _, sp := range specs {
		delete(sp.dict, "UF")
	}
	if got := registry["7.11 t1"].Check(d2); got.Verdict != Fail {
		t.Errorf("an untyped specification missing /UF reports %v (%s), want Fail — it is a "+
			"specification whether or not it says so", got.Verdict, got.Why)
	}
}

// **A specification written as a DIRECT dictionary, inside a direct dictionary, inside an array, is
// still a subject** (P06.S02) — measured against veraPDF on a hand-built document where the only
// defective spec is an annotation's `/FS`, with neither the annotation nor the spec an object of its
// own.
//
// It needs its own test because the walk reaches an INDIRECT annotation from the object table without
// ever descending an array, so every other fixture here leaves that descent unexercised: probed,
// removing the array case entirely left the package green.
func TestADirectSpecificationInsideAnArrayIsFound(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	d := openMutated(t, base, func(_ *Document, page types.Dict) {
		// Nothing here is an indirect object: the array holds a dictionary that holds the spec.
		page["Annots"] = types.Array{types.Dict{
			"Type":    types.Name("Annot"),
			"Subtype": types.Name("FileAttachment"),
			"Rect":    types.NewNumberArray(50, 50, 70, 70),
			"F":       types.Integer(4),
			"FS": types.Dict{
				"Type": types.Name("Filespec"),
				"F":    types.StringLiteral("direct.txt"),
				"EF":   types.Dict{"F": types.StringLiteral("stand-in")},
			},
		}}
	})
	// The stimulus, asserted: the walk must actually find it, or the verdict below says nothing.
	specs, serr := d.fileSpecs()
	if serr != "" {
		t.Fatalf("the specification walk was short: %s", serr)
	}
	found := false
	for _, sp := range specs {
		if s, _ := d.text(sp.dict["F"]); s == "direct.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the direct specification was not reached at all (%d spec(s) found), so the walk "+
			"descends neither arrays nor nested dictionaries", len(specs))
	}
	if got := registry["7.11 t1"].Check(d); got.Verdict != Fail {
		t.Errorf("a direct specification with /EF and no /UF reports %v (%s), want Fail", got.Verdict, got.Why)
	}
}

// **A specification with no embedded file must not stop the scan** (P06.S02, found by a blind
// mutation pass). Replacing the `continue` with a `break` left the whole package green, because no
// fixture put an EF-less specification BEFORE a defective one — and since the population is walked in
// object-number order, which one comes first is a property of the document rather than of the test.
func TestAnEmbeddedFilelessSpecificationDoesNotStopTheScan(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	spec := func(name string, ef bool) types.Dict {
		d := types.Dict{"Type": types.Name("Filespec"), "F": types.StringLiteral(name)}
		if ef {
			// Carries an embedded file and NO /UF — the defect.
			d["EF"] = types.Dict{"F": types.StringLiteral("stand-in")}
		}
		return d
	}
	annot := func(fs types.Dict) types.Dict {
		return types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("FileAttachment"),
			"Rect": types.NewNumberArray(50, 50, 70, 70), "F": types.Integer(4), "FS": fs,
		}
	}
	// **Both are DIRECT dictionaries in one array**, so the order the walk sees them in is the array's
	// own — not a function of object numbers, which a fixture cannot control.
	d := openMutated(t, base, func(_ *Document, page types.Dict) {
		page["Annots"] = types.Array{annot(spec("bare.csv", false)), annot(spec("broken.csv", true))}
	})
	// The stimulus, asserted: the bare specification really does come first, or `break` and `continue`
	// behave identically here and the probe proves nothing.
	specs, serr := d.fileSpecs()
	if serr != "" {
		t.Fatalf("the specification walk was short: %s", serr)
	}
	if len(specs) < 2 {
		t.Fatalf("setup: %d specification(s) found, want both", len(specs))
	}
	if _, hasEF := specs[0].dict["EF"]; hasEF {
		t.Fatalf("setup: the FIRST specification carries an /EF, so nothing is stepped past")
	}
	got := registry["7.11 t1"].Check(d)
	if got.Verdict != Fail {
		t.Errorf("7.11 t1 reports %v (%s); a specification with no embedded file stopped the scan "+
			"before the defective one behind it", got.Verdict, got.Why)
	}
}

// **The XFA value is trimmed at BOTH ends** (P06.S02, found by a blind mutation pass). veraPDF's own
// fixture puts the newline before the closing bracket; a producer that puts it after the text instead
// yields `"required\n"`, which never equals `"required"` — and a dynamic form reads as Pass. Trimming
// only the left survived the whole package.
func TestTheXFAValueIsTrimmedAtBothEnds(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	for _, packet := range []string{
		"<config><acrobat><acrobat7><dynamicRender>required\n</dynamicRender></acrobat7></acrobat></config>",
		"<config><acrobat><acrobat7><dynamicRender>\n  required  \n</dynamicRender></acrobat7></acrobat></config>",
	} {
		if got := verdictOf(t, withXFAConfig(t, base, packet), "7.15 t1"); got.Verdict != Fail {
			t.Errorf("a dynamicRender of %q reports %v (%s), want Fail — the value is the element's "+
				"text, not its whitespace", packet, got.Verdict, got.Why)
		}
	}
}

// **A refusal survives being asked twice** (P06.S02, found by a blind mutation pass). The population
// is memoised, and returning a clean answer on the second ask would mean the first rule to look says
// CannotCheck while every rule after it silently Passes the same truncated document. Only one clause
// reads this door today, so the hole is latent — which is exactly the kind that is cheap now and
// invisible later.
func TestTheSpecificationRefusalSurvivesMemoisation(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	d := openMutated(t, base, func(_ *Document, page types.Dict) {
		// A chain deeper than the walk's bound, so the population is knowingly short.
		deep := types.Dict{}
		cur := deep
		for i := 0; i < maxWalkDepth+5; i++ {
			next := types.Dict{}
			cur["K"] = next
			cur = next
		}
		page["Nested"] = deep
	})
	_, first := d.fileSpecs()
	if first == "" {
		t.Fatal("setup: the walk finished, so there is no refusal to preserve")
	}
	// **Asserted as non-empty, not as equal.** `specsErr` is write-once, so removing the memo makes
	// the second call re-walk and return the SAME string — an equality check could only catch a
	// mutation that CLEARS the error, not one that drops it. What matters to a second reader is that
	// a refusal is still there at all.
	if _, second := d.fileSpecs(); second == "" {
		t.Errorf("asked twice, the door returned %q then nothing — a second reader would grade a "+
			"population it was told was short", first)
	}
	if got := registry["7.11 t1"].Check(d); got.Verdict != CannotCheck {
		t.Errorf("7.11 t1 reports %v (%s) over a population nib knows is short, want CannotCheck", got.Verdict, got.Why)
	}
}

// **A non-boolean `/Suspects` is not the boolean true, so it PASSES** (P06.S02).
//
// Measured: veraPDF passes `/Suspects (true)` written as a STRING, `passedChecks="1"` — the profile's
// `Suspects != true` means the boolean and nothing else. **nib cannot open such a document**, because
// pdfcpu's `validateBooleanEntry` errors rather than deleting, so `Check` emits no report at all where
// veraPDF answers Pass. That divergence is outside the clause and is declared in the plan; the branch
// itself is reached here the way this package always reaches a state pdfcpu refuses — in memory.
func TestANonBooleanSuspectsIsNotTheBooleanTrue(t *testing.T) {
	base := plainDoc(t)
	for _, v := range []types.Object{
		types.StringLiteral("true"), types.Name("true"), types.Integer(1),
	} {
		d := openMutated(t, base, func(d *Document, _ types.Dict) {
			mi := d.dict(d.Catalog["MarkInfo"])
			if mi == nil {
				mi = types.Dict{}
				d.Catalog["MarkInfo"] = mi
			}
			mi["Suspects"] = v
		})
		// The stimulus, asserted: the key really is there and really is not a boolean.
		mi := d.dict(d.Catalog["MarkInfo"])
		if _, isBool := d.boolValue(mi["Suspects"]); isBool {
			t.Fatalf("setup: %v reads as a boolean, so the non-boolean branch is never reached", v)
		}
		if got := registry["7.1 t4"].Check(d); got.Verdict != Pass {
			t.Errorf("/Suspects %v reports %v (%s), want Pass — only the boolean true fails this clause",
				v, got.Verdict, got.Why)
		}
	}
}

// **A `/F` that is not a string names nothing, and veraPDF fails it** (P06.S02).
//
// Measured on a hand-built document: `/F /probe.txt` — a NAME rather than a string — is FAILED by
// veraPDF (`failedChecks="2"`). nib fails it too. Like the untyped case, pdfcpu refuses the document
// outright (`decodeString: dict=fileSpecDict entry=F invalid type types.Name`), so the branch is
// reached in memory. It is the fifth condition of a test whose comment claims four, and nothing
// reached it before this test existed.
func TestANonStringNameOnASpecificationIsAFailure(t *testing.T) {
	base, err := pdfops.AddAttachment(plainDoc(t), "schedule.csv", []byte("a,b\n1,2\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"F", "UF"} {
		d := openMutated(t, base, func(d *Document, _ types.Dict) {})
		specs, serr := d.fileSpecs()
		if serr != "" {
			t.Fatalf("the specification walk was short: %s", serr)
		}
		changed := 0
		for _, sp := range specs {
			if _, hasEF := sp.dict["EF"]; hasEF {
				sp.dict[key] = types.Name("probe.txt")
				changed++
			}
		}
		if changed == 0 {
			t.Fatalf("setup: no specification carries an /EF, so /%s was never replaced", key)
		}
		got := registry["7.11 t1"].Check(d)
		if got.Verdict != Fail {
			t.Fatalf("a name-valued /%s reports %v (%s), want Fail", key, got.Verdict, got.Why)
		}
		if !strings.Contains(got.Why, "not a string") {
			t.Errorf("the refusal is %q, which does not say the value names nothing", got.Why)
		}
	}
}

// **An empty `/XFA []` PASSES**, and it reaches the rule through the VALIDATED catalog (P06.S02).
//
// pdfcpu accepts an empty XFA array, and a non-empty `/Fields` keeps the AcroForm alive, so this is a
// document the checker really sees. `dynamicRender` is then null and `null != 'required'` — refusing
// it would be a CannotCheck where veraPDF passes.
func TestAnEmptyXFAArrayPasses(t *testing.T) {
	base, err := pdfops.AuthorForm(plainDoc(t), []pdfops.FormField{{
		Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Name"}})
	if err != nil {
		t.Fatal(err)
	}
	d := openMutated(t, base, func(d *Document, _ types.Dict) {
		form := d.dict(d.Catalog["AcroForm"])
		if form == nil {
			t.Fatal("setup: AuthorForm's output has no AcroForm to give an /XFA")
		}
		form["XFA"] = types.Array{}
	})
	// The stimulus, asserted: this must be the VALIDATED path, or it measures the re-read again.
	if _, kept := d.Catalog["AcroForm"]; !kept {
		t.Fatal("setup: the AcroForm is not in the validated catalog, so this exercises the fallback")
	}
	if got := registry["7.15 t1"].Check(d); got.Verdict != Pass {
		t.Errorf("an empty /XFA reports %v (%s), want Pass — nothing declares dynamicRender", got.Verdict, got.Why)
	}
}

// **A packet nib's XML parser rejects does not refuse the form** (P06.S02).
//
// Go's `encoding/xml` rejects several things a Java parser accepts and XFA templates routinely carry:
// `encoding="ISO-8859-1"`, `version="1.1"`, and XHTML rich text with `&nbsp;`. Refusing the clause
// because the `template` packet has a named entity would report "could not check" for ordinary forms
// veraPDF passes, so each packet is tried and the first value found wins.
func TestAnUnparseableXFAPacketDoesNotRefuseTheForm(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// The unreadable packet comes FIRST, so a reader that aborts on it never sees the answer.
	d := verdictOf(t, withXFAPackets(t, base,
		"<template><p>a&nbsp;b</p></template>",
		"<config><acrobat><acrobat7><dynamicRender>required</dynamicRender></acrobat7></acrobat></config>",
	), "7.15 t1")
	if d.Verdict != Fail {
		t.Errorf("with an unparseable template packet before the config packet, 7.15 t1 reports %v "+
			"(%s), want Fail — the config packet answered the question", d.Verdict, d.Why)
	}
	// But when NO packet answers and one could not be read, the honest answer is a refusal.
	only := verdictOf(t, withXFAPackets(t, base, "<template><p>a&nbsp;b</p></template>"), "7.15 t1")
	if only.Verdict != CannotCheck {
		t.Errorf("with only an unparseable packet, 7.15 t1 reports %v (%s), want CannotCheck — nothing "+
			"declared the setting and nib could not read the one packet there was", only.Verdict, only.Why)
	}
}

// **The element's text survives being split, and a nested element of the same name does not collapse
// the reading** (P06.S02). A counter merely SET on the opening tag lost both: `requi<!--x-->red` read
// as `requi`, and a nested `dynamicRender` ended the outer one early — each a Pass on a dynamic form.
func TestTheDynamicRenderTextIsReadWhole(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	for _, packet := range []string{
		"<config><dynamicRender>requi<!--x-->red</dynamicRender></config>",
		"<config><dynamicRender><x><dynamicRender/></x>required</dynamicRender></config>",
		"<config><dynamicRender>requi<?pi?>red</dynamicRender></config>",
	} {
		if got := verdictOf(t, withXFAConfig(t, base, packet), "7.15 t1"); got.Verdict != Fail {
			t.Errorf("packet %q reports %v (%s), want Fail — the element's text is `required`",
				packet, got.Verdict, got.Why)
		}
	}
}

// **A specification on a form XObject's `/AF` is a subject, and a form XObject is a STREAM** (P06.S02).
//
// `types.StreamDict` embeds `types.Dict` rather than being one, so a type switch with a `Dict` case and
// no `StreamDict` case walks neither the stream's dictionary nor anything under it. Measured: veraPDF
// FAILS a defective specification hung off a form XObject's `/AF`, and nib found nothing there — a
// false pass on a holder this slice had already measured and written into its own table. The holder
// list was right; it was missed by TYPE.
//
// The indirect spelling survives such a bug by accident, because the object-table loop finds the
// specification independently, so this fixture writes the specification DIRECTLY on the stream.
func TestASpecificationOnAFormXObjectIsFound(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// **Built in memory, because a WRITE defeats the fixture.** Round-tripping through
	// `api.WriteContext` hoists the direct `/AF` dictionary into an object of its own, and the
	// object-table loop then finds it whatever the stream case does — the first version of this test
	// passed with the `StreamDict` case removed for exactly that reason.
	d := openMutated(t, base, func(d *Document, page types.Dict) {
		sd, serr := d.Ctx.NewStreamDictForBuf([]byte(""))
		if serr != nil {
			t.Fatal(serr)
		}
		sd.Dict["Type"] = types.Name("XObject")
		sd.Dict["Subtype"] = types.Name("Form")
		sd.Dict["BBox"] = types.NewNumberArray(0, 0, 10, 10)
		sd.Dict["AF"] = types.Array{types.Dict{
			"Type": types.Name("Filespec"),
			"F":    types.StringLiteral("on-a-form.txt"),
			"EF":   types.Dict{"F": types.StringLiteral("stand-in")},
		}}
		ref, rerr := d.Ctx.IndRefForNewObject(*sd)
		if rerr != nil {
			t.Fatal(rerr)
		}
		res, _ := d.Ctx.DereferenceDict(page["Resources"])
		if res == nil {
			res = types.Dict{}
			page["Resources"] = res
		}
		res["XObject"] = types.Dict{"X0": *ref}
	})
	// The stimulus, asserted: the specification must be reached, and reached THROUGH the stream rather
	// than as an object of its own, or the stream case is not what is under test.
	specs, serr := d.fileSpecs()
	if serr != "" {
		t.Fatalf("the specification walk was short: %s", serr)
	}
	found := false
	for _, sp := range specs {
		if s, _ := d.text(sp.dict["F"]); s == "on-a-form.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the specification on the form XObject was not reached (%d spec(s) found) — a stream "+
			"dictionary is a dictionary and must be walked as one", len(specs))
	}
	if got := registry["7.11 t1"].Check(d); got.Verdict != Fail {
		t.Errorf("7.11 t1 reports %v (%s) for a specification on a form XObject, want Fail", got.Verdict, got.Why)
	}
}

// **The walk does not follow indirect references, and these are the two documents that say why**
// (P06.S02, both found by review and both measured before and after).
//
// Every object in the table is visited by the door's own loop, so following a reference from inside
// one object only reaches what the loop reaches anyway — while making both of these possible.
func TestTheSpecificationWalkIsNotDefeatedByTheObjectGraph(t *testing.T) {
	// A file specification with a defect, so both documents have something real to find.
	spec := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 5 0 R /Names 8 0 R %s >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		5: "<< /Type /StructTreeRoot /K [] >>",
		6: "<< /Type /Filespec /F (probe.txt) /EF << /F 7 0 R >> >>",
		7: "<< /Length 5 >>\nstream\nhello\nendstream",
		8: "<< /EmbeddedFiles << /Names [(probe.txt) 6 0 R] >> >>",
	}
	build := func(extra string, objs map[int]string) []byte {
		out := map[int]string{}
		for k, v := range spec {
			out[k] = v
		}
		out[1] = fmt.Sprintf(spec[1], extra)
		for k, v := range objs {
			out[k] = v
		}
		return buildPDF(out)
	}

	// **Exponential if arrays were re-walked.** Each object holds the previous one TWICE, so following
	// references would visit 2^n paths; measured before the fix, n=21 took 198 ms from a 3 KB file and
	// n=40 would have taken about thirty hours. The budget could not fire because only dictionaries
	// spent it.
	const n = 30
	exp := map[int]string{20: "[]"}
	for i := 1; i <= n; i++ {
		exp[20+i] = fmt.Sprintf("[%d 0 R %d 0 R]", 19+i, 19+i)
	}
	done := make(chan Result, 1)
	go func() { done <- verdictOf(t, build(fmt.Sprintf("/ZZ %d 0 R", 20+n), exp), "7.11 t1") }()
	select {
	case got := <-done:
		if got.Verdict != Fail {
			t.Errorf("the doubling document reports %v (%s), want Fail — the defective specification "+
				"is still there to find", got.Verdict, got.Why)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the doubling document did not finish in 30s — the walk is following references again")
	}

	// **A refusal on an ordinary document.** Depth counted THROUGH references, so any long chain of
	// linked dictionaries exhausted the bound — and an outline is exactly that chain, so a document
	// with sixty-four bookmarks made the whole clause CannotCheck.
	chain := map[int]string{}
	for i := 0; i < 70; i++ {
		next := ""
		if i < 69 {
			next = fmt.Sprintf(" /Next %d 0 R", 21+i)
		}
		chain[20+i] = fmt.Sprintf("<< /Type /Bookmark%s >>", next)
	}
	if got := verdictOf(t, build("/ZZ 20 0 R", chain), "7.11 t1"); got.Verdict != Fail {
		t.Errorf("a document with a seventy-long chain of linked dictionaries reports %v (%s), want "+
			"Fail — the chain is not nesting and must not spend the depth bound", got.Verdict, got.Why)
	}
}

// 7.20 t1 — reference XObjects (P06.S03).
//
// **The population is what the document DRAWS, not what it holds**, and that distinction is the whole
// test. Measured on veraPDF: a form XObject sitting in a page's `/Resources /XObject` that nothing
// draws is reported `0 passed / 0 failed` — no subject — while the same form drawn by a `/X0 Do` is
// FAILED. An object-graph population would report a failure veraPDF does not, which for a checker is
// as bad as missing one.
func TestOnlyADrawnFormXObjectIsASubject(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	// Drawn, carrying /Ref: a definite failure.
	drawn := withReferenceXObject(t, base)
	got := verdictOf(t, drawn, "7.20 t1")
	if got.Verdict != Fail {
		t.Fatalf("a drawn form carrying /Ref reports %v (%s), want Fail", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "/Ref") {
		t.Errorf("the refusal is %q, which does not name the key it read", got.Why)
	}
	// The same form, NOT drawn: no subject.
	undrawn := withUndrawnReferenceXObject(t, base)
	// The stimulus, asserted: the form really is in the document, just not drawn.
	d, oerr := open(undrawn)
	if oerr != nil {
		t.Fatalf("open: %v", oerr)
	}
	refs := 0
	d.eachObjectDict(func(dict types.Dict, _ string) {
		if _, has := dict["Ref"]; has && d.name(dict["Subtype"]) == "Form" {
			refs++
		}
	})
	if refs == 0 {
		t.Fatal("setup: the undrawn fixture carries no form with /Ref, so it measures nothing")
	}
	if v := registry["7.20 t1"].Check(d); v.Verdict != NotApplicable {
		t.Errorf("an UNDRAWN form carrying /Ref reports %v (%s), want NotApplicable — veraPDF "+
			"instantiates no PDXForm for a form nothing draws", v.Verdict, v.Why)
	}
}

// **An appearance stream IS a form XObject**, and nothing draws it with a `Do` — the annotation is
// what puts it on the page, so it is recorded where the appearance walk enters it rather than at the
// operator. Measured: veraPDF FAILS a widget whose `/AP /N` carries `/Ref`.
func TestAnAppearanceStreamIsAFormXObject(t *testing.T) {
	base, err := pdfops.AuthorForm(plainDoc(t), []pdfops.FormField{{
		Page: 1, Rect: [4]float64{100, 700, 300, 720}, Kind: "text", Name: "n", Label: "Name"}})
	if err != nil {
		t.Fatal(err)
	}
	// The control: a form document's widget appearances are forms, so the clause has a SUBJECT and
	// passes. Without this the failure below could come from an empty population.
	if got := verdictOf(t, base, "7.20 t1"); got.Verdict != Pass {
		t.Fatalf("control: a form document reports %v (%s) for 7.20 t1, want Pass — its widget "+
			"appearance streams are form XObjects", got.Verdict, got.Why)
	}
	if got := verdictOf(t, withRefOnAppearance(t, base), "7.20 t1"); got.Verdict != Fail {
		t.Errorf("a widget whose appearance carries /Ref reports %v (%s), want Fail", got.Verdict, got.Why)
	}
}

// 7.16 t1 — the encryption permission bit (P06.S03).
//
// Bit 10 (value 512) is *"extract text and graphics in support of accessibility"*. The subject is the
// ENCRYPTION DICTIONARY, so an unencrypted document has none — which is most documents.
func TestAnEncryptedDocumentMustAllowAccessibleExtraction(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	if got := verdictOf(t, base, "7.16 t1"); got.Verdict != NotApplicable {
		t.Fatalf("an unencrypted document reports %v (%s), want NotApplicable — it has no permissions "+
			"to withhold", got.Verdict, got.Why)
	}
	if got := verdictOf(t, withEncryption(t, base, model.PermissionsAll), "7.16 t1"); got.Verdict != Pass {
		t.Errorf("an encrypted document granting everything reports %v (%s), want Pass", got.Verdict, got.Why)
	}
	got := verdictOf(t, withEncryption(t, base, model.PermissionsNone), "7.16 t1")
	if got.Verdict != Fail {
		t.Fatalf("an encrypted document granting nothing reports %v (%s), want Fail", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "bit 10") || !strings.Contains(got.Why, "-3901") {
		t.Errorf("the refusal is %q, which does not name the bit or quote the /P a reader must change", got.Why)
	}
}

// **nib's own "Protect with a password" output FAILS this clause, and that is recorded rather than
// fixed** (P06.S03). `pdfops.Encrypt` never sets `conf.Permissions`, so the written `/P` is `-3901` and
// bit 10 is clear: a document nib protected cannot be read aloud.
//
// Which permissions that door should grant is a product decision with no single right answer, so the
// checker says so and `/pending 640` owns the writer. **This test is the standing evidence**: if
// `Encrypt` ever starts granting bit 10, it goes red and the claim in the plan and in `/pending 640`
// has to be re-read rather than quietly surviving.
func TestNibsOwnProtectedOutputFailsTheAccessibilityPermission(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	protected, err := pdfops.Encrypt(base, "a-password")
	if err != nil {
		t.Fatal(err)
	}
	d, oerr := openEncrypted(t, protected, "a-password")
	if oerr != "" {
		t.Skipf("nib cannot re-open its own protected output without the password here: %s", oerr)
	}
	got := registry["7.16 t1"].Check(d)
	if got.Verdict != Fail {
		t.Errorf("nib's own protected output reports %v (%s) for 7.16 t1, want Fail — `Encrypt` sets "+
			"no permissions, so /P is -3901 and bit 10 is clear. If this has changed, /pending 640 and "+
			"P06's phase-open note both need re-reading", got.Verdict, got.Why)
	}
}

// **The bit is 10 and nothing else** (P06.S03, found by probing). `PermissionsAll` sets every bit and
// `PermissionsNone` clears every bit, so a fixture built from those two cannot tell bit 10 from any
// other — probed, reading bit 9 instead left the whole package green. These values isolate it.
func TestTheAccessibilityPermissionIsBitTenExactly(t *testing.T) {
	base, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	enc := withEncryption(t, base, model.PermissionsNone)
	for _, c := range []struct {
		p    int
		want Verdict
		why  string
	}{
		// Bit 10 alone set, every other permission denied: the clause is satisfied.
		{512, Pass, "bit 10 is the only bit this clause reads"},
		// Bit 9 set, bit 10 clear — the mutation that survived until this test existed.
		{256, Fail, "bit 9 is not bit 10"},
		// Bit 11 set, bit 10 clear.
		{1024, Fail, "bit 11 is not bit 10"},
		// Everything EXCEPT bit 10. `-3901 &^ 512` would have been the same value as `-3901` — bit 10
		// is already clear there — so that case repeated a fixture two tests above and measured nothing.
		{-1 &^ 512, Fail, "clearing bit 10 alone fails, however many other bits are set"},
		// Everything including bit 10.
		{-1, Pass, "all permissions granted includes bit 10"},
	} {
		d, oerr := openEncrypted(t, enc, "")
		if oerr != "" {
			t.Fatalf("open: %s", oerr)
		}
		ed := d.dict(*d.Ctx.XRefTable.Encrypt)
		if ed == nil {
			t.Fatal("setup: the encryption dictionary does not resolve, so no /P can be set")
		}
		ed["P"] = types.Integer(c.p)
		if got := registry["7.16 t1"].Check(d); got.Verdict != c.want {
			t.Errorf("/P = %d reports %v (%s), want %v — %s", c.p, got.Verdict, got.Why, c.want, c.why)
		}
	}
}

// **A form an OUTER form draws is a subject; one merely sitting in the outer's resources is not**
// (P06.S03). Measured on veraPDF: the nested-and-drawn document reports 1 passed and 1 failed — the
// outer form passes, the inner fails — while the nested-but-undrawn document reports 1 passed and 0
// failed, the outer alone. nib agrees on both.
//
// This is the same drawn/undrawn distinction one level down, and no other fixture covers nesting.
func TestANestedFormIsASubjectOnlyWhenTheOuterDrawsIt(t *testing.T) {
	for _, c := range []struct {
		name  string
		inner string
		want  Verdict
	}{
		{"the outer form draws the inner", "/Y0 Do", Fail},
		{"the outer form merely holds it", "", Pass},
	} {
		t.Run(c.name, func(t *testing.T) {
			objs := map[int]string{
				1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 5 0 R >>",
				2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
				3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R " +
					"/Resources << /XObject << /X0 10 0 R >> >> >>",
				4: "<< /Length 6 >>\nstream\n/X0 Do\nendstream",
				5: "<< /Type /StructTreeRoot /K [] >>",
				9: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Ref << /F << /Type /Filespec " +
					"/F (ext.pdf) >> /Page 0 >> /Length 0 >>\nstream\n\nendstream",
				10: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources "+
					"<< /XObject << /Y0 9 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(c.inner), c.inner),
			}
			if got := verdictOf(t, buildPDF(objs), "7.20 t1"); got.Verdict != c.want {
				t.Errorf("%s: 7.20 t1 reports %v (%s), want %v — measured on veraPDF", c.name, got.Verdict, got.Why, c.want)
			}
		})
	}
}

// **All three appearance states are graded, not just `/N`** (P06.S03). Measured: a widget whose
// `/D` (down) appearance carries `/Ref` is FAILED by veraPDF, and so is one whose `/R` (rollover)
// does — each reporting 1 passed and 1 failed, the clean `/N` passing beside the defective state.
// nib walks `N`, `R` and `D`, and until this test nothing said whether it should.
func TestEveryAppearanceStateIsAFormXObject(t *testing.T) {
	for _, state := range []string{"N", "R", "D"} {
		objs := map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 5 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R "+
				"/Resources << >> /Annots [<< /Type /Annot /Subtype /Widget /FT /Btn /T (b) "+
				"/Rect [0 0 10 10] /F 4 /AP << /N 11 0 R /%s 9 0 R >> >>] >>", state),
			4: "<< /Length 0 >>\nstream\n\nendstream",
			5: "<< /Type /StructTreeRoot /K [] >>",
			9: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Ref << /F << /Type /Filespec " +
				"/F (ext.pdf) >> /Page 0 >> /Length 0 >>\nstream\n\nendstream",
			11: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 0 >>\nstream\n\nendstream",
		}
		if state == "N" {
			// /N is both the clean one and the defective one here; keep one entry.
			objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R " +
				"/Resources << >> /Annots [<< /Type /Annot /Subtype /Widget /FT /Btn /T (b) " +
				"/Rect [0 0 10 10] /F 4 /AP << /N 9 0 R >> >>] >>"
		}
		if got := verdictOf(t, buildPDF(objs), "7.20 t1"); got.Verdict != Fail {
			t.Errorf("a widget whose /%s appearance carries /Ref reports %v (%s), want Fail — "+
				"measured on veraPDF, which fails all three states", state, got.Verdict, got.Why)
		}
	}
}
