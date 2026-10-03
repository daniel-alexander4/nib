package pdfops

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// mediaDoc is one page carrying `media` (an annotation dict body) beside a plain Text note, which every removal must
// keep. indirect puts the media annotation behind a reference (object 4, the shape real files use); otherwise it sits
// in the /Annots array as a direct dict, the shape pdfcpu's own type removal could not see (`/pending 815`).
func mediaDoc(media string, indirect bool, extra map[int]string) []byte {
	slot := media
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		5: "<< /Type /Annot /Subtype /Text /Rect [20 20 30 30] /Contents (a note) >>",
	}
	if indirect {
		objs[4] = media
		slot = "4 0 R"
	}
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [" + slot + " 5 0 R] >>"
	for n, v := range extra {
		objs[n] = v
	}
	return testpdf.Assemble(objs)
}

// richMediaOnOpen is a RichMedia annotation with an embedded asset that activates when the page opens
// (`/RichMediaSettings /Activation /Condition /PO`, ISO 32000-1 Adobe Extension Level 3 §9.6) — it plays with no click.
const richMediaOnOpen = "<< /Type /Annot /Subtype /RichMedia /Rect [0 0 100 100]" +
	" /RichMediaSettings << /Type /RichMediaSettings /Activation << /Type /RichMediaActivation /Condition /PO >> >>" +
	" /RichMediaContent << /Type /RichMediaContent /Assets << /Names [(clip.swf) 6 0 R] >>" +
	" /Configurations [<< /Type /RichMediaConfiguration /Subtype /Flash /Instances [<< /Type /RichMediaInstance /Subtype /Flash /Asset 6 0 R >>] >>] >> >>"

// richMediaAsset is the asset's file specification, an embedded stream.
var richMediaAsset = map[int]string{
	6: "<< /Type /Filespec /F (clip.swf) /UF (clip.swf) /EF << /F 7 0 R >> >>",
	7: "<< /Type /EmbeddedFile /Length 4 >>\nstream\nFWS1\nendstream",
}

// threeDOnInstantiate is a 3D annotation whose 3D stream runs JavaScript when it is instantiated (§13.6.3, /OnInstantiate).
const threeDOnInstantiate = "<< /Type /Annot /Subtype /3D /Rect [0 0 100 100] /3DD 10 0 R" +
	" /3DA << /A /PO >> >>"

var threeDStream = map[int]string{
	10: "<< /Type /3D /Subtype /U3D /OnInstantiate 9 0 R /Length 3 >>\nstream\nU3D\nendstream",
	9:  "<< /Length 13 >>\nstream\napp.alert(1);\nendstream",
}

// mediaFinding returns Scan's finding for a media annotation, failing the test if Scan errors.
func mediaFinding(t *testing.T, pdf []byte) (Finding, bool) {
	t.Helper()
	for _, f := range must(t, pdf).Findings {
		if f.Kind == "media" {
			return f, true
		}
	}
	return Finding{}, false
}

// TestARichMediaAnnotationThatPlaysOnPageOpenIsReported — `/pending 815`: Scan's annotation loop named only
// FileAttachment, so a RichMedia annotation that starts by itself when the page opens scanned clean.
func TestARichMediaAnnotationThatPlaysOnPageOpenIsReported(t *testing.T) {
	for _, indirect := range []bool{true, false} {
		f, ok := mediaFinding(t, mediaDoc(richMediaOnOpen, indirect, richMediaAsset))
		if !ok {
			t.Fatalf("indirect=%v: a RichMedia annotation activated on page open scanned with no media finding — the scan reports the document clean while it plays on open", indirect)
		}
		if f.Severity != "high" || !strings.Contains(f.Detail, "Rich media") || f.Page != 1 {
			t.Errorf("indirect=%v: finding = %+v, want a high-severity rich media finding on page 1", indirect, f)
		}
	}
}

// TestA3DAnnotationThatRunsJavaScriptIsReported — `/pending 815`: StripActive removed 3D annotations but Scan never
// reported one, so the user was told nothing was there.
func TestA3DAnnotationThatRunsJavaScriptIsReported(t *testing.T) {
	for _, indirect := range []bool{true, false} {
		f, ok := mediaFinding(t, mediaDoc(threeDOnInstantiate, indirect, threeDStream))
		if !ok {
			t.Fatalf("indirect=%v: a 3D annotation whose /3DD runs /OnInstantiate JavaScript scanned with no media finding", indirect)
		}
		if f.Severity != "high" || !strings.Contains(f.Detail, "3D") {
			t.Errorf("indirect=%v: finding = %+v, want a high-severity 3D finding", indirect, f)
		}
	}
}

// TestBothTiersRemoveEveryMediaAnnotation — `/pending 815`: StripActive returned success over a RichMedia annotation
// it had not removed (pdfcpu's type table has no RichMedia), RemoveFilesAndMedia did the same, and neither removed a
// media annotation written as a DIRECT dict. The subtype list here is written out, not read from mediaAnnots, so a
// subtype dropped from the table fails rather than silently leaving the loop.
func TestBothTiersRemoveEveryMediaAnnotation(t *testing.T) {
	bodies := map[string]string{
		"RichMedia":      richMediaOnOpen,
		"3D":             threeDOnInstantiate,
		"Screen":         "<< /Type /Annot /Subtype /Screen /Rect [0 0 10 10] >>",
		"Sound":          "<< /Type /Annot /Subtype /Sound /Rect [0 0 10 10] /Sound 8 0 R >>",
		"Movie":          "<< /Type /Annot /Subtype /Movie /Rect [0 0 10 10] /Movie << /F (clip.avi) >> >>",
		"FileAttachment": "<< /Type /Annot /Subtype /FileAttachment /Rect [0 0 10 10] /FS 6 0 R >>",
	}
	extra := map[int]string{
		6: richMediaAsset[6], 7: richMediaAsset[7],
		8: "<< /Type /Sound /R 8000 /Length 2 >>\nstream\n\x00\x00\nendstream",
	}
	for n, v := range threeDStream {
		extra[n] = v
	}

	tiers := []struct {
		name string
		run  func([]byte) ([]byte, error)
	}{{"StripActive", StripActive}, {"RemoveFilesAndMedia", RemoveFilesAndMedia}}
	for subtype, body := range bodies {
		for _, indirect := range []bool{true, false} {
			pdf := mediaDoc(body, indirect, extra)
			if subs, _ := annotFacts(t, pdf); !has(subs, subtype) {
				t.Fatalf("setup: %s indirect=%v carries %v", subtype, indirect, subs)
			}
			for _, tier := range tiers {
				out, err := tier.run(pdf)
				if err != nil {
					t.Fatalf("%s on %s indirect=%v: %v", tier.name, subtype, indirect, err)
				}
				subs, _ := annotFacts(t, out)
				if has(subs, subtype) {
					t.Errorf("%s left a %s annotation (indirect=%v) on the page: %v", tier.name, subtype, indirect, subs)
				}
				if !has(subs, "Text") {
					t.Errorf("%s on %s indirect=%v took the plain Text note too: %v", tier.name, subtype, indirect, subs)
				}
				if f, ok := mediaFinding(t, out); ok {
					t.Errorf("%s on %s indirect=%v: its output still scans with %+v", tier.name, subtype, indirect, f)
				}
			}
		}
	}
}

// TestStripActiveRefusesWhenAMediaAnnotationRemains — the verifier sees what the scan sees: a report carrying a media
// finding is residue, so a removal that failed is a refusal rather than "all active content neutralized".
func TestStripActiveRefusesWhenAMediaAnnotationRemains(t *testing.T) {
	rep := must(t, mediaDoc(richMediaOnOpen, true, richMediaAsset))
	if err := activeResidue(rep); err == nil {
		t.Error("a report holding a RichMedia finding is not residue to StripActive's verifier — a failed removal would be reported as success")
	}
}

// sharedMediaDoc is two pages naming ONE /Annots array (object 12) by reference: a RichMedia annotation and a Text note.
func sharedMediaDoc() []byte {
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 11 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots 12 0 R >>",
		11: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots 12 0 R >>",
		12: "[4 0 R 5 0 R]",
		4:  richMediaOnOpen,
		5:  "<< /Type /Annot /Subtype /Text /Rect [20 20 30 30] /Contents (a note) >>",
	}
	for n, v := range richMediaAsset {
		objs[n] = v
	}
	return testpdf.Assemble(objs)
}

// TestAMediaAnnotationInASharedAnnotsArrayIsRemovedFromEveryPage — two pages naming one /Annots array are both cleaned,
// through the tiers end to end. (pdfcpu's validation inlines the array into each page, so this does not reach the
// branch that rewrites a referenced array; TestTheMediaDoorRewritesAnAnnotsArrayNamedByReference does.)
func TestAMediaAnnotationInASharedAnnotsArrayIsRemovedFromEveryPage(t *testing.T) {
	pdf := sharedMediaDoc()
	if subs, _ := annotFacts(t, pdf); !has(subs, "RichMedia") {
		t.Fatalf("setup: the shared array carries %v", subs)
	}
	for _, tier := range []struct {
		name string
		run  func([]byte) ([]byte, error)
	}{{"StripActive", StripActive}, {"RemoveFilesAndMedia", RemoveFilesAndMedia}} {
		out, err := tier.run(pdf)
		if err != nil {
			t.Fatalf("%s: %v", tier.name, err)
		}
		subs, _ := annotFacts(t, out)
		if has(subs, "RichMedia") {
			t.Errorf("%s left the RichMedia annotation in the /Annots array the pages name by reference: %v", tier.name, subs)
		}
		if n := strings.Count(strings.Join(subs, " "), "Text"); n != 2 {
			t.Errorf("%s: %d Text notes across the two pages, want 2 (%v)", tier.name, n, subs)
		}
	}
}

// TestTheMediaDoorRewritesAnAnnotsArrayNamedByReference drives removeMediaAnnots on an UNVALIDATED read, where the
// pages still hold `12 0 R`: a removal that filtered a copy of the referenced array and dropped it would pass every
// tier-level test above, because validation had already inlined the array.
func TestTheMediaDoorRewritesAnAnnotsArrayNamedByReference(t *testing.T) {
	ctx, err := api.ReadContext(bytes.NewReader(sharedMediaDoc()), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if err := removeMediaAnnots(xt, root); err != nil {
		t.Fatal(err)
	}
	if err := eachPage(xt, root, func(page types.Dict, nr int) {
		if _, ok := page["Annots"].(types.IndirectRef); !ok {
			t.Errorf("page %d: /Annots is %T, want the reference it named — the setup no longer reaches the branch", nr, page["Annots"])
		}
		var subs []string
		for _, a := range derefArray(xt, page["Annots"]) {
			subs = append(subs, nameVal(derefDict(xt, a), "Subtype"))
		}
		if has(subs, "RichMedia") || !has(subs, "Text") {
			t.Errorf("page %d: the referenced /Annots array holds %v after the removal, want only the Text note", nr, subs)
		}
	}); err != nil {
		t.Fatal(err)
	}
}
