package vault

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// TestADamagedVaultIsNotReportedAsAMissingKey — /pending 776.
//
// One flipped ciphertext byte, with the enrolled key present and unwrapping its slot: both unlock
// paths answered ErrKeyMissing, which the status route shows as "key-missing" and the open-at route
// as "that key does not open this vault — it is sealed to a different key". The key was never the
// problem. The control is the same vault with its key gone, which must still read as a missing key —
// without it this test would pass on a build that called every failure unreadable.
func TestADamagedVaultIsNotReportedAsAMissingKey(t *testing.T) {
	pub, keyPath := newKey(t)
	dir := t.TempDir()
	if _, err := Create(dir, pub, keyPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Cipher) == 0 {
		t.Fatal("setup: the envelope has no ciphertext to damage")
	}
	env.Cipher[len(env.Cipher)/2] ^= 0x01
	damaged, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), damaged, 0o600); err != nil {
		t.Fatal(err)
	}

	opens := map[string]func() (*Vault, error){
		"OpenSSH":   func() (*Vault, error) { return OpenSSH(dir) },
		"OpenSSHAt": func() (*Vault, error) { return OpenSSHAt(dir, keyPath, nil) },
	}
	for name, open := range opens {
		_, err := open()
		if !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s on a vault whose contents do not decrypt under a key that unwrapped: got %v, want ErrUnreadable", name, err)
		}
		if errors.Is(err, ErrKeyMissing) {
			t.Errorf("%s reported a damaged vault as a missing key: %v", name, err)
		}
	}

	// CONTROL: the key gone, the file whole again — that IS a missing key.
	if err := os.WriteFile(Path(dir), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	for name, open := range opens {
		if _, err := open(); !errors.Is(err, ErrKeyMissing) || errors.Is(err, ErrUnreadable) {
			t.Errorf("%s with the key file gone: got %v, want ErrKeyMissing and not ErrUnreadable", name, err)
		}
	}
}
