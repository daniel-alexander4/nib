package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfops"
	"nib/internal/pdfread"
)

// TestPagesWritesTheSameDocument — `/pending 870`. `nib pages` is the app's delete and reorder: the document it
// writes keeps the title, the `/ID` and the embedded files of the one it read, over the input (`-w`) and beside it
// (`-o`) alike. It wrote an extract — no title, a new `/ID`, no files — so removing a page from a ceremony's
// document in place destroyed its record.
func TestPagesWritesTheSameDocument(t *testing.T) {
	dir := t.TempDir()
	src := readPDF(t, writePDF(t, dir, "plain.pdf", "a", "b", "c"))
	src, err := pdfops.SetTitle(src, "Lease for 12 Elm Street")
	if err != nil {
		t.Fatal(err)
	}
	if src, err = pdfops.AddAttachment(src, "record.json", []byte(`{"kept":true}`)); err != nil {
		t.Fatal(err)
	}
	facts := func(pdf []byte) (title, id string, files int) {
		ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		if len(ctx.XRefTable.ID) > 0 {
			id = ctx.XRefTable.ID[0].String()
		}
		aa, err := pdfops.Attachments(pdf)
		if err != nil {
			t.Fatal(err)
		}
		return ctx.XRefTable.Title, id, len(aa)
	}
	wantTitle, wantID, _ := facts(src)
	if wantTitle == "" || wantID == "" {
		t.Fatalf("the fixture has title %q and /ID %q; it needs both", wantTitle, wantID)
	}
	for _, c := range []struct {
		name  string
		args  []string
		pages int
	}{
		{"--remove -o", []string{"--remove", "2"}, 2},
		{"--keep -o", []string{"--keep", "3,1"}, 2},
		{"--remove -w", []string{"-w", "--remove", "2"}, 2},
		{"--keep -w", []string{"-w", "--keep", "3,1,2"}, 3},
	} {
		in := filepath.Join(dir, "in.pdf")
		if err := os.WriteFile(in, src, 0o600); err != nil {
			t.Fatal(err)
		}
		out, args := in, append([]string{}, c.args...)
		if c.args[0] != "-w" {
			out = filepath.Join(dir, "out.pdf")
			args = append(args, "-o", out)
		}
		if code := cmdPages(append(args, in)); code != 0 {
			t.Errorf("%s: exit %d, want 0", c.name, code)
			continue
		}
		got := readPDF(t, out)
		if n, _ := pdfops.PageCount(got); n != c.pages {
			t.Errorf("%s: %d pages, want %d", c.name, n, c.pages)
		}
		if title, id, files := facts(got); title != wantTitle || id != wantID || files != 1 {
			t.Errorf("%s: wrote title %q, /ID[0] %s and %d file(s); the document it read has %q, %s and 1",
				c.name, title, id, files, wantTitle, wantID)
		}
	}
}

// TestPagesRefusesToWriteADocumentShortOfAFile: a file that could not be kept is a refusal on the command line,
// which has no undo.
func TestPagesRefusesToWriteADocumentShortOfAFile(t *testing.T) {
	if out, err := sameDocument([]byte("%PDF"), 1, nil); err == nil || out != nil {
		t.Errorf("one file dropped returned %d byte(s) and err=%v; want nothing written and a refusal", len(out), err)
	}
	if out, err := sameDocument([]byte("%PDF"), 0, nil); err != nil || out == nil {
		t.Errorf("no file dropped returned err=%v; want the document", err)
	}
}
