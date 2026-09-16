package server

import (
	"encoding/hex"
	"errors"
	"fmt"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/vault"
)

// A completed ceremony, attested (/pending 497).
//
// # What was wrong
//
// D28's first end state is *"Completed — every signing party on the roster has contributed and the
// delivery round runs"*, and `Termination`'s closed set has always admitted `completed` as one a
// convener can sign. **Nothing ever signed one.** `SignTermination` had two production callers —
// the stop route (`stopped`) and `endCeremony` (`declined`) — so on the convener's machine a
// proceeding every party had signed carried no end state at all, and three surfaces read that
// absence as "not ended":
//
//   - the card offered **Stop** and **Re-issue** (`!c.ended`) and never **Send everyone their copy**
//     (`c.ended`), under a line reading *"Everyone has signed"*;
//   - Stop did not ask whether the document was complete, wrote a write-once `stopped`, and its
//     round told every party *"the convener ended it before every party had signed"* — a false
//     statement about a proceeding that finished exactly as intended, and unrecoverable because the
//     termination is write-once;
//   - the convener's close-out had no way in: `closeOutReason`'s no-termination branch asks
//     `alreadyDelivered`, a stat on a signer's received copy the convener never holds, so a
//     completed ceremony sat in the live set until the grace ran out and was then recorded as
//     `abandoned`.
//
// # Why the mint is at the LAST HOP and not at the listing
//
// `ListStored` never opens a document — measured at P08.S03 as 10/69/195 ms for 100/500/1000
// pages — so completeness cannot be derived where the card is built. The convener's machine is the
// one place a ceremony becomes complete (D22's hub: the convener initiates every hop), and
// `mirrorHop` is the door every such hop passes through, so that is where the fact is written down.
// The deliver route mints lazily as well, for a ceremony that completed on an older build or whose
// mint failed; both go through `attestCompletion`.

// errNotComplete: the document still owes a signature, so `completed` would be a false attestation.
var errNotComplete = errors.New("not every signing party has signed this document yet")

// ceremonyProgress is the ONE reading of "how far has this proceeding got" over a verified record
// and its document (ADR-009).
//
// The roster comes from the RECORD and never from the document, for `l3RosterFrom`'s reason; the
// walk is `p2p.ContributionProgress`, which `NextContributor` and `AdmitContribution` are built on.
// The `/api/ceremony/next` answer, the stop refusal, the deliver refusal and the completion mint all
// ask here, so "complete" cannot mean one thing on the card and another at the route that acts.
func ceremonyProgress(rec ceremony.Record, pdf []byte) (p2p.Progress, error) {
	rh, err := rec.RosterHash()
	if err != nil {
		return p2p.Progress{}, fmt.Errorf("this ceremony's roster could not be checked: %w", err)
	}
	return p2p.ContributionProgress(pdf, l3RosterFrom(rec.Roster, hex.EncodeToString(rh), rec.Intent))
}

// attestCompletion writes the convener's `completed` termination for a ceremony whose document is
// complete, and returns the end state now on record.
//
// **An end state already on record is returned rather than contradicted.** `WriteTermination` is
// write-once, and a ceremony declined or stopped before its document happened to fill up has
// ended in that state; this is not the door that decides otherwise. `ErrBadTermination` is returned
// as-is, for `runDeliveryRound`'s reason: a round that cannot tell what it is carrying does not start.
//
// Refuses `errNotTheConvener` (only the convener attests, and `VerifyAgainst` enforces it at every
// reader) and `errNotComplete`.
func attestCompletion(v *vault.Vault, rec ceremony.Record, pdf []byte) (ceremony.Termination, error) {
	if v == nil {
		return ceremony.Termination{}, errors.New("no vault is open, so this machine cannot say who it is in this ceremony")
	}
	cert, key, err := identity(v)
	if err != nil {
		return ceremony.Termination{}, err
	}
	myFP, err := sign.Fingerprint(cert)
	if err != nil {
		return ceremony.Termination{}, err
	}
	if !isConvener(hex.EncodeToString(myFP), rec) {
		return ceremony.Termination{}, errNotTheConvener
	}
	prev, terr := ceremony.ReadTermination(defaultOutputDir(), rec)
	if terr == nil {
		return prev, nil
	}
	if !errors.Is(terr, ceremony.ErrNoTermination) {
		return ceremony.Termination{}, terr
	}
	pr, perr := ceremonyProgress(rec, pdf)
	if perr != nil {
		return ceremony.Termination{}, perr
	}
	if !pr.Complete {
		return ceremony.Termination{}, errNotComplete
	}
	t, serr := ceremony.SignTermination(rec, ceremony.StateCompleted, cert, key)
	if serr != nil {
		return ceremony.Termination{}, serr
	}
	if werr := ceremony.WriteTermination(defaultOutputDir(), t); werr != nil {
		return ceremony.Termination{}, werr
	}
	return t, nil
}

// attestIfCompleteAfterHop is the last-hop half: best-effort, quiet unless something the user can
// act on went wrong.
//
// **Quiet on the two ordinary outcomes.** Every hop but the last leaves the document incomplete,
// and a party's machine is not the convener; both are the normal case and neither is a failure.
// A failure to WRITE is reported, because its consequence is visible — the card goes on offering
// Stop on a finished proceeding — and the deliver route is the retry, which the notice says.
//
// **`s.unlockedVault()` and not a request's vault, and that is `mirrorHop`'s shape, not a choice
// made here**: it takes the document and nothing else, and `endCeremony` — the other convener-side
// mint on the hop path — reads the vault the same way.
func (s *Server) attestIfCompleteAfterHop(rec ceremony.Record, final []byte) {
	v := s.unlockedVault()
	if v == nil {
		return
	}
	_, err := attestCompletion(v, rec, final)
	switch {
	case err == nil, errors.Is(err, errNotComplete), errors.Is(err, errNotTheConvener):
		return
	}
	s.sess.noteFailure(armInteractive, "completion-not-recorded",
		"Everyone has signed, but Nib could not record that the proceeding is complete.",
		"The document is finished and every signature is on it. What failed is this machine "+
			"writing down that the proceeding is over, so the ceremony may still look live here. "+
			"Send everyone their copy tries again. Reason: "+err.Error())
}
