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
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/krpc"
	"golang.org/x/time/rate"

	"nib/internal/addrscope"
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
	// InvitationSeeds is how many bootstrap addresses a caller supplied out of band, and
	// InvitationSeedsUsed says whether they were actually consulted.
	//
	// **A sibling rather than a contribution to Seeds, and the reason is the rot alarm.**
	// Seeds means "how many SHIPPED addresses were used because the cache was empty", and
	// `Bootstrapped == 0 while Seeds > 0` is what tells an operator that every address Nib
	// ships is dead. Folding invitation seeds into it would leave that alarm unable to
	// separate "our list rotted" from "the invitation's seeds were bad or hostile", and
	// would make `nib rendezvous` print "cold start — no usable node cache" on a machine
	// with a perfectly good cache.
	InvitationSeeds int
	// InvitationSeedsUsed is also the eclipse flag: true means this routing table was
	// bootstrapped from addresses somebody else chose, after everything Nib ships had
	// failed. Every downstream bound still holds — addrscope bounds which victim, the
	// candidate cap bounds fan-out, D33 bounds packets — but the fact that the table came
	// from a stranger's list is worth being able to read.
	// InvitationBootstrapped is how many nodes the INVITATION's seeds contributed.
	//
	// Separate from Bootstrapped for the same reason InvitationSeeds is separate from
	// Seeds: `Bootstrapped == 0 while Seeds > 0` is the shipped list's rot alarm, and
	// crediting the retry's nodes to it makes the alarm unable to fire on exactly the
	// machine the seeds rescued.
	InvitationBootstrapped uint64

	// InvitationSeedsTried is set once the demonstrated-failure retry has run. TRIED and
	// USED are different facts: a retry that reached nothing leaves this true and USED
	// false, and the operator note must not then claim the shipped list worked.
	InvitationSeedsTried bool
	InvitationSeedsUsed  bool
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
	// RefusedResponses is inbound RESPONSES dropped before the BEP-44 get path read
	// them — door three. **Non-zero means somebody who holds our hop key sent a reply
	// shaped to kill us**, and the reachable set is small and named: a node that stored
	// our record, an observer of the put that told it, or a holder of the invitation.
	// Separate from Screened and RefusedQueries for the same reason those two are
	// separate from each other: three doors, three keys, and which one someone knocked
	// on says something different about who they are.
	RefusedResponses uint64
	// RefusedStores is inbound `put` / `announce_peer` queries refused because Nib is a
	// DHT client and does not store other people's data. **A steady non-zero is
	// ordinary** — it is the DHT asking, and Nib declining — so unlike the other two
	// refusal counters this one is not an alarm. It is here to show the refusal is
	// happening at all, because the failure it guards against is silent: a future
	// change that sets `ServerConfig.Store` or drops the gate turns Nib back into an
	// unbounded store and nothing else would say so.
	RefusedStores uint64
	// Responses is inbound replies that reached the library. **Zero while queries are
	// going out is "nothing is answering us"** — a different fact from RefusedResponses
	// (replies arrive and we drop them) and from an empty routing table, and the three
	// want different advice.
	Responses uint64

	// PublishAttempts is calls to Publish that got past their local checks. Zero while a
	// ceremony is armed is a wiring failure, not a network one — nothing even tried.
	PublishAttempts uint64
	// Published is publishes whose token-gathering traversal completed. **It is NOT
	// "records stored by anybody"**, and the name is deliberately not `PublishAccepted`:
	// getput.Put shadows every per-node error and returns nil regardless
	// (exts/getput/getput.go:155-172), so no honest counter here can claim acceptance.
	Published uint64
	// PublishNodes is how many nodes ANSWERED the traversal that collected write tokens,
	// from the last publish. It is the closest true statement available to "we found
	// somewhere to write". Zero with PublishAttempts non-zero means the routing table led
	// nowhere.
	PublishNodes uint64
	// FetchAttempts, Fetched, FetchEmpty split the three outcomes that want different
	// advice: nothing ran / a record came back / the traversal completed and nobody had
	// one. FetchEmpty is the ORDINARY state before a peer publishes and must never be
	// summed with a transport failure.
	FetchAttempts uint64
	Fetched       uint64
	FetchEmpty    uint64
	// FetchUndecodable is a value that arrived and was not bencode. Its own counter
	// because it means somebody served us something shaped wrong, which is a different
	// fact from an absent record and from a record that failed its signature.
	FetchUndecodable uint64
	// FetchNodes is how many nodes answered the last fetch traversal. **It is what makes
	// an empty fetch legible**: zero means the record's absence is not evidence of
	// anything, because we reached nobody who could have had it. Non-zero with
	// FetchEmpty means the DHT is reachable and the peer has not published yet.
	FetchNodes uint64
	// FetchAborted is lookups that did not finish — the caller cancelled, or the budget
	// expired. **Split from FetchEmpty deliberately**: an unfinished lookup says nothing
	// about whether a record exists, and folding the two would make the ladder tell a user
	// their peer has not published when the truth is that we stopped asking.
	FetchAborted uint64
	// PublishSeqCeiling is publishes refused because the sequence number at our own key is
	// at its ceiling. **Non-zero means somebody holding this ceremony's invitation has
	// taken the key** — the in-roster preemption power, in the one form that is permanent
	// rather than a race we can re-win.
	PublishSeqCeiling uint64

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

	screened         atomic.Uint64
	refusedQueries   atomic.Uint64
	refusedResponses atomic.Uint64
	refusedStores    atomic.Uint64
	responses        atomic.Uint64

	publishAttempts   atomic.Uint64
	published         atomic.Uint64
	publishNodes      atomic.Uint64
	fetchAttempts     atomic.Uint64
	fetched           atomic.Uint64
	fetchEmpty        atomic.Uint64
	fetchUndecodable  atomic.Uint64
	fetchNodes        atomic.Uint64
	fetchAborted      atomic.Uint64
	publishSeqCeiling atomic.Uint64

	// mu guards the invitation seeds, which a caller may set at any time while the
	// library's own goroutine reads them from the StartingNodes closure.
	mu       sync.Mutex
	invSeeds []netip.AddrPort
	// self is the probed public endpoint, kept so the seed sampler can refuse to ship it.
	self netip.AddrPort
	// invBootstrapped is what the INVITATION's seeds gained, kept apart from bootstrapped
	// so the shipped list's rot alarm stays readable. See Bootstrap.
	invBootstrapped atomic.Uint64

	invSeedsTried bool
	invSeedsUsed  bool

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
	conn = &screened{PacketConn: conn, dropped: &s.screened, refusedResponses: &s.refusedResponses, responses: &s.responses}

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
		// The second and fourth doors. See gateQuery: a query carrying no arguments dict
		// is a nil dereference inside handleQuery, on a goroutine nothing here can
		// recover — and `put`/`announce_peer` are refused as policy, because Nib is a
		// DHT client and will not hold strangers' bytes.
		OnQuery: s.gateQuery,
		// Exp is what our own store serves BY, and leaving it unset does not mean
		// "never expire" — it means "expire everything, immediately".
		//
		// `NewServer` defaults `Store` and `SendLimiter` for a caller-supplied config
		// (server.go:212-224) but NOT `Exp`; `Exp: 2 * time.Hour` lives only in
		// `NewDefaultServerConfig` (server.go:167), which caveat 7 forbids us. With the
		// zero value, `bep44.NewWrapper(store, 0)` makes `Wrapper.Get` compute
		// `created.Add(0).After(now)` — false for any item stored even a nanosecond ago
		// — so it deletes the item and answers not-found (bep44/store.go:50-64).
		//
		// The consequence is not abstract: **our own published record is deleted the
		// first time anyone reads it, including us.** Nib would be one of the nodes
		// holding its own rendezvous record and would serve it to nobody.
		//
		// Two hours is BEP-44's own recommended item lifetime and the library's own
		// default. It bounds nothing else, because gateQuery refuses inbound writes —
		// the only things in this store are records we put there ourselves.
		Exp: 2 * time.Hour,
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
			s.mu.Lock()
			var extra []netip.AddrPort
			if s.invSeedsTried {
				extra = append(extra, s.invSeeds...)
			}
			s.mu.Unlock()
			out := make([]dht.Addr, 0, len(nodes)+len(seeds))
			for _, n := range nodes {
				out = append(out, dht.NewAddr(n.Addr.UDP()))
			}
			for _, a := range seeds {
				out = append(out, dht.NewAddr(a))
			}
			for _, ap := range extra {
				// net.UDPAddrFromAddrPort, never ResolveUDPAddr — the guard forbids the
				// resolver by name, and the type forbids a hostname by construction.
				out = append(out, dht.NewAddr(net.UDPAddrFromAddrPort(ap)))
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{
		Nodes:                  s.dht.NumNodes(),
		Loaded:                 s.loaded,
		Saved:                  s.saved.Load(),
		CacheRejected:          s.cacheRejected,
		Seeds:                  s.seeds,
		InvitationSeeds:        len(s.invSeeds),
		InvitationBootstrapped: s.invBootstrapped.Load(),
		InvitationSeedsTried:   s.invSeedsTried,
		InvitationSeedsUsed:    s.invSeedsUsed,
		Bootstrapped:           s.bootstrapped.Load(),
		Screened:               s.screened.Load(),
		RefusedQueries:         s.refusedQueries.Load(),
		RefusedResponses:       s.refusedResponses.Load(),
		Responses:              s.responses.Load(),
		RefusedStores:          s.refusedStores.Load(),
		PublishAttempts:        s.publishAttempts.Load(),
		Published:              s.published.Load(),
		PublishNodes:           s.publishNodes.Load(),
		FetchAttempts:          s.fetchAttempts.Load(),
		Fetched:                s.fetched.Load(),
		FetchEmpty:             s.fetchEmpty.Load(),
		FetchUndecodable:       s.fetchUndecodable.Load(),
		FetchNodes:             s.fetchNodes.Load(),
		FetchAborted:           s.fetchAborted.Load(),
		PublishSeqCeiling:      s.publishSeqCeiling.Load(),
		Observed:               s.observed.Load(),
		RejectedLength:         s.rejectedLength.Load(),
		RejectedPort:           s.rejectedPort.Load(),
		RejectedScope:          s.rejectedScope.Load(),
		Disagreements:          s.disagreements.Load(),
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
	err := s.bootstrapOnce(ctx)

	// Invitation seeds are a LAST RESORT, and the trigger is demonstrated failure rather
	// than an empty cache file.
	//
	// The obvious branch — use them when the cache is empty — tests the wrong thing.
	// `Open`'s `len(nodes) == 0` asks whether the FILE is empty, not whether it works, so a
	// machine carrying forty stale cached nodes would never reach the seeds at all. That is
	// precisely the machine invitation seeds exist for: it has spoken to the DHT before,
	// its cache is worthless now, and `Bootstrapped` reads 0 with `Seeds` 0 and nothing
	// says why.
	//
	// Trying them only after a real bootstrap produced nothing also bounds the exposure.
	// Seeds are attacker-controllable and permanently unsigned, so a design that consults
	// them on every run hands whoever wrote the invitation a read receipt per run and a
	// standing chance to eclipse the routing table. Here they are reached only by a machine
	// that already cannot bootstrap any other way.
	s.mu.Lock()
	retry := s.dht.NumNodes() == 0 && len(s.invSeeds) > 0 && !s.invSeedsTried
	if retry {
		// Set BEFORE the retry, because the closure reads it to decide whether to emit the
		// seeds at all. "Tried" is all this can honestly mean at this point.
		//
		// A caller whose context is already dead burns the one shot without reaching a
		// single seed, and that is deliberate: re-arming on a failed attempt is what turns
		// "consulted at most once per process" into "consulted on every retry", which is
		// the read receipt and the standing eclipse chance the one-shot exists to bound.
		// The run whose context died has failed anyway, and the CLI reports it.
		s.invSeedsTried = true
	}
	s.mu.Unlock()
	if !retry {
		return err
	}

	// The retry's gains are counted SEPARATELY.
	//
	// Both attempts added to s.bootstrapped, so on the machine invitation seeds exist for
	// — shipped list dead, seeds rescue it — Stats() reported `Seeds: 5, Bootstrapped: 25`
	// and the rot alarm documented above ("Zero while Seeds is non-zero is the rot alarm")
	// read "the shipped list worked" on a run where every shipped address was dead. The
	// plan defends the Seeds term of that comparison by name; the confounding landed on
	// the other term.
	before := s.bootstrapped.Load()
	err2 := s.bootstrapOnce(ctx)
	if gained := s.bootstrapped.Load() - before; gained > 0 {
		s.bootstrapped.Add(^(gained - 1)) // subtract: these nodes came from the seeds
		s.invBootstrapped.Add(gained)
	}

	// USED is a separate fact from TRIED, and conflating them made the eclipse disclosure
	// lie. The retry runs on the caller's context, which the first attempt may already have
	// exhausted — measured: `err=context deadline exceeded, InvitationSeedsUsed=true,
	// Nodes=0, Bootstrapped=0`. That reported "this routing table came from a list the
	// invitation's sender chose" about a table that came from nothing at all. The flag the
	// operator reads is now set only if the table is actually populated after the retry.
	if s.dht.NumNodes() > 0 {
		s.mu.Lock()
		s.invSeedsUsed = true
		s.mu.Unlock()
		// The attempt that decided the table owns the verdict. Keeping the first error
		// would report a failure over a working table — and the caller prints it.
		return nil
	}
	if err2 != nil {
		return err2
	}
	return err
}

// bootstrapOnce is one traversal, counting what it added.
func (s *Server) bootstrapOnce(ctx context.Context) error {
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

// SeedSample picks up to n live nodes to put in an invitation — the producing half of
// D6's second half.
//
// **Random per call, and that is a privacy property rather than a nicety.** A stable seed
// set is a watermark: two invitations sharing one would link them to a single issuer, and a
// recipient's bootstrap tells whoever chose the addresses that this invitation was opened,
// from this IP, at this moment. Sampling fresh each time removes the first of those; the
// second is inherent to the mechanism and is what the disclosure exists for.
//
// It cannot leak the issuer's own endpoint: a DHT routing table holds other nodes, never
// self. Filtered through addrscope anyway, because a table entry is something strangers
// put there.
func (s *Server) SeedSample(n int) []netip.AddrPort {
	s.mu.Lock()
	self := s.self
	s.mu.Unlock()
	return sampleSeeds(s.dht.Nodes(), self, n)
}

// sampleSeeds is the pure half, separated from the query that feeds it for the same reason
// `classify` is: a hermetic table can only ever hold loopback nodes, which the predicate
// correctly refuses — so driving the filter through a live Server tests nothing. An earlier
// version of the test did exactly that: `SeedSample` stubbed to return nil kept it green,
// because its loop body never ran.
func sampleSeeds(nodes []krpc.NodeInfo, self netip.AddrPort, n int) []netip.AddrPort {
	if n <= 0 {
		return nil
	}
	var usable []netip.AddrPort
	for _, ni := range nodes {
		a, ok := netip.AddrFromSlice(ni.Addr.IP)
		if !ok || ni.Addr.Port <= 0 || ni.Addr.Port > 0xffff {
			continue
		}
		ap := netip.AddrPortFrom(a.Unmap(), uint16(ni.Addr.Port))
		// Never our own endpoint — ENFORCED, not asserted.
		//
		// The doc used to rest this on the library refusing to store its own node in the
		// routing table, which is a check on the ID and not the address: a second Nib on
		// this host, or another peer behind the same office NAT, has a different id and the
		// same public IP. Shipping that to every recipient and every mail server in between
		// is the disclosure this slice says must not happen.
		if self.IsValid() && ap.Addr() == self.Addr() {
			continue
		}
		if addrscope.Seed(ap) {
			usable = append(usable, ap)
		}
	}
	if len(usable) <= n {
		return slices.Clone(usable)
	}
	// Fisher-Yates over crypto/rand: an invitation is a security artifact and a predictable
	// sample is a predictable watermark.
	for i := len(usable) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			// A half-shuffled prefix would silently lose the anti-watermark property this
			// function's whole doc rests on. crypto/rand does not fail on a modern kernel,
			// so refusing costs nothing and keeps the claim true.
			return nil
		}
		k := int(j.Int64())
		usable[i], usable[k] = usable[k], usable[i]
	}
	return slices.Clone(usable[:n])
}

// Seed supplies bootstrap addresses a caller obtained out of band — D6's second half, the
// seeds an invitation carries.
//
// **Opaque `netip.AddrPort`, and after Open rather than through it.** After, because
// `StartingNodes` is evaluated lazily at every traversal, and because the caller that has
// an invitation does not necessarily have one when the socket opens. `netip.AddrPort`
// because it is structurally incapable of holding a hostname — a stronger statement of D6's
// no-resolver rule than the name blacklist that guards this package, and one that survives
// a rename.
//
// This package never learns where they came from. Validation is the caller's, and lives
// where the invitation is (L1: this package may not read one).
func (s *Server) Seed(addrs []netip.AddrPort) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invSeeds = append(s.invSeeds[:0], addrs...)
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
	// A table built from somebody else's addresses is NOT written down.
	//
	// Without this the eclipse outlives the ceremony: the invitation's seeds answer, their
	// neighbours become the persistent cache, and every future run bootstraps from an
	// attacker-chosen list with Seeds 0, InvitationSeeds 0 and InvitationSeedsUsed false —
	// no trace on the machine of where the table came from. The written answer to the
	// eclipse chain bounds what a hostile seed can do *during* a ceremony; this is what
	// stops it becoming permanent.
	//
	// The cost is one cold start next time, which is exactly the situation invitation
	// seeds exist to rescue.
	s.mu.Lock()
	fromStranger := s.invSeedsUsed
	s.mu.Unlock()
	if fromStranger {
		return nil
	}
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
