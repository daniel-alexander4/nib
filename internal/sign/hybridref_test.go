package sign

import (
	"bytes"
	"fmt"
	"sort"
	"testing"
)

// signedHybrid builds a genuinely SIGNED hybrid-reference file (ISO 32000-1 7.5.8.4): a classic
// `xref` table whose trailer names an `/XRefStm`, with the objects listed in `inStm` held in an
// object stream that only the hybrid stream lists. Everything else — the pages, the field, the
// signature dictionary — is an ordinary object in the classic table, and the signature is a real
// detached PKCS#7 over its honest ranges, so a reader that follows `/XRefStm` sees a valid signature.
//
// It is the /pending 733 shape: digitorus/pdf follows `/Prev` and never `/XRefStm`, so with the
// catalog in `inStm` the library reads no catalog at all, and with only the field there it reads a
// catalog whose `/Fields` resolves to nothing.
func signedHybrid(t *testing.T, id identity, inStm map[int]bool) []byte {
	t.Helper()
	return fillSig(t, hybridDoc(baseObjs(sigDict("1", "")), inStm), "1", nil, detached(t, id))
}

// hybridDoc lays objs (numbered 1-5, catalog 1) out as a hybrid-reference file, the objects in inStm
// held in an object stream that only the trailer's `/XRefStm` lists.
func hybridDoc(objs []sobj, inStm map[int]bool) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	off := map[int]int{}
	var hdr, content bytes.Buffer
	var stm []int
	for _, o := range objs {
		if inStm[o.num] {
			fmt.Fprintf(&hdr, "%d %d ", o.num, content.Len())
			content.WriteString(o.body + "\n")
			stm = append(stm, o.num)
			continue
		}
		off[o.num] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.num, o.body)
	}
	const stmNum, xNum, size = 6, 7, 8
	data := hdr.String() + content.String()
	off[stmNum] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/ObjStm/N %d/First %d/Length %d>>\nstream\n%s\nendstream\nendobj\n",
		stmNum, len(stm), hdr.Len(), len(data), data)
	sort.Ints(stm)
	var xdata bytes.Buffer
	var index bytes.Buffer
	for i, n := range stm {
		fmt.Fprintf(&index, "%d 1 ", n)
		xdata.Write([]byte{2, 0, 0, 0, stmNum, 0, byte(i)})
	}
	off[xNum] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<</Type/XRef/Size %d/W[1 4 2]/Index[%s]/Length %d>>\nstream\n",
		xNum, size, bytes.TrimSpace(index.Bytes()), xdata.Len())
	b.Write(xdata.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	xo := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", size)
	for n := 1; n < size; n++ {
		if o, ok := off[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", o)
		} else {
			b.WriteString("0000000000 00000 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<</Size %d/Root 1 0 R/XRefStm %d>>\nstartxref\n%d\n%%%%EOF\n", size, off[xNum], xo)
	return b.Bytes()
}

// TestASignedHybridReferenceFileIsNeverUnsigned is /pending 733's own case: a signed document whose
// signature the library cannot see because part of it is reachable only through `/XRefStm` must not
// read `Unsigned` — "never signed" — and `HasSignatureBlob` (the gate reflow, undo, tagwrite and the
// ceremony's arrival check ask before touching a signed document) must answer true for it.
func TestASignedHybridReferenceFileIsNeverUnsigned(t *testing.T) {
	a := newIdentity(t, "Alice")
	// The deepdive's own fixture: unsigned-by-structure, a /ByteRange token, the catalog carrying
	// /SigFlags only in the hybrid stream. It measured `Verify` = `unsigned` before this fix.
	if st := Verify(hybrid()); st.State == Unsigned {
		t.Errorf("the deepdive's hybrid fixture verifies %q (/pending 733)", st.State)
	}
	for _, c := range []struct {
		name string
		stm  map[int]bool
	}{
		{"catalog only in the hybrid stream", map[int]bool{1: true}},
		{"signature field only in the hybrid stream", map[int]bool{4: true}},
		{"catalog and signature dictionary only in the hybrid stream", map[int]bool{1: true, 5: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := signedHybrid(t, a, c.stm)
			// STIMULUS: the file is one pdfcpu reads (so it reaches the library), and it is signed.
			if err := pdfcpuCanRead(doc); err != nil {
				t.Fatalf("STIMULUS: pdfcpu refuses the hybrid fixture: %v", err)
			}
			if !bytes.Contains(doc, []byte("/XRefStm")) || !bytes.Contains(doc, []byte("/ByteRange[0 ")) {
				t.Fatal("STIMULUS: fixture is not a filled hybrid-reference signed file")
			}
			st := Verify(doc)
			if st.State == Unsigned {
				t.Errorf("a signed hybrid-reference file verifies %q — it reads as never signed (/pending 733)", st.State)
			}
			if !HasSignatureBlob(doc) {
				t.Errorf("HasSignatureBlob = false for a signed hybrid-reference file (/pending 733)")
			}
		})
	}
}

// TestAParseThatFoundNoCatalogIsNotAParseThatFoundNoSignature — the general half of /pending 733:
// a document whose trailer's `/Root` the library cannot resolve to a catalog was not read, so its
// `/ByteRange` is evidence and it is not `Unsigned`. Hybrid-free, so it binds the catalog rule on its
// own and not through the `/XRefStm` scan.
func TestAParseThatFoundNoCatalogIsNotAParseThatFoundNoSignature(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc := synthSigned(t, a)
	// Re-point the trailer's /Root at the Pages object: parseable, and not a catalog.
	i := bytes.LastIndex(doc, []byte("/Root 1 0 R"))
	if i < 0 {
		t.Fatal("STIMULUS: no /Root 1 0 R in the synthetic signed document")
	}
	doc = append([]byte(nil), doc...)
	copy(doc[i:], "/Root 3 0 R")
	if bytes.Contains(doc, []byte("/XRefStm")) {
		t.Fatal("STIMULUS: fixture must not be hybrid")
	}
	if !signatureBlobPresent(doc) {
		t.Errorf("signatureBlobPresent = false for a document whose /Root is not a catalog — the parse found no catalog, not no signature")
	}
}
