package server

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"time"

	"nib/internal/sign"
)

// externalSignerInfo describes an imported PKCS#12 signing identity for the
// settings display and the Finalize "sign as" picker.
type externalSignerInfo struct {
	Present  bool   `json:"present"`
	Subject  string `json:"subject,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
	NotAfter string `json:"notAfter,omitempty"` // RFC3339 expiry
}

// handleExternalSignerGet reports whether an imported signing certificate exists
// and its subject/issuer/expiry (read from the stored public cert — no passphrase
// needed).
func (s *Server) handleExternalSignerGet(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	es, ok := v.ExternalSigner()
	if !ok {
		writeJSON(w, externalSignerInfo{Present: false})
		return
	}
	info := externalSignerInfo{Present: true}
	if cb, _ := pem.Decode(es.CertPEM); cb != nil {
		if cert, err := x509.ParseCertificate(cb.Bytes); err == nil {
			info.Subject = cert.Subject.CommonName
			info.Issuer = cert.Issuer.CommonName
			info.NotAfter = cert.NotAfter.UTC().Format(time.RFC3339)
		}
	}
	writeJSON(w, info)
}

// handleExternalSignerImport decodes an uploaded .p12 (multipart file "p12" plus
// "passphrase"), captures its public certificate + chain for display, and stores
// the bundle in the vault. The passphrase only validates/decodes here; it is NOT
// persisted — the .p12 is stored as imported and re-decoded per sign.
func (s *Server) handleExternalSignerImport(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	p12, ok := formFileBytes(w, r, "p12")
	if !ok {
		return
	}
	leaf, chain, err := sign.ParseP12(p12, r.FormValue("passphrase"))
	if err != nil {
		if errors.Is(err, sign.ErrWrongPassphrase) {
			httpError(w, http.StatusUnauthorized, "wrong passphrase, or not a PKCS#12 file")
			return
		}
		httpError(w, http.StatusBadRequest, "could not read certificate: "+err.Error())
		return
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	var chainPEM []byte
	for _, c := range chain {
		chainPEM = append(chainPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}
	if err := v.SetExternalSigner(p12, certPEM, chainPEM); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save certificate")
		return
	}
	s.handleExternalSignerGet(w, r)
}

// handleExternalSignerRemove deletes the imported signing identity.
func (s *Server) handleExternalSignerRemove(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	if err := v.ClearExternalSigner(); err != nil {
		httpError(w, http.StatusInternalServerError, "could not remove certificate")
		return
	}
	writeJSON(w, externalSignerInfo{Present: false})
}
