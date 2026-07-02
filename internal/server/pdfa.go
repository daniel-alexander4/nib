package server

import (
	"errors"
	"net/http"
	"strings"

	"nib/internal/pdfops"
)

// handlePDFA converts the posted document to a PDF/A-2b archival candidate and
// returns it as a download. Like encrypt/optimize it derives a new artifact from
// the posted (overlay-baked) bytes and never touches the open document. When the
// document can't be made conformant — non-embedded fonts or encryption — it
// returns 400 with the specific reasons so the client can explain why, rather than
// emit a file that falsely claims PDF/A. The result is a candidate: the client
// labels it "verify with veraPDF".
func (s *Server) handlePDFA(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}

	// engine=gs → the general Ghostscript converter (re-embeds fonts, converts
	// colour); default → the pure-Go normalizer (fast, but refuses hard cases).
	if r.URL.Query().Get("engine") == "gs" {
		result, err := pdfops.ConvertPDFAGhostscript(pdfBytes)
		if err != nil {
			if errors.Is(err, pdfops.ErrGhostscriptMissing) {
				httpError(w, http.StatusBadRequest, "Ghostscript is not installed")
			} else {
				httpError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		sendDownload(w, "archival.pdf", "application/pdf", result)
		return
	}

	result, blockers, err := pdfops.PreparePDFA(pdfBytes)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not convert to PDF/A")
		return
	}
	if len(blockers) > 0 {
		httpError(w, http.StatusBadRequest, "Can't convert to PDF/A: "+strings.Join(blockers, " "))
		return
	}
	sendDownload(w, "archival.pdf", "application/pdf", result)
}
