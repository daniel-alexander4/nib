package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"nib/internal/sign"
)

// recentEntry is one recently opened file: the path to reopen with, and the name
// to show in the menu. The name is derived here rather than in the browser, which
// has no way to know where a Windows path ends — it stripped up to the last "/",
// so C:\Users\bob\report.pdf displayed in full and blew out the dropdown.
type recentEntry struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// handleRecent returns the recently opened files (newest first). An empty history
// serializes as [] rather than null, so the client can treat it as a list
// unconditionally.
func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	paths := vaultFrom(r).Recent()
	recent := make([]recentEntry, 0, len(paths))
	for _, p := range paths {
		recent = append(recent, recentEntry{Path: p, Name: filepath.Base(p)})
	}
	writeJSON(w, recent)
}

// handleOpenURL loads a PDF fetched from a URL (server-side, avoiding browser
// CORS). A URL-sourced document has no local path, so it saves as a copy rather
// than overwriting in place.
func (s *Server) handleOpenURL(w http.ResponseWriter, r *http.Request) {
	var req struct{ URL string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	data, err := safeFetch(req.URL, maxPDFBytes, 30*time.Second)
	if err != nil {
		httpError(w, http.StatusBadGateway, "could not fetch PDF")
		return
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		httpError(w, http.StatusUnsupportedMediaType, "URL did not return a PDF")
		return
	}
	s.setDoc(&document{path: "", data: data, sig: sign.Verify(data)})
	writeJSON(w, s.docResponse())
}
