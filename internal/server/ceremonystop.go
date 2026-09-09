package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"nib/internal/ceremony"
)

// The convener's stop (P01.S02c, `/pending 428`, D12).
//
// # What D12 said, and why it was not true
//
// *"A signature is an append; a wrong one cannot be undone, and the remedy is to abandon and
// re-convene, losing every signature collected."* There was no abandon. The ceremony routes were
// convene, invites, ceremonies, next, accept, leave, draft, deliver and delivery; `unconvene` is the
// convene ROLLBACK with one caller inside the failure path; and `endCeremony` had exactly one caller
// — on `p2p.ErrCoSignDeclined`. **A ceremony ended only when a counterparty refused at a hop.**
//
// So a convener who had just watched a wrong signature land had: no control, a ceremony that stayed
// live in the rail offering actions until its deadline, no way to tell the other parties, and — on
// re-convening — a first proceeding still running alongside the second with every party still
// holding a valid invitation to it.
//
// # Why this is not `abandoned`, and why the word matters more than usual
//
// `StateAbandoned` already exists as a DERIVED local conclusion meaning *"a proceeding that ended
// without reaching this machine at all"*, rendered to users as **"No further word"**. Attesting
// under that word would print the product's considered phrase for silence for the one act the
// convener performed deliberately and announced. And `Receipt.State` carries both vocabularies with
// a conflict rule that compares STRINGS, so the two would merge with no trace. See
// `ceremony.StateStopped` for the full argument and for why `cancelled` is worse still.
//
// # The round runs INLINE, and that is the difference between stopping and going away
//
// A two-step stop — mint here, then find and press "Send everyone their copy" — would be left
// unpressed, and the ceremony would look live on every other machine while the convener believed
// they had stopped it. That is the state this route exists to remove, so it must not be able to
// produce it. The round is the same one a completed ceremony uses; what differs is only the payload,
// which `ceremony.DeliversDocument` decides in one place.
type ceremonyStopRequest struct {
	// Ceremony is the id. Nothing else: who may stop it, what is attested and who is told are all
	// decided here.
	Ceremony string `json:"ceremony"`
}

// ceremonyStopResponse reports what the stop attested and who was reached.
type ceremonyStopResponse struct {
	Ceremony string `json:"ceremony"`
	// State is the attested end state, echoed so a client renders the server's word rather than
	// holding its own copy of the vocabulary (D1).
	State string `json:"state"`
	// Parties is one row per party the round walked, exactly as `/api/ceremony/deliver` returns —
	// same shape because it is the same round, and a second shape would be a second thing for a
	// reader to learn.
	Parties []deliveryOutcome `json:"parties"`
}

// handleCeremonyStop attests that the convener has stopped this proceeding, then tells everybody.
func (s *Server) handleCeremonyStop(w http.ResponseWriter, r *http.Request) {
	var req ceremonyStopRequest
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Ceremony) == "" {
		httpError(w, http.StatusBadRequest, "name the ceremony to stop")
		return
	}
	v := vaultFrom(r)
	rec, pdf, err := ceremony.ReadMirror(defaultOutputDir(), req.Ceremony, time.Now())
	if err != nil {
		httpError(w, http.StatusConflict,
			"this ceremony could not be read on this machine: "+err.Error())
		return
	}
	// **Entitlement through the same door the mint uses, with a ZERO budget.** Only the convener
	// may attest an end state — `VerifyAgainst` enforces that at every reader, and refusing here
	// as well means the user is told rather than the far end silently rejecting. The budget is zero
	// because stopping a proceeding whose deadline has passed is still a legitimate thing to do:
	// what a hop needs room for is an exchange, and this is a statement.
	if merr := checkMintAllowed(v, rec, time.Now(), 0); merr != nil {
		if errors.Is(merr, errNotTheConvener) {
			httpError(w, http.StatusForbidden,
				"only the convener can stop a ceremony: this machine is a party to this "+
					"proceeding, not the one that convened it. To take yourself out of it, leave it")
			return
		}
		httpError(w, http.StatusConflict, merr.Error())
		return
	}
	cert, key, ierr := identity(v)
	if ierr != nil {
		httpError(w, http.StatusInternalServerError, "could not read this machine's identity")
		return
	}
	// **Refused if it has already ended, rather than minting a second attestation.**
	// `WriteTermination` is write-once and conflicts on a differing state, so a second, different
	// end state would be refused there — but with a sentence about storage. This says the true
	// thing: the proceeding is already over.
	if prev, perr := ceremony.ReadTermination(defaultOutputDir(), rec); perr == nil {
		httpError(w, http.StatusConflict,
			"this ceremony has already ended ("+prev.State+"), so there is nothing to stop")
		return
	}
	t, terr := ceremony.SignTermination(rec, ceremony.StateStopped, cert, key)
	if terr != nil {
		httpError(w, http.StatusInternalServerError, "could not attest the stop: "+terr.Error())
		return
	}
	if werr := ceremony.WriteTermination(defaultOutputDir(), t); werr != nil {
		httpError(w, http.StatusInternalServerError,
			"Nib could not record that this proceeding has stopped, so the other parties cannot be "+
				"told: "+werr.Error())
		return
	}
	// **The round, INLINE.** See the file header: a stop whose delivery is a second button the user
	// has to find is a stop that leaves the ceremony looking live on every other machine.
	// `runDeliveryRound` reads the termination back off disk and chooses the payload through
	// `DeliversDocument`, so a stopped ceremony carries the attestation rather than the
	// partially-signed document.
	// The mirror's document is handed in as every caller does; `runDeliveryRound` replaces it with
	// the attestation because `DeliversDocument` says a stopped ceremony has no finished file. The
	// bytes are passed rather than nil so the round's own reading of what it carries stays in one
	// place instead of being split between here and there.
	out, rerr := s.runDeliveryRound(r.Context(), v, rec, pdf, nil)
	if rerr != nil {
		// The attestation IS written, so the stop happened even though the telling did not. Said in
		// those terms rather than as a bare failure: the user's next action is to re-run delivery,
		// not to stop it again.
		httpError(w, http.StatusConflict,
			"this proceeding is recorded as stopped on this machine, and Nib could not tell the "+
				"other parties: "+rerr.Error()+" — use Send everyone their copy to try again")
		return
	}
	writeJSON(w, ceremonyStopResponse{Ceremony: req.Ceremony, State: t.State, Parties: out})
}
