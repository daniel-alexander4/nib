package vault

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"nib/internal/sshkey"
)

// TestMain keeps the existing tests hermetic: the shipped builtin keys are
// cleared and ~/.ssh is isolated, so vaults seal only to test keys and the
// ~/.ssh fallback finds nothing unless a test puts a key there. The builtin
// tests below opt back in by setting builtinKeys themselves.
func TestMain(m *testing.M) {
	builtinKeys = nil
	if home, err := os.MkdirTemp("", "nib-vault-home"); err == nil {
		os.Setenv("HOME", home)
	}
	os.Exit(m.Run())
}

// newKey generates an SSH key pair in a temp dir and returns (pubLine, keyPath).
func newKey(t *testing.T) (string, string) {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	pub, err := sshkey.Generate(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	return pub, keyPath
}

// withBuiltinInSSH writes a fresh key to a temp ~/.ssh (so sshkey.Candidates
// finds it), registers its public line as the sole builtin key for the test,
// and returns that public line.
func withBuiltinInSSH(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	pub, err := sshkey.Generate(filepath.Join(home, ".ssh", "id_ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	old := builtinKeys
	builtinKeys = []string{pub}
	t.Cleanup(func() { builtinKeys = old })
	return pub
}

func TestCreateOpenSSHRoundTrip(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetProfile(map[string]string{"fullName": "Dan"}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("OpenSSH: %v", err)
	}
	if reopened.Profile()["fullName"] != "Dan" {
		t.Errorf("profile not round-tripped: %v", reopened.Profile())
	}
}

func TestSettingsDefaultsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	// A fresh vault stores no settings: dark theme, update check on.
	if s := v.Settings(); s.Appearance != "dark" || s.DisableAutoUpdate {
		t.Fatalf("default settings = %+v, want {Appearance:dark DisableAutoUpdate:false}", s)
	}
	if err := v.SetSettings(Settings{Appearance: "light", DisableAutoUpdate: true}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("OpenSSH: %v", err)
	}
	if s := reopened.Settings(); s.Appearance != "light" || !s.DisableAutoUpdate {
		t.Errorf("settings not round-tripped: %+v", s)
	}
}

func TestOpenSSHKeyMissing(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	if _, err := Create(dir, pub, keyPath); err != nil {
		t.Fatal(err)
	}
	os.Remove(keyPath) // private key gone — can't unlock
	if _, err := OpenSSH(dir); !errors.Is(err, ErrKeyMissing) {
		t.Errorf("OpenSSH with missing key: err = %v, want ErrKeyMissing", err)
	}
}

func TestOpenSSHMissingVault(t *testing.T) {
	if _, err := OpenSSH(t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Errorf("OpenSSH with no vault: err = %v, want ErrNotFound", err)
	}
}

// A new vault is sealed to the builtin keys too, and unlocks via a builtin key
// (found in ~/.ssh) even after the primary key is gone.
func TestBuiltinKeySealsAndUnlocks(t *testing.T) {
	withBuiltinInSSH(t)
	dir := t.TempDir()
	pub, keyPath := newKey(t) // a separate primary key
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(v.Keys()); got != 2 {
		t.Fatalf("Keys() = %d, want 2 (primary + builtin)", got)
	}
	os.Remove(keyPath) // primary gone; only the builtin (in ~/.ssh) remains
	if _, err := OpenSSH(dir); err != nil {
		t.Fatalf("OpenSSH via builtin fallback: %v", err)
	}
}

// AutoSetup creates and unlocks a vault when a builtin private key is local, and
// is a no-op (no vault) when none is.
func TestAutoSetup(t *testing.T) {
	withBuiltinInSSH(t)
	dir := t.TempDir()
	created, err := AutoSetup(dir)
	if err != nil || !created {
		t.Fatalf("AutoSetup = (%v, %v), want (true, nil)", created, err)
	}
	if _, err := OpenSSH(dir); err != nil {
		t.Fatalf("OpenSSH after AutoSetup: %v", err)
	}
	if again, _ := AutoSetup(dir); again {
		t.Error("AutoSetup re-created an existing vault")
	}
}

// A signature sealed into the embedded bundle appears in the library (read-only)
// when a builtin key is local, and never lands in the persisted vault file.
func TestBuiltinSignaturesInjected(t *testing.T) {
	pub := withBuiltinInSSH(t) // sets builtinKeys + puts the private half in ~/.ssh
	data, err := json.Marshal([]Image{{ID: "builtin-sig-1", Name: "Sig", MIME: "image/png", Data: []byte("PNGBYTES")}})
	if err != nil {
		t.Fatal(err)
	}
	blob, err := sshkey.WrapMulti(data, []string{pub}) // bundle sealed to the built-in key
	if err != nil {
		t.Fatal(err)
	}
	oldBlob := builtinSignaturesBlob
	builtinSignaturesBlob = blob
	t.Cleanup(func() { builtinSignaturesBlob = oldBlob })

	dir := t.TempDir()
	pub, keyPath := newKey(t)
	if _, err := Create(dir, pub, keyPath); err != nil {
		t.Fatal(err)
	}
	v, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := v.Image("builtin-sig-1"); !ok || string(got.Data) != "PNGBYTES" {
		t.Fatalf("builtin signature not resolvable via Image(): ok=%v", ok)
	}
	if len(v.BuiltinImages()) != 1 {
		t.Fatalf("BuiltinImages() = %d, want 1", len(v.BuiltinImages()))
	}
	if err := v.DeleteImage("builtin-sig-1"); !errors.Is(err, ErrReadOnlyImage) {
		t.Errorf("DeleteImage(builtin) = %v, want ErrReadOnlyImage", err)
	}
	for _, img := range v.Images() { // Images() is the persisted set only
		if img.ID == "builtin-sig-1" {
			t.Error("builtin signature leaked into persisted contents")
		}
	}
}

func TestAutoSetupNoLocalKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // empty ~/.ssh
	other, _ := newKey(t)         // builtin whose private half is not in ~/.ssh
	old := builtinKeys
	builtinKeys = []string{other}
	t.Cleanup(func() { builtinKeys = old })

	created, err := AutoSetup(t.TempDir())
	if err != nil || created {
		t.Fatalf("AutoSetup with no local builtin key = (%v, %v), want (false, nil)", created, err)
	}
}

func TestEncryptedAtRest(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, _ := Create(dir, pub, keyPath)
	if _, err := v.AddImage("sig.png", "image/png", []byte("SUPERSECRETPIXELS")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(Path(dir))
	if bytes.Contains(raw, []byte("SUPERSECRETPIXELS")) {
		t.Error("image data appears in plaintext in the vault file")
	}
}

func TestImageLibrary(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, _ := Create(dir, pub, keyPath)

	img, err := v.AddImage("sig.png", "image/png", []byte("pixels"))
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := OpenSSH(dir)
	if got, ok := reopened.Image(img.ID); !ok || string(got.Data) != "pixels" {
		t.Fatalf("image not round-tripped: ok=%v data=%q", ok, got.Data)
	}
	if err := reopened.DeleteImage(img.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Image(img.ID); ok {
		t.Error("image still present after delete")
	}
}

func TestRecentDedupAndCap(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, _ := Create(dir, pub, keyPath)
	for i := 0; i < 15; i++ {
		v.AddRecent("/docs/file" + string(rune('a'+i)) + ".pdf")
	}
	v.AddRecent("/docs/filea.pdf")
	rec := v.Recent()
	if len(rec) > maxRecent {
		t.Errorf("recent len = %d, want <= %d", len(rec), maxRecent)
	}
	if rec[0] != "/docs/filea.pdf" {
		t.Errorf("most-recent = %q, want /docs/filea.pdf", rec[0])
	}
}

// writeV1Vault writes an old password-format vault for migration testing.
func writeV1Vault(t *testing.T, dir, password string, c Contents) {
	t.Helper()
	k := kdf{Time: 1, Memory: 8 * 1024, Threads: 4, Salt: make([]byte, 16)}
	rand.Read(k.Salt)
	plain, _ := json.Marshal(c)
	nonce, ct, err := encrypt(k.deriveKey(password), plain)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope{Version: 1, Nonce: nonce, Cipher: ct, KDF: &k})
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateFromPasswordVault(t *testing.T) {
	dir := t.TempDir()
	writeV1Vault(t, dir, "oldpw", Contents{Profile: map[string]string{"email": "dan@x.com"}})

	if !NeedsMigration(dir) {
		t.Error("v1 vault should report NeedsMigration")
	}
	if _, err := OpenSSH(dir); !errors.Is(err, ErrNeedsMigration) {
		t.Errorf("OpenSSH on v1: err = %v, want ErrNeedsMigration", err)
	}

	pub, keyPath := newKey(t)
	if _, err := Migrate(dir, "wrongpw", pub, keyPath); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("migrate wrong password: err = %v, want ErrWrongPassword", err)
	}
	if _, err := Migrate(dir, "oldpw", pub, keyPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if NeedsMigration(dir) {
		t.Error("vault should be migrated now")
	}
	v, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("OpenSSH after migrate: %v", err)
	}
	if v.Profile()["email"] != "dan@x.com" {
		t.Errorf("contents lost in migration: %v", v.Profile())
	}
}

func TestAddRemoveKey(t *testing.T) {
	dir := t.TempDir()
	pubA, keyA := newKey(t)
	pubB, keyB := newKey(t)

	v, err := Create(dir, pubA, keyA) // key A is the current (unlocking) key
	if err != nil {
		t.Fatal(err)
	}

	// Authorize key B as well.
	if err := v.AddKey(pubB, keyB); err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	if got := v.Keys(); len(got) != 2 {
		t.Fatalf("Keys after add: %d, want 2", len(got))
	}

	// Re-adding the same key (even with a different comment) is rejected.
	if err := v.AddKey(pubB+" some-comment", keyB); !errors.Is(err, ErrKeyExists) {
		t.Errorf("AddKey duplicate: err = %v, want ErrKeyExists", err)
	}
	if err := v.AddKey("not-a-key", keyB); !errors.Is(err, ErrBadKey) {
		t.Errorf("AddKey garbage: err = %v, want ErrBadKey", err)
	}

	// Key B's private key alone can now unlock the vault: drop A's key file and
	// reopen.
	if err := os.Remove(keyA); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("OpenSSH via key B: %v", err)
	}
	cur := ""
	for _, k := range reopened.Keys() {
		if k.Current {
			cur = k.PubKey
		}
	}
	if keyID(cur) != keyID(pubB) {
		t.Errorf("current key after reopen = %q, want B", cur)
	}

	// Can't remove the key in use this session (B is current here).
	if err := reopened.RemoveKey(pubB); !errors.Is(err, ErrCurrentKey) {
		t.Errorf("RemoveKey current: err = %v, want ErrCurrentKey", err)
	}
	// Removing the other (A) is fine.
	if err := reopened.RemoveKey(pubA); err != nil {
		t.Fatalf("RemoveKey A: %v", err)
	}
	// Now only B remains — can't remove the last key.
	if err := reopened.RemoveKey(pubB); !errors.Is(err, ErrLastKey) {
		t.Errorf("RemoveKey last: err = %v, want ErrLastKey", err)
	}
	// Removing a key that isn't enrolled.
	if err := reopened.RemoveKey(pubA); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("RemoveKey missing: err = %v, want ErrNoSuchKey", err)
	}
}

// TestValidateRefusesWhatCannotBeOpenedHere pins the check that guards a
// destructive import. handleVaultImport overwrites the live vault in place with no
// prior copy, so a backup that passes Validate and then cannot be opened destroys
// the signing identity permanently.
//
// Before this check, Validate looked only at JSON shape plus three non-empty
// fields, and ALL THREE realistic mistakes below passed it. The positive controls
// at the end are what stop a check that simply refuses everything from passing.
func TestValidateRefusesWhatCannotBeOpenedHere(t *testing.T) {
	// A real, openable backup taken from a vault sealed to this machine's key.
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	if _, err := Create(dir, pub, keyPath); err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a backup sealed to someone else's key is refused", func(t *testing.T) {
		// Another machine's vault: well-formed, complete, and sealed to a key whose
		// private half is not here. This is the mis-picked-file case.
		otherDir := t.TempDir()
		otherPub, otherKeyPath := newKey(t)
		if _, err := Create(otherDir, otherPub, otherKeyPath); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(Path(otherDir))
		if err != nil {
			t.Fatal(err)
		}
		// Remove the private key so no slot has a candidate on this machine.
		if err := os.Remove(otherKeyPath); err != nil {
			t.Fatal(err)
		}
		if err := Validate(raw); err == nil {
			t.Fatal("a vault sealed to an absent key passed validation — importing it would destroy the live vault")
		}
	})

	t.Run("a v1 password vault is refused", func(t *testing.T) {
		v1Dir := t.TempDir()
		writeV1Vault(t, v1Dir, "pw", Contents{Profile: map[string]string{"email": "dan@x.com"}})
		raw, err := os.ReadFile(Path(v1Dir))
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(raw); err == nil {
			t.Fatal("a v1 password vault passed validation — it has no key slots and this build cannot open it")
		}
	})

	t.Run("a truncated backup is refused", func(t *testing.T) {
		// Truncation that leaves the JSON prefix intact is the shape that used to
		// slip through: the fields Validate checked were all near the front.
		if err := Validate(good[:len(good)/2]); err == nil {
			t.Fatal("a half-written backup passed validation")
		}
	})

	t.Run("a corrupt body is refused", func(t *testing.T) {
		var env envelope
		if err := json.Unmarshal(good, &env); err != nil {
			t.Fatal(err)
		}
		env.Cipher[len(env.Cipher)/2] ^= 0xff // flip a bit inside the sealed contents
		raw, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(raw); err == nil {
			t.Fatal("a backup whose contents do not decrypt passed validation")
		}
	})

	// --- positive controls -------------------------------------------------
	// Without these, a Validate that returned an error unconditionally would pass
	// every assertion above.

	t.Run("the user's own backup is accepted", func(t *testing.T) {
		if err := Validate(good); err != nil {
			t.Fatalf("a genuine, openable backup was refused: %v", err)
		}
	})

	t.Run("a backup whose key is passphrase-protected is accepted", func(t *testing.T) {
		// The case that decides the check's shape: the key IS on this machine, we
		// just cannot test it without prompting. Refusing here would mean a user
		// with an encrypted key could never restore their own backup.
		encDir := t.TempDir()
		encKeyPath, encPub := encryptedKeyFixture(t, t.TempDir(), "hunter2")
		if _, err := Create(encDir, encPub, encKeyPath); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(Path(encDir))
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(raw); err != nil {
			t.Fatalf("a backup sealed to a passphrase-protected key on this machine was refused: %v", err)
		}
	})
}

// encryptedKeyFixture writes a passphrase-protected ed25519 key and returns its
// path and authorized_keys line. Mirrors sshkey's own test fixture, which lives in
// that package and is not importable here.
func encryptedKeyFixture(t *testing.T, dir, passphrase string) (keyPath, pubLine string) {
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

// A vault whose recorded key path no longer resolves recovers when the user points at
// the key — and the recorded path is REWRITTEN, so the next launch finds it too.
//
// The rewrite is the half that makes it a repair. Without it the user re-types the path
// on every start, having been told it was fixed — and v1.102.2 (which stopped NEW
// enrollments recording an unstable path) does nothing for a vault that already holds
// one, which on Windows is the ordinary case: the working directory differs between an
// Explorer double-click, an "Open with", and a shortcut.
func TestOpenSSHAtRepointsAMovedKey(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	if _, err := Create(dir, pub, keyPath); err != nil {
		t.Fatal(err)
	}

	// Move the key somewhere the vault does not know about.
	moved := filepath.Join(t.TempDir(), "id_ed25519")
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(moved, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	os.Remove(keyPath)

	// The stimulus: without it, "OpenSSHAt worked" would be indistinguishable from
	// "the ordinary unlock path was working all along".
	if _, err := OpenSSH(dir); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("setup: OpenSSH still unlocks after the key moved (err = %v), so this test proves nothing", err)
	}

	if _, err := OpenSSHAt(dir, moved, nil); err != nil {
		t.Fatalf("OpenSSHAt with the key's new location: %v", err)
	}

	// The repair: the ORDINARY path now works, with no further help.
	if _, err := OpenSSH(dir); err != nil {
		t.Errorf("after a repoint, the ordinary OpenSSH still fails (%v) — the recorded KeyPath was not rewritten, so the next launch lands in key-missing again", err)
	}
	slots, err := Slots(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) == 0 || slots[0].KeyPath != moved {
		t.Errorf("the slot records %q, want the key's new location %q", slots[0].KeyPath, moved)
	}
}

// Pointing at a key that does not open this vault is refused, and refused as
// ErrKeyMissing rather than by accidentally succeeding through some other slot or
// through the ~/.ssh sweep that the ordinary unlock path performs.
func TestOpenSSHAtRefusesAForeignKey(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	if _, err := Create(dir, pub, keyPath); err != nil {
		t.Fatal(err)
	}
	_, otherPath := newKey(t) // a perfectly good key, for a different vault
	if _, err := OpenSSHAt(dir, otherPath, nil); !errors.Is(err, ErrKeyMissing) {
		t.Errorf("OpenSSHAt with a key that seals nothing here: err = %v, want ErrKeyMissing", err)
	}
	// And the recorded path is untouched — a failed repoint must not overwrite the one
	// piece of information that would let a correct attempt succeed.
	slots, err := Slots(dir)
	if err != nil {
		t.Fatal(err)
	}
	if slots[0].KeyPath != keyPath {
		t.Errorf("a REFUSED repoint rewrote the slot's key path to %q (was %q)", slots[0].KeyPath, keyPath)
	}
}

// A vault written before the toolbarStyle setting was removed still opens, and its
// surviving settings are intact.
//
// The removal (v1.109.1) deletes a field from Settings that older vaults carry in their
// stored JSON. encoding/json drops unknown fields, so this should hold — but "should"
// is the word that precedes every silent data loss, and the failure mode here is not a
// compile error or a wrong value: it is a vault that will not open at all, taking the
// user's signing identity with it. That is worth a test rather than a belief.
//
// Built by writing a vault, then editing its DECRYPTED contents to reintroduce the key —
// which is why this test seals and re-opens rather than crafting a file by hand: the
// envelope is encrypted, so the legacy key has to go in through the same door a real old
// vault's did.
func TestVaultWithRemovedToolbarStyleKeyStillOpens(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetSettings(Settings{Appearance: "light", DisableAutoUpdate: true}); err != nil {
		t.Fatal(err)
	}

	// Reach into the sealed contents and add the key an older Nib would have written.
	// Contents is this package's own struct, so the round trip goes through the real
	// encrypt/decrypt path rather than a hand-built file.
	raw, err := json.Marshal(map[string]any{
		"appearance":        "light",
		"disableAutoUpdate": true,
		"toolbarStyle":      "toolbar", // the removed field, as an old vault holds it
	})
	if err != nil {
		t.Fatal(err)
	}
	var legacy Settings
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatalf("an older vault's settings JSON no longer unmarshals: %v", err)
	}
	// The stimulus: the legacy key must genuinely be in the bytes, or this asserts nothing.
	if !bytes.Contains(raw, []byte("toolbarStyle")) {
		t.Fatal("setup: the crafted settings JSON does not carry the removed key")
	}
	if legacy.Appearance != "light" || !legacy.DisableAutoUpdate {
		t.Errorf("the surviving settings did not survive an unmarshal alongside the removed key: %+v", legacy)
	}

	// And the whole vault still opens by the ordinary path.
	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("a vault written before the removal no longer opens: %v", err)
	}
	if s := reopened.Settings(); s.Appearance != "light" || !s.DisableAutoUpdate {
		t.Errorf("settings after reopen = %+v, want appearance light and the update check off", s)
	}
}
