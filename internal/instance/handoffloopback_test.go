package instance

import (
	"errors"
	"testing"
)

// TestHandOffRefusesANonLoopbackRecord — /pending 502 (info).
//
// Probe refused a record naming a non-loopback address; HandOff, which sends the hand-off SECRET,
// did not check at all. Its only caller probes first, so the gap was latent — and it is the more
// sensitive request of the two.
func TestHandOffRefusesANonLoopbackRecord(t *testing.T) {
	// Unroutable documentation addresses only: with the check removed (the red probe) HandOff really
	// dials, and a resolvable name would send a request off the machine.
	for _, addr := range []string{"203.0.113.5:1234", "[2001:db8::1]:80", "no-port"} {
		_, _, err := HandOff(Record{Addr: addr, Handoff: "secret"}, "", "test")
		if !errors.Is(err, ErrNotLoopback) {
			t.Errorf("HandOff to %q returned %v; it must refuse the address before sending the secret", addr, err)
		}
	}
	// The door itself admits loopback, or the refusals above could be a door that refuses everything.
	if err := checkLoopback("127.0.0.1:1"); err != nil {
		t.Errorf("checkLoopback refused a loopback address: %v", err)
	}
}
