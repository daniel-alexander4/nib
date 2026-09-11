package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"nib/internal/ceremony"
)

// ceremonyNameRequest names a proceeding, or clears the name when Name is empty.
type ceremonyNameRequest struct {
	// Ceremony is the id whose label is being set.
	Ceremony string `json:"ceremony"`
	// Name is this machine's label. Empty CLEARS it, so the client needs no second route for
	// "forget the name" and the two states on disk stay one.
	Name string `json:"name"`
}

// handleCeremonyName sets this machine's own label for a proceeding.
//
// # Why a ceremony has a name at all, when it already has an Intent
//
// The Intent is the recital — the sentence every party signs, which D20 makes its only home. It is
// a statement of what is agreed, and it is often a whole clause: *"We agree to the lease of 14 Elm
// Row, Edinburgh, for a term of five years"*. That is the right thing to sign and the wrong thing
// to scan a list by. A name is the handle, and the two jobs are different.
//
// # Why the name is LOCAL
//
// Reasoned at `nameFile`, and the short version is that the alternatives are both worse. A shared
// name carried in the record but outside `rosterPreimage` is a claim on the wire that nothing
// anchors — the shape /pending 390, 437 and 440 each found. Inside the preimage it commits, which
// turns a convenience label into a signature-format flag day: `Record.Verify` refuses a newer
// `Version`, so every ceremony in flight would stop verifying the day it shipped.
//
// So each machine names its own, and two parties naming one ceremony differently is honest.
//
// # What it does NOT do
//
// It writes no record, touches no document, and changes nothing any other party can observe — so
// it is not a mutating route in the ADR-001 sense and carries no document pin. It is refused on a
// ceremony this machine has never heard of, because a label on nothing is a file in a directory the
// close-out will never visit.
func (s *Server) handleCeremonyName(w http.ResponseWriter, r *http.Request) {
	var req ceremonyNameRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id := strings.TrimSpace(req.Ceremony)
	if err := ceremony.ValidID(id); err != nil {
		httpError(w, http.StatusBadRequest, "that is not a ceremony id")
		return
	}
	// **A ceremony this machine holds, and the check is the LISTING's own** — `ReadStored` answers
	// for a pre-hop party too (`LoadAbsent` with a `me` marker), which is exactly the party most
	// likely to want a name: several proceedings, no documents yet, nothing to tell them apart.
	// What it refuses is an id with no directory at all.
	st := ceremony.ReadStored(defaultOutputDir(), id, time.Now())
	if st.State == ceremony.LoadAbsent && !st.Joined {
		httpError(w, http.StatusNotFound,
			"this machine has no ceremony with that id, so there is nothing to name")
		return
	}
	if err := ceremony.WriteName(defaultOutputDir(), id, req.Name); err != nil {
		httpError(w, http.StatusInternalServerError, "could not save that name: "+err.Error())
		return
	}
	// The stored name back, not the one that was sent: `WriteName` trims, folds newlines and caps
	// the length, so echoing the request would tell the client a name it does not have.
	writeJSON(w, struct {
		Ceremony string `json:"ceremony"`
		Name     string `json:"name"`
	}{Ceremony: id, Name: ceremony.ReadStored(defaultOutputDir(), id, time.Now()).Name})
}
