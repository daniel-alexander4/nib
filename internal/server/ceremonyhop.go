package server

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
)

// The convener's own door onto a hop (P01.S02b, D1/D15/D22, `/pending 436`).
//
// # What was missing, and it was the whole flow
//
// Before this, a ceremony convened through the product could not be ADVANCED through it. The only
// dial was `/api/session/initiate`, which takes its ceremony from a pasted `invitation` form field
// — and no client surface has ever sent one (`grep -c sinInvite web/app.js web/index.html` -> 0/0).
// So `cer` was nil, `l3Roster()` returned the zero Roster, and `buildCoSigned` refused the convened
// document at its own gate: *"this document is part of a signing ceremony, so it cannot be
// co-signed outside it … Use the ceremony to sign it"* — advice naming a door that did not exist.
// Verified live before this slice, and green at tiers 4 and 6 throughout, because both harnesses
// post `-F invitation=$INV` by hand.
//
// **And the topology makes it the entire proceeding, not one gap.** Under D22's hub every hop has
// the convener at one end, the convener DIALS, and every other party waits — D14's accept arms them
// and the unlock sweep re-arms. So the convener's dial is the only user action that moves a ceremony
// forward at all.
//
// # Two routes, because the appearance cannot be rendered here
//
// The signature block is rasterised by the CLIENT — `renderAttestation` in `web/app.js` is the only
// producer in the tree, and a named search for a Go text-into-raster path finds none. So the client
// must be told what the block says before it can draw one, and it must not be the thing that decides
// whose turn it is. Hence a GET that answers *"it is Bob's turn, and here is the block you would
// draw"* and a POST that re-resolves the turn and dials. The server answers both from the same walk;
// the client never computes a turn.
//
// # Where the document comes from, and why it is NOT the mirror
//
// The deepdive that opened this slice measured the mirror's guarantee and found it is exactly what
// `mirror.go` says it is: `ReadMirror`'s `DocHash` check runs only while the document is UNSIGNED,
// and the `document.sha256` sidecar is *"a damage detector and not an access control: anyone who can
// truncate the document can delete the sidecar"*. Measured with an overlay build: a mirror truncated
// to one signature returns `err = nil`, and a total rollback to the convened bytes passes THROUGH
// the `DocHash` comparison rather than around it.
//
// That is a boundary the repo has already drawn deliberately — a local writer is outside the
// guarantee for `document.pdf` — so it is not a defect. What it is not is a basis for a route to
// pick a party and dial them: the boundary was drawn about a mirror nothing authorised from, and
// this would have been the first thing to authorise from it. So the hop reads the document from the
// OPEN TAB under `X-Nib-Doc`, exactly as `/api/session/initiate` does, and reads the mirror only for
// the roster and the invitation — which are covered by `record.json`'s signature, verified inside
// `ReadMirror`. `/pending 437-440` carry the mirror findings.
type ceremonyHopRequest struct {
	// Ceremony is the id. It is the ONLY thing the client chooses, and that is the point: the
	// party, the invitation and the document are all resolved here.
	Ceremony string `json:"ceremony"`
	// Appearance is the rendered signature block, base64 PNG, from the quote below. Absent on the
	// carry path, where this machine contributes nothing and there is no block to draw.
	Appearance string `json:"appearance,omitempty"`
	// When echoes the quote's timestamp, so the block the user reviewed is the block signed. Same
	// contract `/api/cosign/quote` -> `/api/session/initiate` already has, bounded by maxWhenSkew.
	When string `json:"when,omitempty"`
	// Address is an optional dial hint, as on every other ceremony route. Empty is the LAN ladder.
	Address string `json:"address,omitempty"`
}

// ceremonyHopQuote is what the GET answers: whose turn it is, and the block to draw for it.
type ceremonyHopQuote struct {
	Ceremony string `json:"ceremony"`
	// Party is the label the ROSTER gives the next signer. A person is named, never fingerprinted
	// — the panel's standing rule.
	Party string `json:"party"`
	// Mine is whether the next contributor is this machine. A convener who signs is first in the
	// signing order, so their own turn has nobody to dial and the rail must not offer a call.
	Mine bool `json:"mine"`
	// Contributes is whether THIS machine signs at this hop. False on the carry path, and it is
	// what tells the client whether it needs to render an appearance at all.
	Contributes bool `json:"contributes"`
	// Lines and Rect size the block, in the SHAPE the two existing quote routes already return —
	// `cosignQuote`'s own fields, so the client's `renderAttestation` needs no new branch.
	Lines []string   `json:"lines,omitempty"`
	Rect  [4]float64 `json:"rect,omitempty"`
	When  string     `json:"when,omitempty"`
}

// hopTarget resolves everything a hop needs from a ceremony id, or writes its own refusal.
//
// **Both routes share it, so the GET and the POST cannot disagree about whose turn it is.** They
// resolve independently — the POST does NOT trust the GET's answer — and the POST refuses if the
// turn has moved between them, which is the honest handling of a race the client cannot see.
func (s *Server) hopTarget(w http.ResponseWriter, r *http.Request, id string) (
	rec ceremony.Record, pdf []byte, next ceremony.Party, mine bool, ok bool) {
	if strings.TrimSpace(id) == "" {
		httpError(w, http.StatusBadRequest, "name the ceremony to advance")
		return rec, nil, next, false, false
	}
	// **The document comes from the open tab, pinned (ADR-001, ADR-004).** See the file header for
	// why it is not the mirror's copy.
	doc, dok := s.resolveDoc(w, r)
	if !dok {
		return rec, nil, next, false, false
	}
	// **Asked of the DOCUMENT'S OWN BYTES, not of `document.ceremony` — and the first cut used the
	// field and was wrong.** That field is set in exactly one place, `installCeremonyResult`, which
	// runs when a hop's RESULT arrives. The convene route commits the record into the open document
	// and never sets it, so on the convener's machine — the only machine this route runs on — it is
	// empty for the whole of hop 1. Measured: the control case in `ceremonyhop_test.go` failed with
	// *"the open document is not the one this ceremony is running over"* against a document that
	// was exactly the right one.
	//
	// The bytes cannot be stale in that way: `ceremony.Extract` reads the record embedded in the
	// document, and `ReadMirror` below re-reads and VERIFIES the machine's own copy. Two different
	// questions — "is this the right document open?" and "what does this ceremony say?" — and the
	// second is the one that must be signature-checked.
	inDoc, xerr := ceremony.Extract(doc.data)
	if xerr != nil {
		httpError(w, http.StatusConflict,
			"the open document is not part of a signing ceremony, so there is no hop to run")
		return rec, nil, next, false, false
	}
	if !strings.EqualFold(inDoc.ID, id) {
		// **409 and not 400**: the request is well formed and the document is open; what is wrong
		// is which document it is. ADR-004's own rule for a document the server no longer holds.
		httpError(w, http.StatusConflict,
			"the open document is not the one this ceremony is running over — open that document "+
				"first, then advance it")
		return rec, nil, next, false, false
	}
	v := vaultFrom(r)
	rec, _, err := ceremony.ReadMirror(defaultOutputDir(), id, time.Now())
	if err != nil {
		httpError(w, http.StatusConflict,
			"this ceremony could not be read on this machine: "+err.Error())
		return rec, nil, next, false, false
	}
	// The entitlement and deadline rules, through the one door, with the HOP budget — a hop must
	// fit a whole exchange before the deadline. See mintInvitationFor.
	if merr := checkMintAllowed(v, rec, time.Now(), ceremonyHopBudget()); merr != nil {
		switch {
		case errors.Is(merr, errNotTheConvener):
			httpError(w, http.StatusForbidden,
				"only the convener can call the next party: this machine is a party to this "+
					"ceremony, not the one that convened it")
		default:
			httpError(w, http.StatusConflict, merr.Error())
		}
		return rec, nil, next, false, false
	}
	// **Whose turn, from the SAME walk `/api/ceremony/next` answers with.** `ContributionProgress`
	// is the one implementation (ADR-009); a second derivation here is how two surfaces come to
	// disagree about which party is being called.
	roster := l3RosterFrom(rec.Roster, rosterHashHex(rec), rec.Intent)
	pr, perr := p2p.ContributionProgress(doc.data, roster)
	if perr != nil {
		httpError(w, http.StatusConflict,
			"this ceremony's document could not be read against its roster: "+perr.Error())
		return rec, nil, next, false, false
	}
	if pr.Complete {
		httpError(w, http.StatusConflict,
			"every signing party has already signed this document, so there is nobody left to call")
		return rec, nil, next, false, false
	}
	fp := pr.Order[pr.Done].Fingerprint
	for _, p := range rec.Roster {
		if strings.EqualFold(p.Fingerprint, fp) {
			next = p
			break
		}
	}
	if next.Fingerprint == "" {
		httpError(w, http.StatusInternalServerError,
			"this ceremony's signing order names a party its roster does not")
		return rec, nil, next, false, false
	}
	cert, _, ierr := identity(v)
	if ierr != nil {
		httpError(w, http.StatusInternalServerError, "could not read this machine's identity")
		return rec, nil, next, false, false
	}
	myFP, ferr := sign.Fingerprint(cert)
	if ferr != nil {
		httpError(w, http.StatusInternalServerError, "could not read this machine's fingerprint")
		return rec, nil, next, false, false
	}
	mine = strings.EqualFold(hex.EncodeToString(myFP), next.Fingerprint)
	return rec, doc.data, next, mine, true
}

// rosterLabel is the human name for a party, falling back to a short fingerprint.
//
// **A person is named, never fingerprinted** — the panel's standing rule, and the fallback exists
// because `Label` is optional on the wire while every surface that shows a party needs SOMETHING.
func rosterLabel(p ceremony.Party) string {
	if l := strings.TrimSpace(p.Label); l != "" {
		return l
	}
	if len(p.Fingerprint) > 12 {
		return p.Fingerprint[:12] + "…"
	}
	return p.Fingerprint
}

// handleCeremonyHopQuote answers whose turn it is and what block to draw for it.
//
// **GET, and read-only.** It resolves the turn, it does not start anything, and it is safe to call
// again — a convener who closes the modal and comes back gets the same answer. The POST re-resolves
// rather than trusting what this returned.
func (s *Server) handleCeremonyHopQuote(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("ceremony")
	rec, pdf, next, mine, ok := s.hopTarget(w, r, id)
	if !ok {
		return
	}
	out := ceremonyHopQuote{Ceremony: id, Party: rosterLabel(next), Mine: mine}
	// **`Contributes` is answered even when the turn is this machine's own, and the first cut left
	// it FALSE there.** Measured at tier 6: the quote came back `{"party":"Alice","mine":true,
	// "contributes":false}` for a signing convener whose own turn it was — which is not a
	// short-circuit, it is a wrong answer. `mine` and `contributes` are different questions ("is the
	// next signer me?" and "do I sign at this hop?") and they are BOTH true for a signing convener
	// at hop 1. Publishing a false one because the code returned early is the shape `Stored.Ended`'s
	// own doc forbids: no surface may render an absence as an answer.
	out.Contributes = mine && next.Signs
	if mine {
		// **The convener's own turn has nobody to dial**, and saying so is the honest answer rather
		// than an error: a signing convener is FIRST in the signing order, so this is the ordinary
		// state of a ceremony that has just been convened. The rail renders it as "sign this
		// yourself", which is a different action and not this route's.
		writeJSON(w, out)
		return
	}
	v := vaultFrom(r)
	cert, _, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read this machine's identity")
		return
	}
	myFP, ferr := sign.Fingerprint(cert)
	if ferr != nil {
		httpError(w, http.StatusInternalServerError, "could not read this machine's fingerprint")
		return
	}
	roster := l3RosterFrom(rec.Roster, rosterHashHex(rec), rec.Intent)
	pr, perr := p2p.ContributionProgress(pdf, roster)
	if perr != nil {
		httpError(w, http.StatusConflict, perr.Error())
		return
	}
	// **Does THIS machine sign at this hop?** The same question `carries` answers inside the dial,
	// asked here only to tell the client whether to render a block at all. The dial asks again and
	// its answer is the one that governs — this is a rendering hint, never an authorisation.
	me := hex.EncodeToString(myFP)
	for i, e := range pr.Order {
		if strings.EqualFold(e.Fingerprint, me) {
			out.Contributes = i == pr.Done
			break
		}
	}
	if !out.Contributes {
		writeJSON(w, out)
		return
	}
	// **The time is PINNED here and echoed on the POST**, which is the contract
	// `/api/cosign/quote` already has: without it the block a user consented to and the block
	// signed differ by however long they spent reading it.
	when := time.Now().UTC()
	att, aok := s.cosignAttestation(w, v,
		cosignParams{Fingerprint: next.Fingerprint, Intent: rec.Intent,
			When: when.Format(time.RFC3339)}, roster)
	if !aok {
		return
	}
	out.Lines, out.Rect, out.When = att.AppearanceLines(), p2p.NominalBlockRect(), att.When.UTC().Format(time.RFC3339)
	writeJSON(w, out)
}

// handleCeremonyHop dials the party whose turn it is and runs the hop.
//
// **This is the action the whole wizard was missing.** Everything about which party, which
// invitation and which document is decided HERE; the client sends a ceremony id and, when this
// machine signs, the block it drew from the quote above.
func (s *Server) handleCeremonyHop(w http.ResponseWriter, r *http.Request) {
	var req ceremonyHopRequest
	if err := readJSON(r, &req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rec, pdf, next, mine, ok := s.hopTarget(w, r, req.Ceremony)
	if !ok {
		return
	}
	if mine {
		// **A convener whose own turn it is has nobody to dial**, and a dial to yourself is not a
		// hop. Refused rather than silently no-oped, because the rail offering this at all would be
		// the defect P01.S02's block was set for: an action that does not exist behind a label.
		httpError(w, http.StatusConflict,
			"it is this machine's own turn to sign, so there is nobody to call — sign it here first")
		return
	}
	// **One dial in flight per ceremony.** A hop costs a fresh socket, a DHT bootstrap and a race
	// bounded by `connectDeadline`, all of it off-link under ADR-011; two presses would fan out two
	// of everything against the same party. `s.legs` is a REPORTING map, not a lock, so this is its
	// own guard — keyed the same way, per (ceremony, party), because two different parties of one
	// ceremony are not in contention.
	release, held := s.holdHop(req.Ceremony, next.Fingerprint)
	if !held {
		httpError(w, http.StatusConflict,
			"Nib is already calling "+rosterLabel(next)+" for this ceremony — wait for that to "+
				"finish or fail before starting another")
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
	peerFP, derr := parseFingerprint(next.Fingerprint)
	if derr != nil {
		httpError(w, http.StatusInternalServerError, "this ceremony's roster names a party Nib cannot read")
		return
	}
	// **Minted HERE rather than pasted, which is the whole slice.** Through the one door, with the
	// hop budget, so this route cannot become the third caller that skips a rule.
	inv, ierr := mintInvitationFor(v, rec, next, time.Now(), ceremonyHopBudget())
	if ierr != nil {
		switch {
		case errors.Is(ierr, errNotTheConvener):
			httpError(w, http.StatusForbidden, ierr.Error())
		case errors.Is(ierr, errNoCeremonySecret):
			httpError(w, http.StatusGone, ierr.Error())
		default:
			httpError(w, http.StatusConflict, ierr.Error())
		}
		return
	}
	text, eerr := inv.Encode()
	if eerr != nil {
		httpError(w, http.StatusInternalServerError, "could not build this party's invitation: "+eerr.Error())
		return
	}
	// **The same ceremony identity the pasted path builds**, from the same door — `dialerCeremony`
	// takes invitation TEXT, so minting and encoding here means the two routes converge on one
	// `*ceremonyID` and one hop derivation rather than two.
	cer, cerr := s.dialerCeremony(text, cert, key, peerFP)
	if cerr != nil {
		httpError(w, http.StatusInternalServerError, "could not open this ceremony's rendezvous: "+cerr.Error())
		return
	}
	defer cer.close()

	when := req.When
	if when == "" {
		when = time.Now().UTC().Format(time.RFC3339)
	}
	att, aok := s.cosignAttestation(w, v,
		cosignParams{Fingerprint: next.Fingerprint, Intent: rec.Intent, When: when},
		cer.l3Roster())
	if !aok {
		return
	}
	appearance, derr2 := decodeAppearance(req.Appearance)
	if derr2 != nil {
		httpError(w, http.StatusBadRequest, "the signature block could not be read: "+derr2.Error())
		return
	}
	if err := checkCeremonyDeadline(pdf, time.Now()); err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	s.runHopDial(w, r, v, cer, cert, key, myFP, peerFP, pdf, appearance, att, req.Address, rosterLabel(next))
}

// holdHop takes the single in-flight slot for one (ceremony, party), or reports it taken.
//
// **A check-and-take under ONE lock, not a check then a take.** The two-step version is the defect
// `/pending 387` closed one file over — `enter`'s `live.Err()` / `inFlight.Add(1)` — and it is the
// same shape here: two presses landing together would both see the slot free and both dial.
//
// **Keyed per (ceremony, party) and not per ceremony**, matching `legKey`'s own correction: keying
// on the ceremony alone is unique only because the walk happens to be serial, which is a property
// of today's caller rather than of the key.
func (s *Server) holdHop(id, party string) (func(), bool) {
	k := legKey{ceremony: id, party: strings.ToLower(party)}
	s.legMu.Lock()
	defer s.legMu.Unlock()
	if s.hops == nil {
		s.hops = map[legKey]bool{}
	}
	if s.hops[k] {
		return func() {}, false
	}
	s.hops[k] = true
	return func() {
		s.legMu.Lock()
		delete(s.hops, k)
		s.legMu.Unlock()
	}, true
}

// decodeAppearance reads the client's rendered block, which is base64 PNG or absent.
//
// **Absent is legitimate and is the carry path**, where this machine contributes nothing and there
// is no block to draw. `runHopDial` refuses an EMPTY appearance on the contributing path, so the
// two cases stay distinguishable here rather than being collapsed into "maybe fine".
func decodeAppearance(b64 string) ([]byte, error) {
	if strings.TrimSpace(b64) == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, err
	}
	return raw, nil
}
