package sign

import (
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/digitorus/pdfsign/sign"
)

// libraryText is the one door a signer's text passes on its way into the signing library's
// signature dictionary (ADR-163, /pending 879): `/Name`, `/Reason`, `/Location`, `/ContactInfo`.
//
// # Why
//
// The library writes a text that is not all ASCII as the raw bytes of its UTF-16BE encoding between
// parentheses, escaping nothing (`pdfsign sign/helpers.go:39-48`, `pdfString`, the `!isASCII` arm;
// its ASCII arm does escape). A PDF literal string gives four of those bytes a meaning: `(` and `)`
// nest and close it, `\` starts an escape, and a carriage return is to be read as a line feed
// (ISO 32000-1 7.3.4.2). So a character with one of them in EITHER byte of a UTF-16 unit wrote a
// string that is not the text. Measured before this door, through `Sign`, `SignApproval` and
// `SignExternal` alike, as the name and as the reason:
//
//   - 山 (U+5C71) is `\q`: refused by nib's read-back, `invalid escape sequence \q`;
//   - ب (U+0628) opens a parenthesis nothing closes: refused, `EOF`;
//   - ة (U+0629) closes the string: refused, `unexpected delimiter ')'`;
//   - an accented Latin text with a bracket or a backslash — `Müller :-)` — the same, because `)`
//     is then 0x00 0x29;
//   - 就 (U+5C31) is `\1`, an octal escape: **SIGNED**, with a `/Reason` nobody typed, which nib and
//     pdfcpu both read back as nine bytes of noise. That is the harm; the refusals were the luck.
//
// 1,503 of the 63,488 BMP characters hold a `(`, `)` or `\` byte and 497 more a carriage return
// (2,000), and 32,512 of the supplementary ones in a surrogate half. The carriage return is the
// standard's rule and not a measured failure: nib's reader and pdfcpu's both leave it as it is.
//
// # How, with the library unchanged (ADR-142 holds its patch to the verify path)
//
// The library escapes nothing, so nib hands it a text that is ALREADY the escaped string: every byte
// of the true UTF-16BE encoding written as a three-digit octal escape, `\ddd`. Those four ASCII bytes
// are themselves two UTF-16 units — U+5C30..U+5C33 (the backslash and the first digit) and
// U+3030..U+3737 (the other two) — so the escape is carried as two characters the library encodes
// back to exactly `\ddd`. What it writes is `(` BOM `\ddd\ddd…` `)`: a conforming literal string in
// which every byte but the BOM is a backslash or a digit 0-7, and whose value is the BOM and the
// text. The BOM is the library's own (0xFE 0xFF, neither of them one of the four bytes).
//
// Every non-ASCII text goes this way, not only one holding an affected character: one shape, taken
// by every signature in any script but ASCII, so the common case exercises what the rare one needs.
// It costs eight bytes of file per UTF-16 unit. An all-ASCII text is returned as it came — the
// library's ASCII arm escapes it correctly — and so is an empty one, which the library omits.
//
// # What holds it
//
// This rests on the library NOT escaping. If a later version does, every text here would be escaped
// twice and read back as CJK noise. `signedAsIntended` reads the new signature's `/Name` and
// `/Reason` back and refuses anything but what was asked, so that is a refused signing and never a
// wrong one; `TestTextInAnyScriptIsSignedAsTyped` fails at the bump.
func libraryText(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	var b strings.Builder
	for _, u := range utf16.Encode([]rune(s)) {
		for _, c := range [2]byte{byte(u >> 8), byte(u)} {
			b.WriteRune(rune('\\')<<8 | rune('0'+c>>6))
			b.WriteRune(rune('0'+c>>3&7)<<8 | rune('0'+c&7))
		}
	}
	return b.String()
}

// throughLibraryText returns info with each text the library writes into the signature dictionary
// passed through `libraryText`. The date is the library's own ASCII.
//
// `Info.Name` is also what the library DRAWS when a visible signature has no image
// (`sign/appearance.go`, `shouldDisplayText`). nib never asks for that — `SignApproval` sets an
// appearance only with an image, and since ADR-158 the block is nib's own drawing — and a text drawn
// there would be the escapes, not the name.
func throughLibraryText(info sign.SignDataSignatureInfo) sign.SignDataSignatureInfo {
	info.Name = libraryText(info.Name)
	info.Reason = libraryText(info.Reason)
	info.Location = libraryText(info.Location)
	info.ContactInfo = libraryText(info.ContactInfo)
	return info
}

// textAsAsked is `signedAsIntended`'s last conjunct (ADR-163): the record of the signature nib just
// made carries the `/Name` and `/Reason` nib asked the library to write, as the sweep read them
// through the reader `Verify` reports from (`Value.Text`). They are inside the signature's coverage,
// so a signature whose text is not the text asked for is a signature over something nobody typed.
//
// Only the two fields nib sets and reports; `/Location` and `/ContactInfo` go through `libraryText`
// and nothing in nib fills them.
func textAsAsked(added *Revision, asked sign.SignDataSignatureInfo) error {
	if added.name != asked.Name {
		return fmt.Errorf("the signature's name reads back as %q, not %q", added.name, asked.Name)
	}
	if added.reason != asked.Reason {
		return fmt.Errorf("the signature's reason reads back as %q, not %q", added.reason, asked.Reason)
	}
	return nil
}
