package ceremony

import (
	"os"
	"path/filepath"
)

// StoreRoot is the folder every ceremony store on this machine sits under — `~/nib` — and the ONE
// place that says so (/pending 712 R6-5, ADR-009).
//
// The join was written twice, in `internal/server`'s `defaultOutputDir` and in the CLI's `nibDir`,
// and a third time for the DHT cache the CLI inspects; nothing kept them naming the same folder.
// What stays per surface is what each does when there is NO home directory, because the two
// answers are different questions: the server writes, and falls back to a relative `nib` (a
// misplaced document, and `CloseOutMirror` refuses a relative root for the one destructive
// operation, `ErrRootNotAbsolute`); the CLI only reads, and says it could not look rather than
// reporting "no record".
func StoreRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "nib"), nil
}
