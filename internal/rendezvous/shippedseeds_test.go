package rendezvous

import (
	"context"
	"crypto/rand"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/krpc"

	"nib/internal/addrscope"
	"nib/internal/udpmux"
)

// minAnsweringOperators is the floor seeds.go states: below two, one operator supplies every
// node a cold table starts from, and so every source the self-address majority is counted over.
const minAnsweringOperators = 2

// TestTheShippedSeedsSpanTwoAnsweringOperators — /pending 357, ADR-152.
//
// With invitation seeds refused, the shipped list is the only place a cold machine starts. This
// reads the list itself, never the network: each entry is a usable public address, none is listed
// twice, each names who runs it, and the entries recorded as answering over IPv4 come from at least
// two operators. What it cannot see is whether the record is still TRUE — that is
// TestLiveShippedSeedsAnswerAsRecorded, which `./build/dhtlive.sh` runs.
func TestTheShippedSeedsSpanTwoAnsweringOperators(t *testing.T) {
	if err := checkShippedSeeds(shippedSeeds()); err != "" {
		t.Fatal(err)
	}
	// The list the library is handed is this list, entry for entry.
	nodes := seedNodes()
	if len(nodes) != len(shippedSeeds()) {
		t.Fatalf("seedNodes hands the library %d address(es), the table holds %d", len(nodes), len(shippedSeeds()))
	}
	for i, sd := range shippedSeeds() {
		if got := nodes[i].AddrPort(); got != sd.addr {
			t.Errorf("seedNodes[%d] = %v, the table says %v", i, got, sd.addr)
		}
	}

	// The check is not vacuous: each rule refuses a list that breaks it and only it.
	good := shippedSeeds()
	for name, broken := range map[string][]shippedSeed{
		"a duplicate":            append(append([]shippedSeed{}, good...), good[0]),
		"a private address":      append(append([]shippedSeed{}, good...), shippedSeed{netip.MustParseAddrPort("192.168.1.1:6881"), "x", "X", false}),
		"no operator":            append(append([]shippedSeed{}, good...), shippedSeed{netip.MustParseAddrPort("203.0.114.1:6881"), "x", "", false}),
		"one answering operator": oneOperator(good),
		"the second only on IPv6": append(oneOperator(good),
			shippedSeed{netip.MustParseAddrPort("[2001:4860:4860::8888]:6881"), "x", "Another", true}),
	} {
		if checkShippedSeeds(broken) == "" {
			t.Errorf("a list with %s passed the check", name)
		}
	}
}

// oneOperator is the list with every answering entry of all operators but the first removed.
func oneOperator(in []shippedSeed) []shippedSeed {
	var out []shippedSeed
	keep := ""
	for _, sd := range in {
		if sd.answered {
			if keep == "" {
				keep = sd.operator
			}
			if sd.operator != keep {
				continue
			}
		}
		out = append(out, sd)
	}
	return out
}

// checkShippedSeeds returns what is wrong with a seed list, or "".
func checkShippedSeeds(seeds []shippedSeed) string {
	seen := make(map[netip.AddrPort]bool)
	v4 := make(map[string]bool)
	for _, sd := range seeds {
		switch {
		case !sd.addr.IsValid() || sd.addr.Port() == 0:
			return "a shipped seed is not an address and a port: " + sd.host
		case !addrscope.Seed(sd.addr):
			return "shipped seed " + sd.addr.String() + " is not a public address a bootstrap may use"
		case seen[sd.addr]:
			return "shipped seed " + sd.addr.String() + " is listed twice"
		case sd.operator == "" || sd.host == "":
			return "shipped seed " + sd.addr.String() + " does not say who runs it or what name it was taken from"
		}
		seen[sd.addr] = true
		if sd.answered && sd.addr.Addr().Is4() {
			v4[sd.operator] = true
		}
	}
	if len(v4) < minAnsweringOperators {
		return "fewer than two operators are recorded as answering over IPv4 — one operator would " +
			"choose every node a cold machine starts from (seeds.go, ADR-152)"
	}
	return ""
}

// TestLiveShippedSeedsAnswerAsRecorded asks every shipped seed, one at a time, what Nib asks it
// at a cold start, and prints one SEED line each — /pending 357.
//
// It is how the list's rot is SEEN. The list is consulted only on an empty cache and a cold start
// succeeds while one address answers, so three dead entries of five once looked the same as none
// until somebody measured by hand. It fails on nothing a stranger controls: a seed that stopped
// answering is a line that says ROT, and the harness repeats it; only a record nobody can act on
// — every seed silent, which is no network — skips.
func TestLiveShippedSeedsAnswerAsRecorded(t *testing.T) {
	if os.Getenv("NIB_LIVE_DHT") == "" {
		t.Skip("set NIB_LIVE_DHT=1 (or run ./build/dhtlive.sh) — this test uses the public network")
	}
	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	m := udpmux.New(pc) // owns pc
	defer m.Close()
	// No shipped list and no cache: nothing is traversed, each seed is asked directly.
	s, err := openWith(m.DHT(), t.TempDir(), addrscope.Seed, noShippedSeeds)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	answering := make(map[string]bool)
	total, changed := 0, 0
	for _, sd := range shippedSeeds() {
		got, nodes := 0, 0
		for range 3 {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			var target krpc.ID
			if _, err := rand.Read(target[:]); err != nil {
				t.Fatal(err)
			}
			res := s.dht.Query(ctx, dht.NewAddr(net.UDPAddrFromAddrPort(sd.addr)), "find_node",
				dht.QueryInput{MsgArgs: krpc.MsgArgs{Target: target, Want: []krpc.Want{krpc.WantNodes, krpc.WantNodes6}}})
			cancel()
			if res.ToError() == nil && res.Reply.R != nil {
				got++
				nodes = max(nodes, len(res.Reply.R.Nodes)+len(res.Reply.R.Nodes6))
			}
		}
		verdict := "as recorded"
		answers := got > 0 && nodes > 0
		switch {
		case sd.answered && !answers:
			verdict, changed = "ROT — recorded as answering; re-measure and update seeds.go", changed+1
		case !sd.answered && answers:
			verdict, changed = "BACK — recorded as silent; update seeds.go", changed+1
		}
		if answers {
			answering[sd.operator] = true
			total++
		}
		t.Logf("SEED %-28s %-15s find_node %d/3, %d node(s) — %s", sd.addr, sd.operator, got, nodes, verdict)
	}
	if total == 0 {
		t.Skipf("UNREACHABLE: none of %d shipped seed(s) answered — no network, or every seed is dead; "+
			"this run cannot tell which", len(shippedSeeds()))
	}
	t.Logf("SEEDS %d of %d answered, from %d operator(s); %d differ from the record in seeds.go",
		total, len(shippedSeeds()), len(answering), changed)
}
