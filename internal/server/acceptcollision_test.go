package server

import (
	"net/http"
	"strings"
	"testing"

	"nib/internal/ceremony"
)

// TestARefusedAcceptPinsNobody — /pending 500, and /pending 364's defect through a second door.
//
// The collision refusal (/pending 318, door 2) answered 409 *"Accepting would replace it"* — but
// `pinCeremonyRoster` had already run, so the colliding invitation's convener stayed pinned under the
// LEGITIMATE ceremony's id. Reproduced by the reviewer: `/api/peers` listed them after the 409. The
// 500 branch beside it prunes; this one could not simply prune, because the scope it would prune is
// the legitimate ceremony's own, so the check moves above the pin instead.
//
// Asserted at the ARM door, as /pending 364's test does, because that is the check a pin exists to
// satisfy: a peer the user was told was not accepted must not be one this machine will dial.
func TestARefusedAcceptPinsNobody(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)

	legitText, legitConvener := inviteFor(t, me)
	otherText, otherConvener := inviteFor(t, me)
	legit, err := ceremony.ParseInvitation(legitText)
	if err != nil {
		t.Fatal(err)
	}
	other, err := ceremony.ParseInvitation(otherText)
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: two different proceedings with two different conveners. If they shared a convener, the
	// pin asserted absent below would be present legitimately.
	if strings.EqualFold(legitConvener, otherConvener) || legit.RosterHash == other.RosterHash {
		t.Fatal("setup: the two invitations do not differ in convener and commitment")
	}
	other.ID = legit.ID // same id, different proceeding: the collision
	forged, err := other.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: legitText}); code != http.StatusOK {
		t.Fatalf("setup: the legitimate accept failed: %d %s", code, body)
	}
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept", acceptRequest{Invitation: forged})
	// STIMULUS: this is the collision refusal and not some other one.
	if code != http.StatusConflict || !strings.Contains(body, "Accepting would replace it") {
		t.Fatalf("setup: the colliding accept answered %d %s, want the collision's 409", code, body)
	}

	acode, abody := postForCode(t, c, csrf, ts.URL+"/api/session/arm",
		armRequest{Fingerprint: otherConvener, Bind: "127.0.0.1:0", Transport: "tcp"})
	if acode == http.StatusOK {
		postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
		t.Errorf("after an accept refused 409 %q, arming against ITS convener succeeded — the refusal "+
			"pinned them anyway, scoped to the legitimate ceremony's id", body)
	} else if !strings.Contains(abody, "isn't pinned") {
		t.Errorf("arming against the refused convener failed for the wrong reason (%d %q); this "+
			"assertion is only about the pin", acode, abody)
	}

	// CONTROL: the legitimate convener is still pinned. A fix that pruned the ceremony's scope on
	// the collision would destroy the pin of the proceeding the user DID accept.
	lcode, lbody := postForCode(t, c, csrf, ts.URL+"/api/session/arm",
		armRequest{Fingerprint: legitConvener, Bind: "127.0.0.1:0", Transport: "tcp"})
	if strings.Contains(lbody, "isn't pinned") {
		t.Errorf("the collision refusal unpinned the LEGITIMATE convener (%d %q)", lcode, lbody)
	}
	if lcode == http.StatusOK {
		postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
	}
}
