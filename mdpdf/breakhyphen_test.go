package mdpdf

import (
	"reflect"
	"testing"
)

// hyItem is a test item: a width, and — for a broken word — its head and tail widths.
type hyItem struct {
	name       string
	w          float64
	head, tail *hyItem
	keep       bool // the line so far is the one its break ended
}

func hyBreak(items []hyItem, maxW float64) [][]string {
	lines := BreakGreedy(items, maxW, BreakOps[hyItem]{
		Width: func(i hyItem) float64 { return i.w },
		Space: func(hyItem) float64 { return 1 },
		Hyphenate: func(i hyItem) (hyItem, hyItem, bool) {
			if i.head == nil {
				return i, i, false
			}
			return *i.head, *i.tail, true
		},
		KeepBreak: func(_ []hyItem, i hyItem) bool { return i.keep },
	})
	var out [][]string
	for _, l := range lines {
		var names []string
		for _, i := range l {
			names = append(names, i.name)
		}
		out = append(out, names)
	}
	return out
}

// TestTheBreakerTakesABreakPointOnlyWhereTheWholeWordDoesNot — P08.S07: a broken word sits whole where it fits, is
// broken at its one break point where only its head fits, moves whole to the next line where not even its head fits, and
// is broken at a line's start when it is wider than a whole line; with KeepBreak, the break is taken first.
func TestTheBreakerTakesABreakPointOnlyWhereTheWholeWordDoesNot(t *testing.T) {
	head, tail := &hyItem{name: "ac-", w: 3}, &hyItem{name: "ct", w: 2}
	word := func(keep bool) hyItem { return hyItem{name: "acct", w: 4, head: head, tail: tail, keep: keep} }
	a := hyItem{name: "a", w: 4}
	for _, c := range []struct {
		name  string
		items []hyItem
		maxW  float64
		want  [][]string
	}{
		{"fits whole", []hyItem{a, word(false)}, 9, [][]string{{"a", "acct"}}},
		{"only its head fits", []hyItem{a, word(false)}, 8, [][]string{{"a", "ac-"}, {"ct"}}},
		{"not even its head", []hyItem{a, word(false)}, 7, [][]string{{"a"}, {"acct"}}},
		{"wider than a line", []hyItem{word(false)}, 3, [][]string{{"ac-"}, {"ct"}}},
		{"kept where it was", []hyItem{a, word(true)}, 9, [][]string{{"a", "ac-"}, {"ct"}}},
		{"kept only where its head fits", []hyItem{a, word(true)}, 7, [][]string{{"a"}, {"acct"}}},
	} {
		if got := hyBreak(c.items, c.maxW); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
