package pdfops

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The `/ParentTree` writers' residue, /pending 836 — each a row operation that read the tree some other way than a
// reader does (`parentTreeLookup`), on input only a malformed or unusual producer writes.

// TestANewRowGoesBeforeALargerIndirectKey — `insertNum` ordered by DIRECT integer keys only, while every reader takes
// an indirect key (`numsKey`): a row for key 0 was appended after `30 0 R`, which is 1, and the keys were out of order.
func TestANewRowGoesBeforeALargerIndirectKey(t *testing.T) {
	ctx, tree := ptDoc(t, "/StructParents 0", "/StructParents 1", map[int]string{
		24: "<< /Nums [30 0 R [22 0 R]] >>", 30: "1",
	})
	if _, at := parentTreeLookup(ctx, tree.root["ParentTree"], 1); at != 1 {
		t.Fatalf("setup: the indirect key 1 is not read (value index %d), so its place proves nothing", at)
	}
	if _, err := addMCIDTo(ctx, tree, 1, *types.NewIndirectRef(21, 0)); err != nil {
		t.Fatal(err)
	}
	nums, _ := derefDict(ctx.XRefTable, *types.NewIndirectRef(24, 0))["Nums"].(types.Array)
	var keys []int
	for i := 0; i+1 < len(nums); i += 2 {
		k, _ := numsKey(ctx, nums[i])
		keys = append(keys, k)
	}
	if fmt.Sprint(keys) != "[0 1]" {
		t.Errorf("the keys read %v, want [0 1] — a number tree's keys ascend (ISO 32000-1 §7.9.7), and the new "+
			"row for key 0 belongs before the indirect key 1", keys)
	}
}

// TestARenumberedTreeIsReadAsAReaderReadIt — `renumberParentTree` read the root's `/Nums` for itself: direct integer
// keys only, past the root's `/Limits`, and it left those `/Limits` over keys it had just replaced.
func TestARenumberedTreeIsReadAsAReaderReadIt(t *testing.T) {
	for _, c := range []struct {
		name, pt string
		extra    map[int]string
		claims   string // each page's /StructParents afterwards, "<nil>" where the claim is deleted
		rows     string // the rebuilt /Nums
	}{
		{"bounds over the old keys do not hide the new ones",
			"<< /Limits [5 6] /Nums [5 [21 0 R] 6 [22 0 R]] >>", nil, "0 1", "[0 [(21 0 R)] 1 [(22 0 R)]]"},
		{"a row under an indirect key is kept",
			"<< /Nums [30 0 R [21 0 R] 6 [22 0 R]] >>", map[int]string{30: "5"}, "0 1", "[0 [(21 0 R)] 1 [(22 0 R)]]"},
		{"a row no reader reaches is not carried",
			"<< /Limits [6 6] /Nums [5 [21 0 R] 6 [22 0 R]] >>", nil, "<nil> 0", "[0 [(22 0 R)]]"},
	} {
		objs := map[int]string{24: c.pt}
		for k, v := range c.extra {
			objs[k] = v
		}
		ctx, tree := ptDoc(t, "/StructParents 5", "/StructParents 6", objs)
		if err := renumberParentTree(ctx, tree); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		pt := derefDict(ctx.XRefTable, tree.root["ParentTree"])
		if got := fmt.Sprint(pt["Nums"]); got != c.rows {
			t.Errorf("%s: /Nums is %s, want %s", c.name, got, c.rows)
		}
		claims := fmt.Sprint(pageAt(ctx, nil, 1).Dict["StructParents"], pageAt(ctx, nil, 2).Dict["StructParents"])
		if claims != c.claims {
			t.Errorf("%s: the pages claim %q, want %q", c.name, claims, c.claims)
		}
		// Every claim left resolves for a reader: the root's bounds, if any, span the new keys.
		for p := 1; p <= 2; p++ {
			key, isKey, written := structParentsOf(ctx.XRefTable, pageAt(ctx, nil, p).Dict)
			if !written {
				continue
			}
			if h, _ := parentTreeLookup(ctx, tree.root["ParentTree"], key); !isKey || h == nil {
				t.Errorf("%s: page %d claims key %d and a reader finds no row for it (/Limits %v)", c.name, p, key, pt["Limits"])
			}
		}
	}
}

// TestAGraftedRowIsInsideTheHostsLimits — a merge appended the source's rows to the host root's `/Nums` and left the
// host root's `/Limits` as they were, so a reader honouring them found the host's rows and none of the grafted ones.
func TestAGraftedRowIsInsideTheHostsLimits(t *testing.T) {
	src := taggedFixture()
	host, err := writeMutated(src, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		root := derefDict(ctx.XRefTable, cat["StructTreeRoot"])
		derefDict(ctx.XRefTable, root["ParentTree"])["Limits"] = types.Array{types.Integer(0), types.Integer(0)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if xt, root := structRoot(t, host); derefDict(xt, root["ParentTree"])["Limits"] == nil {
		t.Fatal("setup: the host's /ParentTree root lost its /Limits on the way out, so the merge is of an ordinary host")
	}
	out, err := Append(host, src)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := ctx.XRefTable.Catalog()
	root := derefDict(ctx.XRefTable, cat["StructTreeRoot"])
	if root == nil {
		t.Fatal("the merge carried no tree, so there is no grafted row to look for")
	}
	claims := pageClaims(t, out)
	if len(claims) != 2 || claims[0] == claims[1] || claims[1] < 0 {
		t.Fatalf("the pages claim %v, want two keys — the second page's is the grafted one", claims)
	}
	for p, key := range claims {
		if h, _ := parentTreeLookup(ctx, root["ParentTree"], key); h == nil {
			t.Errorf("page %d claims key %d and a reader finds no row for it: the root's /Limits are %v", p+1, key,
				derefDict(ctx.XRefTable, root["ParentTree"])["Limits"])
		}
	}
}
