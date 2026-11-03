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
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := OneLine(" a \t\n b\x00 "); got != "a b" {
		t.Errorf("OneLine = %q", got)
	}
}
