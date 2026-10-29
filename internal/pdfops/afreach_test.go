package pdfops

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// afMarkers are the bodies of the fixture's three embedded streams (associatedFilesShape): the catalog's, the
// page's and the structure element's file.
var afMarkers = []string{"CATALOG-BYTES", "PAGE-BYTES", "ELEM-BYTES"}

// filesReachable returns, sorted, which of afMarkers sit in an embedded stream REACHABLE from the trailer's
// /Root — a graph walk over references, never a search of the bytes (P02.S06, D6): the output's objects are in
// compressed streams, and an unreachable object is not a file anyone can open.
func filesReachable(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-reading the output: %v", err)
	}
	xt := ctx.XRefTable
	seen := map[int]bool{}
	found := map[string]bool{}
	var walk func(o types.Object)
	walk = func(o types.Object) {
		switch v := o.(type) {
		case types.IndirectRef:
			n := v.ObjectNumber.Value()
			if seen[n] {
				return
			}
			seen[n] = true
			r, err := xt.Dereference(v)
			if err != nil || r == nil {
				return
			}
			walk(r)
		case types.StreamDict:
			if v.Decode() == nil {
				for _, m := range afMarkers {
					if strings.Contains(string(v.Content), m) {
						found[m] = true
					}
				}
			}
			walk(v.Dict)
		case types.Dict:
			for _, e := range v {
				walk(e)
			}
		case types.Array:
			for _, e := range v {
				walk(e)
			}
		}
	}
	walk(*xt.Root)
	var out []string
	for m := range found {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// TestARemovedAssociatedFileIsReachableFromNothing — D6 / P02.S06. pdfcpu v0.16.2 writes the catalog's `/AF`
// (`write.go:382`), so a file the user removed from /EmbeddedFiles that the catalog's /AF also names — PDF/A-3
// lists every embedded file in both — was written back in behind the removal. One test per door.
func TestARemovedAssociatedFileIsReachableFromNothing(t *testing.T) {
	// Stimulus: nothing has been removed yet, and every file is reachable from the source.
	src := associatedFilesShape("1.7", true, "catalog page annot")
	if got := strings.Join(filesReachable(t, src), ","); got != "CATALOG-BYTES,ELEM-BYTES,PAGE-BYTES" {
		t.Fatalf("setup: the fixture reaches %q, want all three files", got)
	}
	doors := map[string]func([]byte) ([]byte, error){
		"RemoveFilesAndMedia": RemoveFilesAndMedia,
		"StripActive":         StripActive,
	}
	for name, door := range doors {
		out, err := door(src)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := filesReachable(t, out); len(got) != 0 {
			t.Errorf("%s: files still reachable from the trailer after the removal: %v", name, got)
		}
	}
}

// TestAnExtractedOrRedactedDocumentCarriesNoAssociatedFile — D6 / P02.S06, the other doors. Pages are taken out
// of a document whose three files are listed in /EmbeddedFiles AND named by /AF (catalog, page, structure
// element); the result is a document handed over, so none of them may ride out in it. The page's own file is
// named by no annotation here: a kept attachment annotation is a visible attachment, not a hidden one.
func TestAnExtractedOrRedactedDocumentCarriesNoAssociatedFile(t *testing.T) {
	src := associatedFilesShape("1.7", true, "catalog page")
	if got := strings.Join(filesReachable(t, src), ","); got != "CATALOG-BYTES,ELEM-BYTES,PAGE-BYTES" {
		t.Fatalf("setup: the fixture reaches %q, want all three files", got)
	}
	// refusable names a door that may answer with the associated-file refusal instead (the write is refused, so
	// nothing leaves); every other door must produce a document, or the test would grade nothing.
	doors := []struct {
		name      string
		run       func([]byte) ([]byte, error)
		refusable bool
	}{
		{"Collect", func(b []byte) ([]byte, error) { return Collect(b, []string{"1"}) }, false},
		{"RedactPages", func(b []byte) ([]byte, error) { return RedactPages(b, map[int]RasterPage{1: rasterPage(t, 200, 200)}) }, false},
		{"SplitPage", func(b []byte) ([]byte, error) { o, _, err := SplitPage(b, 1, 2, 1, false); return o, err }, true},
		{"SplitRegions", func(b []byte) ([]byte, error) {
			o, _, err := SplitRegions(b, 1, [][4]float64{{0, 0, 100, 100}})
			return o, err
		}, true},
	}
	for _, door := range doors {
		name := door.name
		out, err := door.run(src)
		if err != nil {
			if door.refusable && errors.Is(err, pdfread.ErrAssociatedFileLost) {
				continue
			}
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := filesReachable(t, out); len(got) != 0 {
			t.Errorf("%s: files still reachable from the trailer: %v", name, got)
		}
	}
}

// TestAFileOnlyAssociatedFilesReachAreRemovedWithTheAttachments — D6 pin 5. A document whose files only `/AF`
// reaches (no /EmbeddedFiles entry, which ADR-136 refuses to rewrite) is put through the removal that was asked
// for: it is not refused, and leaves no filespec and no embedded stream anywhere reachable.
func TestAFileOnlyAssociatedFilesReachAreRemovedWithTheAttachments(t *testing.T) {
	for _, where := range []string{"catalog", "page", "catalog page"} {
		src := associatedFilesShape("1.7", false, where)
		// Stimulus: the ordinary rewrite of this document IS refused when a page's file is /AF-only, so the
		// removal's success is not the absence of a check.
		if strings.Contains(where, "page") {
			if _, err := Rotate(src, []string{"1"}, 90); !errors.Is(err, pdfread.ErrAssociatedFileLost) {
				t.Fatalf("setup (%s): a rotate answered %v, want the associated-file refusal", where, err)
			}
		}
		for name, door := range map[string]func([]byte) ([]byte, error){"RemoveFilesAndMedia": RemoveFilesAndMedia, "StripActive": StripActive} {
			out, err := door(src)
			if err != nil {
				t.Errorf("%s on /AF at %q alone: %v", name, where, err)
				continue
			}
			if got := filesReachable(t, out); len(got) != 0 {
				t.Errorf("%s on /AF at %q alone: files still reachable: %v", name, where, got)
			}
		}
	}
}

// TestTheScanNamesAnAssociatedFileAndTheRemovalClearsIt — D6 pin 2. A document whose only attachment is a file
// `/AF` names is reported as holding one (the scan kind `attachment`, already removed by StripActive and in
// TestEveryScanKindIsStrippedOrNamedAsLeft's list), and after the removal the scan finds none.
func TestTheScanNamesAnAssociatedFileAndTheRemovalClearsIt(t *testing.T) {
	src := associatedFilesShape("1.7", false, "catalog page")
	has := func(pdf []byte) bool {
		rep, err := Scan(pdf)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range rep.Findings {
			if f.Kind == "attachment" {
				return true
			}
		}
		return false
	}
	if !has(src) {
		t.Fatal("the scan reports no attachment for a document whose only files are named by /AF")
	}
	out, err := RemoveFilesAndMedia(src)
	if err != nil {
		t.Fatal(err)
	}
	if has(out) {
		t.Error("the scan still reports an attachment after the removal")
	}
}
