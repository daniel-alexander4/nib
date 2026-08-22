// Package portmap is Nib's native NAT-PMP (RFC 6886) and PCP (RFC 6887) client — the
// wire codec for asking a home router for a temporary inbound port mapping.
//
// # Why native, and why this file is caveat 6 discharged
//
// D15 makes router port mapping a ladder tier (D8 tier 3), and caveat 6 records that no Go
// port-mapping dependency is in the tree and its licence-compatibility is an *unverified
// assumption*. The two protocols this file speaks are small, well-specified binary
// request/response exchanges to the gateway on UDP 5351 — so the honest way to discharge a
// caveat about a dependency's licence is to **not take the dependency**: the whole mechanism
// is written here, against the RFCs, and there is no third party to vet.
//
// UPnP-IGD (SSDP + SOAP over HTTP) is an order more code and is the tier's narrower-coverage
// arm (S06.T05), not this file.
//
// # What this file is and is not
//
// It is **pure**: `[]byte` ↔ struct, no sockets, no clock, no randomness beyond a nonce the
// caller supplies. That is what lets the request/response pair be driven end to end against an
// in-process mock gateway in the test, rather than a real router — the same reason S05's v6
// analogue runs over loopback. The socket, the 3 s budget, the gateway discovery and the
// armed-only lifecycle are the client (S06.T03) and the lease lifecycle (S07); this is the
// codec they are built on.
package portmap

import (
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

// Port 5351 is where both NAT-PMP and PCP servers listen, on the default gateway.
const GatewayPort = 5351

// Protocol is the transport a mapping is for. A NAT keeps separate mapping tables per
// protocol (RFC 6886 §3.3), which is why D15 requires both when both transports are offered.
type Protocol uint8

const (
	UDP Protocol = 17 // the PCP protocol number; NAT-PMP encodes it as opcode 1
	TCP Protocol = 6  // the PCP protocol number; NAT-PMP encodes it as opcode 2
)

func (p Protocol) String() string {
	switch p {
	case UDP:
		return "udp"
	case TCP:
		return "tcp"
	}
	return fmt.Sprintf("protocol(%d)", uint8(p))
}

// Mapping is the result of a successful request, in a form both protocols map onto: the
// external port the router assigned and the lease it granted. The external IP is carried
// separately (NAT-PMP learns it from a distinct request; PCP returns it inline).
type Mapping struct {
	Protocol     Protocol
	InternalPort uint16
	ExternalPort uint16
	LifetimeSec  uint32

	// The delete handle (S07): which mechanism granted the mapping, and — for UPnP — the SOAP
	// control URL and service type the delete needs. Without these a UPnP mapping cannot be
	// deleted at all (grill C1); the socket protocols need only the internal port and the
	// gateway, which the Client already holds.
	via             mechanism
	upnpControlURL  string
	upnpServiceType string
}

// mechanism is which of the three protocols produced a mapping, so its delete goes out the same
// way it came in.
type mechanism uint8

const (
	mechPCP mechanism = iota
	mechNATPMP
	mechUPnP
)

// ── NAT-PMP (RFC 6886) ───────────────────────────────────────────────────────
//
// The simplest of the two: a 12-byte MAP request, a 16-byte MAP response, and a separate
// 2-byte request for the external address (a NAT-PMP client does not learn its public IP
// from the MAP exchange). Deleting a mapping is a MAP with lifetime 0 and suggested external
// port 0 (§3.4).

const natpmpVersion = 0

var (
	ErrShort         = errors.New("portmap: response too short")
	ErrVersion       = errors.New("portmap: wrong protocol version in response")
	ErrOpcode        = errors.New("portmap: response opcode does not answer the request")
	ErrResultCode    = errors.New("portmap: gateway refused the request")
	ErrNonceMismatch = errors.New("portmap: PCP response nonce does not match the request")
)

// natpmpOpcode maps a transport onto NAT-PMP's request opcode (1 = UDP, 2 = TCP, §3.3).
func natpmpOpcode(p Protocol) (byte, error) {
	switch p {
	case UDP:
		return 1, nil
	case TCP:
		return 2, nil
	}
	return 0, fmt.Errorf("portmap: NAT-PMP has no opcode for %s", p)
}

// EncodeNATPMPMap builds a NAT-PMP MAP request. A lifetime of 0 with externalPort 0 is the
// delete form (§3.4); the caller passes internalPort=the port being released for that.
func EncodeNATPMPMap(p Protocol, internalPort, suggestedExternalPort uint16, lifetimeSec uint32) ([]byte, error) {
	op, err := natpmpOpcode(p)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 12)
	b[0] = natpmpVersion
	b[1] = op
	// b[2:4] reserved, zero.
	binary.BigEndian.PutUint16(b[4:6], internalPort)
	binary.BigEndian.PutUint16(b[6:8], suggestedExternalPort)
	binary.BigEndian.PutUint32(b[8:12], lifetimeSec)
	return b, nil
}

// DecodeNATPMPMap parses a NAT-PMP MAP response and checks it answers a request for p and
// internalPort. A non-zero result code is a refusal, surfaced rather than swallowed — a
// gateway that says "not authorized" (code 2) is a different tier outcome from one that
// never answered, and D19's diagnosis depends on telling them apart.
func DecodeNATPMPMap(resp []byte, p Protocol, internalPort uint16) (Mapping, error) {
	if len(resp) < 16 {
		return Mapping{}, ErrShort
	}
	if resp[0] != natpmpVersion {
		return Mapping{}, ErrVersion
	}
	op, err := natpmpOpcode(p)
	if err != nil {
		return Mapping{}, err
	}
	// The response opcode is the request opcode + 128 (§3.3).
	if resp[1] != op+128 {
		return Mapping{}, fmt.Errorf("%w: got %d, want %d", ErrOpcode, resp[1], op+128)
	}
	if code := binary.BigEndian.Uint16(resp[2:4]); code != 0 {
		return Mapping{}, fmt.Errorf("%w: NAT-PMP result code %d", ErrResultCode, code)
	}
	got := binary.BigEndian.Uint16(resp[8:10])
	if got != internalPort {
		return Mapping{}, fmt.Errorf("%w: response maps internal port %d, we asked for %d", ErrOpcode, got, internalPort)
	}
	return Mapping{
		Protocol:     p,
		InternalPort: got,
		ExternalPort: binary.BigEndian.Uint16(resp[10:12]),
		LifetimeSec:  binary.BigEndian.Uint32(resp[12:16]),
	}, nil
}

// EncodeNATPMPExternalAddress builds the 2-byte request for the gateway's public IPv4 (op 0).
func EncodeNATPMPExternalAddress() []byte {
	return []byte{natpmpVersion, 0}
}

// DecodeNATPMPExternalAddress parses the external-address response (op 128).
func DecodeNATPMPExternalAddress(resp []byte) (netip.Addr, error) {
	if len(resp) < 12 {
		return netip.Addr{}, ErrShort
	}
	if resp[0] != natpmpVersion {
		return netip.Addr{}, ErrVersion
	}
	if resp[1] != 128 {
		return netip.Addr{}, fmt.Errorf("%w: got %d, want 128", ErrOpcode, resp[1])
	}
	if code := binary.BigEndian.Uint16(resp[2:4]); code != 0 {
		return netip.Addr{}, fmt.Errorf("%w: NAT-PMP result code %d", ErrResultCode, code)
	}
	return netip.AddrFrom4([4]byte{resp[8], resp[9], resp[10], resp[11]}), nil
}

// ── PCP (RFC 6887) ───────────────────────────────────────────────────────────
//
// PCP supersedes NAT-PMP and is tried first. Its MAP exchange is a 24-byte common header plus
// a 36-byte MAP opcode body, and unlike NAT-PMP it returns the assigned external IP inline, so
// no second request is needed. The client supplies a 12-byte nonce that the response must echo
// — it binds a response to this request and is checked in constant time.

const (
	pcpVersion   = 2
	pcpOpcodeMap = 1
	// The R bit is the top bit of the opcode byte: 0 in a request, 1 in a response.
	pcpResponseBit = 0x80
)

// EncodePCPMap builds a PCP MAP request. clientIP is this host's internal address (the header
// carries it so a multi-homed gateway knows which the mapping is for); nonce is 12 random
// bytes the caller generates and keeps to match the response.
func EncodePCPMap(p Protocol, nonce [12]byte, clientIP netip.Addr, internalPort, suggestedExternalPort uint16, lifetimeSec uint32) []byte {
	b := make([]byte, 24+36)
	b[0] = pcpVersion
	b[1] = pcpOpcodeMap // R bit 0 = request
	// b[2:4] reserved.
	binary.BigEndian.PutUint32(b[4:8], lifetimeSec)
	cip := clientIP.As16()
	copy(b[8:24], cip[:]) // v4 addresses go in as v4-mapped v6

	body := b[24:]
	copy(body[0:12], nonce[:])
	body[12] = byte(p)
	// body[13:16] reserved.
	binary.BigEndian.PutUint16(body[16:18], internalPort)
	binary.BigEndian.PutUint16(body[18:20], suggestedExternalPort)
	sip := suggestedExternalIP(suggestedExternalPort).As16()
	copy(body[20:36], sip[:])
	return b
}

// suggestedExternalIP returns the all-zero address, which asks the gateway to choose — the
// normal case. Kept a function so a future caller that wants to request a specific external IP
// has one place to change.
func suggestedExternalIP(uint16) netip.Addr { return netip.IPv6Unspecified() }

// DecodePCPMap parses a PCP MAP response, checks the version, the response bit, the result
// code, and — critically — that the nonce echoes the request's.
func DecodePCPMap(resp []byte, p Protocol, nonce [12]byte, internalPort uint16) (Mapping, netip.Addr, error) {
	if len(resp) < 24+36 {
		return Mapping{}, netip.Addr{}, ErrShort
	}
	if resp[0] != pcpVersion {
		return Mapping{}, netip.Addr{}, ErrVersion
	}
	if resp[1] != (pcpResponseBit | pcpOpcodeMap) {
		return Mapping{}, netip.Addr{}, fmt.Errorf("%w: opcode byte %#x is not a MAP response", ErrOpcode, resp[1])
	}
	if code := resp[3]; code != 0 {
		return Mapping{}, netip.Addr{}, fmt.Errorf("%w: PCP result code %d", ErrResultCode, code)
	}
	lifetime := binary.BigEndian.Uint32(resp[4:8])

	body := resp[24:]
	// The nonce echo is the response's binding to this request. Constant-time because it is a
	// value an off-path attacker would have to guess to spoof a mapping reply.
	if subtle.ConstantTimeCompare(body[0:12], nonce[:]) != 1 {
		return Mapping{}, netip.Addr{}, ErrNonceMismatch
	}
	if Protocol(body[12]) != p {
		return Mapping{}, netip.Addr{}, fmt.Errorf("%w: response is for protocol %d, we asked for %s", ErrOpcode, body[12], p)
	}
	gotInternal := binary.BigEndian.Uint16(body[16:18])
	if gotInternal != internalPort {
		return Mapping{}, netip.Addr{}, fmt.Errorf("%w: response maps internal port %d, we asked for %d", ErrOpcode, gotInternal, internalPort)
	}
	externalPort := binary.BigEndian.Uint16(body[18:20])
	var ext16 [16]byte
	copy(ext16[:], body[20:36])
	extIP := netip.AddrFrom16(ext16).Unmap()
	return Mapping{
		Protocol:     p,
		InternalPort: gotInternal,
		ExternalPort: externalPort,
		LifetimeSec:  lifetime,
	}, extIP, nil
}
