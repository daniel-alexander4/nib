package pdfops

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// The review doors — `PLAN-accessibility.md` P08.S06b.

func reviewAll(pr TagProposal) []TagReview {
	out := make([]TagReview, len(pr.Elements))
	for i, e := range pr.Elements {
		out[i] = TagReview{ID: e.ID, Role: e.Role, Text: e.Text}
	}
	return out
}

// TestProposeTagsDescribesEachElementWhereItSits.
func TestProposeTagsDescribesEachElementWhereItSits(t *testing.T) {
	src, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	before := string(src)
	pr, err := ProposeTags(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != before {
		t.Fatal("proposing changed the document's bytes")
	}
	var roles []string
	for i, e := range pr.Elements {
		roles = append(roles, e.Role)
		if e.ID != i {
			t.Errorf("element %d carries id %d", i, e.ID)
		}
		box := e.PageBox
		if box[2]-box[0] <= 0 || box[3]-box[1] <= 0 {
			t.Errorf("element %d names page box %v", i, box)
		}
		r := e.Rect
		if !(r[0] < r[2] && r[1] < r[3] && r[0] >= box[0] && r[2] <= box[2] && r[1] >= box[1] && r[3] <= box[3]) {
			t.Errorf("element %d (%q) has rect %v, not a box inside its page %v", i, e.Text, r, box)
		}
	}
	if got := strings.Join(roles, " "); got != "H1 P H2 LI LI P" {
		t.Errorf("proposed roles %s", got)
	}
}

// TestAReviewIsAppliedInItsOwnOrderWithItsOwnRoles — the three things a reviewer does: reorder,
// retype, ignore — each visible in the committed tree.
func TestAReviewIsAppliedInItsOwnOrderWithItsOwnRoles(t *testing.T) {
	src, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	pr, err := ProposeTags(src)
	if err != nil {
		t.Fatal(err)
	}
	rv := reviewAll(pr) // H1 P H2 LI LI P
	rv[1].Role = "H3"   // retype the opening paragraph
	rv[4].Ignore = true // ignore the second list item
	// move the closing paragraph up to follow the retyped one
	order := []TagReview{rv[0], rv[1], rv[5], rv[2], rv[3], rv[4]}
	out, err := CommitTags(src, order)
	if err != nil {
		t.Fatal(err)
	}
	got := readTruth(t, out)
	want := []truthBlock{
		{heading: true, text: squeeze(pr.Elements[0].Text)},
		{heading: true, text: squeeze(pr.Elements[1].Text)},
		{heading: false, text: squeeze(pr.Elements[5].Text)},
		{heading: true, text: squeeze(pr.Elements[2].Text)},
		{heading: false, text: squeeze(pr.Elements[3].Text)},
	}
	if len(got) != len(want) {
		t.Fatalf("the committed tree reads %d block(s), the review kept %d:\n  got  %+v\n  want %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("block %d reads %+v, reviewed %+v", i, got[i], want[i])
		}
	}
}

// TestAReviewThatDoesNotDescribeTheProposalIsRefused — stale is 409's, malformed is 400's; the server
// tells them apart by these two errors.
func TestAReviewThatDoesNotDescribeTheProposalIsRefused(t *testing.T) {
	src, err := untaggedMarkdown([]byte(commitFixtureMarkdown))
	if err != nil {
		t.Fatal(err)
	}
	pr, err := ProposeTags(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		edit func([]TagReview) []TagReview
		want error
	}{
		{"one element missing", func(r []TagReview) []TagReview { return r[:len(r)-1] }, ErrTagsStale},
		{"a text that changed", func(r []TagReview) []TagReview { r[2].Text += "!"; return r }, ErrTagsStale},
		{"an element listed twice", func(r []TagReview) []TagReview { r[1] = r[0]; return r }, ErrTagsReview},
		{"an unknown id", func(r []TagReview) []TagReview { r[0].ID = 99; return r }, ErrTagsReview},
		{"a role nobody can choose", func(r []TagReview) []TagReview { r[0].Role = "Table"; return r }, ErrTagsReview},
		{"everything ignored", func(r []TagReview) []TagReview {
			for i := range r {
				r[i].Ignore = true
			}
			return r
		}, ErrTagsReview},
	} {
		if _, cerr := CommitTags(src, c.edit(reviewAll(pr))); !errors.Is(cerr, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, cerr, c.want)
		}
	}
}

// TestTheReviewDoorsRouteThroughTheProposer — S06's routing clause: both doors reach the page through
// the proposer, and the proposer through the layout door, so the review surface's pages are read
// exactly as the corpus measured them.
func TestTheReviewDoorsRouteThroughTheProposer(t *testing.T) {
	calls := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range []string{"tagreview.go", "proposer.go"} {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			set := map[string]bool{}
			ast.Inspect(fn, func(n ast.Node) bool {
				if call, isCall := n.(*ast.CallExpr); isCall {
					if id, isIdent := call.Fun.(*ast.Ident); isIdent {
						set[id.Name] = true
					}
				}
				return true
			})
			calls[fn.Name.Name] = set
		}
	}
	for fn, must := range map[string][]string{
		"ProposeTags":      {"proposeStructure"},
		"CommitTags":       {"proposeStructure", "commitProposal"},
		"proposeStructure": {"readPageLayout"},
	} {
		if calls[fn] == nil {
			t.Errorf("%s is gone, so its route is unchecked", fn)
			continue
		}
		for _, m := range must {
			if !calls[fn][m] {
				t.Errorf("%s does not call %s", fn, m)
			}
		}
	}
}

// TestIgnoringEveryElementOnAPageStillCommitsIt — tier 3's keyboard review ignores page two's only
// paragraph and commits; after /pending 495's claim check that page's text was left unmarked, the claim
// was refused and the review could not commit. The ignored page's text is an artifact instead.
func TestIgnoringEveryElementOnAPageStillCommitsIt(t *testing.T) {
	src, err := testpdf.Text("section 1 opening paragraph", "section 2 closing paragraph")
	if err != nil {
		t.Fatal(err)
	}
	pr, err := ProposeTags(src)
	if err != nil {
		t.Fatal(err)
	}
	rv := reviewAll(pr)
	last := len(rv) - 1
	// Stimulus first: the ignored element is the only one on its page, so nothing kept reaches it.
	if len(rv) != 2 || pr.Elements[last].Page == pr.Elements[0].Page {
		t.Fatalf("setup: want one element on each of two pages, got %+v", pr.Elements)
	}
	rv[last].Ignore = true
	out, err := CommitTags(src, rv)
	if err != nil {
		t.Fatalf("a review that ignored every element on a page could not commit: %v", err)
	}
	if n, rerr := UnmarkedTextRuns(out); rerr != nil || n != 0 {
		t.Errorf("%d text run(s) left neither tagged nor an artifact (err %v)", n, rerr)
	}
}
