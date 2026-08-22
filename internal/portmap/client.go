package portmap

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// ErrNoMapping reports that the port-mapping tier obtained nothing — no gateway, or a gateway
// that answered no protocol within the budget. It is an ordinary tier MISS, never a ceremony
// failure (D15): the caller contributes no candidate and races the other tiers unchanged.
var ErrNoMapping = errors.New("portmap: no mapping obtained")

// perAttempt bounds one send-and-wait so PCP's failure does not spend the whole budget before
// NAT-PMP is tried. Two sends per protocol inside it absorb a single lost UDP datagram, which
// is the common transient on a real link; the overall budget (the caller's context, D16's 3 s
// clock-1) is the hard bound over everything.
const perAttempt = 700 * time.Millisecond

// Client obtains a router port mapping. It holds no socket: each Map opens and closes its own,
// because a mapping request is a brief exchange and a long-lived socket would be one more thing
// the armed-only lifecycle (S07) has to tear down on every exit path.
type Client struct {
	// Gateway is where requests are sent. Zero means "discover it" (DefaultGateway). Set by a
	// test to point at a mock; set by S07's caller to the discovered gateway once, so discovery
	// is not repeated per refresh.
	Gateway netip.AddrPort
	// TryUPnP enables the UPnP-IGD fallback (SSDP multicast discovery + SOAP). It is OFF by
	// default ON PURPOSE: SSDP is real multicast on the LAN and could reach a real router, so a
	// zero-value Client used in a test never fires it. The production caller sets it true.
	TryUPnP bool
}

// Map asks the gateway for an inbound mapping of internalPort for proto, trying PCP then
// NAT-PMP, and returns the external IP:port a peer would dial. ctx bounds the whole attempt —
// the caller sets D16's 3 s budget on it. A miss (no gateway, nothing answered) is ErrNoMapping,
// distinct from a ctx cancellation, so D19 can tell "the router offered nothing" from "the user
// abandoned".
//
// internalPort MUST be the shared endpoint's bound port (caveat 7): the mapping is only useful
// for the socket the session actually answers on.
func (c *Client) Map(ctx context.Context, proto Protocol, internalPort uint16) (Mapping, netip.Addr, error) {
	// The first obtain suggests the internal port as the external one; a refresh suggests the
	// port the router already granted, for stability (grill C7). suggestedExternal == 0 lets the
	// router choose.
	return c.mapWithSuggestion(ctx, proto, internalPort, internalPort)
}

// mapWithSuggestion is Map with an explicit suggested external port — the entry point the S07
// refresh uses to ask the router to keep the same external port.
func (c *Client) mapWithSuggestion(ctx context.Context, proto Protocol, internalPort, suggestedExternal uint16) (Mapping, netip.Addr, error) {
	if err := ctx.Err(); err != nil {
		return Mapping{}, netip.Addr{}, err // already abandoned; no socket, no wait
	}
	gw := c.Gateway
	if !gw.IsValid() {
		// A missing gateway is not fatal — UPnP below self-discovers. Leave gw invalid and skip
		// only the socket protocols (diff-grill #3).
		if addr, err := DefaultGateway(); err == nil {
			gw = netip.AddrPortFrom(addr, GatewayPort)
		}
	}

	// PCP and NAT-PMP need the gateway; UPnP does not (it self-discovers over SSDP), so a
	// missing gateway skips the first two rather than ending the whole tier (diff-grill #3).
	if gw.IsValid() {
		if m, ext, err := c.tryGatewayProtocols(ctx, gw, proto, internalPort, suggestedExternal); err == nil {
			return m, ext, nil
		} else if ctx.Err() != nil {
			return Mapping{}, netip.Addr{}, ctx.Err()
		}
	}

	// UPnP-IGD last (D15's order) — the majority of consumer routers, and the most code. Gated
	// so it never fires from a zero-value Client: SSDP is real LAN multicast and would reach a
	// real IGD.
	if c.TryUPnP {
		if ext, port, ctl, st, err := mapViaUPnP(ctx, proto, internalPort, defaultLeaseSec, isPrivateHost); err == nil {
			return Mapping{Protocol: proto, InternalPort: internalPort, ExternalPort: port, LifetimeSec: defaultLeaseSec,
				via: mechUPnP, upnpControlURL: ctl, upnpServiceType: st}, ext, nil
		} else if ctx.Err() != nil {
			return Mapping{}, netip.Addr{}, ctx.Err()
		}
	}

	return Mapping{}, netip.Addr{}, ErrNoMapping
}

// tryGatewayProtocols runs the two socket protocols (PCP then NAT-PMP) against the gateway on
// one dial. Split out so `Map` reads as "gateway protocols, then UPnP" and the no-gateway path
// simply skips this.
func (c *Client) tryGatewayProtocols(ctx context.Context, gw netip.AddrPort, proto Protocol, internalPort, suggestedExternal uint16) (Mapping, netip.Addr, error) {
	conn, err := net.Dial("udp", gw.String())
	if err != nil {
		return Mapping{}, netip.Addr{}, ErrNoMapping
	}
	defer conn.Close()
	// The local address of a dial to the gateway is this host's internal address on the path to
	// it — exactly what PCP's header wants, and it needs no interface enumeration.
	clientIP := conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr()

	if m, ext, err := c.tryPCP(ctx, conn, proto, clientIP, internalPort, suggestedExternal); err == nil {
		m.via = mechPCP
		return m, ext, nil
	} else if ctx.Err() != nil {
		return Mapping{}, netip.Addr{}, ctx.Err()
	}
	if m, ext, err := c.tryNATPMP(ctx, conn, proto, internalPort, suggestedExternal); err == nil {
		m.via = mechNATPMP
		return m, ext, nil
	} else if ctx.Err() != nil {
		return Mapping{}, netip.Addr{}, ctx.Err()
	}
	return Mapping{}, netip.Addr{}, ErrNoMapping
}

// exchange sends req and returns the first response, retransmitting once inside perAttempt (or
// the ctx deadline, whichever is sooner) to ride out a single lost datagram.
func exchange(ctx context.Context, conn net.Conn, req []byte) ([]byte, error) {
	deadline := time.Now().Add(perAttempt)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	buf := make([]byte, 1500)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := conn.Write(req); err != nil {
			return nil, err
		}
		half := time.Now().Add(perAttempt / 2)
		if half.After(deadline) {
			half = deadline
		}
		conn.SetReadDeadline(half)
		n, err := conn.Read(buf)
		if err == nil {
			return buf[:n], nil
		}
		// A ctx cancel mid-Read is observed only when this read deadline (perAttempt/2) fires,
		// so cancellation on the UDP path is bounded-but-not-prompt (diff-grill #5, accepted:
		// the whole call is budget-bounded and the mapping is best-effort).
		if time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("portmap: no response")
}

func (c *Client) tryPCP(ctx context.Context, conn net.Conn, proto Protocol, clientIP netip.Addr, internalPort, suggestedExternal uint16) (Mapping, netip.Addr, error) {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	req := EncodePCPMap(proto, nonce, clientIP, internalPort, suggestedExternal, defaultLeaseSec)
	resp, err := exchange(ctx, conn, req)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	return DecodePCPMap(resp, proto, nonce, internalPort)
}

func (c *Client) tryNATPMP(ctx context.Context, conn net.Conn, proto Protocol, internalPort, suggestedExternal uint16) (Mapping, netip.Addr, error) {
	req, err := EncodeNATPMPMap(proto, internalPort, suggestedExternal, defaultLeaseSec)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	resp, err := exchange(ctx, conn, req)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	m, err := DecodeNATPMPMap(resp, proto, internalPort)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	// NAT-PMP does not carry the external IP in the MAP reply; a second request gets it. A
	// failure here is not fatal to the mapping — the port is mapped — but without the IP there
	// is no candidate to publish, so it is treated as a miss.
	extReq := EncodeNATPMPExternalAddress()
	extResp, err := exchange(ctx, conn, extReq)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	ext, err := DecodeNATPMPExternalAddress(extResp)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	return m, ext, nil
}

// defaultLeaseSec is the requested lease. Short by D15's law (the lease is refreshed while
// armed and must expire on its own after a crash), and the router may grant less. The refresh
// cadence and the delete-on-teardown are S07; this client requests one lease and reads what was
// granted (Mapping.LifetimeSec), which is what S07 schedules the refresh against.
const defaultLeaseSec = 120

// Unmap deletes a mapping obtained by Map, via the same mechanism that granted it (grill C1).
// Best-effort: a failed delete is not fatal because the short lease expires on its own — the
// delete is the clean-teardown path, the lease is the crash-safety floor. The caller passes a
// FRESH, short context (grill C2): close() cancels the arm context first, so a delete derived
// from it would be an instant no-op.
func (c *Client) Unmap(ctx context.Context, m Mapping) error {
	switch m.via {
	case mechUPnP:
		return unmapViaUPnP(ctx, m.upnpControlURL, m.upnpServiceType, m.Protocol, m.ExternalPort)
	default: // mechPCP, mechNATPMP — a MAP with lease 0 for the internal port
		gw := c.Gateway
		if !gw.IsValid() {
			if addr, err := DefaultGateway(); err == nil {
				gw = netip.AddrPortFrom(addr, GatewayPort)
			} else {
				return err
			}
		}
		conn, err := net.Dial("udp", gw.String())
		if err != nil {
			return err
		}
		defer conn.Close()
		var req []byte
		if m.via == mechPCP {
			var nonce [12]byte
			rand.Read(nonce[:])
			clientIP := conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr()
			req = EncodePCPMap(m.Protocol, nonce, clientIP, m.InternalPort, 0, 0) // lease 0 = delete
		} else {
			req, err = EncodeNATPMPMap(m.Protocol, m.InternalPort, 0, 0) // lease 0, suggested 0 (§3.4)
			if err != nil {
				return err
			}
		}
		// A delete is fire-and-forget at the protocol level: the gateway's response, if any,
		// confirms nothing the caller acts on, and waiting for it is what a slow gateway would
		// stall on. Send, and let the lease be the backstop.
		_, err = conn.Write(req)
		return err
	}
}
