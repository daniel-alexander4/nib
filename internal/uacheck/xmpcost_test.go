package uacheck

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
)

// The metadata and XFA readers' cost and ceiling — the P07 phase-close review's R4-1 and R4-5.

// splitPartPacket is an identification whose part is `runs` characters, each its own character-data run: a
// comment after every one splits the text without changing it (the P07 phase-close review's shape, R4-1).
func splitPartPacket(runs int) string {
	return `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/">` +
		`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/"><pdfuaid:part>` +
		strings.Repeat("1<!---->", runs) + `</pdfuaid:part></rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`
}

// TestAPropertySplitIntoManyRunsIsReadInLinearWork — R4-1. A property's text grew by `p.own += raw` once per
// run, so a value split by comments into n runs copied O(n²) bytes: measured 200,000 runs 3.8 s and 400,000
// runs 20.4 s before the builder, 0.35 s and 0.62 s after. Asserted as WORK, not time — the bytes the parse
// allocates, which is where a quadratic copy shows — at n and 2n: linear work doubles, the copy quadruples.
func TestAPropertySplitIntoManyRunsIsReadInLinearWork(t *testing.T) {
	titled, err := pdfops.SetTitle(plainDoc(t), "A named document")
	if err != nil {
		t.Fatal(err)
	}
	const runs = 20000
	alloc := func(n int) uint64 {
		d, err := open(withRawPacket(t, titled, splitPartPacket(n)))
		if err != nil {
			t.Fatal(err)
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		x := parseXMP(d)
		runtime.ReadMemStats(&after)
		// The stimulus first: every run was read into the one property, and nothing else.
		if !x.Readable || len(x.UAPart) != n || strings.Trim(x.UAPart, "1") != "" {
			t.Fatalf("%d runs: the part read back as %d characters (readable=%v, %s), want %d ones",
				n, len(x.UAPart), x.Readable, x.Why, n)
		}
		return after.TotalAlloc - before.TotalAlloc
	}
	one, two := alloc(runs), alloc(2*runs)
	if ratio := float64(two) / float64(one); ratio > 3 {
		t.Errorf("doubling the runs from %d to %d took the parse from %d to %d bytes allocated (×%.1f); linear work "+
			"doubles, and a copy per run quadruples", runs, 2*runs, one, two, ratio)
	}
}

// TestAStreamIsRefusedWhileItDecodesPastItsCeiling — R4-5. The XFA packet was capped AFTER `sd.Decode()`, which
// bounded what nib parsed and nothing about what it inflated, and the XMP packet was not capped at all.
// `decodeWithin` stops the filter at the ceiling. The control sits exactly at it.
func TestAStreamIsRefusedWhileItDecodesPastItsCeiling(t *testing.T) {
	const size = 1 << 20
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(bytes.Repeat([]byte{' '}, size))
	zw.Close()
	stream := func() *types.StreamDict {
		d, err := open(buildPDF(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
			4: fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream",
				z.Len(), z.String()),
		}))
		if err != nil {
			t.Fatal(err)
		}
		sd, _, err := d.Ctx.DereferenceStreamDict(d.Catalog["Metadata"])
		if err != nil || sd == nil {
			t.Fatalf("the metadata stream does not resolve: %v", err)
		}
		return sd
	}
	// The control: at the ceiling the stream decodes whole — the stimulus is its decoded length.
	at := stream()
	if why := decodeWithin(at, size, "the stream"); why != "" || len(at.Content) != size {
		t.Fatalf("a stream of exactly %d decoded bytes under a %d ceiling: %q with %d bytes, want it whole",
			size, size, why, len(at.Content))
	}
	over := stream()
	why := decodeWithin(over, size-1, "the stream")
	if !strings.Contains(why, "exceeds") {
		t.Errorf("a stream one byte past its ceiling: %q, want a refusal naming the ceiling", why)
	}
	// And stopped DURING the decode: nothing past the ceiling was kept.
	if len(over.Content) > size-1 {
		t.Errorf("the refused stream holds %d decoded bytes; the decode ran past its %d-byte ceiling", len(over.Content), size-1)
	}
	// An UNFILTERED stream is handed back raw, with no limit applied to it — the length check after the
	// decode is what refuses it.
	raw, err := open(buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		4: fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length 64 >>\nstream\n%s\nendstream", strings.Repeat(" ", 64)),
	}))
	if err != nil {
		t.Fatal(err)
	}
	rsd, _, _ := raw.Ctx.DereferenceStreamDict(raw.Catalog["Metadata"])
	if why := decodeWithin(rsd, 63, "the stream"); !strings.Contains(why, "exceeds") {
		t.Errorf("an unfiltered stream of 64 bytes under a 63-byte ceiling: %q, want a refusal", why)
	}
	// The production ceilings are the ones the two readers use, and both readers reach the door.
	if maxXMPBytes < 1<<20 || maxXFABytes < 1<<20 {
		t.Errorf("maxXMPBytes %d and maxXFABytes %d: a real packet runs to hundreds of kilobytes", maxXMPBytes, maxXFABytes)
	}
	flated := func(n int) string {
		var b bytes.Buffer
		w := zlib.NewWriter(&b)
		w.Write(bytes.Repeat([]byte{' '}, n))
		w.Close()
		return fmt.Sprintf("/Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", b.Len(), b.String())
	}
	meta, err := open(buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		4: "<< /Type /Metadata /Subtype /XML " + flated(maxXMPBytes+1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if x := parseXMP(meta); x.Readable || !strings.Contains(x.Why, "exceeds") {
		t.Errorf("a metadata packet one byte past maxXMPBytes: readable=%v (%q), want a refusal naming the ceiling", x.Readable, x.Why)
	}
	xfa, err := open(buildPDF(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		6: "<< " + flated(maxXFABytes+1),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, why := xfa.xfaDynamicRender(types.Array{types.StringLiteral("config"), *types.NewIndirectRef(6, 0)}); !strings.Contains(why, "exceeds") {
		t.Errorf("an XFA config packet one byte past maxXFABytes: %q, want a refusal naming the ceiling", why)
	}
}
