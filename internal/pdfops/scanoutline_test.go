package pdfops

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// craftOutlinePDF builds a one-page PDF with a document outline. Each item of items is one TOP-LEVEL bookmark's
// `/A`; nested, when not nil, is the `/A` of a child bookmark under the second top-level item — a bookmark the
// tree reaches through `/First` rather than `/Next`. `page` in an action is replaced by a reference to page 1.
func craftOutlinePDF(t *testing.T, items []types.Dict, nested types.Dict) []byte {
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
	_, pageRef, _, err := xt.PageDict(1, false)
	if err != nil || pageRef == nil {
		t.Fatalf("page 1: %v", err)
	}
	var point func(d types.Dict)
	point = func(d types.Dict) {
		for k, v := range d {
			switch v := v.(type) {
			case types.Name:
				if v == "page" {
					d[k] = types.Array{*pageRef, types.Name("Fit")}
				}
			case types.Dict:
				point(v)
			case types.Array:
				for _, e := range v {
					if ed, ok := e.(types.Dict); ok {
						point(ed)
					}
				}
			}
		}
	}
	newObj := func(d types.Dict) types.IndirectRef {
		ir, err := xt.IndRefForNewObject(d)
		if err != nil {
			t.Fatal(err)
		}
		return *ir
	}
	outlines := types.Dict{"Type": types.Name("Outlines"), "Count": types.Integer(len(items))}
	outRef := newObj(outlines)
	var dicts []types.Dict
	var refs []types.IndirectRef
	for i, act := range items {
		point(act)
		d := types.Dict{"Title": types.StringLiteral("bookmark " + string(rune('A'+i))), "Parent": outRef, "A": act}
		dicts = append(dicts, d)
		refs = append(refs, newObj(d))
	}
	for i := range dicts {
		if i > 0 {
			dicts[i]["Prev"] = refs[i-1]
		}
		if i+1 < len(dicts) {
			dicts[i]["Next"] = refs[i+1]
		}
	}
	outlines["First"], outlines["Last"] = refs[0], refs[len(refs)-1]
	if nested != nil {
		point(nested)
		kid := newObj(types.Dict{"Title": types.StringLiteral("child"), "Parent": refs[1], "A": nested})
		dicts[1]["First"], dicts[1]["Last"], dicts[1]["Count"] = kid, kid, types.Integer(1)
	}
	root["Outlines"] = outRef
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// outlineFacts reads the outline with a walk of its own — independent of Scan, of eachOutlineItem and of
// riskyActions, for annotFacts' reason — and returns each bookmark's title, every action type any bookmark's
// `/A` reaches through `/Next`, and how many `/GoTo` destinations resolve to a live page.
func outlineFacts(t *testing.T, pdf []byte) (titles, actions []string, landed int) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	_, pageRef, _, err := xt.PageDict(1, false)
	if err != nil || pageRef == nil {
		t.Fatalf("page 1 no longer resolves: %v", err)
	}
	var act func(o types.Object, depth int)
	act = func(o types.Object, depth int) {
		d, _ := xt.DereferenceDict(o)
		if d == nil || depth > 8 {
			return
		}
		if s := d.NameEntry("S"); s != nil {
			actions = append(actions, *s)
		}
		if dest, _ := xt.DereferenceArray(d["D"]); len(dest) > 0 {
			if ir, ok := dest[0].(types.IndirectRef); ok && ir.ObjectNumber == pageRef.ObjectNumber {
				if pd, _ := xt.DereferenceDict(ir); pd != nil && pd["MediaBox"] != nil {
					landed++
				}
			}
		}
		if arr, _ := xt.DereferenceArray(d["Next"]); arr != nil {
			for _, a := range arr {
				act(a, depth+1)
			}
			return
		}
		act(d["Next"], depth+1)
	}
	var item func(o types.Object, depth int)
	item = func(o types.Object, depth int) {
		for n := 0; o != nil && n < 100 && depth < 8; n++ {
			d, _ := xt.DereferenceDict(o)
			if d == nil {
				return
			}
			title, _ := xt.DereferenceText(d["Title"])
			titles = append(titles, title)
			act(d["A"], 0)
			item(d["First"], depth+1)
			o = d["Next"]
		}
	}
	if ol, _ := xt.DereferenceDict(root["Outlines"]); ol != nil {
		item(ol["First"], 0)
	}
	sort.Strings(titles)
	sort.Strings(actions)
	return titles, actions, landed
}

// TestABookmarksActionIsSeenAndStripped — `/pending 580`. An outline item's `/A` is an action like any
// annotation's (§12.3.3), and neither Scan nor StripActive walked the outline: a bookmark that runs JavaScript
// scanned clean, survived the strip, and the strip's own re-scan passed it.
//
// Four homes, because each is a separate road to an action: a top-level item, an item reached only through
// another's `/First`, a benign `/GoTo` head whose `/Next` launches a program, and a plain `/GoTo` bookmark —
// which is not active content and must come out working.
func TestABookmarksActionIsSeenAndStripped(t *testing.T) {
	pdf := craftOutlinePDF(t, []types.Dict{
		{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(1)")},
		{"S": types.Name("GoTo"), "D": types.Name("page"),
			"Next": types.Dict{"S": types.Name("Launch"), "F": types.StringLiteral("/bin/sh")}},
		{"S": types.Name("GoTo"), "D": types.Name("page")},
	}, types.Dict{"S": types.Name("URI"), "URI": types.StringLiteral("http://evil.example")})

	titles, before, _ := outlineFacts(t, pdf)
	for _, name := range []string{"JavaScript", "Launch", "URI", "GoTo"} {
		if !has(before, name) {
			t.Fatalf("setup: the crafted outline does not hold a %s action; found %v", name, before)
		}
	}
	if len(titles) != 4 {
		t.Fatalf("setup: %d bookmarks read, want 4 (%v)", len(titles), titles)
	}

	var details []string
	for _, f := range must(t, pdf).Findings {
		if f.Kind == "action" {
			details = append(details, f.Severity+": "+f.Detail)
		}
	}
	joined := strings.Join(details, "\n")
	for _, want := range []string{"high: Runs JavaScript", "high: Launches an external program or file", "medium: Opens a web URL"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the scan did not report a bookmark's action as %q; action findings:\n%s", want, joined)
		}
	}

	stripped, err := StripActive(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(stripped); err != nil {
		t.Fatalf("stripped PDF does not validate: %v", err)
	}
	gotTitles, after, landed := outlineFacts(t, stripped)
	for _, name := range []string{"JavaScript", "Launch", "URI"} {
		if has(after, name) {
			t.Errorf("a bookmark's %s action survives the strip; outline actions left: %v", name, after)
		}
	}
	if strings.Join(gotTitles, "|") != strings.Join(titles, "|") {
		t.Errorf("the strip changed the bookmarks themselves: %v, were %v", gotTitles, titles)
	}
	// The plain bookmark is not active content: it keeps its action, and the page it opens is still there —
	// which is also what says dropping the chained bookmark's `/A` did not take the page its head pointed at.
	if strings.Join(after, "|") != "GoTo" || landed != 1 {
		t.Errorf("the plain /GoTo bookmark did not come through intact: actions %v, %d destination(s) on a live page", after, landed)
	}
}

// A document whose bookmarks only navigate has nothing for the scan to say and nothing for the strip to take.
func TestBookmarksThatOnlyNavigateAreLeftAlone(t *testing.T) {
	pdf := craftOutlinePDF(t, []types.Dict{
		{"S": types.Name("GoTo"), "D": types.Name("page")},
		{"S": types.Name("GoTo"), "D": types.Name("page"), "Next": types.Dict{"S": types.Name("GoTo"), "D": types.Name("page")}},
	}, types.Dict{"S": types.Name("GoTo"), "D": types.Name("page")})
	for _, f := range must(t, pdf).Findings {
		if f.Kind == "action" {
			t.Errorf("a navigating bookmark was reported as active content: %+v", f)
		}
	}
	titles, before, landedBefore := outlineFacts(t, pdf)
	stripped, err := StripActive(pdf)
	if err != nil {
		t.Fatal(err)
	}
	gotTitles, after, landed := outlineFacts(t, stripped)
	if strings.Join(gotTitles, "|") != strings.Join(titles, "|") || strings.Join(after, "|") != strings.Join(before, "|") || landed != landedBefore {
		t.Errorf("the strip changed an outline that only navigates: titles %v→%v, actions %v→%v, destinations %d→%d",
			titles, gotTitles, before, after, landedBefore, landed)
	}
	if landedBefore != 4 {
		t.Fatalf("setup: %d destinations resolve, want 4", landedBefore)
	}
}

// The same chain shape on an ANNOTATION: a `/GoTo` head with a destination, chained to JavaScript. Dropping `/A`
// frees the graph beneath it (dropKey), and the head's `/D` names the page.
func TestStrippingAChainedActionKeepsThePageItsHeadPointedAt(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	pdf := mustMutate(t, base, func(ctx *model.Context) error {
		pd, ref, _, err := ctx.XRefTable.PageDict(1, false)
		if err != nil {
			return err
		}
		pd["Annots"] = types.Array{types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("Link"), "Rect": types.Array{types.Integer(0), types.Integer(0), types.Integer(9), types.Integer(9)},
			"A": types.Dict{"S": types.Name("GoTo"), "D": types.Array{*ref, types.Name("Fit")},
				"Next": types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(3)")}},
		}}
		return nil
	})
	if _, acts := annotFacts(t, pdf); !has(acts, "JavaScript") {
		t.Fatalf("setup: the chained JavaScript action is not in the fixture; found %v", acts)
	}
	stripped, err := StripActive(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if _, acts := annotFacts(t, stripped); has(acts, "JavaScript") {
		t.Errorf("the chained JavaScript action survives: %v", acts)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(stripped), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("the stripped document does not read: %v", err)
	}
	pd, _, _, err := ctx.XRefTable.PageDict(1, false)
	if err != nil || pd == nil || pd["MediaBox"] == nil || pd["Contents"] == nil {
		t.Errorf("page 1 did not survive the strip of an action whose head named it: dict %v, err %v", pd, err)
	}
}

// An outline that loops, and names one item as sibling and child both, is walked once per item: the scan ends,
// and says each bookmark's action once. /Prev is not followed — an item only a /Prev names is on no reader's list.
func TestALoopingOutlineIsWalkedOncePerItem(t *testing.T) {
	pdf := testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Outlines 10 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		10: "<< /Type /Outlines /First 11 0 R /Last 12 0 R >>",
		11: "<< /Title (a) /Parent 10 0 R /Next 12 0 R /First 12 0 R /Prev 13 0 R /A << /S /JavaScript /JS (1) >> >>",
		12: "<< /Title (b) /Parent 10 0 R /Next 11 0 R /First 10 0 R /A << /S /Launch /F (x) >> >>",
		13: "<< /Title (c) /Parent 10 0 R /Next 11 0 R /A << /S /SubmitForm >> >>",
	})
	var got []string
	for _, f := range must(t, pdf).Findings {
		if f.Kind == "action" {
			got = append(got, f.Detail)
		}
	}
	sort.Strings(got)
	want := "Launches an external program or file (from a bookmark)|Runs JavaScript (from a bookmark)"
	if strings.Join(got, "|") != want {
		t.Errorf("a looping outline's actions were reported as %q, want each of the two reachable bookmarks once: %q", got, want)
	}
}
