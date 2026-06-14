package vault

import (
	"bufio"
	"encoding/json"
	"log"
	"strings"

	"nib/internal/sshkey"
)

// builtinKeysRaw (authorized_keys lines) and builtinSignaturesBlob (an age
// ciphertext of a JSON []Image) are empty in the default build and may be
// populated by an embed at build time. Both are package vars so tests can
// substitute their own.

// builtinKeys is the parsed set of authorized public keys this build was
// compiled with (none by default). A package var, not a const, so tests can
// substitute their own.
var builtinKeys = parseAuthorizedKeys(builtinKeysRaw)

func parseAuthorizedKeys(raw string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

// BuiltinKeys returns the authorized public keys this build was compiled with
// (empty by default). Every new or migrated vault is additionally sealed to
// each, so it unlocks on any machine holding one of their private halves with
// no per-machine enrollment.
func BuiltinKeys() []string { return append([]string(nil), builtinKeys...) }

// sealBuiltins appends a slot for every built-in key not already present in
// slots, sealing key to it. Such slots record no private-key path — OpenSSH
// locates the private half among the local ~/.ssh keys at unlock time. A
// malformed line is skipped rather than failing vault creation.
func sealBuiltins(key []byte, slots []Slot) []Slot {
	seen := map[string]bool{}
	for _, s := range slots {
		seen[keyID(s.PubKey)] = true
	}
	for _, bk := range builtinKeys {
		id := keyID(bk)
		if id == "" || seen[id] {
			continue
		}
		wrapped, err := sshkey.Wrap(key, bk)
		if err != nil {
			continue // a bad builtin line just isn't enrolled
		}
		slots = append(slots, Slot{PubKey: bk, Wrapped: wrapped})
		seen[id] = true
	}
	return slots
}

// loadBuiltinSignatures decrypts the embedded signature bundle with whichever
// built-in private key is present in the local ~/.ssh, returning the images. It
// returns nil when the bundle is empty or no key is available — then inert.
func loadBuiltinSignatures() []Image {
	if len(builtinSignaturesBlob) == 0 {
		return nil
	}
	for _, p := range sshkey.Candidates() {
		plain, err := sshkey.Unwrap(builtinSignaturesBlob, p)
		if err != nil {
			continue
		}
		var imgs []Image
		err = json.Unmarshal(plain, &imgs)
		zero(plain) // scrub the decrypted bundle plaintext once parsed (or on failure)
		if err != nil {
			log.Printf("warning: built-in signatures present but did not decode: %v", err)
			return nil
		}
		return imgs
	}
	return nil
}

// AutoSetup creates a vault non-interactively when one of the built-in keys'
// private halves is present in the local ~/.ssh — so a trusted machine needs no
// first-run wizard. Returns (true, nil) when it created one; (false, nil) when
// no built-in key is available locally (the caller falls back to the wizard);
// (false, err) on a real failure.
func AutoSetup(dir string) (bool, error) {
	if Exists(dir) {
		return false, nil
	}
	want := map[string]bool{}
	for _, b := range builtinKeys {
		if id := keyID(b); id != "" {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return false, nil
	}
	for _, p := range sshkey.Candidates() {
		pub, err := sshkey.PublicKeyLine(p)
		if err != nil {
			continue
		}
		if want[keyID(pub)] {
			if _, err := Create(dir, pub, p); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}
