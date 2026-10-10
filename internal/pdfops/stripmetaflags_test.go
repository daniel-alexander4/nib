package pdfops

import (
	"bytes"
	"encoding/base64"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// infoFacts is the Info dictionary's keys, sorted, and every string value in it as text — the NibFlags blob
// decoded, so text hidden inside it is seen.
func infoFacts(t *testing.T, pdf []byte) (keys []string, text string) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ctx.XRefTable.Info == nil {
		return nil, ""
	}
	info := derefDict(ctx.XRefTable, *ctx.XRefTable.Info)
	var all []string
	for k, v := range info {
		keys = append(keys, k)
		s, _ := stringVal(ctx.XRefTable, v)
		if k == flagsKey {
			raw, _ := base64.StdEncoding.DecodeString(s)
			s = string(raw)
		}
		all = append(all, s)
	}
	sort.Strings(keys)
	return keys, strings.Join(all, "\n")
}

// TestStrippingMetadataKeepsTheSigningFlagsAndNothingElse — `/pending 830`. StripMetadata dropped the Info
// dictionary whole, and a flagged document's "sign here" placeholders live there: a recipient who removed the
// metadata before signing lost every one, with nothing said. The flags are kept — rebuilt from the three fields
// nib reads, so nothing a sender tucked into the blob survives a removal that claims nothing identifying is left.
func TestStrippingMetadataKeepsTheSigningFlagsAndNothingElse(t *testing.T) {
	src := craftMetadataPDF(t)
	const placed = `[{"page":1,"frac":[0.1,0.25,0.3,0.05],"type":"sign"},` +
		`{"page":1,"frac":[0.5,0.5,0.2,0.05],"type":"date","note":"Prepared by Ada Lovelace"},` +
		`{"page":1,"frac":[0.5,0.7,0.2,0.05],"type":"Prepared by Ada Lovelace"},` +
		`{"page":1,"frac":[0.5,0.7],"type":"name"},` +
		`"Prepared by Ada Lovelace"]`
	flagged, err := SetFlags(src, []byte(placed))
	if err != nil {
		t.Fatal(err)
	}
	if keys, text := infoFacts(t, flagged); !has(keys, "Author") || !has(keys, flagsKey) || !strings.Contains(text, "Ada Lovelace") {
		t.Fatalf("setup: the flagged document should name an author and carry flags; Info keys %v", keys)
	}

	out, err := StripMetadata(flagged)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(out); err != nil {
		t.Fatalf("the result does not validate: %v", err)
	}
	got, err := FlagsJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	const want = `[{"page":1,"frac":[0.1,0.25,0.3,0.05],"type":"sign"},{"page":1,"frac":[0.5,0.5,0.2,0.05],"type":"date"}]`
	if string(got) != want {
		t.Errorf("the flags after a metadata strip are\n  %s\nwant the two well-formed flags, each with page, frac and type only:\n  %s", got, want)
	}
	keys, text := infoFacts(t, out)
	for _, k := range keys {
		if k != flagsKey && k != "Producer" && k != "CreationDate" && k != "ModDate" {
			t.Errorf("the Info dictionary still holds /%s after a metadata strip (keys %v)", k, keys)
		}
	}
	if strings.Contains(text, "Ada") {
		t.Errorf("text a sender put inside the flags survived the metadata strip: %q", text)
	}
	rep := kinds(must(t, out))
	if rep["info"] || rep["metadata"] {
		t.Errorf("the strip left identifying metadata; findings %+v", must(t, out).Findings)
	}
	if before, after := docID0(t, flagged), docID0(t, out); before == "" || before == after {
		t.Errorf("/ID not regenerated: before=%q after=%q", before, after)
	}
}

// A document with no flags, or with a NibFlags value that is not flags, comes out with no such property — as
// every document did before.
func TestStrippingMetadataInventsNoFlags(t *testing.T) {
	src := craftMetadataPDF(t)
	notFlags, err := rewriteWithConf(src, propertyConf(model.ADDPROPERTIES), func(ctx *model.Context) error {
		return pdfcpu.PropertiesAdd(ctx, map[string]string{flagsKey: base64.StdEncoding.EncodeToString([]byte(`{"author":"Ada Lovelace"}`))})
	})
	if err != nil {
		t.Fatal(err)
	}
	if keys, _ := infoFacts(t, notFlags); !has(keys, flagsKey) {
		t.Fatalf("setup: the property was not written; Info keys %v", keys)
	}
	for name, pdf := range map[string][]byte{"no flags": src, "a blob that is not flags": notFlags} {
		out, err := StripMetadata(pdf)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if keys, text := infoFacts(t, out); has(keys, flagsKey) || strings.Contains(text, "Ada") {
			t.Errorf("%s: the Info dictionary after the strip is %v", name, keys)
		}
	}
}
