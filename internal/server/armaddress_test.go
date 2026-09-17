package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestAnArmRefusesAnAddressItCannotUse — /pending 551.
//
// `armRequest.Address` is read in exactly one branch of `handleSessionArm`: the QUIC ceremony arm,
// where it makes the receive role DIAL as well as accept. Every other arm read the field and threw
// it away, answering 200 — so a caller who typed a peer's address to reach them got an arm that
// never dialled, and nothing on the wire or in the response distinguished that from one that did.
//
// **Driven through the route, not against the predicate**, because the defect was never in a
// condition — it was in the absence of one. A unit test on a helper could not have been written,
// since there was no helper to write it against.
//
// The last assertion is the one that pins WHERE the refusal sits. `handleSessionArm` tears down
// this machine's accept-time arm (`displacePolicyArm`) a few lines below, and a guard placed after
// that would answer 400 having already closed a listener the caller never asked it to close.
func TestAnArmRefusesAnAddressItCannotUse(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.EnableDeliveryRearm()
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)

	if acode, abody := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); acode != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", acode, abody)
	}

	// SETUP: accepting armed this machine by policy (D14). That arm is the incumbent the refusals
	// below must leave alone, so the test is worthless if it is not there — assert it rather than
	// assume it, which is what `TestAnExplicitArmDisplacesTheAcceptTimeArm` does for the same
	// reason at the same door.
	if !armedWithin(srv, 3*time.Second) {
		t.Fatal("setup: accepting did not arm, so there is no policy arm for a refused request " +
			"to wrongly displace and the last assertion proves nothing")
	}
	var before sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &before)

	// (a) An address with no ceremony to dial into. `cer` is nil, so the QUIC branch is not even
	// reachable and the address could only ever have been dropped.
	code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "quic", Address: "127.0.0.1:9",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("a manual arm naming a peer address returned %d %q — the address is unreachable "+
			"code on this path, and a 200 tells the caller a dial will happen that cannot",
			code, body)
	}
	if !strings.Contains(body, "invitation") {
		t.Errorf("the refusal does not name the missing half (an invitation), so the caller "+
			"cannot tell it apart from the transport case: %q", body)
	}

	// (b) A real ceremony, armed over TCP. This is the case a user can actually reach by hand, and
	// it is the one that reads most like it works: the invitation is valid, the peer is pinned,
	// and the arm succeeds — it simply never dials what was typed.
	code, body = postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp",
		Invitation: invitation, Address: "127.0.0.1:9",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("a TCP ceremony arm naming a peer address returned %d %q — a TCP arm listens and "+
			"never dials, so the address was accepted and discarded", code, body)
	}
	if !strings.Contains(body, "QUIC") {
		t.Errorf("the refusal does not say which transport would honour the address, which is "+
			"the one thing that would let the caller fix it: %q", body)
	}

	// Neither refusal cost the incumbent arm. Both the flag and the ADDRESS are compared: a
	// displaced-and-rebuilt arm would be armed again on a different ephemeral port, so the flag
	// alone would report a listener that is not the one this machine had.
	var after sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &after)
	if !after.Armed || after.Address != before.Address {
		t.Errorf("a refused arm tore down the accept-time arm: was armed=%v at %q, now armed=%v "+
			"at %q — the guard must run before displacePolicyArm, or a 400 closes a listener the "+
			"caller never asked it to close",
			before.Armed, before.Address, after.Armed, after.Address)
	}

	// CONTROL: the very same TCP ceremony arm, with the address removed, is accepted. Without this
	// the two refusals above are equally satisfied by a handler that refuses every arm.
	if code3, body3 := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp", Invitation: invitation,
	}); code3 != http.StatusOK {
		t.Fatalf("the same arm without an address returned %d %q — the refusal is not about the "+
			"address", code3, body3)
	}
	postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
}
