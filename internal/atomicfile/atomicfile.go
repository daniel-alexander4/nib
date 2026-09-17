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
	"errors"
	"io"
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
// **That split-export half is measured, not argued** (`/pending 508`; ext4 on NVMe, `nib split
// --every 1` over a 280-page document, parts averaging 1.3 KB, three rounds): 17–21µs per file
// through `os.WriteFile`, 41–53µs through this, and **7.5–8.9 ms** through `WriteDurable`. That is
// 2.1–2.5 s of writing on top of the 3.3 s the page extraction itself takes — the fsync costs about
// 70% as much as the work — and more again on anything slower than an SSD.
//
// **And the two split exports currently disagree**, which is the reason to write the figure down
// rather than the figure alone. The GUI's (`internal/server/export.go`) takes this door and says
// why — *"every part comes from the document still open in this process"*. The CLI's
// (`writeSplitFiles` → `writeNamed`) takes `WriteDurable`, because every CLI write is routed
// through the door built for `-w`, whose contract is about replacing the user's only copy — which
// a split part is not: nothing is replaced and the input is untouched. So the same output is worth
// 34µs a file at one door and 7.9 ms at the other. Not resolved here: `internal/cli`'s
// `atomicdurable_test.go` refuses any call to this function in that package and offers no
// exemption, so moving the CLI side is a guard change and a decision, not a tidy-up.
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

// WriteFrom streams src to path via a temp file and a rename: ATOMIC, and deliberately not durable,
// the same choice `Write` makes and for the same reason — a caller that can re-derive its output
// does not need an fsync. Nib's use is a release artifact, which is re-downloadable by definition.
//
// **The fourth door exists because the other three take `[]byte`, and some writes cannot.** The
// release download is ~95 MB (measured); buffering it whole to reach an atomic door would spend the
// memory the streaming exists to avoid, and hand-rolling temp-plus-rename at the call site is the
// second implementation `atomicdoor_test.go` and `atomicroute_test.go` both exist to refuse. Those
// two guards caught exactly that draft, which is why this door is here rather than a declared
// exemption: `atomicroute_test.go` has no exemption mechanism at all — it fails on any `os.Rename`
// in `internal/server` — so the only honest answer was to move the rename into the door.
//
// onProgress, when non-nil, is called with the running total after each chunk. It is the caller's
// throttle point, not this door's: it fires per read, and a caller that turns every call into an
// event will flood whatever it is feeding.
//
// limit bounds what will be written; a source exceeding it stops with ErrTooLarge and leaves
// nothing behind. A remote `Content-Length` is a claim, and a stream that trusts it fills the disk.
func WriteFrom(path string, src io.Reader, perm os.FileMode, limit int64, onProgress func(int64)) (int64, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".nib-*.tmp")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds; the cleanup on every failure path
	buf := make([]byte, 256<<10)
	var total int64
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if limit > 0 && total+int64(n) > limit {
				tmp.Close()
				return total, ErrTooLarge
			}
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				tmp.Close()
				return total, werr
			}
			total += int64(n)
			if onProgress != nil {
				onProgress(total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			tmp.Close()
			return total, rerr
		}
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return total, err
	}
	if err := tmp.Close(); err != nil {
		return total, err
	}
	return total, os.Rename(tmpName, path)
}

// ErrTooLarge is returned by WriteFrom when the source exceeds the caller's limit.
var ErrTooLarge = errors.New("atomicfile: source exceeds the write limit")

// CreateDurable creates path holding data, REFUSING if anything already exists there, and syncs
// the file and its parent directory before returning.
//
// **The third door, because neither of the other two can say "never replace".** Both rename over
// the destination, which is correct for a file being updated and wrong for one whose previous
// occupant is irreplaceable: a newly generated private key lands where the user's existing SSH
// identity may already be. O_EXCL makes the refusal the kernel's, and the syncs are the reason this
// is not a bare OpenFile — sshkey.Generate wrote the only copy of a new key without one while the
// vault sealed to that key was written durably (/pending 502), so a power loss could keep the lock
// and lose the key.
//
// A failed write removes what it created — O_EXCL guarantees the file is this call's own. The
// error for an existing path satisfies os.IsExist / errors.Is(err, fs.ErrExist).
func CreateDurable(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		f.Close()
		os.Remove(path)
		return err
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	// Best-effort, as in WriteDurable: the file is complete and synced.
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return nil
}

// ReplaceDurable is WriteDurable for a file the USER owns: an existing regular file keeps its
// permission bits, and perm applies only when path does not exist yet.
//
// **Why a fourth door rather than a change to WriteDurable (/pending 499).** WriteDurable applies
// perm to the temp file and renames it over the destination, so saving a user's 0644 original left
// it 0600 — a document shared with a group or served by a local web server stopped being readable
// the moment Nib saved it, with no message. But WriteDurable's other callers (the vault, the
// ceremony mirror) WANT the mode forced: a vault that somehow became 0644 must not stay that way on
// the next write. So the choice is the caller's, and it is made at the call site.
//
// Only the permission bits are carried, never ownership, and the stat happens just before the write:
// a mode changed in that gap is lost, which is the same bound diskChanged states about itself.
func ReplaceDurable(path string, data []byte, perm os.FileMode) error {
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		perm = fi.Mode().Perm()
	}
	return WriteDurable(path, data, perm)
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
