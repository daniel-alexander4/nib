package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 514` — an unmarked PICTURE under a claim of tagging, which is `/pending 495`'s declared
// residue. 495 made a new claim refuse undescribed TEXT and left images, shadings and inline images
// out, because refusing on them would have switched the OCR door off. 514 measured what refusing
// actually buys and the answer was nothing, so the doors mark instead.
//
// # The two measurements that chose `/Artifact`, and where to re-run them
//
// Both are veraPDF's own PDF/UA-1 corpus (`~/nib/verapdfs/PDF_UA-1`, 297 files), read with
// `--flavour ua1 --passed`:
//
//   - **7.1 t3 is `isTaggedContent == true || parentsTags.contains('Artifact') == true`.** Either
//     route conforms, so the clause does not choose between them. Over the whole corpus 39 images sit
//     inside an `/Artifact` sequence and 16 inside a marked one; exactly ONE image in 297 files is
//     inside neither, and it is `7.1-t03-fail-a.pdf`, whose own bookmark reads *"Image is not marked
//     as Artifact or real content"*.
//   - **7.3 t1 is `(Alt != null && Alt != '') || ActualText != null`.** So a `/Figure` the structure
//     editor has not been given alt text for does not FIX the failure, it RENAMES it —
//     `7.3-t01-fail-a.pdf` is a Figure with neither key and veraPDF fails it, and `7.3-t01-fail-b.pdf`
//     shows an EMPTY `/Alt` failing too. (An empty `/ActualText` passes, `7.3-t01-pass-c.pdf`, and
//     writing one to buy a verdict is the claim ADR-032 forbids. nib writes neither.)
//
// And the third, on this door's own fixture: the BARE scan fails 7.1 t3 exactly as the tagged one
// did, so refusing the claim returns a document that fails the same clause with less text in it.

// TestAnOCRdScanDeclaresItsPageImageAnArtifact — the finding as reproduced, then closed.
//
// Before this item `TagOCRLayer` returned `tagged=true` for a scan whose image no element covered:
// veraPDF failed `7.1 t3` at `/Im0 Do` and so did `nib ua`, while `UnmarkedTextRuns` — the only thing
// the claim door could see — answered 0.
func TestAnOCRdScanDeclaresItsPageImageAnArtifact(t *testing.T) {
	scan := scannedPage(t)
	// **The stimulus is asserted in BYTES, not through `uncoveredDrawings`.** A setup check that reads
	// the page through the code under test is satisfied by that code going blind: the mutation that
	// makes the drawing reader stop seeing images would make this fixture report "no picture here" and
	// skip out, which is a setup passing because the defect is present.
	if bs := pageStream(t, scan, 1); !bytes.Contains(bs, []byte("/Im0 Do")) || bytes.Contains(bs, []byte("BMC")) {
		t.Fatalf("setup: the scan's page does not draw an image outside all marked content, so nothing "+
			"below is about a picture:\n%.400s", bs)
	}
	if n, err := UnmarkedTextRuns(scan); err != nil || n != 0 {
		t.Fatalf("setup: the scan has %d unmarked text run(s) (err %v); the point of this item is that "+
			"the TEXT guard sees nothing here", n, err)
	}

	out, tagged, err := TagOCRLayer(scan, ocrWords(), "eng")
	if err != nil {
		t.Fatal(err)
	}
	if !tagged {
		t.Fatal("TagOCRLayer fell back to the plain stamp, so the claim below is not the one under test")
	}
	if n, derr := uncoveredDrawings(out); derr != nil || n != 0 {
		t.Errorf("the tagged scan still draws %d thing(s) no sequence covers (err %v)", n, derr)
	}
	if !bytes.Contains(pageStream(t, out, 1), []byte("/Artifact BMC\n/Im0 Do\nEMC")) {
		t.Errorf("the page image is not bracketed as an artifact:\n%.600s", pageStream(t, out, 1))
	}

	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the scan's 7.1 t3 is UNCHECKED in this run. " +
			"Set NIB_VERAPDF, put verapdf on PATH, or install to ~/verapdf.")
	}
	dir := t.TempDir()
	var files []string
	for n, b := range map[string][]byte{"bare.pdf": scan, "tagged.pdf": out} {
		p := filepath.Join(dir, n)
		if werr := os.WriteFile(p, b, 0o600); werr != nil {
			t.Fatal(werr)
		}
		files = append(files, p)
	}
	cl := ua1FailedClauses(t, vp, files)
	// The control is the whole argument for artifacting rather than refusing: the document the door
	// falls back to fails the SAME clause. If this ever stops failing, the choice needs re-making.
	if cl["bare.pdf"] == nil || !cl["bare.pdf"]["7.1 t3"] {
		t.Fatalf("control: the bare scan does not fail 7.1 t3 (%v), so refusing the claim would have "+
			"produced a conforming document and this door's reasoning no longer holds", sortedClauses(cl["bare.pdf"]))
	}
	if cl["tagged.pdf"] == nil || cl["tagged.pdf"]["7.1 t3"] {
		t.Errorf("an OCR'd scan fails 7.1 t3 (%v)", sortedClauses(cl["tagged.pdf"]))
	}
	// Artifacting must not buy 7.1 t3 by opening 7.3 t1 — the trade the corpus measurement refused.
	if cl["tagged.pdf"] != nil && cl["tagged.pdf"]["7.3 t1"] {
		t.Errorf("an OCR'd scan fails 7.3 t1 (%v), so the picture was described in a way nothing fills",
			sortedClauses(cl["tagged.pdf"]))
	}
}

// TestAFormOnAnImageOnlyPageMakesNoClaim — `/pending 495`'s rule reaching the site its TEXT scoping
// could not: a scanned form's page draws no glyph at all, so the text guard saw nothing to refuse and
// `AuthorTaggedForm` wrote `/Marked true` over a picture no element covers.
//
// The form door does NOT artifact the image, and must not: the picture carries the field labels a
// sighted user reads, and whether the widgets' `/TU` repeats them is not something nib can know. So
// the honest answer here is the one 495 already gives for undescribed text — make no claim.
func TestAFormOnAnImageOnlyPageMakesNoClaim(t *testing.T) {
	host := scannedPage(t)
	if n, err := UnmarkedTextRuns(host); err != nil || n != 0 {
		t.Fatalf("setup: the image-only host has %d unmarked text run(s) (err %v), so the TEXT guard "+
			"would refuse this and the drawing guard is not what is measured", n, err)
	}
	out, tagged, err := AuthorTaggedForm(host, s07Fields())
	if err != nil {
		t.Fatal(err)
	}
	if tagged {
		t.Error("AuthorTaggedForm reported the form tagged on a page whose only content is a picture " +
			"nothing describes")
	}
	if inspectTags(out).claims() {
		t.Error("the returned document claims tagging")
	}
}

// TestACommitDeclaresAPictureNoElementCoversAnArtifact — the commit writer's half. `/pending 495` gave
// it uncovered TEXT and uncovered PATHS in the same loop and stopped there; a picture is the same
// clause reached by the same writer, which is ADR-009's "a rule reaching some sites and not others".
func TestACommitDeclaresAPictureNoElementCoversAnArtifact(t *testing.T) {
	src, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	pictured := withUnmarkedPicture(t, src)
	// Asserted in bytes, for the reason `TestAnOCRdScanDeclaresItsPageImageAnArtifact`'s setup gives.
	if bs := pageStream(t, pictured, 1); !bytes.HasPrefix(bs, []byte("q 40 0 0 40 50 50 cm /NibPic Do Q\n")) {
		t.Fatalf("setup: the host's page does not open by drawing the picture:\n%.400s", bs)
	}
	out, err := commitProposal(pictured, proposeFor(t, pictured).elements)
	if err != nil {
		t.Fatal(err)
	}
	if n, derr := uncoveredDrawings(out); derr != nil || n != 0 {
		t.Errorf("the committed page still draws %d thing(s) no sequence covers (err %v)", n, derr)
	}
	if !bytes.Contains(pageStream(t, out, 1), []byte("/Artifact BMC\n/NibPic Do\nEMC")) {
		t.Errorf("the committed page does not bracket the picture as an artifact:\n%.600s", pageStream(t, out, 1))
	}
}

// TestUncoveredDrawingSpansReadsEveryKindOfDrawing — each branch of the reader, separately, including
// the two it must NOT return.
func TestUncoveredDrawingSpansReadsEveryKindOfDrawing(t *testing.T) {
	images := map[string]bool{"Im0": true}
	for _, c := range []struct {
		name, src   string
		spans       int
		forms       int
		wantEnclose string
	}{
		{"an image XObject", "q 1 0 0 1 0 0 cm /Im0 Do Q", 1, 0, "/Im0 Do"},
		{"an image already an artifact", "/Artifact BMC /Im0 Do EMC", 0, 0, ""},
		{"an image already under an MCID", "/Figure <</MCID 0>> BDC /Im0 Do EMC", 0, 0, ""},
		{"an image under a plain tag", "/Span BMC /Im0 Do EMC", 1, 0, "/Im0 Do"},
		// A form's `Do` is never a span — its content is another stream, which may be drawn twice.
		{"a form XObject", "/Fm0 Do", 0, 1, ""},
		{"a covered form XObject", "/Artifact BMC /Fm0 Do EMC", 0, 1, ""},
		{"a shading", "q /Sh0 sh Q", 1, 0, "/Sh0 sh"},
		{"a covered shading", "/Artifact BMC /Sh0 sh EMC", 0, 0, ""},
		{"an inline image", "BI /W 1 /H 1 /CS /G /BPC 8 ID \x00 EI", 1, 0, ""},
		{"a covered inline image", "/Artifact BMC BI /W 1 /H 1 /CS /G /BPC 8 ID \x00 EI EMC", 0, 0, ""},
		// The path half `/pending 495` built, unchanged by the generalisation.
		{"a stroked path", "0 0 m 10 10 l S", 1, 0, "0 0 m 10 10 l S"},
		{"a `Do` with no name operand", "Do", 0, 0, ""},
	} {
		sp, forms := uncoveredDrawingSpans([]byte(c.src), images)
		if len(sp) != c.spans || len(forms) != c.forms {
			t.Errorf("%s: %d span(s) and %d form(s), want %d and %d", c.name, len(sp), len(forms), c.spans, c.forms)
			continue
		}
		if c.wantEnclose != "" && c.src[sp[0].start:sp[0].end] != c.wantEnclose {
			t.Errorf("%s: the span encloses %q, want %q", c.name, c.src[sp[0].start:sp[0].end], c.wantEnclose)
		}
	}
	// A covered `Do` is still reported as a form, with the cover recorded — that is how the counter
	// knows not to walk into it, and getting the flag backwards would double-count an n-up's sheets.
	if _, forms := uncoveredDrawingSpans([]byte("/Artifact BMC /Fm0 Do EMC"), images); !forms[0].covered {
		t.Error("a `Do` inside an /Artifact sequence is not reported as covered")
	}
	if _, forms := uncoveredDrawingSpans([]byte("/Fm0 Do"), images); forms[0].covered {
		t.Error("a bare `Do` is reported as covered")
	}
}

// withUnmarkedPicture prepends a drawn image XObject, in no marked content, to page 1 — the shape a
// producer leaves when it puts a logo on a page and describes nothing. `withUnmarkedPath`'s sibling.
func withUnmarkedPicture(t *testing.T, pdf []byte) []byte {
	t.Helper()
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		d, _, attrs, err := ctx.PageDict(1, false)
		if err != nil {
			return err
		}
		// A 1×1 greyscale image, uncompressed: the smallest thing that is unmistakably a picture.
		sd, err := ctx.NewStreamDictForBuf([]byte{0x80})
		if err != nil {
			return err
		}
		sd.Dict = types.Dict{
			"Type": types.Name("XObject"), "Subtype": types.Name("Image"),
			"Width": types.Integer(1), "Height": types.Integer(1),
			"ColorSpace": types.Name("DeviceGray"), "BitsPerComponent": types.Integer(8),
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		res := attrs.Resources
		if res == nil {
			res = types.Dict{}
		}
		xo, derr := ctx.DereferenceDict(res["XObject"])
		if derr != nil || xo == nil {
			xo = types.Dict{}
		}
		xo["NibPic"] = *ref
		res["XObject"] = xo
		d["Resources"] = res
		c, err := ctx.PageContent(d, 1)
		if err != nil {
			return err
		}
		return setPageContent(ctx, d, append([]byte("q 40 0 0 40 50 50 cm /NibPic Do Q\n"), c...))
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
