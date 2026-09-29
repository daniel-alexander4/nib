package testpdf

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"time"

	dpdf "github.com/digitorus/pdf"
	psign "github.com/digitorus/pdfsign/sign"
	"github.com/digitorus/timestamp"
)

// DocTimeStamped appends a PAdES document timestamp (`/Type /DocTimeStamp`, `/SubFilter
// /ETSI.RFC3161`) over pdf — the B-LTA shape — from an RFC 3161 authority on loopback that stamps
// whatever hash it is sent, which is what every public authority does and why a document timestamp
// names nobody. The token is REAL: its SignedData encapsulates a TSTInfo, which is what makes the
// record a timestamp and not a relabelled signature (ADR-060). Shared so the sign and cli tests
// measure one fixture.
func DocTimeStamped(pdf []byte) ([]byte, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "Loopback TSA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		req, err := timestamp.ParseRequest(b)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ts := timestamp.Timestamp{HashAlgorithm: req.HashAlgorithm, HashedMessage: req.HashedMessage,
			Time: time.Now(), Nonce: req.Nonce, Policy: []int{1, 2, 3}, AddTSACertificate: true}
		resp, err := ts.CreateResponseWithOpts(cert, k, crypto.SHA256)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(resp)
	}))
	defer srv.Close()
	rdr, err := dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return nil, fmt.Errorf("testpdf: read pdf: %w", err)
	}
	var out bytes.Buffer
	if err := psign.Sign(bytes.NewReader(pdf), &out, rdr, int64(len(pdf)), psign.SignData{
		Signature:       psign.SignDataSignature{CertType: psign.TimeStampSignature},
		DigestAlgorithm: crypto.SHA256, TSA: psign.TSA{URL: srv.URL}}); err != nil {
		return nil, fmt.Errorf("testpdf: document timestamp: %w", err)
	}
	return out.Bytes(), nil
}
