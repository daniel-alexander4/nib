package sign

import (
	"testing"
)

// TestTheSignatureWalksDoNotPanicOnCorruptInput — /pending 502.
//
// trailingContentAfterLastSignature (reached from Verify) and hasCertificationSignature (reached
// from SignApproval) walk digitorus/pdf's Key/Index with no recover, while signatureBlobPresent
// needed one for the same walk (/pending 453). Measured before the fix: 1,047 of 2,482 single-bit
// flips of a signed fixture panicked out of each. Verify itself is not driven here: 4 of the same
// 2,482 flips (a damaged /Filter key on the object stream) take digitorus/pdf's object-stream lexer
// to an unbounded allocation that no recover contains, and that residue is not fixed by this test.
func TestTheSignatureWalksDoNotPanicOnCorruptInput(t *testing.T) {
	base := signedFixture(t)
	var flips, trailErrs, certErrs int
	for off := 0; off < len(base); off += 3 {
		for _, bit := range []byte{0x01, 0x80} {
			doc := append([]byte(nil), base...)
			doc[off] ^= bit
			flips++
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("trailingContentAfterLastSignature panicked on a flip at %d^%#x: %v", off, bit, r)
					}
				}()
				if _, _, err := trailingContentAfterLastSignature(doc); err != nil {
					trailErrs++
				}
			}()
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("hasCertificationSignature panicked on a flip at %d^%#x: %v", off, bit, r)
					}
				}()
				if _, err := hasCertificationSignature(doc); err != nil {
					certErrs++
				}
			}()
		}
	}
	// STIMULUS: the flips really reached corrupt input the walks cannot read. Without this a
	// fixture the parser shrugged off entirely would pass for the wrong reason.
	if trailErrs == 0 || certErrs == 0 {
		t.Fatalf("over %d flips the walks reported %d and %d errors — the corruption never reached them", flips, trailErrs, certErrs)
	}
	t.Logf("flips=%d trailingErrors=%d certificationErrors=%d", flips, trailErrs, certErrs)
}

// TestVerifyDigestRefusesADigestOfTheWrongLength — /pending 502 (info). VerifyDigestSPKI checked
// the length and VerifyDigest did not.
func TestVerifyDigestRefusesADigestOfTheWrongLength(t *testing.T) {
	cert, key, err := GenerateIdentity("A")
	if err != nil {
		t.Fatal(err)
	}
	digest := make([]byte, 32)
	digest[0] = 1
	sig, err := SignDigest(digest, key)
	if err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the right-length digest verifies, so the refusal below is about length alone.
	if err := VerifyDigest(digest, sig, cert); err != nil {
		t.Fatalf("setup: a correct digest did not verify: %v", err)
	}
	// A longer buffer whose first 32 bytes are the digest: ECDSA truncates, so without the length
	// rule this verifies.
	long := append(append([]byte(nil), digest...), 0xAA)
	if err := VerifyDigest(long, sig, cert); err == nil {
		t.Error("a 33-byte buffer verified as a SHA-256 digest")
	}
}
