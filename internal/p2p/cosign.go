package p2p

import (
	"errors"
	"fmt"

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

// PlacementFor is the ONE door onto "where does this party's block go" (ADR-009).
//
// Outside a ceremony there is no roster and no allocated pages, and the answer is `NextPlacement`
// — blocks stacked on the readme page, exactly as the two-party co-sign has always done. Inside
// one it is `ceremonyPlacement` below. The branch lives HERE rather than at the two call sites
// (`coSignExchange` and `buildCoSigned`), because a rule that has to hold at more than one site is
// written once: those two already each carry their own `len(roster.Entries) > 0` test for other
// purposes, and a third and fourth copy deciding PLACEMENT is how they drift apart.
func PlacementFor(pdf []byte, r Roster, me string) (Placement, error) {
	if len(r.Entries) == 0 {
		return NextPlacement(pdf)
	}
	return ceremonyPlacement(pdf, r, me)
}

// ceremonyPlacement is where a ceremony party's visible block goes: on the signature page their
// ROSTER POSITION allocates, at that page's own index.
//
// **The defect it removes was named by the code that created it.** `sigpages.go` says of
// PrepareCeremonyDocument: *"It allocates pages; it does not place blocks on them. stackPlacement
// puts every block on the page it is handed, indexed by the global signer count, so a ceremony of
// nine still has block 8 off the page."* Measured: page order is
// [source…][readme][ceremony][sig 1 … sig n], `NextPlacement` targets the LAST page, so at nine
// signers every block landed on sig page 2 — signature page 1 received nothing and blocks 6, 7 and
// 8 climbed past the 842 pt box, the last of them by 50 pt, silently.
//
// **Every term is derived, so D25's six gets no second copy.** The page comes from
// `SignaturePagesFor` and the within-page index from `blocksPerPage` — the two things
// `sigpages.go` already owns. A literal 6 anywhere on this path would be the ADR-009 shape that
// `SignaturePagesFor` was exported to prevent.
//
// **The index is the party's position, not a count of signatures.** See `SigningPositionOf` for
// the three ways the count is wrong; the short version is that a party's place in the ceremony is
// a fact about the ceremony and `len(Verify(pdf).Signers)` is a fact about a file.
//
// This is a SECOND door beside NextPlacement and deliberately so: they are two rules, not two
// implementations of one. A two-party co-sign has no roster and no allocated pages, and stacks on
// the readme page as it always has. The arithmetic they share stays in stackPlacement.
func ceremonyPlacement(pdf []byte, r Roster, me string) (Placement, error) {
	pos, ok := SigningPositionOf(r, me)
	if !ok {
		return Placement{}, fmt.Errorf("%w: %s has no signing position, so there is no block for "+
			"them to occupy", ErrNotInRoster, shortFP(me))
	}
	total, err := pdfops.PageCount(pdf)
	if err != nil {
		return Placement{}, err
	}
	pages := SignaturePagesFor(len(SigningOrder(r)))
	// The signature pages are the last `pages` of the document. Refused rather than assumed: a
	// document that has lost pages since convene cannot carry the block anywhere sensible, and
	// silently placing it on the readme page is the overlap this slice exists to remove.
	if total < pages+1 {
		return Placement{}, fmt.Errorf("%w: this document has %d page(s) and a ceremony of %d "+
			"signers needs %d signature page(s) after its readme and ceremony pages",
			ErrNoSignaturePages, total, len(SigningOrder(r)), pages)
	}
	first := total - pages + 1 // 1-based page number of signature page 1
	page := first + pos/blocksPerPage
	return fitToPage(pdf, stackPlacement(page, pos%blocksPerPage))
}

// fitToPage translates a placement onto the target page's real MediaBox and refuses one that will
// not fit — by name, never a clamp.
//
// **`stackPlacement` reads no geometry, and `bottom = 40.0` is a distance from the COORDINATE
// ORIGIN rather than from the page's lower edge.** On an A4 page starting at (0,0) those are the
// same point and the distinction never surfaced. They are not the same on a page whose box is
// offset — and Nib's own split path produces exactly that, since `CutPage`'s tiles carry an offset
// MediaBox. So a block placed on a tile sits 40 points above the origin, which can be below the
// visible page entirely.
//
// **Refused rather than clamped**, because pdfcpu clamps overflow silently: P07.S08 measured that
// and it made an instrument blind to the thing it was built to see. A clamp turns *the block is
// off the page* into *the block is a different size*, and only one of those is visible to somebody
// reading the finished document — which is the whole of D25.
func fitToPage(pdf []byte, p Placement) (Placement, error) {
	llx, lly, urx, ury, err := pdfops.PageBox(pdf, p.Page)
	if err != nil {
		return Placement{}, err
	}
	// stackPlacement's rect is relative to the page's lower-left corner, which is what it always
	// meant; translating by the box's origin is what makes that true on a page where the origin is
	// not (0,0).
	r := [4]float64{p.Rect[0] + llx, p.Rect[1] + lly, p.Rect[2] + llx, p.Rect[3] + lly}
	if r[2] > urx || r[3] > ury {
		return Placement{}, fmt.Errorf("%w: the block would occupy %.0f×%.0f up to (%.0f, %.0f) on "+
			"page %d, whose box is (%.0f, %.0f)–(%.0f, %.0f)",
			ErrBlockOffThePage, r[2]-r[0], r[3]-r[1], r[2], r[3], p.Page, llx, lly, urx, ury)
	}
	p.Rect = r
	return p, nil
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
