package discovery

import (
	"net"
	"os"
	"strings"
	"testing"
)

// ifi builds an interface the way the cases below need to talk about one. Addrs()
// cannot be faked on a net.Interface — it queries the kernel by index — so the table
// tests exercise the flag logic and hasV4 separately, and say so rather than
// pretending one test covers both.
func ifi(name string, idx int, flags net.Flags) net.Interface {
	return net.Interface{Index: idx, Name: name, Flags: flags}
}

const (
	up   = net.FlagUp
	mc   = net.FlagMulticast
	bc   = net.FlagBroadcast
	run  = net.FlagRunning
	loop = net.FlagLoopback
	p2p  = net.FlagPointToPoint
)

// TestInterfaceSelectionSkipsWhatItShould is the table the pure function exists for.
//
// Every row is a real machine's real interface: a wifi card, a docker bridge with
// nothing attached, a WireGuard tun, an interface that is down.
//
// **It tests flagsAllow, not chooseInterfaces, and that is the point.** The first
// version built synthetic net.Interface values and passed them to chooseInterfaces —
// which calls Addrs(), which queries the KERNEL BY INDEX. A fixture with Index: 3
// reported the host's interface-3 addresses, so the docker0 row survived the address
// filter only because this machine's third interface happens to have one. On a host
// where it does not — a CI container with lo and eth0 — the row is dropped and the test
// fails claiming "FlagRunning became a filter", which is not what went wrong.
//
// A decision that queries the kernel cannot be table-tested, so the decision was split
// out. The address half is hasV4/HasIPv4 below, pure and synthetic; the two together on
// a REAL interface are driven in the namespace.
func TestInterfaceSelectionSkipsWhatItShould(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags net.Flags
		want  bool
		why   string
	}{
		{"lo", up | run | loop, false,
			"loopback: IP_MULTICAST_LOOP already delivers a local copy, so joining lo " +
				"duplicates every datagram"},
		{"wlp1s0", up | run | bc | mc, true, "an ordinary wifi interface"},
		{"docker0", up | bc | mc, true,
			"UP but no carrier. Selected DELIBERATELY: FlagRunning would exclude it on " +
				"Linux and carries no information on Windows, where it is set from the same " +
				"condition as FlagUp, so filtering on it would mean two different things per " +
				"platform"},
		{"wg0", up | run | p2p, false,
			"point-to-point: no L2 broadcast domain, and a join either does nothing or " +
				"leaks who is signing on this LAN into a VPN"},
		{"eth9", 0, false, "down"},
		{"eth8", mc | bc, false, "not up, whatever else it advertises"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := flagsAllow(tc.flags); got != tc.want {
				t.Errorf("flagsAllow(%v) = %v, want %v — %s", tc.flags, got, tc.want, tc.why)
			}
		})
	}
}

func TestHasV4(t *testing.T) {
	v4 := &net.IPNet{IP: net.IPv4(192, 168, 1, 5), Mask: net.CIDRMask(24, 32)}
	v6 := &net.IPNet{IP: net.ParseIP("fd00::1"), Mask: net.CIDRMask(64, 128)}
	ll6 := &net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)}

	if !hasV4([]net.Addr{v6, v4}) {
		t.Error("an interface with an IPv4 address reported none")
	}
	// The Windows case: v6 up, no v4 lease yet. Linux joins it, Windows refuses it,
	// and requiring the address on both is what keeps the chosen set identical.
	if hasV4([]net.Addr{v6, ll6}) {
		t.Error("an IPv6-only interface reported an IPv4 address — on Windows the IPv4 join " +
			"resolves the interface to an ADDRESS and would fail, so this is the check that " +
			"keeps Linux and Windows choosing the same interfaces")
	}
	if hasV4(nil) {
		t.Error("an interface with no addresses reported an IPv4 address")
	}
}

func TestDescribeNamesEveryInterfaceAndItsVerdict(t *testing.T) {
	all := []net.Interface{ifi("lo", 1, up|loop), ifi("wlp1s0", 2, up|mc)}
	chosen := []net.Interface{all[1]}
	d := describe(all, chosen)
	// Discovery fails silently by nature, so the one thing that must never be
	// missing is a plain statement of what was tried.
	for _, want := range []string{"lo", "skipped", "wlp1s0", "JOINED"} {
		if !strings.Contains(d, want) {
			t.Errorf("the diagnostic does not mention %q: %s", want, d)
		}
	}
	if describe(nil, nil) == "" {
		t.Error("describe returned empty for no interfaces — a diagnostic that says nothing " +
			"when there is nothing is the case it is most needed for")
	}
}

// TestTheIPv4SelectionNeedsAnIPv4Address is the second Windows divergence, at the level
// the pure function can reach.
//
// On Windows an IPv4 group join resolves the interface to an ADDRESS —
// setIPv4MreqToInterface walks ifi.Addrs() and fails without an IPv4 one — where Linux
// uses the interface INDEX. So an interface with IPv6 up and DHCP still pending is
// joinable on Linux and refused on Windows, and no Linux test would ever show it.
//
// Requiring the address on BOTH platforms makes the chosen set identical on both, which
// is worth more than the one interface it costs. The driven half of this, on a real
// IPv6-only interface, is in the namespace — see mcastrepro.sh.
func TestTheIPv4SelectionNeedsAnIPv4Address(t *testing.T) {
	// The two calls differ in exactly one argument, and that argument must be the
	// only thing that decides an IPv6-only interface's fate.
	v6only := []net.Addr{&net.IPNet{IP: net.ParseIP("fd00::1"), Mask: net.CIDRMask(64, 128)}}
	dual := []net.Addr{
		&net.IPNet{IP: net.ParseIP("fd00::1"), Mask: net.CIDRMask(64, 128)},
		&net.IPNet{IP: net.IPv4(10, 0, 0, 1), Mask: net.CIDRMask(24, 32)},
	}
	if HasIPv4(v6only) {
		t.Error("an IPv6-only interface reports an IPv4 address")
	}
	if !HasIPv4(dual) {
		t.Fatal("a dual-stack interface reports no IPv4 address; the assertion above is not " +
			"about the distinction it claims to be")
	}
}

// TestAnIPv6OnlyInterfaceIsSkippedForTheIPv4Group is the driven half, and it needs a real
// interface with a real address — which is why it runs only inside the namespace
// build/mcastrepro.sh creates, where a v6-only dummy exists.
//
// net.Interface.Addrs() queries the kernel by index, so this distinction cannot be faked:
// the table test above exercises HasIPv4 on synthetic addresses, and only a real
// interface exercises the SELECTION.
func TestAnIPv6OnlyInterfaceIsSkippedForTheIPv4Group(t *testing.T) {
	if os.Getenv("NIB_MCAST_NETNS") != "1" {
		t.Skip("not in a prepared network namespace — run build/mcastrepro.sh")
	}
	all, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}

	// Stimulus: the namespace really does contain an interface with IPv6 and no IPv4.
	// Without it, "the v4 set is smaller" would be true of a namespace that never had
	// one, and this test would pass while exercising nothing.
	var v6only string
	for _, ifi := range all {
		if ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifi.Addrs()
		if len(addrs) > 0 && !HasIPv4(addrs) {
			v6only = ifi.Name
		}
	}
	if v6only == "" {
		t.Fatalf("this namespace has no IPv6-only interface, so the divergence is unexercised: %v",
			describeAll(all))
	}

	in := func(set []net.Interface, name string) bool {
		for _, i := range set {
			if i.Name == name {
				return true
			}
		}
		return false
	}
	if in(chooseInterfaces(all, true), v6only) {
		t.Errorf("%s has no IPv4 address and was chosen for the IPv4 group. On Windows that "+
			"join is resolved by ADDRESS and would be refused, so Linux and Windows would "+
			"pick different interfaces — the divergence this requirement exists to remove",
			v6only)
	}
	if !in(chooseInterfaces(all, false), v6only) {
		t.Errorf("%s was skipped for the IPv6 group too. IPv6 joins are index-based on every "+
			"platform; excluding it there costs an interface for no reason", v6only)
	}
	t.Logf("IPV6ONLY %s excluded from the v4 group, kept for v6", v6only)
}

// describeAll is describe() over an empty chosen set, for a failure message.
func describeAll(all []net.Interface) string { return describe(all, nil) }
