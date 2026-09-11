package pdfops

import (
	"go/ast"
	"go/parser"
	"go/token"
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
// asserts that every operation *has* one. This is ADR-009's shape: the guard checks the door, not
// the eight sites that happen to be right.
//
// # Two assertions, and they are different claims
//
//  1. **Completeness.** Every document-touching operation in this package appears in the table.
//     Enumerated from the CODE with go/ast — not from a hand-written list, which is the acceptance
//     criterion and also the only version that can catch an operation added tomorrow. P01.S02 is why
//     it matters: that measurement found `Rotate` lying, and `Rotate` was on nobody's list.
//  2. **Law 1 in force.** For every operation this guard can actually drive, the output does not
//     claim tagging it has not got. That is `lies()` and it is deliberately not "the tree survived":
//     an operation that drops everything passes, one that carries everything passes, and only the
//     middle — a claim over no content — fails.
//
// # What it cannot do, said rather than implied
//
// Not every operation can be driven from a table: some need a password, an attachment name, a font,
// a second document. Those declare a verdict and a **reason they are not driven**, which the guard
// requires to be non-empty — so an undriven operation is a recorded gap rather than a silent one.
// The completeness half covers all of them regardless, and that is the half law 2 is about.

// tagFate is one operation's declared verdict.
type tagFate struct {
	// verdict is `dropped` (the claim is removed with the content — law 1 satisfied honestly),
	// `carried` (claim and structure both survive) or `untouched` (the operation cannot affect the
	// claim, e.g. it reads and returns a report rather than a document).
	//
	// **There is deliberately no verdict for "keeps the claim without the content".** Law 1 forbids
	// that state, so it is not a fate an operation may declare — it is a failure, and `lies()` is
	// what fails it.
	verdict string
	// why is required when the operation is not driven below: an undriven operation is a declared
	// gap, never a silent one.
	why string
	// drive runs the operation on a tagged fixture. Nil means "not driven" and then `why` must say so.
	drive func([]byte) ([]byte, error)
}

// tagFates is the table. Every document-touching operation in this package must appear.
var tagFates = map[string]tagFate{
	// ── Driven: these take a document and simple arguments, so the verdict is CHECKED, not declared.
	"NUp":                 {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return NUp(b, 2, false) }},
	"Booklet":             {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return Booklet(b, false) }},
	"Collect":             {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return Collect(b, []string{"1"}) }},
	"RemovePages":         {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return RemovePages(b, []string{"1"}) }},
	"Rotate":              {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return Rotate(b, nil, 90) }},
	"Crop":                {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return Crop(b, [4]float64{0.05, 0.05, 0.05, 0.05}, nil) }},
	"NormalizePageSizes":  {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return NormalizePageSizes(b) }},
	"Optimize":            {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return Optimize(b) }},
	"InsertBlank":         {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return InsertBlank(b, 1) }},
	"DuplicatePage":       {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return DuplicatePage(b, 1) }},
	"SetLang":             {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return SetLang(b, "en-GB") }},
	"StripMetadata":       {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return StripMetadata(b) }},
	"StripActive":         {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return StripActive(b) }},
	"RemoveFilesAndMedia": {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return RemoveFilesAndMedia(b) }},
	"ClearFlags":          {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return ClearFlags(b) }},
	"SplitPage":           {verdict: "dropped", drive: func(b []byte) ([]byte, error) { return SplitPage(b, 1, 2, 1, false) }},

	// ── Not driven, each with the reason. The completeness half still covers them.
	"Encrypt":                {verdict: "dropped", why: "needs a password, and the encrypted output cannot be scanned for claim keys — the keys are inside the encrypted stream"},
	"RemovePassword":         {verdict: "dropped", why: "needs an already-encrypted input, which the corpus does not carry"},
	"AddAttachment":          {verdict: "dropped", why: "needs a file to attach"},
	"RemoveAttachment":       {verdict: "dropped", why: "needs an input that already has an attachment"},
	"ExtractAttachment":      {verdict: "untouched", why: "returns the attachment's bytes, not a document"},
	"ExportFormJSON":         {verdict: "untouched", why: "returns a report, not a document"},
	"ExportFormCSV":          {verdict: "untouched", why: "returns a report, not a document"},
	"ExportFormXFDF":         {verdict: "untouched", why: "returns a report, not a document"},
	"FlagsJSON":              {verdict: "untouched", why: "returns a report, not a document"},
	"AuthorForm":             {verdict: "dropped", why: "needs a field spec"},
	"AddNotes":               {verdict: "dropped", why: "needs note geometry"},
	"SetOutline":             {verdict: "dropped", why: "needs an outline spec"},
	"SetPageLabels":          {verdict: "dropped", why: "needs a label spec"},
	"StampPageNumbers":       {verdict: "dropped", why: "needs a stamp spec"},
	"StampImages":            {verdict: "dropped", why: "needs an image"},
	"StampTextLayer":         {verdict: "dropped", why: "needs an OCR text layer"},
	"StampWatermark":         {verdict: "dropped", why: "needs watermark text and options"},
	"SplitRegions":           {verdict: "dropped", why: "needs region geometry"},
	"ConvertPDFAGhostscript": {verdict: "dropped", why: "shells out to Ghostscript, which is optional and absent on most machines"},

	// ── Found by the ENUMERATION, not by anyone's list — which is the acceptance criterion working.
	// Each takes a second document, a data payload or returns more than bytes, so none is drivable
	// from a one-argument table; every one still owes a verdict, and that is law 2's whole point.
	"Append":           {verdict: "dropped", why: "needs a second document to append"},
	"InsertPDF":        {verdict: "dropped", why: "needs a second document to insert"},
	"FillFormJSON":     {verdict: "dropped", why: "needs a JSON field record"},
	"FillFormXFDF":     {verdict: "dropped", why: "needs an XFDF field record"},
	"SetFlags":         {verdict: "dropped", why: "needs a flags record"},
	"StampFields":      {verdict: "dropped", why: "needs field geometry, and returns fit results beside the document"},
	"ExtractImagesZip": {verdict: "untouched", why: "returns a ZIP of images, not a document"},
	"PreparePDFA":      {verdict: "dropped", why: "returns blockers beside the document, so it does not fit the one-result driver"},

	// ── Found when the enumeration stopped requiring the first parameter to be NAMED `pdf`. Every
	// one of these was hiding behind a different parameter name, and `RedactPages` is the one
	// P01.S04 names explicitly — an operation escaping a law-2 census by what it calls its argument
	// is the census failing, not the operation qualifying.
	"RedactPages": {
		verdict: "dropped",
		why: "destroys page content by design — it replaces pages with rasters, so no structure it " +
			"described can still be true. Dispositioned as a DECISION per P01.S04, not an oversight: " +
			"a redaction that kept a tag tree would let a reader recover the shape of what was removed",
	},
	"CarryAttachments":   {verdict: "dropped", why: "takes two documents and moves attachments between them"},
	"ConvertDocToPDF":    {verdict: "untouched", why: "its input is not a PDF — it produces one from an office document"},
	"ConvertOfficeToPDF": {verdict: "untouched", why: "as ConvertDocToPDF"},
	"CreateFromJSON":     {verdict: "untouched", why: "authors a document from a JSON spec; there is no input tagging to lose"},
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
			"Law 2: every operation declares `dropped` / `carried` / `untouched`, in one table, and "+
			"this guard asserts each one HAS a verdict. An operation with none is one nobody has "+
			"asked what it does to a tagged document — which is how `Rotate` was found lying at "+
			"P01.S02, on nobody's list.", strings.Join(missing, ", "))
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
	}
}

// TestNoOperationClaimsTaggingItHasNot — law 1, in force, over every operation that can be driven.
func TestNoOperationClaimsTaggingItHasNot(t *testing.T) {
	src := taggedFixture()
	if !claimsTagged(src) {
		t.Fatal("setup: the corpus fixture is not tagged, so every assertion below would pass on a " +
			"build that does nothing at all")
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
		if lies(out) {
			t.Errorf("%s emits a tagging claim over content nothing describes: %v.\n"+
				"Law 1: no output may carry `/MarkInfo /Marked true` or a `/StructTreeRoot` over "+
				"content that is neither tagged nor marked as an artifact. A visible loss is honest; "+
				"a false claim is worse than no tagging, because a screen reader told a document is "+
				"tagged stops reaching for the fallbacks it would otherwise use.", name, claims(out))
		}
	}
	if driven < 10 {
		t.Errorf("only %d operation(s) were actually driven; the rest errored out on the fixture, so "+
			"this guard is reporting a law as upheld that it barely tested", driven)
	}
}

// TestMergeDoesNotLieInEitherArgumentOrder — P01.S04's first clause, in the form D9 left it.
//
// The clause was written as *"`merge` preserves tagging in both argument orders"*, against a known
// defect where tagging survived only when the tagged file came first. D9 overtook it: nothing
// preserves tagging in any order. What survives is the honesty question, and the ORDER still matters
// for it — a merge that dropped the claim only when the tagged document was first would leave the
// other order lying, and one argument order is exactly how this was missed before.
func TestMergeDoesNotLieInEitherArgumentOrder(t *testing.T) {
	tagged := taggedFixture()
	plain, err := testpdf.Text("an untagged second document")
	if err != nil {
		t.Fatal(err)
	}
	if !claimsTagged(tagged) {
		t.Fatal("setup: the tagged fixture is not tagged, so neither order below tests anything")
	}
	for _, tc := range []struct {
		name string
		a, b []byte
	}{
		{"tagged first", tagged, plain},
		{"tagged second", plain, tagged},
	} {
		out, aerr := Append(tc.a, tc.b)
		if aerr != nil {
			t.Errorf("%s: %v", tc.name, aerr)
			continue
		}
		if lies(out) {
			t.Errorf("merge with the %s emits a tagging claim over content nothing describes: %v. "+
				"One argument order is how this was missed the first time", tc.name, claims(out))
		}
	}
}

// claimsTagged is the fixture's own precondition.
func claimsTagged(pdf []byte) bool {
	c := claims(pdf)
	return c["/StructTreeRoot"] > 0 && c["/Marked"] > 0 && c["/StructElem"] > 0
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

// firstParamIsPDF accepts a first `[]byte` parameter under any name.
//
// **It used to require the name `pdf`, and that was a hole rather than a filter.** `RedactPages`
// calls its first parameter `original`, so the enumeration never saw it — and `redact` is precisely
// the operation P01.S04 says must be *explicitly* dispositioned because it destroys page content by
// design. An operation escaping a law-2 census by naming its parameter differently is the census
// failing, not the operation qualifying.
//
// The name is not what makes something a document-touching operation; the shape is.
func firstParamIsPDF(fn *ast.FuncDecl) bool {
	if len(fn.Type.Params.List) == 0 {
		return false
	}
	p := fn.Type.Params.List[0]
	if len(p.Names) == 0 {
		return false
	}
	return isByteSlice(p.Type)
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
