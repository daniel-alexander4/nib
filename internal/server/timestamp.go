package server

import (
	"crypto/sha256"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nib/internal/ots"
)

// handleTimestamp produces an OpenTimestamps proof (.ots) for the posted document.
// It hashes the exact bytes the browser sends (the same baked document Finalize
// signs) and submits only that SHA-256 digest to the public calendar servers —
// never the document. The returned .ots is a sidecar the user keeps alongside the
// PDF; it does not touch the PDF, so it cannot disturb an existing signature.
func (s *Server) handleTimestamp(w http.ResponseWriter, r *http.Request) {
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	digest := sha256.Sum256(pdfBytes)
	proof, err := ots.Stamp(r.Context(), httpFetchClient, digest, ots.DefaultCalendars)
	if err != nil {
		httpError(w, http.StatusBadGateway, "could not reach an OpenTimestamps calendar server")
		return
	}
	sendDownload(w, "document.ots", "application/octet-stream", proof)
}

// handleTimestampVerify checks an uploaded .ots proof against the posted document.
// It hashes the document, confirms the proof is for it, upgrades any still-pending
// commitment via the calendar, and validates the Bitcoin attestation against the
// public block explorers (at least two of which must agree). Only a public block
// height is sent outward — never the document or its hash. An optional `explorer`
// form value overrides the default explorers with a single user-supplied
// Esplora-API endpoint, which is then trusted on its own.
func (s *Server) handleTimestampVerify(w http.ResponseWriter, r *http.Request) {
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	proof, ok := formFileBytes(w, r, "ots")
	if !ok {
		return
	}
	explorers := ots.DefaultExplorers
	minAgree := 2 // default public set: at least two independent explorers must agree
	if custom := strings.TrimSpace(r.FormValue("explorer")); custom != "" {
		if u, err := url.Parse(custom); err != nil || requireHTTPScheme(u) != nil {
			httpError(w, http.StatusBadRequest, "block explorer must be an http(s) URL")
			return
		}
		explorers = []string{custom}
		minAgree = 1 // the user's own node is trusted on its own
	}
	sources := make([]ots.BlockSource, len(explorers))
	for i, e := range explorers {
		sources[i] = ots.NewEsplora(e, httpFetchClient)
	}

	digest := sha256.Sum256(pdfBytes)
	// untrustedFetchClient: the calendar URL fetched during an upgrade comes from
	// the uploaded (untrusted) .ots, so it must not be allowed to reach LAN hosts.
	res, err := ots.VerifyProof(r.Context(), untrustedFetchClient, sources, minAgree, proof, digest)
	if err != nil {
		httpError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	out := struct {
		State   string `json:"state"`
		Height  uint64 `json:"height,omitempty"`
		Time    string `json:"time,omitempty"`
		Sources int    `json:"sources,omitempty"`
	}{State: res.State, Height: res.Height, Sources: res.Sources}
	if !res.Time.IsZero() {
		out.Time = res.Time.Format(time.RFC3339)
	}
	writeJSON(w, out)
}
