package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// The CLI's `/pending 504` findings: the output door, `nib sign`'s dropped error, the watch's
// never-forgotten names, and the help that omitted a command.

// TestAnOutputNamingItsInputNeverTruncatesTheOriginal.
//
// `writeOutput` was `os.WriteFile`, which truncates the target before it writes, so `-o` naming the
// input destroyed the only copy the moment the output began — and a failure part-way left neither.
//
// **The observable is a hard link taken before the command.** A write THROUGH the path rewrites the
// inode both names share, so the link sees the new bytes; a replace (temp file, rename) gives the path a
// new inode and leaves the link holding the original. Crash timing is not reproducible in a test; which
// of the two happened is, and it is the whole difference.
func TestAnOutputNamingItsInputNeverTruncatesTheOriginal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SKIP (not a pass): the observable is a POSIX hard link")
	}
	dir := t.TempDir()
	in := writePDF(t, dir, "only-copy.pdf")
	original := readPDF(t, in)
	keep := filepath.Join(dir, "keep")
	if err := os.Link(in, keep); err != nil {
		t.Skipf("SKIP (not a pass): hard links unavailable here: %v", err)
	}
	if code := cmdRotate([]string{in, "-o", in, "--deg", "90"}); code != 0 {
		t.Fatalf("rotate -o naming its own input exit = %d, want 0", code)
	}
	// STIMULUS: the output differs from the input, or a write-through and a replace look identical.
	if bytes.Equal(readPDF(t, in), original) {
		t.Fatal("setup: the rotation wrote the same bytes, so nothing below can tell a truncation from a replace")
	}
	if got := readPDF(t, keep); !bytes.Equal(got, original) {
		t.Error("the input's own inode was rewritten in place: -o naming the input truncated the only copy " +
			"before the new bytes were safely on disk")
	}
}

// TestAnOutputKeepsTheModeOfTheFileItReplaces — `os.WriteFile` left an existing file's mode alone, so the
// door replacing it must carry the mode across, and give a new file the 0644 it always had.
func TestAnOutputKeepsTheModeOfTheFileItReplaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SKIP (not a pass): POSIX permission bits")
	}
	dir := t.TempDir()
	in := writePDF(t, dir, "in.pdf")
	private := filepath.Join(dir, "private.pdf")
	mustWrite(t, private, []byte("an earlier output"))
	if err := os.Chmod(private, 0o600); err != nil {
		t.Fatal(err)
	}
	if code := cmdOptimize([]string{in, "-o", private}); code != 0 {
		t.Fatalf("optimize exit = %d", code)
	}
	if got := mode(t, private); got != 0o600 {
		t.Errorf("replacing a 0600 output left it %o — a private file became readable", got)
	}
	fresh := filepath.Join(dir, "fresh.pdf")
	if code := cmdOptimize([]string{in, "-o", fresh}); code != 0 {
		t.Fatalf("optimize exit = %d", code)
	}
	if got := mode(t, fresh); got != 0o644 {
		t.Errorf("a new output has mode %o, want 0644", got)
	}
}

// TestAnOutputThatIsADeviceIsWrittenThrough — the door's one named exemption. `-o /dev/null` worked through
// `os.WriteFile`; a rename over a device is not what anyone asked for.
func TestAnOutputThatIsADeviceIsWrittenThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SKIP (not a pass): no /dev/null")
	}
	if fi, err := os.Stat(os.DevNull); err != nil || fi.Mode().IsRegular() {
		t.Skipf("SKIP (not a pass): %s is not a device here (%v)", os.DevNull, err)
	}
	in := writePDF(t, t.TempDir(), "in.pdf")
	if code := cmdOptimize([]string{in, "-o", os.DevNull}); code != 0 {
		t.Errorf("optimize -o %s exit = %d, want 0", os.DevNull, code)
	}
}

// TestSignSaysSoWhenTheClaimCouldNotBeChecked — `nib sign` dropped `DropUAIdentificationUnlessSigned`'s
// error. ADR-032 decides a failed check does not cost the signature ("passed through too, logged"), so the
// signature goes ahead; what it may not do is go ahead in silence, because the claim is then sealed.
func TestSignSaysSoWhenTheClaimCouldNotBeChecked(t *testing.T) {
	dir := t.TempDir()
	base, err := testpdf.Text("to be signed")
	if err != nil {
		t.Fatal(err)
	}
	// A catalog whose /Type pdfcpu's validation refuses and the signer does not read.
	broken := regexp.MustCompile(`/Type\s*/Catalog`).ReplaceAll(base, []byte("/Type /Katalog"))
	// STIMULUS: the check really fails on these bytes, or the warning has nothing to report.
	if bytes.Equal(broken, base) {
		t.Fatal("setup: the fixture's catalog was not found to break")
	}
	if _, derr := pdfops.DropUAIdentificationUnlessSigned(broken, false); derr == nil {
		t.Fatal("setup: the PDF/UA check succeeds on the broken catalog, so no warning is owed")
	}
	in := filepath.Join(dir, "in.pdf")
	mustWrite(t, in, broken)
	p12 := filepath.Join(dir, "id.p12")
	if err := os.WriteFile(p12, makeP12(t, "CLI Tester", "secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NIB_P12_PASSWORD", "secret")
	out := filepath.Join(dir, "signed.pdf")
	var code int
	stderr := captureStderr(t, func() { code = cmdSign([]string{in, "-o", out, "--cert", p12}) })
	if code != 0 {
		t.Fatalf("sign exit = %d (%s) — ADR-032: a failed check never costs the user the signature", code, stderr)
	}
	if !strings.Contains(stderr, "PDF/UA identification could not be checked") {
		t.Errorf("the check failed and the document was signed with nothing said (stderr %q)", stderr)
	}
}

// TestWatchActsOnANewFileThatReusesAGoneFilesName — `processed`, `seen` and `failed` were never pruned, so
// a name that left the directory kept its state for the whole run. One arm per map, each red on its own.
func TestWatchActsOnANewFileThatReusesAGoneFilesName(t *testing.T) {
	t.Run("processed: a replaced file is acted on", func(t *testing.T) {
		dir := t.TempDir()
		p := writePDF(t, dir, "invoice.pdf")
		seen, processed, failed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
		calls := 0
		act := func(string) (string, error) { calls++; return "done", nil }
		scanOnce(dir, seen, processed, failed, act)
		scanOnce(dir, seen, processed, failed, act)
		if calls != 1 {
			t.Fatalf("setup: the first invoice was acted on %d time(s), want 1", calls)
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		scanOnce(dir, seen, processed, failed, act)
		writePDF(t, dir, "invoice.pdf", "a different invoice")
		scanOnce(dir, seen, processed, failed, act)
		scanOnce(dir, seen, processed, failed, act)
		if calls != 2 {
			t.Errorf("a new file reusing a gone file's name was acted on %d time(s) in total, want 2", calls)
		}
	})

	t.Run("failed: a replacement identical to a failure is retried", func(t *testing.T) {
		dir := t.TempDir()
		p := writePDF(t, dir, "scan.pdf")
		data := readPDF(t, p)
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		seen, processed, failed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
		calls, fail := 0, true
		act := func(string) (string, error) {
			calls++
			if fail {
				return "", errors.New("boom")
			}
			return "done", nil
		}
		scanOnce(dir, seen, processed, failed, act)
		scanOnce(dir, seen, processed, failed, act)
		if calls != 1 || len(failed) != 1 {
			t.Fatalf("setup: calls %d, failed %d — want one failed action", calls, len(failed))
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		scanOnce(dir, seen, processed, failed, act)
		// The same bytes and the same mtime: exactly the fingerprint that failed, under a file that is new.
		mustWrite(t, p, data)
		if err := os.Chtimes(p, fi.ModTime(), fi.ModTime()); err != nil {
			t.Fatal(err)
		}
		fail = false
		scanOnce(dir, seen, processed, failed, act)
		scanOnce(dir, seen, processed, failed, act)
		if calls != 2 {
			t.Errorf("a new file whose fingerprint matched a gone file's failure was acted on %d time(s) in total, want 2", calls)
		}
	})

	t.Run("seen: a replacement still settles before it is acted on", func(t *testing.T) {
		dir := t.TempDir()
		p := writePDF(t, dir, "drop.pdf")
		data := readPDF(t, p)
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		seen, processed, failed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
		calls := 0
		act := func(string) (string, error) { calls++; return "done", nil }
		scanOnce(dir, seen, processed, failed, act)
		scanOnce(dir, seen, processed, failed, act)
		if calls != 1 {
			t.Fatalf("setup: acted %d time(s), want 1", calls)
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		scanOnce(dir, seen, processed, failed, act)
		mustWrite(t, p, data)
		if err := os.Chtimes(p, fi.ModTime(), fi.ModTime()); err != nil {
			t.Fatal(err)
		}
		scanOnce(dir, seen, processed, failed, act)
		if calls != 1 {
			t.Errorf("a new file was acted on at first sight, before it settled — the gone file's size and mtime " +
				"stood in for the settle")
		}
	})
}

// TestEveryCommandIsListedInTheHelp — `booklet` was dispatched and missing from `nib help`. The guard is
// the dispatch table itself, so a command added without a help line is caught whatever its name.
func TestEveryCommandIsListedInTheHelp(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "usage")
	if err != nil {
		t.Fatal(err)
	}
	printUsage(f)
	f.Close()
	raw, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	help := string(raw)
	// STIMULUS: the help really was captured, and the table really is the command set.
	if !strings.Contains(help, "nib — local PDF tool") || len(commands) < 20 {
		t.Fatalf("setup: captured %d byte(s) of help against %d command(s)", len(help), len(commands))
	}
	for verb := range commands {
		if !strings.Contains(help, "\n  nib "+verb+" ") {
			t.Errorf("nib %s is a command, and nib help does not list it", verb)
		}
	}
}
