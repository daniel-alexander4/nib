package testpdf

import "strconv"

// TwoPictures is a one-page document drawing two different 4×4 grey pictures, each in its own `q … Q`, and a line
// of text: the fixture the page objects' route, CLI and wrapper tests remove a picture from (PLAN-image-edit
// P01.S02). The first picture's bytes are `NIBPLANTEDIMAGEA`, the second's `NIBPLANTEDIMAGEB`.
func TwoPictures() []byte {
	content := "q 100 0 0 80 10 10 cm /Im0 Do Q\nq 50 0 0 40 200 200 cm /Im1 Do Q\nBT /F1 12 Tf 10 700 Td (Hello) Tj ET"
	image := func(pattern string) string {
		return "<< /Type /XObject /Subtype /Image /Width 4 /Height 4 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 16 >>\nstream\n" + pattern + "\nendstream"
	}
	return assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R /Im1 7 0 R >> >> /Contents 4 0 R >>",
		4: "<< /Length " + strconv.Itoa(len(content)) + " >>\nstream\n" + content + "\nendstream",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: image("NIBPLANTEDIMAGEA"),
		7: image("NIBPLANTEDIMAGEB"),
	})
}
