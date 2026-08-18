package mdpdf

import (
	"strings"
	"testing"
	"unicode"
)

// The Base-14 core fonts encode WinAnsi (CP1252); pdfcpu SILENTLY replaces
// anything else with a space, so Unsupported is the only way a caller learns
// a name will print blank. These cases are the real-world ones: Vietnamese,
// Cyrillic, CJK, Polish, Turkish — versus the Latin-1 accents that print fine.
func TestUnsupported(t *testing.T) {
	cases := []struct {
		in   string
		want string // the runes expected to be unprintable, concatenated
	}{
		{"Jane Doe", ""},
		{"José Müller", ""},        // Latin-1: prints correctly
		{"Zoë Ångström", ""},       // ditto
		{"“smart quotes” — €", ""}, // CP1252 specials: print correctly
		{"Nguyễn", "ễ"},            // Vietnamese: the identity rune is blanked
		{"Đặng Thị", "Đặị"},        // multiple, deduped, first-appearance order
		{"Владимир", "Владимр"},    // Cyrillic; the repeated и dedupes out
		{"中村", "中村"},
		{"Wałęsa", "łę"},
		{"Şahin", "Ş"},
		{"", ""},
	}
	for _, c := range cases {
		got := string(Unsupported(c.in))
		if got != c.want {
			t.Errorf("Unsupported(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The predicate must agree with what Convert actually emits: anything
// Unsupported clears must survive a real render, and anything it reports must
// really be gone. This pins the two together so a pdfcpu upgrade that changes
// the encoder can't silently drift the warning away from the truth.
func TestUnsupportedMatchesRender(t *testing.T) {
	for _, s := range []string{"José Müller", "Nguyễn", "Владимир"} {
		pdf, err := Convert([]byte(s))
		if err != nil {
			t.Fatalf("Convert(%q): %v", s, err)
		}
		if !strings.HasPrefix(string(pdf), "%PDF-") {
			t.Fatalf("Convert(%q) is not a PDF", s)
		}
		// Every rune the predicate passed must be representable in the
		// encoded form; every rune it flagged must not be.
		for _, r := range s {
			enc := encodedWidth(string(r))
			blanked := enc == " " && r != ' '
			flagged := len(Unsupported(string(r))) > 0
			if blanked != flagged {
				t.Errorf("rune %q: encoder blanks=%v but Unsupported says %v", r, blanked, flagged)
			}
		}
	}
}

// Widths are measured on the ENCODED string: "ë" is one emitted glyph, not
// the two bytes pdfcpu's core-font metric would otherwise count.
func TestWidthMeasuresEncodedForm(t *testing.T) {
	s := style{font: fontBody, size: sizeBody}
	if got, want := s.width("Zoe"), s.width("Zoë"); got != want {
		t.Errorf("width(Zoe)=%v, width(Zoë)=%v — an accent must not inflate the measurement", got, want)
	}
	// A blanked rune measures as the space it becomes.
	if got, want := s.width("Nguy n"), s.width("Nguyễn"); got != want {
		t.Errorf("width mismatch: %v vs %v (a blanked rune must measure as its space)", got, want)
	}
}

func TestFormatRunes(t *testing.T) {
	if got, want := FormatRunes([]rune{'ễ', 'В'}), "ễ (U+1EC5), В (U+0412)"; got != want {
		t.Errorf("FormatRunes = %q, want %q", got, want)
	}
	if got := FormatRunes(nil); got != "" {
		t.Errorf("FormatRunes(nil) = %q, want empty", got)
	}
}

// UnsupportedWith reports only what NOTHING can print — the core fonts or a supplied face.
//
// The pair matters because a guard that cries wolf is worse than none. Once ConvertWithFonts
// existed, the original Unsupported would have reported every Cyrillic or CJK rune in a
// document that now prints them perfectly, and a warning nobody can act on is a warning
// people learn to skip.
func TestUnsupportedWithHonoursTheSuppliedFonts(t *testing.T) {
	const thai = "สวัสดี"

	// The premise: these runes are outside the core fonts. If that stops being true the
	// rest of this test is measuring nothing.
	if len(Unsupported(thai)) == 0 {
		t.Fatalf("setup: %q is reported as printable by the core fonts", thai)
	}

	covering := Font{Name: "X", Data: []byte{0}, Covers: []*unicode.RangeTable{unicode.Thai}}
	if got := UnsupportedWith(thai, []Font{covering}); len(got) != 0 {
		t.Errorf("text a supplied font covers is still reported unprintable: %s", FormatRunes(got))
	}

	// And a font that does NOT cover it changes nothing — the guard must not be satisfied
	// by the mere presence of a fallback.
	other := Font{Name: "Y", Data: []byte{0}, Covers: []*unicode.RangeTable{unicode.Greek}}
	if got := UnsupportedWith(thai, []Font{other}); len(got) == 0 {
		t.Error("supplying an unrelated font silenced the warning — any fallback would then look like coverage of everything")
	}
}
