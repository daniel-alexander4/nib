package server

import (
	"net"
	"net/http"
	"testing"
	"time"
)

// TestAnArmThatCannotOpenLeavesTheAcceptTimeArm — `/pending 623` part 2.
//
// `handleSessionArm` displaced this machine's accept-time arm as its first act and only then tried
// to open what would replace it. Three things after that line can refuse — a typed address that
// does not resolve, a ceremony endpoint that will not open, a listener that will not bind — and
// each answered 400 having already torn the incumbent down, so the party who had accepted was off
// the network until the next sweep, on a request that had been refused.
//
// Each refusal is driven through the route with a port another socket already holds, and the
// incumbent is compared by ADDRESS as well as by flag: a displaced-and-rebuilt arm is armed again
// on a different ephemeral port.
func TestAnArmThatCannotOpenLeavesTheAcceptTimeArm(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.EnableDeliveryRearm()
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	if !armedWithin(srv, 3*time.Second) {
		t.Fatal("setup: accepting did not arm, so there is no policy arm for a refused request " +
			"to wrongly displace")
	}
	defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
	var before sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &before)

	// Ports that cannot be bound: one TCP, one UDP, each held by this test.
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no TCP socket to hold here: %v", err)
	}
	defer tcp.Close()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no UDP socket to hold here: %v", err)
	}
	defer udp.Close()

	refusals := map[string]armRequest{
		"a TCP listener whose port is taken": {
			Fingerprint: convenerFP, Bind: tcp.Addr().String(), Transport: transportTCP, Invitation: invitation},
		"a manual arm whose port is taken": {
			Fingerprint: convenerFP, Bind: tcp.Addr().String(), Transport: transportTCP},
		"a QUIC ceremony endpoint whose port is taken": {
			Fingerprint: convenerFP, Bind: udp.LocalAddr().String(), Transport: transportQUIC, Invitation: invitation},
	}
	for name, req := range refusals {
		code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", req)
		if code == http.StatusOK {
			t.Fatalf("setup: %s was armed (%s), so nothing was refused and this case proves nothing", name, body)
		}
		var after sessionStatus
		sessGet(t, c, ts.URL+"/api/session/status", &after)
		if !after.Armed || after.Address != before.Address {
			t.Errorf("%s was refused (%d) and the accept-time arm went with it: was armed=%v at %q, "+
				"now armed=%v at %q — a refused arm must displace nothing",
				name, code, before.Armed, before.Address, after.Armed, after.Address)
		}
	}

	// CONTROL: the same arm on a free port is installed, and it is the new arm that holds the slot.
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: transportTCP, Invitation: invitation,
	}); code != http.StatusOK {
		t.Fatalf("the same arm on a free port returned %d %q — the refusals above are equally "+
			"satisfied by a route that refuses every arm", code, body)
	}
	var after sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &after)
	if !after.Armed || after.Address == before.Address {
		t.Errorf("an explicit arm that opened did not take the slot: armed=%v at %q (the policy arm was at %q)",
			after.Armed, after.Address, before.Address)
	}
}
