package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"nib/internal/discovery"
)

// The verdict table. `nib discover` exists so a user on a machine Nib cannot reach can
// say WHICH of four things went wrong, and it is the only instrument the parked Windows
// verification has — so if this logic is wrong, that run reports the wrong thing and
// nobody would know. Both verdict paths were driven by hand at P03.S05, which is evidence
// and not coverage.
func TestEachVerdictIsReachedByItsOwnCounters(t *testing.T) {
	for _, tc := range []struct {
		name string
		st   discovery.Stats
		code int
		says string
	}{
		{
			// No interface accepted an announcement. The list printed above the
			// verdict is what says which were skipped and why.
			name: "nothing was sent",
			st:   discovery.Stats{Interfaces: 0},
			code: 1,
			says: "nothing was sent",
		},
		{
			// The one cause this command can prove, because our own loopback copy
			// never leaves the host: it went out and did not come back.
			name: "sent, nothing came back",
			st:   discovery.Stats{Interfaces: 2, Sent: 20},
			code: 1,
			says: "NOT ONE came back",
		},
		{
			// The send path works. Nobody else is armed, or they are elsewhere.
			// Exit 0: this is not a fault in this machine.
			name: "heard ourselves, no peers",
			st:   discovery.Stats{Interfaces: 2, Sent: 20, Own: 20},
			code: 0,
			says: "we heard ourselves",
		},
		{
			name: "peers heard",
			st:   discovery.Stats{Interfaces: 2, Sent: 20, Own: 20, Peers: 3},
			code: 0,
			says: "working",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := printVerdict(&out, tc.st)
			got := out.String()
			if !strings.Contains(got, "VERDICT:") {
				t.Fatalf("no verdict was printed at all for %+v: %q", tc.st, got)
			}
			if !strings.Contains(got, tc.says) {
				t.Errorf("counters %+v produced %q, which does not say %q", tc.st, got, tc.says)
			}
			if code != tc.code {
				t.Errorf("counters %+v exited %d, want %d — the exit status is what a script "+
					"checking this machine reads", tc.st, code, tc.code)
			}
		})
	}
}

// TestTheFourVerdictsAreFourDifferentMessages.
//
// The command's whole claim is that *"found nothing" has three causes and a user can act
// differently on each*. Two branches that print the same sentence would satisfy every
// assertion in the table above — each would still contain its own substring — while
// telling the user nothing the collapsed pair could distinguish.
func TestTheFourVerdictsAreFourDifferentMessages(t *testing.T) {
	states := []discovery.Stats{
		{},
		{Interfaces: 2, Sent: 20},
		{Interfaces: 2, Sent: 20, Own: 20},
		{Interfaces: 2, Sent: 20, Own: 20, Peers: 3},
	}
	seen := map[string][]int{}
	for i, st := range states {
		var out bytes.Buffer
		printVerdict(&out, st)
		v := out.String()
		seen[v] = append(seen[v], i)
	}
	// STIMULUS: four states really did produce four renderings to compare. A helper
	// that wrote nothing would leave one empty key and read as "all identical" for a
	// reason that has nothing to do with the verdicts.
	for v := range seen {
		if strings.TrimSpace(v) == "" {
			t.Fatal("setup: a verdict rendered as empty text, so this test is comparing nothing")
		}
	}
	if len(seen) != len(states) {
		for v, which := range seen {
			if len(which) > 1 {
				t.Errorf("states %v all print the same verdict, so the command cannot tell "+
					"them apart: %q", which, v)
			}
		}
	}
}

// TestNothingSentIsDiagnosedBeforeNothingReturned pins the ORDER of the two failing cases.
//
// When nothing was sent, nothing can have come back — so `Own == 0` is *also* true of that
// state, and testing it first would tell a user "a local firewall is dropping multicast"
// about a machine where no announcement was ever attempted. A confident, wrong diagnosis
// is worse than a vague one, and it points the user at their firewall instead of at the
// interface list printed three lines above.
func TestNothingSentIsDiagnosedBeforeNothingReturned(t *testing.T) {
	var out bytes.Buffer
	// The only state a real socket can produce with Sent == 0: nothing out, nothing back.
	printVerdict(&out, discovery.Stats{Interfaces: 3, Own: 0, Sent: 0})
	got := out.String()
	if strings.Contains(got, "firewall") {
		t.Errorf("nothing was sent and the verdict blames a firewall: %q", got)
	}
	if !strings.Contains(got, "nothing was sent") {
		t.Errorf("nothing was sent and the verdict does not say so: %q", got)
	}
}

// TestOffLinkIsLoudOnlyWhenItHappened.
//
// OffLink is the one security-relevant counter in the set — its own doc says traffic from
// outside the link "is not a thing that happens by accident" — and it was omitted from
// this summary while all eight of its siblings were printed. A zero must stay quiet or
// every ordinary run trains the reader to skip the line.
func TestOffLinkIsLoudOnlyWhenItHappened(t *testing.T) {
	var quiet, loud bytes.Buffer
	printSummary(&quiet, discovery.Stats{Interfaces: 1, Sent: 4, Own: 4}, time.Second)
	printSummary(&loud, discovery.Stats{Interfaces: 1, Sent: 4, Own: 4, OffLink: 2}, time.Second)

	if strings.Contains(quiet.String(), "OFF-LINK") {
		t.Errorf("a run with no off-link traffic shouts about it: %q", quiet.String())
	}
	if !strings.Contains(quiet.String(), "off-link") {
		t.Errorf("the off-link counter is missing from an ordinary summary: %q", quiet.String())
	}
	if !strings.Contains(loud.String(), "OFF-LINK") {
		t.Errorf("two off-link datagrams were not called out: %q", loud.String())
	}
}

// TestEverySummaryCounterIsPrinted.
//
// The command's doc gives its reason for printing counters exhaustively: P03's lesson that
// "a counter nobody can read is worth nothing". OffLink was added to discovery.Stats and
// left out of this summary for a whole phase, and nothing failed. A test naming the labels
// one by one would have gone on passing when the tenth counter arrived, so this asserts the
// count instead — every exported field of discovery.Stats gets a line.
func TestEverySummaryCounterIsPrinted(t *testing.T) {
	st := discovery.Stats{
		Interfaces: 11, Sent: 22, SendErrors: 33, Own: 44,
		Peers: 55, Foreign: 66, Malformed: 77, OffLink: 88,
	}
	var out bytes.Buffer
	printSummary(&out, st, 3*time.Second)
	got := out.String()
	// **Two digits, and not 1..8.** The first draft used the values 1 through 8 with a
	// one-second window, and "summary after 1s" contains "1" — so dropping the
	// interfaces counter entirely would have gone unnoticed. Every value below is
	// unique, is not a substring of another, and appears nowhere in the fixed text.
	for _, want := range []string{"11", "22", "33", "44", "55", "66", "77", "88"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary omits the counter whose value is %s: %q", want, got)
		}
	}
}

// TestANonPositiveWindowRefusesInsteadOfDiagnosing.
//
// A zero window skips both loops and then reaches the verdict switch with every counter at
// zero — printing "nothing was sent. Discovery cannot work from this machine" about a
// machine where nothing was attempted. The refusal is the fix and it must keep its exit
// code: 2 is a usage error, and 1 is "this machine has a discovery problem".
func TestANonPositiveWindowRefusesInsteadOfDiagnosing(t *testing.T) {
	for _, arg := range []string{"0", "-1"} {
		code := cmdDiscover([]string{"--seconds", arg})
		if code != 2 {
			t.Errorf("`nib discover --seconds %s` exited %d, want 2 — a usage error, not a "+
				"verdict about the machine", arg, code)
		}
	}
}
