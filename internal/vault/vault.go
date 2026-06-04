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

// Contents is the decrypted vault payload.
type Contents struct {
	Images   []Image           `json:"images,omitempty"`
	Recent   []string          `json:"recent,omitempty"` // recent file paths, newest first
	Identity *Identity         `json:"identity,omitempty"`
	Profile  map[string]string `json:"profile,omitempty"` // autofill field name -> value
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
func OpenSSH(dir string) (*Vault, error) {
	env, err := readEnvelope(dir)
	if err != nil {
		return nil, err
	}
	if env.Version < 2 || len(env.SSH) == 0 {
		return nil, ErrNeedsMigration
	}
	for _, slot := range env.SSH {
		key, ok := unwrapSlot(slot)
		if !ok {
			continue // no available private key for this slot — try the next
		}
		plain, err := decrypt(key, env.Nonce, env.Cipher)
		if err != nil {
			continue
		}
		var c Contents
		if err := json.Unmarshal(plain, &c); err != nil {
			return nil, fmt.Errorf("corrupt vault contents: %w", err)
		}
		return &Vault{path: Path(dir), key: key, ssh: env.SSH, current: slot.PubKey, contents: c, builtinImages: loadBuiltinSignatures()}, nil
	}
	return nil, ErrKeyMissing
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
	plain, err := decrypt(env.KDF.deriveKey(password), env.Nonce, env.Cipher)
	if err != nil {
		return nil, ErrWrongPassword
	}
	var c Contents
	if err := json.Unmarshal(plain, &c); err != nil {
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
func unwrapSlot(slot Slot) ([]byte, bool) {
	tried := map[string]bool{}
	try := func(p string) ([]byte, bool) {
		if p == "" || tried[p] {
			return nil, false
		}
		tried[p] = true
		key, err := sshkey.Unwrap(slot.Wrapped, p)
		return key, err == nil
	}
	if key, ok := try(slot.KeyPath); ok {
		return key, true
	}
	for _, p := range sshkey.Candidates() {
		if key, ok := try(p); ok {
			return key, true
		}
	}
	return nil, false
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

// Validate reports whether raw is a well-formed vault file, for vetting a backup
// before importing it.
func Validate(raw []byte) error {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("corrupt: %w", err)
	}
	if env.Version == 0 || len(env.Cipher) == 0 || len(env.Nonce) == 0 {
		return errors.New("not a vault backup")
	}
	return nil
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
func (v *Vault) BuiltinImages() []Image { return v.builtinImages }

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
	return v.contents.Identity.CertPEM, v.contents.Identity.KeyPEM, true
}

// SetIdentity stores the signing identity and persists the vault.
func (v *Vault) SetIdentity(certPEM, keyPEM []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.contents.Identity = &Identity{CertPEM: certPEM, KeyPEM: keyPEM}
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

// writeFileAtomic writes via a temp file + rename so an interrupted write can't
// corrupt the vault.
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
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
