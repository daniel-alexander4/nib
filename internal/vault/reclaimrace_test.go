package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAReclaimNeverDeletesAVaultAnotherProcessFinished — /pending 807 R8.
//
// The reclaim of a stranded placeholder was Lstat, then remove BY NAME, then O_EXCL. Process B
// judges the placeholder stale; process A then reclaims the same one, claims the name and persists
// a real vault there; B's remove deleted A's vault and B claimed the name — both reported success,
// and A's user had been shown a signing identity whose private half was gone. The seam runs A's
// whole Create inside B's window.
func TestAReclaimNeverDeletesAVaultAnotherProcessFinished(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	if err := os.WriteFile(Path(dir), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * placeholderStale)
	if err := os.Chtimes(Path(dir), old, old); err != nil {
		t.Fatal(err)
	}

	var aVault []byte
	ran := false
	beforeReclaimMove = func() {
		beforeReclaimMove = nil // A's own Create must not re-enter the seam
		ran = true
		if err := os.Remove(Path(dir)); err != nil { // A reclaimed the placeholder first
			t.Fatal(err)
		}
		if _, err := Create(dir, pub, keyPath); err != nil {
			t.Fatalf("setup: process A could not create its vault: %v", err)
		}
		b, err := os.ReadFile(Path(dir))
		if err != nil || len(b) == 0 {
			t.Fatalf("setup: A's vault is not on disk (%d bytes, %v)", len(b), err)
		}
		aVault = b
	}
	t.Cleanup(func() { beforeReclaimMove = nil })

	_, berr := Create(dir, pub, keyPath)
	if !ran {
		t.Fatal("setup: B never reached the reclaim, so the interleaving was not driven")
	}
	got, err := os.ReadFile(Path(dir))
	if err != nil || !bytes.Equal(got, aVault) {
		t.Fatalf("after B's reclaim the vault file holds %d bytes (%v), want A's %d-byte vault — B "+
			"deleted a vault another process had finished, with its content key", len(got), err, len(aVault))
	}
	if berr == nil {
		t.Error("B's Create reported success while A's vault holds the name — two processes both " +
			"believe they set up the vault")
	}
	leftovers, _ := filepath.Glob(Path(dir) + ".stranded-*")
	if len(leftovers) != 0 {
		t.Errorf("the reclaim left %v beside the vault", leftovers)
	}
}
