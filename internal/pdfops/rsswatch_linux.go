//go:build linux

package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"nib/internal/safe"
)

// CeilingEnforced reports whether a converter's memory ceiling is enforced on this OS (R7, Y5). It is
// true here: the ceiling is an RSS WATCHDOG over the converter's process group, never an address-space
// limit, because Chromium dumped core under `ulimit -v 4000000` and wrote no PDF.
const CeilingEnforced = true

// CeilingUnenforcedNote is empty where the ceiling is enforced.
const CeilingUnenforcedNote = ""

var pageSize = int64(os.Getpagesize())

// groupRSS sums the resident set, in bytes, of every process whose process group is pgid, from
// /proc/<pid>/statm. What it covers is stated: a renderer that calls setsid leaves the group and the sum.
func groupRSS(pgid int) int64 {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 {
			continue
		}
		if g, ok := pgrpOf(pid); !ok || g != pgid {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "statm"))
		if err != nil {
			continue
		}
		// size resident shared text lib data dt — the second field, in pages
		if f := bytes.Fields(b); len(f) >= 2 {
			if n, err := strconv.ParseInt(string(f[1]), 10, 64); err == nil {
				total += n * pageSize
			}
		}
	}
	return total
}

// pgrpOf reads a process's group from /proc/<pid>/stat; the command name is parenthesised and may hold
// spaces, so fields are counted from the LAST ')'.
func pgrpOf(pid int) (int, bool) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, false
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, false
	}
	f := bytes.Fields(b[i+1:])
	if len(f) < 3 {
		return 0, false
	}
	g, err := strconv.Atoi(string(f[2]))
	return g, err == nil
}

// watchMemory starts a watchdog over the group led by pgid: every tick it sums the group's resident set
// and, past ceiling bytes, kills the group. It also lowers the group's scheduling priority (best effort:
// `SysProcAttr` has no rlimit or nice field). stop ends the watch and reports whether it killed and the
// peak it saw; it is idempotent and must be called (deferred) by the caller.
func watchMemory(pgid int, ceiling int64, tick time.Duration) (stop func() (killed bool, peak int64)) {
	_ = syscall.Setpriority(syscall.PRIO_PGRP, pgid, 10)
	done := make(chan struct{})
	finished := make(chan struct{})
	var mu sync.Mutex
	var killed bool
	var peak int64
	go func() {
		defer safe.Recover("converter memory watchdog")
		defer close(finished)
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				rss := groupRSS(pgid)
				mu.Lock()
				if rss > peak {
					peak = rss
				}
				over := rss > ceiling && !killed
				if over {
					killed = true
				}
				mu.Unlock()
				if over {
					killGroup(pgid)
				}
			}
		}
	}()
	var once sync.Once
	return func() (bool, int64) {
		once.Do(func() { close(done); <-finished })
		mu.Lock()
		defer mu.Unlock()
		return killed, peak
	}
}
