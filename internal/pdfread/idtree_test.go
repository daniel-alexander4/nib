package pdfread_test

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

func idTreeFile(objs map[int]string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	maxN := 0
	for n := range objs {
		maxN = max(maxN, n)
	}
	offs := map[int]int{}
	for n := 1; n <= maxN; n++ {
		if body, ok := objs[n]; ok {
			offs[n] = b.Len()
			fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
		}
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

// idTreeDoc is a tagged one-page PDF whose /StructTreeRoot /IDTree nests `depth` intermediate nodes under the root.
// split=false: a bare chain, one kid per node, one leaf of 3 keys at the bottom.
// split=true: as pdfcpu's Node.Add nests sorted keys: every node holds [leaf of 2 keys, next node]; the last a leaf of 2.
func idTreeDoc(depth int, split bool) (pdf []byte, nkeys int) {
	objs := map[int]string{
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
	}
	next := 10
	var elems []string
	elem := func(id string) string {
		n := next
		next++
		objs[n] = fmt.Sprintf("<< /Type /StructElem /S /P /P 5 0 R /ID (%s) >>", id)
		elems = append(elems, fmt.Sprintf("%d 0 R", n))
		return fmt.Sprintf("(%s) %d 0 R", id, n)
	}
	keyN := 0
	key := func() string { keyN++; return fmt.Sprintf("id%05d", keyN) }
	leafObj := func(n int) (int, string, string) {
		var parts []string
		var first, last string
		for i := 0; i < n; i++ {
			k := key()
			if first == "" {
				first = k
			}
			last = k
			parts = append(parts, elem(k))
		}
		o := next
		next++
		objs[o] = fmt.Sprintf("<< /Limits [(%s) (%s)] /Names [%s] >>", first, last, strings.Join(parts, " "))
		return o, first, last
	}
	// build top-down: need key order ascending in document order, so allocate leaves level by level.
	type lvl struct {
		leaf        int
		first, last string
	}
	var sideLeaves []lvl
	if split {
		for i := 0; i <= depth; i++ { // root + each intermediate has a side leaf
			o, f, l := leafObj(2)
			sideLeaves = append(sideLeaves, lvl{o, f, l})
		}
	}
	bottomN := 3
	if split {
		bottomN = 2
	}
	bottom, bf, bl := leafObj(bottomN)
	below, lo, hi := bottom, bf, bl
	for i := depth; i >= 1; i-- {
		o := next
		next++
		if split {
			s := sideLeaves[i]
			lo = s.first
			objs[o] = fmt.Sprintf("<< /Limits [(%s) (%s)] /Kids [%d 0 R %d 0 R] >>", lo, hi, s.leaf, below)
		} else {
			objs[o] = fmt.Sprintf("<< /Limits [(%s) (%s)] /Kids [%d 0 R] >>", lo, hi, below)
		}
		below = o
	}
	if split {
		objs[6] = fmt.Sprintf("<< /Kids [%d 0 R %d 0 R] >>", sideLeaves[0].leaf, below)
	} else {
		objs[6] = fmt.Sprintf("<< /Kids [%d 0 R] >>", below)
	}
	objs[5] = fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] /IDTree 6 0 R >>", strings.Join(elems, " "))
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 5 0 R >>"
	return idTreeFile(objs), keyN
}

type idTreeStat struct {
	present                               bool
	entries, nodes, maxDepth              int
	limOK, limMissing, limEmpty, limWrong int
	rootHasLimits                         bool
	dangling                              int // values that do not resolve to a StructElem whose /ID is the key
	samples                               []string
}

func (s idTreeStat) String() string {
	if !s.present {
		return "NO /IDTree"
	}
	return fmt.Sprintf("entries=%d nodes(non-root)=%d depth=%d limits: ok=%d missing=%d empty-string=%d wrong=%d rootHasLimits=%v badValues=%d samples=%v",
		s.entries, s.nodes, s.maxDepth, s.limOK, s.limMissing, s.limEmpty, s.limWrong, s.rootHasLimits, s.dangling, s.samples)
}

// inspectIDTree reads the /IDTree as ctx holds it, node by node, by a loop.
func inspectIDTree(ctx *model.Context) idTreeStat {
	var st idTreeStat
	root, _ := ctx.Catalog()
	str, _ := ctx.DereferenceDict(root["StructTreeRoot"])
	if str == nil {
		return st
	}
	idt, _ := ctx.DereferenceDict(str["IDTree"])
	if idt == nil {
		return st
	}
	st.present = true
	_, st.rootHasLimits = idt["Limits"]
	// post-order via explicit stack: compute first/last key under each node
	type frame struct {
		d     types.Dict
		depth int
		kids  []types.Dict
		i     int
		keys  []string
	}
	str1 := func(o types.Object) (string, bool) {
		o, _ = ctx.Dereference(o)
		s, err := types.StringOrHexLiteral(o)
		if err != nil || s == nil {
			return "", false
		}
		return *s, true
	}
	open := func(d types.Dict, depth int) *frame {
		f := &frame{d: d, depth: depth}
		if depth > st.maxDepth {
			st.maxDepth = depth
		}
		if a, _ := ctx.DereferenceArray(d["Kids"]); a != nil {
			for _, k := range a {
				kd, _ := ctx.DereferenceDict(k)
				if kd != nil {
					f.kids = append(f.kids, kd)
				}
			}
		} else if a, _ := ctx.DereferenceArray(d["Names"]); a != nil {
			for i := 0; i+1 < len(a); i += 2 {
				k, _ := str1(a[i])
				f.keys = append(f.keys, k)
				st.entries++
				ed, _ := ctx.DereferenceDict(a[i+1])
				id, _ := str1(ed["ID"])
				if ed == nil || id != k {
					st.dangling++
				}
			}
		}
		return f
	}
	stack := []*frame{open(idt, 0)}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		if f.i < len(f.kids) {
			f.i++
			stack = append(stack, open(f.kids[f.i-1], f.depth+1))
			continue
		}
		stack = stack[:len(stack)-1]
		if len(stack) > 0 {
			p := stack[len(stack)-1]
			p.keys = append(p.keys, f.keys...)
		}
		if f.depth == 0 {
			continue
		}
		st.nodes++
		sort.Strings(f.keys)
		wantLo, wantHi := "", ""
		if len(f.keys) > 0 {
			wantLo, wantHi = f.keys[0], f.keys[len(f.keys)-1]
		}
		la, _ := ctx.DereferenceArray(f.d["Limits"])
		switch {
		case la == nil || len(la) != 2:
			st.limMissing++
			if len(st.samples) < 4 {
				st.samples = append(st.samples, fmt.Sprintf("d%d:missing(want %s..%s)", f.depth, wantLo, wantHi))
			}
		default:
			lo, _ := str1(la[0])
			hi, _ := str1(la[1])
			if lo == wantLo && hi == wantHi {
				st.limOK++
				break
			}
			if lo == "" || hi == "" {
				st.limEmpty++
			} else {
				st.limWrong++
			}
			if len(st.samples) < 4 {
				st.samples = append(st.samples, fmt.Sprintf("d%d:[%q %q](want %s..%s)", f.depth, lo, hi, wantLo, wantHi))
			}
		}
	}
	return st
}

// TestADeepIDTreeKeepsItsLimits — `/pending 883`, ADR-164's rule reaching the one name tree that is not under
// the catalog's `/Names`. pdfcpu's relaxed validator follows a structure tree's `/IDTree` 100 levels, and past
// that it does not stop: it WRITES empty strings into the `/Limits` of the 100 nodes it did reach, in the
// document's own arrays, and every later write carried them out (measured: 100 emptied of 151, and of 302). A
// reader that finds an id by `/Limits` — a cell's `/Headers` — then finds none. `settleIDTree` makes such a tree
// flat before the validator sees it.
func TestADeepIDTreeKeepsItsLimits(t *testing.T) {
	for _, split := range []bool{false, true} {
		for _, depth := range []int{0, 2, 99, 100, 101, 150} {
			pdf, n := idTreeDoc(depth, split)
			tag := fmt.Sprintf("split=%v depth=%d", split, depth)
			raw, err := api.ReadContext(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("%s: setup: %v", tag, err)
			}
			in := inspectIDTree(raw)
			// Stimulus: the fixture is the tree it says it is, and sound before anything of nib's reads it.
			if in.entries != n || in.maxDepth != depth+1 || in.limOK != in.nodes || in.dangling != 0 {
				t.Fatalf("%s: setup: the fixture is not a sound tree of %d names %d deep: %v", tag, n, depth+1, in)
			}
			for _, door := range []string{"Validated", "ReadOptimized"} {
				var ctx *model.Context
				if door == "Validated" {
					ctx, err = pdfread.Validated(pdf, model.NewDefaultConfiguration())
				} else {
					ctx, err = pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
				}
				if err != nil {
					t.Fatalf("%s %s: %v", tag, door, err)
				}
				out, err := pdfread.Write(ctx)
				if err != nil {
					t.Fatalf("%s %s: write: %v", tag, door, err)
				}
				back, err := api.ReadContext(context.Background(), bytes.NewReader(out), model.NewDefaultConfiguration())
				if err != nil {
					t.Fatalf("%s %s: the output does not read: %v", tag, door, err)
				}
				got := inspectIDTree(back)
				if got.entries != n || got.dangling != 0 {
					t.Errorf("%s %s: the written tree holds %d of %d ids (%d wrong): %v", tag, door, got.entries, n, got.dangling, got)
				}
				if got.limOK != got.nodes {
					t.Errorf("%s %s: %d of %d nodes are written with /Limits that are not their first and last key: %v",
						tag, door, got.nodes-got.limOK, got.nodes, got)
				}
				// A tree the validator follows is left as the file wrote it — the flattening is for the others.
				if depth+1 <= 100 && got.nodes != in.nodes {
					t.Errorf("%s %s: a tree the validator reads whole was reshaped: %d nodes, was %d", tag, door, got.nodes, in.nodes)
				}
			}
		}
	}
}
