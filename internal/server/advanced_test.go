package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/vault"
)

// The advanced-features switch (`/pending 451`). Dan, 2026-09-10: *"default off. menus hidden."*
//
// # The rule every arm below is about
//
// **Off has to mean the function stops.** A toggle that only hides UI is a lie the product tells,
// and this repo has shipped that exact shape twice — `/pending 378` found *"leave this ceremony"*
// releasing nothing, and `/pending 3` is open because discovery announces every 500 ms whether or
// not anyone is looking at the panel. So these drive the DOORS, not the panel.

// setAdvanced writes the four switches directly, which is the state a user would have reached
// through Settings.
func setAdvanced(t *testing.T, v *vault.Vault, a *vault.Advanced) {
	t.Helper()
	cur := v.Settings()
	cur.Advanced = a
	if err := v.SetSettings(cur); err != nil {
		t.Fatal(err)
	}
}

func TestTheDefaultIsOffAndOffRefusesAtTheRoute(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	v := srv.unlockedVault()

	// **Put the vault back to never-been-asked, because `authedClient` deliberately does not leave
	// it there.** That helper switches every advanced feature on so the package's thirty-eight
	// ceremony tests are about ceremonies rather than about this switch — and the price is that
	// THIS file has to restore the default explicitly. It is the right price: the default now has
	// exactly one place where it is written down and driven, instead of being an emergent property
	// of a helper nobody reads.
	setAdvancedNil(t, v)
	if v.Settings().Advanced != nil {
		t.Fatalf("setup: the vault still holds advanced settings %+v, so the refusals below would "+
			"not be about the default at all", v.Settings().Advanced)
	}

	// Timestamping: the route refuses before it reads a body.
	code, body := postForCode(t, c, csrf, ts.URL+"/api/timestamp", struct{}{})
	if code != http.StatusForbidden {
		t.Errorf("POST /api/timestamp with the feature unconfigured returned %d, want 403 — the "+
			"default is off, and off means the calendar servers are not contacted", code)
	}
	if body == "" {
		t.Error("the refusal carried no sentence; a user who forgot they turned it off learns nothing")
	}

	// The ceremony's two creating doors.
	if code, _ := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", struct{}{}); code != http.StatusForbidden {
		t.Errorf("convene returned %d with ceremonies off, want 403", code)
	}
	if code, _ := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept", struct{}{}); code != http.StatusForbidden {
		t.Errorf("accept returned %d with ceremonies off, want 403", code)
	}
}

// TestSwitchingOnOpensTheSameDoors is the positive control, and without it every assertion above
// passes on a build that refuses everything unconditionally.
func TestSwitchingOnOpensTheSameDoors(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	setAdvanced(t, srv.unlockedVault(), &vault.Advanced{Ceremony: true, Timestamp: true})

	// Not 200 — these are empty bodies — but NOT 403 either: the feature gate is past, and what
	// answers now is the route's own validation.
	if code, _ := postForCode(t, c, csrf, ts.URL+"/api/timestamp", struct{}{}); code == http.StatusForbidden {
		t.Error("timestamping is switched ON and the route still refuses it as disabled")
	}
	if code, _ := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", struct{}{}); code == http.StatusForbidden {
		t.Error("ceremonies are switched ON and convene still refuses as disabled")
	}
}

// TestTheArmSweepStopsWhenCeremoniesAreOff — the door a route-only gate would miss.
//
// D14 made accepting an invitation arm, and `rearmCeremonies` renews that arm at every unlock. A
// switch that refused only convene and accept would leave the machine listening for hops on a
// feature its user had turned off, which is the hidden-button-only shape verbatim.
func TestTheArmSweepStopsWhenCeremoniesAreOff(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.EnableDeliveryRearm()
	c, csrf := authedClient(t, ts)
	v := srv.unlockedVault()
	setAdvanced(t, v, &vault.Advanced{Ceremony: true, Discovery: true, Rendezvous: true})

	me := myFingerprint(t, c, ts.URL)
	invitation, _ := inviteFor(t, me)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	// SETUP: the arm actually comes up with the feature ON. Without this, "not armed when off" is
	// satisfied by a machine that never arms at all — the vacuous green in its plainest form.
	if !armedWithin(srv, 3*time.Second) {
		t.Fatal("setup: accepting did not arm with ceremonies switched ON, so this test cannot " +
			"show that switching them off is what stops it")
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{}); code != http.StatusOK {
		t.Fatalf("setup: disarm returned %d: %s", code, body)
	}

	setAdvanced(t, v, &vault.Advanced{})
	srv.rearmCeremonies(v)
	if srv.sess.status().Armed {
		postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
		t.Error("the sweep armed for a ceremony on a machine whose user has switched ceremonies " +
			"off — the feature is still running and only its panel is gone")
	}
}

// TestSwitchingCeremoniesOffIsRefusedWhileOneIsLive.
//
// `Termination` carries two attested end states and `Receipt` two derived ones, and *"the user
// switched the feature off"* is none of them: ending a live proceeding this way would have to be
// written as `abandoned` or `stopped`, both false statements to every other party. Refusing is the
// only outcome the record can express.
func TestSwitchingCeremoniesOffIsRefusedWhileOneIsLive(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	v := srv.unlockedVault()
	setAdvanced(t, v, &vault.Advanced{Ceremony: true})
	rec, _, _ := ceremonyOnDisk(t)

	// SETUP: the ceremony really is live, or the refusal below would be about nothing.
	if st := ceremony.ReadStored(defaultOutputDir(), rec.ID, time.Now()); st.Ended != "" {
		t.Fatalf("setup: the fixture ceremony is already ended (%q)", st.Ended)
	}

	off := advancedRequest{Ceremony: false}
	code, body := postForCode(t, c, csrf, ts.URL+"/api/settings", settingsRequest{Advanced: &off})
	if code != http.StatusConflict {
		t.Errorf("switching ceremonies off with one still running returned %d, want 409 — the "+
			"proceeding would be left unreachable with no state the record can express for it", code)
	}
	if !strings.Contains(body, "has not finished") {
		t.Errorf("the refusal says %q and does not tell the user what is running", body)
	}
	// And nothing was written: a refused change that stored itself anyway is the worst of both.
	if a := v.Settings().Advanced; a == nil || !a.Ceremony {
		t.Errorf("the refused change was stored anyway (%+v)", a)
	}
}

// TestTheSeedLeavesALiveCeremonyReachable — the migration Dan's default-off choice creates.
func TestTheSeedLeavesALiveCeremonyReachable(t *testing.T) {
	// A vault that has never been asked, on a machine that already holds a live ceremony: exactly
	// what an existing install looks like the first time it runs this build.
	sv := seedableVault(t)
	ceremonyOnDisk(t)
	setAdvancedNil(t, sv)

	seedSrv.SeedAdvanced(sv)
	a := sv.Settings().Advanced
	if a == nil {
		t.Fatal("the seed wrote nothing, so it will run again at every unlock and a user who " +
			"switches the ceremony off will find it back on after a restart")
	}
	if !a.Ceremony || !a.Discovery || !a.Rendezvous {
		t.Errorf("the seed left a live ceremony stranded: %+v. Its hop can never arrive, the "+
			"panel that would say so is hidden, and the user has no way to find out why", a)
	}
	if a.Timestamp {
		t.Error("the seed switched timestamping on. There is no on-disk state saying anyone was " +
			"using it, so nothing is stranded by leaving it off — turning it on is a second " +
			"default rather than a migration")
	}
}

// TestTheSeedLeavesAQuietMachineOff is the other half, and it is the common case: an existing
// vault with no live proceeding gets the default Dan asked for, not an exemption from it.
func TestTheSeedLeavesAQuietMachineOff(t *testing.T) {
	sv := seedableVault(t)
	setAdvancedNil(t, sv)
	seedSrv.SeedAdvanced(sv)
	a := sv.Settings().Advanced
	if a == nil {
		t.Fatal("the seed wrote nothing")
	}
	if a.Ceremony || a.Discovery || a.Rendezvous || a.Timestamp {
		t.Errorf("a machine with no live ceremony was seeded %+v — 'default off' became 'off for "+
			"new installs only', which is not what was asked for", a)
	}
}

// TestTheSeedDoesNotUndoTheUsersChoice.
func TestTheSeedDoesNotUndoTheUsersChoice(t *testing.T) {
	sv := seedableVault(t)
	ceremonyOnDisk(t) // live, so the seed WOULD turn the ceremony on if it ran
	setAdvanced(t, sv, &vault.Advanced{})

	seedSrv.SeedAdvanced(sv)
	if a := sv.Settings().Advanced; a == nil || a.Ceremony {
		t.Errorf("the seed overwrote a user who had switched everything off (%+v) — a setting "+
			"that undoes itself at every restart is worse than no setting", a)
	}
}

// seedSrv is the server whose SeedAdvanced is under test. `SeedAdvanced` reads only the vault it is
// handed and `hasLiveCeremony`'s HOME, so any enrolled server serves.
var seedSrv *Server

// seedableVault enrols a real vault and hands it back, with HOME already a sandbox.
//
// **It replaced a `t.Skip` on `srv.unlockedVault() == nil`, which was silently true for all three
// seed tests**: `startServerWith` builds a server and `authedClient` is what enrols one. Three tests
// reported SKIP and nothing was checked — the same shape as a guard whose scan finds no files.
func seedableVault(t *testing.T) *vault.Vault {
	t.Helper()
	ts, srv := startServerWith(t)
	authedClient(t, ts)
	v := srv.unlockedVault()
	if v == nil {
		t.Fatal("enrolling did not leave the server with an unlocked vault")
	}
	seedSrv = srv
	return v
}

// setAdvancedNil puts the vault back to never-been-asked.
func setAdvancedNil(t *testing.T, v *vault.Vault) {
	t.Helper()
	cur := v.Settings()
	cur.Advanced = nil
	if err := v.SetSettings(cur); err != nil {
		t.Fatal(err)
	}
}
