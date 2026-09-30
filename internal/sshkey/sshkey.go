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

	"nib/internal/atomicfile"
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
	// The refusal is O_EXCL's, not a Stat's. See the write below.
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
	// O_CREATE|O_EXCL, because this doc comment says "It refuses to overwrite an existing
	// file" and `os.Stat` + `os.WriteFile` does not enforce that: WriteFile is O_TRUNC, so
	// any Stat error other than ErrNotExist fell through to a truncating write — as did the
	// TOCTOU window between the two calls. The file in question is typically
	// ~/.ssh/id_ed25519: the user's SSH identity AND the key the vault's content key is
	// sealed to, so truncating it is permanent vault loss with no recovery path.
	//
	// This makes the contract the kernel's job rather than a check beside it, and the
	// existing TestGenerateRefusesOverwrite now tests the real mechanism.
	//
	// Durable as well (/pending 502): this is the only copy of the new private key, and the vault
	// sealed to it is written with an fsync — so without one here a power loss could keep the
	// vault and lose the key that opens it. atomicfile.CreateDurable is O_EXCL plus the syncs.
	if err := atomicfile.CreateDurable(privPath, pem.EncodeToMemory(block), 0o600); err != nil {
		if os.IsExist(err) {
			return "", os.ErrExist
		}
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

// PublicKeyLine returns the authorized_keys line for the private key at keyPath.
//
// **The private key is the authority, and a sibling ".pub" is only a convenience** (/pending 712
// R6-3). This used to return the ".pub" whenever one existed, never asking whether it belonged to
// the key beside it — and every caller seals to the answer: `vault.Migrate` seals the whole vault
// (the signing identity included) to it and overwrites the old one, enrol does the same, and
// `AddKey` adds a slot. A stale or mismatched `~/.ssh/id_*.pub` therefore produced a vault that
// opens for no key the user holds, with the only good copy gone.
//
// So the public half is derived from the key wherever the key yields it — a plain key, or an
// encrypted OpenSSH-format one, which carries its public half in cleartext — and a ".pub" is used:
//
//   - when it IS that key's public half: its line is returned, keeping the comment the user gave it;
//   - when the key cannot yield one (legacy PEM encryption, a key type ssh cannot parse, an
//     unreadable key file): as before, because it is then the only answer there is. That residue is
//     declared, not closed: nothing on this machine can check the pair without the passphrase.
//
// A ".pub" that names a DIFFERENT key is refused by name rather than silently overridden, so the
// user learns their files disagree before anything is sealed.
func PublicKeyLine(keyPath string) (string, error) {
	pubFile, pubErr := os.ReadFile(keyPath + ".pub")
	derived, derr := derivePublicKey(keyPath)
	if derr != nil {
		if pubErr == nil {
			return strings.TrimSpace(string(pubFile)), nil
		}
		return "", derr
	}
	if pubErr != nil {
		return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(derived))), nil
	}
	pk, _, _, _, perr := ssh.ParseAuthorizedKey(pubFile)
	if perr != nil || !bytes.Equal(pk.Marshal(), derived.Marshal()) {
		return "", ErrPublicKeyMismatch{KeyPath: keyPath}
	}
	return strings.TrimSpace(string(pubFile)), nil
}

// ErrPublicKeyMismatch refuses a ".pub" that is not the public half of the private key beside it.
type ErrPublicKeyMismatch struct{ KeyPath string }

func (e ErrPublicKeyMismatch) Error() string {
	return e.KeyPath + ".pub is not the public half of " + e.KeyPath + ", so nothing was sealed to " +
		"it; run 'ssh-keygen -y -f " + e.KeyPath + " > " + e.KeyPath + ".pub' to rewrite it from the key"
}

// derivePublicKey reads the public half out of the private key file itself.
func derivePublicKey(keyPath string) (ssh.PublicKey, error) {
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	raw, err := ssh.ParseRawPrivateKey(b)
	if err != nil {
		// An encrypted OpenSSH-format key carries its public half in cleartext, so
		// ssh surfaces it on the error — enough to derive the line without the
		// passphrase (which is only needed later, at unlock). Legacy PEM-encrypted
		// keys don't embed it, so point the user at ssh-keygen to materialize a .pub.
		var miss *ssh.PassphraseMissingError
		if errors.As(err, &miss) {
			if miss.PublicKey != nil {
				return miss.PublicKey, nil
			}
			return nil, errors.New("key is passphrase-protected and has no public (.pub) file; run 'ssh-keygen -y -f " + keyPath + "' to create one")
		}
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return nil, err
	}
	return signer.PublicKey(), nil
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
//
// It returns "" when the home directory is unknown, because there is no sensible
// default then. It used to return the bare name "id_ed25519", which is relative:
// the key landed beside whatever directory Nib was started in, the vault recorded
// that, and the next launch — from a different directory, which on Windows is
// simply a different way of opening the app — could not find it. An empty default
// makes the wizard ask instead of guessing wrong.
func DefaultNewKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "id_ed25519")
}
