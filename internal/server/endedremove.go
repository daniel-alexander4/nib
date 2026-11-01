package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"nib/internal/ceremony"
)

// The user's removal of an ended ceremony — ADR-180, `/pending 874`.
//
// Nothing in Nib removes a closed-out ceremony by itself (ADR-012, `/pending 361`: no timer, ever). This is the one
// route that does, and it acts only when the user presses Remove on a row of the ended list and confirms the
// sentence the window shows. The request names ONE id; the folder is `ceremony.RemoveEnded`'s alone.

type removeEndedRequest struct {
	Ceremony string `json:"ceremony"`
}

// handleEndedRemove removes one ended ceremony from this computer.
func (s *Server) handleEndedRemove(w http.ResponseWriter, r *http.Request) {
	// A second copy of Nib on this machine shares the folder and must not remove from it (the listing says the
	// same of continuing a ceremony): both would be writing to ~/nib.
	if !s.isPrimary() {
		httpError(w, http.StatusConflict, "another copy of Nib is already running on this computer, so this one must not remove ceremonies — use that one.")
		return
	}
	var req removeEndedRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id := strings.TrimSpace(req.Ceremony)
	if err := ceremony.ValidID(id); err != nil {
		httpError(w, http.StatusBadRequest, "that is not a ceremony id")
		return
	}
	n, err := ceremony.RemoveEnded(defaultOutputDir(), id)
	switch {
	case errors.Is(err, ceremony.ErrNotEnded):
		httpError(w, http.StatusNotFound, "that ceremony is not in the finished list on this computer.")
		return
	case err != nil:
		httpError(w, http.StatusInternalServerError, "the ceremony could not be removed: "+err.Error())
		return
	}
	// By id only: a ceremony's name is the user's text and does not belong in a log.
	log.Printf("ceremony %s: removed from the finished list at the user's request (%d folder(s))", id, n)
	writeJSON(w, map[string]any{"removed": true})
}
