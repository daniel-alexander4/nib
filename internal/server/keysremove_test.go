package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/sshkey"
	"nib/internal/vault"
)

// removeKeyOver posts one removal through the route as the window does.
func removeKeyOver(t *testing.T, s *Server, pub string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"pubKey": pub})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/ssh/keys/remove", strings.NewReader(string(body)))
	req.Header.Set("X-CSRF-Token", "test-token")
	rec := httptest.NewRecorder()
	s.requireUnlocked(s.handleKeysRemove)(rec, req)
	return rec
}

func addedKey(t *testing.T, v *vault.Vault) string {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	pub, err := sshkey.Generate(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.AddKey(pub, keyPath); err != nil {
		t.Fatal(err)
	}
	return pub
}

// TestRemovingAKeyOverTheRouteSaysWhyItWasRefused — ADR-144's two refusals reach the window as
// what they are. Both were "could not remove key" with a 500 before they had a case of their own,
// which tells a user nothing they can act on.
func TestRemovingAKeyOverTheRouteSaysWhyItWasRefused(t *testing.T) {
	t.Run("a vault whose key another Nib has replaced", func(t *testing.T) {
		s, v := unlockedServer(t)
		first, second := addedKey(t, v), addedKey(t, v)
		// A second process, opened before the removal: it holds the old key and all three slots.
		stale, err := vault.OpenSSH(s.configDir)
		if err != nil {
			t.Fatal(err)
		}
		if rec := removeKeyOver(t, s, first); rec.Code != http.StatusOK {
			t.Fatalf("the removal answered %d: %s", rec.Code, rec.Body)
		}
		if slots, err := vault.Slots(s.configDir); err != nil || len(slots) != 2 {
			t.Fatalf("after the removal the file carries %d slot(s) (%v), want 2", len(slots), err)
		}
		rotated, err := os.ReadFile(vault.Path(s.configDir))
		if err != nil {
			t.Fatal(err)
		}

		s.mu.Lock()
		s.vault = stale
		s.mu.Unlock()
		rec := removeKeyOver(t, s, second)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "another running Nib") {
			t.Errorf("a removal by a Nib holding the old key answered %d %s, want 409 saying another "+
				"running Nib changed the vault", rec.Code, rec.Body)
		}
		now, err := os.ReadFile(vault.Path(s.configDir))
		if err != nil {
			t.Fatal(err)
		}
		if string(now) != string(rotated) {
			t.Error("the refused removal replaced the vault file")
		}
	})

	t.Run("a key that stays cannot be sealed to", func(t *testing.T) {
		s, v := unlockedServer(t)
		pub := addedKey(t, v)
		// A slot whose public key is not one: written into the file, as damage or an edit would.
		path := vault.Path(s.configDir)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var env map[string]json.RawMessage
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatal(err)
		}
		var slots []map[string]any
		if err := json.Unmarshal(env["ssh"], &slots); err != nil {
			t.Fatal(err)
		}
		slots = append(slots, map[string]any{"pubKey": "ssh-ed25519 not-base64 the-office-laptop", "keyPath": "", "wrapped": "AAAA"})
		if env["ssh"], err = json.Marshal(slots); err != nil {
			t.Fatal(err)
		}
		damaged, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, damaged, 0o600); err != nil {
			t.Fatal(err)
		}
		reopened, err := vault.OpenSSH(s.configDir)
		if err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		s.vault = reopened
		s.mu.Unlock()

		rec := removeKeyOver(t, s, pub)
		if rec.Code != http.StatusConflict {
			t.Fatalf("the removal answered %d %s, want 409", rec.Code, rec.Body)
		}
		for _, want := range []string{`the-office-laptop`, "no key was removed", "Remove that key first"} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Errorf("the refusal %s does not say %q", rec.Body, want)
			}
		}
		if strings.Contains(rec.Body.String(), "not-base64") {
			t.Errorf("the refusal quotes the key line: %s", rec.Body)
		}
		if now, _ := os.ReadFile(path); string(now) != string(damaged) {
			t.Error("the refused removal changed the vault file")
		}
		// And the way out it names works: the unsealable key can itself be removed.
		if rec := removeKeyOver(t, s, "ssh-ed25519 not-base64 the-office-laptop"); rec.Code != http.StatusOK {
			t.Errorf("removing the key that cannot be sealed to answered %d %s, want 200", rec.Code, rec.Body)
		}
	})
}
