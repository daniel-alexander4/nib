package vault

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// TestAVaultHoldingTheRemovedChainStillOpensListsAndSigns — /pending 863.
//
// `ExternalSigner` held a third field, `ChainPEM` (JSON key `chain`), which the import route built and
// stored and nothing read: signing takes the chain from the PKCS#12 bundle itself. The field is gone,
// and every vault that imported a certificate before the removal still carries the key on disk. The
// payload version is NOT moved for this — a build that held the field reads a vault without it (the
// key was `omitempty`), and this build reads a vault with it, which is what this test holds.
//
// The old key is put into the SEALED payload through the vault's own encrypt, the way a real old vault
// came to hold it; `TestVaultWithRemovedToolbarStyleKeyStillOpens` above unmarshals a settings object
// beside the vault and never puts the key in the file.
func TestAVaultHoldingTheRemovedChainStillOpensListsAndSigns(t *testing.T) {
	const pass = "p12pass"
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Old Vault CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0),
		KeyUsage: x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Old Vault Signer"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(2, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(leafDER)
	p12, err := pkcs12.Modern.Encode(leafKey, leaf, []*x509.Certificate{caCert}, pass)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	chainPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SetExternalSigner(p12, certPEM); err != nil {
		t.Fatal(err)
	}

	// Seal the payload again with the key an older Nib wrote beside the other two.
	payload := func() (envelope, map[string]any) {
		t.Helper()
		raw, err := os.ReadFile(Path(dir))
		if err != nil {
			t.Fatal(err)
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatal(err)
		}
		plain, err := decrypt(v.key, env.Nonce, env.Cipher)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(plain, &m); err != nil {
			t.Fatal(err)
		}
		return env, m
	}
	env, m := payload()
	wroteVersion := m["version"]
	es, ok := m["externalSigner"].(map[string]any)
	if !ok {
		t.Fatalf("setup: the sealed payload holds no externalSigner object: %v", m)
	}
	es["chain"] = chainPEM // []byte marshals as base64, as the field did
	plain, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	env.Nonce, env.Cipher, err = encrypt(v.key, plain)
	if err != nil {
		t.Fatal(err)
	}
	old, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), old, 0o600); err != nil {
		t.Fatal(err)
	}
	// The stimulus: the old key is in the file, or everything below asserts nothing.
	if _, m := payload(); m["externalSigner"].(map[string]any)["chain"] == nil {
		t.Fatal("setup: the sealed payload does not carry the removed key")
	}

	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatalf("a vault that holds the removed chain no longer opens: %v", err)
	}
	// Opening is a read. A vault is not rewritten to drop the key.
	if after, err := os.ReadFile(Path(dir)); err != nil || !bytes.Equal(after, old) {
		t.Errorf("opening the vault rewrote its file (read error %v)", err)
	}

	// It lists: the certificate the settings page reads its subject, issuer and expiry from.
	got, ok := reopened.ExternalSigner()
	if !ok {
		t.Fatal("the reopened vault holds no external signer")
	}
	if !bytes.Equal(got.P12, p12) || !bytes.Equal(got.CertPEM, certPEM) {
		t.Fatal("the reopened vault's bundle or certificate is not the one stored")
	}
	blk, _ := pem.Decode(got.CertPEM)
	if blk == nil {
		t.Fatal("the stored certificate is not PEM")
	}
	if c, err := x509.ParseCertificate(blk.Bytes); err != nil || c.Subject.CommonName != "Old Vault Signer" || c.Issuer.CommonName != "Old Vault CA" {
		t.Errorf("the stored certificate reads as %v (error %v), want subject Old Vault Signer issued by Old Vault CA", c, err)
	}

	// It signs, and the chain the removed field held is the one the bundle still carries.
	if _, chain, err := sign.ParseP12(got.P12, pass); err != nil || len(chain) != 1 || !bytes.Equal(chain[0].Raw, caDER) {
		t.Errorf("the bundle's chain = %d certificates (error %v), want the one issuing CA", len(chain), err)
	}
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.SignExternal(pdf, got.P12, pass, sign.Options{Reason: "old vault", When: time.Now()})
	if err != nil {
		t.Fatalf("signing with the reopened vault's certificate: %v", err)
	}
	if len(signed) <= len(pdf) {
		t.Error("signing returned a document no larger than its input")
	}

	// An ordinary save writes the payload this build knows — same version, signer kept, key gone.
	if err := reopened.UpdateSettings(func(s *Settings) { s.Appearance = "light" }); err != nil {
		t.Fatal(err)
	}
	_, m = payload()
	if m["version"] != wroteVersion {
		t.Errorf("payload version after a save = %v, was %v: the removal must not move it", m["version"], wroteVersion)
	}
	kept, _ := m["externalSigner"].(map[string]any)
	if kept["p12"] == nil || kept["cert"] == nil {
		t.Errorf("a save after reopening lost the external signer: %v", kept)
	}
	if kept["chain"] != nil {
		t.Error("a save after reopening still writes the removed key")
	}
}
