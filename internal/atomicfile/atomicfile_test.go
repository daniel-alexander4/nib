package atomicfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The package had NO tests at all until the slice's diff review said so — measured there:
// replacing WriteDurable's body with a plain os.WriteFile (no temp file, no fsync, not
// atomic, and briefly at the process umask's mode) left `internal/vault` and
// `internal/ceremony` both green. The whole reason the package was extracted was unasserted,
// including the 0600 that the vault and the ceremony mirror both depend on.

func TestWriteDurableWritesTheContentAtTheRequestedMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	want := []byte("thirty-two bytes of key material")
	if err := WriteDurable(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("read back %q, want %q", got, want)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// The mode is the caller's, applied BEFORE the rename — so the file is never briefly
	// readable at the umask's default under its final name. Both consumers rely on this: the
	// vault holds the signing key and the mirror holds a signing in progress.
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode is %04o, want 0600 — a plain os.WriteFile takes the process umask, "+
			"which on a shared machine is world-readable", perm)
	}
}

func TestWriteDurableOverwritesAndLeavesNoTemporary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.nib")
	if err := WriteDurable(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteDurable(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "second" {
		t.Errorf("read back %q after overwrite, want %q", got, "second")
	}
	// A temp file left behind is how a directory the user browses fills with debris — and
	// ~/nib IS browsable through Nib's own file dialogs.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the directory holds %v — the temp file was not cleaned up", names)
	}
}

// TestWriteDurableREPLACESTheFileRatherThanWritingThrough is the property that distinguishes
// this from `os.WriteFile`, and the first draft of this test could not see it.
//
// That draft tried to prove non-truncation by making the write fail against a read-only
// DIRECTORY — and os.WriteFile does not need directory permission to reopen an existing file,
// so it succeeded, and the test's own stimulus guard turned the difference into a t.Skip.
// Measured: replacing WriteDurable's body with os.WriteFile left all three tests GREEN. A
// skip that fires on the case under test is a pass nobody earned.
//
// Two discriminators that are deterministic and need no failure injection:
//
//  1. **The inode changes.** os.WriteFile opens the existing file with O_TRUNC and writes
//     through the same inode — so the moment it starts, the old contents are gone and an
//     interrupted write leaves a truncated file where the only copy of the signing identity
//     used to be. A temp-plus-rename never touches the target: the new bytes land on a new
//     inode and the rename swaps them in whole.
//  2. **The mode is applied on every write.** os.WriteFile honours perm only when it CREATES
//     the file; overwriting one that already exists leaves the old mode. WriteDurable chmods
//     the temp file before the rename, so 0600 is what the caller gets even if the file was
//     already there at something laxer — which is the case that matters, because both
//     consumers overwrite.
func TestWriteDurableREPLACESTheFileRatherThanWritingThrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.nib")

	// Pre-create at a LAX mode, which is the state os.WriteFile would preserve.
	if err := os.WriteFile(path, []byte("the original"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the file really is there at 0644 first, or both assertions below are about
	// a file that was freshly created and prove nothing about overwriting.
	if before.Mode().Perm() != 0o644 {
		t.Fatalf("setup: pre-created at %04o, want 0644", before.Mode().Perm())
	}
	beforeIno, ok := inode(before)
	if !ok {
		t.Skip("this platform does not expose an inode; the replace half cannot be observed here")
	}

	if err := WriteDurable(path, []byte("the replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := after.Mode().Perm(); perm != 0o600 {
		t.Errorf("after overwriting a 0644 file the mode is %04o, want 0600 — os.WriteFile "+
			"honours perm only on CREATE, so a vault or a ceremony mirror written over an "+
			"existing file would keep whatever mode it had", perm)
	}
	afterIno, _ := inode(after)
	if afterIno == beforeIno {
		t.Errorf("the file kept inode %d, so the bytes were written THROUGH the existing "+
			"file — the target is truncated the moment the write starts, and an interrupted "+
			"write leaves a truncated vault where the only copy of the signing identity was. "+
			"A temp-plus-rename lands on a new inode.", beforeIno)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "the replacement" {
		t.Errorf("read back %q, want %q", got, "the replacement")
	}
}

// inode returns the file's inode number where the platform exposes one.
func inode(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Ino), true
}
