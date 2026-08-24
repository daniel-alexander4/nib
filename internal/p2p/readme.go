package p2p

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"nib/internal/pdfops"
	"nib/mdpdf"
)

// trustClaims are the load-bearing honest-trust statements the co-signing readme
// must make. They are also expected verbatim in the in-app About dialog: this
// slice is the single source that keeps the two explanations from drifting, and
// a drift-guard test asserts every claim appears both in the rendered readme
// body and in the embedded About copy. Edit the wording here and in both
// surfaces together, or the test fails.
var trustClaims = []string{
	"hasn't changed since it was signed",     // tamper-evidence
	"signed by whoever holds",                // key-holder
	"vouched for by a certificate authority", // self-generated identity, no CA
	"qualified electronic signature",         // explicitly not QES
	// The two N-party claims (P07.S08). Before them every entry above was a
	// SINGLE-signature claim, which the About dialog satisfied incidentally while
	// containing no co-signing copy at all — so "the About dialog says the same
	// thing and the guard is green" was dischargeable by an About that never
	// mentioned co-signing. These are the entries that make the guard bite on
	// what C08 is actually about.
	"two or more people",                     // the ceremony is N, not 2
	"have not checked each other's identity", // the hub's limitation, stated
}

// readmeTitle and readmeParagraphs are the canonical, generic boilerplate of the
// appended trust-explainer page. Per-party specifics (who accepted whom) live in
// the visible attestation blocks, not here — the readme is added before ANY
// signature, when no attestation exists yet. The body is built from these, so the
// rendered page cannot drift from this source.
//
// **It states what the DOCUMENT RECORDS, never what the people DID off the page**
// (P07.S08). The sentence this replaced said each signer "pinned the other's
// identity out of band … by comparing a fingerprint in person", which is a claim
// about human conduct that no code on either exchange route can observe: the live
// session does run a fail-closed spoken check (verify.go's runVerification), but
// the manual pass-the-file route has no channel and runs none, and neither knows
// whether anyone met. A recital of what the parties did, drafted by neither party
// and sealed inside their signatures, is the one kind a document does not recover
// from — so the page asserts only facts about itself.
//
// It is also true at N=2 and at N=9, deliberately. This slice ships seven slices
// before the N-party model does, so every document produced in between is an
// ordinary two-party co-sign carrying NO ceremony record at all — and prose
// written as though a convener and a roster existed would be false on every one of
// them, permanently, because PrepareDocument refuses an already-signed document.
const readmeTitle = "About this co-signed document"

var readmeParagraphs = []string{
	"This PDF was signed by two or more people using Nib, a tool that fills, signs, and verifies " +
		"PDFs entirely on each person's own computer — nothing is uploaded. Each signature is an " +
		"approval signature: it does not lock the document, but it proves that the content that " +
		"person signed has not changed since.",
	"What each signature proves: the document hasn't changed since it was signed " +
		"(tamper-evidence); it was signed by whoever holds that person's signing key (key-holder); " +
		"and, if a timestamp authority was used, when it was signed.",
	"Each signature covers the document as it stood at that moment — including that signer's own " +
		"acceptance block — but not anything added afterward. Each later acceptance block is added " +
		"after the signature before it; that is normal and expected. A verifier reports anything " +
		"added after the last signature separately.",
	"Who each signer accepted: every signature names one party, in the acceptance block on the " +
		"preceding pages, and that is the only identity that signature vouches for. Where two parties " +
		"sign together they accept each other. Where more than two do, one of them convenes the " +
		"proceeding and tells the rest who is on it — so parties who never connected to each other " +
		"have not checked each other's identity, and no signature here claims they did.",
	"These identities are self-generated on each machine and are not vouched for by a certificate " +
		"authority or any third party. This is not a qualified electronic signature (QES): it proves " +
		"the document is intact and who held each key — not the legal identity of any signer to a " +
		"stranger.",
	"To verify: open this PDF in Nib (or another PDF signature verifier). Nib shows each signer, " +
		"whether the document is unchanged since each signature, and whether anything was added " +
		"after the last signature.",
	"What is authoritative is the signature itself — who each party accepts is recorded inside their " +
		"signature and shown in the verifier's signature details. The acceptance block printed on the " +
		"page above is a human-readable convenience; trust the signature details, not the printed block.",
}

// readmeBody is the full text of the page, paragraphs separated by a blank line.
// Exposed (package-internal) so the drift-guard test can assert the trust claims
// against the same text that is rendered.
func readmeBody() string {
	return strings.Join(readmeParagraphs, "\n\n")
}

// Readme page layout constants (A4 portrait, lower-left origin).
const (
	readmePageW   = 595.0
	readmePageH   = 842.0
	readmeLeft    = 62.0
	readmeRight   = 62.0
	readmeBodyTop = 735.0 // baseline of the first body line
	readmeLeading = 14.0
	readmeFontPt  = 11.0
	readmeFont    = "Helvetica"
)

// readmeLines word-wraps the body into individual rendered lines, with one blank
// line between paragraphs. Each line is drawn as its own positioned text run
// rather than relying on pdfcpu's multi-line text box, whose wrapping proved
// unreliable; per-line placement renders correctly across viewers.
func readmeLines() []string {
	maxW := readmePageW - readmeLeft - readmeRight
	var out []string
	for i, para := range readmeParagraphs {
		if i > 0 {
			out = append(out, "") // blank line between paragraphs
		}
		out = append(out, wrapText(para, maxW, readmeFontPt)...)
	}
	return out
}

// ErrReadmeOverflow reports a body that will not fit above the signature blocks
// stacked on the same page.
var ErrReadmeOverflow = errors.New("the trust-explainer body does not fit above the signature blocks")

// readmeFloor is the lowest baseline a body line may occupy: the TOP of the
// signature-block stack this page will carry.
//
// Derived from stackPlacement rather than written as a literal, because the defect
// is precisely that readmeBodyTop/readmeLeading here and bottom/height/gap there
// were independent numbers with nothing relating them. A literal would restate one
// of them and drift (ADR-009: a rule gets one door).
//
// **It is pinned at TWO signers deliberately, and that is narrower than the
// defect.** D25 measured that block 3 — the FOURTH signature — already spans
// y 328..412 and paints over five lines of this page on the unmodified tree, and
// that block 8 is off an A4 page entirely. Fixing that is P07.S06, which places
// blocks on the pages P07.S02 allocates. What this floor prevents is the
// regression THIS slice could introduce: growing the body until it collides in the
// two-party co-sign, which is the only ceremony the product can currently run.
// S06 re-points this at the allocated pages and the pin goes away.
func readmeFloor() float64 {
	return stackPlacement(1, 1).Rect[3]
}

// readmeLastBaseline is the y of the last line RenderReadme will draw.
//
// The COMPUTED y, not the rendered one, and the distinction is the whole reason
// this guard exists. Measured: pdfcpu CLAMPS the position it emits — a requested y
// of -50 and of -5000 both land at 421.0, A4's vertical centre — so an overflowing
// page renders its surplus lines stacked on ONE baseline as an illegible smear
// while RenderReadme returns nil and PageCount stays 1. Reading the rendered
// position therefore saturates: forty overflow lines and four hundred are
// indistinguishable. Reading the extracted TEXT is blind too, because every line is
// still in the content stream. The computed value is the only instrument with reach.
func readmeLastBaseline(lines []string) float64 {
	return readmeBodyTop - readmeLeading*float64(len(lines)-1)
}

// RenderReadme builds the one-page trust-explainer PDF (A4, crisp vector text):
// a bold title as a page header, then the body as one positioned text run per
// wrapped line.
func RenderReadme() ([]byte, error) {
	lines := readmeLines()
	// Refuse here rather than letting a later assertion notice: this is the one
	// door that produces the page, so a caller cannot ship an unreadable one.
	if last := readmeLastBaseline(lines); last <= readmeFloor() {
		return nil, fmt.Errorf("%w: %d lines put the last baseline at %.0f and the block stack starts at %.0f — remove %d line(s)",
			ErrReadmeOverflow, len(lines), last, readmeFloor(),
			int((readmeFloor()-last)/readmeLeading)+1)
	}
	text := make([]any, 0, len(lines)+1)
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
	spec := map[string]any{
		"paper":  "A4P",
		"origin": "LowerLeft",
		"fonts": map[string]any{
			"title": map[string]any{"name": "Helvetica-Bold", "size": 16},
			"body":  map[string]any{"name": "Helvetica", "size": 11},
		},
		"header": map[string]any{
			"font":   map[string]any{"name": "$title"},
			"left":   readmeTitle,
			"height": 30,
			"dx":     int(readmeLeft),
			"dy":     30,
			"border": false,
		},
		"pages": map[string]any{
			"1": map[string]any{
				"content": map[string]any{"text": text},
			},
		},
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return pdfops.CreateFromJSON(b)
}

// wrapText greedily wraps s into lines no wider than maxW points at the given
// Helvetica point size.
//
// The width comes from mdpdf.CoreWidth — the real Base-14 metrics, measured the
// way pdfcpu emits them. It replaced a hand-rolled three-bucket estimate
// (0.30/0.52/0.85 em) carrying a 3% safety factor, which was measured to
// under-count the shipped text by up to 3.29% — a margin thinner than its own
// error, so today's page fitting the column was luck rather than design. Capitals
// were the worst case at ~14% under, because every capital but M and W took the
// 0.52 default against a real ~0.72.
//
// **Fixed HERE rather than later, and the reason is the digest.** Changing how
// text is measured moves every line break, which moves the rendered page, which
// moves pdfops.ContentDigest — and that is ceremony.Record's DocHash, which is an
// axis of RosterHash. Today no production code authors a record, so the change is
// free; from P07.S02 it is unpayable. Same argument that moved this slice ahead of
// S02 for the prose, applied to the metrics.
//
// One token longer than the column is still emitted alone rather than split: that
// is a deliberate limit, and readmeOverflow does not catch it because it is a
// horizontal defect. TestEveryReadmeLineFitsTheColumn is what does.
func wrapText(s string, maxW, fontPt float64) []string {
	var lines []string
	var cur string
	for _, word := range strings.Fields(s) {
		try := word
		if cur != "" {
			try = cur + " " + word
		}
		if mdpdf.CoreWidth(try, readmeFont, int(fontPt)) > maxW && cur != "" {
			lines = append(lines, cur)
			cur = word
		} else {
			cur = try
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// AppendReadme renders the trust-explainer page and appends it to pdf as a
// trailing page. It must be called before any signature so every signature
// covers the readme.
func AppendReadme(pdf []byte) ([]byte, error) {
	readme, err := RenderReadme()
	if err != nil {
		return nil, err
	}
	return pdfops.Append(pdf, readme)
}
