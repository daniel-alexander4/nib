package server

// Save-As / Export destination support: a directory browser (handleListDir) and
// a writer (handleWriteFile) that back the UI's save dialog. Flatten, editable
// save, finalize, and the ZIP/PNG exports all let the user pick a name and a
// folder; the default folder is ~/nib. The browser can't choose a folder on
// its own (Chromium --app has no save dialog), so the server writes the bytes.

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"nib/internal/atomicfile"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// expandHome turns a leading "~" into the user's home directory, so the UI can
// pass friendly paths like "~/nib". Either separator is accepted after the tilde:
// a Windows user types "~\Documents", and os.IsPathSeparator is the OS's own
// answer to what counts as one — on POSIX it accepts only "/", so behaviour there
// is exactly what it always was.
func expandHome(p string) string {
	if p != "~" && !(len(p) > 1 && p[0] == '~' && os.IsPathSeparator(p[1])) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
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

// dirEntry is one browsable child of the listed folder: the name to show, and the
// full path to navigate or open with. The server builds that path because only it
// knows the separator — the browser used to join with "/", which on Windows
// produced C:\Users\bob/file.pdf and made the client a second, drifting authority
// on what a path looks like.
type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type listDirResponse struct {
	Path   string     `json:"path"`             // the (absolute) folder being listed
	Parent string     `json:"parent"`           // its parent, or "" at a filesystem root
	Roots  []string   `json:"roots,omitempty"`  // other drives to jump to, at a root; empty off Windows
	Reason string     `json:"reason,omitempty"` // why it came back empty: missing | denied | notdir | unreadable
	Dirs   []dirEntry `json:"dirs"`             // sub-directories, sorted, dotfiles hidden
	Files  []dirEntry `json:"files"`            // PDF files, sorted — for the open browser
}

// handleListDir lists a folder's sub-directories (and PDF files) so the save and
// open dialogs can navigate. An empty path defaults to ~/nib.
//
// A folder that can't be read reports Reason rather than passing for an empty
// one — the two were indistinguishable, so a denied share, a dead drive letter
// and a typo all looked like "this folder has nothing in it". The single
// exception is the save dialog's first open, which asks for ~/nib before it
// exists by sending no path at all; that one is expected and stays quiet, since
// the write step creates the folder.
func (s *Server) handleListDir(w http.ResponseWriter, r *http.Request) {
	asked := strings.TrimSpace(r.URL.Query().Get("path"))
	p := expandHome(asked)
	if p == "" {
		p = defaultOutputDir()
	}
	p = filepath.Clean(p)
	resp := listDirResponse{Path: p, Dirs: []dirEntry{}, Files: []dirEntry{}}
	if parent := filepath.Dir(p); parent != p {
		resp.Parent = parent
	} else {
		resp.Roots = browseRoots() // the walk ran out — offer the other drives
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		if asked != "" {
			resp.Reason = listDirReason(p, err)
		}
		writeJSON(w, resp)
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		child := dirEntry{Name: name, Path: filepath.Join(p, name)}
		if e.IsDir() {
			resp.Dirs = append(resp.Dirs, child)
		} else if strings.EqualFold(filepath.Ext(name), ".pdf") {
			resp.Files = append(resp.Files, child)
		}
	}
	sort.Slice(resp.Dirs, func(i, j int) bool { return resp.Dirs[i].Name < resp.Dirs[j].Name })
	sort.Slice(resp.Files, func(i, j int) bool { return resp.Files[i].Name < resp.Files[j].Name })
	writeJSON(w, resp)
}

// listDirReason classifies a failed listing as a short token the UI turns into a
// sentence — the same split decryptResponse uses, so the wording stays in one
// place (the browser) and the server stays machine-readable.
//
// The stat comes first deliberately. Asked to list a file, Linux fails with
// ENOTDIR but Windows fails with ENOENT, so an errno-first order reports an
// existing file as "missing" — and a Linux-only test never sees it, because
// Linux is the one platform that answers correctly either way.
func listDirReason(path string, err error) string {
	if info, serr := os.Stat(path); serr == nil && !info.IsDir() {
		return "notdir"
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "missing"
	case errors.Is(err, fs.ErrPermission):
		return "denied"
	}
	return "unreadable"
}

// handleWriteFile writes posted bytes into a chosen folder, creating it as
// needed. Multipart: a "data" file part plus "dir" and "name" fields.
//
// The folder and the file name arrive separately and are joined here, because
// only the server knows the separator. Joining server-side is also what lets the
// name be contained to the folder: it is free text the user typed, so "../x"
// must not walk out of the directory the dialog says it is writing to. The split
// writer has always guarded its title-derived names this way; this one did not,
// and a name of "../../out.pdf" escaped silently with a 200.
func (s *Server) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	dir, ok := destDir(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		httpError(w, http.StatusBadRequest, "no file name")
		return
	}
	target, ok := containedJoin(dir, name)
	if !ok {
		httpError(w, http.StatusBadRequest, "unsafe file name")
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
	if err := os.MkdirAll(dir, 0o755); err != nil { // containment guarantees Dir(target) == dir
		httpError(w, http.StatusInternalServerError, "could not create folder")
		return
	}
	// The only copy: the signature on it cannot be re-derived identically, so a save that
	// reports success must mean the bytes are on disk (atomicfile.WriteDurable's own rule).
	if err := atomicfile.WriteDurable(target, data, 0o600); err != nil {
		httpError(w, http.StatusInternalServerError, "could not write file")
		return
	}
	writeJSON(w, map[string]string{"path": target})
}
