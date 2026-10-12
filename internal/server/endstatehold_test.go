package server

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// /pending 588 — the round's end-state publish waits out the link's hold (ADR-011).
//
// It was the one road to a DHT verb that never called `holdDHT`: it ran after its leg and took that
// for the hold, which is true of a leg that burns `connectDeadline` and false of one that ends
// early — refused by a party it found on the link, or failed before it browsed. Driven at the sink
// `rendezvousswitch_test.go` builds, where every datagram a traversal sends is counted.
func TestTheRoundsEndStatePublishWaitsOutTheLinksHold(t *testing.T) {
	publish := func(t *testing.T, ctx context.Context, hold time.Duration) (count func() int64, done <-chan struct{}) {
		t.Helper()
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
		shared.door.rzOn = switchPredicate(true)
		ch := make(chan struct{})
		go func() {
			defer close(ch)
			s.publishEndStateFor(ctx, inv, term, shared, time.Now().Add(hold))
		}()
		t.Cleanup(func() { <-ch; closeShared() })
		return count, ch
	}

	t.Run("nothing leaves inside the hold, and it publishes after", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		count, done := publish(t, ctx, 900*time.Millisecond)
		time.Sleep(450 * time.Millisecond)
		if got := count(); got != 0 {
			t.Errorf("%d datagram(s) reached a DHT node 450 ms into a 900 ms hold — the end-state publish "+
				"left the machine before the link had its window (ADR-011)", got)
		}
		time.Sleep(750 * time.Millisecond)
		// The other side, so the hold is not "never publish": past it, the traversal reaches the sink.
		if got := count(); got == 0 {
			t.Error("setup: nothing reached the sink after the hold either, so the zero above says nothing")
		}
		cancel()
		<-done
	})

	t.Run("a round that ends inside the hold publishes nothing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		count, done := publish(t, ctx, 900*time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		cancel()
		<-done
		if got := settle(count); got != 0 {
			t.Errorf("%d datagram(s) reached a DHT node from a round that ended inside its hold", got)
		}
	})
}

// TestEveryRoadToADHTVerbServesTheHold is the routing half (ADR-009): a function that calls
// `dhtPublish` or `dhtFetch` calls `holdDHT` itself, or is the one named exemption whose only
// caller does. `TestTheDHTVerbsHaveExactlyOneDoorEach` holds that nothing reaches the verbs
// except through those two.
func TestEveryRoadToADHTVerbServesTheHold(t *testing.T) {
	// publishCandidates is one publish of a loop; the loop's caller serves the hold once, before
	// the first, because a republish must not wait a window each time.
	const exempt, exemptCaller = "publishCandidates", "publishWhenSlow"

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]map[string]bool{} // function -> selector names it calls
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok {
					continue
				}
				seen := map[string]bool{}
				ast.Inspect(fn, func(m ast.Node) bool {
					call, ok := m.(*ast.CallExpr)
					if !ok {
						return true
					}
					switch f := call.Fun.(type) {
					case *ast.SelectorExpr:
						seen[f.Sel.Name] = true
					case *ast.Ident:
						seen[f.Name] = true
					}
					return true
				})
				calls[fn.Name.Name] = seen
			}
		}
	}
	var roads []string
	for name, seen := range calls {
		if !seen["dhtPublish"] && !seen["dhtFetch"] {
			continue
		}
		roads = append(roads, name)
		if name == exempt {
			continue
		}
		if !seen["holdDHT"] {
			t.Errorf("%s reaches a DHT verb and never calls holdDHT — the link gets its window before "+
				"anything leaves the machine (ADR-011), and the hold is served by the function that sends", name)
		}
	}
	sort.Strings(roads)
	// SETUP floor: the four roads must be found, or a clean pass is a scan that read nothing.
	if want := "feedCandidates,fetchEndStateWhenSlow,publishCandidates,publishEndStateFor"; strings.Join(roads, ",") != want {
		t.Errorf("the roads to a DHT verb are %v, want %s — a new road needs its hold (or a named exemption here)", roads, want)
	}
	for name, seen := range calls {
		if seen[exempt] && name != exemptCaller {
			t.Errorf("%s calls %s, which serves no hold of its own; only %s may (it holds first)", name, exempt, exemptCaller)
		}
	}
	if !calls[exemptCaller]["holdDHT"] || !calls[exemptCaller][exempt] {
		t.Errorf("%s no longer both holds and publishes — the exemption for %s rests on it", exemptCaller, exempt)
	}
}
