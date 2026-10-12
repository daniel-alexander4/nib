package pdfops

import (
	"fmt"
	"strings"
	"testing"
)

// `/pending 653`: a batch of edits read the structure tree again before EVERY edit and once more at the end.
//
// Measured before the change on the producers corpus, `alt` edits in one batch, best of three, 1 / 10 / 50 edits:
//
//	indesign/census-p60-280.pdf   34,802 elements   1.96 s   4.61 s   23.8 s
//	antennahouse/irs-p15.pdf       6,892 elements   1.37 s   2.23 s    5.89 s
//	word/fda-82395.pdf             1,458 elements   0.34 s   0.52 s    1.20 s
//	acrobat/fda-170357.pdf         2,064 elements   0.24 s   0.33 s    0.64 s
//
// (one read of the census tree alone: 0.22 s). After it, on a busier machine: 3.9 / 3.7 / 3.2 s, 0.98 / 0.96 /
// 0.93 s, 0.33 / 0.41 / 0.31 s, 0.68 / 0.56 / 0.63 s — flat in the number of edits, for the one kind of edit that
// leaves the tree as it was read. A batch of any other kind reads as often as it did.

// manyParagraphs is a tree of n paragraphs under one section, numbered from firstParagraph.
const firstParagraph = 100

func manyParagraphs(n int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		4: "<< /Length 1 >>\nstream\n \nendstream",
		7: "<< /Type /StructTreeRoot /K [8 0 R] >>",
	}
	var kids strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", firstParagraph+i)
		objs[firstParagraph+i] = "<< /Type /StructElem /S /P /P 8 0 R >>"
	}
	objs[8] = "<< /Type /StructElem /S /Sect /P 7 0 R /K [" + kids.String() + "] >>"
	return assembleFixture(objs)
}

// TestABatchOfDescriptionsReadsTheTreeOnce — fifty `alt` edits cost what one does, not a tree apiece. Read by what
// the batch allocates: a tree of 4,000 elements is most of what editing this fixture allocates at all.
func TestABatchOfDescriptionsReadsTheTreeOnce(t *testing.T) {
	const paragraphs, many = 4000, 50
	src := manyParagraphs(paragraphs)
	batch := func(n int) []structEdit {
		edits := make([]structEdit, n)
		for i := range edits {
			edits[i] = structEdit{kind: editAlt, elem: firstParagraph + i*(paragraphs/n), value: "described"}
		}
		return edits
	}
	var out []byte
	var err error
	one := allocated(func() { _, err = applyStructEdits(src, batch(1)) })
	if err != nil {
		t.Fatal(err)
	}
	fifty := allocated(func() { out, err = applyStructEdits(src, batch(many)) })
	if err != nil {
		t.Fatal(err)
	}
	if fifty > one+one/2 {
		t.Errorf("a batch of %d alt edits allocated %d KiB against %d KiB for one — the tree is read again for each, "+
			"and an alt edit changes nothing the tree holds", many, fifty>>10, one>>10)
	}
	// And the batch did what it says: each named element is described, and no other.
	tree, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	described := 0
	for _, e := range tree.Elements {
		if e.HasAlt && e.Alt == "described" {
			described++
		}
	}
	if described != many {
		t.Errorf("%d elements carry the description after a batch of %d", described, many)
	}
}

// TestAnEditAfterOneThatChangesTheTreeReadsItAgain — the other half: only an `alt` edit leaves the tree as read. A
// scope is refused on anything but a header cell, by the type the TREE holds, so a retype before it in the same
// batch is seen only through a second read; and a move resolves its parent through links a move before it changed.
func TestAnEditAfterOneThatChangesTheTreeReadsItAgain(t *testing.T) {
	out, err := applyStructEdits(editFixture(),
		[]structEdit{{kind: editAlt, elem: 10, value: "first"}, {kind: editRetype, elem: 10, value: "TH"},
			{kind: editScope, elem: 10, value: "Row"}, {kind: editAlt, elem: 10, value: "last"}})
	if err != nil {
		t.Fatalf("a retype to TH and then a scope, in one batch: %v — the scope read the type the tree held before the retype", err)
	}
	tree, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tree.Elements {
		if e.ID == 10 && (e.Kind != "TH" || e.Scope != "Row" || e.Alt != "last") {
			t.Errorf("element 10 is a %s with scope %q and description %q, want TH, Row, last", e.Kind, e.Scope, e.Alt)
		}
	}
	// 11 is 10's kid: moved out under 8, 10 may then move under it — refused as "inside its own kid" by a tree
	// read before the first move.
	if _, err := applyStructEdits(editFixture(),
		[]structEdit{{kind: editMove, elem: 11, parent: 8, index: -1}, {kind: editMove, elem: 10, parent: 11, index: -1}}); err != nil {
		t.Errorf("a move under an element the move before it took out from under this one: %v", err)
	}
}
