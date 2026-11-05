package docmodel

import (
	"strings"
	"unicode"
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
	case IsBidiControl(r), r == 0xfeff, r == 0x2028, r == 0x2029:
		return true
	}
	return false
}

// IsBidiControl says r is one of the embedding, override and isolate controls (U+202A-U+202E, U+2066-U+2069) that Clean
// drops; it is the ONE definition, so the count the export reports (/pending 947) and the drop cannot disagree. The
// plain marks U+200E, U+200F and U+061C are not these and are kept.
func IsBidiControl(r rune) bool {
	return r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069
}

// BidiControls is how many of those controls s holds, which is how many Clean will drop from it.
func BidiControls(s string) (n int) {
	for _, r := range s {
		if IsBidiControl(r) {
			n++
		}
	}
	return n
}

// BidiControlsIn counts the controls Clean will drop from the model's text — the four fields a writer reads: a span's
// Text, a block's Marker, an image's Alt and the Meta Title (TestEveryModelStringIsCountedOrNamed holds that a string
// field added to the model is counted or named an exemption). The reader puts the sum in Report.BidiControls, so the
// export can say text-direction controls were removed instead of removing them silently.
func BidiControlsIn(d Document) (n int) {
	n = BidiControls(d.Meta.Title)
	spans := func(ss []Span) {
		for _, sp := range ss {
			n += BidiControls(sp.Text)
		}
	}
	for _, b := range d.Blocks {
		n += BidiControls(b.Marker)
		spans(b.Spans)
		for _, row := range b.Rows {
			for _, c := range row {
				for _, p := range c.Paragraphs {
					spans(p)
				}
			}
		}
		if b.Image != nil {
			n += BidiControls(b.Image.Alt)
		}
	}
	return n
}

// OneLine is Clean with every run of whitespace, newlines and tabs included, folded to a single space.
func OneLine(s string) string {
	return strings.Join(strings.Fields(Clean(s)), " ")
}

// Letters is how many right-to-left letters and how many other letters s holds: the one count a direction is read from
// (ADR-009), by the model reader for a block and by the positioned writer for a paragraph.
func Letters(s string) (rtl, ltr int) {
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Hebrew, unicode.Arabic, unicode.Syriac, unicode.Thaana, unicode.Nko):
			rtl++
		case unicode.IsLetter(r):
			ltr++
		}
	}
	return rtl, ltr
}

// DirOf is "rtl" when right-to-left letters outnumber the rest in s, else "ltr".
func DirOf(s string) string {
	if rtl, ltr := Letters(s); rtl > ltr {
		return "rtl"
	}
	return "ltr"
}
