package p2p

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"nib/internal/pairing"
	"nib/internal/pdfops"
)

// Signature-page allocation (D25), and the whole of it is forced by one fact: the page
// count is inside pdfops.ContentDigest, which is ceremony.Record's DocHash, which is an
// axis of RosterHash. A slice that changes how many pages a prepared document has moves
// the digest under every record already convened — so this is decided once, here, in the
// pre-signing pass, and never again.

// CeremonyID is a ceremony's 128-bit id as this package needs it: RAW BYTES, not the hex
// string the record carries.
//
// **The type is the guard, and it replaced one.** These pages are vector text through
// pdfcpu's CreateFromJSON, which is where S08's four measured hazards live — a `%` is a
// placeholder introducer ("Acme %v Ltd" renders the version string), a WinAnsi-unencodable
// rune becomes a space with err == nil, an embedded newline positions a second run upward
// over existing text, and an over-long token is never split. A `string` parameter is a
// channel for all four, and the first draft of this door had one: measured, it accepted
// "%v not an id\nsecond line" and rendered it.
//
// A fixed-size byte array cannot express any of them — this package does the hex encoding, so
// the rendered form is 32 characters of [0-9a-f] by construction. Unrepresentable rather than
// validated, which is also why there is no second copy of internal/ceremony's idPattern here:
// p2p cannot import that package, and a duplicated predicate is the shape ADR-009 refuses.
type CeremonyID [16]byte

// String is the hex form, and it is the ONLY way these bytes reach a page.
func (c CeremonyID) String() string { return hex.EncodeToString(c[:]) }

// blocksPerPage is D25's six.
//
// **Eight fit and six was chosen, deliberately.** Measured: stackPlacement puts block i at
// y = 40 + 96i with height 84, so block 7 tops out at 796 on an 842pt A4 page and block 8
// is off it. D25 records both numbers and picks six because "six leaves room for a page
// heading and a margin that is not a rounding error" — which is why renderSignaturePage
// draws a heading, and why deleting that heading would make six arbitrary and eight
// correct. TestSixIsBelowTheGeometricCeiling holds the two facts together.
const blocksPerPage = 6

// SignaturePagesFor is signaturePages, exported so a caller can assert against the rule
// rather than restating it. A test that hard-codes the divisor is a second copy of D25.
func SignaturePagesFor(signers int) int { return signaturePages(signers) }

// signaturePages is how many dedicated signature pages a ceremony of n signers needs.
//
// **No special case at small n, and dropping the one I first wrote is the point.** The first
// draft returned 0 for n <= 2, so that a two-party ceremony would keep the appearance of
// today's two-party co-sign — blocks stacked on the readme page. That was solving a problem
// that does not exist: today's co-sign is NOT a ceremony (it has no Record), it goes through
// PrepareDocument, and this function never runs for it. A convened document is a new artifact
// with no shipped appearance to preserve, so it gets the simple rule and blocks never share a
// page with prose.
//
// What the special case would have cost: a block's page would depend on its index under two
// different rules, and S06 has to place blocks against whatever this returns.
func signaturePages(signers int) int {
	return (signers + blocksPerPage - 1) / blocksPerPage
}

// renderCeremonyPage draws the page that says what proceeding this document belongs to.
//
// # What it carries, and why it carries nothing else
//
// The ceremony id, how many parties are obliged to sign, and the convener — named by
// FINGERPRINT and by the six-word pairing name DERIVED from it. Not the recital, not any
// party's label, not any capacity.
//
// The argument for a fuller page was that a flattened or scanned bundle otherwise carries
// [NibRoster:<hash>] tokens whose preimage exists nowhere in the exhibit. **That is true and
// it is not fixable here:** RosterHash digests DocHash, and DocHash is ContentDigest of the
// page that would carry it — a fixed point. No printed page can ever make those tokens
// recomputable, so a flattened copy is unverifiable with the full roster on it and
// unverifiable without. Once verifiability is off the table this page's job is legibility and
// honest incompleteness, and that needs no free text.
//
// **The recital is deliberately absent, and this is the sharp one.** C15 requires it verbatim
// in every signature block. A block is a RASTERISED PNG, so it never passes through pdfcpu's
// text path; this page is vector text through CreateFromJSON, which is exactly where S08's
// four measured hazards live (a `%` is a placeholder introducer, WinAnsi-unencodable runes
// become spaces with err == nil, an embedded newline positions a second run upward, an
// over-long token is never split). Escaping it here would give one committed string two
// renderings that can differ, and one of them is the one C15 calls verbatim — the Party.Name
// shape. So the recital has exactly one surface, and it is S07's.
//
// Everything below is hex, an integer, or six words from a frozen ASCII wordlist (2048 words,
// a-z, <= 8 characters, no % and no backslash — checked), so no hazard is reachable.
func renderCeremonyPage(id CeremonyID, convenerFP []byte, obliged int) ([]byte, error) {
	ceremonyID := id.String()
	name := ""
	if n, err := pairing.Name(convenerFP); err == nil {
		name = n
	}
	lines := []string{
		"This document is part of a Nib signing ceremony.",
		"",
		"Ceremony: " + ceremonyID,
		fmt.Sprintf("Parties obliged to sign: %d", obliged),
		"Convened by: " + hex.EncodeToString(convenerFP),
	}
	if name != "" {
		lines = append(lines, "  which reads as: "+name)
	}
	lines = append(lines,
		"",
		"Each signature below carries a token committing to this ceremony's roster.",
		"A signature block is present for every party who has signed so far; if there",
		"are fewer blocks than the number above, this ceremony is not finished.",
	)
	text := make([]any, 0, len(lines))
	y := readmeBodyTop
	for _, ln := range lines {
		if ln != "" {
			text = append(text, map[string]any{
				"value": ln,
				"pos":   []any{readmeLeft, y},
				"font":  map[string]any{"name": "$body"},
			})
		}
		y -= readmeLeading
	}
	return renderPage(text)
}

// renderSignaturePage draws one signature page: a heading and nothing else.
//
// **The heading is load-bearing, not decoration** — see blocksPerPage. It is also forced
// twice over: pdfops.CreateFromJSON refuses a page with no content at all ("Please supply
// page \"content\""), so a truly blank page is not constructible through the door this
// package already uses.
//
// **It carries no party-supplied bytes**, and that is the property that keeps S08's four
// measured hazards off this page — a convener-typed string here would be reachable by
// WinAnsi-unencodable runes (measured: they render as spaces with err == nil), by `%`
// (pdfcpu treats it as a placeholder introducer, so "Acme %v Ltd" renders the version
// string), by an embedded newline (a second run positioned upward over existing text) and
// by a token too long for wrapText to split. The ceremony id is hex and page numbers are
// integers, so none of them is reachable.
func renderSignaturePage(id CeremonyID, page, of int) ([]byte, error) {
	ceremonyID := id.String()
	heading := fmt.Sprintf("Signatures — ceremony %s — page %d of %d", ceremonyID, page, of)
	return renderPage([]any{map[string]any{
		"value": heading,
		"pos":   []any{readmeLeft, readmeBodyTop},
		"font":  map[string]any{"name": "$body"},
	}})
}

// renderPage is the one CreateFromJSON spec these pages share, so the paper size, origin and
// font table are stated once rather than per page.
func renderPage(text []any) ([]byte, error) {
	spec := map[string]any{
		"paper":  "A4P",
		"origin": "LowerLeft",
		"fonts": map[string]any{
			"title": map[string]any{"name": readmeTitleFont, "size": readmeTitlePt},
			"body":  map[string]any{"name": readmeFont, "size": readmeFontPt},
		},
		"pages": map[string]any{
			"1": map[string]any{"content": map[string]any{"text": text}},
		},
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return pdfops.CreateFromJSON(b)
}

// PrepareCeremonyDocument readies pdf for a ceremony of `signers` signing parties: the
// trust-explainer readme, then however many signature pages D25 allocates, appended in that
// order so every later signature covers all of them.
//
// **This is the one pre-signing geometry door.** PrepareDocument is expressed through it so
// there is not a second place that decides how many pages a prepared document has — ADR-009,
// and the reason NominalBlockRect exists two files over.
//
// # Why render-and-append rather than insert
//
// Measured at P07.S02a's grill: pdfops.InsertBlank(pdf, pageCount+1) returns the document
// UNCHANGED and with no error — 3 pages in, 3 pages out — so the obvious way to allocate
// trailing pages reports success and does nothing. It inserts before a 1-based index and
// silently drops anything past the end.
//
// # What this does NOT do
//
// It allocates pages; it does not place blocks on them. stackPlacement puts every block on
// the page it is handed, indexed by the global signer count, so a ceremony of nine still has
// block 8 off the page — D25's page-box clip, which is S06's. What allocation does fix is
// the overlap half: NextPlacement targets the last page, which is now an allocated one, so
// blocks stop painting over the trust explainer.
func PrepareCeremonyDocument(pdf []byte, id CeremonyID, convenerFP []byte, signers int) ([]byte, error) {
	if signers < 1 {
		return nil, fmt.Errorf("a ceremony needs at least one signing party, not %d", signers)
	}
	out, err := PrepareDocument(pdf)
	if err != nil {
		return nil, err
	}
	cer, err := renderCeremonyPage(id, convenerFP, signers)
	if err != nil {
		return nil, fmt.Errorf("render the ceremony page: %w", err)
	}
	if out, err = pdfops.Append(out, cer); err != nil {
		return nil, fmt.Errorf("append the ceremony page: %w", err)
	}
	n := signaturePages(signers)
	for i := 1; i <= n; i++ {
		page, err := renderSignaturePage(id, i, n)
		if err != nil {
			return nil, fmt.Errorf("render signature page %d of %d: %w", i, n, err)
		}
		if out, err = pdfops.Append(out, page); err != nil {
			return nil, fmt.Errorf("append signature page %d of %d: %w", i, n, err)
		}
	}
	return out, nil
}
