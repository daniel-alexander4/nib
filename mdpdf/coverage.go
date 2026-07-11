package mdpdf

import (
	"strings"
	"unicode/utf8"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Unsupported reports the distinct runes in text that this package CANNOT
// print, in first-appearance order.
//
// The output uses the PDF Base-14 core fonts, whose encoding is WinAnsi
// (CP1252). pdfcpu maps any rune outside that repertoire to a SPACE rather
// than erroring — so "Nguyễn" silently renders as "Nguy n" and Cyrillic
// renders as blanks, in a valid PDF that no test can tell from a correct
// one. Callers who put user-supplied text (a patient's name, a letterhead)
// into a document must therefore check it themselves and warn a human BEFORE
// the PDF becomes a record; there is no error to catch afterwards.
//
// Latin-1 accents (é, ö, ñ, å) and the CP1252 extras (curly quotes, en/em
// dashes, €) are all printable and are NOT reported. An empty result means
// the text renders exactly as written.
func Unsupported(text string) []rune {
	var out []rune
	seen := map[rune]bool{}
	for _, r := range text {
		if printable(r) || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// printable mirrors pdfcpu's core-font encoder (model.DecodeUTF8ToByte): a
// rune survives if it fits in a byte, or if it is one of the CP1252
// specials in the 0x80–0x9F band. Anything else becomes a space, so we
// determine coverage by asking the encoder itself — no second table to drift.
func printable(r rune) bool {
	if r <= 0xFF {
		return true
	}
	// A single-rune round trip through the real encoder: it returns a space
	// for every rune it cannot represent. (A rune that IS a space is caught
	// by the r <= 0xFF branch above, so a space here always means "dropped".)
	return model.DecodeUTF8ToByte(string(r)) != " "
}

// encodedWidth measures text as pdfcpu will actually EMIT it. For core fonts
// pdfcpu measures byte-by-byte, so a multi-byte rune like "ë" is counted
// twice while only one glyph is drawn — wrap math and the x-advance of every
// following fragment on the line drift apart from the output. Measuring the
// encoded form keeps the layout honest for any text the encoder touches.
func encodedWidth(text string) string {
	if !utf8.ValidString(text) {
		return text // encoder passes invalid UTF-8 through untouched
	}
	if isASCII(text) {
		return text // the common case: no allocation, no decode
	}
	return model.DecodeUTF8ToByte(text)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// FormatRunes renders a rune set for a human-facing warning: the characters
// themselves plus their code points ("ễ (U+1EC5)").
func FormatRunes(rs []rune) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, string(r)+" (U+"+upperHex(r)+")")
	}
	return strings.Join(parts, ", ")
}

func upperHex(r rune) string {
	const digits = "0123456789ABCDEF"
	var b []byte
	for v := uint32(r); v > 0; v >>= 4 {
		b = append([]byte{digits[v&0xF]}, b...)
	}
	for len(b) < 4 {
		b = append([]byte{'0'}, b...)
	}
	return string(b)
}
