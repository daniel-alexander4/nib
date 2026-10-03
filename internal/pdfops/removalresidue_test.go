package pdfops

import (
	"archive/zip"
	"bytes"
	"errors"
	"log"
	"strconv"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// The residue of the P02 and P03 phase-close reviews' pdfops findings (`/pending 595-599, 812`). Each test
// plants the thing a removal claims to take out, or the input a door claims to refuse, and asserts the claim.

// xmpImageDoc is one page drawing an image XObject that carries its OWN XMP packet — a camera's — and a form
// XObject with one of its own; the catalog has none, so a scan that reads only the catalog sees nothing.
func xmpImageDoc() []byte {
	const xmp = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><tiff:Make xmlns:tiff="http://ns.adobe.com/tiff/1.0/">Jane's Camera</tiff:Make></x:xmpmeta>`
	const xmp2 = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><xmp:CreatorTool xmlns:xmp="http://ns.adobe.com/xap/1.0/">Jane's Draw</xmp:CreatorTool></x:xmpmeta>`
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R /Resources << /XObject << /Im1 5 0 R /Fm1 7 0 R >> >> >>",
		4: "<< /Length 36 >>\nstream\nq 10 0 0 10 0 0 cm /Im1 Do Q /Fm1 Do\nendstream",
		5: "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Metadata 6 0 R /Length 1 >>\nstream\n\x80\nendstream",
		6: "<< /Type /Metadata /Subtype /XML /Length " + strconv.Itoa(len(xmp)) + " >>\nstream\n" + xmp + "\nendstream",
		7: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Metadata 8 0 R /Length 10 >>\nstream\n0 0 5 5 re\nendstream",
		8: "<< /Type /Metadata /Subtype /XML /Length " + strconv.Itoa(len(xmp2)) + " >>\nstream\n" + xmp2 + "\nendstream",
	})
}

// holdersIn walks every object of pdf directly — not through Scan, which reads through the same
// metadataHolders the strip does — and counts the dictionaries still carrying /Metadata.
func holdersIn(t *testing.T, pdf []byte) int {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Free {
			continue
		}
		var d types.Dict
		switch o := e.Object.(type) {
		case types.Dict:
			d = o
		case types.StreamDict:
			d = o.Dict
		}
		if _, ok := d.Find("Metadata"); ok {
			n++
		}
	}
	return n
}

// TestStripMetadataRemovesAnImagesAndAFormsOwnXMP — `/pending 595`: only the catalog's and the pages' /Metadata
// went, so an image's own XMP (camera, author, software) survived "strip metadata", and Scan, which looked only
// at the catalog, reported nothing left. Both shapes: the hand-written file, and the same file after pdfcpu has
// packed it into object streams, where a byte search cannot see a dictionary at all.
func TestStripMetadataRemovesAnImagesAndAFormsOwnXMP(t *testing.T) {
	plain := xmpImageDoc()
	packed, err := writeMutated(plain, func(*model.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for name, doc := range map[string][]byte{"plain": plain, "object streams": packed} {
		if got := holdersIn(t, doc); got != 2 {
			t.Fatalf("%s: setup: %d holders of /Metadata, want the image's and the form's", name, got)
		}
		if !foundKind(t, doc, "metadata") {
			t.Errorf("%s: Scan does not report an image's or a form's own XMP; findings=%+v", name, must(t, doc).Findings)
		}
		out, err := StripMetadata(doc)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := holdersIn(t, out); got != 0 {
			t.Errorf("%s: %d dictionaries still carry /Metadata after StripMetadata reported success", name, got)
		}
		if foundKind(t, out, "metadata") {
			t.Errorf("%s: the residual scan still reports metadata: %+v", name, must(t, out).Findings)
		}
	}
}

// TestEveryRemovalRefusesWhatItsRemitStillFinds drives each removal's judgment directly: the doors make a
// residue unreachable today, which is why the check that catches the NEXT mechanism is proved on its own.
func TestEveryRemovalRefusesWhatItsRemitStillFinds(t *testing.T) {
	rep := func(kind string) ScanReport { return ScanReport{Findings: []Finding{{Kind: kind, Detail: "x"}}} }
	for _, c := range []struct {
		kind    string
		refused error
		remit   func(Finding) bool
	}{
		{"metadata", ErrMetadataRemains, identifying},
		{"info", ErrMetadataRemains, identifying},
		{"attachment", ErrFilesOrMediaRemain, filesOrMedia},
		{"media", ErrFilesOrMediaRemain, filesOrMedia},
	} {
		if err := residue(rep(c.kind), c.refused, c.remit); !errors.Is(err, c.refused) {
			t.Errorf("a %q finding left in the output passed verification: %v", c.kind, err)
		}
	}
	// And the remits are not "everything": a removal is refused only over what it claims to remove.
	if err := residue(rep("javascript"), ErrFilesOrMediaRemain, filesOrMedia); err != nil {
		t.Errorf("Remove files & media does not claim JavaScript, and was refused over it: %v", err)
	}
	if err := residue(rep("javascript"), ErrMetadataRemains, identifying); err != nil {
		t.Errorf("Strip metadata does not claim JavaScript, and was refused over it: %v", err)
	}
	// Every media kind Scan can report is inside RemoveFilesAndMedia's remit.
	for sub, m := range mediaAnnots {
		if !filesOrMedia(Finding{Kind: m.kind}) {
			t.Errorf("%s annotations report kind %q, which RemoveFilesAndMedia's verification does not check", sub, m.kind)
		}
	}
}

// TestRemoveFilesAndMediaRemovesATreePdfcpuCannotList — `/pending 812`: `removeAllAttachments` asked pdfcpu's
// `ListAttachments` first and read its error as "nothing attached", and RemoveFilesAndMedia had no re-scan. The
// filespec here has no /EF — an ordinary reference to an external file — and listing it does not even error:
// it nil-dereferences, so both removals panicked on it.
func TestRemoveFilesAndMediaRemovesATreePdfcpuCannotList(t *testing.T) {
	doc := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles << /Names [(payload.exe) 4 0 R] >> >> >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
		4: "<< /Type /Filespec /F (payload.exe) /UF (payload.exe) >>",
	})
	if !foundKind(t, doc, "attachment") {
		t.Fatal("setup: Scan does not see the embedded-files tree, so its absence below proves nothing")
	}
	ctx, err := pdfread.ReadOptimized(doc, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if listable := func() (ok bool) {
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		_, lerr := ctx.ListAttachments()
		return lerr == nil
	}(); listable {
		t.Fatal("setup: pdfcpu lists this tree, so the test does not reach the unlistable case")
	}
	for name, remove := range map[string]func([]byte) ([]byte, error){
		"RemoveFilesAndMedia": RemoveFilesAndMedia,
		"StripActive":         StripActive,
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s panicked on an external-file filespec: %v", name, r)
				}
			}()
			out, err := remove(doc)
			if err != nil {
				t.Errorf("%s refused a tree it can remove: %v", name, err)
				return
			}
			if foundKind(t, out, "attachment") {
				t.Errorf("%s left the embedded-files tree and reported success", name)
			}
		}()
	}
}

// TestWriteMutatedTurnsAPdfcpuFaultIntoAnError — `/pending 596`: pdfcpu reports some failures by panicking with a
// `fault.Panic`; every sibling door caught it, and a mutation through writeMutated escaped as a panic.
func TestWriteMutatedTurnsAPdfcpuFaultIntoAnError(t *testing.T) {
	doc, err := testpdf.Text("one")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a pdfcpu fault inside writeMutated escaped as a panic: %v", r)
		}
	}()
	if _, err := writeMutated(doc, func(*model.Context) error {
		fault.Fail("a malformed object")
		return nil
	}); err == nil {
		t.Fatal("a pdfcpu fault inside writeMutated returned no error")
	}
}

// failingReader yields some bytes and then fails, the shape of an image decode that dies part-way.
type failingReader struct{ sent bool }

func (f *failingReader) Read(p []byte) (int, error) {
	if !f.sent {
		f.sent = true
		return copy(p, "\x89PNG partial"), nil
	}
	return 0, errors.New("decode failed part-way")
}

// TestAnImageThatFailsPartWayLeavesNoEntry — `/pending 599`: the entry was created before the copy, so a decode
// that failed part-way left a truncated file in the archive, and the per-page fallback carried on past it.
func TestAnImageThatFailsPartWayLeavesNoEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if n, err := addZipImage(zw, "broken.png", &failingReader{}); err == nil || n != 0 {
		t.Fatalf("a failed read reported n=%d err=%v", n, err)
	}
	if n, err := addZipImage(zw, "good.png", strings.NewReader("whole")); err != nil || n != 1 {
		t.Fatalf("a whole read reported n=%d err=%v", n, err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if len(names) != 1 || names[0] != "good.png" {
		t.Errorf("archive entries %v, want only good.png — a half-read image is not an extracted one", names)
	}
}

// TestStampTextLayerRefusesAPageBelowOne — `/pending 597`: a word naming page 0 was stamped on page 1, so Find
// landed on a page for text the engine never read there.
func TestStampTextLayerRefusesAPageBelowOne(t *testing.T) {
	doc, err := testpdf.Text("one", "two")
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []int{0, -3} {
		if _, err := StampTextLayer(doc, []Word{{Page: page, Rect: [4]float64{72, 700, 140, 712}, Text: "Invoice"}}, "eng"); err == nil {
			t.Errorf("a word naming page %d was stamped rather than refused", page)
		}
	}
	if _, err := StampTextLayer(doc, []Word{{Page: 1, Rect: [4]float64{72, 700, 140, 712}, Text: "Invoice"}}, "eng"); err != nil {
		t.Errorf("a word on page 1 was refused: %v", err)
	}
}

// TestPageLabelsAlwaysNamePageIndexZero — `/pending 598`: ISO 32000-1 §12.4.2, "The tree shall include a value
// for page index 0". A labelling that started later wrote a tree without one.
func TestPageLabelsAlwaysNamePageIndexZero(t *testing.T) {
	doc, err := testpdf.Text("one", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		start   int
		keys    []int
		emptyAt int // the index of the empty-label entry, or -1
	}{
		{start: 2, keys: []int{0, 1}, emptyAt: 0},
		{start: 1, keys: []int{0}, emptyAt: -1},
	} {
		out, err := SetPageLabels(doc, []PageLabelRange{{Start: c.start, Style: "decimal", First: 1}})
		if err != nil {
			t.Fatal(err)
		}
		ctx := readCtx(t, out)
		root, err := ctx.XRefTable.Catalog()
		if err != nil {
			t.Fatal(err)
		}
		pl := derefDict(ctx.XRefTable, root["PageLabels"])
		nums := derefArray(ctx.XRefTable, pl["Nums"])
		var keys []int
		for i := 0; i+1 < len(nums); i += 2 {
			k, _ := nums[i].(types.Integer)
			keys = append(keys, k.Value())
		}
		if len(keys) != len(c.keys) || keys[0] != 0 {
			t.Errorf("a labelling from page %d wrote keys %v, want %v", c.start, keys, c.keys)
			continue
		}
		if c.emptyAt >= 0 {
			d := derefDict(ctx.XRefTable, nums[2*c.emptyAt+1])
			if _, s := d.Find("S"); s {
				t.Errorf("the leading pages were given a numbering style: %v", d)
			}
			if _, p := d.Find("P"); p {
				t.Errorf("the leading pages were given a prefix: %v", d)
			}
		}
	}
}

// TestAMergeErasesASourcesSignature — `/pending 812`: Append and Combine carried a signed source's signature
// into a document its /ByteRange does not cover, so an unsigned host came out `invalid`. The HOST's own
// signature is left to the commit's rule (/pending 455), and is asserted untouched here.
func TestAMergeErasesASourcesSignature(t *testing.T) {
	base, err := testpdf.Text("one", "two")
	if err != nil {
		t.Fatal(err)
	}
	other, err := testpdf.Text("x", "y")
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := sign.GenerateIdentity("Signer")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.SignApproval(base, cert, key, sign.Options{Name: "A", Reason: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !sign.HasSignatureBlob(signed) {
		t.Fatal("setup: the signed source carries no blob")
	}
	for name, run := range map[string]func() ([]byte, error){
		"Append":  func() ([]byte, error) { return Append(other, signed) },
		"Combine": func() ([]byte, error) { return Combine([][]byte{other, signed, other}) },
	} {
		out, err := run()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if sign.HasSignatureBlob(out) {
			t.Errorf("%s: a source's signature rode into the merged document", name)
		}
		if st := sign.Verify(out).State; st != sign.Unsigned {
			t.Errorf("%s: the merged document of an unsigned host reads %v, want unsigned", name, st)
		}
	}
	host, err := Append(signed, other)
	if err != nil {
		t.Fatal(err)
	}
	if !sign.HasSignatureBlob(host) {
		t.Error("Append erased the HOST's signature — that is the commit's rule to apply, not the merge's")
	}
}

// TestNUpSaysWhatItLeftBehind — `/pending 812`: `carryNoteAnnots` counts the annotations an n-up drops so the
// residue is a figure, and NUp discarded the figure.
func TestNUpSaysWhatItLeftBehind(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	filled, err := FillFormJSON(form, []byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Jane Doe"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var logged bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logged)
	defer log.SetOutput(prev)
	if _, err := NUp(filled, 2, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logged.String(), "annotation(s) were not carried") {
		t.Errorf("an n-up that dropped a form's widgets said nothing; log=%q", logged.String())
	}
	if (noteCarry{carried: 3}).residue() != "" {
		t.Error("a carry that left nothing behind reported a residue")
	}
}
