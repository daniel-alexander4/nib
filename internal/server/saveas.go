package server

// Save-As / Export destination support: a directory browser (handleListDir) and
// a writer (handleWriteFile) that back the UI's save dialog. Flatten, editable
// save, finalize, and the ZIP/PNG exports all let the user pick a name and a
// folder; the default folder is ~/nib. The browser can't choose a folder on
// its own (Chromium --app has no save dialog), so the server writes the bytes.

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// expandHome turns a leading "~" or "~/" into the user's home directory, so the
// UI can pass friendly paths like "~/nib".
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p[1:], "/"))
		}
	}
	return p
}

// defaultOutputDir is where flattened/exported files land unless the user picks
// another folder: ~/nib.
func defaultOutputDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "nib"
	}
	return filepath.Join(home, "nib")
}

type listDirResponse struct {
	Path   string   `json:"path"`   // the (absolute) folder being listed
	Parent string   `json:"parent"` // its parent, or "" at the filesystem root
	Dirs   []string `json:"dirs"`   // sub-directory names, sorted, dotfiles hidden
	Files  []string `json:"files"`  // PDF file names, sorted — for the open browser
}

// handleListDir lists a folder's sub-directories (and PDF files) so the save and
// open dialogs can navigate. An empty path defaults to ~/nib. A folder that
// doesn't exist yet (e.g. ~/nib before the first save) lists as empty rather
// than erroring — the write step creates it.
func (s *Server) handleListDir(w http.ResponseWriter, r *http.Request) {
	p := expandHome(strings.TrimSpace(r.URL.Query().Get("path")))
	if p == "" {
		p = defaultOutputDir()
	}
	p = filepath.Clean(p)
	resp := listDirResponse{Path: p, Dirs: []string{}, Files: []string{}}
	if parent := filepath.Dir(p); parent != p {
		resp.Parent = parent
	}
	if entries, err := os.ReadDir(p); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if e.IsDir() {
				resp.Dirs = append(resp.Dirs, name)
			} else if strings.EqualFold(filepath.Ext(name), ".pdf") {
				resp.Files = append(resp.Files, name)
			}
		}
		sort.Strings(resp.Dirs)
		sort.Strings(resp.Files)
	}
	writeJSON(w, resp)
}

// handleWriteFile writes posted bytes to a chosen absolute path, creating parent
// directories as needed. Multipart: a "data" file part plus a "path" field.
func (s *Server) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	target := filepath.Clean(expandHome(strings.TrimSpace(r.FormValue("path"))))
	if target == "" || target == "." {
		httpError(w, http.StatusBadRequest, "no destination path")
		return
	}
	if !filepath.IsAbs(target) {
		httpError(w, http.StatusBadRequest, "destination must be an absolute path")
		return
	}
	file, _, err := r.FormFile("data")
	if err != nil {
		httpError(w, http.StatusBadRequest, "no data")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read data")
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		httpError(w, http.StatusInternalServerError, "could not create folder")
		return
	}
	if err := writeFileAtomic(target, data); err != nil {
		httpError(w, http.StatusInternalServerError, "could not write file")
		return
	}
	writeJSON(w, map[string]string{"path": target})
}
