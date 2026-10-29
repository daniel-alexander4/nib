package pdfops

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// xrefTableOf is an xref table holding objs, each referenced refs[nr] times (1 where unsaid) — built in memory, so a
// chain of any length costs no parse.
func xrefTableOf(objs map[int]types.Object, refs map[int]int) *model.XRefTable {
	zero, gen := int64(0), 65535
	xt := &model.XRefTable{Table: map[int]*model.XRefTableEntry{0: {Free: true, Offset: &zero, Generation: &gen}}}
	size := 1
	for nr, o := range objs {
		g, n := 0, 1
		if c, said := refs[nr]; said {
			n = c
		}
		xt.Table[nr] = &model.XRefTableEntry{Object: o, Generation: &g, RefCount: n}
		size = max(size, nr+1)
	}
	xt.Size = &size
	return xt
}

func ref(nr int) types.IndirectRef { return *types.NewIndirectRef(nr, 0) }

// chainUnder is n actions, each naming the next by /Next, and the dictionary whose /A names the first.
func chainUnder(n int) (*model.XRefTable, types.Dict) {
	objs := make(map[int]types.Object, n)
	for i := 1; i <= n; i++ {
		d := types.Dict{"S": types.Name("JavaScript")}
		if i < n {
			d["Next"] = ref(i + 1)
		}
		objs[i] = d
	}
	return xrefTableOf(objs, nil), types.Dict{"A": ref(1)}
}

// TestDroppingAKeyDoesNotRecurseDownAChain — pdfcpu's `DeleteObject` calls itself once per reference, so the stack
// `dropKey` needed was the length of the chain under the key: measured 32 MiB for 100,000 links, and Go ends the
// process, not the request, at 1 GB. The chain here is long enough to show the growth and far short of that.
func TestDroppingAKeyDoesNotRecurseDownAChain(t *testing.T) {
	const links = 100_000
	xt, holder := chainUnder(links)
	grew := make(chan uint64)
	go func() { // a goroutine of its own: its stack is what is measured, before it can shrink
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		dropKey(xt, holder, "A")
		runtime.ReadMemStats(&after)
		grew <- after.StackInuse - min(before.StackInuse, after.StackInuse)
	}()
	if g := <-grew; g > 1<<20 {
		t.Errorf("dropping a key over a chain of %d objects grew the stack by %d KiB, want under 1024 — the walk "+
			"recurses once per link, and a long enough chain ends the process", links, g>>10)
	}
	if _, kept := holder["A"]; kept {
		t.Error("the key is still there")
	}
	for i := 1; i <= links; i++ {
		if !xt.Table[i].Free {
			t.Fatalf("object %d of the chain was not freed", i)
		}
	}
}

// TestFreeGraphFreesWhatPdfcpuFrees — the walk is pdfcpu's `DeleteObject` without the recursion, so over the shapes
// that decide anything it leaves the table as `DeleteObject` leaves it: shared objects counted down and kept, a
// cycle ended, a stream's dictionary followed, direct containers entered.
func TestFreeGraphFreesWhatPdfcpuFrees(t *testing.T) {
	build := func() (*model.XRefTable, types.Object) {
		objs := map[int]types.Object{
			1: types.Dict{"Next": ref(2), "Kids": types.Array{ref(3), types.Array{ref(4)}, types.Dict{"X": ref(5)}}},
			2: types.Dict{"Back": ref(1), "Shared": ref(6)}, // a cycle back to 1, and one of 6's two references
			3: types.StreamDict{Dict: types.Dict{"Font": ref(7)}},
			4: types.Dict{"Shared": ref(6)}, // the other reference to 6: whichever comes second frees it
			5: types.Integer(5),
			6: types.Dict{"Deep": ref(8)},
			7: types.Dict{"Page": ref(9)}, // 9 is referenced from elsewhere too, and stays
			8: types.Name("leaf"),
			9: types.Dict{"Type": types.Name("Page")},
		}
		return xrefTableOf(objs, map[int]int{6: 2, 9: 2}), ref(1)
	}
	state := func(xt *model.XRefTable) string {
		var nrs []int
		for nr := range xt.Table {
			nrs = append(nrs, nr)
		}
		sort.Ints(nrs)
		s := ""
		for _, nr := range nrs[1:] {
			s += fmt.Sprintf("%d:free=%v,refs=%d ", nr, xt.Table[nr].Free, xt.Table[nr].RefCount)
		}
		return s
	}
	theirs, o := build()
	if err := theirs.DeleteObject(context.Background(), o); err != nil {
		t.Fatalf("setup: pdfcpu's DeleteObject: %v", err)
	}
	ours, o := build()
	freeGraph(ours, o)
	want, got := state(theirs), state(ours)
	const kept = "9:free=false,refs=1 "
	if len(want) < len(kept) || want[len(want)-len(kept):] != kept {
		t.Fatalf("setup: pdfcpu left %q, want every object freed but the shared page", want)
	}
	if got != want {
		t.Errorf("freeGraph left the table as\n  %s\npdfcpu's DeleteObject leaves it as\n  %s", got, want)
	}
}
