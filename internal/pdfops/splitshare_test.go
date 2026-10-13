package pdfops

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// writeTime is a date and time as an XMP packet spells one.
var writeTime = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d[^<"]*`)

// catalogGraph renders everything pdf's catalog reaches as text that does not depend on object numbers: each
// object is written once, where a walk from the catalog first meets it (dictionary keys sorted, arrays in order),
// and every reference to it is its place in that order. Two files give the same text exactly when their object
// graphs are the same graph — the strongest comparison of two writes that number their objects differently.
func catalogGraph(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("reading a part back: %v", err)
	}
	xt := ctx.XRefTable
	seen := map[int]int{}
	var b strings.Builder
	var walk func(o types.Object)
	walk = func(o types.Object) {
		switch v := o.(type) {
		case nil:
			b.WriteString("null")
		case types.IndirectRef:
			nr := v.ObjectNumber.Value()
			if at, ok := seen[nr]; ok {
				fmt.Fprintf(&b, "#%d", at)
				return
			}
			seen[nr] = len(seen)
			fmt.Fprintf(&b, "#%d=", seen[nr])
			e, found := xt.FindTableEntryLight(nr)
			if !found || e == nil || e.Free {
				b.WriteString("free")
				return
			}
			walk(e.Object)
		case types.Dict:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			b.WriteString("<<")
			for _, k := range keys {
				b.WriteString("/" + k + " ")
				walk(v[k])
				b.WriteString(" ")
			}
			b.WriteString(">>")
		case types.StreamDict:
			if v.Dict.Type() != nil && *v.Dict.Type() == "Metadata" {
				// A packet carries the time of the write that made it (and so its compressed length moves);
				// everything else in it must agree.
				if err := v.Decode(); err != nil {
					t.Fatalf("decoding a metadata packet: %v", err)
				}
				b.WriteString("packet:" + writeTime.ReplaceAllString(string(v.Content), "<when>"))
				return
			}
			walk(v.Dict)
			fmt.Fprintf(&b, "stream:%x", sha256.Sum256(v.Raw))
		case types.Array:
			b.WriteString("[")
			for _, e := range v {
				walk(e)
				b.WriteString(" ")
			}
			b.WriteString("]")
		default:
			fmt.Fprintf(&b, "%T:%v", o, o)
		}
	}
	walk(*xt.Root)
	return b.String()
}

// partFacts is what a part IS, read through the doors the rest of nib reads a document through.
func partFacts(t *testing.T, pdf []byte) map[string]string {
	t.Helper()
	facts := map[string]string{"graph": catalogGraph(t, pdf)}
	n, err := PageCount(pdf)
	facts["pages"] = fmt.Sprint(n, err)
	d, err := ContentDigest(pdf)
	facts["digest"] = fmt.Sprint(d, err)
	st, err := ReadStructure(pdf)
	js, _ := json.Marshal(st)
	facts["structure"] = fmt.Sprint(string(js), err)
	facts["files"] = fmt.Sprint(shownFiles(t, pdf))
	af, err := ReadAnswerFacts(pdf)
	facts["answer"] = fmt.Sprintf("%+v %v", af, err)
	ol, err := Outline(pdf)
	facts["outline"] = fmt.Sprint(ol, err)
	facts["catalog"] = catalogKeys(t, pdf)
	facts["protected"] = fmt.Sprint(protectionOf(t, pdf))
	return facts
}

// splitFixtures are the documents the two paths are held against each other on: one of everything a selection
// decides about, each long enough for the shared path to build an intermediate document.
func splitFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	combine := func(pdfs ...[]byte) []byte {
		out, err := Combine(pdfs)
		if err != nil {
			t.Fatalf("building a fixture: %v", err)
		}
		return out
	}
	layered := layeredFixture(t, nil)
	return map[string][]byte{
		"tagged, one element a page":      identicalPagesFixture(9),
		"tagged, links and a shared tree": combine(subsetFixture(), subsetFixture()),
		"tagged, in page order, spanning": combine(pageOrderedFixture(true), pageOrderedFixture(false)),
		"page labels":                     pageLabelledFixture(t, 8),
		"outline":                         bookmarkedPDF(t, 8, []pdfcpu.Bookmark{{Title: "A", PageFrom: 1}, {Title: "B", PageFrom: 4}, {Title: "C", PageFrom: 7}}),
		"rich":                            combine(richFixture(t), richFixture(t)),
		"fields":                          combine(formOnPagesFixture(t), formOnPagesFixture(t)),
		"layers":                          combine(layered, layered),
		"named destinations":              combine(destFixture(t), destFixture(t)),
		"inherited resources":             combine(inheritedResourcesDoc(), sharedResourceDoc(true), sharedResourceDoc(false), inheritedResourcesDoc()),
		"everything": combine(subsetFixture(), richFixture(t), layered, pageLabelledFixture(t, 5), formOnPagesFixture(t),
			destFixture(t), identicalPagesFixture(4), sharedFormOnTwoPages()),
		"a marked form two pages draw": sharedFormOnTwoPages(),
		"owner-restricted":             ownerRestricted(t, combine(subsetFixture(), subsetFixture())),
	}
}

// sharedFormOnTwoPages is six tagged pages, the first two drawing ONE form that holds marked content. Either page
// alone carries its tree; the two together fail the carry's gate (a form's content is drawn twice), which is the
// case an intermediate document must not be trusted in.
func sharedFormOnTwoPages() []byte {
	form := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 0 0 Td (shared) Tj ET\nEMC\n"
	draw := "q 1 0 0 1 72 700 cm /Fm0 Do Q\n"
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R 4 0 R 20 0 R 23 0 R 26 0 R 29 0 R] /Count 6 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm0 11 0 R >> >> /Contents 6 0 R >>",
		4: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Fm0 11 0 R >> >> /Contents 6 0 R >>",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		6: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(draw), draw),
		8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [<< /Type /MCR /Pg 3 0 R /Stm 11 0 R /MCID 0 >>] >>",
		11: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 20] /StructParents 0 "+
			"/Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
	}
	kids, nums := "8 0 R", "0 [8 0 R]"
	for i := 0; i < 4; i++ {
		page, stream, elem := 20+3*i, 21+3*i, 22+3*i
		c := fmt.Sprintf("/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (page %d) Tj ET\nEMC\n", i+3)
		objs[page] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents %d 0 R /StructParents %d >>", stream, i+1)
		objs[stream] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c)
		objs[elem] = fmt.Sprintf("<< /Type /StructElem /S /P /P 7 0 R /Pg %d 0 R /K [0] >>", page)
		kids += fmt.Sprintf(" %d 0 R", elem)
		nums += fmt.Sprintf(" %d [%d 0 R]", i+1, elem)
	}
	objs[7] = fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] /ParentTree 9 0 R /ParentTreeNextKey 5 >>", kids)
	objs[9] = fmt.Sprintf("<< /Nums [%s] >>", nums)
	return assembleFixture(objs)
}

// splitSelections are the shapes of split a document of n pages is cut into: one page a part, the same backwards,
// runs, and parts that overlap and repeat a page another part has.
func splitSelections(n int) map[string][]string {
	var singles, backwards, runs, overlapping []string
	for p := 1; p <= n; p++ {
		singles = append(singles, fmt.Sprint(p))
		backwards = append(backwards, fmt.Sprint(n+1-p))
		if p%2 == 1 && p < n {
			runs = append(runs, fmt.Sprintf("%d-%d", p, p+1))
		}
		thru := p + 2
		if thru > n {
			thru = n
		}
		overlapping = append(overlapping, fmt.Sprintf("%d-%d", p, thru), fmt.Sprint(p))
	}
	return map[string][]string{"one page a part": singles, "backwards": backwards, "runs of two": runs, "overlapping, repeating": overlapping}
}

// TestASharedSplitGivesThePartsTheDirectSplitGives — `/pending 834`. A split's parts are DEFINED as one `Collect`
// of the source per part (`collectEachDirect`); the shared path cuts each from a smaller document, and must give
// the same parts: the same object graph under the catalog, digest, structure, files, facts and outline.
func TestASharedSplitGivesThePartsTheDirectSplitGives(t *testing.T) {
	shared, abandoned := 0, 0
	for name, pdf := range splitFixtures(t) {
		n, err := PageCount(pdf)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for shape, sels := range splitSelections(n) {
			if len(sels) < minSharedParts {
				continue // the shared path is not asked; the count below says how many splits it was
			}
			want, _, err := collectEachDirect(pdf, sels)
			if err != nil {
				t.Fatalf("%s, %s: the direct split: %v", name, shape, err)
			}
			got, _, err := collectEach(pdf, sels)
			if err != nil {
				t.Fatalf("%s, %s: the split: %v", name, shape, err)
			}
			if _, ok := collectEachShared(pdf, sels); ok {
				shared++
			} else {
				abandoned++
			}
			for i := range want {
				w, g := partFacts(t, want[i]), partFacts(t, got[i])
				for k := range w {
					if w[k] != g[k] {
						t.Errorf("%s, %s, part %d (%s): %s differs from the direct split's\n shared: %.4000s\n direct: %.4000s",
							name, shape, i+1, sels[i], k, g[k], w[k])
					}
				}
			}
		}
	}
	// Both halves must have been exercised, or the comparison above was of the direct path with itself.
	if shared < 30 || abandoned < 1 {
		t.Errorf("the shared path answered %d splits and was abandoned for %d; want it answering most and abandoned for the marked form two pages draw", shared, abandoned)
	}
}

// TestASharedSplitIsAbandonedWhereAnIntermediateWouldLoseTheTree is the gate by itself: pages 1 and 2 draw one
// marked form, so the document holding both has no tree while each page cut from the source has one.
func TestASharedSplitIsAbandonedWhereAnIntermediateWouldLoseTheTree(t *testing.T) {
	pdf := sharedFormOnTwoPages()
	sels := []string{"1", "2", "3", "4", "5", "6"}
	if _, ok := collectEachShared(pdf, sels); ok {
		t.Fatalf("the shared path answered a split whose intermediate document (pages 1-3) fails the carry's gate")
	}
	parts, _, err := collectEach(pdf, sels)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range parts {
		if i == 1 {
			continue // page 2 draws the form of an element that belongs to page 1: it has no element of its own
		}
		if st, err := ReadStructure(p); err != nil || !st.Tagged || len(st.Elements) == 0 {
			t.Errorf("part %d has no structure (tagged %v, %d elements, err %v); cut from the source it carries its page's", i+1, st.Tagged, len(st.Elements), err)
		}
	}
}

// TestASharedSplitRefusesAsTheDirectSplitDoes: a refusal is the direct path's, word for word and for the same part.
func TestASharedSplitRefusesAsTheDirectSplitDoes(t *testing.T) {
	pdf := identicalPagesFixture(8)
	for _, sels := range [][]string{
		{"1", "2", "3", "9", "5"},
		{"1", "2", "3", "4", "0"},
		{"1", "2", "x", "4", "5"},
		{"1", "2", "3", "4", strings.Repeat("1,", 120) + "1"},
	} {
		_, wantAt, want := collectEachDirect(pdf, sels)
		_, gotAt, got := collectEach(pdf, sels)
		if want == nil || got == nil || want.Error() != got.Error() || wantAt != gotAt {
			t.Errorf("%v: refused with %q for part %d, the direct split with %q for part %d", sels, got, gotAt, want, wantAt)
		}
	}
}
