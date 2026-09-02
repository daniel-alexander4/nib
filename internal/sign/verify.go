// Package sign handles PDF digital-signature verification (and, from M5,
// creation). Every opened document is checked so the UI can show an
// untampered / modified / unsigned badge plus per-signer detail.
//
// Tamper-evidence is purely cryptographic: pdfsign's verifier recomputes the
// signed byte-range hash. Any edit by any tool flips ValidSignature to false.
// Whether the signer's certificate chains to a trusted CA (TrustedIssuer) is a
// separate identity question we deliberately ignore here — Nib cares about
// integrity, not third-party trust. We report every signer, not just the
// first, and how much weight each signing time carries (see TimeBacking).
package sign

import (
	"bytes"
	"encoding/hex"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/verify"
)

// State is the integrity verdict for a document.
type State string

const (
	// Unsigned: the document carries no signature. Not an error — most PDFs.
	Unsigned State = "unsigned"
	// Valid: a signature is present and the document is unmodified since signing.
	Valid State = "valid"
	// Invalid: a signature is present but the document was modified since signing
	// (or the signature itself fails to verify) — i.e. tamper-evident.
	Invalid State = "invalid"
)

// TimeBacking says how much a signer's signing time can be trusted.
type TimeBacking string

const (
	// NoTime: the signature records no signing time at all.
	NoTime TimeBacking = "none"
	// SelfAsserted: a time is present but stated by the signer — it proves
	// nothing on its own, since the signer chose the value.
	SelfAsserted TimeBacking = "self-asserted"
	// TSA: the time is fixed by an RFC3161 timestamp token from a timestamp
	// authority, independent of the signer.
	TSA TimeBacking = "tsa"
)

// SignerInfo is the per-signer detail surfaced to the UI.
type SignerInfo struct {
	Name        string      `json:"name,omitempty"`        // certificate subject common name, when present
	Valid       bool        `json:"valid"`                 // this signer's byte-range hash checks out
	When        string      `json:"when,omitempty"`        // signing time (display string), when present
	TimeBacking TimeBacking `json:"timeBacking"`           // none / self-asserted / tsa
	Reason      string      `json:"reason,omitempty"`      // signature /Reason; for co-signing, carries the attestation
	Fingerprint string      `json:"fingerprint,omitempty"` // hex SHA-256 SPKI of the signer's cert (the identity that signed)
}

// Status is the verification result surfaced to the UI.
type Status struct {
	State   State        `json:"state"`
	Signers []SignerInfo `json:"signers,omitempty"`
	// AddedAfter is true when the document carries content in a revision later
	// than its most-recent signature — added after signing, covered by no
	// signature. It does NOT make the existing signatures invalid (each still
	// proves its own content intact); it warns that the final document is not
	// wholly signed. In multi-party signing only content after the LAST
	// signature is flagged — content added between signatures is expected.
	AddedAfter bool `json:"addedAfter,omitempty"`
}

// Verify reports whether data is unsigned, signed-and-untampered, or
// signed-and-modified, with per-signer detail. It never returns an error for
// the ordinary unsigned case — a PDF without signatures simply reports Unsigned.
func Verify(data []byte) Status {
	resp, err := verify.Verify(bytes.NewReader(data), int64(len(data)))
	if err != nil || resp == nil || len(resp.Signers) == 0 {
		// Zero parseable signers covers two very different documents: one that is
		// genuinely unsigned, and one whose signature blob is present but fails to
		// parse (the library drops that signer with no top-level error). The latter
		// is tamper-evident — silently downgrading it to Unsigned would hide a
		// corrupted signature — so cross-check the PDF for an actual signature blob
		// before calling it unsigned.
		if err == nil && resp != nil && signatureBlobPresent(data) {
			return Status{State: Invalid}
		}
		return Status{State: Unsigned}
	}

	// A document is untampered only if every signer's byte-range hash checks out.
	st := Status{State: Valid}
	for i := range resp.Signers {
		si := signerInfo(&resp.Signers[i])
		if !si.Valid {
			st.State = Invalid
		}
		st.Signers = append(st.Signers, si)
	}
	// Flag content appended after the most-recent signature. Two rules, and the
	// second is the one the "two enumerations" review asked for.
	//
	// **A parse failure here must not change the integrity VERDICT** — a signature that
	// hashed correctly is still valid over its own byte range whatever the trailing check
	// does, so `State` is untouched. That was always the intent.
	//
	// **But a parse failure must not be reported as "no trailing content" either.** This
	// check and `verify.Verify` are two different enumerations of the same document — one
	// walks the xref for signature blobs, this one walks AcroForm/Fields for byte ranges —
	// and the whole worry the review raised is that the warning could go quiet
	// independently of the verdict. Discarding the error did exactly that: on a document
	// this call cannot read, `AddedAfter` silently became false and a Valid-looking result
	// claimed the document was wholly signed. It is unreachable on a Valid document *today*
	// (both calls use dpdf on the same bytes, so one cannot parse when the other cannot),
	// which is precisely why a silent discard is a trap rather than a bug: the day this
	// check grows an error path the other does not share, "clean" becomes a lie with no
	// test failing. Fail-closed — an unreadable trailing check on a signed document reports
	// AddedAfter=true, "I could not confirm the document ends at its signature", which for
	// an integrity tool is the safe direction and which both the CLI (exit non-zero) and
	// the web badge (warn) already render correctly.
	trailing, sawSig, terr := trailingContentAfterLastSignature(data)
	st.AddedAfter = addedAfterVerdict(trailing, sawSig, terr, len(st.Signers) > 0)
	return st
}

// addedAfterVerdict combines the trailing-content check's result with its error under one
// rule: content found OR the check could not run means "warn". It is a named function and
// not an inline `a || err != nil` because the fail-closed direction is the whole point of
// it — an inline expression is one careless refactor away from `a` alone, which is the
// silent-clean behaviour this replaced, and nothing would fail. This is what the test binds.
func addedAfterVerdict(trailing, sawSignature bool, err error, librarySawSigners bool) bool {
	// **The two enumerations disagreeing is itself a "could not confirm".** The library walks
	// `rdr.Xref()` for `Adobe.PPKLite` objects; this check walks AcroForm/Fields for `FT /Sig`
	// byte ranges. They are genuinely different walks over the same parsed document, so a
	// document whose AcroForm carries `/SigFlags` (which the library requires) but whose
	// `/Fields` does not list the signature satisfies one and not the other — no parse failure
	// anywhere. This check then returns "nothing trailing" because it found nothing to measure
	// against, and a Valid document is reported as wholly signed while the bytes after its
	// signature are covered by nothing.
	//
	// The old signature could not express this: it returned `(false, nil)` for "no signature
	// fields here" and for "the signatures cover everything" alike, so the caller could not tell
	// an agreement from an absence (/pending 270).
	if librarySawSigners && !sawSignature {
		return true
	}
	return trailing || err != nil
}

// HasSignatureBlob reports whether the PDF carries a signature field with contents, independently
// of whether any library can PARSE those contents.
//
// **Exported because `Verify`'s `Unsigned` is not the same question, and a caller that needs the
// stricter one was silently getting the looser (P08.S03).** `Verify` downgrades to `Unsigned`
// whenever `verify.Verify` returns an error — the cross-check below is reached only on the
// `err == nil` path — so a document whose signature blob is present but which that library cannot
// parse reads as unsigned. That is the right answer for "is there a valid signature"; it is the
// WRONG answer for "may I compare this document's content digest against a convene-time hash",
// where treating a signed document as unsigned produces a tampering accusation for a library
// divergence.
func HasSignatureBlob(pdf []byte) bool { return signatureBlobPresent(pdf) }

// signatureBlobPresent reports whether the document has an AcroForm signature
// field carrying a non-empty /Contents — i.e. a real PKCS#7 blob the verifier
// should have been able to parse. It's the discriminator between a genuinely
// unsigned document (no such field, or an empty placeholder field left by another
// tool's "prepare for signing") and one whose sole signature failed to parse. A
// parse failure here is treated as "no signature" — best-effort, never panics.
func signatureBlobPresent(pdf []byte) bool {
	r, err := dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return false
	}
	acro := r.Trailer().Key("Root").Key("AcroForm")
	if acro.IsNull() {
		return false
	}
	fields := acro.Key("Fields")
	for i := 0; i < fields.Len(); i++ {
		f := fields.Index(i)
		if f.Key("FT").Name() != "Sig" {
			continue
		}
		if len(f.Key("V").Key("Contents").RawString()) > 0 {
			return true
		}
	}
	return false
}

// trailingContentAfterLastSignature reports whether pdf has content beyond the
// coverage of its most-recent signature — a revision appended after the last
// signature, covered by none. Each signature's /ByteRange is
// [start1 len1 start2 len2]; the signed content ends at start2+len2, and a
// later (incremental) signature covers more, so the most-recent signature has
// the largest coverage end. If the file is larger than every signature's
// coverage, content was added after the last one. Unsigned docs report false.
//
// `sawSignature` reports whether THIS walk found a signature field at all, and it exists
// because "no signature here" and "the signatures cover everything" were the same return
// value — which is what let a disagreement between the two enumerations read as clean.
func trailingContentAfterLastSignature(pdf []byte) (trailing, sawSignature bool, err error) {
	r, err := dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return false, false, err
	}
	acro := r.Trailer().Key("Root").Key("AcroForm")
	if acro.IsNull() {
		return false, false, nil
	}
	fields := acro.Key("Fields")
	var maxEnd int64
	signed := false
	for i := 0; i < fields.Len(); i++ {
		f := fields.Index(i)
		if f.Key("FT").Name() != "Sig" {
			continue
		}
		br := f.Key("V").Key("ByteRange")
		if br.Len() < 4 {
			continue
		}
		signed = true
		if end := br.Index(2).Int64() + br.Index(3).Int64(); end > maxEnd {
			maxEnd = end
		}
	}
	if !signed {
		// **Not the same fact as "nothing was appended", and the caller needs both.** This
		// walk found no signature to measure against; whether one exists is the other
		// enumeration's answer.
		return false, false, nil
	}
	return int64(len(pdf)) > maxEnd, true, nil
}

// signerInfo projects a pdfsign verify.Signer onto the integrity-focused subset
// Nib surfaces. Time backing is derived from which time the signature actually
// carries — an independent RFC3161 timestamp token (TSA) versus a signer-
// supplied /M time (self-asserted) — NOT from the library's TimeSource field:
// under our default (secure) verify options that field reports "current_time"
// whenever no timestamp token is present, because the library refuses to trust
// signer-supplied time. Token presence is the honest signal.
func signerInfo(s *verify.Signer) SignerInfo {
	const layout = "2006-01-02 15:04 MST"
	si := SignerInfo{Name: s.Name, Valid: s.ValidSignature, Reason: s.Reason}
	// The signer's own cert is the leaf of the bundled chain; its SPKI fingerprint
	// is the identity that signed (see fingerprintOf). Nib identities are
	// self-signed single certs, so element 0 is the signer.
	if len(s.Certificates) > 0 && s.Certificates[0].Certificate != nil {
		si.Fingerprint = hex.EncodeToString(fingerprintOf(s.Certificates[0].Certificate))
	}
	switch {
	case s.TimeStamp != nil && !s.TimeStamp.Time.IsZero():
		si.TimeBacking = TSA
		si.When = s.TimeStamp.Time.UTC().Format(layout)
	case s.SignatureTime != nil:
		si.TimeBacking = SelfAsserted
		si.When = s.SignatureTime.UTC().Format(layout)
	default:
		si.TimeBacking = NoTime
	}
	return si
}
