package rendezvous

import (
	"net"
	"testing"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

// A hand-built KRPC responder.
//
// Two real dht.Servers on loopback WOULD drive the happy path — the library sets the
// `ip` field on every non-error reply (server.go:687) — but they can only ever tell the
// truth, and every case worth testing here is a node that does not: a liar, a truncated
// field, a bogon, a carrier-NAT address, five nodes that each see something different.
// So the peer is built rather than borrowed.
//
// Each fake binds a DIFFERENT 127.x.0.1 address, because probeTargets counts distinct
// /24 prefixes and all of 127.0.0.x is one of them — nodes on 127.0.0.2 and 127.0.0.3
// would be a single source, and every classification below would collapse to unknown
// for a reason that has nothing to do with the code under test.
type fakeNode struct {
	pc   net.PacketConn
	addr *net.UDPAddr
	id   krpc.ID
}

// newFakeNode answers every query with `answer`, which is handed the decoded query and
// returns the raw bytes to send — raw, so a test can send something no encoder would
// produce.
func newFakeNode(t *testing.T, host string, answer func(q krpc.Msg, id krpc.ID) []byte) *fakeNode {
	t.Helper()
	pc, err := net.ListenPacket("udp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Skipf("cannot bind %s (%v) — this platform does not offer the whole 127/8", host, err)
	}
	f := &fakeNode{pc: pc, addr: pc.LocalAddr().(*net.UDPAddr)}
	for i := range f.id {
		f.id[i] = byte(i + 1) // non-zero; loopback is "local" so BEP-42 security passes
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			var q krpc.Msg
			if err := bencode.Unmarshal(buf[:n], &q); err != nil {
				continue
			}
			if b := answer(q, f.id); b != nil {
				pc.WriteTo(b, from)
			}
		}
	}()
	t.Cleanup(func() { pc.Close() })
	return f
}

// truthfully reports a fixed public endpoint, the way a well-behaved BEP-42 node does.
func reports(self string, port int) func(krpc.Msg, krpc.ID) []byte {
	return func(q krpc.Msg, id krpc.ID) []byte {
		return bencode.MustMarshal(krpc.Msg{
			T:  q.T,
			Y:  "r",
			R:  &krpc.Return{ID: id},
			IP: krpc.NodeAddr{IP: net.ParseIP(self), Port: port},
		})
	}
}

// rawIP answers with an `ip` value of exactly these bytes — including lengths the
// encoder would never emit, which is the point.
func rawIP(ipBytes []byte) func(krpc.Msg, krpc.ID) []byte {
	return func(q krpc.Msg, id krpc.ID) []byte {
		body := bencode.MustMarshal(krpc.Msg{T: q.T, Y: "r", R: &krpc.Return{ID: id}})
		// Splice the ip field in by hand: the message is a bencoded dict and `ip`
		// sorts before `r`, `t` and `y`.
		out := []byte("d2:ip")
		out = append(out, []byte(itoa(len(ipBytes)))...)
		out = append(out, ':')
		out = append(out, ipBytes...)
		return append(out, body[1:]...) // drop the leading 'd', keep the rest
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
