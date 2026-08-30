package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/vault"
)

// Co-signing (Track A): two endpoints implement one user action. The visible
// attestation block must be rendered to an image in the browser (there is no
// server-side text renderer), yet its text and placement have to come from Go so
// the on-page block can't drift from the signed /Reason — **which is false inside a ceremony
// and was never true there (/pending 317, corrected 2026-08-30): this function does not stamp,
// and the signing path does.** So /quote hands the
// client the exact lines and rectangle to render, and /sign signs from the same
// inputs — the client never re-derives the attestation, only rasterizes it.

// maxIntentLen was a silent 200-rune clamp on the manual co-sign path (/pending 286). It is gone:
// the bound that matters is `p2p.IntentFitsBlock`, which measures the rendered WIDTH against the
// block's geometry rather than counting runes, and refuses. See buildAttestation.

// cosignParams are the attestation inputs shared by /quote and /sign.
type cosignParams struct {
	Fingerprint string `json:"fingerprint"` // the accepted peer; must already be pinned
	Intent      string `json:"intent"`
	When        string `json:"when"` // RFC3339; minted by /quote, echoed back to /sign
}

// cosignQuote is what /quote returns: the canonical attestation lines and the
// placement rectangle for the client to rasterize, plus the pinned signing time.
type cosignQuote struct {
	Lines []string   `json:"lines"`
	Rect  [4]float64 `json:"rect"` // llx, lly, urx, ury in PDF points (the client sizes the PNG to its aspect)
	// No `page`: one was published and never read. Both callers rasterize from `lines` and
	// `rect` and post the PNG back, and the server re-derives the placement when it signs —
	// so the page number was an echo the client had no use for. Deleted with
	// decryptResponse.ok (/pending 254).
	When string `json:"when"`
}

// cosignAttestation builds the attestation both calls sign over, from the same
// inputs, so the rendered block and the signed /Reason always agree. It refuses
// any peer the user hasn't pinned out-of-band (the honest-trust requirement) and
// caps the intent length. Writes the HTTP error itself and returns ok=false.
// maxWhenSkew bounds how far a client-supplied attestation time may sit from the
// server's own clock. See the use in cosignAttestation.
const maxWhenSkew = 24 * time.Hour

func (s *Server) cosignAttestation(w http.ResponseWriter, v *vault.Vault, p cosignParams) (p2p.Attestation, bool) {
	fp, err := parseFingerprint(p.Fingerprint)
	if err != nil {
		httpError(w, http.StatusBadRequest, "not a valid fingerprint")
		return p2p.Attestation{}, false
	}
	label, ok := pinnedLabel(v, fp)
	if !ok {
		httpError(w, http.StatusBadRequest, "that peer isn't pinned — pin their fingerprint first")
		return p2p.Attestation{}, false
	}
	// **REFUSED, not clamped, and through the door the ceremony path already uses (/pending 286).**
	//
	// This silently did `intent = string(rs[:maxIntentLen])` — no error, no echo — on a value that
	// is signed into the attestation's `/Reason` and rendered on the block. The repo's law is
	// refuse-not-clamp: `ErrReadmeOverflow` is the precedent, and P07.S08's finding that pdfcpu
	// CLAMPS overflow is what made its own instrument blind.
	//
	// **And 200 runes was the wrong bound anyway, because count is not width.** `IntentFitsBlock`
	// measures the rendered string against the block's real geometry — "MMMM" and "iiii" differ by
	// nearly 3x at these metrics — so a rune count is wrong for capitals and wasteful for lower
	// case. The convene door has refused on that measurement since P07.S02a; this is the MANUAL
	// co-sign path, which was never routed through it. One rule, one door (ADR-009).
	intent := p.Intent
	if !p2p.IntentFitsBlock(intent) {
		httpError(w, http.StatusBadRequest, fmt.Sprintf(
			"that intent is %d characters and about %d fit. The signature block carries it in "+
				"full, so Nib refuses an intent it would have to cut rather than showing a "+
				"shortened one above your signature.",
			len([]rune(intent)), p2p.MaxIntentRunes(intent)))
		return p2p.Attestation{}, false
	}
	// The client may name the time, but not an arbitrary one: `when` is signed into the
	// attestation, so an unbounded value lets a caller mint a co-signature dated years
	// back or forward and have Nib's own key vouch for it. Bounded to a day either side
	// of now — enough for clock skew and a slow consent, not enough to backdate.
	when := time.Now()
	if p.When != "" {
		if t, err := time.Parse(time.RFC3339, p.When); err == nil {
			if d := t.Sub(when); d > -maxWhenSkew && d < maxWhenSkew {
				when = t
			}
		}
	}
	return p2p.Attestation{
		Signer:            "Nib User",
		AcceptedPeer:      hex.EncodeToString(fp),
		AcceptedPeerLabel: label,
		Intent:            intent,
		When:              when,
	}, true
}

// pinnedLabel returns the label the given fingerprint is pinned under, and whether
// it is pinned at all.
func pinnedLabel(v *vault.Vault, fp []byte) (string, bool) {
	for _, p := range v.PinnedPeers() {
		if bytes.Equal(p.Fingerprint, fp) {
			return p.Label, true
		}
	}
	return "", false
}

// attestationView is one signer's attestation for the verify-side display: the
// p2p reading plus whether the viewer has pinned that signer's identity locally.
type attestationView struct {
	p2p.SignerAttestation
	Pinned bool `json:"pinned"`
}

type attestationsResponse struct {
	Attestations []attestationView `json:"attestations"`
	// Obliged is how many parties this document's ceremony record obliges to sign, and Signed how
	// many of them have a valid signature on it (C16/C18, P07.S05a).
	//
	// **Zero obliged means "no ceremony record was readable here" and the client must say
	// nothing** — an ordinary two-party co-sign has no roster, and reporting "0 of 0 signed"
	// about one would be a verdict on a proceeding that does not exist. It is the same
	// three-state discipline the proceeding line already uses.
	//
	// Together they are C18: a nine-party ceremony abandoned at hop 5 renders *untampered, 5
	// signers, every attestation matched, one proceeding* without them, and no surface says four
	// obliged parties never signed. And they are C16: a `signs:false` convener is not obliged, so
	// a ceremony they carried to completion reads complete rather than short a signer.
	Obliged int `json:"obliged,omitempty"`
	Signed  int `json:"signed,omitempty"`
}

// handleAttestations returns the co-signing attestations on the open document —
// each signer's accepted peer, whether it cross-binds to a real co-signer
// (Matched), and whether the viewer has pinned that signer. Read-only; the
// signature-details modal fetches it lazily.
func (s *Server) handleAttestations(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	// Not resolveDoc: this route's nil answer is an empty list rather than a 404,
	// because "no document open" is not an error for a panel that polls. A named
	// document we do not hold still is one.
	doc, err := s.docFor(r)
	if err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	if doc == nil {
		writeJSON(w, attestationsResponse{Attestations: []attestationView{}})
		return
	}
	// **The cached status, not a re-verify (P07.S04, clause 6).** `document.sig` is computed
	// wherever the bytes are installed; this route was calling `ReadAttestations`, which verifies
	// the whole file again — signature-count × document-SIZE work, on a request path, with the
	// answer already sitting beside the bytes. Measured at the slice's grill: the cost is
	// dominated by size rather than by signature count, because each signature's byte range is
	// hashed over the whole document.
	//
	// The proceeding lookup is CONDITIONAL, because it costs a pdfcpu attachment parse and this
	// is request-handling code. A document whose signatures name no ceremony has no proceeding to
	// be checked against, so the question is not asked — the same discriminator the client uses
	// before it says anything about proceedings at all.
	// **The proceeding lookup is UNCONDITIONAL, and every cheaper gate was measured and refused
	// (P07.S05a).**
	//
	// It was `ClaimsAProceeding` — does any signature name a ceremony. That is the right question
	// for the agreement verdict and the wrong one for completeness: a **convened but unsigned**
	// document has no signatures at all and is exactly the case C18 is about, 0 of N obliged
	// signers. Measured at tier 6, the route answered `{"attestations":[]}` with no counts for
	// precisely that document.
	//
	// Two cheaper gates were built and both are unsound:
	//
	//   - **A byte scan for the attachment's name is a FALSE NEGATIVE on a real record.** Measured:
	//     `Extract` succeeds while `bytes.Contains(pdf, "nib-ceremony.json")` is false, because
	//     pdfcpu puts the file-spec name in a compressed object stream. It is not even a necessary
	//     condition, so it cannot gate anything.
	//   - **Caching the proceeding beside `doc.sig` is unsound twice over.** Fourteen sites assign
	//     `sig`, so there is no one door to hang it on (ADR-009 would have to be built first), and
	//     `ProceedingOf` takes `now` because the record expires — a cached proceeding answers a
	//     question about a moment that has passed.
	//
	// So it is one pdfcpu read per request on this route, and the hot-path rule is satisfied by
	// what the route no longer does rather than by a gate. Before S04 it called `ReadAttestations`,
	// which re-verifies every signature over the whole file: size × signers. This is one read of
	// the file, independent of signature count, and the route is opened by a user clicking the
	// signature-details button — not a per-frame path. Measured at ~0.5 ms on a 3 KB document.
	proc := ceremony.ProceedingOf(s.docBytes(doc), time.Now())
	atts := p2p.Attestations(doc.sig, proc)
	views := make([]attestationView, 0, len(atts))
	for _, a := range atts {
		view := attestationView{SignerAttestation: a}
		if fp, err := hex.DecodeString(a.Fingerprint); err == nil && len(fp) == 32 {
			_, view.Pinned = pinnedLabel(v, fp)
		}
		views = append(views, view)
	}
	signed, obliged := p2p.Completeness(atts, proc)
	writeJSON(w, attestationsResponse{Attestations: views, Signed: signed, Obliged: obliged})
}

func (s *Server) handleCosignQuote(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var p cosignParams
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&p); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if p.When == "" {
		p.When = time.Now().UTC().Format(time.RFC3339)
	}
	att, ok := s.cosignAttestation(w, v, p)
	if !ok {
		return
	}
	// Not resolveDoc: this route answers 400 rather than 404 for a missing
	// document, because quoting requires one and asking without one is a client
	// bug. Preserved as-is; only the 409 arm is new.
	doc, err := s.docFor(r)
	if err != nil {
		httpError(w, http.StatusConflict, "that document is no longer open")
		return
	}
	if doc == nil {
		httpError(w, http.StatusBadRequest, "no document open")
		return
	}
	// **The size template, not a placement — and this is the SECOND copy of that rule
	// (P07.S06, ADR-009).** `NominalBlockRect` was written because "the rule had TWO
	// implementations"; it fixed the hand-copied literal in this package and left this site,
	// which computed a real placement — a full PageCount plus a sign.Verify over the open
	// document — to publish a rect whose POSITION nothing reads. `web/app.js:956` takes
	// `rect[2]-rect[0]` and `rect[3]-rect[1]` and nothing else, and the comment that used to
	// sit here said exactly that while doing the opposite.
	//
	// The divergence was invisible precisely because the discarded half is the half that
	// differs. It stops being invisible the moment placement needs a roster: the responder's
	// block goes on the RECEIVED document, so this route has no roster and must not have one —
	// binding to the open document would use the wrong page geometry, which is the reason
	// `NominalBlockRect` records for existing at all. `/sign` computes the authoritative
	// placement on the document that will actually carry it.
	writeJSON(w, cosignQuote{
		Lines: att.AppearanceLines(),
		Rect:  p2p.NominalBlockRect(),
		When:  att.When.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleCosignSign(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	cleanup, ok := parseMultipart(w, r, maxPDFBytes)
	if !ok {
		return
	}
	defer cleanup()
	pdfBytes, ok := formFileBytes(w, r, "pdf")
	if !ok {
		return
	}
	var p cosignParams
	if raw := r.FormValue("params"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			httpError(w, http.StatusBadRequest, "invalid params")
			return
		}
	}
	att, ok := s.cosignAttestation(w, v, p)
	if !ok {
		return
	}
	appearance, ok := formFileBytes(w, r, "appearance")
	if !ok {
		return
	}
	cert, key, err := identity(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	// The manual co-sign route has no ceremony, so the zero Roster: there is no signing
	// order for a two-party exchange to be out of.
	signed, ok := s.buildCoSigned(w, pdfBytes, cert, key, att, appearance, p2p.Roster{})
	if !ok {
		return
	}
	sendDownload(w, "co-signed.pdf", "application/pdf", signed)
}

// buildCoSigned prepares the document if needed and contributes this user's
// signature, returning the co-signed bytes. The first signer appends the readme
// (PrepareDocument) before signing; a later signer's document is already prepared
// and signed, so it is co-signed as-is by an incremental update. Shared by the
// Track A download path (/api/cosign/sign) and the live dial path
// (/api/session/initiate). Writes the HTTP error itself and returns ok=false.
// roster is the ceremony's signing order, or the zero Roster outside a ceremony (P07.S03).
func (s *Server) buildCoSigned(w http.ResponseWriter, pdf, cert, key []byte, att p2p.Attestation, appearance []byte, roster p2p.Roster) ([]byte, bool) {
	// **L3 (D23), before a single byte is signed.** This is the INITIATING party's contribution
	// entry point — the second of the two the rule has to hold at, and the one in this package.
	//
	// Before `PrepareDocument` and before `Contribute`, on the same reasoning the ceremony
	// deadline check states one caller up: refusing after the local signature is applied leaves
	// the user signed into a position the ceremony says is not theirs, and a signature cannot be
	// taken back off a document.
	//
	// 409 rather than 400: this is a refusal about the STATE of a proceeding, not about a
	// malformed request, and the user's action is to wait rather than to correct a field.
	// Hoisted out of the ceremony branch below (P07.S06): the placement needs it too, and
	// computing it twice would be a second answer to "who am I" in one function.
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read your own fingerprint")
		return nil, false
	}
	if len(roster.Entries) > 0 {
		if err := p2p.AdmitContribution(pdf, roster, hex.EncodeToString(myFP)); err != nil {
			httpError(w, http.StatusConflict, err.Error())
			return nil, false
		}
		// **What this signature ACCEPTS comes off the roster too (P07.S05, D22 amended).**
		//
		// The caller built `att` before the ceremony was resolved, from the peer it is dialling —
		// which is the wire peer, and under a carry route the wire peer is a non-signing convener.
		// A first signer would then attest to somebody who never signs, and the two contribution
		// doors would disagree about what `AcceptedPeer` MEANS: `coSignExchange` already reads it
		// off the roster. Overridden here rather than at the caller because this is the same door
		// the gate above hangs on, and a rule at one of two doors is the ADR-009 shape.
		//
		// The first signer accepts "" — there is nobody before them, and that is C14 as amended.
		att.AcceptedPeer = p2p.PredecessorOf(roster, hex.EncodeToString(myFP))
		// And it NAMES the ceremony — and, since P07.S07a, this party: their label, their
		// capacity and their position in the signing order, through the one door the receiving
		// side also uses. `att.Signer` arrives here as the `"Nib User"` constant
		// `cosignAttestation` sets for the manual co-sign; inside a ceremony the roster overrides
		// it, which is why the constant is left where it is rather than deleted — a document
		// co-signed outside a ceremony genuinely has no roster to be named by.
		p2p.StampCommitment(&att, roster, hex.EncodeToString(myFP))
	}
	prepared := pdf
	if sign.Verify(pdf).State == sign.Unsigned {
		p, err := p2p.PrepareDocument(pdf)
		if err != nil {
			// 400 only for the one failure that IS the caller's document — an
			// already-signed file, which PrepareDocument refuses by name. Everything
			// else here is Nib's own: a bad render spec, or ErrReadmeOverflow, which
			// says the trust page's own boilerplate no longer fits its own page. A 400
			// on those tells a small-practice user their document is bad and sends them
			// looking at the wrong thing.
			if errors.Is(err, p2p.ErrReadmeOverflow) {
				httpError(w, http.StatusInternalServerError,
					"Nib could not build the trust-explainer page that goes on a co-signed "+
						"document, so nothing was signed. This is a fault in Nib, not in your file.")
				return nil, false
			}
			httpError(w, http.StatusBadRequest, "could not prepare document: "+err.Error())
			return nil, false
		}
		prepared = p
	}
	// One door, branching on the roster (P07.S06). Inside a ceremony this party's block goes on
	// the signature page their ROSTER POSITION allocates — not on the last page indexed by a count
	// of signatures, which put every block of a nine-party ceremony on the second signature page
	// and the last of them 50 pt off it. `myFP` is already computed above for AdmitContribution.
	place, err := p2p.PlacementFor(prepared, roster, hex.EncodeToString(myFP))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not place attestation: "+err.Error())
		return nil, false
	}
	signed, err := p2p.Contribute(prepared, cert, key, att, appearance, place)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	return signed, true
}
