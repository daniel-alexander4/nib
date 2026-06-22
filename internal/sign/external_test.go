package sign

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"testing"
	"time"

	"nib/internal/testpdf"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// makeP12 builds a passphrase-protected .p12 holding a leaf certificate (CN=cn)
// issued by a self-signed CA, the leaf's private key, and the CA in the chain.
func makeP12(t *testing.T, cn, passphrase string) []byte {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)

	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	leaf, _ := x509.ParseCertificate(leafDER)

	pfx, err := pkcs12.Modern.Encode(leafKey, leaf, []*x509.Certificate{caCert}, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	return pfx
}

func TestSignExternal(t *testing.T) {
	p12 := makeP12(t, "Jane External", "p12pass")
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}

	// Wrong passphrase is reported distinctly so the UI can reprompt.
	if _, _, err := ParseP12(p12, "nope"); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("ParseP12 wrong passphrase: err = %v, want ErrWrongPassphrase", err)
	}
	// Correct passphrase yields the leaf + CA chain.
	leaf, chain, err := ParseP12(p12, "p12pass")
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "Jane External" {
		t.Errorf("leaf CN = %q, want Jane External", leaf.Subject.CommonName)
	}
	if len(chain) != 1 {
		t.Errorf("chain len = %d, want 1 (the CA)", len(chain))
	}

	// Signing with the imported identity produces a valid signature whose signer
	// is the external cert (CN), not Nib's native identity.
	signed, err := SignExternal(pdf, p12, "p12pass", Options{Reason: "Signed with my cert", When: time.Now()})
	if err != nil {
		t.Fatalf("SignExternal: %v", err)
	}
	st := Verify(signed)
	if st.State != Valid {
		t.Fatalf("verify state = %q, want valid", st.State)
	}
	if len(st.Signers) == 0 || st.Signers[0].Name != "Jane External" {
		t.Fatalf("signer = %+v, want Name 'Jane External'", st.Signers)
	}

	// Wrong passphrase at sign time → ErrWrongPassphrase.
	if _, err := SignExternal(pdf, p12, "wrong", Options{}); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("SignExternal wrong passphrase: err = %v, want ErrWrongPassphrase", err)
	}
}
