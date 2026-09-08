package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// /pending 378 — leaving a ceremony did not stop this machine listening for it.
//
// # The defect this drives
//
// D14 made accepting an invitation ARM, and the arm is renewed at every unlock, so a party who
// changed their mind had no lever short of quitting Nib. `POST /api/ceremony/leave` shipped as that
// lever and its own doc said the prune *"stops the arm on the next sweep and it never comes back"*.
// Only the second half was true. `rearmCeremonies` **skips** a ceremony it holds no invitation for
// — `continue`, not a teardown — so the standing arm went on holding the single interactive slot
// and its QUIC endpoint until the process exited. Named search over the leave path returned **zero**
// occurrences of `disarm`.
//
// # Why the assertion is the SLOT and not a flag
//
// `armed:false` on its own is what a fix that cleared a boolean would produce. The thing the user
// is owed back is the slot, so the test takes it: a second arm for a different peer must now
// succeed where it was refused `409 a session is already armed` a moment earlier. That refusal is
// the setup, so the assertion cannot pass against a build that never armed in the first place.
func TestLeavingACeremonyReleasesTheArmItHeld(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	left, leftConvener := inviteFor(t, me)
	// A SECOND ceremony, and it is what makes "the slot is free again" observable. Re-arming the
	// one just left cannot show it: leaving revokes that convener's ceremony pin, so the arm is
	// refused *"that peer isn't pinned"* — correct behaviour, and indistinguishable here from a
	// slot that is still held. Measured, on the first run of this test.
	other, otherConvener := inviteFor(t, me)

	for _, inv := range []string{left, other} {
		if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
			acceptRequest{Invitation: inv}); code != http.StatusOK {
			t.Fatalf("setup: accept returned %d: %s", code, body)
		}
	}
	leftArm := armRequest{
		Fingerprint: leftConvener, Bind: "127.0.0.1:0",
		Transport: transportQUIC, Invitation: left,
	}
	otherArm := armRequest{
		Fingerprint: otherConvener, Bind: "127.0.0.1:0",
		Transport: transportQUIC, Invitation: other,
	}
	// SETUP 1: the arm goes up. A non-200 here and every assertion below would be satisfied by a
	// build that cannot arm at all.
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", leftArm); code != http.StatusOK {
		t.Fatalf("setup: the ceremony arm returned %d: %s", code, body)
	}
	defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})

	// SETUP 2: the slot is genuinely taken — the state the user is asking to be let out of. Without
	// it the final assertion could pass because nothing was ever holding the slot.
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", otherArm); code != http.StatusConflict {
		t.Fatalf("setup: arming a second ceremony returned %d %q, want %d — the slot is not held, "+
			"so releasing it cannot be observed here", code, body, http.StatusConflict)
	}

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/leave",
		leaveRequest{Ceremony: ceremonyIDIn(t, left)}); code != http.StatusOK {
		t.Fatalf("leave returned %d: %s", code, body)
	}

	var st sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &st)
	if st.Armed {
		t.Errorf("this machine is still armed for a ceremony the user has just left. The prune " +
			"stops the arm coming BACK; nothing tore down the one already up, so it holds the " +
			"single interactive slot and its endpoint until Nib is quit — which is the condition " +
			"the leave lever exists to relieve")
	}
	// **And the slot is genuinely free, not merely flagged free.** SETUP 2 proved this exact
	// request was refused while the arm stood.
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", otherArm); code != http.StatusOK {
		t.Errorf("after leaving, arming a different ceremony returned %d %q — the arm was cleared "+
			"as a flag and the listener it stood for is still holding the slot", code, body)
	}
}

// TestLeavingOneCeremonyLeavesAnotherAlone is the other arm, and it is what keeps the teardown from
// being `disarm()` with extra steps.
//
// `stopListeningFor` matches on the ceremony id. A teardown that matched everything would satisfy
// the test above completely — and would tear down an unrelated ceremony's arm, or the delivery arm
// of a ceremony this party has already signed, on a user action that named neither.
func TestLeavingOneCeremonyLeavesAnotherAlone(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	kept, keptConvener := inviteFor(t, me)
	left, _ := inviteFor(t, me)

	for _, inv := range []string{kept, left} {
		if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
			acceptRequest{Invitation: inv}); code != http.StatusOK {
			t.Fatalf("setup: accept returned %d: %s", code, body)
		}
	}
	// The arm this machine keeps: the one whose ceremony is NOT being left.
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: keptConvener, Bind: "127.0.0.1:0",
		Transport: transportQUIC, Invitation: kept,
	}); code != http.StatusOK {
		t.Fatalf("setup: arming the kept ceremony returned %d: %s", code, body)
	}
	defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})

	leftID := ceremonyIDIn(t, left)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/leave",
		leaveRequest{Ceremony: leftID}); code != http.StatusOK {
		t.Fatalf("leave returned %d: %s", code, body)
	}

	var st sessionStatus
	sessGet(t, c, ts.URL+"/api/session/status", &st)
	if !st.Armed {
		t.Error("leaving one ceremony tore down the arm this machine holds for a DIFFERENT one. " +
			"The user named one proceeding; a teardown that matches everything takes the peer " +
			"they are still waiting for with it, and nothing tells them")
	}
}

// ceremonyIDIn reads the id out of the invitation the caller already holds, through the package's
// own parser rather than by re-deriving it.
func ceremonyIDIn(t *testing.T, invitation string) string {
	t.Helper()
	inv, err := ceremony.ParseInvitation(strings.TrimSpace(invitation))
	if err != nil {
		t.Fatal(err)
	}
	return inv.ID
}

// TestTheCloseOutSweepReleasesTheArmToo — the second call site of `stopListeningFor`, and it had
// none of its own coverage.
//
// **Recorded because it was a MUTATION SURVIVOR.** With the sweep's call deleted, every test
// written alongside the fix stayed green: they all drive the leave route, and the two sites are
// different doors onto one rule. A rule that reaches one of its two sites is the shape ADR-009
// names, and the way it is found is a mutation on the site nobody drove.
//
// The user's leave and the sweep's close-out are the same decision — *this machine considers the
// ceremony over* — reached by a button and by a deadline. A party who has signed holds a delivery
// arm waiting for their finished copy, and a ceremony being moved out of the live set must not go
// on holding the slot for it.
func TestTheCloseOutSweepReleasesTheArmToo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, v := unlockedServer(t)
	rec, _, _ := ceremonyOnDisk(t)

	// Make it closeable: a delivered document is the sweep's central case. Without this the
	// assertion below is satisfied by a sweep that closes nothing at all.
	path := deliveredPathFor(rec)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("the finished document"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	srv.instanceToken = "this-one"
	srv.mu.Unlock()

	// The arm this ceremony holds. Placed through the session's own setter rather than by opening
	// a socket: what is under test is the SLOT being released, and a listener would add a second
	// thing that can fail for reasons this test is not about.
	cer := &ceremonyID{inv: ceremony.Invitation{ID: rec.ID}}
	if !srv.sess.armCeremony(cer, "127.0.0.1:0", func() {}) {
		t.Fatal("setup: the slot could not be taken, so releasing it cannot be observed")
	}
	if !srv.sess.status().Armed {
		t.Fatal("setup: the session does not report armed after taking the slot")
	}

	srv.closeOutEnded(v, time.Now())

	if srv.sess.status().Armed {
		t.Error("the sweep closed a ceremony out of the live set and left this machine listening " +
			"for it. The close-out moves the folder and drops the pins; an arm that outlives both " +
			"holds the single interactive slot for a proceeding this machine has decided is over")
	}
}
