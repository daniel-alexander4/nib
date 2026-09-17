package pdfops

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// Discovery for the two optional external converters, in ONE door (ADR-009).
//
// # Why PATH alone is not "is it installed"
//
// `exec.LookPath` answers "is this name on PATH", and nothing else: it reads PATH (plus
// PATHEXT on Windows) and never the registry, never `/Applications`. But the STOCK install
// of both tools is off PATH on two of the three platforms nib ships to —
// `/Applications/LibreOffice.app/Contents/MacOS/soffice` on macOS, and
// `%ProgramFiles%\LibreOffice\program\soffice.exe` on Windows. So a PATH-only probe cannot
// tell "not installed" from "installed where we did not look", and it reports the same
// `false` for both.
//
// That matters more now than it did, because the UI has stopped being silent about the
// answer. A narrowed file picker is a shrug; a line that says nib could not find LibreOffice
// is a claim, and a claim has to be true for the user who already installed it.
//
// **`internal/browser` already made this exact decision, in the opposite direction**, and
// this is its shape: `findChromium` tries `LookPath` and then absolute per-OS candidates,
// because a bundle install is invisible to PATH. What is NOT copied is its `fileExists`,
// which stats only — see `runnable`.
//
// # The fallback is monotonic
//
// PATH always wins; candidates are a second pass. So this can only ever find MORE than the
// old probe, never less, and a wrong candidate stats false and costs nothing. That property
// is what makes widening safe to ship without a flag.

// runnable reports whether path is a regular file this process can plausibly execute.
//
// **Deliberately not `internal/browser.fileExists`, whose comment claims "a regular,
// runnable file" while its body is `os.Stat` + `!IsDir()`.** A stat-only probe answers
// "something is at this path", and promoting a present-but-unrunnable install is the one
// failure that is WORSE than today's: an unrunnable soffice that starts and hangs burns the
// full two-minute `officeConvertTimeout` and returns "LibreOffice timed out converting this
// document" — no path, no reason — where a missing one returns a sentence naming the remedy.
//
// The mode bits narrow that window; they do not close it. A quarantined `.app`, an ACL'd
// WindowsApps install, or a `noexec` mount all pass this and fail at exec. That residue is
// the inventory's G3 and is stated rather than papered over.
func runnable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	// Windows carries executability in the extension, not in a mode bit, and the candidate
	// lists name the `.exe` explicitly.
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// lookTool returns the first of names found on PATH, or failing that the first candidate
// pattern matching a runnable file. "" when nothing is found.
//
// Candidates are globs so a versioned install directory can be expressed: Ghostscript on
// Windows lives in `gs\gs10.03.1\bin`, which no fixed string can name. A literal path is a
// glob matching itself, so both kinds go through one loop.
//
// Selection among several matches is LEXICAL, not a version comparison. Any working install
// is acceptable here and the common case is exactly one; ranking them would be a version
// parser nothing needs.
func lookTool(names []string, candidates []string) string {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, pat := range candidates {
		matches, err := filepath.Glob(pat)
		if err != nil {
			// The only error Glob returns is ErrBadPattern, reachable only from a malformed
			// literal in our own lists — so the guard is a test over the lists, not a log line.
			continue
		}
		for _, m := range matches {
			if runnable(m) {
				return m
			}
		}
	}
	return ""
}

// toolCache caches a FOUND path for the process lifetime and re-probes an empty answer on
// every call.
//
// **The asymmetry is the whole design.** A `sync.Once` cached both answers, so a user who
// installed the tool while nib was running stayed refused until the process ended — and
// "restart nib" is not a simple instruction here: a relaunch HANDS OFF to the running
// instance (`cmd/nib/main.go:handedOff`), and idle-exit needs every window closed plus a
// 10s grace and never fires at all in a `noBrowser` run. Re-probing the empty answer is
// what makes the UI's "Check again" honest.
//
// It is affordable because the answer is read once per page load, not per request:
// `/api/status` is on no timer (the ~1s poller is `/api/session/status`, a different route).
// Measured at the grill: a miss costs ~107µs (2 names) / ~159µs (3 names) on a 19-entry
// PATH. A hit costs nothing after the first.
//
// It also makes the absent branch testable without a reset seam — which the repo forbids
// adding to shipped code (`build/redproof.sh`: "a switch whose whole purpose is to break the
// program ... would ship in the binary users run"). `lookTool` is a pure function and the
// tests drive it directly.
type toolCache struct {
	mu   sync.Mutex
	path string
}

func (c *toolCache) find(names []string, candidates []string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path != "" {
		return c.path
	}
	c.path = lookTool(names, candidates)
	return c.path
}

// programFileRoots reads the Windows install roots from the ENVIRONMENT rather than
// hardcoding `C:`, so a system installed on another drive still resolves. Duplicates are
// dropped because `ProgramW6432` and `ProgramFiles` are the same path on a 64-bit process.
func programFileRoots() []string {
	var roots []string
	seen := map[string]bool{}
	for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		r := os.Getenv(v)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		roots = append(roots, r)
	}
	return roots
}

// libreOfficeCandidates are the stock off-PATH install locations, per OS. Linux and the
// BSDs return none on purpose: every packaged install symlinks onto PATH, so the first pass
// already answers and a candidate list would be dead weight that rots.
func libreOfficeCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Applications/LibreOffice.app/Contents/MacOS/soffice"}
	case "windows":
		var out []string
		for _, root := range programFileRoots() {
			out = append(out, filepath.Join(root, "LibreOffice", "program", "soffice.exe"))
		}
		return out
	}
	return nil
}

// ghostscriptCandidates are Windows-only for the same reason: elsewhere gs is packaged onto
// PATH. The `gs*` segment is why candidates are globs — the install directory carries the
// version.
func ghostscriptCandidates() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	var out []string
	for _, root := range programFileRoots() {
		out = append(out,
			filepath.Join(root, "gs", "gs*", "bin", "gswin64c.exe"),
			filepath.Join(root, "gs", "gs*", "bin", "gswin32c.exe"),
		)
	}
	return out
}
