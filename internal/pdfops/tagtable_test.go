package pdfops

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/png"
	"os"
	"sort"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// The tag-fate table and its guard — `PLAN-accessibility.md` P01.S03, law 2.
//
// # The law
//
// **Every operation declares its tag fate.** One verdict per operation, in one table, and the guard
// asserts both that every operation *has* one and that the one it has is **true**.
//
// # Three assertions, and they are different claims
//
//  1. **Completeness.** Every document-touching operation in this package appears in the table.
//     Enumerated from the CODE with go/ast — not from a hand-written list, which is the acceptance
//     criterion and also the only version that can catch an operation added tomorrow.
//  2. **Correctness — and this half did not exist until 2026-09-11.** For every operation the guard
//     can drive, the MEASURED fate must equal the declared one. Without it the table was a list of
//     assertions nothing checked, and **19 of its 33 rows were wrong**: every one said `dropped`
//     while the operation carried the tree intact. A census that cannot be wrong is not a census.
//  3. **Law 1 in force.** No driven operation may emit an `orphaned` output — a tagging claim over
//     a tree that describes nothing in the document. This is asserted over the whole population
//     rather than at the one door that fixes it, so an operation that starts lying goes red whether
//     or not anybody remembered to route it through `honest`.
//
// # What it cannot do, said rather than implied
//
// Not every operation can be driven from a table: some need a password, an attachment name, a font,
// a raster map. Those declare a verdict and a **reason they are not driven**, which the guard
// requires to be non-empty — so an undriven operation is a recorded gap rather than a silent one.
// The completeness half covers all of them regardless, and that is the half law 2 is about.

// tagFate is one operation's declared verdict.
type tagFate struct {
	// verdict is one of:
	//
	//   - `carried`   — the claim and a live tree both survive, reaching every page with content.
	//   - `dropped`   — the claim goes with the content it described. Honest: a visible loss.
	//   - `partial`   — the claim and a live tree survive, but do not reach every page with content.
	//   - `untouched` — the operation cannot affect the claim (it returns a report, not a document).
	//
	// **`orphaned` is not a fate an operation may declare.** It is law 1's violation — a claim over
	// a tree that describes nothing in the document — and the guard fails it rather than recording
	// it. `NUp` produced exactly that state and now routes through `honest`.
	verdict string
	// why is required when the operation is not driven below: an undriven operation is a declared
	// gap, never a silent one.
	why string
	// drive runs the operation on a tagged fixture. Nil means "not driven" and then `why` must say so.
	drive func([]byte) ([]byte, error)
}

// tagFates is the table. Every document-touching operation in this package must appear.
//
// **Every driven verdict below was MEASURED, not reasoned.** The same measurement was taken twice —
// against the hand-built fixture this file drives, and against a 4-page LibreOffice-produced
// document with 45 struct elements — and the two agree operation for operation. The LibreOffice
// figures are recorded at `PLAN-accessibility.md` D9.
var tagFates = map[string]tagFate{
	// ── CARRIED. The tree survives intact. Every one of these was declared `dropped` until the
	// census was measured rather than asserted; the byte count that produced that declaration could
	// not see a compressed object stream, so it reported a carried tree as an empty one.
	"Rotate":              {verdict: "carried", drive: func(b []byte) ([]byte, error) { return Rotate(b, nil, 90) }},
	"Optimize":            {verdict: "carried", drive: func(b []byte) ([]byte, error) { return Optimize(b) }},
	"NormalizePageSizes":  {verdict: "carried", drive: func(b []byte) ([]byte, error) { return NormalizePageSizes(b) }},
	"SetLang":             {verdict: "carried", drive: func(b []byte) ([]byte, error) { return SetLang(b, "en-GB") }},
	"StripMetadata":       {verdict: "carried", drive: func(b []byte) ([]byte, error) { return StripMetadata(b) }},
	"StripActive":         {verdict: "carried", drive: func(b []byte) ([]byte, error) { return StripActive(b) }},
	"RemoveFilesAndMedia": {verdict: "carried", drive: func(b []byte) ([]byte, error) { return RemoveFilesAndMedia(b) }},
	"ClearFlags":          {verdict: "carried", drive: func(b []byte) ([]byte, error) { return ClearFlags(b) }},
	"SetOutline":          {verdict: "carried", drive: func(b []byte) ([]byte, error) { return SetOutline(b, []OutlineItem{{Title: "x", Page: 1}}) }},
	"SetPageLabels": {verdict: "carried", drive: func(b []byte) ([]byte, error) {
		return SetPageLabels(b, []PageLabelRange{{Start: 1, Style: "decimal"}})
	}},
	"StampPageNumbers": {verdict: "carried", drive: func(b []byte) ([]byte, error) { return StampPageNumbers(b, PageNumberStyle{}) }},
	"StampWatermark":   {verdict: "carried", drive: func(b []byte) ([]byte, error) { return StampWatermark(b, "DRAFT", WatermarkStyle{}) }},
	"AddNotes":         {verdict: "carried", drive: func(b []byte) ([]byte, error) { return AddNotes(b, []Note{{Page: 1, X: 10, Y: 10, Text: "n"}}) }},
	"AddAttachment":    {verdict: "carried", drive: func(b []byte) ([]byte, error) { return AddAttachment(b, "a.txt", []byte("hi")) }},
	"StampImages": {verdict: "carried", drive: func(b []byte) ([]byte, error) {
		return StampImages(b, []Stamp{{Page: 1, Rect: [4]float64{10, 10, 60, 60}, PNG: onePixelPNG()}})
	}},
	"SetFlags": {verdict: "carried", drive: func(b []byte) ([]byte, error) { return SetFlags(b, []byte(`{"a":1}`)) }},
	"AuthorForm": {verdict: "carried", drive: func(b []byte) ([]byte, error) {
		return AuthorForm(b, []FormField{{Page: 1, Rect: [4]float64{10, 10, 200, 40}, Kind: "text", Name: "f1"}})
	}},
	"StampFields": {verdict: "carried", drive: func(b []byte) ([]byte, error) {
		o, _, e := StampFields(b, []Field{{Page: 1, Rect: [4]float64{10, 10, 200, 30}, Text: "t"}})
		return o, e
	}},
	"RemoveAttachment": {verdict: "carried", drive: func(b []byte) ([]byte, error) {
		x, e := AddAttachment(b, "a.txt", []byte("hi"))
		if e != nil {
			return nil, e
		}
		return RemoveAttachment(x, "a.txt")
	}},
	// **`InsertBlank` is `carried`, and the reason is a rule about what counts as undescribed.** It
	// adds a page with no content stream at all, and a page with nothing on it has nothing to tag —
	// counting it as undescribed would make adding an empty page a law-1 violation. See
	// `tagState.undescribed`.
	"InsertBlank": {verdict: "carried", drive: func(b []byte) ([]byte, error) { return InsertBlank(b, 1) }},

	// ── DROPPED. The claim goes with the content it described, which is law 1 satisfied honestly.
	// These are the page-set and page-composition operations: the tree cannot survive a subset it no
	// longer describes, and pdfcpu drops root and elements together rather than keeping a dangling
	// claim. That is D9's question answered — for a KEPT subset the remap is still open.
	"Collect":       {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return Collect(b, []string{"1"}) }},
	"RemovePages":   {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return RemovePages(b, []string{"1"}) }},
	"Crop":          {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return Crop(b, [4]float64{0.05, 0.05, 0.05, 0.05}, nil) }},
	"DuplicatePage": {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return DuplicatePage(b, 1) }},
	"SplitPage":     {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return SplitPage(b, 1, 2, 1, false) }},
	"SplitRegions":  {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return SplitRegions(b, 1, [][4]float64{{0, 0, 100, 100}}) }},
	"Booklet":       {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return Booklet(b, false) }},
	"InsertPDF":     {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return InsertPDF(b, untaggedFixture(), 1) }},
	"CarryAttachments": {verdict: "dropped", drive: func(b []byte) ([]byte, error) {
		o, _, e := CarryAttachments(b, untaggedFixture())
		return o, e
	}},
	// **`NUp` is `dropped` because `honest` makes it so, and it is the reason `honest` exists.**
	// `api.NUp` composes new page objects and carries the source catalog onto them, so the raw
	// output claims tagging over a tree whose every element points at a page that is gone — the one
	// measured `orphaned` output in this package. `TestNUpDoesNotClaimTaggingItHasNot` is the
	// dedicated guard; this row keeps it in the census.
	"NUp": {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return NUp(b, 2, false) }},

	// ── PARTIAL. A live tree that does not reach every page with content.
	//
	// **`Append` is the whole `partial` category, and it is a DECISION that it is not enforced.**
	// `api.MergeRaw` takes the first document's catalog whole, so appending an untagged document to
	// a tagged one keeps the claim over a page nothing describes — while every original element
	// stays anchored to a live page. Stripping would destroy a whole live tree to fix one page, and
	// `p2p/readme.go` and `p2p/sigpages.go` take this path for every ceremony document, so the cure
	// is worse than the disease at the only scale that matters. Recorded here, parked for Dan, and
	// the argument-order half is asserted separately below.
	"Append": {verdict: "partial", drive: func(b []byte) ([]byte, error) { return Append(b, untaggedFixture()) }},
	// `Combine` is `Append`'s sibling — the package's other `api.MergeRaw` caller, and it was
	// invisible to the census until the enumeration stopped requiring a `[]byte` first parameter.
	"Combine": {verdict: "partial", drive: func(b []byte) ([]byte, error) {
		return Combine([][]byte{b, untaggedFixture()})
	}},

	// ── UNTOUCHED. These return a report, an archive or an attachment — not a document — so there is
	// no output that could carry a claim.
	"ExtractAttachment":  {verdict: "untouched", why: "returns the attachment's bytes, not a document"},
	"ExportFormJSON":     {verdict: "untouched", why: "returns a report, not a document"},
	"ExportFormCSV":      {verdict: "untouched", why: "returns a report, not a document"},
	"ExportFormXFDF":     {verdict: "untouched", why: "returns a report, not a document"},
	"FlagsJSON":          {verdict: "untouched", why: "returns a report, not a document"},
	"ExtractImagesZip":   {verdict: "untouched", why: "returns a ZIP of images, not a document"},
	"ConvertDocToPDF":    {verdict: "untouched", why: "its input is not a PDF — it produces one from an office document"},
	"ConvertOfficeToPDF": {verdict: "untouched", why: "as ConvertDocToPDF"},
	"CreateFromJSON":     {verdict: "untouched", why: "authors a document from a JSON spec; there is no input tagging to lose"},

	// ── Declared but not driven, each with the reason. The completeness half still covers them.
	"Encrypt":                {verdict: "carried", why: "the encrypted output cannot be parsed without the password, so the oracle cannot read it back — the keys are inside the encrypted stream"},
	"RemovePassword":         {verdict: "carried", why: "needs an already-encrypted input, which the corpus does not carry"},
	"FillFormJSON":           {verdict: "carried", why: "pdfcpu refuses the fixture's single text field (`no form fields affected`); the sibling `AuthorForm` drives the same write path and is measured"},
	"FillFormXFDF":           {verdict: "carried", why: "as FillFormJSON"},
	"StampTextLayer":         {verdict: "carried", why: "needs an OCR text layer and its fonts installed"},
	"ConvertPDFAGhostscript": {verdict: "dropped", why: "shells out to Ghostscript, which is optional and absent on most machines"},
	"PreparePDFA":            {verdict: "carried", why: "its output is unreadable to the oracle on the minimal fixture; measured `carried` on the LibreOffice document (D9)"},
	"RedactPages": {
		verdict: "dropped",
		why: "needs a raster map. Dispositioned as a DECISION per P01.S04, not an oversight: it " +
			"destroys page content by design — it replaces pages with rasters, so no structure it " +
			"described can still be true, and a redaction that kept a tag tree would let a reader " +
			"recover the shape of what was removed",
	},
}

// TestEveryOperationDeclaresItsTagFate — law 2's completeness half, enumerated from the code.
func TestEveryOperationDeclaresItsTagFate(t *testing.T) {
	ops := documentTouchingOps(t)
	if len(ops) < 25 {
		t.Fatalf("only %d document-touching operation(s) found in this package; the enumeration has "+
			"stopped recognising them and every assertion below is vacuous", len(ops))
	}
	var missing []string
	for _, name := range ops {
		if _, ok := tagFates[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these operations take a document and return one, and declare no tag fate: %s.\n"+
			"Law 2: every operation declares `carried` / `dropped` / `partial` / `untouched`, in one "+
			"table, and this guard asserts each one HAS a verdict. An operation with none is one "+
			"nobody has asked what it does to a tagged document.", strings.Join(missing, ", "))
	}
	// The other direction: a table row for an operation that no longer exists is a claim about
	// code that is not there — the shape `/pending 423` closed on for exemption lists.
	have := map[string]bool{}
	for _, n := range ops {
		have[n] = true
	}
	var stale []string
	for name := range tagFates {
		if !have[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("the table declares a tag fate for operations this package no longer has: %s",
			strings.Join(stale, ", "))
	}
	// An undriven operation must say why, or the gap is silent.
	for name, f := range tagFates {
		if f.drive == nil && strings.TrimSpace(f.why) == "" {
			t.Errorf("%s is not driven by this guard and gives no reason — an undriven operation "+
				"has to be a recorded gap, not an omission", name)
		}
		if f.verdict == "orphaned" {
			t.Errorf("%s declares `orphaned`, which is not a fate an operation may have — it is "+
				"law 1's violation, and the guard fails it rather than recording it", name)
		}
	}
}

// TestEveryDeclaredFateIsTheMEASUREDFate — law 2's correctness half.
//
// **This is the assertion whose absence let 19 wrong verdicts sit in the table.** The declarations
// were derived from a byte count that could not see a compressed object stream, every one of them
// said `dropped`, and nothing ever compared a declaration to the document the operation produced.
func TestEveryDeclaredFateIsTheMEASUREDFate(t *testing.T) {
	src := taggedFixture()
	s := inspectTags(src)
	if !s.claims() || s.anchored < 1 {
		t.Fatal("setup: the corpus fixture is not a tagged document, so every assertion below would " +
			"pass on a build that does nothing at all")
	}
	driven := 0
	for name, f := range tagFates {
		if f.drive == nil {
			continue
		}
		out, err := f.drive(src)
		if err != nil {
			// An operation that refuses the fixture outright cannot lie about it. Recorded rather
			// than failed: the corpus is one document and not every operation applies to it.
			t.Logf("%s: not exercised on this fixture (%v)", name, err)
			continue
		}
		driven++
		got := fate(out)
		if got == "orphaned" {
			t.Errorf("%s emits a tagging claim over a tree that describes nothing in the document: "+
				"%v.\nLaw 1: no output may carry `/MarkInfo /Marked true` or a `/StructTreeRoot` "+
				"over content that is neither tagged nor marked as an artifact. A visible loss is "+
				"honest; a false claim is worse than no tagging, because a screen reader told a "+
				"document is tagged stops reaching for the fallbacks it would otherwise use.",
				name, claims(out))
			continue
		}
		if got != f.verdict {
			t.Errorf("%s declares %q and measures %q (%v).\nLaw 2 is not satisfied by a table of "+
				"assertions — the declaration has to be TRUE. 19 rows here said `dropped` about "+
				"operations that carry the tree intact, and nothing compared them to a document.",
				name, f.verdict, got, claims(out))
		}
	}
	if driven < 20 {
		t.Errorf("only %d operation(s) were actually driven; the rest errored out on the fixture, so "+
			"this guard is reporting a census as checked that it barely tested", driven)
	}
}

// TestMergeDoesNotOrphanInEitherArgumentOrder — P01.S04's first clause, in the form the measurement
// leaves it.
//
// The clause was written as *"`merge` preserves tagging in both argument orders"*, against a defect
// where tagging survived only when the tagged file came first. **That defect is real**: `MergeRaw`
// takes the first document's catalog whole, so tagged-first keeps the claim and tagged-second drops
// it. It was struck in error on 2026-09-11 when a byte count reported both orders as dropping.
//
// What the order must not do is ORPHAN. One argument order is exactly how this was missed before.
func TestMergeDoesNotOrphanInEitherArgumentOrder(t *testing.T) {
	tagged := taggedFixture()
	plain, err := testpdf.Text("an untagged second document")
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(tagged); !s.claims() || s.anchored < 1 {
		t.Fatal("setup: the tagged fixture is not tagged, so neither order below tests anything")
	}
	for _, tc := range []struct {
		name string
		a, b []byte
		want string
	}{
		{"tagged first", tagged, plain, "partial"},
		{"tagged second", plain, tagged, "dropped"},
	} {
		out, aerr := Append(tc.a, tc.b)
		if aerr != nil {
			t.Errorf("%s: %v", tc.name, aerr)
			continue
		}
		if got := fate(out); got != tc.want {
			t.Errorf("merge with the %s measures %q, want %q (%v). The argument order decides which "+
				"catalog survives, and one order is how this was missed the first time",
				tc.name, got, tc.want, claims(out))
		}
	}
}

// onePixelPNG is the smallest image the stamp row can be driven with.
func onePixelPNG() []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		panic(err) // a fixture this package cannot build is a broken build, not a test failure
	}
	return buf.Bytes()
}

// untaggedFixture is the second document the merge and insert rows need.
func untaggedFixture() []byte {
	b, err := testpdf.Text("an untagged second document")
	if err != nil {
		panic(err) // a fixture this package cannot build is a broken build, not a test failure
	}
	return b
}

// documentTouchingOps enumerates, from the source, every exported function in this package whose
// first parameter is a PDF and whose first result is bytes — the population law 2 binds.
//
// **Parsed, not grepped.** A name scan over this package would match the word in a comment, and this
// repo has paid for that twice (`/pending 410`, `/pending 445`).
func documentTouchingOps(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, n, nil, 0)
		if perr != nil {
			t.Fatalf("%s: %v", n, perr)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() || fn.Type.Params == nil {
				continue
			}
			if !firstParamIsPDF(fn) || !firstResultIsBytes(fn) {
				continue
			}
			out = append(out, fn.Name.Name)
		}
	}
	sort.Strings(out)
	return out
}

// firstParamIsPDF accepts a first parameter that is a document — `[]byte` — or a slice of them,
// `[][]byte`, under any name.
//
// **This filter has been a hole twice, and both holes hid a real operation.** It first required the
// first parameter to be *named* `pdf`: `RedactPages` calls its first parameter `original`, so the
// enumeration never saw it — and `redact` is precisely the operation P01.S04 says must be
// *explicitly* dispositioned. It then accepted only `[]byte`, which excluded `Combine(pdfs
// [][]byte)` — the package's **other** `api.MergeRaw` caller, and so the other operation capable of
// producing a document whose structure tree describes only some of its pages.
//
// An operation escaping a law-2 census by the shape or the name of its argument is the census
// failing, not the operation qualifying. Neither the name nor the arity is what makes something a
// document-touching operation.
func firstParamIsPDF(fn *ast.FuncDecl) bool {
	if len(fn.Type.Params.List) == 0 {
		return false
	}
	p := fn.Type.Params.List[0]
	if len(p.Names) == 0 {
		return false
	}
	if isByteSlice(p.Type) {
		return true
	}
	a, ok := p.Type.(*ast.ArrayType)
	return ok && a.Len == nil && isByteSlice(a.Elt)
}

func firstResultIsBytes(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return false
	}
	return isByteSlice(fn.Type.Results.List[0].Type)
}

func isByteSlice(e ast.Expr) bool {
	a, ok := e.(*ast.ArrayType)
	if !ok || a.Len != nil {
		return false
	}
	id, ok := a.Elt.(*ast.Ident)
	return ok && id.Name == "byte"
}
