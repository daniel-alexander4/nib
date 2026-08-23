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
	// OnRequestSent is the send-time recorder; see SetOnRequestSent. A Client is constructed
	// per mapper (ceremonynet.go), so this is not shared state.
	OnRequestSent func(Mapping)
}

// SetOnRequestSent installs the SEND-TIME recorder: a callback fired the moment a mapping
// REQUEST has left this host, carrying a handle sufficient to delete whatever that request may
// have created — before any reply is known, and whether or not one ever arrives.
//
// **Why the send and not the reply.** Every error path here returns a zero Mapping, so a request
// that reached the router and then lost its answer left a mapping nothing could ever delete; it
// lived to lease expiry, and after the ceremony frees the internal port another process on this
// machine can bind it and be publicly reachable through the orphaned pinhole. P05.S07 T02's grill
// required "records the mapping the moment the request is SENT, not only on screened success",
// and nothing implemented it (/pending 257).
//
// The two sharpest cases are not lost replies at all — they are paths that hold a CONFIRMED
// mapping in a local variable and drop it: a NAT-PMP MAP that decoded before the follow-up
// external-address exchange failed, and a UPnP AddPortMapping that returned 200 before
// GetExternalIP failed. Both are ordinary, and both said so in their own comments.
//
// A recorded handle is "may exist", never "does exist": deleting a mapping that was never made
// is a no-op on all three mechanisms, which is what makes recording early the safe direction.
// The one exception is a definitive UPnP refusal, which is NOT recorded — see mapViaUPnP.
func (c *Client) SetOnRequestSent(fn func(Mapping)) { c.OnRequestSent = fn }

// sent fires the recorder if one is installed.
func (c *Client) sent(m Mapping) {
	if c.OnRequestSent != nil {
		c.OnRequestSent(m)
	}
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

// Refresh renews an existing mapping, asking the router to keep the same external port so the
// address already published stays valid (grill C7/P2). Returns the new Mapping — whose external
// port the caller must compare to the old one, because the router MAY assign a different port
// and that is item 20's stale-record case, not something Refresh can prevent.
func (c *Client) Refresh(ctx context.Context, m Mapping) (Mapping, netip.Addr, error) {
	nm, ext, err := c.mapWithSuggestion(ctx, m.Protocol, m.InternalPort, m.ExternalPort)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	// The new mapping carries its OWN mechanism and delete handle — mapWithSuggestion labels a
	// UPnP result with a freshly re-discovered control URL, and a socket-protocol result with
	// mechPCP/mechNATPMP (deleted by InternalPort, no URL needed). Copying the OLD mapping's via
	// forward would mislabel a refresh that now succeeds over PCP as UPnP, so a later Unmap would
	// aim the SOAP endpoint at a mapping the socket protocol holds — deleting the stale one and
	// leaking the live one. Trust nm's own labels.
	return nm, ext, nil
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

	// **A definitive REFUSAL is carried out, not flattened into "nothing answered".**
	// Every mechanism reports one the same way — NAT-PMP and PCP result codes, IGD's
	// UPnPError — and until now they all ended here as a bare ErrNoMapping, which made
	// "the router refused" and "no router answered" the same value from this line on. They
	// are opposite facts: a refusal proves the router IS the user's and IS reachable, which
	// is the one case where advising a manual port-forward is right (/pending 263).
	var refused error
	// PCP and NAT-PMP need the gateway; UPnP does not (it self-discovers over SSDP), so a
	// missing gateway skips the first two rather than ending the whole tier (diff-grill #3).
	if gw.IsValid() {
		if m, ext, err := c.tryGatewayProtocols(ctx, gw, proto, internalPort, suggestedExternal); err == nil {
			return m, ext, nil
		} else if ctx.Err() != nil {
			return Mapping{}, netip.Addr{}, ctx.Err()
		} else if errors.Is(err, ErrResultCode) {
			refused = err
		}
	}

	// UPnP-IGD last (D15's order) — the majority of consumer routers, and the most code. Gated
	// so it never fires from a zero-value Client: SSDP is real LAN multicast and would reach a
	// real IGD.
	if c.TryUPnP {
		if ext, port, ctl, st, err := mapViaUPnP(ctx, proto, internalPort, DefaultLeaseSec, isPrivateHost, c.sent, discoverIGD); err == nil {
			// LifetimeSec here is what we ASKED for and LifetimeObserved says so: IGD's
			// AddPortMapping response has no lease out-argument, so there is nothing to read
			// back. Reading it with a second GetSpecificPortMappingEntry round trip would not
			// fit the 3 s budget on the slowest mechanism; carrying the request as though it
			// were the answer is what the flag exists to stop.
			return Mapping{Protocol: proto, InternalPort: internalPort, ExternalPort: port, LifetimeSec: DefaultLeaseSec,
				LifetimeObserved: false,
				via:              mechUPnP, upnpControlURL: ctl, upnpServiceType: st}, ext, nil
		} else if ctx.Err() != nil {
			return Mapping{}, netip.Addr{}, ctx.Err()
		} else if errors.Is(err, ErrResultCode) {
			refused = err
		}
	}

	if refused != nil {
		return Mapping{}, netip.Addr{}, refused
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

	// A refusal is carried out of here too, and it has to be: flattening it one level lower
	// than mapWithSuggestion hides it just as completely (/pending 263). Found by the test —
	// the first draft of that fix only checked this function's RETURN, which was always
	// ErrNoMapping.
	var refused error
	if m, ext, err := c.tryPCP(ctx, conn, proto, clientIP, internalPort, suggestedExternal); err == nil {
		m.via = mechPCP
		return m, ext, nil
	} else if ctx.Err() != nil {
		return Mapping{}, netip.Addr{}, ctx.Err()
	} else if errors.Is(err, ErrResultCode) {
		refused = err
	}
	if m, ext, err := c.tryNATPMP(ctx, conn, proto, internalPort, suggestedExternal); err == nil {
		m.via = mechNATPMP
		return m, ext, nil
	} else if ctx.Err() != nil {
		return Mapping{}, netip.Addr{}, ctx.Err()
	} else if errors.Is(err, ErrResultCode) {
		refused = err
	}
	if refused != nil {
		return Mapping{}, netip.Addr{}, refused
	}
	return Mapping{}, netip.Addr{}, ErrNoMapping
}

// exchange sends req and returns the first response, retransmitting once inside perAttempt (or
// the ctx deadline, whichever is sooner) to ride out a single lost datagram.
func exchange(ctx context.Context, conn net.Conn, req []byte, onSent func()) ([]byte, error) {
	first := true
	deadline := time.Now().Add(perAttempt)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	buf := make([]byte, 1500)
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := conn.Write(req); err != nil {
			return nil, err
		}
		// The datagram is out. From here the router may have acted on it whatever we see next,
		// so the handle is recorded now — once, not per retransmit: the retry carries the same
		// nonce and ports, and both RFCs make a repeated MAP idempotent.
		if first && onSent != nil {
			onSent()
			first = false
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
	req := EncodePCPMap(proto, nonce, clientIP, internalPort, suggestedExternal, DefaultLeaseSec)
	resp, err := exchange(ctx, conn, req, func() {
		c.sent(Mapping{Protocol: proto, InternalPort: internalPort, via: mechPCP, nonce: nonce})
	})
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	m, ext, err := DecodePCPMap(resp, proto, nonce, internalPort)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	m.nonce = nonce // the delete needs it; see Mapping.nonce
	return m, ext, nil
}

func (c *Client) tryNATPMP(ctx context.Context, conn net.Conn, proto Protocol, internalPort, suggestedExternal uint16) (Mapping, netip.Addr, error) {
	req, err := EncodeNATPMPMap(proto, internalPort, suggestedExternal, DefaultLeaseSec)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	resp, err := exchange(ctx, conn, req, func() {
		c.sent(Mapping{Protocol: proto, InternalPort: internalPort, via: mechNATPMP})
	})
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
	extResp, err := exchange(ctx, conn, extReq, nil) // creates no mapping; nothing to record
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	ext, err := DecodeNATPMPExternalAddress(extResp)
	if err != nil {
		return Mapping{}, netip.Addr{}, err
	}
	return m, ext, nil
}

// DefaultLeaseSec is the requested lease.
//
// **Exported so a caller can name the number it asked for.** The server's diagnostics compare a
// granted lease against it (/pending 260), and a second copy of 120 in another package is the
// kind of duplicate that stops matching the day this one moves. Short by D15's law (the lease is refreshed while
// armed and must expire on its own after a crash), and the router may grant less. The refresh
// cadence and the delete-on-teardown are S07; this client requests one lease and reads what was
// granted (Mapping.LifetimeSec), which is what S07 schedules the refresh against.
const DefaultLeaseSec = 120

// ObserveLease asks the router what lease it actually granted, for the one mechanism that
// cannot say so in its reply.
//
// **UPnP-IGD's AddPortMapping has no lease out-argument**, so the mapping a UPnP obtain returns
// carries our REQUEST wearing the granted lease's name — on the mechanism D15 says most consumer
// routers run. `LifetimeObserved` recorded that honestly and nothing ever resolved it: an IGDv1
// that silently ignores NewLeaseDuration installs a PERMANENT mapping and answers 200, and
// nothing in the tree could tell that from a 120-second lease (/pending 260).
//
// It is a separate call rather than part of Map or Refresh, and that is measured rather than
// preferred: `discoverIGD` never returns early on a LOCATION, so it burns its full deadline out
// of a 3 s budget that then has to cover three SOAP round trips. A fourth does not fit. The
// caller runs this off that path, on its own context.
//
// Non-UPnP mechanisms report their lease in the MAP reply and are returned unchanged.
func (c *Client) ObserveLease(ctx context.Context, m Mapping) (Mapping, error) {
	if m.via != mechUPnP || m.LifetimeObserved {
		return m, nil
	}
	lease, leaseOK, err := verifiedUPnPEntry(ctx, igdHTTPClient(), m.upnpControlURL, m.upnpServiceType, m.Protocol, m.ExternalPort)
	if err != nil {
		return m, err // unreachable, unimplemented, or not ours — it stays unobserved
	}
	if !leaseOK {
		// The entry is ours and carries no readable lease. "Unobserved" is the honest state;
		// treating an absent tag as 0 would report a parse failure as a permanent mapping.
		return m, nil
	}
	m.LifetimeObserved = true
	if lease == 0 {
		m.LeasePermanent = true // and LifetimeSec stays put — see Mapping.LeasePermanent
		return m, nil
	}
	m.LifetimeSec = lease
	return m, nil
}

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
			// The mapping's OWN nonce, not a fresh one. PCP names a mapping by
			// (nonce, protocol, internal port), so a delete minting a new nonce asks the
			// router to remove a mapping that never existed — a silent no-op, on the success
			// path as much as anywhere else.
			clientIP := conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr()
			req = EncodePCPMap(m.Protocol, m.nonce, clientIP, m.InternalPort, 0, 0) // lease 0 = delete
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
