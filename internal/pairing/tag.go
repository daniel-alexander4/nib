package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
)

// The announcement tag (ADR-151): what an armed Nib says about itself on the local link, in place
// of its six-word name.
//
// The name is a fixed function of a fingerprint that never rotates, so a name broadcast on every
// attached segment was one constant every listener could read and compare between two arms, two
// days or two networks without knowing whose it was. A tag is the same recognition — "this is the
// peer I already pinned" — bound to a salt drawn fresh for each announcer, so two arms of one
// identity share no byte.
//
// # What it does NOT hide, stated because the construction invites the wrong reading
//
// **Anyone who holds the fingerprint can test a tag against it**, exactly as a pin holder does:
// that is the whole mechanism, and a fingerprint is not a secret (it is in every document its
// owner signed). So a party who has such a document can still confirm "this identity is armed on
// this link now". What the salt takes away is the reading that needed no fingerprint at all: a
// listener who does not already hold the fingerprint cannot tell two arms apart from two
// strangers, and cannot learn the six words to look for later.
const (
	// SaltLen is the per-announcer salt. 128 bits so two salts never coincide by chance.
	SaltLen = 16
	// TagLen is the truncated hash. 128 bits: a chance match against somebody else's pin costs a
	// dial the TLS pin then refuses, and at this length it does not happen. The six-word name it
	// replaces carried 66.
	TagLen = 16
)

// tagLabel separates this hash from every other SHA-256 Nib computes over a fingerprint.
const tagLabel = "nib-discovery-tag-v1\x00"

// tagOf is the ONE construction: SHA-256(label ‖ salt ‖ fingerprint), truncated. Every input has
// a fixed length, so the concatenation is unambiguous without length prefixes.
func tagOf(fp []byte, salt [SaltLen]byte) [TagLen]byte {
	h := sha256.New()
	h.Write([]byte(tagLabel))
	h.Write(salt[:])
	h.Write(fp)
	var sum [sha256.Size]byte
	h.Sum(sum[:0])
	var tag [TagLen]byte
	copy(tag[:], sum[:])
	return tag
}

// NewTag draws a fresh salt and returns it with the tag of fp under it.
//
// **This is the only way to get a tag to SEND, and it takes no salt** — so a salt is never reused
// across announcers and never derived from anything stable, by construction rather than by each
// caller remembering to. One call per announcer; the announcer repeats that pair for its own life
// and no longer.
func NewTag(fp []byte) (salt [SaltLen]byte, tag [TagLen]byte, err error) {
	if len(fp) != sha256.Size {
		return salt, tag, errFingerprintLen
	}
	if _, err = rand.Read(salt[:]); err != nil {
		return [SaltLen]byte{}, tag, err
	}
	return salt, tagOf(fp, salt), nil
}

// TagMatches reports whether tag is fp's under salt. Like [Matches], it runs only from a
// fingerprint the caller ALREADY HOLDS: there is no way from a salt and a tag back to one.
func TagMatches(fp []byte, salt [SaltLen]byte, tag [TagLen]byte) bool {
	if len(fp) != sha256.Size {
		return false
	}
	want := tagOf(fp, salt)
	return subtle.ConstantTimeCompare(want[:], tag[:]) == 1
}
