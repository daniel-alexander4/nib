package pdfops

import (
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

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
			pr, err := readPageRuns(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got := joinRunText(pr.runs); !strings.Contains(got, "Hello, divided world") {
				t.Errorf("the divided page read as %q (noText=%v), want its text", got, pr.noText)
			}
		})
	}
}
