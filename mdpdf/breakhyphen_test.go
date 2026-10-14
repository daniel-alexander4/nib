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

// TestTheBreakerKeepsALineItIsToldToKeep — KeepLine (`pdfops`' reflow, /pending 797): an item starts a new line though
// it would fit (LineApart), joins a line though it would not (LineTogether), and is placed by what fits otherwise. It
// is asked with the number of the line being filled, and never of an empty line.
func TestTheBreakerKeepsALineItIsToldToKeep(t *testing.T) {
	items := []hyItem{{name: "a", w: 4}, {name: "b", w: 4}, {name: "c", w: 4}, {name: "d", w: 4}}
	lines := func(maxW float64, keep func(line int, cur []hyItem, item hyItem) LineKeep) [][]string {
		var out [][]string
		for _, l := range BreakGreedy(items, maxW, BreakOps[hyItem]{
			Width: func(i hyItem) float64 { return i.w }, Space: func(hyItem) float64 { return 1 }, KeepLine: keep,
		}) {
			var names []string
			for _, i := range l {
				names = append(names, i.name)
			}
			out = append(out, names)
		}
		return out
	}
	for _, c := range []struct {
		name string
		maxW float64
		keep func(line int, cur []hyItem, item hyItem) LineKeep
		want [][]string
	}{
		{"nothing kept", 9, func(int, []hyItem, hyItem) LineKeep { return LineFree }, [][]string{{"a", "b"}, {"c", "d"}}},
		{"apart, though it fits", 9, func(_ int, _ []hyItem, i hyItem) LineKeep {
			if i.name == "b" {
				return LineApart
			}
			return LineFree
		}, [][]string{{"a"}, {"b", "c"}, {"d"}}},
		{"together, though it does not fit", 9, func(_ int, _ []hyItem, i hyItem) LineKeep {
			if i.name == "c" {
				return LineTogether
			}
			return LineFree
		}, [][]string{{"a", "b", "c"}, {"d"}}},
		{"by the number of the line", 9, func(line int, cur []hyItem, _ hyItem) LineKeep {
			if len(cur) == 0 {
				t.Error("asked of an empty line")
			}
			if line == 1 {
				return LineApart
			}
			return LineFree
		}, [][]string{{"a", "b"}, {"c"}, {"d"}}},
	} {
		if got := lines(c.maxW, c.keep); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
