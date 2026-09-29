package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// TestSanitizeNeverExitsZeroOverARemainingOpenAction (`/pending 729`).
//
// `nib sanitize` has no residual re-scan of its own: it wrote whatever StripActive returned and exited 0.
// A catalog OpenAction whose action names an object the file lacks survived the strip (pdfcpu kept the key
// when its graph delete failed), so the "sanitized" file still ran JavaScript on open. The property is the
// CLI's, whatever the mechanism: exit 0 means the file written scans clean; anything else writes nothing.
func TestSanitizeNeverExitsZeroOverARemainingOpenAction(t *testing.T) {
	base, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(base), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	root["OpenAction"] = types.Dict{
		"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(1)"),
		"Foo": types.IndirectRef{ObjectNumber: 99},
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	pdf := buf.Bytes()

	// STIMULUS: the OpenAction is live in the input, and pdfcpu's own delete — the old path — keeps it.
	rep, err := pdfops.Scan(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !hasKind(rep, "openAction") {
		t.Fatalf("setup: the input scans no openAction: %+v", rep.Findings)
	}
	rc, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	rroot, err := rc.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if err := rc.XRefTable.DeleteDictEntry(rroot, "OpenAction"); err == nil {
		t.Fatal("setup: pdfcpu's DeleteDictEntry succeeded — the fixture no longer reaches the defect")
	}
	if _, ok := rroot.Find("OpenAction"); !ok {
		t.Fatal("setup: pdfcpu removed the key despite failing — the old path was not broken here")
	}

	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.pdf")
	code := cmdSanitize([]string{in, "-o", out})
	written, rerr := os.ReadFile(out)
	if code != 0 {
		if rerr == nil {
			t.Errorf("sanitize exited %d and still wrote %s", code, out)
		}
		return
	}
	if rerr != nil {
		t.Fatalf("sanitize exited 0 and wrote nothing: %v", rerr)
	}
	rep2, err := pdfops.Scan(written)
	if err != nil {
		t.Fatal(err)
	}
	if hasKind(rep2, "openAction") {
		t.Error("nib sanitize exited 0 over a file whose OpenAction still runs on open")
	}
}

func hasKind(rep pdfops.ScanReport, kind string) bool {
	for _, f := range rep.Findings {
		if f.Kind == kind {
			return true
		}
	}
	return false
}
