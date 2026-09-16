package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"nib/internal/testpdf"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// /pending 506. A wrong certificate passphrase must NOT answer 401.
//
// The web client's `apiFetch` reads every 401 as "the vault is locked": it refreshes the status and
// throws before the calling handler sees the response. Both routes that decode a .p12 answered a wrong
// passphrase with 401, so the handlers' own "wrong passphrase" branches were dead and the user was told
// nothing — on the import, and again at Finalize, where the passphrase is typed per signature.
//
// The assertion is `!= 401` AND `== 422`, separately: the property that matters to the client is the
// first, and the second pins the status the client now branches on.

func testP12(t *testing.T, passphrase string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(7),
		Subject:               pkix.Name{CommonName: "Wrong Passphrase Test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pfx, err := pkcs12.Modern.Encode(key, cert, nil, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	return pfx
}

func importP12(t *testing.T, c *http.Client, csrf, base string, p12 []byte, passphrase string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("p12", "id.p12")
	fw.Write(p12)
	mw.WriteField("passphrase", passphrase)
	mw.Close()
	return write(t, c, csrf, http.MethodPost, base+"/api/identity/external", mw.FormDataContentType(), &buf)
}

func TestAWrongCertificatePassphraseIsNotALockedVault(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	p12 := testP12(t, "right")

	// Import, wrong passphrase.
	resp := importP12(t, c, csrf, ts.URL, p12, "wrong")
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("import with a wrong passphrase answered 401 — the web client reads that as a locked vault and shows the user nothing")
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("import with a wrong passphrase = %d, want 422", resp.StatusCode)
	}

	// Stimulus for the Finalize half: the right passphrase imports, or there is no external signer to
	// fail the passphrase against and the 422 below could come from "no imported certificate" instead.
	resp = importP12(t, c, csrf, ts.URL, p12, "right")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: import with the right passphrase = %d, want 200", resp.StatusCode)
	}

	// Finalize as the external signer, wrong passphrase.
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"reason": "test", "signAs": "external", "passphrase": "wrong"})
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	mw.WriteField("params", string(params))
	mw.Close()
	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/finalize", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("finalize with a wrong certificate passphrase answered 401 — the web client reads that as a locked vault and shows the user nothing")
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("finalize with a wrong certificate passphrase = %d, want 422", resp.StatusCode)
	}
}
