package sign

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/verify"
	"github.com/digitorus/pkcs7"
)

// signerFingerprintsByBag maps each signature's certificate bag to the fingerprint of the
// certificate that ACTUALLY signed it — the one the SignerInfo names by issuer and serial.
//
// # Why this exists at all (ADR-051, /pending 613)
//
// `verify.Signer` reports the bag and not the signer: `buildCertificateChainsWithOptions`
// appends every certificate in `p7.Certificates` order and drops the issuer-and-serial that
// picked one of them, so nothing in the library's result says which. Reading the identity out
// of element 0 was therefore a guess, and it is a guess an attacker chooses: the bag sits in
// the `/Contents` hole in the `/ByteRange`, so it is not covered by the signature, its DER type
// is a SET (unordered by definition), and `p7.Verify` finds the signer by issuer and serial
// rather than by position. A signature made with the attacker's own key, carrying the victim's
// certificate first, verified `Valid` under the victim's fingerprint — a forged "X co-signed"
// that `p2p.Completeness` counted towards the roster and `unverifiedSigners` waved through as a
// pinned peer.
//
// # Keyed by the bag, because the library gives nothing else to key by
//
// The result cannot be a slice parallel to `resp.Signers`: this walk and the library's are two
// enumerations, and the repo has been bitten before by treating two walks of one document as
// interchangeable (`addedAfterVerdict`). The bag is the one value both sides hold in common and
// it survives the round trip byte for byte, so it is the join key — ORDER included, since two
// bags differing only in order are two different bags and the caller must not match across them.
//
// Where two signatures in one document share a bag but not a signer, the key is ambiguous and
// the entry is blanked: nothing may report an identity it has not established.
func signerFingerprintsByBag(pdf []byte) (m map[string]string) {
	// Positional, as in signatureBlobPresent: `digitorus/pdf`'s lazy dereferences panic on
	// ordinary corruption, and a nil map is the fail-closed answer — every lookup misses and
	// no fingerprint is reported.
	defer func() {
		if r := recover(); r != nil {
			m = nil
		}
	}()
	r, err := dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return nil
	}
	out := map[string]string{}
	// **This must be the SAME walk the library performs, and a cheaper one is a hole.**
	//
	// The library enumerates signatures by resolving every object in the document and testing it
	// for `/Filter /Adobe.PPKLite` (`pdfsign verify/verify.go:84-93`); it never consults
	// `AcroForm/Fields`, needing only that `/SigFlags` exists. A first cut of this function read
	// `/Fields` instead, because that is 176 µs on a 400-page document against 21.7 ms for the
	// sweep. **It was wrong, and the way it was wrong is the whole reason this file exists.**
	//
	// The bag is the join key and it lives in the unsigned `/Contents` hole, so the attacker
	// writes BOTH sides of it. Two walks over different object sets let them be written
	// separately: author the real signature — `/Filter /Adobe.PPKLite`, the attacker's key, bag
	// `[attacker, victim]` — reachable through the xref but NOT listed in `/Fields`, and add a
	// decoy `/Fields` entry with `/FT /Sig` whose `/V /Contents` carries the same two certificate
	// DERs in the same order, a `SignerInfo` naming the victim, and no `/Filter` key so the
	// library skips it. The cheap walk keys the bag from the decoy, the library's signer looks the
	// same bag up, and the victim's fingerprint comes back on a `Valid` signature — /pending 613
	// restored verbatim, by the fix for it.
	//
	// **"A gap is fail-closed because the map is read by key" was the false step.** A gap means
	// some OTHER object supplies the key, not that no object does. With one enumeration, any
	// second signature carrying the same bag is one the library also reports, so it lands in this
	// same map: same bag, different signer, and the entry is blanked rather than believed.
	//
	// **The cost is the sweep's cost and it is paid deliberately.** Measured: 18.7 ms on a
	// 400-page signed document beside the 20.0 ms `verify.Verify` call it follows, 4.9 ms at 100
	// pages, 201 µs at one page — roughly 0.6× to 1.0× that call, because it is the same sweep.
	// Whole-`Verify` totals over the same fixtures: 781 µs, 10.4 ms, 44.8 ms. Measured range 1 to
	// 400 pages, 3 KB to 79 KB, and not extrapolated past it.
	for _, x := range r.Xref() {
		v := r.Resolve(x.Ptr(), x.Ptr())
		if v.Key("Filter").Name() != "Adobe.PPKLite" {
			continue
		}
		blob := []byte(v.Key("Contents").RawString())
		if len(blob) == 0 {
			continue
		}
		p7, err := pkcs7.Parse(blob)
		if err != nil {
			continue // the library dropped this signer too; there is nothing to key
		}
		key := bagKey(rawsOf(p7.Certificates))
		// GetOnlySigner is nil for a bag that names no signer this document carries, and for
		// the multi-SignerInfo shape a PDF signature never has. Either way nib cannot say who
		// signed, and the empty string is what it says instead.
		fp := ""
		if cert := p7.GetOnlySigner(); cert != nil {
			fp = hex.EncodeToString(fingerprintOf(cert))
		}
		recordSigner(out, key, fp)
	}
	return out
}

// recordSigner files one signature's (bag, signer) pair, blanking a key two signatures disagree
// about.
//
// **Two signatures can share a bag and not a signer, and only the attacker would build that.**
// The bag is unsigned bytes, so nothing stops one document carrying two blobs with identical
// certificate lists whose `SignerInfo`s name different certificates in it. Keeping the first
// answer would hand the second signature an identity it does not have — /pending 613 with an extra
// step — so the key is blanked and both report nothing.
//
// A blanked key STAYS blanked: after the first disagreement `prev` is `""`, so any later non-empty
// fingerprint disagrees with it too. That is deliberate and it is the reason this is a named
// function rather than four lines inside the loop — the property is about a sequence of writes,
// which is not visible at any single one of them.
func recordSigner(out map[string]string, key, fp string) {
	if prev, seen := out[key]; seen {
		if prev != fp {
			out[key] = ""
		}
		return
	}
	out[key] = fp
}

// bagKeyOfSigner is signerFingerprintsByBag's key for what the library reported, and it is the
// only place the two enumerations are joined. A bag holding a nil certificate cannot be keyed —
// the lookup then misses, which is the fail-closed direction.
func bagKeyOfSigner(s *verify.Signer) string {
	raws := make([][]byte, 0, len(s.Certificates))
	for _, c := range s.Certificates {
		if c.Certificate == nil {
			return ""
		}
		raws = append(raws, c.Certificate.Raw)
	}
	return bagKey(raws)
}

// bagKey hashes a certificate bag, contents and order. Each element is length-prefixed so that
// no regrouping of the same bytes across certificate boundaries can collide with another bag.
func bagKey(raws [][]byte) string {
	if len(raws) == 0 {
		return ""
	}
	h := sha256.New()
	var n [8]byte
	for _, raw := range raws {
		binary.BigEndian.PutUint64(n[:], uint64(len(raw)))
		h.Write(n[:])
		h.Write(raw)
	}
	return string(h.Sum(nil))
}

func rawsOf(certs []*x509.Certificate) [][]byte {
	raws := make([][]byte, 0, len(certs))
	for _, c := range certs {
		raws = append(raws, c.Raw)
	}
	return raws
}
