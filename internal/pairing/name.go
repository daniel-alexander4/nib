// Package pairing encodes an identity fingerprint as a short spoken name.
//
// A Nib identity is a SHA-256 of the certificate's SubjectPublicKeyInfo — 64 hex
// characters, which nobody reads aloud correctly. This package turns the leading bits
// of one into six ordinary English words that a person can say down a phone, and back
// again.
//
// # What the name is, and what it is not
//
// The name is a **display identity**: shown beside a signature, spoken to confirm "yes,
// that is me". It is derived from the key, so it cannot be claimed by someone else
// without grinding a keypair whose fingerprint shares the same leading 66 bits.
//
// **It is not a pin, and nothing here decodes one.** Pairing pins the full 32-byte
// fingerprint carried in an invitation; no screen accepts a name as a way to pin a peer.
// That is why [Matches] is the exported comparison and the decoder is not exported: a
// public function handing back 66 bits invites a caller to treat them as an identity,
// which is exactly the shape the design removed.
package pairing

import (
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// wordlist.txt is a frozen artifact, generated once by gen/gen_wordlist.py and pinned by
// a checksum in the tests. See gen/README.md for the sources and the selection rules.
//
//go:embed wordlist.txt
var wordlistFile string

// NameWords is the number of words in a pairing name.
const NameWords = 6

// bitsPerWord is fixed by the list length: 2048 = 2^11.
const bitsPerWord = 11

// VerificationWords is the number of words in a session verification string (D4): four,
// read aloud by one party and confirmed by the other after a channel is established.
const VerificationWords = 4

// VerificationBits is 44. Four words is all the design allows — the check is spoken, and
// a longer string is one people stop reading to the end of — so the security argument
// cannot rest on its width. It rests on the commitment step: with contributions committed
// before either side sees the other's, an attacker must GUESS the string (2⁻⁴⁴ per
// attempt, and a wrong guess is heard by two people) rather than SEARCH for a collision
// between two legs, which at 44 bits is a birthday problem costing about 2²² — seconds.
const VerificationBits = VerificationWords * bitsPerWord

// NameBits is how much of the fingerprint a name commits to — 66 bits.
//
// All of it is payload; there is no checksum. Six words is 66 bits with nothing left
// over, so spending any of it on a checksum spends the one property the name has.
// Detecting a transposition inside the decoder would cost bits to protect input no user
// can supply — nothing accepts a typed name — while the grinding cost of a lookalike
// name falls by the same factor. A transposition is caught where it actually happens:
// by [Matches] returning false, and by the human who hears the words in the wrong order.
const NameBits = NameWords * bitsPerWord

var (
	errWordCount      = errors.New("a pairing name is six words")
	errNotInList      = errors.New("not a word from the pairing list")
	errFingerprintLen = errors.New("a fingerprint is 32 bytes")
	errDigestLen      = errors.New("a verification digest is too short")
)

var (
	once  sync.Once
	words []string
	index map[string]int
)

func load() {
	once.Do(func() {
		words = strings.Fields(wordlistFile)
		index = make(map[string]int, len(words))
		for i, w := range words {
			index[w] = i
		}
	})
}

// allWords returns the wordlist. Unexported: nothing outside this package has a reason
// to hold 2048 words, and an exported accessor would be one more thing to keep true. The
// guards that assert the list's own properties live in this package and use it.
//
// The slice is the package's own backing array and must not be modified. Stated rather
// than defended with a copy: the only callers are in this file's own tests, and copying
// 2048 strings per call to guard against a caller that cannot exist is cost with no reader.
func allWords() []string {
	load()
	return words
}

// Name encodes the leading [NameBits] of fp as six space-separated words.
//
// The bits are taken big-endian from byte 0 — the most significant 66 bits of the
// fingerprint — and split into six 11-bit indices, most significant first. That choice
// is arbitrary in the sense that any fixed selection would do, and it is fixed forever
// in the sense that changing it changes every name: the vectors in the tests pin it,
// not this comment.
func Name(fp []byte) (string, error) {
	load()
	// Exactly 32, not "enough bytes to read 66 bits".
	//
	// The encoding reads nine bytes and never looks at the rest, so a length check
	// derived from what the encoder touches would accept a nine-byte slice — and hand
	// back a name IDENTICAL to the one the full fingerprint produces. A truncated or
	// mistaken value would then present as a correct-looking identity, which is the one
	// answer this package must never give. There is a single producer of these bytes
	// (sign.Fingerprint, a SHA-256 of the SPKI) and it has one length.
	if len(fp) != sha256.Size {
		return "", fmt.Errorf("%w: got %d", errFingerprintLen, len(fp))
	}
	return render(fp, NameWords), nil
}

// Verification renders the leading [VerificationBits] of digest as four words — the
// spoken check both sides compare once a channel is up (D4).
//
// digest is the output of the session's own derivation, not a fingerprint: what makes the
// two sides agree is that they hashed the same committed values, and what makes two
// sessions differ is that those values are fresh each time. This function only renders.
func Verification(digest []byte) (string, error) {
	load()
	if len(digest)*8 < VerificationBits {
		return "", fmt.Errorf("%w: got %d bytes, need at least %d",
			errDigestLen, len(digest), (VerificationBits+7)/8)
	}
	return render(digest, VerificationWords), nil
}

// render is the one bit-packer. Both renderings share it deliberately: two copies of
// "split big-endian into 11-bit indices" is two places for the convention to drift, and
// the drift would be invisible — each would round-trip against itself.
func render(b []byte, n int) string {
	out := make([]string, n)
	for i := range out {
		out[i] = words[bitsAt(b, i*bitsPerWord)]
	}
	return strings.Join(out, " ")
}

// bitsAt reads the 11 bits starting at bit offset off, big-endian.
func bitsAt(fp []byte, off int) int {
	v := 0
	for i := 0; i < bitsPerWord; i++ {
		b := off + i
		bit := (fp[b/8] >> (7 - uint(b%8))) & 1
		v = v<<1 | int(bit)
	}
	return v
}

// Matches reports whether fp is a fingerprint that produces name.
//
// This is the comparison the product uses, and it is the whole of the exported
// verification surface. It rejects a transposition, a substituted word, and a wrong
// name, by the same mechanism and without the encoding having to reserve anything: two
// different word sequences decode to two different bit strings.
//
// Whitespace and case are normalised, because the name travels through people —
// retyped, pasted out of an email, read off a screen.
func Matches(fp []byte, name string) bool {
	want, err := Name(fp)
	if err != nil {
		return false
	}
	got, err := normalize(name)
	if err != nil {
		return false
	}
	return got == want
}

// normalize lowercases, collapses whitespace, and checks the shape — the shared front
// half of Matches and decode, so the two cannot disagree about what a name is.
//
// Lowercasing is only safe because every word in the list is lowercase, and Matches
// compares against Name's output verbatim. That is asserted in wordlist_test.go
// (TestWordlistProperties); it is named here because this is the line that depends on
// it, and a capital in the list would break every retyped name from here.
func normalize(name string) (string, error) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(name)))
	if len(parts) != NameWords {
		return "", fmt.Errorf("%w: got %d", errWordCount, len(parts))
	}
	load()
	for _, p := range parts {
		if _, ok := index[p]; !ok {
			return "", fmt.Errorf("%w: %q", errNotInList, p)
		}
	}
	return strings.Join(parts, " "), nil
}

// decode turns a name back into the [NameBits] it encodes, returned as 9 bytes whose
// last 6 bits are zero.
//
// Deliberately unexported. It exists so the round-trip is testable in both directions —
// an encoding proved injective only in one direction is not proved injective — and for
// no other reason. Exporting it would offer callers a half-fingerprint, and a
// half-fingerprint that looks like an identity is the misuse this design removed.
func decode(name string) ([]byte, error) {
	norm, err := normalize(name)
	if err != nil {
		return nil, err
	}
	out := make([]byte, (NameBits+7)/8)
	for i, w := range strings.Fields(norm) {
		v := index[w]
		for j := 0; j < bitsPerWord; j++ {
			if v&(1<<(bitsPerWord-1-j)) != 0 {
				b := i*bitsPerWord + j
				out[b/8] |= 1 << (7 - uint(b%8))
			}
		}
	}
	return out, nil
}
