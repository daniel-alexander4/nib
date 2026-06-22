package server

import (
	"reflect"
	"testing"
)

func TestSplitSpans(t *testing.T) {
	// Every N pages chunks the sequence; the last chunk is short and a single-page
	// chunk is bare ("5", not "5-5").
	got, err := splitSpans("every", "2", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1-2", "3-4", "5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("every 2 of 5 = %v, want %v", got, want)
	}

	// Custom ranges: each token becomes one file; a bare page stays bare.
	got, err = splitSpans("ranges", "", "1-3, 4-8, 9", 9)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1-3", "4-8", "9"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ranges = %v, want %v", got, want)
	}

	// Malformed / out-of-range / empty / unknown-mode are all rejected.
	for _, tc := range []struct {
		mode, every, ranges string
		n                   int
	}{
		{"every", "0", "", 5},    // zero pages-per-file
		{"every", "x", "", 5},    // not a number
		{"ranges", "", "4-2", 5}, // descending
		{"ranges", "", "1-9", 5}, // past the end
		{"ranges", "", "abc", 5}, // not a number
		{"ranges", "", "", 5},    // nothing entered
		{"bogus", "", "", 5},     // unknown mode
	} {
		if _, err := splitSpans(tc.mode, tc.every, tc.ranges, tc.n); err == nil {
			t.Errorf("splitSpans(%q,%q,%q,%d) = nil error, want error", tc.mode, tc.every, tc.ranges, tc.n)
		}
	}
}
