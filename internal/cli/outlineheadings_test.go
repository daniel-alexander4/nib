package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
)

// TestOutlineProposeListsWhatTheHeadingsGiveAndSetWritesIt — /pending 910. `--propose` writes nothing and its
// `--json` is the list `--set` reads back, so a proposal can be edited between the two and the written outline
// is what was edited.
func TestOutlineProposeListsWhatTheHeadingsGiveAndSetWritesIt(t *testing.T) {
	dir := t.TempDir()
	pdf, err := pdfops.ConvertDocToPDF([]byte("# Alpha\n\nbody text\n\n## Beta\n\nbody text\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, pdf, 0o644); err != nil {
		t.Fatal(err)
	}

	jsonOut, code := captureStdout(t, func() int { return cmdOutline([]string{in, "--propose", "--json"}) })
	if code != 0 {
		t.Fatalf("outline --propose exit = %d", code)
	}
	var items []pdfops.OutlineItem
	if err := json.Unmarshal([]byte(jsonOut), &items); err != nil || len(items) != 2 {
		t.Fatalf("--propose --json = %q (%v), want two bookmarks", jsonOut, err)
	}
	if has, _ := pdfops.Outline(pdf); len(has) != 0 {
		t.Fatal("setup: the input already carried an outline, so 'proposing writes nothing' proves nothing")
	}
	if after := readPDF(t, in); string(after) != string(pdf) {
		t.Error("--propose changed the file")
	}

	items[1].Title = "Beta, edited"
	edited, _ := json.Marshal(items)
	setFile := filepath.Join(dir, "outline.json")
	if err := os.WriteFile(setFile, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.pdf")
	if code := cmdOutline([]string{in, "--set", setFile, "-o", out}); code != 0 {
		t.Fatalf("outline --set exit = %d", code)
	}
	got, err := pdfops.Outline(readPDF(t, out))
	if err != nil || len(got) != 2 || got[0].Title != "Alpha" || got[1].Title != "Beta, edited" || got[1].Level != items[1].Level {
		t.Errorf("written outline = %+v (%v), want the edited proposal", got, err)
	}

	if code := cmdOutline([]string{in, "--set", filepath.Join(dir, "missing.json"), "-o", filepath.Join(dir, "x.pdf")}); code == 0 {
		t.Error("a missing --set file exited 0")
	}
}
