package uacheck

import (
	"fmt"
	"strings"
	"testing"
)

// The checker reads a divided `/Contents` the way veraPDF does — at token boundaries — /pending 719.
//
// **Measured against veraPDF 1.x (`~/verapdf`, `--flavour ua1`) on 2026-10-03**, with the fixtures this test builds:
//
//	single stream "…EMC 0 0 1 1 ref"        7.1 t3 passes  (`ref` is one unknown operator: nothing is painted)
//	single stream "…EMC 0 0 1 1 re f"       7.1 t3 FAILS   (content[1]/contentItem[0], the untagged rectangle)
//	array ["…EMC 0 0 1 1 re", "f"]          7.1 t3 FAILS   (content[1]/contentItem[0] — the same finding as the line above)
//	array ["…EMC % c", "0 0 1 1 re f"]      7.1 t3 FAILS   (the comment does not swallow the second stream's first line)
//	array ["…EMC", "q 0 0 1 1 re f Q"]      7.1 t3 FAILS   (one finding — the single-stream `EMCq` fails TWO, an unclosed sequence)
//
// So veraPDF separates the streams of an array, and pdfcpu's bare join — which the checker read until this test —
// fused the first two shapes into a page that draws nothing untagged: a false PASS against the oracle. The single-
// stream controls pin that the difference is the join and not the operators.
func TestTheCheckerJoinsADividedPageAsVeraPDFDoes(t *testing.T) {
	const artifact = "/Artifact BMC 0 0 5 5 re f EMC"
	page := func(parts ...string) []byte {
		stream := func(body string) string {
			return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body)
		}
		refs := make([]string, len(parts))
		objs := map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 4 0 R /Lang (en) >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			4: "<< /Type /StructTreeRoot >>",
		}
		for i, p := range parts {
			objs[5+i] = stream(p)
			refs[i] = fmt.Sprintf("%d 0 R", 5+i)
		}
		contents := refs[0]
		if len(parts) > 1 {
			contents = "[" + strings.Join(refs, " ") + "]"
		}
		objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents " + contents + " >>"
		return buildPDF(objs)
	}
	for _, c := range []struct {
		name  string
		pdf   []byte
		wantV Verdict
	}{
		{"control: one stream, fused", page(artifact + " 0 0 1 1 ref"), Pass},
		{"control: one stream, separated", page(artifact + " 0 0 1 1 re f"), Fail},
		{"an array divided between two regular tokens", page(artifact+" 0 0 1 1 re", "f"), Fail},
		{"an array whose first stream ends in a comment", page(artifact+" % c", "0 0 1 1 re f"), Fail},
	} {
		if got := verdictOf(t, c.pdf, "7.1 t3"); got.Verdict != c.wantV {
			t.Errorf("%s: 7.1 t3 is %v (%s), want %v — veraPDF's verdict on these bytes", c.name, got.Verdict, got.Why, c.wantV)
		}
	}
}
