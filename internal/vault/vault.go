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
	"nib/internal/atomicfile"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"nib/internal/sshkey"

	"golang.org/x/crypto/argon2"
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
	// Ceremonies are the ids of the ceremonies that created this pin, or empty for one the
	// user made themselves (D29).
	//
	// It exists so an invitation's pins can be taken away again. Consuming an invitation pins
	// the parties it needs — people the user has never met and may never meet again — and
	// without a scope those strangers stay in the peer list for good. A pin the user made has
	// no ceremony and survives every prune.
	//
	// **A SET, not one id, and that is a measured fix rather than generality (P07.S02b).** It
	// was a single string, and a second ceremony naming a party the first had already pinned
	// left the pin scoped to the FIRST — so finishing or declining ceremony A unpinned a peer
	// ceremony B still needed, and B's next arm failed with "that peer isn't pinned". Measured
	// before the change: two AddCeremonyPeer calls for one fingerprint left `ceremony-A`, and
	// pruning A removed the pin outright. The same counterparty across two matters is the
	// ordinary case for this product's user, not an exotic one.
	//
	// **No migration, and that is a fact about the field rather than an assumption.**
	// `AddCeremonyPeer` had zero production callers until this slice, so no vault anywhere
	// carries a ceremony-scoped pin and there is nothing to carry forward. `contentsVersion`
	// moves so an older build refuses the payload rather than silently dropping the key — which
	// would turn every ceremony pin into a permanent one, the exact harm D29 exists to prevent.
	Ceremonies []string `json:"ceremonies,omitempty"`
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

// CeremonySecret is one party's invitation secret, held by the CONVENER.
//
// # Why the convener holds N-1 secrets at rest at all
//
// A recipient's invitation travels in the arm request and is never persisted —
// `internal/server`'s sessionArmRequest says so at the line, and that reasoning is
// unchanged. This is the other side: the convener MINTS every party's secret and is the only
// one who can re-issue an invitation. Before P07.S02a those secrets existed in exactly one
// HTTP response, so closing the tab made the ceremony unrecoverable rather than merely
// stalled. They go in the vault, sealed to the user's SSH key, because that is where the
// signing key and the pinned peers already live and D29 puts key material there rather than
// beside ordinary files in ~/nib.
type CeremonySecret struct {
	// Ceremony is the record's 32-hex id.
	Ceremony string `json:"ceremony"`
	// Fingerprint is the party's SHA-256 SPKI as RAW BYTES.
	//
	// Bytes, not the hex string, and it matters: a map or a string key reintroduces the
	// case predicate that P07.S02 spent a slice making unrepresentable, at a NEW site where
	// nothing compares it against a signed copy. `bytes.Equal` has one answer.
	Fingerprint []byte `json:"fingerprint"`
	// Secret is the 32 bytes the rendezvous, the record encryption and the channel binding
	// are all derived from.
	Secret []byte `json:"secret"`
}

// Contents is the decrypted vault payload.
type Contents struct {
	// Version is what this build wrote into the PAYLOAD, distinct from the envelope's.
	//
	// **Added 2026-08-24 (P07.S02a), and the envelope's gate does not reach here.**
	// checkEnvelopeVersion refuses a vault whose ENVELOPE is newer, and its comment explains
	// exactly why that matters: encoding/json discards unknown keys, so an older build that
	// opens and re-saves silently drops everything it does not know. That reasoning applies
	// word for word to Contents, which had no version at all — measured: a payload carrying a
	// `ceremonySecrets` key opens on a build that lacks the field, and one AddRecent (i.e.
	// opening any PDF) rewrites the file without it.
	//
	// Harmless while the only unknown fields were things nobody read. Not harmless now: the
	// secrets below are the only copy, and losing them makes a ceremony unrecoverable —
	// which is the precise state S02a exists to prevent.
	Version        int               `json:"version,omitempty"`
	Images         []Image           `json:"images,omitempty"`
	Recent         []string          `json:"recent,omitempty"` // recent file paths, newest first
	Identity       *Identity         `json:"identity,omitempty"`
	ExternalSigner *ExternalSigner   `json:"externalSigner,omitempty"`
	Profile        map[string]string `json:"profile,omitempty"` // autofill field name -> value
	Settings       Settings          `json:"settings,omitempty"`
	PinnedPeers    []PinnedPeer      `json:"pinnedPeers,omitempty"`
	// CeremonySecrets is a flat slice, mirroring PinnedPeers, not a map.
	//
	// A map would have to be keyed by a hex STRING (a []byte cannot be a map key), which is
	// the case predicate again; and a prune over a map-of-maps is a delete plus an
	// empty-parent sweep, two rules where a slice has one loop.
	CeremonySecrets []CeremonySecret `json:"ceremonySecrets,omitempty"`
}

// contentsVersion is what this build writes into Contents, and the highest it will open.
// contentsVersion is the vault payload's format version (D32).
//
// **2 as of P07.S02b:** `PinnedPeer.Ceremony` became `PinnedPeer.Ceremonies`, a set, so a pin two
// ceremonies both need survives either one ending. An older build would drop the unknown
// `ceremonies` key on its next ordinary save and leave every ceremony pin PERMANENT — the harm
// D29 exists to prevent — so the version moves and `checkContentsVersion` refuses rather than
// silently rewriting. Nothing is migrated because there is nothing to migrate: `AddCeremonyPeer`
// had no production caller before that slice, so no vault carries a scoped pin.
const contentsVersion = 2

// checkContentsVersion refuses a payload written by a NEWER Nib, for checkEnvelopeVersion's
// reason applied one layer in. A zero is a vault written before the field existed and is
// read as version 0, not refused: those payloads are this build's own.
func checkContentsVersion(v int) error {
	if v > contentsVersion {
		return fmt.Errorf("this vault's contents were written by a newer version of Nib "+
			"(payload format %d, this build understands %d) — update Nib rather than opening "+
			"it here, or anything the newer version stored will be dropped the next time "+
			"Nib saves", v, contentsVersion)
	}
	return nil
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
	// **The Exists check above is the friendly answer; THIS is the guarantee.** Exists →
	// Save is check-then-act, and Save writes through writeFileAtomic, whose rename
	// CLOBBERS. Two processes reaching first-run setup together both see no vault, both
	// seal a fresh content key, and the second rename silently replaces the first — so a
	// user who has already been shown a signing identity loses the private half of it, with
	// no error anywhere. `GET /api/status` can run AutoSetup, so the trigger is a page load.
	//
	// O_EXCL makes the file's creation the thing that races, and the kernel settles it. The
	// placeholder is zero bytes; Save overwrites it a moment later through the atomic path,
	// which is what gives the real contents their durability.
	f, err := os.OpenFile(Path(dir), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, errors.New("vault already exists")
		}
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	v, err := newSealed(Path(dir), pubLine, keyPath, Contents{})
	if err != nil {
		// The placeholder is an empty file that readEnvelope would call corrupt, and
		// Exists would call a vault. Nothing here has succeeded, so it must not survive.
		_ = os.Remove(Path(dir))
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
		c, derr = decodeContents(plain)
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
		c, err = decodeContents(plain)
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
	c, err = decodeContents(plain)
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
		// A content key that will never seal anything, but the package zeroes secret
		// material on every other path and an exception nobody wrote down is how the
		// discipline erodes.
		zero(key)
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
	// **The CEILING, through the same door `Open` uses (/pending 287).**
	//
	// This had a floor and no ceiling, so a vault stamped with a future format passed validation,
	// was written over the user's own by the import handler, and was then refused by `Open` — the
	// previous vault gone and the new one unopenable until they update. The handler's own doc
	// calls this function *"the only thing standing between a mis-picked file and the permanent
	// loss of the signing identity"*, and it let through exactly the file the reader would not
	// open, AFTER the overwrite.
	//
	// `checkEnvelopeVersion` is the door and it already existed; this caller simply did not use
	// it, which is `checkEnvelopeVersion`'s own recorded history repeating at the one site the
	// original fix did not reach. ADR-009: a rule gets one door, and every site calls it.
	//
	// It is checked BEFORE the floor below, so a future-format vault is refused as a future format
	// rather than as one that "predates SSH-key sealing" — the second sentence would send the user
	// looking for a migration that is not what they need.
	if err := checkEnvelopeVersion(env.Version); err != nil {
		return err
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
		// The payload is parsed for its VERSION as well as its shape: a backup whose contents
		// were written by a newer Nib is refused here rather than after the overwrite. (The
		// envelope's own ceiling at this door is /pending 287 and is deliberately not folded
		// in — it is a filed, grill-pending item and gets its own pass.)
		if _, uerr = decodeContents(plain); uerr != nil {
			zero(plain)
			return errors.New("backup is corrupt: its contents do not parse, or they were " +
				"written by a newer version of Nib")
		}
		zero(plain)
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

// SetIdentityIfAbsent stores the signing identity ONLY if the vault has none, persists the vault,
// and returns whichever identity is authoritative afterwards — the caller's, or the one that was
// already there.
//
// # Why there is no unconditional setter
//
// This replaced `SetIdentity`, which overwrote whatever was present. Its one caller —
// `internal/server`'s `identity()` — was a read-then-write across two separate lock holds:
// `Identity()` said absent, the caller minted a key, `SetIdentity` stored it. Nothing held the lock
// across the gap, so two near-simultaneous first callers both minted and the second overwrote the
// first. **Measured: eight concurrent callers against one fresh vault produced 3 distinct
// identities, 6 of them holding a certificate the vault did not hold** (/pending 285).
//
// A checked setter beside the unconditional one would have left the defect one call site away. The
// unconditional one had exactly one caller and no other need, so it is gone — the race is not
// fixed here, it is **unrepresentable**, which is the difference between a bug that stays fixed and
// one that comes back through a second door.
//
// # Why the caller mints speculatively rather than passing a callback
//
// Key generation stays OUTSIDE this lock. A `mint func()` called under the lock would put an
// arbitrary caller's work inside the vault's critical section, and `save()` is already doing disk
// I/O there. So the caller mints first and offers the result; a loser's key is simply discarded and
// it receives the winner's, which is the property that matters — every caller leaves holding the
// identity the vault actually has.
func (v *Vault) SetIdentityIfAbsent(certPEM, keyPEM []byte) (cert, key []byte, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if id := v.contents.Identity; id != nil {
		// Somebody else won. Hand back theirs, not the caller's.
		return append([]byte(nil), id.CertPEM...), append([]byte(nil), id.KeyPEM...), nil
	}
	v.contents.Identity = &Identity{CertPEM: certPEM, KeyPEM: keyPEM}
	if serr := v.save(); serr != nil {
		// Leave no identity behind that the disk does not have: a later caller must mint again
		// rather than inherit one this process failed to persist.
		v.contents.Identity = nil
		return nil, nil, serr
	}
	return append([]byte(nil), certPEM...), append([]byte(nil), keyPEM...), nil
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
			Ceremonies:  append([]string(nil), p.Ceremonies...),
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
			// **A ceremony pin never RENAMES an existing pin (P07.S02b).**
			//
			// The doc comment above already said an existing pin is never downgraded, and the
			// code honoured that for `Ceremony` and not for `Label` — so accepting an
			// invitation would have overwritten the user's own private nickname for a peer
			// they had pinned themselves, with whatever label the convener published. That is
			// a stranger editing this user's peer list by inviting them, and it was invisible
			// because `AddCeremonyPeer` had no production caller until this slice gave it one.
			//
			// The user's own pin (ceremony == "") still updates the label, because that IS the
			// user renaming their own peer, which is what AddPinnedPeer is for.
			if ceremony == "" {
				v.contents.PinnedPeers[i].Label = label
				// Promotion is one-way: a user pin stays a user pin, and a ceremony pin
				// becomes a user pin the moment the user pins it themselves. Every scope goes
				// with it — a pin the user has made their own must not be taken away by a
				// prune for a ceremony that happened to introduce them.
				v.contents.PinnedPeers[i].Ceremonies = nil
			} else {
				if v.contents.PinnedPeers[i].Label == "" {
					// An unnamed pin is not a name to protect. This is the only case where a
					// ceremony may write a label onto a pin it did not create.
					v.contents.PinnedPeers[i].Label = label
				}
				// A user pin (no scopes) is NOT given one: a ceremony must not be able to
				// make a relationship the user established revocable.
				if len(v.contents.PinnedPeers[i].Ceremonies) > 0 {
					v.contents.PinnedPeers[i].Ceremonies = addScope(
						v.contents.PinnedPeers[i].Ceremonies, ceremony)
				}
			}
			return v.save()
		}
	}
	fresh := PinnedPeer{
		Fingerprint: append([]byte(nil), fingerprint...),
		Label:       label,
	}
	if ceremony != "" {
		fresh.Ceremonies = []string{ceremony}
	}
	v.contents.PinnedPeers = append(v.contents.PinnedPeers, fresh)
	return v.save()
}

// PruneCeremonyPeers removes every pin created by the named ceremony, leaving pins the
// user made themselves (D29).
//
// **Returns how many pins it TOUCHED, not how many it removed** — corrected 2026-08-29 by the
// P08.S01 deepdive. `n` increments on every scope DROP (below), and a pin a second ceremony still
// needs is counted here and then kept. `TestTwoCeremoniesCanShareAPin` asserts exactly that and
// calls it "pins touched", so the behaviour is the tested one and this sentence was the wrong half.
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
		// **The scope is dropped; the PIN goes only when its last scope does.** A pin two
		// ceremonies both needed used to be removed by whichever ended first, leaving the
		// other's next arm refusing an unpinned peer. A user pin has no scopes at all and is
		// untouched here, which is the property `Ceremonies`' own doc calls one-way promotion.
		before := len(p.Ceremonies)
		if before == 0 {
			kept = append(kept, p)
			continue
		}
		p.Ceremonies = dropScope(p.Ceremonies, ceremony)
		if len(p.Ceremonies) == before {
			kept = append(kept, p)
			continue
		}
		n++
		if len(p.Ceremonies) > 0 {
			kept = append(kept, p) // another ceremony still needs this peer
		}
	}
	v.contents.PinnedPeers = kept
	return n, v.save()
}

// addScope returns scopes with id present exactly once, preserving order.
func addScope(scopes []string, id string) []string {
	for _, s := range scopes {
		if s == id {
			return scopes
		}
	}
	return append(scopes, id)
}

// dropScope returns scopes without id.
func dropScope(scopes []string, id string) []string {
	out := scopes[:0:0]
	for _, s := range scopes {
		if s != id {
			out = append(out, s)
		}
	}
	return out
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
// envelopeVersion is what this build writes, and the highest it will open.
const envelopeVersion = 2

// checkEnvelopeVersion refuses a vault written by a NEWER Nib.
//
// Every gate here was `env.Version < 2` — a floor with no ceiling. A vault carrying
// Version 3 sailed through, was decrypted as v2, and `save()` then unconditionally rewrote
// `envelope{Version: 2, …}` — silently downgrading the file and dropping every envelope
// field this build does not know, because encoding/json discards unknown keys. That is
// reached by an ordinary user: a downgrade, a second machine, or a vault synced through a
// shared folder between two versions. The vault holds the only copy of the signing
// identity, so a silent lossy rewrite of it is the worst shape this package has.
// decodeContents parses a decrypted payload and refuses one written by a newer Nib.
//
// **One door, because the rule had four call sites and would have had four copies.** The
// payload is unmarshalled at four places — the SSH repair path, OpenSSH, the password path
// and the backup validator — and a version gate applied at three of them is a gate that does
// not exist. ADR-009: the rule is written once and every site calls it.
func decodeContents(plain []byte) (Contents, error) {
	var c Contents
	if err := json.Unmarshal(plain, &c); err != nil {
		return Contents{}, err
	}
	if err := checkContentsVersion(c.Version); err != nil {
		return Contents{}, err
	}
	return c, nil
}

func checkEnvelopeVersion(v int) error {
	if v > envelopeVersion {
		return fmt.Errorf("this vault was written by a newer version of Nib (format %d, "+
			"this build understands %d) — update Nib rather than opening it here, or it "+
			"will be rewritten and anything the newer version stored will be lost",
			v, envelopeVersion)
	}
	return nil
}

func (v *Vault) save() error {
	// Stamp the payload version on every write, so a file this build saves is readable back
	// as this build's. Set here rather than at each mutator for the reason save() is the one
	// door they all pass through.
	v.contents.Version = contentsVersion
	plain, err := json.Marshal(v.contents)
	if err != nil {
		return err
	}
	// Zeroed like every read path does.
	//
	// openSSH, OpenSSHAt, Migrate and loadBuiltinSignatures all call zero(plain) with
	// comments explaining that this buffer carries Identity.KeyPEM — the PDF-signing
	// private key — and ExternalSigner.P12. save() dropped it on the floor, and save()
	// runs on every AddRecent, SetSettings, AddImage and AddPinnedPeer, so the heap
	// accumulated plaintext copies of the signing key in proportion to ordinary UI
	// activity. The read path's discipline was undone by the write path.
	defer zero(plain)
	nonce, ct, err := encrypt(v.key, plain)
	if err != nil {
		return err
	}
	out, err := json.Marshal(envelope{Version: envelopeVersion, Nonce: nonce, Cipher: ct, SSH: v.ssh})
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
	// **The refusal lives HERE, not at each caller.** It was written at three of the six
	// readEnvelope callers, and the three it missed were the three where it matters most or
	// where it is hardest to see: Migrate, which decrypts and then unconditionally rewrites
	// the file through newSealed + Save — the exact silent-downgrade path the refusal was
	// written to stop, reached by an ordinary user with a synced vault; NeedsMigration,
	// which answers `env.Version < 2` about a file it may not understand; and Slots, which
	// would list the key slots of a vault this build must not touch.
	//
	// A rule copied to some callers is not a rule. Nothing legitimately reads an envelope
	// from a newer build — a v1 password vault is OLDER and still passes, which is what
	// Migrate needs.
	if err := checkEnvelopeVersion(env.Version); err != nil {
		return nil, err
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
// WriteFileAtomicDurable is writeFileAtomic, exported for the one caller outside this
// package that writes a VAULT (handleVaultImport). Anything that is NOT a vault should call
// internal/atomicfile directly and choose its own mode.
//
// **`internal/server` HAD a function of the same name with a different contract** — it renamed
// atomically and never fsynced — and `handleVaultImport` used it to replace `vault.nib`, so the
// rename was atomic and the data blocks were not durable: a power loss inside the writeback window
// left the vault present and garbage while the original, the only copy of the identity, was
// already gone. Two same-named functions with different durability contracts is also how nobody
// noticed.
//
// **That twin is gone (/pending 287, 2026-08-27).** Its four callers now name their own mode at
// the call site — `atomicfile.WriteDurable` for the three that hold the only copy of something
// (in-place save, save-as, a document a peer sent) and `atomicfile.Write` for the one that does
// not (a split export, re-derivable from a document still open). The paragraph above is kept in
// the past tense rather than deleted, because the shape it describes is the reason this comment
// exists and a reader arriving at a third copy needs to know it has happened before.
func WriteFileAtomicDurable(path string, data []byte) error { return writeFileAtomic(path, data) }

// writeFileAtomic is the vault's durable write: 0600, via internal/atomicfile.
//
// **The implementation moved out at P07.S02a** and the mode stayed here, which is the split
// that matters — the vault holds keys, so its perm is not a caller's choice. The rule itself
// now has one door for the three consumers that need it (this, the vault import, and the
// ceremony mirror); it had two implementations with different contracts before, and this
// file's own comment above records what that cost.
func writeFileAtomic(path string, data []byte) error {
	return atomicfile.WriteDurable(path, data, 0o600)
}

// --- ceremony secrets (P07.S02a) ---------------------------------------------

// CeremonySecrets returns every secret held for one ceremony, deep-copied.
//
// Deep-copied like PinnedPeers, and here it is not merely tidy: the slices are 32-byte
// channel secrets, and handing a caller the backing array would let anything downstream
// mutate the vault's own copy without going through save().
func (v *Vault) CeremonySecrets(ceremony string) []CeremonySecret {
	v.mu.Lock()
	defer v.mu.Unlock()
	var out []CeremonySecret
	for _, s := range v.contents.CeremonySecrets {
		if s.Ceremony != ceremony {
			continue
		}
		out = append(out, CeremonySecret{
			Ceremony:    s.Ceremony,
			Fingerprint: append([]byte(nil), s.Fingerprint...),
			Secret:      append([]byte(nil), s.Secret...),
		})
	}
	return out
}

// CeremonySecret returns one party's secret for one ceremony.
func (v *Vault) CeremonySecret(ceremony string, fingerprint []byte) ([]byte, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, s := range v.contents.CeremonySecrets {
		if s.Ceremony == ceremony && bytes.Equal(s.Fingerprint, fingerprint) {
			return append([]byte(nil), s.Secret...), true
		}
	}
	return nil, false
}

// AddCeremonySecret stores one party's invitation secret and persists the vault.
//
// Upsert by (ceremony, fingerprint) like addPinned, so re-issuing an invitation to one party
// replaces that party's secret rather than accumulating two rows for one seat — of which
// only one would ever be found again.
func (v *Vault) AddCeremonySecret(ceremony string, fingerprint, secret []byte) error {
	if ceremony == "" {
		// The same refusal PruneCeremonyPeers makes, for the same reason: there is no
		// ceremony called "", and a secret filed under one is a secret nothing can prune.
		return errors.New("a ceremony secret needs a ceremony id")
	}
	if len(fingerprint) == 0 || len(secret) == 0 {
		return errors.New("a ceremony secret needs a party and a secret")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.contents.CeremonySecrets {
		s := &v.contents.CeremonySecrets[i]
		if s.Ceremony == ceremony && bytes.Equal(s.Fingerprint, fingerprint) {
			// Zero the one being replaced. PruneCeremonySecrets does this fourteen lines
			// below with the reason at the line — "do not let the old backing array outlive
			// it" — and re-issuing an invitation drops a 32-byte secret exactly as removing
			// one does. Same rule, and it was applied at one of the two places it applies.
			zero(s.Secret)
			s.Secret = append([]byte(nil), secret...)
			return v.save()
		}
	}
	v.contents.CeremonySecrets = append(v.contents.CeremonySecrets, CeremonySecret{
		Ceremony:    ceremony,
		Fingerprint: append([]byte(nil), fingerprint...),
		Secret:      append([]byte(nil), secret...),
	})
	return v.save()
}

// PruneCeremonySecrets removes every secret for one ceremony. Returns how many went.
//
// The teardown half, and it exists in the same slice as the write for a reason recorded at
// P07.S02a's grill: `RemoveMirror` and `PruneCeremonyPeers` both shipped with ZERO production
// callers, so the ceremony's residue already had two owners that were never wired. A third
// write with no delete would be the same shape again, and this one is key material.
func (v *Vault) PruneCeremonySecrets(ceremony string) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ceremony == "" {
		return 0, errors.New("prune needs a ceremony id")
	}
	var kept []CeremonySecret
	var going [][]byte
	n := 0
	for _, s := range v.contents.CeremonySecrets {
		if s.Ceremony == ceremony {
			going = append(going, s.Secret)
			n++
			continue
		}
		kept = append(kept, s)
	}
	before := v.contents.CeremonySecrets
	v.contents.CeremonySecrets = kept
	// **Zero only once the removal is DURABLE.** The first draft zeroed inside the loop, so a
	// failing save() left the on-disk vault still holding every secret while the in-memory
	// copies were already scrubbed — key material at rest with nothing in the process that
	// could re-write or re-read it, and the caller (unconvene) discards the error. Restoring
	// the slice on failure keeps the two views of the vault agreeing.
	if err := v.save(); err != nil {
		v.contents.CeremonySecrets = before
		return 0, err
	}
	for _, sec := range going {
		zero(sec)
	}
	return n, nil
}
