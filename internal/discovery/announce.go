// Package discovery is tier 1 of the connection ladder (D8): two Nibs on the same
// link find each other with no address typed and no internet.
//
// # What an announcement may say, and why it is so little
//
// L1 is the law here: "the rendezvous path affects reachability only, never
// identity." Nothing learned from multicast may influence WHICH peer is accepted —
// acceptance is the pinned fingerprint at the TLS handshake plus the spoken
// verification string, and neither is reachable from this package. An announcement
// that lies can waste two seconds; it can never substitute a signer.
//
// So an announcement carries exactly two facts and a nonce:
//
//   - the sender's six-word NAME (D3), which is a public encoding of 66 bits of
//     their fingerprint and is already displayed on their own screen;
//   - the port their armed session is listening on, AND the transport it speaks;
//   - a per-instance nonce, which exists only so a sender can recognise its own
//     announcement coming back (see below).
//
// # Why the transport is part of the address (ADR-010, format version 2)
//
// A port without its transport is not an address. The armed side listens on exactly
// one transport, chosen at arm time, and under QUIC its port is a **UDP** port while
// under TCP it is a TCP port — the same number means two different sockets. Version 1
// carried the number alone, so the dialing side picked a transport from its own
// request and a QUIC-armed peer discovered on the link was dialled over TCP: a
// connection refused, reported to the user as "that peer is not reachable".
//
// It stayed latent because the web client never sends a transport at all (so the GUI
// was TCP-only end to end) and because `build/pairrepro.sh` passes `-F transport=` to
// BOTH sides out of band, so tier 4 could not see the disagreement it was configured
// past. Two harnesses agreeing because they were told the same answer is not the same
// as the protocol carrying it.
//
// This is reachability, never identity, so L1 is untouched: the transport says how to
// open a socket and has no bearing on which peer is accepted at the handshake.
//
// It carries the name and not the fingerprint because that is what the receiving
// side can actually use: pairing.Matches(fp, name) recomputes the name from a
// fingerprint the receiver ALREADY HOLDS. There is no way to go from a name to a
// fingerprint — pairing.decode is unexported on purpose, because "a half-fingerprint
// that looks like an identity is the misuse this design removed". That export
// boundary is what makes L1 structural here rather than a rule someone has to
// remember: a browser can only recognise peers it has already pinned, and discovery
// therefore cannot serve first contact at all. Two strangers still need an
// invitation (D21).
//
// # Why the nonce, and why not the source address
//
// IP_MULTICAST_LOOP defaults to ON, so a sender hears its own announcement. That is
// deliberate and must stay: it is the only firewall-independent liveness check on
// the send path, and it is what lets two instances on one host discover each other.
// But it means every browser must discard its own announcements, and the source
// address is the wrong discriminator — on a multi-homed machine the copy arrives
// from whichever interface address was used, so recognising it means enumerating
// every local address and re-checking on every change. A nonce minted once per
// process is exact and costs nothing.
//
// # The socket is internet-facing, on every platform
//
// Go binds a multicast listener to the WILDCARD, not to the group: net's
// sock_posix.go rewrites a multicast bind address to 0.0.0.0, deliberately and with
// a comment saying so. Both net.ListenMulticastUDP and net.ListenPacket+JoinGroup
// therefore accept ordinary unicast on that port from anyone who can route to the
// host — measured, not assumed. Binding the group address does refuse unicast, but
// only on Linux and only through a raw-socket construct behind a //go:build tag,
// which is the exact shape that has already produced one silent defect in this repo.
//
// So this parser assumes hostile input as a baseline rather than as a hardening
// pass: a fixed-size header read before anything is allocated on the content, a hard
// length cap, and no state created from an unauthenticated datagram.
package discovery

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"nib/internal/pairing"
)

const (
	// magic prefixes every announcement so a datagram that is not ours is rejected
	// on its first four bytes. This socket shares a group with whatever else the
	// network is doing, and a parser that has to reason about the payload before
	// deciding it is foreign is a parser doing work an attacker chose.
	magic = "NIBd"

	// version is the announcement format version. An announcement whose version is
	// not this one is refused rather than best-guessed: a version field that is
	// read and then ignored is decoration, and this is a wire format other Nibs
	// parse (ADR-007).
	//
	// **2 since ADR-010**, which added the transport byte. Refused and not
	// best-guessed for the reason above and one more: a version-1 announcement's
	// port could be either transport, and guessing is exactly the defect the field
	// was added to remove. Nib has no users and forbids compatibility shims, so
	// there is no version-1 speaker to keep working.
	version = 2

	// nonceLen is the per-instance self-recognition nonce.
	nonceLen = 8

	// MaxDatagram bounds a whole announcement. It is far larger than the biggest
	// legal one (a six-word name is at most 6*8+5 = 53 bytes) and far smaller than
	// a UDP datagram, so the cap costs nothing legitimate and denies an attacker
	// the ability to make this package allocate.
	MaxDatagram = 256

	// The field offsets, named ONCE.
	//
	// They were arithmetic written inline in `Parse` and again in the test that
	// mutates a datagram field by field. Adding the transport byte moved the port and
	// the test went on patching the old offset — so "zero port" set the transport to 0
	// and the port's high byte to 0, left a port of 251, and the parser accepted it.
	// A refusal test that silently stops testing its own refusal is the worst shape a
	// guard can take, and the cause was one number in two places (ADR-009).
	offVersion   = len(magic)
	offTransport = offVersion + 1
	offPort      = offTransport + 1
	offNonce     = offPort + 2
	offNameLen   = offNonce + nonceLen

	// headerLen is magic + version + transport + port + nonce + name length.
	headerLen = offNameLen + 1
)

var (
	// ErrNotOurs reports a datagram that is not a Nib announcement at all. It is
	// separated from ErrMalformed because the two mean different things to a
	// diagnostic: foreign traffic on a shared group is expected and uninteresting,
	// and a malformed NIBd datagram is worth counting.
	ErrNotOurs = errors.New("not a nib announcement")

	// ErrMalformed reports a datagram that claims to be ours and is not valid.
	ErrMalformed = errors.New("malformed announcement")

	// ErrVersion reports an announcement from a Nib speaking a version this one
	// does not. Distinct from ErrMalformed so the diagnostic can say "there is a
	// Nib here and we cannot talk to it", which is a very different message.
	ErrVersion = errors.New("unsupported announcement version")
)

// Transport names the socket an armed session is listening on, as one wire byte.
//
// It is an enumeration on the wire and not a string, so the parser's whole job is a
// range check: a name would let an announcement choose how many bytes this package
// allocates and how they are compared, for a field with two legal values.
//
// **These constants are NOT internal/p2p's**, and the duplication is deliberate rather
// than an oversight. `TestNothingHereCanReachAnIdentity` forbids this package importing
// p2p, vault or sign — that guard is what makes L1 structural instead of remembered —
// so the wire encoding is owned here and `internal/server` maps between the two at the
// layer that holds both. A shared constant would be a shared import.
type Transport uint8

const (
	// TransportTCP is zero so the zero Announcement is a TCP one, matching the
	// server's own rule that an absent transport means TCP.
	TransportTCP  Transport = 0
	TransportQUIC Transport = 1
)

func (t Transport) String() string {
	switch t {
	case TransportTCP:
		return "tcp"
	case TransportQUIC:
		return "quic"
	default:
		return fmt.Sprintf("transport(%d)", uint8(t))
	}
}

// valid reports whether t is a transport this version of the format defines.
func (t Transport) valid() bool { return t == TransportTCP || t == TransportQUIC }

// Announcement is what an armed Nib says on the link.
//
// There is no fingerprint field and there is no address field. The fingerprint is
// L1's whole point. The address is not carried because the receiver already has a
// better one: the source address of the datagram itself, which cannot be forged
// past the point where it would also have to receive the reply.
type Announcement struct {
	// Name is the sender's six-word pairing name (D3).
	Name string
	// Port is where the sender's armed session is listening.
	Port uint16
	// Transport is the socket that port belongs to. A UDP port and a TCP port with
	// the same number are different endpoints, so this travels WITH the port.
	Transport Transport
	// Nonce identifies the sending PROCESS, so it can discard its own copies.
	Nonce [nonceLen]byte
}

// Encode renders an announcement. It refuses to encode one that would not parse,
// because a sender that emits garbage is harder to diagnose than one that fails.
func (a Announcement) Encode() ([]byte, error) {
	if err := a.check(); err != nil {
		return nil, err
	}
	out := make([]byte, 0, headerLen+len(a.Name))
	out = append(out, magic...)
	out = append(out, version)
	out = append(out, byte(a.Transport))
	out = binary.BigEndian.AppendUint16(out, a.Port)
	out = append(out, a.Nonce[:]...)
	out = append(out, byte(len(a.Name)))
	out = append(out, a.Name...)
	return out, nil
}

// check is the one place an announcement's shape is decided, so the encoder and the
// parser cannot drift into disagreeing about what is legal.
func (a Announcement) check() error {
	if a.Port == 0 {
		return fmt.Errorf("%w: no port", ErrMalformed)
	}
	// In `check` and therefore on BOTH sides, like every other rule here: this
	// function exists so the encoder and the parser cannot drift into disagreeing
	// about what is legal, and a transport validated only on parse would let this
	// Nib emit an announcement it would itself refuse.
	if !a.Transport.valid() {
		return fmt.Errorf("%w: unknown transport %d", ErrMalformed, uint8(a.Transport))
	}
	if len(strings.Fields(a.Name)) != pairing.NameWords {
		return fmt.Errorf("%w: name is not %d words", ErrMalformed, pairing.NameWords)
	}
	if len(a.Name) > 0xff {
		return fmt.Errorf("%w: name too long", ErrMalformed)
	}
	// Against the DATAGRAM cap, not just the length byte. Encode's doc promises it
	// "refuses to encode one that would not parse", and bounding the name at 0xff alone
	// did not keep that promise: six 40-character words is 245 bytes, encodes to 261,
	// and Parse rejects it as over the 256-byte cap. The two ends of the format
	// disagreed about what is legal, which is the one thing this function exists to
	// prevent. Unreachable from the product only because pairing.Name is at most 53
	// bytes — an invariant held by a caller, not by the check that claims to hold it.
	if headerLen+len(a.Name) > MaxDatagram {
		return fmt.Errorf("%w: %d bytes exceeds the %d-byte cap", ErrMalformed,
			headerLen+len(a.Name), MaxDatagram)
	}
	return nil
}

// Parse reads an announcement off the wire.
//
// It reads the fixed header before touching the payload, and every length it uses
// comes from a bound it checked first. b is not retained: the returned Name is a
// fresh string, so a caller may reuse its read buffer, which is the normal shape for
// a UDP read loop and a source of aliasing bugs when a parser forgets.
// Every error path returns the ZERO announcement, never the partially-filled one.
// That is not tidiness: the first draft returned what it had parsed so far, so a
// caller who ignored the error — the ordinary mistake in a read loop — would act on
// an attacker-chosen port and nonce. Caught by a test asserting the returned value
// on the refusal paths, which is a check worth having anywhere a parser has more
// than one way out.
func Parse(b []byte) (Announcement, error) {
	var a Announcement
	if len(b) > MaxDatagram {
		return Announcement{}, fmt.Errorf("%w: %d bytes exceeds the %d-byte cap", ErrMalformed, len(b), MaxDatagram)
	}
	if len(b) < headerLen {
		// Too short even to be ours: this is the common case on a shared group and
		// must be the cheapest path through the function.
		return Announcement{}, ErrNotOurs
	}
	if string(b[:len(magic)]) != magic {
		return Announcement{}, ErrNotOurs
	}
	if b[offVersion] != version {
		return Announcement{}, fmt.Errorf("%w: %d, want %d", ErrVersion, b[offVersion], version)
	}
	a.Transport = Transport(b[offTransport])
	a.Port = binary.BigEndian.Uint16(b[offPort : offPort+2])
	copy(a.Nonce[:], b[offNonce:offNonce+nonceLen])
	n := int(b[offNameLen])
	i := headerLen
	if len(b)-i != n {
		// Exact, not "at least": trailing bytes are as much a malformed
		// announcement as missing ones, and accepting them would let a sender
		// smuggle a payload past every other check here.
		return Announcement{}, fmt.Errorf("%w: declared %d name bytes, %d remain", ErrMalformed, n, len(b)-i)
	}
	a.Name = string(b[i:])
	if err := a.check(); err != nil {
		return Announcement{}, err
	}
	return a, nil
}
