package server

import (
	"net/http"
	"time"

	"nib/internal/ceremony"
	"nib/internal/vault"
)

// The advanced-features switch (`/pending 451`).
//
// Dan, 2026-09-09: *"I will want a settings page which shows those items and allows me to
// enable/disable those functions."* And 2026-09-10, on the default: *"default off. menus hidden."*
//
// # The rule this file exists to keep
//
// **Off means the function STOPS, not that its button is hidden.** Hiding is the second half and
// never the whole of it — this repo has shipped the hidden-button-only shape twice (`/pending 378`,
// where leaving a ceremony released nothing, and `/pending 3`, where discovery announces every
// 500 ms whether or not anyone is looking at the panel). So every check here is at the door that
// DOES the thing: the announcer's constructor, the DHT bootstrap, the arm sweep, the two routes.
//
// # One door, per ADR-009
//
// `advancedOn` is the only reader of the setting. A per-site `if enabled` is how six rules reached
// some sites and not others before; a caller asks this and nothing else.

// advancedFeature names one of the four subsystems, so a caller cannot pass a string nobody serves.
type advancedFeature int

const (
	featCeremony advancedFeature = iota
	featDiscovery
	featRendezvous
	featTimestamp
)

func (f advancedFeature) String() string {
	switch f {
	case featCeremony:
		return "signing ceremonies"
	case featDiscovery:
		return "local network discovery"
	case featRendezvous:
		return "remote peer rendezvous"
	case featTimestamp:
		return "timestamping"
	}
	return "that feature"
}

// advancedOn reports whether one subsystem is switched on for this machine.
//
// **A locked vault answers FALSE for everything**, and that is the honest answer rather than a
// defensive one: the setting lives in the vault, so a machine that cannot read it does not know
// that the user asked for the feature, and doing a network thing on a maybe is the failure this
// switch exists to prevent. Every caller is already behind `requireUnlocked` or holds a vault.
func advancedOn(v *vault.Vault, f advancedFeature) bool {
	if v == nil {
		return false
	}
	a := v.Settings().Advanced
	if a == nil {
		return false // never configured — the default, which is off
	}
	switch f {
	case featCeremony:
		return a.Ceremony
	case featDiscovery:
		return a.Discovery
	case featRendezvous:
		return a.Rendezvous
	case featTimestamp:
		return a.Timestamp
	}
	return false
}

// refuseIfOff writes the 403 a disabled feature earns, and returns true when it did.
//
// **403 rather than 404**, because the route exists and the answer is about permission this machine
// has given itself; a 404 would send a caller looking for a typo. The sentence names the feature and
// where the switch is, since a user meeting this has almost certainly forgotten they turned it off.
func refuseIfOff(w http.ResponseWriter, v *vault.Vault, f advancedFeature) bool {
	if advancedOn(v, f) {
		return false
	}
	httpError(w, http.StatusForbidden,
		"this machine has "+f.String()+" switched off — turn it back on under Settings → Advanced features")
	return true
}

// SeedAdvanced resolves a vault that has never been asked, ONCE, and writes the answer down.
//
// # Why a seed at all, when the default is simply off
//
// Because "default off" and "switch four things off on an existing install" are different acts, and
// only the first is what Dan asked for. A machine that accepted an invitation last week holds a pin,
// a `me` marker and an arm waiting for its hop; shipping a version that quietly stops serving it
// would mean the proceeding never advances and **nothing on screen says why** — the panel is hidden
// too. That is the shape `/pending 462` closed on: correct internals, and a user who cannot see what
// happened.
//
// # So the seed is as narrow as it can be
//
// Exactly one condition turns anything on: a ceremony on this disk that has **not ended**. Where
// there is one, the three subsystems it needs to advance — the ceremony itself, the local link and
// the rendezvous — come up on. Everything else, on every other machine, starts off. A fresh vault
// gets nothing; an existing vault with no live proceeding gets nothing, which is the common case and
// is exactly the default Dan asked for.
//
// Timestamping is never seeded on. There is no on-disk state that says a user has been using it, so
// there is nothing to strand — and inventing a proxy for "they probably want it" would turn a narrow
// migration into a second default.
//
// # Written down, so it happens once
//
// The result is stored whatever it decides, which is what makes `Advanced` non-nil from then on. A
// user who switches the ceremony off afterwards stays off: without the write, the next unlock would
// see nil again and turn it back on, and a setting that undoes itself at every restart is worse than
// no setting.
func (s *Server) SeedAdvanced(v *vault.Vault) {
	if v == nil {
		return
	}
	cur := v.Settings()
	if cur.Advanced != nil {
		return // already answered, by the seed or by the user
	}
	live := hasLiveCeremony()
	cur.Advanced = &vault.Advanced{Ceremony: live, Discovery: live, Rendezvous: live}
	// Best-effort, deliberately: a vault that cannot be written is a vault the user has bigger
	// problems with, and refusing to start over a preference would be the wrong trade. The cost of
	// the write failing is that the seed runs again next time and reaches the same answer.
	_ = v.SetSettings(cur)
}

// hasLiveCeremony reports whether this machine holds a proceeding that has not ended.
//
// `ListStored` reads `record.json` and nothing else (P08.S03), and `rearmCeremonies` already walks
// exactly this list at every unlock — so this costs a directory read the unlock path was making
// anyway. `Ended` is empty for a running ceremony AND for one whose end nobody has observed, and
// treating the unknown as live is the right direction here: it errs towards leaving a feature on.
func hasLiveCeremony() bool {
	stored, err := ceremony.ListStored(defaultOutputDir(), time.Now())
	if err != nil {
		return false
	}
	for _, st := range stored {
		if st.Ended == "" {
			return true
		}
	}
	return false
}

// gateRendezvous stamps the advanced-features answer onto a ceremony this server is about to use.
//
// **Every production path that builds a `ceremonyID` goes through here**, which is what makes
// `ensureBootstrapped`'s nil-means-allowed safe: nil is the CLI and the tests, and
// `TestEveryCeremonyThisServerUsesIsGated` asserts no server path leaves one unstamped.
//
// A closure over the server rather than a captured bool, because the user can switch the DHT off
// while a ceremony is open and the next bootstrap attempt should see that.
func (s *Server) gateRendezvous(c *ceremonyID) *ceremonyID {
	if c == nil {
		return nil
	}
	c.rzOn = func() bool { return advancedOn(s.unlockedVault(), featRendezvous) }
	return c
}
