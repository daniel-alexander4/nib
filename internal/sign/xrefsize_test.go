package sign

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"

	dpdf "github.com/digitorus/pdf"
)

// xrefSizeDoc is a ~400-byte file whose xref names an object number past ISO 32000-1's limit, in each of the four
// places the patched reader used to size its table from (NOTICE.nib divergence 7).
func xrefSizeDoc(kind string) []byte {
	// Padded past the reader's 200-byte `%%EOF` search, which a shorter file never gets beyond.
	head := "%PDF-1.7\n%" + strings.Repeat("x", 256) + "\n2 0 obj<</Type/Sig/Filter/Adobe.PPKLite/ByteRange[0 1 2 3]/Contents<00>>>endobj\n"
	off := len(head)
	tail := fmt.Sprintf("startxref\n%d\n%%%%EOF\n", off)
	switch kind {
	case "stream /Size":
		return []byte(head + "1 0 obj<</Type/XRef/Size 1500000000/W[1 1 1]/Index[0 1]/Length 3/Root 2 0 R>>stream\n\x01\x00\x00\nendstream\nendobj\n" + tail)
	case "stream /Index":
		return []byte(head + "1 0 obj<</Type/XRef/Size 3/W[1 1 1]/Index[1500000000 1]/Length 3/Root 2 0 R>>stream\n\x01\x00\x00\nendstream\nendobj\n" + tail)
	case "stream /W":
		return []byte(head + "1 0 obj<</Type/XRef/Size 3/W[1 1 90000000000]/Index[0 1]/Length 3/Root 2 0 R>>stream\n\x01\x00\x00\nendstream\nendobj\n" + tail)
	case "stream /Size inside the ISO limit": // the re-review: legal by Annex C, 512 MB from 469 bytes
		return []byte(head + "1 0 obj<</Type/XRef/Size 8388607/W[1 1 1]/Index[0 1]/Length 3/Root 2 0 R>>stream\n\x01\x00\x00\nendstream\nendobj\n" + tail)
	case "table subsection inside the ISO limit": // the re-review: 2.9 GB from 438 bytes
		return []byte(head + "xref\n8388000 1\n0000000009 00000 n \ntrailer<</Size 1/Root 2 0 R>>\n" + tail)
	default: // a classic table's subsection
		return []byte(head + "xref\n1500000000 1\n0000000009 00000 n \ntrailer<</Size 1/Root 2 0 R>>\n" + tail)
	}
}

// TestAnXrefPastThePDFObjectLimitIsRefusedNotAllocated — `/Size 1500000000` was a 48 GB `make` inside
// `HasSignatureBlob`, which runs ungated on every save and every ceremony arrival: a fatal out-of-memory no recover
// holds (the PLAN-returned-document P02 phase-close review, measured under `ulimit -v`). The reader refuses it as a
// malformed xref, and the blob check answers from its byte scan.
func TestAnXrefPastThePDFObjectLimitIsRefusedNotAllocated(t *testing.T) {
	for _, kind := range []string{"stream /Size", "stream /Index", "stream /W", "table subsection",
		"stream /Size inside the ISO limit", "table subsection inside the ISO limit"} {
		t.Run(kind, func(t *testing.T) {
			doc := xrefSizeDoc(kind)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
			runtime.ReadMemStats(&after)
			// The limit is proportional to the file (eight entries per byte, at least 2^16): a few hundred bytes may
			// name 65,536 objects, a few MB of table, never the ISO limit's quarter-gigabyte and more.
			if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 16<<20 {
				t.Errorf("reading a %d-byte xref allocated %d MB", len(doc), alloc>>20)
			}
			if err == nil {
				t.Fatalf("the reader accepted an xref naming objects far past what a %d-byte file holds", len(doc))
			}
			want := "past the"
			if kind == "stream /W" {
				want = "invalid W array" // a 90 GB entry buffer, refused before it is made
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("refused for another reason (%v): the bound is not what refused it", err)
			}
			if !HasSignatureBlob(doc) {
				t.Error("the byte scan's answer was lost: the file carries a /ByteRange")
			}
		})
	}
	// The control: an xref naming objects inside the limit still reads.
	ok := []byte(strings.Replace(string(xrefSizeDoc("stream /Index")), "/Index[1500000000 1]", "/Index[0 1]", 1))
	if _, err := dpdf.NewReader(bytes.NewReader(ok), int64(len(ok))); err != nil && strings.Contains(err.Error(), "past the") {
		t.Fatalf("an xref inside the limit was refused by it: %v", err)
	}
}

// TestAnXrefStreamIndexPastItsSizeExtendsTheTable — an /Index pair past the stream's /Size indexed the table past its
// length and panicked, where a classic subsection extends it (NOTICE.nib divergence 7 aligns the two).
func TestAnXrefStreamIndexPastItsSizeExtendsTheTable(t *testing.T) {
	doc := []byte(strings.Replace(string(xrefSizeDoc("stream /Index")), "/Size 3/W[1 1 1]/Index[1500000000 1]/Length 3",
		"/Size 2/W[1 1 1]/Index[5 1]/Length 3", 1))
	if !bytes.Contains(doc, []byte("/Index[5 1]")) {
		t.Fatal("stimulus: the fixture was not rewritten")
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("an /Index past /Size panicked: %v", r)
		}
	}()
	_, _ = dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
}
