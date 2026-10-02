package sign

import (
	"bytes"
	"crypto"
	_ "crypto/sha1"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"sort"

	"github.com/digitorus/pkcs7"
)

// RevisionCause names why the version a signer signed could not be handed back (D7, as amended by the P02
// plan-review). Each is a different sentence to the user, so they are never lumped.
type RevisionCause string

const (
	// RevisionNoSignature: nothing in the file is a signature — a document timestamp is not one (ADR-060), so a
	// timestamp-only document lands here too, and the sentence must not say "nothing signed this".
	RevisionNoSignature RevisionCause = "no-signature"
	// RevisionResaved: a signature NAMING the requested certificate is in the file and does not verify against it. The
	// name is the SignerInfo's own claim, which anyone can write (the certificate rides in every document its owner
	// signed), so this is never "your signature" unless `Attributed` — and even then it says only "made with that key
	// over other bytes", which a re-save and a transplant both produce (plan-review C1, W10).
	RevisionResaved RevisionCause = "resaved"
	// RevisionNotYours: the file holds signatures, and none of them names the requested certificate.
	RevisionNotYours RevisionCause = "not-your-signature"
	// RevisionPrefixFailed: a candidate version was cut out of the file and did not stand up on its own (D4) — it did
	// not parse, or the signature did not re-verify across exactly its bytes.
	RevisionPrefixFailed RevisionCause = "prefix-failed-reverify"
	// RevisionCouldNotCheck: the whole file could not be checked and no candidate re-verified. **Parked for Dan as an
	// amendment to D7** (plan-review C2): none of the four causes is true of a file nib could not read, and borrowing
	// one would state something nib never observed.
	RevisionCouldNotCheck RevisionCause = "could-not-check"
)

// maxRevisionCandidates bounds how many prefixes `SignedRevisionFor` re-verifies, each a full `Revisions`.
const maxRevisionCandidates = 16

// maxCandidateRecords bounds how many naming records are considered at all; each costs at most one signature check
// over its signed attributes (~0.1 ms) before anything is hashed.
const maxCandidateRecords = 1024

// screenBudgetFactor bounds the bytes hashed screening candidates when the whole file did not verify, as a multiple
// of the file's length. **Measured 2026-10-02**: without a screen, 40 appended copies of the signer's own blob
// claiming ends past the real one filled every verify slot ahead of it — `could-not-check` for a version that was in
// the file, after 13 s at 40 MB; with it, 1.4 s and the version. Hashing is ~3% of a verify per file length, so the
// budget is under one verify. **Declared residual**: copies an attacker makes WELL-FORMED (each its own dictionary
// with its own gap holding the signer's blob) sort with the genuine record, and enough of them covering the whole file
// exhaust the budget first — the answer is then `could-not-check`, which is honest, never `not-your-signature`.
const screenBudgetFactor = 16

// SignedRevision is what the file says about one signer's signed version: the prefix, or why there is none.
type SignedRevision struct {
	// Prefix is the signed version — a slice of the input, capped at its own length so an append cannot write into
	// the caller's file. Nil on a refusal.
	Prefix []byte `json:"-"`
	// Cause is empty when Prefix is set.
	Cause RevisionCause `json:"cause,omitempty"`
	// Obj is the object number of the signature that re-verified inside the prefix, and End the prefix's length.
	Obj uint32 `json:"obj,omitempty"`
	End int64  `json:"end,omitempty"`
	// RedefinedObj is set when the version was found only through a record the file's latest revision refused — a
	// later revision redefined the signature's dictionary (plan-review W8). That is evidence, and P03 says so.
	RedefinedObj uint32 `json:"redefinedObj,omitempty"`
	// Later names records naming this certificate that reach past End or out of the file: a later signature of the
	// same name that did not hold (W3). Their names are unverified claims.
	Later []uint32 `json:"later,omitempty"`
	// Earlier is the coverage end of each earlier verified signature by the same certificate (W3).
	Earlier []int64 `json:"earlier,omitempty"`
	// Refused is every refused record in the file, on a refusal (I6).
	Refused []RefusedSignature `json:"refused,omitempty"`
	// Attributed, on `resaved` only: the SignerInfo's signature over its signed attributes checks against the named
	// certificate's key (C1). False means the name is only a claim.
	Attributed bool `json:"attributed,omitempty"`
}

// SignedRevisionFor returns the version of pdf that the signer with this certificate fingerprint signed (D1), or the
// reason there is none. A version is returned only if it stands up on its own: cut out of the file, it parses and that
// signer's signature re-verifies across exactly its bytes (D4). An identity the file merely NAMES chooses which
// versions to try and never, by itself, what nib says (plan-review C1).
//
// **One verify of the whole file, then one per candidate** (`maxRevisionCandidates`). Candidates come from the
// records `verifyIndexed` returns — beside its error, where it has one — and never from a second sweep, which would put
// the digitorus reader on a file the pdfcpu gate refused (ADR-041).
func SignedRevisionFor(pdf []byte, fingerprint string) SignedRevision {
	_, revs, err := verifyIndexed(pdf)
	cands := revisionCandidates(pdf, revs, err, fingerprint)
	failed := false
	verifies := 0
	budget := int64(screenBudgetFactor) * int64(len(pdf))
	for _, c := range cands {
		// A candidate the library verified over a file that verified is exact. Any other is only a NAME, and one is
		// re-verified only if it could hold: its SignerInfo checks against the named key, and the bytes its ranges
		// select hash to the digest that signature covers. A copy of the signer's own blob with other ranges cannot
		// pass. What remains is the budget (`screenBudgetFactor`'s declared
		// residual): enough well-formed copies spend it, and the search then ends as `could-not-check`.
		if !c.checked && !c.screen(pdf, &budget) {
			continue
		}
		if verifies == maxRevisionCandidates {
			break
		}
		verifies++
		prefix := pdf[:c.end:c.end]
		prs, perr := revs, err
		if c.end != int64(len(pdf)) || err != nil {
			prs, perr = Revisions(prefix)
		}
		if perr != nil {
			failed = true
			continue
		}
		obj, ok := holder(prs, fingerprint, c.end)
		if !ok {
			failed = true
			continue
		}
		out := SignedRevision{Prefix: prefix, Obj: obj, End: c.end, Later: laterNaming(pdf, revs, fingerprint, c.end)}
		if c.onlyRefused {
			out.RedefinedObj = c.obj
		}
		for _, r := range revs {
			if r.Fingerprint == fingerprint && r.countsAsSigner() && r.CoverageEnd < c.end {
				out.Earlier = append(out.Earlier, r.CoverageEnd)
			}
		}
		return out
	}
	out := SignedRevision{Refused: refusedOf(revs)}
	switch {
	case err != nil:
		out.Cause = RevisionCouldNotCheck
	case failed:
		out.Cause = RevisionPrefixFailed
	default:
		out.Cause = RevisionNoSignature
		for _, r := range revs {
			if fingerprint != "" && r.named == fingerprint && !r.Timestamp {
				out.Cause = RevisionResaved
				if r.proof.attributed() {
					out.Attributed = true
				}
				continue
			}
			if out.Cause != RevisionResaved && (r.countsAsSigner() || (r.Cause != "" && r.hasContents)) {
				out.Cause = RevisionNotYours
			}
		}
	}
	return out
}

// candidate is one version worth re-verifying: where a record naming the signer says its signature ends.
type candidate struct {
	end int64
	// obj is a record that proposed this end; onlyRefused is true when every record proposing it was refused.
	obj         uint32
	onlyRefused bool
	// proposers are the records proposing this end, for the screen.
	proposers []*Revision
	// checked is true when a proposer was verified by the library over a file that verified: the screen is then the
	// library's own, and exact.
	checked bool
}

// screen reports whether any record proposing this end could hold: its SignerInfo checks against the certificate it
// names, and its ranges' bytes hash to the messageDigest that signature covers. The bytes a record would hash are
// charged to budget BEFORE they are hashed; once a record would overrun it, budget is spent (-1) and nothing is hashed
// again — every later candidate is refused here, which ends the search as `could-not-check`.
func (c candidate) screen(pdf []byte, budget *int64) bool {
	for _, r := range c.proposers {
		if r.proof == nil {
			continue
		}
		bare := r.proof.signedAttrs == nil // signs the content itself, so the check IS the hash of the ranges
		if !bare && !r.proof.attributed() {
			continue
		}
		cost, ok := rangeCost(r.ByteRange, len(pdf))
		if !ok {
			continue
		}
		if cost > *budget {
			*budget = -1
			return false
		}
		*budget -= cost
		if bare {
			if r.proof.encapsulated || r.proof.checks(selected(pdf, r.ByteRange)) {
				return true
			}
			continue
		}
		if r.proof.digestMatches(pdf, r.ByteRange) {
			return true
		}
	}
	return false
}

// revisionCandidates is the seam P02.S02 extends with the file's earlier revision boundaries.
//
// A candidate is the last-pair end of a record whose SignerInfo names the fingerprint and which is not a document
// timestamp (a timestamp authority's certificate is never "the version you signed"). While the whole file verified,
// only a record the library VERIFIED is a candidate — the prefix hashes the same bytes, so that screen is exact and
// free. When it did not, every naming record is one: the error says nothing about which record was at fault (one
// appended negative-length copy fails the whole file while the signer's own version is intact — C2). Refused records
// are candidates on purpose: a later revision redefining the signer's dictionary leaves the original ByteRange on the
// refused record, and its prefix is the signer's version. Ends proposed by a well-formed record come first, then the
// rest, each newest first.
func revisionCandidates(pdf []byte, revs []Revision, err error, fingerprint string) []candidate {
	if fingerprint == "" {
		return nil
	}
	at := map[int64]int{}
	var out []candidate
	considered := 0
	for i := range revs {
		r := &revs[i]
		// A record the library checked and FAILED is no candidate while the whole file verified: the prefix would hash
		// the same bytes. A record the library never enumerated (`libPos < 0` — e.g. a later revision dropped
		// `/SigFlags`) was not checked at all, so it is screened like any name on the error path.
		if r.named != fingerprint || r.Timestamp || (err == nil && r.libPos >= 0 && !r.Verified) {
			continue
		}
		end, ok := lastPairEnd(r.ByteRange, len(pdf))
		if !ok {
			continue
		}
		if considered == maxCandidateRecords {
			break
		}
		considered++
		checked := err == nil && r.Verified
		if j, seen := at[end]; seen {
			if r.Cause == "" {
				out[j].onlyRefused = false
			}
			out[j].checked = out[j].checked || checked
			out[j].proposers = append(out[j].proposers, r)
			continue
		}
		at[end] = len(out)
		out = append(out, candidate{end: end, obj: r.Obj, onlyRefused: r.Cause != "", checked: checked, proposers: []*Revision{r}})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].onlyRefused != out[j].onlyRefused {
			return !out[i].onlyRefused
		}
		return out[i].end > out[j].end
	})
	return out
}

// lastPairEnd is where a raw ByteRange's last pair ends (D5), refused for anything a slice could not take.
func lastPairEnd(br []int64, size int) (int64, bool) {
	if len(br) < 2 || len(br)%2 != 0 {
		return 0, false
	}
	a, b := br[len(br)-2], br[len(br)-1]
	if a < 0 || b < 0 {
		return 0, false
	}
	end := satAdd(a, b)
	return end, end > 0 && end <= int64(size)
}

// holder is the record inside a cut-out prefix that proves it: a verified, well-formed signer with this fingerprint
// whose coverage is the whole prefix (I2 — the check that can fail; the length matches by construction of the cut).
func holder(revs []Revision, fingerprint string, end int64) (uint32, bool) {
	for _, r := range revs {
		if r.Fingerprint == fingerprint && r.Verified && r.countsAsSigner() && r.CoverageEnd == end {
			return r.Obj, true
		}
	}
	return 0, false
}

// laterNaming lists records naming the fingerprint whose signature claims to reach past end, or out of the file.
func laterNaming(pdf []byte, revs []Revision, fingerprint string, end int64) []uint32 {
	var out []uint32
	for _, r := range revs {
		if r.named != fingerprint || r.Timestamp {
			continue
		}
		if e, ok := lastPairEnd(r.ByteRange, len(pdf)); !ok || e > end {
			out = append(out, r.Obj)
		}
	}
	return out
}

// signedAttribute mirrors pkcs7's attribute so the SET can be re-marshalled as it was signed.
type signedAttribute struct {
	Type  asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"set"`
}

var (
	oidDigestSHA1    = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidDigestSHA256  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidDigestSHA384  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidDigestSHA512  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	oidMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
)

// hashOf is the hash a SignerInfo's digest algorithm names — the one the library itself uses for both the content and
// the signed attributes. SHA-1 is in: third-party signatures still use it, and the question here is whether bytes were
// signed with a key, not whether the algorithm is fit for new signatures.
func hashOf(d asn1.ObjectIdentifier) (crypto.Hash, bool) {
	switch {
	case d.Equal(oidDigestSHA1):
		return crypto.SHA1, true
	case d.Equal(oidDigestSHA256):
		return crypto.SHA256, true
	case d.Equal(oidDigestSHA384):
		return crypto.SHA384, true
	case d.Equal(oidDigestSHA512):
		return crypto.SHA512, true
	}
	return 0, false
}

// signerProof is what `SignedRevisionFor` keeps of a signature blob, COPIED out of it so nothing aliases `/Contents`.
type signerProof struct {
	cert         *x509.Certificate // the one certificate the SignerInfo names
	signedAttrs  []byte            // the DER of the signed attributes as a SET OF — the bytes the signature covers
	signature    []byte            // EncryptedDigest
	digest       asn1.ObjectIdentifier
	encryption   asn1.ObjectIdentifier // DigestEncryptionAlgorithm
	messageDigst []byte                // the messageDigest attribute; nil when absent
	encapsulated bool                  // the SignedData carries its content (the `adbe.pkcs7.sha1` shape)
}

// proofOf copies the single SignerInfo's proof out of p7, or nil when there is not exactly one named signer.
func proofOf(p7 *pkcs7.PKCS7) *signerProof {
	cert := p7.GetOnlySigner()
	if cert == nil || len(p7.Signers) != 1 {
		return nil
	}
	own, err := x509.ParseCertificate(bytes.Clone(cert.Raw))
	if err != nil {
		return nil
	}
	si := p7.Signers[0]
	pr := &signerProof{cert: own, signature: bytes.Clone(si.EncryptedDigest), digest: append(asn1.ObjectIdentifier(nil), si.DigestAlgorithm.Algorithm...),
		encryption: append(asn1.ObjectIdentifier(nil), si.DigestEncryptionAlgorithm.Algorithm...), encapsulated: len(p7.Content) > 0}
	if len(si.AuthenticatedAttributes) == 0 {
		return pr
	}
	attrs := make([]signedAttribute, len(si.AuthenticatedAttributes))
	for i, a := range si.AuthenticatedAttributes {
		attrs[i] = signedAttribute{a.Type, a.Value}
		if a.Type.Equal(oidMessageDigest) {
			var md []byte
			if _, err := asn1.Unmarshal(a.Value.Bytes, &md); err == nil {
				pr.messageDigst = bytes.Clone(md)
			}
		}
	}
	// The signature is over the attributes' DER as a SET OF (RFC 5652 §5.4), which asn1 sorts on Marshal — the same
	// re-marshal the library verifies against.
	enc, err := asn1.Marshal(struct {
		A []signedAttribute `asn1:"set"`
	}{attrs})
	if err != nil {
		return pr
	}
	var set asn1.RawValue
	if _, err := asn1.Unmarshal(enc, &set); err != nil {
		return pr
	}
	pr.signedAttrs = bytes.Clone(set.Bytes)
	return pr
}

// signatureAlgorithm maps a SignerInfo's digest and encryption algorithms EXACTLY as the library does (`pkcs7`
// `getSignatureAlgorithm`, verify.go): the ECDSA-with-hash OIDs fix the hash; every RSA OID — `sha256WithRSA` included
// — and every curve OID take it from the digest algorithm; Ed25519 signs its message whole. A shape the library
// verifies and this refuses throws a genuine version away on the screened path, so the two must not drift.
func (pr *signerProof) signatureAlgorithm() (x509.SignatureAlgorithm, bool) {
	enc, d := pr.encryption, pr.digest
	byDigest := func(sha1, sha256, sha384, sha512 x509.SignatureAlgorithm) (x509.SignatureAlgorithm, bool) {
		switch {
		case d.Equal(pkcs7.OIDDigestAlgorithmSHA1):
			return sha1, true
		case d.Equal(pkcs7.OIDDigestAlgorithmSHA256):
			return sha256, true
		case d.Equal(pkcs7.OIDDigestAlgorithmSHA384):
			return sha384, true
		case d.Equal(pkcs7.OIDDigestAlgorithmSHA512):
			return sha512, true
		}
		return 0, false
	}
	switch {
	case enc.Equal(pkcs7.OIDDigestAlgorithmECDSASHA1):
		return x509.ECDSAWithSHA1, true
	case enc.Equal(pkcs7.OIDDigestAlgorithmECDSASHA256):
		return x509.ECDSAWithSHA256, true
	case enc.Equal(pkcs7.OIDDigestAlgorithmECDSASHA384):
		return x509.ECDSAWithSHA384, true
	case enc.Equal(pkcs7.OIDDigestAlgorithmECDSASHA512):
		return x509.ECDSAWithSHA512, true
	case enc.Equal(pkcs7.OIDEncryptionAlgorithmRSA), enc.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA1),
		enc.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA256), enc.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA384),
		enc.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA512):
		return byDigest(x509.SHA1WithRSA, x509.SHA256WithRSA, x509.SHA384WithRSA, x509.SHA512WithRSA)
	case enc.Equal(pkcs7.OIDEncryptionAlgorithmECDSAP256), enc.Equal(pkcs7.OIDEncryptionAlgorithmECDSAP384),
		enc.Equal(pkcs7.OIDEncryptionAlgorithmECDSAP521):
		return byDigest(x509.ECDSAWithSHA1, x509.ECDSAWithSHA256, x509.ECDSAWithSHA384, x509.ECDSAWithSHA512)
	case enc.Equal(pkcs7.OIDEncryptionAlgorithmEDDSA25519):
		return x509.PureEd25519, true
	}
	return 0, false
}

// checks reports whether the SignerInfo's signature verifies over signed with the named certificate's key.
func (pr *signerProof) checks(signed []byte) bool {
	alg, ok := pr.signatureAlgorithm()
	return ok && pr.cert.CheckSignature(alg, signed, pr.signature) == nil
}

// attributed reports whether the SignerInfo's signature over its signed attributes checks against the key of the
// certificate it names (plan-review C1) — "made with that key", over bytes this file may no longer hold. It is computed
// only where `SignedRevisionFor` screens a candidate or is about to say `resaved`, never on the verdict path. A
// SignerInfo with NO signed attributes signs the content itself, so there is nothing to attribute apart from bytes
// this file holds: it is false here, and the screen checks it over the bytes its ranges select instead.
func (pr *signerProof) attributed() bool {
	return pr != nil && pr.signedAttrs != nil && pr.checks(pr.signedAttrs)
}

// rangeCost is how many bytes a ByteRange selects, refused where a pair is negative, runs past the file, or does not
// begin after the previous pair ends — so one record costs at most the file's length to hash (P02.S01's review: one
// copy listing `0 L` sixty-four times spent the whole budget).
func rangeCost(br []int64, size int) (int64, bool) {
	if len(br) == 0 || len(br)%2 != 0 {
		return 0, false
	}
	var cost, prev int64
	for i := 0; i < len(br); i += 2 {
		a, n := br[i], br[i+1]
		if a < prev || n < 0 || satAdd(a, n) > int64(size) {
			return 0, false
		}
		cost = satAdd(cost, n)
		prev = a + n
	}
	return cost, true
}

// selected is the bytes a ByteRange selects, joined; the ranges must already have passed `rangeCost`.
func selected(pdf []byte, br []int64) []byte {
	var out []byte
	for i := 0; i < len(br); i += 2 {
		out = append(out, pdf[br[i]:br[i]+br[i+1]]...)
	}
	return out
}

// digestMatches reports whether the bytes a ByteRange selects hash to the messageDigest the signed attributes carry.
// A SignerInfo that encapsulates its content (the `adbe.pkcs7.sha1` shape), carries no messageDigest, or names a digest
// this does not know cannot be screened this way and passes, so it costs a full re-verify like any candidate. The
// ranges must already have passed `rangeCost`.
func (pr *signerProof) digestMatches(pdf []byte, br []int64) bool {
	if pr == nil {
		return false
	}
	if pr.encapsulated || pr.messageDigst == nil {
		return true
	}
	h, ok := hashOf(pr.digest)
	if !ok {
		return true
	}
	if _, ok := rangeCost(br, len(pdf)); !ok {
		return false
	}
	hw := h.New()
	for i := 0; i < len(br); i += 2 {
		hw.Write(pdf[br[i] : br[i]+br[i+1]])
	}
	return bytes.Equal(hw.Sum(nil), pr.messageDigst)
}
