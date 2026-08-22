package portmap

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

// A mock gateway speaking BOTH protocols on one UDP socket, so the codec is driven end to end
// rather than round-tripped against itself. It is the caveat-6 discharge's own proof: the whole
// dependency is here, and here is where it is exercised against something that behaves like a
// router rather than like the encoder.
//
// It grants a fixed external port (the "router's choice"), which is what lets the test assert
// the client reads the ASSIGNED port and not the suggested one — a decoder that returned the
// value it sent would pass a self-round-trip and fail here.
type mockGateway struct {
	pc           net.PacketConn
	extPort      uint16      // the port the router assigns, deliberately != suggested
	extIP        netip.Addr  // the public IP the router reports
	natpmpResult uint16      // non-zero to simulate a refusal
	pcpResult    byte        // non-zero to simulate a refusal
	silentPCP    bool        // drop PCP requests, to force NAT-PMP fallback
	leases       chan uint32 // records the lease of each MAP request seen (0 = a delete)
}

func newMockGateway(t *testing.T) *mockGateway { return newMockGatewayRefusing(t, 0, 0) }

// newMockGatewaySilentPCP answers NAT-PMP but drops PCP, to drive the fallback path. The flag
// is fixed before the serve goroutine starts, so nothing writes it after a reader exists.
func newMockGatewaySilentPCP(t *testing.T) *mockGateway {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &mockGateway{pc: pc, extPort: 51234, extIP: netip.MustParseAddr("203.0.113.7"), silentPCP: true, leases: make(chan uint32, 8)}
	t.Cleanup(func() { pc.Close() })
	go g.serve()
	return g
}

// newMockGatewayRefusing fixes the result codes BEFORE the serve goroutine starts, so the
// mock's fields are never written after a reader exists — the codes are config, not state.
func newMockGatewayRefusing(t *testing.T, natpmp uint16, pcp byte) *mockGateway {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &mockGateway{pc: pc, extPort: 51234, extIP: netip.MustParseAddr("203.0.113.7"), natpmpResult: natpmp, pcpResult: pcp, leases: make(chan uint32, 8)}
	t.Cleanup(func() { pc.Close() })
	go g.serve()
	return g
}

func (g *mockGateway) addr() string { return g.pc.LocalAddr().String() }

func (g *mockGateway) serve() {
	buf := make([]byte, 1500)
	for {
		n, from, err := g.pc.ReadFrom(buf)
		if err != nil {
			return
		}
		req := buf[:n]
		var resp []byte
		switch {
		case req[0] == pcpVersion:
			if g.silentPCP {
				continue // drop it, forcing the client to fall back to NAT-PMP
			}
			resp = g.replyPCP(req)
		case req[0] == natpmpVersion && n == 2: // external-address request
			resp = g.replyNATPMPExternal()
		case req[0] == natpmpVersion:
			resp = g.replyNATPMPMap(req)
		}
		if resp != nil {
			g.pc.WriteTo(resp, from)
		}
	}
}

func (g *mockGateway) replyNATPMPMap(req []byte) []byte {
	g.recordLease(binary.BigEndian.Uint32(req[8:12]))
	resp := make([]byte, 16)
	resp[0] = natpmpVersion
	resp[1] = req[1] + 128
	binary.BigEndian.PutUint16(resp[2:4], g.natpmpResult)
	binary.BigEndian.PutUint32(resp[4:8], 1000) // epoch
	copy(resp[8:10], req[4:6])                  // echo internal port
	binary.BigEndian.PutUint16(resp[10:12], g.extPort)
	binary.BigEndian.PutUint32(resp[12:16], binary.BigEndian.Uint32(req[8:12])) // echo lifetime
	return resp
}

func (g *mockGateway) replyNATPMPExternal() []byte {
	resp := make([]byte, 12)
	resp[0] = natpmpVersion
	resp[1] = 128
	binary.BigEndian.PutUint32(resp[4:8], 1000)
	v4 := g.extIP.As4()
	copy(resp[8:12], v4[:])
	return resp
}

func (g *mockGateway) replyPCP(req []byte) []byte {
	g.recordLease(binary.BigEndian.Uint32(req[4:8]))
	resp := make([]byte, 24+36)
	resp[0] = pcpVersion
	resp[1] = pcpResponseBit | pcpOpcodeMap
	resp[3] = g.pcpResult
	binary.BigEndian.PutUint32(resp[4:8], binary.BigEndian.Uint32(req[4:8])) // echo lifetime
	binary.BigEndian.PutUint32(resp[8:12], 1000)                             // epoch
	body, rbody := resp[24:], req[24:]
	copy(body[0:12], rbody[0:12])   // echo the nonce
	body[12] = rbody[12]            // echo protocol
	copy(body[16:18], rbody[16:18]) // echo internal port
	binary.BigEndian.PutUint16(body[18:20], g.extPort)
	ext := g.extIP.As16()
	copy(body[20:36], ext[:])
	return resp
}

func (g *mockGateway) recordLease(l uint32) {
	if g.leases != nil {
		select {
		case g.leases <- l:
		default:
		}
	}
}

func roundtrip(t *testing.T, gw string, req []byte) []byte {
	t.Helper()
	c, err := net.Dial("udp", gw)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1500)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return buf[:n]
}

func TestNATPMPMapRoundTripThroughAGateway(t *testing.T) {
	g := newMockGateway(t)
	const internal = 40404
	req, err := EncodeNATPMPMap(UDP, internal, 40404, 7200)
	if err != nil {
		t.Fatal(err)
	}
	m, err := DecodeNATPMPMap(roundtrip(t, g.addr(), req), UDP, internal)
	if err != nil {
		t.Fatal(err)
	}
	// The ASSIGNED port, not the suggested one. This is the assertion a self-round-trip
	// cannot make: the gateway chose 51234 and we asked for 40404.
	if m.ExternalPort != g.extPort {
		t.Errorf("external port %d, want the gateway's assigned %d — the decoder may be returning the suggested port it sent", m.ExternalPort, g.extPort)
	}
	if m.InternalPort != internal {
		t.Errorf("internal port %d, want %d", m.InternalPort, internal)
	}
	if m.LifetimeSec != 7200 {
		t.Errorf("lifetime %d, want 7200", m.LifetimeSec)
	}
}

func TestNATPMPExternalAddressThroughAGateway(t *testing.T) {
	g := newMockGateway(t)
	ip, err := DecodeNATPMPExternalAddress(roundtrip(t, g.addr(), EncodeNATPMPExternalAddress()))
	if err != nil {
		t.Fatal(err)
	}
	if ip != g.extIP {
		t.Errorf("external IP %v, want %v", ip, g.extIP)
	}
}

func TestPCPMapRoundTripThroughAGateway(t *testing.T) {
	g := newMockGateway(t)
	const internal = 40404
	nonce := [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	client := netip.MustParseAddr("192.168.1.50")
	req := EncodePCPMap(TCP, nonce, client, internal, internal, 7200)
	m, ext, err := DecodePCPMap(roundtrip(t, g.addr(), req), TCP, nonce, internal)
	if err != nil {
		t.Fatal(err)
	}
	if m.ExternalPort != g.extPort {
		t.Errorf("external port %d, want the gateway's assigned %d", m.ExternalPort, g.extPort)
	}
	if ext != g.extIP {
		t.Errorf("PCP external IP %v, want %v (PCP returns it inline, unlike NAT-PMP)", ext, g.extIP)
	}
	if m.Protocol != TCP {
		t.Errorf("protocol %v, want tcp", m.Protocol)
	}
}

// A gateway refusal (non-zero result code) is a distinct outcome from silence, and the codec
// must surface it — D19's diagnosis tells "the router said no" apart from "the router never
// answered", and it cannot if the decoder drops the code.
func TestARefusalIsSurfacedNotSwallowed(t *testing.T) {
	g := newMockGatewayRefusing(t, 2, 2) // "not authorized/refused" on both

	req, _ := EncodeNATPMPMap(UDP, 40404, 40404, 7200)
	if _, err := DecodeNATPMPMap(roundtrip(t, g.addr(), req), UDP, 40404); !errors.Is(err, ErrResultCode) {
		t.Errorf("NAT-PMP refusal was not surfaced as ErrResultCode: %v", err)
	}
	nonce := [12]byte{9, 9, 9}
	preq := EncodePCPMap(UDP, nonce, netip.MustParseAddr("192.168.1.50"), 40404, 40404, 7200)
	if _, _, err := DecodePCPMap(roundtrip(t, g.addr(), preq), UDP, nonce, 40404); !errors.Is(err, ErrResultCode) {
		t.Errorf("PCP refusal was not surfaced as ErrResultCode: %v", err)
	}
}

// The PCP nonce binds a response to its request. A reply carrying a different nonce — an
// off-path spoof, or a stale datagram from a previous exchange — must be rejected, or a
// stranger who can land a packet on our socket dictates our published external address.
func TestPCPRejectsAResponseWithTheWrongNonce(t *testing.T) {
	sent := [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	req := EncodePCPMap(UDP, sent, netip.MustParseAddr("192.168.1.50"), 40404, 40404, 7200)
	// Hand-build a response that is valid in every way EXCEPT the nonce.
	resp := make([]byte, 24+36)
	resp[0] = pcpVersion
	resp[1] = pcpResponseBit | pcpOpcodeMap
	body := resp[24:]
	copy(body[0:12], []byte{0xff, 0xff, 0xff}) // a DIFFERENT nonce
	body[12] = byte(UDP)
	binary.BigEndian.PutUint16(body[16:18], 40404)
	binary.BigEndian.PutUint16(body[18:20], 51234)
	if _, _, err := DecodePCPMap(resp, UDP, sent, 40404); !errors.Is(err, ErrNonceMismatch) {
		t.Fatalf("a response with the wrong nonce was accepted (err=%v) — an off-path packet could set our external address", err)
	}
	// SETUP / control: the SAME response with the right nonce decodes, so the rejection above
	// is the nonce and not some other malform:
	copy(body[0:12], req[24:36])
	if _, _, err := DecodePCPMap(resp, UDP, sent, 40404); err != nil {
		t.Fatalf("the control response with the correct nonce did not decode: %v", err)
	}
}

// The delete form: lifetime 0. Both protocols use it (NAT-PMP §3.4; PCP lifetime 0), and S07's
// delete-on-every-exit-path lifecycle is built on it, so the encoder must produce it.
func TestTheDeleteFormIsLifetimeZero(t *testing.T) {
	req, err := EncodeNATPMPMap(UDP, 40404, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint32(req[8:12]) != 0 {
		t.Error("NAT-PMP delete must carry lifetime 0")
	}
	if binary.BigEndian.Uint16(req[6:8]) != 0 {
		t.Error("NAT-PMP delete must carry suggested external port 0 (RFC 6886 §3.4)")
	}
	preq := EncodePCPMap(UDP, [12]byte{1}, netip.MustParseAddr("192.168.1.50"), 40404, 0, 0)
	if binary.BigEndian.Uint32(preq[4:8]) != 0 {
		t.Error("PCP delete must carry lifetime 0")
	}
}
