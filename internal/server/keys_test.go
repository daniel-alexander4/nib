package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Creating a key from the Authorized-keys dialog generates a fresh keypair on
// disk and authorizes it on the vault, leaving the original key in place.
func TestKeysAddCreateGeneratesAndAuthorizes(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	newKey := filepath.Join(t.TempDir(), "second_ed25519")
	resp := write(t, c, csrf, "POST", ts.URL+"/api/ssh/keys", "application/json",
		jsonBody(addKeyRequest{Mode: "create", KeyPath: newKey}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create-key status = %d, want 200", resp.StatusCode)
	}
	var got keysResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Keys) != 2 {
		t.Fatalf("authorized keys = %d, want 2 (original + created)", len(got.Keys))
	}
	if _, err := os.Stat(newKey); err != nil {
		t.Fatalf("private key not written: %v", err)
	}
	if _, err := os.Stat(newKey + ".pub"); err != nil {
		t.Fatalf("public key not written: %v", err)
	}
}

// Creating at a path that already holds a key must not clobber it.
func TestKeysAddCreateRefusesToOverwrite(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	existing := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(existing, []byte("not really a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := write(t, c, csrf, "POST", ts.URL+"/api/ssh/keys", "application/json",
		jsonBody(addKeyRequest{Mode: "create", KeyPath: existing}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("create over existing status = %d, want 409", resp.StatusCode)
	}
	if b, _ := os.ReadFile(existing); string(b) != "not really a key" {
		t.Fatal("existing key file was overwritten")
	}
}
