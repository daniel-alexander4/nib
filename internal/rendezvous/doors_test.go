package rendezvous

import (
	"net"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

// P04.S03's doors: the third one the library dies through, and the policy that stops
// Nib being a store for strangers.

// send delivers one raw datagram to a node's socket and returns once it is written.
func send(t *testing.T, to *net.UDPAddr, b []byte) {
	t.Helper()
	c, err := net.DialUDP("udp", nil, to)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write(b); err != nil {
		t.Fatal(err)
	}
}

// waitFor polls until want() or the budget expires, so a test never grades a counter
// before the read loop has had a chance to move it.
func waitFor(t *testing.T, what string, want func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if want() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// bep44Response builds a `get` reply carrying a public key. seq==nil is the kill.
func bep44Response(withSeq bool) []byte {
	var r krpc.Return
	for i := range r.K {
		r.K[i] = byte(i + 1)
	}
	if withSeq {
		n := int64(7)
		r.Seq = &n
	}
	m := krpc.Msg{T: "zz", Y: krpc.YResponse, R: &r}
	b, err := bencode.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

// TestAResponseCarryingAKeyWithNoSeqCannotKillTheProcess — DOOR THREE.
//
// exts/getput.Get reads `*r.Seq` at getput.go:55 inside a `&&`, after the target
// matches and with nothing between the operands checking it. `Seq` is `*int64,
// omitempty` (krpc/msg.go:85). The dereference runs on a goroutine started bare at
// traversal/operation.go:236, and the library's only recover is krpc/error.go:50, which
// is unrelated — so this is the same unrecoverable shape as the first two doors.
//
// The two assertions before the grade are the point of the test. Neither existing door
// covers this, and a version of this file that only counted the drop would not show it.
func TestAResponseCarryingAKeyWithNoSeqCannotKillTheProcess(t *testing.T) {
	kill := bep44Response(false)

	// STIMULUS 1: door one passes it. The datagram decodes perfectly — which is this
	// package's own recorded lesson, that screening the decoder is not screening the
	// library. If this ever fails, door one grew to cover door three and the drop below
	// would be counting something else.
	if !survivesDecode(kill) {
		t.Fatal("the kill datagram does not survive decode — door one now catches it, " +
			"so this test is no longer driving door three")
	}
	// STIMULUS 2: door two cannot see it either. gateQuery is OnQuery; this is a
	// response and carries R, not A.
	var m krpc.Msg
	if err := bencode.Unmarshal(kill, &m); err != nil {
		t.Fatal(err)
	}
	if m.A != nil {
		t.Fatal("the kill datagram carries an arguments dict, so gateQuery would see it")
	}
	if !killsGetHandler(m) {
		t.Fatal("killsGetHandler does not recognise its own stimulus")
	}

	n := newNode(t)
	before := n.rz.Stats().RefusedResponses
	send(t, n.addr, kill)
	waitFor(t, "the response to be refused", func() bool {
		return n.rz.Stats().RefusedResponses == before+1
	})

	// And the process is alive to say so — the whole point.
	if got := n.rz.Stats().Nodes; got < 0 {
		t.Fatalf("impossible node count %d", got)
	}
}

// TestAResponseCARRYINGSeqIsNotRefused is the other half, and it is what stops the drop
// above from being "refuse every response".
//
// A rule that dropped all BEP-44 replies would pass the test above and silently break
// every fetch this slice exists to perform.
func TestAResponseCarryingSeqIsNotRefused(t *testing.T) {
	ok := bep44Response(true)
	var m krpc.Msg
	if err := bencode.Unmarshal(ok, &m); err != nil {
		t.Fatal(err)
	}
	if killsGetHandler(m) {
		t.Fatal("a response carrying seq was judged a kill — the fetch path is now dead")
	}

	n := newNode(t)
	before := n.rz.Stats().RefusedResponses
	send(t, n.addr, ok)

	// ARRIVAL EVIDENCE, and without it this half of the test is vacuous.
	//
	// A negative assertion on a counter cannot tell "delivered, correctly not refused"
	// from "never delivered at all" — measured, every field of Stats was identical before
	// and after, so this test passed with the send() deleted. Stats().Responses exists to
	// close that: it counts replies that reached the library, so a non-zero here says the
	// datagram arrived AND passed the screen, which is exactly the claim being made.
	waitFor(t, "the response to be seen by the screen",
		func() bool { return n.rz.Stats().Responses == 1 })

	if got := n.rz.Stats().RefusedResponses; got != before {
		t.Fatalf("a well-formed BEP-44 response was refused (%d -> %d)", before, got)
	}
}

// TestNibDoesNotStoreOtherPeoplesData — the fourth door, which is a policy.
//
// Leaving ServerConfig.Store unset installs bep44.NewMemory() (server.go:219-220), an
// uncapped map with eviction only on a read of that exact target. Refusing the two
// queries that write to it closes an unauthenticated remote memory-growth path at a door
// Nib already owns.
func TestNibDoesNotStoreOtherPeoplesData(t *testing.T) {
	for _, q := range []string{"put", "announce_peer"} {
		t.Run(q, func(t *testing.T) {
			n := newNode(t)
			before := n.rz.Stats().RefusedStores

			// A WELL-FORMED query — it carries its arguments dict, so gateQuery's first
			// clause passes it and only the store policy can refuse it. Without this the
			// test would pass on the nil-args rule and prove nothing about the policy.
			var id krpc.ID
			for i := range id {
				id[i] = byte(i + 1)
			}
			tok := "tok"
			seq := int64(1)
			msg := krpc.Msg{
				T: "zz", Y: krpc.YQuery, Q: q,
				A: &krpc.MsgArgs{ID: id, Token: tok, Seq: &seq},
			}
			b, err := bencode.Marshal(msg)
			if err != nil {
				t.Fatal(err)
			}
			var back krpc.Msg
			if err := bencode.Unmarshal(b, &back); err != nil {
				t.Fatal(err)
			}
			if back.A == nil {
				t.Fatal("stimulus lost its arguments dict — this would be refused by the " +
					"nil-args rule and would say nothing about the store policy")
			}

			send(t, n.addr, b)
			waitFor(t, q+" to be refused", func() bool {
				return n.rz.Stats().RefusedStores == before+1
			})
			// It must be counted as a STORE refusal, not folded into the nil-args one:
			// three doors, three keys.
			if got := n.rz.Stats().RefusedQueries; got != 0 {
				t.Fatalf("a well-formed %s was counted as a malformed query (RefusedQueries=%d)", q, got)
			}
		})
	}
}

// TestAGetQueryIsStillAnswered — the over-blocking guard.
//
// If the store policy were written as "refuse queries" rather than "refuse the two that
// write", Nib would stop participating in the DHT at all and every test above would
// still pass.
func TestAGetQueryIsStillAnswered(t *testing.T) {
	n := newNode(t)
	var id krpc.ID
	for i := range id {
		id[i] = byte(i + 1)
	}
	var target krpc.ID
	msg := krpc.Msg{T: "zz", Y: krpc.YQuery, Q: "get", A: &krpc.MsgArgs{ID: id, Target: target}}
	b, err := bencode.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	c, err := net.DialUDP("udp", nil, n.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write(b); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 2048)
	nn, err := c.Read(buf)
	if err != nil {
		t.Fatalf("no reply to a `get` query — the store policy is refusing reads too: %v", err)
	}
	var reply krpc.Msg
	if err := bencode.Unmarshal(buf[:nn], &reply); err != nil {
		t.Fatalf("reply did not decode: %v", err)
	}
	if reply.Y != krpc.YResponse {
		t.Fatalf("`get` was answered with y=%q, want a response", reply.Y)
	}
	if got := n.rz.Stats().RefusedStores; got != 0 {
		t.Fatalf("a `get` was counted as a store refusal (RefusedStores=%d)", got)
	}
}
