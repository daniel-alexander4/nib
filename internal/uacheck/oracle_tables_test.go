package uacheck

import (
	"archive/zip"
	"bytes"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"sort"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
)

// The oracle's table-and-figure documents — `PLAN-accessibility.md` P09.S05.

// tableFigureCorpus is LibreOffice's own table (TH /Scope /Column) and figure (/Alt), each stripped of
// the value a new rule reads, and a variant with row headers written through the structure editor.
func tableFigureCorpus(t *testing.T) []oracleDoc {
	t.Helper()
	src, err := pdfops.ConvertOfficeToPDF(tableFigureODT(t), "odt")
	if err != nil {
		t.Fatalf("corpus: LibreOffice is present and could not convert the table-and-figure fixture: %v", err)
	}
	return []oracleDoc{
		{"LibreOffice table and figure", src},
		{"LibreOffice table and figure − /Alt", byteStripped(t, src, "/Alt", "/Xlt", 1)},
		{"LibreOffice table and figure − /Scope", byteStripped(t, src, "/Scope", "/Xcope", 2)},
		{"LibreOffice table with row headers", rowHeaders(t, src)},
	}
}

// byteStripped renames key to with. A byte edit is valid only where no object stream could hold the key
// (never byte-count a compressed PDF), and only when the key occurs exactly as often as it was measured
// to, so both are asserted first.
func byteStripped(t *testing.T, pdf []byte, key, with string, want int) []byte {
	t.Helper()
	if n := bytes.Count(pdf, []byte("/ObjStm")); n != 0 {
		t.Fatalf("corpus: the document has %d object stream(s), so a byte strip of %s cannot be trusted", n, key)
	}
	if n := bytes.Count(pdf, []byte(key)); n != want {
		t.Fatalf("corpus: %d occurrence(s) of %s, measured %d", n, key, want)
	}
	return bytes.ReplaceAll(pdf, []byte(key), []byte(with))
}

// rowHeaders keeps the header row's Column scopes and makes each data row's first cell a TH with Scope
// Row, through `pdfops.EditStructure` — the product door — so a table headed in both directions is
// compared with veraPDF.
func rowHeaders(t *testing.T, pdf []byte) []byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	var nums []int
	for n := range ctx.XRefTable.Table {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	var rows [][]int
	for _, n := range nums {
		e := ctx.XRefTable.Table[n]
		if e == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if s := d.NameEntry("S"); s == nil || *s != "TR" {
			continue
		}
		kids, _ := ctx.DereferenceArray(d["K"])
		var row []int
		for _, k := range kids {
			if ir, isRef := k.(types.IndirectRef); isRef {
				row = append(row, ir.ObjectNumber.Value())
			}
		}
		rows = append(rows, row)
	}
	if len(rows) != 3 || len(rows[0]) != 2 {
		t.Fatalf("corpus: the table reads %v, measured three rows of two cells", rows)
	}
	var edits []pdfops.StructureEdit
	for _, row := range rows[1:] {
		edits = append(edits,
			pdfops.StructureEdit{Kind: "retype", Element: row[0], Value: "TH"},
			pdfops.StructureEdit{Kind: "scope", Element: row[0], Value: "Row"})
	}
	out, err := pdfops.EditStructure(pdf, edits)
	if err != nil {
		t.Fatalf("corpus: the structure editor could not write row headers: %v", err)
	}
	return out
}

// tableFigureODT is a heading, a paragraph, a table with a header row, and an image with a title.
//
// **Not `oracleODT`'s container**, and the difference is measured: an embedded picture needs a manifest
// entry and a real `Modified` on every part, which `internal/pdfops`'s ODT builder records LibreOffice
// refusing without. The builder is not shared across packages for `buildPDF`'s stated reason.
func tableFigureODT(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{0x80, 0x80, 0x80, 0xff})
		}
	}
	var pic bytes.Buffer
	if err := png.Encode(&pic, img); err != nil {
		t.Fatal(err)
	}
	cell := func(s string) string {
		return `<table:table-cell office:value-type="string"><text:p>` + s + `</text:p></table:table-cell>`
	}
	body := `<text:h text:outline-level="1">Report</text:h><text:p>Intro paragraph.</text:p>` +
		`<table:table table:name="T1"><table:table-column table:number-columns-repeated="2"/>` +
		`<table:table-header-rows><table:table-row>` + cell("Name") + cell("Qty") + `</table:table-row></table:table-header-rows>` +
		`<table:table-row>` + cell("Apple") + cell("3") + `</table:table-row>` +
		`<table:table-row>` + cell("Pear") + cell("5") + `</table:table-row></table:table>` +
		`<text:p><draw:frame draw:name="img1" text:anchor-type="as-char" svg:width="2cm" svg:height="1.5cm">` +
		`<draw:image xlink:href="Pictures/a.png" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/>` +
		`<svg:title>A grey square</svg:title></draw:frame></text:p>`

	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	now := time.Now()
	mime := []byte("application/vnd.oasis.opendocument.text")
	mw, err := zw.CreateRaw(&zip.FileHeader{Name: "mimetype", Method: zip.Store, Modified: now, CRC32: crc32.ChecksumIEEE(mime),
		CompressedSize64: uint64(len(mime)), UncompressedSize64: uint64(len(mime))})
	if err != nil {
		t.Fatal(err)
	}
	mw.Write(mime)
	for _, part := range []struct {
		name string
		data []byte
	}{
		{"content.xml", []byte(`<?xml version="1.0" encoding="UTF-8"?><office:document-content` +
			` xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"` +
			` xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"` +
			` xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"` +
			` xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"` +
			` xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"` +
			` xmlns:xlink="http://www.w3.org/1999/xlink" office:version="1.2">` +
			`<office:body><office:text>` + body + `</office:text></office:body></office:document-content>`)},
		{"META-INF/manifest.xml", []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
			`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">` +
			`<manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/>` +
			`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>` +
			`<manifest:file-entry manifest:full-path="Pictures/a.png" manifest:media-type="image/png"/>` +
			`</manifest:manifest>`)},
		{"Pictures/a.png", pic.Bytes()},
	} {
		fw, ferr := zw.CreateHeader(&zip.FileHeader{Name: part.name, Method: zip.Deflate, Modified: now})
		if ferr != nil {
			t.Fatal(ferr)
		}
		fw.Write(part.data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
