// Package atomicfile writes a file so that an interrupted write cannot leave a corrupt or
// stale one behind.
//
// # Why this is its own package
//
// The rule had one implementation in `internal/vault` and a same-named, WEAKER one in
// `internal/server` — the vault's own comment records what that cost: `handleVaultImport`
// called the rename-only version to replace `vault.nib`, so the swap was atomic and the data
// blocks were not durable. A power loss inside the writeback window leaves the vault present
// and garbage while the original — the only copy of the signing identity — is already gone.
// And, in that comment's words, *"two same-named functions with different durability
// contracts is also how nobody noticed"*.
//
// P07.S02a added a third consumer (the ceremony mirror), so the choice was to import the
// vault's for a file that is not a vault, or to give the rule one door. ADR-009 settles it:
// a rule holding at more than one call site is written ONCE and every site calls it.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write writes data to path via a temp file and a rename: ATOMIC, and deliberately not durable.
//
// **The pair exists so a caller has to choose, and the choice is the point.** `WriteDurable`'s own
// contract draws the line — *"callers that hold the only copy of something get this; callers that
// can re-derive their output do not need it"* — and a single function cannot express that. A door
// that only offered durability would put an fsync per file on a split export, which writes one file
// per part from a document that is still open; a door that only offered this would leave a user's
// saved original recoverable only as far as the page cache.
//
// So: re-derivable output takes this. Anything that is the only copy takes `WriteDurable`, and
// says so at the call site.
//
// perm is applied to the temp file BEFORE the rename, for the same reason as below: the file is
// never briefly readable at the process umask's default under its final name.
//
// **The temp pattern is `.nib-*.tmp` and the extension is load-bearing** (/pending 316). It came
// from `internal/cli`'s hand-rolled twin when that twin was folded in here, and it is kept for
// both halves of the name: `.nib-` gives a stranded temp its provenance — a user finding
// `.nib-2891.tmp` in their Documents folder can tell who left it — and the `.tmp` SUFFIX is what
// keeps `filepath.Ext` off `.pdf`, which is how the CLI's watcher tells an input from an
// in-flight write. A dotfile prefix alone was enough for every scanner in this repo (checked:
// `watch.go`'s `.pdf` filter, `saveas.go`'s dot-skip, `ListStored`'s id filter), but nothing
// outside this repo was ever asked.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".nib-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// WriteDurable writes data to path via a temp file, fsync, rename and a parent-directory
// fsync.
//
// **The syncs make the rename durable, not merely atomic**, and the distinction is the whole
// point: without them a crash right after the rename can still leave a stale or truncated
// file on disk. Callers that hold the only copy of something get this; callers that can
// re-derive their output do not need it.
//
// perm is applied to the temp file BEFORE the rename, so the file is never briefly readable
// at the process umask's default under its final name.
func WriteDurable(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".nib-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// Persist the directory entry so the rename itself survives a crash. Best-effort: the
	// rename has already succeeded, and a caller that cannot open its own directory has a
	// bigger problem than this sync.
	d, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer d.Close()
	_ = d.Sync()
	return nil
}
