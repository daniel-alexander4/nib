package server

import (
	"net/http"

	"nib/internal/sign"
)

// The history budget. maxUndoDepth caps each document's undo stack; maxUndoBytes is
// a SINGLE GLOBAL figure covering every open document's undo and redo bytes together.
//
// Both halves of that sentence are deliberate and both changed in P03.S04:
//
//   - Global, not per-document. A per-document budget would be 256 MiB × N open
//     documents, which is the growth D8's pin refuses — the whole point of moving the
//     rings onto documents is that the memory ceiling must NOT move with them.
//   - The pair, not just undo. The previous budget counted only the undo stack, on the
//     reasoning that evicting from redo's far end silently shortens the user's redo
//     reach. That reasoning was sound and the consequence was a real ceiling of ~2×
//     maxUndoBytes for one document — and 2N× once documents multiply. Counting the
//     pair is what closes it.
//
// It is a bound with one named exception, not a hard cap: the document that just
// GREW always keeps its most recent undo entry, so a single document holding one
// state larger than the budget exceeds it rather than losing the ability to undo.
// Stated because a cap the code does not enforce is worse than a smaller one it does.
//
// "The document that just grew" is not "the active document", and this comment said
// active while the code (tier 3 below) trims `grown`. ADR-003 names that exact
// conflation as a live defect rather than a naming quibble — one that passed every
// test — so the inverted wording sat nine lines above the constant, which is the
// first thing anyone reaching for the budget reads.
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
// It reports whether the commit landed: false means the document this operation was
// pinned to is no longer registered and the work was discarded. Callers MUST check it and
// answer **409**, not 200 with an empty docResponse (a success reply for discarded work)
// and not 404.
//
// **409, and this comment said 404 until v1.116.3.** ADR-004 postdates it: *"An id naming a
// document the server no longer holds is 409, never 404. 404 already means 'no document
// open'; a closed tab is a different fact, and the client must tell them apart to remove the
// tab rather than blank the app."* By the time this returns false `resolveDoc` has already
// produced a non-nil document, so the ONLY way to get here is a close landing mid-flight —
// which is exactly the fact ADR-004 assigns 409. `web/app.js` hooks 409 to reconcile and
// drop the stale tab; a 404 gets no such handling, so closing one tab during a long
// operation on it left a tab where everything failed. The check
// belongs here and not in the caller because only this function holds the lock
// across the test and the write; a caller that tested the document first would leave a
// window for a close to land in between, which is the very defect.
func (s *Server) commitMutation(doc *document, input, result []byte) bool {
	sig := sign.Verify(result)
	s.mu.Lock()
	defer s.mu.Unlock()
	// The TARGET document, passed in, not whatever happens to be active. Resolving
	// active here would let an operation addressed to one document commit into
	// another — ADR-001's corruption arriving through the helper rather than
	// through a forgotten header, and invisible at every call site.
	//
	// P01.S01's property is unchanged and strictly stronger: the test and the write
	// still happen under one lock hold, so a close landing in between cannot yield
	// a success for discarded work. The test is now "is this document still
	// registered" rather than "is anything open".
	if doc == nil || !s.isRegisteredLocked(doc) {
		return false
	}
	doc.undo = append(doc.undo, input)
	clearRedo(doc)
	// A fresh edit is a fresh history: whatever was evicted before, this document
	// now has something undoable again and the flag would otherwise be reported
	// alongside a non-empty stack, which reads as a lie.
	doc.historyEvicted = false
	s.trimHistoryLocked(doc)
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
func (s *Server) commitBarrier(doc *document, result []byte) bool {
	sig := sign.Verify(result)
	s.mu.Lock()
	defer s.mu.Unlock()
	// See commitMutation. It matters more here: these are the irreversible
	// operations, so committing one into the wrong document destroys content that
	// undo is deliberately unable to bring back.
	if doc == nil || !s.isRegisteredLocked(doc) {
		return false
	}
	clearUndo(doc)
	clearRedo(doc)
	// A barrier is not an eviction: the history is gone because the user asked for
	// content to be destroyed, not because memory ran short. Reporting it as an
	// eviction would tell them their redaction cost them their undo stack.
	doc.historyEvicted = false
	doc.data = result
	doc.sig = sig
	return true
}

// clearUndo / clearRedo drop one document's history stack, niling entries so the
// (potentially large) byte slices they hold are released to the GC. Caller holds s.mu.
func clearUndo(doc *document) {
	for i := range doc.undo {
		doc.undo[i] = nil
	}
	doc.undo = nil
}

func clearRedo(doc *document) {
	for i := range doc.redo {
		doc.redo[i] = nil
	}
	doc.redo = nil
}

// historyBytes reports the bytes one document's two stacks hold together.
func historyBytes(doc *document) int {
	total := 0
	for _, b := range doc.undo {
		total += len(b)
	}
	for _, b := range doc.redo {
		total += len(b)
	}
	return total
}

// historyBytesLocked reports the bytes held across EVERY open document — the figure
// maxUndoBytes bounds. Caller holds s.mu.
func (s *Server) historyBytesLocked() int {
	total := 0
	for _, d := range s.docs {
		total += historyBytes(d)
	}
	return total
}

// historyBudget is the byte ceiling this server enforces. It exists so the eviction
// tests can drive the budget with kilobytes instead of allocating 256 MiB per case —
// a test that cannot afford to run is a test that does not run. Production never sets
// the field, so the constant is what ships.
func (s *Server) historyBudget() int {
	if s.maxHistoryBytes > 0 {
		return s.maxHistoryBytes
	}
	return maxUndoBytes
}

// trimHistoryLocked enforces the depth cap on `active` and the global byte budget
// across every open document. Caller holds s.mu; `active` is the document that just
// grew, which is the only one whose entries may be trimmed individually.
//
// **Eviction happens in two different units, and that is the design, not an
// inconsistency.**
//
// An INACTIVE document loses its history WHOLE. Two reasons, and either alone would
// decide it. First, convergence: a budget covering undo+redo cannot be met by
// dropping undo entries alone, because a document whose bytes all sit in redo has
// nothing left to give and the loop would spin against a ceiling it cannot reach.
// Second, honesty: a partially-trimmed history is precisely the silent eviction the
// plan-review pin refuses — the user keeps an undo button that reaches less far than
// it did, with nothing anywhere saying so. Dropping the history whole is a state the
// document can report (historyEvicted), and a half-dropped one is not.
//
// The ACTIVE document keeps the entry-by-entry trim it has always had, because that
// is ordinary depth-cap behaviour the user experiences as "undo remembers 30 steps"
// and it must not change.
//
// Order among inactive documents is OPEN ORDER, oldest first. Least-recently-active
// is the better model and is not available: nothing records a last-active moment
// until document switching exists (P06). Recorded as a default rather than
// approximated with a signal that would be wrong.
//
// **`grown` is not the same document as the active one, and conflating them is a
// live defect, not a naming quibble.** This whole phase exists so an operation can be
// addressed to a document the user is not looking at; when one is, `grown` is that
// inactive document while s.activeID names another. A pass that protected `grown` and
// treated everything else as evictable would then throw away the history of the
// document the user actually has open, to make room for one they do not — the
// acceptance clause exactly inverted, and green against every test that grows the
// active document. So eviction walks three tiers, in order: documents that are
// neither grown nor active, then the active document, then `grown`'s own entries.
func (s *Server) trimHistoryLocked(grown *document) {
	// Depth first, on the document that grew. This is unchanged behaviour and the
	// cap that binds in every realistic case — 30 states of an ordinary PDF is far
	// below the byte budget, which is why single-document behaviour is unaffected by
	// the budget's move to the pair (see ADR-003, which records that premise).
	for len(grown.undo) > maxUndoDepth {
		grown.undo[0] = nil
		grown.undo = grown.undo[1:]
	}

	if s.historyBytesLocked() <= s.historyBudget() {
		return
	}

	// evict drops one document's history whole, reporting whether that was enough.
	evict := func(d *document) bool {
		if d == nil || d == grown || (len(d.undo) == 0 && len(d.redo) == 0) {
			return false
		}
		clearUndo(d)
		clearRedo(d)
		d.historyEvicted = true
		s.historyEvictions++
		return s.historyBytesLocked() <= s.historyBudget()
	}

	// Tier 1: documents the user is neither editing nor looking at.
	active := s.activeDocLocked()
	for _, d := range s.docs {
		if d == active {
			continue
		}
		if evict(d) {
			return
		}
	}

	// Tier 2: the active document — evicted only once every other history is gone,
	// and still ahead of breaking the budget, because an unbounded ceiling costs the
	// user more than a history they can be told about.
	if evict(active) {
		return
	}

	// Tier 3: the bytes are the grown document's own. Trim its undo from the oldest
	// end, keeping the last entry: a document whose single most recent state exceeds
	// the whole budget stays undoable rather than being silently stripped of the one
	// thing undo is for. This is the named exception in the budget's contract.
	for len(grown.undo) > 1 && s.historyBytesLocked() > s.historyBudget() {
		grown.undo[0] = nil
		grown.undo = grown.undo[1:]
	}
}

// handleUndo reverts the document to the state before the last undoable operation,
// moving the current state onto the redo stack. With nothing to undo it simply
// returns the current state. The client reloads the bytes from /api/pdf.
func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	// Resolved from the header, and the request stays BODYLESS — which is the
	// whole reason D15 chose a header. An earlier exit criterion said these two
	// routes would "stop being bodyless"; it predates D15 and assumed a body was
	// how an id arrives. Adding one here would edit exactly the schema the header
	// exists to leave alone. See ADR-004.
	doc, err := s.docFor(r)
	if err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	s.mu.Lock()
	if doc == nil || len(doc.undo) == 0 {
		s.mu.Unlock()
		writeJSON(w, s.docResponse(doc))
		return
	}
	last := len(doc.undo) - 1
	prev := doc.undo[last]
	doc.undo[last] = nil
	doc.undo = doc.undo[:last]
	doc.redo = append(doc.redo, doc.data)
	// Trimmed here as handleRedo trims on its own push. ADR-003 bounds the undo+redo
	// PAIR against one global budget, and this push is not byte-neutral: undoing a large
	// OCR or optimize result moves a big doc.data onto redo while popping a small prev,
	// so without this the total walks past the ceiling with nothing evicting.
	s.trimHistoryLocked(doc)
	doc.data = prev
	doc.sig = sign.Verify(prev)
	s.mu.Unlock()
	writeJSON(w, s.docResponse(doc))
}

// handleRedo re-applies the last undone operation, moving the current state back
// onto the undo stack. With nothing to redo it returns the current state.
func (s *Server) handleRedo(w http.ResponseWriter, r *http.Request) {
	// Resolved from the header, and the request stays BODYLESS — which is the
	// whole reason D15 chose a header. An earlier exit criterion said these two
	// routes would "stop being bodyless"; it predates D15 and assumed a body was
	// how an id arrives. Adding one here would edit exactly the schema the header
	// exists to leave alone. See ADR-004.
	doc, err := s.docFor(r)
	if err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	s.mu.Lock()
	if doc == nil || len(doc.redo) == 0 {
		s.mu.Unlock()
		writeJSON(w, s.docResponse(doc))
		return
	}
	last := len(doc.redo) - 1
	next := doc.redo[last]
	doc.redo[last] = nil
	doc.redo = doc.redo[:last]
	doc.undo = append(doc.undo, doc.data)
	s.trimHistoryLocked(doc)
	doc.data = next
	doc.sig = sign.Verify(next)
	s.mu.Unlock()
	writeJSON(w, s.docResponse(doc))
}
