package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
)

// ceremonyCopyFacts is what the X-Nib-Ceremony-Copy header says about the bytes the route returns: the body is this
// machine's stored copy of the ceremony's document, so everything else about it rides beside it (ADR-072's shape).
//
// Four facts, each worded by the sheet (`ceremonyCopyLines`). The ceremony's id and the open file's size were here and
// were dropped before shipping: the client read neither, and a published field with no reader is the class
// `published.test.mjs` exists to refuse.
type ceremonyCopyFacts struct {
	// Ended is true when the copy came from `~/nib/ended/<id>` — the ceremony was closed out (ADR-012).
	Ended bool `json:"ended,omitempty"`
	// Signed reports whether the stored copy carries a signature at all. The mirror is this machine's LAST hop, so an
	// unsigned one is the convened original — before anybody signed.
	Signed bool `json:"signed,omitempty"`
	// Same is true when the open document IS the stored copy, byte for byte.
	Same bool `json:"same,omitempty"`
	// Extends is true when the open document begins with the stored copy byte for byte and is LONGER: everything past
	// `len(copy)` was appended after the version this machine held. Never true with Same (the P03 phase-close review:
	// an identical file read as "everything after it was appended").
	Extends bool `json:"extends,omitempty"`
}

// ceremonyCopyCause names why there is no stored copy to hand over (422, ADR-072).
type ceremonyCopyCause string

const (
	copyNoRecord            ceremonyCopyCause = "no-record"            // the open document names no ceremony
	copyRecordInvalid       ceremonyCopyCause = "record-invalid"       // it names one with a record that does not verify
	copyNotOnThisMachine    ceremonyCopyCause = "not-on-this-machine"  // no stored copy, live or ended, under that id
	copyDifferentProceeding ceremonyCopyCause = "different-proceeding" // a stored ceremony with that id is ANOTHER proceeding
	copyDamaged             ceremonyCopyCause = "damaged"              // the stored copy does not match its own record
	copyUnreadable          ceremonyCopyCause = "unreadable"           // anything else reading it failed
)

// ceremonyCopyRefusal is the 422 body: why there is no stored copy to hand over. A named type, so the published-shape
// censuses see its field (an anonymous struct escapes both).
type ceremonyCopyRefusal struct {
	Cause ceremonyCopyCause `json:"cause"`
}

// handleDocumentCeremonyCopy answers GET /api/document/ceremony-copy: the bytes this machine stored for the signing
// ceremony the open document belongs to, or a 422 naming why there are none (PLAN-returned-document P03.S03). It is the
// second link of the dispute surface's fallback chain, asked only when the user presses it — never on open (D10).
//
// **The ceremony is found from the record EMBEDDED in the document**, because `doc.ceremony` has one writer,
// `installCeremonyResult`, reached only when a hop's result arrives — and a document that came back is opened cold. **And it is handed over only when
// the stored record's roster commitment equals the embedded one's** (`ceremony.ReadMirrorFor`, which compares before it
// reports damage): `Record.Verify` is an internal-consistency check, so a record minted whole may name any id it likes,
// and without the comparison a planted id would choose which stored ceremony this document is compared with — or which
// one's damage it is told about. A different proceeding is refused as one cause and described no further.
//
// Behind `requireUnlocked` like every other route that reads an open document, though it reads nothing from the vault:
// a document route that answered while every other one refuses would be the one door that differs.
func (s *Server) handleDocumentCeremonyCopy(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	// The bytes and the registration in one hold, as `handleDocumentRevision` takes them: a document closed since
	// `resolveDoc` is ADR-004's 409, never answered.
	s.mu.Lock()
	if !s.isRegisteredLocked(doc) {
		s.mu.Unlock()
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	data := doc.data
	s.mu.Unlock()
	refuse := func(c ceremonyCopyCause) {
		writeJSONStatus(w, http.StatusUnprocessableEntity, ceremonyCopyRefusal{Cause: c})
	}
	now := time.Now()
	named, err := ceremony.Extract(data)
	if errors.Is(err, ceremony.ErrNoRecord) {
		refuse(copyNoRecord)
		return
	}
	if err != nil || named.Verify(now) != nil {
		refuse(copyRecordInvalid)
		return
	}
	pdf, ended, err := ceremony.ReadMirrorFor(defaultOutputDir(), named, now)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		refuse(copyNotOnThisMachine)
		return
	case errors.Is(err, ceremony.ErrDifferentProceeding):
		refuse(copyDifferentProceeding)
		return
	case errors.Is(err, ceremony.ErrMirrorDamaged):
		refuse(copyDamaged)
		return
	case err != nil:
		refuse(copyUnreadable)
		return
	}
	same := bytes.Equal(data, pdf)
	facts := ceremonyCopyFacts{Ended: ended, Signed: sign.HasSignatureBlob(pdf), Same: same,
		Extends: !same && bytes.HasPrefix(data, pdf)}
	hdr, err := json.Marshal(facts)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not describe the ceremony's copy")
		return
	}
	w.Header().Set("X-Nib-Ceremony-Copy", string(hdr))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	_, _ = w.Write(pdf)
}
