package pdfops

import (
	"bytes"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// withXFA is base carrying an XFA packet in its `/AcroForm` and `/NeedsRendering` as given — written by pdfcpu's
// writer directly, because `pdfread.Write` now removes exactly this.
func withXFA(t *testing.T, base []byte, needsRendering bool) []byte {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	sd, err := ctx.NewStreamDictForBuf([]byte(`<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/"><template/></xdp:xdp>`))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := sd.Encode(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ref, err := ctx.IndRefForNewObject(*sd)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	form, _ := ctx.DereferenceDict(root["AcroForm"])
	if form == nil {
		form = types.Dict{}
		root["AcroForm"] = form
	}
	form["XFA"] = *ref
	root["NeedsRendering"] = types.Boolean(needsRendering)
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out.Bytes()
}

// TestNoOperationWritesAnXFAFormOrChangesAPlaceholder — ADR-145, over the tag-fate census's whole driven
// population, so an operation added tomorrow is asked the moment it gets a census row.
//
// Two questions of every operation that returns a document. Driven on a document with an XFA packet and
// `/NeedsRendering false`: what it returns carries neither. Driven on the same document with `/NeedsRendering
// true` — pages that are only a placeholder: it returns nothing but `pdfread.ErrDynamicXFA`, or the bytes it was
// given.
func TestNoOperationWritesAnXFAFormOrChangesAPlaceholder(t *testing.T) {
	fixtures := map[bool][2][]byte{} // twoPages -> {static, dynamic}
	for two, base := range map[bool][]byte{false: labelledFixture(t), true: labelled(t, taggedTwoPageFixture())} {
		static, dynamic := withXFA(t, base, false), withXFA(t, base, true)
		if got, err := pdfread.ReadXFA(static); err != nil || got != pdfread.XFAStatic {
			t.Fatalf("setup: the static fixture reads as %q, %v", got, err)
		}
		if got, err := pdfread.ReadXFA(dynamic); err != nil || got != pdfread.XFADynamic {
			t.Fatalf("setup: the dynamic fixture reads as %q, %v", got, err)
		}
		fixtures[two] = [2][]byte{static, dynamic}
	}
	names := make([]string, 0, len(tagFates))
	for n := range tagFates {
		names = append(names, n)
	}
	sort.Strings(names)
	driven, refused := 0, 0
	var kept, changedPlaceholder, otherwise []string
	for _, name := range names {
		f := tagFates[name]
		drive := f.uaDrive
		if drive == nil {
			drive = f.drive
		}
		if drive == nil {
			continue // `TestNoOperationCarriesAnIdentificationItDidNotVerify` holds every such row to a reason
		}
		static, dynamic := fixtures[f.twoPages][0], fixtures[f.twoPages][1]
		if out, err := drive(static); err == nil && !bytes.Equal(out, static) {
			driven++
			if got, rerr := pdfread.ReadXFA(out); rerr != nil || got != "" {
				kept = append(kept, name)
			}
		}
		out, err := drive(dynamic)
		switch {
		case errors.Is(err, pdfread.ErrDynamicXFA), err != nil && strings.Contains(err.Error(), pdfread.DynamicXFARefusal):
			refused++
		case err != nil:
			otherwise = append(otherwise, name+" ("+err.Error()+")")
		case !bytes.Equal(out, dynamic):
			changedPlaceholder = append(changedPlaceholder, name)
		}
	}
	if len(kept) > 0 {
		t.Errorf("%d operation(s) return a document that still carries an XFA form or /NeedsRendering: %s.\n"+
			"Write through pdfread.Write, which drops both together (ADR-145).", len(kept), strings.Join(kept, ", "))
	}
	if len(changedPlaceholder) > 0 {
		t.Errorf("%d operation(s) return a changed copy of a document whose pages are a placeholder for a dynamic "+
			"XFA form: %s.\nWrite the context that was read through pdfread.Write, or ask pdfread.RefuseDynamicXFA "+
			"before moving its pages into another (ADR-145).", len(changedPlaceholder), strings.Join(changedPlaceholder, ", "))
	}
	if driven < 20 || refused < 20 {
		t.Errorf("%d operation(s) were driven on the static fixture and %d refused the placeholder; the census is "+
			"reporting coverage it barely has", driven, refused)
	}
	t.Logf("driven %d, refused the placeholder %d; failed on it for another reason: %s", driven, refused, strings.Join(otherwise, "; "))
}

// TestBytesChangedOutsideThisPackageLoseTheirXFAForm — ADR-145: the door for a posted save and a document about
// to be signed drops the form in the write it makes for the claim, pays no write for a document with neither,
// leaves a signed document alone, and answers a placeholder with the one error its callers treat as a refusal.
func TestBytesChangedOutsideThisPackageLoseTheirXFAForm(t *testing.T) {
	plain := taggedFixture()
	static, dynamic := withXFA(t, plain, false), withXFA(t, plain, true)

	out, err := DropUAIdentificationUnlessSigned(static, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, rerr := pdfread.ReadXFA(out); rerr != nil || got != "" {
		t.Errorf("an unsigned document with an XFA form came back reading as %q, %v", got, rerr)
	}
	if same, err := DropUAIdentificationUnlessSigned(static, true); err != nil || !bytes.Equal(same, static) {
		t.Errorf("a signed document was rewritten (err %v) — its signature would not survive that", err)
	}
	if same, err := DropUAIdentificationUnlessSigned(plain, false); err != nil || !bytes.Equal(same, plain) {
		t.Errorf("a document with no claim and no XFA form was rewritten (err %v)", err)
	}
	if same, err := DropUAIdentificationUnlessSigned(dynamic, false); !errors.Is(err, pdfread.ErrDynamicXFA) || !bytes.Equal(same, dynamic) {
		t.Errorf("a placeholder came back with err %v (want ErrDynamicXFA) and changed=%v", err, !bytes.Equal(same, dynamic))
	}
	if same, err := DropUAIdentificationUnlessSigned(dynamic, true); err != nil || !bytes.Equal(same, dynamic) {
		t.Errorf("a signed placeholder is the declared gap and comes back as it was: err %v", err)
	}
	// The doors that move a placeholder's pages into another document ask before they move them: what `Write`
	// knows of the document does not follow its pages there.
	if _, _, err := InsertPDF(plain, dynamic, 1, false); !errors.Is(err, pdfread.ErrDynamicXFA) {
		t.Errorf("inserting a placeholder into another document: %v, want ErrDynamicXFA", err)
	}
	if _, _, err := SplitPage(dynamic, 1, 2, 1, false); !errors.Is(err, pdfread.ErrDynamicXFA) {
		t.Errorf("splitting a placeholder's page: %v, want ErrDynamicXFA", err)
	}
	if _, err := NUp(dynamic, 2, false); !errors.Is(err, pdfread.ErrDynamicXFA) {
		t.Errorf("imposing a placeholder: %v, want ErrDynamicXFA", err)
	}
	// What the server asks of every document it answers for, from the read it already makes.
	for _, tc := range []struct {
		pdf  []byte
		want string
	}{{plain, ""}, {static, pdfread.XFAStatic}, {dynamic, pdfread.XFADynamic}} {
		if _, got, err := FlagsAndXFA(tc.pdf); err != nil || got != tc.want {
			t.Errorf("FlagsAndXFA = %q, %v, want %q", got, err, tc.want)
		}
	}
	// Remove active content is what a convener is told takes a static form out.
	stripped, err := StripActive(static)
	if err != nil {
		t.Fatal(err)
	}
	if got, rerr := pdfread.ReadXFA(stripped); rerr != nil || got != "" {
		t.Errorf("after Remove active content the document reads as %q, %v", got, rerr)
	}
}
