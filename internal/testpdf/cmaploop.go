package testpdf

import (
	"bytes"
	"fmt"
)

// Documents whose embedded CMaps name each other through `/UseCMap` — `/pending 675`. pdfcpu's validator
// recursed on the loop until the process died; every door that reads a PDF is tested against these.

// assemble writes objs as a classic-xref PDF.
func assemble(objs map[int]string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	maxN := 0
	for n := range objs {
		maxN = max(maxN, n)
	}
	offs := map[int]int{}
	for n := 1; n <= maxN; n++ {
		if body, ok := objs[n]; ok {
			offs[n] = b.Len()
			fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
		}
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", maxN+1)
	for n := 1; n <= maxN; n++ {
		if off, ok := offs[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", maxN+1, xref)
	return b.Bytes()
}

const sysInfo = "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >>"

// cmap is an embedded CMap stream named name, with use as its `/UseCMap` value ("" for none).
func cmap(name, use string) string {
	prog := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /" + name + " def " +
		"1 begincodespacerange <0000> <FFFF> endcodespacerange 1 begincidrange <0000> <FFFF> 0 endcidrange " +
		"endcmap CMapName currentdict /CMap defineresource pop end end"
	if use != "" {
		use = "/UseCMap " + use + " "
	}
	return fmt.Sprintf("<< /Type /CMap /CMapName /%s %s%s /Length %d >>\nstream\n%s\nendstream", name, use, sysInfo, len(prog), prog)
}

// type0WithCMaps is one page showing text in a Type 0 font whose /Encoding is the embedded CMap at 20.
func type0WithCMaps(cmaps map[int]string) []byte {
	content := "BT /F0 12 Tf 10 10 Td <2121> Tj ET"
	o := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /Font << /F0 10 0 R >> >> >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		10: "<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding 20 0 R /DescendantFonts [11 0 R] >>",
		11: "<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light " + sysInfo + " /FontDescriptor 12 0 R >>",
		12: "<< /Type /FontDescriptor /FontName /STSong-Light /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 " +
			"/Ascent 880 /Descent -120 /CapHeight 880 /StemV 80 >>",
	}
	for k, v := range cmaps {
		o[k] = v
	}
	return assemble(o)
}

// UseCMapLoops are the two `/UseCMap` loop shapes (`/pending 675`), each with the object chain its loop follows
// from the font's /Encoding at 20: two CMaps naming each other, and one naming itself.
func UseCMapLoops() map[string]struct {
	PDF  []byte
	Loop []int
} {
	return map[string]struct {
		PDF  []byte
		Loop []int
	}{
		"two CMaps naming each other": {type0WithCMaps(map[int]string{20: cmap("A", "21 0 R"), 21: cmap("B", "20 0 R")}), []int{20, 21, 20}},
		"a CMap naming itself":        {type0WithCMaps(map[int]string{20: cmap("A", "20 0 R")}), []int{20, 20}},
	}
}

// UseCMapChain is the legitimate shape: the CMap at 20 uses the one at 21, which uses a predefined CMap by name.
func UseCMapChain() []byte {
	return type0WithCMaps(map[int]string{20: cmap("A", "21 0 R"), 21: cmap("B", "/GB-EUC-H")})
}

// FormChain is depth form XObjects of one /Length, each drawing the next under its own /Resources —
// `/pending 706`'s comparison shape, over which pdfcpu's optimize pass cost 90 s at 400. A test outside
// pdfops and uacheck (each of which keeps its own copy beside the measurements it names) takes it from here.
func FormChain(depth int) []byte {
	form := func(extra string) string {
		return fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] %s /Length 5 >>\nstream\n/X Do\nendstream", extra)
	}
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X 100 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length 5 >>\nstream\n/X Do\nendstream",
	}
	for i := 0; i < depth; i++ {
		objs[100+i] = form(fmt.Sprintf("/Resources << /XObject << /X %d 0 R >> >>", 101+i))
	}
	objs[100+depth] = form("")
	return assemble(objs)
}
