package pdfops

import (
	"bytes"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

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
