package pdfops

import (
	"errors"
	"testing"

	"nib/internal/testpdf"
)

// TestEveryPageDoorRefusesAPageTheDocumentDoesNotHave — `/pending 823`. Measured before the fix on a one-page
// document: rotate page 5, delete page 5 and crop page 5 each returned no error (the delete leaving the one page
// it had), and a stamp keyed page 0 landed on page 1. Each must now refuse through the one door, as redaction
// does (`/pending 819`), and return no bytes.
func TestEveryPageDoorRefusesAPageTheDocumentDoesNotHave(t *testing.T) {
	one, err := testpdf.Text("alpha")
	if err != nil {
		t.Fatal(err)
	}
	png := pngBytes(t, 40, 20)
	rect := [4]float64{10, 10, 60, 30}
	cases := []struct {
		name string
		run  func() ([]byte, error)
	}{
		{"rotate page 5", func() ([]byte, error) { return Rotate(one, []string{"5"}, 90) }},
		{"rotate 1 and 5", func() ([]byte, error) { return Rotate(one, []string{"1", "5"}, 90) }},
		{"rotate page 0", func() ([]byte, error) { return Rotate(one, []string{"0"}, 90) }},
		{"rotate even of one", func() ([]byte, error) { return Rotate(one, []string{"even"}, 90) }},
		{"rotate a selection netting nothing", func() ([]byte, error) { return Rotate(one, []string{"1", "!1"}, 90) }},
		{"delete page 5", func() ([]byte, error) { return RemovePages(one, []string{"5"}) }},
		{"crop page 5", func() ([]byte, error) { return Crop(one, [4]float64{0.1, 0.1, 0.5, 0.5}, []string{"5"}) }},
		{"collect 1 and 5", func() ([]byte, error) { return Collect(one, []string{"1", "5"}) }},
		{"duplicate page 5", func() ([]byte, error) { return DuplicatePage(one, 5) }},
		{"stamp an image on page 0", func() ([]byte, error) {
			return StampImages(one, []Stamp{{Page: 0, Rect: rect, PNG: png}})
		}},
		{"stamp an image on page 5", func() ([]byte, error) {
			return StampImages(one, []Stamp{{Page: 5, Rect: rect, PNG: png}})
		}},
		{"stamp a field on page 0", func() ([]byte, error) {
			out, _, err := StampFields(one, []Field{{Page: 0, Rect: rect, Text: "x"}})
			return out, err
		}},
		{"stamp a field on page 5", func() ([]byte, error) {
			out, _, err := StampFields(one, []Field{{Page: 5, Rect: rect, Text: "x"}})
			return out, err
		}},
	}
	for _, c := range cases {
		out, err := c.run()
		if !errors.Is(err, ErrPageNotInDocument) {
			t.Errorf("%s: err = %v, want ErrPageNotInDocument", c.name, err)
			continue
		}
		var pe *PageRangeError
		if !errors.As(err, &pe) || pe.Pages != 1 {
			t.Errorf("%s: refusal = %#v, want a PageRangeError against 1 page", c.name, pe)
		}
		if out != nil {
			t.Errorf("%s: a refusal returned %d bytes", c.name, len(out))
		}
	}
}

// TestASelectionMayStillClipAtTheEnd — the refusal is of a TERM that names no page, so the forms a user writes to
// mean "to the end" keep working, as does a page that exists.
func TestASelectionMayStillClipAtTheEnd(t *testing.T) {
	three, err := testpdf.Text("a", "b", "c")
	if err != nil {
		t.Fatal(err)
	}
	for _, sel := range [][]string{{"2-"}, {"2-9"}, {"3"}, {"1-3", "!2"}, {"odd"}} {
		if _, err := Rotate(three, sel, 90); err != nil {
			t.Errorf("rotate %v of three pages: %v", sel, err)
		}
	}
	if _, err := RemovePages(three, []string{"3-9"}); err != nil {
		t.Errorf("delete 3-9 of three pages: %v", err)
	}
	if out, err := StampImages(three, []Stamp{{Page: 3, Rect: [4]float64{10, 10, 60, 30}, PNG: pngBytes(t, 40, 20)}}); err != nil || out == nil {
		t.Errorf("stamp on the last page: (%d bytes, %v)", len(out), err)
	}
}
