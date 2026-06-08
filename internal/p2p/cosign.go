package p2p

import (
	"errors"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// PrepareDocument readies pdf for co-signing by appending the trust-explainer
// readme as a trailing page, so every later signature covers it. The initiating
// party calls it once, before anyone signs.
//
// It refuses an already-signed document: appending the readme is a full rewrite,
// which would break any existing signature, and the readme is meant to precede
// all signatures anyway.
func PrepareDocument(pdf []byte) ([]byte, error) {
	if sign.Verify(pdf).State != sign.Unsigned {
		return nil, errors.New("document is already signed; append the readme before any signature")
	}
	return AppendReadme(pdf)
}

// Placement is where a signer's visible attestation block sits.
type Placement struct {
	Page int        // 1-based
	Rect [4]float64 // llx, lly, urx, ury in PDF points
}

// NextPlacement returns where the next signer's visible attestation block goes:
// stacked upward from the bottom of the last (readme) page, above any block
// already placed, so the two parties' blocks don't overlap. The caller renders
// the appearance image to this rect's size before signing.
func NextPlacement(pdf []byte) (Placement, error) {
	n, err := pdfops.PageCount(pdf)
	if err != nil {
		return Placement{}, err
	}
	existing := len(sign.Verify(pdf).Signers)
	return stackPlacement(n, existing), nil
}

// Contribute adds one party's acceptance attestation and approval signature to
// pdf as a single visible signature, and returns the result. Each party calls it
// in turn on the file they receive: the first on the output of PrepareDocument,
// the second on the file the first sends back. Because a visible signature adds
// its appearance in the same increment as the signature, the second party's
// contribution never disturbs the first party's signature.
//
// appearance is the rendered PNG/JPEG of the visible attestation block (sized to
// p.Rect, see Attestation.AppearanceLines); if empty the signature is invisible
// but still carries the machine-readable attestation in its signed /Reason.
func Contribute(pdf, certPEM, keyPEM []byte, att Attestation, appearance []byte, p Placement) ([]byte, error) {
	opts := sign.Options{
		Name:   att.Signer,
		Reason: att.reason(),
		When:   att.When,
	}
	if len(appearance) > 0 {
		opts.Appearance = &sign.Appearance{Image: appearance, Page: p.Page, Rect: p.Rect}
	}
	return sign.SignApproval(pdf, certPEM, keyPEM, opts)
}

// stackPlacement positions the next attestation block on the last page, stacked
// upward from the bottom margin so successive parties' blocks sit side by side in
// document space rather than overlapping.
func stackPlacement(lastPage, index int) Placement {
	const (
		x0     = 40.0
		x1     = 320.0
		bottom = 40.0
		height = 84.0
		gap    = 12.0
	)
	y0 := bottom + float64(index)*(height+gap)
	return Placement{
		Page: lastPage,
		Rect: [4]float64{x0, y0, x1, y0 + height},
	}
}
