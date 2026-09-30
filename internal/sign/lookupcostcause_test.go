package sign

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"testing"

	dpdf "github.com/digitorus/pdf"
)

// TestALookupCostRefusalSaysWhy is /pending 760 part 1: a document the lookup-cost ceiling refuses
// (`libraryLookupCost`) was read `Invalid` with no `Unchecked` cause, so the CLI said "modified since
// signing" of a document whose signature nobody checked. The stimulus is a signed document with five
// appended object streams, each one member behind 15 MB of decoded blanks (under pdfcpu's own 16 MiB
// `/First` limit, so pdfcpu reads it) — 75 MB together, past the byte ceiling the patched reader
// would hold — each flate-encoded to ~15 KB.
func TestALookupCostRefusalSaysWhy(t *testing.T) {
	prev := synthSigned(t, newIdentity(t, "Alice"))
	const pad, streams = 15_000_000, 5
	doc := appendObjStms(t, prev, pad, streams)
	if _, err := lookupCostOf(t, doc); !errors.Is(err, errLookupCostCeiling) {
		t.Fatalf("STIMULUS: lookup cost err=%v, want errLookupCostCeiling", err)
	}
	st := Verify(doc)
	if st.State != Invalid {
		t.Fatalf("STIMULUS: state %q, want invalid", st.State)
	}
	if st.Unchecked != UncheckedLookupCost || !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("unchecked=%q addedAfter=%v cause=%q, want %q/true/%q (/pending 760)",
			st.Unchecked, st.AddedAfter, st.AddedAfterCause, UncheckedLookupCost, AddedAfterCouldNotCheck)
	}
}

// appendObjStms appends an incremental revision to prev holding k flate object streams, numbered
// from prev's `/Size` in pairs (the stream, then its one member), each `/N 1` with its member at
// `/First pad` behind blanks, and an xref stream naming them all.
func appendObjStms(t *testing.T, prev []byte, pad, k int) []byte {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(prev), int64(len(prev)))
	if err != nil {
		t.Fatal(err)
	}
	base := int(r.Trailer().Key("Size").Int64())
	put := func(d *bytes.Buffer, typ byte, a, c int) {
		d.Write([]byte{typ, byte(a >> 24), byte(a >> 16), byte(a >> 8), byte(a), byte(c >> 8), byte(c)})
	}
	blank := bytes.Repeat([]byte{' '}, 1<<20)
	var b, data bytes.Buffer
	b.Write(prev)
	b.WriteString("\n")
	for i := 0; i < k; i++ {
		stm := base + 2*i
		head := fmt.Sprintf("%d 0", stm+1)
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		w.Write([]byte(head))
		for left := pad - len(head); left > 0; left -= len(blank) {
			w.Write(blank[:min(left, len(blank))])
		}
		w.Write([]byte("<</F 1>>"))
		w.Close()
		put(&data, 1, b.Len(), 0)
		put(&data, 2, stm, 0)
		fmt.Fprintf(&b, "%d 0 obj\n<</Type/ObjStm/N 1/First %d/Filter/FlateDecode/Length %d>>\nstream\n", stm, pad, z.Len())
		b.Write(z.Bytes())
		b.WriteString("\nendstream\nendobj\n")
	}
	x := base + 2*k
	xo := b.Len()
	put(&data, 1, xo, 0)
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Index[%d %d]/Root 1 0 R/Prev %d/Length %d>>\nstream\n",
		x, x+1, base, 2*k+1, lastStartxref(t, prev), data.Len())
	b.Write(data.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", xo)
	return b.Bytes()
}
