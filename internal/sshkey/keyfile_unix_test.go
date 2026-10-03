//go:build unix

package sshkey

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestAKeyPathThatIsNotAKeyFileIsRefusedPromptly — /pending 807 R8.
//
// An imported vault backup names each slot's key path, and `Validate` unwraps every slot from that
// untrusted file. `os.ReadFile` reads to EOF, so a FIFO hung the import and `/dev/zero` grew the
// heap without bound. Every key read must refuse a non-regular file, and an oversized one, at once.
func TestAKeyPathThatIsNotAKeyFileIsRefusedPromptly(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "id_fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	big := filepath.Join(dir, "id_big")
	if err := os.WriteFile(big, []byte(strings.Repeat("A", maxKeyFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}

	// The FIFO FIRST: under the old read it blocks forever, and the order keeps `/dev/zero` — which
	// under the old read exhausts memory — from being reached by a red run.
	for _, path := range []string{fifo, "/dev/zero", big} {
		for name, read := range map[string]func() error{
			"Unwrap":        func() error { _, err := Unwrap([]byte("x"), path, "", nil); return err },
			"PublicKeyLine": func() error { _, err := PublicKeyLine(path); return err },
		} {
			done := make(chan error, 1)
			go func() { done <- read() }()
			select {
			case err := <-done:
				if err == nil {
					t.Errorf("%s(%s) succeeded", name, path)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("%s(%s) is still reading after 3 s — a key path the backup chose holds the "+
					"import hostage", name, path)
			}
		}
	}
}
