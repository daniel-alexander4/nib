package docmodel

import (
	"strings"
	"unicode/utf8"
)

// Clean is the ONE door text passes through on its way to any writer (PLAN-import-export Y9): invalid UTF-8 becomes
// U+FFFD, and the characters XML 1.0 and every text format here forbid or misread — C0 controls other than tab and
// newline, DEL, the C1 block, U+FFFE and U+FFFF — are dropped. A PDF's text carries `\x00`-`\x08` more often than one
// would think. Three more classes go (phase-close review P02, /pending 941 #15): the bidirectional embedding, override and
// isolate controls (U+202A-U+202E, U+2066-U+2069), which reorder the text around them in whatever reads the output and
// are how "invoice_\u202Efdp.exe" is made; the byte-order mark U+FEFF, which is invisible text mid-line; and the Unicode
// line and paragraph separators U+2028 and U+2029, which become a newline (JavaScript, XML 1.1 and several viewers read
// them as a break, Markdown and XML 1.0 do not, so one meaning is chosen here). The plain direction marks U+200E, U+200F
// and U+061C are kept: they are how right-to-left text is correct. Tab and newline survive; a writer that needs one line
// folds them itself.
func Clean(s string) string {
	if s == "" {
		return s
	}
	clean := utf8.ValidString(s)
	if clean {
		for _, r := range s {
			if forbidden(r) {
				clean = false
				break
			}
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	for _, r := range s { // a range over invalid UTF-8 yields U+FFFD
		switch {
		case r == '\u2028' || r == '\u2029':
			b.WriteByte('\n')
		case !forbidden(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func forbidden(r rune) bool {
	switch {
	case r == '\t' || r == '\n':
		return false
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f, r == 0xfffe, r == 0xffff:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0xfeff, r == 0x2028, r == 0x2029:
		return true
	}
	return false
}

// OneLine is Clean with every run of whitespace, newlines and tabs included, folded to a single space.
func OneLine(s string) string {
	return strings.Join(strings.Fields(Clean(s)), " ")
}
