package pdfops

import (
	"bytes"
	"testing"
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
