package pdfops

import (
	"fmt"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The `/ParentTree` lookup door, `parentTreeLookup` — `/pending 786`. Every writer acts on the pair it names and every
// reader reads that pair, and the pair is the one uacheck (`uacheck/structure.go`, `parentTreeEntry`) and veraPDF read.

// nthNums is the value of the n-th pair (0-based) of object nr's /Nums, as text.
func nthNums(t *testing.T, ctxObj func(int) types.Dict, nr, n int) string {
	t.Helper()
	nums, _ := ctxObj(nr)["Nums"].(types.Array)
	if 2*n+1 >= len(nums) {
		t.Fatalf("object %d's /Nums has no pair %d: %v", nr, n, nums)
	}
	return fmt.Sprint(nums[2*n+1])
}

// TestARepeatedKeyIsWrittenInThePairThatIsRead — a flat `/Nums` naming key 0 twice. A reader takes the LAST pair (one
// node's /Nums is a map); the writers took the FIRST, so `addMCIDTo` sized the MCID from the last pair (2) and wrote it
// into the first, `0 [21 null 21]`, where no reader looks: the new MCID was unreachable.
func TestARepeatedKeyIsWrittenInThePairThatIsRead(t *testing.T) {
	ctx, tree := ptDoc(t, "/StructParents 0", "/StructParents 1", map[int]string{
		24: "<< /Nums [0 [21 0 R] 0 [21 0 R 21 0 R] 1 [22 0 R]] >>",
	})
	obj := func(nr int) types.Dict { return derefDict(ctx.XRefTable, *types.NewIndirectRef(nr, 0)) }
	mcid, err := addMCIDTo(ctx, tree, 1, *types.NewIndirectRef(21, 0))
	if err != nil || mcid != 2 {
		t.Fatalf("addMCIDTo: MCID %d (%v), want 2 — the next slot of the pair a reader reads", mcid, err)
	}
	if got := nthNums(t, obj, 24, 0); got != "[(21 0 R)]" {
		t.Errorf("the shadowed first pair was written: %s", got)
	}
	if got := nthNums(t, obj, 24, 1); got != "[(21 0 R) (21 0 R) (21 0 R)]" {
		t.Errorf("the pair a reader reads is %s, want the new MCID 2 in it", got)
	}
	if row, _, _ := rowFor(ctx, tree, 0); len(row) != 3 {
		t.Errorf("rowFor reads %d slots for key 0, want the 3 just written", len(row))
	}
	if arrays, _ := parentTreeEntries(ctx, tree); len(arrays[0]) != 3 {
		t.Errorf("parentTreeEntries reads %d slots for key 0, want 3", len(arrays[0]))
	}

	if err := clearParentTreeSlot(ctx, tree, 0, 0); err != nil {
		t.Fatal(err)
	}
	if got := nthNums(t, obj, 24, 0); got != "[(21 0 R)]" {
		t.Errorf("clearing emptied the shadowed first pair: %s", got)
	}
	if got := nthNums(t, obj, 24, 1); got != "[null (21 0 R) (21 0 R)]" {
		t.Errorf("clearing slot 0 left the pair a reader reads as %s", got)
	}
}

// TestTheHolderIsTheNodeAReaderSearches — the holder walk took the first node with the key anywhere, ignoring `/Limits`
// and reading a `/Nums` node's `/Kids`; a reader skips a node whose /Limits exclude the key and answers a /Nums node
// from its /Nums alone. Each case adds an MCID for page 1 and asks which node took it.
func TestTheHolderIsTheNodeAReaderSearches(t *testing.T) {
	for _, c := range []struct {
		name string
		objs map[int]string
		// want is the object whose /Nums gains the row the reader then reads, as row; other is left alone.
		want, other int
		mcid        int
		row         string
	}{
		{"a leaf whose /Limits exclude the key is passed over", map[int]string{
			24: "<< /Kids [25 0 R 26 0 R] >>",
			25: "<< /Limits [1 1] /Nums [0 [22 0 R]] >>",
			26: "<< /Limits [0 1] /Nums [0 [21 0 R] 1 [22 0 R]] >>",
		}, 26, 25, 1, "[(21 0 R) (21 0 R)]"},
		{"a /Nums node's /Kids are never read", map[int]string{
			24: "<< /Nums [1 [22 0 R]] /Kids [25 0 R] >>",
			25: "<< /Limits [0 0] /Nums [0 [21 0 R]] >>",
		}, 24, 25, 0, "[(21 0 R)]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, tree := ptDoc(t, "/StructParents 0", "/StructParents 1", c.objs)
			obj := func(nr int) types.Dict { return derefDict(ctx.XRefTable, *types.NewIndirectRef(nr, 0)) }
			before := fmt.Sprint(obj(c.other)["Nums"])
			mcid, err := addMCIDTo(ctx, tree, 1, *types.NewIndirectRef(21, 0))
			if err != nil || mcid != c.mcid {
				t.Fatalf("addMCIDTo: MCID %d (%v), want %d", mcid, err, c.mcid)
			}
			if got := fmt.Sprint(obj(c.other)["Nums"]); got != before {
				t.Errorf("object %d, which no reader searches for key 0, was written: %s", c.other, got)
			}
			if got := nthNums(t, obj, c.want, 0); got != c.row {
				t.Errorf("object %d's row for key 0 is %s, want %s", c.want, got, c.row)
			}
			if row, _, _ := rowFor(ctx, tree, 0); fmt.Sprint(row) != c.row {
				t.Errorf("rowFor reads %v, want %s", row, c.row)
			}
		})
	}
}

// TestTheConsistencyReaderReadsTheRowALookupFinds — `parentTreeEntries` (what `checkStructConsistency` and the
// completeness predicate judge) took the last pair met in a walk of every node, so a later node whose /Limits hide the
// key overrode the row every reader and writer uses.
func TestTheConsistencyReaderReadsTheRowALookupFinds(t *testing.T) {
	ctx, tree := ptDoc(t, "/StructParents 0", "/StructParents 1", map[int]string{
		24: "<< /Kids [25 0 R 26 0 R] >>",
		25: "<< /Limits [0 1] /Nums [0 [21 0 R] 1 [22 0 R]] >>",
		26: "<< /Limits [5 5] /Nums [0 [22 0 R 22 0 R 22 0 R]] >>",
	})
	arrays, _ := parentTreeEntries(ctx, tree)
	if got := fmt.Sprint(arrays[0]); got != "[21]" {
		t.Errorf("key 0 reads as %s, want [21] — the row in object 25, which a lookup finds", got)
	}
}

// TestAKeyARootsLimitsHideGetsTheRowAReaderThenReads — a flat tree whose root `/Limits` exclude key 0, which its /Nums
// holds: no reader reaches that pair, so the writer starts a new one AFTER it and widens the /Limits, and the new pair is
// the one every reader then takes. Writing into the hidden pair would size the MCID from nothing and write slot 0 of a
// row other content already fills.
func TestAKeyARootsLimitsHideGetsTheRowAReaderThenReads(t *testing.T) {
	ctx, tree := ptDoc(t, "/StructParents 0", "/StructParents 1", map[int]string{
		24: "<< /Limits [1 1] /Nums [0 [22 0 R] 1 [22 0 R]] >>",
	})
	obj := func(nr int) types.Dict { return derefDict(ctx.XRefTable, *types.NewIndirectRef(nr, 0)) }
	mcid, err := addMCIDTo(ctx, tree, 1, *types.NewIndirectRef(21, 0))
	if err != nil || mcid != 0 {
		t.Fatalf("addMCIDTo: MCID %d (%v), want 0 — no reader reaches a row for key 0", mcid, err)
	}
	if got := nthNums(t, obj, 24, 0); got != "[(22 0 R)]" {
		t.Errorf("the hidden pair was written: %s", got)
	}
	if got := nthNums(t, obj, 24, 1); got != "[(21 0 R)]" {
		t.Errorf("the new pair is %s, want MCID 0 naming element 21", got)
	}
	if got := fmt.Sprint(obj(24)["Limits"]); got != "[0 1]" {
		t.Errorf("the root's /Limits are %s, want [0 1]", got)
	}
	if row, _, _ := rowFor(ctx, tree, 0); fmt.Sprint(row) != "[(21 0 R)]" {
		t.Errorf("rowFor reads %v, want the new pair", row)
	}
}
