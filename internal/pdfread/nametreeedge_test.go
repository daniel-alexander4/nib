package pdfread_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// heldTreeDepth is how many nodes deep the catalog name tree kind is in ctx as the read left it — the dictionaries
// themselves, so a tree `settleNameTrees` flattened for the validator reads 1.
func heldTreeDepth(t *testing.T, ctx *model.Context, kind string) int {
	t.Helper()
	root, err := ctx.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	names, _ := ctx.DereferenceDict(root["Names"])
	type frame struct {
		o types.Object
		d int
	}
	deepest := 0
	stack := []frame{{names[kind], 1}}
	for steps := 0; len(stack) > 0 && steps < 1<<16; steps++ {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n, _ := ctx.DereferenceDict(f.o)
		if n == nil {
			continue
		}
		deepest = max(deepest, f.d)
		kids, _ := ctx.DereferenceArray(n["Kids"])
		for _, k := range kids {
			stack = append(stack, frame{k, f.d + 1})
		}
	}
	return deepest
}

// TestATreeIsFlattenedForTheValidatorOnlyPastItsDepth — P02 red-proof survivor 12. The validator follows 100 nodes,
// so a catalog tree 100 deep is left as the file wrote it and one 101 deep is made flat. Both sides of the edge are
// held: flattening at 100 reshapes a tree the validator reads whole, and flattening only past 101 hands it a tree it
// then refuses or drops.
func TestATreeIsFlattenedForTheValidatorOnlyPastItsDepth(t *testing.T) {
	for _, c := range []struct {
		between int // NameTree.Depth: the leaf sits between+1 below the root
		flat    bool
	}{{98, false}, {99, false}, {100, true}, {101, true}} {
		doc := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: c.between})
		if depth, _ := rawNameTree(t, doc, "EmbeddedFiles"); depth != c.between+1 {
			t.Fatalf("setup: the fixture is %d deep, want %d", depth, c.between+1)
		}
		ctx, err := pdfread.Validated(doc, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%d between: %v", c.between, err)
		}
		if got := keptNames(ctx, "EmbeddedFiles"); len(got) != 1 {
			t.Errorf("%d between: the read holds %v, want the one name", c.between, got)
		}
		held := heldTreeDepth(t, ctx, "EmbeddedFiles")
		if c.flat && held != 1 {
			t.Errorf("%d between (%d deep): the tree the validator was handed is %d deep, want it made flat", c.between, c.between+2, held)
		}
		if !c.flat && held != c.between+2 {
			t.Errorf("%d between (%d deep): the tree the validator was handed is %d deep, want it left as written", c.between, c.between+2, held)
		}
	}
}

// TestADeepIDTreeTheWalkCouldNotReadWholeIsNotFlattened — P02 red-proof survivor 11. A walk stopped at the names
// cap holds a PART of the tree; flattening to that part would write the rest out of the document. The ids it did
// not count must still be in what is written (v0.16.2 refuses such a tree outright, so a refusal is the other accepted answer).
func TestADeepIDTreeTheWalkCouldNotReadWholeIsNotFlattened(t *testing.T) {
	pdf, n := idTreeDoc(150, false)
	old := *pdfread.MaxNameTreeNames
	*pdfread.MaxNameTreeNames = 2
	defer func() { *pdfread.MaxNameTreeNames = old }()
	if n <= 2 {
		t.Fatalf("setup: the fixture holds %d ids, want more than the cap of 2", n)
	}
	// v0.16.2 refuses a tree this deep that nib left to it, and that refusal is the right answer here: what must not
	// happen is a document that reads and writes with fewer ids than it held.
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		if !strings.Contains(err.Error(), "max recursion depth") {
			t.Fatalf("the refusal is not the validator's depth one: %v", err)
		}
		return
	}
	out, err := pdfread.Write(ctx)
	if err != nil {
		t.Fatal(err)
	}
	back, err := api.ReadContext(context.Background(), bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if got := inspectIDTree(back); got.entries != n {
		t.Errorf("the written tree holds %d of %d ids after a walk that stopped at the cap: %v", got.entries, n, got)
	}
}
