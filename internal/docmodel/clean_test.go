package docmodel

import "testing"

func TestCleanDropsWhatNoFormatCarries(t *testing.T) {
	for in, want := range map[string]string{
		"a\x00b\x01\x08c":         "abc",
		"keep\ttab\nnl":           "keep\ttab\nnl",
		"del\x7f c1\u0085 \u009f": "del c1 ",
		"nonchar￾￿":               "nonchar",
		"bad\xffutf8":             "bad�utf8",
		"héllo שלום":              "héllo שלום",
		// /pending 941 #15: one table row per class, each separately falsifiable.
		"a\u202Eb\u202Ac\u202Dd\u202Ce\u202Bf": "abcdef",  // embeddings and overrides
		"a\u2066b\u2067c\u2068d\u2069e":        "abcde",   // isolates
		"a\ufeffb":                             "ab",      // byte-order mark mid-line
		"a\u2028b\u2029c":                      "a\nb\nc", // separators become a newline
		"a\u2028b":                             "a\nb",
		"a\u2029b":                             "a\nb",
		"a\u200eb\u200fc\u061cd":               "a\u200eb\u200fc\u061cd", // direction marks stay
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := OneLine(" a \t\n b\x00 "); got != "a b" {
		t.Errorf("OneLine = %q", got)
	}
}
