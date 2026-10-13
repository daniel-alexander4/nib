package pairing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// TestTheTagIsAFixedConstruction pins the hash against a value computed OUTSIDE this package
// (python3 hashlib): SHA-256("nib-discovery-tag-v1\x00" ‖ salt ‖ fingerprint), first 16 bytes.
// Every Nib on a link must compute the same tag from the same pin, so a change here is a wire
// format change and belongs with a version bump, not in a refactor.
func TestTheTagIsAFixedConstruction(t *testing.T) {
	fp := sha256.Sum256([]byte{1})
	var salt [SaltLen]byte
	for i := range salt {
		salt[i] = byte(i)
	}
	got := tagOf(fp[:], salt)
	if hex.EncodeToString(got[:]) != "314c57fbccb29b081b4b2f3b709f6af2" {
		t.Fatalf("tag = %x, want 314c57fbccb29b081b4b2f3b709f6af2 — the construction moved, and every "+
			"Nib of the version before this change now fails to recognise this one", got)
	}
	if !TagMatches(fp[:], salt, got) {
		t.Fatal("the fixed vector does not match itself")
	}
}

// TestATagMatchesItsOwnFingerprintUnderItsOwnSaltOnly — recognition, and each way it must fail.
func TestATagMatchesItsOwnFingerprintUnderItsOwnSaltOnly(t *testing.T) {
	alice := sha256.Sum256([]byte("alice"))
	bob := sha256.Sum256([]byte("bob"))
	salt, tag, err := NewTag(alice[:])
	if err != nil {
		t.Fatal(err)
	}
	if !TagMatches(alice[:], salt, tag) {
		t.Fatal("a tag does not match the fingerprint it was made from; nothing below means anything")
	}
	if TagMatches(bob[:], salt, tag) {
		t.Error("a tag matched somebody else's fingerprint")
	}
	otherSalt := salt
	otherSalt[0] ^= 1
	if TagMatches(alice[:], otherSalt, tag) {
		t.Error("a tag matched under a salt it was not made with — the salt is not in the hash")
	}
	otherTag := tag
	otherTag[TagLen-1] ^= 1
	if TagMatches(alice[:], salt, otherTag) {
		t.Error("a tag with one bit changed still matched — the comparison does not cover the whole tag")
	}
	// A truncated fingerprint is refused on both doors, as Name refuses one.
	if TagMatches(alice[:9], salt, tag) {
		t.Error("a nine-byte fingerprint matched")
	}
	if _, _, err := NewTag(alice[:9]); err == nil {
		t.Error("NewTag accepted a nine-byte fingerprint")
	}
}

// TestEveryNewTagHasItsOwnSalt — ADR-151's whole purpose. Two announcers of ONE identity must
// have nothing in common an observer could join them by, and a salt reused is exactly that.
func TestEveryNewTagHasItsOwnSalt(t *testing.T) {
	fp := sha256.Sum256([]byte("alice"))
	salts := map[[SaltLen]byte]bool{}
	tags := map[[TagLen]byte]bool{}
	for i := 0; i < 64; i++ {
		salt, tag, err := NewTag(fp[:])
		if err != nil {
			t.Fatal(err)
		}
		if salt == ([SaltLen]byte{}) {
			t.Fatal("NewTag returned the zero salt")
		}
		if salts[salt] {
			t.Fatalf("NewTag returned the salt %x twice for one identity: the salt is derived from "+
				"something stable, so every arm of this identity carries one constant", salt)
		}
		if tags[tag] {
			t.Fatalf("NewTag returned the tag %x twice for one identity", tag)
		}
		salts[salt], tags[tag] = true, true
		// The tag gives away nothing the name is made of: not the fingerprint's leading bytes
		// (the 66 bits the six words encode start there).
		if bytes.Contains(tag[:], fp[:4]) || bytes.Contains(salt[:], fp[:4]) {
			t.Fatalf("the fingerprint's leading bytes are in the salt or tag (%x / %x)", salt, tag)
		}
	}
}

// TestTheTagHasItsOwnDomain — the label is what keeps this hash from being any other SHA-256
// Nib computes over a fingerprint, so it is asserted to be IN the hash rather than read.
func TestTheTagHasItsOwnDomain(t *testing.T) {
	if !strings.HasPrefix(tagLabel, "nib-discovery-tag-v") || !strings.HasSuffix(tagLabel, "\x00") {
		t.Fatalf("label %q is not a nib-discovery-tag-vN label ending in a NUL", tagLabel)
	}
	fp := sha256.Sum256([]byte{1})
	var salt [SaltLen]byte
	salt[0] = 1
	bare := sha256.Sum256(append(append([]byte(nil), salt[:]...), fp[:]...))
	got := tagOf(fp[:], salt)
	if bytes.Equal(got[:], bare[:TagLen]) {
		t.Fatal("the tag is SHA-256(salt ‖ fingerprint) with no label")
	}
}
