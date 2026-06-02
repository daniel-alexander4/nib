package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"nib/internal/sign"
)

// handleRecent returns the recently opened file paths (newest first). An empty
// history serializes as [] rather than null, so the client can treat it as a
// list unconditionally.
func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	recent := s.unlockedVault().Recent()
	if recent == nil {
		recent = []string{}
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
	data, err := fetchPDF(req.URL)
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

// fetchPDF downloads a PDF with a size cap and timeout.
func fetchPDF(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errStatus(resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxPDFBytes))
}
