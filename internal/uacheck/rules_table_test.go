package uacheck

import (
	"strings"
	"testing"
)

// P03.S04 — every table below was run through veraPDF 1.30.2 before it was pinned, and each verdict is
// veraPDF's. The corpus has only fail files for 7.2 t41 (whose one file fails 7.2-42 in fact) and t43.

func TestTableGeometryAgreesWithVeraPDFBothWays(t *testing.T) {
	const regular = "Document(Table(TR(TH!r2!scope=Row,TH!c2!scope=Column),TR(TD,TD)))"
	for _, tc := range []struct{ clause, fail string }{
		{"7.2 t15", "Document(Table(TR(TD,TD!r2),TR(TD!c2)))"},
		{"7.2 t41", "Document(Table(TR(TD,TD!r3),TR(TD,TD)))"},
		{"7.2 t42", "Document(Table(TR(TD,TD),TR(TD,TD,TD)))"},
		{"7.2 t43", "Document(Table(TR(TD,TD),TR(TD)))"},
		{"7.5 t1", "Document(Table(TR(TH!id=a,TH!id=b),TR(TD,TD)))"},
		{"7.5 t2", "Document(Table(TR(TH!id=a,TH!id=b),TR(TD!headers=zz,TD)))"},
	} {
		if got := verdictOf(t, treeDoc("", regular), tc.clause); got.Verdict != Pass {
			t.Errorf("%s over a regular spanned table = %v (%s), want Pass", tc.clause, got.Verdict, got.Why)
		}
		if got := verdictOf(t, treeDoc("", tc.fail), tc.clause); got.Verdict != Fail {
			t.Errorf("%s over %s = %v (%s), want Fail", tc.clause, tc.fail, got.Verdict, got.Why)
		}
	}
}

// TestTheLayoutIsVeraPDFsStepForStep — the edges `GFSETable.checkTable` decides, each measured.
func TestTheLayoutIsVeraPDFsStepForStep(t *testing.T) {
	for _, tc := range []struct {
		name, spec, clause string
		want               Verdict
	}{
		{"a row with no cells falls short at width 0", "Document(Table(TR(TD,TD),TR))", "7.2 t43", Fail},
		{"an overflow is t42, never t43", "Document(Table(TR(TD,TD),TR(TD,TD,TD)))", "7.2 t43", Pass},
		{"a short row is t43, never t42", "Document(Table(TR(TD,TD),TR(TD)))", "7.2 t42", Pass},
		{"the first error ends the layout: an intersection is not also a short row", "Document(Table(TR(TD,TD!r2),TR(TD!c2)))", "7.2 t43", Pass},
		{"a ColSpan of 0 places nothing and the row still fills", "Document(Table(TR(TD,TD),TR(TD!c0,TD,TD)))", "7.2 t42", Pass},
		{"a header matched by ID connects the cell", "Document(Table(TR(TH!id=a,TH!id=b),TR(TD!headers=a,TD!headers=b)))", "7.5 t1", Pass},
		{"a TD directly under a Column-scoped TH is connected", "Document(Table(TR(TD,TH!scope=Column),TR(TH,TD)))", "7.5 t1", Pass},
		// A Scope connects only in its own direction, measured both ways.
		{"a Column-scoped TH to the LEFT does not connect", "Document(Table(TR(TH,TH),TR(TH!scope=Column,TD)))", "7.5 t1", Fail},
		{"a Row-scoped TH ABOVE does not connect", "Document(Table(TR(TH,TH!scope=Row),TR(TH,TD)))", "7.5 t1", Fail},
		{"the data cell at [0][0] is never asked", "Document(Table(TR(TD,TH!scope=Column),TR(TH!scope=Row,TD)))", "7.5 t1", Pass},
		{"only the FIRST headerless cell is flagged, so unknown Headers after it pass", "Document(Table(TR(TH,TH),TR(TD,TD!headers=zz)))", "7.5 t2", Pass},
		// veraPDF 1.30.2 throws ArrayIndexOutOfBoundsException here: row 2 lies past the one row it counted.
		{"a table veraPDF's own layout cannot place is CannotCheck", "Document(Table(TR(TD!r0,TD),TR(TD,TD)))", "7.2 t42", CannotCheck},
		{"no Table is not applicable", "Document(P)", "7.2 t41", NotApplicable},
		{"a TD outside any Table passes", "Document(TD)", "7.5 t1", Pass},
	} {
		got := verdictOf(t, treeDoc("", tc.spec), tc.clause)
		if got.Verdict != tc.want {
			t.Errorf("%s: %s over %s = %v (%s), want %v", tc.name, tc.clause, tc.spec, got.Verdict, got.Why, tc.want)
		}
		// A rule that panics is reported as CannotCheck too, so the unlayable verdict must carry the layout's
		// own reason — found when removing the guard left this row green through a recovered nil dereference.
		if tc.want == CannotCheck && !strings.Contains(got.Why, "rows veraPDF counts") {
			t.Errorf("%s: the reason %q is not the layout's; a recovered panic reads as CannotCheck too", tc.name, got.Why)
		}
	}
}

// TestTheLayoutSurvivesWhatVeraPDFSurvives — found by P03.S04's review and measured on veraPDF 1.30.2. veraPDF
// truncates its running counts to 32 bits (Java `int += long`), so a crafted span wraps rather than sizing
// the grid; an untruncated port asked for 103 GB and the process died unrecoverably. Each verdict is veraPDF's,
// except where veraPDF itself runs out of memory, where nib's answer is CannotCheck and it returns at once.
func TestTheLayoutSurvivesWhatVeraPDFSurvives(t *testing.T) {
	for _, tc := range []struct {
		name, spec, clause string
		want               Verdict
	}{
		{"a RowSpan of 2^32+1 wraps the height to 1 and fails t41", "Document(Table(TR(TD!r4294967297,TD)))", "7.2 t41", Fail},
		{"a ColSpan of -(2^32-1) wraps the width to 1 and the next row falls short", "Document(Table(TR(TD!c-4294967295),TR(TD)))", "7.2 t43", Fail},
		// veraPDF 1.30.2: OutOfMemory, "Java heap space".
		{"a grid of two billion slots is not laid out", "Document(Table(TR(TD!c1000000000),TR(TD)))", "7.2 t42", CannotCheck},
		{"one empty-string Headers entry joins to '' — t1, not t2", "Document(Table(TR(TH,TH),TR(TD!headers=,TD)))", "7.5 t1", Fail},
		{"one empty-string Headers entry is not an unknown header", "Document(Table(TR(TH,TH),TR(TD!headers=,TD)))", "7.5 t2", Pass},
		{"an empty-name Scope still scopes the header", "Document(Table(TR(TH!scope=,TH!scope=),TR(TD,TD)))", "7.5 t1", Pass},
	} {
		if got := verdictOf(t, treeDoc("", tc.spec), tc.clause); got.Verdict != tc.want {
			t.Errorf("%s: %s = %v (%s), want %v", tc.name, tc.clause, got.Verdict, got.Why, tc.want)
		}
	}
}
