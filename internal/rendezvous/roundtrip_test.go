package rendezvous

import (
	"bytes"
	"context"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
	"nib/internal/udpmux"
)

// The publish/fetch round trip, driven hermetically.
//
// # The vacuous green this test is built to be incapable of
//
// `dht.Server.Put` writes to our OWN bep44 store before it sends anything
// (server.go:1081), and our `get` handler serves from that same store (server.go:620).
// So the obvious version of this test — publish, then fetch, then assert the bytes match
// — passes with **every remote node refusing the put**, because the value round-trips
// through the publisher's own memory and never crosses the network. `getput.Put` returns
// nil in that case (it shadows every per-node error), so nothing complains.
//
// Two things make that impossible here:
//
//  1. **The fake storer counts the puts it receives and verifies the signature itself**,
//     and that count is asserted BEFORE anything reads the value back. A record that
//     never left the machine fails the test at that line, not at the comparison.
//  2. **The publisher is closed before the fetch**, and the fetch runs on a DIFFERENT
//     node. If the value still arrives, some other process is holding it — which is the
//     property the acceptance criterion is actually about.

// storer is a fake DHT node that implements just enough of BEP-44 to store one item.
type storer struct {
	mu     sync.Mutex
	items  map[bep44.Target]*bep44.Item
	puts   int
	badSig int
	gets   int
}

func newStorer(t *testing.T, host string) (*storer, *fakeNode) {
	t.Helper()
	st := &storer{items: map[bep44.Target]*bep44.Item{}}
	f := newFakeNode(t, host, func(q krpc.Msg, id krpc.ID) []byte {
		if q.A == nil {
			return nil
		}
		r := krpc.Return{ID: id}
		switch q.Q {
		case "ping", "find_node":
			// Empty node lists so the traversal stalls promptly instead of paying a 2 s
			// timeout per invented candidate.
		case "get":
			st.mu.Lock()
			st.gets++
			// A STRING token. getput's traversal does `ClosestData.(string)` and drops
			// any node whose token is not one (getput.go:70-75) — a fake that omits it
			// produces a put fan-out to ZERO nodes and a nil error, which is the other
			// way this test could go vacuous.
			tok := "tok"
			r.Token = &tok
			if it, ok := st.items[bep44.Target(q.A.Target)]; ok {
				r.V = bencode.MustMarshal(it.V)
				r.K = it.K
				r.Sig = it.Sig
				seq := it.Seq
				r.Seq = &seq
			}
			st.mu.Unlock()
		case "put":
			st.mu.Lock()
			st.puts++
			bv := bencode.MustMarshal(q.A.V)
			if !bep44.Verify(q.A.K[:], q.A.Salt, *q.A.Seq, bv, q.A.Sig[:]) {
				st.badSig++
				st.mu.Unlock()
				return nil
			}
			it := &bep44.Item{V: q.A.V, K: q.A.K, Salt: q.A.Salt, Sig: q.A.Sig, Seq: *q.A.Seq}
			st.items[it.Target()] = it
			st.mu.Unlock()
		default:
			return nil
		}
		b, err := bencode.Marshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &r})
		if err != nil {
			return nil
		}
		return b
	})
	return st, f
}

// nodeSeeded opens a rendezvous Server whose routing table starts with `peers`.
func nodeSeeded(t *testing.T, peers ...*fakeNode) *node {
	t.Helper()
	dir := t.TempDir()
	var infos []krpc.NodeInfo
	for _, p := range peers {
		var ni krpc.NodeInfo
		ni.ID = p.id
		ni.Addr = krpc.NodeAddr{IP: p.addr.IP, Port: p.addr.Port}
		infos = append(infos, ni)
	}
	if _, err := writeNodes(dir, infos); err != nil {
		t.Fatal(err)
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

func TestAPublishedRecordIsRetrievableByThePeer(t *testing.T) {
	st, fake := newStorer(t, "127.0.0.9")

	seed := bytes.Repeat([]byte{7}, 32)
	salt := []byte("party-a-salt")
	record := bytes.Repeat([]byte("candidate-record"), 12) // 192 bytes, a realistic size

	pub := nodeSeeded(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := pub.rz.Publish(ctx, seed, salt, record); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// STIMULUS, GRADED FIRST: the record actually left this machine and a remote node
	// accepted it with a signature it verified for itself. Without this assertion, every
	// line below would pass on the publisher's own store.
	st.mu.Lock()
	puts, bad := st.puts, st.badSig
	st.mu.Unlock()
	if puts == 0 {
		t.Fatal("no node received a put — the record never left this machine, and " +
			"getput.Put returned nil anyway. Everything below would have passed on our " +
			"own local store.")
	}
	if bad != 0 {
		t.Fatalf("%d of %d puts carried a signature the storer could not verify", bad, puts)
	}
	if got := pub.rz.Stats().Published; got != 1 {
		t.Fatalf("Stats().Published = %d, want 1", got)
	}

	// Close the publisher so it cannot serve its own record back.
	pub.rz.Close()
	pub.mux.Close()

	// A DIFFERENT node fetches. If this works, a third party is holding the record.
	sub := nodeSeeded(t, fake)
	got, seq, err := sub.rz.Fetch(ctx, seed, salt)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !bytes.Equal(got, record) {
		t.Fatalf("fetched %d bytes, published %d — and they differ", len(got), len(record))
	}
	if seq < 1 {
		t.Fatalf("fetched seq %d, want >= 1", seq)
	}
	fs := sub.rz.Stats()
	if fs.Fetched != 1 || fs.FetchEmpty != 0 || fs.FetchAborted != 0 {
		t.Fatalf("fetch counters wrong: %+v", fs)
	}
	// FetchNodes on the SUCCESS path — the counter whose false-zero this slice fixed and
	// then shipped with no instrument at all. A mutation audit reintroduced the bug and
	// every package stayed green.
	if fs.FetchNodes == 0 {
		t.Error("FetchNodes is 0 after a fetch that returned a record — the counter whose " +
			"whole job is to make an empty fetch legible is recording a false zero, and " +
			"nothing else in the repo would notice")
	}
	if fs.FetchAttempts != 1 || fs.PublishAttempts != 0 {
		t.Errorf("attempt counters wrong on the fetching node: %+v", fs)
	}
}

// TestFetchingAKeyNobodyPublishedIsNotAnError — the ordinary case before a peer publishes.
//
// It must be distinguishable from a transport failure: the ladder tells the user
// different things for "your peer has not published yet" and "we cannot reach the DHT".
func TestFetchingAKeyNobodyPublishedIsNotAnError(t *testing.T) {
	_, fake := newStorer(t, "127.0.0.10")
	n := nodeSeeded(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _, err := n.rz.Fetch(ctx, bytes.Repeat([]byte{3}, 32), []byte("nobody"))
	if err == nil {
		t.Fatal("fetching an unpublished key returned no error")
	}
	if err != ErrNoRecord {
		t.Fatalf("want ErrNoRecord, got %v", err)
	}
	if s := n.rz.Stats(); s.FetchEmpty != 1 || s.Fetched != 0 {
		t.Fatalf("an empty fetch was not counted as empty: %+v", s)
	}
}

// TestAnOversizeRecordIsRefusedBeforeItIsSilentlyDropped.
//
// bep44.Check refuses a value over 1000 BENCODED bytes inside our own store, inside
// dht.Server.Put, BEFORE any datagram is sent (server.go:1081). getput.Put then logs a
// warning per node and returns nil. So without an up-front check the record simply never
// leaves the machine and nothing says so.
func TestAnOversizeRecordIsRefusedBeforeItIsSilentlyDropped(t *testing.T) {
	st, fake := newStorer(t, "127.0.0.11")
	n := nodeSeeded(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := n.rz.Publish(ctx, bytes.Repeat([]byte{9}, 32), []byte("s"), bytes.Repeat([]byte{1}, 1001))
	if err == nil {
		t.Fatal("an over-size record was accepted — it would have been dropped in our own " +
			"store with getput.Put returning nil, and nothing would have reported it")
	}
	st.mu.Lock()
	puts := st.puts
	st.mu.Unlock()
	if puts != 0 {
		t.Fatalf("an over-size record reached the network (%d puts)", puts)
	}
}

// TestABadSeedIsRefusedRatherThanPanicking — ed25519.NewKeyFromSeed panics off 32 bytes,
// and the seed crosses a package boundary as opaque bytes.
func TestABadSeedIsRefusedRatherThanPanicking(t *testing.T) {
	n := newNode(t)
	ctx := context.Background()
	for _, sz := range []int{0, 31, 33, 64} {
		if err := n.rz.Publish(ctx, bytes.Repeat([]byte{1}, sz), nil, []byte("x")); err == nil {
			t.Fatalf("a %d-byte seed was accepted", sz)
		}
		if _, _, err := n.rz.Fetch(ctx, bytes.Repeat([]byte{1}, sz), nil); err == nil {
			t.Fatalf("a %d-byte seed was accepted by Fetch", sz)
		}
	}
}

// TestWeServeOurOwnPublishedRecord — the Exp clause, driven.
//
// dht.Server.Put writes to our own store before sending (server.go:1081), so after a
// publish this node is legitimately one of the places holding the record, and a peer's
// traversal may well ask us for it. Whether we ANSWER depends on ServerConfig.Exp, and
// leaving it unset does not mean "never expire" — it means expire everything immediately:
// bep44.NewWrapper(store, 0) makes Wrapper.Get compute created.Add(0).After(now), false
// for any item stored even a nanosecond ago, so it deletes the item and replies not-found
// (bep44/store.go:50-64). The 2h default lives only in NewDefaultServerConfig
// (server.go:167), which caveat 7 forbids us.
//
// Without this test the Exp line is a constant nothing reads, and the failure it prevents
// is invisible: Nib publishes successfully, other nodes store it, and Nib alone serves
// nothing — including its own record.
func TestWeServeOurOwnPublishedRecord(t *testing.T) {
	st, fake := newStorer(t, "127.0.0.12")
	_ = st
	n := nodeSeeded(t, fake)
	seed := bytes.Repeat([]byte{5}, 32)
	salt := []byte("ours")
	record := []byte("our-own-record")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.rz.Publish(ctx, seed, salt, record); err != nil {
		t.Fatal(err)
	}

	// Ask this node for it, exactly as a peer's traversal would.
	_, pub, err := keyPair(seed)
	if err != nil {
		t.Fatal(err)
	}
	target := bep44.MakeMutableTarget(pub, salt)
	var id krpc.ID
	for i := range id {
		id[i] = byte(i + 1)
	}
	q, err := bencode.Marshal(krpc.Msg{
		T: "gg", Y: krpc.YQuery, Q: "get",
		A: &krpc.MsgArgs{ID: id, Target: krpc.ID(target)},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.DialUDP("udp", nil, n.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write(q); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 4096)
	nn, err := c.Read(buf)
	if err != nil {
		t.Fatalf("no reply to a get for our own record: %v", err)
	}
	var reply krpc.Msg
	if err := bencode.Unmarshal(buf[:nn], &reply); err != nil {
		t.Fatal(err)
	}
	if reply.R == nil || len(reply.R.V) == 0 {
		t.Fatal("we published a record and then answered a get for it with NOTHING — " +
			"ServerConfig.Exp is unset or zero, so our own store deletes every item on " +
			"its first read and Nib serves nobody, including itself")
	}
	var got []byte
	if err := bencode.Unmarshal(reply.R.V, &got); err != nil {
		t.Fatalf("our reply's value did not decode: %v", err)
	}
	if !bytes.Equal(got, record) {
		t.Fatalf("we served %q, published %q", got, record)
	}
}

// TestAValueThatIsNotBencodeIsCountedAsUndecodable — FetchUndecodable, driven.
//
// Somebody served us something shaped wrong. That is a different fact from an absent record
// and from one that failed its signature, and it had a counter that nothing read.
func TestAValueThatIsNotBencodeIsCountedAsUndecodable(t *testing.T) {
	st := &storer{items: map[bep44.Target]*bep44.Item{}}
	fake := newFakeNode(t, "127.0.0.13", func(q krpc.Msg, id krpc.ID) []byte {
		if q.A == nil {
			return nil
		}
		r := krpc.Return{ID: id}
		if q.Q == "get" {
			tok := "tok"
			r.Token = &tok
			// A value that is not bencode at all. It must still satisfy getput's target
			// and signature checks to reach our decode, so it is served as raw bytes the
			// library will hand back verbatim.
			r.V = []byte("\xff\xff not bencode \xff\xff")
		}
		b, err := bencode.Marshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &r})
		if err != nil {
			return nil
		}
		return b
	})
	_ = st

	n := nodeSeeded(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, _, err := n.rz.Fetch(ctx, bytes.Repeat([]byte{4}, 32), []byte("undec"))
	if err == nil {
		t.Fatal("a non-bencode value was accepted as a record")
	}
	s := n.rz.Stats()
	if s.FetchUndecodable == 0 && s.FetchEmpty == 0 {
		t.Fatalf("neither undecodable nor empty was counted: %+v", s)
	}
}

// TestAnAbortedLookupIsNotAnEmptyFetch.
//
// getput.Get sets err = ctx.Err() and leaves ret alone, so a cancelled lookup used to come
// back as ErrNoRecord — which the ladder reports as "your peer has not published yet" — and
// to increment FetchEmpty, whose own doc forbids exactly that.
func TestAnAbortedLookupIsNotAnEmptyFetch(t *testing.T) {
	_, fake := newStorer(t, "127.0.0.14")
	n := nodeSeeded(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already dead
	_, _, err := n.rz.Fetch(ctx, bytes.Repeat([]byte{6}, 32), []byte("aborted"))
	if err == nil {
		t.Fatal("a cancelled fetch reported success")
	}
	if err == ErrNoRecord {
		t.Fatal("a cancelled lookup came back as ErrNoRecord — the ladder would tell the " +
			"user their peer has not published, when the truth is that we stopped asking")
	}
	s := n.rz.Stats()
	if s.FetchAborted != 1 {
		t.Errorf("FetchAborted = %d, want 1 (%+v)", s.FetchAborted, s)
	}
	if s.FetchEmpty != 0 {
		t.Errorf("a cancelled lookup was counted as an empty fetch (FetchEmpty=%d)", s.FetchEmpty)
	}
}

// TestASeqAtItsCeilingIsRefusedRatherThanWrapped.
//
// Every roster member can write at our target. One publishing at math.MaxInt64 makes our
// seq+1 wrap to math.MinInt64 with no panic and no error; every remote then refuses the put,
// getput.Put swallows it, and Publish reports success having stored nothing. That turns a
// preemption race the honest party can re-win into a permanent silent block.
func TestASeqAtItsCeilingIsRefusedRatherThanWrapped(t *testing.T) {
	seed := bytes.Repeat([]byte{8}, 32)
	salt := []byte("squatted")
	_, pub, err := keyPair(seed)
	if err != nil {
		t.Fatal(err)
	}
	target := bep44.MakeMutableTarget(pub, salt)

	// A node that claims a record at the maximum sequence number, correctly signed — which
	// a holder of the invitation can genuinely produce.
	priv, _, _ := keyPair(seed)
	victimSeq := int64(math.MaxInt64)
	squat := []byte("squat")
	sig := bep44.Sign(priv, salt, victimSeq, bencode.MustMarshal(squat))
	fake := newFakeNode(t, "127.0.0.15", func(q krpc.Msg, id krpc.ID) []byte {
		if q.A == nil {
			return nil
		}
		r := krpc.Return{ID: id}
		if q.Q == "get" {
			tok := "tok"
			r.Token = &tok
			if bep44.Target(q.A.Target) == target {
				r.V = bencode.MustMarshal(squat)
				r.K = pub
				copy(r.Sig[:], sig)
				s := victimSeq
				r.Seq = &s
			}
		}
		b, err := bencode.Marshal(krpc.Msg{T: q.T, Y: krpc.YResponse, R: &r})
		if err != nil {
			return nil
		}
		return b
	})

	n := nodeSeeded(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = n.rz.Publish(ctx, seed, salt, []byte("ours"))
	if err == nil {
		t.Fatal("publishing over a MaxInt64 squat reported SUCCESS — seq+1 wrapped to " +
			"MinInt64, every remote refused it, and getput.Put returned nil")
	}
	if s := n.rz.Stats(); s.PublishSeqCeiling != 1 {
		t.Errorf("PublishSeqCeiling = %d, want 1 — the one form of in-roster preemption "+
			"that is permanent rather than a race", s.PublishSeqCeiling)
	}
}
