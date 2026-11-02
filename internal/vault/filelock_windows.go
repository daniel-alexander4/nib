//go:build windows

package vault

import (
	"os"

	"nib/internal/filelock"
)

// tryLock and unlock are `internal/filelock`'s — the one primitive (ADR-009) — under the names this package uses.
func tryLock(f *os.File) (got bool, err error) { return filelock.Try(f) }

func unlock(f *os.File) { filelock.Unlock(f) }
