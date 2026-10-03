package pdfread_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// formChain is a page drawing form 10, each form naming the next in its `/Resources /XObject` — `links` forms, the
// last naming `ring` (0 for none: the chain ends).
func formChain(links, ring int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R /Resources << /XObject << /F 10 0 R >> >> >>",
		4: "<< /Length 4 >>\nstream\n/F Do\nendstream",
	}
	for i := 0; i < links; i++ {
		next := ""
		switch {
		case i+1 < links:
			next = fmt.Sprintf("/Resources << /XObject << /F %d 0 R >> >> ", 11+i)
		case ring != 0:
			next = fmt.Sprintf("/Resources << /XObject << /F %d 0 R >> >> ", ring)
		}
		objs[10+i] = fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] %s/Length 1 >>\nstream\nq\nendstream", next)
	}
	return testpdf.Assemble(objs)
}

// TestAChainOfFormsDeeperThanTheBoundIsRefused — the reference door stopped at every edge pdfcpu guards, form
// XObjects among them, so a loop-free chain of forms reached pdfcpu's validator whole, one recursion per link: 700,000
// links (~120 MB) were `fatal error: stack overflow` inside `Validated` (/pending 803, measured), which no recover
// holds. The door now bounds the depth through guarded edges too; a shallow chain, and a ring pdfcpu stops at its
// own mark, still read.
func TestAChainOfFormsDeeperThanTheBoundIsRefused(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	if _, err := pdfread.Validated(formChain(9000, 0), conf); !errors.Is(err, pdfread.ErrReferenceDepth) {
		t.Fatalf("a 9,000-form chain: %v, want ErrReferenceDepth", err)
	}
	if _, err := pdfread.Validated(formChain(100, 0), conf); err != nil {
		t.Fatalf("control: a 100-form chain was refused: %v", err)
	}
	if _, err := pdfread.Validated(formChain(3, 10), conf); err != nil {
		t.Fatalf("control: three forms in a ring, which pdfcpu validates once each, were refused: %v", err)
	}
}

// TestAChainAlternatingGuardedAndUnguardedEdgesIsMeasuredWhole — the bound is over the whole recursion, not each
// kind of edge apart: a form whose tiling pattern's resources name the next form is two levels a link, of which the
// unguarded walk sees one and a forms-only count would see the other.
func TestAChainAlternatingGuardedAndUnguardedEdgesIsMeasuredWhole(t *testing.T) {
	const links = 5000 // 10,000 levels: past the bound together, under it apart
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R /Resources << /XObject << /F 10 0 R >> >> >>",
		4: "<< /Length 4 >>\nstream\n/F Do\nendstream",
	}
	for i := 0; i < links; i++ {
		form, pat := 10+2*i, 11+2*i
		res := ""
		if i+1 < links {
			res = fmt.Sprintf("/Resources << /Pattern << /P %d 0 R >> >> ", pat)
			objs[pat] = fmt.Sprintf("<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 1 1] "+
				"/XStep 1 /YStep 1 /Resources << /XObject << /F %d 0 R >> >> /Length 1 >>\nstream\nq\nendstream", form+2)
		}
		objs[form] = fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] %s/Length 1 >>\nstream\nq\nendstream", res)
	}
	if _, err := pdfread.Validated(testpdf.Assemble(objs), model.NewDefaultConfiguration()); !errors.Is(err, pdfread.ErrReferenceDepth) {
		t.Fatalf("a form/pattern chain 10,000 levels deep: %v, want ErrReferenceDepth", err)
	}
}
