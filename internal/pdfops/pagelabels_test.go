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
