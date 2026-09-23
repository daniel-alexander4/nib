package pdfops

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The structure-tree model — `PLAN-accessibility.md` P05.S02.

// twinElementFixture is a tagged document whose two struct elements have BYTE-IDENTICAL
// dictionaries. It exists for one reason: `inspectTags` used to key its visited set on the
// dictionary's content and counted them as one.
func twinElementFixture() []byte {
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (one) Tj ET\nEMC\n" +
		"/P <</MCID 1>> BDC\nBT /F1 24 Tf 72 660 Td (two) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R >>",
		// 8 and 10 differ in nothing at all.
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R >>",
		9:  "<< /Nums [0 [8 0 R 10 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R >>",
	})
}

// unrepresentableKidFixture carries a `/K` entry the model has no category for.
func unrepresentableKidFixture() []byte {
	content := "BT /F1 24 Tf 72 700 Td (x) Tj ET\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8: "<< /Type /StructElem /S /P /Pg 3 0 R /K [11 0 R] >>",
		// Not an element, not an MCR, not an OBJR.
		11: "<< /Type /Bookmark /Title (nonsense) >>",
	})
}

// assembleFixture writes numbered objects into a minimal PDF. Shared by the fixtures above and
// deliberately the same shape as `taggedFixture`'s body, which it was lifted from.
func assembleFixture(objs map[int]string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offs := map[int]int{}
	maxN := 0
	for n := range objs {
		if n > maxN {
			maxN = n
		}
	}
	for n := 1; n <= maxN; n++ {
		body, ok := objs[n]
		if !ok {
			continue
		}
		offs[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", maxN+1)
	for n := 1; n <= maxN; n++ {
		if off, ok := offs[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", maxN+1, xref)
	return b.Bytes()
}

// readTree is the test-side door into the model: read a document and parse its tree.
func readTree(t *testing.T, pdf []byte) (*structTree, error) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	return readStructTree(ctx, live)
}

// countElementsIndependently walks the tree a second time, from scratch, keyed on object number —
// so the model's count is compared against something that does not share its code.
//
// **A model checked against itself confirms itself.** This is the independent walk: twelve lines
// that know nothing about `structElem` and could not inherit its mistakes.
func countElementsIndependently(t *testing.T, pdf []byte) int {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := ctx.XRefTable.Catalog()
	root, _ := ctx.DereferenceDict(cat["StructTreeRoot"])
	if root == nil {
		return 0
	}
	n, seen := 0, map[int]bool{}
	var walk func(types.Object)
	walk = func(o types.Object) {
		if ind, ok := o.(types.IndirectRef); ok {
			if seen[ind.ObjectNumber.Value()] {
				return
			}
			seen[ind.ObjectNumber.Value()] = true
		}
		if arr, e := ctx.DereferenceArray(o); e == nil && arr != nil {
			for _, x := range arr {
				walk(x)
			}
			return
		}
		if _, isInt := o.(types.Integer); isInt {
			return
		}
		d, e := ctx.DereferenceDict(o)
		if e != nil || d == nil {
			return
		}
		if ty := d.NameEntry("Type"); ty == nil || *ty == "StructElem" {
			if d.NameEntry("S") != nil {
				n++
			}
		}
		if k, ok := d["K"]; ok {
			walk(k)
		}
	}
	walk(root["K"])
	return n
}

// TestTheModelCountsWhatTheDocumentContains — P05.S02's first acceptance clause, and the twin
// fixture is the case that used to fail.
func TestTheModelCountsWhatTheDocumentContains(t *testing.T) {
	for _, c := range []struct {
		name string
		pdf  []byte
		want int
	}{
		{"the tagged corpus fixture", taggedFixture(), 1},
		{"two BYTE-IDENTICAL elements", twinElementFixture(), 2},
	} {
		tree, err := readTree(t, c.pdf)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := tree.elements(); got != c.want {
			t.Errorf("%s: the model counts %d element(s), the document contains %d", c.name, got, c.want)
		}
		if got := countElementsIndependently(t, c.pdf); got != c.want {
			t.Errorf("%s: the independent walk counts %d, want %d — the fixture is wrong, not the model",
				c.name, got, c.want)
		}
		if got := inspectTags(c.pdf).elements; got != c.want {
			t.Errorf("%s: inspectTags reports %d element(s), want %d — it has drifted from the "+
				"model it is supposed to route through (D8, ADR-009)", c.name, got, c.want)
		}
	}
}

// TestInspectTagsAgreesWithTheModelOnEveryFixture is the ADR-009 half: one tree, one traversal.
func TestInspectTagsAgreesWithTheModelOnEveryFixture(t *testing.T) {
	for name, pdf := range map[string][]byte{
		"tagged":   taggedFixture(),
		"twin":     twinElementFixture(),
		"untagged": untaggedFixture(),
	} {
		s := inspectTags(pdf)
		tree, err := readTree(t, pdf)
		if err != nil {
			if s.elements != 0 || s.anchored != 0 {
				t.Errorf("%s: the model refuses the tree but inspectTags still reports %d "+
					"element(s) — they are not the same traversal", name, s.elements)
			}
			continue
		}
		if s.elements != tree.elements() || s.anchored != tree.anchored() {
			t.Errorf("%s: inspectTags says elements=%d anchored=%d; the model says %d/%d",
				name, s.elements, s.anchored, tree.elements(), tree.anchored())
		}
	}
}

// TestAKidTheModelCannotClassifyIsRefused — the second acceptance clause.
//
// Skipping an unrecognised `/K` entry would produce a model describing fewer children than the
// document has, and every count taken from it would be quietly wrong. That is the failure this
// slice exists to prevent, so it must not be the parser's own behaviour.
//
// # Why this reads the document WITHOUT validation, which is a finding in itself
//
// **pdfcpu already refuses this document.** `api.ReadValidateAndOptimize` answers
// `validateStructElementKArrayElement: invalid dictType Bookmark (should be "StructElem" or "OBJR"
// or "MCR")` — so through nib's normal door the model's own refusal is unreachable, because the
// three kinds pdfcpu permits are exactly the three the model represents.
//
// That is worth knowing rather than worth celebrating. The model's branch is a second line of
// defence against a pdfcpu that relaxes, a caller that reads without validating, or a fourth kind
// added to the specification — and a defence nothing exercises is a defence nobody can trust. So it
// is driven here through `api.ReadContext`, which parses without validating.
func TestAKidTheModelCannotClassifyIsRefused(t *testing.T) {
	pdf := unrepresentableKidFixture()

	// The floor: the VALIDATING door must reject this, or the paragraph above is wrong and this
	// test is reading a document nib would happily accept.
	if _, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration()); err == nil {
		t.Fatal("pdfcpu now ACCEPTS a /K entry of /Type /Bookmark — the model's refusal has become " +
			"reachable through nib's normal door, which is a change worth knowing about")
	}

	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("even the non-validating read refused the fixture: %v", err)
	}
	live := map[int]bool{}
	for p := 1; p <= ctx.PageCount; p++ {
		if ir, e := ctx.PageDictIndRef(p); e == nil && ir != nil {
			live[ir.ObjectNumber.Value()] = true
		}
	}
	_, terr := readStructTree(ctx, live)
	if terr == nil {
		t.Fatal("a /K entry of /Type /Bookmark was accepted — the model silently dropped a child " +
			"and every count it reports for this document is short by one")
	}
	if !strings.Contains(terr.Error(), "Bookmark") {
		t.Errorf("the refusal does not name what it could not represent: %v", terr)
	}
}

// TestADocumentWithNoTreeIsNotAnError: absent and malformed are different answers, and a caller
// that cannot tell them apart will report every untagged document as broken.
func TestADocumentWithNoTreeIsNotAnError(t *testing.T) {
	_, err := readTree(t, untaggedFixture())
	if err != errNoStructTree {
		t.Errorf("reading an untagged document gave %v, want errNoStructTree", err)
	}
}

// TestARealRoleMappedTreeIsRepresented — the population clause, and the generated fixtures cannot
// cover it: none of them has a `/RoleMap`, an `OBJR`, or an integer MCID under an element.
//
// The fixture is produced by LibreOffice from HTML at test time rather than committed, for the
// reason `corpus_test.go` gives about opaque binaries — and it SKIPS loudly rather than passing
// when LibreOffice is absent, because a population clause that silently drops its only real
// document is the clause failing.
func TestARealRoleMappedTreeIsRepresented(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is absent, so the only REAL structure tree in the " +
			"population — role-mapped names, an OBJR, integer MCIDs — is not exercised here. The " +
			"generated fixtures have none of the three.")
	}
	const html = `<html><body><h1>A heading</h1><p>Body with <b>bold</b>.</p>` +
		`<ul><li>one</li><li>two</li></ul>` +
		`<table border="1"><tr><th>H</th></tr><tr><td>c</td></tr></table>` +
		`<p>A <a href="https://example.com">link</a>.</p></body></html>`
	dir := t.TempDir()
	in := filepath.Join(dir, "rich.html")
	if err := os.WriteFile(in, []byte(html), 0o600); err != nil {
		t.Fatal(err)
	}
	// Through `fileURL`, not `"file://"+path`, for the reason that helper states: the bare
	// concatenation is right only for an absolute POSIX path, and these two harnesses were the
	// last places still doing it after production stopped (/pending 542).
	cmd := exec.Command(libreOfficePath(), "-env:UserInstallation="+fileURL(filepath.ToSlash(filepath.Join(dir, "prof"))),
		"--headless", "--convert-to", "pdf", in, "--outdir", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("SKIP (not a pass): LibreOffice could not convert the fixture: %v\n%s", err, out)
	}
	pdf, err := os.ReadFile(filepath.Join(dir, "rich.pdf"))
	if err != nil {
		t.Skipf("SKIP (not a pass): no converted fixture: %v", err)
	}

	tree, err := readTree(t, pdf)
	if err != nil {
		t.Fatalf("the model refused a tree LibreOffice produced: %v", err)
	}

	// The floor: this document must actually contain the three things it is here for, or the
	// assertions below are about a document that does not test them.
	var roles, objrs, mcids int
	roles = len(tree.roleMap)
	types := map[string]int{}
	for _, e := range tree.elems {
		types[e.kind]++
		for _, k := range e.kids {
			switch k.kind {
			case kidOBJR:
				objrs++
			case kidMCID:
				mcids++
			}
		}
	}
	if roles == 0 {
		t.Error("the real tree carries no /RoleMap, so role mapping is unexercised")
	}
	if objrs == 0 {
		t.Error("the real tree carries no OBJR, so that /K kind is unexercised")
	}
	if mcids == 0 {
		t.Error("the real tree carries no integer MCID, so that /K kind is unexercised")
	}
	if len(types) < 5 {
		t.Errorf("the real tree has %d distinct element type(s) — too uniform to exercise the model", len(types))
	}

	// And the model must agree with the independent walk on the count.
	if got, want := tree.elements(), countElementsIndependently(t, pdf); got != want {
		t.Errorf("the model counts %d element(s), an independent object-keyed walk counts %d", got, want)
	}
	if s := inspectTags(pdf); s.elements != tree.elements() {
		t.Errorf("inspectTags says %d, the model says %d", s.elements, tree.elements())
	}

	names := make([]string, 0, len(types))
	for n := range types {
		names = append(names, n)
	}
	sort.Strings(names)
	t.Logf("real tree: %d elements, %d type(s) %v, %d role-map entry(ies), %d OBJR, %d MCID",
		tree.elements(), len(types), names, roles, objrs, mcids)
}

// sharedElementFixture references ONE element from two parents — a DAG rather than a tree. Legal
// enough for pdfcpu, and the case that tells a working visited set from an absent one: without it
// the element is walked, and counted, twice.
func sharedElementFixture() []byte {
	content := "BT /F1 24 Tf 72 700 Td (x) Tj ET\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 13 0 R >>",
		8: "<< /Type /StructElem /S /Sect /Pg 3 0 R /K [12 0 R] >>",
		// 12 is the child of BOTH 8 and 10.
		10: "<< /Type /StructElem /S /Sect /Pg 3 0 R /K [12 0 R] >>",
		12: "<< /Type /StructElem /S /P /Pg 3 0 R >>",
		13: "<< /Nums [0 [12 0 R]] >>",
	})
}

// danglingStructParentsFixture gives a page a `/StructParents` key the `/ParentTree` has no entry
// for — the page declares a row in a table that has no such row, so every MCID on it is unreachable
// from the tree while the document still looks tagged.
func danglingStructParentsFixture() []byte {
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (x) Tj ET\nEMC\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 4 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>",
		8: "<< /Type /StructElem /S /P /Pg 3 0 R /K [0] >>",
		// The page says key 4; the tree only has key 0.
		9: "<< /Nums [0 [8 0 R]] >>",
	})
}

// cyclicElementFixture has an element whose `/K` points back at its own ancestor.
func cyclicElementFixture() []byte {
	content := "BT /F1 24 Tf 72 700 Td (x) Tj ET\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /StructTreeRoot /K [8 0 R] >>",
		8: "<< /Type /StructElem /S /Sect /Pg 3 0 R /K [9 0 R] >>",
		9: "<< /Type /StructElem /S /P /Pg 3 0 R /K [8 0 R] >>",
	})
}

// TestASharedElementIsWalkedOnce — the visited set, which nothing else in this package drives.
//
// **The generated corpus is all simple trees**, so disabling the visited set left every other test
// green. A structure tree that reaches one element from two parents is legal and ordinary, and
// without the set that element is walked and counted twice — a model that reports more children
// than the document has, which is the same class of wrongness as reporting fewer.
func TestASharedElementIsWalkedOnce(t *testing.T) {
	pdf := sharedElementFixture()
	tree, err := readTree(t, pdf)
	if err != nil {
		t.Fatalf("the model refused a DAG: %v", err)
	}
	// Three distinct elements: two parents and one shared child.
	if got := tree.elements(); got != 3 {
		t.Errorf("the model counts %d element(s); the document has 3 distinct ones, one of which "+
			"is reached from two parents. %d means the visited set is not working",
			got, tree.elements())
	}
	if got := len(tree.byObj); got != 3 {
		t.Errorf("the model indexes %d element(s) by object number, want 3", got)
	}
	if s := inspectTags(pdf).elements; s != 3 {
		t.Errorf("inspectTags reports %d, the model says 3", s)
	}
}

// TestACyclicTreeIsRefusedRatherThanFollowedForever.
//
// The visited set stops a cycle through INDIRECT elements; the depth bound is what stops one that
// the visited set cannot see. Both are asserted, because between them they are the reason a
// malformed document cannot take the process with it — `/pending 454` is this repo's standing
// lesson about walking a PDF without a bound.
func TestACyclicTreeIsRefusedRatherThanFollowedForever(t *testing.T) {
	pdf := cyclicElementFixture()
	// It must terminate. The result may be a refusal or a finite tree — what it may not be is a
	// hang, and a test that hangs is how you find out.
	done := make(chan struct{})
	var tree *structTree
	var err error
	go func() {
		defer close(done)
		tree, err = readTree(t, pdf)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("reading a cyclic structure tree did not terminate in 10s")
	}
	if err == nil && tree.elements() > 2 {
		t.Errorf("a two-element cycle produced %d elements — the walk went round at least once",
			tree.elements())
	}
	t.Logf("a cyclic tree resolved to err=%v", err)
}

// TestStandardRoleStopsAtTheFirstTypeItRecognises — P03.S01. The Tags panel's resolver followed the role
// map for ten hops and stopped at nothing it recognised, the misreading `uacheck.standardType` had. Each
// row is a shape veraPDF 1.30.2 was measured on in that slice.
func TestStandardRoleStopsAtTheFirstTypeItRecognises(t *testing.T) {
	long := map[string]string{}
	for i := 0; i < 11; i++ {
		long[fmt.Sprintf("T%d", i)] = fmt.Sprintf("T%d", i+1)
	}
	long["T11"] = "H2"
	for _, tc := range []struct {
		name    string
		roleMap map[string]string
		kind    string
		want    string
	}{
		{"a chain through a standard type stops there", map[string]string{"Alpha": "P", "P": "Zed"}, "Alpha", "P"},
		{"a standard type the map sends elsewhere reads as where it is sent", map[string]string{"TR": "TD"}, "TR", "TD"},
		{"a chain longer than ten hops still resolves", long, "T0", "H2"},
		{"a loop answers the name as written", map[string]string{"Loopy": "Ringy", "Ringy": "Loopy"}, "Loopy", "Loopy"},
		{"a self-map types as itself", map[string]string{"LI": "LI"}, "LI", "LI"},
		// From /TR the loop comes back to /TR; from /Zed it reaches /TR by mapping and stops. (Asking the
		// revisit before or after recognition gives the same answers here — for this resolver a loop answers
		// the name as written, and that name IS /TR — so no row can pin the order; uacheck's walk, whose loop
		// answer differs, pins it.)
		{"a loop entered from a standard start answers the name as written", map[string]string{"TR": "Zed", "Zed": "TR"}, "TR", "TR"},
		{"the same loop entered from its other name stops at the standard type", map[string]string{"TR": "Zed", "Zed": "TR"}, "Zed", "TR"},
		// A loop entered from OUTSIDE it: the name as written is the entry, not the name revisited.
		{"a loop entered from outside it answers the entry", map[string]string{"Alpha": "Loopy", "Loopy": "Ringy", "Ringy": "Loopy"}, "Alpha", "Alpha"},
		{"an unmapped name is itself", nil, "Custom", "Custom"},
	} {
		if got := standardRole(&structTree{roleMap: tc.roleMap}, tc.kind); got != tc.want {
			t.Errorf("%s: standardRole(%q) = %q, want %q", tc.name, tc.kind, got, tc.want)
		}
		// The memo (P03's phase close) settles every name a walk passes: each name must answer on a tree that
		// has already walked the others exactly what it answers on a fresh one, whichever order they ran in.
		var names []string
		for k, v := range tc.roleMap {
			names = append(names, k, v)
		}
		sort.Strings(names)
		for _, order := range [][]string{names, reversed(names)} {
			shared := &structTree{roleMap: tc.roleMap}
			for _, n := range order {
				if got, fresh := standardRole(shared, n), standardRole(&structTree{roleMap: tc.roleMap}, n); got != fresh {
					t.Errorf("%s: after the names before it, standardRole(%q) = %q; on a fresh tree %q", tc.name, n, got, fresh)
				}
			}
		}
	}
}

func reversed(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}

// TestStandardRoleIsLinearInTheChain — P03's phase-close review measured 60 s on the Tags panel's route for a
// tree whose 20,000 elements each start a different link of one 20,000-name chain. Every link must now
// answer the chain's end, in time proportional to the chain.
func TestStandardRoleIsLinearInTheChain(t *testing.T) {
	const n = 20000
	rm := map[string]string{}
	for i := 0; i < n; i++ {
		rm[fmt.Sprintf("T%d", i)] = fmt.Sprintf("T%d", i+1)
	}
	rm[fmt.Sprintf("T%d", n)] = "H2"
	tree := &structTree{roleMap: rm}
	start := time.Now()
	for i := n; i >= 0; i-- { // end first, then start first: the memo must hold from either side
		if got := standardRole(tree, fmt.Sprintf("T%d", i)); got != "H2" {
			t.Fatalf("T%d = %q, want H2", i, got)
		}
	}
	for i := 0; i <= n; i++ {
		standardRole(tree, fmt.Sprintf("T%d", i))
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("%d chain links took %v; the walk is not memoised", n, el)
	}
}

// TestAnIndirectRoleMapNameIsFollowed — P03's phase close: the checker dereferences a role map value stored as an
// indirect name (`uacheck.Document.name`), and this reader kept only direct names, so `/Alpha → 12 0 R (/P)`
// typed as P in the checker and as Alpha in the Tags panel. The direct row is the control.
func TestAnIndirectRoleMapNameIsFollowed(t *testing.T) {
	for _, tc := range []struct{ name, alpha string }{{"direct", "/P"}, {"indirect", "12 0 R"}} {
		tree, err := readTree(t, assembleFixture(map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
			7:  "<< /Type /StructTreeRoot /K 8 0 R /RoleMap << /Alpha " + tc.alpha + " >> >>",
			8:  "<< /Type /StructElem /S /Alpha /P 7 0 R >>",
			12: "/P",
		}))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := standardRole(tree, "Alpha"); got != "P" {
			t.Errorf("%s: /Alpha → %s types as %q, want P", tc.name, tc.alpha, got)
		}
	}
}
