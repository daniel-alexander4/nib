package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCreateDurableWritesANewFileAndNeverReplacesOne — /pending 502.
//
// The door sshkey.Generate writes a new private key through. The sync itself is not observable
// without cutting power; what is observable is the half that makes it safe to call where an
// irreplaceable file may already sit: it creates, and it refuses.
func TestCreateDurableWritesANewFileAndNeverReplacesOne(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "id_ed25519")

	if err := CreateDurable(p, []byte("new key"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(got, []byte("new key")) {
		t.Fatalf("read back %q (err %v), want the written content", got, err)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v, want 0600", fi.Mode().Perm())
		}
	}

	err = CreateDurable(p, []byte("attacker"), 0o600)
	if !os.IsExist(err) {
		t.Errorf("a second CreateDurable over an existing file returned %v, want an exists error", err)
	}
	if got, _ := os.ReadFile(p); !bytes.Equal(got, []byte("new key")) {
		t.Errorf("the refused create changed the existing file to %q", got)
	}
}
