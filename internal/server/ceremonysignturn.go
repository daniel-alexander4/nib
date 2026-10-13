package server

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/vault"
)

// The convener's own turn (`/pending 869`, ADR-133).
//
// # What was missing
//
// Under D22's hub the convener dials every hop, and `POST /api/ceremony/hop` resolves its target as
// the roster's next signer — so at the convener's OWN turn there is nobody to dial and the hop
// refused, while `/api/cosign/sign` and the window's `/api/session/initiate` refuse a convened
// document because the window sends no invitation. A convener who signs — the setup sheet's default
// — had no route in the window that put their signature on the document, at any roster position.
// The signing door itself always worked there (`convenerturn_test.go`); what was missing was a
// route that reaches it.
//
// # What this is, and what it is not
//
// It is a hop with the dial taken out. Everything before the signature is the hop's — `hopTarget`
// resolves the ceremony, the pinned document and the turn; the arrival and deadline checks are the
// ones `runHopDial` runs; `buildCoSigned` is the one signing door — and everything after it is the
// hop's too: `installCeremonyResult`, then `mirrorHop` before the response returns. Nothing leaves
// the machine and no invitation is used to reach anybody.
//
// It is NOT a way for a party who did not convene to sign: they are called (D22), and `hopTarget`
// refuses them as it refuses them a hop. And it is not a second quote route: the block to draw is
// answered by `GET /api/ceremony/hop`, which already answered `mine` and `contributes` here and now
// carries the lines beside them.
type ceremonySignTurnRequest struct {
	// Ceremony is the id — the only thing the client chooses, as on the hop.
	Ceremony string `json:"ceremony"`
	// Appearance is the rendered signature block, base64 PNG, drawn from the quote's lines.
	Appearance string `json:"appearance"`
	// When echoes the quote's timestamp, so the block the convener read is the block signed.
	When string `json:"when,omitempty"`
}

// ownTurnAttestation is the attestation a convener signs at their own turn.
//
// **It names no wire peer, because there is none.** `cosignAttestation` starts from the party being
// dialled and requires them pinned; here nobody is dialled. What the signature accepts is the
// roster's answer — the signing predecessor, written by `StampCommitment`, the one door both
// contribution paths and every quote call — and nobody for the party who signs first. The label
// beside it is the one this vault pinned that predecessor under, or nothing.
//
// The quote and the signing route both call this, so the block drawn and the `/Reason` signed are
// one attestation (the rule `cosignAttestation` states for its own two callers).
func ownTurnAttestation(w http.ResponseWriter, v *vault.Vault, roster p2p.Roster, meFP, rawWhen string) (p2p.Attestation, bool) {
	when, werr := signedWhen(rawWhen, time.Now())
	if werr != nil {
		httpError(w, http.StatusBadRequest, werr.Error())
		return p2p.Attestation{}, false
	}
	if when.IsZero() {
		when = time.Now()
	}
	att := p2p.Attestation{Signer: "Nib User", Intent: roster.Intent, When: when}
	p2p.StampCommitment(&att, roster, meFP)
	if fp, err := parseFingerprint(att.AcceptedPeer); att.AcceptedPeer != "" && err == nil {
		if label, ok := pinnedLabel(v, fp); ok {
			att.AcceptedPeerLabel = label
		}
	}
	// The joint height rule, asked of the stamped block — the block that will exist. Convene has
	// already refused a recital that cannot fit; this is the same measurement at the signature.
	if !p2p.BlockFits(att) {
		lines, limit, worst, _ := p2p.BlockOverflow(att)
		httpError(w, http.StatusBadRequest, fmt.Sprintf(
			"your signature block needs %d lines and %d is the limit, so it would render too small "+
				"to read and nothing was signed. The longest part is %s.", lines, limit, worst))
		return p2p.Attestation{}, false
	}
	return att, true
}

// handleCeremonySignTurn puts this machine's signature on the ceremony's document at its own turn.
func (s *Server) handleCeremonySignTurn(w http.ResponseWriter, r *http.Request) {
	var req ceremonySignTurnRequest
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// The ceremony, the PINNED document's bytes (ADR-001, ADR-004), the convener entitlement and
	// the deadline, and whose turn it is — the hop's own resolver, so the two routes cannot
	// disagree about any of them.
	rec, pdf, next, mine, ok := s.hopTarget(w, r, req.Ceremony)
	if !ok {
		return
	}
	if !mine {
		httpError(w, http.StatusConflict,
			"it is "+rosterLabel(next)+"’s turn to sign this document, not yours, so nothing was "+
				"signed — call them from the ceremony panel")
		return
	}
	// One signing in flight per ceremony: the hop's own slot, keyed on this party.
	release, held := s.holdHop(req.Ceremony, next.Fingerprint)
	if !held {
		httpError(w, http.StatusConflict,
			"Nib is already signing your turn for this ceremony — wait for that to finish")
		return
	}
	defer release()

	v := vaultFrom(r)
	cert, key, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read this machine's identity")
		return
	}
	myFP, ferr := sign.Fingerprint(cert)
	if ferr != nil {
		httpError(w, http.StatusInternalServerError, "could not read this machine's fingerprint")
		return
	}
	me := hex.EncodeToString(myFP)
	cer, ok := s.ownTurnCeremony(w, v, rec, me)
	if !ok {
		return
	}
	// **The same checks, in the same order, as the hop runs before it signs (`runHopDial`), through
	// the same functions (ADR-009)** — the document's own record against the one this machine
	// verified, the content anchor while the document is unsigned, the deadline with room for a
	// hop, and whether this party contributes at all. All of it BEFORE `buildCoSigned`: a signature
	// cannot be taken back off a document.
	now := time.Now()
	if err := cer.checkArrival(pdf, now, &s.sess); err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	if err := checkCeremonyDeadline(pdf, now); err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	carrying, cerr := cer.carries(me, pdf)
	if cerr != nil {
		httpError(w, http.StatusConflict, "this ceremony's document could not be read well enough "+
			"to tell whether you have already signed it, so Nib will not sign: "+cerr.Error())
		return
	}
	if carrying {
		// Unreachable behind `mine` today — a party whose turn it is contributes — and kept so the
		// route says what the one function says rather than assuming it (as the hop quote does).
		httpError(w, http.StatusConflict, "this is not a turn at which you sign — either your "+
			"signature is already on this document or another party signs first — so nothing "+
			"was signed")
		return
	}
	att, aok := ownTurnAttestation(w, v, cer.l3Roster(), me, req.When)
	if !aok {
		return
	}
	appearance, derr := decodeAppearance(req.Appearance)
	if derr != nil {
		httpError(w, http.StatusBadRequest, "the signature block could not be read: "+derr.Error())
		return
	}
	// Refused rather than defaulted, as on the hop: an empty appearance is an invisible signature,
	// and that is a silent downgrade for a window whose render failed.
	if len(appearance) == 0 {
		httpError(w, http.StatusBadRequest,
			"your signature block was not sent, so Nib will not sign: an empty block would put an "+
				"invisible signature on the document and there is no way to take one back off")
		return
	}
	signed, ok := s.buildCoSigned(w, pdf, cert, key, att, appearance, cer.l3Roster())
	if !ok {
		return
	}
	// **Installed and mirrored exactly as a hop's result is** — the relay door (ADR-008's byte cap,
	// no freeze: this is a party's signature arriving, not a user edit), then the mirror BEFORE the
	// response returns (C22; ADR-012 — on this machine the mirror is the durable copy of the
	// signature just made).
	s.adoptForCeremony(req.Ceremony, pdf)
	installed, ierr := s.installCeremonyResult(req.Ceremony, arrivalDocName("", cer, signed), signed)
	if wroteCommitFailure(w, ierr) {
		return
	}
	s.mirrorHop(signed)
	writeJSON(w, s.docResponse(installed))
}

// ownTurnCeremony is the ceremony identity the own-turn checks are asked of, or a written refusal.
//
// **Minted through the one door and then disarmed.** `checkArrival`, `carries` and `l3Roster` are
// methods of a `*ceremonyID` and read only its invitation; the door that builds one from a verified
// record is `mintInvitationFor` (entitlement and deadline inside it). It mints FOR a party, so it is
// asked for a party other than this machine, and the channel secret that came with it is dropped at
// once: nobody is reached at this turn, and a secret this function has no use for is not carried
// through a signing path. The identity has no endpoint, no hop and no gate — nothing here dials.
func (s *Server) ownTurnCeremony(w http.ResponseWriter, v *vault.Vault, rec ceremony.Record, me string) (*ceremonyID, bool) {
	var other ceremony.Party
	for _, p := range rec.Roster {
		if !strings.EqualFold(p.Fingerprint, me) {
			other = p
			break
		}
	}
	if other.Fingerprint == "" {
		httpError(w, http.StatusConflict, "this ceremony's roster names nobody but this machine")
		return nil, false
	}
	inv, ierr := mintInvitationFor(v, rec, other, time.Now(), ceremonyHopBudget())
	if ierr != nil {
		switch {
		case errors.Is(ierr, errNotTheConvener):
			httpError(w, http.StatusForbidden, ierr.Error())
		case errors.Is(ierr, errNoCeremonySecret):
			httpError(w, http.StatusGone, "this ceremony has ended on this machine, so there is "+
				"nothing left to sign in it")
		default:
			httpError(w, http.StatusConflict, ierr.Error())
		}
		return nil, false
	}
	inv.Secret = nil
	// Stamped like every ceremony identity this server builds, so the rule "none is left unstamped"
	// has no exception to remember — though with no endpoint this one cannot bootstrap anything.
	return s.gateRendezvous(&ceremonyID{inv: inv, me: me}), true
}

// adoptForCeremony marks the open document whose bytes were just signed as this ceremony's baton,
// when no open document is marked yet.
//
// `installCeremonyResult` replaces the document whose `ceremony` field names the id, and that field
// is written at convene and by an earlier install — so a ceremony document opened cold (after a
// restart) carries none, and the signed result would open BESIDE the unsigned tab the convener
// pressed the button on. The document is identified by the bytes this request captured at its start
// (ADR-001): the same backing array, not merely equal content, so nothing is compared under `s.mu`
// and a document that has changed since is left alone.
func (s *Server) adoptForCeremony(id string, captured []byte) {
	if len(captured) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.docs {
		if d.ceremony == id {
			return
		}
	}
	for _, d := range s.docs {
		if d.ceremony == "" && len(d.data) == len(captured) && &d.data[0] == &captured[0] {
			d.ceremony = id
			return
		}
	}
}
