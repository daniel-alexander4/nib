package rendezvous

import (
	"net"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
)

// TestEveryTrustingReaderOfTheTableGoesThroughTheDoor is ADR-009's guard for /pending 775: the
// routing table is read in exactly two functions — `answeredNodes`, the door, and `Nodes`, the
// raw diagnostic — so a fourth reader cannot act on a node that merely queried us without this
// naming it. It checks the routing, not what each site says.
func TestEveryTrustingReaderOfTheTableGoesThroughTheDoor(t *testing.T) {
	names, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fn := regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?(\w+)`)
	allowed := map[string]bool{"answeredNodes": true, "Nodes": true}
	found := map[string]bool{}
	for _, e := range names {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		heads := fn.FindAllStringSubmatchIndex(src, -1)
		for i, h := range heads {
			end := len(src)
			if i+1 < len(heads) {
				end = heads[i+1][0]
			}
			if !strings.Contains(src[h[0]:end], ".dht.Nodes()") {
				continue
			}
			name := src[h[2]:h[3]]
			found[name] = true
			if !allowed[name] {
				t.Errorf("%s: %s reads the library's routing table raw — a node that merely queried us is "+
					"in it; read s.answeredNodes() (contacts.go)", e.Name(), name)
			}
		}
	}
	for name := range allowed {
		if !found[name] {
			t.Errorf("%s no longer reads the routing table — the census is matching nothing, re-point it", name)
		}
	}
}

// TestANodeThatNeverAnsweredIsNeitherProbedNorSampled drives the door's two new callers. The
// node is planted, because nothing puts a silent node in the table any more.
func TestANodeThatNeverAnsweredIsNeitherProbedNorSampled(t *testing.T) {
	n := newNode(t)
	stranger := &net.UDPAddr{IP: net.ParseIP("127.41.0.1"), Port: 6883}
	plantUnanswered(t, n.rz, stranger)
	if got := len(n.rz.Nodes()); got != 1 {
		t.Fatalf("setup: the routing table holds %d node(s), want the planted 1", got)
	}
	if got := n.rz.probeTargets(); len(got) != 0 {
		t.Errorf("probeTargets = %v — a node that never answered us would be asked where we are, and believed", got)
	}
	if got := n.rz.answeredNodes(); len(got) != 0 {
		t.Errorf("answeredNodes = %v, want none: SeedSample would ship it to an invitation's recipient", got)
	}

	// And the other side, so the rule is not "nobody": once it has answered, it is a target.
	ap := apOf(stranger)
	n.rz.contacts.sent(ap)
	n.rz.contacts.answered(ap)
	if got := n.rz.probeTargets(); len(got) != 1 {
		t.Errorf("probeTargets = %v after the node answered, want it", got)
	}
}

// TestAQueryRestatedReadOnlyIsTheSameQuery pins asReadOnly's fidelity: the library handles the
// restatement, so everything it reads from a query must survive it, and the one thing added is
// the flag. The a-less rows are gateQuery's kill shapes — they must still arrive with no `a`.
func TestAQueryRestatedReadOnlyIsTheSameQuery(t *testing.T) {
	for _, raw := range []string{
		"d1:ad2:id20:aaaaaaaaaaaaaaaaaaaae1:q4:ping1:t2:zz1:y1:qe",
		"d1:ad2:id20:aaaaaaaaaaaaaaaaaaaa6:target20:bbbbbbbbbbbbbbbbbbbb4:wantl2:n42:n6ee1:q9:find_node1:t2:zz1:y1:qe",
		"d1:ad2:id20:aaaaaaaaaaaaaaaaaaaa9:info_hash20:bbbbbbbbbbbbbbbbbbbbe1:q9:get_peers1:t2:zz1:v4:NB011:y1:qe",
		"d1:ad2:id20:aaaaaaaaaaaaaaaaaaaa3:seqi7e6:target20:bbbbbbbbbbbbbbbbbbbbe1:q3:get1:t2:zz1:y1:qe",
		"d1:ad2:id20:aaaaaaaaaaaaaaaaaaaa5:token3:tok1:v5:helloe1:q3:put1:t2:zz1:y1:qe",
		"d1:q13:announce_peer1:t2:zz1:y1:qe",
		"d1:q3:put1:t2:zz1:y1:qe",
	} {
		m, derr, ok := decodeScreened([]byte(raw))
		if !ok || derr != nil {
			t.Fatalf("setup: %q does not decode (%v)", raw, derr)
		}
		b, ok := asReadOnly(m, 0x10000)
		if !ok {
			t.Errorf("%q could not be restated", raw)
			continue
		}
		var back krpc.Msg
		if err := bencode.Unmarshal(b, &back); err != nil {
			t.Errorf("%q: the restatement does not decode: %v", raw, err)
			continue
		}
		if !back.ReadOnly {
			t.Errorf("%q: restated without the read-only flag, so the sender is admitted to the table", raw)
		}
		want := m
		want.ReadOnly = true
		if !reflect.DeepEqual(back, want) {
			t.Errorf("%q: the restatement is a different query\n got %+v (a=%+v)\nwant %+v (a=%+v)", raw, back, back.A, want, want.A)
		}
		if _, ok := asReadOnly(m, len(b)-1); ok {
			t.Errorf("%q: a restatement one byte longer than the buffer was handed through", raw)
		}
	}
}

// TestAMalformedDatagramIsDroppedBeforeTheLibraryLogsIt is /pending 775's log flood: the library
// writes a datagram its decoder returns an error for to the process log, whole, once per
// datagram (server.go:310). The decoder is the same one, so the screen drops it — counted apart
// from Screened, which means "would have killed us" — and the DHT still answers.
func TestAMalformedDatagramIsDroppedBeforeTheLibraryLogsIt(t *testing.T) {
	n := newNode(t)
	att, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer att.Close()

	// `y` as an integer: a type error, which is none of the three syntax errors the library
	// stays quiet about — so each of these was a log line holding the datagram.
	bad := []byte("d1:t2:zz1:yi5ee")
	if m, derr, ok := decodeScreened(bad); !ok || derr == nil {
		t.Fatalf("setup: %q is not a survivable decode error (ok=%v derr=%v m=%+v)", bad, ok, derr, m)
	}
	const sent = 5
	for i := 0; i < sent; i++ {
		if _, err := att.WriteTo(bad, n.addr); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "the malformed datagrams to be counted", func() bool { return n.rz.Stats().Malformed >= sent })
	st := n.rz.Stats()
	if st.Malformed != sent || st.Screened != 0 {
		t.Errorf("Malformed=%d Screened=%d, want %d and 0 — a malformed datagram is not one that would have killed us", st.Malformed, st.Screened, sent)
	}

	att.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := att.WriteTo([]byte("d1:ad2:id20:aaaaaaaaaaaaaaaaaaaae1:q4:ping1:t2:zz1:y1:qe"), n.addr); err != nil {
		t.Fatal(err)
	}
	if _, _, err := att.ReadFrom(make([]byte, 2048)); err != nil {
		t.Fatalf("the DHT did not answer a well-formed ping after the malformed ones: %v", err)
	}
	if got := n.rz.Stats().Malformed; got != sent {
		t.Errorf("Malformed went %d -> %d on a well-formed ping", sent, got)
	}
}
