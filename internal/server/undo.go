package server

import (
	"errors"
	"fmt"
	"net/http"

	"nib/internal/ceremony"
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
func (s *Server) commitMutation(doc *document, input, result []byte) error {
	sig := sign.Verify(result)
	// **D29's freeze, on the SERVER's bytes and BEFORE the lock — both halves were wrong in
	// the first draft and the slice's own diff review found each.**
	//
	// It checked `input`, and the doc comment claimed the input "carries a record only if a
	// ceremony already existed". False at two of six call sites: `pages.go` and `outline.go`
	// pass `formFileBytes(w, r, "pdf")` — the CLIENT's bytes. A request posting a PDF with
	// nib-ceremony.json stripped therefore passed the freeze and then replaced doc.data,
	// bypassing D29 on exactly the two routes whose input the server does not own. The
	// document the rule is about is the one the server holds.
	//
	// And it ran inside the lock. ceremony.Extract is a full pdfcpu parse, so every commit in
	// the product was doing one while holding the GLOBAL server mutex. docBytes takes its own
	// brief lock; the window between that snapshot and the lock below is benign, because the
	// only way a document gains a record is convene, which is itself a barrier that would be
	// refused if one already existed.
	// nil is a caller passing a document that was already gone; the registration test below
	// owns that case and answers errDocClosed. Reading bytes off it first would panic on a
	// path this door is contractually required to handle.
	if doc != nil {
		if err := ceremonyFreeze(s.docBytes(doc)); err != nil {
			return err
		}
	}
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
		return errDocClosed
	}
	if err := s.byteCapLocked(doc, result); err != nil {
		return err
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
	return nil
}

// errDocClosed is the commit doors' other refusal: the target document was closed while the
// operation was running. Named rather than a bare false, because the doors now have two
// reasons to refuse and a caller mapping one boolean onto two sentences cannot tell the user
// which happened.
var errDocClosed = errors.New("that document is no longer open")

// byteCapLocked refuses a commit that would push the open documents past ADR-005's aggregate
// ceiling. Caller holds s.mu.
//
// ADR-005 says the cap bounds count AND aggregate bytes, and until now only addDocCapped
// enforced the byte half — that is, only at OPEN. Every operation that GROWS a document in
// place went straight past it: an OCR text layer, an N-up, a scan import, an attachment. Two
// 200 MiB documents plus a third that an attachment grows to 300 MiB is 700 MiB, refused by
// nothing, and the ADR's sentence was true only of the door it was written at.
//
// It sums the OTHER documents and adds the incoming result, so replacing a document's bytes
// with smaller ones can never be refused — the check is on the total after the write, not on
// the delta.
func (s *Server) byteCapLocked(doc *document, result []byte) error {
	total := len(result)
	for _, d := range s.docs {
		if d != doc {
			total += len(d.data)
		}
	}
	if total > s.docBudget() {
		return ErrTooManyBytes
	}
	return nil
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
func (s *Server) commitBarrier(doc *document, result []byte) error {
	sig := sign.Verify(result)
	// D29's freeze, on the server's bytes and before the lock — see commitMutation. It
	// matters more at this door: a barrier operation is destructive, so redaction on a
	// convened document would leave every other party's copy hashing to bytes that no longer
	// exist anywhere.
	// nil is a caller passing a document that was already gone; the registration test below
	// owns that case and answers errDocClosed. Reading bytes off it first would panic on a
	// path this door is contractually required to handle.
	if doc != nil {
		if err := ceremonyFreeze(s.docBytes(doc)); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// See commitMutation. It matters more here: these are the irreversible
	// operations, so committing one into the wrong document destroys content that
	// undo is deliberately unable to bring back.
	if doc == nil || !s.isRegisteredLocked(doc) {
		return errDocClosed
	}
	if err := s.byteCapLocked(doc, result); err != nil {
		return err
	}
	clearUndo(doc)
	clearRedo(doc)
	// A barrier is not an eviction: the history is gone because the user asked for
	// content to be destroyed, not because memory ran short. Reporting it as an
	// eviction would tell them their redaction cost them their undo stack.
	doc.historyEvicted = false
	doc.data = result
	doc.sig = sig
	return nil
}

// wroteCommitFailure maps a commit door's refusal onto the reply, and exists so the mapping
// is written once. Eight call sites hand-mirroring it is eight chances to have seven — which
// is exactly how ErrStampTextUnrepresentable reached two producers of three.
//
// **409 for both reasons, and the byte cap is why that is not sloppiness.** The five
// user-initiated install routes have always answered 409 for ErrTooManyBytes, so a second
// status for the same fact would mean the same refusal reads differently depending on which
// door the user reached it through. ADR-004's rule is that a document the server no longer
// holds is 409 and never 404; it does not reserve 409 for that one fact, and the client's
// 409 hook reconciles and then shows this message either way. The cost of the shared code is
// one extra GET /api/docs on a cap refusal, which finds nothing changed.
func wroteCommitFailure(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	httpError(w, http.StatusConflict, err.Error())
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

	// Tier 3: the bytes are the grown document's own. Trim from the oldest end of BOTH
	// rings, keeping the last entry of each.
	//
	// # Both rings, because undo alone cannot converge
	//
	// This walked `grown.undo` only, and `handleUndo` pushes onto `grown.redo`. With one
	// document open — Nib's default — tier 1 skips `active` (which IS `grown`), tier 2's
	// `evict` returns false on `d == grown`, and tier 3 then trimmed a ring the bytes were
	// not in. ADR-003 states the impossibility as a premise: *"a budget covering undo+redo
	// cannot be met by dropping undo entries alone: a document whose bytes all sit in redo
	// has nothing to give."* That is exactly the post-undo state of a lone document.
	//
	// The newest entry of each ring survives, which is the ADR's named exception generalised:
	// after an undo the newest REDO entry is the state the user would get back by pressing
	// redo, and it is as much "the one thing the button is for" as the newest undo entry is.
	//
	// # And it is recorded
	//
	// This function's own contract says a partially-trimmed history is "precisely the silent
	// eviction the plan-review pin refuses — the user keeps an undo button that reaches less
	// far than it did, with nothing anywhere saying so", and tier 3 did exactly that half-drop
	// while setting neither flag. It sets them now, so the document can report it.
	trimmed := false
	for {
		over := s.historyBytesLocked() > s.historyBudget()
		if !over {
			break
		}
		switch {
		case len(grown.undo) > 1:
			grown.undo[0] = nil
			grown.undo = grown.undo[1:]
		case len(grown.redo) > 1:
			grown.redo[0] = nil
			grown.redo = grown.redo[1:]
		default:
			// One entry left in each: the named exception. Stop rather than spin against a
			// ceiling this document cannot reach — which is the other half of the ADR's
			// premise, and why this loop needs an exit that is not "under budget".
			if trimmed {
				grown.historyEvicted = true
				s.historyEvictions++
			}
			return
		}
		trimmed = true
	}
	if trimmed {
		grown.historyEvicted = true
		s.historyEvictions++
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

// --- D29's freeze -------------------------------------------------------------

// ErrCeremonyFrozen refuses a mutating operation on a document under a live ceremony.
//
// Its own sentinel so the refusal can name the ceremony rather than reading as a generic
// failure — D29's wording is "refuse and name the ceremony", because a user told only that
// an edit failed will try again.
var ErrCeremonyFrozen = errors.New("this document is part of a signing ceremony")

// ceremonyFreeze refuses to mutate a document that already carries a ceremony record.
//
// # Why it tests the DOCUMENT THE SERVER HOLDS, and not the result
//
// Convene itself commits — it appends a readme, signature pages and a ceremony page, and
// embeds the record — so a guard on the RESULT would refuse the one operation that is
// supposed to create a ceremony. The pre-op document is what distinguishes "a ceremony
// already exists" from "one is being created".
//
// It must be the SERVER's copy of that document, never the operation's `input` parameter:
// two of commitMutation's six call sites pass bytes posted by the client, so a request with
// the record stripped would otherwise walk straight through.
//
// # Why here rather than at each route — and the one route this did NOT reach
//
// D29 says mutating operations refuse; there are a dozen of them, and a rule enforced at
// eleven is not a rule. Both commit doors call this, so eleven routes inherit it.
//
// **They are not all of them, and the first draft of this comment said they were.** It read
// "a thirteenth route inherits it without anybody remembering to" — a confident false
// statement, found by this slice's own diff review: `handleSave` writes the file itself and
// assigns doc.data under the registry lock, reaching NEITHER door, while sitting in tier 2's
// MUTATING inventory. It calls this function directly now, and
// `TestEveryMutatingRouteReachesTheCeremonyFreeze` asserts the ROUTING for the whole
// inventory rather than testing the two doors — because a test that calls commitMutation
// in-process cannot see a route that skips it, which is exactly how this got through.
//
// # What this replaces, and it was not a stopgap — it never fired at all
//
// The client warns before a destructive edit via `confirmSignatureLoss` (web/app.js), and
// D29 already says "a client confirm is not a freeze". Measured at P07.S02's grill, it is
// weaker even than that: the predicate is `!isSigned()`, `isSigned()` reads
// `state !== 'unsigned'`, and a convened document IS unsigned — an attachment is not a
// signature — so on precisely these documents the confirm short-circuits to true and **no
// dialog is shown at all**.
//
// The freeze is unconditional on the ceremony's deadline. An expired ceremony is still a
// ceremony whose parties hold invitations naming this document's hash; silently allowing
// edits once the clock runs out would break their copies rather than this user's.
func ceremonyFreeze(docBytes []byte) error {
	rec, err := ceremony.Extract(docBytes)
	if err != nil {
		// No record, or one that will not parse. A document whose record is unreadable is
		// not demonstrably under a ceremony, and refusing every edit on an unparseable
		// attachment would strand a user with no way out — D34's self-healing rule.
		return nil
	}
	return fmt.Errorf("%w: it belongs to ceremony %s, and editing it now would change the "+
		"document every other party was invited to sign. Their copies would stop matching "+
		"this one", ErrCeremonyFrozen, rec.ID)
}
