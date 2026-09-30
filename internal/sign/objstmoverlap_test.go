package sign

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// overlapDoc is an unsigned one-page PDF carrying the text `/ByteRange` (so `Verify` sweeps it) with
// one object stream, object 4: the given header and content, and an xref stream listing each of the
// header's ids as a member at its index.
func overlapDoc(ids []int, hdr, content string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%/ByteRange\n")
	plain := map[int]string{
		1: "<</Type/Catalog/Pages 2 0 R>>",
		2: "<</Type/Pages/Kids[3 0 R]/Count 1>>",
		3: "<</Type/Page/Parent 2 0 R/MediaBox[0 0 9 9]>>",
		4: fmt.Sprintf("<</Type/ObjStm/N %d/First %d/Length %d>>\nstream\n%s%s\nendstream", len(ids), len(hdr), len(hdr)+len(content), hdr, content),
	}
	members := map[int]int{}
	top := 4
	for i, id := range ids {
		members[id] = i
		top = max(top, id)
	}
	var nums []int
	for n := range plain {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	off := map[int]int{}
	for _, n := range nums {
		off[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, plain[n])
	}
	x := top + 1
	off[x] = b.Len()
	var data bytes.Buffer
	put := func(typ byte, a, c int) {
		data.Write([]byte{typ, byte(a >> 24), byte(a >> 16), byte(a >> 8), byte(a), byte(c >> 8), byte(c)})
	}
	for n := 0; n <= x; n++ {
		idx, in := members[n]
		switch {
		case n == 0:
			put(0, 0, 0xffff)
		case in:
			put(2, 4, idx)
		case off[n] != 0:
			put(1, off[n], 0)
		default:
			put(0, 0, 0)
		}
	}
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Root 1 0 R/Length %d>>\nstream\n", x, x+1, data.Len())
	b.Write(data.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", off[x])
	return b.Bytes()
}

// TestOverlappingObjectStreamMembersAreRefused is /pending 760 parts 2: a lookup reads its member from
// the member's offset, however far that parse runs, and the cost model charges each stream's decoded
// bytes once — which is what honest, non-overlapping members read. Two shapes read far more, measured
// before the patched reader refused them (best of three, one pass): 100 header ids naming ONE 1 MB
// member, 7.5 s a pass (400: 44 s) at 0.0005 of the ceiling; and 2,000 offsets 0..1999 into one
// `[[[…]]]` of depth 2,000, 2.3 s a pass (8,000: 30 s, quadratic) at 0.006. pdfcpu reads both, so
// `Verify`'s readability gate does not stop them. The patched reader now refuses each as it reads
// (`dpdf.ErrObjStmTooCostly`), and the sweep names that the lookup-cost ceiling.
func TestOverlappingObjectStreamMembersAreRefused(t *testing.T) {
	sameOffset := func(k int) []byte {
		var hdr strings.Builder
		var ids []int
		for i := 0; i < k; i++ {
			fmt.Fprintf(&hdr, "%d 0 ", 5+i)
			ids = append(ids, 5+i)
		}
		return overlapDoc(ids, hdr.String(), "["+strings.Repeat("0 ", 1<<19)+"]")
	}
	nested := func(k int) []byte {
		var hdr strings.Builder
		var ids []int
		for i := 0; i < k; i++ {
			fmt.Fprintf(&hdr, "%d %d ", 5+i, i)
			ids = append(ids, 5+i)
		}
		return overlapDoc(ids, hdr.String(), strings.Repeat("[", k)+strings.Repeat("]", k))
	}
	for _, c := range []struct {
		name string
		doc  []byte
	}{
		{"100 ids naming one 1 MB member", sameOffset(100)},
		{"2,000 offsets into one nested member", nested(2000)},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := pdfcpuRead(c.doc); err != nil {
				t.Fatalf("STIMULUS: pdfcpu refuses the fixture (%v), so Verify's gate would stop it first", err)
			}
			if _, err := lookupCostOf(t, c.doc); err != nil {
				t.Fatalf("STIMULUS: the cost model refuses the fixture (%v); this is the shape it cannot see", err)
			}
			t0 := time.Now()
			_, err := sweepRevisions(c.doc)
			el := time.Since(t0)
			if !errors.Is(err, errLookupCostCeiling) {
				t.Errorf("sweep: err=%v after %v, want errLookupCostCeiling — the patched reader must refuse overlapping member reads (/pending 760)", err, el)
			}
			if st := Verify(c.doc); st.Unchecked != UncheckedLookupCost {
				t.Errorf("Verify: unchecked=%q state=%q, want %q", st.Unchecked, st.State, UncheckedLookupCost)
			}
		})
	}
}

// TestAMemberThatDecodesPastTheReadersCapIsRefused: the cost model charges a stream's decoded bytes
// only as far as the furthest member's OFFSET, and a member's parse may run far past it — here one
// member at offset 0 is an array of 70 MB of blanks (~70 KB flate-encoded). The patched reader HOLDS
// what it decodes for the pass, so it refuses to decode past 64 MiB of one stream at all.
func TestAMemberThatDecodesPastTheReadersCapIsRefused(t *testing.T) {
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write([]byte("5 0 ["))
	blank := bytes.Repeat([]byte{' '}, 1<<20)
	for i := 0; i < 70; i++ {
		w.Write(blank)
	}
	w.Write([]byte("]"))
	w.Close()
	enc := z.Bytes()
	doc := rawObjStmDoc(map[int]stmSpec{4: {hdr: "5 0 ", n: 1, first: 4, flate: true, mangle: func([]byte) []byte { return enc }}},
		map[int][2]int{5: {4, 0}})
	if _, err := lookupCostOf(t, doc); err != nil {
		t.Fatalf("STIMULUS: the cost model refuses the fixture (%v); this is the shape it cannot see", err)
	}
	if _, err := sweepRevisions(doc); !errors.Is(err, errLookupCostCeiling) {
		t.Errorf("sweep: err=%v, want errLookupCostCeiling — the reader must not decode past its cap (/pending 760)", err)
	}
}
