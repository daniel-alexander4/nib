package sign

import (
	"strings"
	"testing"
)

// TestTheSignatureWalksDoNotPanicOnCorruptInput — /pending 502.
//
// trailingContentAfterLastSignature (reached from Verify) and hasCertificationSignature (reached
// from SignApproval) walked digitorus/pdf's Key/Index with no recover, while signatureBlobPresent
// needed one for the same walk (/pending 453). Measured before the fix: 1,047 of 2,482 single-bit
// flips of a signed fixture panicked out of each. **The first walk is gone** (P01.S02): coverage is
// read from the revision sweep's records, and the sweep is the walk driven below in its place.
//
// **Verify itself is still not driven here, and the count this comment used to give was a sample.**
// It said "4 of the same 2,482", which is what the `off += 3` stride below can see; sweeping EVERY
// offset found **12 of 7,438** (/pending 509). All twelve are a damaged `/Filter` key on the object
// stream, and all twelve are in `verify.Verify` rather than in either walk driven here — the three
// steps were re-run individually on a crashing fixture and all three returned normally. The residue
// is closed by ADR-041's readability gate, and its own test forks a process, because this one could
// not: a `fatal error: out of memory` takes the test binary.
func TestTheSignatureWalksDoNotPanicOnCorruptInput(t *testing.T) {
	base := signedFixture(t)
	var flips, certErrs, sweepErrs, sweepPanics, gated int
	for off := 0; off < len(base); off += 3 {
		for _, bit := range []byte{0x01, 0x80} {
			doc := append([]byte(nil), base...)
			doc[off] ^= bit
			flips++
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("hasCertificationSignature panicked on a flip at %d^%#x: %v", off, bit, r)
					}
				}()
				if _, err := ungatedCertification(doc); err != nil {
					certErrs++
				}
			}()
			// The revision sweep (P01.S01, T08) walks the same lazy dereferences over every xref
			// object, and `Verify` reaches it on the p2p arm too. A panic must come back as an error,
			// and an error must not be silent: a corrupt document that yields a clean empty sweep
			// would read as "no signature here". It is driven only behind ADR-041's readability
			// gate, exactly as `Verify` runs it: ungated, a damaged object stream reaches the
			// lexer that spins at EOF (`readHexString`) — measured here, the sweep hung — which
			// is the residue that gate exists to keep away from every digitorus parse.
			if pdfcpuCanRead(doc) != nil {
				continue
			}
			gated++
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("sweepRevisions panicked on a flip at %d^%#x: %v", off, bit, r)
					}
				}()
				if _, err := sweepRevisions(doc); err != nil {
					sweepErrs++
					if strings.HasPrefix(err.Error(), "read pdf: ") {
						sweepPanics++
					}
				}
			}()
		}
	}
	// STIMULUS: the flips really reached corrupt input the walks cannot read. Without this a
	// fixture the parser shrugged off entirely would pass for the wrong reason.
	// And the sweep's CONTAINED panics were reported, not merely its reader's parse errors: a
	// recover that swallowed the panic into a clean result would still leave the parse errors
	// counted above.
	if sweepPanics == 0 {
		t.Fatalf("over %d readable flips the sweep reported no contained panic — a recover that returns a "+
			"clean empty sweep would read as \"no signature here\"", gated)
	}
	if certErrs == 0 || sweepErrs == 0 {
		t.Fatalf("over %d flips the walks reported %d and %d errors — the corruption never reached them",
			flips, certErrs, sweepErrs)
	}
	t.Logf("flips=%d certificationErrors=%d readable=%d sweepErrors=%d sweepPanics=%d", flips, certErrs, gated, sweepErrs, sweepPanics)
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
