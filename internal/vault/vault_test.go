package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
	// A fresh vault stores no settings: classic "menus" layout, update check on.
	if s := v.Settings(); s.ToolbarStyle != "menus" || s.DisableAutoUpdate {
		t.Fatalf("default settings = %+v, want {ToolbarStyle:menus DisableAutoUpdate:false}", s)
	}
	if err := v.SetSettings(Settings{ToolbarStyle: "both", DisableAutoUpdate: true}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("OpenSSH: %v", err)
	}
	if s := reopened.Settings(); s.ToolbarStyle != "both" || !s.DisableAutoUpdate {
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
