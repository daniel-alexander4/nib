package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// craftActivePDF builds a one-page PDF salted with active content: an
// auto-run OpenAction, a document-level JavaScript name tree, document
// additional actions, and a Link annotation carrying a URI action.
func craftActivePDF(t *testing.T) []byte {
	t.Helper()
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	root["OpenAction"] = types.Dict{"Type": types.Name("Action"), "S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(1)")}
	root["Names"] = types.Dict{"JavaScript": types.Dict{"Names": types.Array{}}}
	root["AA"] = types.Dict{"WC": types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("x")}}
	pd, _, _, err := xt.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	pd["Annots"] = types.Array{
		types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("Link"),
			"A": types.Dict{"S": types.Name("URI"), "URI": types.StringLiteral("http://evil.example")},
		},
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func kinds(rep ScanReport) map[string]bool {
	m := map[string]bool{}
	for _, f := range rep.Findings {
		m[f.Kind] = true
	}
	return m
}

func TestScanFindsActiveContent(t *testing.T) {
	rep, err := Scan(craftActivePDF(t))
	if err != nil {
		t.Fatal(err)
	}
	got := kinds(rep)
	for _, want := range []string{"openAction", "javascript", "additionalActions", "action"} {
		if !got[want] {
			t.Errorf("scan missed %q; findings=%+v", want, rep.Findings)
		}
	}
}

func TestScanCleanHasNoSeriousFindings(t *testing.T) {
	pdf, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Scan(pdf)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		if f.Severity != "low" {
			t.Errorf("clean PDF produced a %s finding: %+v", f.Severity, f)
		}
	}
}

func TestScanReadOnly(t *testing.T) {
	pdf := craftActivePDF(t)
	before := append([]byte(nil), pdf...)
	if _, err := Scan(pdf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pdf, before) {
		t.Error("Scan mutated its input")
	}
}

func TestStripActiveRemovesActiveContent(t *testing.T) {
	stripped, err := StripActive(craftActivePDF(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(stripped); err != nil {
		t.Fatalf("stripped PDF does not validate: %v", err)
	}
	got := kinds(must(t, stripped))
	for _, gone := range []string{"openAction", "javascript", "additionalActions", "action"} {
		if got[gone] {
			t.Errorf("strip left %q behind; findings=%+v", gone, must(t, stripped).Findings)
		}
	}
}

// TestTiersDiffer pins the difference between the surgical strip and the gentle
// safe removal: StripActive neutralizes the URI action, RemoveFilesAndMedia
// leaves it (it only touches embedded files and media annotations). Both must
// produce a valid document.
func TestTiersDiffer(t *testing.T) {
	pdf := craftActivePDF(t)

	safe, err := RemoveFilesAndMedia(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(safe); err != nil {
		t.Fatalf("safe removal does not validate: %v", err)
	}
	if !kinds(must(t, safe))["action"] {
		t.Error("RemoveFilesAndMedia should keep the link's URI action, but it is gone")
	}

	strip, err := StripActive(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if kinds(must(t, strip))["action"] {
		t.Error("StripActive should remove the URI action, but it survived")
	}
}

func must(t *testing.T, pdf []byte) ScanReport {
	t.Helper()
	rep, err := Scan(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// craftMetadataPDF builds a one-page PDF carrying identifying metadata: an /Info
// dict (Author/Title/Creator/Subject/Keywords) and an XMP /Metadata stream.
func craftMetadataPDF(t *testing.T) []byte {
	t.Helper()
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	infoRef, err := xt.IndRefForNewObject(types.Dict{
		"Author":   types.StringLiteral("Jane Doe"),
		"Title":    types.StringLiteral("Q3 Layoff Plan"),
		"Creator":  types.StringLiteral("Microsoft Word"),
		"Subject":  types.StringLiteral("Confidential"),
		"Keywords": types.StringLiteral("secret, internal"),
	})
	if err != nil {
		t.Fatal(err)
	}
	xt.Info = infoRef
	xmp := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>` +
		`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
		`<dc:creator><rdf:Seq><rdf:li>Jane Doe</rdf:li></rdf:Seq></dc:creator>` +
		`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`
	sd, err := xt.NewStreamDictForBuf([]byte(xmp))
	if err != nil {
		t.Fatal(err)
	}
	sd.Dict["Type"] = types.Name("Metadata")
	sd.Dict["Subtype"] = types.Name("XML")
	if err := sd.Encode(); err != nil {
		t.Fatal(err)
	}
	mref, err := xt.IndRefForNewObject(*sd)
	if err != nil {
		t.Fatal(err)
	}
	root["Metadata"] = *mref
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// docID0 returns the first (permanent) element of the trailer /ID, for asserting
// it is regenerated by a scrub.
func docID0(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.ID) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", ctx.ID[0])
}

func TestScanFindsMetadata(t *testing.T) {
	rep := must(t, craftMetadataPDF(t))
	got := kinds(rep)
	for _, want := range []string{"info", "metadata"} {
		if !got[want] {
			t.Errorf("scan missed %q; findings=%+v", want, rep.Findings)
		}
	}
	var joined string
	for _, f := range rep.Findings {
		joined += f.Detail + "\n"
	}
	for _, want := range []string{"Jane Doe", "Q3 Layoff Plan", "Microsoft Word"} {
		if !strings.Contains(joined, want) {
			t.Errorf("scan should surface %q in a finding; details=%q", want, joined)
		}
	}
}

func TestStripMetadataRemovesIdentifyingMetadata(t *testing.T) {
	src := craftMetadataPDF(t)
	out, err := StripMetadata(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("stripped doc does not validate: %v", err)
	}
	got := kinds(must(t, out))
	for _, gone := range []string{"info", "metadata"} {
		if got[gone] {
			t.Errorf("strip left %q behind; findings=%+v", gone, must(t, out).Findings)
		}
	}
	// The permanent /ID must be regenerated, not carried over.
	if before, after := docID0(t, src), docID0(t, out); before == "" || before == after {
		t.Errorf("/ID not regenerated: before=%q after=%q", before, after)
	}
}

// annotFacts re-reads a PDF and walks its page annotations DIRECTLY — every /Subtype it
// finds, and every action /S name reachable through /A and its /Next chain.
//
// It exists because the obvious independent check does not work here and quietly passes:
// pdfcpu writes object streams, so annotation dictionaries are COMPRESSED and a
// `bytes.Contains(pdf, []byte("/JavaScript"))` is false before the strip as well as
// after. A byte assertion written against that is vacuous — it is green whatever the
// strip does. (Measured: craftChainedActionPDF's own output contains none of /JavaScript,
// /Launch, /GoTo or /Annots as plain bytes.)
//
// It walks the object graph itself rather than calling Scan, which is the independence
// that matters: Scan and StripActive read the same riskyActions map, so a check built on
// Scan cannot see a type missing from that map.
func annotFacts(t *testing.T, pdf []byte) (subtypes []string, actions []string) {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-reading the PDF: %v", err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	var walk func(types.Dict, int)
	walk = func(act types.Dict, depth int) {
		if act == nil || depth > 32 {
			return
		}
		if n, ok := act["S"].(types.Name); ok {
			actions = append(actions, n.Value())
		}
		switch next := act["Next"].(type) {
		case types.Array:
			for _, a := range next {
				d, _ := xt.DereferenceDict(a)
				walk(d, depth+1)
			}
		default:
			if d, _ := xt.DereferenceDict(act["Next"]); d != nil {
				walk(d, depth+1)
			}
		}
	}
	eachPage(xt, root, func(page types.Dict, _ int) {
		arr, _ := xt.DereferenceArray(page["Annots"])
		for _, a := range arr {
			annot, _ := xt.DereferenceDict(a)
			if annot == nil {
				continue
			}
			if n, ok := annot["Subtype"].(types.Name); ok {
				subtypes = append(subtypes, n.Value())
			}
			d, _ := xt.DereferenceDict(annot["A"])
			walk(d, 0)
		}
	})
	return subtypes, actions
}

func has(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// craftChainedActionPDF builds a page whose Link annotation carries a BENIGN head action
// (/GoTo) chaining through /Next to a JavaScript action, and a second annotation whose
// /Next is an ARRAY — both forms §12.6.1 allows.
func craftChainedActionPDF(t *testing.T) []byte {
	t.Helper()
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pd, _, _, err := ctx.XRefTable.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	pd["Annots"] = types.Array{
		types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("Link"),
			"A": types.Dict{
				"S":    types.Name("GoTo"), // benign head
				"Next": types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(2)")},
			},
		},
		types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("Link"),
			"A": types.Dict{
				"S": types.Name("GoTo"),
				"Next": types.Array{
					types.Dict{"S": types.Name("GoTo")},
					types.Dict{"S": types.Name("Launch"), "F": types.StringLiteral("/bin/sh")},
				},
			},
		},
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// A risky action reached through a /Next chain is found and stripped like any other.
//
// The review's finding was that a benign head hid the whole chain from both Scan and
// StripActive: `<< /S /GoTo /Next << /S /JavaScript >> >>` reported clean and survived.
// Both /Next forms are driven, because a dict and an array are separate code paths and
// only one of them would have been written first.
func TestScanAndStripFollowNextChains(t *testing.T) {
	pdf := craftChainedActionPDF(t)

	rep := must(t, pdf)
	if !kinds(rep)["action"] {
		t.Fatalf("a JavaScript action behind a benign /GoTo head was not reported; findings=%+v", rep.Findings)
	}

	stripped, err := StripActive(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(stripped); err != nil {
		t.Fatalf("stripped PDF does not validate: %v", err)
	}
	if kinds(must(t, stripped))["action"] {
		t.Error("StripActive left a chained risky action behind")
	}
	// Independent of Scan AND of riskyActions — see annotFacts for why the obvious
	// byte-level version of this check is vacuous.
	_, before := annotFacts(t, pdf)
	if !has(before, "JavaScript") || !has(before, "Launch") {
		t.Fatalf("setup: the crafted chains do not contain the risky actions this asserts on; found %v", before)
	}
	_, after := annotFacts(t, stripped)
	for _, name := range []string{"JavaScript", "Launch"} {
		if has(after, name) {
			t.Errorf("a %s action reachable through /Next survives the strip; actions left: %v", name, after)
		}
	}
}

// The strip is checked against a list this file owns, NOT against riskyActions.
//
// This is the finding's real teeth. TestStripActiveRemovesActiveContent verifies the
// strip by re-running Scan, and Scan and StripActive read the SAME riskyActions map — so
// an action type missing from the map is missing from the strip and from the check that
// would have caught it, by construction. A gap in the map was invisible to every test
// built on it, which is how /Next and six action types went unnoticed.
//
// The literal below is therefore deliberately a second, hand-maintained copy. Adding a
// type to riskyActions without adding it here fails, and so does the reverse. That
// duplication is the point: two independent statements of a security-relevant set are
// worth more than one statement checked against itself.
func TestRiskyActionsCoverTheTypesThatCanRunOrHide(t *testing.T) {
	want := []string{
		"JavaScript", "Launch", "SubmitForm", "ImportData", "GoToR", "GoToE", "URI",
		"Rendition", "Movie", "Sound", "SetOCGState", "GoTo3DView", "Hide",
	}
	for _, name := range want {
		if _, ok := riskyActions[name]; !ok {
			t.Errorf("%s is an action that can run code, reach outside the document, or change what is visible, and riskyActions does not list it — so Scan does not report it and StripActive does not remove it", name)
		}
	}
	if len(riskyActions) != len(want) {
		t.Errorf("riskyActions has %d entries and this test names %d — one of them gained a type the other did not, and the whole value of this check is that they are maintained separately", len(riskyActions), len(want))
	}
	// Every listed type also needs a human-readable detail, or the scan report tells the
	// user "SetOCGState action" and leaves them to look it up.
	for _, name := range want {
		if actionDetail(name) == name+" action" {
			t.Errorf("actionDetail(%q) falls through to the generic form — the report will not say what the action does", name)
		}
	}
}

// The tier hierarchy holds in the direction the UI promises: whatever the gentle
// RemoveFilesAndMedia takes out, the stronger StripActive takes out too.
//
// It did not. StripActive left Screen/Movie/Sound/3D/FileAttachment annotations in place
// while RemoveFilesAndMedia removed them, so "remove all active content" was weaker than
// "remove files and media" for exactly the annotations whose purpose is to play
// something. TestTiersDiffer pinned the OTHER direction — the URI action the strip
// removes and the gentle tier keeps — and a difference in one direction reads as the
// hierarchy being tested when only half of it is.
func TestStripActiveIsAtLeastAsStrongAsRemoveFilesAndMedia(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pd, _, _, err := ctx.XRefTable.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	// INDIRECT references, not direct dicts, and this is load-bearing rather than style.
	// pdfcpu's RemoveAnnotations works off ctx.PageAnnots, a cache that validation fills
	// only for annotations reached through an indirect reference — so with direct dicts
	// NEITHER tier removes anything and this test passes without the fix it exists for.
	// Measured that way first: the earlier draft was green against the unfixed code.
	// Real-world PDFs use indirect refs, so this fixture is also the honest one.
	screen, err := ctx.XRefTable.IndRefForNewObject(types.Dict{
		"Type": types.Name("Annot"), "Subtype": types.Name("Screen"), "Rect": types.NewNumberArray(0, 0, 10, 10)})
	if err != nil {
		t.Fatal(err)
	}
	// /Movie is a required entry for a Movie annotation and pdfcpu's writer validates it —
	// the annotation is silently refused without it.
	movie, err := ctx.XRefTable.IndRefForNewObject(types.Dict{
		"Type": types.Name("Annot"), "Subtype": types.Name("Movie"), "Rect": types.NewNumberArray(0, 0, 10, 10),
		"Movie": types.Dict{"F": types.StringLiteral("clip.avi")}})
	if err != nil {
		t.Fatal(err)
	}
	pd["Annots"] = types.Array{*screen, *movie}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	pdf := buf.Bytes()

	// The stimulus, read through the object graph rather than the bytes — pdfcpu writes
	// object streams, so a bytes.Contains here is false for a document that DOES carry
	// the annotation. This assertion firing is what caught that, on this test's first run.
	subtypes, _ := annotFacts(t, pdf)
	if !has(subtypes, "Screen") || !has(subtypes, "Movie") {
		t.Fatalf("setup: the crafted PDF carries %v, not the media annotations this compares on", subtypes)
	}

	gentle, err := RemoveFilesAndMedia(pdf)
	if err != nil {
		t.Fatal(err)
	}
	strong, err := StripActive(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(strong); err != nil {
		t.Fatalf("stripped PDF does not validate: %v", err)
	}
	gentleLeft, _ := annotFacts(t, gentle)
	strongLeft, _ := annotFacts(t, strong)
	for _, subtype := range []string{"Screen", "Movie"} {
		if !has(gentleLeft, subtype) && has(strongLeft, subtype) {
			t.Errorf("a %s annotation survives StripActive but not RemoveFilesAndMedia — the stronger tier is weaker than the gentle one for exactly the annotations whose purpose is to play something", subtype)
		}
	}
}

// eachPage numbers a duplicated page the way a reader renders it, and still refuses a
// cycle.
//
// The walk deduplicated by object number to bound a malformed tree, which also skipped a
// page object legitimately referenced twice — pdf.js walks without deduplicating and shows
// two pages, so Scan's numbering ran a page early from that point on and every finding
// after it pointed at the wrong page. On a security scan that is the number the user acts
// on: it is where they go to look at what was found.
//
// Both halves are driven, because a fix for one is easy to write in a way that breaks the
// other — dropping the guard entirely fixes the count and hangs on a cycle.
func TestEachPageCountsDuplicatesAndStopsCycles(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 80, 80), rasterPage(t, 80, 80)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	pages := derefDict(xt, root["Pages"])
	kids := derefArray(xt, pages["Kids"])
	if len(kids) != 2 {
		t.Fatalf("setup: the fixture has %d kids, want 2", len(kids))
	}

	// The same page object, referenced a second time — what a hand-built or crafted file
	// can contain, and what a reader shows as a third page.
	pages["Kids"] = append(append(types.Array{}, kids...), kids[0])
	count := 0
	eachPage(xt, root, func(_ types.Dict, nr int) { count = nr })
	if count != 3 {
		t.Errorf("a page object referenced twice was counted %d times, want 3 — the reader shows three pages, so every finding after the duplicate is reported a page early", count)
	}

	// A cycle: the page-tree node lists itself. The walk must terminate, and must not
	// count the loop.
	selfRef, err := xt.IndRefForNewObject(pages)
	if err != nil {
		t.Fatal(err)
	}
	pages["Kids"] = types.Array{*selfRef, kids[0]}
	done := make(chan int, 1)
	go func() {
		n := 0
		eachPage(xt, root, func(_ types.Dict, nr int) { n = nr })
		done <- n
	}()
	select {
	case n := <-done:
		if n == 0 {
			t.Error("a cyclic page tree yielded no pages at all — the guard is refusing the real kid too")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("eachPage did not terminate on a cyclic page tree")
	}
}

// craftFieldScriptPDF builds a PDF whose ONLY active content is a field-level script on a
// PARENT field dict — the shape Acrobat produces for a multi-widget field, and the standard
// home (§12.7.5.3) for /AA /K keystroke, /F format, /V validate and /C calculate scripts.
//
// The parent is deliberately NOT an annotation and is NOT in any page's /Annots. That is the
// whole point: the page walk sees only the widget kid, which carries no action, so the
// document scanned clean.
func craftFieldScriptPDF(t *testing.T) []byte {
	t.Helper()
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	pd, _, _, err := xt.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	// The widget kid: an ordinary annotation with no action of its own.
	kid := types.Dict{
		"Type": types.Name("Annot"), "Subtype": types.Name("Widget"),
		"Rect": types.NewNumberArray(10, 10, 110, 40), "FT": types.Name("Tx"),
	}
	kidRef, err := xt.IndRefForNewObject(kid)
	if err != nil {
		t.Fatal(err)
	}
	// The parent field: holds the scripts, is not an annotation, is on no page.
	parent := types.Dict{
		"FT": types.Name("Tx"), "T": types.StringLiteral("total"),
		"DA":   types.StringLiteral("/Helv 0 Tf 0 g"),
		"Kids": types.Array{*kidRef},
		"AA": types.Dict{
			"C": types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert('calculate')")},
			"K": types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert('keystroke')")},
		},
	}
	parentRef, err := xt.IndRefForNewObject(parent)
	if err != nil {
		t.Fatal(err)
	}
	kid["Parent"] = *parentRef
	pd["Annots"] = types.Array{*kidRef}

	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	// DA and a font resource, because pdfcpu's validator requires them on a form field
	// and refuses the whole document otherwise — the fixture has to be a PDF a reader
	// actually accepts, or the walk under test is never reached.
	root["AcroForm"] = types.Dict{
		"Fields": types.Array{*parentRef},
		"CO":     types.Array{*parentRef},
		"DA":     types.StringLiteral("/Helv 0 Tf 0 g"),
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestFieldLevelScriptsAreSeenAndStripped.
//
// `Scan` read `/AcroForm` only for `/XFA` and `StripActive` deleted only `/XFA`. Neither
// walked `/Fields` or `/CO`. The page walk catches `/AA` and `/A` on widget ANNOTATIONS,
// which covers a merged field+widget dict and nothing else — so a PDF whose only active
// content is a parent field's script scanned CLEAN, and because `server/scan.go`'s residual
// re-scan is this same detector, StripActive then reported "all active content neutralized"
// with the scripts intact.
func TestFieldLevelScriptsAreSeenAndStripped(t *testing.T) {
	pdf := craftFieldScriptPDF(t)

	// STIMULUS, independent of Scan: the scripts really are in the field tree, and really
	// are NOT on any page annotation. Without both halves this test could pass against a
	// fixture the page walk already covered.
	parentAA, annotAA := fieldTreeFacts(t, pdf)
	if !parentAA {
		t.Fatal("setup: the parent field carries no /AA, so there is nothing for the new walk to find")
	}
	if annotAA {
		t.Fatal("setup: a page annotation carries /AA — the page walk already covers that " +
			"shape, so this fixture is not the case the finding is about")
	}

	rep, err := Scan(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var sawAction bool
	for _, f := range rep.Findings {
		if f.Kind == "additionalActions" {
			sawAction = true
		}
	}
	if !sawAction {
		t.Errorf("Scan reports no active content for a document whose form fields carry "+
			"keystroke and calculate JavaScript: %+v", rep.Findings)
	}

	stripped, err := StripActive(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if aa, _ := fieldTreeFacts(t, stripped); aa {
		t.Error("StripActive left the field's /AA in place while reporting that all active " +
			"content was neutralized")
	}
	// And the residual re-scan the server performs must agree.
	rep2, err := Scan(stripped)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep2.Findings {
		if f.Kind == "additionalActions" {
			t.Errorf("residual scan still reports %s: %s", f.Kind, f.Detail)
		}
	}
}

// fieldTreeFacts walks the AcroForm field tree DIRECTLY, independent of eachFormField, and
// reports whether any field dict carries /AA and whether any PAGE annotation does.
//
// Independent for the reason annotFacts is: a check built on the walker under test cannot
// see the walker failing to reach a node.
func fieldTreeFacts(t *testing.T, pdf []byte) (fieldAA, annotAA bool) {
	t.Helper()
	// ReadValidateAndOptimize, not ReadContext: the latter does not populate the page
	// tree, so xt.PageDict returns "page not found" — the same class of pdfcpu
	// population trap this package records for ctx.PageAnnots and for font dicts.
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-reading the PDF: %v", err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	af, _ := xt.DereferenceDict(root["AcroForm"])
	var walk func(types.Object, int)
	walk = func(o types.Object, depth int) {
		if depth > 32 {
			return
		}
		d, _ := xt.DereferenceDict(o)
		if d == nil {
			return
		}
		if _, ok := d.Find("AA"); ok {
			// A merged field+widget is BOTH; only a node absent from /Annots counts here.
			if nameVal(d, "Subtype") != "Widget" {
				fieldAA = true
			}
		}
		kids, _ := xt.DereferenceArray(d["Kids"])
		for _, k := range kids {
			walk(k, depth+1)
		}
	}
	if af != nil {
		fields, _ := xt.DereferenceArray(af["Fields"])
		for _, f := range fields {
			walk(f, 0)
		}
	}
	pd, _, _, err := xt.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	annots, _ := xt.DereferenceArray(pd["Annots"])
	for _, a := range annots {
		d, _ := xt.DereferenceDict(a)
		if d == nil {
			continue
		}
		if _, ok := d.Find("AA"); ok {
			annotAA = true
		}
	}
	return fieldAA, annotAA
}
