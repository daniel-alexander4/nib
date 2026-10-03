//go:build linux

package sign

import (
	"bytes"
	"fmt"
	"runtime"
	"syscall"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"
)

// memberLookupCPU is the CPU the patched reader spends on ONE pass of in-stream member lookups — only those, on a
// fresh Reader, best of three — read from the thread's own rusage, so the rest of a loaded machine does not count.
func memberLookupCPU(t *testing.T, doc []byte) time.Duration {
	t.Helper()
	best := time.Duration(1 << 62)
	for rep := 0; rep < 3; rep++ {
		r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
		if err != nil {
			t.Fatalf("STIMULUS: the reader refuses the fixture: %v", err)
		}
		xs := r.Xref()
		runtime.GC()
		runtime.LockOSThread()
		var a, b syscall.Rusage
		syscall.Getrusage(syscall.RUSAGE_THREAD, &a)
		for _, x := range xs {
			if sp := x.Stream(); sp.GetID() != 0 {
				p := x.Ptr()
				r.Resolve(p, p)
			}
		}
		syscall.Getrusage(syscall.RUSAGE_THREAD, &b)
		runtime.UnlockOSThread()
		cpu := func(u syscall.Rusage) int64 { return syscall.TimevalToNsec(u.Utime) + syscall.TimevalToNsec(u.Stime) }
		best = min(best, time.Duration(cpu(b)-cpu(a)))
	}
	return best
}

// singleMemberStreams is k object streams of one tiny member each — every lookup a visit to a stream of its own.
func singleMemberStreams(k int) []byte {
	streams := map[int]stmSpec{}
	members := map[int][2]int{}
	for s := 0; s < k; s++ {
		id := 4 + k + s
		streams[4+s] = stmSpec{hdr: fmt.Sprintf("%d 0 ", id), content: "<</F 1>>\n", n: 1, first: -1, flate: true}
		members[id] = [2]int{4 + s, 0}
	}
	return rawObjStmDoc(streams, members)
}

// TestTheLookupModelChargesAtLeastWhatALookupCosts — `/pending 762`: the ceiling binds before about six seconds a
// pass only if every unit it charges is at least a nanosecond of the reader's work. Re-measured cleanly (member
// lookups only, thread CPU, 2026-10-03), the model was above the cost on every shape but one: 4,000 streams of one
// member each, where a lookup is all fixed cost, charged 0.93 of what it cost — the visit weight was fitted where a
// visit's stream was already decoded. The shapes here are each kind of term: the fixed cost (single-member streams,
// one stream of many members), the /Extends chain (visits times members).
func TestTheLookupModelChargesAtLeastWhatALookupCosts(t *testing.T) {
	if testing.Short() {
		t.Skip("SKIP (not a pass): timing the reader")
	}
	for _, c := range []struct {
		name string
		doc  []byte
	}{
		{"4000 single-member streams", singleMemberStreams(4000)},
		{"one stream of 4000 members", streamDoc(0, 4000, 0, false)},
		{"/Extends chain 100x100", chainDoc(100, 100, false)},
	} {
		r, err := dpdf.NewReader(bytes.NewReader(c.doc), int64(len(c.doc)))
		if err != nil {
			t.Fatal(err)
		}
		cost, err := libraryLookupCost(r)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		measured := memberLookupCPU(t, c.doc)
		ratio := float64(cost.work()) / float64(measured.Nanoseconds())
		t.Logf("%-28s charged %v, cost %v of CPU: %.2fx", c.name, time.Duration(cost.work()), measured, ratio)
		// A margin of 1.1: thread CPU has a tick's resolution and the fitting machine is the one measuring.
		if ratio < 1.1 {
			t.Errorf("%s: the model charges %.2fx of what the lookups cost — under it, the ceiling admits passes "+
				"past its six seconds", c.name, ratio)
		}
	}
}
