package rendezvous

import (
	"bytes"
	"net"
	"strings"
	"sync/atomic"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

// # The library has two doors a stranger can kill it through, and they need different keys
//
// Both are the same shape — a panic on the goroutine `dht.NewServer` starts for itself
// (server.go:247), where no call of ours is on the stack to recover it — and neither
// needs a session, a routing-table entry, or a race. The mux's rule is that anything
// which is not QUIC is KRPC and goes to the DHT, so one datagram to Nib's session port
// is the whole attack.
//
// **Door one is the decoder**, closed below by `screened`. **Door two is the handler**,
// closed by `gateQuery`, and it is worth saying plainly that the first version of this
// file shipped with the second door open while its comment read as a clean bill of
// health. Screening the decoder is not screening the library.
//
// # Door one: the decoder, killed by 21 bytes
//
// `krpc.NodeAddr.UnmarshalBinary` allocates `make(net.IP, len(b)-2)` with no length
// check (nodeaddr.go:35). A bencoded `ip` value of zero or one byte therefore asks for
// a negative-length slice, and three separate design choices conspire to make that
// fatal rather than an error:
//
//   - the bencode decoder deliberately RE-PANICS runtime errors rather than converting
//     them (decode.go:43-46), because it treats them as programmer error;
//   - the decode happens in `dht.Server.processPacket` (server.go:287), which has no
//     recover;
//   - and that runs on the goroutine `dht.NewServer` starts for itself
//     (server.go:247), so there is no call of ours anywhere on the stack to recover it.
//
// Measured, not reasoned: `d2:ip0:1:t2:zz1:y1:re` — 21 bytes — from any host that can
// send one datagram to Nib's session port takes the whole process down. It needs no
// session, no entry in our routing table, and wins no race, because the mux's rule is
// that anything which is not QUIC is KRPC and goes to the DHT.
//
// # Why this screens with the library's own decoder rather than a length filter
//
// A hand-written filter would have to know which fields reach `UnmarshalBinary` — `ip`
// today, `values` as well, and whatever a future version adds — and it would be wrong
// silently the moment that set changed. Running the SAME decoder the library will run,
// under a recover, cannot be wrong about which datagrams are dangerous: it asks the
// authority instead of imitating it. The decoder is pure over the datagram, so a packet
// that survives here survives there.
//
// The cost is one extra decode per inbound DHT datagram. It is paid on the DHT view
// only — QUIC never touches this path.
//
// It also does NOT cover door two, and cannot: a query that decodes perfectly can still
// kill the handler, and there is no way to ask the library "would you survive handling
// this" without handling it. See gateQuery.
// It wraps as a net.PacketConn and nothing more. anacrolix/dht touches only LocalAddr,
// ReadFrom, WriteTo and Close (server.go:158, :347, :818, :1185) — no type assertion, no
// SetReadBuffer — so the embedding loses nothing today. A library that started asserting
// for *net.UDPConn or for a buffer-sizing method would get the wrapper instead and fail
// silently, which is the cost of every shim and is worth knowing about rather than
// discovering.
type screened struct {
	net.PacketConn
	dropped          *atomic.Uint64
	refusedResponses *atomic.Uint64
}

// ReadFrom skips what would kill the process and hands the rest through untouched.
//
// One decode, two jobs. The decode was already being paid for door one, and door three
// is a question about the decoded message rather than about the bytes, so classifying
// responses here costs nothing beyond what this loop already spent.
func (s *screened) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		n, addr, err := s.PacketConn.ReadFrom(p)
		if err != nil {
			// Deadlines and closure must NOT be swallowed by the loop.
			return n, addr, err
		}
		m, ok := decodeScreened(p[:n])
		if !ok {
			s.dropped.Add(1)
			continue
		}
		if killsGetHandler(m) {
			s.refusedResponses.Add(1)
			continue
		}
		return n, addr, nil
	}
}

// killsGetHandler reports whether this is door three: a RESPONSE that kills the BEP-44
// get path.
//
// # Door three, and why neither of the other two sees it
//
// `exts/getput.Get` reads the reply to our own `get` at getput.go:55:
//
//	} else if sha1.Sum(append(r.K[:], salt...)) == target && bep44.Verify(r.K[:], salt, *r.Seq, ...)
//
// `Seq` is `*int64` with `omitempty` (krpc/msg.go:85), Go evaluates `&&` left to right,
// and nothing between the two operands checks it. So a reply carrying the right `k` and
// **no `seq`** dereferences nil, on a goroutine started bare at traversal/operation.go:236
// with no recover anywhere in the chain — the library's only recover is krpc/error.go:50
// and it is unrelated.
//
// Door one cannot see it: the datagram decodes perfectly, which is this file's own
// recorded lesson. Door two cannot see it either: `gateQuery` is wired as `OnQuery` and
// fires on QUERIES; this is a response and carries `m.R`, not `m.A`. That is three
// separate doors now, each invisible to the ones before it.
//
// # The asymmetry is the tell, and it is the same one P04.S02 found
//
// The library's OWN server guards exactly this field on the inbound `put`
// (server.go:578-583, "expected seq argument"). The check exists. It was applied in one
// direction only — precisely as `get_peers` guarded `args.Token` while `announce_peer`
// and `put` did not.
//
// # Who can send it
//
// Not any node we query: the `get` query carries only the 20-byte target, so a responder
// must already hold `k` to make `sha1(k‖salt)` match, which is a preimage problem. The
// reachable set is exactly the people who were GIVEN `k` — the ~8 nodes that stored the
// record (a `put` hands them `k`, `salt` and `sig` in the clear), anyone on the path of
// that put, and **any holder of the ceremony invitation**, since the hop key is derived
// from the secret. In a two-party ceremony that last one is the counterparty. A protocol
// whose premise is that the two parties do not fully trust each other must not hand
// either of them a one-datagram kill on the other.
//
// The rule is stated as a property of the message — a response carrying `k` must carry
// `seq` — rather than as a patch for one call site, so it keeps covering the library if
// another BEP-44 read path grows the same shape.
func killsGetHandler(m krpc.Msg) bool {
	return m.Y == krpc.YResponse && m.R != nil &&
		m.R.K != ([32]byte{}) && m.R.Seq == nil
}

// maxStrLenExceeded is the decoder's own wording for a string longer than the bound.
//
// Matched as text because `parseString` returns a plain `fmt.Errorf` with no sentinel
// and no typed error to check (decode.go:206). A guard asserts the library still says
// it, so a dependency bump cannot disarm the drop below in silence — the same rule this
// repo applies to every log line it greps: verify the emitted string, once, against real
// output.
const maxStrLenExceeded = "exceeds limit"

// survivesDecode reports whether the library's decoder can be run over these bytes
// without panicking — and refuses one class it would survive but should not be asked to.
//
// A clean decode ERROR returns true on purpose. The library rejects malformed KRPC
// perfectly well and logs it; this function's job is the crash, and widening it to "is
// this valid" would move a decision out of the library and into a copy of it.
//
// # The one exception, and why it is not that widening
//
// bencode allocates a string's declared length BEFORE reading it (decode.go:233), and
// its default ceiling is ~128 MiB (decode.go:17), which `bencode.Unmarshal` gives no way
// to lower. So `d1:t134217727:zze` — seventeen bytes — allocates 128 MiB, fails, and is
// discarded. Measured: 17 bytes in, 134,221,493 bytes allocated, per decode. Screening
// it with an unbounded decoder does that TWICE, once here and once in the library.
//
// The bound below is `len(b)`: a bencoded string longer than the datagram containing it
// cannot exist, so this can never be narrower than reality and can never change which
// datagrams are judged fatal. And exceeding it is not a judgment about validity — it is
// arithmetic about the bytes in hand — so dropping is not the library's decision being
// taken away from it. Under the bound the same seventeen bytes cost 640.
func survivesDecode(b []byte) bool {
	_, ok := decodeScreened(b)
	return ok
}

// decodeScreened is survivesDecode's body, returning the message it decoded so the
// caller does not pay for a second one. A datagram that does not survive yields the zero
// Msg, which no caller may read — `ok` is the only thing that says whether it means
// anything.
func decodeScreened(b []byte) (m krpc.Msg, ok bool) {
	defer func() {
		if recover() != nil {
			m, ok = krpc.Msg{}, false
		}
	}()
	d := bencode.NewDecoder(bytes.NewReader(b))
	d.MaxStrLen = int64(len(b))
	if err := d.Decode(&m); err != nil && strings.Contains(err.Error(), maxStrLenExceeded) {
		return krpc.Msg{}, false
	}
	return m, true
}

// gateQuery refuses inbound queries the library's own handlers dereference their way
// through.
//
// `handleQuery` binds `args := m.A` and then, for `announce_peer` and `put`, reads
// `args.Token` with no nil check (server.go:508, :540, :573). `get_peers` guards the
// same field one case earlier — the check exists, it was simply not applied to the other
// two. So a query with **no `a` dict at all** is a nil dereference: 34 bytes of
// `d1:q13:announce_peer1:t2:zz1:y1:qe`, or 23 of the `put` equivalent, and the process
// is gone. Reproduced through Nib's own mux with a well-formed ping answered first.
//
// # The rule is general because the alternative was measured, not guessed
//
// Every query name the library knows was fuzzed against three shapes of `a` — absent,
// present-with-an-id, and present-but-empty. Exactly two combinations crash, and in both
// the dict is **entirely absent**; an empty `a` is survivable for every query type,
// including the two that die without one. So the rule is "a query must carry its
// arguments dict", which is what BEP-5 requires of every query anyway — not a list of
// two query names that would silently stop covering the library the day it grows a
// third.
//
// Refused silently rather than with a KRPC error: the sender is unauthenticated, and
// answering it would make Nib a small reflector for anyone who cares to point it at
// someone.
//
// This gate is why `OnQuery` is set at all, and it runs before the switch that panics
// (server.go:496 against :508). It does NOT stop the sender being added to our routing
// table, which happens four lines earlier at :492 — a separate property, and one the
// self-address probe's per-prefix rule is what actually bounds.
//
// # It also refuses `put` and `announce_peer` outright, which is a policy and not a crash fix
//
// Nib is a DHT **client**. It issues `get` and `put` for its own rendezvous record and
// needs nothing else from the protocol. Serving other people's writes is not a capability
// it wants, and accepting them today is not a decision anyone took — it is what happens
// when `ServerConfig.Store` is left unset: `NewServer` installs `bep44.NewMemory()`
// (server.go:219-220), a bare `map[Target]*Item` with **no cap and no eviction**
// (bep44/memory.go:21-28). Expiry is lazy and per-target — `Wrapper.Get` deletes only the
// key someone asks for (bep44/store.go:50-64) — so an attacker who puts and never gets
// leaves entries resident for the life of the process. One `get_peers` buys a write token
// good for the whole token interval, and each accepted put is up to 1000 bytes.
//
// That is an unauthenticated remote memory-growth path in a product whose ADR-005 caps
// open DOCUMENTS at 512 MiB because memory matters. And it is worse than a resource
// question: it means a program for handling legal documents holds arbitrary bytes from
// strangers, at their choosing, on a user's machine.
//
// Refusing here closes it at a door Nib already owns, with no new type and no custom
// Store, and leaves our own publishing untouched — `dht.Server.Put` writes to our own
// store directly (server.go:1081) and never travels through this hook.
//
// The honest cost, recorded rather than glossed: Nib takes BEP-44 storage service from
// the network and returns none. The alternative is holding strangers' data, and for this
// product that trade is not close.
func (s *Server) gateQuery(m *krpc.Msg, _ net.Addr) bool {
	if m == nil || m.A == nil {
		s.refusedQueries.Add(1)
		return false
	}
	if m.Q == "put" || m.Q == "announce_peer" {
		s.refusedStores.Add(1)
		return false
	}
	return true
}
