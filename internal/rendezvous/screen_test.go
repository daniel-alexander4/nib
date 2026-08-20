package rendezvous

import (
	"bytes"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

// TestTwentyOneBytesCannotKillTheProcess.
//
// The defect, reproduced before it was fixed: `krpc.NodeAddr.UnmarshalBinary` does
// `make(net.IP, len(b)-2)` with no length check (nodeaddr.go:35), the bencode decoder
// deliberately re-panics runtime errors (decode.go:45), and the decode runs on the
// goroutine dht.NewServer starts for itself (server.go:247) — so nothing Nib writes is
// on that stack to recover it. `d2:ip0:1:t2:zz1:y1:re` from ANY host that can reach the
// session port took the whole process down.
//
// The assertion is not "we did not crash", which would also be true of a test whose
// datagram never arrived. It is that the DHT **still answers** afterwards: that proves
// the serve goroutine is alive, which is the thing the panic killed.
func TestTwentyOneBytesCannotKillTheProcess(t *testing.T) {
	n := newNode(t)
	att, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	att.SetDeadline(time.Now().Add(5 * time.Second))

	ping := []byte("d1:ad2:id20:aaaaaaaaaaaaaaaaaaaae1:q4:ping1:t2:zz1:y1:qe")
	answers := func(what string) {
		t.Helper()
		if _, err := att.WriteTo(ping, n.addr); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 2048)
		if _, _, err := att.ReadFrom(buf); err != nil {
			t.Fatalf("the DHT did not answer a well-formed ping %s: %v", what, err)
		}
	}

	// Stimulus: it answers BEFORE the attack, so "it answers after" is a survival and
	// not a property of a socket that was always going to reply.
	answers("before the attack")

	before := n.rz.Stats().Screened
	if _, err := att.WriteTo([]byte("d2:ip0:1:t2:zz1:y1:re"), n.addr); err != nil {
		t.Fatal(err)
	}
	answers("after the attack") // if the screen failed, the process is already gone

	if got := n.rz.Stats().Screened; got != before+1 {
		t.Errorf("Screened went %d -> %d, want +1 — the process survived, but not because "+
			"this datagram was recognised as the one that kills it, so a diagnostic could "+
			"not tell an attack from a quiet day", before, got)
	}
}

// TestAMalformedDatagramIsNotScreened — the screen's job is the crash and nothing wider.
//
// Rejecting everything the decoder dislikes would move a decision out of the library and
// into a copy of it, and the copy would drift. Garbage that decodes to an error is the
// library's to log and discard.
func TestAMalformedDatagramIsNotScreened(t *testing.T) {
	n := newNode(t)
	att, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()

	// Stimulus: the fatal one IS screened, so a zero below means "this one passed",
	// not "the screen counts nothing".
	if _, err := att.WriteTo([]byte("d2:ip0:1:t2:zz1:y1:re"), n.addr); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for n.rz.Stats().Screened == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n.rz.Stats().Screened != 1 {
		t.Fatalf("setup: the fatal datagram was not screened (%d)", n.rz.Stats().Screened)
	}

	before := n.rz.Stats().Screened
	if _, err := att.WriteTo([]byte("not bencode at all"), n.addr); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if got := n.rz.Stats().Screened; got != before {
		t.Errorf("Screened went %d -> %d — an ordinary malformed datagram was dropped by "+
			"the screen. The library rejects those itself, and a screen that decides more "+
			"than the crash is a second parser to keep in step with the first", before, got)
	}
}

// TestSurvivesDecodeIsDecidedByTheLibrarysOwnDecoder — the unit, table-driven.
func TestSurvivesDecodeIsDecidedByTheLibrarysOwnDecoder(t *testing.T) {
	for _, c := range []struct {
		name string
		b    string
		want bool
	}{
		{"a zero-length ip value is the kill", "d2:ip0:1:t2:zz1:y1:re", false},
		{"a one-byte ip value is the kill", "d2:ip1:\x011:t2:zz1:y1:re", false},
		{"a two-byte ip value is survivable", "d2:ip2:\x01\x021:t2:zz1:y1:re", true},
		{"a proper six-byte ip value", "d2:ip6:\x01\x02\x03\x04\x1a\xe11:t2:zz1:y1:re", true},
		{"a zero-length values entry is the kill", "d1:rd2:id20:aaaaaaaaaaaaaaaaaaaa6:valuesl0:eee1:t2:zz1:y1:re", false},
		{"not bencode", "not bencode at all", true},
		{"empty", "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := survivesDecode([]byte(c.b)); got != c.want {
				t.Errorf("survivesDecode = %v, want %v", got, c.want)
			}
		})
	}
}

// TestAQueryWithNoArgumentsCannotKillTheProcess — the SECOND door.
//
// The first version of screen.go closed the decoder and left this open, with a comment
// that read as a clean bill of health. `handleQuery` binds `args := m.A` and then reads
// `args.Token` for `announce_peer` and `put` with no nil check (server.go:540, :573),
// while `get_peers` guards exactly that field one case earlier. So a query with no `a`
// dict is a nil dereference on the DHT's own goroutine.
//
// These datagrams DECODE PERFECTLY — `survivesDecode` returns true for both — which is
// the point: screening the decoder is not screening the library.
func TestAQueryWithNoArgumentsCannotKillTheProcess(t *testing.T) {
	n := newNode(t)
	att, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	att.SetDeadline(time.Now().Add(5 * time.Second))

	answers := func(what string) {
		t.Helper()
		if _, err := att.WriteTo([]byte("d1:ad2:id20:aaaaaaaaaaaaaaaaaaaae1:q4:ping1:t2:zz1:y1:qe"), n.addr); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 2048)
		if _, _, err := att.ReadFrom(buf); err != nil {
			t.Fatalf("the DHT did not answer a well-formed ping %s: %v", what, err)
		}
	}
	answers("before the attack")

	before := n.rz.Stats().RefusedQueries
	for _, evil := range []string{
		"d1:q13:announce_peer1:t2:zz1:y1:qe", // 34 bytes
		"d1:q3:put1:t2:zz1:y1:qe",            // 23 bytes
	} {
		// Stimulus, and the reason this test is not a duplicate of the screen's: the
		// DECODER is perfectly happy with these. If that ever stops being true the
		// screen would be silently doing this test's job and nobody would know which
		// door was actually shut.
		if !survivesDecode([]byte(evil)) {
			t.Fatalf("%q no longer decodes cleanly — this test is meant to prove the "+
				"HANDLER door is shut, and it is now testing the decoder door instead", evil)
		}
		if _, err := att.WriteTo([]byte(evil), n.addr); err != nil {
			t.Fatal(err)
		}
		answers("after " + evil) // if the gate failed, the process is already gone
	}

	if got := n.rz.Stats().RefusedQueries; got != before+2 {
		t.Errorf("RefusedQueries went %d -> %d, want +2 — the process survived, but not "+
			"because these were recognised, so a diagnostic could not tell an attack from "+
			"a quiet day", before, got)
	}
	if n.rz.Stats().Screened != 0 {
		t.Errorf("Screened = %d — these went through the decoder screen, and counting them "+
			"there would merge two different doors into one number", n.rz.Stats().Screened)
	}
}

// TestAWellFormedQueryIsStillAnswered — the gate must not close the door on everyone.
func TestAWellFormedQueryIsStillAnswered(t *testing.T) {
	n := newNode(t)
	att, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()
	att.SetDeadline(time.Now().Add(5 * time.Second))

	for _, q := range []string{
		"d1:ad2:id20:aaaaaaaaaaaaaaaaaaaae1:q4:ping1:t2:z11:y1:qe",
		"d1:ad2:id20:aaaaaaaaaaaaaaaaaaaa6:target20:bbbbbbbbbbbbbbbbbbbbe1:q9:find_node1:t2:z21:y1:qe",
	} {
		if _, err := att.WriteTo([]byte(q), n.addr); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 2048)
		if _, _, err := att.ReadFrom(buf); err != nil {
			t.Fatalf("no reply to %q: %v — a node that stops answering gets dropped from "+
				"other nodes' routing tables, and our own reachability goes with it", q, err)
		}
	}
	if got := n.rz.Stats().RefusedQueries; got != 0 {
		t.Errorf("RefusedQueries = %d after two well-formed queries — the gate is refusing "+
			"traffic it exists to let through", got)
	}
}

// TestAnAllocationBombIsDroppedNotDecodedTwice.
//
// bencode allocates a declared string length before reading it and its default ceiling
// is ~128 MiB, so seventeen bytes buy 128 MiB of allocation — measured at 134,221,493
// bytes per decode. Screening with an unbounded decoder would pay that twice: once here
// and once in the library.
func TestAnAllocationBombIsDroppedNotDecodedTwice(t *testing.T) {
	bomb := []byte("d1:t134217727:zze")

	// Stimulus: it really is a bomb under the library's own default bound, so the
	// cheapness below is the fix and not a claim about a harmless datagram.
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	var junk krpc.Msg
	_ = bencode.Unmarshal(bomb, &junk)
	runtime.ReadMemStats(&b)
	if grew := b.TotalAlloc - a.TotalAlloc; grew < 1<<24 {
		t.Fatalf("an unbounded decode of %d bytes allocated only %d — the bomb no longer "+
			"goes off, so this test is measuring nothing", len(bomb), grew)
	}

	if survivesDecode(bomb) {
		t.Error("the bomb passed the screen — the library then decodes it too, and the " +
			"whole point is that it never reaches a second decoder")
	}
	runtime.ReadMemStats(&a)
	for i := 0; i < 10; i++ {
		survivesDecode(bomb)
	}
	runtime.ReadMemStats(&b)
	if grew := b.TotalAlloc - a.TotalAlloc; grew > 1<<20 {
		t.Errorf("10 screened bombs allocated %d bytes — the bound is not being applied, "+
			"and the screen has become the amplifier", grew)
	}
}

// TestTheDecoderStillSaysExceedsLimit.
//
// survivesDecode drops the allocation bomb by matching the decoder's own wording, there
// being no sentinel or typed error to check. A dependency bump that reworded it would
// silently turn the drop back into a pass-through, and nothing else would notice — so
// the emitted string is asserted against real output rather than trusted.
func TestTheDecoderStillSaysExceedsLimit(t *testing.T) {
	var m krpc.Msg
	d := bencode.NewDecoder(bytes.NewReader([]byte("d1:t99999999:zze")))
	d.MaxStrLen = 8
	err := d.Decode(&m)
	if err == nil {
		t.Fatal("the decoder accepted a string longer than its MaxStrLen; the bound itself is gone")
	}
	if !strings.Contains(err.Error(), maxStrLenExceeded) {
		t.Errorf("the decoder now says %q, which does not contain %q — survivesDecode "+
			"matches on that text, so the allocation bomb is passing through again",
			err, maxStrLenExceeded)
	}
}
