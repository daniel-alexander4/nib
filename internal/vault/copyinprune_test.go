package vault

import (
	"bytes"
	"testing"
)

// TestTheIdentitySettersCopyIn — /pending 807 R8, the rule `AddImage` states for /pending 567.
//
// `SetExternalSigner` and `SetIdentityIfAbsent` stored the caller's slices as given, so a caller
// that reused its buffer rewrote the vault's private key material in memory under the lock.
func TestTheIdentitySettersCopyIn(t *testing.T) {
	v := newVault(t)
	p12, cert := []byte("p12-bytes"), []byte("cert-pem")
	if err := v.SetExternalSigner(p12, cert); err != nil {
		t.Fatal(err)
	}
	ic, ik := []byte("id-cert"), []byte("id-key")
	if _, _, err := v.SetIdentityIfAbsent(ic, ik); err != nil {
		t.Fatal(err)
	}
	for _, b := range [][]byte{p12, cert, ic, ik} {
		for i := range b {
			b[i] = 'X'
		}
	}
	e, ok := v.ExternalSigner()
	if !ok || string(e.P12) != "p12-bytes" || string(e.CertPEM) != "cert-pem" {
		t.Errorf("the caller rewrote its buffers and the vault's external signer changed with them: %+v", e)
	}
	gc, gk, ok := v.Identity()
	if !ok || !bytes.Equal(gc, []byte("id-cert")) || !bytes.Equal(gk, []byte("id-key")) {
		t.Errorf("the caller rewrote its buffers and the vault's identity changed with them: %q %q", gc, gk)
	}
}

// TestAPruneThatMatchesNothingWritesNothing — /pending 807 R8.
//
// Every close-out runs all three ceremony prunes. `PruneCeremonyInvitations` returns before the
// save when nothing matched; `PruneCeremonyPeers` and `PruneCeremonySecrets` rewrote the whole
// vault, durably, regardless.
func TestAPruneThatMatchesNothingWritesNothing(t *testing.T) {
	v := newVault(t)
	if err := v.AddCeremonySecret("other", []byte("fp"), bytes.Repeat([]byte{7}, 32)); err != nil {
		t.Fatal(err)
	}
	for name, prune := range map[string]func() (int, error){
		"PruneCeremonyPeers":       func() (int, error) { return v.PruneCeremonyPeers("absent") },
		"PruneCeremonySecrets":     func() (int, error) { return v.PruneCeremonySecrets("absent") },
		"PruneCeremonyInvitations": func() (int, error) { return v.PruneCeremonyInvitations("absent") },
	} {
		done := withFailingWrite(t)
		n, err := prune()
		if writes := done(); writes != 0 || err != nil || n != 0 {
			t.Errorf("%s over a ceremony with nothing to prune wrote the vault %d time(s) (n=%d, %v), want none",
				name, writes, n, err)
		}
	}
	// The control: a prune that matches still writes.
	done := withFailingWrite(t)
	_, err := v.PruneCeremonySecrets("other")
	if writes := done(); writes != 1 || err == nil {
		t.Fatalf("control: a matching prune wrote %d time(s) (%v), want 1 failing write", writes, err)
	}
}
