package sign

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
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

// Fingerprint returns the SHA-256 of the certificate's SubjectPublicKeyInfo —
// the stable pin for an identity. It hashes the public key, not the whole
// certificate, so the pin survives any re-issue of the cert (new validity,
// fields) as long as the signing key is the same. This is the value exchanged
// out-of-band (QR / safety-number) to pin a peer.
func Fingerprint(certPEM []byte) ([]byte, error) {
	b, _ := pem.Decode(certPEM)
	if b == nil {
		return nil, errors.New("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return nil, err
	}
	return fingerprintOf(cert), nil
}

// fingerprintOf is the one place the identity fingerprint is computed: the
// SHA-256 of the certificate's SubjectPublicKeyInfo. Both Fingerprint (from PEM)
// and the verifier (from a parsed signer cert) route through it so a pin, an
// attestation's accepted-peer, and a verified signer all hash the same bytes.
func fingerprintOf(cert *x509.Certificate) []byte {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return sum[:]
}

// FingerprintCert is Fingerprint for an already-parsed certificate — the same
// SHA-256 SPKI value, exposed for the transport verifier, which works from parsed
// peer certs. Routes through the one fingerprint source so a pin and a verified
// peer hash identical bytes.
func FingerprintCert(cert *x509.Certificate) []byte {
	return fingerprintOf(cert)
}

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

// Appearance is an optional visible signature appearance: an image baked as the
// signature's on-page widget, added in the same incremental revision as the
// signature itself. This is the PDF-native way to attach visible content to a
// signature — one writer, one revision — so it always renders and is always
// covered, unlike a separately-added annotation.
type Appearance struct {
	Image []byte     // PNG or JPEG to draw as the widget
	Page  int        // 1-based page to place it on
	Rect  [4]float64 // llx, lly, urx, ury in PDF points
}

// Options controls a finalize-and-sign operation.
type Options struct {
	Name       string      // signer common name, recorded in the signature
	Reason     string      // e.g. "Finalized in Nib"; for co-signing, carries the attestation
	When       time.Time   // signing time
	TSAURL     string      // optional RFC3161 timestamp authority
	Appearance *Appearance // optional visible appearance (approval signatures only)
}

// Sign applies a certification signature (DocMDP "no changes allowed") to pdf
// using the given PEM identity. Any later edit invalidates it — that is the
// tamper-evidence. The signature is invisible; callers bake any visible mark
// into the page content before signing. This is the solo-Finalize path.
func Sign(pdfBytes, certPEM, keyPEM []byte, opts Options) ([]byte, error) {
	cert, signer, err := ParseIdentity(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	data := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType:   sign.CertificationSignature,
			DocMDPPerm: sign.DoNotAllowAnyChangesPerms,
			Info:       sign.SignDataSignatureInfo{Name: opts.Name, Reason: opts.Reason, Date: opts.When},
		},
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	}
	if opts.TSAURL != "" {
		data.TSA = sign.TSA{URL: opts.TSAURL}
	}
	return runSign(pdfBytes, data)
}

// SignApproval applies an approval signature to pdf. Unlike Sign it asserts no
// DocMDP, so it does not lock the document: several parties can co-sign one PDF,
// each adding a signature by incremental update without invalidating the others
// (the basis of P2P / multi-signer). It refuses to co-sign a document that
// already carries a certification ("no changes") signature — a later signature
// would break that certification for strict validators.
func SignApproval(pdfBytes, certPEM, keyPEM []byte, opts Options) ([]byte, error) {
	certified, err := hasCertificationSignature(pdfBytes)
	if err != nil {
		return nil, err
	}
	if certified {
		return nil, errors.New("document is certified (no changes allowed); it cannot be co-signed")
	}
	cert, signer, err := ParseIdentity(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	data := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType: sign.ApprovalSignature,
			Info:     sign.SignDataSignatureInfo{Name: opts.Name, Reason: opts.Reason, Date: opts.When},
		},
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	}
	if opts.TSAURL != "" {
		data.TSA = sign.TSA{URL: opts.TSAURL}
	}
	if a := opts.Appearance; a != nil && len(a.Image) > 0 {
		page := a.Page
		if page < 1 {
			page = 1
		}
		data.Appearance = sign.Appearance{
			Visible:     true,
			Page:        uint32(page),
			LowerLeftX:  a.Rect[0],
			LowerLeftY:  a.Rect[1],
			UpperRightX: a.Rect[2],
			UpperRightY: a.Rect[3],
			Image:       a.Image,
		}
	}
	return runSign(pdfBytes, data)
}

// runSign performs the digitorus incremental signing of pdfBytes with the given
// signature data and returns the signed bytes.
func runSign(pdfBytes []byte, data sign.SignData) ([]byte, error) {
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

// hasCertificationSignature reports whether pdf carries a certification (DocMDP)
// signature — the "no changes allowed" kind a later approval signature would
// break for strict validators. Nib's own Verify is purely cryptographic and
// does not surface the signature type, so we read it from the PDF structure:
// an AcroForm signature field whose /V has a /Reference with /TransformMethod
// /DocMDP. (Top-level signature fields only — the conventional placement.)
func hasCertificationSignature(pdf []byte) (bool, error) {
	r, err := dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return false, fmt.Errorf("read pdf: %w", err)
	}
	acro := r.Trailer().Key("Root").Key("AcroForm")
	if acro.IsNull() {
		return false, nil
	}
	fields := acro.Key("Fields")
	for i := 0; i < fields.Len(); i++ {
		f := fields.Index(i)
		if f.Key("FT").Name() != "Sig" {
			continue
		}
		refs := f.Key("V").Key("Reference")
		for j := 0; j < refs.Len(); j++ {
			if refs.Index(j).Key("TransformMethod").Name() == "DocMDP" {
				return true, nil
			}
		}
	}
	return false, nil
}

// ParseIdentity decodes an identity's PEM certificate and key into a parsed
// certificate and its signer. Exposed for the P2P transport layer, which mints a
// short-lived child certificate signed by the identity key.
func ParseIdentity(certPEM, keyPEM []byte) (*x509.Certificate, crypto.Signer, error) {
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
