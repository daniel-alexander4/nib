package uacheck

import (
	"fmt"
	"strings"
	"testing"
)

// The numbered-heading rule — `/pending 487`. Each tree is built directly, so the verdict can only come from
// the heading sequence: no content, no language, nothing else a rule could read.

// headingTree builds a tagged one-page document whose root holds one element per type in kinds, in order.
// A kind of the form "Name=H2" writes /Name and role-maps it to H2.
func headingTree(kinds ...string) []byte {
	objs := map[int]string{
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
	}
	var kids, roleMap []string
	for i, k := range kinds {
		n := 10 + i
		s := k
		if name, mapped, ok := strings.Cut(k, "="); ok {
			s = name
			roleMap = append(roleMap, "/"+name+" /"+mapped)
		}
		objs[n] = fmt.Sprintf("<< /Type /StructElem /S /%s /P 7 0 R /Pg 3 0 R >>", s)
		kids = append(kids, fmt.Sprintf("%d 0 R", n))
	}
	root := "<< /Type /StructTreeRoot /K [" + strings.Join(kids, " ") + "]"
	if len(roleMap) > 0 {
		root += " /RoleMap << " + strings.Join(roleMap, " ") + " >>"
	}
	objs[7] = root + " >>"
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>"
	return buildPDF(objs)
}

func TestNumberedHeadingsStartAtH1AndNeverSkipALevel(t *testing.T) {
	for _, c := range []struct {
		name  string
		kinds []string
		want  Verdict
		why   string
	}{
		{"no headings at all", []string{"P", "P"}, NotApplicable, ""},
		{"H1 → H2 → H3", []string{"H1", "P", "H2", "H3"}, Pass, ""},
		{"repeated H1", []string{"H1", "H1", "H1"}, Pass, ""},
		{"a return to a shallower level", []string{"H1", "H2", "H3", "H4", "H2", "H3", "H1"}, Pass, ""},
		{"the first heading is H2", []string{"H2", "H3"}, Fail, "first heading is H2"},
		{"H2 then H4", []string{"H1", "H2", "H4"}, Fail, "H4 follows H2"},
		{"a role-mapped skip", []string{"H1", "Deep=H3"}, Fail, "H3 follows H1"},
		{"a role-mapped nest", []string{"Title=H1", "H2"}, Pass, ""},
	} {
		got := verdictOf(t, headingTree(c.kinds...), "7.4.2 t1")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.4.2 t1 reports %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
			continue
		}
		if c.why != "" && !strings.Contains(got.Why, c.why) {
			t.Errorf("%s: the reason %q does not say %q", c.name, got.Why, c.why)
		}
	}
}

// headingTreeMapped is headingTree with the role map written whole, so a loop can name types no element has.
func headingTreeMapped(roleMap string, kinds ...string) []byte {
	objs := map[int]string{
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
	}
	var kids []string
	for i, k := range kinds {
		objs[10+i] = fmt.Sprintf("<< /Type /StructElem /S /%s /P 7 0 R /Pg 3 0 R >>", k)
		kids = append(kids, fmt.Sprintf("%d 0 R", 10+i))
	}
	objs[7] = "<< /Type /StructTreeRoot /K [" + strings.Join(kids, " ") + "] /RoleMap << " + roleMap + " >> >>"
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>"
	return buildPDF(objs)
}

// headingTreeWith is headingTree with each root kid written as given — a reference to one of the elements below, or
// an inline element.
func headingTreeWith(kids ...string) []byte {
	return buildPDF(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		7:  "<< /Type /StructTreeRoot /K [" + strings.Join(kids, " ") + "] >>",
		10: "<< /Type /StructElem /S /H2 /P 7 0 R /Pg 3 0 R >>",
		11: "<< /Type /StructElem /S /H1 /P 7 0 R /Pg 3 0 R >>",
		12: "<< /Type /StructElem /S /H3 /P 7 0 R /Pg 3 0 R >>",
		13: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R >>",
	})
}

// TestAHeadingFailureBeforeTheCutIsDefinite — /pending 692. 7.4.2 t1 refused whenever the structure walk was cut,
// before looking at what it HAD read; every other rule holds "a definite failure beats a refusal" (`heldRefusal`).
// For this ORDER-dependent rule a failure is definite exactly when every element before it was read — inside the
// walk's prefix — and a failure after a skipped subtree is not, because the subtree may hold the heading that
// changes it. Both of the walk's cuts are driven: the depth bound (a Div chain past `maxWalkDepth`) and the
// `/K`-entry budget.
func TestAHeadingFailureBeforeTheCutIsDefinite(t *testing.T) {
	deep := "[]"
	for i := 0; i < maxWalkDepth+3; i++ {
		deep = "<< /S /Div /K " + deep + " >>"
	}
	for _, c := range []struct {
		name string
		kids []string
		want Verdict
	}{
		{"the first heading is H2, then a subtree too deep to read", []string{"10 0 R", deep}, Fail},
		{"H1 then H3, then a subtree too deep to read", []string{"11 0 R", "12 0 R", deep}, Fail},
		// The controls: the same failures AFTER the cut are not definite — the skipped subtree may hold an H1 or H2.
		{"a subtree too deep to read, then H2", []string{deep, "10 0 R"}, CannotCheck},
		{"H1, a subtree too deep to read, then H3", []string{"11 0 R", deep, "12 0 R"}, CannotCheck},
		{"H1 then a subtree too deep to read", []string{"11 0 R", deep}, CannotCheck},
	} {
		got := verdictOf(t, headingTreeWith(c.kids...), "7.4.2 t1")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.4.2 t1 = %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
		if c.want == CannotCheck && !strings.Contains(got.Why, "nests deeper") {
			t.Errorf("%s: the refusal %q is not the depth cut this fixture supplies", c.name, got.Why)
		}
	}

	// The `/K`-entry budget: the walk reads nothing past it.
	saved := maxStructEntries
	t.Cleanup(func() { maxStructEntries = saved })
	maxStructEntries = 2
	for _, c := range []struct {
		name string
		kids []string
		want Verdict
	}{
		{"H2 inside the budget", []string{"10 0 R", "13 0 R", "13 0 R", "13 0 R"}, Fail},
		{"H2 past the budget", []string{"13 0 R", "13 0 R", "13 0 R", "10 0 R"}, CannotCheck},
	} {
		got := verdictOf(t, headingTreeWith(c.kids...), "7.4.2 t1")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.4.2 t1 = %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
		if c.want == CannotCheck && !strings.Contains(got.Why, "/K entries") {
			t.Errorf("%s: the refusal %q is not the entry budget this fixture supplies", c.name, got.Why)
		}
	}
}

// TestAnElementOnARoleMapLoopIsNoHeading — R4-10. 7.4.2 t1 refused on any role-map loop, because the looped
// element "may be a heading"; veraPDF types it as nothing standard, so the sequence is judged without it. Every
// row is veraPDF 1.30.2's verdict on exactly this document (the P07 phase close).
func TestAnElementOnARoleMapLoopIsNoHeading(t *testing.T) {
	for _, c := range []struct {
		name, roleMap string
		kinds         []string
		want          Verdict
	}{
		{"a looped element between H1 and H2", "/Foo /Bar /Bar /Foo", []string{"H1", "Foo", "H2"}, Pass},
		{"a looped element between H1 and H3", "/Foo /Bar /Bar /Foo", []string{"H1", "Foo", "H3"}, Fail},
		{"a looped element before an H2", "/Foo /Bar /Bar /Foo", []string{"Foo", "H2"}, Fail},
		// The element's own /S is a heading's name, and that name is on the loop: still no heading.
		{"an /H2 on a loop between H1 and H3", "/H2 /Zed /Zed /H2", []string{"H1", "H2", "H3"}, Fail},
		{"an /H2 on a loop after H1", "/H2 /Zed /Zed /H2", []string{"H1", "H2"}, Pass},
		{"an /H3 on a loop after H1", "/H3 /Zed /Zed /H3", []string{"H1", "H3"}, Pass},
		{"an /H1 on a loop before an H2", "/H1 /Zed /Zed /H1", []string{"H1", "H2"}, Fail},
	} {
		got := verdictOf(t, headingTreeMapped(c.roleMap, c.kinds...), "7.4.2 t1")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.4.2 t1 = %v (%s), want %v — veraPDF's verdict on this document", c.name, got.Verdict, got.Why, c.want)
		}
	}
}
