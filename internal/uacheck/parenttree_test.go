package uacheck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// parentTreeKeyDoc is one annotation whose /StructParent is 1, under a parent tree `tree` (object 9 and any nodes it
// names) whose key 1 may name object 10, an Annot element, or object 11, a P element — so 7.18.1 t1 passes exactly when
// the value the tree answers for key 1 is object 10.
func parentTreeKeyDoc(tree map[int]string) []byte {
	extra := map[int]string{
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R 11 0 R] /ParentTree 9 0 R >>",
		11: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R >>",
	}
	for k, v := range tree {
		extra[k] = v
	}
	return annotFixture{annot: note(), elem: "/S /Annot", extra: extra}.build()
}

// TestTheParentTreeAnswersAKeyAsVeraPDFLooksItUp — the P07 phase-close re-review, RR2-1, measured on veraPDF 1.30.2:
// `PDNumberTreeNode.getObject` is a depth-first search taking the FIRST node holding the key (the walk kept the last,
// and the first two rows were a live false fail and a live false pass), the LAST pair within one /Nums array, never a
// /Nums node's /Kids, and nothing a node's /Limits excludes. A node reached twice — here the same node twice in one
// /Kids array — answers as it did the first time.
func TestTheParentTreeAnswersAKeyAsVeraPDFLooksItUp(t *testing.T) {
	const annot, p = "1 10 0 R", "1 11 0 R"
	cases := []struct {
		name string
		tree map[int]string
		want Verdict // veraPDF's, on 7.18.1 t1
	}{
		{"a key in two nodes, the Annot row first", map[int]string{9: "<< /Kids [40 0 R 41 0 R] >>",
			40: "<< /Nums [0 [8 0 R] " + annot + "] /Limits [0 1] >>", 41: "<< /Nums [" + p + "] /Limits [1 1] >>"}, Pass},
		{"a key in two nodes, the P row first", map[int]string{9: "<< /Kids [40 0 R 41 0 R] >>",
			40: "<< /Nums [0 [8 0 R] " + p + "] /Limits [0 1] >>", 41: "<< /Nums [" + annot + "] /Limits [1 1] >>"}, Fail},
		{"a key twice in one node, the Annot row last", map[int]string{9: "<< /Nums [0 [8 0 R] " + p + " " + annot + "] >>"}, Pass},
		{"a key twice in one node, the P row last", map[int]string{9: "<< /Nums [0 [8 0 R] " + annot + " " + p + "] >>"}, Fail},
		{"a key only below a node that has /Nums", map[int]string{9: "<< /Nums [0 [8 0 R]] /Kids [41 0 R] >>",
			41: "<< /Nums [" + annot + "] /Limits [1 1] >>"}, Fail},
		{"a key the first node's /Limits excludes", map[int]string{9: "<< /Kids [40 0 R 41 0 R] >>",
			40: "<< /Nums [0 [8 0 R] " + annot + "] /Limits [0 0] >>", 41: "<< /Nums [" + p + "] /Limits [1 1] >>"}, Fail},
		{"a node reached twice before the other", map[int]string{9: "<< /Kids [40 0 R 40 0 R 41 0 R] >>",
			40: "<< /Nums [0 [8 0 R] " + annot + "] /Limits [0 1] >>", 41: "<< /Nums [" + p + "] /Limits [1 1] >>"}, Pass},
		// Reached first under an ancestor's /Limits that excludes the key, then directly: the second reach offers it.
		{"a node reached first under a /Limits excluding the key, then directly", map[int]string{9: "<< /Kids [42 0 R 40 0 R 41 0 R] >>",
			42: "<< /Kids [40 0 R] /Limits [0 0] >>", 40: "<< /Nums [0 [8 0 R] " + annot + "] /Limits [0 1] >>",
			41: "<< /Nums [" + p + "] /Limits [1 1] >>"}, Pass},
	}
	// Under first-found, a key found AFTER a part nib could not read is not known to be the one veraPDF takes: the
	// unread part, earlier in the search, may hold it. Here it does — veraPDF finds the Annot row at the bottom of a deep
	// chain first and passes. Within `maxParentTreeDepth` nib reads on and finds it too (RR3-2: the bound was 64, and a
	// chain of 67 was refused where veraPDF answers); one level more and it refuses — the whole document, since veraPDF
	// itself stops reporting somewhere past 4,000 (measured) — rather than answer from the P row after it.
	deepDoc := func(n int) []byte {
		deep := map[int]string{7: "<< /Type /StructTreeRoot /K [8 0 R 10 0 R 11 0 R] /ParentTree << /Kids [300 0 R 41 0 R] >> >>",
			41: "<< /Nums [" + p + "] /Limits [1 1] >>"}
		for i := 0; i < n; i++ {
			deep[300+i] = fmt.Sprintf("<< /Kids [%d 0 R] /Limits [1 1] >>", 301+i)
		}
		deep[300+n] = "<< /Nums [" + annot + "] /Limits [1 1] >>"
		return parentTreeKeyDoc(deep)
	}
	under := deepDoc(maxParentTreeDepth - 1) // the Nums node sits at depth maxParentTreeDepth
	if got := verdictOf(t, under, "7.18.1 t1"); got.Verdict != Pass {
		t.Errorf("a key first found at the depth bound: 7.18.1 t1 = %v (%s), want Pass — veraPDF's", got.Verdict, got.Why)
	}
	if got := verdictOf(t, deepDoc(maxParentTreeDepth), "7.18.1 t1"); got.Verdict != CannotCheck || !strings.Contains(got.Why, "parent tree") {
		t.Errorf("a key first found past the depth bound, then again after it: 7.18.1 t1 = %v (%s), want CannotCheck "+
			"naming the parent tree", got.Verdict, got.Why)
	}
	if got := veraAsk(t, [][]byte{under}); got != nil && got[0]["7.18.1 t1"] != "passed" {
		t.Errorf("the deep document: veraPDF now says %q, measured \"passed\"", got[0]["7.18.1 t1"])
	}

	var docs [][]byte
	for _, c := range cases {
		pdf := parentTreeKeyDoc(c.tree)
		if got := verdictOf(t, pdf, "7.18.1 t1"); got.Verdict != c.want {
			t.Errorf("%s: 7.18.1 t1 = %v (%s), want %v — veraPDF's", c.name, got.Verdict, got.Why, c.want)
		}
		docs = append(docs, pdf)
	}
	want := map[Verdict]string{Pass: "passed", Fail: "failed"}
	for i, got := range veraAsk(t, docs) {
		if got["7.18.1 t1"] != want[cases[i].want] {
			t.Errorf("%s: veraPDF now says %q, measured %q", cases[i].name, got["7.18.1 t1"], want[cases[i].want])
		}
	}
}

// undecodableRef installs an object pdfcpu cannot decode — an object-stream member whose offsets fall outside its
// stream — and returns a reference to it: a PRESENT value `Dereference` errors on, the one kind of /Nums or /Kids nib
// refuses rather than reads as empty (RR3-3).
func undecodableRef(t *testing.T, d *Document) types.IndirectRef {
	t.Helper()
	nr := 1
	for d.Ctx.XRefTable.Table[nr] != nil {
		nr++
	}
	osd := &types.ObjectStreamDict{StreamDict: types.StreamDict{Dict: types.Dict{}, Content: []byte("x")}}
	d.Ctx.XRefTable.Table[nr] = &model.XRefTableEntry{Object: types.NewLazyObjectStreamObject(osd, 5, 1,
		func(context.Context, string) (types.Object, error) { return nil, errors.New("undecodable") })}
	ir := types.IndirectRef{ObjectNumber: types.Integer(nr)}
	if _, err := d.Ctx.Dereference(ir); err == nil {
		t.Fatal("stimulus: the undecodable reference dereferences without error")
	}
	return ir
}

// TestTheParentTreeReadsLoopsAndOddValuesAsVeraPDFDoes — the P07 phase-close re-review, RR3-2 and RR3-3, each row
// measured on veraPDF 1.30.2. The parent tree's root is written inline, because pdfcpu refuses to open a looped or
// malformed tree whose root is an object (its validator stops at depth 100 or on the wrong type) and admits these.
//
// `PDNumberTreeNode` throws `LoopedException` when a node lists its own or an ancestor's OBJECT NUMBER among its /Kids,
// and veraPDF reports nothing on the document — but only when a lookup REACHES that node: a loop after the node holding
// the key, under /Limits excluding it, or below a /Nums node is never parsed. It builds every kid before searching one,
// so a loop listed after the kid holding the key still throws. A node shared by two parents is not a loop. A node
// written INLINE that is its own descendant has no object number for the check to see, and veraPDF recurses until its
// stack runs out — no report either. A /Nums or /Kids that is not an array is empty, not unread (RR3-3).
func TestTheParentTreeReadsLoopsAndOddValuesAsVeraPDFDoes(t *testing.T) {
	const k0, annot, nothing = "0 [8 0 R]", "1 10 0 R", CannotCheck
	holds := "<< /Nums [" + k0 + " " + annot + "] /Limits [0 1] >>"
	root := func(pt string) string {
		return "<< /Type /StructTreeRoot /K [8 0 R 10 0 R 11 0 R] /ParentTree " + pt + " >>"
	}
	cases := []struct {
		name     string
		tree     map[int]string
		want     Verdict // on 7.18.1 t1; `nothing` is every clause refused as veraPDF reports nothing
		overflow bool    // veraPDF's StackOverflowError, measured through the CLI (its report is not one veraAsk reads)
	}{
		{"a node listing itself, the keys nowhere", map[int]string{7: root("<< /Kids [40 0 R] >>"), 40: "<< /Kids [40 0 R] >>"}, nothing, false},
		{"a node listing itself, after the node holding the keys", map[int]string{7: root("<< /Kids [41 0 R 40 0 R] >>"),
			40: "<< /Kids [40 0 R] >>", 41: holds}, Pass, false},
		{"a node listing itself, first, its /Limits excluding the keys", map[int]string{7: root("<< /Kids [40 0 R 41 0 R] >>"),
			40: "<< /Limits [5 5] /Kids [40 0 R] >>", 41: holds}, Pass, false},
		{"a /Nums node listing itself in /Kids", map[int]string{7: root("<< /Kids [40 0 R] >>"),
			40: "<< /Nums [" + k0 + " " + annot + "] /Kids [40 0 R] >>"}, Pass, false},
		{"two nodes listing each other, first", map[int]string{7: root("<< /Kids [40 0 R 41 0 R] >>"),
			40: "<< /Kids [42 0 R] >>", 42: "<< /Kids [40 0 R] >>", 41: holds}, nothing, false},
		{"a node listing itself after a kid that holds the keys", map[int]string{7: root("<< /Kids [40 0 R] >>"),
			40: "<< /Kids [41 0 R 40 0 R] >>", 41: holds}, nothing, false},
		{"one node twice in a /Kids", map[int]string{7: root("<< /Kids [40 0 R 40 0 R 41 0 R] >>"),
			40: "<< /Kids [42 0 R] >>", 42: "<< /Nums [5 null] >>", 41: holds}, Pass, false},
		{"a diamond", map[int]string{7: root("<< /Kids [40 0 R 43 0 R 41 0 R] >>"), 40: "<< /Kids [42 0 R] >>",
			43: "<< /Kids [42 0 R] >>", 42: "<< /Nums [5 null] >>", 41: holds}, Pass, false},
		{"an inline node its own descendant, the keys nowhere", map[int]string{7: root("<< /Kids 44 0 R >>"),
			44: "[<< /Kids 44 0 R >>]"}, nothing, true},
		{"an inline node its own descendant, after the keys", map[int]string{7: root("<< /Kids [41 0 R 45 0 R] >>"),
			45: "<< /Kids 44 0 R >>", 44: "[<< /Kids 44 0 R >>]", 41: holds}, Pass, false},
		{"an inline node its own descendant, before the keys", map[int]string{7: root("<< /Kids [45 0 R 41 0 R] >>"),
			45: "<< /Kids 44 0 R >>", 44: "[<< /Kids 44 0 R >>]", 41: holds}, nothing, true},
		{"an integer /Kids", map[int]string{7: root("<< /Kids [40 0 R 41 0 R] >>"), 40: "<< /Kids 5 >>", 41: holds}, Pass, false},
		{"a string /Nums", map[int]string{7: root("<< /Kids [40 0 R 41 0 R] >>"), 40: "<< /Nums (x) >>", 41: holds}, Pass, false},
		{"a dictionary /Nums", map[int]string{7: root("<< /Kids [40 0 R 41 0 R] >>"), 40: "<< /Nums << /A 1 >> >>", 41: holds}, Pass, false},
		{"a string /Nums on the root, the keys in its /Kids", map[int]string{7: root("<< /Nums (x) /Kids [41 0 R] >>"), 41: holds}, Fail, false},
	}
	var docs [][]byte
	var asked []int
	for i, c := range cases {
		pdf := parentTreeKeyDoc(c.tree)
		rep, err := Check(pdf)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if c.want == nothing {
			for _, r := range rep.Results {
				if r.Verdict != CannotCheck || !strings.HasPrefix(r.Why, "veraPDF reports nothing on this document —") {
					t.Errorf("%s: %s = %v (%s), want CannotCheck — veraPDF reports nothing on this document", c.name, r.Clause, r.Verdict, r.Why)
					break
				}
			}
		} else if got := verdictIn(t, rep, "7.18.1 t1"); got != c.want {
			t.Errorf("%s: 7.18.1 t1 = %v, want %v — veraPDF's", c.name, got, c.want)
		}
		if !c.overflow {
			docs, asked = append(docs, pdf), append(asked, i)
		}
	}
	want := map[Verdict]string{Pass: "passed", Fail: "failed"}
	for j, got := range veraAsk(t, docs) {
		c := cases[asked[j]]
		switch {
		case c.want == nothing && got != nil:
			t.Errorf("%s: veraPDF now reports on it, measured nothing (LoopedException)", c.name)
		case c.want != nothing && (got == nil || got["7.18.1 t1"] != want[c.want]):
			t.Errorf("%s: veraPDF now says %q, measured %q", c.name, got["7.18.1 t1"], want[c.want])
		}
	}
}

// limitsTree is RR3-1's shape: D levels of indirect arrays of W inline nodes, each naming the next level's array, the
// first two levels carrying /Limits that open a different key range per node — so a walk keyed on the range a node is
// reached under read each node once per range, W^3·D reads (60 s at W=60, measured). The last level holds key 0.
func limitsTree(W, D int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 4 0 R /MarkInfo << /Marked true >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		4: "<< /Type /StructTreeRoot /ParentTree << /Kids 10 0 R >> >>",
	}
	for lvl := 0; lvl < D; lvl++ {
		var b strings.Builder
		b.WriteString("[")
		for j := 0; j < W; j++ {
			switch {
			case lvl == D-1:
				b.WriteString("<< /Nums [0 null] >> ")
			case lvl == 0:
				fmt.Fprintf(&b, "<< /Limits [%d 2000000000] /Kids %d 0 R >> ", j-W, 11+lvl)
			case lvl == 1:
				fmt.Fprintf(&b, "<< /Limits [-2000000000 %d] /Kids %d 0 R >> ", 1000+j, 11+lvl)
			default:
				fmt.Fprintf(&b, "<< /Kids %d 0 R >> ", 11+lvl)
			}
		}
		b.WriteString("]")
		objs[10+lvl] = b.String()
	}
	return buildPDF(objs)
}

// TestAParentTreeLookupReadsEachNodeOnce — RR3-1. A lookup reads each node at most once, so a key the tree lacks costs
// its /Kids entries — W²(D−1)+W+1 reads here, where the walk it replaced made W³·D — and a document's lookups share
// one budget: exactly at the ceiling the key is definitely absent, one read under it the key is refused and so is the
// document, since the part nib did not read may hold a loop.
func TestAParentTreeLookupReadsEachNodeOnce(t *testing.T) {
	const W, D, absent = 10, 60, 5
	pdf := limitsTree(W, D)
	d, err := open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, why := d.parentTreeEntry(0); !found || why != "" || d.ptNodes != D+1 {
		t.Fatalf("stimulus: key 0 found %v (why %q) in %d reads, want found in %d — the first path", found, why, d.ptNodes, D+1)
	}
	before := d.ptNodes
	if _, found, why := d.parentTreeEntry(absent); found || why != "" {
		t.Fatalf("key %d: found %v, why %q — want definitely absent", absent, found, why)
	}
	reads := d.ptNodes - before
	if want := W*W*(D-1) + W + 1; reads != want {
		t.Fatalf("an absent key read %d nodes, want %d — each node's /Kids once", reads, want)
	}
	saved := maxParentTreeReads
	t.Cleanup(func() { maxParentTreeReads = saved })
	for _, tc := range []struct {
		ceiling int
		refuse  bool
	}{{reads, false}, {reads - 1, true}} {
		maxParentTreeReads = tc.ceiling
		d, err := open(pdf)
		if err != nil {
			t.Fatal(err)
		}
		_, found, why := d.parentTreeEntry(absent)
		if d.ptNodes < tc.ceiling {
			t.Fatalf("ceiling %d: the lookup read %d nodes — the stimulus did not reach the ceiling", tc.ceiling, d.ptNodes)
		}
		if refused := strings.Contains(why, "budget"); refused != tc.refuse || found {
			t.Fatalf("ceiling %d over %d reads: found %v, why %q, want refusal %v", tc.ceiling, reads, found, why, tc.refuse)
		}
		if doc := d.reportsNothing(); strings.Contains(doc, "budget") != tc.refuse {
			t.Fatalf("ceiling %d: reportsNothing = %q, want a refusal naming the budget %v", tc.ceiling, doc, tc.refuse)
		}
	}
}

// ruleVerdict is verdictOf with `reportsNothing` stepped past: the clause's own answer. A parent tree nib stopped
// reading refuses the whole document (RR3-2), so a test of a clause's OWN refusal runs the rule directly or it measures
// the document-level refusal in front of it.
func ruleVerdict(t *testing.T, pdf []byte, clause string) Result {
	t.Helper()
	d, err := open(pdf)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	rule, ok := registry[clause]
	if !ok {
		t.Fatalf("clause %q is not registered", clause)
	}
	return runOne(rule, d)
}

// TestTheLoopQuestionAsksInlineAnnotationsKeysToo — RR3-2's key sweep. veraPDF looks up an annotation's or a form field's
// /StructParent whether the holder is an object or written inline (in /Annots, /Fields or a field's /Kids), so the loop
// question asks both: the xref sweep (`structParentKeys`) sees objects, and the annotation-and-field walk sees the inline
// holders (re-review round 3 — the field half was missing). The stimulus is the key having been ASKED of the tree.
// An inline FIELD cannot be built: pdfcpu refuses a direct dictionary in /Fields ("corrupt form field array entry") and
// in a field's /Kids ("entries must be indirect reference"), measured — so that half is unreachable through a file
// today, and is covered because the walk that asks it is the same walk every field rule reads.
func TestTheLoopQuestionAsksInlineAnnotationsKeysToo(t *testing.T) {
	for _, tc := range []struct {
		name string
		pdf  []byte
	}{
		{"an inline annotation", annotFixture{annots: "[<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /StructParent 7 >>]",
			elem: "/S /Annot"}.build()},
	} {
		d, err := open(tc.pdf)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if keys := d.structParentKeys(); fmt.Sprint(keys) != "[0]" {
			t.Fatalf("%s: precondition: the xref sweep sees %v, want [0] — the key under test must be the inline holder's alone", tc.name, keys)
		}
		d.parentTreeNothing()
		if _, asked := d.ptAns[7]; !asked {
			t.Errorf("%s: the loop question never asked key 7, the inline holder's /StructParent", tc.name)
		}
	}
}

// TestAnUnreadableParentTreeNodeRefusesTheLoopQuestion — re-review round 3. A lookup that stops at a /Nums (or /Kids)
// nib cannot dereference has not read the rest of the tree, where veraPDF's lookup may go on and meet a loop, so
// `parentTreeNothing` must refuse the document — not only that key. The control is the same document with a readable,
// empty /Nums: the question is answered ("" — no loop).
func TestAnUnreadableParentTreeNodeRefusesTheLoopQuestion(t *testing.T) {
	build := func(nums func(d *Document) types.Object) *Document {
		return openMutated(t, (annotFixture{annot: note(), elem: annotTag}).build(), func(d *Document, _ types.Dict) {
			root := d.dict(d.Catalog["StructTreeRoot"])
			if root == nil {
				t.Fatal("the fixture has no StructTreeRoot")
			}
			root["ParentTree"] = types.Dict{"Nums": nums(d)}
		})
	}
	control := build(func(*Document) types.Object { return types.Array{} })
	if why := control.parentTreeNothing(); why != "" {
		t.Fatalf("control: a readable, empty parent tree answers %q, want \"\"", why)
	}
	d := build(func(d *Document) types.Object { return undecodableRef(t, d) })
	if len(d.structParentKeys()) == 0 {
		t.Fatal("stimulus: the fixture asks no /StructParent key, so no lookup reaches the unreadable node")
	}
	if why := d.parentTreeNothing(); !strings.Contains(why, "stopped short") {
		t.Errorf("a parent tree that stops at an unreadable /Nums answers the loop question %q, want the document refused", why)
	}
}
