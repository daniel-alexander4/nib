package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path/filepath"

	"nib/internal/instance"
)

// handoffRequest is what a second launch sends: a path, and nothing else (D20's third
// constraint — the hand-off differs from an ordinary open in how it is authenticated,
// not in what it can do).
type handoffRequest struct {
	Path string `json:"path"`
}

// handoffResponse tells the launch what happened, because the launch has a decision to
// make and "200" does not answer it.
//
// The four outcomes are distinct on purpose. `opened` and `focused` both mean "you are
// done, exit"; `queued` means "you are done, the user will see it when they unlock";
// `refused` means "say so somewhere the user can see". A single ok/error pair would
// collapse queued into opened, and a launch would report success for a document that
// will not appear until an unlock that may never come.
type handoffResponse struct {
	Result string `json:"result"` // opened | focused | queued | refused
	Reason string `json:"reason,omitempty"`
}

// SetHandoffSecret tells the server which secret authorises POST /api/handoff. Empty
// means this instance published no record, and the route then refuses everything.
func (s *Server) SetHandoffSecret(secret string) {
	s.mu.Lock()
	s.handoffSecret = secret
	s.mu.Unlock()
}

// handleHandoff takes a path from another launch of this application and opens it here.
//
// **Authorised by the hand-off secret AND a loopback origin** (D20). The secret does the
// work — a browser page cannot read a 0600 file in the config directory, so it can never
// hold one — and the origin check is defence in depth at the cost of one wrap, using the
// guard the pre-unlock routes already use.
//
// **It answers while LOCKED, and that is the case it exists for.** Nib starts locked, so
// the first double-click of every session lands here with no vault. Refusing would send
// the launch down its become-primary path and produce two Nibs every morning — the
// phase's own failure mode, triggered by its commonest input. The path is queued instead
// and opens when the user unlocks, which they were about to do anyway.
func (s *Server) handleHandoff(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	secret := s.handoffSecret
	unlocked := s.vault != nil
	s.mu.Unlock()
	if secret == "" || !instance.TokenMatches(r.Header.Get(instance.HeaderHandoff), secret) {
		httpError(w, http.StatusForbidden, "not this instance")
		return
	}

	var req handoffRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil || req.Path == "" {
		httpError(w, http.StatusBadRequest, "a path is required")
		return
	}
	// Canonicalise ONCE, here, so the comparison below and the install below see the same
	// string. `handleOpen` cleans (`server.go:337`) and this route did not, so a document
	// opened through the UI was stored cleaned while a handed-off one was stored raw —
	// and D16's compare is a string compare. Today's only caller happens to send a clean
	// path (`initialFile` runs `filepath.Abs`, which cleans), so nothing was broken; the
	// route's correctness rested on a property of one caller, which is not where a
	// route's correctness belongs.
	path := filepath.Clean(req.Path)

	// D16: a path already open is activated, not opened twice. Two tabs on one path are
	// two independent working copies of the same file, and whichever saves last silently
	// discards the other's work.
	if doc := s.docForPath(path); doc != nil {
		s.mu.Lock()
		s.activeID = doc.id
		s.mu.Unlock()
		writeJSON(w, handoffResponse{Result: "focused"})
		return
	}

	// `unlocked` was read before the body was decoded, so it may already be stale — and
	// the stale direction is the harmful one. If the vault is adopted in that window, the
	// drain has already run and found an empty queue, so a path appended afterwards waits
	// for an unlock that has happened and will not happen again: the document silently
	// never opens. queuePendingOpen therefore re-checks under the SAME lock adoptVault
	// sets the vault under, and reports false when there is nothing left to wait for.
	if !unlocked && s.queuePendingOpen(path) {
		writeJSON(w, handoffResponse{Result: "queued"})
		return
	}
	if err := s.openHandedOff(path); err != nil {
		writeJSON(w, handoffResponse{Result: "refused", Reason: err.Error()})
		return
	}
	writeJSON(w, handoffResponse{Result: "opened"})
}

// openHandedOff installs a path through the SAME machinery an ordinary open uses —
// D20's third constraint, and the reason this is one function rather than a handler that
// reads a file and appends to the registry. A self-contained version would be a second,
// less-checked way to install a document: no LooksLikePDF (so any readable file becomes
// the open document with canSave true, and Save clobbers it), no size cap, no document
// cap.
func (s *Server) openHandedOff(path string) error {
	data, ref := readInstallablePDF(path)
	if ref != nil {
		// Its own wording, kept deliberately. This door is the OS handing Nib a file the
		// user double-clicked, not a path anyone typed, so "file not found" would read as
		// a bug report rather than an answer. ADR-009 unifies the CHECKS; it explicitly
		// does not require every site to print the same sentence.
		switch ref.kind {
		case refuseTooLarge:
			return errHandoff("that PDF is too large")
		case refuseUnreadable:
			return errHandoff("that file could not be read")
		case refuseNotPDF:
			return errHandoff("that file isn't a PDF")
		default:
			return errHandoff("that file could not be opened")
		}
	}
	if _, err := s.addDocCapped(newPathDoc(path, data)); err != nil {
		return err
	}
	return nil
}

type errHandoff string

func (e errHandoff) Error() string { return string(e) }

// docForPath finds an open document by the path it was opened from. Empty paths never
// match: an upload, a combine and an arrival all have none, and treating them as equal
// would make a hand-off "focus" an unrelated document.
func (s *Server) docForPath(path string) *document {
	if path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.docs {
		if d.path == path {
			return d
		}
	}
	return nil
}

// queuePendingOpen holds a path handed off to a locked instance until it unlocks.
//
// **Memory only, and bounded.** Never persisted: a queued path dies with the process, so
// there is no stale state for the next morning to reason about, and the user simply
// clicks again. Bounded by the document cap because that is the number that will actually
// open — queueing more would be promising something the cap then refuses. The OLDEST is
// dropped, because the most recent double-click is the one the user is waiting on.
//
// Reports whether the path was queued. **False means the vault is already open** — the
// caller must install it directly rather than leave it waiting for an unlock that has
// already happened. That check is here, rather than at the call site, because here is
// where it is under the same lock `adoptVault` sets the vault under: either this sees the
// vault and declines to queue, or it appends first and the drain that follows finds it.
func (s *Server) queuePendingOpen(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vault != nil {
		return false
	}
	for _, p := range s.pendingOpens {
		if p == path {
			return true // the same file clicked twice is one document, not two
		}
	}
	s.pendingOpens = append(s.pendingOpens, path)
	if len(s.pendingOpens) > maxOpenDocs {
		s.pendingOpens = s.pendingOpens[len(s.pendingOpens)-maxOpenDocs:]
	}
	return true
}

// drainPendingOpens opens everything a locked instance was handed. Called from adoptVault
// — the ONE place a vault becomes available, factored for exactly this reason.
func (s *Server) drainPendingOpens() {
	s.mu.Lock()
	queued := s.pendingOpens
	s.pendingOpens = nil
	s.mu.Unlock()
	for _, path := range queued {
		if s.docForPath(path) != nil {
			continue
		}
		if err := s.openHandedOff(path); err != nil {
			// Logged rather than surfaced: the user is at the unlock screen and has no
			// place to receive it yet. The document simply does not appear, which is
			// the same outcome as the refusal they would have seen while unlocked.
			log.Printf("handed-off document %q could not be opened after unlock: %v", path, err)
		}
	}
}
