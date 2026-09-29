package pdfread

import "testing"

// TestNeedsSeparator — the two ways a bare concatenation changes a tokenization, and the look-alikes that must
// not trigger a separator: a `%` inside a string is not a comment, and a delimiter already ends a token.
func TestNeedsSeparator(t *testing.T) {
	for _, c := range []struct {
		prev, next string
		want       bool
	}{
		{"(A) Tj", "ET", true},
		{"/ ", "F1", false},
		{"q /", "F1 12 Tf", true}, // the empty name `/` would become `/F1`
		{"q /F1", "/F2", false},
		{"q % a comment", "Q", true},
		{"q % a comment", " Q", true}, // the comment still swallows the rest of the line
		{"(A) Tj", " ET", false},
		{"(A) Tj", "% a comment\nET", false}, // `%` is a delimiter: it already ends `Tj`
		{"(A) Tj\n", "ET", false},
		{"(A) Tj\r", "ET", false},
		{"(50%)", "Tj", false},
		{"(50%) Tj", "ET", true},
		{"(50%) Tj ", "ET", false},
		{"/F1 12 Tf", "(x) Tj", false},
		{"[(a)]", "TJ", false},
		{"q", "/GS0 gs", false},
		{"", "ET", false},
		{"ET", "", false},
	} {
		if got := needsSeparator([]byte(c.prev), []byte(c.next)); got != c.want {
			t.Errorf("needsSeparator(%q, %q) = %v, want %v", c.prev, c.next, got, c.want)
		}
	}
}
