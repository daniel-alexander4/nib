package p2p

import (
	"encoding/json"
	"strings"

	"nib/internal/pdfops"
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
}

// readmeTitle and readmeParagraphs are the canonical, generic boilerplate of the
// appended trust-explainer page. Per-party specifics (who accepted whom) live in
// the visible attestation blocks, not here — the readme is added before either
// signature, when the second party's attestation does not yet exist. The body is
// built from these, so the rendered page cannot drift from this source.
const readmeTitle = "About this co-signed document"

var readmeParagraphs = []string{
	"This PDF was signed by two people using Nib, a tool that fills, signs, and verifies " +
		"PDFs entirely on each person's own computer — nothing is uploaded. Each signature is an " +
		"approval signature: it does not lock the document, but it proves that the content that " +
		"person signed has not changed since.",
	"What each signature proves: the document hasn't changed since it was signed " +
		"(tamper-evidence); it was signed by whoever holds that person's signing key (key-holder); " +
		"and, if a timestamp authority was used, when it was signed.",
	"Each signature covers the document as it stood at that moment — including that signer's own " +
		"acceptance block — but not anything added afterward. In two-party signing the second " +
		"person's acceptance block is added after the first signature; that is normal and expected. " +
		"A verifier reports anything added after the last signature separately.",
	"How the two people know who they signed with: each pinned the other's identity out of band " +
		"(for example by comparing a fingerprint in person or over a trusted channel) and recorded " +
		"that acceptance in the visible block on the preceding pages. These identities are " +
		"self-generated on each machine and are not vouched for by a certificate authority or any " +
		"third party. This is not a qualified electronic signature (QES): it proves the document is " +
		"intact and who held each key — not the legal identity of either person to a stranger.",
	"To verify: open this PDF in Nib (or another PDF signature verifier). Nib shows each signer, " +
		"whether the document is unchanged since each signature, and whether anything was added " +
		"after the last signature.",
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

// RenderReadme builds the one-page trust-explainer PDF (A4, crisp vector text):
// a bold title as a page header, then the body as one positioned text run per
// wrapped line.
func RenderReadme() ([]byte, error) {
	lines := readmeLines()
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
// Helvetica point size, using an approximate per-glyph width (exact AFM widths
// are unnecessary for boilerplate, and a small safety factor keeps lines clear of
// the right edge).
func wrapText(s string, maxW, fontPt float64) []string {
	limit := maxW * 0.97
	var lines []string
	var cur string
	for _, word := range strings.Fields(s) {
		try := word
		if cur != "" {
			try = cur + " " + word
		}
		if textWidth(try, fontPt) > limit && cur != "" {
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

// textWidth estimates the rendered width of s in points at the given Helvetica
// point size from approximate per-glyph em widths.
func textWidth(s string, fontPt float64) float64 {
	var em float64
	for _, r := range s {
		em += glyphEm(r)
	}
	return em * fontPt
}

// glyphEm returns an approximate Helvetica advance width in em units. Narrow and
// wide glyphs are bucketed; everything else takes the 0.5 average.
func glyphEm(r rune) float64 {
	switch r {
	case ' ', 'i', 'j', 'l', 't', 'f', 'r', 'I', '.', ',', ';', ':', '!', '|', '\'', '(', ')', '[', ']':
		return 0.30
	case 'm', 'w', 'M', 'W', '—':
		return 0.85
	default:
		return 0.52
	}
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
