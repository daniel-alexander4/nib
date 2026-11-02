//go:build linux

package tempdir

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// ReapSupported is true where a Reaper can find an orphaned converter group.
const ReapSupported = true

// clockTicks is Linux's USER_HZ, the unit of the start time in /proc/<pid>/stat. It is 100 on every
// architecture Nib ships for; sysconf(_SC_CLK_TCK) is not reachable without cgo.
const clockTicks = 100

// NewReaper returns a Reaper that kills the members of a recorded process group — the converter's
// children, which outlive their launcher when Nib dies first (`soffice` exits, `soffice.bin` stays).
//
// It kills a process only when ALL of: its process group is the recorded one, it started no earlier
// than the directory was made, and its executable's base name is one of known. A pid reused by an
// unrelated program fails the second or third test. The group id alone is never trusted.
func NewReaper(known []string) Reaper {
	bt := bootTime()
	return func(pgid int, since time.Time) {
		if bt.IsZero() {
			return
		}
		ents, err := os.ReadDir("/proc")
		if err != nil {
			return
		}
		for _, e := range ents {
			pid, err := strconv.Atoi(e.Name())
			if err != nil || pid <= 1 {
				continue
			}
			g, started, ok := statOf(pid)
			if !ok || g != pgid {
				continue
			}
			if bt.Add(time.Duration(started) * time.Second / clockTicks).Before(since.Add(-2 * time.Second)) {
				continue
			}
			exe, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe"))
			if err != nil || !contains(known, filepath.Base(exe)) {
				continue
			}
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// statOf reads a process's group and start time (clock ticks after boot) from /proc/<pid>/stat. The
// command name is parenthesised and may hold spaces and parentheses, so fields are counted from the
// LAST ')'.
func statOf(pid int) (pgrp int, start uint64, ok bool) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, 0, false
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, 0, false
	}
	f := bytes.Fields(b[i+1:])
	// f[0]=state f[1]=ppid f[2]=pgrp ... f[19]=starttime
	if len(f) < 20 {
		return 0, 0, false
	}
	g, err1 := strconv.Atoi(string(f[2]))
	s, err2 := strconv.ParseUint(string(f[19]), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return g, s, true
}

func bootTime() time.Time {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte("btime ")) {
			if n, err := strconv.ParseInt(string(bytes.TrimSpace(line[6:])), 10, 64); err == nil {
				return time.Unix(n, 0)
			}
		}
	}
	return time.Time{}
}
