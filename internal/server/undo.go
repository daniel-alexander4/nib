package server

import (
	"net/http"

	"nib/internal/sign"
)

// undoBudget bounds the in-memory undo/redo history: at most maxUndoDepth states
// per stack, and at most maxUndoBytes total across the UNDO stack, oldest evicted
// first. The redo stack is only depth-capped (transitively, by what undo held), not
// byte-capped: undo and redo are one linear history split at a cursor, and evicting
// from redo's far end would silently truncate the user's redo reach — so a deep
// undo of large documents can transiently hold up to ~2× maxUndoBytes across the
// two stacks until the history is rewritten or cleared. That peak is accepted;
// the byte budget is what protects memory on the growing (undo) side.
const (
	maxUndoDepth = 30
	maxUndoBytes = 256 << 20
)

// commitMutation records an undoable change: it pushes the operation's INPUT (the
// bytes the op transformed — the exact state to return to) onto the undo stack,
// drops any redo history, then makes result the current document. Callers pass the
// input they actually operated on — posted bytes for page/outline ops, doc.data
// for in-place ops — so undo restores precisely the pre-op document (including any
// overlay the client baked into the input).
//
// It reports whether the commit landed: false means no document is open and the
// work was discarded. Callers MUST check it and answer 404 rather than replying
// 200 with an empty docResponse — a success reply for discarded work. The check
// belongs here and not in the caller because only this function holds the lock
// across the test and the write; a caller that tested the document first would leave a
// window for a close to land in between, which is the very defect.
func (s *Server) commitMutation(input, result []byte) bool {
	sig := sign.Verify(result)
	s.mu.Lock()
	defer s.mu.Unlock()
	doc := s.activeDocLocked()
	if doc == nil {
		return false
	}
	s.undo = append(s.undo, input)
	s.clearRedoLocked()
	s.trimUndoLocked()
	doc.data = result
	doc.sig = sig
	return true
}

// commitBarrier makes result the current document and CLEARS the undo/redo
// history. Used by operations that destroy content the user expects to be gone for
// good (redaction, flatten/remove-originals): retaining pre-op snapshots would let
// undo resurrect the very content the operation removed.
//
// Reports whether the commit landed, on the same contract as commitMutation —
// and it matters more here, because the operations that come through this door
// are the irreversible ones. Telling a user their redaction succeeded when it
// was discarded is the worst reply this server can give.
func (s *Server) commitBarrier(result []byte) bool {
	sig := sign.Verify(result)
	s.mu.Lock()
	defer s.mu.Unlock()
	doc := s.activeDocLocked()
	if doc == nil {
		return false
	}
	s.clearUndoLocked()
	s.clearRedoLocked()
	doc.data = result
	doc.sig = sig
	return true
}

// clearUndoLocked / clearRedoLocked drop a history stack, niling entries so the
// (potentially large) byte slices they hold are released to the GC. Caller holds s.mu.
func (s *Server) clearUndoLocked() {
	for i := range s.undo {
		s.undo[i] = nil
	}
	s.undo = nil
}

func (s *Server) clearRedoLocked() {
	for i := range s.redo {
		s.redo[i] = nil
	}
	s.redo = nil
}

// trimUndoLocked enforces the depth and total-byte caps, evicting oldest entries
// first and niling them so their bytes are released. Caller holds s.mu.
func (s *Server) trimUndoLocked() {
	total := 0
	for _, b := range s.undo {
		total += len(b)
	}
	for len(s.undo) > 1 && (len(s.undo) > maxUndoDepth || total > maxUndoBytes) {
		total -= len(s.undo[0])
		s.undo[0] = nil
		s.undo = s.undo[1:]
	}
}

// handleUndo reverts the document to the state before the last undoable operation,
// moving the current state onto the redo stack. With nothing to undo it simply
// returns the current state. The client reloads the bytes from /api/pdf.
func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.activeDocLocked()
	if doc == nil || len(s.undo) == 0 {
		s.mu.Unlock()
		writeJSON(w, s.docResponse())
		return
	}
	last := len(s.undo) - 1
	prev := s.undo[last]
	s.undo[last] = nil
	s.undo = s.undo[:last]
	s.redo = append(s.redo, doc.data)
	doc.data = prev
	doc.sig = sign.Verify(prev)
	s.mu.Unlock()
	writeJSON(w, s.docResponse())
}

// handleRedo re-applies the last undone operation, moving the current state back
// onto the undo stack. With nothing to redo it returns the current state.
func (s *Server) handleRedo(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	doc := s.activeDocLocked()
	if doc == nil || len(s.redo) == 0 {
		s.mu.Unlock()
		writeJSON(w, s.docResponse())
		return
	}
	last := len(s.redo) - 1
	next := s.redo[last]
	s.redo[last] = nil
	s.redo = s.redo[:last]
	s.undo = append(s.undo, doc.data)
	s.trimUndoLocked()
	doc.data = next
	doc.sig = sign.Verify(next)
	s.mu.Unlock()
	writeJSON(w, s.docResponse())
}
