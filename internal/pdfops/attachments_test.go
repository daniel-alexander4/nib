package pdfops

import (
	"bytes"
	"testing"
	"time"

	"crypto/sha256"
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"image"
	"image/color"
	"image/png"
	"nib/internal/testpdf"
)

// TestAttachmentName pins the basename reduction: paths are stripped and the
// dot-path names that survive a separator strip ("." / "..") reduce to "", which
// AddAttachment rejects — so no downstream disk write can ever see a dot-path.
func TestAttachmentName(t *testing.T) {
	cases := map[string]string{
		"report.pdf":     "report.pdf",
		"a/b/report.pdf": "report.pdf",
		`..\..\x`:        "x",
		"..":             "",
		".":              "",
		"../..":          "",
		"  spaced  ":     "spaced",
	}
	for in, want := range cases {
		if got := attachmentName(in); got != want {
			t.Errorf("attachmentName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAttachmentRoundTrip proves the full cycle on one in-memory document: a
// fresh doc has no attachments, AddAttachment embeds one, Attachments lists it,
// and ExtractAttachment returns the exact original bytes.
func TestAttachmentRoundTrip(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	if aa, err := Attachments(base); err != nil || len(aa) != 0 {
		t.Fatalf("fresh doc: got %v (err %v), want no attachments", aa, err)
	}

	payload := []byte("hello, this is an attached note\nwith two lines")
	withAtt, err := AddAttachment(base, "notes.txt", payload)
	if err != nil {
		t.Fatal(err)
	}

	aa, err := Attachments(withAtt)
	if err != nil {
		t.Fatal(err)
	}
	if len(aa) != 1 || aa[0].Name != "notes.txt" {
		t.Fatalf("after add: got %+v, want one attachment named notes.txt", aa)
	}

	got, err := ExtractAttachment(withAtt, "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("extracted bytes != original:\n got %q\nwant %q", got, payload)
	}
}

// TestAddAttachmentRejectsDuplicate proves a same-named attachment is rejected
// (rather than silently stored under a mangled key) — including when a path
// resolves to an existing basename.
func TestAddAttachmentRejectsDuplicate(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	one, err := AddAttachment(base, "dup.bin", []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddAttachment(one, "dup.bin", []byte("b")); err == nil {
		t.Error("a second attachment with the same name should be rejected")
	}
	if _, err := AddAttachment(one, "sub/dir/dup.bin", []byte("c")); err == nil {
		t.Error("a path resolving to an existing basename should be rejected")
	}
}

// TestExtractMissingAttachment proves extracting a name that isn't there errors
// rather than returning empty bytes.
func TestExtractMissingAttachment(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractAttachment(base, "nope.txt"); err == nil {
		t.Error("extracting from a doc with no attachments should error")
	}
}

// craftPageFileAttachmentPDF builds a one-page PDF carrying a page-level
// /FileAttachment annotation (a separate carrier from the catalog name tree that
// AddAttachment/pdfcpu only write). pdfcpu has no constructor for it, so the annot
// dict is hand-assembled — the embedded stream + filespec via pdfcpu helpers, then
// dropped into the page's /Annots, as scan_test.go does for a Link annotation.
func craftPageFileAttachmentPDF(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	sd, err := ctx.NewEmbeddedStreamDict(bytes.NewReader(data), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	fs, err := ctx.NewFileSpecDict(name, name, "", *sd)
	if err != nil {
		t.Fatal(err)
	}
	pd, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	pd["Annots"] = types.Array{types.Dict{
		"Type":    types.Name("Annot"),
		"Subtype": types.Name("FileAttachment"),
		"FS":      fs,
		"Rect":    types.NewNumberArray(100, 100, 120, 120),
	}}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestPageFileAttachment proves a page-level /FileAttachment — which the catalog
// name tree (and so pdfcpu's ListAttachments) does not cover — is now both listed
// (tagged with its page) and extractable with its exact original bytes, closing
// the gap where Scan flagged a file the Attachments panel couldn't see.
func TestPageFileAttachment(t *testing.T) {
	payload := []byte("attached at the page level\nnot in the name tree")
	pdf := craftPageFileAttachmentPDF(t, "page-note.txt", payload)

	aa, err := Attachments(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(aa) != 1 || aa[0].Name != "page-note.txt" {
		t.Fatalf("got %+v, want one attachment named page-note.txt", aa)
	}
	if aa[0].Desc != "Attached to page 1" {
		t.Errorf("desc = %q, want %q", aa[0].Desc, "Attached to page 1")
	}

	got, err := ExtractAttachment(pdf, "page-note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("extracted bytes != original:\n got %q\nwant %q", got, payload)
	}
}

// docFacts reports which document-level things survive in pdf. Walks the catalog directly
// rather than calling any pdfops accessor, so a broken accessor cannot make it agree.
func docFacts(t *testing.T, pdf []byte) (outline, lang, attach bool) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-reading the PDF: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	_, outline = root.Find("Outlines")
	_, lang = root.Find("Lang")
	if names, _ := ctx.XRefTable.DereferenceDict(root["Names"]); names != nil {
		_, attach = names.Find("EmbeddedFiles")
	}
	return
}

// TestARedactionDoesNotShipTheEmbeddedOriginal.
//
// **This is the regression the previous fix caused, and it is the sharper of the two.**
// v1.116.16 made `Collect` re-add the source's attachments so a page reorder would not destroy
// the ceremony record. `RedactPages` builds its untouched runs with `Collect` — so every
// redaction, split and page-export started carrying every embedded file into its output.
// Measured before the fix: a document with `original-unredacted.xlsx` came out of a redaction
// with the payload readable.
//
// `RedactPages`' contract is that the content is "genuinely gone — not merely covered", and an
// embedded file is not covered by rasterising a page.
func TestARedactionDoesNotShipTheEmbeddedOriginal(t *testing.T) {
	var pages []RasterPage
	for i := 0; i < 3; i++ {
		pages = append(pages, rasterPage(t, 200, 200))
	}
	base, err := ImagesToPDF(pages)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := AddAttachment(base, "original-unredacted.xlsx", []byte("SECRET PAYROLL"))
	if err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the payload really is in the source, or "absent from the output" is a
	// statement about a document that never had it.
	if a, aerr := Attachments(doc); aerr != nil || len(a) != 1 {
		t.Fatalf("setup: the source carries %d attachment(s) (%v)", len(a), aerr)
	}

	for _, c := range []struct {
		name string
		fn   func([]byte) ([]byte, error)
	}{
		{"RedactPages", func(b []byte) ([]byte, error) {
			return RedactPages(b, map[int]RasterPage{2: rasterPage(t, 200, 200)})
		}},
		{"Collect (export a page range)", func(b []byte) ([]byte, error) {
			return Collect(b, []string{"1-2"})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, ferr := c.fn(doc)
			if ferr != nil {
				t.Fatal(ferr)
			}
			got, aerr := Attachments(out)
			if aerr != nil {
				t.Fatal(aerr)
			}
			if len(got) != 0 {
				payload, _ := ExtractAttachment(out, got[0].Name)
				t.Errorf("%d embedded file(s) survived — %q, %q. An embedded original is not "+
					"covered by rasterising a page, and this output is what somebody is handed",
					len(got), got[0].Name, payload)
			}
		})
	}
}

// TestPageOperationsKeepWhatIsNotPageIndexed.
//
// `api.Collect` builds a brand-new context and migrates only the selected page dicts, the
// AcroForm fields whose widgets are on those pages, and Names["Dests"]. Everything else in the
// catalog is left behind — and a review filed that as an asymmetry (delete keeps them,
// everything else drops them). Measured: **both** doors lost /Lang and the attachments.
//
// Both halves are asserted here BECAUSE fixing one broke the other. The first fix carried
// attachments inside the primitive and leaked them into redactions (see the test above); the
// correction moved the attachment carry to an explicit opt-in the REARRANGEMENT ops call. So
// this test now pins where each half lives: /Lang in the primitive, attachments in
// CarryAttachments — and a future change that moves either one back has to fail one of them.
func TestPageOperationsKeepWhatIsNotPageIndexed(t *testing.T) {
	var pages []RasterPage
	for i := 0; i < 5; i++ {
		pages = append(pages, rasterPage(t, 200, 200))
	}
	base, err := ImagesToPDF(pages)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := SetLang(base, "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	doc, err = AddAttachment(doc, "nib-ceremony.json", []byte(`{"id":"abc"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, lang, attach := docFacts(t, doc); !lang || !attach {
		t.Fatalf("setup: lang=%v attach=%v before any page operation", lang, attach)
	}

	for _, c := range []struct {
		name string
		fn   func([]byte) ([]byte, error)
	}{
		{"Collect (reorder)", func(b []byte) ([]byte, error) {
			return Collect(b, []string{"2", "1", "3", "4", "5"})
		}},
		{"RemovePages", func(b []byte) ([]byte, error) { return RemovePages(b, []string{"2"}) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, ferr := c.fn(doc)
			if ferr != nil {
				t.Fatal(ferr)
			}
			// /Lang survives the PRIMITIVE, with no opt-in: it is a language tag, not
			// content, and every derived artifact should carry it.
			if _, lang, _ := docFacts(t, out); !lang {
				t.Error("/Lang is gone — the document stops declaring its language to a " +
					"screen reader, which is the accessibility property Nib ships")
			}
			// The attachments do NOT, until the caller asks.
			if _, _, attach := docFacts(t, out); attach {
				t.Error("the primitive carried attachments on its own — that is what leaked " +
					"an embedded original into a redaction")
			}
			// And with the opt-in, the ceremony record comes back READABLE, not as a
			// dangling reference: splicing the name tree across contexts produces refs into
			// the source xref and a document that will not re-read at all.
			carried, dropped, cerr := CarryAttachments(doc, out)
			if cerr != nil {
				t.Fatal(cerr)
			}
			if dropped != 0 {
				t.Errorf("CarryAttachments dropped %d — a silent miss is exactly what loses "+
					"the record this exists to preserve", dropped)
			}
			got, xerr := ExtractAttachment(carried, "nib-ceremony.json")
			if xerr != nil {
				t.Errorf("the ceremony record did not survive the opt-in carry: %v", xerr)
			} else if string(got) != `{"id":"abc"}` {
				t.Errorf("the record's bytes changed: %q", got)
			}
		})
	}

	// The half deliberately NOT carried, asserted so the decision stays visible. An outline
	// destination names a page OBJECT; after a Collect those objects are new and reordered,
	// so carrying it would send the reader to the wrong page — wrong is worse than absent.
	withOutline, err := SetOutline(doc, []OutlineItem{{Title: "Start", Page: 1}, {Title: "Middle", Page: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if o, _, _ := docFacts(t, withOutline); !o {
		t.Fatal("setup: no outline to lose")
	}
	reordered, err := Collect(withOutline, []string{"3", "1", "2", "4", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if o, _, _ := docFacts(t, reordered); o {
		t.Error("an outline survived a reorder — its destinations name page objects that no " +
			"longer mean what they did. If this is intentional, remap them and update this test")
	}
}

// TestContentDigestIsStableAcrossARewriteAndSeesTheImage.
//
// `ContentDigest` had **no test of any kind** before this one, despite its doc making four
// specific measured claims — which is part of why it was blind to `/Resources` for as long
// as it was.
//
// The two properties are in tension and that is the whole design. It cannot be a byte hash,
// because pdfcpu's rewrite is not idempotent and the convener would get one number and every
// later party a different one. And it must still notice a changed page, or it answers
// nothing. Both are asserted here against the same document.
// differentPNG is pngBytes with a different picture at the same dimensions.
func differentPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 251), uint8(y % 253), 7, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestContentDigestIsStableAcrossARewriteAndSeesTheImage(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	first, err := ContentDigest(base)
	if err != nil {
		t.Fatal(err)
	}

	// STIMULUS: the rewrite really is non-idempotent, or "stable across a rewrite" is a
	// claim about an operation that changed nothing and the test proves nothing.
	once, err := Optimize(base)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := Optimize(once)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(once, twice) {
		t.Fatal("setup: two rewrites produced identical bytes — a byte hash would do, and " +
			"this function's whole premise has changed")
	}

	for name, b := range map[string][]byte{"rewritten once": once, "rewritten twice": twice} {
		got, derr := ContentDigest(b)
		if derr != nil {
			t.Fatal(derr)
		}
		if got != first {
			t.Errorf("%s changed the digest (%s vs %s) — the convener and every later "+
				"party would compute different numbers for one document",
				name, got[:16], first[:16])
		}
	}

	// The half it was blind to, driven the way an attacker would: two documents whose
	// pages are the SAME SIZE and carry DIFFERENT pictures.
	//
	// This is the strong form, and it is the one that matters. Both produce byte-identical
	// content streams — same page box, same `/Im0 Do` boilerplate — so any difference in
	// the digest can only come from the resources. Corrupting a stream instead would be
	// caught by the decode marker alone, which is a weaker property: it would not see a
	// substitution that stays decodable, and a substituted contract does stay decodable.
	// (Proven: emptying the hashed resource BODIES while keeping the marker left the
	// XOR-based assertion below still green.)
	other, err := ImagesToPDF([]RasterPage{{Image: differentPNG(t, 400, 400), W: 200, H: 200}})
	if err != nil {
		t.Fatal(err)
	}
	otherDigest, err := ContentDigest(other)
	if err != nil {
		t.Fatal(err)
	}
	if otherDigest == first {
		t.Error("two documents with the same page geometry and DIFFERENT page images hash " +
			"identically — for a scanned contract that is the clauses, the amounts and the " +
			"signature block, all invisible to the digest a ceremony is convened over")
	}

	// And the corruption case, which the decode marker also covers.
	swapped, err := writeMutated(base, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		d, _, _, perr := xt.PageDict(1, false)
		if perr != nil {
			return perr
		}
		res, _ := xt.DereferenceDict(d["Resources"])
		xo, _ := xt.DereferenceDict(res["XObject"])
		if xo == nil {
			return errors.New("no XObject to swap")
		}
		for n := range xo {
			sd, _, serr := xt.DereferenceStreamDict(xo[n])
			if serr != nil || sd == nil {
				continue
			}
			for i := range sd.Raw {
				sd.Raw[i] ^= 0xff
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := ContentDigest(swapped)
	if err != nil {
		t.Fatal(err)
	}
	if after == first {
		t.Error("swapping the page image left the digest unchanged — for a scanned " +
			"document the content stream is invariant boilerplate and the image IS the " +
			"page, so a party could rewrite the clauses before the first signature and " +
			"CheckDocument would return clean")
	}
}

// TestContentDigestMovesOnEveryAxisAReaderCanSee.
//
// The first coverage pass hashed the content stream and the XObject stream BODY, and a review
// found four ways to change what a reader sees with the digest unmoved. Each is driven here as
// its own axis, and each is red on its own — a table, because a single "some mutation moves
// it" assertion is satisfied by whichever axis happens to work.
//
// The window that matters is the pre-first-signature hop `ceremony.CheckDocument` guards,
// where there are no signatures to fall back on and the counterparty holds the document.
func TestContentDigestMovesOnEveryAxisAReaderCanSee(t *testing.T) {
	base, err := ImagesToPDF([]RasterPage{rasterPage(t, 200, 200)})
	if err != nil {
		t.Fatal(err)
	}
	// A text stamp, so the page carries a /Font resource — the axis that was inert.
	base, err = StampWatermark(base, "CONFIDENTIAL", WatermarkStyle{}.sanitize())
	if err != nil {
		t.Fatal(err)
	}
	before, err := ContentDigest(base)
	if err != nil {
		t.Fatal(err)
	}

	// STIMULUS: the page really carries both resource kinds, or three of the axes below are
	// mutating something that is not there and pass for the wrong reason.
	func() {
		ctx, rerr := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
		if rerr != nil {
			t.Fatal(rerr)
		}
		d, _, _, perr := ctx.XRefTable.PageDict(1, false)
		if perr != nil {
			t.Fatal(perr)
		}
		res, _ := ctx.XRefTable.DereferenceDict(d["Resources"])
		if res == nil {
			t.Fatal("setup: the page has no /Resources")
		}
		if xo, _ := ctx.XRefTable.DereferenceDict(res["XObject"]); xo == nil || len(xo) == 0 {
			t.Fatal("setup: the page has no XObject — the image axes test nothing")
		}
		// The font is NOT at page level: pdfcpu puts a watermark inside a form XObject
		// whose OWN /Resources hold it. That is the recursion axis — a form's resources are
		// one level below anything a page-level-only walk reads — and finding it here is
		// what makes the font mutation below a real test rather than a no-op.
		if fd := findFontDict(ctx.XRefTable, res, 0); fd == nil {
			t.Fatal("setup: no font dict reachable from the page resources — the font axis " +
				"tests nothing, which is exactly the state that let \"Font\" sit inert")
		}
	}()

	for _, c := range []struct {
		name string
		why  string
		mut  func(ctx *model.Context) error
	}{
		{
			name: "the image's /Decode array",
			why:  "inverts the whole page image; the stream bytes are untouched",
			mut: func(ctx *model.Context) error {
				return onFirstXObject(ctx, func(sd *types.StreamDict) {
					sd.Dict["Decode"] = types.NewNumberArray(1, 0, 1, 0, 1, 0)
				})
			},
		},
		{
			name: "the image's /Width",
			why:  "the reader's interpretation of the same bytes",
			mut: func(ctx *model.Context) error {
				return onFirstXObject(ctx, func(sd *types.StreamDict) {
					sd.Dict["Width"] = types.Integer(7)
				})
			},
		},
		{
			name: "/CropBox",
			why:  "excises a paragraph from what every reader displays",
			mut: func(ctx *model.Context) error {
				d, _, _, err := ctx.XRefTable.PageDict(1, false)
				if err != nil {
					return err
				}
				d["CropBox"] = types.NewNumberArray(0, 0, 50, 50)
				return nil
			},
		},
		{
			name: "/Rotate",
			why:  "turns the page ninety degrees",
			mut: func(ctx *model.Context) error {
				d, _, _, err := ctx.XRefTable.PageDict(1, false)
				if err != nil {
					return err
				}
				d["Rotate"] = types.Integer(90)
				return nil
			},
		},
		{
			name: "the font's /BaseFont",
			why:  "changes every glyph the page renders; \"Font\" was inert in resourceKinds",
			mut: func(ctx *model.Context) error {
				d, _, _, err := ctx.XRefTable.PageDict(1, false)
				if err != nil {
					return err
				}
				res, _ := ctx.XRefTable.DereferenceDict(d["Resources"])
				fd := findFontDict(ctx.XRefTable, res, 0)
				if fd == nil {
					return errors.New("no font dict to mutate")
				}
				fd["BaseFont"] = types.Name("Wingdings")
				return nil
			},
		},
		{
			name: "a sticky-note annotation",
			why:  "Nib's own AddNotes writes these, and they touch no content stream",
			mut: func(ctx *model.Context) error {
				d, _, _, err := ctx.XRefTable.PageDict(1, false)
				if err != nil {
					return err
				}
				d["Annots"] = types.Array{types.Dict{
					"Type": types.Name("Annot"), "Subtype": types.Name("Text"),
					"Rect":     types.NewNumberArray(10, 10, 30, 30),
					"Contents": types.StringLiteral("the price is wrong"),
				}}
				return nil
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, merr := writeMutated(base, c.mut)
			if merr != nil {
				t.Fatalf("mutating: %v", merr)
			}
			after, derr := ContentDigest(out)
			if derr != nil {
				t.Fatalf("digest: %v", derr)
			}
			if after == before {
				t.Errorf("the digest is unchanged after mutating %s — %s. In the window "+
					"CheckDocument guards there is no signature to fall back on",
					c.name, c.why)
			}
		})
	}
}

// onFirstXObject applies fn to the page's first XObject stream, in place.
func onFirstXObject(ctx *model.Context, fn func(*types.StreamDict)) error {
	d, _, _, err := ctx.XRefTable.PageDict(1, false)
	if err != nil {
		return err
	}
	res, _ := ctx.XRefTable.DereferenceDict(d["Resources"])
	xo, _ := ctx.XRefTable.DereferenceDict(res["XObject"])
	for k := range xo {
		ir, ok := xo[k].(types.IndirectRef)
		if !ok {
			continue
		}
		e, ok := ctx.XRefTable.FindTableEntryForIndRef(&ir)
		if !ok || e == nil {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok {
			continue
		}
		fn(&sd)
		e.Object = sd
		return nil
	}
	return errors.New("no XObject stream to mutate")
}

// findFontDict returns the first font dictionary reachable from a resource dict, following
// form XObjects' own resources. pdfcpu nests a watermark's font one level down, so a
// page-level-only search finds nothing and every font assertion silently tests air.
func findFontDict(xt *model.XRefTable, res types.Dict, depth int) types.Dict {
	if res == nil || depth > 8 {
		return nil
	}
	if fo, _ := xt.DereferenceDict(res["Font"]); fo != nil {
		for k := range fo {
			if fd, _ := xt.DereferenceDict(fo[k]); fd != nil {
				return fd
			}
		}
	}
	xo, _ := xt.DereferenceDict(res["XObject"])
	for k := range xo {
		sd, _, err := xt.DereferenceStreamDict(xo[k])
		if err != nil || sd == nil {
			continue
		}
		inner, _ := xt.DereferenceDict(sd.Dict["Resources"])
		if fd := findFontDict(xt, inner, depth+1); fd != nil {
			return fd
		}
	}
	return nil
}

// TestContentDigestSeesAPageLevelFont — the axis the watermark fixture cannot reach.
//
// `TestContentDigestMovesOnEveryAxisAReaderCanSee` mutates a font, but pdfcpu nests a
// watermark's font inside a form XObject, so that assertion is carried by `hashObject`'s
// recursion and **dropping "Font" from `resourceKinds` leaves it green** — measured. An
// ordinary text document has the font directly on the page, which is the entry `resourceKinds`
// exists for and the one that was inert.
func TestContentDigestSeesAPageLevelFont(t *testing.T) {
	pdf, err := testpdf.Text("the price is four hundred pounds")
	if err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the font really is at PAGE level, not one form down.
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, _, _, err := ctx.XRefTable.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := ctx.XRefTable.DereferenceDict(d["Resources"])
	fo, _ := ctx.XRefTable.DereferenceDict(res["Font"])
	if len(fo) == 0 {
		t.Fatal("setup: no page-level /Font — this test would be carried by the XObject " +
			"recursion and would not exercise resourceKinds at all")
	}

	before, err := ContentDigest(pdf)
	if err != nil {
		t.Fatal(err)
	}
	out, err := writeMutated(pdf, func(c *model.Context) error {
		pd, _, _, perr := c.XRefTable.PageDict(1, false)
		if perr != nil {
			return perr
		}
		r, _ := c.XRefTable.DereferenceDict(pd["Resources"])
		f, _ := c.XRefTable.DereferenceDict(r["Font"])
		for k := range f {
			fd, _ := c.XRefTable.DereferenceDict(f[k])
			if fd != nil {
				fd["BaseFont"] = types.Name("Wingdings")
				return nil
			}
		}
		return errors.New("no font dict")
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := ContentDigest(out)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Error("swapping the page font's /BaseFont left the digest unchanged — every glyph " +
			"the page renders can change while the ceremony digest says it is the same document")
	}
}

// TestTheStreamBodyItselfEntersTheDigest.
//
// Every document-level axis above is carried by the stream DICT — two images of the same
// dimensions still differ in `/Length`, so `hashObject` distinguishes them without ever
// reading a byte of the body. Measured: replacing `hashChunk(h, body)` with a zero-length
// write left the whole package green.
//
// So the body is asserted here directly, with two streams whose dicts are byte-identical and
// whose content differs — the one construction a document-level fixture cannot produce,
// because any real re-encode moves `/Length` too. This is the substitution that matters: a
// swapped page image that stays decodable and the same size.
func TestTheStreamBodyItselfEntersTheDigest(t *testing.T) {
	mk := func(content string) types.StreamDict {
		return types.StreamDict{
			Dict: types.Dict{
				"Type": types.Name("XObject"), "Subtype": types.Name("Image"),
				"Width": types.Integer(4), "Height": types.Integer(4),
				"Length": types.Integer(len(content)),
			},
			Raw:     []byte(content),
			Content: []byte(content),
		}
	}
	a, b := mk("AAAABBBBCCCCDDDD"), mk("ZZZZBBBBCCCCDDDD")

	// STIMULUS: the dicts really are identical, or the difference below could come from
	// anywhere and this test says nothing about the body.
	da, db := sha256.New(), sha256.New()
	hashObject(nil, a.Dict, da, 0)
	hashObject(nil, b.Dict, db, 0)
	if !bytes.Equal(da.Sum(nil), db.Sum(nil)) {
		t.Fatal("setup: the two dicts hash differently, so this measures the dict and not the body")
	}
	if len(a.Raw) != len(b.Raw) {
		t.Fatal("setup: the bodies differ in length, which the dict would already have caught")
	}

	ha, hb := sha256.New(), sha256.New()
	hashStreamBody(&a, ha)
	hashStreamBody(&b, hb)
	if bytes.Equal(ha.Sum(nil), hb.Sum(nil)) {
		t.Error("two streams with identical dicts and different bodies hash the same — the " +
			"page image can be substituted for one of the same size and the digest will not move")
	}
}
