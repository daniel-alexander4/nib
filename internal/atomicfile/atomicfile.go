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
// **And the two split exports disagree on purpose**, which is the reason to write the figure down
// rather than the figure alone. The GUI's (`internal/server/export.go`) takes this door and says
// why — *"every part comes from the document still open in this process"*. The CLI's
// (`writeSplitFiles` → `writeNamed`) takes `ReplaceDurable`, because every CLI write is routed
// through the door built for `-w`. **The premise is conceded and the conclusion is not**
// (/pending 550): a split part is indeed not the user's only copy, nothing is replaced and the
// input is untouched — but `writeNamed` is not reached for its fsync alone. It also resolves a
// symlink at the destination and carries an existing file's mode, and `--out-dir` is a folder the
// user chose. Every door here finishes with a rename and a rename over a link REPLACES the link,
// so moving the CLI's split to this function would reintroduce, in a directory the user named,
// exactly the silent loss /pending 515 removed from that package. The fsync is bought back at that
// price or not at all, and it is not worth it.
//
// Re-measured on that disposition, since the figure is the whole argument (same host, the write
// loop timed inside the real `nib split --every 1`, three rounds, 280 parts): 16.8–20.5 ms a part
// durable against 73–106 µs here — 4.7–5.7 s of a 28–66 s command, 9–20% — and on the shape people
// run, 28 parts, 224–276 ms of 3.3–6.6 s. The split's own run-to-run spread on a loaded machine
// was ±38 s.
//
// So `internal/cli`'s `atomicdurable_test.go` keeps its refusal and gains no exemption door. It
// did gain something else: it tested for the literal `atomicfile.Write(` and so could not see
// `WriteFrom`, which is this door's own choice under another name.
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
//
// # It follows a symlink to its target, and that is the door for the whole tree (/pending 515)
//
// Every door here finishes with a rename, and a rename over a symlink REPLACES THE LINK: the user's
// link becomes a regular file holding the new bytes, the real document it pointed at keeps the old
// ones, and nothing says so. `internal/cli`'s `writeNamed` learned this the hard way — *"an in-place
// rewrite of a symlink silently destroyed the link, left the real file untouched, and dropped the
// output in the link's directory instead"* — and it now calls this function rather than carrying a
// second copy of the resolution (ADR-009). The GUI's two save doors had no copy at all, so saving a
// document opened through a link did exactly that.
//
// **Resolved HERE, immediately before the write, and deliberately not at the point the file was
// opened.** A path resolved when a document is opened and used when it is saved writes to whatever
// the link pointed at then — so a link re-pointed in between sends the save to the wrong file, which
// is a worse failure than the one being fixed. It also makes the resolution invisible to the user:
// `doc.path` is what Nib shows her and what Open Recent records, and a link she re-points on purpose
// is not something Nib may quietly replace with today's target. The window here is the width of the
// write, the same bound this function's permission stat already states about itself.
//
// **Only the user-owned door does this.** `WriteDurable`'s own callers write nib's files — the
// vault, the ceremony mirror, the sidecars — where a link is not part of the contract and following
// one would let anything that can drop a link into `~/nib` redirect a write.
//
// A link that cannot be resolved is left alone: `EvalSymlinks` fails on a dangling link and on a
// loop, and neither has a target to write to. Renaming over the link is then the only thing that can
// succeed, and it is what happened before this existed.
//
// **The declared exposure**: a link planted at the destination by someone else sends this write to
// wherever it points, within what the user could already write. That is the same trade
// `internal/cli` has made since /pending 504 for a path named on the command line, and every caller
// here names a path the user chose in a file dialog — over an existing file, only after the route's
// own `os.Stat` refusal has been answered with an explicit `overwrite=1`.
func ReplaceDurable(path string, data []byte, perm os.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		perm = fi.Mode().Perm()
	}
	return WriteDurable(path, data, perm)
}

// UserFileMode is the permission bits a NEW file the user named is created with, at every door
// (/pending 572).
//
// # It exists because three doors were answering this alone, and two of them disagreed
//
// `ReplaceDurable`'s `perm` applies only when the path does not exist yet, so it is invisible on
// every ordinary save and decides everything about a Save As. The three call sites each named a
// literal: `internal/cli`'s `writeNamed` had always given a new file **0644**, and the GUI's Save
// and Save As both passed **0600** — the same operation, two surfaces, two answers, chosen by
// nobody. That is byte-for-byte `/pending 570`'s split-part finding at a third door.
//
// # 0644, and the argument is already written one package over
//
// `pdfops.SplitPartMode` settled the identical question for a split part and its reasoning is the
// whole of this one: *"0600 is nib's habit for what nib owns — the vault, the ceremony mirror, the
// sidecars — and it is the wrong habit for a file the user asked to be written into a folder they
// named."* Behind that sits `/pending 499`, which is why `ReplaceDurable` exists at all: forcing
// 0600 onto a user's document meant *"a document shared with a group or served by a local web
// server stopped being readable the moment Nib saved it, with no message."* A Save As is that
// document, not a copy of it — if anything it is the clearer case, because a split part is
// re-derivable and this is the user taking their document OUT of nib.
//
// # The declared exposure, because 0600 is not a silly answer
//
// Saving a private document to a new path makes it as readable as the destination folder allows.
// That is a real widening and it is the price. Three things bound it, the same three
// `SplitPartMode` states: the folder's own permissions still gate access; the CLI has always
// written 0644, so this is the status quo at one of the three doors rather than a new exposure;
// and the destination is a path the user typed into a file dialog.
//
// **Carrying the SOURCE document's mode was considered and refused**, and it is refused here for
// the reason it was there: a document uploaded through the browser has no path on disk and so no
// mode to carry, which would make the doors agree for some documents and not others — the defect
// rebuilt out of a better motive.
//
// **This is not `WriteDurable`'s default and must not become one.** That door writes nib's own
// files, where 0600 is right and where forcing it is the point.
const UserFileMode os.FileMode = 0o644

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
