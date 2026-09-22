package uacheck

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The four named short reads of `/pending 507`, each dispositioned by what it can actually do.
//
// `reach_test.go` holds /pending 496's law — a rule never answers Pass or NotApplicable over content its
// reader did not reach. These are the reads 496's agent found and left: the role map's hop bound, the two
// dropped `/Annots` errors, the `/AP` entry that is not a stream, and `resourcesOf`'s depth bound.
//
// Two of them behave differently from the bounds 496 fixed, and the difference is the point:
//
//   - **The role map's bound was not a case for `CannotCheck` at all.** veraPDF 1.30.2 resolves a
//     thirty-hop chain exactly, so a checker that answered "could not follow it" past ten hops would
//     answer `CannotCheck` where the oracle answers a clean Pass or Fail. The chain is followed to its
//     end now and only a CYCLE is unresolvable.
//   - **`resourcesOf`'s bound is unreachable and is pinned as such below**, because pdfcpu gives the page
//     its inherited `/Resources` before any rule runs.

// roleMapDoc is a tagged one-page document carrying an H-candidate, a Figure with alternate text and a
// one-cell table, each named by a producer-private type resolved through roleMap.
func roleMapDoc(roleMap string) []byte {
	content := "/Alpha << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /Nums [0 [8 0 R]] >>",
		7:  fmt.Sprintf("<< /Type /StructTreeRoot /K [8 0 R 10 0 R 11 0 R] /ParentTree 6 0 R /RoleMap << %s >> >>", roleMap),
		8:  "<< /Type /StructElem /S /Alpha /P 7 0 R /Pg 3 0 R /K 0 >>",
		10: "<< /Type /StructElem /S /Bravo /P 7 0 R /Alt (a picture) >>",
		11: "<< /Type /StructElem /S /Charlie /P 7 0 R /K [12 0 R] >>",
		12: "<< /Type /StructElem /S /TR /P 11 0 R /K [13 0 R] >>",
		13: "<< /Type /StructElem /S /TD /P 12 0 R /A << /O /Table /Headers [(h1)] >> >>",
	}
	return buildPDF(objs)
}

// resolvedRoleMap maps the three private types straight onto standard ones.
const resolvedRoleMap = "/Alpha /H1 /Bravo /Figure /Charlie /Table"

// cyclicRoleMap sends Alpha around a loop; Bravo and Charlie still resolve, so a rule that reported only
// on the elements it COULD type would still answer.
const cyclicRoleMap = "/Alpha /Delta /Delta /Echo /Echo /Alpha /Bravo /Figure /Charlie /Table"

// TestACyclicRoleMapIsCannotCheckNeverAPass — an element whose standard type nib cannot determine is not an
// element of no interest, and the three tree rules each answered a CONFORMANT verdict over one.
func TestACyclicRoleMapIsCannotCheckNeverAPass(t *testing.T) {
	// Stimulus: with the map resolved, every rule settles — the fixture carries each rule's subject.
	for _, clause := range []string{"7.4.2 t1", "7.3 t1", "7.5 t1"} {
		if got := verdictOf(t, roleMapDoc(resolvedRoleMap), clause); got.Verdict != Pass {
			t.Fatalf("control: with the role map resolved, %s reports %v (%s), want Pass — the fixture does not carry its subject", clause, got.Verdict, got.Why)
		}
	}
	// 7.5 t1 left this list with P03.S04: it follows veraPDF's table algorithm now (`rules_table.go`), in which
	// an element on a loop has no standard type and so is never a table, a row or a cell — and the in-repo
	// oracle agrees with its verdict over the loop document. The other two keep `/pending 507`'s refusal.
	for _, clause := range []string{"7.4.2 t1", "7.3 t1"} {
		got := verdictOf(t, roleMapDoc(cyclicRoleMap), clause)
		if got.Verdict != CannotCheck {
			t.Errorf("with /Alpha role-mapped around a loop, %s reports %v (%s) over an element nib cannot type, want CannotCheck", clause, got.Verdict, got.Why)
			continue
		}
		if !strings.Contains(got.Why, "loop") {
			t.Errorf("%s: the reason %q does not say the role map loops", clause, got.Why)
		}
	}
}

// widgetRoleDoc is a document whose one widget belongs to an element named by a producer-private type.
func widgetRoleDoc(roleMap string) []byte {
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [30 0 R] >>",
		6:  "<< /Nums [1 9 0 R] >>",
		7:  fmt.Sprintf("<< /Type /StructTreeRoot /K [9 0 R] /ParentTree 6 0 R /RoleMap << %s >> >>", roleMap),
		9:  "<< /Type /StructElem /S /Foxtrot /P 7 0 R /K << /Type /OBJR /Obj 30 0 R >> >>",
		30: "<< /Type /Annot /Subtype /Widget /Rect [0 0 10 10] /StructParent 1 >>",
	}
	return buildPDF(objs)
}

func TestAWidgetWhoseElementCannotBeTypedIsCannotCheck(t *testing.T) {
	if got := verdictOf(t, widgetRoleDoc("/Foxtrot /Form"), "7.18.4 t1"); got.Verdict != Pass {
		t.Fatalf("control: /Foxtrot mapped to /Form reports %v for 7.18.4 t1 (%s), want Pass", got.Verdict, got.Why)
	}
	got := verdictOf(t, widgetRoleDoc("/Foxtrot /Golf /Golf /Foxtrot"), "7.18.4 t1")
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "loop") {
		t.Errorf("a widget whose element's role map loops reports %v for 7.18.4 t1 (%s), want CannotCheck naming the loop", got.Verdict, got.Why)
	}
}

// roleChainDoc is roleMapDoc's H-candidate alone, with /Alpha chained through n private types to final.
func roleChainDoc(n int, final string) []byte {
	rm := []string{fmt.Sprintf("/Alpha /T%d", 0)}
	for i := 0; i < n-1; i++ {
		rm = append(rm, fmt.Sprintf("/T%d /T%d", i, i+1))
	}
	rm = append(rm, fmt.Sprintf("/T%d /%s", n-1, final))
	content := "/Alpha << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /ViewerPreferences << /DisplayDocTitle true >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: "<< /Nums [0 [8 0 R]] >>",
		7: fmt.Sprintf("<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 6 0 R /RoleMap << %s >> >>", strings.Join(rm, " ")),
		8: "<< /Type /StructElem /S /Alpha /P 7 0 R /Pg 3 0 R /K 0 >>",
	}
	return buildPDF(objs)
}

// TestARoleMapChainIsFollowedToItsEndAndVeraPDFSaysSo — the half of `/pending 507` that a `CannotCheck`
// would have got WRONG.
//
// nib stopped after ten hops and returned the tenth name, so a document whose only heading is reached
// through a longer chain had no heading at all: `7.4.2 t1` answered NotApplicable — a conformant verdict —
// whether the chain ended `H3` (veraPDF fails it) or `H1` (veraPDF passes it). One was a false pass and the
// other was lost reach, and no hop bound reported honestly recovers the second.
//
// veraPDF 1.30.2 is the oracle when it is installed; the wanted verdicts below are what it answered when
// this was written, so the assertion stands on a machine without it.
func TestARoleMapChainIsFollowedToItsEndAndVeraPDFSaysSo(t *testing.T) {
	cases := []struct {
		hops  int
		final string
		want  Verdict
	}{
		{3, "H3", Fail}, {3, "H1", Pass},
		{12, "H3", Fail}, {12, "H1", Pass},
		{30, "H3", Fail}, {30, "H1", Pass},
	}
	vp := veraPDFPath()
	dir := t.TempDir()
	for _, c := range cases {
		pdf := roleChainDoc(c.hops, c.final)
		got := verdictOf(t, pdf, "7.4.2 t1")
		if got.Verdict != c.want {
			t.Errorf("a %d-hop role map chain ending /%s reports %v for 7.4.2 t1 (%s), want %v",
				c.hops, c.final, got.Verdict, got.Why, c.want)
		}
		if vp == "" {
			continue
		}
		p := fmt.Sprintf("%s/chain-%d-%s.pdf", dir, c.hops, c.final)
		if err := os.WriteFile(p, pdf, 0o644); err != nil {
			t.Fatal(err)
		}
		out, _ := exec.Command(vp, "--flavour", "ua1", "--passed", p).Output()
		vera := ""
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, `clause="7.4.2" testNumber="1"`) {
				vera = line
			}
		}
		switch {
		case vera == "":
			t.Errorf("veraPDF reported no 7.4.2 t1 result for the %d-hop chain ending /%s — the oracle saw nothing to compare", c.hops, c.final)
		case c.want == Fail && !strings.Contains(vera, `status="failed"`):
			t.Errorf("veraPDF does not fail 7.4.2 t1 on a %d-hop chain ending /%s: %s", c.hops, c.final, strings.TrimSpace(vera))
		case c.want == Pass && !strings.Contains(vera, `status="passed"`):
			t.Errorf("veraPDF does not pass 7.4.2 t1 on a %d-hop chain ending /%s: %s", c.hops, c.final, strings.TrimSpace(vera))
		}
	}
	if vp == "" {
		t.Log("SKIP (not a pass): veraPDF is absent, so the verdicts above stand on the reading recorded in " +
			"this test's comment rather than on a measurement made now.")
	}
}

// openMutated opens pdf and hands the page dictionary to mutate before any rule reads it.
//
// The three reads below cannot be reached through a file: pdfcpu's own validator refuses the document
// first, measured against v0.13.0 on every shape tried (a `/Annots` that is a name, an integer, a
// dictionary or an indirect reference to either; an `/AP` entry that is a dictionary, direct or indirect,
// on a Widget, a Square, a Link and an unknown subtype). That is a fact about the DEPENDENCY, not about
// nib, and `annots, _ :=` rested on it while saying so nowhere. So the state is made in memory, where
// nothing stands between the reader and the branch.
func openMutated(t *testing.T, pdf []byte, mutate func(d *Document, page types.Dict)) *Document {
	t.Helper()
	d, err := open(pdf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	page, _, _, perr := d.Ctx.PageDict(1, false)
	if perr != nil || page == nil {
		t.Fatalf("page 1 does not resolve: %v", perr)
	}
	mutate(d, page)
	return d
}

// annotDoc is a one-widget, one-text-run document that settles both rules below when nothing is mutated.
func annotDoc() []byte {
	content := "/P << /MCID 0 >> BDC BT /F1 12 Tf 72 700 Td (x) Tj ET EMC"
	ap := "BT /F1 12 Tf 10 10 Td (v) Tj ET"
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Annots [30 0 R] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6:  "<< /Nums [0 [8 0 R] 1 9 0 R] >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 9 0 R] /ParentTree 6 0 R >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 >>",
		9:  "<< /Type /StructElem /S /Form /P 7 0 R /K << /Type /OBJR /Obj 30 0 R >> >>",
		30: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (f) /Rect [0 0 10 10] /F 4 /StructParent 1 /AP << /N 31 0 R >> >>",
		31: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(ap), ap),
	}
	return buildPDF(objs)
}

// TestAnAnnotsThatDoesNotResolveIsNotAPageWithoutWidgets — 7.18.4 t1 counted widgets through a dropped
// error, so a page whose `/Annots` nib cannot read answered NotApplicable: "the document has no widget
// annotations".
func TestAnAnnotsThatDoesNotResolveIsNotAPageWithoutWidgets(t *testing.T) {
	check := registry["7.18.4 t1"].Check
	if got := check(openMutated(t, annotDoc(), func(*Document, types.Dict) {})); got.Verdict != Pass {
		t.Fatalf("control: unmutated, 7.18.4 t1 reports %v (%s), want Pass — the fixture does not carry its subject", got.Verdict, got.Why)
	}
	d := openMutated(t, annotDoc(), func(_ *Document, page types.Dict) {
		page["Annots"] = types.Name("NotAnArray")
	})
	got := check(d)
	if got.Verdict != CannotCheck {
		t.Fatalf("with an /Annots that does not resolve to an array, 7.18.4 t1 reports %v (%s), want CannotCheck", got.Verdict, got.Why)
	}
	if !strings.Contains(got.Why, "/Annots could not be read") {
		t.Errorf("the reason %q does not say the /Annots could not be read", got.Why)
	}
}

// TestAnUnreadAppearanceIsNotAnAnnotationWithoutOne — the appearance walk dropped three reads: the
// `/Annots` array, an `/AP` that is not a dictionary, and an `/AP` entry that is not a stream. A font used
// only inside an appearance is invisible past any of them, and 7.21.4.1 t1 is the clause that reads it.
func TestAnUnreadAppearanceIsNotAnAnnotationWithoutOne(t *testing.T) {
	check := registry["7.21.4.1 t1"].Check
	// The appearance draws the same non-embedded Helvetica the page does, so the control FAILS. What the
	// mutations must change is the verdict's kind, not its polarity: a rule that could not walk the
	// appearance must not report on it at all.
	if got := check(openMutated(t, annotDoc(), func(*Document, types.Dict) {})); got.Verdict != Fail {
		t.Fatalf("control: unmutated, 7.21.4.1 t1 reports %v (%s), want Fail — the fixture does not carry its subject", got.Verdict, got.Why)
	}
	cases := map[string]struct {
		mutate func(d *Document, page types.Dict)
		says   string
	}{
		"an /Annots that is not an array": {
			func(_ *Document, page types.Dict) { page["Annots"] = types.Name("NotAnArray") },
			"/Annots could not be read",
		},
		"an /AP that is not a dictionary": {
			func(d *Document, page types.Dict) { annotOf(d, page)["AP"] = types.Name("NotADict") },
			"an /AP that is not a dictionary",
		},
		"an /AP /N that is not a stream": {
			func(d *Document, page types.Dict) {
				d.dict(annotOf(d, page)["AP"])["N"] = types.Dict{"NotAStream": types.Boolean(true)}
			},
			"is not a stream nib can read",
		},
	}
	for name, c := range cases {
		got := check(openMutated(t, annotDoc(), c.mutate))
		if got.Verdict != CannotCheck {
			t.Errorf("%s: 7.21.4.1 t1 reports %v (%s) over an appearance nib never walked, want CannotCheck", name, got.Verdict, got.Why)
			continue
		}
		if !strings.Contains(got.Why, c.says) {
			t.Errorf("%s: the reason %q does not say %q", name, got.Why, c.says)
		}
	}
}

// annotOf returns page's first annotation dictionary, for a test to mutate in place.
func annotOf(d *Document, page types.Dict) types.Dict {
	annots, err := d.Ctx.DereferenceArray(page["Annots"])
	if err != nil || len(annots) == 0 {
		panic("annotOf: the fixture has no annotation to mutate")
	}
	return d.dict(annots[0])
}

// TestPdfcpuGivesThePageItsInheritedResources pins the measurement that OVERTURNED `resourcesOf`'s bound
// as a short read worth reporting.
//
// `/pending 507` filed the 64-level stop in `resourcesOf` as a fourth case. It is unreachable: pdfcpu's
// `ReadValidateAndOptimize` writes a page's inherited `/Resources` onto the page dictionary itself, so the
// `/Parent` climb the bound guards runs ZERO times, whatever the page tree's depth. The bound stays as the
// cycle guard it is, and this test is here so the overturn is not an assertion in a comment: if pdfcpu
// stops flattening, the climb matters again and this goes red with the reason.
func TestPdfcpuGivesThePageItsInheritedResources(t *testing.T) {
	// One page under a chain of 70 /Pages nodes, with /Resources only on the root of that chain.
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> >>",
		2: "<< /Type /Pages /Kids [100 0 R] /Count 1 /Resources << /Font << /F1 5 0 R >> >> >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	content := "BT /F1 12 Tf 72 700 Td (x) Tj ET"
	objs[4] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
	parent, depth := 2, 70
	for i := 0; i < depth; i++ {
		n := 100 + i
		kid := fmt.Sprintf("%d 0 R", n+1)
		if i == depth-1 {
			kid = "3 0 R"
		}
		objs[n] = fmt.Sprintf("<< /Type /Pages /Parent %d 0 R /Kids [%s] /Count 1 >>", parent, kid)
		parent = n
	}
	objs[3] = fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>", parent)

	d, err := open(buildPDF(objs))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	page, _, _, perr := d.Ctx.PageDict(1, false)
	if perr != nil || page == nil {
		t.Fatalf("page 1 does not resolve: %v", perr)
	}
	if d.dict(page["Resources"]) == nil {
		t.Fatalf("pdfcpu no longer puts the inherited /Resources on the page dictionary, so resourcesOf's "+
			"/Parent climb now runs for real and its %d-level bound is reachable again — /pending 507's fourth "+
			"case was overturned on this measurement and needs re-opening", maxWalkDepth)
	}
	if d.resourcesOf(page) == nil {
		t.Errorf("resourcesOf found no resources for a page %d levels down, with /Resources on the root", depth)
	}
	// And the rule that reads them settles, which is what the bound would have cost.
	if got := verdictOf(t, buildPDF(objs), "7.21.4.1 t1"); got.Verdict != Fail {
		t.Errorf("7.21.4.1 t1 reports %v (%s) for a page %d levels down drawing non-embedded Helvetica, want Fail", got.Verdict, got.Why, depth)
	}
}
