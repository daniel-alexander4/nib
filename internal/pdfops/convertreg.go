package pdfops

import (
	"errors"
	"sync"
	"time"

	"nib/internal/safe"
)

// The live converters (ADR-185, D31).
//
// `killTreeOnCancel` is reached only by a context cancel, and a process exit can beat a cancel: Quit
// (ADR-169), the idle exit and a crash all left `soffice.bin` running with the user's source in its
// temp directory. So every converter's process group is registered here between `Start` and `Wait`,
// and `KillConverters` — called from the one `shutdown()` that Quit, idle exit and SIGTERM reach —
// kills them all synchronously before the process leaves.

// ErrShuttingDown is a converter asked for after KillConverters ran. Nothing starts a process on a
// Nib that is leaving.
var ErrShuttingDown = errors.New("Nib is closing, so it did not start another conversion")

var converters = struct {
	mu     sync.Mutex
	groups map[int]struct{}
	closed bool
	// starting counts converters between "allowed to start" and "registered": KillConverters waits for it to reach
	// zero, so a process started in the instant a shutdown begins is registered before the sweep of the registry
	// rather than killed by its own goroutine after Nib has already exited.
	starting int
}{groups: map[int]struct{}{}}

// beginStart is asked before a converter is started. It refuses once KillConverters has run and otherwise
// holds the shutdown off until endStart.
func beginStart() error {
	converters.mu.Lock()
	defer converters.mu.Unlock()
	if converters.closed {
		return ErrShuttingDown
	}
	converters.starting++
	return nil
}

func endStart() {
	converters.mu.Lock()
	converters.starting--
	converters.mu.Unlock()
}

// registerConverter records a converter group (its leader's pid) and returns the function that
// forgets it. It refuses once KillConverters has run.
func registerConverter(pid int) (forget func(), err error) {
	converters.mu.Lock()
	defer converters.mu.Unlock()
	if converters.closed {
		return nil, ErrShuttingDown
	}
	converters.groups[pid] = struct{}{}
	return func() {
		converters.mu.Lock()
		delete(converters.groups, pid)
		converters.mu.Unlock()
	}, nil
}

// LiveConverters reports how many converter groups are registered. For the shutdown test and the
// status view.
func LiveConverters() int {
	converters.mu.Lock()
	defer converters.mu.Unlock()
	return len(converters.groups)
}

// killWait bounds KillConverters: a kill that cannot finish in this long is reported, not waited on.
const killWait = time.Second

// KillConverters ends every registered converter group and refuses new ones. It is synchronous and
// bounded to killWait in total, and returns how many groups it signalled. Idempotent.
func KillConverters() int {
	converters.mu.Lock()
	converters.closed = true
	converters.mu.Unlock()
	// A converter that was allowed to start before the flag flipped is registered within microseconds of Start;
	// wait (bounded) for those so the snapshot below holds it.
	for deadline := time.Now().Add(killWait); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		converters.mu.Lock()
		n := converters.starting
		converters.mu.Unlock()
		if n == 0 {
			break
		}
	}
	converters.mu.Lock()
	pids := make([]int, 0, len(converters.groups))
	for pid := range converters.groups {
		pids = append(pids, pid)
	}
	converters.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer safe.Recover("kill converters")
		defer close(done)
		for _, pid := range pids {
			killGroup(pid)
		}
	}()
	select {
	case <-done:
	case <-time.After(killWait):
	}
	return len(pids)
}

// converterExecutables are the base names the startup sweep will kill an orphaned group member for:
// the converters' own binaries, never anything else that happens to share a group id.
var converterExecutables = []string{"soffice", "soffice.bin", "oosplash", "gs"}

// ConverterExecutables is that list, for `tempdir.NewReaper`.
func ConverterExecutables() []string { return append([]string(nil), converterExecutables...) }
