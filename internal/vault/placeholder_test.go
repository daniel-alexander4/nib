package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAFailedSaveInCreateLeavesNoVaultBehind — /pending 502.
//
// TestAFailedCreateLeavesNoVaultBehind covers a failure inside newSealed. Create then returned
// `v, v.Save()`, so a Save that failed — a full disk, a read-only config directory — left the
// zero-byte placeholder, which Exists calls a vault and readEnvelope calls corrupt.
func TestAFailedSaveInCreateLeavesNoVaultBehind(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	saved := writeFileAtomic
	t.Cleanup(func() { writeFileAtomic = saved })
	calls := 0
	writeFileAtomic = func(string, []byte) error { calls++; return errors.New("disk full") }

	if _, err := Create(dir, pub, keyPath); err == nil {
		t.Fatal("Create succeeded although its Save failed")
	}
	// STIMULUS: the failure really came from Save, after the name was claimed.
	if calls != 1 {
		t.Fatalf("setup: the failing write was called %d times, want 1", calls)
	}
	if Exists(dir) {
		t.Error("a Create whose Save failed left a file Exists() calls a vault — every later launch reports key-missing and enrol answers 409")
	}
	writeFileAtomic = saved
	if _, err := Create(dir, pub, keyPath); err != nil {
		t.Errorf("after a failed Save, a second Create could not set up the vault: %v", err)
	}
}

// TestAStrandedPlaceholderDegradesToSetup — /pending 502.
//
// A process that died between claiming the name and writing the vault left an empty file for good.
// A zero-byte file older than any in-flight Create is taken for that placeholder: not a vault, and
// reclaimed by the next Create. A FRESH one is still honoured, or two concurrent first runs would
// both proceed and the second rename would replace the first vault.
func TestAStrandedPlaceholderDegradesToSetup(t *testing.T) {
	pub, keyPath := newKey(t)

	t.Run("stale", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(Path(dir), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-2 * placeholderStale)
		if err := os.Chtimes(Path(dir), old, old); err != nil {
			t.Fatal(err)
		}
		// STIMULUS: the file really is there, empty, and unreadable as a vault.
		if _, err := readEnvelope(dir); err == nil {
			t.Fatal("setup: an empty vault file parsed as an envelope")
		}
		if Exists(dir) {
			t.Error("a stranded zero-byte placeholder is reported as a vault")
		}
		if _, err := Create(dir, pub, keyPath); err != nil {
			t.Fatalf("Create could not reclaim a stranded placeholder: %v", err)
		}
		if _, err := OpenSSH(dir); err != nil {
			t.Errorf("the vault created over a stranded placeholder does not open: %v", err)
		}
	})

	t.Run("fresh", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(Path(dir), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if !Exists(dir) {
			t.Error("a placeholder younger than any Create could hold it was treated as stranded")
		}
		if _, err := Create(dir, pub, keyPath); err == nil {
			t.Error("Create proceeded over another Create's in-flight placeholder")
		}
	})
}

// TestAnUnreadableVaultIsDistinguishable — /pending 502. Every way a present vault file cannot be
// read carries ErrUnreadable, so status can stop calling it a missing key.
func TestAnUnreadableVaultIsDistinguishable(t *testing.T) {
	pub, keyPath := newKey(t)
	cases := map[string][]byte{
		"not json":      []byte("{garbage"),
		"newer version": []byte(`{"version": 99}`),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, fileName), body, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenSSH(dir); !errors.Is(err, ErrUnreadable) {
				t.Errorf("OpenSSH on a %s vault returned %v, want ErrUnreadable", name, err)
			}
		})
	}
	// The sentinel is not on a readable vault whose key is simply absent.
	dir := t.TempDir()
	if _, err := Create(dir, pub, keyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(keyPath, keyPath+".moved"); err != nil {
		t.Fatal(err)
	}
	_ = os.Rename(keyPath+".pub", keyPath+".pub.moved")
	if _, err := OpenSSH(dir); errors.Is(err, ErrUnreadable) {
		t.Errorf("a readable vault with its key moved away reported ErrUnreadable (%v)", err)
	}
}
