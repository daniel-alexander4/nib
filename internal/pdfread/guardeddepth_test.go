package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
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

// formsNamingTheirDictionary is n forms whose /Resources all name the one indirect /XObject dictionary that holds
// them, which the page draws from — one strongly connected component of n forms, every edge in it guarded.
func formsNamingTheirDictionary(n int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R /Resources << /XObject 6 0 R >> >>",
		4: "<< /Length 6 >>\nstream\n/F0 Do\nendstream",
	}
	var names strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&names, "/F%d %d 0 R ", i, 100+i)
		objs[100+i] = "<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /XObject 6 0 R >> /Length 1 >>\nstream\nq\nendstream"
	}
	objs[6] = "<< " + names.String() + ">>"
	return testpdf.Assemble(objs)
}

// TestAComponentIsChargedByItsGuardedTargetsAndItsUnguardedPath — ADR-077 charged a cycle of n objects ((n+2)/2)²,
// the worst split of n into guarded and unguarded any component could have, so 2,000 forms naming their own
// `/XObject` dictionary were a million levels and refused, a fixture `RemovePages` had always read (`/pending 706`).
// pdfcpu marks each form before recursing through it (xObject.go:762) and the dictionary is no node of its own, so its
// deepest stack is 2,000 forms: the charge is (g+1)·l (ADR-081), here 2,001 × 1. Past the bound an all-guarded
// component is still refused, because there the stack really is that deep: a ring of 9,000 forms is 9,000 levels from
// wherever pdfcpu enters it.
func TestAComponentIsChargedByItsGuardedTargetsAndItsUnguardedPath(t *testing.T) {
	conf := model.NewDefaultConfiguration()
	if _, err := pdfread.Validated(formsNamingTheirDictionary(2000), conf); err != nil {
		t.Fatalf("2,000 forms naming their own dictionary, 2,000 levels at most, were refused: %v", err)
	}
	if _, err := pdfread.Validated(formChain(9000, 10), conf); !errors.Is(err, pdfread.ErrReferenceDepth) {
		t.Fatalf("a ring of 9,000 forms, 9,000 levels deep: %v, want ErrReferenceDepth", err)
	}
}

// TestAnUnguardedPathIsWalkedOncePerGuardedTarget — the charge's other factor. k forms, each a tiling pattern of the
// next (an unguarded edge: `validatePattern` dereferences without a mark, pattern.go:127), the last also drawing every
// one of them as a form (guarded). pdfcpu's worst order enters each form once as an XObject and walks the rest of the
// pattern chain under each entry — k + (k-1) + … = k(k+1)/2 levels, 8,515 at k = 130, past the bound. ((n+2)/2)²
// charged it 4,356 and let it through; (g+1)·l charges 131 × 130.
func TestAnUnguardedPathIsWalkedOncePerGuardedTarget(t *testing.T) {
	read := func(k, named int) error {
		objs := map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R /Resources << /XObject << /F 10 0 R >> >> >>",
			4: "<< /Length 4 >>\nstream\n/F Do\nendstream",
		}
		var all strings.Builder
		for i := 0; i < named; i++ {
			fmt.Fprintf(&all, "/F%d %d 0 R ", i, 10+i)
		}
		for i := 0; i < k; i++ {
			res := fmt.Sprintf("/Resources << /Pattern << /P %d 0 R >> >> ", 11+i)
			if i == k-1 {
				res = "/Resources << /XObject << " + all.String() + ">> >> "
			}
			objs[10+i] = fmt.Sprintf("<< /Subtype /Form /PatternType 1 /PaintType 1 /TilingType 1 "+
				"/XStep 1 /YStep 1 /BBox [0 0 10 10] %s/Length 1 >>\nstream\nq\nendstream", res)
		}
		_, err := pdfread.Validated(testpdf.Assemble(objs), model.NewDefaultConfiguration())
		return err
	}
	if err := read(130, 130); !errors.Is(err, pdfread.ErrReferenceDepth) {
		t.Fatalf("130 forms re-walking their pattern chain once per form, ~8,500 levels: %v, want ErrReferenceDepth", err)
	}
	if err := read(60, 60); err != nil {
		t.Fatalf("control: 60 forms, ~1,800 levels, did not read: %v", err)
	}
	// g counts what a GUARDED edge enters: closed by one form reference, 200 pattern links are entered once, 200
	// levels (charged 2 × 200). Counting every target as guarded charges 201 × 200 and refuses it.
	if err := read(200, 1); err != nil {
		t.Fatalf("control: a 200-link pattern ring closed by one form, 200 levels, did not read: %v", err)
	}
}

// TestASharedDictionaryOfGuardedEntriesIsOneNode — the depth pass read through an indirect `/XObject`, `/Font`,
// `/Resources` or `/AP` dictionary as if each object naming it held its entries, so n objects naming one dictionary of
// n entries were n² edges: 4,000 forms naming their own `/XObject` dictionary (~300 KB) took the pass 22 s, before
// pdfcpu's validator had started. Each such dictionary is a node of its own now; the pass is timed alone, against a
// ceiling far above its ~50 ms and far below the quadratic's minutes.
func TestASharedDictionaryOfGuardedEntriesIsOneNode(t *testing.T) {
	widgets := func(n int) []byte {
		objs := map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			6: "<< /N 7 0 R >>",
		}
		var annots, states strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&annots, "%d 0 R ", 100+i)
			objs[100+i] = "<< /Type /Annot /Subtype /Widget /Rect [0 0 1 1] /AP 6 0 R >>"
			fmt.Fprintf(&states, "/S%d %d 0 R ", i, 100+n+i)
			objs[100+n+i] = "<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] /Length 1 >>\nstream\nq\nendstream"
		}
		objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Annots [" + annots.String() + "] >>"
		objs[7] = "<< " + states.String() + ">>"
		return testpdf.Assemble(objs)
	}
	for _, c := range []struct {
		name string
		pdf  []byte
	}{
		{"8,000 forms naming their own /XObject dictionary", formsNamingTheirDictionary(8000)},
		{"8,000 widgets sharing one /AP of 8,000 states", widgets(8000)},
	} {
		ctx, err := api.ReadContext(bytes.NewReader(c.pdf), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		start := time.Now()
		_ = pdfread.ValidatorDepth(ctx)
		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("%s: the depth pass took %v", c.name, d)
		}
	}
}

// TestAnIndirectResourceDictionaryIsNoLevel — a pass-through node is a dictionary pdfcpu reads in the same frames
// whether it is direct or indirect (measured on `/XObject`: 1,081 bytes of stack a form level either way), so it adds
// no level. Counted as one, a chain of 3,000 forms each reaching the next through an indirect `/Resources` and an
// indirect `/XObject` would be 9,000 levels and refused; it is 3,000.
func TestAnIndirectResourceDictionaryIsNoLevel(t *testing.T) {
	const links = 3000
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R /Resources << /XObject << /F 10 0 R >> >> >>",
		4: "<< /Length 4 >>\nstream\n/F Do\nendstream",
	}
	for i := 0; i < links; i++ {
		form, res, xobj := 10+3*i, 11+3*i, 12+3*i
		next := ""
		if i+1 < links {
			next = fmt.Sprintf("/Resources %d 0 R ", res)
			objs[res] = fmt.Sprintf("<< /XObject %d 0 R >>", xobj)
			objs[xobj] = fmt.Sprintf("<< /F %d 0 R >>", form+3)
		}
		objs[form] = fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] %s/Length 1 >>\nstream\nq\nendstream", next)
	}
	if _, err := pdfread.Validated(testpdf.Assemble(objs), model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("a chain of 3,000 forms through indirect resource dictionaries, 3,000 levels: %v", err)
	}
}
