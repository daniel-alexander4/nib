package fontcode

import "testing"

// TestGlyphTextReadsAGlyphNameAsAdobeSaysIt — the AGL specification's rule, one case each way. The list's own entries,
// a ".suffix" dropped, "_" joining components, uniXXXX groups and uXXXX[XX]; and every shape that names nothing.
func TestGlyphTextReadsAGlyphNameAsAdobeSaysIt(t *testing.T) {
	for _, c := range []struct {
		name, want string
		ok         bool
	}{
		{"A", "A", true},
		{"fi", "ﬁ", true}, // a presentation ligature stays what the list says; a text reader expands it
		{"f_i", "fi", true},
		{"f_f_i", "ffi", true},
		{"a.alt", "a", true},
		{"uni0042", "B", true},
		{"uni00420043", "BC", true},
		{"u0042", "B", true},
		{"u1F600", "\U0001F600", true},
		{"uni004", "", false},  // not groups of four
		{"uni004a", "", false}, // lower-case hex
		{"uniD800", "", false}, // a surrogate half
		{"u110000", "", false}, // past U+10FFFF
		{"g123", "", false},    // a producer's own glyph id
		{"cid00017", "", false},
		{"", "", false},
		{".notdef", "", false}, // Adobe's rule drops the suffix; nothing is left
		{"A_g123", "", false},  // one unreadable component fails the name
	} {
		if got, ok := GlyphText(c.name); got != c.want || ok != c.ok {
			t.Errorf("GlyphText(%q) = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
	if got := GlyphList()["fi"]; got != "ﬁ" {
		t.Errorf("the list's own fi is %q", got)
	}
}
