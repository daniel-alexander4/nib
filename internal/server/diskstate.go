package server

import (
	"crypto/sha256"
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
	if d.path == "" {
		d.disk = nil
		return
	}
	info, err := os.Stat(d.path)
	if err != nil {
		// Nothing to compare against later; diskChanged answers false and says nothing
		// rather than warning about a file it could not read.
		d.disk = nil
		return
	}
	d.disk = &diskState{info: info, sum: sha256.Sum256(d.data)}
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
