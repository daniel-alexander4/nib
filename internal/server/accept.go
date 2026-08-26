package server

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"nib/internal/ceremony"
	"nib/internal/sign"
)

// The accept route (P07.S02b): the invitee's half of D21.
//
// # Why a door of its own rather than a fold into the arm
//
// The obvious shape is to parse the invitation inside `handleSessionArm` and pin from it before
// the `pinnedLabel` check. Three things are wrong with that. It puts a vault WRITE on the arm
// path, so arming stops being idempotent — a re-arm after a timeout would re-pin. It ties the
// pin's lifetime to one arm, so a party who restarts Nib between reading the invitation and
// arming is unpinned again, which is exactly the manual step D21 removes. And it would move the
// refusal at `pinnedLabel`, which is a load-bearing check on two doors; this route leaves both
// byte-identical, which is what makes the change safe to reason about.
//
// # What it does NOT do
//
// It does not store the invitation, and it does not store its secret. The arm already takes the
// invitation text on every call (`armRequest.Invitation`), so nothing needs it at rest — and D29
// is explicit that the secret does not belong in `~/nib/ceremonies/`. Storing it in the vault
// would be defensible and is not needed by anything, so it is not done: the smallest thing that
// removes the manual pin is the pin.

type acceptRequest struct {
	Invitation string `json:"invitation"`
}

// acceptedParty is one roster entry as the API hands it back — the public facts only.
type acceptedParty struct {
	Fingerprint string `json:"fingerprint"`
	Label       string `json:"label"`
	Capacity    string `json:"capacity"`
	Signs       bool   `json:"signs"`
	// Name is the six-word pairing name, DERIVED from the fingerprint — the same identity
	// vocabulary every other peer control uses.
	Name string `json:"name"`
	// Convener and Self mark the two entries a reader needs to find without re-deriving them.
	Convener bool `json:"convener"`
	Self     bool `json:"self"`
}

type acceptResponse struct {
	Ceremony string          `json:"ceremony"`
	Roster   []acceptedParty `json:"roster"`
	// Pinned is how many parties this call pinned FOR THIS CEREMONY — one for a counterparty
	// by construction, because D22 is a hub and the convener is their only possible hop
	// partner.
	//
	// **It counts parties pinned, not pins created**, and the difference is stated because the
	// first draft's comment claimed the opposite: a party already in the peer list is counted
	// here too, since after this call they are pinned for this ceremony either way. Accepting
	// the same invitation twice is idempotent and reports the same number both times.
	Pinned int `json:"pinned"`
	// Signing is how many parties are obliged to sign, which is what a party reading this
	// wants to know about the proceeding they have been asked into.
	Signing int `json:"signing"`
}

func (s *Server) handleCeremonyAccept(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req acceptRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	inv, err := ceremony.ParseInvitation(strings.TrimSpace(req.Invitation))
	if err != nil {
		// ParseInvitation's four sentences are already written for a person — "that is not a
		// Nib invitation", "this invitation is damaged", and the two direction-aware version
		// ones. Replacing them with a single generic line is what made the old prefix check
		// tell users their ordinary invitation came from the future.
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	// **The commitment is required at the DOOR, not at MatchesRecord.**
	//
	// An invitation with no `RosterHash` carries nothing binding it to one ceremony record, so
	// it can never be reconciled against the document when it arrives. Pinning from it would
	// establish trust on the strength of something that can never be checked. `ParseInvitation`
	// does not enforce this — it is a v2 field and the parser's job is the format — so it is
	// enforced here, where the consequence lives.
	if inv.RosterHash == "" {
		httpError(w, http.StatusBadRequest,
			"this invitation carries no commitment, so nothing could ever bind it to a "+
				"ceremony document — ask for a fresh copy")
		return
	}
	cert, _, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError,
			"could not prepare your signing identity: "+err.Error())
		return
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read your own fingerprint")
		return
	}
	me := hex.EncodeToString(myFP)

	// The convener must be a roster member, and this is the first place anything checks it.
	//
	// `ceremonyFor` derives the hop from `ConvenerFingerprint` and refuses any pair without
	// the convener at one end — so an invitation naming a convener who is not in the roster
	// produces a ceremony in which no hop exists, discovered at arm time as "this invitation
	// does not put you and that peer on one hop". Refusing here names the actual fault.
	convener, found := rosterEntry(inv.Roster, inv.ConvenerFingerprint)
	if !found {
		httpError(w, http.StatusBadRequest,
			"this invitation names a convener who is not one of its parties, so it describes "+
				"a ceremony with no hops in it")
		return
	}
	if _, mine := rosterEntry(inv.Roster, me); !mine {
		// Named rather than generic: the ordinary cause is an invitation forwarded to the
		// wrong person, and "you are not one of the parties" is the sentence that lets them
		// work that out.
		httpError(w, http.StatusBadRequest,
			"this invitation does not name you as one of its parties — it was meant for "+
				"somebody else")
		return
	}
	if strings.EqualFold(me, inv.ConvenerFingerprint) {
		// The convener's pins are established by the convene route. Accepting your own
		// invitation would pin nobody (the door skips self) and report success, which reads
		// as "the ceremony is ready" to whoever called it.
		httpError(w, http.StatusBadRequest,
			"you convened this ceremony — its parties were pinned when you convened it, and "+
				"there is nothing here for you to accept")
		return
	}

	// **The convener alone, and D22 is why.** `hopBetween` refuses any pair that does not have
	// the convener at one end, so this party can never be on a hop with anybody else; pinning
	// the rest of the roster would pin up to thirty peers it can never dial. See
	// pinCeremonyRoster.
	n, perr := pinCeremonyRoster(v, inv.ID, []ceremony.Party{convener}, me)
	if perr != nil {
		httpError(w, http.StatusInternalServerError,
			"the invitation was read but its pin could not be saved, so nothing was "+
				"accepted: "+perr.Error())
		return
	}

	roster := make([]acceptedParty, 0, len(inv.Roster))
	signing := 0
	for _, p := range inv.Roster {
		if p.Signs {
			signing++
		}
		fp, _ := parseFingerprint(p.Fingerprint)
		roster = append(roster, acceptedParty{
			Fingerprint: p.Fingerprint,
			Label:       p.Label,
			Capacity:    p.Capacity,
			Signs:       p.Signs,
			Name:        nameOrEmpty(fp),
			Convener:    strings.EqualFold(p.Fingerprint, inv.ConvenerFingerprint),
			Self:        strings.EqualFold(p.Fingerprint, me),
		})
	}
	writeJSON(w, acceptResponse{
		Ceremony: inv.ID,
		Roster:   roster,
		Pinned:   n,
		Signing:  signing,
	})
}

// rosterEntry finds a party by fingerprint, case-insensitively.
//
// Case-insensitive although `ParseInvitation` lowercases every roster fingerprint, because this
// is also called with `ConvenerFingerprint`, which the parser does NOT normalise — and a
// convener whose own fingerprint is uppercase would otherwise be reported as absent from a
// roster that contains them.
func rosterEntry(roster []ceremony.Party, fingerprint string) (ceremony.Party, bool) {
	for _, p := range roster {
		if strings.EqualFold(p.Fingerprint, fingerprint) {
			return p, true
		}
	}
	return ceremony.Party{}, false
}
