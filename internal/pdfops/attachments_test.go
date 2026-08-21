package pdfops

import (
	"bytes"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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

// TestPageOperationsKeepWhatIsNotPageIndexed.
//
// `api.Collect` builds a brand-new context and migrates only the selected page dicts, the
// AcroForm fields whose widgets are on those pages, and Names["Dests"]. Everything else in
// the catalog is left behind.
//
// **A review filed this as an asymmetry — delete keeps them, everything else drops them —
// and the measurement refuted that.** Both doors lost /Lang and the attachments. The
// asymmetry was not real; the loss was total. For the ceremony that is the sharp end:
// `nib-ceremony.json` is an attachment, so reordering or deleting a page after
// PrepareDocument destroyed the record and the handler returned 200.
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

	// STIMULUS: both really are present before the operation, or "still present" after is
	// a statement about a document that never had them.
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
		{"Collect (subset)", func(b []byte) ([]byte, error) { return Collect(b, []string{"1-3"}) }},
		{"RemovePages", func(b []byte) ([]byte, error) { return RemovePages(b, []string{"2"}) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := c.fn(doc)
			if err != nil {
				t.Fatal(err)
			}
			_, lang, attach := docFacts(t, out)
			if !attach {
				t.Error("the embedded files are gone — nib-ceremony.json is an attachment, " +
					"so this destroys a ceremony record and reports success")
			}
			if !lang {
				t.Error("/Lang is gone — the document stops declaring its language to a " +
					"screen reader, which is the accessibility property Nib ships")
			}
			// The attachment must still be READABLE, not merely present: a name tree
			// spliced across contexts leaves refs that dangle, and the first attempt at
			// this fix produced exactly that — a document that would not re-read at all
			// ("dict=fileSpecDict required entry=F missing").
			got, xerr := ExtractAttachment(out, "nib-ceremony.json")
			if xerr != nil {
				t.Errorf("the attachment survived as an unreadable reference: %v", xerr)
			} else if string(got) != `{"id":"abc"}` {
				t.Errorf("the attachment's bytes changed: %q", got)
			}
		})
	}

	// And the half that is deliberately NOT carried, asserted so the decision is visible
	// rather than looking like an oversight. An outline destination names a page OBJECT;
	// after a Collect those objects are new and reordered, so carrying it would send the
	// reader to the wrong page — wrong is worse than absent. Remapping is per-operation
	// work and a different change; Outline/SetOutline exist for a caller that wants it.
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
		t.Error("an outline survived a reorder — its destinations name page objects that " +
			"no longer mean what they did, so it now points at the wrong pages. If this " +
			"is intentional, the destinations must be remapped and this test updated")
	}
}
