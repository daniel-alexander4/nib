package testpdf

import "fmt"

// PassSkipped is a one-page document pdfcpu's optimize pass is not run over, so Nib skips it (`pdfread.Unaffordable`):
// a page drawing one form whose `/Foo` — a key pdfcpu's validator never follows — names a chain `N 0 obj [N+1 0 R]`
// longer than the estimate's `maxShapeDepth` (1<<13). It reads, validates and writes like any other.
func PassSkipped() []byte {
	const links = 9000
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R /Resources << /XObject << /Fm0 5 0 R >> >> >>",
		4: "<< /Length 8 >>\nstream\n/Fm0 Do\nendstream",
		5: "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Foo 10 0 R /Length 0 >>\nstream\n\nendstream",
	}
	for i := 0; i < links; i++ {
		objs[10+i] = fmt.Sprintf("[%d 0 R]", 11+i)
	}
	objs[10+links] = "[]"
	return Assemble(objs)
}
