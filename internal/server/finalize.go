package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/vault"
)

// finalizeParams are the JSON options posted alongside the PDF.
type finalizeParams struct {
	Reason     string         `json:"reason"`
	Watermark  watermarkParam `json:"watermark"`
	TSAURL     string         `json:"tsaUrl"`
	SignAs     string         `json:"signAs"`     // "" / "native" (default) | "external"
	Passphrase string         `json:"passphrase"` // PKCS#12 passphrase when signAs == "external"
}

// watermarkParam is the label text plus its style. Empty text means no watermark.
type watermarkParam struct {
	Text                  string `json:"text"`
	pdfops.WatermarkStyle        // color, opacity, scale, angle
}

// handleFinalize signs the posted (already form-filled / flattened) PDF with a
// certification signature, optionally adds a visible appearance image and a
// trusted timestamp, optionally password-protects it, and returns the result
// for download. Any later edit invalidates the signature — the tamper-evidence.
func (s *Server) handleFinalize(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
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
	// The timestamp authority is user-supplied and dialed by the signing library;
	// allow only an http(s) URL through (public TSAs are commonly plain http).
	if p.TSAURL != "" {
		if u, err := url.Parse(p.TSAURL); err != nil || requireHTTPScheme(u) != nil {
			httpError(w, http.StatusBadRequest, "timestamp authority must be an http(s) URL")
			return
		}
	}

	// Bake the watermark onto every page as content, then certify invisibly: the
	// signature covers the mark, and an invisible certification is the only
	// visible-mark-plus-certification combination the signing library allows.
	var err error
	if p.Watermark.Text != "" {
		pdfBytes, err = pdfops.StampWatermark(pdfBytes, p.Watermark.Text, p.Watermark.WatermarkStyle)
		// 400 and the whole sentence, because this one is the user's to fix and the
		// alternative was baking something they did not type onto a document this same
		// handler is about to SIGN. See wroteStampTextError.
		if wroteStampTextError(w, err) {
			return
		}
		if err != nil {
			httpError(w, http.StatusInternalServerError, "could not stamp watermark")
			return
		}
	}

	// Sign with the native vault identity (default) or, when chosen, an imported
	// PKCS#12 certificate decoded fresh from its passphrase for this one signature.
	opts := sign.Options{Reason: p.Reason, When: time.Now(), TSAURL: p.TSAURL}
	var signed []byte
	if p.SignAs == "external" {
		es, ok := v.ExternalSigner()
		if !ok {
			httpError(w, http.StatusBadRequest, "no imported signing certificate")
			return
		}
		signed, err = sign.SignExternal(pdfBytes, es.P12, p.Passphrase, opts)
		if errors.Is(err, sign.ErrWrongPassphrase) {
			httpError(w, http.StatusUnauthorized, "wrong passphrase")
			return
		}
	} else {
		opts.Name = "Nib User"
		cert, key, ierr := identity(v)
		if ierr != nil {
			httpError(w, http.StatusInternalServerError, "could not load signing identity")
			return
		}
		signed, err = sign.Sign(pdfBytes, cert, key, opts)
	}
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not sign: "+err.Error())
		return
	}
	// TRIPWIRE: do not password-protect the signed output here. Encrypting after
	// signing rewrites the file and invalidates the certification signature's
	// ByteRange (a silent tamper-evidence failure), and the current libraries have
	// no encryption-aware signing (digitorus/pdfsign cannot sign or emit encrypted
	// PDFs; pdfcpu's Encrypt is signature-unaware). Confidentiality and certification
	// are mutually exclusive here — if password protection is ever wanted, it belongs
	// in a separate, signature-free export, not bolted onto finalize.
	sendDownload(w, "finalized.pdf", "application/pdf", signed)
}

// handleIdentity exports the user's public signing certificate (PEM), so a
// recipient can confirm a signature came from this Nib identity.
func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	cert, _, err := identity(vaultFrom(r))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	sendDownload(w, "nib-cert.pem", "application/x-pem-file", cert)
}

// identity returns the vault's signing identity, generating one on first use.
//
// **The mint is SPECULATIVE and the store decides (/pending 285).** This was a read-then-write
// across two separate vault lock holds — `Identity()` said absent, this minted, `SetIdentity`
// overwrote — and nothing held the lock across the gap. Measured: eight concurrent first callers
// against one fresh vault produced **3 distinct identities, 6 of them holding a certificate the
// vault did not hold**. It needs only near-simultaneous first calls, which two browser tabs will
// do, and `identity()` has eight callers.
//
// For `finalize` the loser signed with an orphaned key, which is bad and local. For a **ceremony
// record** it is durable and cross-party: the record would name a convener whose key the machine
// had discarded, so no later hop could act as convener and no re-convene could prove continuity.
//
// `SetIdentityIfAbsent` returns whichever identity is authoritative, so a loser discards its key
// and leaves holding the winner's. Generation stays outside the vault lock — the point is which
// identity is STORED, not who did the work.
func identity(v *vault.Vault) (cert, key []byte, err error) {
	cert, key, ok := v.Identity()
	if ok {
		return cert, key, nil
	}
	cert, key, err = sign.GenerateIdentity("Nib User")
	if err != nil {
		return nil, nil, err
	}
	return v.SetIdentityIfAbsent(cert, key)
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

// readFormFile reads one multipart file header fully into memory, closing it on
// every path. Used by the handlers that iterate over many uploaded parts.
func readFormFile(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func sendDownload(w http.ResponseWriter, name, mediaType string, data []byte) {
	w.Header().Set("Content-Type", mediaType)
	// mime.FormatMediaType rather than string concatenation, because ONE caller's name is
	// attacker-controlled: handleAttachmentExtract passes the filename recorded inside the
	// PDF. A name containing a double quote broke out of the quoting and could smuggle
	// further parameters — `evil"; filename="other.exe` — and every other caller passing a
	// constant is no reason to hand-roll a header that the stdlib formats correctly.
	// (Go's server already replaces CR and LF in header values, so this is the quoting
	// half, not the header-splitting half.)
	disp := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	if disp == "" { // only when the name cannot be represented at all
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", disp)
	_, _ = w.Write(data)
}
