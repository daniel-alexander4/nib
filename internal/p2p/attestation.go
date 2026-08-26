// Package p2p assembles the serverless two-party co-signing artifact: a single
// PDF carrying both parties' approval signatures, each binding a visible +
// machine-readable acceptance attestation (who the signer accepts the
// counterparty to be, and that they agree to sign). The file is exchanged
// out-of-band; there is no live transport here (that is the deferred Track B).
//
// Each party's contribution is a single visible approval signature: the visible
// attestation block is the signature's own appearance (one incremental revision,
// always covered and always rendered), and the machine-readable binding lives in
// the signature's signed /Reason field. After one clean pre-signing pass that
// appends the readme, every later revision is a pure signing increment — no
// interleaved structural writers — which is what keeps the artifact consistent
// and renderable across viewers.
package p2p

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nib/internal/sign"

	"nib/mdpdf"
)

// defaultIntent is the agreement an attestation records when the caller gives none.
const defaultIntent = "I agree to sign this document."

// Attestation is one party's identity+intent confirmation. The signer's own
// identity comes from the signing certificate; this records what is not already
// in the signature: which counterparty the signer accepts (by SHA-256 SPKI
// fingerprint, see sign.Fingerprint) and the intent.
//
// It is the source of BOTH the visible appearance block (AppearanceLines) and the signed
// /Reason (reason). It is not, however, ENFORCED to be: Contribute takes an already
// rasterised appearance image from its caller and signs it alongside a /Reason built here,
// and no code compares the two — a bitmap cannot be checked against a string. This comment
// used to assert that the two "cannot disagree", which was a promise nothing kept. What is
// true: the client renders the appearance from AppearanceLines, so they agree by
// construction on the path Nib actually uses, and a caller that supplied something else
// would produce a document whose picture and whose signed text differ.
type Attestation struct {
	Signer            string    // signer's identity common name (-> signature Name)
	AcceptedPeer      string    // hex SHA-256 SPKI of the accepted counterparty (-> /Reason)
	AcceptedPeerLabel string    // human label the signer pinned for the peer (-> /Reason, appearance)
	Intent            string    // what the signer agrees to (-> /Reason, appearance)
	When              time.Time // signing time
	// RosterHash is the Ceremony Record's commitment, hex, carried as a
	// [NibRoster:<hash>] token (D2's UX pin, D20). Empty on an ordinary two-party
	// co-sign, which has no record.
	//
	// It is what makes N signatures a record of ONE proceeding rather than a chain of
	// pairwise claims: signer 3 attesting only to signer 2 says nothing about what signer
	// 1 agreed to, and every signature carrying the same commitment does.
	RosterHash string
	// RosterVersion is the record FORMAT version the commitment was computed under, and it
	// travels beside it (P07.S04).
	//
	// **Without it a format skew is indistinguishable from tampering, through the one surface
	// D32 excused.** `FormatVersion` is the first substantive axis of `rosterPreimage`
	// (`ceremony/record.go`), so two builds at different versions digest the IDENTICAL roster to
	// different hashes. The commitments then disagree, and the client's honest reading of that
	// is *"This document was not produced by a single agreed proceeding"* — an accusation about
	// the parties, caused by one of them having updated Nib. Carrying the version lets a reader
	// say which it is.
	//
	// Zero is refused by `reason()` when a hash is present: an unversioned commitment is one
	// nothing can interpret, and there is no population to be lenient towards — no production
	// attestation has ever carried a commitment at all.
	RosterVersion int
}

// attestationTag marks a /Reason as one this package WROTE, and ReadAttestations requires
// it before reading the peer token.
//
// **What it fixes.** Without it, any signature whose reason merely contained a bracketed
// 64-hex token parsed as a co-signing attestation — and an ordinary Finalize signature's
// reason is free text the signer types. A reason carrying a crafted [SPKI:…] therefore
// reached crossBind and could be reported as accepting a co-signer.
//
// **What it does NOT prove, and this matters more than what it does.** The reason is signed
// by the signer's own key, so a signer who wants to can type this tag as easily as anything
// else. It cannot distinguish "Nib's co-sign flow produced this" from "the signer wrote
// it": both are the same key asserting the same string. What it removes is the INCIDENTAL
// case — a reason that collides by accident or by copy-paste — and it makes the format
// self-describing. The verdict built on it is worded accordingly, and correctly: the UI
// says each signature ATTESTS to the other's key, which is a claim about what was asserted
// rather than about which program asserted it.
const attestationTag = "[NibCoSign:1]"

// spkiToken matches the machine-readable peer fingerprint embedded in /Reason.
var spkiToken = regexp.MustCompile(`\[SPKI:([0-9a-fA-F]{64})\]`)

// rosterToken matches the Ceremony Record commitment embedded in /Reason (D20), with the record
// FORMAT version it was computed under (P07.S04).
//
// **Only the versioned form is a token.** There is no unversioned population to be lenient
// towards — measured: no production attestation has ever carried a commitment — and accepting
// both would leave a reader unable to say whether a missing version means "old build" or "the
// field was stripped", which is the ambiguity the version exists to remove. The same argument
// `InvitationVersion` made at P07.S02: a required field is a format change, and a format with no
// population in the field is free to make.
var rosterToken = regexp.MustCompile(`\[NibRoster:([0-9]{1,4}):([0-9a-fA-F]{64})\]`)

func (a Attestation) intent() string {
	if a.Intent == "" {
		return defaultIntent
	}
	return a.Intent
}

// safeText strips the square brackets out of user-supplied text. The /Reason
// carries the accepted peer as a [SPKI:<hex>] token that ReadAttestations parses
// with the FIRST match; the label is interpolated before that token, so without
// this a crafted label (or intent) could inject a second [SPKI:...] that wins the
// parse and misrepresents which peer was accepted. Removing brackets makes the
// real token the only one that can appear.
// safeHex is safeText's counterpart for the two fields that must be bare hex.
//
// reason() applied safeText to the label and the intent and interpolated AcceptedPeer and
// RosterHash RAW, while safeText's own doc claims "Removing brackets makes the real token
// the only one that can appear". Both parsers use FindStringSubmatch (first match), so a
// crafted AcceptedPeer of the form "<64hex>] [NibRoster:<evil64hex>" places an earlier
// roster token and wins the parse at readRoster. Not reachable on the real path — the only
// non-test producer is hex.EncodeToString — but these are plain strings on an exported
// struct with no charset check at the encoding site, so the claim was wider than what
// enforced it. Empty rather than escaped: a non-hex fingerprint is not a fingerprint.
func safeHex(s string) string {
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return ""
		}
	}
	return s
}

func safeText(s string) string {
	return strings.NewReplacer("[", "", "]", "").Replace(s)
}

// reason encodes the attestation into a signature /Reason: a human sentence with
// a machine-parseable [SPKI:<hex>] token, so it reads cleanly in any viewer's
// signature panel yet round-trips exactly. The user-controlled label and intent
// are bracket-stripped so they can't forge the token (see safeText).
func (a Attestation) reason() string {
	roster := ""
	// **Both, or neither.** A commitment with no format version is one nothing can interpret:
	// `FormatVersion` is the first substantive axis of the roster preimage, so a reader handed a
	// bare hash cannot tell a different ceremony from a different record format. Emitting no
	// token is the fail-CLOSED direction — `markOneProceeding` treats a missing commitment as
	// disqualifying, so the signature reads as "not part of this proceeding" rather than as a
	// commitment somebody might compare. `TestARosterHashWithoutAVersionCarriesNoToken` drives it.
	if a.RosterHash != "" && a.RosterVersion > 0 {
		// Placed BEFORE the user-controlled text, like the SPKI token, and matched by a
		// regexp requiring exactly 64 hex — safeText strips brackets from the label and
		// intent, so neither can forge one.
		roster = " [NibRoster:" + strconv.Itoa(a.RosterVersion) + ":" + safeHex(a.RosterHash) + "]"
	}
	return fmt.Sprintf("%s Accepts %s [SPKI:%s]%s. %s", attestationTag,
		safeText(a.AcceptedPeerLabel), safeHex(a.AcceptedPeer), roster, safeText(a.intent()))
}

// AppearanceLines is the visible attestation block text, one entry per line — the
// canonical content a client renders into the signature's appearance image. The
// full fingerprint lives in the signed /Reason; here it is shortened for the eye.
func (a Attestation) AppearanceLines() []string {
	return []string{
		"Nib co-signing attestation",
		fmt.Sprintf("Signer: %s", a.Signer),
		fmt.Sprintf("Accepts: %s  [%s]", a.AcceptedPeerLabel, shortFingerprint(a.AcceptedPeer)),
		fmt.Sprintf("Intent: %s", a.intent()),
		fmt.Sprintf("Time: %s", a.When.UTC().Format("2006-01-02 15:04 MST")),
	}
}

// SignerAttestation is one verified signer's attestation, read back from the
// co-signed artifact.
type SignerAttestation struct {
	Signer       string `json:"signer"`       // signature common name
	Fingerprint  string `json:"fingerprint"`  // hex SPKI of the signer's own cert (the identity that signed)
	AcceptedPeer string `json:"acceptedPeer"` // hex SPKI parsed from /Reason ("" if absent)
	Reason       string `json:"reason"`       // raw signed /Reason
	When         string `json:"when"`         // signing time (display)
	Valid        bool   `json:"valid"`        // this signature verifies
	// Matched is true when the accepted peer is actually one of the other signers
	// on this document AND both signatures verify — i.e. this signer's valid
	// signature attests to a real, valid co-signer's key, not a claim about someone
	// absent or a tampered signature. For two-party mutual co-signing both are
	// Matched. A broken signature attests to nothing, so it never Matches.
	Matched bool `json:"matched"`
	// RosterHash is the Ceremony Record commitment this signature carries ("" on an
	// ordinary two-party co-sign, which has no record).
	RosterHash string `json:"rosterHash,omitempty"`
	// RosterVersion is the record format version that commitment was computed under, 0 when the
	// signature carries none. Published so a reader can tell a FORMAT SKEW from a disagreement:
	// two builds at different versions digest the same roster to different hashes, and calling
	// that "not one proceeding" accuses the parties of something an update caused (D32).
	RosterVersion int `json:"rosterVersion,omitempty"`
	// OneProceeding is true when every VALID signature on the document carries the same
	// non-empty roster commitment.
	//
	// False is not "unsigned" and not "invalid" — it is a document whose signers did not
	// all agree to the same ceremony, which is a distinct thing to report and the reason
	// this is a separate field rather than folded into Matched. A verifier that said only
	// "co-signed" about such a document would be describing a proceeding that did not
	// happen.
	OneProceeding bool `json:"oneProceeding,omitempty"`
}

// ReadAttestations returns each signer's attestation from a co-signed PDF, in
// signature order. It reads the verified signers (so an attestation is only as
// trustworthy as its signature), parses the accepted-peer fingerprint from the
// signed /Reason, and cross-binds each accepted peer against the other signers'
// actual fingerprints (Matched).
func ReadAttestations(pdf []byte) []SignerAttestation {
	return Attestations(sign.Verify(pdf), Proceeding{})
}

// Proceeding is what the caller knows about the ceremony a document is supposed to belong to.
//
// **Primitives, not a `ceremony.Record`, for the reason L3's `Roster` is** — `p2p` cannot import
// `internal/ceremony`: since P07.S02a that is a production import cycle, not a test one. The
// caller holds the record and passes what this package needs to check against.
type Proceeding struct {
	// Commitment is the RosterHash of the record the DOCUMENT carries, hex, or "" when the
	// caller has none to offer.
	//
	// **Empty means `OneProceeding` can never be true, and that is the fix rather than a
	// limitation.** `markOneProceeding` used to compare the signatures' commitments only to each
	// other, so a document with no ceremony record at all — whose signers had both written the
	// same 64-hex value they chose themselves — reported one proceeding on every signature, and
	// the client rendered *"✓ One proceeding — every signature on this document commits to the
	// same ceremony."* Measured at P07.S04's grill. The token lives inside the signed `/Reason`,
	// so it is a value the signer picks; agreement among signers is not evidence that the thing
	// they agree about exists.
	Commitment string
}

// ClaimsAProceeding reports whether any signature on an already-verified document names a
// ceremony at all.
//
// **A cheap discriminator so the expensive lookup is conditional (P07.S04).** Answering "which
// proceeding is this document's" costs an attachment extraction — a pdfcpu parse — and the
// attestations route is request-handling code, where `CLAUDE.md` says work goes only if it is
// unavoidable. It is avoidable here: a document whose signatures name no ceremony has no
// proceeding to check them against, and that is the overwhelming majority of documents. This
// reads the already-verified `/Reason` strings and parses nothing.
//
// It is NEARLY the discriminator the client uses to decide whether to say anything about
// proceedings (`web/app.js`: `attested.filter((a) => a.rosterHash)`) — nearly, because this one
// also requires the signature to be valid, and the client's does not. The difference is safe in
// one direction only and that is why it is written down: a document whose ONLY ceremony token is
// on an invalid signature skips the lookup here, so no commitment is supplied, so nothing reports
// one proceeding — and the client then shows its warning, which is the honest answer for a
// tampered signature naming a ceremony. The reverse would not be safe, and nothing produces it.
func ClaimsAProceeding(st sign.Status) bool {
	for _, sg := range st.Signers {
		if !sg.Valid || !strings.Contains(sg.Reason, attestationTag) {
			continue
		}
		if rosterToken.MatchString(sg.Reason) {
			return true
		}
	}
	return false
}

// Attestations reads each signer's attestation from an ALREADY VERIFIED document.
//
// **Split out at P07.S04 for two reasons that turn out to be one seam.** A caller that already
// holds a `sign.Status` — every HTTP handler with an open document does, in `document.sig` — was
// re-verifying the whole file to get here, which is signature-count × document-size work on a
// request path with the answer already cached beside it. And the proceeding check needs somewhere
// to take the document's own commitment from, which a `(pdf []byte)` signature has nowhere to put.
func Attestations(st sign.Status, p Proceeding) []SignerAttestation {
	out := make([]SignerAttestation, 0, len(st.Signers))
	for _, s := range st.Signers {
		sa := SignerAttestation{Signer: s.Name, Fingerprint: s.Fingerprint, Reason: s.Reason, When: s.When, Valid: s.Valid}
		// The tag is required, not just the token — see attestationTag. safeText strips
		// brackets from the label and intent this package interpolates, so a co-signer
		// cannot smuggle either marker through the fields it controls.
		if strings.Contains(s.Reason, attestationTag) {
			if m := spkiToken.FindStringSubmatch(s.Reason); m != nil {
				sa.AcceptedPeer = m[1]
			}
			if m := rosterToken.FindStringSubmatch(s.Reason); m != nil {
				// **Version 0 is refused on the way IN as well as on the way out.** `reason()`
				// emits neither half without both; a signer writing `[NibRoster:0:…]` into their
				// own /Reason by hand would otherwise produce the pair the writer is forbidden
				// to produce, and a state reachable through one door and not the other is a
				// state nothing downstream was written against.
				if v, err := strconv.Atoi(m[1]); err == nil && v > 0 {
					sa.RosterVersion = v
					sa.RosterHash = m[2]
				}
			}
		}
		out = append(out, sa)
	}
	crossBind(out)
	markOneProceeding(out, p.Commitment)
	return out
}

// crossBind sets Matched on each attestation: an accepted peer counts only if some
// OTHER signer with a VALID signature actually holds that fingerprint on this
// document, and this signer's own signature is valid too — a tampered signature
// attests to nothing, so it must not produce a "mutually co-signed" verdict.
func crossBind(atts []SignerAttestation) {
	for i := range atts {
		if atts[i].AcceptedPeer == "" || !atts[i].Valid {
			continue
		}
		for j := range atts {
			if j != i && atts[j].Valid && atts[j].Fingerprint == atts[i].AcceptedPeer {
				atts[i].Matched = true
				break
			}
		}
	}
}

// shortFingerprint renders the leading bytes of a hex fingerprint in spaced quads
// for at-a-glance comparison, eliding the rest. The full value is the pin.
func shortFingerprint(hexFP string) string {
	if len(hexFP) <= 16 {
		return hexFP
	}
	return hexFP[0:4] + " " + hexFP[4:8] + " " + hexFP[8:12] + " " + hexFP[12:16] + "..."
}

// markOneProceeding sets OneProceeding when every VALID signature commits to the ceremony record
// THIS DOCUMENT CARRIES (D20, D2's UX pin).
//
// Only valid signatures are considered, for the same reason crossBind ignores invalid
// ones: a broken signature attests to nothing, so letting it vote would let a tampered
// signature deny a genuine ceremony. And an empty commitment on any valid signature is
// disqualifying rather than ignored — a signer who carried no record is a signer who
// agreed to something else.
//
// **`want` is the record's own commitment, and comparing to it rather than to the other
// signatures is P07.S04's whole point.** Measured at its grill: two parties signing with the same
// arbitrary value they chose themselves, on a document with no ceremony record at all, reported
// one proceeding on every signature — and the client renders that as "✓ One proceeding". The
// token lives inside the signed `/Reason`, so agreement among signers is a fact about what they
// wrote, not evidence that the ceremony they name exists.
//
// An empty `want` therefore disqualifies: a caller that cannot say which ceremony this document
// belongs to must not be able to produce the ✓.
func markOneProceeding(ats []SignerAttestation, want string) {
	if want == "" {
		return
	}
	n := 0
	for _, a := range ats {
		if !a.Valid {
			continue
		}
		n++
		if !strings.EqualFold(a.RosterHash, want) {
			return
		}
	}
	if n == 0 {
		return
	}
	for i := range ats {
		if ats[i].Valid {
			ats[i].OneProceeding = true
		}
	}
}

// blockTextPt is the point size the attestation block's text renders at.
//
// The block is rasterised by the client (web/app.js renderAttestation) onto a canvas sized
// rect × 3, at `min(lineH*0.7, 9*scale)` pixels — so the effective size in PDF points, once
// the image is stretched back to fill the rect, is capped at 9. Named here rather than left
// implicit because the intent bound below is computed from it, and a bound derived from a
// number nobody wrote down is a bound that drifts when the canvas changes.
const blockTextPt = 9

// blockTextWidth is the drawable width of one block line, in points: the rect's width less
// the padding the rasteriser leaves on each side (4 canvas px at scale 3 ≈ 4 points once
// stretched back).
func blockTextWidth() float64 {
	r := NominalBlockRect()
	return (r[2] - r[0]) - 2*4
}

// IntentFitsBlock reports whether a recital renders in full on a signature block.
//
// # Why this is a REFUSAL and not a clamp
//
// `internal/server`'s cosignAttestation silently truncates the intent at 200 runes, and the
// client's `ctx.fillText` takes no maxWidth, so anything wider than the block is silently
// clipped at the canvas edge. Two independent silent truncations of one string — and under
// C15 that string is the ceremony's recital, committed inside RosterHash and required to
// appear *verbatim* on every block. A recital that renders cut in half is a document that
// says something other than what everyone signed.
//
// This repo's law is refuse-not-clamp (ErrReadmeOverflow is the precedent, and S08's finding
// that pdfcpu CLAMPS overflow is what made its own instrument blind), so the convene door
// refuses and the convener retypes rather than discovering it on the finished document.
//
// # The limit this bound exposes, stated rather than hidden
//
// It is ONE line, because AppearanceLines emits one entry per line and nothing wraps the
// recital across several. That makes the ceiling tight for a real recital. Widening it is
// **S07's** — the slice that owns rendering into blocks — and the fix there is to wrap the
// recital over however many lines it needs, at which point this bound is recomputed from the
// same geometry rather than raised by hand.
func IntentFitsBlock(intent string) bool {
	return mdpdf.CoreWidth("Intent: "+intent, readmeFont, blockTextPt) <= blockTextWidth()
}

// MaxIntentRunes is the longest recital that fits, measured rather than asserted — the
// number a refusal quotes so a convener knows how much to cut.
//
// Measured rather than a fixed character count, because count is not width: "MMMM" and
// "iiii" differ by nearly 3x at these metrics, so any constant is wrong for capitals or
// wasteful for lower case.
//
// **Binary search, and the linear version was a denial of service.** The first draft walked
// n down one rune at a time, calling IntentFitsBlock — a full CoreWidth over the prefix — at
// every step. Measured on the shipped metrics: 24ms at 1,000 runes, 2.45s at 10,000, and
// **77 seconds at 50,000**. The convene route's body limit is 1 MiB, so a convener pasting a
// clause out of the contract into the recital field could hang one core for hours with no
// response ever written. Quadratic work on a request path is reachable without malice, which
// is what made it a defect rather than a slow path.
//
// The predicate is monotone in n — a prefix of a fitting string fits — so a bisection is
// exact, not an approximation, and it is O(log n) calls.
func MaxIntentRunes(intent string) int {
	rs := []rune(intent)
	lo, hi := 0, len(rs) // lo always fits, hi is not yet known to
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if IntentFitsBlock(string(rs[:mid])) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}
