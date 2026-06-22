package pdfops

import (
	"bytes"
	"fmt"
	"strings"
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
