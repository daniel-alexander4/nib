package sign

import (
	"crypto"
	"crypto/x509"
	"errors"

	"github.com/digitorus/pdfsign/sign"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// ErrWrongPassphrase reports that the supplied passphrase did not decrypt the
// PKCS#12 bundle (a wrong password, or bytes that aren't a valid .p12).
var ErrWrongPassphrase = errors.New("wrong passphrase or not a PKCS#12 file")

// ParseP12 decodes a PKCS#12 (.p12/.pfx) bundle with passphrase and returns its
// leaf certificate and CA chain — the PUBLIC parts only; the private key is
// discarded. Used at import to validate the passphrase and capture the
// certificate for display, without ever persisting a decrypted private key.
func ParseP12(p12 []byte, passphrase string) (leaf *x509.Certificate, chain []*x509.Certificate, err error) {
	_, leaf, chain, err = decodeP12(p12, passphrase)
	return leaf, chain, err
}

// SignExternal applies a certification signature to pdf using an imported PKCS#12
// identity (the user's own / CA-issued certificate), decoded fresh from p12 with
// passphrase for this one signature — the private key is never persisted. The
// leaf certificate and its CA chain are embedded so a verifier that trusts the
// issuing CA sees a trusted signature.
//
// It mirrors Sign (a certification signature, DocMDP locking the document) and is
// the solo-Finalize path for an imported identity. It is deliberately NOT used for
// co-signing: co-signing always uses the native vault identity, whose SPKI is the
// pinned peer fingerprint.
func SignExternal(pdfBytes, p12 []byte, passphrase string, opts Options) ([]byte, error) {
	key, leaf, chain, err := decodeP12(p12, passphrase)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("imported key is not a signer")
	}
	name := opts.Name
	if name == "" {
		name = leaf.Subject.CommonName
	}
	data := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType:   sign.CertificationSignature,
			DocMDPPerm: sign.DoNotAllowAnyChangesPerms,
			Info:       sign.SignDataSignatureInfo{Name: name, Reason: opts.Reason, Date: opts.When},
		},
		Signer:      signer,
		Certificate: leaf,
		// digitorus embeds CertificateChains[0][1:] as the parents, so the leaf
		// leads and the CA intermediates follow.
		CertificateChains: [][]*x509.Certificate{append([]*x509.Certificate{leaf}, chain...)},
		DigestAlgorithm:   crypto.SHA256,
	}
	if opts.TSAURL != "" {
		data.TSA = sign.TSA{URL: opts.TSAURL}
	}
	return runSign(pdfBytes, data)
}

// decodeP12 wraps the PKCS#12 decode, mapping a wrong/garbage password to the
// ErrWrongPassphrase sentinel so callers can reprompt.
func decodeP12(p12 []byte, passphrase string) (key any, leaf *x509.Certificate, chain []*x509.Certificate, err error) {
	key, leaf, chain, err = pkcs12.DecodeChain(p12, passphrase)
	if err != nil {
		if errors.Is(err, pkcs12.ErrIncorrectPassword) {
			return nil, nil, nil, ErrWrongPassphrase
		}
		return nil, nil, nil, err
	}
	return key, leaf, chain, nil
}
