package contentstream

import (
	"strings"
	"testing"
)

// Replace — `PLAN-accessibility.md` P06.S06's caller for the feature this package deferred.

func TestReplaceStandsInForTheOriginalSpan(t *testing.T) {
	src := []byte("/Artifact <</Subtype /Watermark>> BDC q /Fm0 Do Q EMC")
	got, err := NewEdit(src).Replace(0, 33, []byte("/Span <</MCID 0>>")).Apply()
	if err != nil {
		t.Fatal(err)
	}
	want := "/Span <</MCID 0>> BDC q /Fm0 Do Q EMC"
	if string(got) != want {
		t.Errorf("Replace produced %q, want %q", got, want)
	}
}

func TestSeveralReplacementsUseORIGINALCoordinates(t *testing.T) {
	// The property the whole package rests on: every offset is computed once, against the stream as
	// it was read, and the edits may be queued in any order.
	src := []byte("AAA BBB CCC")
	got, err := NewEdit(src).
		Replace(8, 11, []byte("zzz")).
		Replace(0, 3, []byte("xxx")).
		Apply()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "xxx BBB zzz" {
		t.Errorf("got %q, want %q — a later replacement was shifted by an earlier one, so the "+
			"offsets are not original-stream coordinates", got, "xxx BBB zzz")
	}
}

func TestAReplacementCanBeBracketedByInsertions(t *testing.T) {
	src := []byte("MIDDLE")
	got, err := NewEdit(src).
		InsertBefore(0, []byte("<")).
		Replace(0, 6, []byte("core")).
		InsertBefore(6, []byte(">")).
		Apply()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<core>" {
		t.Errorf("got %q, want %q — an insertion at a replacement's own boundary is a caller "+
			"bracketing what it replaces, which is legitimate", got, "<core>")
	}
}

func TestOverlappingReplacementsAreREFUSED(t *testing.T) {
	src := []byte("ABCDEFGH")
	_, err := NewEdit(src).
		Replace(0, 5, []byte("x")).
		Replace(3, 8, []byte("y")).
		Apply()
	if err == nil {
		t.Fatal("two replacements claiming bytes 3..5 were applied. One of them silently wins and " +
			"the stream is plausible and wrong — which is the failure this package exists not to have")
	}
	if !strings.Contains(err.Error(), "overlaps") {
		t.Errorf("the error does not name the overlap: %v", err)
	}
}

func TestAReplacementOutsideTheStreamIsRefused(t *testing.T) {
	if _, err := NewEdit([]byte("ABC")).Replace(1, 99, []byte("x")).Apply(); err == nil {
		t.Error("a replacement running past the end of the stream was applied")
	}
	if _, err := NewEdit([]byte("ABC")).Replace(2, 1, []byte("x")).Apply(); err == nil {
		t.Error("a replacement whose end precedes its start was applied")
	}
}
