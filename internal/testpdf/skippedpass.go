package testpdf

import "fmt"

// PassSkipped is a one-page document pdfcpu's optimize pass cannot be run over, so Nib skips it
// (`pdfread.Unaffordable`): an image XObject whose `/Subtype` is written as a reference, which the relaxed
// validator reads through and the pass dereferences as a name. It reads, validates and writes like any other.
func PassSkipped() []byte {
	const font = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	const image = "<< /Type /XObject /Subtype %s /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\n\x00\nendstream"
	return Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources << /Font << /F1 8 0 R /F2 9 0 R >> /XObject << /I1 6 0 R /I2 7 0 R >> >> >>",
		6:  fmt.Sprintf(image, "20 0 R"),
		7:  fmt.Sprintf(image, "/Image"),
		8:  font,
		9:  font,
		20: "/Image",
	})
}
