package sign

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"

	"github.com/digitorus/pdfsign/verify"
)

// The certificate bag is no longer the join key (ADR-058 supersedes ADR-051's): the library's
// signers are lined up with the sweep's records by position (`joinLibrary`, revisions.go), and the
// bag survives as the per-position cross-check that catches a shift. What a signature's identity
// is — the certificate its SignerInfo names, never the bag's first element — is still ADR-051's,
// and the sweep computes it (`sweep`).

// bagKeyOfSigner is the cross-check's key for what the library reported, and whether there is
// one. A bag holding a nil certificate cannot be keyed, and neither can an empty one.
//
// **"Cannot be keyed" is its own answer and not the empty key** (/pending 653 part 10). Both
// conditions used to return "", and "" is also what `bagKey` gives a RECORD whose blob carries no
// certificates — so the join's comparison could find the two equal, and failed closed only because
// the library has never been seen to report a nil certificate. The caller refuses on `false`.
func bagKeyOfSigner(s *verify.Signer) (string, bool) {
	raws := make([][]byte, 0, len(s.Certificates))
	for _, c := range s.Certificates {
		if c.Certificate == nil {
			return "", false
		}
		raws = append(raws, c.Certificate.Raw)
	}
	if len(raws) == 0 {
		return "", false
	}
	return bagKey(raws), true
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
