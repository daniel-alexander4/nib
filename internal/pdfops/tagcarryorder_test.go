package pdfops

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// identicalPagesFixture is n pages with BYTE-IDENTICAL content, each tagged, page i carrying
// `/StructParents` i-1 — `repeatedPagesFixture` at any length.
func identicalPagesFixture(n int) []byte {
	content := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (the same words) Tj ET\nEMC\n"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var kids, elems, nums strings.Builder
	for i := 0; i < n; i++ {
		page, stream, elem := 10+3*i, 11+3*i, 12+3*i
		fmt.Fprintf(&kids, "%d 0 R ", page)
		fmt.Fprintf(&elems, "%d 0 R ", elem)
		fmt.Fprintf(&nums, "%d [%d 0 R] ", i, elem)
		objs[page] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents %d 0 R /StructParents %d >>", stream, i)
		objs[stream] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
		objs[elem] = fmt.Sprintf("<< /Type /StructElem /S /P /P 7 0 R /Pg %d 0 R /K [0] >>", page)
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), n)
	objs[7] = fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] /ParentTree 9 0 R /ParentTreeNextKey %d >>", elems.String(), n)
	objs[9] = fmt.Sprintf("<< /Nums [%s] >>", nums.String())
	return assembleFixture(objs)
}

// formKeys is each sheet's form XObjects by resource name with the `/StructParents` each carries:
// `1: Fm1=0 Fm2=1`. It is what the carry decides — which form anchors which source page.
func formKeys(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadAndValidate(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var b strings.Builder
	for p := 1; p <= ctx.PageCount; p++ {
		d, _, _, perr := ctx.PageDict(p, false)
		if perr != nil || d == nil {
			t.Fatalf("page %d: %v", p, perr)
		}
		res, _ := ctx.DereferenceDict(d["Resources"])
		xod, _ := ctx.DereferenceDict(res["XObject"])
		var parts []string
		for name, v := range xod {
			sd, _, serr := ctx.DereferenceStreamDict(v)
			if serr != nil || sd == nil {
				t.Fatalf("page %d /%s: %v", p, name, serr)
			}
			key := "none"
			if k := sd.Dict.IntEntry("StructParents"); k != nil {
				key = fmt.Sprint(*k)
			}
			parts = append(parts, fmt.Sprintf("%s=%s", name, key))
		}
		sort.Strings(parts)
		fmt.Fprintf(&b, "%d: %s\n", p, strings.Join(parts, " "))
	}
	return b.String()
}

// TestTheCarryPairsFormsWithPagesTheSameWayEveryTime — /pending 601. The carry matched forms to source pages
// by ranging over two maps (the sheet's `/XObject` dictionary and the captured pages), so where pages draw
// the same content — the one case in which more than one pairing matches — which form took which page's
// `/StructParents` changed from run to run, and with it which sheet position an element's content was read
// from. Both sides are now walked in order: forms by name (`Fm2` before `Fm10`), pages by page number. Go
// randomises a map's iteration per `range`, so repeating the n-up is the stimulus.
//
// **It is the PAIRING that is compared, and the file's bytes cannot be.** pdfcpu's writer stamps a random
// `/ID` and the time on each write, and its optimize pass picks which of two equal forms survives a fusion
// from a map of its own, so no two n-ups are byte-equal or even number their objects alike (measured: 39 of
// 39 differ byte for byte, and 5-14 of 39 by object number, with the carry's order fixed).
//
// Twelve pages on one sheet, so the names reach two digits: a plain string sort pairs `Fm10` with page 2.
func TestTheCarryPairsFormsWithPagesTheSameWayEveryTime(t *testing.T) {
	const pages = 12
	src := identicalPagesFixture(pages)
	if d := completeness(t, src); len(d) > 0 {
		t.Fatalf("setup: the fixture is incomplete before the n-up:%s", defectLines(d))
	}
	var want []string
	for i := 1; i <= pages; i++ {
		want = append(want, fmt.Sprintf("Fm%d=%d", i, i-1))
	}
	sort.Strings(want)
	wantKeys := "1: " + strings.Join(want, " ") + "\n"

	const runs = 30
	wrong := 0
	for i := 0; i < runs; i++ {
		out, err := NUp(src, pages, false)
		if err != nil {
			t.Fatal(err)
		}
		if got := fate(out); got != "carried" {
			t.Fatalf("run %d: the n-up measures %q, want %q — nothing was carried, so nothing is compared", i, got, "carried")
		}
		if got := formKeys(t, out); got != wantKeys {
			if wrong == 0 {
				t.Logf("run %d pairs\n%swant\n%s", i, got, wantKeys)
			}
			wrong++
		}
	}
	if wrong > 0 {
		t.Errorf("%d of %d n-ups of one document paired a form with a page other than the one it is drawn for; "+
			"the pairing follows the order a map was walked in", wrong, runs)
	}
}
