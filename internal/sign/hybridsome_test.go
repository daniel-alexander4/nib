package sign

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// classicRevision writes one revision with a CLASSIC `xref` table — the first when prev is nil, an
// incremental update of prev (its `/Prev` the previous table) otherwise. Objects in `stm` go into
// one uncompressed object stream listed ONLY by a cross-reference stream the trailer names as
// `/XRefStm` (ISO 32000-1 7.5.8.4), so a reader that never follows `/XRefStm` cannot see them;
// with `stm` empty the revision is not hybrid at all.
func classicRevision(t *testing.T, prev []byte, classic, stm []sobj, size int) []byte {
	t.Helper()
	var b bytes.Buffer
	prevKey := ""
	if prev == nil {
		b.WriteString("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	} else {
		prevKey = fmt.Sprintf("/Prev %d", lastStartxref(t, prev))
		b.Write(prev)
	}
	off := map[int]int{}
	for _, o := range classic {
		off[o.num] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.num, o.body)
	}
	xrefStm := ""
	if len(stm) > 0 {
		stmNum, xNum := size-2, size-1
		var hdr, content bytes.Buffer
		for _, o := range stm {
			fmt.Fprintf(&hdr, "%d %d ", o.num, content.Len())
			content.WriteString(o.body + "\n")
		}
		data := hdr.String() + content.String()
		off[stmNum] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<</Type/ObjStm/N %d/First %d/Length %d>>\nstream\n%s\nendstream\nendobj\n",
			stmNum, len(stm), hdr.Len(), len(data), data)
		var index, xdata bytes.Buffer
		for i, o := range stm {
			fmt.Fprintf(&index, "%d 1 ", o.num)
			xdata.Write([]byte{2, 0, 0, 0, byte(stmNum), 0, byte(i)})
		}
		off[xNum] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Index[%s]/Length %d>>\nstream\n",
			xNum, size, bytes.TrimSpace(index.Bytes()), xdata.Len())
		b.Write(xdata.Bytes())
		b.WriteString("\nendstream\nendobj\n")
		xrefStm = fmt.Sprintf("/XRefStm %d", off[xNum])
	}
	xo := b.Len()
	var nums []int
	for n := range off {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	var x strings.Builder
	x.WriteString("xref\n")
	if prev == nil {
		fmt.Fprintf(&x, "0 %d\n0000000000 65535 f \n", size)
		for n := 1; n < size; n++ {
			if o, ok := off[n]; ok {
				fmt.Fprintf(&x, "%010d 00000 n \n", o)
			} else {
				x.WriteString("0000000000 00000 f \n")
			}
		}
	} else {
		for _, n := range nums {
			fmt.Fprintf(&x, "%d 1\n%010d 00000 n \n", n, off[n])
		}
	}
	b.WriteString(x.String())
	fmt.Fprintf(&b, "trailer\n<</Size %d/Root 1 0 R%s%s>>\nstartxref\n%d\n%%%%EOF\n", size, prevKey, xrefStm, xo)
	return b.Bytes()
}

// secondRevision appends a second revision to a document Alice signed (signature 5, field 4): its
// catalog lists a second field 6, pointing at dictionary 7 whose body is sig7. The objects named in
// inStm are held in an object stream that only the revision's `/XRefStm` lists; with inStm empty the
// revision is an ordinary classic one.
func secondRevision(t *testing.T, a identity, sig7 string, inStm map[int]bool) []byte {
	t.Helper()
	first := fillSig(t, classicRevision(t, nil, baseObjs(sigDict("1", "")), nil, 6), "1", nil, detached(t, a))
	var classic, stm []sobj
	for _, o := range []sobj{
		{num: 1, body: "<</Type/Catalog/Pages 2 0 R/AcroForm<</SigFlags 3/Fields[4 0 R 6 0 R]>>>>"},
		{num: 6, body: "<</FT/Sig/T(Signature2)/V 7 0 R>>"},
		{num: 7, body: sig7},
	} {
		if inStm[o.num] {
			stm = append(stm, o)
		} else {
			classic = append(classic, o)
		}
	}
	size := 8
	if len(stm) > 0 {
		size = 10 // the object stream and the cross-reference stream
	}
	return classicRevision(t, first, classic, stm, size)
}

// twoSignedHybrid is /pending 749's file: two honest signatures, Alice's an ordinary object in the
// classic table and Bob's — second revision, covering the whole file — with the objects in inStm
// reachable only through that revision's `/XRefStm`.
func twoSignedHybrid(t *testing.T, a, b identity, inStm map[int]bool) []byte {
	t.Helper()
	return fillSig(t, secondRevision(t, a, sigDict("2", ""), inStm), "2", nil, detached(t, b))
}

// TestASignatureOnlyTheHybridStreamListsIsNamedBesideTheOnesNibChecked is /pending 749's own case.
// /pending 741 named an unchecked signature only where the library found NO signer; a file whose
// classic table lists Alice's signature and whose `/XRefStm` lists Bob's read `Valid` over Alice
// alone — measured on the pre-fix tree: `valid`, 1 signer, `appended`, no `unchecked`, and
// `Revisions` returned one record without error. Bob's signature was neither counted nor named, and
// had it FAILED the verdict would have read the same.
//
// The verdict must not vouch for a population it did not read: `Invalid`, `unchecked` naming the
// hybrid stream, `could-not-check`, Alice still listed (she was checked), and `Revisions` refuses.
// And it must name the gap only where one exists: a hybrid revision whose stream holds no signature,
// or holds only an empty placeholder, reads as before.
func TestASignatureOnlyTheHybridStreamListsIsNamedBesideTheOnesNibChecked(t *testing.T) {
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")

	t.Run("the second signature only in the hybrid stream", func(t *testing.T) {
		doc := twoSignedHybrid(t, a, b, map[int]bool{6: true, 7: true})
		// STIMULUS: pdfcpu reads it (so it reaches the library), it is hybrid, and pdfcpu's reading
		// holds Bob's signature where the sweep holds only Alice's.
		ctx, err := pdfcpuRead(doc)
		if err != nil || !hybridReference(doc) {
			t.Fatalf("STIMULUS: pdfcpu err=%v hybrid=%v", err, hybridReference(doc))
		}
		if revs := mustSweep(t, doc); len(revs) != 1 || revs[0].Obj != 5 || ctx.XRefTable.Table[7] == nil {
			t.Fatalf("STIMULUS: the sweep holds %d records and pdfcpu's object 7 is %v — Bob's signature is not hidden from the one and read by the other", len(revs), ctx.XRefTable.Table[7])
		}
		st := Verify(doc)
		if len(st.Signers) != 1 || st.Signers[0].Fingerprint != a.fp || !st.Signers[0].Valid {
			t.Fatalf("STIMULUS: signers %+v, want Alice alone and valid", st.Signers)
		}
		if st.State != Invalid {
			t.Errorf("state %q over Alice alone, with Bob's signature unread — a verdict over a population nib did not read (/pending 749)", st.State)
		}
		if st.Unchecked != UncheckedHybridReference || !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
			t.Errorf("unchecked=%q addedAfter=%v cause=%q, want %q/true/%q (/pending 749)",
				st.Unchecked, st.AddedAfter, st.AddedAfterCause, UncheckedHybridReference, AddedAfterCouldNotCheck)
		}
		if revs, err := Revisions(doc); err == nil {
			t.Errorf("Revisions returned %d record(s) and no error for a document carrying a signature it holds no record of", len(revs))
		}
		if !HasSignatureBlob(doc) {
			t.Error("HasSignatureBlob = false for a signed document")
		}
	})

	for _, c := range []struct {
		name    string
		doc     func() []byte
		signers int
		after   bool
	}{
		{"control: the same two signatures, nothing hybrid", func() []byte { return twoSignedHybrid(t, a, b, nil) }, 2, false},
		// The library finds Bob's dictionary in the classic table (its xref sweep never needs the
		// field), so the stream hides nothing that is a signature.
		{"hybrid: only the second field in the stream", func() []byte { return twoSignedHybrid(t, a, b, map[int]bool{6: true}) }, 2, false},
		// An empty `/Contents` claims nothing (`refusedOf` never publishes one), so an unread one is
		// no gap: Alice's verdict stands, with the second revision reported as appended.
		{"hybrid: an empty placeholder only in the stream", func() []byte {
			return secondRevision(t, a, "<</Type/Sig/Filter/Adobe.PPKLite/SubFilter/adbe.pkcs7.detached/Contents<>>>", map[int]bool{6: true, 7: true})
		}, 1, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := c.doc()
			if err := pdfcpuCanRead(doc); err != nil {
				t.Fatalf("STIMULUS: pdfcpu refuses the fixture: %v", err)
			}
			st := Verify(doc)
			if st.State != Valid || len(st.Signers) != c.signers || st.Unchecked != "" || st.AddedAfter != c.after {
				t.Errorf("state=%q signers=%d unchecked=%q addedAfter=%v(%q), want valid/%d/none/%v — a gap named where none exists",
					st.State, len(st.Signers), st.Unchecked, st.AddedAfter, st.AddedAfterCause, c.signers, c.after)
			}
			if _, err := Revisions(doc); err != nil {
				t.Errorf("Revisions: %v", err)
			}
		})
	}
}
