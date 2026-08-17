package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestSetLang(t *testing.T) {
	out, err := SetLang(threePagePDF(t), "th")
	if err != nil {
		t.Fatalf("SetLang: %v", err)
	}
	// The /Lang must survive a validating re-read (every later op reads this way).
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(out), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := root["Lang"].(types.StringLiteral); string(s) != "th" {
		t.Fatalf("catalog /Lang = %q, want \"th\"", root["Lang"])
	}
}

func TestSetLangRejectsEmpty(t *testing.T) {
	if _, err := SetLang(threePagePDF(t), "   "); err == nil {
		t.Error("blank language tag should error")
	}
}

func TestOCRLangToBCP47(t *testing.T) {
	cases := map[string]string{
		"eng": "en", "tha": "th", "hin": "hi", "ell": "el", "ukr": "uk",
		"": "", "zzz": "",
	}
	for code, want := range cases {
		if got := OCRLangToBCP47(code); got != want {
			t.Errorf("OCRLangToBCP47(%q) = %q, want %q", code, got, want)
		}
	}
}
