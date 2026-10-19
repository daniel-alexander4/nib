package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// recordLaunches replaces launchNib for the test and returns the paths it was handed, in order.
func recordLaunches(t *testing.T) *[]string {
	t.Helper()
	var got []string
	prev := launchNib
	launchNib = func(path string) error { got = append(got, path); return nil }
	t.Cleanup(func() { launchNib = prev })
	return &got
}

func writePrintout(t *testing.T, path string, mod time.Time) {
	t.Helper()
	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

// TestWatchOpenHandsANewPDFToNibAndAPrintoutSavedOverItAgain — /pending 403's "Print to Nib".
//
// A print dialog offers the same name each time and asks to replace it, so the second printout is
// the same path with new bytes. `open` must hand that one over too, and must not hand over a file
// that was in the folder before the watch began until it changes.
func TestWatchOpenHandsANewPDFToNibAndAPrintoutSavedOverItAgain(t *testing.T) {
	got := recordLaunches(t)
	dir := t.TempDir()
	t0 := time.Now().Add(-time.Hour)
	before := filepath.Join(dir, "before.pdf")
	writePrintout(t, before, t0)

	seen, processed, failed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
	done := map[string]fileState{}
	markExisting(dir, processed, done)
	if len(done) != 1 {
		t.Fatalf("markExisting recorded %d states, want 1", len(done))
	}

	out := filepath.Join(dir, "output.pdf")
	writePrintout(t, out, t0)
	for i := 0; i < 3; i++ {
		scan(dir, seen, processed, failed, done, watchOps["open"])
	}
	if len(*got) != 1 || (*got)[0] != out {
		t.Fatalf("after one printout: launched %q, want exactly [%s] — never the file already there", *got, out)
	}

	// The second printout, saved over the first.
	writePrintout(t, out, t0.Add(time.Minute))
	for i := 0; i < 3; i++ {
		scan(dir, seen, processed, failed, done, watchOps["open"])
	}
	if len(*got) != 2 || (*got)[1] != out {
		t.Fatalf("after a printout saved over it: launched %q, want it handed over a second time", *got)
	}

	// The file that was there at the start, once it changes.
	writePrintout(t, before, t0.Add(2*time.Minute))
	for i := 0; i < 3; i++ {
		scan(dir, seen, processed, failed, done, watchOps["open"])
	}
	if len(*got) != 3 || (*got)[2] != before {
		t.Fatalf("after the older file changed: launched %q, want it handed over", *got)
	}
}

// TestWatchRewritingOperationsStillActOncePerName — the counter-pin: acting again on change is
// `open`'s alone (/pending 508), and scanOnce, which every rewriting operation runs through, keeps
// once per name.
func TestWatchRewritingOperationsStillActOncePerName(t *testing.T) {
	if watchDone("open") == nil {
		t.Fatal("open does not act again on change")
	}
	for _, op := range []string{"optimize", "sanitize", "timestamp", "ua"} {
		if watchDone(op) != nil {
			t.Fatalf("%s acts again on change", op)
		}
	}
	got := recordLaunches(t)
	dir := t.TempDir()
	t0 := time.Now().Add(-time.Hour)
	out := filepath.Join(dir, "output.pdf")
	writePrintout(t, out, t0)
	seen, processed, failed := map[string]fileState{}, map[string]bool{}, map[string]fileState{}
	for i := 0; i < 3; i++ {
		scanOnce(dir, seen, processed, failed, watchOps["open"])
	}
	writePrintout(t, out, t0.Add(time.Minute))
	for i := 0; i < 3; i++ {
		scanOnce(dir, seen, processed, failed, watchOps["open"])
	}
	if len(*got) != 1 {
		t.Fatalf("once-per-name scan launched %d times, want 1", len(*got))
	}
}

// TestWatchOpenNeverHandsOverASymlink — the watch's regular-files rule holds for open too, so a
// link dropped in the folder does not open a document from elsewhere on disk.
func TestWatchOpenNeverHandsOverASymlink(t *testing.T) {
	got := recordLaunches(t)
	outside, dir := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "private.pdf")
	writePrintout(t, victim, time.Now().Add(-time.Hour))
	if err := os.Symlink(victim, filepath.Join(dir, "bait.pdf")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	seen, processed, failed, done := map[string]fileState{}, map[string]bool{}, map[string]fileState{}, map[string]fileState{}
	for i := 0; i < 3; i++ {
		scan(dir, seen, processed, failed, done, watchOps["open"])
	}
	if len(*got) != 0 {
		t.Fatalf("launched %q through a symlink", *got)
	}
}

// TestWatchOpenLaunchesThisBinaryWithTheAbsolutePath — the door is the double-click's: this
// executable, the path as its only argument, so the launch's own hand-off decides the rest.
func TestWatchOpenLaunchesThisBinaryWithTheAbsolutePath(t *testing.T) {
	cmd, err := nibLaunch("/tmp/x.pdf")
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if cmd.Path != exe || len(cmd.Args) != 2 || cmd.Args[1] != "/tmp/x.pdf" {
		t.Fatalf("launch is %q %q, want %q with the path alone", cmd.Path, cmd.Args, exe)
	}
	if cmd.Stderr != os.Stderr {
		t.Error("the launch's refusals would not reach the watch's terminal")
	}
	if cmd.SysProcAttr == nil {
		t.Error("the launch shares the watch's process group: Ctrl-C on the watch closes the Nib it opened")
	}
	got := recordLaunches(t)
	if _, err := watchOpen("rel.pdf"); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || !filepath.IsAbs((*got)[0]) {
		t.Fatalf("watchOpen handed %q, want an absolute path", *got)
	}
}
