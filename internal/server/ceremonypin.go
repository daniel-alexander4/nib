package server

import (
	"fmt"
	"strings"

	"nib/internal/ceremony"
	"nib/internal/vault"
)

// The ceremony pin, one door (P07.S02b, ADR-009).
//
// D21 exists to remove a step: a party invited to a ceremony must not have to pin anybody by
// hand. Both session doors refuse an unpinned peer before they have even looked at the
// invitation — `handleSessionArm` at the `pinnedLabel` check, and the co-sign door at its own —
// so without this the invitation is parsed twenty-seven lines after the refusal it was supposed
// to answer.
//
// **Two callers, and the second is the one the plan did not have.** The invitee's side is
// `POST /api/ceremony/accept`. The convener's side is `POST /api/ceremony/convene`, which never
// pinned anything: its only vault write was `AddCeremonySecret`, so the convener manually pinned
// N-1 parties before arming against any of them — D21's harm at a second site. ADR-009: the rule
// is written once and every site calls it.

// pinCeremonyRoster pins the parties in `parties`, ceremony-scoped to `id`, skipping `selfFP`.
// It returns how many pins it established.
//
// **Who is in `parties` is the CALLER's, and the two callers pass very different sets** — which
// is a fact about D22 rather than an inconsistency. `hopBetween` refuses any pair that does not
// have the convener at one end ("under a convener hub every hop has the convener at one end"),
// so a counterparty's set of possible hop partners is `{the convener}`, of size one, and the
// convener's is everybody else. Passing a counterparty the whole roster would pin up to thirty
// strangers it can never dial — verbatim the harm D29 exists to prevent, delivered by its own
// fix.
//
// Ceremony-scoped, so `PruneCeremonyPeers` can take them away again (D29). An existing pin is
// never downgraded and never renamed — see `vault.AddCeremonyPeer`.
func pinCeremonyRoster(v *vault.Vault, id string, parties []ceremony.Party, selfFP string) (int, error) {
	if id == "" {
		// A ceremony pin with no ceremony is a permanent pin wearing a revocable label:
		// `PruneCeremonyPeers` refuses an empty id, so nothing would ever take it away.
		return 0, fmt.Errorf("a ceremony pin needs a ceremony id")
	}
	n := 0
	for _, p := range parties {
		if strings.EqualFold(p.Fingerprint, selfFP) {
			continue
		}
		fp, err := parseFingerprint(p.Fingerprint)
		if err != nil {
			// Named, because the alternative is a partial pin set: some parties pinned and
			// some not, with the caller's next arm failing against whichever was skipped.
			return n, fmt.Errorf("roster entry %q does not carry a usable fingerprint", p.Label)
		}
		label := p.Label
		if label == "" {
			// The six-word pairing name, which is what every other peer control in the
			// product shows when there is no label. An empty label would put a blank row in
			// the peer list, which reads as a bug rather than as an unnamed peer.
			label = nameOrEmpty(fp)
		}
		if err := v.AddCeremonyPeer(fp, label, id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
