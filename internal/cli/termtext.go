package cli

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// termText is the ONE door for text the user did not write on its way to a terminal (/pending 727).
//
// A PDF's strings are the author's: a bookmark title, an attachment's name, alt text, a ceremony
// party's label and a tag's role name all decode `#1B` or `\033` to a real escape byte, and printed
// raw that byte is a terminal control sequence — it can recolour, retitle or clear the window, or
// rewrite the line above so `nib verify` appears to say something it did not. A newline in a title
// forges a whole extra output line, so newlines and tabs are escaped too, not only the C0 block.
//
// Every control rune (C0, DEL, C1), every bidirectional formatting rune (U+202A–U+202E,
// U+2066–U+2069, U+200E/F, U+061C — they reorder what the reader sees without changing what was
// printed) and every byte that is not valid UTF-8 is written as a visible escape (`\x1b`,
// `‮`). Everything else, accents and symbols included, passes through unchanged: unlike `%q`
// it does not quote, so ordinary titles read as they always have.
//
// `errf` routes every message through it, so an error that wraps document text cannot reach the
// terminal raw either; stdout sites call it on each document-derived field.
// `TestEveryDocumentStringReachesTheTerminalEscaped` holds the commands to it.
func termText(s string) string {
	if !needsTermEscape(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r < 0x80 && unsafeTermRune(r):
			fmt.Fprintf(&b, `\x%02x`, r)
		case unsafeTermRune(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

func needsTermEscape(s string) bool {
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && n == 1) || unsafeTermRune(r) {
			return true
		}
		i += n
	}
	return false
}

func unsafeTermRune(r rune) bool {
	if unicode.IsControl(r) {
		return true
	}
	switch {
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0x200E, r == 0x200F, r == 0x061C:
		return true
	}
	return false
}
