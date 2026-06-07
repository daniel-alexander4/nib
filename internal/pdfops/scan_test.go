package pdfops

import (
	"bytes"
	"testing"

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
