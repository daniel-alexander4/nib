package pdfops

import (
	"reflect"
	"testing"
)

// `/pending 910` — bookmarks proposed from the headings the tagger reads. The assertions are the whole
// contract: the levels follow the heading numbers and deepen one at a time, a repeated heading is told apart
// (SetOutline refuses a repeat), nothing is written, and SetOutline accepts exactly what is proposed.

const headingsFixtureMarkdown = "# Alpha\n\nbody text\n\n### Deep\n\nbody text\n\n## Beta\n\nbody text\n\n## Beta\n\nbody text\n\n# Alpha\n\nbody text\n"

func TestOutlineFromHeadingsFollowsTheHeadingsAndSetOutlineTakesIt(t *testing.T) {
	src, err := untaggedMarkdown([]byte(headingsFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	before := string(src)
	items, err := OutlineFromHeadings(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != before {
		t.Fatal("proposing bookmarks changed the document's bytes")
	}
	var titles []string
	var levels []int
	for _, it := range items {
		titles = append(titles, it.Title)
		levels = append(levels, it.Level)
	}
	// H1 H3 H2 H2 H1 → 0, then the H3 brought down to 1 (it follows a level-0 item), then 1 1 0.
	if want := []int{0, 1, 1, 1, 0}; !reflect.DeepEqual(levels, want) {
		t.Errorf("levels = %v, want %v", levels, want)
	}
	if want := []string{"Alpha", "Deep", "Beta", "Beta (p. 1)", "Alpha (p. 1)"}; !reflect.DeepEqual(titles, want) {
		t.Errorf("titles = %q, want %q", titles, want)
	}
	// What is proposed is what the writer takes, and reads back as proposed.
	out, err := SetOutline(src, items)
	if err != nil {
		t.Fatalf("SetOutline refused the proposal: %v", err)
	}
	back, err := Outline(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != len(items) {
		t.Errorf("the written outline has %d bookmarks, the proposal %d", len(back), len(items))
	}
}

func TestOutlineFromHeadingsOfADocumentWithNoneIsEmptyNotAnError(t *testing.T) {
	src, err := untaggedMarkdown([]byte("just a paragraph of words\n"))
	if err != nil {
		t.Fatal(err)
	}
	items, err := OutlineFromHeadings(src)
	if err != nil || items == nil || len(items) != 0 {
		t.Errorf("got %v, %v; want an empty, non-nil list", items, err)
	}
}

// The proposer numbers heading SIZES, and nib's own Markdown has three, so the six roles are checked as the
// table they are: H<n> is bookmark level n-1, and no other role is a heading.
func TestEveryHeadingRoleIsABookmarkLevelAndNothingElseIs(t *testing.T) {
	for n := 1; n <= 6; n++ {
		if got, ok := headingRoleLevel["H"+string(rune('0'+n))]; !ok || got != n-1 {
			t.Errorf("H%d = %d, %v; want level %d", n, got, ok, n-1)
		}
	}
	if len(headingRoleLevel) != 6 {
		t.Errorf("%d roles are headings, want six", len(headingRoleLevel))
	}
}
