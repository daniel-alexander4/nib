package pdfops

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

func svgFixture(t *testing.T) []byte {
	t.Helper()
	for _, f := range testpdf.ExportSet() {
		if f.Name == "covered" {
			return f.PDF
		}
	}
	t.Fatal("no export fixture")
	return nil
}

func skipNoPdftocairo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftocairo"); err != nil {
		t.Skip("pdftocairo (poppler) not installed")
	}
}

// withTool puts a fake pdftocairo (a shell script) first on PATH and forgets the cached find.
func withFakeTool(t *testing.T, script string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pdftocairo"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	forgetPdftocairo()
	t.Cleanup(forgetPdftocairo)
}

func forgetPdftocairo() {
	pdftocairoCache.mu.Lock()
	pdftocairoCache.path = ""
	pdftocairoCache.mu.Unlock()
}

func TestExportPageSVGDrawsTheRequestedPage(t *testing.T) {
	skipNoPdftocairo(t)
	forgetPdftocairo()
	pdf := svgFixture(t)
	out, err := ExportPageSVG(context.Background(), pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "<?xml") || !strings.Contains(s, "<svg") {
		t.Fatalf("not an SVG: %.80q", s)
	}
	for _, bad := range []string{"<script", "javascript:", "http://", "https://"} {
		// the namespaces are the only absolute URLs a drawing of a page may hold
		if strings.Contains(strings.ReplaceAll(strings.ReplaceAll(s, "http://www.w3.org/2000/svg", ""), "http://www.w3.org/1999/xlink", ""), bad) {
			t.Errorf("the drawing holds %q", bad)
		}
	}
}

func TestExportPageSVGRefusesAPageTheDocumentLacks(t *testing.T) {
	skipNoPdftocairo(t)
	forgetPdftocairo()
	pdf := svgFixture(t)
	n, _ := PageCount(pdf)
	for _, p := range []int{0, -1, n + 1} {
		if _, err := ExportPageSVG(context.Background(), pdf, p); !errors.Is(err, ErrSVGPage) {
			t.Errorf("page %d: %v, want ErrSVGPage", p, err)
		}
	}
	if _, err := ExportPageSVG(context.Background(), []byte("not a pdf"), 1); !errors.Is(err, ErrSVGPage) {
		t.Errorf("a non-PDF: %v, want ErrSVGPage", err)
	}
}

func TestExportPageSVGWithoutTheToolIsAMissingToolRefusal(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	forgetPdftocairo()
	t.Cleanup(forgetPdftocairo)
	_, err := ExportPageSVG(context.Background(), svgFixture(t), 1)
	if tool, ok := MissingToolFor(err); !ok || tool != ToolPdftocairo {
		t.Fatalf("err %v classified %v %v, want ToolPdftocairo", err, tool, ok)
	}
	if PdftocairoAvailable() {
		t.Error("available with an empty PATH")
	}
}

func TestExportPageSVGRefusesWhatIsNotADrawing(t *testing.T) {
	// A tool that exits 0 and writes something that is not an SVG (the output is its last argument).
	withFakeTool(t, `for a; do o="$a"; done; echo "hello" > "$o"`)
	if _, err := ExportPageSVG(context.Background(), svgFixture(t), 1); !errors.Is(err, ErrSVGUnreadable) {
		t.Fatalf("%v, want ErrSVGUnreadable", err)
	}
}

func TestExportPageSVGTakesAFailingToolAsAnError(t *testing.T) {
	withFakeTool(t, `echo boom >&2; exit 1`)
	_, err := ExportPageSVG(context.Background(), svgFixture(t), 1)
	if err == nil || errors.Is(err, ErrSVGUnreadable) || !strings.Contains(err.Error(), "pdftocairo") {
		t.Fatalf("%v, want a runConvert error naming pdftocairo", err)
	}
}

func TestPdftocairoArgsAskForExactlyOnePage(t *testing.T) {
	got := strings.Join(pdftocairoArgs("a.pdf", "b.svg", 3), " ")
	if got != "-svg -f 3 -l 3 a.pdf b.svg" {
		t.Fatalf("args %q", got)
	}
}
