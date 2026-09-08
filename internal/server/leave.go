package server

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
)

// The leave route (P05.S01, D17): a party stops taking part in a ceremony, locally.
//
// # Why it exists
//
// Since D14 (P02.S02) accepting an invitation ARMS, and the arm is renewed at every unlock. So a
// party who has changed their mind, or who was invited to something they want nothing to do with,
// had no lever at all short of quitting Nib — and quitting is not a decision, it is a pause. This
// is the lever.
//
// # Why it is a prune and not a message
//
// `rearmCeremonies` keys on the stored invitation: a ceremony this machine holds none for is
// skipped, so removing it stops the arm ever coming back. Nothing has to be told and nothing has to
// be signed, which is exactly what makes leaving local.
//
// **The prune alone does not release the arm that is already up, and this comment claimed it did**
// (*"stops the arm on the next sweep"*, until /pending 378 read the sweep). A skip is not a
// teardown, so the standing arm held the single interactive slot until Nib quit. The route calls
// `stopListeningFor` for that; the prune is what stops it returning.
//
// # Why it is NOT a decline, stated at the door because the two are one keystroke apart
//
// A decline is an attested refusal the convener learns about and the roster is entitled to act on.
// Leaving reaches nobody. Minting a termination here would put a withdrawal on the record that the
// user did not choose, and `Termination`'s set is closed at two for reasons that do not bend for a
// local action. `closeOutCeremony` does not mint one — the convener does — so routing through it
// is safe, and `StateLeft` keeps the two apart in the local receipt.

type leaveRequest struct {
	Ceremony string `json:"ceremony"`
}

func (s *Server) handleCeremonyLeave(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req leaveRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id := strings.TrimSpace(req.Ceremony)
	if err := ceremony.ValidID(id); err != nil {
		httpError(w, http.StatusBadRequest, "that is not a ceremony id")
		return
	}

	// **Refused once this machine has signed, and the discriminator is P02.S02's.** On a machine
	// that did not convene, `WriteMirror`'s only production callers are this party's own completed
	// hop — so a record on disk means the hop has happened.
	//
	// The refusal is not paternalism about a decision the user is entitled to make. After signing,
	// leaving withdraws nothing: the signature is on the document, the other parties are relying
	// on it, and the roster's copy is unaffected. All it does is remove the invitation
	// `rearmDeliveries` needs, so the only effect is that this party's own finished copy never
	// arrives. It is a strictly self-harming action with no upside, and the honest response is to
	// say so rather than to perform it.
	st := ceremony.ReadStored(defaultOutputDir(), id, time.Now())
	if st.State == ceremony.LoadOK {
		httpError(w, http.StatusConflict,
			"you have already signed this document, so there is nothing left to withdraw — your "+
				"signature is on it and the other parties are relying on it. Leaving now would "+
				"only stop your own finished copy reaching you.")
		return
	}

	text, held := v.CeremonyInvitationFor(id)
	if !held {
		// Honest rather than a 404: the ceremony may well be on disk. What is absent is this
		// machine's participation in it, which is the thing being asked to end.
		httpError(w, http.StatusConflict,
			"this machine holds no invitation for that ceremony, so it is not taking part in it "+
				"and nothing is listening for it — there is nothing to leave.")
		return
	}
	inv, err := ceremony.ParseInvitation(text)
	if err != nil {
		httpError(w, http.StatusConflict,
			"this machine's copy of that invitation cannot be read, so Nib cannot tell whose "+
				"ceremony it is: "+err.Error())
		return
	}

	cert, _, ierr := identity(v)
	if ierr != nil {
		httpError(w, http.StatusInternalServerError, "could not read your signing identity")
		return
	}
	myFP, ferr := sign.Fingerprint(cert)
	if ferr != nil {
		httpError(w, http.StatusInternalServerError, "could not read your own fingerprint")
		return
	}
	// **The convener is refused and pointed at the right door.** A convener holds the record and
	// the document; their leaving would strand every other party mid-proceeding with no way to
	// finish and nothing to tell them why. Tearing down a ceremony you convened is `unconvene`,
	// which exists and takes the four stores this one does.
	if strings.EqualFold(hex.EncodeToString(myFP), inv.ConvenerFingerprint) {
		httpError(w, http.StatusConflict,
			"you convened this ceremony, so leaving it is not the thing you want — every other "+
				"party is waiting on you, and walking away would leave them with no way to "+
				"finish and nothing saying why.")
		return
	}

	// **The live arm goes HERE, before the close-out, and the route's own doc above was wrong
	// about this (/pending 378).** It said pruning the invitation *"stops the arm on the next sweep
	// and it never comes back"* — the second half is true and the first is not:
	// `rearmCeremonies` **skips** a ceremony it holds no invitation for rather than tearing one
	// down, so nothing released the arm this user has just said they want no part of. It held the
	// single interactive slot until Nib was quit, which is the exact condition the lever exists to
	// relieve.
	//
	// **Before the close-out rather than after, so a failing vault teardown cannot leave the user
	// still listening.** `closeOutCeremony` reports its failure and returns 500; disarming after it
	// would mean the one path that answers "Nib could not finish leaving" is also the one that
	// leaves the socket open. Disarming an unarmed ceremony is a no-op, so the order costs nothing
	// on the ordinary path.
	s.stopListeningFor(id)
	if err := s.closeOutCeremony(v, id, ceremony.StateLeft, time.Now()); err != nil {
		httpError(w, http.StatusInternalServerError,
			"this machine could not finish leaving that ceremony, so it may still hold its pins "+
				"or its secret: "+err.Error())
		return
	}
	writeJSON(w, struct {
		Ceremony string `json:"ceremony"`
		State    string `json:"state"`
	}{Ceremony: id, State: ceremony.StateLeft})
}
