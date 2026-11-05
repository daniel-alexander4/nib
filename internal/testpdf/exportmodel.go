package testpdf

import "fmt"

// ExportModelSet is the fixtures the document model's reader is judged on (PLAN-import-export P02.S03c, X6, X7): what
// it may read of a page, and the author's tree against the proposal. Each Expect names the property.
func ExportModelSet() []ExportFixture {
	fonts := "/Font << /F1 5 0 R /F2 6 0 R >>"
	base := map[int]string{5: exportFont, 6: exportBold}
	with := func(extra map[int]string) map[int]string {
		m := map[int]string{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	gray := "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\x00\nendstream"
	grayMasked := "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /SMask 11 0 R /Length 4 >>\nstream\n\x00\xff\xff\x00\nendstream"
	page := func(content, res string, extra map[int]string) []byte {
		return onePageWith(content, fonts+" "+res, "", with(extra))
	}
	keep := text("F1", 12, 100, 700, "kept heading words")
	under := text("F1", 12, 100, 600, "text under a box")
	alpha := "<< /Type /ExtGState /ca 0.05 >>"
	ocg := "<< /Type /OCG /Name (Layer) >>"
	ocCatalog := "<< /Type /Catalog /Pages 2 0 R /OCProperties << /OCGs [10 0 R] /D << /OFF [10 0 R] >> >> >>"
	tagged := func(content string, kids ...string) []byte {
		objs := with(map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-US) >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Resources << " + fonts + " >> /Contents 4 0 R >>",
			4:  stream(content),
			7:  "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /ParentTreeNextKey 1 >>",
			8:  "<< /Type /StructElem /S /Document /P 7 0 R /K [10 0 R 11 0 R] >>",
			9:  "<< /Nums [0 [10 0 R 11 0 R]] >>",
			10: "<< /Type /StructElem /S /H1 /P 8 0 R /Pg 3 0 R /K 0 >>",
			11: "<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K 1 >>",
		})
		for i, k := range kids {
			objs[10+i] = k
		}
		return assemble(objs)
	}
	mc := func(tag string, id int, body string) string {
		return fmt.Sprintf("/%s <</MCID %d>> BDC\n%sEMC\n", tag, id, body)
	}
	title := mc("H1", 0, text("F2", 20, 72, 720, "Main Title"))
	body := mc("P", 1, text("F1", 11, 72, 690, "Body paragraph words here"))
	return []ExportFixture{
		{"vis-alpha", page(keep+"/G1 gs\n"+under+"0 g 90 590 300 30 re f\n", "/ExtGState << /G1 10 0 R >>", map[int]string{10: alpha}), "a 5% box over text keeps it; counted maybeCovered"},
		{"vis-clip", page(keep+under+"q 0 0 50 50 re W n 0 g 90 590 300 30 re f Q\n", "", nil), "a clipped box keeps the text; counted maybeCovered"},
		{"vis-shaded-header", page("0.8 g 90 590 300 30 re f 0 g\n"+keep+under, "", nil), "a shaded box drawn BEFORE the text keeps it"},
		{"vis-image-over", page(keep+under+"q 300 0 0 40 90 590 cm /Im1 Do Q\n", "/XObject << /Im1 10 0 R >>", map[int]string{10: gray}), "an opaque image drawn after the text covers it"},
		{"vis-softmask-over", page(keep+under+"q 300 0 0 40 90 590 cm /Im1 Do Q\n", "/XObject << /Im1 10 0 R >>", map[int]string{10: grayMasked, 11: gray}), "a soft-masked image keeps the text"},
		{"vis-layer-off", func() []byte {
			return onePageWith(keep+"/OC /oc1 BDC\n"+under+"EMC\n", fonts+" /Properties << /oc1 10 0 R >>", "", with(map[int]string{1: ocCatalog, 10: ocg}))
		}(), "text in a layer OFF in /D is dropped; counted hiddenLayer"},
		{"vis-offpage", page(keep+text("F1", 12, 2000, 600, "far off the page"), "", nil), "text wholly off the page is dropped; counted offPage"},
		{"vis-scan-header", page("q 612 0 0 792 0 0 cm /Im1 Do Q\n"+text("F2", 14, 72, 740, "Typed header")+"BT /F1 12 Tf 3 Tr 100 500 Td (scanned body words) Tj ET\n",
			"/XObject << /Im1 10 0 R >>", map[int]string{10: gray}), "hidden OCR body is kept under a typed header"},
		{"vis-hidden-over-visible", page(keep+"BT /F1 12 Tf 3 Tr 100 700 Td (kept heading words) Tj ET\n", "", nil), "hidden text that visible text covers is dropped; counted hiddenText"},
		{"vis-artifact", page(keep+"/Artifact BMC\n"+under+"EMC\n", "", nil), "text inside an /Artifact sequence is not content: left out and counted artifactText"},
		{"vis-artifact-only", page("/Artifact BMC\n"+keep+"EMC\n", "", nil), "a page whose only text is an artifact still draws text: counted, not NoText, not a scan"},
		{"vis-artifact-over-picture", page("q 612 0 0 792 0 0 cm /Im1 Do Q\n/Artifact BMC\n"+keep+"EMC\n", "/XObject << /Im1 10 0 R >>", map[int]string{10: gray}), "a picture page with artifact text is a picture, not 'Scanned page': it draws text"},
		{"tagged-complete", tagged(title + body), "tree accounts for the page: source tree, H1 then P"},
		{"tagged-untagged-paragraph", tagged(title + body + text("F1", 11, 72, 660, "Untagged extra words")), "a visible paragraph the tree lacks: page flagged tree-incomplete, proposal used"},
		{"tagged-covered", tagged(title + body + "0 g 60 680 300 30 re f\n"), "tagged text beneath a box: the tree names words not seen, so the proposal is used and the words are not exported"},
	}
}
