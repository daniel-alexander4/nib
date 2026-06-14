// Package safe holds small helpers for running goroutines without letting a
// single panic take down the whole desktop process.
package safe

import "log"

// Recover, deferred at the very top of a goroutine body, swallows and logs any
// panic so one bad input — a malformed inbound co-sign document, a hostile
// server response, a nil deref deep in a dependency — degrades to a failed
// operation instead of crashing Nib and losing the user's unsaved document. The
// goroutine's other defers (disarm, Close, wg.Done) still run as the stack
// unwinds. label identifies the goroutine in the log line.
func Recover(label string) {
	if r := recover(); r != nil {
		log.Printf("recovered from panic in %s: %v", label, r)
	}
}
