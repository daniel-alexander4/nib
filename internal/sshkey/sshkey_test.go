package sshkey

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateWrapUnwrap(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	pubLine, err := Generate(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if pubLine == "" {
		t.Fatal("empty public key line")
	}

	secret := []byte("0123456789abcdef0123456789abcdef") // 32-byte content key
	wrapped, err := Wrap(secret, pubLine)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	if bytes.Contains(wrapped, secret) {
		t.Error("wrapped blob contains the plaintext secret")
	}

	got, err := Unwrap(wrapped, keyPath, pubLine, nil)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Errorf("unwrapped secret mismatch: got %x", got)
	}
}

func TestGenerateRefusesOverwrite(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if _, err := Generate(keyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(keyPath); err == nil {
		t.Error("Generate over an existing key should fail")
	}
}

func TestUnwrapWrongKeyFails(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "k1")
	p2 := filepath.Join(dir, "k2")
	pub1, _ := Generate(p1)
	Generate(p2)
	wrapped, _ := Wrap([]byte("secret-payload-here"), pub1)
	if _, err := Unwrap(wrapped, p2, pub1, nil); err == nil {
		t.Error("unwrapping with the wrong key should fail")
	}
}

// encryptedKey writes a passphrase-protected ed25519 key and returns its path and
// authorized_keys line — the fixture for the passphrase-unwrap tests.
func encryptedKey(t *testing.T, dir, passphrase string) (keyPath, pubLine string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "nib-test", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	keyPath = filepath.Join(dir, "id_ed25519_enc")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return keyPath, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

func TestPublicKeyLineEncryptedNoPub(t *testing.T) {
	// An encrypted OpenSSH key with no .pub sibling: the line is derived from the
	// cleartext public half the format embeds, with no passphrase. This is the
	// enroll case that used to dead-end on "passphrase protected".
	dir := t.TempDir()
	keyPath, want := encryptedKey(t, dir, "hunter2")
	got, err := PublicKeyLine(keyPath)
	if err != nil {
		t.Fatalf("PublicKeyLine: %v", err)
	}
	if got != want {
		t.Errorf("public key line mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestPublicKeyLinePlainNoPub(t *testing.T) {
	// An unencrypted key whose .pub was removed still resolves via the private key.
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	want, err := Generate(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(keyPath + ".pub"); err != nil {
		t.Fatal(err)
	}
	got, err := PublicKeyLine(keyPath)
	if err != nil {
		t.Fatalf("PublicKeyLine: %v", err)
	}
	if got != want {
		t.Errorf("public key line mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestUnwrapPassphrase(t *testing.T) {
	dir := t.TempDir()
	keyPath, pubLine := encryptedKey(t, dir, "hunter2")
	secret := []byte("0123456789abcdef0123456789abcdef")
	wrapped, err := Wrap(secret, pubLine)
	if err != nil {
		t.Fatal(err)
	}

	// No passphrase on an encrypted key → ErrPassphraseRequired (prompt + retry).
	if _, err := Unwrap(wrapped, keyPath, pubLine, nil); !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("no passphrase: err = %v, want ErrPassphraseRequired", err)
	}
	// Wrong passphrase → ErrWrongPassphrase (reprompt).
	if _, err := Unwrap(wrapped, keyPath, pubLine, []byte("nope")); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase: err = %v, want ErrWrongPassphrase", err)
	}
	// Correct passphrase → recovers the secret (key stays encrypted on disk).
	got, err := Unwrap(wrapped, keyPath, pubLine, []byte("hunter2"))
	if err != nil {
		t.Fatalf("correct passphrase: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Errorf("unwrapped secret mismatch: got %x", got)
	}
}

// Generate's overwrite refusal moved from `os.Stat` to `O_CREATE|O_EXCL` at v1.116.12,
// and NO test is added for it, deliberately.
//
// The doc says "It refuses to overwrite an existing file" and `os.Stat` + `os.WriteFile`
// did not enforce that — WriteFile is O_TRUNC, so any Stat error other than ErrNotExist
// fell through to a truncating write of what is typically ~/.ssh/id_ed25519: the user's SSH
// identity AND the key the vault's content key is sealed to. The change is right on merit
// and makes the contract the kernel's job.
//
// But the two versions could not be told apart by any case that can be built here. The
// obvious probe — a key inside a directory with no execute permission — has `os.Stat` fail
// with EACCES and then `os.WriteFile` fail with EACCES too, so the old code refused for its
// own reason and the test passed against it. The genuine difference is the TOCTOU window
// between the check and the write, which a test cannot open. Recorded rather than papered
// over with a green that implies coverage it does not have.
//
// TestGenerateRefusesOverwrite above still covers the ordinary case, and now tests the real
// mechanism rather than a check beside it.
