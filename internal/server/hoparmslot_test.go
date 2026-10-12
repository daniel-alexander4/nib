package server

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// TestARefusedHopArmOpensNoSocket — `/pending 517` clause (ii), the shape of `/pending 381`.
//
// **The count is zero, and it is asserted by making the bind IMPOSSIBLE rather than by counting
// file descriptors.** `armCeremonyHop` opened a UDP socket, started a DHT server on it and opened a
// handshaked QUIC listener, and only then asked for the interactive slot — closing all three again
// when the slot was taken. An fd census in this process would have to see past the httptest server,
// the vault and whatever else the suite left running; an unbindable address answers the same
// question exactly, because the two orderings produce two DIFFERENT sentinels:
//
//   - slot asked first  -> errSessionArmed, and net.ListenPacket was never reached;
//   - endpoint first    -> errCeremonyEndpoint, because the bind is the thing that failed.
//
// So the assertion is which error comes back, and the second half is what stops that being a trick:
// with the slot FREE, the very same call must answer errCeremonyEndpoint — which proves the address
// really is unbindable, and that the first half was the slot refusing rather than some path that
// never binds at all.
func TestARefusedHopArmOpensNoSocket(t *testing.T) {
	// `127.0.0.1:` with a port that does not parse. `net.ListenPacket` fails in the resolver, so
	// this needs no DNS, no privilege, and no port that might happen to be free.
	const unbindable = "127.0.0.1:not-a-port"

	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	// The USER's own arm, so `displacePolicyArm` cannot hand the slot over — the door under test
	// must find it genuinely taken.
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0",
		Transport: transportQUIC, Invitation: invitation,
	}); code != http.StatusOK {
		t.Fatalf("setup: the first QUIC ceremony arm returned %d: %s", code, body)
	}

	// THE CASE. A zero-value ceremony identity is enough: neither branch reads a field of it before
	// the two lines under test, and `close()` is nil-safe on every field it does touch.
	err := srv.armCeremonyHop(context.Background(), &ceremonyID{}, nil, nil, nil, nil,
		unbindable, "Convener", sessionModeCoSign, true)
	if !errors.Is(err, errSessionArmed) {
		t.Errorf("a hop arm whose slot was already taken answered %v, want %v — the endpoint is "+
			"opened before the slot is asked for, so every sweep on a machine whose user holds "+
			"the slot binds a UDP socket, starts a DHT server and opens a QUIC listener that "+
			"nothing will ever own, and then closes all three", err, errSessionArmed)
	}

	// THE FLOOR. Free the slot and repeat: the same call must now get as far as the bind and fail
	// there. Without this, an `armCeremonyHop` that refused everything before opening anything
	// would satisfy the assertion above.
	postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
	ferr := srv.armCeremonyHop(context.Background(), &ceremonyID{}, nil, nil, nil, nil,
		unbindable, "Convener", sessionModeCoSign, true)
	if !errors.Is(ferr, errCeremonyEndpoint) {
		t.Fatalf("with the slot FREE the same call answered %v, want %v — %q is not actually "+
			"unbindable, so the refusal above proves nothing about ordering", ferr,
			errCeremonyEndpoint, unbindable)
	}
}
