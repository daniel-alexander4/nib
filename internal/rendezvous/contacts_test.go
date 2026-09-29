package rendezvous

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"

	"nib/internal/udpmux"
)

func apOf(a *net.UDPAddr) netip.AddrPort {
	return netip.AddrPortFrom(netip.MustParseAddr(a.IP.String()).Unmap(), uint16(a.Port))
}

// TestAStrangersNodesListCannotAimOurQueriesAtTheLAN is /pending 743 part 1.
//
// anacrolix's TraversalNodeFilter admits any address a response lists that is not port 0 or
// 0/8, and BEP-42 exempts private and loopback addresses from the ID check — so a hostile node
// in the public DHT answering `find_node` with a LAN address had us query it. Here the "public
// DHT" is one loopback node (the Server's rule admits it and nothing else, standing in for
// addrscope.Seed admitting public space), and the address it lists is a second loopback socket
// the rule refuses, standing in for the user's printer. Before the fix the LAN socket received
// the traversal's find_node.
func TestAStrangersNodesListCannotAimOurQueriesAtTheLAN(t *testing.T) {
	var lanHits atomic.Int64
	lan := newFakeNode(t, "127.0.0.3", func(q krpc.Msg, id krpc.ID) []byte {
		lanHits.Add(1)
		return bencode.MustMarshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &krpc.Return{ID: id}})
	})
	lan.id[0] = 0x55 // a different ID from the hostile node's, so the traversal treats it as new

	var hostileHits atomic.Int64
	hostile := newFakeNode(t, "127.0.0.2", func(q krpc.Msg, id krpc.ID) []byte {
		hostileHits.Add(1)
		return bencode.MustMarshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &krpc.Return{
			ID:    id,
			Nodes: krpc.CompactIPv4NodeInfo{infoOf(lan)},
		}})
	})
	public := apOf(hostile.addr)
	onlyPublic := func(ap netip.AddrPort) bool { return ap == public }

	dir := t.TempDir()
	if _, err := writeNodes(dir, []krpc.NodeInfo{infoOf(hostile)}); err != nil {
		t.Fatal(err)
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m := udpmux.New(pc)
	rz, err := open(m.DHT(), dir, onlyPublic)
	if err != nil {
		m.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { rz.Close(); m.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = rz.Bootstrap(ctx)
	time.Sleep(300 * time.Millisecond) // a query the traversal did send has time to land

	// STIMULUS: the traversal really did ask the public node, whose answer names the LAN one.
	if hostileHits.Load() == 0 {
		t.Fatal("setup: the bootstrap never queried the public node, so its nodes list was never read")
	}
	if n := lanHits.Load(); n != 0 {
		t.Errorf("the LAN address a public node listed received %d KRPC quer(ies) — a stranger aimed our "+
			"traversal at an address the node-cache rule refuses", n)
	}
	if rz.Stats().RefusedSends == 0 {
		t.Error("RefusedSends = 0 — the refusal happened nowhere an operator can see it")
	}
}

// TestAQueryToAnOutOfScopeAddressIsRefusedAndAReplyIsNot pins both halves of the door on a
// production Server: our own query to loopback is refused, while our REPLY to a loopback querier
// still goes out (TestOneInboundPingDoesNotReplaceAGoodNodeCache's setup relies on the second).
func TestAQueryToAnOutOfScopeAddressIsRefusedAndAReplyIsNot(t *testing.T) {
	victim := openProduction(t, goodCache(4))
	peer := newNode(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := peer.rz.Ping(ctx, victim.addr); err != nil {
		t.Fatalf("a production Server did not answer a loopback querier: %v — replies must not be held to the rule", err)
	}
	err := victim.rz.Ping(ctx, peer.addr)
	if err == nil {
		t.Fatal("a production Server pinged a loopback address — the outbound door let a query through that addrscope.Seed refuses")
	}
	if !errors.Is(err, errRefusedSend) && victim.rz.Stats().RefusedSends == 0 {
		t.Errorf("the ping failed (%v) but not at the door: RefusedSends = 0", err)
	}
}

// TestANodeThatOnlyQueriedUsIsNotSaved is /pending 743 part 2: the save keeps table nodes that
// ANSWERED a query of ours, and a node that merely queried us is in the table too. With a
// hermetic rule (loopback in scope) the 707 scope filter cannot stop it, so this is the quality
// signal alone. Before the fix the pinger was written to the cache.
func TestANodeThatOnlyQueriedUsIsNotSaved(t *testing.T) {
	victim := newNode(t)
	stranger := newNode(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := stranger.rz.Ping(ctx, victim.addr); err != nil {
		t.Fatalf("setup: ping: %v", err)
	}
	if victim.rz.Stats().Nodes == 0 {
		t.Fatal("setup: the stranger is not in the victim's routing table, so the save has nothing to refuse")
	}
	if err := victim.rz.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := (&Server{dir: victim.dir}).loadNodes()
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, ni := range got {
		if ap, _ := nodeAddrPort(ni); ap == apOf(stranger.addr) {
			t.Errorf("the stranger %v, which only ever queried us, was written to the node cache", ap)
		}
	}

	// And the other side: a node WE queried and that answered is saved, so the rule is not
	// "save nothing".
	if err := stranger.rz.Close(); err != nil {
		t.Fatal(err)
	}
	kept, err := (&Server{dir: stranger.dir}).loadNodes()
	if err != nil {
		t.Fatalf("the pinger saved no cache: %v", err)
	}
	found := false
	for _, ni := range kept {
		if ap, _ := nodeAddrPort(ni); ap == apOf(victim.addr) {
			found = true
		}
	}
	if !found {
		t.Errorf("the node we pinged and that answered was not saved (%d saved)", len(kept))
	}
}

// TestTheSaveKeepsTheAnsweredAndDemotesTheSilent pins cacheSet's quality ordering and the
// contacts record directly (/pending 743 parts 2 and 3).
func TestTheSaveKeepsTheAnsweredAndDemotesTheSilent(t *testing.T) {
	nodes := goodCache(4)
	ap := func(i int) netip.AddrPort { a, _ := nodeAddrPort(nodes[i]); return a }

	c := newContacts()
	c.sent(ap(0))
	c.answered(ap(0)) // table node 0: we asked, it answered
	c.sent(ap(1))     // cached node 1: we asked, silence
	c.answered(ap(3)) // node 3: a "response" we never asked for
	if got := c.state(ap(3)); got != contactUnknown {
		t.Errorf("an unsolicited response marked %v as %v — a stranger could vouch for itself", ap(3), got)
	}

	table := []krpc.NodeInfo{nodes[0], nodes[3]}
	prev := []krpc.NodeInfo{nodes[1], nodes[2]}
	out, fresh := cacheSet(table, prev, func(netip.AddrPort) bool { return true }, c.state)
	if fresh != 1 {
		t.Errorf("fresh = %d, want 1 — only the table node that answered us counts", fresh)
	}
	var order []netip.AddrPort
	for _, ni := range out {
		a, _ := nodeAddrPort(ni)
		order = append(order, a)
	}
	want := []netip.AddrPort{ap(0), ap(2), ap(1)}
	if len(order) != len(want) {
		t.Fatalf("saved %v, want %v (answered, then unknown cached, then silent cached; the table node that never answered is not saved)", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("saved %v, want %v — a silent cached node must go after an unknown one, so the cap drops the dead first", order, want)
			break
		}
	}

	// The record is bounded.
	small := &contacts{queried: make(map[netip.AddrPort]bool), capacity: 2}
	for i := 0; i < 4; i++ {
		small.sent(ap(i))
	}
	if len(small.queried) != 2 {
		t.Errorf("contacts holds %d addresses past a capacity of 2", len(small.queried))
	}
}
