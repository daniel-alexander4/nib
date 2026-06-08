package sign

import (
	"bytes"
	"testing"
)

func TestFingerprint(t *testing.T) {
	certA, _, err := GenerateIdentity("A")
	if err != nil {
		t.Fatal(err)
	}
	certB, _, _ := GenerateIdentity("B")

	fpA, err := Fingerprint(certA)
	if err != nil {
		t.Fatal(err)
	}
	if len(fpA) != 32 {
		t.Errorf("fingerprint len = %d, want 32 (SHA-256)", len(fpA))
	}
	if fpA2, _ := Fingerprint(certA); !bytes.Equal(fpA, fpA2) {
		t.Error("fingerprint not deterministic for the same cert")
	}
	if fpB, _ := Fingerprint(certB); bytes.Equal(fpA, fpB) {
		t.Error("distinct identities share a fingerprint")
	}
	if _, err := Fingerprint([]byte("not a pem")); err == nil {
		t.Error("expected error on invalid PEM")
	}
}
