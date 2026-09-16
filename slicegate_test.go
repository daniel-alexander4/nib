package nib

import (
	"os/exec"
	"strings"
	"testing"
)

// /pending 426 — the slice gate is a rule enforced by a sentence, and it lapsed five times.
//
// `CLAUDE.md` requires a change touching `internal/server`'s session / ceremony / delivery /
// discovery paths, `internal/p2p` or `internal/rendezvous` to run tiers 4 and 6 — **and to print
// one line when it does NOT fire**, because a trigger nobody records is one nobody can tell was
// evaluated. Five consecutive commits recorded neither, three of them squarely on trigger paths.
// The backstop run found nothing wrong, which is exactly what makes it worth a mechanism: a gate
// that found nothing is indistinguishable afterwards from a gate that was never needed.
//
// **And it lapsed four more times on 2026-09-09, in the sweep that closed this item** — all four
// comment-only changes to `internal/p2p/l3.go`, `internal/server/delivery.go`,
// `internal/server/ceremonyid.go` and `internal/ceremony/`. Comment-only is not an exemption the
// rule offers, and "I did not think of it because nothing executable changed" is precisely the
// shape a sentence cannot prevent.
//
// So the rule gets a caller. It reads the commit message, because that is where the rule already
// says the line goes.
//
// **Bounded from a baseline**, and that is the honest way to introduce it: history is not
// rewritten to satisfy a check added today, so the scan starts at the commit where the check
// landed. The predicate itself is unit-tested below, so the bound does not leave it unexercised.
func TestEveryCommitTouchingATriggerPathRecordsTheSliceGate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on PATH")
	}
	if err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		t.Skip("not a git work tree (a release tarball, or a vendored copy)")
	}
	// The baseline: the commit this check landed in. Everything from here on complies.
	const baseline = "8074859"
	out, err := exec.Command("git", "log", "--format=%H", baseline+"..HEAD").Output()
	if err != nil {
		t.Skipf("cannot read the log from %s (a shallow clone, or a rebased history): %v", baseline, err)
	}
	shas := strings.Fields(string(out))
	for _, sha := range shas {
		files, ferr := exec.Command("git", "show", "--name-only", "--format=", sha).Output()
		if ferr != nil {
			t.Fatalf("git show %s: %v", sha, ferr)
		}
		if !touchesSliceGateTrigger(strings.Fields(string(files))) {
			continue
		}
		msg, merr := exec.Command("git", "log", "-1", "--format=%B", sha).Output()
		if merr != nil {
			t.Fatalf("git log %s: %v", sha, merr)
		}
		if !recordsSliceGate(string(msg)) {
			subj, _ := exec.Command("git", "log", "-1", "--format=%s", sha).Output()
			t.Errorf("%s touches a slice-gate trigger path and its message records neither a run "+
				"nor a non-firing line: %s\n\tCLAUDE.md requires one either way — a trigger "+
				"nobody records is one nobody can tell was evaluated, and this rule has already "+
				"lapsed nine times (/pending 426).", sha[:8], strings.TrimSpace(string(subj)))
		}
	}
}

// touchesSliceGateTrigger reports whether a change touches the paths CLAUDE.md names.
//
// `internal/ceremony` is included and CLAUDE.md's sentence does not name it, which is a
// deliberate widening rather than a misreading: the rule's subject is "a change no single-process
// test can see", and the ceremony package is where the record, the mirror and the termination
// live — every one of them read by two machines. Narrower than the sentence would let a
// mirror-format change through, which is the shape both regressions the rule records took.
//
// **Widened at /pending 505 to what tiers 4 and 6 actually drive.** The server list named the
// ceremony's own files and none of the network half those tiers exist for: the arm and accept
// path (`accept`, `arm*`, `arrival*`, `rearm`), the dial and its races (`dial*`, `race*`,
// `rerace`, `glare`, `punch`, `identityrace`), the link and its announcements (`lan*`,
// `announce*`, `discover*` — which also covers the old `discovery` — `browsequiet`, `linkhold`,
// `lazybootstrap`), candidates and port mapping (`candidate*`, `portmap*`), the pinned-peer
// routes (`peers`), the hop and its pre-hop legs (`hop*`, `prehop*`, `walklegs`, `endstatepull`),
// the consent and spoken-check gates a two-process run parks on (`consent*`, `spokencheck`),
// the shared socket and transports (`shared*`, `allquic`, `dualfamily`) and `rendezvous*`. Three
// packages every one of those reaches joined the list for the same reason: `internal/udpmux` (the
// one socket QUIC and the DHT share), `internal/portmap` (every arm asks it, and its budget is
// spent inside a tier-4 hop), and `internal/addrscope` (which candidates are dialled at all).
// Measured against history before widening: no commit from the baseline on touches a newly
// named path without recording the gate, so the widening needs no second baseline.
func touchesSliceGateTrigger(files []string) bool {
	for _, f := range files {
		switch {
		case strings.HasPrefix(f, "internal/p2p/"),
			strings.HasPrefix(f, "internal/rendezvous/"),
			strings.HasPrefix(f, "internal/ceremony/"),
			strings.HasPrefix(f, "internal/discovery/"),
			strings.HasPrefix(f, "internal/udpmux/"),
			strings.HasPrefix(f, "internal/portmap/"),
			strings.HasPrefix(f, "internal/addrscope/"):
			return true
		}
		if strings.HasPrefix(f, "internal/server/") {
			base := strings.TrimPrefix(f, "internal/server/")
			for _, p := range sliceGateServerPrefixes {
				if strings.HasPrefix(base, p) {
					return true
				}
			}
		}
	}
	return false
}

// sliceGateServerPrefixes — the `internal/server/` file-name prefixes tiers 4 and 6 drive. See
// touchesSliceGateTrigger for why each is here.
var sliceGateServerPrefixes = []string{
	// the ceremony's own files (the original list; `discover` subsumes `discovery`)
	"session", "ceremony", "delivery", "discover", "closeout", "convene", "cosign", "leave",
	// arm, accept and arrival
	"accept", "arm", "arrival", "rearm",
	// the dial and its races
	"dial", "race", "rerace", "glare", "punch", "identityrace",
	// the link, announcements and the rendezvous
	"lan", "announce", "browsequiet", "linkhold", "lazybootstrap", "rendezvous",
	// candidates, port mapping and pinned peers
	"candidate", "portmap", "peers",
	// the hop and its legs
	"hop", "prehop", "walklegs", "endstatepull",
	// the gates a two-process run parks on
	"consent", "spokencheck",
	// the shared socket and the transports
	"shared", "allquic", "dualfamily",
}

// recordsSliceGate reports whether a commit message says what happened to the gate.
//
// Deliberately loose about WORDING and strict about presence: the rule asks for a line, not a
// format, and a check that demands an exact phrase makes people write the phrase.
func recordsSliceGate(msg string) bool {
	l := strings.ToLower(msg)
	for _, k := range []string{"slice gate", "tier 4", "tiers 4", "tier 6", "tiers 6"} {
		if strings.Contains(l, k) {
			return true
		}
	}
	return false
}

// The predicates, exercised directly — the scan above is bounded from a baseline, so without
// this the rule's logic would ship untested until the next commit that happens to trip it.
func TestTheSliceGatePredicatesAreNotVacuous(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
		want  bool
	}{
		{"a delivery change", []string{"internal/server/delivery.go"}, true},
		{"a p2p change", []string{"internal/p2p/l3.go"}, true},
		{"a ceremony change", []string{"internal/ceremony/mirror.go"}, true},
		{"rendezvous", []string{"internal/rendezvous/dht.go"}, true},
		{"a comment-only delivery change is STILL a trigger", []string{"internal/server/delivery.go", "README.md"}, true},
		// /pending 505 — the network half tiers 4 and 6 drive, one row per family so a dropped
		// prefix fails by name.
		{"the accept route", []string{"internal/server/accept.go"}, true},
		{"the arm's progress", []string{"internal/server/armprogress.go"}, true},
		{"glare", []string{"internal/server/glare.go"}, true},
		{"the LAN link", []string{"internal/server/lan.go"}, true},
		{"hole punching", []string{"internal/server/punch.go"}, true},
		{"the re-race", []string{"internal/server/rerace.go"}, true},
		{"the port mapper", []string{"internal/server/portmapper.go"}, true},
		{"pinned peers", []string{"internal/server/peers.go"}, true},
		{"the discover route", []string{"internal/server/discover.go"}, true},
		{"the shared socket", []string{"internal/server/sharedsocket_test.go"}, true},
		{"the demultiplexer", []string{"internal/udpmux/mux.go"}, true},
		{"port mapping", []string{"internal/portmap/upnp.go"}, true},
		{"address scope", []string{"internal/addrscope/scope.go"}, true},
		{"an unrelated server file", []string{"internal/server/export.go"}, false},
		{"the save route is not the network", []string{"internal/server/saveas.go"}, false},
		{"the window stream is not the network", []string{"internal/server/window.go"}, false},
		{"pdfops", []string{"internal/pdfops/pdfops.go"}, false},
		{"docs only", []string{"README.md", "docs/adr/030-x.md"}, false},
		{"web only", []string{"web/app.js"}, false},
	} {
		if got := touchesSliceGateTrigger(tc.files); got != tc.want {
			t.Errorf("touchesSliceGateTrigger(%v) = %v, want %v (%s)", tc.files, got, tc.want, tc.name)
		}
	}
	for _, tc := range []struct {
		name string
		msg  string
		want bool
	}{
		{"a run recorded", "…\n\nSLICE GATE FIRED: tier 6 27/27, tier 4 -n 3 PASS.", true},
		{"a non-firing line", "…\n\nSlice gate: tiers 4 and 6 do NOT fire — pdfops only.", true},
		{"lower case", "ran tier 4 at three parties", true},
		{"silence", "fix(server): a thing\n\nTiers 0-3 green.", false},
		{"tiers 0-3 is not the gate", "Tiers: 1 green, 2 green 277/277, 3 green 111/111.", false},
	} {
		if got := recordsSliceGate(tc.msg); got != tc.want {
			t.Errorf("recordsSliceGate(%q) = %v, want %v (%s)", tc.msg, got, tc.want, tc.name)
		}
	}
}
