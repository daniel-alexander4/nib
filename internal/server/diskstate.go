package server

import (
	"crypto/sha256"
	"net/http"
	"os"

	"nib/internal/sign"
)

// The file underneath an open document can change, and until /pending 333 nothing in
// Nib could tell. A document records `path` and `data` and nothing that describes the
// FILE — no size, no mtime, no hash — so the bytes read at open were served forever,
// and the only `os.Stat` on an open path (server.go, handleOpen) sized the file before
// the read and threw the result away.
//
// Two halves of one defect, and the second is the expensive one:
//
//   - The REPORT. A hard reload cannot help, because it re-fetches from the same
//     in-memory copy — so the user has no way to learn the file moved. That is the
//     symptom /pending 333 was filed for.
//   - The REFUSAL. handleSave writes doc.path with no precondition, so saving a stale
//     document silently destroys whatever reached the file in the meantime. The stale
//     render is the symptom; this is the consequence.
//
// One predicate, two named consumers (ADR-009): docResponse reports, handleSave refuses.
// Both go through diskChanged and neither re-implements it.

// diskState is what a document remembers about the file it was read from. It is
// recorded from a stat taken AFTER the read, never the one taken before it: the
// install doors stat to size-check and then read, and a change landing in that window
// would otherwise be baked in as the baseline and the document born stale.
//
// `info` is kept whole rather than reduced to its parts because os.SameFile needs it —
// that is the portable identity door (device+inode on unix, volume+file index on
// Windows), and reaching for syscall.Stat_t instead would force a //go:build split for
// a fact the stdlib already answers everywhere.
type diskState struct {
	info os.FileInfo
	sum  [32]byte
}

// recordDisk stamps the document with the file's current identity and content hash.
//
// Called at both install doors and again inside handleSave's write. **The re-record on
// save is not optional**: WriteDurable renames, so the inode AND the mtime change on
// every save Nib itself performs. Omit it and a document reads "changed on disk" from
// the moment the user first saves it — a warning that is armed forever, that no other
// test would catch, and that would teach the user to ignore the banner.
func recordDisk(d *document) {
	d.disk = stampFor(d.path, d.data)
}

// stampFor builds the baseline for `path` holding `data`. It is the ONE place that says
// what a baseline IS (ADR-009); recordDisk assigns it for a caller that already holds the
// lock, and handleReload calls it BEFORE taking the lock.
//
// That split exists for a measured reason rather than a stylistic one: the sha256 here
// runs at roughly 0.7 GB/s, so a 200 MiB document — the maxPDFBytes ceiling — is ~0.3 s
// of hashing, and doing it under s.mu would serialize every other route behind it. The
// same argument docResponse makes about its own stat, and the mistake armWindowFor
// shipped. handleSave keeps calling recordDisk under the lock because it is re-stamping
// bytes it has just written and nothing else is contending for that document.
//
// nil means "nothing to compare against later": a path-less document, or a file that
// cannot be stat'd. diskChanged answers false on a nil baseline and says nothing, rather
// than warning about a file it could not read.
func stampFor(path string, data []byte) *diskState {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return &diskState{info: info, sum: sha256.Sum256(data)}
}

// newPathDoc builds a document for a file read from disk, with its baseline stamped.
//
// **The one door** (ADR-009). There are exactly two places a document is built from a
// path — handleOpen and openHandedOff — and a baseline stamped at one of them and not
// the other is worse than none: the unstamped door produces documents that can never
// report a change, silently, and the guard for that is a count of the callers rather
// than a comparison of two copies of the stamping. A third install door must come
// through here too.
func newPathDoc(path string, data []byte) *document {
	d := &document{path: path, data: data, sig: sign.Verify(data)}
	recordDisk(d)
	return d
}

// diskOf reads the baseline under the server lock, so the unlocked stat that follows
// works from a value that cannot be torn by a concurrent save. `disk` is a pointer
// replaced wholesale by recordDisk and never mutated in place — the same invariant
// docResponse's unlocked read of doc.data already rests on.
//
// This exists because `path` gets away with an unlocked read and `disk` must not:
// path is written once before registerLocked publishes the document, while disk is
// re-recorded on every save. Reading it the way the line above reads path would be a
// real data race, and the two sitting next to each other is exactly how that gets
// written by accident.
func (s *Server) diskOf(doc *document) *diskState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if doc == nil {
		return nil
	}
	return doc.disk
}

// diskChanged reports whether path now holds content other than what was recorded.
//
// **The comparison is against the RECORDED bytes, never against doc.data**, and that is
// the whole correctness of it. The question is "has the file moved since we read it",
// not "does the file differ from what we are showing" — those two come apart the moment
// the user edits, and the second would light the banner on every unsaved change. Which
// is why a hash is recorded at open rather than the check comparing against live bytes.
//
// Cheap in the case that is taken essentially every time: one stat, and identity + size
// + mtime agreeing ends it. The read-and-hash happens only when that trigger fires,
// which needs the file to have actually been rewritten.
//
// A size difference is decided without reading: different lengths cannot be the same
// bytes. The read is therefore reached only by a same-size rewrite or a bare mtime
// touch — and the touch is exactly why the content check has to exist at all, since
// "this file has changed" is a FALSE STATEMENT about a file whose bytes are identical.
//
// **Declared gaps**, in the manner of the //go:build siblings elsewhere in the tree:
//
//   - A stat error answers false, including os.ErrNotExist. A deleted file is not a
//     changed one for the purpose this serves: saving recreates it and destroys
//     nothing, so there is no data loss to warn about and no true sentence to print.
//   - It is advisory by construction. The stat precedes the write, so the file can
//     change in the gap — the same TOCTOU bound isRegistered states about itself. It
//     narrows the window from unbounded to microseconds; it does not close it.
func diskChanged(path string, rec *diskState) bool {
	if path == "" || rec == nil {
		return false
	}
	fresh, err := os.Stat(path)
	if err != nil {
		return false
	}
	if os.SameFile(rec.info, fresh) &&
		fresh.Size() == rec.info.Size() &&
		fresh.ModTime().Equal(rec.info.ModTime()) {
		return false
	}
	if fresh.Size() != rec.info.Size() {
		return true
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return sha256.Sum256(onDisk) != rec.sum
}

// handleReload re-reads the file under an open document and installs those bytes into the
// SAME document (POST /api/reload). It is the remedy half of /pending 333, whose detection
// half shipped without one: the banner could say the file had moved and the only way to act
// on it was to close the tab and open the file again.
//
// **Same id, and that reverses what the Reload button used to do.** The button went through
// /api/open — a NEW document in a NEW tab, then a close of the old one — and argued the new
// id was honest because the bytes differ. The tree says otherwise: six sites replace
// doc.data under a stable id (handleUndo, handleRedo, handleSave, both commit doors, and
// installCeremonyResult), so "different bytes under one id" is already what this package
// means by an edit. Keeping the id is also what lets the client repaint the view it has
// instead of building a second one, which is the difference between a reload the user asked
// for and one that fires by itself and moves her furniture. The old route also reported
// sameFileOpen on every reload — true for the instant both documents were registered, and a
// false sentence by the time the user read it.
//
// **It commits through commitMutation rather than assigning doc.data, and that is the whole
// design** (ADR-009). That door already enforces ADR-008's byte cap, runs D29's ceremony
// freeze against the SERVER's bytes, re-tests registration under the write's own lock, and
// pushes the outgoing bytes onto the undo ring. So a convened document refuses a reload with
// no new predicate written for it, and a reload is undoable — which is the safety net an
// action the user did not ask for owes her.
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// Read unlocked, and `path` is the one document field that may be read that way: it is
	// written once before registerLocked publishes the document and never again. `disk`
	// next to it must not be (diskOf), and the two sitting together is how that gets done
	// by accident.
	path := doc.path
	if path == "" {
		httpError(w, http.StatusBadRequest,
			"that document was not opened from a file, so there is nothing to reload")
		return
	}
	// Up to maxPDFBytes of file I/O, deliberately outside s.mu — a syscall under the global
	// server mutex serializes every route behind it, and on a network mount a hung read
	// becomes a hung process.
	data, ref := readInstallablePDF(path)
	if ref != nil {
		httpError(w, ref.status, ref.msg)
		return
	}
	// The baseline is stamped from the bytes this reload just read, and stamped BEFORE the
	// commit rather than after it. Doing it afterwards would hash doc.data a second time
	// under the lock for the same answer; doing it before means a file rewritten again in
	// the window pairs a fresh stat with bytes it does not describe, so diskChanged reports
	// true and the banner re-arms. That direction is the safe one — it re-asks a question
	// already answered, where the reverse would go quiet over a file that had moved.
	stamp := stampFor(path, data)
	// The state the undo entry records, read ONCE. Unlike the operation routes this one
	// does not consume the document's bytes at all — the new content comes from the file —
	// so there is no second read that could disagree with it and record an undo target the
	// document never held.
	before := s.docBytes(doc)
	if err := s.commitMutation(doc, before, data); wroteCommitFailure(w, err) {
		return
	}
	s.mu.Lock()
	// Only onto a document the registry still holds. commitMutation made that test under
	// its own hold and released it; a close landing in the gap must not leave a baseline on
	// a dropped document. **Not optional either way**: without the re-stamp the document
	// reports diskChanged from the moment it is reloaded, so the banner is armed forever
	// and the user learns to ignore it — the trap handleSave names at its own re-stamp.
	if s.isRegisteredLocked(doc) {
		doc.disk = stamp
	}
	s.mu.Unlock()
	writeJSON(w, s.docResponse(doc))
}
