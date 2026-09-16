package portmap

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

// TestAReplyToTheRetransmissionDoesNotAnswerTheNextExchange — /pending 501.
//
// `exchange` returned the first datagram it read. A gateway slower than the 350 ms half-window
// answers the request AND its retransmission, so the second MAP reply was still queued when the
// external-address exchange began, which read it as its own answer — a wrong opcode — and a
// working router produced no mapping at all. Reproduced by the review before it was fixed.
func TestAReplyToTheRetransmissionDoesNotAnswerTheNextExchange(t *testing.T) {
	g := newMockGatewayWith(t, func(g *mockGateway) {
		g.silentPCP = true // NAT-PMP only: the path that makes a second exchange on one socket
		g.replyDelay = perAttempt/2 + 50*time.Millisecond
	})
	c := &Client{Gateway: mockAddrPort(t, g)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m, ext, err := c.Map(ctx, UDP, 40000)

	// STIMULUS: the MAP really was retransmitted, so a duplicate reply really was on its way.
	// Without two MAP requests there is no stale datagram and nothing below is about one.
	maps := 0
	for drained := false; !drained; {
		select {
		case l := <-g.leases:
			if l > 0 {
				maps++
			}
		default:
			drained = true
		}
	}
	if maps < 2 {
		t.Fatalf("setup: the gateway saw %d MAP request(s); a slow gateway must have been sent the "+
			"retransmission, or there is no duplicate reply to mistake for an answer", maps)
	}

	if err != nil {
		t.Fatalf("a working NAT-PMP gateway %v slow yielded no mapping: %v — the retransmission's "+
			"duplicate reply was read as the external-address answer", g.replyDelay, err)
	}
	if ext != g.extIP {
		t.Errorf("external IP %v, want %v", ext, g.extIP)
	}
	if m.ExternalPort != g.extPort {
		t.Errorf("external port %d, want %d", m.ExternalPort, g.extPort)
	}
}

// TestAStaleRefusalDoesNotAnswerAPCPRequest — /pending 501.
//
// Two defects composed here. `DecodePCPMap` read the result code before the nonce, so ANY
// datagram carrying a non-zero code was a refusal whatever it echoed; and `exchange` took the
// first datagram. A NOT_AUTHORIZED reply to some other request therefore ended a PCP obtain as
// "the router refused" — the one outcome that advises a manual port-forward. RFC 6887 §8.3: a
// response that does not match "is ignored".
func TestAStaleRefusalDoesNotAnswerAPCPRequest(t *testing.T) {
	g := newMockGatewayWith(t, func(g *mockGateway) { g.staleRefusalFirst = true })
	c := &Client{Gateway: mockAddrPort(t, g)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	m, _, err := c.Map(ctx, UDP, 40404)

	// STIMULUS: the stale refusal was sent ahead of the real answer.
	if g.stalesSent.Load() == 0 {
		t.Fatal("setup: no stale refusal was sent, so nothing here was offered a datagram to ignore")
	}
	if errors.Is(err, ErrResultCode) {
		t.Fatalf("a refusal echoing another request's nonce was carried out as the router refusing "+
			"this one: %v", err)
	}
	if err != nil {
		t.Fatalf("no mapping: %v", err)
	}
	if !m.SameTarget(Mapping{Protocol: UDP, InternalPort: 40404, via: mechPCP}) {
		t.Errorf("the mapping did not come from PCP — the stale datagram ended the PCP exchange " +
			"and the client fell back to NAT-PMP past a router that had answered PCP correctly")
	}
	if m.ExternalPort != g.extPort {
		t.Errorf("external port %d, want %d", m.ExternalPort, g.extPort)
	}
}

// TestAPCPRefreshRenewsUnderTheMappingsOwnNonce — /pending 501.
//
// RFC 6887 §11.2.1: "When renewing a mapping, the PCP client MUST use the same Mapping Nonce
// value that was used in the original mapping request." `Refresh` minted a fresh one, and §11.3
// has a conforming server answer that with NOT_AUTHORIZED — so a refresh against a real PCP router
// was refused, and the lease ran out under a mapping the caller believed renewed.
func TestAPCPRefreshRenewsUnderTheMappingsOwnNonce(t *testing.T) {
	g := newMockGatewayWith(t, func(g *mockGateway) { g.enforceNonce = true })
	c := &Client{Gateway: mockAddrPort(t, g)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	m, _, err := c.Map(ctx, UDP, 40404)
	if err != nil {
		t.Fatal(err)
	}
	if !m.SameTarget(Mapping{Protocol: UDP, InternalPort: 40404, via: mechPCP}) {
		t.Fatal("setup: the obtain did not come from PCP, so the nonce is not the subject of this row")
	}
	nextNonce := func(what string) [12]byte {
		t.Helper()
		select {
		case n := <-g.nonces:
			return n
		case <-time.After(3 * time.Second):
			t.Fatalf("the gateway saw no PCP request for the %s", what)
			return [12]byte{}
		}
	}
	obtained := nextNonce("obtain")

	// STIMULUS: the fixture really refuses a different nonce. Without this, a mock that accepted
	// anything would let a nonce-minting Refresh pass the assertions below.
	gw := mockAddrPort(t, g)
	conn, err := net.Dial("udp", gw.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var other [12]byte
	other[0] = obtained[0] ^ 0xff
	clientIP := conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr()
	probe := EncodePCPMap(UDP, other, clientIP, 40404, 0, DefaultLeaseSec)
	resp := roundtrip(t, gw.String(), probe)
	nextNonce("probe")
	if _, _, derr := DecodePCPMap(resp, UDP, other, 40404); !errors.Is(derr, ErrResultCode) {
		t.Fatalf("setup: the gateway accepted a MAP with a different nonce (%v), so it cannot tell "+
			"a correct refresh from a nonce-minting one", derr)
	}

	nm, _, err := c.Refresh(ctx, m)
	renewed := nextNonce("refresh")
	if renewed != obtained {
		t.Errorf("the refresh carried nonce %x but the mapping was created with %x — RFC 6887 "+
			"§11.2.1 requires the same nonce, and §11.3 refuses a different one", renewed, obtained)
	}
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !nm.SameTarget(Mapping{Protocol: UDP, InternalPort: 40404, via: mechPCP}) {
		t.Error("the refresh did not renew over PCP — the router refused it and the client fell back")
	}
	if nm.nonce != obtained {
		t.Errorf("the renewed mapping carries nonce %x, want %x — its later delete would name "+
			"a mapping that does not exist", nm.nonce, obtained)
	}
}

// TestANATPMPRefusalForAnotherPortIsNotThisRequestsRefusal — /pending 501.
//
// `DecodeNATPMPMap` read the result code before the internal port, so a refusal naming another
// mapping was this request's refusal. RFC 6886 §3.3 has an error response carry "the requested
// mapping", so the port is there to match before the code is believed — and `exchange` now reads
// past a datagram that does not match.
func TestANATPMPRefusalForAnotherPortIsNotThisRequestsRefusal(t *testing.T) {
	g := newMockGatewayRefusing(t, 2, 0)
	req, err := EncodeNATPMPMap(UDP, 40001, 0, DefaultLeaseSec)
	if err != nil {
		t.Fatal(err)
	}
	resp := roundtrip(t, g.addr(), req)

	// STIMULUS: the datagram really is a refusal, for port 40001.
	if _, derr := DecodeNATPMPMap(resp, UDP, 40001); !errors.Is(derr, ErrResultCode) {
		t.Fatalf("setup: the gateway's reply is not a refusal for its own port (%v)", derr)
	}
	_, derr := DecodeNATPMPMap(resp, UDP, 40404)
	if errors.Is(derr, ErrResultCode) {
		t.Errorf("a refusal naming port 40001 decoded as a refusal of a request for 40404: %v", derr)
	}
	if !errors.Is(derr, ErrOpcode) {
		t.Errorf("decode = %v; want the mismatch reported as not answering the request", derr)
	}
}

// TestSSDPLocationMustNameItsSender — /pending 501.
//
// `discoverIGD` threw the SSDP source address away, so a responder on the link could name any
// other private host as the IGD and Nib would GET its description and POST SOAP at it.
func TestSSDPLocationMustNameItsSender(t *testing.T) {
	resp := func(loc string) []byte {
		return []byte("HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\nLOCATION: " + loc + "\r\n\r\n")
	}
	from := func(ip string) net.Addr { return net.UDPAddrFromAddrPort(netip.MustParseAddrPort(ip)) }
	cases := []struct {
		name string
		src  net.Addr
		loc  string
		want bool
	}{
		{"the sender's own address", from("192.168.1.1:1900"), "http://192.168.1.1:5000/rootDesc.xml", true},
		{"a v4-mapped sender", net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr("::ffff:192.168.1.1"), 1900)), "http://192.168.1.1:5000/rootDesc.xml", true},
		{"another private host", from("192.168.1.66:1900"), "http://192.168.1.1:5000/rootDesc.xml", false},
		{"a hostname", from("192.168.1.1:1900"), "http://router.lan:5000/rootDesc.xml", false},
		{"no UDP source", &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 1900}, "http://192.168.1.1:5000/rootDesc.xml", false},
	}
	for _, tc := range cases {
		got := ssdpLocationFrom(tc.src, resp(tc.loc))
		if (got != "") != tc.want {
			t.Errorf("%s: got %q, want accepted=%v", tc.name, got, tc.want)
		}
		if tc.want && got != tc.loc {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.loc)
		}
	}
}
