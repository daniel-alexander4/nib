package server

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
)

// Whose turn is it — the panel's enabled action, from the server's own L3 door (P06.S03).
//
// **The rule is not written here and must never be.** P06's criterion is that the panel's enabled
// action is *"computed from the record by the same function the server's L3 check uses"*, and that
// function is `p2p.NextContributor` — which P07.S03a wrote in its question form one phase early,
// for this slice, saying so in its own doc: *"a predicate that could only refuse would force that
// slice to retrofit a read-only query, which is two derivations of one rule and the ADR-009 shape
// this gate exists to avoid."* `AdmitContribution`, the refusing form, is built on the same call.
// This file is a route, not a rule.
//
// **Its own route rather than a field of the listing, and that is a cost decision inherited from
// P08.S03.** `NextContributor` needs the DOCUMENT, and `ListStored` never opens one — measured at
// 10 / 69 / 195 ms for 100 / 500 / 1000 pages, superlinear, with the whole listing designed around
// not paying it. A `next` field on `ceremoniesResponse` would pay that per ceremony per listing,
// which is what `/pending 360` is already about. One ceremony, fetched when a user opens that card:
// the cost lands where somebody asked a question and nowhere else.
//
// **Lock-free, on P06.S01's footing and for its reason.** Nothing here needs the vault: the record
// and the document are ordinary files under `~/nib`, and *"is it my turn"* is answerable because
// P06.S02 recorded which party this machine is. It wears `requirePublicLoopback` — the origin
// check `requireUnlocked` does not apply to GET — and it is a pure read, which `/pending 365`'s
// class is about keeping true.

// ceremonyNextResponse is what the panel renders as the next action.
type ceremonyNextResponse struct {
	// Ceremony is the id asked about, echoed so a late response cannot be rendered against the
	// wrong card.
	Ceremony string `json:"ceremony"`
	// State is what L3 concluded: "waiting" (somebody's turn), "complete" (every signing party has
	// signed), or "unavailable" (the document or record could not be read well enough to say).
	//
	// **Three states and not two.** A route that answered "waiting for X" or nothing would make
	// "the ceremony is finished" and "Nib cannot tell" the same screen, and those want opposite
	// actions from the user — one is done, the other needs somebody to look at a file.
	State string `json:"state"`
	// Label, Capacity and Position describe the party whose turn it is, empty unless State is
	// "waiting". Position is 1-based within the SIGNING order, which is what a person counts.
	Label    string `json:"label,omitempty"`
	Capacity string `json:"capacity,omitempty"`
	Position int    `json:"position,omitempty"`
	Of       int    `json:"of,omitempty"`
	// IsMe is whether that party is this machine, from the marker P06.S02 records.
	//
	// **Its absence and `false` are different facts and the panel must be able to tell them
	// apart**, which is why `MeKnown` sits beside it: a machine that never recorded its position
	// is not a machine whose turn it is not.
	IsMe    bool `json:"isMe"`
	MeKnown bool `json:"meKnown"`
	// Reason is the sentence for a non-waiting state, already written for a person.
	Reason string `json:"reason,omitempty"`
}

// handleCeremonyNext answers whose turn it is for one ceremony.
func (s *Server) handleCeremonyNext(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("ceremony")
	if err := ceremony.ValidID(id); err != nil {
		httpError(w, http.StatusBadRequest, "that is not a ceremony id")
		return
	}
	now := time.Now()
	root := defaultOutputDir()
	// **`ReadStored` first, for its four classes.** It is the door that already turns a damaged,
	// absent, skewed or unverifiable directory into a sentence written for a person, and answering
	// this question over a record that did not verify would be reading an order out of a file this
	// machine has refused to trust — the same rule `Stored.Me` follows one field over.
	st := ceremony.ReadStored(root, id, now)
	if st.State != ceremony.LoadOK {
		writeJSON(w, ceremonyNextResponse{Ceremony: id, State: "unavailable", Reason: st.Reason})
		return
	}
	rec, pdf, err := ceremony.ReadMirror(root, id, now)
	if err != nil {
		writeJSON(w, ceremonyNextResponse{Ceremony: id, State: "unavailable",
			Reason: "this ceremony's document could not be read on this machine: " + err.Error()})
		return
	}

	// **The roster comes from `record.json` and never from the document.** `l3Roster`'s own doc
	// states the rule for the arm side — *"a gate reading the document's own record answers its own
	// question"* — and it holds here for the same reason. `record.json` is the verified record
	// written at accept or convene time; `ceremony.Extract(pdf)` would be the document's own claim
	// about itself. Both are reached through `l3RosterFrom`, which is the one conversion.
	// **The commitment is DERIVED here, exactly as `Invitation` derives it** — `Record.RosterHash`
	// is a method returning bytes, and `hex.EncodeToString` of it is what an invitation carries
	// (`invitation.go:342`). A record whose roster cannot be digested cannot be reasoned about at
	// all, so that failure is `unavailable` rather than a roster with an empty commitment: L3
	// treats an empty `Commitment` as "the caller has none to offer" and would carry on checking
	// order against a roster whose integrity nothing established.
	rh, herr := rec.RosterHash()
	if herr != nil {
		writeJSON(w, ceremonyNextResponse{Ceremony: id, State: "unavailable",
			Reason: "this ceremony's roster could not be checked: " + herr.Error()})
		return
	}
	roster := l3RosterFrom(rec.Roster, hex.EncodeToString(rh), rec.Intent)
	next, nerr := p2p.NextContributor(pdf, roster)
	if nerr != nil {
		out := ceremonyNextResponse{Ceremony: id, State: "unavailable", Reason: nerr.Error()}
		if errors.Is(nerr, p2p.ErrCeremonyComplete) {
			// **Complete is an OUTCOME and not a failure**, and folding it into "unavailable"
			// would tell a user whose ceremony finished that Nib could not read their document.
			out.State, out.Reason = "complete", "every signing party has signed this document"
		}
		writeJSON(w, out)
		return
	}

	pos, of := signingPositionIn(roster, next.Fingerprint)
	writeJSON(w, ceremonyNextResponse{
		Ceremony: id, State: "waiting",
		Label: next.Label, Capacity: next.Capacity, Position: pos, Of: of,
		IsMe:    st.Me != "" && strings.EqualFold(st.Me, next.Fingerprint),
		MeKnown: st.Me != "",
	})
}

// signingPositionIn is where a party sits in the SIGNING order, 1-based, and how many there are.
//
// **The signing order and not the roster order**, which differ whenever a convener does not sign
// (D22) — "party 2 of 3" counted over the roster would tell a signer they are one place later than
// the document will show. `p2p.SigningOrder` is the one place that distinction lives, so this
// counts what it returns rather than filtering the roster again.
//
// Takes the roster the caller already built rather than rebuilding it, so the position and the
// answer it labels cannot be computed from two different rosters.
func signingPositionIn(roster p2p.Roster, fingerprint string) (pos, of int) {
	order := p2p.SigningOrder(roster)
	for i, e := range order {
		if strings.EqualFold(e.Fingerprint, fingerprint) {
			pos = i + 1
		}
	}
	return pos, len(order)
}
