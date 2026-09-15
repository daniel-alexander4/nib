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
