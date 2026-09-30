package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// TestADividedPageReadsItsText — ADR-056 through a real reader. A page whose `/Contents` array divides `(…) Tj`
// from `ET`, or ends its first stream in a comment, is legal; read through pdfcpu's bare join it shows no text,
// because `TjET` is one unknown operator and a comment swallows the line after it. `readPageRuns` is the reader
// reflow and the autotagger both stand on, so it is the one this asserts.
//
// **The comment shape is not a row here**: in this fixture the comment swallows only `ET`, which this reader does
// not need to show the run, so the row could not go red (probed). Its meaning is asserted token by token in
// `pdfread`'s `TestAContentsArrayIsJoinedAtTokenBoundaries`. The white-space row is the control.
func TestADividedPageReadsItsText(t *testing.T) {
	for _, c := range []struct {
		name  string
		shape testpdf.JoinShape
	}{
		{"fused operators", testpdf.JoinRegular},
		{"a white-space division", testpdf.JoinSafe},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf, _, err := testpdf.SplitContents("Hello, divided world", c.shape)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			pr, err := readPageRuns(ctx, pageAt(ctx, nil, 1))
			if err != nil {
				t.Fatal(err)
			}
			if got := joinRunText(pr.runs); !strings.Contains(got, "Hello, divided world") {
				t.Errorf("the divided page read as %q (noText=%v), want its text", got, pr.noText)
			}
		})
	}
}

// TestANUpCarriesTheNoteOfADividedPage — the P05 phase-close review's regression. `api.NUp` writes each source page into
// a form from pdfcpu's OWN join of its `/Contents`, and the note carry identifies a placement by comparing that form with
// the source page's content. Read through ADR-056's door, a fused page's content gains a separator pdfcpu's join lacks,
// the comparison fails, and the note is dropped. The carry compares against what pdfcpu wrote, so it reads pdfcpu's join
// — a named exemption — and this is the fixture it is exempt for.
func TestANUpCarriesTheNoteOfADividedPage(t *testing.T) {
	pdf, _, err := testpdf.SplitContents("a divided page with a note", testpdf.JoinRegular)
	if err != nil {
		t.Fatal(err)
	}
	// The stimulus: the fixture must be one the door separates, or the carry never meets the two joins.
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, _, _, _ := ctx.PageDict(1, false)
	raw, _ := ctx.PageContent(d, 1) //pagecontent:exempt test — the join pdfcpu's NUp writes
	door, _ := pdfread.PageContent(ctx, d, 1)
	if bytes.Equal(raw, door) {
		t.Fatal("the fixture's join needs no separator, so it cannot show the two joins disagreeing")
	}
	noted, err := AddNotes(pdf, []Note{{Page: 1, X: 72, Y: 700, Text: "the divided page's note"}})
	if err != nil {
		t.Fatal(err)
	}
	up, err := NUp(noted, 2, false)
	if err != nil {
		t.Fatalf("n-up: %v", err)
	}
	if got := notesOn(t, up); len(got) != 1 {
		t.Errorf("one note went into the n-up and %d came out — the carry lost the divided page's placement", len(got))
	}
}

// TestANUpCarriesTheTagsOfADividedPage — the tag half of the same regression. `carryTagsThroughNUp` matches each
// n-up form byte for byte against its source page's content, and the form holds pdfcpu's join. This is
// `collidingMCIDFixture` with page 1's `/Contents` divided between `Tj` and `ET` with no white-space — legal, and a
// join the door separates — so read through the door the match fails and page 1's tags are not carried.
func TestANUpCarriesTheTagsOfADividedPage(t *testing.T) {
	must := mustFn(t)
	a := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (ALPHA) Tj"
	b := "ET\nEMC\n"
	two := "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (BRAVO) Tj ET\nEMC\n"
	divided := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R 13 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents [4 0 R 15 0 R] /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(a), a),
		15: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(b), b),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R 10 0 R] /ParentTree 9 0 R /ParentTreeNextKey 2 >>",
		8:  "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [0] >>",
		9:  "<< /Nums [0 [8 0 R] 1 [10 0 R]] >>",
		10: "<< /Type /StructElem /S /P /P 7 0 R /Pg 13 0 R /K [0] >>",
		13: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 14 0 R /StructParents 1 >>",
		14: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(two), two),
	})
	// The stimulus: the undivided twin carries through an n-up, and the divided page is one the door separates.
	whole := inspectTags(must(NUp(collidingMCIDFixture(), 2, false)))
	if !whole.claimsHonestly() || whole.elements == 0 {
		t.Fatalf("the undivided fixture does not carry its tags through an n-up (%v), so this cannot isolate the join", whole)
	}
	ctx, err := pdfread.Validated(divided, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, _, _, _ := ctx.PageDict(1, false)
	raw, _ := ctx.PageContent(d, 1) //pagecontent:exempt test — the join pdfcpu's NUp writes
	door, _ := pdfread.PageContent(ctx, d, 1)
	if bytes.Equal(raw, door) {
		t.Fatal("the divided fixture's join needs no separator, so it cannot show the two joins disagreeing")
	}
	got := inspectTags(must(NUp(divided, 2, false)))
	if !got.claimsHonestly() || got.elements != whole.elements || got.anchored != whole.anchored {
		t.Errorf("the divided document n-ups to %v; its undivided twin to %v — the carry lost the divided page", got, whole)
	}
}

// TestAWrappedPageClosesItsClipAfterATrailingComment — the phase-close review's finding on `wrapPageToBox`: it wrote
// ` Q` straight after the page's content, and content may end inside a `%` comment, which runs to the END OF LINE. The
// comment swallowed the `Q`, so the clip `q` that keeps a neighbouring tile from bleeding in was never closed.
func TestAWrappedPageClosesItsClipAfterATrailingComment(t *testing.T) {
	pdf, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, _, _, _ := ctx.PageDict(1, false)
	if err := setPageContent(ctx, d, []byte("0 0 10 10 re f % a trailing comment")); err != nil {
		t.Fatal(err)
	}
	if err := wrapPageToBox(ctx, d, 1, 0, 0, 100, 100, 1, 100, 100, 0, 0); err != nil {
		t.Fatal(err)
	}
	out, err := pdfread.PageContent(ctx, d, 1)
	if err != nil {
		t.Fatal(err)
	}
	depth, last := 0, ""
	for _, tk := range contentstream.Tokenize(out) {
		if tk.Kind != contentstream.Operator {
			continue
		}
		switch last = string(tk.Bytes(out)); last {
		case "q":
			depth++
		case "Q":
			depth--
		}
	}
	if depth != 0 || last != "Q" {
		t.Errorf("the wrapped page ends with %q at save depth %d — the clip is left open\n%q", last, depth, out)
	}
}
