package discovery

import (
	"net"
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
// nothing attached, a WireGuard tun, an interface whose DHCP lease has not landed.
// None of them is reachable by testing on whatever host happens to run the suite.
func TestInterfaceSelectionSkipsWhatItShould(t *testing.T) {
	// Interfaces with no index cannot report addresses, so this table is about the
	// FLAG decisions only; hasV4 is tested directly below.
	all := []net.Interface{
		ifi("lo", 1, up|run|loop),
		ifi("wlp1s0", 2, up|run|bc|mc),
		ifi("docker0", 3, up|bc|mc), // UP but no carrier — the idle-bridge case
		ifi("wg0", 4, up|run|p2p),   // a VPN
		ifi("eth9", 5, 0),           // down
	}

	// Stimulus: the fixture really contains each interesting case, so a selection
	// that returned nothing would not pass by accident.
	if len(all) != 5 {
		t.Fatalf("the table lost a case: %d rows", len(all))
	}

	got := chooseInterfaces(all, false)
	names := map[string]bool{}
	for _, i := range got {
		names[i.Name] = true
	}

	// Every one of these is skipped for a DIFFERENT reason, so they are asserted
	// separately — a single "want [wlp1s0 docker0]" would pass with the loopback and
	// point-to-point rules collapsed into one.
	if names["lo"] {
		t.Error("loopback was joined. IP_MULTICAST_LOOP already delivers a local copy, so " +
			"joining lo duplicates every datagram")
	}
	if names["wg0"] {
		t.Error("a point-to-point interface was joined — a link-local join on a VPN either " +
			"does nothing or leaks who is signing on this LAN into the tunnel")
	}
	if names["eth9"] {
		t.Error("a down interface was joined")
	}
	// docker0 IS selected, and that is deliberate: FlagRunning would exclude it on
	// Linux and carries no information on Windows, where it is set from the same
	// condition as FlagUp. Selecting on it would mean two different things per
	// platform, so it is reported and not filtered.
	if !names["docker0"] {
		t.Error("docker0 was skipped, which means FlagRunning became a filter — it is " +
			"degenerate on Windows and would make the selection platform-dependent")
	}
	if !names["wlp1s0"] {
		t.Fatal("the ordinary wifi interface was not selected; nothing else here matters")
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
