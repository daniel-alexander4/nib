package sign

import (
	"bytes"
	"strings"
	"testing"
)

// TestASignatureWhoseNamesAreEscapedIsStillSigned is /pending 579: a name may spell any character
// `#xx` (ISO 32000-1 7.3.5), and the two byte scans that decide "there is no signature here" without
// a parser read only the plain spelling. Each document below is GENUINELY signed — a detached PKCS#7
// over its honest ranges, which the library verifies once it is let in.
//
// Measured before the fix: the first read `unsigned` from `Verify`; the second was unsigned to
// `Verify` and to `HasSignatureBlob`; the third was `valid` to `Verify` and unsigned to
// `HasSignatureBlob`, the gate reflow, undo, OCR, tag writes and the PDF/UA drop ask before a rewrite.
func TestASignatureWhoseNamesAreEscapedIsStillSigned(t *testing.T) {
	a := newIdentity(t, "Alice")
	plain := sigDict("1", "")
	escaped := strings.Replace(plain, "/ByteRange", "/Byte#52ange", 1)
	if escaped == plain {
		t.Fatal("STIMULUS: the signature dictionary's /ByteRange was not respelled")
	}
	fieldInStream := map[int]bool{4: true}
	for _, c := range []struct {
		name string
		doc  []byte
	}{
		{"an escaped /ByteRange", fillSig(t, synthRevision(t, nil, baseObjs(escaped), 1), "1", nil, detached(t, a))},
		{"an escaped /ByteRange in a hybrid file whose field only the stream lists",
			fillSig(t, hybridDoc(baseObjs(escaped), fieldInStream), "1", nil, detached(t, a))},
		{"an escaped /XRefStm, the field only in the stream it names",
			fillSig(t, bytes.Replace(hybridDoc(baseObjs(plain), fieldInStream), []byte("/XRefStm"), []byte("/XRef#53tm"), 1), "1", nil, detached(t, a))},
	} {
		if bytes.Contains(c.doc, []byte("/ByteRange")) && bytes.Contains(c.doc, []byte("/XRefStm")) {
			t.Fatalf("STIMULUS %s: both names are still spelled plainly", c.name)
		}
		st := Verify(c.doc)
		if st.State != Valid || len(st.Signers) != 1 || st.Signers[0].Fingerprint != a.fp {
			t.Errorf("%s: Verify reads %v with %d signers, want valid with Alice's — the document is signed, and a "+
				"reader that decodes the name sees it", c.name, st.State, len(st.Signers))
		}
		if !HasSignatureBlob(c.doc) {
			t.Errorf("%s: HasSignatureBlob is false — every gate that asks it before rewriting a document would "+
				"rewrite this one and destroy its signature", c.name)
		}
	}

	// The other direction: a `#` alone is not a signature. An unsigned document that escapes some
	// other name still answers by inspection and never reaches the parser.
	unsigned := synthRevision(t, nil, []sobj{
		{num: 1, body: "<</Type/Catalog/Pages 2 0 R>>"},
		{num: 2, body: "<</Type/Pages/Kids[3 0 R]/Count 1>>"},
		{num: 3, body: "<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]/Byte#52ang 1/#42yteRang 2>>"},
	}, 1)
	if scanForSignatureBlob(unsigned) || HasSignatureBlob(unsigned) || Verify(unsigned).State != Unsigned {
		t.Errorf("an unsigned document with escaped names that are not /ByteRange reads scan=%v blob=%v state=%v, want unsigned throughout",
			scanForSignatureBlob(unsigned), HasSignatureBlob(unsigned), Verify(unsigned).State)
	}
}

// TestNameInReadsANameInEitherSpelling holds the scan's own contract, one spelling per row.
func TestNameInReadsANameInEitherSpelling(t *testing.T) {
	for _, c := range []struct {
		pdf  string
		want bool
	}{
		{"<</ByteRange[0 1 2 3]>>", true},
		{"<</Byte#52ange[0 1 2 3]>>", true},
		{"<</#42#79#74#65#52#61#6e#67#65[0 1 2 3]>>", true}, // every character escaped, lower-case hex
		{"<</ByteRang#65>>", true},
		{"<</ByteRang#6", false},    // an escape the file ends inside
		{"<</ByteRang#6g>>", false}, // not an escape
		{"<</Byte#53ange>>", false},
		{"Byte#52ange", false}, // no name begins here
		{"<</F#41 1>>", false},
		{"", false},
	} {
		if got := nameIn([]byte(c.pdf), "ByteRange"); got != c.want {
			t.Errorf("nameIn(%q, ByteRange) = %v, want %v", c.pdf, got, c.want)
		}
	}
}
