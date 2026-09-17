package pdfops

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The discovery door's tests. `lookTool` is a pure function precisely so these can drive the
// ABSENT branch, which nothing could reach before: the old probe was a package-level
// `sync.Once` with no reset, so the first call in a test binary fixed the answer for every
// later one, and every existing test simply skips when the tool is missing. The repo forbids
// adding a reset seam to shipped code for a test's benefit (`build/redproof.sh`: "a switch
// whose whole purpose is to break the program ... would ship in the binary users run"), so the
// seam is the function signature rather than a mutator.

func TestLookToolPrefersPATHAndFallsBackToTheInstallLocations(t *testing.T) {
	dir := t.TempDir()

	// A PATH that deliberately misses: the whole point of the fallback.
	t.Setenv("PATH", filepath.Join(dir, "nothing-here"))

	planted := filepath.Join(dir, "soffice")
	if err := os.WriteFile(planted, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := lookTool([]string{"nib-no-such-tool"}, nil); got != "" {
		t.Errorf("a tool that is on no PATH and has no candidates resolved to %q", got)
	}

	got := lookTool([]string{"nib-no-such-tool"}, []string{planted})
	if got != planted {
		t.Errorf("lookTool did not fall back to the candidate: got %q, want %q\n\t"+
			"This is the whole defect: a stock macOS LibreOffice lives in an .app bundle and a "+
			"default Windows install under %%ProgramFiles%%, neither of which is on PATH, so a "+
			"PATH-only probe reports 'not installed' for a machine that has it.", got, planted)
	}
}

func TestLookToolRefusesACandidateItCannotRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SKIP (not a pass): Windows carries executability in the extension, not a mode bit, " +
			"so there is no not-executable file to plant")
	}
	dir := t.TempDir()
	t.Setenv("PATH", filepath.Join(dir, "nothing-here"))

	notExec := filepath.Join(dir, "soffice")
	if err := os.WriteFile(notExec, []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lookTool([]string{"nib-no-such-tool"}, []string{notExec}); got != "" {
		t.Errorf("a present-but-unrunnable candidate was accepted as %q.\n\t"+
			"This is the one failure WORSE than reporting absent: a widened picker sends the user "+
			"at a converter that cannot start, and an unrunnable soffice that hangs burns the full "+
			"%v timeout and returns a reasonless 'timed out', where a missing one names the remedy.",
			got, officeConvertTimeout)
	}

	aDir := filepath.Join(dir, "adir")
	if err := os.Mkdir(aDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := lookTool([]string{"nib-no-such-tool"}, []string{aDir}); got != "" {
		t.Errorf("a DIRECTORY matched as a runnable tool: %q", got)
	}
}

// The versioned-directory case is why candidates are globs and not literals: Ghostscript on
// Windows installs into `gs\gs10.03.1\bin`, a path no fixed string can name.
func TestLookToolMatchesAVersionedInstallDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", filepath.Join(dir, "nothing-here"))

	binDir := filepath.Join(dir, "gs", "gs10.03.1", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planted := filepath.Join(binDir, "gswin64c")
	if err := os.WriteFile(planted, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	pattern := filepath.Join(dir, "gs", "gs*", "bin", "gswin64c")
	if got := lookTool([]string{"nib-no-such-tool"}, []string{pattern}); got != planted {
		t.Errorf("the glob did not match the versioned install: got %q, want %q", got, planted)
	}
}

// The candidate lists are data, and a malformed pattern in them would fail SILENTLY —
// `filepath.Glob`'s only error is ErrBadPattern and `lookTool` skips it, which is correct at
// runtime and useless as a guard. So the guard is here, over the lists themselves.
func TestTheCandidateListsAreWellFormedGlobs(t *testing.T) {
	lists := map[string][]string{
		"libreOfficeCandidates": libreOfficeCandidates(),
		"ghostscriptCandidates": ghostscriptCandidates(),
	}
	seen := 0
	for name, pats := range lists {
		for _, p := range pats {
			seen++
			if _, err := filepath.Glob(p); err != nil {
				t.Errorf("%s has a malformed pattern %q: %v — lookTool skips it silently, so this "+
					"candidate would never match and nothing at runtime would say so", name, p, err)
			}
			if !filepath.IsAbs(p) {
				t.Errorf("%s has a relative candidate %q — a relative path resolves against the "+
					"process's working directory, which is wherever the user launched Nib from",
					name, p)
			}
		}
	}
	// The stimulus floor. On Linux both lists are empty BY DESIGN (packages symlink onto PATH),
	// so an empty result is correct there and must not be read as the lists having rotted away.
	if runtime.GOOS == "linux" && seen != 0 {
		t.Errorf("linux declared %d candidates; it is meant to declare none, because every "+
			"packaged install is already on PATH", seen)
	}
	if runtime.GOOS != "linux" && seen == 0 {
		t.Errorf("%s declared no candidates at all — the fallback this file exists for is dead "+
			"on the platform that needs it", runtime.GOOS)
	}
}

func TestRunnableIsNotAStatCheck(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && runnable(f) {
		t.Error("runnable() accepted a mode-0644 file.\n\t" +
			"internal/browser.fileExists is os.Stat + !IsDir() while its comment claims 'a regular, " +
			"runnable file' — copying that shape here is what this test exists to prevent.")
	}
	if runnable(filepath.Join(dir, "absent")) {
		t.Error("runnable() accepted a path that does not exist")
	}
}

// The cache's asymmetry IS the feature: a hit is kept for the process, an empty answer is
// re-probed on every call. Without that, a user who installs the converter while Nib is
// running stays refused until the process ends — and "restart Nib" is not simple here, because
// a relaunch hands off to the running instance.
func TestAnEmptyAnswerIsReprobedAndAHitIsKept(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", filepath.Join(dir, "nothing-here"))
	planted := filepath.Join(dir, "tool")

	var c toolCache
	if got := c.find([]string{"nib-no-such-tool"}, []string{planted}); got != "" {
		t.Fatalf("setup: the tool resolved to %q before it was planted", got)
	}

	// The user installs it. No restart, no reset call.
	if err := os.WriteFile(planted, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := c.find([]string{"nib-no-such-tool"}, []string{planted}); got != planted {
		t.Errorf("the cache did not re-probe an empty answer: got %q, want %q — the UI's "+
			"'Check again' is built on this, and without it that button is a lie", got, planted)
	}

	// And now it sticks, so the common case costs one probe rather than one per request.
	if err := os.Remove(planted); err != nil {
		t.Fatal(err)
	}
	if got := c.find([]string{"nib-no-such-tool"}, []string{planted}); got != planted {
		t.Errorf("a found path was re-probed and lost: got %q, want the cached %q", got, planted)
	}
}
