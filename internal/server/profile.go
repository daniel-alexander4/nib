package server

import (
	"encoding/json"
	"io"
	"net/http"
)

// handleProfileGet returns the autofill profile (field name -> value).
func (s *Server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.unlockedVault().Profile())
}

// handleProfileSet replaces the autofill profile.
func (s *Server) handleProfileSet(w http.ResponseWriter, r *http.Request) {
	var p map[string]string
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&p); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.unlockedVault().SetProfile(p); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save profile")
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
