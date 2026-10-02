package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unsafe"

	"nib/internal/sign"
)

// reFingerprint is a certificate fingerprint as `sign` writes one: the SHA-256 of the SPKI, 64 lowercase hex digits.
var reFingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)

// maxRevisionFactList caps each list the X-Nib-Revision header carries, so a hostile file cannot grow the header past
// what a browser accepts; a capped list says so (`truncated`).
const maxRevisionFactList = 1024

// revisionFacts is what the X-Nib-Revision header says about the bytes the route returns (plan-review W5): the body is
// the signed version itself, so everything else about it rides beside it. Built field by field from
// `sign.SignedRevision` — a projection, not a marshal, so each fact has a named reader here.
type revisionFacts struct {
	Obj             uint32   `json:"obj"`
	End             int64    `json:"end"`
	EarlierRevision bool     `json:"earlierRevision,omitempty"`
	RedefinedObj    uint32   `json:"redefinedObj,omitempty"`
	Later           []uint32 `json:"later,omitempty"`
	LaterUnchecked  bool     `json:"laterUnchecked,omitempty"`
	Earlier         []int64  `json:"earlier,omitempty"`
	Truncated       bool     `json:"truncated,omitempty"`
	// History is the working copy's RECORDED history (W9): "undo" when there are undoable edits, "evicted" when they
	// were dropped to keep memory bounded (ADR-003), "none" otherwise. "none" does NOT mean "the bytes as they
	// arrived": a barrier operation clears undo without marking it evicted, and a save replaces the bytes with no
	// history at all. P03 words it accordingly.
	History string `json:"history"`
	// Size is the length of the working copy the walk read — taken with `data`, under the same lock — so "your
	// version is the whole file" and "N bytes were added after it" compare one snapshot (the P03.S02 review: the client
	// had compared `end` with the size pdf.js loaded earlier, which a co-signature arriving in between makes false).
	Size int64 `json:"size"`
}

// revisionRefusal is the 422 body: why there is no version, and what nib could not read.
type revisionRefusal struct {
	Cause      sign.RevisionCause      `json:"cause"`
	Refused    []sign.RefusedSignature `json:"refused,omitempty"`
	Attributed bool                    `json:"attributed,omitempty"`
}

// handleDocumentRevision answers GET /api/document/revision?signer=<fingerprint>: the bytes of the version of this
// document that signer signed, with what is known about them in X-Nib-Revision — or a 422 naming why there is none
// (D7, plan-review W5). 422 and not 409: 409 is ADR-004's "not that document" and makes the client reconcile its tabs.
// On demand only (D10): nothing calls it when a document opens.
func (s *Server) handleDocumentRevision(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	fp := strings.ToLower(r.URL.Query().Get("signer"))
	if !reFingerprint.MatchString(fp) {
		// An unnormalised fingerprint would match nothing and read "not your signature" — a false statement.
		httpError(w, http.StatusBadRequest, "signer must be a certificate fingerprint: 64 hex digits")
		return
	}
	// The bytes and their history in ONE hold: a mutation between two reads would pair history with other bytes.
	s.mu.Lock()
	data := doc.data
	history := "none"
	switch {
	case len(doc.undo) > 0:
		history = "undo"
	case doc.historyEvicted:
		history = "evicted"
	}
	s.mu.Unlock()
	// doc.data is replaced wholesale and never mutated in place, so the slice is safe outside the lock; its address
	// is in the key so an edit landing mid-walk never shares a stale answer (I5).
	key := fmt.Sprintf("%s|%s|%p|%d", doc.id, fp, unsafe.SliceData(data), len(data))
	walk := s.revisionFor
	if walk == nil {
		walk = sign.SignedRevisionFor
	}
	v, _, _ := s.revisionFlight.Do(key, func() (any, error) {
		return walk(data, fp), nil
	})
	sr := v.(sign.SignedRevision)
	if sr.Prefix == nil {
		writeJSONStatus(w, http.StatusUnprocessableEntity, revisionRefusal{Cause: sr.Cause, Refused: sr.Refused, Attributed: sr.Attributed})
		return
	}
	facts := revisionFacts{Size: int64(len(data)), Obj: sr.Obj, End: sr.End, EarlierRevision: sr.EarlierRevision, RedefinedObj: sr.RedefinedObj,
		Later: sr.Later, LaterUnchecked: sr.LaterUnchecked, Earlier: sr.Earlier, History: history}
	if len(facts.Later) > maxRevisionFactList {
		facts.Later, facts.Truncated = facts.Later[:maxRevisionFactList], true
	}
	if len(facts.Earlier) > maxRevisionFactList {
		facts.Earlier, facts.Truncated = facts.Earlier[:maxRevisionFactList], true
	}
	hdr, err := json.Marshal(facts)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not describe the signed version")
		return
	}
	w.Header().Set("X-Nib-Revision", string(hdr))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(sr.Prefix)))
	_, _ = w.Write(sr.Prefix)
}
