package docmodel

import (
	"strings"
	"unicode/utf8"
)

// Clean is the ONE door text passes through on its way to any writer (PLAN-import-export Y9): invalid UTF-8 becomes
// U+FFFD, and the characters XML 1.0 and every text format here forbid or misread — C0 controls other than tab and
// newline, DEL, the C1 block, U+FFFE and U+FFFF — are dropped. A PDF's text carries `\x00`-`\x08` more often than one
// would think. Tab and newline survive; a writer that needs one line folds them itself.
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
		if !forbidden(r) {
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
	}
	return false
}

// OneLine is Clean with every run of whitespace, newlines and tabs included, folded to a single space.
func OneLine(s string) string {
	return strings.Join(strings.Fields(Clean(s)), " ")
}
