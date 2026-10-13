package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// rawNameTree is the depth and name count of the catalog name tree called kind as the BYTES hold it — read
// without the validator and without nib's own walk, so it measures what was written and not what a read made of it.
func rawNameTree(t *testing.T, pdf []byte, kind string) (depth, names int) {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := ctx.Catalog()
	nd, _ := ctx.DereferenceDict(root["Names"])
	type frame struct {
		o types.Object
		d int
	}
	stack := []frame{{nd[kind], 0}}
	for steps := 0; len(stack) > 0 && steps < 1<<22; steps++ {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n, _ := ctx.DereferenceDict(f.o)
		if n == nil {
			continue
		}
		depth = max(depth, f.d)
		if kids, _ := ctx.DereferenceArray(n["Kids"]); kids != nil {
			for _, k := range kids {
				stack = append(stack, frame{k, f.d + 1})
			}
			continue
		}
		a, _ := ctx.DereferenceArray(n["Names"])
		names += len(a) / 2
	}
	return depth, names
}

// keptNames is the keys the validated context holds for kind, in the tree's order.
func keptNames(ctx *model.Context, kind string) []string {
	var keys []string
	if n := ctx.Names[kind]; n != nil {
		_ = n.Process(ctx.XRefTable, func(_ *model.XRefTable, k string, _ *types.Object) error {
			keys = append(keys, k)
			return nil
		})
	}
	return keys
}

// TestATreeDeeperThanTheValidatorFollowsIsReadWhole is ADR-164's first half. pdfcpu's validator gives up 100
// nodes down and drops what it could not reach — the whole tree where the deep kid is the last, that kid where it
// is not — and says nothing. Each tree is 150 deep and holds one name at the bottom; with a sibling leaf under the
// root it holds two, and the validator alone keeps exactly the shallow one.
func TestATreeDeeperThanTheValidatorFollowsIsReadWhole(t *testing.T) {
	for _, kind := range []string{"EmbeddedFiles", "Dests", "JavaScript"} {
		for _, sibling := range []bool{false, true} {
			doc := testpdf.NameTreeDoc(testpdf.NameTree{Kind: kind, Depth: 150, Sibling: sibling})
			if depth, names := rawNameTree(t, doc, kind); depth != 151 || names == 0 {
				t.Fatalf("%s: the fixture is %d deep with %d names — want 151 deep", kind, depth, names)
			}
			ctx, err := pdfread.Validated(doc, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("%s sibling=%v: %v", kind, sibling, err)
			}
			want := []string{"deep.txt"}
			if sibling {
				want = append(want, "zz-shallow.txt")
			}
			if got := keptNames(ctx, kind); strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s sibling=%v: the read holds %v — want %v: a name the validator could not reach was "+
					"dropped and nothing said so", kind, sibling, got, want)
			}
			if pdfread.NameTreeUnread(ctx.XRefTable, kind) {
				t.Errorf("%s sibling=%v: marked unread, though every name was read", kind, sibling)
			}
			// And a write of that context carries it: the bytes hold the name, in a tree any reader walks.
			out, err := pdfread.Write(ctx)
			if err != nil {
				t.Fatalf("%s sibling=%v: write: %v", kind, sibling, err)
			}
			if depth, names := rawNameTree(t, out, kind); names != len(want) || depth > pdfread.MaxWrittenNameTreeDepth {
				t.Errorf("%s sibling=%v: written %d deep with %d names — want %d names at most %d deep",
					kind, sibling, depth, names, len(want), pdfread.MaxWrittenNameTreeDepth)
			}
		}
	}
}

// TestATreeTheValidatorDropsIsMarkedAndNotWritten is the second half: a tree nib's own walk cannot vouch for (its
// `/Kids` loop) is left to the validator, which drops it — and the context then says so. `Write` refuses it, a
// merge is refused it as a source, and only the removal of that very tree lets a write through.
func TestATreeTheValidatorDropsIsMarkedAndNotWritten(t *testing.T) {
	for _, kind := range []string{"EmbeddedFiles", "Dests", "JavaScript"} {
		doc := testpdf.NameTreeDoc(testpdf.NameTree{Kind: kind, Depth: 3, Loop: true})
		ctx, err := pdfread.Validated(doc, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if got := keptNames(ctx, kind); len(got) != 0 {
			t.Fatalf("%s: the validator kept %v of a looped tree — the fixture no longer shows the loss", kind, got)
		}
		if !pdfread.NameTreeUnread(ctx.XRefTable, kind) {
			t.Fatalf("%s: a tree holding a name the context does not is not marked unread", kind)
		}
		for _, other := range []string{"EmbeddedFiles", "Dests", "JavaScript"} {
			if other != kind && pdfread.NameTreeUnread(ctx.XRefTable, other) {
				t.Errorf("%s: %s is marked unread too", kind, other)
			}
		}
		if _, err := pdfread.Write(ctx); !errors.Is(err, pdfread.ErrNameTreeLost) {
			t.Errorf("%s: Write answered %v — want ErrNameTreeLost: the copy would be the document without the tree", kind, err)
		}
		if err := pdfread.RefuseNameTreeLoss(ctx); !errors.Is(err, pdfread.ErrNameTreeLost) {
			t.Errorf("%s: RefuseNameTreeLoss answered %v", kind, err)
		}
		host, _ := testpdf.Text("host")
		if _, err := pdfread.MergeRaw([][]byte{host, doc}); !errors.Is(err, pdfread.ErrNameTreeLost) {
			t.Errorf("%s: merged as a source: %v — want ErrNameTreeLost", kind, err)
		}
		pdfread.ForgetNameTree(ctx.XRefTable, kind)
		out, err := pdfread.Write(ctx)
		if err != nil {
			t.Fatalf("%s: after the tree's removal was declared, Write: %v", kind, err)
		}
		if _, names := rawNameTree(t, out, kind); names != 0 {
			t.Errorf("%s: the written copy holds %d names", kind, names)
		}
	}
	// A loop is met once and no more: the walk reads the one name and says it cannot vouch for the tree.
	loop := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: 3, Loop: true})
	raw, err := api.ReadContext(bytes.NewReader(loop), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if names, _, whole := pdfread.WalkNameTree(raw, "EmbeddedFiles"); names != 1 || whole {
		t.Errorf("a looped tree: the walk read %d names and whole=%v — want 1 and false", names, whole)
	}
	// A tree both deep AND looped is not given a new shape: only a tree read exactly is rewritten, so it is
	// left to the validator, dropped, and marked.
	deepLoop := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: 150, Loop: true})
	dctx, err := pdfread.Validated(deepLoop, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if !pdfread.NameTreeUnread(dctx.XRefTable, "EmbeddedFiles") {
		t.Error("a tree deep and looped is not marked unread")
	}
	// Past nib's own bound the walk stops, and the tree is marked whatever the validator kept: the count is
	// not known, so "nothing was lost" is not known either.
	six := testpdf.WithEmbedded(
		testpdf.Embedded{Key: "a", F: "a", UF: "a", Data: "1"}, testpdf.Embedded{Key: "b", F: "b", UF: "b", Data: "2"},
		testpdf.Embedded{Key: "c", F: "c", UF: "c", Data: "3"}, testpdf.Embedded{Key: "d", F: "d", UF: "d", Data: "4"},
		testpdf.Embedded{Key: "e", F: "e", UF: "e", Data: "5"}, testpdf.Embedded{Key: "f", F: "f", UF: "f", Data: "6"})
	was := *pdfread.MaxNameTreeNames
	*pdfread.MaxNameTreeNames = 5
	cctx, err := pdfread.Validated(six, model.NewDefaultConfiguration())
	*pdfread.MaxNameTreeNames = was
	if err != nil {
		t.Fatal(err)
	}
	if got := keptNames(cctx, "EmbeddedFiles"); len(got) != 6 {
		t.Fatalf("the validator kept %d of six names", len(got))
	}
	if !pdfread.NameTreeUnread(cctx.XRefTable, "EmbeddedFiles") {
		t.Error("a tree with more names than nib's walk reads is not marked unread")
	}
	// A document whose trees were all read is written as it always was.
	plain := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: 2})
	ctx, err := pdfread.Validated(plain, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if pdfread.NameTreeUnread(ctx.XRefTable, "EmbeddedFiles") {
		t.Error("a shallow tree read whole is marked unread")
	}
	if _, err := pdfread.Write(ctx); err != nil {
		t.Errorf("a shallow tree: %v", err)
	}
}

// TestNoWriteNestsANameTreePastWhatAReaderFollows is the third half. pdfcpu's `Node.Add` splits a full leaf in two
// and never rebalances: keys added in order make a chain a node deeper for every two keys, so the 204th file
// attached through nib made a tree 101 deep that nib's own next read dropped whole. The door rebuilds it.
func TestNoWriteNestsANameTreePastWhatAReaderFollows(t *testing.T) {
	base, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	for _, total := range []int{203, 204, 1000, 10000} {
		for _, order := range []string{"sorted", "reverse", "random"} {
			if testing.Short() && total > 1000 {
				continue
			}
			keys := make([]string, total)
			for i := range keys {
				keys[i] = fmt.Sprintf("f%06d.txt", i)
			}
			switch order {
			case "reverse":
				for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
					keys[i], keys[j] = keys[j], keys[i]
				}
			case "random":
				rand.New(rand.NewSource(1)).Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
			}
			ctx, err := pdfread.ReadOptimized(base, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			for _, k := range keys {
				if err := ctx.AddAttachment(model.Attachment{Reader: strings.NewReader(k), ID: k}, false); err != nil {
					t.Fatal(err)
				}
			}
			before := keptNames(ctx, "EmbeddedFiles")
			out, err := pdfread.Write(ctx)
			if err != nil {
				t.Fatal(err)
			}
			depth, names := rawNameTree(t, out, "EmbeddedFiles")
			if names != total || depth > pdfread.MaxWrittenNameTreeDepth {
				t.Errorf("%d %s: written %d deep with %d names — want all %d at most %d deep",
					total, order, depth, names, total, pdfread.MaxWrittenNameTreeDepth)
			}
			// Read again with pdfcpu's validator ALONE — an older nib, or pdfcpu — every name is there, in order.
			plain, err := api.ReadContext(bytes.NewReader(out), model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			if err := api.ValidateContext(plain); err != nil {
				t.Fatal(err)
			}
			if after := keptNames(plain, "EmbeddedFiles"); strings.Join(after, "\n") != strings.Join(before, "\n") {
				t.Errorf("%d %s: pdfcpu's own read of the written copy holds %d names, the context held %d — or in another order",
					total, order, len(after), len(before))
			}
		}
	}
}
