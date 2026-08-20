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
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/krpc"
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
}

// Server is a DHT bound to a socket somebody else owns.
type Server struct {
	dht  *dht.Server
	dir  string
	once sync.Once

	loaded int
	saved  atomic.Uint64
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

	nodes, err := s.loadNodes()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// A corrupt cache is not fatal — it is a first run with extra steps. Said
		// out loud rather than swallowed, because "no nodes" and "nodes I could not
		// read" want different advice.
		return nil, fmt.Errorf("read the cached node list: %w", err)
	}
	s.loaded = len(nodes)

	cfg := &dht.ServerConfig{
		Conn:       conn,
		NoSecurity: false,
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
			out := make([]dht.Addr, 0, len(nodes))
			for _, n := range nodes {
				out = append(out, dht.NewAddr(n.Addr.UDP()))
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
		Nodes:  s.dht.NumNodes(),
		Loaded: s.loaded,
		Saved:  s.saved.Load(),
	}
}

// Ping is the one query this slice needs: it is how a cached node is confirmed
// live, and how the table is populated on a first run.
func (s *Server) Ping(ctx context.Context, addr *net.UDPAddr) error {
	res := s.dht.Ping(addr)
	return res.Err
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

// loadNodes reads the cache. The format is the compact 6-byte form BEP-5 already
// uses on the wire, so there is no second encoding to keep in step with the first.
func (s *Server) loadNodes() ([]krpc.NodeInfo, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, bootstrapFile))
	if err != nil {
		return nil, err
	}
	const rec = 26 // 20-byte node id + 6-byte compact IPv4 endpoint
	if len(b)%rec != 0 {
		return nil, fmt.Errorf("cached node list is %d bytes, not a multiple of %d", len(b), rec)
	}
	out := make([]krpc.NodeInfo, 0, len(b)/rec)
	for i := 0; i+rec <= len(b); i += rec {
		var ni krpc.NodeInfo
		copy(ni.ID[:], b[i:i+20])
		ni.Addr.IP = net.IP(append([]byte(nil), b[i+20:i+24]...))
		ni.Addr.Port = int(binary.BigEndian.Uint16(b[i+24 : i+26]))
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
	buf := make([]byte, 0, len(nodes)*26)
	var n uint64
	for _, ni := range nodes {
		v4 := ni.Addr.IP.To4()
		if v4 == nil || ni.Addr.Port <= 0 || ni.Addr.Port > 0xffff {
			continue // IPv4 entries only; the compact form has no room for v6
		}
		buf = append(buf, ni.ID[:]...)
		buf = append(buf, v4...)
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

// waitForNodes is a helper for callers that need a usable table before querying.
func (s *Server) waitForNodes(ctx context.Context, want int) error {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		if s.dht.NumNodes() >= want {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
