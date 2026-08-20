// Package rendezvous is the DHT half of the connection ladder: the signalling
// channel over which two Nibs exchange the endpoints they will dial (D6).
//
// # It shares the session's socket, and that is the whole reason it exists here
//
// Caveat 7: a NAT mapping — learned or requested — is a function of the INTERNAL
// IP:port, so a self-address probe on a throwaway socket measures a mapping the
// session will never use. The mapped port, the DHT probe and the live session must
// therefore be one socket. `internal/udpmux` is what makes that possible, and
// caveat 7's amendment is explicit that "shares" means *through that demultiplexer*
// rather than by any other arrangement.
//
// So this package never opens a socket. It takes one that already exists.
//
// # What it is not
//
// It is not a peer-to-peer transport and it never decides who is talking to whom.
// L1: nothing learned here may influence which peer is accepted. This package
// therefore does not import the vault, the signing code or the session core, and a
// guard enforces that — the same structural shape `internal/discovery` carries for
// the same law.
package rendezvous

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/krpc"
	"golang.org/x/time/rate"
)

// bootstrapFile is where the cached node list lives.
//
// D6 is explicit that bootstrap uses "a cached node list, populated on first
// contact — not hardcoded bootstrap hostnames". The distinction is not stylistic:
// a hostname is a DNS lookup, a DNS lookup is a third party watching who starts a
// ceremony and when, and on a network that blocks or rewrites DNS it is also the
// single point where the whole ladder fails before it begins.
const bootstrapFile = "dht-nodes"

// Stats is what the rendezvous did. Its failure mode, like discovery's, is silence.
type Stats struct {
	// Nodes is the routing table's size right now.
	Nodes int
	// Loaded is how many nodes came from the cache at startup. **Zero on a first
	// ever run is correct**; zero on a later run means the cache did not survive,
	// which is the difference between "new here" and "broken".
	Loaded int
	// Saved is how many were written back on close.
	Saved uint64
	// CacheRejected is true when a node cache existed and could not be parsed. The
	// run continues as a cold start, so without this field the two are
	// indistinguishable — and they want different advice.
	CacheRejected bool
	// Seeds is how many shipped seed addresses were used because the cache was
	// empty. Non-zero means this was a cold start.
	Seeds int
	// Bootstrapped is how many nodes the bootstrap traversals ADDED to the table — not
	// how many seeds replied, which is a different question this cannot answer, since
	// the traversal learns nodes from nodes. **Zero while Seeds is non-zero is the rot
	// alarm**: nothing reachable came of every address Nib ships, which will eventually
	// be true and is otherwise invisible.
	Bootstrapped uint64

	// Screened is datagrams dropped before the library's decoder saw them, each of
	// which would have panicked the process. **Non-zero is an attack or a badly
	// broken node** — and either way Nib is alive to report it.
	Screened uint64
	// RefusedQueries is inbound queries stopped before the library's HANDLERS saw
	// them — a different door and therefore a different counter, because a datagram
	// that kills the decoder and one that kills `announce_peer` say different things
	// about who is sending it. Same reading: non-zero and still running.
	RefusedQueries uint64

	// Observed is replies that carried a usable `ip` field.
	Observed uint64
	// RejectedLength, RejectedPort and RejectedScope are replies whose `ip` was
	// refused, split by cause rather than summed. A 4-byte `ip` decodes into a
	// PLAUSIBLE port with no error, so a length refusal is silent corruption caught;
	// a scope refusal is somebody pointing us at a victim. Lumping them would make
	// the interesting one unreadable.
	RejectedLength uint64
	RejectedPort   uint64
	RejectedScope  uint64
	// Disagreements is observations that differed from a majority **that formed
	// anyway** — the lying-node case specifically, which is the one with no other
	// signal.
	//
	// Zero under an endpoint-dependent classification is STRUCTURAL, not quiet: there
	// is no winning group to be outside of, and the classification is itself the
	// report. A first version of this comment claimed non-zero meant "either real
	// endpoint-dependence or a liar", which the code never did and which would have
	// had a reader looking for the wrong thing.
	Disagreements uint64
}

// Server is a DHT bound to a socket somebody else owns.
type Server struct {
	dht  *dht.Server
	dir  string
	once sync.Once

	loaded        int
	cacheRejected bool
	seeds         int
	saved         atomic.Uint64
	bootstrapped  atomic.Uint64

	screened       atomic.Uint64
	refusedQueries atomic.Uint64

	observed       atomic.Uint64
	rejectedLength atomic.Uint64
	rejectedPort   atomic.Uint64
	rejectedScope  atomic.Uint64
	disagreements  atomic.Uint64
}

// Open starts a DHT on the supplied connection.
//
// conn is the demultiplexer's DHT view. Ownership does not transfer: closing this
// Server closes the DHT but never the socket, because the session on the other view
// is still using it.
func Open(conn net.PacketConn, dir string) (*Server, error) {
	// Refused explicitly, because the failure is silent otherwise: dht.NewServer
	// opens its OWN socket when Conn is nil (server.go:1046). That DHT would work
	// perfectly — and its self-address probe would measure a NAT mapping belonging
	// to a socket the session never uses, which is the whole of caveat 7. Nothing
	// downstream could tell.
	if conn == nil {
		return nil, errors.New("rendezvous: no connection — the DHT must share the " +
			"session's socket (caveat 7), never open one of its own")
	}
	s := &Server{dir: dir}

	// Nothing reaches the library's decoder unscreened. See screen.go: 21 bytes of
	// UDP from any host kill the process otherwise, on a goroutine nothing here owns.
	conn = &screened{PacketConn: conn, dropped: &s.screened}

	nodes, err := s.loadNodes()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// A corrupt cache is not fatal — it is a first run with extra steps.
		//
		// This comment said exactly that from the day it was written, and the code
		// under it returned an error, so an unreadable `dht-nodes` file stopped Nib
		// from starting at all. The comment was right; the code now agrees with it,
		// and Stats().CacheRejected is how the difference stays visible instead of
		// being swallowed.
		s.cacheRejected = true
		nodes = nil
	}
	s.loaded = len(nodes)

	// A genuinely cold machine has nowhere to start, which is what D6's 2026-08-19
	// amendment exists to fix. Seeds are consulted ONLY when the cache is empty, so a
	// machine that has ever spoken to the DHT never touches them again.
	var seeds []*net.UDPAddr
	if len(nodes) == 0 {
		seeds = seedNodes()
		s.seeds = len(seeds)
	}

	cfg := &dht.ServerConfig{
		Conn:       conn,
		NoSecurity: false,
		// Nib's own rate limit, not the library's package-level one.
		//
		// `dht.DefaultSendLimiter` is a **process-global** `rate.Limiter` shared by
		// every Server (server.go:222-224), and replies go through it with
		// `wait=false` (server.go:691, :813) — so when the burst is gone a reply is
		// not delayed, it is **dropped**, and `reply()` only logs it. A node that
		// stops answering gets dropped from other nodes' routing tables, and nothing
		// here would say why.
		//
		// Found by driving it: the self-address probe's 16-query fan-out plus
		// bootstrap drained the shared burst, and a ping between two of our own
		// servers then timed out with the receiving side's mux showing the query had
		// arrived. Owning the limiter makes the rate a property of Nib rather than of
		// whatever else shares the process.
		//
		// `WaitToReply` stays false on purpose: this is an unauthenticated inbound
		// path, and under a genuine flood dropping is the right backpressure —
		// blocking would park a goroutine per attacker packet.
		SendLimiter: rate.NewLimiter(250, 64),
		// The second door. See gateQuery: a query carrying no arguments dict is a nil
		// dereference inside handleQuery, on a goroutine nothing here can recover.
		OnQuery: s.gateQuery,
		// The cached list, and nothing else.
		//
		// **Read before it was written down, because the obvious belief is wrong.**
		// The hostname bootstrap — GlobalBootstrapAddrs, which resolves
		// router.bittorrent.com and friends — lives in NewDefaultServerConfig, and
		// NewServer applies that whole default config ONLY when it is handed a nil
		// one (server.go:158-168, :1042). For a config we build, NewServer fills in
		// Conn, Logger, Store and SendLimiter and leaves StartingNodes alone, and
		// TraversalStartingNodes treats nil as "no initial nodes" (:1268).
		//
		// So nil here would mean no bootstrap at all rather than hostnames — the
		// opposite of what a first draft of this comment claimed. Setting it is
		// still right, because "the cache, deliberately" and "nothing, by omission"
		// should not look identical in the source.
		StartingNodes: func() ([]dht.Addr, error) {
			out := make([]dht.Addr, 0, len(nodes)+len(seeds))
			for _, n := range nodes {
				out = append(out, dht.NewAddr(n.Addr.UDP()))
			}
			for _, a := range seeds {
				out = append(out, dht.NewAddr(a))
			}
			return out, nil
		},
	}
	d, err := dht.NewServer(cfg)
	if err != nil {
		return nil, fmt.Errorf("start the DHT: %w", err)
	}
	s.dht = d
	return s, nil
}

// Nodes is the routing table, for the cache and for a diagnostic.
func (s *Server) Nodes() []krpc.NodeInfo { return s.dht.Nodes() }

// Stats reports what happened.
func (s *Server) Stats() Stats {
	return Stats{
		Nodes:          s.dht.NumNodes(),
		Loaded:         s.loaded,
		Saved:          s.saved.Load(),
		CacheRejected:  s.cacheRejected,
		Seeds:          s.seeds,
		Bootstrapped:   s.bootstrapped.Load(),
		Screened:       s.screened.Load(),
		RefusedQueries: s.refusedQueries.Load(),
		Observed:       s.observed.Load(),
		RejectedLength: s.rejectedLength.Load(),
		RejectedPort:   s.rejectedPort.Load(),
		RejectedScope:  s.rejectedScope.Load(),
		Disagreements:  s.disagreements.Load(),
	}
}

// Ping confirms a node is live, and populates the table on a first run.
//
// Two things here are deliberate and were both wrong in the first version.
//
// It goes through Query rather than the library's Ping, because `Server.Ping` passes
// `context.TODO()` (server.go:1064) — so this function took a context and silently
// discarded it, and every caller's deadline was decoration.
//
// And it returns `ToError()` rather than `res.Err`, because a node that answers with a
// KRPC *error reply* leaves `Err` nil: the query succeeded, the answer was "no". Read
// the old way, a node refusing us counted as a healthy one.
func (s *Server) Ping(ctx context.Context, addr *net.UDPAddr) error {
	res := s.dht.Query(ctx, dht.NewAddr(addr), "ping", dht.QueryInput{})
	return res.ToError()
}

// Bootstrap turns whatever starting nodes exist — the cache, or the seeds on a cold
// machine — into a usable routing table.
//
// The self-address probe needs this and cannot substitute for it: measured against the
// public DHT on 2026-08-19, the bootstrap routers answer queries but **none of them
// returns the `ip` field** the probe reads. A router-only table yields zero
// observations, so "bootstrapped" and "able to see ourselves" are two states and this
// is the step between them.
func (s *Server) Bootstrap(ctx context.Context) error {
	before := s.dht.NumNodes()
	_, err := s.dht.BootstrapContext(ctx)
	// Read ONCE. Called twice — for the comparison and again for the subtraction — a
	// table that shrank in between yields a negative int, and converting that to uint64
	// makes it roughly eighteen quintillion.
	after := s.dht.NumNodes()
	if after > before {
		s.bootstrapped.Add(uint64(after - before))
	}
	return err
}

// Close saves the node list and stops the DHT. **It does not close the socket** —
// the session shares it.
func (s *Server) Close() error {
	var err error
	s.once.Do(func() {
		err = s.saveNodes()
		s.dht.Close()
	})
	return err
}

// nodeRecord is the on-disk size of one cached node: a 20-byte id, a 16-byte address,
// and a 2-byte port.
//
// # Why 16 bytes and not the 6-byte compact form BEP-5 uses on the wire
//
// It was 26 bytes, mirroring the wire, and that quietly excluded IPv6 entirely: the
// writer skipped every node whose address had no IPv4 form, and the "an empty table
// must not truncate a good cache" rule then meant a v6-only host wrote **no cache at
// all, ever**, and cold-started on every single run. The existing restart test could
// not see it, because it runs on 127.0.0.1.
//
// So the record stores the 16-byte form and an IPv4 node goes in v4-mapped. Fixed-size
// records keep the corruption check ("is it a multiple of the record size") exactly as
// cheap as it was, which a tagged or variable-length encoding would not.
const nodeRecord = 20 + 16 + 2

// cacheMagic prefixes the file so its layout is stated rather than inferred.
//
// Without it the only check available is "is the length a multiple of the record
// size", and record sizes collide: the previous 26-byte layout and this 38-byte one
// share a multiple at 494 bytes, so a cache holding exactly **19 old records parses
// cleanly as 13 new ones** — thirteen node addresses assembled from the wrong offsets.
// That is worse than a refusal in a way that matters: a non-empty node list means the
// seed addresses are NOT consulted, so the run bootstraps from thirteen fictions and a
// cold start it could have recovered from becomes one it cannot.
//
// Eight bytes, and any future layout change becomes a version bump rather than an
// arithmetic coincidence away from silent corruption.
const cacheMagic = "NIBdht01"

// loadNodes reads the cache.
func (s *Server) loadNodes() ([]krpc.NodeInfo, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, bootstrapFile))
	if err != nil {
		return nil, err
	}
	if len(b) < len(cacheMagic) || string(b[:len(cacheMagic)]) != cacheMagic {
		return nil, fmt.Errorf("cached node list does not begin with %q — it was written by "+
			"a different version of Nib, or it is not ours", cacheMagic)
	}
	b = b[len(cacheMagic):]
	if len(b)%nodeRecord != 0 {
		return nil, fmt.Errorf("cached node list is %d bytes of records, not a multiple of %d",
			len(b), nodeRecord)
	}
	out := make([]krpc.NodeInfo, 0, len(b)/nodeRecord)
	for i := 0; i+nodeRecord <= len(b); i += nodeRecord {
		var ni krpc.NodeInfo
		copy(ni.ID[:], b[i:i+20])
		ni.Addr.IP = net.IP(append([]byte(nil), b[i+20:i+36]...))
		ni.Addr.Port = int(binary.BigEndian.Uint16(b[i+36 : i+38]))
		out = append(out, ni)
	}
	return out, nil
}

// saveNodes writes the routing table back.
func (s *Server) saveNodes() error {
	n, err := writeNodes(s.dir, s.dht.Nodes())
	if err != nil {
		return err
	}
	s.saved.Store(n)
	return nil
}

// writeNodes is the write, split from the query that feeds it.
//
// Separate because the rule worth testing — an empty table must not truncate a good
// cache — cannot be reached through a method that gets its input from a live DHT.
// The same lesson P03 learned about interface selection: a decision entangled with
// the thing it queries is a decision no test can put in a state.
func writeNodes(dir string, nodes []krpc.NodeInfo) (uint64, error) {
	buf := make([]byte, 0, len(cacheMagic)+len(nodes)*nodeRecord)
	buf = append(buf, cacheMagic...)
	var n uint64
	for _, ni := range nodes {
		ip16 := ni.Addr.IP.To16()
		if ip16 == nil || ni.Addr.Port <= 0 || ni.Addr.Port > 0xffff {
			continue
		}
		buf = append(buf, ni.ID[:]...)
		buf = append(buf, ip16...)
		buf = binary.BigEndian.AppendUint16(buf, uint16(ni.Addr.Port))
		n++
	}
	if n == 0 {
		// Nothing learned. Do NOT truncate a good cache with an empty one — a run
		// that never reached the network would otherwise destroy the list that
		// would have let the next run start, turning one bad network day into a
		// permanently cold start.
		return 0, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	tmp := filepath.Join(dir, bootstrapFile+".tmp")
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, filepath.Join(dir, bootstrapFile)); err != nil {
		return 0, err
	}
	return n, nil
}
