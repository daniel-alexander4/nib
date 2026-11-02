//go:build !linux

package pdfops

import "time"

// CeilingEnforced is false off Linux: there is no /proc to sum a process group's resident set from, and
// Windows and macOS have no `Setpriority`/`PRIO_PGRP` equivalent used here (R7). The gap is DECLARED:
// CeilingUnenforcedNote is what a tool's missing-tool wording appends, and `TestTheCeilingSiblingsAreHonest`
// fails a sibling whose note is empty while its constant says false, or whose body is empty.
const CeilingEnforced = false

// CeilingUnenforcedNote says so, for the notice of a tool that runs here.
const CeilingUnenforcedNote = "On this system Nib cannot limit how much memory the converter uses."

// watchMemory enforces nothing and says so by its constant; it still returns a stop that reports a peak of 0.
func watchMemory(pgid int, ceiling int64, tick time.Duration) (stop func() (killed bool, peak int64)) {
	return func() (bool, int64) { return false, 0 }
}
