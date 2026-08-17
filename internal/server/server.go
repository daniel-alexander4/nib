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
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/vault"
)

// maxPDFBytes caps how large a PDF the server will read or accept on save.
const maxPDFBytes = 200 << 20 // 200 MiB

// parseMultipart caps the request body at max, parses the multipart form, and
// hands back a cleanup that removes any parts the parser spilled to temp files —
// net/http never removes those itself, so skipping the cleanup leaks disk until
// the process exits. On failure it writes the 400 and returns ok=false.
//
//	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
//	if !ok {
//		return
//	}
//	defer cleanup()
func parseMultipart(w http.ResponseWriter, r *http.Request, max int64) (cleanup func(), ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	if err := r.ParseMultipartForm(max); err != nil {
		httpError(w, http.StatusBadRequest, "could not parse upload")
		return nil, false
	}
	return func() { _ = r.MultipartForm.RemoveAll() }, true
}

// docID names one open document. It is deliberately two fields rather than a
// packed integer, so a test can fail on one without the other.
//
// Seq is monotonic for the life of the process and is **never reused** — not
// reclaimed when a document closes, never an index into the registry. That is the
// whole of ADR-001's second half: the pinning law reduces to an id comparison, so
// a recycled id defeats it silently and completely (close document 3, open another
// that is also 3, and an operation pinned to the old one passes its check and
// commits to the new). Cheap to get right here; impossible to detect later,
// because the failure looks like a correct comparison.
//
// Epoch is a per-process nonce, and it is why Seq alone is not enough. Seq restarts
// at 1 when the process does. Normally unreachable — the default bind is
// 127.0.0.1:0, so a restart takes a new port and a surviving browser tab cannot
// reconnect — but NIB_ADDR pins a fixed port for headless runs (cmd/nib/main.go),
// and there a stale tab reconnects and can name an id the new process has since
// reassigned. Same corruption, crossing a process boundary.
type docID struct {
	Epoch string
	Seq   uint64
}

func (d docID) String() string { return d.Epoch + ":" + strconv.FormatUint(d.Seq, 10) }

// valid reports whether this id names anything. The zero docID never does: Seq
// starts at 1, so a zero-valued id is "unset" and can never collide with a real one.
func (d docID) valid() bool { return d.Seq != 0 && d.Epoch != "" }

// document is one open PDF in the session.
type document struct {
	// id is assigned once, at open, and never changes or repeats. See docID.
	id docID

	// path is the local file the document was opened from. Empty when the
	// document was uploaded through the browser (no server-side path), in which
	// case it cannot be saved in place.
	path string
	data []byte
	sig  sign.Status

	// undo/redo are this document's own history. They live here rather than on the
	// Server because a per-server ring means an operation on one document pops a
	// state belonging to another — the same class of defect as an unpinned
	// operation, arriving through the history instead of through a route (D8).
	//
	// The byte budget that bounds them is NOT per-document: see trimHistoryLocked.
	undo [][]byte // prior document states, oldest first
	redo [][]byte // states rolled back by undo

	// historyEvicted records that this document's history was dropped to keep the
	// global budget, rather than never having existed. Without it the two are
	// indistinguishable on the wire — `canUndo:false` reads identically for "you
	// have made no edits" and "your edits are no longer undoable" — which is the
	// silent eviction the plan-review pin refuses. Sticky: it describes something
	// that happened to the document, and it is cleared only by a barrier or a
	// fresh history.
	historyEvicted bool
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

	// The document registry. Today it holds at most one entry and every route
	// acts on the active id, so behaviour is identical to the single `doc` field
	// this replaces — the registry exists so that P03.S02 can put ids on the wire
	// and later phases can hold several. Reached through activeDoc()/activeDocLocked()
	// and never read directly, which is what keeps the ~16 call sites' guard shape
	// (`doc := …; if doc == nil`) intact and the diff reviewable.
	docs     []*document // open documents, in open order
	activeID docID       // the document routes act on unless told otherwise
	nextSeq  uint64      // monotonic; incremented per open, never reset or reclaimed
	epoch    string      // per-process nonce; see docID

	// maxHistoryBytes overrides the global history budget. Zero means maxUndoBytes,
	// which is what production always uses; only the eviction tests set it, so they
	// can exercise the budget with kilobytes rather than a quarter of a gigabyte.
	maxHistoryBytes int

	// historyEvictions counts documents whose history has been dropped to keep the
	// global byte budget. Class-1: read by the eviction tests, and the only place a
	// silent eviction would show up as a number rather than as a missing effect.
	historyEvictions uint64

	sess session // armed live co-signing listener (Nib's one routable surface)
}

// New returns a Server serving the given UI asset tree (and licence texts), with
// its vault in configDir. version is the running build, surfaced by the update
// check and the About dialog.
func New(web, legal fs.FS, configDir, version string) *Server {
	// Install the vendored non-Latin OCR fonts now, before any request can trigger
	// pdfcpu's once-only font-registry load (see pdfops.InstallOCRFonts). A failure
	// only costs Thai/Devanagari OCR, so log and carry on rather than refuse to boot.
	if err := pdfops.InstallOCRFonts(); err != nil {
		log.Printf("server: could not install OCR fonts: %v", err)
	}
	// The epoch is per-process and is minted before anything can open a document,
	// so no id can ever be issued without one.
	return &Server{web: web, legal: legal, configDir: configDir, version: version, epoch: newToken()}
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
	mux.HandleFunc("POST /api/ssh/unlock", requirePublicLoopback(s.handleUnlock))

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
	mux.HandleFunc("GET /api/docs", s.requireUnlocked(s.handleDocs))
	mux.HandleFunc("POST /api/close", s.requireUnlocked(s.handleClose))
	mux.HandleFunc("POST /api/close-view", s.requireUnlocked(s.handleCloseView))
	mux.HandleFunc("POST /api/save", s.requireUnlocked(s.handleSave))
	mux.HandleFunc("GET /api/listdir", s.requireUnlocked(s.handleListDir))
	mux.HandleFunc("POST /api/write", s.requireUnlocked(s.handleWriteFile))
	mux.HandleFunc("POST /api/bake", s.requireUnlocked(s.handleBake))
	mux.HandleFunc("POST /api/flags", s.requireUnlocked(s.handleFlags))
	mux.HandleFunc("POST /api/form/author", s.requireUnlocked(s.handleFormAuthor))
	mux.HandleFunc("POST /api/form/fill-csv", s.requireUnlocked(s.handleFormFillCSV))
	mux.HandleFunc("POST /api/form/fill-xfdf", s.requireUnlocked(s.handleFormFillXFDF))

	// Page operations.
	mux.HandleFunc("POST /api/pages", s.requireUnlocked(s.handlePages))
	mux.HandleFunc("POST /api/redact", s.requireUnlocked(s.handleRedact))
	mux.HandleFunc("POST /api/ocr", s.requireUnlocked(s.handleOCR))
	mux.HandleFunc("GET /api/outline", s.requireUnlocked(s.handleOutlineGet))
	mux.HandleFunc("POST /api/outline", s.requireUnlocked(s.handleOutlineSet))

	// Undo / redo of document-state operations.
	mux.HandleFunc("POST /api/undo", s.requireUnlocked(s.handleUndo))
	mux.HandleFunc("POST /api/redo", s.requireUnlocked(s.handleRedo))

	// Hidden-content scan and sanitize.
	mux.HandleFunc("GET /api/scan", s.requireUnlocked(s.handleScan))
	mux.HandleFunc("POST /api/sanitize", s.requireUnlocked(s.handleSanitize))
	mux.HandleFunc("POST /api/encrypt", s.requireUnlocked(s.handleEncrypt))
	mux.HandleFunc("POST /api/decrypt", s.requireUnlocked(s.handleDecrypt))
	mux.HandleFunc("GET /api/attachments", s.requireUnlocked(s.handleAttachmentsList))
	mux.HandleFunc("POST /api/attachments/add", s.requireUnlocked(s.handleAttachmentAdd))
	mux.HandleFunc("POST /api/attachments/extract", s.requireUnlocked(s.handleAttachmentExtract))

	// Finalize / export / autofill.
	mux.HandleFunc("POST /api/finalize", s.requireUnlocked(s.handleFinalize))
	mux.HandleFunc("POST /api/timestamp", s.requireUnlocked(s.handleTimestamp))
	mux.HandleFunc("POST /api/timestamp/verify", s.requireUnlocked(s.handleTimestampVerify))
	mux.HandleFunc("GET /api/identity", s.requireUnlocked(s.handleIdentity))
	mux.HandleFunc("GET /api/identity/external", s.requireUnlocked(s.handleExternalSignerGet))
	mux.HandleFunc("POST /api/identity/external", s.requireUnlocked(s.handleExternalSignerImport))
	mux.HandleFunc("POST /api/identity/external/remove", s.requireUnlocked(s.handleExternalSignerRemove))
	mux.HandleFunc("POST /api/assemble", s.requireUnlocked(s.handleAssemble))
	mux.HandleFunc("POST /api/optimize", s.requireUnlocked(s.handleOptimize))
	mux.HandleFunc("POST /api/pdfa", s.requireUnlocked(s.handlePDFA))
	mux.HandleFunc("POST /api/office", s.requireUnlocked(s.handleOffice))
	mux.HandleFunc("POST /api/table", s.requireUnlocked(s.handleTableExport))
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
	// ID names this document on the wire — the client echoes it back on every
	// document-touching request. Without it the client has nothing to send and
	// every request silently takes docFor's compatibility default, which is the
	// unpinned path D7 exists to prevent. Omitted when nothing is open, so the
	// empty docResponse stays byte-identical to what it was before.
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Path      string          `json:"path"`            // empty => upload origin, no in-place save
	CanSave   bool            `json:"canSave"`         // true when a save would overwrite Path
	Signature sign.Status     `json:"signature"`       // untampered / modified / unsigned
	Flags     json.RawMessage `json:"flags,omitempty"` // embedded sign/date/initial placeholders, if any
	CanUndo   bool            `json:"canUndo"`         // an undoable operation is on the stack
	CanRedo   bool            `json:"canRedo"`         // an undone operation can be re-applied

	// HistoryEvicted distinguishes "your edit history was dropped to free memory"
	// from "you have not edited this document" — states that canUndo:false reports
	// identically. The plan-review pin requires eviction to be OBSERVABLE, and the
	// effect alone is not an observation: a user switching back to a document would
	// find an empty undo stack and no account of where it went. Omitted while false,
	// so a document that has never been evicted serializes exactly as before.
	HistoryEvicted bool `json:"historyEvicted,omitempty"`
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
	// A path-opened document reports canSave, so a non-PDF opened here would be
	// overwritten with PDF bytes by the next Save — the one open surface where
	// getting this wrong destroys the file.
	if !pdfops.LooksLikePDF(data) {
		httpError(w, http.StatusUnsupportedMediaType, "that file isn't a PDF")
		return
	}
	// Adds rather than replaces (P06.S01): opening a second file leaves the first
	// open and reachable through the tab strip. Before this, setDoc emptied the whole
	// registry while the client re-pointed only the ACTIVE view — so with an arrival
	// open, an Open left the second view rendering a document the server no longer
	// held, every pinned request against it a 409 with the pages still on screen.
	installed, err := s.addDocCapped(&document{path: path, data: data, sig: sign.Verify(data)})
	if err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	_ = vaultFrom(r).AddRecent(path) // best-effort; failure to record is non-fatal
	writeJSON(w, s.docResponse(installed))
}

// handleUpload accepts a PDF posted from the browser file-picker. Such a
// document has no server-side path, so it can be filled and re-downloaded but
// not saved in place.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
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
	// No path here, so nothing can be overwritten — but an unreadable "document"
	// still leaves the viewer stuck on "Loading…" with no explanation.
	if !pdfops.LooksLikePDF(data) {
		httpError(w, http.StatusUnsupportedMediaType, "that file isn't a PDF")
		return
	}
	installed, err := s.addDocCapped(&document{path: "", data: data, sig: sign.Verify(data)})
	if err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	resp := s.docResponse(installed)
	resp.Name = header.Filename
	writeJSON(w, resp)
}

// handleClose puts the open document down without quitting: setDoc(nil) drops the
// bytes and clears both history rings, leaving the server in exactly the state it
// launches in. It is deliberately idempotent — closing with nothing open is a 200
// and the same empty response, on the ErrNotEncrypted-passes-through precedent
// (v1.57.0), since the only way to reach it is a race against a disabled control.
//
// Note what this makes newly possible: until now s.doc was nil only before the
// first open, so no reader had to tolerate nil at an arbitrary moment. Now every
// reader must, and every reader does — each nil-checks before dereferencing, and
// the two in undo.go that dereference without their own check do so under a
// single lock hold that checked nil first.
func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	// Addressed like any other document route: naming a document this server does
	// not hold is a 409, not a silent close of whatever happens to be active.
	if _, err := s.docFor(r); err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	s.setDoc(nil)
	// The zero response, built from a nil document rather than from "nothing is
	// active".
	//
	// NOT because other documents stay open — they do not. setDoc(nil) clears the
	// whole registry, so a close is a CLOSE ALL, matching the client's own
	// closeDocument ("Close is CLOSE ALL"). This comment used to assert the
	// opposite in so many words, positioned exactly where a reader would look for
	// the guarantee. Per-document close is P06's, with the tab strip.
	//
	// The reason the response is built from nil is narrower: re-resolving "active"
	// after a close would describe whatever the registry answers next, which is not
	// the document the user just closed.
	writeJSON(w, s.docResponse(nil))
}

// docsResponse is the whole open set: every document in registry order, plus which one
// is active.
//
// **A list plus an id, mirroring the server's own model**, rather than a flat list with
// an `active: true` on one entry. The registry IS an ordered slice and an active id, so
// this shape cannot represent a state the server cannot be in — where a flag on each
// entry can say two documents are active, or none, and the client would have to decide
// which lie to believe.
type docsResponse struct {
	Docs     []docResponse `json:"docs"`
	ActiveID string        `json:"activeId,omitempty"`
}

// handleDocs answers what the server currently holds. It is the client's only way to
// learn that, and until P06.S03 there was no such route and no caller for one: the
// client asked "what is the ACTIVE document" (`/api/doc`) from exactly one place, the
// co-sign arrival poll, and never asked what else was open.
//
// That gap is why a reload came back showing zero documents while the server still held
// them, and why a path-less document — an upload, a combine, an office conversion, an
// arrival — was unreachable for the rest of the process's life once the page reloaded:
// the only way back to a document was to open it by path, and those have none.
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	docs := append([]*document(nil), s.docs...)
	active := s.activeID
	s.mu.Unlock()

	// docResponse takes the lock itself, so the snapshot above is released first. A
	// document closed in between yields its zero response and is filtered out — the
	// list describes what was open, not what the caller hoped was.
	out := docsResponse{Docs: make([]docResponse, 0, len(docs))}
	if active.valid() {
		out.ActiveID = active.String()
	}
	for _, d := range docs {
		if dr := s.docResponse(d); dr.ID != "" {
			out.Docs = append(out.Docs, dr)
		}
	}
	writeJSON(w, out)
}

// handleCloseView closes the ADDRESSED document and leaves the others open. Its
// response describes whatever is active afterwards, which is how the client learns
// which tab to move to (see removeDoc).
//
// **A separate route from /api/close rather than a new meaning for it.** Re-pointing
// /api/close at one document was the first shape and it is refused: seven tests assert
// that route empties the registry, and changing what a shipped route means underneath
// its tests is how a green suite stops describing the code. The destructive operation
// keeps its established name.
//
// **And there is deliberately no "omit the id to close everything".** D15 makes a
// missing id default to the ACTIVE document, so that shape would make the most
// destructive operation in the app the one that happens when a header is forgotten —
// which is verbatim the plan-review critical P03 was given, arriving through the close
// route instead of through a mutation. Omitting the id here closes the active document,
// the same narrow default every other route has.
func (s *Server) handleCloseView(w http.ResponseWriter, r *http.Request) {
	doc, err := s.docFor(r)
	if err != nil || doc == nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	next := s.removeDoc(doc)
	writeJSON(w, s.docResponse(next))
}

// --- API: pdf bytes / save ----------------------------------------------------

// handlePDF streams the current document's bytes for pdf.js to render.
func (s *Server) handlePDF(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
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
	doc, ok := s.resolveDoc(w, r)
	if !ok {
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
	// Re-checked after the body read, which is the slow part and the whole window:
	// resolveDoc ran before it, so a close can land in between. commitMutation's
	// contract names this caller in so many words — "a caller that tested the
	// document first would leave a window for a close to land in between, which is
	// the very defect" — and handleSave does not route through it, so it inherited
	// the defect rather than the guard. Without this, a save addressed to a closed
	// document still wrote the file and still answered 200 with an EMPTY
	// docResponse: a success reply for work that was discarded.
	//
	// Refused before the write, not after. The document is gone, so this save is an
	// operation against something the server no longer holds (ADR-001) and it
	// should not reach the disk at all.
	if !s.isRegistered(doc) {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	if err := writeFileAtomic(doc.path, data); err != nil {
		httpError(w, http.StatusInternalServerError, "could not write file")
		return
	}

	sig := sign.Verify(data)
	s.mu.Lock()
	// The narrow remainder: a close landing during the file write itself. The bytes
	// are on disk — the user asked for them there — but the state must not be
	// recorded onto a document the registry has dropped, and the reply must say so
	// rather than describing a document that is not open.
	if !s.isRegisteredLocked(doc) {
		s.mu.Unlock()
		httpError(w, http.StatusConflict, "that document was closed while it was being saved")
		return
	}
	doc.data = data
	doc.sig = sig
	s.mu.Unlock()
	writeJSON(w, s.docResponse(doc))
}

// --- helpers ------------------------------------------------------------------

// activeDoc returns the document routes act on, or nil when none is open. It
// takes the lock itself.
//
// This is the single accessor D6 asks for, and the reason it exists is the shape
// of its call sites rather than the lookup itself: ~16 handlers previously wrote
// `s.mu.Lock(); doc := s.doc; s.mu.Unlock(); if doc == nil {`. They now write
// `doc := s.activeDoc(); if doc == nil {` — the same guard, three fewer lines of
// ceremony, and one place to change when "active" stops meaning "the only one".
// A handler that reaches past this into s.docs is the one that breaks first.
func (s *Server) activeDoc() *document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeDocLocked()
}

// activeDocLocked is activeDoc for callers already holding s.mu — the undo/redo
// paths, which must test and mutate the document under one hold.
func (s *Server) activeDocLocked() *document {
	for _, d := range s.docs {
		if d.id == s.activeID {
			return d
		}
	}
	return nil
}

// errNoSuchDoc means the request named a document this server does not hold —
// closed, or issued by a previous process. Distinct from "nothing is open", and
// the distinction is the point: see resolveDoc.
var errNoSuchDoc = errors.New("no such document")

// parseDocID reads the wire form, "<epoch>:<seq>".
func parseDocID(raw string) (docID, bool) {
	epoch, seq, ok := strings.Cut(raw, ":")
	if !ok || epoch == "" {
		return docID{}, false
	}
	n, err := strconv.ParseUint(seq, 10, 64)
	if err != nil || n == 0 {
		return docID{}, false
	}
	return docID{Epoch: epoch, Seq: n}, true
}

// docFor resolves the document a request addresses: the X-Nib-Doc header, or a
// `doc` query parameter, or — when neither is given — the active document.
//
// The default is deliberate and narrow (D15). Around twenty existing Go tests and
// every `nib` CLI verb address one document by construction, and requiring an id
// would edit them all to say nothing new. But **the default is for those callers
// only**: the web client always sends an id, enforced in apiFetch rather than by
// discipline, because a call site that merely *forgets* the header would silently
// get "whatever is active" — which during a document switch is the wrong document,
// committed having passed no check. That enforcement is P03.S03's.
//
// Both carriers are accepted everywhere rather than the query parameter being
// special-cased to /api/pdf. D15's exception is about what the *client* sends —
// pdf.js owns that fetch and a header would mean opting into its plumbing — and a
// server that tolerates both needs no path-shaped branch to say so.
func (s *Server) docFor(r *http.Request) (*document, error) {
	raw := r.Header.Get("X-Nib-Doc")
	if raw == "" {
		raw = r.URL.Query().Get("doc")
	}
	if raw == "" {
		return s.activeDoc(), nil
	}
	id, ok := parseDocID(raw)
	if !ok {
		return nil, errNoSuchDoc
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Epoch first, and not as a style preference: an id from a previous process
	// carries a Seq from that process's counter, so a check that compared Seq
	// first would accept a stale id exactly whenever the two counters happened to
	// line up — which is the case this exists to refuse.
	if id.Epoch != s.epoch {
		return nil, errNoSuchDoc
	}
	for _, d := range s.docs {
		if d.id == id {
			return d, nil
		}
	}
	return nil, errNoSuchDoc
}

// resolveDoc is docFor for the thirteen handlers whose nil branch is the standard
// 404; it writes the refusal itself and reports whether the caller should carry on.
//
// The two statuses are different facts and the client branches on them: 404 is
// "nothing is open" and the app shows its empty state; 409 is "not that one" and
// the client drops a stale tab while leaving the rest alone. They therefore differ
// in body as well as status — a shared message would make them one fact to
// whoever reads the response.
func (s *Server) resolveDoc(w http.ResponseWriter, r *http.Request) (*document, bool) {
	doc, err := s.docFor(r)
	if err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return nil, false
	}
	if doc == nil {
		httpError(w, http.StatusNotFound, "no document open")
		return nil, false
	}
	return doc, true
}

// isRegisteredLocked reports whether doc is still one this server holds. Caller
// holds s.mu. Identity, not equality: a document that was closed and replaced is a
// different pointer even if its contents match.
// isRegistered is isRegisteredLocked for a caller that does not already hold the
// lock. Advisory by nature — the answer can be stale the instant it returns — so
// it belongs before expensive work, never as the only guard on a commit. The
// authoritative test is isRegisteredLocked, taken under the same hold as the write.
func (s *Server) isRegistered(doc *document) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRegisteredLocked(doc)
}

func (s *Server) isRegisteredLocked(doc *document) bool {
	for _, d := range s.docs {
		if d == doc {
			return true
		}
	}
	return false
}

// registerLocked mints the next id, installs doc in the registry and makes it
// active. Caller holds s.mu. It is the ONE place an id is issued, so ADR-001's
// never-reused law has a single site to hold rather than one per entry point — a
// second issuer would defeat the law by collision instead of by reuse.
func (s *Server) registerLocked(doc *document) {
	s.nextSeq++
	doc.id = docID{Epoch: s.epoch, Seq: s.nextSeq}
	s.docs = append(s.docs, doc)
	s.activeID = doc.id
}

// maxOpenDocs is D9's count cap: eight documents, refused rather than degrading.
//
// It is HALF of D9, and the half that needs no measurement. The decision is "count
// AND aggregate bytes, refusing on whichever binds first", with the byte figure
// chosen against a measurement in P06 — that is P06.S04's, and it is deliberately
// not guessed here. What lands now is the count, because P06.S01 is the change that
// makes unbounded growth reachable: before it, Open REPLACED, so a user's exposure
// was one document however many times they opened one. After it, Open adds, and
// twenty large scans are a keyboard shortcut. Shipping the thing that creates an
// exposure two slices ahead of the thing that bounds it is not a defensible order,
// so the free half travels with it.
const maxOpenDocs = 8

// ErrTooManyOpen refuses an open at the cap. A sentinel rather than a bare string so
// the route can answer 409 for this and 500 for anything else, and so the test can
// assert the specific refusal rather than being satisfied by any error at all.
//
// **The wording tracks what the UI can actually do.** In S01 it read "use Close, then
// reopen the ones you need", because Close was still close-ALL and telling a user to
// close ONE would have sent them looking for a control that did not exist. S02 adds
// Close view and the per-tab ×, so the instruction is now true and this line changed
// with the feature rather than after it.
var ErrTooManyOpen = errors.New("too many documents open (limit " + strconv.Itoa(maxOpenDocs) + ") — close one first")

// addDocCapped is addDoc for the five USER-initiated install routes: open, open-url,
// upload, combine, office. It refuses at maxOpenDocs.
//
// **The cap lives here and not in addDoc, and that distinction is the whole point.**
// addDoc's other callers are arrivals (D10) — a completed co-signature, whose bytes
// exist because a counterparty already did the work. Refusing an arrival at the cap
// does not decline a request; it DESTROYS a signed document that has no other home,
// since the session path installs it and nothing writes it to disk. A user opening a
// ninth file can close one and try again; a peer who has already signed cannot.
//
// **The test and the append happen under one lock hold**, for the reason
// commitMutation's contract comment gives one layer down: a caller that checked the
// count first would leave a window for a concurrent open to land in between, and two
// requests passing the check at seven both reach nine. One user with two browser
// panes is enough, which is exactly how the /api/doc race was reproduced.
func (s *Server) addDocCapped(doc *document) (*document, error) {
	if doc == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.docs) >= maxOpenDocs {
		return nil, ErrTooManyOpen
	}
	s.registerLocked(doc)
	return doc, nil
}

// addDoc opens doc ALONGSIDE whatever is already open and makes it active,
// returning it. This is D10's arrival path, and the difference from setDoc is
// entirely in what it does NOT do: it drops no document and clears no history.
//
// **It activates as well as adding, deliberately.** A background arrival should
// badge a tab rather than seize the view — that is the right behaviour and it is
// P06's, because there is no tab strip yet. An added-but-inactive document would be
// unreachable: the client learns of an arrival by re-reading the active document, so
// the feature would ship as "your co-signature completed and you can never see it".
// Activating also keeps today's observable behaviour exactly, which confines this
// change to the part that was invisible anyway — the previous document surviving.
//
// **Revisited when tabs landed (P06.S01, 2026-08-17): an arrival still activates.**
// The strip now exists, so "badge a tab rather than seize the view" is buildable —
// and it is deliberately not built here, because it is the one behaviour change in
// this phase that **no tier can observe**. An arrival originates in a p2p session
// with a pinned peer, so driving "the arrival did NOT take the view" needs a live
// counterparty; the standing two-machine VERIFY item is the only instrument that
// would see it. Shipping an unobservable change to the co-signing path, in the
// slice that is already changing how every open works, buys a nicety at the price
// of the one flow whose failures are cryptographic. The revisit is recorded rather
// than left as a dangling "when tabs land", which is the doc-vs-code shape this
// repo keeps finding.
func (s *Server) addDoc(doc *document) *document {
	if doc == nil {
		return nil
	}
	s.mu.Lock()
	s.registerLocked(doc)
	s.mu.Unlock()
	return doc
}

// setDoc installs doc as the open document, REPLACING whatever was open, and
// releasing the outgoing histories. nil closes the document.
//
// **Corrected 2026-08-17 (P06.S01): that list of callers is gone.** Opening a file, an
// upload, a combine and an office conversion all moved to addDocCapped, because with a
// tab strip the user asking for this document is no longer the user asking to be rid of
// that one. What is left in PRODUCTION is a single caller — handleClose, passing nil —
// so the non-nil path is now exercised only by tests, which use it to build a registry
// in a known state and to assert replace-versus-add.
//
// It is kept rather than collapsed into a close, and the reason is a date rather than a
// preference: P06.S02 splits Close view from Close all and is the slice that decides
// what the server-side shape of a close becomes. Renaming this now and again in one
// slice's time is churn across twenty test call sites for no gain. Recorded here so
// that "setDoc has a production caller that replaces" cannot be read off the signature
// again — which is exactly what this comment used to invite.
//
// It returns the document it installed (nil for a close), so the callers that install
// and then describe a document pass the one they just installed rather than
// re-resolving "the active one" — which was the same answer until arrivals started
// adding, and is no longer.
func (s *Server) setDoc(doc *document) *document {
	s.mu.Lock()
	// Release the outgoing documents' histories explicitly rather than leaving them
	// to become garbage with the documents themselves. Dropping the registry entry
	// does make them unreachable *eventually*, but a request already in flight still
	// holds its `doc` pointer — an OCR or export handler can hold one for many
	// seconds — and until it returns, that document's rings keep up to maxUndoDepth
	// whole PDFs alive. This is what keeps P01's "close retains no document bytes"
	// criterion true by construction instead of by garbage-collector timing.
	for _, d := range s.docs {
		if d == doc {
			continue // reinstalling the same document: not an outgoing one
		}
		clearUndo(d)
		clearRedo(d)
	}
	s.docs = nil
	s.activeID = docID{}
	if doc != nil {
		// A fresh id per open: never the closed document's, never an index.
		s.registerLocked(doc)
	}
	// No history to clear: a fresh document carries its own empty stacks, and the
	// outgoing document's history is released with the document itself. Clearing a
	// server-global pair here is what P03.S04 removed — the rings now belong to the
	// documents, so replacing the registry replaces the history with it.
	s.mu.Unlock()
	return doc
}

// removeDoc drops ONE document from the registry and returns whatever is active
// afterwards — the neighbour when the closed document was the active one, unchanged
// when it was not, nil when nothing is left.
//
// **The server picks the neighbour, and the response is how the client learns it.**
// Both sides deriving "which document is active now" would be two derivations of one
// rule, and they diverge silently rather than loudly: the strip highlights one document
// while the unpinned fallback on /api/pdf serves another, and nothing reports a
// disagreement. Same argument as registerLocked being the one id issuer.
//
// The neighbour is the NEXT document, falling back to the previous — the one that
// visually takes the closed one's place in the strip.
//
// **The rings are released explicitly**, exactly as setDoc does and for the same
// reason: dropping the registry entry makes the document unreachable eventually, but a
// request already in flight still holds its pointer — an OCR or export handler can hold
// one for many seconds — and until it returns, that document's rings keep up to
// maxUndoDepth whole PDFs alive. This is what keeps "a close retains no document bytes"
// true by construction rather than by garbage-collector timing.
func (s *Server) removeDoc(doc *document) *document {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, d := range s.docs {
		if d == doc {
			idx = i
			break
		}
	}
	if idx < 0 {
		// Already gone — a concurrent close, or a document from before a close-all.
		// Not an error: the caller asked for a state that already holds.
		return s.activeDocLocked()
	}
	clearUndo(doc)
	clearRedo(doc)
	s.docs = append(s.docs[:idx], s.docs[idx+1:]...)
	if s.activeID == doc.id {
		switch {
		case idx < len(s.docs):
			s.activeID = s.docs[idx].id // the next one shifted into this slot
		case len(s.docs) > 0:
			s.activeID = s.docs[len(s.docs)-1].id // closed the last: fall back to the previous
		default:
			s.activeID = docID{} // nothing left; the zero id names nothing
		}
	}
	return s.activeDocLocked()
}

// docResponse builds the metadata response for ONE document — the one the caller
// resolved, never "whatever is active". It takes the lock itself; callers must not
// hold it. A nil or no-longer-registered document yields the zero response, which is
// what the routes already send when nothing is open.
//
// Taking the document as an argument is what makes a document able to report on
// ITSELF. The previous signature took none and read s.activeDocLocked(), so an
// inactive document had no way to say anything — including that its history had been
// evicted, which is a clause P03.S04 owes and could not have expressed.
//
// **Every field is snapshotted under one lock hold.** The previous version released
// the lock and then read doc.path, doc.sig and doc.data, which was a live data race
// against undo.go's `doc.data = result` — reproduced with -race on an ordinary
// /api/doc concurrent with /api/pages, i.e. one user with two browser panes. See
// TestDocResponseRacesMutation, which fails against that shape.
func (s *Server) docResponse(doc *document) docResponse {
	s.mu.Lock()
	if doc == nil || !s.isRegisteredLocked(doc) {
		s.mu.Unlock()
		return docResponse{}
	}
	resp := docResponse{
		ID:             doc.id.String(),
		Name:           filepath.Base(doc.path),
		Path:           doc.path,
		CanSave:        doc.path != "",
		Signature:      doc.sig,
		CanUndo:        len(doc.undo) > 0,
		CanRedo:        len(doc.redo) > 0,
		HistoryEvicted: doc.historyEvicted,
	}
	// The bytes are read AFTER the lock is released, deliberately: FlagsJSON parses
	// the PDF, and holding the server mutex across a parse would serialize every
	// request behind it. Copying the slice header here is sufficient, because
	// doc.data is always REPLACED wholesale and never mutated in place (all five
	// writers assign a fresh slice) — so the array this header points at cannot
	// change under the parse. That invariant is what makes the unlocked read safe;
	// mutating doc.data in place would silently reintroduce the race.
	data := doc.data
	s.mu.Unlock()

	// Surface embedded signing placeholders so the recipient's UI can rebuild
	// them on open (the read half of the flag round-trip; the write half is
	// /api/flags). A parse failure just means "no flags" — never blocks the open.
	if flags, err := pdfops.FlagsJSON(data); err == nil && json.Valid(flags) {
		resp.Flags = flags
	}
	return resp
}

// docBytes snapshots a document's current bytes under the lock.
//
// Every handler that reads doc.data must come through here. Reading the field
// directly is a data race against undo.go's `doc.data = result` and handleSave's
// write — the same race docResponse above documents at length and fixed for
// itself, leaving twelve siblings unfixed. It is not theoretical: driving
// GET /api/scan concurrently with POST /api/pages trips the detector immediately,
// which is one user with two panes.
//
// The lock is held only for the slice-header copy, never across the PDF parse that
// follows, for the reason docResponse gives: doc.data is always REPLACED wholesale
// and never mutated in place, so the array this header points at cannot change
// underneath a parse. That invariant is what makes one accessor enough instead of
// holding the mutex through every operation — and what a future in-place mutation
// of doc.data would silently break, for every caller at once.
func (s *Server) docBytes(doc *document) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return doc.data
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
