package pdfops

import (
	"fmt"
	"testing"

	"nib/internal/testpdf"
)

// TestAPagesPicturesComeInOneOrderEveryRun — pdfcpu returns a page's pictures from a map, so their order
// changed between runs (the corpus pass caught two swapped on a real document). `nib images` names a file and
// numbers a picture by that order, and a program that stores either must mean the same picture next time.
func TestAPagesPicturesComeInOneOrderEveryRun(t *testing.T) {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
	}
	var xobjs, content string
	const n = 8
	for i := 0; i < n; i++ {
		num := 10 + i
		// Gray bytes 100+i; the object number rises with i, so the order wanted is the byte order.
		objs[num] = fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 1 >>\nstream\n%c\nendstream", rune(100+i))
		xobjs += fmt.Sprintf("/Im%d %d 0 R ", i, num)
		content += fmt.Sprintf("q 10 0 0 10 %d 0 cm /Im%d Do Q\n", 10*i, i)
	}
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /XObject << " + xobjs + ">> >> >>"
	objs[4] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
	pdf := testpdf.Assemble(objs)
	var first []string
	for run := 0; run < 25; run++ {
		files, err := ExtractImageFiles(pdf)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != n {
			t.Fatalf("%d pictures, want %d: the fixture reaches nothing", len(files), n)
		}
		var got []string
		for i, f := range files {
			if f.Page != 1 || f.Index != i+1 {
				t.Fatalf("picture %d is page %d index %d, want page 1 index %d", i, f.Page, f.Index, i+1)
			}
			got = append(got, fmt.Sprintf("%d:%d", f.objNr, len(f.Data)))
			if i > 0 && f.objNr <= files[i-1].objNr {
				t.Fatalf("run %d: object %d follows %d: not ascending", run, f.objNr, files[i-1].objNr)
			}
		}
		if first == nil {
			first = got
		} else if fmt.Sprint(first) != fmt.Sprint(got) {
			t.Fatalf("run %d order %v differs from run 0 %v", run, got, first)
		}
	}
}
