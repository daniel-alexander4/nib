package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
)

// TestSignerKinNamesEachSignerInOrder — PLAN-returned-document P03.S02: "you" is the server's to say. `signerKin` is the
// one door for the badge's count and the per-signer list the returned-document sheet asks the signed version for; both
// come from one read, aligned with `Signature.Signers`, and both are nil when nothing is asked.
func TestSignerKinNamesEachSignerInOrder(t *testing.T) {
	peer, stranger := strings.Repeat("bb", 32), strings.Repeat("cc", 32)
	sig := func(fps ...string) sign.Status {
		st := sign.Status{State: sign.Valid}
		for _, f := range fps {
			st.Signers = append(st.Signers, sign.SignerInfo{Valid: true, Fingerprint: f})
		}
		return st
	}
	if n, whose := signerKin(nil, sig(peer)); n != nil || whose != nil {
		t.Fatalf("a locked vault answered %v %v — it read nothing, so both must be absent", n, whose)
	}

	ts, srv := startServerWith(t)
	authedClient(t, ts)
	srv.mu.Lock()
	v := srv.vault
	srv.mu.Unlock()
	if v == nil {
		t.Fatal("setup: the vault is not open")
	}
	if n, whose := signerKin(v, sign.Status{State: sign.Unsigned}); n != nil || whose != nil {
		t.Fatalf("an unsigned document answered %v %v", n, whose)
	}
	peerFP, _ := hex.DecodeString(peer)
	if err := v.AddCeremonyPeer(peerFP, "A counterparty", "c"); err != nil {
		t.Fatal(err)
	}
	cert, _, err := identity(v)
	if err != nil {
		t.Fatal(err)
	}
	selfFP, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	self := hex.EncodeToString(selfFP)

	// The order is the signers' order, an upper-case fingerprint is the same signer, and an empty one is nobody's.
	n, whose := signerKin(v, sig(stranger, strings.ToUpper(self), peer, ""))
	if want := []string{"", "you", "known", ""}; !reflect.DeepEqual(whose, want) {
		t.Fatalf("whose %q, want %q — aligned with Signature.Signers", whose, want)
	}
	if n == nil || *n != 2 {
		t.Fatalf("count %v, want 2: the stranger and the signer nobody could name — the count and the list are one rule", n)
	}

	// The imported external signer's certificate is "you" too: it counted as a stranger while only the identity was read.
	extPEM, extFP := selfSignedCert(t)
	if _, w := signerKin(v, sig(extFP)); w[0] != "" {
		t.Fatalf("stimulus: the external certificate reads %q before it is imported", w[0])
	}
	if err := v.SetExternalSigner([]byte("p12"), extPEM); err != nil {
		t.Fatal(err)
	}
	if n, w := signerKin(v, sig(extFP, self)); !reflect.DeepEqual(w, []string{"you", "you"}) || *n != 0 {
		t.Fatalf("with the external signer imported: whose %q count %d, want both yours and 0 unverified", w, *n)
	}
	if got := unverifiedSigners(v, sig(extFP, stranger)); got == nil || *got != 1 {
		t.Fatalf("the badge's count %v: it must be the same door's", got)
	}
}

// selfSignedCert is a throwaway certificate as PEM and its SPKI fingerprint in hex.
func selfSignedCert(t *testing.T) ([]byte, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "external"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	fp, err := sign.Fingerprint(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	return certPEM, hex.EncodeToString(fp)
}
