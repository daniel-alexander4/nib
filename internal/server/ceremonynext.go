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
	// signed), "ended" (a party declined, or the deadline passed), "accepted" (this machine is a
	// party and nothing has arrived for it yet), or "unavailable" (the document or record could not
	// be read well enough to say).
	//
	// **More than two, and each addition is a screen the others got wrong.** A route that answered
	// "waiting for X" or nothing would make "the ceremony is finished" and "Nib cannot tell" the
	// same screen, and those want opposite actions from the user — one is done, the other needs
	// somebody to look at a file. `ended` and `accepted` were each carved out of a state that was
	// answering for them wrongly, not added for symmetry.
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
	// Parties is every roster member with what the document says about them (P04.S02, D6). It is
	// what the rail's worklist renders above `ceremony.SittingCeiling`.
	//
	// **On THIS route and not on the listing, and the measurement decided it.**
	// `internal/p2p/railcost_test.go` measured `NextContributor` at 26.6 ms on a 200-page signed
	// document — two to four times a single `sign.Verify` — so per-party progress on
	// `/api/ceremonies` would be tens of milliseconds per ceremony per request, on a route that
	// already pays a `ReadMirror` each (`/pending 360`). Here the document is already open and
	// already walked, so this costs nothing that was not already spent.
	//
	// **In ROSTER order, so the client never joins an index.** `Position` above is 1-based within
	// the SIGNING order and the panel numbers over the full roster; joining them would mark a
	// non-signing convener as the current signer in every ceremony the convene form can produce,
	// since `Convene` prepends the convener at position 0. Sending the states removes the join.
	Parties []ceremonyPartyState `json:"parties,omitempty"`
	// Worklist is whether this ceremony is past `ceremony.SittingCeiling` and should render as a
	// worklist rather than as one sentence (D6).
	//
	// **The SERVER decides, and the client is told.** The threshold is `ceremony.SittingCeiling`,
	// which already exists and already reaches the user through `WarnSittingCeiling` at convene —
	// its own doc calls it "what the UI should be designed and copy-written for". A JS comparison
	// against a literal 8 would be a second copy of that number, and two roster-size regimes that
	// agree on the day they are written is ADR-009's named failure. It is also D1's rule: the rail
	// renders `next`'s answer and the client holds no step of its own.
	Worklist bool `json:"worklist,omitempty"`
}

// ceremonyPartyState is one roster member's standing in the proceeding.
//
// **Derived, never decided here.** Every field is a reading of `p2p.Progress`'s two facts — the
// signing order and how many signatures form a valid prefix of it — which is the ONE walk
// (`ContributionProgress`). "Has party k signed" already has three implementations in this tree and
// two of them use different rules; a fourth would be ADR-009's named failure, so this type computes
// nothing and only re-expresses what the gate concluded.
type ceremonyPartyState struct {
	// Label is what the convener called them. No fingerprint: the phase's criterion is that the
	// primary flow contains no hex, and a fingerprint is hex.
	Label string `json:"label,omitempty"`
	// Capacity is the role they sign in, which D20 makes part of the agreement.
	Capacity string `json:"capacity,omitempty"`
	// State is one of: "signed", "signing" (their turn), "waiting" (not yet reached), or
	// "watching" (a party who does not sign at all — D22's non-signing convener).
	//
	// **"watching" is a fourth word and not an omission.** A non-signing party rendered as
	// "waiting" would tell a coordinator that somebody still owes a signature they will never
	// give, which at a full roster is exactly the misreading the worklist exists to prevent.
	State string `json:"state"`
	// IsMe marks this machine's own row, from the same marker the panel already uses.
	IsMe bool `json:"isMe"`
}

// partyStates re-expresses `p2p.Progress` as one row per ROSTER member, in roster order.
//
// It is the only place that maps the gate's conclusion onto words, and it decides nothing: a
// party's index in the SIGNING order against `Done` is the whole rule, and a party absent from that
// order does not sign.
func partyStates(roster []ceremony.Party, pr p2p.Progress, me string) []ceremonyPartyState {
	at := make(map[string]int, len(pr.Order))
	for i, e := range pr.Order {
		at[strings.ToLower(e.Fingerprint)] = i
	}
	out := make([]ceremonyPartyState, 0, len(roster))
	for _, p := range roster {
		row := ceremonyPartyState{
			Label:    p.Label,
			Capacity: p.Capacity,
			IsMe:     me != "" && strings.EqualFold(me, p.Fingerprint),
			State:    "watching",
		}
		if i, ok := at[strings.ToLower(p.Fingerprint)]; ok {
			switch {
			case i < pr.Done:
				row.State = "signed"
			case i == pr.Done:
				row.State = "signing"
			default:
				row.State = "waiting"
			}
		}
		out = append(out, row)
	}
	return out
}

// endedReason returns the person-facing sentence for a ceremony that has ended, or "" when it has
// not. It reads only the stored state, so it costs nothing and can run before the document is
// opened.
//
// **Two sources, and only one of them is attested.** `Stored.Ended` carries a termination a party
// signed — `declined` is the one that matters here. Expiry is *derived*: nobody can sign "the
// deadline passed", which is why `ceremony.StateExpired` lives with the close-out's derived states
// and deliberately not in `Termination`'s set. Both end the proceeding; only one has an author.
func endedReason(st ceremony.Stored, now time.Time) string {
	if st.Ended != "" && st.Ended != ceremony.StateCompleted {
		if st.Ended == ceremony.StateDeclined {
			return "a party declined, so this ceremony has ended"
		}
		return "this ceremony has ended: " + st.Ended
	}
	// Zero means the record carries no deadline, which is not an expiry.
	if !st.Expires.IsZero() && now.After(st.Expires) {
		return "this ceremony's deadline has passed"
	}
	return ""
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
		// **`accepted` is a definite answer and `unavailable` is the absence of one, so the
		// commonest invitee state may not be reported as the second (/pending 377).** A party who
		// has accepted holds a directory with a `me` marker and no `record.json` until the document
		// reaches their hop. Nib has read everything there is to read about that ceremony and the
		// answer is *nothing has arrived yet* — which is not "Nib could not read enough to say",
		// the meaning `unavailable` carries in this route's own field doc.
		//
		// **A fifth `LoadState` was the other shape and the code refuses it.** `deliveryWindowFor`
		// (`session.go`) and `checkDeliveredPayload` (`delivery.go`) both test
		// `st.State == ceremony.LoadAbsent` to mean *exactly* this party, and each would silently
		// stop matching in the one case it was written for — the delivery arm's window and the
		// invitation-anchored end-state check. So the discriminator is a FIELD on `Stored` —
		// `Joined` — which is the argument `Stored.Ended`'s own doc already makes.
		//
		// **And it is `Joined` rather than `Me`, which the first cut got wrong and an existing
		// guard caught.** `Me` is a position and must stay empty on every class that is not
		// `LoadOK`; `Joined` is the marker's presence, which needs no roster to mean something.
		state := "unavailable"
		if st.State == ceremony.LoadAbsent && st.Joined {
			state = "accepted"
		}
		writeJSON(w, ceremonyNextResponse{Ceremony: id, State: state, Reason: st.Reason})
		return
	}

	// **A proceeding that has ENDED is answered here, before the document is opened.**
	//
	// Until this, the three states were `waiting`, `complete` and `unavailable`, and this file
	// contained no occurrence of `Expires` or of a decline — so a ceremony whose deadline had
	// passed, or whose party had refused, still answered *somebody's turn* and the panel invited
	// the user to carry on with a proceeding that was over.
	//
	// **Before `ReadMirror`, and that placement is not tidiness.** Answering costs a document read
	// this route's own header measures at 10 / 69 / 195 ms for 100 / 500 / 1000 pages,
	// superlinear — and a ceremony that has ended has no next contributor to compute. The cheap
	// answer is also the correct one.
	//
	// **Not the close-out's predicate, and they are different questions rather than one rule with
	// two implementations.** `closeOutReason` waits for `Expires` plus `closeOutGrace` because it
	// decides whether to ARCHIVE a directory, and a late round must be allowed to finish first.
	// This decides what to tell a person to do *now*, and that turns the moment the deadline
	// passes. Unifying them would make the panel offer a call for the whole grace window.
	//
	// `completed` is deliberately NOT handled here: it already has its own state, reached below
	// through `p2p.ErrCeremonyComplete`, and rerouting it would change an answer that is correct.
	if reason := endedReason(st, now); reason != "" {
		writeJSON(w, ceremonyNextResponse{Ceremony: id, State: "ended", Reason: reason})
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
	// **One walk, two readings** (ADR-009). `NextContributor` is itself a thin reading of this, so
	// calling it here as well would verify the document a second time for an answer already held.
	pr, nerr := p2p.ContributionProgress(pdf, roster)
	if nerr == nil && pr.Complete {
		nerr = p2p.ErrCeremonyComplete
	}
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

	next := pr.Order[pr.Done]
	pos, of := signingPositionIn(roster, next.Fingerprint)
	writeJSON(w, ceremonyNextResponse{
		Ceremony: id, State: "waiting",
		Label: next.Label, Capacity: next.Capacity, Position: pos, Of: of,
		IsMe:    st.Me != "" && strings.EqualFold(st.Me, next.Fingerprint),
		MeKnown: st.Me != "",
		Parties: partyStates(rec.Roster, pr, st.Me),
		// Measured at P04.S01: with real capacities the card's one action leaves the fold at four
		// parties and with bare names it survives to sixteen, so eight sits inside the bracket.
		Worklist: len(rec.Roster) > ceremony.SittingCeiling,
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
