package sign

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/sign"
)

// GenerateIdentity creates a self-signed ECDSA signing identity and returns its
// certificate and private key as PEM. Self-signed is sufficient for Nib's
// purpose — tamper-evidence (integrity), not third-party identity trust.
func GenerateIdentity(commonName string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(30, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// Options controls a finalize-and-sign operation.
type Options struct {
	Name       string     // signer common name, recorded in the signature
	Reason     string     // e.g. "Finalized in Nib"
	When       time.Time  // signing time
	Appearance []byte     // optional PNG drawn as the visible signature stamp
	Page       int        // 1-based page for the visible appearance
	Rect       [4]float64 // llx, lly, urx, ury in PDF points
	TSAURL     string     // optional RFC3161 timestamp authority
}

// Sign applies a certification signature (DocMDP "no changes allowed") to pdf
// using the given PEM identity. Any later edit invalidates it — that is the
// tamper-evidence. A visible appearance is added when Options.Appearance is set.
func Sign(pdfBytes, certPEM, keyPEM []byte, opts Options) ([]byte, error) {
	cert, signer, err := parseIdentity(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	data := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType:   sign.CertificationSignature,
			DocMDPPerm: sign.DoNotAllowAnyChangesPerms,
			Info: sign.SignDataSignatureInfo{
				Name:   opts.Name,
				Reason: opts.Reason,
				Date:   opts.When,
			},
		},
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	}
	if len(opts.Appearance) > 0 {
		page := opts.Page
		if page < 1 {
			page = 1
		}
		data.Appearance = sign.Appearance{
			Visible:     true,
			Page:        uint32(page),
			LowerLeftX:  opts.Rect[0],
			LowerLeftY:  opts.Rect[1],
			UpperRightX: opts.Rect[2],
			UpperRightY: opts.Rect[3],
			Image:       opts.Appearance,
		}
	}
	if opts.TSAURL != "" {
		data.TSA = sign.TSA{URL: opts.TSAURL}
	}

	rdr, err := dpdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	var out bytes.Buffer
	if err := sign.Sign(bytes.NewReader(pdfBytes), &out, rdr, int64(len(pdfBytes)), data); err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	return out.Bytes(), nil
}

func parseIdentity(certPEM, keyPEM []byte) (*x509.Certificate, crypto.Signer, error) {
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, nil, errors.New("invalid identity PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, nil, errors.New("key is not a signer")
	}
	return cert, signer, nil
}
