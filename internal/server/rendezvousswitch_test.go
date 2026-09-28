package server

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// /pending 690 — every DHT verb honours the rendezvous switch.
//
// `ensureBootstrapped` is the only reader of `rzOn`, and three roads reached the DHT around it: the
// round's end-state publish, the pre-hop party's end-state pull, and the candidate feed (which called
// the door and threw its refusal away). With an empty routing table a traversal starts from the
// node cache or the shipped seeds, so each of them sent to strangers with the switch OFF.
//
// # The instrument: a sink the traversal cannot avoid
//
// A rendezvous whose node cache names exactly one loopback socket, and nothing else — a non-empty
// cache means the shipped seeds are never consulted (`rendezvous.Open`), so every datagram any
// traversal sends goes to the sink and this test never touches the internet. Each case is measured
// twice: switch ON must put datagrams on the sink (the stimulus — without it "zero" is a sink nobody
// could reach), and switch OFF must put none.

// sinkConfigDir writes a node cache naming one loopback UDP socket, and counts what reaches it.
func sinkConfigDir(t *testing.T) (string, func() int64) {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	var n atomic.Int64
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, _, rerr := pc.ReadFrom(buf); rerr != nil {
				return
			}
			n.Add(1)
		}
	}()
	// The cache's on-disk shape (`rendezvous/dht.go`: `cacheMagic`, then 20-byte id, 16-byte IP,
	// 2-byte big-endian port per node). Written by hand because the writer is unexported, and a
	// wrong shape is caught by the ON case below reading zero.
	rec := make([]byte, 38)
	if _, err := rand.Read(rec[:20]); err != nil {
		t.Fatal(err)
	}
	copy(rec[20:36], net.ParseIP("127.0.0.1").To16())
	binary.BigEndian.PutUint16(rec[36:], uint16(pc.LocalAddr().(*net.UDPAddr).Port))
	dir := t.TempDir()
	cache := nodeCacheDir(dir)
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "dht-nodes"), append([]byte("NIBdht01"), rec...), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, n.Load
}

// settle gives a traversal that is going to send the time to send, then reads the sink.
func settle(count func() int64) int64 {
	time.Sleep(300 * time.Millisecond)
	return count()
}

// switchPredicate is the rendezvous switch as `gateRendezvous` stamps it, fixed for a test.
func switchPredicate(on bool) func() bool { return func() bool { return on } }

func TestTheRoundsEndStatePublishHonoursTheRendezvousSwitch(t *testing.T) {
	for _, on := range []bool{true, false} {
		dir, count := sinkConfigDir(t)
		inv, rec, convCert, convKey, _, _, _, _ := liveCeremonyFull(t)
		term, err := ceremony.SignTermination(rec, ceremony.StateDeclined, convCert, convKey)
		if err != nil {
			t.Fatal(err)
		}
		s := &Server{configDir: dir}
		shared, closeShared, err := s.openRoundRendezvous("127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		shared.door.rzOn = switchPredicate(on)
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		s.publishEndStateFor(ctx, inv, term, shared)
		cancel()
		got := settle(count)
		closeShared()
		checkSink(t, "the round's end-state publish", on, got)
	}
}

func TestThePreHopEndStatePullHonoursTheRendezvousSwitch(t *testing.T) {
	for _, on := range []bool{true, false} {
		dir, count := sinkConfigDir(t)
		cer, _ := armedCeremonyAt(t, dir)
		cer.rzOn = switchPredicate(on)
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		(&Server{}).fetchEndStateWhenSlow(ctx, cer, 0)
		cancel()
		checkSink(t, "the pre-hop end-state pull", on, settle(count))
	}
}

func TestTheCandidateFeedHonoursTheRendezvousSwitch(t *testing.T) {
	for _, on := range []bool{true, false} {
		dir, count := sinkConfigDir(t)
		cer, peerFP := armedCeremonyAt(t, dir)
		cer.rzOn = switchPredicate(on)
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		out := make(chan candidate, 8)
		go func() {
			for range out { //nolint:revive // drain
			}
		}()
		cer.feedCandidates(ctx, out, peerFP, "", "", 0, rendezvousInterval)
		cancel()
		checkSink(t, "the candidate feed", on, settle(count))
	}
}

func checkSink(t *testing.T, road string, on bool, got int64) {
	t.Helper()
	if on && got == 0 {
		t.Errorf("setup: with the switch ON %s sent nothing to the sink, so the OFF case's zero "+
			"would say nothing about the switch", road)
	}
	if !on && got != 0 {
		t.Errorf("with remote peer rendezvous switched OFF, %s sent %d datagram(s) to a DHT node. "+
			"ensureBootstrapped is the one door onto the DHT and the only reader of the switch "+
			"(ADR-011); a verb that goes around it sends to strangers the user opted out of", road, got)
	}
}

// TestTheDHTVerbsHaveExactlyOneDoorEach is ADR-009's guard for /pending 690: it checks the ROUTING,
// not the text at each site. `TestTheDHTBootstrapHasExactlyOneDoor` polices `Bootstrap` and could
// not see a `Publish` or `Fetch` that never asked the door at all — which is how three roads went
// around the switch. Any `<x>.rz.Publish(` or `<x>.rz.Fetch(` outside `dhtPublish`/`dhtFetch` fails.
func TestTheDHTVerbsHaveExactlyOneDoorEach(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	callers := map[string][]string{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				ast.Inspect(fn, func(m ast.Node) bool {
					call, ok := m.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || (sel.Sel.Name != "Publish" && sel.Sel.Name != "Fetch") {
						return true
					}
					recv, ok := sel.X.(*ast.SelectorExpr)
					if !ok || recv.Sel.Name != "rz" {
						return true
					}
					callers[sel.Sel.Name] = append(callers[sel.Sel.Name], fn.Name.Name+" ("+path+")")
					return true
				})
			}
		}
	}
	for verb, door := range map[string]string{"Publish": "dhtPublish", "Fetch": "dhtFetch"} {
		got := callers[verb]
		// SETUP floor: the door itself must be found, or an empty list is a scan that read nothing.
		if len(got) != 1 || !strings.HasPrefix(got[0], door+" ") {
			t.Errorf("rz.%s must be called from %s and nowhere else — the door is what reads the "+
				"rendezvous switch (/pending 690, ADR-011). Callers: %v", verb, door, got)
		}
	}
}
