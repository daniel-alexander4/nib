package pdfops

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// readCtx re-reads bytes the way every pdfops door does, so a fixture that pdfcpu would refuse
// fails here rather than three assertions later.
func readCtx(t *testing.T, pdf []byte) *model.Context {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	return ctx
}

// pageObjNrs returns the output's page object numbers in document order.
func pageObjNrs(t *testing.T, ctx *model.Context) []int {
	t.Helper()
	var out []int
	for p := 1; p <= ctx.PageCount; p++ {
		ir, err := ctx.PageDictIndRef(p)
		if err != nil || ir == nil {
			t.Fatalf("page %d has no reference: %v", p, err)
		}
		out = append(out, ir.ObjectNumber.Value())
	}
	return out
}

// nestedInheritingFixture builds the shape no fixture in this package had before: a page tree with
// an intermediate /Pages node that supplies /CropBox and /Rotate to the pages beneath it by
// INHERITANCE, so nothing on those page dictionaries says what they are.
func nestedInheritingFixture(t *testing.T) []byte {
	t.Helper()
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	out, err := writeMutated(src, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		pagesRef := root["Pages"].(types.IndirectRef)
		pages := derefDict(ctx.XRefTable, pagesRef)
		kids := pages["Kids"].(types.Array)
		mid := types.Dict{
			"Type":    types.Name("Pages"),
			"Parent":  pagesRef,
			"Kids":    types.Array{kids[1], kids[2]},
			"Count":   types.Integer(2),
			"CropBox": types.NewNumberArray(20, 20, 400, 500),
			"Rotate":  types.Integer(90),
		}
		midRef, ierr := ctx.XRefTable.IndRefForNewObject(mid)
		if ierr != nil {
			return ierr
		}
		for _, k := range []types.Object{kids[1], kids[2]} {
			d := derefDict(ctx.XRefTable, k)
			d["Parent"] = *midRef
			delete(d, "CropBox")
			delete(d, "Rotate")
		}
		pages["Kids"] = types.Array{kids[0], *midRef}
		pages["Count"] = types.Integer(3)
		return nil
	})
	if err != nil {
		t.Fatalf("build nested fixture: %v", err)
	}
	// The stimulus floor: the fixture must actually inherit, or every assertion below passes for
	// the fixture's reason instead of the code's.
	ctx := readCtx(t, out)
	d, _, inh, err := ctx.PageDict(2, false)
	if err != nil {
		t.Fatalf("nested fixture page 2: %v", err)
	}
	if _, own := d["CropBox"]; own {
		t.Fatal("setup: page 2 carries its own /CropBox, so nothing here tests inheritance")
	}
	if inh.CropBox == nil || inh.Rotate != 90 {
		t.Fatalf("setup: page 2 does not inherit (cropBox=%v rotate=%d)", inh.CropBox, inh.Rotate)
	}
	return out
}

// destFixture builds a document carrying a /Names /Dests name tree with one destination per page.
// Nothing in this package had one, so "prune the destinations that name a dropped page" had no
// population to run over.
func destFixture(t *testing.T) []byte { return destFixtureOf(t, "alpha", "beta", "gamma") }

// destFixtureOf gives every page a named destination.
func destFixtureOf(t *testing.T, pages ...string) []byte {
	t.Helper()
	all := make([]int, len(pages))
	for i := range pages {
		all[i] = i + 1
	}
	return destFixtureNaming(t, pages, all...)
}

// destFixtureNaming gives destinations only to the pages named, so a selection can drop every one.
func destFixtureNaming(t *testing.T, pages []string, destPages ...int) []byte {
	t.Helper()
	src, err := testpdf.Text(pages...)
	if err != nil {
		t.Fatal(err)
	}
	out, err := writeMutated(src, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		var names types.Array
		for _, p := range destPages {
			ir, perr := ctx.PageDictIndRef(p)
			if perr != nil {
				return perr
			}
			// Page 1's destination is written in the DICTIONARY shape (`<< /D [ref /Fit] >>`) and
			// the rest as bare arrays, so both forms the resolver handles are exercised.
			var dest types.Object = types.Array{*ir, types.Name("Fit")}
			if p == 1 {
				dest = types.Dict{"D": types.Array{*ir, types.Name("Fit")}}
			}
			names = append(names, types.StringLiteral(fmt.Sprintf("page%d", p)), dest)
		}
		destsRef, ierr := ctx.XRefTable.IndRefForNewObject(types.Dict{"Names": names})
		if ierr != nil {
			return ierr
		}
		nmRef, ierr := ctx.XRefTable.IndRefForNewObject(types.Dict{"Dests": *destsRef})
		if ierr != nil {
			return ierr
		}
		root["Names"] = *nmRef
		return nil
	})
	if err != nil {
		t.Fatalf("build dest fixture: %v", err)
	}
	ctx := readCtx(t, out)
	if ctx.Names["Dests"] == nil {
		t.Fatal("setup: the fixture's /Dests tree did not survive its own write — nothing below is tested")
	}
	if got := len(destKeys(t, ctx)); got != len(destPages) {
		t.Fatalf("setup: fixture has %d destinations, want %d", got, len(destPages))
	}
	return out
}

func destKeys(t *testing.T, ctx *model.Context) []string {
	t.Helper()
	node := ctx.Names["Dests"]
	if node == nil {
		return nil
	}
	var keys []string
	if err := node.Process(ctx.XRefTable, func(_ *model.XRefTable, k string, _ *types.Object) error {
		keys = append(keys, k)
		return nil
	}); err != nil {
		t.Fatalf("walk dests: %v", err)
	}
	sort.Strings(keys)
	return keys
}

// TestCollectEmitsTheExactOrderAsked pins what TestPageOps never could: it asserts page COUNT only,
// so a selection that silently sorted, deduplicated or reversed itself passed it. Pages are built at
// distinct widths so the order is readable from the output alone.
func TestCollectEmitsTheExactOrderAsked(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{
		rasterPage(t, 80, 110),
		rasterPage(t, 120, 90),
		rasterPage(t, 200, 150),
	})
	if err != nil {
		t.Fatal(err)
	}
	conf := model.NewDefaultConfiguration()
	baseDims, err := api.PageDims(bytes.NewReader(base), conf)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Collect(base, []string{"3", "1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := api.PageDims(bytes.NewReader(out), conf)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 0, 1} // source indices, in the order asked for
	if len(got) != len(want) {
		t.Fatalf("page count = %d, want %d", len(got), len(want))
	}
	for i, srcIdx := range want {
		if got[i].Width != baseDims[srcIdx].Width {
			t.Errorf("output page %d width = %.0f, want %.0f (source page %d)",
				i+1, got[i].Width, baseDims[srcIdx].Width, srcIdx+1)
		}
	}
}

// TestANestedPageTreeKeepsWhatItInherited is the reorder case the plan's original shape could not
// have satisfied: a cross-subtree permutation MOVES a page to a different parent, so whatever it
// inherited has to be written onto it first.
func TestANestedPageTreeKeepsWhatItInherited(t *testing.T) {
	src := nestedInheritingFixture(t)
	before := readCtx(t, src)
	var wantCrop []float64
	for p := 1; p <= before.PageCount; p++ {
		_, _, inh, err := before.PageDict(p, false)
		if err != nil {
			t.Fatal(err)
		}
		w := 0.0
		if inh.CropBox != nil {
			w = inh.CropBox.Width()
		}
		wantCrop = append(wantCrop, w)
	}

	// Page 3 first: it lives under the intermediate node, page 1 does not.
	out, err := Collect(src, []string{"3", "1"})
	if err != nil {
		t.Fatal(err)
	}
	after := readCtx(t, out)
	if after.PageCount != 2 {
		t.Fatalf("page count = %d, want 2", after.PageCount)
	}
	for i, srcPage := range []int{3, 1} {
		_, _, inh, derr := after.PageDict(i+1, false)
		if derr != nil {
			t.Fatal(derr)
		}
		gotCrop := 0.0
		if inh.CropBox != nil {
			gotCrop = inh.CropBox.Width()
		}
		if gotCrop != wantCrop[srcPage-1] {
			t.Errorf("output page %d (source %d): effective /CropBox width = %.0f, want %.0f",
				i+1, srcPage, gotCrop, wantCrop[srcPage-1])
		}
		wantRotate := 0
		if srcPage != 1 {
			wantRotate = 90
		}
		if inh.Rotate != wantRotate {
			t.Errorf("output page %d (source %d): effective /Rotate = %d, want %d",
				i+1, srcPage, inh.Rotate, wantRotate)
		}
		if inh.Resources == nil {
			t.Errorf("output page %d (source %d): lost its /Resources", i+1, srcPage)
		}
	}
	// The clause says the page RENDERS identically, and the four attributes are only the geometry
	// half of that. The other half is that the content stream is the same bytes it always was.
	srcFP := contentFingerprints(t, src)
	outFP := contentFingerprints(t, out)
	for i, srcPage := range []int{3, 1} {
		if outFP[i] != srcFP[srcPage-1] {
			t.Errorf("output page %d is not source page %d's content (%s vs %s)",
				i+1, srcPage, outFP[i], srcFP[srcPage-1])
		}
	}
}

// contentFingerprints hashes each page's decoded content stream, in document order.
func contentFingerprints(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx := readCtx(t, pdf)
	var out []string
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, err := ctx.PageDict(p, false)
		if err != nil {
			t.Fatalf("page %d: %v", p, err)
		}
		var buf bytes.Buffer
		add := func(o types.Object) {
			sd, _, serr := ctx.DereferenceStreamDict(o)
			if serr != nil || sd == nil {
				return
			}
			if derr := sd.Decode(); derr == nil {
				buf.Write(sd.Content)
			}
		}
		if arr, ok := d["Contents"].(types.Array); ok {
			for _, o := range arr {
				add(o)
			}
		} else {
			add(d["Contents"])
		}
		sum := sha256.Sum256(buf.Bytes())
		out = append(out, hex.EncodeToString(sum[:])[:8])
	}
	return out
}

// TestASubsetLeavesExactlyTheCatalogItLeftBefore is the slice's central instrument.
//
// Moving the selection into the source context turns a whitelist into a blacklist: pdfcpu's fresh
// context carried only what it chose to migrate, and an in-place rewrite carries everything it is
// not told to drop. A list of keys somebody enumerated cannot police that — the failure is the key
// nobody thought of. So the oracle is the OLD implementation itself, run side by side.
//
// It compares the /Names subtree too, and not just the catalog's top level: a /Names that stopped
// holding /Dests and started holding /EmbeddedFiles compares equal at the top level, and that is
// exactly the leak this exists to catch.
func TestASubsetLeavesExactlyTheCatalogItLeftBefore(t *testing.T) {
	src := richFixture(t)

	shape := func(pdf []byte) string {
		ctx := readCtx(t, pdf)
		root, err := ctx.XRefTable.Catalog()
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		for k := range root {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var names []string
		for n := range ctx.Names {
			names = append(names, n)
		}
		sort.Strings(names)
		info := "Info:none"
		if ctx.Info != nil {
			if d := derefDict(ctx.XRefTable, *ctx.Info); d != nil {
				if s := d.StringLiteralEntry("Title"); s != nil {
					info = "Info:Title=" + s.Value()
				} else {
					info = "Info:noTitle"
				}
			}
		}
		return fmt.Sprintf("catalog=%v names=%v %s", keys, names, info)
	}

	// The floor. Without it both sides collapse to the same empty shape if the fixture ever stops
	// carrying an outline, labels, a name tree or a title — and the test greens on nothing.
	srcShape := shape(src)
	for _, want := range []string{"Outlines", "PageLabels", "Names", "Info:Title=",
		"Metadata", "ViewerPreferences", "OpenAction", "PageMode"} {
		if !strings.Contains(srcShape, want) {
			t.Fatalf("setup: the fixture does not carry %s (%s) — this test would compare two "+
				"empty catalogs and pass having graded nothing", want, srcShape)
		}
	}

	for _, c := range []struct {
		name string
		mine func([]byte) ([]byte, error)
		old  func([]byte) ([]byte, error)
	}{
		{"Collect", func(b []byte) ([]byte, error) { return Collect(b, []string{"3", "1"}) }, oldCollect},
		{"RemovePages", func(b []byte) ([]byte, error) { return RemovePages(b, []string{"2"}) }, oldRemovePages},
	} {
		mine, err := c.mine(src)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		old, err := c.old(src)
		if err != nil {
			t.Fatalf("%s (old): %v", c.name, err)
		}
		if got, want := shape(mine), shape(old); got != want {
			t.Errorf("%s:\n new: %s\n old: %s", c.name, got, want)
		}
	}
}

// oldCollect and oldRemovePages are the implementations this slice replaced, kept as the comparison
// oracle above and reachable from nowhere else.
func oldCollectOrder(pdf []byte, order []string) ([]byte, error) {
	var out bytes.Buffer
	if err := api.Collect(bytes.NewReader(pdf), &out, order, nil); err != nil {
		return nil, err
	}
	return carryLang(pdf, out.Bytes())
}

func oldCollect(pdf []byte) ([]byte, error) { return oldCollectOrder(pdf, []string{"3", "1"}) }

func oldRemovePages(pdf []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := api.RemovePages(bytes.NewReader(pdf), &out, []string{"2"}, nil); err != nil {
		return nil, err
	}
	return carryLang(pdf, out.Bytes())
}

// formOnPagesFixture puts an ordinary (non-signature) text field on each of pages 2 and 3, as a
// merged field/widget — the shape most producers emit. Every form fixture in this package is one
// page, so before this nothing could ask what a SUBSET does to a form.
func formOnPagesFixture(t *testing.T) []byte {
	t.Helper()
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	out, err := writeMutated(src, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		var fields types.Array
		for _, page := range []int{2, 3} {
			pageRef, perr := ctx.PageDictIndRef(page)
			if perr != nil {
				return perr
			}
			field := types.Dict{
				"Type":    types.Name("Annot"),
				"Subtype": types.Name("Widget"),
				"FT":      types.Name("Tx"),
				"T":       types.StringLiteral(fmt.Sprintf("field_p%d", page)),
				"Rect":    types.NewNumberArray(50, 50, 250, 90),
				"P":       *pageRef,
				"F":       types.Integer(4),
				"DA":      types.StringLiteral("/Helv 0 Tf 0 g"),
			}
			fref, ierr := ctx.XRefTable.IndRefForNewObject(field)
			if ierr != nil {
				return ierr
			}
			pd := derefDict(ctx.XRefTable, *pageRef)
			pd["Annots"] = append(derefArray(ctx.XRefTable, pd["Annots"]), *fref)
			fields = append(fields, *fref)
		}
		formRef, ierr := ctx.XRefTable.IndRefForNewObject(types.Dict{"Fields": fields, "DA": types.StringLiteral("/Helv 0 Tf 0 g")})
		if ierr != nil {
			return ierr
		}
		root["AcroForm"] = *formRef
		return nil
	})
	if err != nil {
		t.Fatalf("build form fixture: %v", err)
	}
	return out
}

func formFieldNames(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx := readCtx(t, pdf)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	form := derefDict(ctx.XRefTable, root["AcroForm"])
	if form == nil {
		return nil
	}
	var names []string
	for _, f := range derefArray(ctx.XRefTable, form["Fields"]) {
		fd := derefDict(ctx.XRefTable, f)
		if fd == nil {
			continue
		}
		if s := fd.StringLiteralEntry("T"); s != nil {
			names = append(names, s.Value())
		}
	}
	sort.Strings(names)
	return names
}

// TestASubsetKeepsTheFormFieldsWhoseWidgetsSurvive is parity against the old implementation, which
// got this for free: pdfcpu's `migrateFields` rebuilt the destination AcroForm from the widgets on
// the pages it migrated.
//
// It exists because the first cut of the in-place prune deleted EVERY AcroForm — it rebuilt page
// references with `types.NewIndirectRef`, whose pointer result `DereferenceDict` does not match, so
// the set of surviving widgets was always empty. The whole suite stayed green, because every other
// form fixture here is a single page and no test had ever subset a form.
func TestASubsetKeepsTheFormFieldsWhoseWidgetsSurvive(t *testing.T) {
	src := formOnPagesFixture(t)
	if got := formFieldNames(t, src); len(got) != 2 {
		t.Fatalf("setup: fixture has fields %v, want two — nothing below is tested otherwise", got)
	}
	for _, c := range []struct {
		name  string
		order []string
	}{
		{"keep the page with one field", []string{"2"}},
		{"keep both field pages, reordered", []string{"3", "2"}},
		{"keep only a page with no field", []string{"1"}},
	} {
		mine, err := Collect(src, c.order)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		old, err := oldCollectOrder(src, c.order)
		if err != nil {
			t.Fatalf("%s (old): %v", c.name, err)
		}
		got, want := formFieldNames(t, mine), formFieldNames(t, old)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: fields = %v, want %v (what the previous implementation kept)",
				c.name, got, want)
		}
	}
}

// richFixture carries one of everything a subset has to decide about.
func richFixture(t *testing.T) []byte {
	t.Helper()
	src := destFixture(t)
	var err error
	if src, err = SetOutline(src, []OutlineItem{{Title: "First", Page: 1}, {Title: "Third", Page: 3}}); err != nil {
		t.Fatal(err)
	}
	if src, err = SetPageLabels(src, []PageLabelRange{{Start: 1, Style: "roman-lower"}}); err != nil {
		t.Fatal(err)
	}
	if src, err = AddAttachment(src, "notes.txt", []byte("an embedded file")); err != nil {
		t.Fatal(err)
	}
	src, err = writeMutated(src, func(ctx *model.Context) error {
		if ctx.Info == nil {
			ref, ierr := ctx.XRefTable.IndRefForNewObject(types.Dict{})
			if ierr != nil {
				return ierr
			}
			ctx.Info = ref
		}
		d := derefDict(ctx.XRefTable, *ctx.Info)
		d["Title"] = types.StringLiteral("SECRET-CLIENT-MATTER")
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		// The rest of what a real catalog carries and a subset must decide about.
		root["ViewerPreferences"] = types.Dict{"DisplayDocTitle": types.Boolean(true)}
		root["PageMode"] = types.Name("UseOutlines")
		first, perr := ctx.PageDictIndRef(1)
		if perr != nil {
			return perr
		}
		root["OpenAction"] = types.Array{*first, types.Name("Fit")}
		packet := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title><rdf:Alt><rdf:li xml:lang="x-default">SECRET-CLIENT-MATTER</rdf:li></rdf:Alt></dc:title></rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`
		sd, serr := ctx.NewStreamDictForBuf([]byte(packet))
		if serr != nil {
			return serr
		}
		sd.InsertName("Type", "Metadata")
		sd.InsertName("Subtype", "XML")
		if eerr := sd.Encode(); eerr != nil {
			return eerr
		}
		mref, ierr := ctx.XRefTable.IndRefForNewObject(*sd)
		if ierr != nil {
			return ierr
		}
		root["Metadata"] = *mref
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// TestANamedDestinationToADroppedPageDoesNotSurvive drives the /Dests prune. pdfcpu's own migration
// does not remove such a destination — it patches the reference through a lookup the dropped page is
// absent from, producing `0 0 R`.
func TestANamedDestinationToADroppedPageDoesNotSurvive(t *testing.T) {
	src := destFixture(t)
	out, err := Collect(src, []string{"3", "1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	got := destKeys(t, ctx)
	for _, k := range got {
		if strings.Contains(k, "page2") {
			t.Errorf("the destination for the dropped page survived: %v", got)
		}
	}
	if len(got) != 2 {
		t.Errorf("destinations = %v, want the two naming kept pages", got)
	}
	live := map[int]bool{}
	for _, nr := range pageObjNrs(t, ctx) {
		live[nr] = true
	}
	node := ctx.Names["Dests"]
	if node == nil {
		t.Fatal("the whole /Dests tree went, not just the dropped page's destination")
	}
	if err := node.Process(ctx.XRefTable, func(xt *model.XRefTable, k string, v *types.Object) error {
		if !destNamesAKeptPage(xt, *v, live) {
			t.Errorf("destination %q does not resolve to a page of the output", k)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestDuplicatePageEmitsDistinctPageObjects — a page tree that reaches one object twice is refused
// outright by pdfcpu (ErrPageTreeDuplicate), so the clone is mandatory, and a shared /Annots array
// would put one annotation, whose /P names a single page, on two.
func TestDuplicatePageEmitsDistinctPageObjects(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	out, err := DuplicatePage(src, 2)
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	if ctx.PageCount != 4 {
		t.Fatalf("page count = %d, want 4", ctx.PageCount)
	}
	nrs := pageObjNrs(t, ctx)
	seen := map[int]bool{}
	for i, nr := range nrs {
		if seen[nr] {
			t.Errorf("page %d reuses object %d — the page tree names one object twice (%v)", i+1, nr, nrs)
		}
		seen[nr] = true
	}
}

// TestASubsetLeavesNoTraceOfADroppedPage is a BYTE-level assertion and not a page-tree one.
//
// pdfcpu writes by reachability, so unlinking a page is normally enough — but anything still
// reachable from the catalog that names that page re-anchors it, and a structure element's /Pg is
// exactly such a reference. "Removed" and "hidden" are indistinguishable from the page tree alone.
func TestASubsetLeavesNoTraceOfADroppedPage(t *testing.T) {
	const canary = "ZZQXCANARYZZ"
	src, err := testpdf.Text("alpha", canary, "gamma")
	if err != nil {
		t.Fatal(err)
	}
	if raw, streams := fileCarries(src, canary); !raw && streams == 0 {
		t.Fatal("setup: the canary is not in the source at all, so its absence below proves nothing")
	}
	for _, c := range []struct {
		name string
		op   func([]byte) ([]byte, error)
	}{
		{"Collect", func(b []byte) ([]byte, error) { return Collect(b, []string{"3", "1"}) }},
		{"RemovePages", func(b []byte) ([]byte, error) { return RemovePages(b, []string{"2"}) }},
	} {
		out, oerr := c.op(src)
		if oerr != nil {
			t.Fatalf("%s: %v", c.name, oerr)
		}
		if raw, streams := fileCarries(out, canary); raw || streams > 0 {
			t.Errorf("%s: the dropped page's text is still in the output (raw=%v streams=%d)",
				c.name, raw, streams)
		}
	}
}

// TestASubsetDoesNotCarryTheDocumentsInfo — /Info travels into every extract and split artifact if
// an in-place rewrite keeps it, and it does not survive today. Measured before the change: a title
// and nib's own NibFlags were both gone.
func TestASubsetDoesNotCarryTheDocumentsInfo(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	src, err = writeMutated(src, func(ctx *model.Context) error {
		if ctx.Info == nil {
			ref, ierr := ctx.XRefTable.IndRefForNewObject(types.Dict{})
			if ierr != nil {
				return ierr
			}
			ctx.Info = ref
		}
		d := derefDict(ctx.XRefTable, *ctx.Info)
		d["Title"] = types.StringLiteral("SECRET-CLIENT-MATTER")
		d["NibFlags"] = types.StringLiteral("flagpayload")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before := readCtx(t, src)
	if d := derefDict(before.XRefTable, *before.Info); d == nil || d.StringLiteralEntry("Title") == nil {
		t.Fatal("setup: the fixture has no /Info title, so its absence below proves nothing")
	}
	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	if ctx.Info == nil {
		return
	}
	d := derefDict(ctx.XRefTable, *ctx.Info)
	if d == nil {
		return
	}
	if s := d.StringLiteralEntry("Title"); s != nil {
		t.Errorf("the document title rode into the subset: %q", s.Value())
	}
	if s := d.StringLiteralEntry("NibFlags"); s != nil {
		t.Errorf("NibFlags rode into the subset: %q", s.Value())
	}
}

// TestABadPageSelectionIsReportedInNibsVoice — pdfcpu's own text names a library the user did not
// choose to use, and it reaches them through /api/pages and `nib pages`, both of which hand a raw
// user string straight to the selector.
func TestABadPageSelectionIsReportedInNibsVoice(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		op   func() ([]byte, error)
	}{
		{"no page selected", func() ([]byte, error) { return Collect(src, []string{"9"}) }},
		{"unreadable token", func() ([]byte, error) { return Collect(src, []string{"banana"}) }},
		{"remove everything", func() ([]byte, error) { return RemovePages(src, []string{"1-2"}) }},
	} {
		_, err := c.op()
		if err == nil {
			t.Errorf("%s: expected a refusal, got none", c.name)
			continue
		}
		if strings.Contains(err.Error(), "pdfcpu:") {
			t.Errorf("%s: the refusal names pdfcpu: %v", c.name, err)
		}
		if !strings.HasPrefix(err.Error(), "pdfops:") {
			t.Errorf("%s: the refusal is not in nib's voice: %v", c.name, err)
		}
	}
}

// TestAMalformedPageTreeIsRefusedNotPanicked — fault.Catch recovers only pdfcpu's own fault.Panic
// and re-panics everything else, so a nil dereference in the walk would kill the process rather than
// fail the operation.
func TestAMalformedPageTreeIsRefusedNotPanicked(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta")
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, src)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	pagesRef := root["Pages"].(types.IndirectRef)
	pages := derefDict(ctx.XRefTable, pagesRef)

	// A /Kids entry that is a number rather than a reference, and a self-referential tree: both are
	// shapes a hand-rolled walk trips over, and neither can be produced by writing a valid file, so
	// they are driven against the walk directly.
	for _, c := range []struct {
		name string
		kids types.Array
	}{
		{"a kid that is not a reference", types.Array{types.Integer(7)}},
		{"a node that is its own kid", types.Array{pagesRef}},
	} {
		pages["Kids"] = c.kids
		pages["Count"] = types.Integer(1)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panicked instead of returning an error: %v", c.name, r)
				}
			}()
			if _, _, werr := collectLeaves(ctx.XRefTable, root); werr == nil {
				t.Errorf("%s: accepted a malformed page tree", c.name)
			}
		}()
	}
}

// TestASubsetErasesASignatureWhoseWidgetIsOnAKeptPage covers the case nib's OWN signatures cannot
// reach, and it exists because a mutation proved the guard inert without it.
//
// Measured: a nib approval signature's field is `FT /Sig` with no kids and is referenced from no
// page's /Annots, so pruning the AcroForm to fields with a surviving widget removes it on its own —
// disabling `dropSignature` left `TestWhatEachDocumentPrimitiveDoesToASignature` green. A document
// signed elsewhere with a VISIBLE signature is the other shape, and it is the one a user actually
// receives and reorders: its widget sits on a page, so the field survives the prune and the blob
// would be left behind as rubble.
//
// That matters beyond tidiness. Four gates key on a document having no signature — ADR-013's three
// `DocHash` anchors, which would go quiet rather than fire; `p2p.ContributionProgress`, which
// hard-refuses `sign.Invalid`; `sign.SignApproval`'s /DocMDP refusal; and
// `DropUAIdentificationUnlessSigned` at three call sites.
func TestASubsetErasesASignatureWhoseWidgetIsOnAKeptPage(t *testing.T) {
	base, err := testpdf.Text("one", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := sign.GenerateIdentity("Signer")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.SignApproval(base, cert, key, sign.Options{Name: "A", Reason: "r"})
	if err != nil {
		t.Fatal(err)
	}
	// Give the signature a visible widget on page 1 — the merged field/widget shape other producers
	// emit. The rewrite invalidates the signature, which is irrelevant here: the property under test
	// is whether a page operation leaves a BLOB behind, not whether it verifies.
	visible, err := writeMutated(signed, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		form := derefDict(ctx.XRefTable, root["AcroForm"])
		if form == nil {
			return fmt.Errorf("the signed fixture has no AcroForm")
		}
		fields := derefArray(ctx.XRefTable, form["Fields"])
		if len(fields) == 0 {
			return fmt.Errorf("the signed fixture has no form field")
		}
		fr := fields[0].(types.IndirectRef)
		fd := derefDict(ctx.XRefTable, fr)
		pageRef, perr := ctx.PageDictIndRef(1)
		if perr != nil {
			return perr
		}
		fd["Subtype"] = types.Name("Widget")
		fd["Rect"] = types.NewNumberArray(50, 50, 250, 110)
		fd["P"] = *pageRef
		page := derefDict(ctx.XRefTable, *pageRef)
		page["Annots"] = append(derefArray(ctx.XRefTable, page["Annots"]), fr)
		return nil
	})
	if err != nil {
		t.Fatalf("build visible-signature fixture: %v", err)
	}

	// The stimulus floor, twice over: the fixture must carry a blob, and its signature field must be
	// on a page the subset KEEPS — otherwise the prune removes it and this test grades nothing.
	if !sign.HasSignatureBlob(visible) {
		t.Fatal("setup: the fixture carries no signature blob, so its absence below proves nothing")
	}
	ctx := readCtx(t, visible)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	form := derefDict(ctx.XRefTable, root["AcroForm"])
	if form == nil {
		t.Fatal("setup: the fixture lost its AcroForm")
	}
	fields := derefArray(ctx.XRefTable, form["Fields"])
	if len(fields) == 0 {
		t.Fatal("setup: the fixture lost its signature field")
	}
	sigRef := fields[0].(types.IndirectRef)
	page1 := derefDict(ctx.XRefTable, mustPageRef(t, ctx, 1))
	found := false
	for _, a := range derefArray(ctx.XRefTable, page1["Annots"]) {
		if ar, ok := a.(types.IndirectRef); ok && ar.ObjectNumber == sigRef.ObjectNumber {
			found = true
		}
	}
	if !found {
		t.Fatal("setup: the signature field is not on page 1, so the AcroForm prune would erase it " +
			"on its own and this test would pass without exercising the signature drop")
	}

	out, err := Collect(visible, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if sign.HasSignatureBlob(out) {
		t.Error("a subset that kept the signature's own page left the signature blob behind; " +
			"four gates key on a document having no signature and all four would change quietly")
	}
}

func mustPageRef(t *testing.T, ctx *model.Context, page int) types.IndirectRef {
	t.Helper()
	ref, err := ctx.PageDictIndRef(page)
	if err != nil || ref == nil {
		t.Fatalf("page %d has no reference: %v", page, err)
	}
	return *ref
}

// TestHalfTheDestinationsDroppedLeavesTheOtherHalf is the case the three-destination fixture cannot
// reach. The emptiness test after the prune originally compared the number of destinations REMOVED
// against the number REMAINING — equal exactly when half of them pointed at dropped pages — so a
// two-destination document that lost one lost the whole name tree, and with it the destination that
// was still perfectly good.
func TestHalfTheDestinationsDroppedLeavesTheOtherHalf(t *testing.T) {
	src := destFixtureOf(t, "alpha", "beta")
	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	got := destKeys(t, ctx)
	if len(got) != 1 {
		t.Fatalf("destinations = %v, want exactly the one naming the kept page", got)
	}
	if !strings.Contains(got[0], "page1") {
		t.Errorf("the surviving destination is %q, want the one naming page 1", got[0])
	}
}

// TestEveryDestinationGoingLeavesNoNameTree drives the branch its sibling cannot: when NOTHING
// survives the prune, the tree and the catalog's /Names key go with it rather than lingering empty.
//
// It needs a fixture whose destinations all name pages the subset drops, which `destFixtureOf`
// cannot give — it puts one destination on every page, so any selection keeps at least one.
func TestEveryDestinationGoingLeavesNoNameTree(t *testing.T) {
	src := destFixtureNaming(t, []string{"alpha", "beta", "gamma"}, 2, 3)
	before := destKeys(t, readCtx(t, src))
	if len(before) != 2 {
		t.Fatalf("setup: fixture has destinations %v, want two, both naming pages 2 and 3", before)
	}
	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	if got := destKeys(t, ctx); len(got) != 0 {
		t.Errorf("destinations = %v, want none — every one named a dropped page", got)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, has := root["Names"]; has {
		t.Error("the catalog kept a /Names key with nothing under it")
	}
}

// TestASurvivingDestinationDoesNotDragTheAttachmentsWithIt is the leak the attachment tests cannot
// see, and the reason they cannot is worth stating: they use documents with no named destinations.
//
// The catalog's name trees are internalized into `ctx.Names` at validation and re-bound at write by
// `BindNameTrees`, which only ever `Update`s `"Dests"` — it never removes a sibling subtree. So
// clearing `ctx.Names` is sufficient exactly when no destination survives, because then the whole
// `/Names` key is deleted and everything under it goes with it. Give the same document ONE surviving
// destination and `/Names` must stay — at which point `/EmbeddedFiles` rides out with it, into every
// extract, split and redaction.
func TestASurvivingDestinationDoesNotDragTheAttachmentsWithIt(t *testing.T) {
	src := destFixture(t)
	src, err := AddAttachment(src, "original-unredacted.txt", []byte("SECRET PAYROLL"))
	if err != nil {
		t.Fatal(err)
	}
	// Both halves of the stimulus, asserted: an attachment to leak, and a destination that will
	// survive the subset and so keep `/Names` alive.
	before, err := Attachments(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 {
		t.Fatalf("setup: fixture carries %d attachment(s), want 1", len(before))
	}
	if got := len(destKeys(t, readCtx(t, src))); got == 0 {
		t.Fatal("setup: fixture has no destinations, so /Names would be deleted wholesale and " +
			"this test would pass without exercising the leak")
	}

	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(destKeys(t, readCtx(t, out))); got == 0 {
		t.Fatal("no destination survived the subset, so /Names went wholesale and the leak was " +
			"not exercised — the fixture, not the code, decided this")
	}
	after, err := Attachments(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Errorf("the subset carried %d attachment(s) out with the surviving destination: %v",
			len(after), after)
	}
	if raw, streams := fileCarries(out, "SECRET PAYROLL"); raw || streams > 0 {
		t.Errorf("the embedded payload is still in the output bytes (raw=%v streams=%d)", raw, streams)
	}
}

// TestAKeptFieldDoesNotKeepAWidgetOnADroppedPage is the re-anchoring case, and it is why pruning
// `/Fields` alone is not enough.
//
// A form field with widgets on two pages survives a subset that keeps one of them — correctly. But
// its `/Kids` still referenced the other widget, whose `/P` names the page this operation dropped,
// and pdfcpu writes by REACHABILITY: the dropped page's dictionary and its `/Contents` went into the
// output. The page was hidden, not removed, and no page-tree assertion can tell those apart.
func TestAKeptFieldDoesNotKeepAWidgetOnADroppedPage(t *testing.T) {
	const canary = "ZZDROPPEDPAGEZZ"
	src, err := testpdf.Text("alpha", canary)
	if err != nil {
		t.Fatal(err)
	}
	src, err = writeMutated(src, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		parent := types.Dict{"FT": types.Name("Tx"), "T": types.StringLiteral("shared")}
		pref, ierr := ctx.XRefTable.IndRefForNewObject(parent)
		if ierr != nil {
			return ierr
		}
		var kids types.Array
		for _, page := range []int{1, 2} {
			pageRef, perr := ctx.PageDictIndRef(page)
			if perr != nil {
				return perr
			}
			w := types.Dict{
				"Type": types.Name("Annot"), "Subtype": types.Name("Widget"),
				"Rect": types.NewNumberArray(50, 50, 250, 90), "P": *pageRef,
				"Parent": *pref, "F": types.Integer(4),
				"DA": types.StringLiteral("/Helv 0 Tf 0 g"),
			}
			wref, werr := ctx.XRefTable.IndRefForNewObject(w)
			if werr != nil {
				return werr
			}
			pd := derefDict(ctx.XRefTable, *pageRef)
			pd["Annots"] = append(derefArray(ctx.XRefTable, pd["Annots"]), *wref)
			kids = append(kids, *wref)
		}
		parent["Kids"] = kids
		formRef, ierr := ctx.XRefTable.IndRefForNewObject(
			types.Dict{"Fields": types.Array{*pref}, "DA": types.StringLiteral("/Helv 0 Tf 0 g")})
		if ierr != nil {
			return ierr
		}
		root["AcroForm"] = *formRef
		return nil
	})
	if err != nil {
		t.Fatalf("build shared-field fixture: %v", err)
	}
	// The floor: the field must really span both pages, or the prune has nothing to get wrong.
	ctx := readCtx(t, src)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	form := derefDict(ctx.XRefTable, root["AcroForm"])
	if form == nil {
		t.Fatal("setup: the fixture lost its AcroForm")
	}
	fields := derefArray(ctx.XRefTable, form["Fields"])
	if len(fields) != 1 {
		t.Fatalf("setup: fixture has %d top-level field(s), want 1", len(fields))
	}
	if n := len(derefArray(ctx.XRefTable, derefDict(ctx.XRefTable, fields[0])["Kids"])); n != 2 {
		t.Fatalf("setup: the field has %d widget(s), want 2 — one per page", n)
	}
	if raw, streams := fileCarries(src, canary); !raw && streams == 0 {
		t.Fatal("setup: the canary is not in the source, so its absence below proves nothing")
	}

	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if raw, streams := fileCarries(out, canary); raw || streams > 0 {
		t.Errorf("the dropped page's text is still in the output, re-anchored through the kept "+
			"field's other widget (raw=%v streams=%d)", raw, streams)
	}
	after := readCtx(t, out)
	aroot, err := after.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	aform := derefDict(after.XRefTable, aroot["AcroForm"])
	if aform == nil {
		t.Fatal("the field was dropped entirely; its widget on the kept page should have saved it")
	}
	af := derefArray(after.XRefTable, aform["Fields"])
	if len(af) != 1 {
		t.Fatalf("fields = %d, want the one whose widget survived", len(af))
	}
	if n := len(derefArray(after.XRefTable, derefDict(after.XRefTable, af[0])["Kids"])); n != 1 {
		t.Errorf("the surviving field still lists %d widget(s), want 1", n)
	}
}

// TestALinkToADroppedPageIsUnlinked is the same failure through the other door: an annotation on a
// page that SURVIVES, whose destination names a page that does not.
func TestALinkToADroppedPageIsUnlinked(t *testing.T) {
	const canary = "ZZLINKTARGETZZ"
	src, err := testpdf.Text("alpha", "beta", canary)
	if err != nil {
		t.Fatal(err)
	}
	src, err = writeMutated(src, func(ctx *model.Context) error {
		target, perr := ctx.PageDictIndRef(3)
		if perr != nil {
			return perr
		}
		first, perr := ctx.PageDictIndRef(1)
		if perr != nil {
			return perr
		}
		link := types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("Link"),
			"Rect": types.NewNumberArray(50, 50, 250, 90), "P": *first,
			"Dest": types.Array{*target, types.Name("Fit")},
		}
		lref, ierr := ctx.XRefTable.IndRefForNewObject(link)
		if ierr != nil {
			return ierr
		}
		pd := derefDict(ctx.XRefTable, *first)
		pd["Annots"] = append(derefArray(ctx.XRefTable, pd["Annots"]), *lref)
		return nil
	})
	if err != nil {
		t.Fatalf("build link fixture: %v", err)
	}
	if raw, streams := fileCarries(src, canary); !raw && streams == 0 {
		t.Fatal("setup: the canary is not in the source, so its absence below proves nothing")
	}

	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if raw, streams := fileCarries(out, canary); raw || streams > 0 {
		t.Errorf("the dropped page's text is still in the output, re-anchored through a link on "+
			"the kept page (raw=%v streams=%d)", raw, streams)
	}
	ctx := readCtx(t, out)
	page1 := derefDict(ctx.XRefTable, mustPageRef(t, ctx, 1))
	for _, a := range derefArray(ctx.XRefTable, page1["Annots"]) {
		ad := derefDict(ctx.XRefTable, a)
		if ad == nil {
			continue
		}
		if _, has := ad["Dest"]; has {
			t.Error("the link kept a destination naming a page that is gone")
		}
	}
}

// TestATaggedSubsetEARNSItsStructureKeys grades the allowlist's sharpest pair from the other side.
//
// **It asserted the opposite until P02.S04b**, and the inversion is the slice: the catalog is still a
// default-deny allowlist, and `/StructTreeRoot` and `/MarkInfo` are now restored ON TOP of it, only
// when the carry pruned the tree onto the pages kept and found it still anchored to them. So the keys
// are earned rather than allowlisted, and a refused carry writes exactly the document this test used
// to assert. The refusal's own reader is `TestACarryThatAnchorsNothingIsRefused`.
func TestATaggedSubsetEARNSItsStructureKeys(t *testing.T) {
	src := repeatedPagesFixture()
	before := readCtx(t, src)
	broot, err := before.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if broot["StructTreeRoot"] == nil || broot["MarkInfo"] == nil {
		t.Fatal("setup: the fixture is not tagged, so this test grades nothing")
	}
	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"StructTreeRoot", "MarkInfo"} {
		if _, has := root[k]; !has {
			t.Errorf("a subset of a tagged document did not carry /%s. The two go together: a tree "+
				"without the claim describes a document that does not say it is tagged, and "+
				"/MarkInfo without a tree is the one shape orphaned() fires on", k)
		}
	}
	// The dropped page's element went with it, so nothing in the tree names a page that is gone.
	if v, defects, orphans, elems := carryOf(t, out); v != "carried" || len(defects) > 0 || len(orphans) > 0 || elems != 1 {
		t.Errorf("the carried subset is %s with %d defect(s), %d orphan page(s) and %d element(s), "+
			"want a clean carried document holding the one element that describes the page it kept",
			v, len(defects), len(orphans), elems)
	}
}

// TestADuplicatedPageGetsItsOwnAnnotations reaches `clonePage`'s annotation path, which no test
// touched: the page `DuplicatePage` was driven over had no `/Annots` at all, so the code the
// function's own comment calls mandatory was graded by nothing.
func TestADuplicatedPageGetsItsOwnAnnotations(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	src, err = writeMutated(src, func(ctx *model.Context) error {
		pageRef, perr := ctx.PageDictIndRef(2)
		if perr != nil {
			return perr
		}
		note := types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("Text"),
			"Rect": types.NewNumberArray(10, 10, 30, 30), "P": *pageRef,
			"Contents": types.StringLiteral("a note"),
		}
		nref, ierr := ctx.XRefTable.IndRefForNewObject(note)
		if ierr != nil {
			return ierr
		}
		pd := derefDict(ctx.XRefTable, *pageRef)
		pd["Annots"] = types.Array{*nref}
		return nil
	})
	if err != nil {
		t.Fatalf("build annotated fixture: %v", err)
	}
	out, err := DuplicatePage(src, 2)
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	if ctx.PageCount != 4 {
		t.Fatalf("page count = %d, want 4", ctx.PageCount)
	}
	// Pages 2 and 3 are the original and its copy. Each must own its annotation, and each
	// annotation's /P must name the page it is actually on.
	seen := map[int]bool{}
	for _, p := range []int{2, 3} {
		ref := mustPageRef(t, ctx, p)
		pd := derefDict(ctx.XRefTable, ref)
		annots := derefArray(ctx.XRefTable, pd["Annots"])
		if len(annots) != 1 {
			t.Fatalf("page %d has %d annotation(s), want 1", p, len(annots))
		}
		ar, ok := annots[0].(types.IndirectRef)
		if !ok {
			t.Fatalf("page %d's annotation is not a reference", p)
		}
		if seen[ar.ObjectNumber.Value()] {
			t.Errorf("pages 2 and 3 share annotation object %d; one annotation cannot be on two "+
				"pages, since its /P names exactly one", ar.ObjectNumber.Value())
		}
		seen[ar.ObjectNumber.Value()] = true
		ad := derefDict(ctx.XRefTable, ar)
		pRef, ok := ad["P"].(types.IndirectRef)
		if !ok {
			t.Errorf("page %d's annotation has no /P", p)
			continue
		}
		if pRef.ObjectNumber != ref.ObjectNumber {
			t.Errorf("page %d's annotation names page object %d, want %d",
				p, pRef.ObjectNumber.Value(), ref.ObjectNumber.Value())
		}
	}
}

// TestASubsetDoesNotCarryTheDocumentsPermanentID — `/ID[0]` is a permanent document identifier, and
// pdfcpu preserves it while minting only `/ID[1]`. The previous implementation had no ID to preserve
// because it built its context from nothing, so every extract and split got a fresh pair; carrying
// it would tie every derived artifact back to the document it came from.
func TestASubsetDoesNotCarryTheDocumentsPermanentID(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	before := readCtx(t, src)
	if len(before.ID) == 0 {
		t.Skip("the fixture carries no /ID, so there is nothing to carry across")
	}
	was := before.ID[0]
	out, err := Collect(src, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	after := readCtx(t, out)
	if len(after.ID) > 0 && after.ID[0] == was {
		t.Errorf("the subset kept the source's permanent /ID[0], which ties the artifact to the "+
			"document it was derived from: %v", was)
	}
}

// TestSelectPagesRefusesASelectionItCannotHonour reaches `selectPages`' own bounds checks. Its two
// production callers pre-empt them — `PagesForPageCollection` errors on an empty selection and
// `RemovePages` refuses "every page" first — so without this the guards had no reader at all.
func TestSelectPagesRefusesASelectionItCannotHonour(t *testing.T) {
	src, err := testpdf.Text("alpha", "beta")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		keep []int
		want string
	}{
		{"nothing named", nil, "does not name any page"},
		{"past the end", []int{3}, "is not in this document"},
		{"zero", []int{0}, "is not in this document"},
		{"negative", []int{-1}, "is not in this document"},
	} {
		_, err := writeMutated(src, func(ctx *model.Context) error {
			_, serr := selectPages(ctx, c.keep, false)
			return serr
		})
		if err == nil {
			t.Errorf("%s: expected a refusal, got none", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: refusal was %q, want it to mention %q", c.name, err, c.want)
		}
	}
}

// TestAPagesOwnValueBeatsWhatItWouldInherit — the walk copies each node's own attributes over what
// it inherits, for its whole subtree. The nested fixture deletes those keys from its children, so
// only pure inheritance was graded and the shadowing rule was untested.
func TestAPagesOwnValueBeatsWhatItWouldInherit(t *testing.T) {
	src := nestedInheritingFixture(t)
	src, err := writeMutated(src, func(ctx *model.Context) error {
		// Page 3 keeps its own rotation, against the 90 its parent supplies.
		ref, perr := ctx.PageDictIndRef(3)
		if perr != nil {
			return perr
		}
		derefDict(ctx.XRefTable, *ref)["Rotate"] = types.Integer(180)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before := readCtx(t, src)
	if _, _, inh, derr := before.PageDict(3, false); derr != nil || inh.Rotate != 180 {
		t.Fatalf("setup: page 3's own /Rotate does not win (rotate=%v err=%v)", inh, derr)
	}
	out, err := Collect(src, []string{"3", "2"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	_, _, own, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	if own.Rotate != 180 {
		t.Errorf("the page's own /Rotate became %d, want 180 — its parent's 90 overrode it", own.Rotate)
	}
	_, _, sibling, err := ctx.PageDict(2, false)
	if err != nil {
		t.Fatal(err)
	}
	if sibling.Rotate != 90 {
		t.Errorf("the sibling that had no /Rotate of its own became %d, want the inherited 90",
			sibling.Rotate)
	}
	if sibling.MediaBox == nil {
		t.Error("the sibling lost its /MediaBox, which is inheritable and must be materialized")
	}
}

// TestASubsetErasesASignatureSplitFromItsWidget covers the other producer shape: a `/FT /Sig` field
// whose widget is a separate `/Kids` entry rather than merged into the field itself. Only the merged
// shape was graded, so the recursion that reaches a signature's widgets ran in no test — and a
// widget left on a kept page keeps `/V`, and with it the blob.
func TestASubsetErasesASignatureSplitFromItsWidget(t *testing.T) {
	base, err := testpdf.Text("alpha", "beta")
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := sign.GenerateIdentity("Signer")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.SignApproval(base, cert, key, sign.Options{Name: "A", Reason: "r"})
	if err != nil {
		t.Fatal(err)
	}
	split, err := writeMutated(signed, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		form := derefDict(ctx.XRefTable, root["AcroForm"])
		if form == nil {
			return fmt.Errorf("the signed fixture has no AcroForm")
		}
		fields := derefArray(ctx.XRefTable, form["Fields"])
		if len(fields) == 0 {
			return fmt.Errorf("the signed fixture has no form field")
		}
		fr := fields[0].(types.IndirectRef)
		fd := derefDict(ctx.XRefTable, fr)
		pageRef, perr := ctx.PageDictIndRef(1)
		if perr != nil {
			return perr
		}
		// The widget becomes a separate object under the field's /Kids, on a page the subset keeps.
		w := types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("Widget"),
			"Rect": types.NewNumberArray(50, 50, 250, 110), "P": *pageRef, "Parent": fr,
		}
		wref, ierr := ctx.XRefTable.IndRefForNewObject(w)
		if ierr != nil {
			return ierr
		}
		fd["Kids"] = types.Array{*wref}
		pd := derefDict(ctx.XRefTable, *pageRef)
		pd["Annots"] = append(derefArray(ctx.XRefTable, pd["Annots"]), *wref)
		return nil
	})
	if err != nil {
		t.Fatalf("build split-signature fixture: %v", err)
	}
	if !sign.HasSignatureBlob(split) {
		t.Fatal("setup: the fixture carries no signature blob, so its absence below proves nothing")
	}
	ctx := readCtx(t, split)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	form := derefDict(ctx.XRefTable, root["AcroForm"])
	fields := derefArray(ctx.XRefTable, form["Fields"])
	if len(fields) == 0 {
		t.Fatal("setup: the fixture lost its signature field")
	}
	fd := derefDict(ctx.XRefTable, fields[0])
	if n := len(derefArray(ctx.XRefTable, fd["Kids"])); n != 1 {
		t.Fatalf("setup: the signature field has %d kid(s), want 1 — the split shape is the point", n)
	}

	out, err := Collect(split, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if sign.HasSignatureBlob(out) {
		t.Error("a signature whose widget is a separate /Kids entry survived the subset")
	}
	after := readCtx(t, out)
	aroot, err := after.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	page1 := derefDict(after.XRefTable, mustPageRef(t, after, 1))
	for _, a := range derefArray(after.XRefTable, page1["Annots"]) {
		if ad := derefDict(after.XRefTable, a); ad != nil && nameVal(ad, "Subtype") == "Widget" {
			t.Error("the signature's widget is still on the kept page after its field was removed")
		}
	}
	if _, has := aroot["AcroForm"]; has {
		if f := derefDict(after.XRefTable, aroot["AcroForm"]); f != nil {
			if _, sf := f["SigFlags"]; sf {
				t.Error("/SigFlags survived a document with no signature")
			}
		}
	}
}
