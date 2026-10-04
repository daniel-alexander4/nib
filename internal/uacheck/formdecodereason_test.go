package uacheck

import (
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestAnUndecodableFormKeepsTheFirstReason — /pending 838. doXObject's "could not be decoded" branch
// set `contentErr` without the `== ""` guard every sibling has, so a later undecodable form replaced the
// reason already recorded — and the report named the last unread stream, not the first. Two forms that
// cannot be decoded, drawn in order: the reason must name the first.
func TestAnUndecodableFormKeepsTheFirstReason(t *testing.T) {
	const content = "/XA Do /XB Do"
	bad := "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Filter /FlateDecode /Length 9 >>\nstream\nnot-flate\nendstream"
	pdf := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R " +
			"/Resources << /XObject << /XA 5 0 R /XB 6 0 R >> >> >>",
		4: "<< /Length 13 >>\nstream\n" + content + "\nendstream",
		5: bad,
		6: bad,
	})
	rep, err := Check(pdf)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Results {
		if r.Clause != "7.1 t3" {
			continue
		}
		if r.Verdict != CannotCheck || !strings.Contains(r.Why, "could not be decoded") {
			t.Fatalf("setup: 7.1 t3 = %v (%s), want a cannot-check about an undecodable form", r.Verdict, r.Why)
		}
		if !strings.Contains(r.Why, "XA") || strings.Contains(r.Why, "XB") {
			t.Errorf("the cannot-check reason names %q — a later undecodable form replaced the first one's reason", r.Why)
		}
		return
	}
	t.Fatal("setup: 7.1 t3 is not in the report")
}
