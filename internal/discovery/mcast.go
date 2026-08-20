package discovery

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// The groups and the port.
//
// IPv4 uses 239.255.90.90, in 239.0.0.0/8 — the range RFC 2365 reserves for local
// administration, which is exactly what this is. IPv6 uses ff12::6e69:62, in ff12::/16:
// link-local scope with the transient flag set, which is the range RFC 4291 provides
// for groups that are not permanently assigned by IANA. Neither squats on an
// allocation belonging to something else, and the group ID spells "nib".
//
// **The scope guarantee is the TTL, not the address.** Scope bits in an address are a
// statement of intent that a router may respect; a hop limit of 1 is arithmetic. Both
// are set, and the TTL is the one that means it.
//
// The port is Nib's own rather than 5353. Sharing the mDNS group would work out of the
// box on a default-deny desktop — measured, it is the one port such a host already
// permits — but it puts non-mDNS bytes on a group other implementations parse as mDNS.
// Doing that honestly means speaking DNS-SD, which is a phase of its own. ADR-007
// records the trade and where the cost is paid instead.
const (
	group4 = "239.255.90.90"
	group6 = "ff12::6e69:62"
	// Port is the discovery port. Exported because a diagnostic and a firewall rule
	// both need to name it, and a number a user must be told is not an internal.
	Port = 8446
	// hopLimit keeps every announcement on the link. One hop: a router decrements
	// to zero and drops it, whatever it thinks of the address's scope bits.
	hopLimit = 1
)

// Stats is what the socket did. Discovery's failure mode is silence, so every one of
// these exists to turn "nothing happened" into a sentence.
type Stats struct {
	// Interfaces is how many interfaces were joined.
	Interfaces int
	// Sent is announcements written, counted per interface — so one Announce on a
	// two-interface host adds two.
	Sent uint64
	// SendErrors is writes that failed. Non-zero with Sent zero is a send path that
	// never worked; non-zero with Sent climbing is one interface of several.
	SendErrors uint64
	// Own is datagrams recognised as this process's own loopback copies. **Non-zero
	// is the good reading**: it proves the send path reaches the kernel and comes
	// back, independent of any firewall, which is the only such proof available.
	Own uint64
	// Peers is announcements from somebody else, parsed and returned to the caller.
	Peers uint64
	// Foreign is datagrams that were not Nib announcements — ordinary on a shared
	// link, and counted rather than ignored so a diagnostic can distinguish "quiet
	// link" from "busy link, no Nibs".
	Foreign uint64
	// Malformed is datagrams that claimed to be ours and were not.
	Malformed uint64
	// NoControlMessage is set when the platform could not supply the arrival
	// interface. It is not an error and not a fallback: it is Windows, where
	// x/net's SetControlMessage is unimplemented, and it is recorded because a
	// filter written against a nil control message silently accepts everything.
	NoControlMessage bool
}

// Socket is one open discovery socket: joined on the chosen interfaces, able to
// announce on each of them, and reading announcements from the link.
//
// It deliberately does not import internal/udpmux. Caveat 7's plan-review pin says so
// directly: the mux exists because a NAT mapping is a function of the internal
// IP:port, so the mapped port, the DHT probe and the live session must share one
// socket. Link-local multicast carries no NAT mapping and needs to share nothing.
type Socket struct {
	pc net.PacketConn
	p4 *ipv4.PacketConn
	p6 *ipv6.PacketConn

	ifis  []net.Interface
	all   []net.Interface
	nonce [nonceLen]byte

	group4Addr *net.UDPAddr
	group6Addr *net.UDPAddr

	closeOnce sync.Once

	interfaces int
	sent       atomic.Uint64
	sendErrors atomic.Uint64
	own        atomic.Uint64
	peers      atomic.Uint64
	foreign    atomic.Uint64
	malformed  atomic.Uint64
	noCM       atomic.Bool
}

// Seen is one announcement observed on the link, with where it came from.
//
// From is the source address of the datagram, and it is why the announcement does not
// carry an address of its own: a claimed address can be anything, whereas a source
// address is where a reply would actually go. Combined with the announced port it is
// the candidate to dial.
type Seen struct {
	Announcement
	From *net.UDPAddr
}

// Open binds the discovery socket and joins the group on every interface that
// qualifies. It returns an error only if nothing could be joined at all — a host with
// one usable interface and three unusable ones is the normal case, not a failure.
func Open(nonce [nonceLen]byte) (*Socket, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("enumerate interfaces: %w", err)
	}
	return open(all, nonce)
}

func open(all []net.Interface, nonce [nonceLen]byte) (*Socket, error) {
	// SO_REUSEADDR, and it is not optional. net.ListenPacket does not set it — the
	// stdlib only does so when the BIND ADDRESS is multicast, and this binds the
	// wildcard — so without this a second Nib on the same machine cannot open the
	// socket at all, and same-host discovery is exactly the case tier 4 drives.
	//
	// Only SO_REUSEADDR, deliberately: darwin and the BSDs need SO_REUSEPORT as well
	// for two listeners to share a port. That would mean a //go:build file per
	// platform, and a no-op sibling is the shape that already shipped one silent
	// defect here (ReplaceOthers returning 0 off Linux). So the gap is left open and
	// named instead, with a test that asserts two sockets CAN share the port — which
	// fails loudly on darwin if this turns out to be insufficient, rather than
	// leaving a tagged file nobody can verify.
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		if err := c.Control(func(fd uintptr) {
			serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		}); err != nil {
			return err
		}
		return serr
	}}

	pc, err := lc.ListenPacket(nil, "udp", fmt.Sprintf(":%d", Port))
	if err != nil {
		return nil, fmt.Errorf("bind discovery socket: %w", err)
	}

	s := &Socket{pc: pc, all: all, nonce: nonce}
	s.group4Addr = &net.UDPAddr{IP: net.ParseIP(group4), Port: Port}
	s.group6Addr = &net.UDPAddr{IP: net.ParseIP(group6), Port: Port}
	s.p4 = ipv4.NewPacketConn(pc)
	s.p6 = ipv6.NewPacketConn(pc)

	// The arrival interface, where the platform will say. The error is CHECKED and
	// recorded rather than discarded: on Windows this returns errNotImplemented and
	// every later control message is nil with a nil error, so a filter written as
	// `if cm != nil && cm.IfIndex != want` silently accepts everything. Nothing here
	// filters on it — the self-nonce does that — but a diagnostic that reports an
	// arrival interface it never had would be worse than one that says it cannot.
	if err := s.p4.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		s.noCM.Store(true)
	}
	if err := s.p6.SetControlMessage(ipv6.FlagInterface, true); err != nil {
		s.noCM.Store(true)
	}
	_ = s.p4.SetMulticastTTL(hopLimit)
	_ = s.p6.SetMulticastHopLimit(hopLimit)

	// Joins. IPv4 is asked for interfaces that have an IPv4 address, for the Windows
	// reason chooseInterfaces documents; IPv6 has no such constraint.
	joined := map[int]net.Interface{}
	for _, ifi := range chooseInterfaces(all, true) {
		if err := s.p4.JoinGroup(&ifi, s.group4Addr); err == nil {
			joined[ifi.Index] = ifi
		}
	}
	for _, ifi := range chooseInterfaces(all, false) {
		if err := s.p6.JoinGroup(&ifi, s.group6Addr); err == nil {
			joined[ifi.Index] = ifi
		}
	}
	if len(joined) == 0 {
		pc.Close()
		return nil, fmt.Errorf("joined no interfaces — %s", describe(all, nil))
	}
	for _, ifi := range joined {
		s.ifis = append(s.ifis, ifi)
	}
	s.interfaces = len(s.ifis)
	return s, nil
}

// Interfaces names what was joined, so a test can assert the selection rather than
// trusting whatever the stdlib picked, and a diagnostic can print it.
func (s *Socket) Interfaces() []string {
	out := make([]string, 0, len(s.ifis))
	for _, ifi := range s.ifis {
		out = append(out, ifi.Name)
	}
	return out
}

// Describe renders the whole selection, joined and skipped, with flags.
func (s *Socket) Describe() string { return describe(s.all, s.ifis) }

// LocalPort is the bound port.
func (s *Socket) LocalPort() int {
	if a, ok := s.pc.LocalAddr().(*net.UDPAddr); ok {
		return a.Port
	}
	return 0
}

// Announce sends one announcement on every joined interface, over both families.
//
// Once per interface, because there is no send-to-all primitive: an unbound send goes
// out exactly one interface, chosen by the route table — which on a machine whose
// default route is a VPN is the wrong one, and nothing errors. Measured.
//
// It reports how many writes succeeded. A caller that gets (0, nil) has sent nothing
// on a host with interfaces, which is a state worth being able to distinguish from an
// error.
func (s *Socket) Announce(a Announcement) (int, error) {
	b, err := a.Encode()
	if err != nil {
		return 0, err
	}
	var ok int
	var errs []string
	for _, ifi := range s.ifis {
		ifi := ifi
		if err := s.p4.SetMulticastInterface(&ifi); err == nil {
			if _, err := s.p4.WriteTo(b, nil, s.group4Addr); err == nil {
				ok++
				s.sent.Add(1)
			} else {
				s.sendErrors.Add(1)
				errs = append(errs, ifi.Name+"/v4: "+err.Error())
			}
		}
		if err := s.p6.SetMulticastInterface(&ifi); err == nil {
			if _, err := s.p6.WriteTo(b, nil, s.group6Addr); err == nil {
				ok++
				s.sent.Add(1)
			} else {
				s.sendErrors.Add(1)
				errs = append(errs, ifi.Name+"/v6: "+err.Error())
			}
		}
	}
	if ok == 0 && len(errs) > 0 {
		return 0, fmt.Errorf("every interface refused the announcement: %s", strings.Join(errs, "; "))
	}
	return ok, nil
}

// ErrOwn reports that the datagram read was this process's own loopback copy.
//
// Returned rather than swallowed because it is the send path's only
// firewall-independent proof of life: a caller waiting for peers can ignore it, and a
// diagnostic can say "I heard myself, so the problem is not my sending".
var ErrOwn = errors.New("own announcement")

// Read waits for one announcement.
//
// A caller loops on this, treating ErrOwn as informational, ErrNotOurs and
// ErrMalformed as noise to count, and a Seen as a candidate. The deadline is the
// caller's: this is where a browse budget (D16's 2 s) is applied, one layer up.
func (s *Socket) Read(deadline time.Time) (Seen, error) {
	if err := s.pc.SetReadDeadline(deadline); err != nil {
		return Seen{}, err
	}
	buf := make([]byte, MaxDatagram+1) // +1 so an over-cap datagram is SEEN to be over
	n, _, src, err := s.p4.ReadFrom(buf)
	if err != nil {
		return Seen{}, err
	}
	ua, _ := src.(*net.UDPAddr)

	a, perr := Parse(buf[:n])
	switch {
	case errors.Is(perr, ErrNotOurs):
		s.foreign.Add(1)
		return Seen{}, perr
	case perr != nil:
		s.malformed.Add(1)
		return Seen{}, perr
	}
	// The self-filter is the nonce and not the source address. On a multi-homed host
	// the loopback copy arrives from whichever interface address was used, so
	// recognising it by address means enumerating every local address and redoing it
	// whenever they change; a per-process nonce is exact.
	if a.Nonce == s.nonce {
		s.own.Add(1)
		return Seen{}, ErrOwn
	}
	s.peers.Add(1)
	return Seen{Announcement: a, From: ua}, nil
}

// Stats reports what happened. Safe to call concurrently.
func (s *Socket) Stats() Stats {
	return Stats{
		Interfaces:       s.interfaces,
		Sent:             s.sent.Load(),
		SendErrors:       s.sendErrors.Load(),
		Own:              s.own.Load(),
		Peers:            s.peers.Load(),
		Foreign:          s.foreign.Load(),
		Malformed:        s.malformed.Load(),
		NoControlMessage: s.noCM.Load(),
	}
}

// Close leaves the groups and closes the socket.
func (s *Socket) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.pc.Close() })
	return err
}
