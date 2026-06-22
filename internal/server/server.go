// Package server is Nib's local HTTP server: it serves the embedded UI and a
// small JSON API for opening, fetching, and saving the current PDF.
//
// The interactive fill itself happens in the browser via pdf.js — its
// saveDocument() writes the user's form edits back into the PDF and hands the
// finished bytes to /api/save, which persists them. The server's PDF role in M1
// is therefore just I/O plus signature verification; pdfcpu-side operations
// (stamp, flatten, sign, headless fill) arrive in later milestones.
package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/vault"
)

// maxPDFBytes caps how large a PDF the server will read or accept on save.
const maxPDFBytes = 200 << 20 // 200 MiB

// document is the single PDF currently open in the session.
type document struct {
	// path is the local file the document was opened from. Empty when the
	// document was uploaded through the browser (no server-side path), in which
	// case it cannot be saved in place.
	path string
	data []byte
	sig  sign.Status
}

// Server holds the embedded UI, the auth session, and the current document.
type Server struct {
	web       fs.FS
	legal     fs.FS  // LICENSE + THIRD-PARTY-NOTICES.md, served read-only for the About dialog
	configDir string // where the vault lives (os.UserConfigDir()/nib)
	version   string // running build version, reported by the update check and the About dialog

	setupMu sync.Mutex // serializes first-run vault setup so AutoSetup runs once

	mu    sync.Mutex
	vault *vault.Vault // unlocked vault, nil until the SSH key unlocks it
	csrf  string       // per-process CSRF token, issued when the vault unlocks
	doc   *document    // current open PDF

	sess session // armed live co-signing listener (Nib's one routable surface)
}

// New returns a Server serving the given UI asset tree (and licence texts), with
// its vault in configDir. version is the running build, surfaced by the update
// check and the About dialog.
func New(web, legal fs.FS, configDir, version string) *Server {
	return &Server{web: web, legal: legal, configDir: configDir, version: version}
}

// Handler builds the HTTP routes. Status and key enrollment/migration are public
// so the UI can run its first-run wizard; every document and vault route is gated
// behind an unlocked vault. API patterns take precedence over the static files.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public — reachable before the vault is unlocked.
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/update/check", s.handleUpdateCheck)
	mux.HandleFunc("POST /api/ssh/enroll", requirePublicLoopback(s.handleEnroll))
	mux.HandleFunc("POST /api/ssh/migrate", requirePublicLoopback(s.handleMigrate))

	// Protected — require the vault unlocked (+ CSRF on writes).
	mux.HandleFunc("GET /api/vault/export", s.requireUnlocked(s.handleVaultExport))
	mux.HandleFunc("POST /api/vault/import", s.requireUnlocked(s.handleVaultImport))
	mux.HandleFunc("GET /api/ssh/keys", s.requireUnlocked(s.handleKeysList))
	mux.HandleFunc("POST /api/ssh/keys", s.requireUnlocked(s.handleKeysAdd))
	mux.HandleFunc("POST /api/ssh/keys/remove", s.requireUnlocked(s.handleKeysRemove))
	mux.HandleFunc("GET /api/peers", s.requireUnlocked(s.handlePeersList))
	mux.HandleFunc("POST /api/peers/pin", s.requireUnlocked(s.handlePeersPin))
	mux.HandleFunc("POST /api/peers/remove", s.requireUnlocked(s.handlePeersRemove))
	mux.HandleFunc("POST /api/cosign/quote", s.requireUnlocked(s.handleCosignQuote))
	mux.HandleFunc("POST /api/cosign/sign", s.requireUnlocked(s.handleCosignSign))
	mux.HandleFunc("GET /api/attestations", s.requireUnlocked(s.handleAttestations))
	mux.HandleFunc("POST /api/session/arm", s.requireUnlocked(s.handleSessionArm))
	mux.HandleFunc("POST /api/session/disarm", s.requireUnlocked(s.handleSessionDisarm))
	mux.HandleFunc("GET /api/session/status", s.requireUnlocked(s.handleSessionStatus))
	mux.HandleFunc("GET /api/session/pending-pdf", s.requireUnlocked(s.handleSessionPendingPDF))
	mux.HandleFunc("POST /api/session/respond", s.requireUnlocked(s.handleSessionRespond))
	mux.HandleFunc("POST /api/session/quote", s.requireUnlocked(s.handleSessionQuote))
	mux.HandleFunc("POST /api/session/initiate", s.requireUnlocked(s.handleSessionInitiate))
	mux.HandleFunc("POST /api/session/send", s.requireUnlocked(s.handleSessionSend))
	mux.HandleFunc("POST /api/open", s.requireUnlocked(s.handleOpen))
	mux.HandleFunc("POST /api/open-url", s.requireUnlocked(s.handleOpenURL))
	mux.HandleFunc("GET /api/recent", s.requireUnlocked(s.handleRecent))
	mux.HandleFunc("POST /api/upload", s.requireUnlocked(s.handleUpload))
	mux.HandleFunc("POST /api/combine", s.requireUnlocked(s.handleCombine))
	mux.HandleFunc("GET /api/pdf", s.requireUnlocked(s.handlePDF))
	mux.HandleFunc("GET /api/doc", s.requireUnlocked(s.handleDoc))
	mux.HandleFunc("POST /api/save", s.requireUnlocked(s.handleSave))
	mux.HandleFunc("GET /api/listdir", s.requireUnlocked(s.handleListDir))
	mux.HandleFunc("POST /api/write", s.requireUnlocked(s.handleWriteFile))
	mux.HandleFunc("POST /api/bake", s.requireUnlocked(s.handleBake))
	mux.HandleFunc("POST /api/flags", s.requireUnlocked(s.handleFlags))

	// Page operations.
	mux.HandleFunc("POST /api/pages", s.requireUnlocked(s.handlePages))
	mux.HandleFunc("POST /api/redact", s.requireUnlocked(s.handleRedact))
	mux.HandleFunc("GET /api/outline", s.requireUnlocked(s.handleOutlineGet))
	mux.HandleFunc("POST /api/outline", s.requireUnlocked(s.handleOutlineSet))

	// Hidden-content scan and sanitize.
	mux.HandleFunc("GET /api/scan", s.requireUnlocked(s.handleScan))
	mux.HandleFunc("POST /api/sanitize", s.requireUnlocked(s.handleSanitize))
	mux.HandleFunc("POST /api/decrypt", s.requireUnlocked(s.handleDecrypt))
	mux.HandleFunc("GET /api/attachments", s.requireUnlocked(s.handleAttachmentsList))
	mux.HandleFunc("POST /api/attachments/add", s.requireUnlocked(s.handleAttachmentAdd))
	mux.HandleFunc("POST /api/attachments/extract", s.requireUnlocked(s.handleAttachmentExtract))

	// Finalize / export / autofill.
	mux.HandleFunc("POST /api/finalize", s.requireUnlocked(s.handleFinalize))
	mux.HandleFunc("POST /api/timestamp", s.requireUnlocked(s.handleTimestamp))
	mux.HandleFunc("POST /api/timestamp/verify", s.requireUnlocked(s.handleTimestampVerify))
	mux.HandleFunc("GET /api/identity", s.requireUnlocked(s.handleIdentity))
	mux.HandleFunc("POST /api/assemble", s.requireUnlocked(s.handleAssemble))
	mux.HandleFunc("POST /api/extract", s.requireUnlocked(s.handleExtract))
	mux.HandleFunc("POST /api/extract-images", s.requireUnlocked(s.handleExtractImages))
	mux.HandleFunc("POST /api/split-bookmarks", s.requireUnlocked(s.handleSplitBookmarks))
	mux.HandleFunc("POST /api/split-pages", s.requireUnlocked(s.handleSplitPages))
	mux.HandleFunc("GET /api/form-data", s.requireUnlocked(s.handleFormData))
	mux.HandleFunc("GET /api/profile", s.requireUnlocked(s.handleProfileGet))
	mux.HandleFunc("POST /api/profile", s.requireUnlocked(s.handleProfileSet))
	mux.HandleFunc("POST /api/settings", s.requireUnlocked(s.handleSettings))

	// Image library.
	mux.HandleFunc("GET /api/images", s.requireUnlocked(s.handleImagesList))
	mux.HandleFunc("POST /api/images", s.requireUnlocked(s.handleImageAdd))
	mux.HandleFunc("GET /api/images/{id}", s.requireUnlocked(s.handleImageGet))
	mux.HandleFunc("DELETE /api/images/{id}", s.requireUnlocked(s.handleImageDelete))

	// Licence texts (public, read-only) — the About dialog fetches these.
	mux.Handle("GET /legal/", http.StripPrefix("/legal/", http.FileServerFS(s.legal)))

	// Static UI (public — it renders the first-run wizard).
	mux.Handle("/", http.FileServerFS(s.web))
	return loopbackOnly(mux)
}

// --- API: open / upload -------------------------------------------------------

type openRequest struct {
	Path string `json:"path"`
}

// docResponse is the metadata returned after a document is opened or saved.
// The PDF bytes themselves are fetched separately from /api/pdf.
type docResponse struct {
	Name      string          `json:"name"`
	Path      string          `json:"path"`            // empty => upload origin, no in-place save
	CanSave   bool            `json:"canSave"`         // true when a save would overwrite Path
	Signature sign.Status     `json:"signature"`       // untampered / modified / unsigned
	Flags     json.RawMessage `json:"flags,omitempty"` // embedded sign/date/initial placeholders, if any
}

// handleOpen loads a PDF from a server-side path. Opening by path is what makes
// in-place Save possible (the browser file-picker can't reveal a real path).
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var req openRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	path := filepath.Clean(req.Path)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		httpError(w, http.StatusNotFound, "file not found")
		return
	}
	if info.Size() > maxPDFBytes {
		httpError(w, http.StatusRequestEntityTooLarge, "PDF exceeds size limit")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read file")
		return
	}
	s.setDoc(&document{path: path, data: data, sig: sign.Verify(data)})
	_ = vaultFrom(r).AddRecent(path) // best-effort; failure to record is non-fatal
	writeJSON(w, s.docResponse())
}

// handleUpload accepts a PDF posted from the browser file-picker. Such a
// document has no server-side path, so it can be filled and re-downloaded but
// not saved in place.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPDFBytes)
	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, http.StatusBadRequest, "no file uploaded")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	s.setDoc(&document{path: "", data: data, sig: sign.Verify(data)})
	resp := s.docResponse()
	resp.Name = header.Filename
	writeJSON(w, resp)
}

// --- API: pdf bytes / save ----------------------------------------------------

// handlePDF streams the current document's bytes for pdf.js to render.
func (s *Server) handlePDF(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(doc.data)
}

// handleSave persists filled PDF bytes produced by pdf.js saveDocument(). For a
// path-opened document it overwrites the original in place (the AcroForm case —
// non-destructive, fields stay editable). Upload-origin documents have no path
// and are rejected here; Save-As lands in a later milestone.
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return
	}
	if doc.path == "" {
		httpError(w, http.StatusConflict, "document has no path; use Save As")
		return
	}

	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPDFBytes))
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read body")
		return
	}
	if err := writeFileAtomic(doc.path, data); err != nil {
		httpError(w, http.StatusInternalServerError, "could not write file")
		return
	}

	sig := sign.Verify(data)
	s.mu.Lock()
	doc.data = data
	doc.sig = sig
	s.mu.Unlock()
	writeJSON(w, s.docResponse())
}

// --- helpers ------------------------------------------------------------------

func (s *Server) setDoc(doc *document) {
	s.mu.Lock()
	s.doc = doc
	s.mu.Unlock()
}

// docResponse builds the metadata response for the current document. It
// takes the lock itself; callers must not hold it.
func (s *Server) docResponse() docResponse {
	s.mu.Lock()
	doc := s.doc
	s.mu.Unlock()
	if doc == nil {
		return docResponse{}
	}
	resp := docResponse{
		Name:      filepath.Base(doc.path),
		Path:      doc.path,
		CanSave:   doc.path != "",
		Signature: doc.sig,
	}
	// Surface embedded signing placeholders so the recipient's UI can rebuild
	// them on open (the read half of the flag round-trip; the write half is
	// /api/flags). A parse failure just means "no flags" — never blocks the open.
	if flags, err := pdfops.FlagsJSON(doc.data); err == nil && json.Valid(flags) {
		resp.Flags = flags
	}
	return resp
}

// writeFileAtomic writes data to a temp file in the same directory then renames
// it over path, so an interrupted save can't leave a half-written PDF.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".nib-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// loopbackOnly admits a request only when both the connecting peer and the
// requested Host are loopback. The peer-IP check is an unspoofable socket fact
// that self-enforces the loopback bind; the Host allowlist defends against DNS
// rebinding, where the socket is loopback but the browser carries an attacker's
// origin. The bind is the primary control — these are defence in depth, layered
// on top of the per-request CSRF + origin checks the unlocked routes apply.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !peerIsLoopback(r.RemoteAddr) || !hostIsLoopback(r.Host) {
			httpError(w, http.StatusForbidden, "loopback only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// peerIsLoopback reports whether the connection's remote address is a loopback IP.
func peerIsLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostIsLoopback reports whether the request's Host header names the loopback
// interface — the set nib binds and accepts, kept in sync with loopbackBind.
func hostIsLoopback(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
