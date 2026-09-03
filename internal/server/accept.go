package server

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
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
// # What it persists, and what changed (P08.S01)
//
// It stores the invitation **text** in the vault, keyed by ceremony, and pins the convener.
//
// It used to store nothing, on the argument that "the arm already takes the invitation text on
// every call, so nothing needs it at rest". That was true and is no longer: D24 makes a ceremony
// span quitting Nib, and a party who has accepted an invitation and then restarts has a pin, an
// identity, and no way to be a party to the ceremony again — `ceremonyFor` starts at
// `ParseInvitation(text)`, and the rendezvous key, both salts and the channel binding are all HKDF
// over the secret inside that text. The manual step D21 removed came back one process boundary out.
//
// **The vault, never `~/nib/ceremonies/`.** The text contains the secret, and D29 puts key material
// in the sealed store rather than beside the documents. It could not go in the mirror in any case:
// `WriteMirror` needs a Record, and an invitee holds none until the document reaches its hop.

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
	text := strings.TrimSpace(req.Invitation)
	inv, err := ceremony.ParseInvitation(text)
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
	// **Which party this machine is, recorded while the vault is open (P06.S02).** `me` is right
	// here and was thrown away; a reader without a vault cannot work it out, because it needs
	// `identity(v)`. Best-effort on purpose: a ceremony whose marker failed to write is readable,
	// resumable and signable exactly as before, and refusing an accept over a label would trade
	// the whole ceremony for it. The panel says "we do not know which of these is you" until the
	// next write, which is the honest sentence for an absent marker.
	if merr := ceremony.WriteMe(defaultOutputDir(), inv.ID, me); merr != nil {
		log.Printf("accepted ceremony %s: could not record which party this machine is: %v — the "+
			"ceremony works, but the panel cannot show your position in it", inv.ID, merr)
	}
	n, perr := pinCeremonyRoster(v, inv.ID, []ceremony.Party{convener}, me)
	if perr != nil {
		httpError(w, http.StatusInternalServerError,
			"the invitation was read but its pin could not be saved, so nothing was "+
				"accepted: "+perr.Error())
		return
	}
	// **The persist FAILS the accept, and that is the whole point of doing it here (P08.S01).**
	//
	// The half-state this refuses is a pin with no stored invitation: arming would then get past
	// `pinnedLabel` — the check the pin exists to satisfy — and fail at `ceremonyFor` with nothing
	// to parse, on a machine whose user was told the invitation was accepted. A 500 that names both
	// halves is the honest answer; a logged warning would leave the user to discover it at the
	// moment the baton arrives.
	//
	// **A different proceeding may not take over this ceremony id's slot (/pending 318, door 2).**
	//
	// `AddCeremonyInvitation` upserts BY CEREMONY, so accepting an invitation whose id collides
	// with one already stored replaces it silently. That is worse than the mirror half it pairs
	// with: `armInvitation` resolves `req.Ceremony` from the vault, so after the swap the panel row
	// still labelled *ceremony X* arms for the attacker's ceremony Y — the user's "continue" button
	// points at a different proceeding, against a peer who passes the spoken check because they are
	// legitimately pinned.
	//
	// Same discriminator as `WriteMirror`'s door and for the same reasons: `RosterHash` is what the
	// convener signed and it covers every axis. It is already required non-empty above, so there is
	// nothing to fall back on. A stored invitation that no longer parses is treated as absent
	// rather than as a collision — local damage must not block an honest accept, which is the
	// asymmetry `refuseDifferentProceeding` argues at the other door.
	if prev, ok := v.CeremonyInvitationFor(inv.ID); ok {
		if old, perr := ceremony.ParseInvitation(prev); perr == nil && old.RosterHash != inv.RosterHash {
			httpError(w, http.StatusConflict,
				"another ceremony is already stored under this id on this machine, and it is not "+
					"this one. Accepting would replace it, and the entry you already have would "+
					"then continue somebody else's proceeding. One of the two has to be convened "+
					"again.")
			return
		}
	}
	// The TRIMMED text, not `req.Invitation`: what is stored is what `ParseInvitation` accepted.
	if err := v.AddCeremonyInvitation(inv.ID, text); err != nil {
		httpError(w, http.StatusInternalServerError,
			"the invitation was read and its pin saved, but the invitation itself could not be "+
				"stored, so this machine could not rejoin the ceremony after a restart: "+err.Error())
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
