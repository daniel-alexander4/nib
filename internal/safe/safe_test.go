package safe

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

//go:noinline
func panicsDeepInADependency() {
	var m map[string]int
	m["x"] = 1 // assignment to a nil map
}

// TestARecoveredPanicSaysWhere — /pending 712 R6-8. The log line is the only diagnosis a
// stranger's machine offers, and it carried the panic value alone: what, never where.
func TestARecoveredPanicSaysWhere(t *testing.T) {
	var buf bytes.Buffer
	saved, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	defer func() { log.SetOutput(saved); log.SetFlags(flags) }()
	func() {
		defer Recover("probe")
		panicsDeepInADependency()
	}()
	out := buf.String()
	// STIMULUS: the panic was recovered and logged at all.
	if !strings.Contains(out, "recovered from panic in probe") {
		t.Fatalf("no recovery line was logged: %q", out)
	}
	if !strings.Contains(out, "panicsDeepInADependency") {
		t.Errorf("the recovery line does not name the frame that panicked, so a user's log says what "+
			"failed and never where:\n%s", out)
	}
}
