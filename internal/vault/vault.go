// Package vault is Nib's encrypted local store. A single file holds the
// user's signing identity, image library, autofill profile, and recent files,
// encrypted at rest with a random content key.
//
// That content key is sealed ("wrapped") to the user's SSH public key via age
// and stored in a key slot; at startup it is unwrapped with the matching SSH
// private key, so the vault unlocks with no password. There is no password —
// losing the SSH key means the vault is unrecoverable, by design.
package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
	"nib/internal/sshkey"
)

// fileName is the vault's on-disk name within the app config directory.
const fileName = "vault.nib"

const keyLen = 32 // AES-256 content key

var (
	// ErrNotFound: no vault file at the given location.
	ErrNotFound = errors.New("no vault")
	// ErrKeyMissing: a vault exists but no enrolled SSH key could unlock it
	// (the private key file is missing, moved, or doesn't match).
	ErrKeyMissing = errors.New("ssh key unavailable")
	// ErrKeyLocked: the enrolled key file is present but passphrase-protected, so
	// promptless unlock can't proceed — the caller should prompt for a passphrase
	// and retry via OpenSSHWithPassphrase. Distinct from ErrKeyMissing so the UI
	// offers a passphrase prompt rather than "key not found".
	ErrKeyLocked = errors.New("ssh key is passphrase-protected")
	// ErrWrongPassphrase: a passphrase was supplied but didn't decrypt the key.
	ErrWrongPassphrase = errors.New("wrong passphrase")
	// ErrNeedsMigration: the vault is the old password format.
	ErrNeedsMigration = errors.New("vault needs migration")
	// ErrWrongPassword: migration was given the wrong old password.
	ErrWrongPassword = errors.New("wrong password")
	// ErrKeyExists: the public key is already an authorized key.
	ErrKeyExists = errors.New("key already authorized")
	// ErrNoSuchKey: no enrolled slot matches the public key.
	ErrNoSuchKey = errors.New("no such authorized key")
	// ErrLastKey: refused to remove the only authorized key (would orphan the vault).
	ErrLastKey = errors.New("cannot remove the only authorized key")
	// ErrCurrentKey: refused to remove the key this session unlocked with.
	ErrCurrentKey = errors.New("cannot remove the key in use this session")
	// ErrBadKey: the public-key line could not be parsed.
	ErrBadKey = errors.New("invalid public key")
	// ErrReadOnlyImage: refused to modify a built-in (binary-shipped) image.
	ErrReadOnlyImage = errors.New("built-in image is read-only")
)

// Slot seals the content key to one SSH key. Multiple slots (one per key/machine)
// can unlock the same vault; for now Nib enrolls a single key.
type Slot struct {
	PubKey  string `json:"pubKey"`  // authorized_keys line
	KeyPath string `json:"keyPath"` // private key path used to unwrap at startup
	Wrapped []byte `json:"wrapped"` // age-wrapped content key
}

// envelope is the on-disk JSON. v2 stores the content ciphertext plus SSH slots;
// the KDF field is retained only so an old v1 (password) vault still parses for
// one-time migration.
type envelope struct {
	Version int    `json:"version"`
	Nonce   []byte `json:"nonce"`
	Cipher  []byte `json:"cipher"`
	SSH     []Slot `json:"ssh,omitempty"`
	KDF     *kdf   `json:"kdf,omitempty"` // v1 only
}

// kdf is the v1 password key-derivation, kept for migration reads.
type kdf struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	Salt    []byte `json:"salt"`
}

func (k kdf) deriveKey(password string) []byte {
	return argon2.IDKey([]byte(password), k.Salt, k.Time, k.Memory, k.Threads, keyLen)
}

// Image is a stored library image (a signature, logo, etc.).
type Image struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	MIME string `json:"mime"` // "image/png" or "image/jpeg"
	Data []byte `json:"data"`
}

// Identity is the user's self-signed PDF-signing certificate and private key.
type Identity struct {
	CertPEM []byte `json:"cert"`
	KeyPEM  []byte `json:"key"`
}

// PinnedPeer is another Nib identity the user has pinned out-of-band by its
// SHA-256 SPKI fingerprint (see sign.Fingerprint), so that peer's signatures
// and co-signing sessions can be recognized. Label is a human name for it.
type PinnedPeer struct {
	Fingerprint []byte `json:"fingerprint"`
	Label       string `json:"label,omitempty"`
	// Ceremony is the id of the ceremony whose invitation created this pin, or "" for one
	// the user made themselves (D29).
	//
	// It exists so an invitation's pins can be taken away again. Consuming an invitation
	// pins every party on its roster — people the user has never met and may never meet
	// again — and without a scope those strangers stay in the peer list for good. A pin
	// the user promoted has no ceremony and survives every prune.
	Ceremony string `json:"ceremony,omitempty"`
}

// Settings holds the user's togglable UI preferences. Zero values are the
// defaults: an empty Appearance means the dark theme and a false
// DisableAutoUpdate means the startup update check runs — so an older vault that
// predates this struct keeps the prior behaviour with no migration.
//
// A vault written before v1.109.1 also carries a "toolbarStyle" key. It is ignored:
// encoding/json drops unknown fields, and the next Save writes the struct without
// it. Nothing is lost, because nothing ever read it — the menus/toolbar/both layout
// switch was validated here and in the settings route and was never built in the
// client, so no vault holds a value a user chose.
type Settings struct {
	Appearance            string   `json:"appearance,omitempty"`            // "dark" (default) | "light"
	DisableAutoUpdate     bool     `json:"disableAutoUpdate,omitempty"`     // skip the startup update check
	RecentHighlightColors []string `json:"recentHighlightColors,omitempty"` // last-used highlight colors, newest first (#rrggbb)
}

// ExternalSigner is an imported PKCS#12 signing identity (the user's own /
// CA-issued certificate) used ONLY for solo Finalize signing — never as the peer
// identity (which stays the native self-signed key). The .p12 is stored exactly
// as imported (still passphrase-encrypted); the passphrase is supplied per sign
// and never persisted, so the private key is never at rest in decrypted form.
// CertPEM/ChainPEM hold the public certificate and chain, captured at import for
// display.
type ExternalSigner struct {
	P12      []byte `json:"p12"`
	CertPEM  []byte `json:"cert"`
	ChainPEM []byte `json:"chain,omitempty"`
}

// Contents is the decrypted vault payload.
type Contents struct {
	Images         []Image           `json:"images,omitempty"`
	Recent         []string          `json:"recent,omitempty"` // recent file paths, newest first
	Identity       *Identity         `json:"identity,omitempty"`
	ExternalSigner *ExternalSigner   `json:"externalSigner,omitempty"`
	Profile        map[string]string `json:"profile,omitempty"` // autofill field name -> value
	Settings       Settings          `json:"settings,omitempty"`
	PinnedPeers    []PinnedPeer      `json:"pinnedPeers,omitempty"`
}

const maxRecent = 10

// Vault is an opened (decrypted) store. It holds the content key in memory so it
// can re-encrypt on save. A single process serves it to concurrent HTTP
// handlers, so mu guards every access to the mutable in-memory state (contents
// and ssh); accessors return copies so callers never share a live slice or map.
type Vault struct {
	mu       sync.Mutex
	path     string
	key      []byte // content key
	ssh      []Slot
	current  string // PubKey of the slot that unlocked this session
	contents Contents
	// builtinImages are signatures shipped in the binary, decrypted this session
	// from a built-in key. They appear in the library read-only and are never
	// persisted (Save writes only contents).
	builtinImages []Image
}

// Path returns the vault file location within dir.
func Path(dir string) string { return filepath.Join(dir, fileName) }

// DefaultDir is where Nib keeps its vault, per-OS: ~/.config/nib,
// ~/Library/Application Support/nib, or %AppData%\nib.
func DefaultDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "nib")
}

// Exists reports whether a vault file is present in dir.
func Exists(dir string) bool {
	_, err := os.Stat(Path(dir))
	return err == nil
}

// Slots returns the enrolled key slots (for status display) without unlocking.
func Slots(dir string) ([]Slot, error) {
	env, err := readEnvelope(dir)
	if err != nil {
		return nil, err
	}
	return env.SSH, nil
}

// NeedsMigration reports whether the vault is the old password format.
func NeedsMigration(dir string) bool {
	env, err := readEnvelope(dir)
	return err == nil && (env.Version < 2 || env.KDF != nil)
}

// Create initialises a new vault sealed to the given SSH public key, recording
// keyPath as where to find the private key for startup unlock.
func Create(dir, pubLine, keyPath string) (*Vault, error) {
	if Exists(dir) {
		return nil, errors.New("vault already exists")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	v, err := newSealed(Path(dir), pubLine, keyPath, Contents{})
	if err != nil {
		return nil, err
	}
	return v, v.Save()
}

// OpenSSH unlocks the vault by trying each enrolled key slot's private key.
func OpenSSH(dir string) (*Vault, error) { return openSSH(dir, nil) }

// OpenSSHWithPassphrase unlocks a vault whose enrolled key is passphrase-protected,
// decrypting that key in memory with passphrase. The key file stays encrypted on
// disk. Returns ErrWrongPassphrase if the passphrase doesn't fit.
func OpenSSHWithPassphrase(dir string, passphrase []byte) (*Vault, error) {
	return openSSH(dir, passphrase)
}

// OpenSSHAt unlocks the vault using a private key the user has POINTED AT, and — on
// success — rewrites the matching slot's recorded KeyPath to that location.
//
// **This is the recovery half of the key-path fix.** v1.102.2 stopped new enrollments
// recording a path that will not survive the next launch (a `~`-prefixed or relative one,
// anchored to whatever the process CWD happened to be), and did nothing for a vault that
// already holds one. Such a user lands in `key-missing`, whose only offered action was
// "restore that key file, then retry" — pointing at a path that may be meaningless, like
// a directory literally named `~`. The vault is not lost: it is still sealed to that
// key's PUBLIC half, so if the private key can be found anywhere on disk it unwraps.
// The gap was purely that there was no way to say where.
//
// The rewrite is what makes it a repair rather than a one-off unlock — otherwise the next
// launch lands in `key-missing` again and the user re-types the path forever.
//
// Note what this cannot do: if the key file is genuinely gone, nothing recovers the
// vault. That is the documented "lose every authorized key and the vault is
// unrecoverable" property, not a bug, and this function reports ErrKeyMissing for it.
func OpenSSHAt(dir, keyPath string, passphrase []byte) (*Vault, error) {
	env, err := readEnvelope(dir)
	if err != nil {
		return nil, err
	}
	if env.Version < 2 || len(env.SSH) == 0 {
		return nil, ErrNeedsMigration
	}
	var slotErr error
	for i, slot := range env.SSH {
		// The SUPPLIED path only. unwrapSlot would also sweep ~/.ssh and try the slot's
		// own recorded path, and either would make "the key you pointed at opened it"
		// untrue — the KeyPath rewrite below would then record a path that did not do
		// the unwrapping.
		key, uerr := sshkey.Unwrap(slot.Wrapped, keyPath, slot.PubKey, passphrase)
		if uerr != nil {
			if slotErr == nil || errors.Is(uerr, sshkey.ErrWrongPassphrase) || errors.Is(uerr, sshkey.ErrPassphraseRequired) {
				slotErr = uerr
			}
			continue
		}
		plain, derr := decrypt(key, env.Nonce, env.Cipher)
		if derr != nil {
			zero(key)
			continue
		}
		var c Contents
		derr = json.Unmarshal(plain, &c)
		zero(plain)
		if derr != nil {
			zero(key)
			return nil, fmt.Errorf("corrupt vault contents: %w", derr)
		}
		env.SSH[i].KeyPath = keyPath // the repair
		v := &Vault{path: Path(dir), key: key, ssh: env.SSH, current: slot.PubKey, contents: c, builtinImages: loadBuiltinSignatures()}
		// Saved immediately: an unlock that did not persist the new path leaves the user
		// in key-missing on the next launch, having been told it was fixed.
		if serr := v.Save(); serr != nil {
			return nil, serr
		}
		return v, nil
	}
	switch {
	case errors.Is(slotErr, sshkey.ErrWrongPassphrase):
		return nil, ErrWrongPassphrase
	case errors.Is(slotErr, sshkey.ErrPassphraseRequired):
		return nil, ErrKeyLocked
	default:
		return nil, ErrKeyMissing
	}
}

// openSSH unlocks via the enrolled SSH slots. passphrase nil is the promptless
// path (unencrypted key, with the ~/.ssh candidate fallback); a non-nil passphrase
// targets the enrolled key path only and decrypts it in memory. When no slot
// unlocks, the most actionable reason wins: ErrWrongPassphrase, then ErrKeyLocked
// (present but encrypted, prompt for a passphrase), else ErrKeyMissing.
func openSSH(dir string, passphrase []byte) (*Vault, error) {
	env, err := readEnvelope(dir)
	if err != nil {
		return nil, err
	}
	if env.Version < 2 || len(env.SSH) == 0 {
		return nil, ErrNeedsMigration
	}
	var slotErr error
	for _, slot := range env.SSH {
		key, ok, uerr := unwrapSlot(slot, passphrase)
		if !ok {
			if slotErr == nil || errors.Is(uerr, sshkey.ErrWrongPassphrase) || errors.Is(uerr, sshkey.ErrPassphraseRequired) {
				slotErr = uerr
			}
			continue // no available private key for this slot — try the next
		}
		plain, err := decrypt(key, env.Nonce, env.Cipher)
		if err != nil {
			zero(key) // this slot's unwrapped content key is unused — scrub it
			continue
		}
		var c Contents
		err = json.Unmarshal(plain, &c)
		zero(plain) // scrub the decrypted plaintext (carries the signing key), as Migrate does
		if err != nil {
			zero(key) // discard path: the unwrapped content key won't be retained — scrub it too
			return nil, fmt.Errorf("corrupt vault contents: %w", err)
		}
		return &Vault{path: Path(dir), key: key, ssh: env.SSH, current: slot.PubKey, contents: c, builtinImages: loadBuiltinSignatures()}, nil
	}
	switch {
	case errors.Is(slotErr, sshkey.ErrWrongPassphrase):
		return nil, ErrWrongPassphrase
	case errors.Is(slotErr, sshkey.ErrPassphraseRequired):
		return nil, ErrKeyLocked
	default:
		return nil, ErrKeyMissing
	}
}

// Migrate converts an old password vault to an SSH-sealed one: it decrypts with
// the old password and re-keys to the given SSH key.
func Migrate(dir, password, pubLine, keyPath string) (*Vault, error) {
	env, err := readEnvelope(dir)
	if err != nil {
		return nil, err
	}
	if env.KDF == nil {
		return nil, errors.New("not a password vault")
	}
	// Best-effort: wipe the derived key and decrypted plaintext (which carries the
	// signing private key) once we're done with each, to shorten their residency.
	key := env.KDF.deriveKey(password)
	plain, err := decrypt(key, env.Nonce, env.Cipher)
	zero(key)
	if err != nil {
		return nil, ErrWrongPassword
	}
	var c Contents
	err = json.Unmarshal(plain, &c)
	zero(plain)
	if err != nil {
		return nil, fmt.Errorf("corrupt vault contents: %w", err)
	}
	v, err := newSealed(Path(dir), pubLine, keyPath, c)
	if err != nil {
		return nil, err
	}
	return v, v.Save()
}

// unwrapSlot recovers the content key for a slot. It tries the slot's recorded
// private-key path first, then the local ~/.ssh keys — so a builtin slot, which
// records no path, still unlocks on a machine holding its private half. The
// first key that decrypts wins.
func unwrapSlot(slot Slot, passphrase []byte) (key []byte, ok bool, err error) {
	tried := map[string]bool{}
	try := func(p string) bool {
		if p == "" || tried[p] {
			return false
		}
		tried[p] = true
		k, e := sshkey.Unwrap(slot.Wrapped, p, slot.PubKey, passphrase)
		if e == nil {
			key, ok = k, true
			return true
		}
		// Keep the most actionable failure: a passphrase-related error beats a
		// generic "key not found", so the caller can prompt or report precisely.
		if err == nil || errors.Is(e, sshkey.ErrPassphraseRequired) || errors.Is(e, sshkey.ErrWrongPassphrase) {
			err = e
		}
		return false
	}
	if try(slot.KeyPath) {
		return key, true, nil
	}
	// The ~/.ssh candidate sweep is a promptless convenience; a user-supplied
	// passphrase is for the enrolled key path only, so don't apply it to others.
	if passphrase == nil {
		for _, p := range sshkey.Candidates() {
			if try(p) {
				return key, true, nil
			}
		}
	}
	return nil, false, err
}

// newSealed builds a Vault with a fresh content key sealed to pubLine and to
// each builtin key (so it also opens on the machines holding those keys).
func newSealed(path, pubLine, keyPath string, c Contents) (*Vault, error) {
	key := make([]byte, keyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	wrapped, err := sshkey.Wrap(key, pubLine)
	if err != nil {
		return nil, err
	}
	slots := sealBuiltins(key, []Slot{{PubKey: pubLine, KeyPath: keyPath, Wrapped: wrapped}})
	return &Vault{
		path:     path,
		key:      key,
		ssh:      slots,
		current:  pubLine,
		contents: c,
	}, nil
}

// --- authorized keys ----------------------------------------------------------

// KeyInfo describes one enrolled authorized key for display and management. It
// omits the wrapped content key — only metadata is exposed.
type KeyInfo struct {
	PubKey  string `json:"pubKey"`  // authorized_keys line
	KeyPath string `json:"keyPath"` // private key path used to unwrap at startup
	Current bool   `json:"current"` // the key this session unlocked with
}

// Keys lists the enrolled authorized keys, flagging the one in use this session.
func (v *Vault) Keys() []KeyInfo {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]KeyInfo, 0, len(v.ssh))
	cur := keyID(v.current)
	for _, s := range v.ssh {
		out = append(out, KeyInfo{PubKey: s.PubKey, KeyPath: s.KeyPath, Current: keyID(s.PubKey) == cur})
	}
	return out
}

// AddKey authorizes another SSH public key to unlock this vault: it seals the
// content key to pubLine and records keyPath as where that key's private half
// lives on the machine that holds it. The key material (type+blob, ignoring any
// comment) must not already be enrolled.
func (v *Vault) AddKey(pubLine, keyPath string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	pubLine = strings.TrimSpace(pubLine)
	id := keyID(pubLine)
	if id == "" {
		return ErrBadKey
	}
	for _, s := range v.ssh {
		if keyID(s.PubKey) == id {
			return ErrKeyExists
		}
	}
	wrapped, err := sshkey.Wrap(v.key, pubLine)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadKey, err)
	}
	v.ssh = append(v.ssh, Slot{PubKey: pubLine, KeyPath: keyPath, Wrapped: wrapped})
	return v.save()
}

// RemoveKey drops the slot for pubLine. It refuses to remove the only enrolled
// key (which would orphan the vault) or the key this session unlocked with.
func (v *Vault) RemoveKey(pubLine string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	id := keyID(pubLine)
	idx := -1
	for i, s := range v.ssh {
		if keyID(s.PubKey) == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNoSuchKey
	}
	if len(v.ssh) == 1 {
		return ErrLastKey
	}
	if keyID(v.ssh[idx].PubKey) == keyID(v.current) {
		return ErrCurrentKey
	}
	v.ssh = append(v.ssh[:idx], v.ssh[idx+1:]...)
	return v.save()
}

// keyID returns the comparable identity of an authorized_keys line — its
// algorithm and key blob — ignoring any trailing comment, so the same key with
// a different comment compares equal.
func keyID(line string) string {
	f := strings.Fields(line)
	if len(f) < 2 {
		return ""
	}
	return f[0] + " " + f[1]
}

// Validate reports whether raw is a vault file this machine could actually open,
// for vetting a backup before importing it.
//
// It checks openability and not merely shape, because of what the caller does next:
// an import OVERWRITES the live vault, in place, with no prior copy. A vault that
// cannot be opened here is therefore not a recoverable mistake — it destroys the
// sealed content key and with it the PDF-signing identity every pinned peer has
// fingerprinted. That the vault is unrecoverable without its key is a deliberate
// property of this design; reaching that state by picking the wrong file in a
// dialog is not.
//
// The shape checks alone passed all three realistic mistakes: a truncated backup
// with an intact JSON prefix, someone else's vault, and an old v1 password vault
// (KDF != nil was never rejected).
//
// A slot that is present but whose private key is passphrase-protected COUNTS AS
// OPENABLE. This is the case that decides the whole shape of the check: that key
// is on this machine and the backup is genuinely the user's, we simply cannot
// prove it without prompting. Refusing it would turn a data-loss bug into a
// can't-restore-my-own-backup bug, which is the same harm wearing the opposite
// sign. So the bar is "at least one slot is a candidate on this machine", not
// "at least one slot opens right now".
func Validate(raw []byte) error {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("corrupt: %w", err)
	}
	if env.Version == 0 || len(env.Cipher) == 0 || len(env.Nonce) == 0 {
		return errors.New("not a vault backup")
	}
	if env.Version < 2 || len(env.SSH) == 0 {
		// A v1 password vault parses perfectly and has no key slots at all, so
		// importing one would replace a working vault with a file this build has no
		// path to open. Migration is a separate, deliberate route (see Migrate).
		return errors.New("this backup predates SSH-key sealing and cannot be imported directly")
	}

	var locked bool
	for _, slot := range env.SSH {
		key, ok, uerr := unwrapSlot(slot, nil)
		if !ok {
			// Present-but-locked is a candidate; no key at all is not.
			if errors.Is(uerr, sshkey.ErrPassphraseRequired) || errors.Is(uerr, sshkey.ErrWrongPassphrase) {
				locked = true
			}
			continue
		}
		// The slot opened. Prove the ciphertext it guards opens too, so a corrupt
		// or truncated body is refused here rather than after it has replaced the
		// only good copy.
		plain, derr := decrypt(key, env.Nonce, env.Cipher)
		zero(key)
		if derr != nil {
			return errors.New("backup is corrupt: its contents do not decrypt")
		}
		var c Contents
		uerr = json.Unmarshal(plain, &c)
		zero(plain)
		if uerr != nil {
			return errors.New("backup is corrupt: its contents do not parse")
		}
		return nil
	}
	if locked {
		return nil
	}
	return errors.New("this backup is sealed to a key this machine does not have")
}

// --- contents accessors -------------------------------------------------------

// Images returns a copy of the stored library images (including their data).
func (v *Vault) Images() []Image {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]Image(nil), v.contents.Images...)
}

// BuiltinImages returns the binary-shipped signatures decrypted this session
// (empty unless a built-in key was available). They're read-only.
func (v *Vault) BuiltinImages() []Image {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]Image(nil), v.builtinImages...)
}

// Image returns the image with the given id, from the stored library or the
// built-in signatures.
func (v *Vault) Image(id string) (Image, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, img := range v.contents.Images {
		if img.ID == id {
			return img, true
		}
	}
	for _, img := range v.builtinImages {
		if img.ID == id {
			return img, true
		}
	}
	return Image{}, false
}

// AddImage stores a new image and persists the vault.
func (v *Vault) AddImage(name, mime string, data []byte) (Image, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	img := Image{ID: newID(), Name: name, MIME: mime, Data: data}
	v.contents.Images = append(v.contents.Images, img)
	return img, v.save()
}

// DeleteImage removes the image with the given id and persists the vault. A
// built-in (binary-shipped) image can't be deleted.
func (v *Vault) DeleteImage(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, img := range v.builtinImages {
		if img.ID == id {
			return ErrReadOnlyImage
		}
	}
	out := v.contents.Images[:0]
	for _, img := range v.contents.Images {
		if img.ID != id {
			out = append(out, img)
		}
	}
	v.contents.Images = out
	return v.save()
}

// Identity returns the stored signing identity; ok is false if none exists yet.
func (v *Vault) Identity() (certPEM, keyPEM []byte, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.contents.Identity == nil {
		return nil, nil, false
	}
	id := v.contents.Identity
	return append([]byte(nil), id.CertPEM...), append([]byte(nil), id.KeyPEM...), true
}

// SetIdentity stores the signing identity and persists the vault.
func (v *Vault) SetIdentity(certPEM, keyPEM []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.contents.Identity = &Identity{CertPEM: certPEM, KeyPEM: keyPEM}
	return v.save()
}

// ExternalSigner returns a copy of the imported PKCS#12 signing identity, if any.
func (v *Vault) ExternalSigner() (*ExternalSigner, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.contents.ExternalSigner == nil {
		return nil, false
	}
	e := v.contents.ExternalSigner
	return &ExternalSigner{
		P12:      append([]byte(nil), e.P12...),
		CertPEM:  append([]byte(nil), e.CertPEM...),
		ChainPEM: append([]byte(nil), e.ChainPEM...),
	}, true
}

// SetExternalSigner stores the imported PKCS#12 bundle (as imported) and its
// captured public certificate/chain, and persists the vault.
func (v *Vault) SetExternalSigner(p12, certPEM, chainPEM []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.contents.ExternalSigner = &ExternalSigner{P12: p12, CertPEM: certPEM, ChainPEM: chainPEM}
	return v.save()
}

// ClearExternalSigner removes the imported signing identity and persists.
func (v *Vault) ClearExternalSigner() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.contents.ExternalSigner = nil
	return v.save()
}

// PinnedPeers returns a copy of the pinned-peer list.
func (v *Vault) PinnedPeers() []PinnedPeer {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]PinnedPeer, len(v.contents.PinnedPeers))
	for i, p := range v.contents.PinnedPeers {
		// Ceremony is copied, and its absence used to be a live trap: this accessor
		// rebuilt each pin from Fingerprint and Label only, so every caller outside
		// this package saw Ceremony == "" on ALL pins and could not tell a
		// ceremony-scoped pin (D29) from a user pin. The lifecycle worked — addPinned
		// and PruneCeremonyPeers read v.contents directly — which is exactly what
		// made it invisible: a public field on a public struct that was always the
		// zero value, with no test able to fail for it.
		out[i] = PinnedPeer{
			Fingerprint: append([]byte(nil), p.Fingerprint...),
			Label:       p.Label,
			Ceremony:    p.Ceremony,
		}
	}
	return out
}

// AddPinnedPeer pins a peer by fingerprint and label and persists. Re-pinning an
// existing fingerprint updates its label rather than duplicating it.
func (v *Vault) AddPinnedPeer(fingerprint []byte, label string) error {
	return v.addPinned(fingerprint, label, "")
}

// AddCeremonyPeer pins a peer for the duration of one ceremony (D29).
//
// **An existing pin is never downgraded to a ceremony pin.** If the user already pinned
// this peer themselves, the pin stays theirs and survives the prune — a ceremony must not
// be able to take away a relationship the user established, and an invitation naming
// someone already in the peer list is the ordinary case, not an attack.
func (v *Vault) AddCeremonyPeer(fingerprint []byte, label, ceremony string) error {
	return v.addPinned(fingerprint, label, ceremony)
}

func (v *Vault) addPinned(fingerprint []byte, label, ceremony string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.contents.PinnedPeers {
		if bytes.Equal(v.contents.PinnedPeers[i].Fingerprint, fingerprint) {
			v.contents.PinnedPeers[i].Label = label
			// Promotion is one-way: a user pin stays a user pin, and a ceremony pin becomes
			// a user pin the moment the user pins it themselves.
			if ceremony == "" {
				v.contents.PinnedPeers[i].Ceremony = ""
			}
			return v.save()
		}
	}
	v.contents.PinnedPeers = append(v.contents.PinnedPeers, PinnedPeer{
		Fingerprint: append([]byte(nil), fingerprint...),
		Label:       label,
		Ceremony:    ceremony,
	})
	return v.save()
}

// PruneCeremonyPeers removes every pin created by the named ceremony, leaving pins the
// user made themselves (D29). Returns how many were removed.
func (v *Vault) PruneCeremonyPeers(ceremony string) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ceremony == "" {
		// An empty ceremony id would match every user pin and delete the lot. Refusing is
		// the only safe reading: there is no ceremony called "".
		return 0, errors.New("prune needs a ceremony id")
	}
	var kept []PinnedPeer
	n := 0
	for _, p := range v.contents.PinnedPeers {
		if p.Ceremony == ceremony {
			n++
			continue
		}
		kept = append(kept, p)
	}
	v.contents.PinnedPeers = kept
	return n, v.save()
}

// RemovePinnedPeer unpins the peer with the given fingerprint (no-op if absent).
func (v *Vault) RemovePinnedPeer(fingerprint []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	var kept []PinnedPeer
	for _, p := range v.contents.PinnedPeers {
		if !bytes.Equal(p.Fingerprint, fingerprint) {
			kept = append(kept, p)
		}
	}
	v.contents.PinnedPeers = kept
	return v.save()
}

// Profile returns a copy of the autofill field name -> value map (never nil).
func (v *Vault) Profile() map[string]string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make(map[string]string, len(v.contents.Profile))
	for k, val := range v.contents.Profile {
		out[k] = val
	}
	return out
}

// SetProfile replaces the autofill profile and persists the vault. The map is
// copied so a caller that keeps mutating its own copy can't race the vault.
func (v *Vault) SetProfile(p map[string]string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	cp := make(map[string]string, len(p))
	for k, val := range p {
		cp[k] = val
	}
	v.contents.Profile = cp
	return v.save()
}

// Settings returns the user's UI preferences, with defaults filled in for an
// older vault that has none stored.
func (v *Vault) Settings() Settings {
	v.mu.Lock()
	defer v.mu.Unlock()
	s := v.contents.Settings
	if s.Appearance == "" {
		s.Appearance = "dark"
	}
	return s
}

// SetSettings replaces the stored UI preferences and persists the vault.
func (v *Vault) SetSettings(s Settings) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.contents.Settings = s
	return v.save()
}

// Recent returns a copy of recently opened file paths, newest first.
func (v *Vault) Recent() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.contents.Recent...)
}

// AddRecent records path as the most recent file (deduped, capped) and persists.
func (v *Vault) AddRecent(path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]string, 0, maxRecent)
	out = append(out, path)
	for _, p := range v.contents.Recent {
		if p != path {
			out = append(out, p)
		}
		if len(out) >= maxRecent {
			break
		}
	}
	v.contents.Recent = out
	return v.save()
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Save encrypts the current contents with the content key and writes the vault
// atomically, keeping the existing key slots.
func (v *Vault) Save() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.save()
}

// save is Save without locking, for callers that already hold v.mu.
func (v *Vault) save() error {
	plain, err := json.Marshal(v.contents)
	if err != nil {
		return err
	}
	nonce, ct, err := encrypt(v.key, plain)
	if err != nil {
		return err
	}
	out, err := json.Marshal(envelope{Version: 2, Nonce: nonce, Cipher: ct, SSH: v.ssh})
	if err != nil {
		return err
	}
	return writeFileAtomic(v.path, out)
}

func readEnvelope(dir string) (*envelope, error) {
	raw, err := os.ReadFile(Path(dir))
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("corrupt vault: %w", err)
	}
	return &env, nil
}

// --- AES-256-GCM helpers ------------------------------------------------------

// zero overwrites b — best-effort scrubbing of secret material before GC.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func encrypt(key, plaintext []byte) (nonce, ciphertext []byte, err error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, nil), nil
}

func decrypt(key, nonce, ciphertext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil) // errors on wrong key / tampering
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// writeFileAtomic writes via a temp file + fsync + rename (+ parent-dir fsync)
// so an interrupted write can't corrupt the vault — the syncs make the rename
// durable, not merely atomic: without them a crash right after the rename can
// leave a stale or truncated file on disk. The vault holds keys; it gets the
// full-durability treatment.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vault-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// Persist the directory entry so the rename itself survives a crash.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil // rename succeeded; dir-sync is best-effort belt-and-suspenders
	}
	defer dir.Close()
	_ = dir.Sync()
	return nil
}
