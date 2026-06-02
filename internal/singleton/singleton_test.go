//go:build linux

package singleton

import "testing"

// ReplaceOthers must exclude the current process — calling it here must not
// SIGTERM the test runner, and with no sibling Nib processes it signals none.
func TestReplaceOthersExcludesSelf(t *testing.T) {
	if n := ReplaceOthers(); n != 0 {
		t.Logf("signalled %d sibling(s) — unexpected in isolation, but self was not killed", n)
	}
	// Reaching this line proves the current process was not terminated.
}
