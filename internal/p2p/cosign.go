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

// NominalBlockRect is the attestation block's rect at index 0 — the size a client
// rasterises its appearance image to, before the server computes the authoritative
// placement on the document that will actually carry it.
//
// It exists because the rule had TWO implementations. `internal/server/session.go`
// returned a hand-copied `{40, 40, 320, 124}` with a comment saying it "mirrors
// stackPlacement's constant block size", and the only thing asserting that was a
// test comparing the literal to 280x84 — the copy agreeing with itself, never with
// stackPlacement. ADR-009: a rule gets one door, and the guard checks the door.
//
// **The hand-copy's REASON was sound and is preserved.** `handleSessionQuote`
// deliberately never reads the open document, because the responder's block goes on
// the *received* document and binding to the open one would use the wrong page
// geometry. So this is a size template, not a placement — the caller wants a rect of
// the right shape and must not care where it says it is. Only the aspect is consumed
// (`web/app.js:956` reads `rect[2]-rect[0]` and `rect[3]-rect[1]` and nothing else).
func NominalBlockRect() [4]float64 { return stackPlacement(1, 0).Rect }

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
