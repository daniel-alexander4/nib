// Package sshkey wraps small secrets to an SSH public key and unwraps them with
// the matching private key, using age's SSH support. Nib uses it to seal the
// vault's content key to the user's SSH key, so the vault unlocks at startup
// from that key with no password.
package sshkey

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"golang.org/x/crypto/ssh"
)

// Wrap seals secret to the SSH public key given as an authorized_keys line
// (ed25519 or RSA). The result is an age ciphertext.
func Wrap(secret []byte, pubLine string) ([]byte, error) {
	rcpt, err := agessh.ParseRecipient(pubLine)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, rcpt)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(secret); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WrapMulti seals secret to several SSH public keys (authorized_keys lines) at
// once, so any one of their private keys can unwrap it — an age ciphertext with
// multiple recipients.
func WrapMulti(secret []byte, pubLines []string) ([]byte, error) {
	if len(pubLines) == 0 {
		return nil, errors.New("no recipients")
	}
	rcpts := make([]age.Recipient, 0, len(pubLines))
	for _, line := range pubLines {
		r, err := agessh.ParseRecipient(line)
		if err != nil {
			return nil, err
		}
		rcpts = append(rcpts, r)
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, rcpts...)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(secret); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ErrPassphraseRequired means the key at keyPath is passphrase-protected but no
// passphrase was supplied — the caller should prompt for one and retry.
// ErrWrongPassphrase means a passphrase was supplied but didn't decrypt the key.
var (
	ErrPassphraseRequired = errors.New("ssh key is passphrase-protected")
	ErrWrongPassphrase    = errors.New("wrong passphrase")
)

// Unwrap recovers a secret wrapped by Wrap, using the private key at keyPath.
// pubLine is the authorized_keys line the secret was wrapped to (the recipient);
// it is needed only on the passphrase path, to build the encrypted identity.
//
// With passphrase nil this is the promptless startup unlock: it expects an
// unencrypted key and returns ErrPassphraseRequired if the key turns out to be
// passphrase-protected. With a passphrase it decrypts such a key IN MEMORY (the
// key file stays encrypted on disk — that's the at-rest hardening), returning
// ErrWrongPassphrase if the passphrase doesn't fit.
func Unwrap(wrapped []byte, keyPath, pubLine string, passphrase []byte) ([]byte, error) {
	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	id, err := agessh.ParseIdentity(pemBytes)
	if err != nil {
		var miss *ssh.PassphraseMissingError
		if !errors.As(err, &miss) {
			return nil, err
		}
		if passphrase == nil {
			return nil, ErrPassphraseRequired
		}
		// NewEncryptedSSHIdentity needs the recipient public key to match stanzas;
		// the vault stores it alongside the slot, so use that rather than relying on
		// a sibling .pub or the error's optional PublicKey.
		pub, _, _, _, perr := ssh.ParseAuthorizedKey([]byte(pubLine))
		if perr != nil {
			return nil, perr
		}
		id, err = agessh.NewEncryptedSSHIdentity(pub, pemBytes, func() ([]byte, error) { return passphrase, nil })
		if err != nil {
			return nil, err
		}
	}
	r, err := age.Decrypt(bytes.NewReader(wrapped), id)
	if err != nil {
		// agessh wraps a bad passphrase as this message (with %v, so no error chain
		// to match on) before age sees it as a hard failure rather than a non-match.
		if strings.Contains(err.Error(), "failed to decrypt SSH key file") {
			return nil, ErrWrongPassphrase
		}
		return nil, err
	}
	return io.ReadAll(r)
}

// Generate creates a new unencrypted ed25519 key pair, writing the private key
// to privPath and the public key to privPath+".pub". It returns the public key
// as an authorized_keys line. It refuses to overwrite an existing file.
func Generate(privPath string) (pubLine string, err error) {
	if _, err := os.Stat(privPath); err == nil {
		return "", os.ErrExist
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, "nib")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(privPath), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(privPath, pem.EncodeToMemory(block), 0o600); err != nil {
		return "", err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", err
	}
	pubLine = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if err := os.WriteFile(privPath+".pub", []byte(pubLine+"\n"), 0o644); err != nil {
		return "", err
	}
	return pubLine, nil
}

// PublicKeyLine returns the authorized_keys line for the private key at keyPath,
// preferring a sibling ".pub" file and otherwise deriving it from the key.
func PublicKeyLine(keyPath string) (string, error) {
	if b, err := os.ReadFile(keyPath + ".pub"); err == nil {
		return strings.TrimSpace(string(b)), nil
	}
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return "", err
	}
	raw, err := ssh.ParseRawPrivateKey(b)
	if err != nil {
		return "", err
	}
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}

// Candidates lists existing default private keys under ~/.ssh.
func Candidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		p := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// DefaultNewKeyPath is where a freshly created key is written: ~/.ssh/id_ed25519.
func DefaultNewKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "id_ed25519"
	}
	return filepath.Join(home, ".ssh", "id_ed25519")
}
