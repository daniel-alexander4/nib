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
	Name        string      `json:"name,omitempty"` // certificate subject common name, when present
	Valid       bool        `json:"valid"`          // this signer's byte-range hash checks out
	When        string      `json:"when,omitempty"` // signing time (display string), when present
	TimeBacking TimeBacking `json:"timeBacking"`    // none / self-asserted / tsa
}

// Status is the verification result surfaced to the UI.
type Status struct {
	State   State        `json:"state"`
	Signers []SignerInfo `json:"signers,omitempty"`
}

// Verify reports whether data is unsigned, signed-and-untampered, or
// signed-and-modified, with per-signer detail. It never returns an error for
// the ordinary unsigned case — a PDF without signatures simply reports Unsigned.
func Verify(data []byte) Status {
	resp, err := verify.Verify(bytes.NewReader(data), int64(len(data)))
	if err != nil || resp == nil || len(resp.Signers) == 0 {
		// No parseable signature dictionary == unsigned for our purposes.
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
	return st
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
	si := SignerInfo{Name: s.Name, Valid: s.ValidSignature}
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
