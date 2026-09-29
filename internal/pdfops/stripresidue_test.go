package pdfops

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// danglingRef names an object the file does not have. One inside an action's graph is enough to make
// pdfcpu's DeleteDictEntry fail — and it fails BEFORE deleting the key (`/pending 729`).
var danglingRef = types.IndirectRef{ObjectNumber: 99}

// danglingJS is `<< /S /JavaScript /JS (app.alert(1)) /Foo 99 0 R >>` — a live action that validates.
func danglingJS() types.Dict {
	return types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(1)"), "Foo": danglingRef}
}

// residueSite is one place a scrub removes a key: how to plant a dangling-graph value there, and how to
// find the dict holding the key again in a read context.
type residueSite struct {
	name  string
	key   string
	plant func(t *testing.T, xt *model.XRefTable, root types.Dict)
	find  func(t *testing.T, xt *model.XRefTable, root types.Dict) types.Dict
}

func residuePage(t *testing.T, xt *model.XRefTable) types.Dict {
	t.Helper()
	pd, _, _, err := xt.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	return pd
}

var catalogOpenAction = residueSite{
	name: "catalog /OpenAction", key: "OpenAction",
	plant: func(_ *testing.T, _ *model.XRefTable, root types.Dict) { root["OpenAction"] = danglingJS() },
	find:  func(_ *testing.T, _ *model.XRefTable, root types.Dict) types.Dict { return root },
}

var annotationAA = residueSite{
	name: "annotation /AA", key: "AA",
	plant: func(t *testing.T, xt *model.XRefTable, _ types.Dict) {
		residuePage(t, xt)["Annots"] = types.Array{types.Dict{
			"Type": types.Name("Annot"), "Subtype": types.Name("Link"),
			"Rect": types.Array{types.Integer(0), types.Integer(0), types.Integer(10), types.Integer(10)},
			"AA":   types.Dict{"E": danglingJS()},
		}}
	},
	find: func(t *testing.T, xt *model.XRefTable, _ types.Dict) types.Dict {
		for _, a := range derefArray(xt, residuePage(t, xt)["Annots"]) {
			if d := derefDict(xt, a); d != nil {
				return d
			}
		}
		t.Fatal("setup: the page has no annotation")
		return nil
	},
}

var pageAA = residueSite{
	name: "page /AA", key: "AA",
	plant: func(t *testing.T, xt *model.XRefTable, _ types.Dict) {
		residuePage(t, xt)["AA"] = types.Dict{"O": danglingJS()}
	},
	find: func(t *testing.T, xt *model.XRefTable, _ types.Dict) types.Dict { return residuePage(t, xt) },
}

var namesJavaScript = residueSite{
	name: "/Names /JavaScript", key: "JavaScript",
	plant: func(_ *testing.T, _ *model.XRefTable, root types.Dict) {
		root["Names"] = types.Dict{"JavaScript": types.Dict{"Names": types.Array{types.StringLiteral("a"), danglingJS()}}}
	},
	find: func(_ *testing.T, xt *model.XRefTable, root types.Dict) types.Dict {
		return derefDict(xt, root["Names"])
	},
}

// plantResidue builds a one-page PDF with site's dangling-graph value planted.
func plantResidue(t *testing.T, site residueSite) []byte {
	t.Helper()
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	site.plant(t, ctx.XRefTable, root)
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// assertOldPathKeepsKey is every test's STIMULUS: the key is in the input, and pdfcpu's own
// DeleteDictEntry — the call every scrub used to make and discard — fails on it and leaves the key, read
// through the same door the rewrite reads through. Without both halves the fixture is not the defect.
func assertOldPathKeepsKey(t *testing.T, pdf []byte, site residueSite) {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: the fixture does not read: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	d := site.find(t, ctx.XRefTable, root)
	if _, ok := d.Find(site.key); !ok {
		t.Fatalf("setup: /%s is not in the input at %s", site.key, site.name)
	}
	if err := ctx.XRefTable.DeleteDictEntry(d, site.key); err == nil {
		t.Fatalf("setup: pdfcpu's DeleteDictEntry succeeded at %s — the fixture no longer reaches the defect", site.name)
	}
	if _, ok := d.Find(site.key); !ok {
		t.Fatalf("setup: pdfcpu's DeleteDictEntry removed /%s despite failing — the old path was not broken here", site.key)
	}
}

// keyIn reports whether site's key survives in pdf, read independently of the scrub.
func keyIn(t *testing.T, pdf []byte, site residueSite) bool {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	d := site.find(t, ctx.XRefTable, root)
	if d == nil {
		return false
	}
	_, ok := d.Find(site.key)
	return ok
}

// TestStripActiveRemovesAKeyWhoseObjectGraphWillNotDelete (`/pending 729`).
//
// Every delete in StripActive was `_ = xt.DeleteDictEntry(...)`, and pdfcpu deletes the key only after
// freeing the graph beneath it. One dangling reference in an action made the free fail, the key stayed,
// StripActive returned success, and `nib sanitize` wrote a file whose OpenAction still ran.
func TestStripActiveRemovesAKeyWhoseObjectGraphWillNotDelete(t *testing.T) {
	for _, site := range []residueSite{catalogOpenAction, annotationAA, pageAA, namesJavaScript} {
		t.Run(site.name, func(t *testing.T) {
			pdf := plantResidue(t, site)
			assertOldPathKeepsKey(t, pdf, site)
			rep, err := Scan(pdf)
			if err != nil {
				t.Fatal(err)
			}
			if activeResidue(rep) == nil {
				t.Fatalf("setup: Scan reports nothing active in the input: %+v", rep.Findings)
			}

			out, err := StripActive(pdf)
			if err != nil {
				t.Fatalf("StripActive refused a document it should be able to clean: %v", err)
			}
			if keyIn(t, out, site) {
				t.Errorf("/%s survives StripActive at %s", site.key, site.name)
			}
			rep2, err := Scan(out)
			if err != nil {
				t.Fatal(err)
			}
			if err := activeResidue(rep2); err != nil {
				t.Errorf("the stripped output still scans active: %v", err)
			}
		})
	}
}

// TestStripMetadataRemovesAKeyWhoseObjectGraphWillNotDelete — the same defect at StripMetadata's two
// deletes, where the residue is identity rather than execution.
func TestStripMetadataRemovesAKeyWhoseObjectGraphWillNotDelete(t *testing.T) {
	site := residueSite{
		name: "catalog /Metadata", key: "Metadata",
		plant: func(t *testing.T, xt *model.XRefTable, root types.Dict) {
			sd, err := xt.NewStreamDictForBuf([]byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"/>`))
			if err != nil {
				t.Fatal(err)
			}
			sd.Dict["Type"] = types.Name("Metadata")
			sd.Dict["Subtype"] = types.Name("XML")
			sd.Dict["Foo"] = danglingRef
			if err := sd.Encode(); err != nil {
				t.Fatal(err)
			}
			ref, err := xt.IndRefForNewObject(*sd)
			if err != nil {
				t.Fatal(err)
			}
			root["Metadata"] = *ref
		},
		find: func(_ *testing.T, _ *model.XRefTable, root types.Dict) types.Dict { return root },
	}
	pdf := plantResidue(t, site)
	assertOldPathKeepsKey(t, pdf, site)

	out, err := StripMetadata(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if keyIn(t, out, site) {
		t.Error("/Metadata survives StripMetadata, which reported success")
	}
}

// TestStripActiveRefusesARemainingFinding drives the verifier's judgment directly: the door fix makes a
// residue unreachable through StripActive today, which is exactly why the check that catches the NEXT
// mechanism has to be proved on its own.
func TestStripActiveRefusesARemainingFinding(t *testing.T) {
	exempt := ScanReport{Findings: []Finding{
		{Kind: "metadata", Severity: "low", Detail: "XMP metadata stream"},
		{Kind: "info", Severity: "low", Detail: "Author: x"},
	}}
	if err := activeResidue(exempt); err != nil {
		t.Errorf("metadata and info are StripMetadata's remit, not a residue: %v", err)
	}
	left := ScanReport{Findings: append(exempt.Findings,
		Finding{Kind: "openAction", Severity: "high", Detail: "Runs an action automatically when the document opens"})}
	if err := activeResidue(left); !errors.Is(err, ErrActiveContentRemains) {
		t.Errorf("an OpenAction left in the output passed verification: err=%v", err)
	}
	// And the whole path: a verified input that is not a PDF is refused, not waved through.
	if err := verifyStripped([]byte("not a pdf")); !errors.Is(err, ErrActiveContentRemains) {
		t.Errorf("an output that cannot be re-scanned passed verification: err=%v", err)
	}
}

// TestStripActiveRemovesANonEmptyJavaScriptNameTree — found by the verifier the first time it ran
// (`/pending 729`), and independent of any dangling reference: pdfcpu parses a non-empty `/Names
// /JavaScript` tree into `xt.Names` and its writer binds that copy back into `/Names`, so deleting the key
// restored it on write. craftActivePDF's tree is EMPTY, which pdfcpu never parses, so the existing strip
// test could not see it.
func TestStripActiveRemovesANonEmptyJavaScriptNameTree(t *testing.T) {
	site := namesJavaScript
	site.plant = func(_ *testing.T, _ *model.XRefTable, root types.Dict) {
		root["Names"] = types.Dict{"JavaScript": types.Dict{"Names": types.Array{
			types.StringLiteral("a"),
			types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(1)")},
		}}}
	}
	pdf := plantResidue(t, site)
	// STIMULUS: the tree is non-empty and pdfcpu holds a parsed copy of it — the copy its writer rebinds.
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if ctx.XRefTable.Names["JavaScript"] == nil {
		t.Fatal("setup: pdfcpu holds no parsed JavaScript name tree, so there is nothing for its writer to rebind")
	}

	out, err := StripActive(pdf)
	if err != nil {
		t.Fatalf("StripActive refused: %v", err)
	}
	if keyIn(t, out, site) {
		t.Error("/Names /JavaScript survives StripActive — rebound by pdfcpu's writer from its parsed copy")
	}
}
