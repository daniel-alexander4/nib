package rendezvous

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"nib/internal/addrscope"
	"nib/internal/udpmux"
)

// nodeWithCache opens a Server whose node cache holds exactly `cached`.
func nodeWithCache(t *testing.T, cached []krpc.NodeInfo) *node {
	t.Helper()
	dir := t.TempDir()
	if len(cached) > 0 {
		if _, err := writeNodes(dir, cached); err != nil {
			t.Fatal(err)
		}
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := udpmux.New(pc)
	rz, err := Open(m.DHT(), dir)
	if err != nil {
		m.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { rz.Close(); m.Close() })
	return &node{mux: m, rz: rz, dir: dir, addr: m.LocalAddr().(*net.UDPAddr)}
}

func infoOf(p *fakeNode) krpc.NodeInfo {
	var ni krpc.NodeInfo
	ni.ID = p.id
	ni.Addr = krpc.NodeAddr{IP: p.addr.IP, Port: p.addr.Port}
	return ni
}

// deadCache is a cached node at an address nothing answers on.
func deadCache(t *testing.T) []krpc.NodeInfo {
	t.Helper()
	var ni krpc.NodeInfo
	for i := range ni.ID {
		ni.ID[i] = byte(i + 9)
	}
	ni.Addr = krpc.NodeAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
	return []krpc.NodeInfo{ni}
}

// TestInvitationSeedsAreUsedWhenTHECACHEISWARMANDDEAD.
//
// **This is the fork the plan's original wording hid.** `Open`'s branch is
// `if len(nodes) == 0` — it asks whether the cache FILE is empty, not whether it works. A
// machine carrying stale cached nodes never reaches the shipped seeds at all, which is
// precisely the machine invitation seeds exist for: it has spoken to the DHT before, its
// cache is worthless now, and nothing says why.
//
// So the trigger is a bootstrap that produced nothing, not an absent file. Driven here with
// a cache pointing at a port nothing listens on.
func TestInvitationSeedsAreUsedWhenTheCacheIsWarmAndDead(t *testing.T) {
	_, live := newStorer(t, "127.0.0.20")
	n := nodeWithCache(t, deadCache(t))

	// STIMULUS: the cache is non-empty, so the SHIPPED list was never consulted — without
	// this the test could pass by having gone to the real internet.
	if got := n.rz.Stats().Seeds; got != 0 {
		t.Fatalf("Seeds = %d — the shipped list was consulted, so this is not the "+
			"warm-cache case at all", got)
	}
	if n.rz.Stats().Loaded == 0 {
		t.Fatal("the cache did not load; the fork under test is not being driven")
	}

	n.rz.Seed([]netip.AddrPort{netip.MustParseAddrPort(live.addr.String())})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := n.rz.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	st := n.rz.Stats()
	if !st.InvitationSeedsUsed {
		t.Fatal("the cache was warm and dead and the invitation's seeds were never tried — " +
			"which is the machine they exist for")
	}
	if st.Nodes == 0 {
		t.Error("the invitation seeds were used and the table is still empty")
	}
	if st.Seeds != 0 {
		t.Errorf("Seeds = %d — the shipped list must stay untouched, and its rot alarm "+
			"must not be about somebody else's addresses", st.Seeds)
	}
	if st.InvitationSeeds != 1 {
		t.Errorf("InvitationSeeds = %d, want 1", st.InvitationSeeds)
	}
}

// TestInvitationSeedsAreNotUsedWhenTheCacheWorks — the other direction, and the one that
// bounds the exposure: seeds an attacker may have chosen are a LAST resort, not a per-run
// contact.
func TestInvitationSeedsAreNotUsedWhenTheCacheWorks(t *testing.T) {
	_, good := newStorer(t, "127.0.0.21")
	_, hostile := newStorer(t, "127.0.0.22")
	n := nodeWithCache(t, []krpc.NodeInfo{infoOf(good)})

	n.rz.Seed([]netip.AddrPort{netip.MustParseAddrPort(hostile.addr.String())})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := n.rz.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	st := n.rz.Stats()
	// STIMULUS: the working cache really did produce a table, so "seeds unused" means
	// "not needed" rather than "nothing happened at all".
	if st.Nodes == 0 {
		t.Fatal("the working cache produced no table; this test cannot distinguish " +
			"'seeds not needed' from 'bootstrap did nothing'")
	}
	if st.InvitationSeedsUsed {
		t.Error("the cache bootstrapped fine and the invitation's addresses were contacted " +
			"anyway — that is a read receipt for the sender on every run, and a standing " +
			"chance to eclipse a table that did not need them")
	}
	if st.InvitationSeeds != 1 {
		t.Errorf("InvitationSeeds = %d, want 1 — supplied is a different fact from used", st.InvitationSeeds)
	}
}

// TestSeedSampleFiltersAndDoesNotWatermark.
//
// The first version of this was vacuous twice over, and a mutation proved it: `SeedSample`
// stubbed to `return nil` kept it green, because `newNode` never bootstraps, so the table
// was empty, so the loop body never ran. And "NotStable" was in the name and nowhere in the
// body — no two samples were ever compared.
func TestSeedSampleFiltersAndDoesNotWatermark(t *testing.T) {
	mk := func(ip string, port int, idb byte) krpc.NodeInfo {
		var ni krpc.NodeInfo
		for i := range ni.ID {
			ni.ID[i] = idb
		}
		ni.Addr = krpc.NodeAddr{IP: net.ParseIP(ip), Port: port}
		return ni
	}
	self := netip.MustParseAddrPort("93.184.216.34:41000")
	nodes := []krpc.NodeInfo{
		mk("127.0.0.1", 6881, 1),       // loopback
		mk("192.168.1.9", 6881, 2),     // private
		mk("100.64.0.7", 6881, 3),      // CGNAT
		mk("224.0.0.1", 6881, 4),       // multicast
		mk("8.8.8.8", 53, 5),           // routable, reflection port
		mk("1.1.1.1", 443, 6),          // routable, TCP-fallback port — Target allows, Seed must not
		mk("93.184.216.34", 6881, 7),   // OUR OWN address on another port
		mk("212.129.33.59", 6881, 8),   // good
		mk("87.98.162.88", 6881, 9),    // good
		mk("67.215.246.10", 6881, 10),  // good
		mk("82.221.103.244", 6881, 11), // good
	}

	got := sampleSeeds(nodes, self, 8)
	if len(got) != 4 {
		t.Fatalf("sampled %v — want exactly the four routable high-port strangers", got)
	}
	for _, ap := range got {
		if !addrscope.Seed(ap) {
			t.Errorf("sampled %v, which Nib would itself refuse as a seed", ap)
		}
		if ap.Addr() == self.Addr() {
			t.Errorf("sampled our OWN address %v — that ships the issuer's public IP to "+
				"every recipient and every mail server between", ap)
		}
	}

	// Not a watermark: two samples of the same over-n table must not be the same sequence.
	// A stable set links two invitations to one issuer.
	var same int
	const rounds = 12
	first := sampleSeeds(nodes, self, 2)
	for i := 0; i < rounds; i++ {
		next := sampleSeeds(nodes, self, 2)
		if len(next) != 2 {
			t.Fatalf("sample %d returned %v", i, next)
		}
		if next[0] == first[0] && next[1] == first[1] {
			same++
		}
	}
	if same == rounds {
		t.Errorf("%d samples of the same table were identical — the seed set is a stable "+
			"per-issuer watermark", rounds)
	}

	if got := sampleSeeds(nodes, self, 0); got != nil {
		t.Errorf("sampleSeeds(_, _, 0) = %v, want nil", got)
	}
	if got := sampleSeeds(nil, self, 4); got != nil {
		t.Errorf("an empty table yielded %v", got)
	}
}

// TestSeedsTriedButUselessIsNotReportedAsUsed.
//
// TRIED and USED are different facts, and conflating them made the eclipse disclosure lie.
// The retry runs on the caller's context, which the first attempt may already have spent —
// measured: `err=context deadline exceeded, InvitationSeedsUsed=true, Nodes=0`. The note an
// operator reads would then say "this routing table came from a list the invitation's
// sender chose" about a table that came from nothing at all.
func TestSeedsTriedButUselessIsNotReportedAsUsed(t *testing.T) {
	n := nodeWithCache(t, deadCache(t))
	// Seeds that are themselves dead: the retry runs and still yields nothing.
	n.rz.Seed([]netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:2")})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = n.rz.Bootstrap(ctx)

	st := n.rz.Stats()
	// STIMULUS: the retry really did RUN.
	//
	// InvitationSeeds alone proves only that Seed() was called — with just that check,
	// deleting the whole retry block from Bootstrap left this test green: seeds 1, used
	// false, nodes 0, all as asserted. The flag that says the retry happened is the one
	// that has to be checked, and it is the flag this test is named for.
	if st.InvitationSeeds != 1 {
		t.Fatalf("InvitationSeeds = %d — the seeds were never supplied", st.InvitationSeeds)
	}
	if !st.InvitationSeedsTried {
		t.Fatal("the retry never ran, so 'tried but useless' is not what this measured — " +
			"delete Bootstrap's retry block and the rest of this test still passes")
	}
	if st.Nodes != 0 {
		// Not a Skip: a fixture pointing at 127.0.0.1:2 that produces a routing table
		// means the fixture is broken, and skipping hides that.
		t.Fatalf("the dead seeds produced %d node(s) — the fixture is not what this test "+
			"assumes", st.Nodes)
	}
	if st.InvitationSeedsUsed {
		t.Error("the seeds were tried and produced nothing, and the disclosure says the " +
			"table came from them — a confident false statement about where this " +
			"machine's view of the DHT originated")
	}
}

// F8 — the concurrent-Bootstrap hazard, REFUTED by trying to prove it.
//
// The S06 review argued two concurrent Bootstrap calls could interleave across the
// read-modify-write of invSeedsTried and burn the one-shot seed retry without taking it.
// A serialising lock was added and then removed, because neither half survived measurement:
//
//   - Removing the added lock left a four-goroutine test PASSING. The one shot is already
//     decided under `s.mu`, so only one caller can ever set `tried`, and the loser skipping
//     its retry is the invariant working rather than a burn.
//   - Removing `s.mu` from that same read-modify-write ALSO left it passing, under -race.
//     Each Bootstrap spends seconds in its traversal, so four callers never overlap at the
//     critical section at all — the test had no reach over the lock that does the work.
//
// Recorded here rather than as a lock plus a doc comment claiming to enforce something
// `s.mu` already enforces. `TestSeedsTriedButUselessIsNotReportedAsUsed` above is the guard
// that does have reach, and it is over the fact that actually went wrong: TRIED vs USED.

// TestTheShippedListsRotAlarmSurvivesAnInvitationRescue.
//
// `dht.go`'s own doc: *"Zero while Seeds is non-zero is the rot alarm"* — the signal that
// every address Nib ships is dead. Both bootstrap attempts added to the same
// `bootstrapped` counter, so on the machine invitation seeds exist for (shipped list dead,
// seeds rescue it) `Stats()` reported `Seeds: 5, Bootstrapped: 25` and the alarm read "the
// shipped list worked" on a run where every shipped address had failed.
//
// The plan defends the `Seeds` term of that comparison by name. The confounding landed on
// the other term.
func TestTheShippedListsRotAlarmSurvivesAnInvitationRescue(t *testing.T) {
	n := nodeWithCache(t, deadCache(t))
	// A seed that answers, standing in for the invitation's list. The shipped list is
	// unreachable from this test's namespace, so attempt one gains nothing — which is the
	// machine under test.
	fake := newFakeNode(t, "127.0.0.14", reports("93.184.216.34", 4000))
	ap, ok := netip.AddrFromSlice(fake.addr.IP)
	if !ok {
		t.Fatal("the fake node's address is not an IP")
	}
	n.rz.Seed([]netip.AddrPort{netip.AddrPortFrom(ap.Unmap(), uint16(fake.addr.Port))})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = n.rz.Bootstrap(ctx)

	st := n.rz.Stats()
	// STIMULUS: the retry ran and the seeds actually produced a table. Without both, the
	// assertion below is about a run where nothing happened.
	if !st.InvitationSeedsTried {
		t.Skip("the retry did not run here; nothing to attribute")
	}
	if st.Nodes == 0 {
		t.Skip("the invitation seed produced no table; nothing to attribute")
	}

	if st.Bootstrapped != 0 {
		t.Errorf("Bootstrapped = %d after a run where only the INVITATION's seeds worked "+
			"— the shipped list's rot alarm (Bootstrapped==0 while Seeds>0) cannot fire "+
			"on the one machine it exists for", st.Bootstrapped)
	}
	if st.InvitationBootstrapped == 0 {
		t.Error("InvitationBootstrapped = 0 — the nodes the seeds gained are attributed to " +
			"nobody, which is the opposite mistake")
	}
}
