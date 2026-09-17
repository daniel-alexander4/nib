package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestASplitNeverWritesOverTheDocumentItIsSplitting — /pending 569.
//
// # The defect, measured through the real command before this existed
//
// `nib split collide/foo1-2.pdf --out-dir collide --ranges 1-2 --prefix foo` replaced the
// 105,102-byte input with the 94,254-byte part: exit 0, md5 `235c2019…` → `fe51b69e…`, and the
// only thing printed was the part's own path — which is the input's path, the tell nobody reads.
// (`/pending 550` measured the same shape at 339,657 → 94,539 bytes.)
//
// `writeSplitFiles` checked CONTAINMENT only — that the part lands inside the chosen directory —
// and never that the name it was about to write is the file it had just read. A split part is a
// strict SUBSET of its input, so the write is always a loss, and it is the one case where the
// part is the user's only copy of the whole.
//
// # The observable is the input's bytes, not the exit code
//
// A refusal that wrote the part first and reported afterwards would still have destroyed the
// document, so the assertion is that the original bytes are still there. The exit code is checked
// too, because writing nothing and reporting success is a different defect with the same file
// contents.
func TestASplitNeverWritesOverTheDocumentItIsSplitting(t *testing.T) {
	dir := t.TempDir()
	// The name a `--prefix foo --ranges 1-2` split produces: SanitizeFilename("foo" + "1-2").
	in := writePDF(t, dir, "foo1-2.pdf", "a", "b", "c")
	original := readPDF(t, in)

	code := cmdSplit([]string{in, "--out-dir", dir, "--ranges", "1-2", "--prefix", "foo"})

	if got := readPDF(t, in); !bytes.Equal(got, original) {
		t.Errorf("the split overwrote the document it was splitting: %d bytes in, %d bytes after. "+
			"A part is a subset of its input, so this is the whole document replaced by two of its "+
			"pages, with exit %d", len(original), len(got), code)
	}
	if code == 0 {
		t.Errorf("a split whose part names its own input exited 0; it must refuse before the first " +
			"write, because nothing downstream can tell the user their document is gone")
	}
}

// TestASplitRefusesBeforeItWritesAnything — the refusal is a REFUSAL, not a partial run.
//
// A door that wrote parts 1..k and then discovered the collision at k+1 would leave the folder
// half-filled under a non-zero exit, and the user would have to work out which parts landed. The
// check therefore runs over every part before the first write, and this asserts the folder is
// untouched — the input, and nothing else.
func TestASplitRefusesBeforeItWritesAnything(t *testing.T) {
	dir := t.TempDir()
	// Three spans; the COLLIDING one is last, so a per-part check would already have written two.
	in := writePDF(t, dir, "p3.pdf", "a", "b", "c")
	before := readPDF(t, in)

	if code := cmdSplit([]string{in, "--out-dir", dir, "--ranges", "1,2,3", "--prefix", "p"}); code == 0 {
		t.Fatal("a split whose LAST part names its own input exited 0")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wrote []string
	for _, e := range ents {
		if e.Name() != "p3.pdf" {
			wrote = append(wrote, e.Name())
		}
	}
	if len(wrote) > 0 {
		t.Errorf("the refusal left %v behind: the collision was found part-way, so the earlier "+
			"parts had already been written. Check every output path before the first write", wrote)
	}
	if got := readPDF(t, in); !bytes.Equal(got, before) {
		t.Error("the input was overwritten by the last part")
	}
}

// TestASplitSeesThroughASymlinkToItsOwnInput.
//
// The CLI's write door resolves a symlink before writing (`atomicfile.ReplaceDurable`, /pending
// 515), so a link in the output folder named as a part sends the write to whatever it points at —
// and if that is the input, the input dies through the link. Comparing path STRINGS cannot see
// this: `<dir>/foo1-2.pdf` and `<elsewhere>/doc.pdf` are different spellings of one file. The door
// compares file IDENTITY (`os.SameFile`, device + inode), which is why this case is covered by the
// same check rather than by a second one.
func TestASplitSeesThroughASymlinkToItsOwnInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SKIP (not a pass): the observable is a POSIX symlink")
	}
	root := t.TempDir()
	src := filepath.Join(root, "real")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	in := writePDF(t, src, "doc.pdf", "a", "b", "c")
	original := readPDF(t, in)
	// A link in the output folder under the exact name the split will produce.
	if err := os.Symlink(in, filepath.Join(out, "foo1-2.pdf")); err != nil {
		t.Skipf("SKIP (not a pass): symlinks unavailable here: %v", err)
	}

	code := cmdSplit([]string{in, "--out-dir", out, "--ranges", "1-2", "--prefix", "foo"})

	if got := readPDF(t, in); !bytes.Equal(got, original) {
		t.Errorf("a link in the output folder pointing at the input sent the part through it: "+
			"%d bytes became %d, exit %d. The check compares path strings, which cannot see two "+
			"names for one file", len(original), len(got), code)
	}
	if code == 0 {
		t.Error("the split exited 0 having written a part through a link to its own input")
	}
}

// TestASplitPartIsReadableByTheAccountTheUserGaveItTo — /pending 570, the CLI half.
//
// A new part is 0644 here and was 0600 from the GUI, for the same operation over the same
// re-derivable output, because each door inherited whatever mode its helper happened to pass. The
// server's twin of this test asserts the other end; both now name `pdfops.SplitPartMode`, so the
// pair fails if either door drifts. The input is deliberately 0600, which pins the refused third
// option — carrying the source document's mode onto its parts.
func TestASplitPartIsReadableByTheAccountTheUserGaveItTo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SKIP (not a pass): POSIX permission bits")
	}
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf", "a", "b", "c")
	if err := os.Chmod(in, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "parts")
	if code := cmdSplit([]string{in, "--out-dir", out, "--ranges", "1-2,3", "--prefix", "part"}); code != 0 {
		t.Fatalf("split exit = %d, want 0", code)
	}
	for _, n := range []string{"part1-2.pdf", "part3.pdf"} {
		info, err := os.Stat(filepath.Join(out, n))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("a part written by the CLI is %#o, want 0644 — the GUI writes the same part "+
				"0644, so a split's output must not depend on which surface produced it (%s)", got, n)
		}
	}
}

// TestAMailMergeNeverWritesOverTheFormItIsFilling — the same door, the other caller.
//
// `nib fill IN --data rows.csv --out-dir DIR` runs through `writeSplitFiles` too (`FillFormCSV`
// returns `[]pdfops.SplitPart`), so a CSV whose name column produces the form's own filename
// replaces the form with one filled copy of it. Found while fixing the split; it is the same
// defect reached through a different verb, and it is the reason the rule lives in a door rather
// than in `cmdSplit`.
func TestAMailMergeNeverWritesOverTheFormItIsFilling(t *testing.T) {
	dir := t.TempDir()
	form := fillTestForm(t, dir)
	// The merge names each output after the name column, so a row spelling the form's own base
	// name aims one output straight at it.
	base := filepath.Base(form)
	name := base[:len(base)-len(filepath.Ext(base))]
	csv := filepath.Join(dir, "rows.csv")
	mustWrite(t, csv, []byte("outname,fullName\n"+name+",Ada\n"))
	original := readPDF(t, form)

	code := cmdFill([]string{form, "--data", csv, "--out-dir", dir, "--name-col", "outname"})

	if got := readPDF(t, form); !bytes.Equal(got, original) {
		t.Errorf("the mail-merge overwrote the form it was filling: %d bytes became %d, exit %d",
			len(original), len(got), code)
	}
	if code == 0 {
		t.Error("a mail-merge whose output names the form exited 0")
	}
}
