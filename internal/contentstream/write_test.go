package contentstream

import (
	"strings"
	"testing"
	"time"
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

// TestAnInsertionInsideAReplacementIsREFUSED — `/pending 774`. The insertion names a byte the
// replacement removes; it used to be written after the replacement's text ("xy" here), an offset the
// caller never gave. The replacement's own two ends stay legitimate, in either queue order.
func TestAnInsertionInsideAReplacementIsREFUSED(t *testing.T) {
	src := []byte("ABCDEFGH")
	for _, queue := range []func(*Edit) *Edit{
		func(e *Edit) *Edit { return e.Replace(2, 6, []byte("x")).InsertBefore(4, []byte("y")) },
		func(e *Edit) *Edit { return e.InsertBefore(4, []byte("y")).Replace(2, 6, []byte("x")) },
	} {
		got, err := queue(NewEdit(src)).Apply()
		if err == nil {
			t.Fatalf("an insertion at 4 inside the replacement [2,6) was applied as %q — it has been "+
				"moved to an offset the caller never gave", got)
		}
		if !strings.Contains(err.Error(), "inside the replacement [2,6)") {
			t.Errorf("the error does not name the replacement the insertion fell in: %v", err)
		}
	}
	for at, want := range map[int]string{2: "ABxyGH", 6: "ABxyGH", 1: "AyBxGH", 7: "ABxGyH"} {
		got, err := NewEdit(src).Replace(2, 6, []byte("x")).InsertBefore(at, []byte("y")).Apply()
		if err != nil || string(got) != want {
			t.Errorf("an insertion at %d beside the replacement [2,6) gave %q, %v; want %q", at, got, err, want)
		}
	}
	// A replacement that has ended no longer refuses what follows it.
	got, err := NewEdit(src).Replace(0, 2, []byte("x")).Replace(4, 6, []byte("z")).InsertBefore(3, []byte("y")).Apply()
	if err != nil || string(got) != "xCyDzGH" {
		t.Errorf("an insertion between two replacements gave %q, %v; want %q", got, err, "xCyDzGH")
	}
}

// TestEditsQueuedInAnyOrderComeOutInStreamOrderAndCallOrder — the order the sort must keep, held
// against a list long and disordered enough that a sort which is not stable in call order shows it
// (`/pending 795` replaced the insertion sort).
func TestEditsQueuedInAnyOrderComeOutInStreamOrderAndCallOrder(t *testing.T) {
	const n = 500
	src := []byte(strings.Repeat(".", n))
	e := NewEdit(src)
	var want strings.Builder
	for i := n - 1; i >= 0; i-- { // back to front, three at every offset
		for _, c := range "abc" {
			e.InsertBefore(i, []byte(string(c)))
		}
	}
	for i := 0; i < n; i++ {
		want.WriteString("abc.")
	}
	got, err := e.Apply()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want.String() {
		t.Errorf("1,500 insertions queued back to front did not come out in stream order with each "+
			"offset's three in call order: starts %q", got[:24])
	}
}

// TestApplyDoesNotCostTheSquareOfWhatIsQueued — `/pending 795`, measured: 100,000 insertions queued
// back to front took about 36 s under the insertion sort and about 15 ms after it. The bound is
// two orders of magnitude above the second figure so that a loaded machine cannot reach it and the
// square cannot fit under it.
func TestApplyDoesNotCostTheSquareOfWhatIsQueued(t *testing.T) {
	const n = 100000
	e := NewEdit([]byte(strings.Repeat(".", n)))
	for i := n - 1; i >= 0; i-- {
		e.InsertBefore(i, []byte("x"))
	}
	start := time.Now()
	got, err := e.Apply()
	if err != nil || len(got) != 2*n {
		t.Fatalf("Apply gave %d bytes, %v; want %d", len(got), err, 2*n)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("applying %d insertions queued back to front took %v; the sort is quadratic again", n, d)
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
