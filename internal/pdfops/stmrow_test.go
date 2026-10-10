package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The P06 phase-close review's findings on the carry's gate (/pending 665, R2 #2 and #3).

// pageAndFormSameMCIDFixture is a page with its own /MCID 0 and a form it draws with another /MCID 0, each
// claiming its own `/ParentTree` row — ADR-038's shape, and well formed. formRow is the form's row and
// formMCID the MCID the form's element claims, so a test can break either.
func pageAndFormSameMCIDFixture(formRow string, formMCID int) []byte {
	form := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 0 0 Td (in the form) Tj ET\nEMC\n"
	page := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 700 Td (on the page) Tj ET\nEMC\n" +
		"q 1 0 0 1 72 400 cm /Fm0 Do Q\n"
	return assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 " +
			"/Resources << /Font << /F1 5 0 R >> /XObject << /Fm0 11 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R] 1 " + formRow + "] >>",
		10: fmt.Sprintf("<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [<< /Type /MCR /Pg 3 0 R /Stm 11 0 R /MCID %d >>] >>", formMCID),
		11: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 20] /StructParents 1 "+
			"/Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
	})
}

func defectKeys(defects []structDefect) string {
	keys := make([]string, len(defects))
	for i, d := range defects {
		keys[i] = d.key
	}
	return strings.Join(keys, "; ")
}

func hasStructTree(t *testing.T, pdf []byte) bool {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	_, has := root["StructTreeRoot"]
	return has
}

// TestAnMCIDInAFormIsCheckedAgainstTheFormsRow — the check resolved every MCID through the PAGE's row, so a
// form's /MCID 0 was compared with the element that owns the page's /MCID 0 and the document was called
// inconsistent. The carry then dropped the tree, on the identity selection too.
func TestAnMCIDInAFormIsCheckedAgainstTheFormsRow(t *testing.T) {
	good := pageAndFormSameMCIDFixture("[10 0 R]", 0)
	if d := completeness(t, good); len(d) != 0 {
		t.Errorf("a page and a form it draws each hold an /MCID 0 in a row of their own, and the gate reports: %s", defectKeys(d))
	}
	out, err := Collect(good, []string{"1"})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !hasStructTree(t, out) {
		t.Error("keeping the document's one page dropped its structure tree")
	}

	// The form's row is still held to the same two invariants, by its own key.
	if d := completeness(t, pageAndFormSameMCIDFixture("[8 0 R]", 0)); !defectsKeyed(d, "mcid-owner key=1 mcid=0 obj=10") {
		t.Errorf("the form's row names another element as the owner of its /MCID 0, and that is not reported: %s", defectKeys(d))
	}
	if d := completeness(t, pageAndFormSameMCIDFixture("[10 0 R]", 3)); !defectsKeyed(d, "mcid-range obj=10 key=1 mcid=3") {
		t.Errorf("an element claims /MCID 3 of a form whose row has one slot, and that is not reported: %s", defectKeys(d))
	}
}

// TestAnAppearanceStreamsClaimIsSeenByTheGate — the carry's enumeration walks an annotation's appearance
// forms and the gate's did not, so a row only an appearance claims was "unowned" to the gate.
func TestAnAppearanceStreamsClaimIsSeenByTheGate(t *testing.T) {
	ap := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 0 0 Td (in the appearance) Tj ET\nEMC\n"
	page := "/P <</MCID 0>> BDC\nBT /F1 12 Tf 72 700 Td (on the page) Tj ET\nEMC\n"
	pdf := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Annots [12 0 R] " +
			"/Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [<< /Type /MCR /Pg 3 0 R /Stm 11 0 R /MCID 0 >>] >>",
		11: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 200 20] /StructParents 1 "+
			"/Resources << /Font << /F1 5 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(ap), ap),
		12: "<< /Type /Annot /Subtype /Stamp /Rect [72 300 272 320] /Contents (a stamp) /AP << /N 11 0 R >> >>",
	})
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Stimulus: the carry's own enumeration finds the claim, so the two are being compared over a subject.
	var carrySees []int
	eachParentTreeClaim(ctx, func(key int, _ func(int)) { carrySees = append(carrySees, key) })
	if fmt.Sprint(carrySees) != "[0 1]" {
		t.Fatalf("setup: the carry's enumeration sees keys %v, want [0 1] — the appearance stream is not claiming one", carrySees)
	}
	if owners := parentTreeOwners(ctx); len(owners[1]) != 1 {
		t.Errorf("key 1 is claimed by an annotation's appearance stream and the gate's enumeration finds %d owner(s): %v",
			len(owners[1]), owners)
	}
	if d := completeness(t, pdf); defectsKeyed(d, "unowned-key") {
		t.Errorf("the gate calls a row an appearance stream claims unowned: %s", defectKeys(d))
	}
}
