package vault

import (
	"bytes"
	"testing"
)

// TestAnImageAccessorHandsBackBytesTheCallerOwns — /pending 567.
//
// The `Vault` struct's own doc says *"accessors return copies so callers never share a live slice or
// map"*. `Images`, `BuiltinImages` and `Image` did not: a shallow `append([]Image(nil), …)` copies the
// slice headers and `return img` copies the struct by value, and in both cases `Image.Data` stays
// aliased to the vault's live contents. `PinnedPeers`, `CeremonySecrets`, `Identity` and
// `ExternalSigner` all deep-copy their `[]byte` explicitly — one with a comment saying why — so
// images were the odd ones out, which is what made this a defect rather than a convention.
//
// **Mutating the RETURNED slice is the whole assertion**, because that is the only thing that tells
// a copy from an alias. It was latent when it was found — every caller read — and a test that only
// checked the bytes came back equal would have passed against the aliasing shape.
func TestAnImageAccessorHandsBackBytesTheCallerOwns(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	original := []byte{0x89, 'P', 'N', 'G', 1, 2, 3, 4}
	// **Handed in as the caller's own live slice, and then scribbled on**, which asserts the
	// write-side half in the same breath: `AddImage` used to retain this array, so an upload handler
	// reusing its buffer would have rewritten a stored signature. `addPinned` had always copied a
	// fingerprint in; images were the odd ones out in both directions.
	handedIn := append([]byte(nil), original...)
	added, err := v.AddImage("a signature", "image/png", handedIn)
	if err != nil {
		t.Fatal(err)
	}
	handedIn[0] = 0x00
	id := added.ID

	// `Image(id)` is the only accessor left that hands out pixel data — `Images()` and
	// `BuiltinImages()` had the same aliasing and were DELETED rather than fixed, having lost their
	// last production caller when the listing moved to `ImageMetas`.
	get := func() []byte {
		img, ok := v.Image(id)
		if !ok {
			t.Fatal("Image did not find the image just added")
		}
		return img.Data
	}

	// SETUP: the accessor hands back the bytes that were stored, or the mutation below scribbles on
	// something unrelated and the re-read proves nothing. This also asserts the WRITE side: the
	// slice handed to `AddImage` was scribbled on above, so equality here means it was copied in.
	got := get()
	if !bytes.Equal(got, original) {
		t.Fatalf("setup: Image returned %v, want %v — AddImage retained the caller's slice", got, original)
	}
	got[0] = 0xFF

	if after := get(); !bytes.Equal(after, original) {
		t.Errorf("writing to the slice Image returned changed the vault's own copy: now %v, was %v. "+
			"The struct's doc says an accessor returns a copy, and a caller that believes it can "+
			"decode, crop or re-encode in place would corrupt the stored signature with no error "+
			"anywhere", after, original)
	}
}

// TestTheImageListingNeverTouchesPixelData — the other half of /pending 567, and the reason the
// copy above is affordable.
//
// `handleImagesList` renders id, name and MIME and has never looked at a pixel — and it called
// `v.Images()` TWICE, once to size its slice and once to range over it. Deep-copying there as well
// would have doubled a cost the listing had no use for, which is the hot-path objection the item
// raised; `ImageMetas` answers it by not reading the data at all rather than by trading against it.
//
// **Asserted on the returned VALUE, not by timing**, because a benchmark that got slower would not
// say the data was copied and a benchmark that did not would not say it was left alone.
func TestTheImageListingNeverTouchesPixelData(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	added, err := v.AddImage("a signature", "image/png", bytes.Repeat([]byte{7}, 4096))
	if err != nil {
		t.Fatal(err)
	}

	metas := v.ImageMetas()
	if len(metas) != 1 {
		t.Fatalf("ImageMetas returned %d entries, want 1", len(metas))
	}
	// SETUP: it really is describing the image that was added, or "carries no data" is true of an
	// empty answer.
	if metas[0].ID != added.ID || metas[0].Name != "a signature" || metas[0].MIME != "image/png" {
		t.Fatalf("ImageMetas lost the metadata a listing renders: %+v", metas[0])
	}
	if metas[0].Builtin {
		t.Error("a stored library image is reported as binary-shipped, which is the one thing the " +
			"listing distinguishes them by")
	}
}
