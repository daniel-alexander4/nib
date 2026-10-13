package pdfread_test

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// `/pending 795`: pdfcpu's validator inflates the catalog's `/Metadata` whole and parses it, on every read, before
// any of nib's code holds the context — 1,024 MiB allocated and 7.2 s for a 199 KB file whose packet inflates to
// 200 MiB.

// packetDocument is a one-page document whose catalog packet is body, flate-encoded.
func packetDocument(t *testing.T, body []byte) []byte {
	t.Helper()
	var z bytes.Buffer
	zw, err := zlib.NewWriterLevel(&z, zlib.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	zw.Write(body)
	zw.Close()
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Metadata 5 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		4: "<< /Length 12 >>\nstream\n0 0 1 1 re f\nendstream",
		5: fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", z.Len(), z.String()),
	})
}

// titledPacket is an XMP packet pdfcpu's validator reads a title from, padded with spaces to size bytes.
func titledPacket(size int) []byte {
	p := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title><rdf:Alt><rdf:li xml:lang="x-default">Lease` +
		`</rdf:li></rdf:Alt></dc:title></rdf:Description></rdf:RDF></x:xmpmeta>`
	return append([]byte(p), bytes.Repeat([]byte{' '}, max(size-len(p), 0))...)
}

// TestAPacketPastTheLimitIsNotInflatedByTheRead — the read sets the packet aside for the validator, so the
// document is still read, the packet is still in it afterwards, and the validator took nothing from it.
func TestAPacketPastTheLimitIsNotInflatedByTheRead(t *testing.T) {
	// Control: a packet inside the limit is read by the validator as it always was.
	ctx, err := pdfread.ReadOptimized(packetDocument(t, titledPacket(0)), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if ctx.XRefTable.CatalogXMPMeta == nil || !strings.Contains(ctx.XRefTable.Title, "Lease") {
		t.Fatalf("control: the validator read no title from an ordinary packet (title %q)", ctx.XRefTable.Title)
	}

	const size = 200 << 20
	pdf := packetDocument(t, titledPacket(size))
	if len(pdf) > 1<<20 {
		t.Fatalf("setup: the fixture is %d bytes, want a small file that inflates", len(pdf))
	}
	alloc := allocatedBy(func() { ctx, err = pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration()) })
	if err != nil {
		t.Fatalf("a document whose packet inflates to %d MiB was refused: %v — the packet is set aside, the document is read", size>>20, err)
	}
	if most := uint64(100 << 20); alloc > most {
		t.Errorf("reading a document whose packet inflates to %d MiB allocated %d MiB, want under %d MiB — the "+
			"validator must not be handed a packet past pdfread.MaxPacketBytes", size>>20, alloc>>20, most>>20)
	}
	if ctx.XRefTable.CatalogXMPMeta != nil {
		t.Error("the validator read a packet past the limit")
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, kept := root["Metadata"]; !kept {
		t.Error("the catalog has no /Metadata after the read — the packet is set aside for the validation only, and put back")
	}
	// And it is written back out as it was: a read and a write lose nothing.
	out, err := pdfread.Write(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("/Metadata")) {
		t.Error("a document read and written lost its /Metadata key")
	}
}
