package server

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/vault"
)

// finalizeParams are the JSON options posted alongside the PDF.
type finalizeParams struct {
	Reason   string     `json:"reason"`
	Page     int        `json:"page"`
	Rect     [4]float64 `json:"rect"` // llx, lly, urx, ury (PDF points)
	TSAURL   string     `json:"tsaUrl"`
	Password string     `json:"password"` // optional output protection
}

// handleFinalize signs the posted (already form-filled / flattened) PDF with a
// certification signature, optionally adds a visible appearance image and a
// trusted timestamp, optionally password-protects it, and returns the result
// for download. Any later edit invalidates the signature — the tamper-evidence.
func (s *Server) handleFinalize(w http.ResponseWriter, r *http.Request) {
	v := s.unlockedVault()
	if err := r.ParseMultipartForm(maxPDFBytes); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return
	}
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}

	var p finalizeParams
	if raw := r.FormValue("params"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			httpError(w, http.StatusBadRequest, "invalid params")
			return
		}
	}

	cert, key, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load signing identity")
		return
	}

	opts := sign.Options{
		Name:   "Nib User",
		Reason: p.Reason,
		When:   time.Now(),
		Page:   p.Page,
		Rect:   p.Rect,
		TSAURL: p.TSAURL,
	}
	if appearance, _ := formFileBytes(nil, r, "appearance"); len(appearance) > 0 {
		opts.Appearance = appearance
	}

	signed, err := sign.Sign(pdfBytes, cert, key, opts)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not sign: "+err.Error())
		return
	}
	if p.Password != "" {
		signed, err = pdfops.Encrypt(signed, p.Password)
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not protect output")
			return
		}
	}
	sendDownload(w, "finalized.pdf", "application/pdf", signed)
}

// handleIdentity exports the user's public signing certificate (PEM), so a
// recipient can confirm a signature came from this Nib identity.
func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	cert, _, err := identity(s.unlockedVault())
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	sendDownload(w, "nib-cert.pem", "application/x-pem-file", cert)
}

// identity returns the vault's signing identity, generating one on first use.
func identity(v *vault.Vault) (cert, key []byte, err error) {
	cert, key, ok := v.Identity()
	if ok {
		return cert, key, nil
	}
	cert, key, err = sign.GenerateIdentity("Nib User")
	if err != nil {
		return nil, nil, err
	}
	if err := v.SetIdentity(cert, key); err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

// formFileBytes reads a named multipart file. If w is nil, missing files are not
// reported as errors (used for optional parts).
func formFileBytes(w http.ResponseWriter, r *http.Request, field string) ([]byte, bool) {
	file, _, err := r.FormFile(field)
	if err != nil {
		if w != nil {
			httpError(w, http.StatusBadRequest, "missing "+field)
		}
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		if w != nil {
			httpError(w, http.StatusBadRequest, "could not read "+field)
		}
		return nil, false
	}
	return data, true
}

func sendDownload(w http.ResponseWriter, name, mime string, data []byte) {
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write(data)
}
