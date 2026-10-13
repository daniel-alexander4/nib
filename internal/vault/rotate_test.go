package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"nib/internal/sshkey"
)

// ADR-144, /pending 668 #6: removing an authorized key replaces the content key.

// threeKeyVault is a vault on a fresh dir sealed to keys A (current), B and C, holding one recent
// file so there is content to read back. It returns the public lines and private-key paths in
// that order.
func threeKeyVault(t *testing.T) (v *Vault, dir string, pubs, paths [3]string) {
	t.Helper()
	dir = t.TempDir()
	for i := range pubs {
		pubs[i], paths[i] = newKey(t)
	}
	v, err := Create(dir, pubs[0], paths[0])
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 3; i++ {
		if err := v.AddKey(pubs[i], paths[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.AddRecent("/before-the-removal.pdf"); err != nil {
		t.Fatal(err)
	}
	return v, dir, pubs, paths
}

func mustEnvelope(t *testing.T, dir string) *envelope {
	t.Helper()
	env, err := readEnvelope(dir)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

// TestRemovingAKeyReplacesTheContentKey — the property itself, read off the file.
func TestRemovingAKeyReplacesTheContentKey(t *testing.T) {
	v, dir, pubs, paths := threeKeyVault(t)

	before := mustEnvelope(t, dir)
	oldKeySlice := v.key
	oldKey := append([]byte(nil), v.key...)
	removedSlot := before.SSH[1]
	if keyID(removedSlot.PubKey) != keyID(pubs[1]) {
		t.Fatal("setup: slot 1 is not key B")
	}
	if before.Gen != 0 {
		t.Fatalf("setup: a vault whose key was never replaced is at generation %d, want 0", before.Gen)
	}

	if err := v.RemoveKey(pubs[1]); err != nil {
		t.Fatalf("RemoveKey: %v", err)
	}

	after := mustEnvelope(t, dir)
	if after.Gen != 1 {
		t.Errorf("after one removal the file is at generation %d, want 1", after.Gen)
	}
	// The slots that stay: same keys, same recorded paths, same order — values only.
	if len(after.SSH) != 2 {
		t.Fatalf("after removing one of three keys the file carries %d slot(s), want 2", len(after.SSH))
	}
	for i, want := range []int{0, 2} {
		if after.SSH[i].PubKey != pubs[want] || after.SSH[i].KeyPath != paths[want] {
			t.Errorf("slot %d is %q at %q, want the key enrolled as %q at %q — a remaining slot's "+
				"PubKey and KeyPath are carried, never rebuilt", i, keyID(after.SSH[i].PubKey),
				after.SSH[i].KeyPath, keyID(pubs[want]), paths[want])
		}
	}

	// THE OLD KEY OPENS NOTHING WRITTEN NOW. The removed key's slot, taken from the file as it was
	// before the removal, still unwraps — to the old key — and that key does not decrypt the new
	// file. This is exactly what the removed machine holds: its private key and an old vault.nib.
	recovered, err := sshkey.Unwrap(removedSlot.Wrapped, paths[1], removedSlot.PubKey, nil)
	if err != nil {
		t.Fatalf("setup: the removed key no longer unwraps its own old slot: %v", err)
	}
	if !bytes.Equal(recovered, oldKey) {
		t.Fatal("setup: the removed key's old slot did not unwrap to the old content key")
	}
	if plain, derr := decrypt(recovered, after.Nonce, after.Cipher); derr == nil {
		zero(plain)
		t.Error("the content key the removed key can still recover from an older vault file " +
			"decrypts the vault written after the removal — the slot was dropped and the key " +
			"was not replaced (/pending 668 #6)")
	}

	// Every key that stays unwraps its slot to ONE key, which is not the old one and opens the file.
	var newKey []byte
	for i, want := range []int{0, 2} {
		k, uerr := sshkey.Unwrap(after.SSH[i].Wrapped, paths[want], after.SSH[i].PubKey, nil)
		if uerr != nil {
			t.Fatalf("the key that stays in slot %d cannot unwrap it: %v", i, uerr)
		}
		if bytes.Equal(k, oldKey) {
			t.Errorf("slot %d still seals the OLD content key", i)
		}
		if newKey != nil && !bytes.Equal(k, newKey) {
			t.Errorf("slot %d seals a different key from slot 0", i)
		}
		newKey = k
	}
	if _, derr := decrypt(newKey, after.Nonce, after.Cipher); derr != nil {
		t.Errorf("the key the remaining slots seal does not open the file: %v", derr)
	}

	// In memory: the vault holds the new key, proven, and the old one is scrubbed.
	if !bytes.Equal(v.key, newKey) {
		t.Error("the vault in memory does not hold the key the file is now under")
	}
	if v.unproven {
		t.Error("the key is still marked unproven after the write that proved it")
	}
	if !bytes.Equal(oldKeySlice, make([]byte, keyLen)) {
		t.Error("the replaced content key was left in memory unscrubbed")
	}

	// The contents came through, and the other machine — holding only key C — opens it.
	if err := os.Remove(paths[0]); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("the vault does not open with a key that stayed: %v", err)
	}
	if got := reopened.Recent(); len(got) != 1 || got[0] != "/before-the-removal.pdf" {
		t.Errorf("after the removal the vault's recent files read %q, want the one stored before it", got)
	}
	// And with only the removed key left on the machine, nothing opens it.
	if err := os.Remove(paths[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSSH(dir); !errors.Is(err, ErrKeyMissing) {
		t.Errorf("opening with only the removed key = %v, want ErrKeyMissing", err)
	}

	// The vault that did the removal goes on saving under the new key.
	if err := v.AddRecent("/after-the-removal.pdf"); err != nil {
		t.Fatalf("a save after the removal: %v", err)
	}
	if env := mustEnvelope(t, dir); env.Gen != 1 {
		t.Errorf("an ordinary save moved the generation to %d, want it left at 1", env.Gen)
	} else if _, derr := decrypt(newKey, env.Nonce, env.Cipher); derr != nil {
		t.Errorf("the save after the removal is not under the new key: %v", derr)
	}
}

// TestRemovingABuiltinKeyDoesNotEnrolItAgain — newSealed seals to every built-in key, so a
// rotation built on it would put back the slot the user just removed.
func TestRemovingABuiltinKeyDoesNotEnrolItAgain(t *testing.T) {
	builtin := withBuiltinInSSH(t)
	pub, keyPath := newKey(t)
	dir := t.TempDir()
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	other, otherPath := newKey(t)
	if err := v.AddKey(other, otherPath); err != nil {
		t.Fatal(err)
	}
	if got := len(mustEnvelope(t, dir).SSH); got != 3 {
		t.Fatalf("setup: %d slot(s), want the creating key, the built-in one and the added one", got)
	}
	if err := v.RemoveKey(builtin); err != nil {
		t.Fatal(err)
	}
	for _, s := range mustEnvelope(t, dir).SSH {
		if keyID(s.PubKey) == keyID(builtin) {
			t.Error("the built-in key was removed and the rotation sealed the new key to it again")
		}
	}
	if got := len(mustEnvelope(t, dir).SSH); got != 2 {
		t.Errorf("after the removal the file carries %d slot(s), want 2", got)
	}
}

// TestAFailedRemovalLeavesTheOldFileAndTheOldKey — one row per place a rotation can fail. After
// each: the file is byte-for-byte the old one, no staged file is left beside it, the slots and the
// content key in memory are the old ones, and the vault still saves and reopens.
func TestAFailedRemovalLeavesTheOldFileAndTheOldKey(t *testing.T) {
	// restaged writes through the real door with the staged bytes altered, so what the door's
	// check reads back is not what save() meant to write.
	restaged := func(alter func(*envelope)) func(string, []byte, func(string) error) error {
		real := writeFileAtomic
		return func(path string, data []byte, check func(string) error) error {
			var env envelope
			if err := json.Unmarshal(data, &env); err != nil {
				return err
			}
			alter(&env)
			out, err := json.Marshal(env)
			if err != nil {
				return err
			}
			return real(path, out, check)
		}
	}
	rows := []struct {
		name string
		// arm makes the removal fail. It may replace writeFileAtomic or damage a slot.
		arm     func(t *testing.T, v *Vault)
		wantErr string // a fragment of the refusal
		wantIs  error
		// writes is how many times the write door must have been entered: the stimulus check.
		writes int
	}{
		{
			name: "a remaining slot cannot be sealed to",
			arm: func(t *testing.T, v *Vault) {
				v.ssh[2].PubKey = "ssh-ed25519 not-base64 the-office-laptop"
			},
			wantIs:  ErrKeyNotSealable,
			wantErr: `"the-office-laptop"`,
			writes:  0,
		},
		{
			name: "the staged file does not open with the new key",
			arm: func(t *testing.T, v *Vault) {
				writeFileAtomic = restaged(func(e *envelope) { e.Cipher[len(e.Cipher)/2] ^= 1 })
			},
			wantErr: "did not open with its new key",
			writes:  1,
		},
		{
			name: "the staged file lost a slot",
			arm: func(t *testing.T, v *Vault) {
				writeFileAtomic = restaged(func(e *envelope) { e.SSH = e.SSH[:1] })
			},
			wantErr: "did not read back with the keys it was written with",
			writes:  1,
		},
		{
			name: "the staged file is not a vault",
			arm: func(t *testing.T, v *Vault) {
				real := writeFileAtomic
				writeFileAtomic = func(path string, data []byte, check func(string) error) error {
					return real(path, data[:len(data)/2], check)
				}
			},
			wantErr: "did not read back as a vault",
			writes:  1,
		},
		{
			name: "the rename fails after the staged file was proven",
			arm: func(t *testing.T, v *Vault) {
				writeFileAtomic = func(path string, data []byte, check func(string) error) error {
					staged := path + ".staged-by-the-test"
					if err := os.WriteFile(staged, data, 0o600); err != nil {
						return err
					}
					defer os.Remove(staged)
					if err := check(staged); err != nil {
						t.Errorf("setup: the check refused a good staged file: %v", err)
						return err
					}
					return errors.New("rename: no space left on device")
				}
			},
			wantErr: "no space left on device",
			writes:  1,
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			v, dir, pubs, _ := threeKeyVault(t)
			fileBefore, err := os.ReadFile(Path(dir))
			if err != nil {
				t.Fatal(err)
			}
			keysBefore := v.Keys()
			keySlice := v.key
			keyBefore := append([]byte(nil), v.key...)
			pubKey2 := v.ssh[2].PubKey

			real := writeFileAtomic
			t.Cleanup(func() { writeFileAtomic = real })
			row.arm(t, v)
			// Counted around whatever arm installed.
			armed := writeFileAtomic
			writes := 0
			writeFileAtomic = func(path string, data []byte, check func(string) error) error {
				writes++
				return armed(path, data, check)
			}

			rerr := v.RemoveKey(pubs[1])
			writeFileAtomic = real
			if rerr == nil {
				t.Fatal("RemoveKey reported success, so the vault file was replaced by one that " +
					"was never opened, or a key that stays was silently left out")
			}
			if writes != row.writes {
				t.Fatalf("setup: the write door was entered %d time(s), want %d", writes, row.writes)
			}
			v.ssh[2].PubKey = pubKey2 // undo the first row's damage; a no-op for the others
			if row.wantIs != nil && !errors.Is(rerr, row.wantIs) {
				t.Errorf("RemoveKey = %v, want %v", rerr, row.wantIs)
			}
			if !strings.Contains(rerr.Error(), row.wantErr) {
				t.Errorf("RemoveKey = %q, want it to say %q", rerr, row.wantErr)
			}
			if strings.Contains(rerr.Error(), string(keyBefore)) {
				t.Error("the refusal carries the content key")
			}

			fileAfter, err := os.ReadFile(Path(dir))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(fileAfter, fileBefore) {
				t.Error("the vault file changed although the removal failed — the old file must " +
					"stay until the new one has been written, read back and renamed")
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 1 {
				t.Errorf("a failed removal left %d entries in the vault's directory, want the vault alone", len(entries))
			}
			if !slices.Equal(v.Keys(), keysBefore) {
				t.Errorf("after a failed removal the keys in memory are %v, want the original %v", v.Keys(), keysBefore)
			}
			if !sameArray(v.key, keySlice) || !bytes.Equal(v.key, keyBefore) {
				t.Error("after a failed removal the vault in memory holds a content key the file " +
					"is not under — its next save would write a vault no enrolled key can open")
			}
			if v.gen != 0 || v.onDisk != 0 || v.unproven {
				t.Errorf("after a failed removal gen=%d onDisk=%d unproven=%v, want 0, 0, false", v.gen, v.onDisk, v.unproven)
			}

			// It is still a working vault: it saves, and what it saves opens.
			if err := v.AddRecent("/after-the-failed-removal.pdf"); err != nil {
				t.Fatalf("a save after the failed removal: %v", err)
			}
			reopened, err := OpenSSH(dir)
			if err != nil {
				t.Fatalf("the vault does not reopen after a failed removal: %v", err)
			}
			if got := len(reopened.Keys()); got != 3 {
				t.Errorf("the reopened vault has %d key(s), want all 3", got)
			}
		})
	}
}

// TestTheDoorPutsBackAKeyChangedInPlace — the door snapshots the key's BYTES, not only its slice.
// RemoveKey swaps the slice, which a header copy would undo; this is the mutation a header copy
// would not.
func TestTheDoorPutsBackAKeyChangedInPlace(t *testing.T) {
	v := newVault(t)
	want := append([]byte(nil), v.key...)
	done := withFailingWrite(t)
	v.mu.Lock()
	err := v.mutateLocked(func() {
		v.key[0] ^= 0xff
		v.gen = 7
		v.unproven = true
	})
	v.mu.Unlock()
	if writes := done(); writes != 1 {
		t.Fatalf("setup: the failing write ran %d time(s), want 1", writes)
	}
	if err == nil {
		t.Fatal("the door reported success although its save failed")
	}
	if !bytes.Equal(v.key, want) {
		t.Error("a content key overwritten in place was not put back when the save failed")
	}
	if v.gen != 0 || v.unproven {
		t.Errorf("after the failed save gen=%d unproven=%v, want 0 and false", v.gen, v.unproven)
	}
}

// TestAStaleProcessCannotWriteItsOldKeyOverTheRotatedFile — the guard against the second writer.
//
// `stale` stands for a second Nib process: it opened the vault before the removal, so it holds the
// old content key and the old slots, the removed one among them. Nothing locks the file between
// processes, and before the generation any save it made — a recent file — put all of that back.
func TestAStaleProcessCannotWriteItsOldKeyOverTheRotatedFile(t *testing.T) {
	v, dir, pubs, _ := threeKeyVault(t)
	stale, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}
	preRotation, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.RemoveKey(pubs[1]); err != nil {
		t.Fatal(err)
	}
	rotated, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}

	attempts := map[string]func() error{
		"a recent file": func() error { return stale.AddRecent("/from-the-stale-process.pdf") },
		"a key removal": func() error { return stale.RemoveKey(pubs[2]) },
		"a key added": func() error {
			pub, p := newKey(t)
			return stale.AddKey(pub, p)
		},
	}
	for name, attempt := range attempts {
		if err := attempt(); !errors.Is(err, ErrKeyChanged) {
			t.Errorf("%s saved by a process holding the old key = %v, want ErrKeyChanged", name, err)
		}
		now, rerr := os.ReadFile(Path(dir))
		if rerr != nil {
			t.Fatal(rerr)
		}
		if !bytes.Equal(now, rotated) {
			t.Fatalf("%s from a process holding the old key replaced the rotated vault file — "+
				"the removed key is authorized again and the file is back under the old content key", name)
		}
	}
	// The refused changes are not left in the stale process's memory either.
	if got := stale.Recent(); slices.Contains(got, "/from-the-stale-process.pdf") {
		t.Error("the refused save left its change in memory")
	}
	if got := len(stale.Keys()); got != 3 {
		t.Errorf("the stale process holds %d key(s) after its refused changes, want its original 3", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("refused writes left %d entries in the vault's directory, want the vault alone", len(entries))
	}

	// The other direction is NOT refused, on purpose. A writer the guard could not stop — a build
	// that predates the generation — puts the old file back; the process that did the removal then
	// finds a LOWER generation than its own, and its save is what restores the removal.
	if err := os.WriteFile(Path(dir), preRotation, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := v.AddRecent("/restores-the-removal.pdf"); err != nil {
		t.Fatalf("a save over an older generation was refused: %v", err)
	}
	env := mustEnvelope(t, dir)
	if env.Gen != 1 || len(env.SSH) != 2 {
		t.Errorf("after the rotating process saved again the file is at generation %d with %d "+
			"slot(s), want 1 and 2", env.Gen, len(env.SSH))
	}
}

// preGenerationEnvelope is the envelope exactly as builds before ADR-144 declare it: no Gen.
type preGenerationEnvelope struct {
	Version int    `json:"version"`
	Nonce   []byte `json:"nonce"`
	Cipher  []byte `json:"cipher"`
	SSH     []Slot `json:"ssh,omitempty"`
	KDF     *kdf   `json:"kdf,omitempty"`
}

// TestTheGenerationIsNotAFormatChange — both directions of "an older build and this one read each
// other's vaults", which is the condition the counter was approved under.
func TestTheGenerationIsNotAFormatChange(t *testing.T) {
	v, dir, pubs, paths := threeKeyVault(t)

	// 1. A vault whose key was never replaced is written WITHOUT the field: the file an older
	// build reads today is the file it always read.
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if _, has := keys["gen"]; has {
		t.Error("a vault whose key was never replaced carries a gen field")
	}

	// 2. An older build opens a vault this build ROTATED: same envelope version, the unknown
	// field ignored, a slot unwrapped, the payload decrypted and decoded.
	if err := v.RemoveKey(pubs[1]); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if string(keys["gen"]) != "1" {
		t.Fatalf("setup: the rotated file's gen is %q, want 1", keys["gen"])
	}
	if string(keys["version"]) != "2" {
		t.Errorf("the rotated file is envelope version %s; an older build refuses anything above 2", keys["version"])
	}
	var old preGenerationEnvelope
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatalf("the older envelope does not parse a rotated vault: %v", err)
	}
	key, err := sshkey.Unwrap(old.SSH[0].Wrapped, paths[0], old.SSH[0].PubKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decrypt(key, old.Nonce, old.Cipher)
	if err != nil {
		t.Fatalf("an older build cannot decrypt a rotated vault: %v", err)
	}
	if _, err := decodeContents(plain); err != nil {
		t.Fatalf("an older build cannot decode a rotated vault: %v", err)
	}

	// 3. That older build saves — its envelope has no gen — and this build opens the result: a
	// missing generation is 0, and the vault saves again.
	out, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), out, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("this build cannot open a vault an older build wrote: %v", err)
	}
	if reopened.gen != 0 || reopened.onDisk != 0 {
		t.Errorf("a file with no gen opened at generation %d/%d, want 0", reopened.gen, reopened.onDisk)
	}
	if err := reopened.AddRecent("/after-the-older-build.pdf"); err != nil {
		t.Errorf("a save over a vault an older build wrote: %v", err)
	}
	if got := len(reopened.Keys()); got != 2 {
		t.Errorf("the vault the older build wrote has %d key(s), want the 2 that stayed", got)
	}
}

// TestAFreshlySealedVaultIsProvenBeforeItReplacesAnything — newSealed's key has opened nothing, and
// Migrate's write replaces the password vault, the only copy of the signing identity.
func TestAFreshlySealedVaultIsProvenBeforeItReplacesAnything(t *testing.T) {
	pub, keyPath := newKey(t)
	v, err := newSealed(filepath.Join(t.TempDir(), fileName), pub, keyPath, Contents{})
	if err != nil {
		t.Fatal(err)
	}
	if !v.unproven {
		t.Fatal("a vault sealed a moment ago is not marked unproven, so its first write is not read back")
	}
	if err := v.persist(); err != nil {
		t.Fatal(err)
	}
	if v.unproven {
		t.Error("the first write did not clear unproven")
	}
}

// TestTheGenerationIsReadFromTheHeadOfTheFile — generationOf's five answers. It reads the first
// bytes only, so the cases are about what those bytes are.
func TestTheGenerationIsReadFromTheHeadOfTheFile(t *testing.T) {
	big := strings.Repeat("A", 1<<20)
	rows := []struct {
		name, file string
		absent     bool
		gen        uint64
		ok         bool
	}{
		{name: "a rotated vault", file: `{"version":2,"gen":7,"nonce":"AAAA","cipher":"` + big + `"}`, gen: 7, ok: true},
		{name: "a vault never rotated", file: `{"version":2,"nonce":"AAAA","cipher":"` + big + `"}`, gen: 0, ok: true},
		{name: "a password vault", file: `{"version":1,"nonce":"AAAA","cipher":"AAAA","kdf":{}}`, gen: 0, ok: true},
		{name: "Create's placeholder", file: ``, ok: false},
		{name: "not JSON", file: `not a vault`, ok: false},
		{name: "no file", absent: true, ok: false},
	}
	for _, row := range rows {
		path := filepath.Join(t.TempDir(), fileName)
		if !row.absent {
			if err := os.WriteFile(path, []byte(row.file), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		gen, ok := generationOf(path)
		if gen != row.gen || ok != row.ok {
			t.Errorf("%s: generationOf = %d, %v; want %d, %v", row.name, gen, ok, row.gen, row.ok)
		}
	}
}
