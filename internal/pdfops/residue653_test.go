package pdfops

import (
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"

	"nib/internal/testpdf"
)

// Small residue from the P05 phase-close review (/pending 653), one test per part.

// TestANegativeNumberIsNotAnOptionIndex — the index bound had no floor, so "-1" was "in range" and the
// whole form was reported as fillable with a value no option answers to.
func TestANegativeNumberIsNotAnOptionIndex(t *testing.T) {
	box := func(v string) []*form.ComboBox {
		return []*form.ComboBox{{Name: "colour", Value: v, Options: []string{"red", "green"}}}
	}
	if err := optionValuesOffered(nil, box("-1"), nil); err == nil {
		t.Error(`"-1" was accepted as an option index: no option has a negative index`)
	}
	// The controls: an index that is one, a value that is one, and an index past the end.
	for v, ok := range map[string]bool{"1": true, "green": true, "2": false} {
		if err := optionValuesOffered(nil, box(v), nil); (err == nil) != ok {
			t.Errorf("value %q: err = %v, want accepted = %v", v, err, ok)
		}
	}
}

// TestAnAttachmentIsNeverListedUnderADotPath — the listing cleaned a name and, when the cleaner refused it,
// showed the raw key instead: the names the cleaner refuses are exactly the ones that must not be shown.
func TestAnAttachmentIsNeverListedUnderADotPath(t *testing.T) {
	for _, key := range []string{"..", ".", "dir/"} {
		doc := testpdf.Assemble(map[int]string{
			1: "<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles << /Names [(" + key + ") 4 0 R] >> >> >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
			4: "<< /Type /Filespec /F (" + key + ") /UF (" + key + ") >>",
		})
		list, err := Attachments(doc)
		if err != nil {
			t.Fatalf("key %q: %v", key, err)
		}
		if len(list) != 1 {
			t.Fatalf("key %q: %d attachment(s) listed, want 1 — the test has no subject", key, len(list))
		}
		a := list[0]
		if a.ID != key {
			t.Errorf("key %q: the entry's ID is %q — it is reached by its key, whatever it is shown as", key, a.ID)
		}
		if a.Name == "" || a.Name == "." || a.Name == ".." || strings.ContainsAny(a.Name, `/\`) {
			t.Errorf("key %q is listed under the name %q, which goes to a Content-Disposition file name", key, a.Name)
		}
	}
}

// TestAHeadingPieceOfALongParagraphHasALevel — the level table was built from whole paragraphs and looked
// up by piece, so a heading that was the first piece of a paragraph too long to be one got level 0.
func TestAHeadingPieceOfALongParagraphHasALevel(t *testing.T) {
	body := "Body text that fills the measure of the page with enough characters to be the body size by far"
	l := groupRuns([]textRun{
		run("What to bring", 72, 760, 120, 16),
		run("1. A pen that writes", 72, 742, 160, 16),
		run("2. The signed copy", 72, 724, 150, 16),
		run("3. Proof of address", 72, 706, 150, 16),
		run(body+body, 72, 670, 450, 12), run(body+body, 72, 656, 450, 12), run(body+body, 72, 642, 450, 12),
	})
	// The shape, stated rather than left to the grouping door: the four large lines are ONE paragraph, too
	// long to be a heading whole, and its first piece — up to the first list label — is one.
	var big textParagraph
	var rest []textParagraph
	for _, par := range l.paragraphs {
		if roundHalf(par.lines[0].size) == 16 {
			big.column = par.column
			big.lines = append(big.lines, par.lines...)
		} else {
			rest = append(rest, par)
		}
	}
	if len(big.lines) != 4 || len(big.lines) <= maxHeadingLines {
		t.Fatalf("setup: the large paragraph has %d line(s), want 4 — more than a heading may have", len(big.lines))
	}
	l.paragraphs = append([]textParagraph{big}, rest...)
	p := proposeFromLayouts([]pageLayout{l})
	if got := strings.Join(roles(p), " "); !strings.HasPrefix(got, "H1 LI LI LI") {
		t.Errorf("roles = %s, want the heading and its three items first", got)
	}
	for _, e := range p.elements {
		if !reviewRoles[e.role] {
			t.Errorf("the proposal holds the role %q, which no review accepts — the commit refuses it whole", e.role)
		}
	}

	// And a piece that is a list item lends its size to no level: it is proposed as an item, so the one
	// heading on the page is still the first level.
	item := proposeRuns(
		run("1. Largest of all", 72, 760, 200, 20),
		run("A heading", 72, 720, 100, 16),
		run(body+body, 72, 690, 450, 12), run(body+body, 72, 676, 450, 12),
	)
	if got := strings.Join(roles(item), " "); !strings.HasPrefix(got, "LI H1") {
		t.Errorf("roles = %s, want LI H1 first — a large list item is not a heading size", got)
	}
}
