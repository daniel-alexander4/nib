package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/vault"
)

// Co-signing (Track A): two endpoints implement one user action. The visible
// attestation block must be rendered to an image in the browser (there is no
// server-side text renderer), yet its text and placement have to come from Go so
// the on-page block can't drift from the signed /Reason. So /quote hands the
// client the exact lines and rectangle to render, and /sign signs from the same
// inputs — the client never re-derives the attestation, only rasterizes it.

const maxIntentLen = 200

// cosignParams are the attestation inputs shared by /quote and /sign.
type cosignParams struct {
	Fingerprint string `json:"fingerprint"` // the accepted peer; must already be pinned
	Intent      string `json:"intent"`
	When        string `json:"when"` // RFC3339; minted by /quote, echoed back to /sign
}

// cosignQuote is what /quote returns: the canonical attestation lines and the
// placement rectangle for the client to rasterize, plus the pinned signing time.
type cosignQuote struct {
	Lines []string   `json:"lines"`
	Rect  [4]float64 `json:"rect"` // llx, lly, urx, ury in PDF points (the client sizes the PNG to its aspect)
	// No `page`: one was published and never read. Both callers rasterize from `lines` and
	// `rect` and post the PNG back, and the server re-derives the placement when it signs —
	// so the page number was an echo the client had no use for. Deleted with
	// decryptResponse.ok (/pending 254).
	When string `json:"when"`
}

// cosignAttestation builds the attestation both calls sign over, from the same
// inputs, so the rendered block and the signed /Reason always agree. It refuses
// any peer the user hasn't pinned out-of-band (the honest-trust requirement) and
// caps the intent length. Writes the HTTP error itself and returns ok=false.
// maxWhenSkew bounds how far a client-supplied attestation time may sit from the
// server's own clock. See the use in cosignAttestation.
const maxWhenSkew = 24 * time.Hour

func (s *Server) cosignAttestation(w http.ResponseWriter, v *vault.Vault, p cosignParams) (p2p.Attestation, bool) {
	fp, err := parseFingerprint(p.Fingerprint)
	if err != nil {
		httpError(w, http.StatusBadRequest, "not a valid fingerprint")
		return p2p.Attestation{}, false
	}
	label, ok := pinnedLabel(v, fp)
	if !ok {
		httpError(w, http.StatusBadRequest, "that peer isn't pinned — pin their fingerprint first")
		return p2p.Attestation{}, false
	}
	intent := p.Intent
	if rs := []rune(intent); len(rs) > maxIntentLen {
		intent = string(rs[:maxIntentLen])
	}
	// The client may name the time, but not an arbitrary one: `when` is signed into the
	// attestation, so an unbounded value lets a caller mint a co-signature dated years
	// back or forward and have Nib's own key vouch for it. Bounded to a day either side
	// of now — enough for clock skew and a slow consent, not enough to backdate.
	when := time.Now()
	if p.When != "" {
		if t, err := time.Parse(time.RFC3339, p.When); err == nil {
			if d := t.Sub(when); d > -maxWhenSkew && d < maxWhenSkew {
				when = t
			}
		}
	}
	return p2p.Attestation{
		Signer:            "Nib User",
		AcceptedPeer:      hex.EncodeToString(fp),
		AcceptedPeerLabel: label,
		Intent:            intent,
		When:              when,
	}, true
}

// pinnedLabel returns the label the given fingerprint is pinned under, and whether
// it is pinned at all.
func pinnedLabel(v *vault.Vault, fp []byte) (string, bool) {
	for _, p := range v.PinnedPeers() {
		if bytes.Equal(p.Fingerprint, fp) {
			return p.Label, true
		}
	}
	return "", false
}

// attestationView is one signer's attestation for the verify-side display: the
// p2p reading plus whether the viewer has pinned that signer's identity locally.
type attestationView struct {
	p2p.SignerAttestation
	Pinned bool `json:"pinned"`
}

type attestationsResponse struct {
	Attestations []attestationView `json:"attestations"`
}

// handleAttestations returns the co-signing attestations on the open document —
// each signer's accepted peer, whether it cross-binds to a real co-signer
// (Matched), and whether the viewer has pinned that signer. Read-only; the
// signature-details modal fetches it lazily.
func (s *Server) handleAttestations(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	// Not resolveDoc: this route's nil answer is an empty list rather than a 404,
	// because "no document open" is not an error for a panel that polls. A named
	// document we do not hold still is one.
	doc, err := s.docFor(r)
	if err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	if doc == nil {
		writeJSON(w, attestationsResponse{Attestations: []attestationView{}})
		return
	}
	atts := p2p.ReadAttestations(s.docBytes(doc))
	views := make([]attestationView, 0, len(atts))
	for _, a := range atts {
		view := attestationView{SignerAttestation: a}
		if fp, err := hex.DecodeString(a.Fingerprint); err == nil && len(fp) == 32 {
			_, view.Pinned = pinnedLabel(v, fp)
		}
		views = append(views, view)
	}
	writeJSON(w, attestationsResponse{Attestations: views})
}

func (s *Server) handleCosignQuote(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var p cosignParams
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&p); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if p.When == "" {
		p.When = time.Now().UTC().Format(time.RFC3339)
	}
	att, ok := s.cosignAttestation(w, v, p)
	if !ok {
		return
	}
	// Not resolveDoc: this route answers 400 rather than 404 for a missing
	// document, because quoting requires one and asking without one is a client
	// bug. Preserved as-is; only the 409 arm is new.
	doc, err := s.docFor(r)
	if err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	if doc == nil {
		httpError(w, http.StatusBadRequest, "no document open")
		return
	}
	// The placement's position varies, but its size is constant, and the client
	// only uses the rectangle's width:height to size the rasterized block — /sign
	// recomputes the authoritative placement on the prepared document.
	place, err := p2p.NextPlacement(s.docBytes(doc))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not place attestation")
		return
	}
	writeJSON(w, cosignQuote{
		Lines: att.AppearanceLines(),
		Rect:  place.Rect,
		When:  att.When.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleCosignSign(w http.ResponseWriter, r *http.Request) {
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
	var p cosignParams
	if raw := r.FormValue("params"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			httpError(w, http.StatusBadRequest, "invalid params")
			return
		}
	}
	att, ok := s.cosignAttestation(w, v, p)
	if !ok {
		return
	}
	appearance, ok := formFileBytes(w, r, "appearance")
	if !ok {
		return
	}
	cert, key, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	signed, ok := s.buildCoSigned(w, pdfBytes, cert, key, att, appearance)
	if !ok {
		return
	}
	sendDownload(w, "co-signed.pdf", "application/pdf", signed)
}

// buildCoSigned prepares the document if needed and contributes this user's
// signature, returning the co-signed bytes. The first signer appends the readme
// (PrepareDocument) before signing; a later signer's document is already prepared
// and signed, so it is co-signed as-is by an incremental update. Shared by the
// Track A download path (/api/cosign/sign) and the live dial path
// (/api/session/initiate). Writes the HTTP error itself and returns ok=false.
func (s *Server) buildCoSigned(w http.ResponseWriter, pdf, cert, key []byte, att p2p.Attestation, appearance []byte) ([]byte, bool) {
	prepared := pdf
	if sign.Verify(pdf).State == sign.Unsigned {
		p, err := p2p.PrepareDocument(pdf)
		if err != nil {
			// 400 only for the one failure that IS the caller's document — an
			// already-signed file, which PrepareDocument refuses by name. Everything
			// else here is Nib's own: a bad render spec, or ErrReadmeOverflow, which
			// says the trust page's own boilerplate no longer fits its own page. A 400
			// on those tells a small-practice user their document is bad and sends them
			// looking at the wrong thing.
			if errors.Is(err, p2p.ErrReadmeOverflow) {
				httpError(w, http.StatusInternalServerError,
					"Nib could not build the trust-explainer page that goes on a co-signed "+
						"document, so nothing was signed. This is a fault in Nib, not in your file.")
				return nil, false
			}
			httpError(w, http.StatusBadRequest, "could not prepare document: "+err.Error())
			return nil, false
		}
		prepared = p
	}
	place, err := p2p.NextPlacement(prepared)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not place attestation")
		return nil, false
	}
	signed, err := p2p.Contribute(prepared, cert, key, att, appearance, place)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return signed, true
}
