package server

import (
	"crypto/sha256"
	"net/http"

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
