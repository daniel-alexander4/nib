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

// `/pending 688` — nothing the redacted page ALONE drew survives in the redacted file.
//
// A subset materializes inherited `/Resources` onto every kept page (`selectPages`), and a producer
// that shares one resource dictionary across pages hands every page the same thing without any
// inheritance at all. Either way the kept page's dictionary NAMED the form and the image only the
// redacted page drew, and pdfcpu writes by reachability — so the raster replaced the page and the
// file still carried its drawing. Measured by the P07 phase-close review (R5-1), and reproduced here
// first.
//
// **The search runs over DECODED bytes.** Every stream in the output is decoded through pdfcpu and
// searched alongside the raw file, because a compressed stream hides its text from a byte search —
// never byte-count a compressed PDF.

const (
	secretText  = "SECRETPAGEONE"
	secretImage = "SECRETIMAGEBYTES"
	publicText  = "publicpagetwo"
)

// everyDecodedByte is the raw file followed by the decoded body of every stream object in it.
func everyDecodedByte(t *testing.T, pdf []byte) []byte {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("the output does not read: %v", err)
	}
	var all bytes.Buffer
	all.Write(pdf)
	streams := 0
	for nr, e := range ctx.XRefTable.Table {
		if e == nil || e.Free || e.Object == nil {
			continue
		}
		sd, _, derr := ctx.XRefTable.DereferenceStreamDict(*types.NewIndirectRef(nr, 0))
		if derr != nil || sd == nil {
			continue
		}
		streams++
		if len(sd.FilterPipeline) == 0 {
			all.Write(sd.Raw)
			continue
		}
		if sd.Decode() != nil {
			t.Fatalf("object %d is a stream this search cannot decode, so its absence proves nothing", nr)
		}
		all.Write(sd.Content)
	}
	if streams == 0 {
		t.Fatal("setup: the output holds no stream at all, so the search below reads nothing")
	}
	return all.Bytes()
}

// sharedResourceDoc is two pages. Page 1 draws a form (whose text is secretText) and an image (whose
// samples are secretImage); page 2 draws its own text and nothing of page 1's. Where the resource
// dictionary lives is the parameter: `inherited` puts it on the /Pages node, otherwise it is one
// indirect object both pages name.
func sharedResourceDoc(inherited bool) []byte {
	form := fmt.Sprintf("BT /F1 12 Tf 72 700 Td (%s) Tj ET", secretText)
	res := "<< /XObject << /Fm1 6 0 R /Im1 9 0 R >> /Font << /F1 7 0 R >> /ProcSet [/PDF /Text] >>"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		5: stream("q /Fm1 Do Q q 16 0 0 1 72 72 cm /Im1 Do Q"),
		6: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
		7: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		8: stream(fmt.Sprintf("BT /F1 12 Tf 72 700 Td (%s) Tj ET", publicText)),
		9: fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 16 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>\nstream\n%s\nendstream", len(secretImage), secretImage),
	}
	if inherited {
		objs[2] = "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] /Resources " + res + " >>"
		objs[3] = "<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>"
		objs[4] = "<< /Type /Page /Parent 2 0 R /Contents 8 0 R >>"
	} else {
		objs[2] = "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>"
		objs[3] = "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Resources 10 0 R >>"
		objs[4] = "<< /Type /Page /Parent 2 0 R /Contents 8 0 R /Resources 10 0 R >>"
		objs[10] = res
	}
	return assembleFixture(objs)
}

func stream(s string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(s), s)
}

// TestRedactionLeavesNothingOnlyTheRedactedPageDrew — the item's property, over both shapes of
// sharing, asserted on the stimulus first: the source DOES carry both secrets, so their absence
// afterwards is the redaction's doing.
func TestRedactionLeavesNothingOnlyTheRedactedPageDrew(t *testing.T) {
	for _, inherited := range []bool{true, false} {
		name := map[bool]string{true: "inherited from /Pages", false: "one shared indirect dictionary"}[inherited]
		t.Run(name, func(t *testing.T) {
			src := sharedResourceDoc(inherited)
			before := everyDecodedByte(t, src)
			if !bytes.Contains(before, []byte(secretText)) || !bytes.Contains(before, []byte(secretImage)) {
				t.Fatal("setup: the source does not carry both secrets, so their absence proves nothing")
			}
			out, err := RedactPages(src, map[int]RasterPage{1: rasterPage(t, 612, 792)})
			if err != nil {
				t.Fatal(err)
			}
			after := everyDecodedByte(t, out)
			if bytes.Contains(after, []byte(secretText)) {
				t.Errorf("the redacted file still carries the form only page 1 drew (%q) — the kept page's "+
					"resources named it, and pdfcpu writes by reachability", secretText)
			}
			if bytes.Contains(after, []byte(secretImage)) {
				t.Errorf("the redacted file still carries the image only page 1 drew (%q)", secretImage)
			}
			if !bytes.Contains(after, []byte(publicText)) {
				t.Error("the kept page's own text is gone — the prune removed what the page draws")
			}
			if n, _ := PageCount(out); n != 2 {
				t.Errorf("the redacted file has %d pages, want 2", n)
			}
		})
	}
}

// TestRemovingAPageLeavesNothingOnlyItDrew — the same primitive serves every subset door, so the
// same property holds of "delete page 1": `pageselect.go`'s header already calls a removed page's
// content surviving a leak, and it is the same leak one door over.
func TestRemovingAPageLeavesNothingOnlyItDrew(t *testing.T) {
	out, err := RemovePages(sharedResourceDoc(true), []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	after := everyDecodedByte(t, out)
	if bytes.Contains(after, []byte(secretText)) || bytes.Contains(after, []byte(secretImage)) {
		t.Error("removing page 1 left the drawing only page 1 made in the file")
	}
	if !bytes.Contains(after, []byte(publicText)) {
		t.Error("the kept page's own text is gone")
	}
}

// keptResources reads the single page of a one-page subset and returns each resource category's
// names.
func keptResources(t *testing.T, pdf []byte) map[string][]string {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	leaves, _, err := collectLeaves(ctx.XRefTable, root)
	if err != nil || len(leaves) != 1 {
		t.Fatalf("want one page, got %d (%v)", len(leaves), err)
	}
	pd := leaves[0].dic
	out := map[string][]string{}
	res := derefDict(ctx.XRefTable, pd["Resources"])
	for cat, v := range res {
		sub := derefDict(ctx.XRefTable, v)
		for n := range sub {
			out[cat] = append(out[cat], n)
		}
		if sub == nil {
			out[cat] = append(out[cat], "<"+fmt.Sprint(v)+">")
		}
	}
	return out
}

// TestThePruneKeepsEverythingAKeptPageCanName — the other direction, which is what a user sees: a
// prune that removes one entry the page draws blanks part of a page nobody redacted. Each resource
// reached ONLY indirectly must survive:
//
//   - a font named by a form that has no /Resources of its own (it reads the page's), and one named
//     by an annotation's appearance stream with none;
//   - a colour space named by an INLINE IMAGE, which the tokenizer holds as one opaque token;
//   - a name spelled with a `#xx` escape, which pdfcpu decodes in the dictionary key;
//   - an ExtGState, a Shading and a Properties entry, each named by its own operator.
//
// And a key outside Table 33 is dropped even when it names something the page draws. **That clause is
// a backstop, not coverage:** pdfcpu's own read already drops an unknown resource key (measured — with
// the prune removed the key is gone too), so it cannot go red through a file today. It is kept for the
// day pdfcpu stops doing that.
func TestThePruneKeepsEverythingAKeptPageCanName(t *testing.T) {
	inner := "BT /F2 12 Tf 72 700 Td (form text) Tj ET"
	content := "/Fm2 Do /GS1 gs /Sh1 sh /OC /MC1 BDC EMC BT /F#31 12 Tf (x) Tj ET " +
		"BI /W 1 /H 1 /CS /CS1 /BPC 8 ID \x00 EI"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] /Resources << " +
			"/XObject << /Fm2 6 0 R /Unused 6 0 R >> /Font << /F1 7 0 R /F2 7 0 R /F3 7 0 R /F4 7 0 R >> " +
			"/ExtGState << /GS1 << /CA 1 >> /GS2 << /CA 0.5 >> >> /Shading << /Sh1 9 0 R >> " +
			"/Properties << /MC1 << /Type /OCG /Name (l) >> >> /ColorSpace << /CS1 /DeviceGray /CS2 /DeviceRGB >> " +
			"/ProcSet [/PDF /Text] /NotAResourceKey << /Fm2 6 0 R >> >> >>",
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Annots [10 0 R] >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 8 0 R >>",
		5:  stream(content),
		10: "<< /Type /Annot /Subtype /FreeText /Rect [0 0 50 20] /DA (/F4 9 Tf) /AP << /N 11 0 R >> >>",
		11: "<< /Type /XObject /Subtype /Form /BBox [0 0 50 20] /Length 21 >>\nstream\nBT /F4 9 Tf (a) Tj ET\nendstream",
		6:  fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Length %d >>\nstream\n%s\nendstream", len(inner), inner),
		7:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		8:  stream("0 0 1 1 re f"),
		9:  "<< /ShadingType 2 /ColorSpace /DeviceGray /Coords [0 0 1 0] /Function << /FunctionType 2 /Domain [0 1] /C0 [0] /C1 [1] /N 1 >> >>",
	}
	out, err := collectWithoutStructure(assembleFixture(objs), []string{"1"}, refuseUnreadable)
	if err != nil {
		t.Fatal(err)
	}
	got := keptResources(t, out)
	want := map[string][]string{
		"XObject":    {"Fm2"},
		"Font":       {"F1", "F2", "F4"},
		"ExtGState":  {"GS1"},
		"Shading":    {"Sh1"},
		"Properties": {"MC1"},
		"ColorSpace": {"CS1"},
	}
	for cat, names := range want {
		for _, n := range names {
			if !has(got[cat], n) {
				t.Errorf("/%s /%s is drawn by the kept page and was pruned (kept: %v)", cat, n, got[cat])
			}
		}
	}
	for cat, n := range map[string]string{"XObject": "Unused", "Font": "F3", "ExtGState": "GS2", "ColorSpace": "CS2"} {
		if has(got[cat], n) {
			t.Errorf("/%s /%s is named by nothing the kept page draws and survived the prune", cat, n)
		}
	}
	if _, kept := got["NotAResourceKey"]; kept {
		t.Error("a resource key outside ISO 32000-1 Table 33 survived — default-deny drops it, as the catalog does")
	}
	if len(got["ProcSet"]) == 0 {
		t.Error("/ProcSet names no resource and must be kept whole")
	}
}

// TestAFormsOwnSharedResourcesArePrunedToo — a form with /Resources of its own reads THOSE, and a
// producer that points them at the page's shared dictionary hands the form everything the redacted
// page drew. The form's resources are pruned to what the form draws, once, for every page using it.
func TestAFormsOwnSharedResourcesArePrunedToo(t *testing.T) {
	logo := "BT /F1 9 Tf (logo) Tj ET"
	secret := fmt.Sprintf("BT /F1 12 Tf (%s) Tj ET", secretText)
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>",
		3:  "<< /Type /Page /Parent 2 0 R /Contents 5 0 R /Resources 10 0 R >>",
		4:  "<< /Type /Page /Parent 2 0 R /Contents 8 0 R /Resources 10 0 R >>",
		5:  stream("/Logo Do /Fm1 Do"),
		6:  fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources 10 0 R /Length %d >>\nstream\n%s\nendstream", len(secret), secret),
		7:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		8:  stream("/Logo Do"),
		9:  fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources 10 0 R /Length %d >>\nstream\n%s\nendstream", len(logo), logo),
		10: "<< /XObject << /Fm1 6 0 R /Logo 9 0 R >> /Font << /F1 7 0 R >> >>",
	}
	out, err := RedactPages(assembleFixture(objs), map[int]RasterPage{1: rasterPage(t, 612, 792)})
	if err != nil {
		t.Fatal(err)
	}
	after := everyDecodedByte(t, out)
	if bytes.Contains(after, []byte(secretText)) {
		t.Error("the kept page's logo form names the shared dictionary, and through it the form only the " +
			"redacted page drew survived")
	}
	if !bytes.Contains(after, []byte("logo")) {
		t.Error("the logo the kept page draws is gone")
	}
}

// TestRedactionRefusesAPageItCannotRead — redaction FAILS CLOSED. A kept page whose content cannot be
// decoded cannot say what it names, so its resources cannot be pruned; for redaction that is a
// refusal, because keeping them keeps whatever the redacted page shared with it. The other subset
// doors keep the page as it was, which is what they did before the prune existed.
func TestRedactionRefusesAPageItCannotRead(t *testing.T) {
	src := sharedResourceDoc(true)
	broken := "xx not flate xx"
	src = bytes.Replace(src, []byte(fmt.Sprintf("<< /Length %d >>\nstream\nBT /F1 12 Tf 72 700 Td (%s) Tj ET\nendstream", len("BT /F1 12 Tf 72 700 Td ("+publicText+") Tj ET"), publicText)),
		[]byte(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", len(broken), broken)), 1)
	if !bytes.Contains(src, []byte(broken)) {
		t.Fatal("setup: the kept page's content was not replaced with an undecodable stream")
	}
	_, err := RedactPages(src, map[int]RasterPage{1: rasterPage(t, 612, 792)})
	if err == nil || !strings.Contains(err.Error(), "cannot tell") {
		t.Errorf("redaction over a kept page it cannot read returned %v, want a refusal", err)
	} else if !strings.Contains(err.Error(), "page 2 ") {
		t.Errorf("the refusal %q does not name page 2, the source page the user knows", err)
	}
	if _, rerr := RemovePages(src, []string{"1"}); rerr != nil {
		t.Errorf("removing a page from the same document failed (%v) — only redaction refuses", rerr)
	}
}
