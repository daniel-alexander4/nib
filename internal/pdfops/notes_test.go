package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestAddNotes proves a note becomes a /Text annotation on the right page with
// the comment as its Contents.
func TestAddNotes(t *testing.T) {
	base := sizedPDF(t, 2, 200, 200)
	out, err := AddNotes(base, []Note{{Page: 1, X: 50, Y: 150, Text: "Check this clause"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadContext(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	found := ""
	page2 := 0
	eachPage(xt, root, func(page types.Dict, nr int) {
		for _, a := range derefArray(xt, page["Annots"]) {
			annot := derefDict(xt, a)
			if annot == nil || nameVal(annot, "Subtype") != "Text" {
				continue
			}
			if c, ok := annot.Find("Contents"); ok {
				if s, derr := xt.DereferenceText(c); derr == nil {
					if nr == 1 {
						found = s
					} else {
						page2++
					}
				}
			}
		}
	})
	if found != "Check this clause" {
		t.Errorf("page 1 note Contents = %q, want %q", found, "Check this clause")
	}
	if page2 != 0 {
		t.Errorf("page 2 has %d text notes, want 0 (note was for page 1 only)", page2)
	}
}

// TestAddNotesEmpty is a no-op for no notes.
func TestAddNotesEmpty(t *testing.T) {
	base := sizedPDF(t, 1, 200, 200)
	out, err := AddNotes(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, base) {
		t.Error("AddNotes(nil) should return the document unchanged")
	}
}
