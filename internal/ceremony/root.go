package ceremony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StoreRoot is the folder every ceremony store on this machine sits under — `~/nib` — and the ONE
// place that says so (/pending 712 R6-5, ADR-009).
//
// The join was written twice, in `internal/server`'s `defaultOutputDir` and in the CLI's `nibDir`,
// and a third time for the DHT cache the CLI inspects; nothing kept them naming the same folder.
// What stays per surface is what each does when there is NO home directory, because the two
// answers are different questions: the server writes, and falls back to a relative `nib` (a
// misplaced document, and every ceremony store refuses a relative root through `storeDir`); the CLI only reads, and says it could not look rather than
// reporting "no record".
func StoreRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "nib"), nil
}

// storeDir is the ONE join from a store root to a ceremony folder, and it refuses a relative root
// (/pending 631, 813, ADR-009).
//
// **The refusal was at `EndedDir` alone**, so `MirrorDir` accepted the relative `"nib"` that
// `defaultOutputDir` falls back to and wrote a ceremony's mirror — for a non-convener the only copy
// of its own signature — under whatever the working directory was, where ADR-012's close-out
// could never move it; and `ReadMirrorFor`, asking `EndedDir` second, silently dropped `ended/`
// and answered from the live folder alone. One root rule for every folder means a machine with
// no home directory refuses ceremonies at the first door, loudly, rather than half-running them.
func storeDir(root string, sub ...string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: %q", ErrRootNotAbsolute, root)
	}
	return filepath.Join(append([]string{root}, sub...)...), nil
}

// InStore reports whether path lies inside the ceremony store's working folders under root: the live
// `ceremonies/` and the `ended/` a close-out moves one into (ADR-012). It answers the question "is this
// the ceremony's own copy of a document", which is not the same as "is this a signed document": a final
// copy the user keeps outside these folders is an ordinary file.
//
// A relative root answers false, as `storeDir` refuses it, and a path is compared both as written and
// with its symlinks resolved, so a link into the store does not walk around the answer.
func InStore(root, path string) bool {
	if path == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	forms := []string{abs}
	// The folder is resolved and the name kept: the file itself need not exist for the answer.
	if r, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		if joined := filepath.Join(r, filepath.Base(abs)); joined != abs {
			forms = append(forms, joined)
		}
	}
	for _, sub := range []string{liveDir, endedDir} {
		dir, err := storeDir(root, sub)
		if err != nil {
			return false
		}
		dirs := []string{dir}
		if r, err := filepath.EvalSymlinks(dir); err == nil && r != dir {
			dirs = append(dirs, r)
		}
		for _, d := range dirs {
			for _, p := range forms {
				if rel, err := filepath.Rel(d, p); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return true
				}
			}
		}
	}
	return false
}
