package pdfops

import (
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestEveryFormClaimsItsKeyHoweverDeepItIsNested — /pending 766 part 8. `eachFormXObject` was a recursion cut at
// `maxFormDrawDepth` that said nothing, so a form nested past it was never offered: its `/StructParents` was no
// claim to the completeness gate, and `RemoveStructure` left it in place, naming a tree that was gone. The
// visited set is what bounds the walk, so every form is offered, however deep.
func TestEveryFormClaimsItsKeyHoweverDeepItIsNested(t *testing.T) {
	const key = 5
	for _, depth := range []int{maxFormDrawDepth, maxFormDrawDepth + 2, 99} {
		ctx := readForWalk(t, formNest(depth))
		leaf := 100 + depth - 1
		sd, _, err := ctx.DereferenceStreamDict(*types.NewIndirectRef(leaf, 0))
		if err != nil || sd == nil {
			t.Fatalf("setup: %d forms deep, the leaf (object %d) is not a stream: %v", depth, leaf, err)
		}
		sd.Dict["StructParents"] = types.Integer(key)

		pages := scanPages(ctx)
		if len(pages) != 1 || pages[0].res == nil {
			t.Fatalf("setup: want one page with resources, got %d", len(pages))
		}
		var offered []int
		eachFormXObject(ctx, pages[0].res, map[int]bool{}, func(nr int, _ *types.StreamDict) { offered = append(offered, nr) })
		if len(offered) != depth {
			t.Errorf("%d forms nested one in the next: the walk offered %d of them", depth, len(offered))
		}
		for i, nr := range offered {
			if nr != 100+i {
				t.Errorf("%d forms deep: offer %d is object %d, want %d — a form's own forms come before its siblings, outermost first", depth, i, nr, 100+i)
				break
			}
		}
		if owners := parentTreeOwners(ctx)[key]; len(owners) != 1 {
			t.Errorf("%d forms deep: the leaf's /StructParents %d has %d claimant(s) to the completeness gate, want 1: %v", depth, key, len(owners), owners)
		}
	}
}
