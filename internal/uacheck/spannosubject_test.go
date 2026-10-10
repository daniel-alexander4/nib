package uacheck

import (
	"fmt"
	"testing"
)

// TestTheSpanLanguageClausesHaveNoSubjectWithoutMarkedContent — `/pending 674`.
//
// 7.2 t30, t31 and t32 run once per marked-content sequence. A catalog `/Lang` settles every one of them, so it is
// asked first — and nib answered Pass on that alone, on a document with no sequence at all, where veraPDF has no
// subject. Every row was run on veraPDF 1.30.2 and is asked again here whenever veraPDF is present. The rows with a
// sequence are the other half: a rule that answered "no subject" whenever the catalog declares a language would
// pass the first three and is what they are there to refuse.
func TestTheSpanLanguageClausesHaveNoSubjectWithoutMarkedContent(t *testing.T) {
	st := func(c string) string { return "<< /Length " + fmtInt(len(c)) + " >>\nstream\n" + c + "\nendstream" }
	doc := func(cat, content string) []byte {
		return buildPDF(map[int]string{1: "<< /Type /Catalog /Pages 2 0 R " + cat + " >>", 2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 9 0 R >> >> /Contents 4 0 R >>", 4: st(content),
			9: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"})
	}
	plain := "BT /F1 12 Tf 72 700 Td (abc) Tj ET"
	bmc := "/P BMC BT /F1 12 Tf 72 700 Td (abc) Tj ET EMC"
	span := "/Span << /ActualText (x) >> BDC BT /F1 12 Tf 72 700 Td (abc) Tj ET EMC"
	cases := []struct {
		name string
		pdf  []byte
		want Verdict // of 7.2 t30; t31 and t32 are the same but for the Span row
		vera string
	}{
		{"no marked content, a catalog /Lang", doc("/Lang (en)", plain), NotApplicable, "none"},
		{"a blank page, a catalog /Lang", doc("/Lang (en)", "0 0 1 1 re f"), NotApplicable, "none"},
		{"an empty structure tree, a catalog /Lang, no marked content", doc("/Lang (en) /StructTreeRoot << /Type /StructTreeRoot /K [] >>", plain), NotApplicable, "none"},
		{"no marked content, no /Lang", doc("", plain), NotApplicable, "none"},
		{"a sequence, a catalog /Lang", doc("/Lang (en)", bmc), Pass, "passed"},
		{"a sequence, no /Lang", doc("", bmc), Pass, "passed"},
		{"a Span with /ActualText, a catalog /Lang", doc("/Lang (en)", span), Pass, "passed"},
		{"a Span with /ActualText, no /Lang", doc("", span), Fail, "failed"},
	}
	var docs [][]byte
	for _, c := range cases {
		docs = append(docs, c.pdf)
	}
	vera := veraAsk(t, docs)
	for i, c := range cases {
		for _, clause := range []string{"7.2 t30", "7.2 t31", "7.2 t32"} {
			want, wantVera := c.want, c.vera
			if c.want == Fail && clause != "7.2 t30" { // the Span carries /ActualText only
				want, wantVera = Pass, "passed"
			}
			if got := verdictOf(t, c.pdf, clause); got.Verdict != want {
				t.Errorf("%s: %s reports %v (%s), want %v", c.name, clause, got.Verdict, got.Why, want)
			}
			if vera != nil && vera[i][clause] != wantVera {
				t.Errorf("%s: veraPDF now says %q for %s where %q was measured — the row's expectation is stale", c.name, vera[i][clause], clause, wantVera)
			}
		}
	}
}

// TestAStoppedWalkUnderACatalogLanguageStillPasses — the other half of `/pending 674`'s condition. "No subject" is
// claimed only by a walk that FINISHED: twelve forms down the walk stops having recorded no sequence, and what it did
// not read may hold one. Every sequence there would pass under a catalog `/Lang`, so the answer stays Pass — the
// reading the P04 close chose over CannotCheck — and never "the document has no marked-content sequences", which
// nib did not look far enough to say.
func TestAStoppedWalkUnderACatalogLanguageStillPasses(t *testing.T) {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X 10 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length 5 >>\nstream\n/X Do\nendstream",
	}
	const depth = 12
	for i := 0; i < depth; i++ {
		body, res := "/X Do", fmt.Sprintf("<< /XObject << /X %d 0 R >> >>", 11+i)
		if i == depth-1 {
			body, res = "/P BMC 0 0 1 1 re f EMC", "<< >>"
		}
		objs[10+i] = fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources %s /Length %d >>\nstream\n%s\nendstream", res, len(body), body)
	}
	pdf := buildPDF(objs)
	// Stimulus: the walk did stop, and recorded nothing — or the row below is about a different document.
	if got := verdictOf(t, pdf, "7.1 t1"); got.Verdict != CannotCheck {
		t.Fatalf("setup: 7.1 t1 reports %v (%s) on a sequence twelve forms down, want CannotCheck — the walk did not stop short of it", got.Verdict, got.Why)
	}
	for _, clause := range []string{"7.2 t30", "7.2 t31", "7.2 t32"} {
		if got := verdictOf(t, pdf, clause); got.Verdict != Pass {
			t.Errorf("%s reports %v (%s) where the walk stopped before the document's one sequence, want Pass", clause, got.Verdict, got.Why)
		}
	}
}
