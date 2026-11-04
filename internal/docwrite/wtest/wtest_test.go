package wtest

import "testing"

// ReadBack returns the words beyond the model's, and SameWords holds them to a pinned list: an invented word standing
// where a list label was expected has the same COUNT and not the same words (/pending 941 #21).
func TestReadBackNamesTheExtraWordsAndAnInventedOneIsNotALabel(t *testing.T) {
	want := []string{"a", "b"}
	extra, ok := ReadBack("a 1. b", want)
	if !ok || !SameWords(extra, []string{"1."}) {
		t.Errorf("a label: ok=%v extra=%q", ok, extra)
	}
	extra, ok = ReadBack("a Z b", want)
	if !ok || SameWords(extra, []string{"1."}) {
		t.Errorf("an invented word passed for the label: ok=%v extra=%q", ok, extra)
	}
	if _, ok := ReadBack("b a", want); ok {
		t.Errorf("out of order passed")
	}
	if _, ok := ReadBack("a", want); ok {
		t.Errorf("a dropped word passed")
	}
	if extra, ok := ReadBack("\ufeffa b", want); !ok || len(extra) != 0 {
		t.Errorf("a byte-order mark is not a word: ok=%v extra=%q", ok, extra)
	}
	if SameWords([]string{"x"}, []string{"x", "y"}) || SameWords([]string{"x", "z"}, []string{"x", "y"}) || !SameWords(nil, nil) {
		t.Errorf("SameWords is wrong")
	}
}
