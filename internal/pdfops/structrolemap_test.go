package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The role map, published and edited — ADR-127.

// roleFixture is a one-page tree typed mostly with the document's own names:
//
//	8  Document
//	  20 Heading 1          MCID 0 "Alpha"     mapped to H1
//	  21 Heading 1          MCID 1 "Beta"
//	  22 Preformatted Text  MCID 2 "Gamma"     mapped to Code
//	  23 Orphan Type        MCID 3 "Delta"     mapped to nothing
//	  24 Chained            MCID 4 "Epsilon"   mapped to Heading 1, and so to H1
//	  25 P                  MCID 5 "Zeta"
//
// and one mapping no element uses, Unused → Note. withMap false leaves the root with no `/RoleMap` at all;
// indirect writes the map as an object of its own.
func roleFixture(withMap, indirect bool) []byte {
	var body strings.Builder
	for i, w := range []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta"} {
		fmt.Fprintf(&body, "/P <</MCID %d>> BDC\nBT /F1 12 Tf 72 %d Td (%s) Tj ET\nEMC\n", i, 700-20*i, w)
	}
	const roleMap = "<< /Heading#201 /H1 /Preformatted#20Text /Code /Unused /Note /Chained /Heading#201 >>"
	root := "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R"
	objs := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", body.Len(), body.String()),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		8:  "<< /Type /StructElem /S /Document /P 7 0 R /K [20 0 R 21 0 R 22 0 R 23 0 R 24 0 R 25 0 R] >>",
		9:  "<< /Nums [0 [20 0 R 21 0 R 22 0 R 23 0 R 24 0 R 25 0 R]] >>",
		20: "<< /Type /StructElem /S /Heading#201 /P 8 0 R /Pg 3 0 R /K [0] >>",
		21: "<< /Type /StructElem /S /Heading#201 /P 8 0 R /Pg 3 0 R /K [1] >>",
		22: "<< /Type /StructElem /S /Preformatted#20Text /P 8 0 R /Pg 3 0 R /K [2] >>",
		23: "<< /Type /StructElem /S /Orphan#20Type /P 8 0 R /Pg 3 0 R /K [3] >>",
		24: "<< /Type /StructElem /S /Chained /P 8 0 R /Pg 3 0 R /K [4] >>",
		25: "<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K [5] >>",
	}
	switch {
	case !withMap:
	case indirect:
		root += " /RoleMap 30 0 R"
		objs[30] = roleMap
	default:
		root += " /RoleMap " + roleMap
	}
	objs[7] = root + " >>"
	return assembleFixture(objs)
}

// mapping is a rolemap edit.
func mapping(role, to string) structEdit { return structEdit{kind: editRoleMap, role: role, value: to} }

// rawRoleMap is the written `/RoleMap`, as names, and whether the root carries the key as a reference.
func rawRoleMap(t *testing.T, pdf []byte) (map[string]string, bool, bool) {
	t.Helper()
	ctx, root := rawElement(t, pdf, 7)
	o, has := root["RoleMap"]
	if !has {
		return nil, false, false
	}
	_, isRef := o.(types.IndirectRef)
	d, err := ctx.DereferenceDict(o)
	if err != nil || d == nil {
		t.Fatalf("the written /RoleMap is not a dictionary: %v", o)
	}
	out := map[string]string{}
	for k, v := range d {
		n, _ := v.(types.Name)
		out[k] = n.Value()
	}
	return out, true, isRef
}

// standards is each element's type as written and as resolved, by object number.
func standards(v structureView) map[int]string {
	out := map[int]string{}
	for _, e := range v.elements {
		out[e.id] = e.kind + " → " + e.standard
	}
	return out
}

// TestTheRoleMapIsPublishedSortedWithWhatEachNameMeans — each custom name, what it is mapped to as written,
// the standard type that leads to, and how many elements carry it; an untagged document and a tree with no
// map answer an empty list, never null.
func TestTheRoleMapIsPublishedSortedWithWhatEachNameMeans(t *testing.T) {
	for _, indirect := range []bool{false, true} {
		tree, err := ReadStructure(roleFixture(true, indirect))
		if err != nil {
			t.Fatal(err)
		}
		want := []RoleMapping{
			{Name: "Chained", To: "Heading 1", Standard: "H1", Elements: 1},
			{Name: "Heading 1", To: "H1", Standard: "H1", Elements: 2},
			{Name: "Preformatted Text", To: "Code", Standard: "Code", Elements: 1},
			{Name: "Unused", To: "Note", Standard: "Note", Elements: 0},
		}
		if !reflect.DeepEqual(tree.RoleMap, want) {
			t.Errorf("indirect %v: the role map reads %+v, want %+v", indirect, tree.RoleMap, want)
		}
	}
	for name, pdf := range map[string][]byte{"a tree with no role map": roleFixture(false, false), "an untagged document": untaggedFixture()} {
		tree, err := ReadStructure(pdf)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if tree.RoleMap == nil || len(tree.RoleMap) != 0 {
			t.Errorf("%s: the role map reads %#v, want an empty list that is not nil", name, tree.RoleMap)
		}
	}
	// A name the map leads in a circle answers itself, as an element typed with it reads.
	loop := bytes.Replace(roleFixture(true, false), []byte("/Unused /Note"), []byte("/Unused /Unused2 /Unused2 /Unused"), 1)
	tree, err := ReadStructure(loop)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range tree.RoleMap {
		if m.Name == "Unused" && (m.To != "Unused2" || m.Standard != "Unused") {
			t.Errorf("a name on a loop reads %+v, want it mapped to Unused2 and resolving to itself", m)
		}
	}
}

// TestARoleMapEditSetsReplacesAndRemovesOneMapping — and changes nothing else: every other element reads the
// type it read, no content stream changes, and the tree is consistent and valid. The map is written where it
// lives, direct or as its own object.
func TestARoleMapEditSetsReplacesAndRemovesOneMapping(t *testing.T) {
	for _, indirect := range []bool{false, true} {
		src := roleFixture(true, indirect)
		before := viewOf(t, src)
		check := func(what string, out []byte, changed map[int]string, wantMap map[string]string) {
			t.Helper()
			want := standards(before)
			for id, s := range changed {
				want[id] = s
			}
			after := viewOf(t, out)
			if got := standards(after); !reflect.DeepEqual(got, want) {
				t.Errorf("indirect %v, %s: the elements read %v, want %v", indirect, what, got, want)
			}
			if !reflect.DeepEqual(textByElement(after), textByElement(before)) || !reflect.DeepEqual(contentStreams(t, out), contentStreams(t, src)) {
				t.Errorf("indirect %v, %s: an element's text or a content stream changed", indirect, what)
			}
			got, has, isRef := rawRoleMap(t, out)
			if !has || isRef != indirect || !reflect.DeepEqual(got, wantMap) {
				t.Errorf("indirect %v, %s: the written /RoleMap reads %v (present %v, a reference %v), want %v written as it was", indirect, what, got, has, isRef, wantMap)
			}
			requireConsistent(t, what, out)
			if err := Validate(out); err != nil {
				t.Errorf("indirect %v, %s: the document does not validate: %v", indirect, what, err)
			}
		}
		base := map[string]string{"Heading 1": "H1", "Preformatted Text": "Code", "Unused": "Note", "Chained": "Heading 1"}
		with := func(k, v string) map[string]string {
			out := map[string]string{}
			for n, to := range base {
				out[n] = to
			}
			if v == "" {
				delete(out, k)
			} else {
				out[k] = v
			}
			return out
		}
		// A new mapping, for a name with a space in it that an element carries.
		check("mapping an unmapped type", mustApply(t, src, mapping("Orphan Type", "BlockQuote")),
			map[int]string{23: "Orphan Type → BlockQuote"}, with("Orphan Type", "BlockQuote"))
		// A replaced one: both headings, and the name chained onto it.
		check("replacing a mapping", mustApply(t, src, mapping("Heading 1", "H2")),
			map[int]string{20: "Heading 1 → H2", 21: "Heading 1 → H2", 24: "Chained → H2"}, with("Heading 1", "H2"))
		// A chain replaced by an arrow onto a standard type.
		check("replacing a chained mapping", mustApply(t, src, mapping("Chained", "H3")),
			map[int]string{24: "Chained → H3"}, with("Chained", "H3"))
		// A removed one no element uses, and a new one no element uses.
		check("removing an unused mapping", mustApply(t, src, mapping("Unused", "")), nil, with("Unused", ""))
		check("mapping a name no element uses", mustApply(t, src, mapping("Sidebar", "Sect")), nil, with("Sidebar", "Sect"))
	}
}

// TestTheRoleMapKeyComesAndGoesWithItsMappings — created on the root for the first mapping, removed with
// the last.
func TestTheRoleMapKeyComesAndGoesWithItsMappings(t *testing.T) {
	src := roleFixture(false, false)
	if _, has, _ := rawRoleMap(t, src); has {
		t.Fatal("setup: the fixture has a /RoleMap")
	}
	out := mustApply(t, src, mapping("Orphan Type", "Note"), mapping("Spare", "P"))
	if got, has, _ := rawRoleMap(t, out); !has || !reflect.DeepEqual(got, map[string]string{"Orphan Type": "Note", "Spare": "P"}) {
		t.Fatalf("the written /RoleMap reads %v (present %v), want the two mappings", got, has)
	}
	if got := byID(viewOf(t, out))[23]; got.standard != "Note" || got.kind != "Orphan Type" {
		t.Errorf("element 23 reads %s as %s, want Orphan Type as Note", got.kind, got.standard)
	}
	one := mustApply(t, out, mapping("Spare", ""))
	if got, has, _ := rawRoleMap(t, one); !has || len(got) != 1 {
		t.Errorf("with one of two mappings removed the /RoleMap reads %v (present %v)", got, has)
	}
	// The last one goes once nothing is typed with it.
	none := mustApply(t, one, structEdit{kind: editRetype, elem: 23, value: "Note"}, mapping("Orphan Type", ""))
	if got, has, _ := rawRoleMap(t, none); has {
		t.Errorf("with its last mapping removed the root still carries a /RoleMap: %v", got)
	}
	requireConsistent(t, "removing the last mapping", none)
	// An indirect map whose last mapping goes leaves the root too.
	ind := roleFixture(true, true)
	gone := mustApply(t, ind,
		structEdit{kind: editRetype, elem: 20, value: "H1"}, structEdit{kind: editRetype, elem: 21, value: "H1"},
		structEdit{kind: editRetype, elem: 22, value: "Code"}, structEdit{kind: editRetype, elem: 24, value: "H1"},
		mapping("Heading 1", ""), mapping("Preformatted Text", ""), mapping("Unused", ""), mapping("Chained", ""))
	if got, has, _ := rawRoleMap(t, gone); has {
		t.Errorf("with every mapping removed the root still carries a /RoleMap: %v", got)
	}
}

// TestARoleMapEditThatCannotBeAppliedIsRefused — each with a sentence, and each ErrTagsReview.
func TestARoleMapEditThatCannotBeAppliedIsRefused(t *testing.T) {
	src := roleFixture(true, false)
	for _, c := range []struct {
		name string
		ed   structEdit
		says string
	}{
		{"no name", mapping("", "P"), "needs the name"},
		{"a standard type as the name", mapping("H1", "H2"), "H1 is a standard structure type"},
		{"a standard type removed", mapping("P", ""), "P is a standard structure type"},
		{"a target that is not standard", mapping("Orphan Type", "Heading 1"), "is not a standard structure type"},
		{"a target that is the name itself", mapping("Orphan Type", "Orphan Type"), "is not a standard structure type"},
		{"removing a mapping two tags use", mapping("Heading 1", ""), "2 tag(s) are still of type Heading 1"},
		{"removing a mapping one tag uses", mapping("Preformatted Text", ""), "1 tag(s) are still of type Preformatted Text"},
		{"removing a mapping that is not there", mapping("Orphan Type", ""), "no mapping for Orphan Type"},
		{"removing one from a name nothing knows", mapping("Nowhere", ""), "no mapping for Nowhere"},
		{"the mapping it already has", mapping("Heading 1", "H1"), "already mapped to H1"},
		{"an element", structEdit{kind: editRoleMap, elem: 20, role: "Orphan Type", value: "P"}, "names a type, not an element"},
		{"a parent", structEdit{kind: editRoleMap, parent: 8, role: "Orphan Type", value: "P"}, "names a type, not an element"},
		{"header cells", structEdit{kind: editRoleMap, headers: []int{20}, role: "Orphan Type", value: "P"}, "names a type, not an element"},
	} {
		_, err := applyStructEdits(src, []structEdit{c.ed})
		if !errors.Is(err, ErrTagsReview) || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: err = %v, want ErrTagsReview saying %q", c.name, err, c.says)
		}
	}
	if _, err := applyStructEdits(roleFixture(false, false), []structEdit{mapping("Orphan Type", "")}); !errors.Is(err, ErrTagsReview) {
		t.Errorf("removing a mapping from a document with no role map: err = %v, want ErrTagsReview", err)
	}
	// A /RoleMap that is not a dictionary is not written over: the read refuses the document (pdfcpu's
	// validation, before `setRoleMapping`'s own check is reached), and that is not a fault of the request.
	broken := bytes.Replace(roleFixture(false, false), []byte("/ParentTree 9 0 R"), []byte("/ParentTree 9 0 R /RoleMap [/A /P]"), 1)
	if _, err := applyStructEdits(broken, []structEdit{mapping("Orphan Type", "P")}); err == nil || errors.Is(err, ErrTagsReview) {
		t.Errorf("a /RoleMap that is an array: err = %v, want a refusal that is not about the request", err)
	}
	// As a request sends it: the kind by name, the name in Role, and an absent index as -1.
	out, err := EditStructure(src, []StructureEdit{{Kind: "rolemap", Role: "Orphan Type", Value: "Note", Index: -1}})
	if err != nil {
		t.Fatalf("the rolemap edit as a request sends it: %v", err)
	}
	if got := byID(viewOf(t, out))[23].standard; got != "Note" {
		t.Errorf("element 23 resolves to %s, want Note", got)
	}
}

// TestARoleMapEditOnEveryProducersRoleMap — every document under ~/nib/producers that carries a role map:
// its first custom name is mapped to another standard type and back. In between, only the elements whose
// type leads through that name read differently; afterwards the tree reads as it did; and the consistency
// defects are the ones the document had.
func TestARoleMapEditOnEveryProducersRoleMap(t *testing.T) {
	home, _ := os.UserHomeDir()
	var files []string
	_ = filepath.Walk(filepath.Join(home, "nib", "producers"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) == 0 {
		t.Skip("SKIP (not a pass): no corpus under ~/nib/producers, so the role map edit is UNCHECKED on a producer's document.")
	}
	edited := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		tree, err := ReadStructure(src)
		if err != nil || !tree.Tagged {
			continue
		}
		// The first mapping an edit can turn round: a custom name written straight onto a standard type.
		var m *RoleMapping
		for i := range tree.RoleMap {
			if r := tree.RoleMap[i]; !standardStructTypes[r.Name] && standardStructTypes[r.To] {
				m = &tree.RoleMap[i]
				break
			}
		}
		if m == nil {
			continue
		}
		other := "Div"
		if m.To == "Div" {
			other = "Sect"
		}
		name := filepath.Base(filepath.Dir(f)) + "/" + filepath.Base(f)
		before := viewOf(t, src)
		// Read as the editor's own read door reads, which takes documents pdfcpu's strict read refuses.
		defects := func(pdf []byte) int {
			ctx, err := inspectionRead(pdf)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			tr, err := readStructTree(ctx, livePageObjects(ctx))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return len(checkStructConsistency(ctx, tr))
		}
		was := defects(src)
		out, err := applyStructEdits(src, []structEdit{mapping(m.Name, other)})
		if err != nil {
			t.Errorf("%s: mapping %s to %s: %v", name, m.Name, other, err)
			continue
		}
		after := viewOf(t, out)
		if len(after.elements) != len(before.elements) {
			t.Errorf("%s: %d element(s) became %d", name, len(before.elements), len(after.elements))
			continue
		}
		moved := 0
		for i, b := range before.elements {
			a := after.elements[i]
			a.standard, b.standard = "", ""
			if !reflect.DeepEqual(a, b) {
				t.Errorf("%s: element %d reads differently beyond its resolved type", name, i)
				break
			}
			if after.elements[i].standard != before.elements[i].standard {
				moved++
				if after.elements[i].standard != other {
					t.Errorf("%s: element %d (%s) resolved to %s and now to %s, want %s", name, i, b.kind, before.elements[i].standard, after.elements[i].standard, other)
				}
			}
		}
		if moved < m.Elements {
			t.Errorf("%s: %d element(s) are typed %s and only %d resolve differently", name, m.Elements, m.Name, moved)
		}
		if now := defects(out); now != was {
			t.Errorf("%s: the tree had %d consistency defect(s) and has %d", name, was, now)
		}
		back, err := applyStructEdits(out, []structEdit{mapping(m.Name, m.To)})
		if err != nil {
			t.Errorf("%s: mapping %s back to %s: %v", name, m.Name, m.To, err)
			continue
		}
		if bv := viewOf(t, back); !reflect.DeepEqual(reading(bv), reading(before)) || !reflect.DeepEqual(bv.roles, before.roles) {
			t.Errorf("%s: mapped away and back, the tree does not read as it did", name)
		}
		edited++
	}
	if edited == 0 {
		t.Fatalf("%d producer document(s) found and no role map edited — the claim is unchecked", len(files))
	}
	t.Logf("a role map edit applied and taken back on %d producer document(s)", edited)
}

// TestMappingACustomTypeClearsItsUA1Clause — veraPDF's verdict: a custom type the role map leads nowhere
// fails 7.1 t5; mapped by the edit, the document fails every clause it failed but that one; and replacing a
// mapping that was already sound changes nothing.
func TestMappingACustomTypeClearsItsUA1Clause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so ADR-127's ua1 differential is UNCHECKED in this run.")
	}
	src := roleFixture(true, false)
	mapped := mustApply(t, src, mapping("Orphan Type", "P"))
	docs := map[string][]byte{
		"original.pdf": src,
		"mapped.pdf":   mapped,
		"remapped.pdf": mustApply(t, mapped, mapping("Preformatted Text", "P"), mapping("Unused", ""), mapping("Spare", "Div")),
	}
	dir := t.TempDir()
	var files []string
	for n, b := range docs {
		f := filepath.Join(dir, n)
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	cl := ua1FailedClauses(t, vp, files)
	orig, got, re := cl["original.pdf"], cl["mapped.pdf"], cl["remapped.pdf"]
	if orig == nil || got == nil || re == nil {
		t.Fatal("veraPDF could not validate the fixture or an edited copy, so there is no differential")
	}
	if !orig["7.1 t5"] {
		t.Fatalf("the fixture does not fail 7.1 t5 — it fails %v", sortedClauses(orig))
	}
	want := map[string]bool{}
	for c := range orig {
		if c != "7.1 t5" {
			want[c] = true
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("with the type mapped the document fails %v, want %v — everything it failed but 7.1 t5", sortedClauses(got), sortedClauses(want))
	}
	if !reflect.DeepEqual(re, got) {
		t.Errorf("with sound mappings replaced, removed and added the document fails %v, and failed %v", sortedClauses(re), sortedClauses(got))
	}
}
