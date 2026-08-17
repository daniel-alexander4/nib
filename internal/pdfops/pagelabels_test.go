package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestSetPageLabels(t *testing.T) {
	pdf := threePagePDF(t)
	out, err := SetPageLabels(pdf, []PageLabelRange{
		{Start: 1, Style: "roman-lower", First: 1},
		{Start: 2, Style: "decimal", First: 1, Prefix: "A-"},
	})
	if err != nil {
		t.Fatalf("SetPageLabels: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("output is not a PDF")
	}

	// The number tree must survive a validating re-read — every later document
	// op reads through ReadValidateAndOptimize, so an invalid tree would break them.
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	d, err := ctx.DereferenceDict(root["PageLabels"])
	if err != nil || d == nil {
		t.Fatalf("PageLabels missing: %v", err)
	}
	nums, err := ctx.DereferenceArray(d["Nums"])
	if err != nil {
		t.Fatalf("Nums: %v", err)
	}
	// Two ranges → [key0 labeldict key1 labeldict]; keys are 0-based page indices.
	if len(nums) != 4 {
		t.Fatalf("Nums length = %d, want 4", len(nums))
	}
	if k, _ := nums[0].(types.Integer); int(k) != 0 {
		t.Fatalf("first key = %v, want 0", nums[0])
	}
	if k, _ := nums[2].(types.Integer); int(k) != 1 {
		t.Fatalf("second key = %v, want 1 (page 2, 0-based)", nums[2])
	}
}

func TestSetPageLabelsRejects(t *testing.T) {
	pdf := threePagePDF(t) // 3 pages
	cases := map[string][]PageLabelRange{
		"empty":          nil,
		"bad style":      {{Start: 1, Style: "octal"}},
		"start too high": {{Start: 9, Style: "decimal"}},
		"start zero":     {{Start: 0, Style: "decimal"}},
		"duplicate page": {{Start: 1, Style: "decimal"}, {Start: 1, Style: "roman-lower"}},
	}
	for name, ranges := range cases {
		if _, err := SetPageLabels(pdf, ranges); err == nil {
			t.Errorf("%s: expected an error, got nil", name)
		}
	}
}

// TestSetPageLabelsEscapesPrefix pins the escaping of the one field a user types
// freely. A /P entry is a PDF string literal written verbatim by pdfcpu, so a
// prefix carrying "(" or ")" escapes its own literal.
//
// Measured before the fix, on exactly these inputs:
//   - "Exhibit 1) x"  -> the label came back truncated to "Exhibit 1", with the
//     remainder strewn through the object stream as loose tokens.
//   - "Exhibit (1 x"  -> the whole PageLabels number tree became unreadable
//     ("missing Kids or Nums entry"), so every later op on that document failed,
//     because they all go through ReadValidateAndOptimize.
//
// The test asserts the ROUND TRIP, not just that a PDF came out. Asserting only
// that SetPageLabels returned no error, or that the bytes start with %PDF, passes
// against both failures above — which is why the original test (prefix "A-", no
// assertion on the value read back) was green over this for the life of the feature.
func TestSetPageLabelsEscapesPrefix(t *testing.T) {
	for _, prefix := range []string{
		"Exhibit 1) x",  // closes the literal early
		"Exhibit (1 x",  // opens one that is never closed
		`Ex\hibit `,     // the escape character itself
		"Ex(1)h ",       // balanced: legal already, must not regress
		"Plain-A ",      // the ordinary case
	} {
		t.Run(prefix, func(t *testing.T) {
			out, err := SetPageLabels(threePagePDF(t), []PageLabelRange{
				{Start: 1, Style: "decimal", First: 1, Prefix: prefix},
			})
			if err != nil {
				t.Fatalf("SetPageLabels(%q): %v", prefix, err)
			}
			ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("prefix %q made the document unreadable: %v", prefix, err)
			}
			root, err := ctx.XRefTable.Catalog()
			if err != nil {
				t.Fatal(err)
			}
			d, err := ctx.DereferenceDict(root["PageLabels"])
			if err != nil || d == nil {
				t.Fatalf("PageLabels missing after re-read: %v", err)
			}
			nums, err := ctx.DereferenceArray(d["Nums"])
			if err != nil || len(nums) < 2 {
				t.Fatalf("Nums broken: %v (%d entries)", err, len(nums))
			}
			lbl, err := ctx.DereferenceDict(nums[1])
			if err != nil {
				t.Fatal(err)
			}
			sl, ok := lbl["P"].(types.StringLiteral)
			if !ok {
				t.Fatalf("/P is %T, want StringLiteral", lbl["P"])
			}
			got, err := types.StringLiteralToString(sl)
			if err != nil {
				t.Fatalf("/P will not decode: %v", err)
			}
			if got != prefix {
				t.Errorf("prefix did not round-trip: wrote %q, read back %q", prefix, got)
			}
		})
	}
}
