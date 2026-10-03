package portmap

import (
	"context"
	"errors"
	"testing"
)

// TestAUPnPAddDoesNotClaimAPortAnotherHostHolds — /pending 807 R7.
//
// `AddPortMapping` claims external = internal port with no prior look. A conformant IGD refuses a
// port another LAN host holds; a non-conformant one overwrites it and hands Nib that host's
// mapping. The mock here is the non-conformant kind — it accepts every add — so only asking first
// keeps the add from being sent.
func TestAUPnPAddDoesNotClaimAPortAnotherHostHolds(t *testing.T) {
	at := func(m *mockIGD) func(context.Context) ([]string, error) {
		return func(context.Context) ([]string, error) { return []string{m.location()}, nil }
	}

	m := newMockIGD(t)
	m.entryClient, m.entryDesc = "192.168.1.77", "Console"
	_, _, _, _, err := mapViaUPnP(context.Background(), UDP, 40404, 120, anyHost, nil, at(m))
	select {
	case body := <-m.added:
		t.Errorf("an AddPortMapping was sent for a port the IGD says 192.168.1.77 holds — a "+
			"non-conformant IGD overwrites it: %s", body)
	default:
	}
	if !errors.Is(err, ErrResultCode) {
		t.Errorf("another host's mapping read as %v, want ErrResultCode — the answer a conformant IGD gives", err)
	}

	// The controls: no evidence of a holder, and this host's own mapping (a renewal), both map.
	for name, set := range map[string]func(*mockIGD){
		"an empty answer": func(m *mockIGD) { m.entryClient, m.entryDesc = "", "" },
		"our own mapping": func(*mockIGD) {}, // the mock's default entry is this host's Nib mapping
	} {
		m := newMockIGD(t)
		set(m)
		if _, _, _, _, err := mapViaUPnP(context.Background(), UDP, 40404, 120, anyHost, nil, at(m)); err != nil {
			t.Errorf("control (%s): the mapping was refused: %v", name, err)
		}
	}
}
