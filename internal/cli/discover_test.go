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

// TestTheSummarySaysWhenOnlyONEFamilyJoined.
//
// # The failure this exists for
//
// `Stats.Interfaces` is a count of distinct INTERFACES, and `Socket.open` lists an interface
// when EITHER the IPv4 or the IPv6 join succeeds. So a host on which every IPv4 join failed
// reports exactly the same number as a healthy dual-stack host, no error is raised, and
// discovery is silently IPv6-only.
//
// That is not hypothetical: `ListenConfig.ListenPacket(… "udp" …)` yields an AF_INET6 socket,
// and the IPv4 joins on it succeed on Linux only through a Linux-specific `ipv6_setsockopt`
// passthrough. On macOS/BSD and Windows, `IP_ADD_MEMBERSHIP` on an AF_INET6 socket is the
// classic EINVAL/WSAEINVAL. `nib discover` on a real Windows box is what settles it — and
// before this, the command it would be settled with could not express the answer.
//
// # Why all four cases
//
// A test that drove only the IPv4-missing case would pass against `if st.Joined4 == 0` with
// no `Joined6 > 0` guard — which fires on a socket that joined NOTHING (already a hard error
// one layer down) and, worse, would say "IPv6-only" about a host that has no groups at all.
// Dual-stack is here for the opposite reason: it is the case that must stay SILENT, and an
// alarm that fires on a healthy host is one people learn to ignore.
func TestTheSummarySaysWhenOnlyONEFamilyJoined(t *testing.T) {
	for _, c := range []struct {
		name       string
		st         discovery.Stats
		want, deny string
	}{
		{"dual stack is silent",
			discovery.Stats{Interfaces: 2, Joined4: 2, Joined6: 2, Sent: 4, Own: 4},
			"(2 IPv4, 2 IPv6)", "NOTE: no IP"},
		{"no IPv4 join is named",
			discovery.Stats{Interfaces: 2, Joined4: 0, Joined6: 2, Sent: 4, Own: 4},
			"IPv6-ONLY", "IPv4-ONLY"},
		{"no IPv6 join is named",
			discovery.Stats{Interfaces: 2, Joined4: 2, Joined6: 0, Sent: 4, Own: 4},
			"IPv4-ONLY", "IPv6-ONLY"},
		{"nothing joined says neither, because that is a different failure",
			discovery.Stats{Interfaces: 0, Joined4: 0, Joined6: 0},
			"(0 IPv4, 0 IPv6)", "ONLY"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			printSummary(&out, c.st, 3*time.Second)
			got := out.String()
			// STIMULUS: the summary really rendered. Without it the "deny" assertions below
			// pass against an empty buffer, which is the same green a broken printer gives.
			if !strings.Contains(got, "interfaces joined") {
				t.Fatalf("printSummary produced no interface line, so nothing below is "+
					"measuring the summary:\n%s", got)
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("the summary does not say %q — a one-family join failure that no "+
					"output names is discovery silently working on half the addresses it "+
					"should:\n%s", c.want, got)
			}
			if strings.Contains(got, c.deny) {
				t.Errorf("the summary says %q, which is not true of this host:\n%s", c.deny, got)
			}
		})
	}
}
