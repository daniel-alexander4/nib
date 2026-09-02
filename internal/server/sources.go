package server

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"net/url"
	"nib/internal/pdfops"
	"nib/internal/sign"
	"path"
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

// urlDocName is the display name for a document fetched by URL: the last path segment, or a
// plain fallback when the URL has none (a bare host, or a path ending in a slash).
func urlDocName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "downloaded.pdf"
	}
	base := path.Base(u.Path)
	if base == "." || base == "/" || base == "" {
		return "downloaded.pdf"
	}
	return base
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
	if !pdfops.LooksLikePDF(data) {
		httpError(w, http.StatusUnsupportedMediaType, "URL did not return a PDF")
		return
	}
	// The name comes from the URL's own path, so an opened-by-URL document is not "Untitled"
	// after a reload. Three of the five path-less producers were given a name and this was
	// one of the two that were not — see document.name.
	installed, cerr := s.addDocCapped(&document{
		path: "", name: urlDocName(req.URL), data: data, sig: sign.Verify(data),
	})
	if cerr != nil {
		httpError(w, http.StatusConflict, cerr.Error())
		return
	}
	writeJSON(w, s.docResponse(installed))
}
