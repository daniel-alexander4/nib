package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/sshkey"
	"nib/internal/vault"
)

// unlockedServer returns a Server holding a freshly created, unlocked vault.
func unlockedServer(t *testing.T) (*Server, *vault.Vault) {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	pub, err := sshkey.Generate(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	s := New(os.DirFS("."), dir, "test")
	s.vault = v
	s.csrf = "test-token"
	return s, v
}

// TestRequireUnlockedPinsVaultToRequest proves the fix for the nil-deref race:
// a protected handler reads the vault snapshot requireUnlocked took, and that
// snapshot stays valid even if a concurrent vault import nils s.vault while the
// handler is running. Before the fix, handlers re-fetched s.unlockedVault() and
// would observe nil here.
func TestRequireUnlockedPinsVaultToRequest(t *testing.T) {
	s, want := unlockedServer(t)

	var before, after *vault.Vault
	probe := func(w http.ResponseWriter, r *http.Request) {
		before = vaultFrom(r)
		// Simulate /api/vault/import resetting the vault mid-request.
		s.mu.Lock()
		s.vault = nil
		s.mu.Unlock()
		after = vaultFrom(r)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	s.requireUnlocked(probe)(httptest.NewRecorder(), req)

	if before != want {
		t.Fatalf("vaultFrom before reset = %p, want the request snapshot %p", before, want)
	}
	if after != want {
		t.Fatalf("vaultFrom after reset = %p, want the request snapshot %p (request scope not isolated)", after, want)
	}
	if s.unlockedVault() != nil {
		t.Fatal("s.vault should be nil after the simulated import; the re-fetch path is what the fix avoids")
	}
}

// TestRequireUnlockedRejectsLocked confirms the gate still 401s when no vault is
// open, and never reaches the handler (so vaultFrom is never nil inside one).
func TestRequireUnlockedRejectsLocked(t *testing.T) {
	s := New(os.DirFS("."), t.TempDir(), "test") // s.vault stays nil (locked)

	called := false
	probe := func(w http.ResponseWriter, r *http.Request) { called = true }

	rec := httptest.NewRecorder()
	s.requireUnlocked(probe)(rec, httptest.NewRequest(http.MethodGet, "/api/recent", nil))

	if called {
		t.Fatal("handler ran while locked; requireUnlocked must reject first")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
