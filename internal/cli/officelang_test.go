package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// officeCatalogLang reads the catalog /Lang, "" where there is none.
func officeCatalogLang(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	s, _ := root["Lang"].(types.StringLiteral)
	return string(s)
}

// TestOfficeDeclaresTheLanguageItIsTold — `/pending 471`, the CLI door. Markdown, because it needs
// no LibreOffice and because nib is otherwise never told its language, so the control is clean.
func TestOfficeDeclaresTheLanguageItIsTold(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "notizen.md")
	mustWrite(t, in, []byte("# Notizen\n\nGuten Tag.\n"))

	plain := filepath.Join(dir, "plain.pdf")
	if code := cmdOffice([]string{in, "-o", plain}); code != 0 {
		t.Fatalf("office exit = %d, want 0", code)
	}
	// Control: without --lang nib declares nothing, or "de-AT" below could be a default.
	if got := officeCatalogLang(t, readPDF(t, plain)); got != "" {
		t.Fatalf("control: a Markdown conversion with no --lang declares %q — nib was told nothing", got)
	}

	told := filepath.Join(dir, "told.pdf")
	if code := cmdOffice([]string{in, "-o", told, "--lang", "DE-at"}); code != 0 {
		t.Fatalf("office --lang exit = %d, want 0", code)
	}
	if got := officeCatalogLang(t, readPDF(t, told)); got != "de-AT" {
		t.Errorf("office --lang DE-at declares /Lang %q, want the canonical de-AT", got)
	}

	// `/pending 486`: a named language on nib's own Markdown conversion earns the PDF/UA identification, and
	// the same conversion without one does not.
	if ok, err := testpdf.ClaimsUA(readPDF(t, plain)); err != nil || ok {
		t.Errorf("a Markdown conversion with no --lang claims PDF/UA (err %v) — nobody named its language", err)
	}
	if ok, err := testpdf.ClaimsUA(readPDF(t, told)); err != nil || !ok {
		t.Errorf("a Markdown conversion with --lang does not claim PDF/UA (err %v)", err)
	}
}

// TestOfficeRefusesALanguageItCannotDeclare — and writes nothing: `pdfops.SetLang`'s door refuses it
// before the output is written.
func TestOfficeRefusesALanguageItCannotDeclare(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "notes.md")
	mustWrite(t, in, []byte("# Notes\n"))
	out := filepath.Join(dir, "out.pdf")
	if code := cmdOffice([]string{in, "-o", out, "--lang", "german"}); code == 0 {
		t.Fatal("office --lang german exited 0 — a language name is not a tag nib can declare")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("a refused --lang still wrote %s (stat err %v)", out, err)
	}
}
