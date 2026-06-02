// Package sign handles PDF digital-signature verification (and, from M5,
// creation). For M1 only verification is wired up: every opened document is
// checked so the UI can show an untampered / modified / unsigned badge.
//
// Tamper-evidence is purely cryptographic: pdfsign's verifier recomputes the
// signed byte-range hash. Any edit by any tool flips ValidSignature to false.
// Whether the signer's certificate chains to a trusted CA (TrustedIssuer) is a
// separate identity question we deliberately ignore here — Nib cares about
// integrity, not third-party trust.
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

// Status is the verification result surfaced to the UI.
type Status struct {
	State  State  `json:"state"`
	Signer string `json:"signer,omitempty"` // certificate subject common name, when present
	When   string `json:"when,omitempty"`   // signing time (RFC3339), when present
}

// Verify reports whether data is unsigned, signed-and-untampered, or
// signed-and-modified. It never returns an error for the ordinary unsigned
// case — a PDF without signatures simply reports Unsigned.
func Verify(data []byte) Status {
	resp, err := verify.Verify(bytes.NewReader(data), int64(len(data)))
	if err != nil || resp == nil || len(resp.Signers) == 0 {
		// No parseable signature dictionary == unsigned for our purposes.
		return Status{State: Unsigned}
	}

	// A document is untampered only if every signer's byte-range hash checks out.
	st := Status{State: Valid}
	for i := range resp.Signers {
		s := &resp.Signers[i]
		if !s.ValidSignature {
			st.State = Invalid
		}
		if st.Signer == "" {
			st.Signer = s.Name
		}
		if st.When == "" && s.SignatureTime != nil {
			st.When = s.SignatureTime.UTC().Format("2006-01-02 15:04 MST")
		}
	}
	return st
}
