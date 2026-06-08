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
	"strings"
	"time"

	"nib/internal/sign"
)

// defaultIntent is the agreement an attestation records when the caller gives none.
const defaultIntent = "I agree to sign this document."

// Attestation is one party's identity+intent confirmation. The signer's own
// identity comes from the signing certificate; this records what is not already
// in the signature: which counterparty the signer accepts (by SHA-256 SPKI
// fingerprint, see sign.Fingerprint) and the intent. It is the single source for
// both the visible appearance block and the signed /Reason, so the two cannot
// disagree.
type Attestation struct {
	Signer            string    // signer's identity common name (-> signature Name)
	AcceptedPeer      string    // hex SHA-256 SPKI of the accepted counterparty (-> /Reason)
	AcceptedPeerLabel string    // human label the signer pinned for the peer (-> /Reason, appearance)
	Intent            string    // what the signer agrees to (-> /Reason, appearance)
	When              time.Time // signing time
}

// spkiToken matches the machine-readable peer fingerprint embedded in /Reason.
var spkiToken = regexp.MustCompile(`\[SPKI:([0-9a-fA-F]{64})\]`)

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
func safeText(s string) string {
	return strings.NewReplacer("[", "", "]", "").Replace(s)
}

// reason encodes the attestation into a signature /Reason: a human sentence with
// a machine-parseable [SPKI:<hex>] token, so it reads cleanly in any viewer's
// signature panel yet round-trips exactly. The user-controlled label and intent
// are bracket-stripped so they can't forge the token (see safeText).
func (a Attestation) reason() string {
	return fmt.Sprintf("Accepts %s [SPKI:%s]. %s", safeText(a.AcceptedPeerLabel), a.AcceptedPeer, safeText(a.intent()))
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
	// on this document — i.e. this signer attests to a real co-signer's key, not a
	// claim about someone absent. For two-party mutual co-signing both are Matched.
	Matched bool `json:"matched"`
}

// ReadAttestations returns each signer's attestation from a co-signed PDF, in
// signature order. It reads the verified signers (so an attestation is only as
// trustworthy as its signature), parses the accepted-peer fingerprint from the
// signed /Reason, and cross-binds each accepted peer against the other signers'
// actual fingerprints (Matched).
func ReadAttestations(pdf []byte) []SignerAttestation {
	st := sign.Verify(pdf)
	out := make([]SignerAttestation, 0, len(st.Signers))
	for _, s := range st.Signers {
		sa := SignerAttestation{Signer: s.Name, Fingerprint: s.Fingerprint, Reason: s.Reason, When: s.When, Valid: s.Valid}
		if m := spkiToken.FindStringSubmatch(s.Reason); m != nil {
			sa.AcceptedPeer = m[1]
		}
		out = append(out, sa)
	}
	// Cross-binding: an accepted peer counts only if some OTHER signer actually
	// holds that fingerprint on this document.
	for i := range out {
		if out[i].AcceptedPeer == "" {
			continue
		}
		for j := range out {
			if j != i && out[j].Fingerprint == out[i].AcceptedPeer {
				out[i].Matched = true
				break
			}
		}
	}
	return out
}

// shortFingerprint renders the leading bytes of a hex fingerprint in spaced quads
// for at-a-glance comparison, eliding the rest. The full value is the pin.
func shortFingerprint(hexFP string) string {
	if len(hexFP) <= 16 {
		return hexFP
	}
	return hexFP[0:4] + " " + hexFP[4:8] + " " + hexFP[8:12] + " " + hexFP[12:16] + "..."
}
