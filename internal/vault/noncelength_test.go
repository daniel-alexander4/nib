package vault

import (
	"crypto/rand"
	"testing"
)

// TestDecryptRefusesANonceOfTheWrongLength — `cipher.AEAD.Open` PANICS on a nonce whose length is not
// the AEAD's, and the nonce comes from the vault file. A damaged or hand-edited vault must fail to
// unlock, not take the process down. Found by the P02 phase-close review, v1.138.15.
func TestDecryptRefusesANonceOfTheWrongLength(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	nonce, ct, err := encrypt(key, []byte("the vault's contents"))
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := decrypt(key, nonce, ct); err != nil || string(plain) != "the vault's contents" {
		t.Fatalf("setup: the well-formed envelope does not round-trip (%q, %v)", plain, err)
	}
	for _, n := range [][]byte{nil, nonce[:len(nonce)-1], append(append([]byte{}, nonce...), 0)} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("a %d-byte nonce panicked (%v); it must be refused with an error", len(n), r)
				}
			}()
			if _, err := decrypt(key, n, ct); err == nil {
				t.Errorf("a %d-byte nonce decrypted; it must be refused", len(n))
			}
		}()
	}
}
