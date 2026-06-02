package sshkey

import (
	"bytes"
	"path/filepath"
	"testing"
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

	got, err := Unwrap(wrapped, keyPath)
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
	if _, err := Unwrap(wrapped, p2); err == nil {
		t.Error("unwrapping with the wrong key should fail")
	}
}
