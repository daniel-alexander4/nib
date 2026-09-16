package server

import (
	"os"
	"strings"
	"testing"

	"nib/internal/vault"
)

// P02 residue — `/pending 519`: the seed and a user's save lost each other's writes.
//
// # What went wrong
//
// `SeedAdvanced` runs on the unlock goroutine, after the close-out sweep, the delivery re-arm and a
// DHT socket open, so it lands hundreds of milliseconds past unlock — which is when a user is in
// Settings. It read the settings, found `Advanced` unanswered, and wrote its copy back. A save
// landing in that gap was overwritten: the user ticked the ceremony boxes and the seed put all four
// switches back to off, with no error. The whole `Settings` struct goes back, so every other
// preference set in that window went with it. It surfaced as
// `TestAcceptingArmsTheListenerAndOnlyInARealNibProcess` failing under full-suite load with
// `accept returned 403: "this machine has signing ceremonies switched off"`, and passing solo.
//
// # Two readers were written first and BOTH were vacuous — recorded because the shape recurs
//
// The first raced `SeedAdvanced` against `vault.UpdateSettings` in two goroutines for 40 rounds.
// Probed against the reverted, genuinely racing seed it stayed **green**: two callers that each hold
// the lock across their own read and write cannot produce the losing interleaving, which needs one
// writer's read and write to straddle the other's commit.
//
// The second drove the real `/api/settings` route concurrently and asserted the result was never a
// "mix" of the two writers. That cannot fail either: both writers replace the whole `Advanced` block,
// so a lost update yields one writer's intended answer, never a mixture.
//
// Asserting that the user's answer survives a race would grade the SCHEDULER, not the code — the
// losing window is microseconds wide and the assertion is a distribution, which `/code-review`'s
// concurrency rule refuses for exactly this reason. So the property is asserted where it is
// deterministic: the seed must go through the door that holds the lock across read and write. The
// door's own concurrency guarantee is proven in `internal/vault`
// (`TestUpdateSettingsHoldsTheLockAcrossReadAndWrite`), where a correct door and a broken one differ
// under every schedule.

// TestTheSeedRoutesThroughTheLockingDoor — the discriminating reader for `/pending 519`.
func TestTheSeedRoutesThroughTheLockingDoor(t *testing.T) {
	src, err := os.ReadFile("advanced.go")
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(src), "func (s *Server) SeedAdvanced(")
	if i < 0 {
		t.Fatal("SeedAdvanced is gone from advanced.go, so its route is unchecked and the " +
			"assertions below are vacuous")
	}
	body := funcBodyFrom(string(src), i)
	if body == "" {
		t.Fatal("SeedAdvanced's body did not brace-match")
	}
	if !strings.Contains(body, "v.UpdateSettings(") {
		t.Error("SeedAdvanced does not write through UpdateSettings. Its guard and its write must " +
			"happen under ONE hold of the vault lock: read the settings, decide, and write back as " +
			"three steps and a save landing in between is discarded — which is /pending 519")
	}
	if strings.Contains(body, "v.SetSettings(") {
		t.Error("SeedAdvanced calls SetSettings. That writes the whole struct back from a copy read " +
			"earlier, so anything saved since that read is silently lost — including, measured, a " +
			"user switching the ceremony features on seconds after unlock")
	}
}

// TestTheSeedNeverOverwritesAnAnswerThatExists — the guard's behaviour, deterministically.
//
// **What this catches and what it does not.** It fires if the guard is removed or inverted, which is
// the likeliest future regression. It does NOT distinguish a guard asked inside the lock from one
// asked outside it — the old, racing shape passes this too, because sequentially its guard is
// correct. That distinction is the routing test above; this one pins the behaviour the routing
// serves, so neither test alone is the whole claim.
func TestTheSeedNeverOverwritesAnAnswerThatExists(t *testing.T) {
	sv := seedableVault(t)
	ceremonyOnDisk(t) // live, so a seed that DID run would switch everything on

	// The user's answer: everything off, deliberately, on a machine where the seed disagrees.
	setAdvanced(t, sv, &vault.Advanced{})

	seedSrv.SeedAdvanced(sv)

	a := sv.Settings().Advanced
	if a == nil {
		t.Fatal("the seed cleared the user's answer entirely")
	}
	if a.Ceremony || a.Discovery || a.Rendezvous {
		t.Errorf("the seed overwrote an answer that already existed (%+v) — a setting that undoes "+
			"itself at every unlock is worse than no setting", a)
	}
}
