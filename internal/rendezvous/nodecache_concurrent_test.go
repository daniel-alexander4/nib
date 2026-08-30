package rendezvous

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/anacrolix/dht/v2/krpc"
)

// TestTwoWritersCannotTearTheNodeCache — /pending 316.
//
// # The defect this exists against
//
// `writeNodes` hand-rolled its own temp-file-plus-rename with a FIXED temp name,
// `dht-nodes.tmp`, outside `internal/atomicfile`. `nodeCacheDir` is one path per config dir and
// three sites open a `rendezvous.Server` against it — one per ceremony — each saving on `Close()`,
// plus the second Nib this repo deliberately permits. So two writers really can meet here.
//
// The mechanism is not "the rename raced". Writer B opens the fixed temp name BEFORE A renames it,
// so both hold a descriptor on the same inode; after A's rename that inode IS the published cache,
// and B's truncate-and-write goes straight through it. The rename's atomicity is defeated, and so
// is this function's own rule twenty lines up — *"Do NOT truncate a good cache with an empty
// one"* — because the truncation happens to the live file rather than to a temp.
//
// # What is asserted, and why it is the cache and not the error
//
// A failed write is recoverable: the next `Close()` writes again, and a cold start is one slow
// run. A TORN cache is not, and it is silent — `loadNodes` either refuses it (and the run
// bootstraps from seeds, which is the good case) or, at an unlucky length, parses it into node
// addresses assembled from the wrong offsets. `cacheMagic`'s own doc records that arithmetic. So
// the assertion is that every reader sees a cache that parses, throughout.
func TestTwoWritersCannotTearTheNodeCache(t *testing.T) {
	dir := t.TempDir()

	nodes := func(n int, seed byte) []krpc.NodeInfo {
		out := make([]krpc.NodeInfo, 0, n)
		for i := 0; i < n; i++ {
			var ni krpc.NodeInfo
			for j := range ni.ID {
				ni.ID[j] = seed
			}
			ni.Addr = krpc.NodeAddr{IP: net.IPv4(10, 0, byte(i>>8), byte(i)), Port: 6881 + i}
			out = append(out, ni)
		}
		return out
	}
	// Two writers with DIFFERENT sizes, so a torn file is a length nothing legitimate produces.
	big, small := nodes(400, 0xAA), nodes(11, 0xBB)

	// Seed a good cache, and prove the fixture reads back before anything races it — a check
	// that only ever ran against a broken cache proves nothing about tearing.
	if _, err := writeNodes(dir, big); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s := &Server{dir: dir}
	if got, err := s.loadNodes(); err != nil || len(got) != len(big) {
		t.Fatalf("setup: the seeded cache does not read back: %d node(s), %v", len(got), err)
	}

	const rounds = 300
	var wg sync.WaitGroup
	var mu sync.Mutex
	var writeErrs []error
	write := func(set []krpc.NodeInfo) {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			if _, err := writeNodes(dir, set); err != nil {
				mu.Lock()
				writeErrs = append(writeErrs, err)
				mu.Unlock()
			}
		}
	}
	wg.Add(2)
	go write(big)
	go write(small)

	// A third goroutine reads throughout. It is the one that can see a tear.
	done := make(chan struct{})
	var reads, torn int
	var firstTear error
	go func() {
		defer close(done)
		r := &Server{dir: dir}
		for {
			select {
			case <-done:
				return
			default:
			}
			got, err := r.loadNodes()
			if os.IsNotExist(err) {
				continue // never observed with the door, but not itself a tear
			}
			reads++
			if err != nil {
				torn++
				if firstTear == nil {
					firstTear = err
				}
				continue
			}
			// A cache that parses must be one of the two whole sets, never a blend.
			if len(got) != len(big) && len(got) != len(small) {
				torn++
				if firstTear == nil {
					firstTear = fmt.Errorf("cache holds %d nodes, which is neither writer's set "+
						"(%d or %d)", len(got), len(big), len(small))
				}
			}
			if reads > 20000 {
				return
			}
		}
	}()
	wg.Wait()
	<-done

	if len(writeErrs) > 0 {
		t.Errorf("%d of %d writes failed, e.g. %v. Two writers sharing one fixed temp name make "+
			"the loser's rename fail on a name the winner already consumed.",
			len(writeErrs), rounds*2, writeErrs[0])
	}
	// STIMULUS: a reader that never ran cannot have seen a tear, and would report a clean pass.
	if reads < 50 {
		t.Fatalf("setup: the reader completed only %d read(s) — it did not overlap the writers, "+
			"so its clean result says nothing", reads)
	}
	if torn > 0 {
		t.Errorf("%d of %d reads saw a node cache that was neither writer's file (first: %v).\n"+
			"A fixed temp name lets the second writer's descriptor follow the rename into the "+
			"PUBLISHED cache, so its truncate-and-write lands on the live file — defeating both "+
			"the rename's atomicity and writeNodes' own \"do NOT truncate a good cache\" rule.",
			torn, reads, firstTear)
	}
	t.Logf("node cache: %d write(s), %d read(s), %d torn", rounds*2, reads, torn)
}

// TestTheNodeCacheLeavesNoTemporaryBehind is also the first coverage `atomicfile.Write` has ever
// had: every test in `internal/atomicfile` drives `WriteDurable`, and /pending 316 gave `Write`
// its first production caller. A leftover temp in the user's config directory is not inert —
// nothing in the tree sweeps one.
func TestTheNodeCacheLeavesNoTemporaryBehind(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeNodes(dir, []krpc.NodeInfo{{Addr: krpc.NodeAddr{IP: net.IPv4(10, 0, 0, 1), Port: 6881}}}); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != bootstrapFile {
		t.Errorf("after one write the cache directory holds %v, want exactly [%s]. A leftover "+
			"temp is not inert: it is a file in the user's config directory that nothing sweeps.",
			names, bootstrapFile)
	}
	if _, err := os.Stat(filepath.Join(dir, bootstrapFile+".tmp")); err == nil {
		t.Error("the old fixed temp name is still being written — the door was not reached")
	}
}
