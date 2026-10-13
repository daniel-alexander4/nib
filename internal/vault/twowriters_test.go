package vault

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Two Nib processes on one vault file (ADR-154, /pending 873).
//
// A second `*Vault` opened on the same directory stands for a second process throughout: each value
// holds its own copy of the contents, and each takes the file lock through its own open of the lock
// file, which is what two processes do. flock and LockFileEx are per open file, so two opens in one
// process exclude each other exactly as two processes would.

// TestAWriteFromOneProcessIsNotLostToAnother — the lost update, and it is not about key removal.
//
// Every save wrote the WHOLE vault from one process's memory, so a process that had not seen
// another's write wrote the file back without it. Measured before the lock on this test: `a` pins a
// peer and authorizes a key, `b` records a recent file, and a fresh open found neither the peer nor
// the key — the second machine's key slot was gone from the file with no error anywhere.
func TestAWriteFromOneProcessIsNotLostToAnother(t *testing.T) {
	a := newVault(t)
	dir := filepath.Dir(a.path)
	b, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}
	fp := bytes.Repeat([]byte{7}, 32)
	if err := a.AddPinnedPeer(fp, "from a"); err != nil {
		t.Fatal(err)
	}
	pub, keyPath := newKey(t)
	if err := a.AddKey(pub, keyPath); err != nil {
		t.Fatal(err)
	}
	if err := b.AddRecent("/from-b.pdf"); err != nil {
		t.Fatalf("a write from the second process: %v", err)
	}
	// And back the other way: a's memory predates b's write.
	if err := a.SetProfile(map[string]string{"name": "from a again"}); err != nil {
		t.Fatal(err)
	}

	fresh, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(fresh.PinnedPeers()); n != 1 {
		t.Errorf("the vault on disk holds %d pinned peer(s), want the 1 the first process pinned — "+
			"the second process wrote its own copy of the vault over it", n)
	}
	if n := len(fresh.Keys()); n != 2 {
		t.Errorf("the vault on disk holds %d authorized key(s), want 2 — the key the first process "+
			"authorized was dropped by the second process's save", n)
	}
	if r := fresh.Recent(); len(r) != 1 || r[0] != "/from-b.pdf" {
		t.Errorf("recent files on disk = %v, want the one the second process recorded", r)
	}
	if got := fresh.Profile()["name"]; got != "from a again" {
		t.Errorf("profile name on disk = %q, want the first process's last write", got)
	}
	// The process that wrote last sees what the other wrote: it read the file before changing it.
	if n := len(a.Recent()); n != 1 {
		t.Errorf("the first process holds %d recent file(s) after its write, want the second's 1", n)
	}
}

// TestAVaultThisProcessCannotOpenIsNotWrittenOver — the file was replaced by a DIFFERENT vault at
// the same generation (another process imported a backup). Nothing in the generation says so; the
// write is refused by name, and the imported file stays.
func TestAVaultThisProcessCannotOpenIsNotWrittenOver(t *testing.T) {
	a := newVault(t)
	other := newVault(t)
	if err := other.AddRecent("/in-the-import.pdf"); err != nil {
		t.Fatal(err)
	}
	imported, err := os.ReadFile(other.path)
	if err != nil {
		t.Fatal(err)
	}
	// As a second process's import does it: no handle on `a`.
	if err := Replace(a.path, nil, imported); err != nil {
		t.Fatal(err)
	}
	if err := a.AddRecent("/after.pdf"); !errors.Is(err, ErrChangedElsewhere) {
		t.Errorf("a write over a vault file this process cannot open = %v, want ErrChangedElsewhere", err)
	}
	now, err := os.ReadFile(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(now, imported) {
		t.Error("the vault another process imported was written over by a process that never opened it")
	}
	if len(a.Recent()) != 0 {
		t.Error("the refused write stayed in memory")
	}
}

// TestTheGenerationCheckAndTheRenameAreOneStepAcrossProcesses — /pending 873 part 1.
//
// The check and the rename were two steps with nothing held between them. `stale` is stopped
// between its check and its rename (the write seam runs under the file lock, after
// beforeReplacing's turn); a key removal started in that window must WAIT for it, and so lands
// after it. Without the lock the removal's rename went first and the stale rename then put the
// old key and the removed slot back.
func TestTheGenerationCheckAndTheRenameAreOneStepAcrossProcesses(t *testing.T) {
	v, dir, pubs, _ := threeKeyVault(t)
	stale, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}

	inWindow, release := make(chan struct{}), make(chan struct{})
	real := writeFileAtomic
	var once sync.Once
	writeFileAtomic = func(path string, data []byte, check func(string) error) error {
		return real(path, data, func(staged string) error {
			if check != nil {
				if err := check(staged); err != nil {
					return err
				}
			}
			// Only the stale writer's first save stops here, checked and not yet renamed.
			stop := false
			once.Do(func() { stop = true })
			if stop {
				close(inWindow)
				<-release
			}
			return nil
		})
	}
	t.Cleanup(func() { writeFileAtomic = real })

	staleDone := make(chan error, 1)
	go func() { staleDone <- stale.AddRecent("/from-the-stale-process.pdf") }()
	<-inWindow

	removed := make(chan error, 1)
	go func() { removed <- v.RemoveKey(pubs[1]) }()
	select {
	case err := <-removed:
		t.Fatalf("a key removal finished (%v) while another process stood between its generation "+
			"check and its rename — nothing is held across the two", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-staleDone; err != nil {
		t.Fatalf("the stale process's write: %v", err)
	}
	if err := <-removed; err != nil {
		t.Fatalf("the removal, once the other write had landed: %v", err)
	}

	env := mustEnvelope(t, dir)
	if env.Gen != 1 || len(env.SSH) != 2 {
		t.Fatalf("the file on disk is generation %d with %d slot(s), want the rotated vault "+
			"(generation 1, 2 slots): the stale rename landed after the removal's", env.Gen, len(env.SSH))
	}
	// And the removal was built on the stale process's write, not over it.
	fresh, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r := fresh.Recent(); len(r) == 0 || r[0] != "/from-the-stale-process.pdf" {
		t.Errorf("recent files after the removal = %v, want the one written just before it kept", r)
	}
}

// besidesTheLock lists a vault directory without the lock file, which is created once and stays
// (ADR-154) — for the tests that assert a failed write left no staging file behind.
func besidesTheLock(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.Name() != fileName+lockSuffix {
			names = append(names, e.Name())
		}
	}
	return names
}

// TestAWriterThatCannotGetTheLockIsRefusedByName — a bounded wait, then a named refusal; the vault
// in memory and the file are as they were.
func TestAWriterThatCannotGetTheLockIsRefusedByName(t *testing.T) {
	v := newVault(t)
	held, err := lockVaultFile(v.path)
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	was := lockWait
	lockWait = 150 * time.Millisecond
	t.Cleanup(func() { lockWait = was })

	before, err := os.ReadFile(v.path)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := v.AddRecent("/while-locked.pdf"); !errors.Is(err, ErrBusy) {
		t.Fatalf("a write while another process holds the vault's lock = %v, want ErrBusy", err)
	}
	if waited := time.Since(start); waited < lockWait {
		t.Errorf("refused after %v, before the %v wait was over", waited, lockWait)
	}
	if n, err := v.PruneCeremonyPeers("none"); !errors.Is(err, ErrBusy) {
		t.Errorf("a prune with nothing to prune, while locked out = %d, %v; want ErrBusy — it "+
			"judged a copy of the vault it could not bring up to date", n, err)
	}
	// The two roads to the file that are not mutators wait on the same lock: an import, and the
	// unlock that rewrites a key's recorded path.
	if err := Replace(v.path, nil, before); !errors.Is(err, ErrBusy) {
		t.Errorf("an import while another process holds the lock = %v, want ErrBusy", err)
	}
	if _, err := OpenSSHAt(filepath.Dir(v.path), v.ssh[0].KeyPath, nil); !errors.Is(err, ErrBusy) {
		t.Errorf("an unlock that rewrites the key path, while locked out = %v, want ErrBusy", err)
	}
	after, err := os.ReadFile(v.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || len(v.Recent()) != 0 {
		t.Error("a refused write changed the file or stayed in memory")
	}
	held.release()
	if err := v.AddRecent("/after.pdf"); err != nil {
		t.Errorf("a write once the lock was released: %v", err)
	}
}

// TestALockLeftBehindIsNotALock — the lock is the kernel's, never the file's presence: a lock file
// left by a process that is gone holds nothing.
func TestALockLeftBehindIsNotALock(t *testing.T) {
	v := newVault(t)
	if err := os.WriteFile(lockPath(v.path), []byte("left by a crash"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := v.AddRecent("/a.pdf"); err != nil {
		t.Errorf("a write beside a lock file nobody holds: %v", err)
	}
}

// TestTheLoneWriterReadsOnlyTheHead — a Nib running alone pays the head read and no more: the file
// under the nonce it last wrote is not read back. Observed through a file damaged PAST its head,
// which a whole read would refuse to open and a head read never sees; the save then replaces it,
// as it always has.
func TestTheLoneWriterReadsOnlyTheHead(t *testing.T) {
	v := newVault(t)
	if err := v.AddRecent("/a.pdf"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(v.path)
	if err != nil {
		t.Fatal(err)
	}
	at := bytes.Index(raw, []byte(`"cipher":"`)) + len(`"cipher":"`) + 8
	if at < generationHead {
		at = generationHead + 8
	}
	raw[at] ^= 1 // one base64 character of the ciphertext
	if err := os.WriteFile(v.path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSSH(filepath.Dir(v.path)); err == nil {
		t.Fatal("setup: the damaged file still opens, so nothing below shows it was not read")
	}
	if err := v.AddRecent("/b.pdf"); err != nil {
		t.Fatalf("a write by the only writer, over the file it last wrote = %v — the whole file was "+
			"read back on a write nobody else had touched", err)
	}
	if _, err := OpenSSH(filepath.Dir(v.path)); err != nil {
		t.Errorf("the vault after that write: %v", err)
	}
}

// TestNoSaveWithoutTheFileLock— save() itself refuses, so a new path to it cannot skip the lock.
func TestNoSaveWithoutTheFileLock(t *testing.T) {
	v := newVault(t)
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.save(); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Errorf("save() with the file lock not held = %v, want a refusal naming the lock", err)
	}
}

// TestEveryVaultMutatorBeginsAWrite — the routing (ADR-009): a function that calls mutateLocked
// took the write lock through beginWrite, which is also what brings memory up to date with the
// file BEFORE the function judges anything. A mutator that locked v.mu itself would validate
// against a copy another process has since changed.
func TestEveryVaultMutatorBeginsAWrite(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "vault.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	mutators := 0
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil || fd.Name.Name == "mutateLocked" {
			continue
		}
		calls := map[string]token.Pos{}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if name := calleeName(c.Fun); name != "" {
					if _, seen := calls[name]; !seen {
						calls[name] = c.Pos()
					}
				}
			}
			return true
		})
		if _, ok := calls["mutateLocked"]; !ok {
			continue
		}
		mutators++
		begin, ok := calls["beginWrite"]
		if !ok {
			t.Errorf("%s calls mutateLocked without beginWrite: it writes a copy of the vault that "+
				"was never brought up to date with the file, outside the cross-process lock", fd.Name.Name)
			continue
		}
		if begin > calls["mutateLocked"] {
			t.Errorf("%s begins its write after mutating", fd.Name.Name)
		}
	}
	if mutators < 19 {
		t.Errorf("the scan saw %d mutators; there are at least nineteen — it has stopped matching", mutators)
	}
}
