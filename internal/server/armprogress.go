package server

import "time"

// What is happening while a ceremony is armed (P06.S05, D15, D16 amendment, D34).
//
// **This is not the diagnosis and must not become one.** `diagnosisView` answers *why nothing has
// connected*, and it is deliberately gated on `bootstrapDone` — a cause computed before the DHT has
// had its chance would accuse the wrong tier. That gate is right, and it is also why the screen is
// currently BLANK for the longest stretch of an arm: under ADR-011 nothing bootstraps until the
// local link has had its window, and where a browse has already answered the hold is
// `lanFirstBudget`, thirty seconds. D16's criterion is that the connection screen shows per-tier
// progress for the whole connect deadline and never a blank spinner, so what was missing is a live
// view that is not a verdict.
//
// **It publishes; it does not track.** Every field below is read from state that already exists for
// other reasons — `linkWatchAt`/`linkSeenAt` are ADR-011's evidence hold, `bootstrapDone` gates the
// diagnosis, and the mapper's lease and refusal already feed D19. Adding a second source of truth
// for any of them would be the duplicate-derivation defect one layer up from the code that has it.

// armProgress is what each tier of D8's ladder is doing, right now.
type armProgress struct {
	// Link is the local-link tier: "watching" once this arm is listening for its peer on the
	// link, "found" once it has resolved a sighting of the party it is expecting.
	//
	// **"found" means its OWN expected peer, not any announcement.** ADR-011's hold renews on
	// evidence — *"every resolved sighting of its own expected peer renews the hold"* — and a
	// screen that said "found" for a stranger's announcement would tell a user the ceremony is
	// progressing when nothing of theirs has been seen.
	Link string `json:"link,omitempty"`
	// DHT is the rendezvous tier: "holding" while ADR-011's window has not elapsed, "reaching"
	// once the bootstrap has been attempted.
	//
	// **"holding" is the state the screen has never had a word for**, and it is the one that
	// lasts longest on a LAN. It is not a failure and not a delay to apologise for: it is the
	// product deliberately not touching the public network until the link has had its chance.
	DHT string `json:"dht,omitempty"`
	// Router is the port-mapping tier (D15): "open" with a Port, "silent" when no gateway
	// answered, "refused" when one answered and said no, "unroutable" when the answer could not
	// be published, and "" when this arm never asked.
	//
	// **Four states and not two, because D9's advice diverges between three of them.** Silence
	// means there may be no gateway to ask; refused means *"the router is the user's, is
	// reachable, and said no"*; unroutable means a double-NAT and points at a VPN rather than a
	// port-forward. A screen collapsing them into "no mapping" would give one answer to three
	// different users.
	Router string `json:"router,omitempty"`
	// Port is the external port a mapping opened, when Router is "open".
	Port uint16 `json:"port,omitempty"`
}

// armProgressOf reads the live tier state for an armed ceremony.
//
// Nil for no ceremony — the manual and plain co-sign paths have no ladder to report on, and a
// progress view for them would be four empty strings pretending to be information.
func (c *ceremonyID) armProgressOf(now time.Time) *armProgress {
	if c == nil {
		return nil
	}
	out := &armProgress{}
	if c.linkWatchAt.Load() != 0 {
		out.Link = "watching"
	}
	if c.linkSeenAt.Load() != 0 {
		// **"found" rather than the word this state would obviously use.** The published-observable
		// scan claims a reader for a JSON field when a `.js` reader mentions the field's tag as a
		// bare word, and the LAN-browse response's list field has the obvious tag — so a client
		// rendering this value under that name would silently claim a reader for a field that has
		// none and is parked under `/pending 23`. The state is "this arm resolved a sighting of its
		// OWN expected peer", which "found" says exactly as well. Recorded here rather than in the
		// client because the wire value is where the choice actually lives.
		out.Link = "found"
	}
	if c.bootstrapDone.Load() {
		out.DHT = "reaching"
	} else if out.Link != "" {
		// **"holding" is only claimed where something is actually holding.** An arm that is not
		// watching the link is not waiting for it either — the dial side, and the plain paths —
		// and saying "holding" there would describe a wait nobody is doing.
		out.DHT = "holding"
	}
	c.mu.Lock()
	pm, unroutable, refused := c.portMap, c.mapUnroutable, c.mapRefused
	c.mu.Unlock()
	switch {
	case unroutable:
		out.Router = "unroutable"
	case refused:
		out.Router = "refused"
	case pm == nil:
		// Never asked. Distinct from asking and hearing nothing, which is "silent".
	default:
		if port, have := pm.lease(); have {
			out.Router, out.Port = "open", port
		} else {
			out.Router = "silent"
		}
	}
	if out.Link == "" && out.DHT == "" && out.Router == "" {
		// Nothing to say is said by saying nothing, rather than by four empty strings that a
		// client would render as a ladder with no rungs.
		return nil
	}
	return out
}
