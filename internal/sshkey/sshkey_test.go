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
