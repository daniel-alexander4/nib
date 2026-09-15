package pdfops

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The product's readers of `/Lang` and `/Marked`, over every legal encoding — `/pending 489`.
//
// veraPDF's corpus stores a `/Lang` as a hex string and as an indirect object, and a `/Marked` indirectly.
// Three product sites read only the direct forms: OCR overwrote an author's language it could not read,
// a page operation dropped a hex one, and the tag-fate census missed an indirect `/Marked`.

// hexUTF16 is s as a producer writes a hex string: UTF-16BE with a byte-order mark.
func hexUTF16(s string) types.HexLiteral {
	var b strings.Builder
	b.WriteString("FEFF")
	for _, r := range s { // a language tag is ASCII, so every rune is one UTF-16 unit
		fmt.Fprintf(&b, "%04X", r)
	}
	return types.HexLiteral(b.String())
}

// withCatalogLang returns pdf with its catalog /Lang set to lang, encoded as enc: "hex" or "indirect".
func withCatalogLang(t *testing.T, pdf []byte, lang, enc string) []byte {
	t.Helper()
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		cat, err := ctx.XRefTable.Catalog()
		if err != nil {
			return err
		}
		switch enc {
		case "hex":
			cat["Lang"] = hexUTF16(lang)
		case "indirect":
			ir, err := ctx.IndRefForNewObject(types.StringLiteral(lang))
			if err != nil {
				return err
			}
			cat["Lang"] = *ir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// langOf reads pdf's catalog /Lang through the package's own reader.
func langOf(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	return readLang(ctx.XRefTable, cat["Lang"])
}

func TestAnOCRRunKeepsAnAuthorsLanguageInEveryEncoding(t *testing.T) {
	// Control: with nothing declared, the OCR run DOES write its language — otherwise "kept" below is vacuous.
	plain, tagged, err := TagOCRLayer(scannedPage(t), ocrWords(), "eng")
	if err != nil || !tagged {
		t.Fatalf("setup: TagOCRLayer: tagged %v, %v", tagged, err)
	}
	if got := langOf(t, plain); got != "en" {
		t.Fatalf("control: an OCR run on a document declaring no language wrote %q, want \"en\"", got)
	}
	for _, enc := range []string{"hex", "indirect"} {
		src := withCatalogLang(t, scannedPage(t), "de-DE", enc)
		if got := langOf(t, src); got != "de-DE" {
			t.Fatalf("setup (%s): the fixture reads %q, want de-DE", enc, got)
		}
		out, _, err := TagOCRLayer(src, ocrWords(), "eng")
		if err != nil {
			t.Fatal(err)
		}
		if got := langOf(t, out); got != "de-DE" {
			t.Errorf("an author's %s /Lang de-DE became %q after an English OCR run — an OCR run does not overrule an author", enc, got)
		}
	}
}

func TestAPageOperationCarriesAHexLanguage(t *testing.T) {
	for _, n := range []int{1, 3} {
		src := withCatalogLang(t, pagesPDF(t, n), "en-GB", "hex")
		if got := langOf(t, src); got != "en-GB" {
			t.Fatalf("setup: the %d-page fixture reads %q, want en-GB", n, got)
		}
		out, err := Crop(src, [4]float64{0.05, 0.05, 0.05, 0.05}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := langOf(t, out); got != "en-GB" {
			t.Errorf("Crop on %d page(s) turned a hex /Lang en-GB into %q — a page operation dropped the language", n, got)
		}
	}
}

func TestTheTagCensusReadsAnIndirectMarked(t *testing.T) {
	marked := func(value bool) []byte {
		out, err := writeMutated(pagesPDF(t, 1), func(ctx *model.Context) error {
			cat, err := ctx.XRefTable.Catalog()
			if err != nil {
				return err
			}
			ir, err := ctx.IndRefForNewObject(types.Boolean(value))
			if err != nil {
				return err
			}
			cat["MarkInfo"] = types.Dict{"Marked": *ir}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if !inspectTags(marked(true)).marked {
		t.Error("an indirect /Marked true is read as not marked — the census would miss a tagging claim")
	}
	if inspectTags(marked(false)).marked {
		t.Error("control: an indirect /Marked false is read as marked")
	}
}

// TestThePackageReadsLangAndMarkedThroughItsOwnReaders — routing, for the product side.
func TestThePackageReadsLangAndMarkedThroughItsOwnReaders(t *testing.T) {
	bare := regexp.MustCompile(`\["(Lang|Marked)"\]\.\(types\.|(Boolean|String|Name)Entry\("(Lang|Marked)"\)|DereferenceString(Literal|OrHexLiteral)\([^)]*"Lang"`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	scanned, langCalls, boolCalls := 0, 0, 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		src := string(b)
		for i, line := range strings.Split(src, "\n") {
			if bare.MatchString(line) {
				t.Errorf("%s:%d reads /Lang or /Marked with a bare cast — use readLang / readBool: %s", f, i+1, strings.TrimSpace(line))
			}
		}
		langCalls += strings.Count(src, "readLang(")
		boolCalls += strings.Count(src, "readBool(")
	}
	if scanned < 30 {
		t.Fatalf("the scan read %d file(s) — not the package", scanned)
	}
	// declareOCRLanguage and carryLang call readLang; the census calls readBool; each definition is one more.
	if langCalls < 3 {
		t.Errorf("readLang is called %d time(s) — a reader has stopped going through it", langCalls)
	}
	if boolCalls < 2 {
		t.Errorf("readBool is called %d time(s) — the census has stopped going through it", boolCalls)
	}
}
