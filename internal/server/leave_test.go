package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// P05.S01 — leaving a ceremony (D17).
//
// # The defect these drive
//
// Since D14 (v1.128.6) accepting an invitation ARMS, and `rearmCeremonies` renews that arm at every
// unlock. A party who changed their mind had no lever at all short of quitting Nib — and quitting
// is a pause, not a decision: the next unlock arms again.

// leave posts the request and returns the code and body.
func leave(t *testing.T, c *http.Client, csrf, base, id string) (int, string) {
	t.Helper()
	return postForCode(t, c, csrf, base+"/api/ceremony/leave", leaveRequest{Ceremony: id})
}

// TestLeavingStopsTheArmAndKeepsItStopped is the slice's whole acceptance clause.
//
// **The second sweep is the assertion, not the first.** A prune that stopped the arm once and let
// the next unlock raise it again would pass any check made immediately after leaving — and the
// next unlock is exactly what D14 added. So the arm is driven, left, and the sweep is run AGAIN,
// which is the operation a restart performs.
func TestLeavingStopsTheArmAndKeepsItStopped(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.EnableDeliveryRearm()
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, _ := inviteFor(t, me)

	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation})
	if code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	var ar acceptResponse
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}

	// SETUP: the arm actually came up. Without this, "not armed after leaving" is satisfied by a
	// machine that never armed at all, which is what this test would look like if D14 regressed.
	if !armedWithin(srv, 3*time.Second) {
		t.Fatal("setup: accepting did not arm, so this test cannot show that leaving stops an arm")
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{}); code != http.StatusOK {
		t.Fatalf("setup: disarm returned %d: %s", code, body)
	}

	if code, body := leave(t, c, csrf, ts.URL, ar.Ceremony); code != http.StatusOK {
		t.Fatalf("leaving returned %d: %s", code, body)
	}

	// THE ASSERTION: the sweep a restart would run finds nothing to arm for.
	srv.mu.Lock()
	v := srv.vault
	srv.mu.Unlock()
	srv.rearmCeremonies(v)
	if srv.sess.status().Armed {
		postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
		t.Error("the sweep armed for a ceremony this machine had left — leaving is supposed to " +
			"remove the stored invitation the sweep keys on, so an arm here means the next " +
			"unlock brings it straight back and leaving is a gesture rather than a decision")
	}
}

// TestLeavingWritesNoTermination — D17's other half, and the one a user cannot see going wrong.
//
// Leaving reaches nobody. A termination is an ATTESTED refusal the convener learns about and the
// roster is entitled to act on, which is a decline — a different thing, one keystroke away, that
// this user did not choose.
func TestLeavingWritesNoTermination(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, _ := inviteFor(t, me)

	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation})
	if code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	var ar acceptResponse
	if err := json.Unmarshal([]byte(body), &ar); err != nil {
		t.Fatal(err)
	}
	if code, body := leave(t, c, csrf, ts.URL, ar.Ceremony); code != http.StatusOK {
		t.Fatalf("leaving returned %d: %s", code, body)
	}
	_ = srv

	// The receipt records what THIS machine did, and it must say `left` rather than `declined` —
	// they are different facts and only the receipt can keep them apart locally.
	res, err := c.Get(ts.URL + "/api/ceremonies")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var listing ceremoniesResponse
	if err := json.NewDecoder(res.Body).Decode(&listing); err != nil {
		t.Fatal(err)
	}
	var states []string
	for _, r := range listing.Ended {
		if r.Ceremony == ar.Ceremony {
			states = append(states, r.State)
		}
	}
	if len(states) != 1 {
		t.Fatalf("the listing carries %d receipt(s) for the ceremony this machine left, want 1: %v",
			len(states), states)
	}
	if states[0] != ceremony.StateLeft {
		t.Errorf("leaving recorded the end state %q — %q is an attested refusal the convener acts "+
			"on, and this user chose to stop taking part, which reaches nobody",
			states[0], ceremony.StateDeclined)
	}
}

// TestLeavingIsRefusedWhereItWouldOnlyCostTheUser covers both refusals, and each names a different
// harm.
func TestLeavingIsRefusedWhereItWouldOnlyCostTheUser(t *testing.T) {
	t.Run("a ceremony this machine convened", func(t *testing.T) {
		ts, srv := startServerWith(t)
		c, csrf := authedClient(t, ts)
		me := myFingerprint(t, c, ts.URL)
		invitation, _ := inviteForConvener(t, me)
		inv, err := ceremony.ParseInvitation(invitation)
		if err != nil {
			t.Fatal(err)
		}
		srv.mu.Lock()
		v := srv.vault
		srv.mu.Unlock()
		if err := v.AddCeremonyInvitation(inv.ID, invitation); err != nil {
			t.Fatal(err)
		}
		// SETUP: the invitation is genuinely readable, so the refusal below is the convener rule
		// and not the unparseable-invitation arm one branch above it.
		if _, ok := v.CeremonyInvitationFor(inv.ID); !ok {
			t.Fatal("setup: the invitation did not store, so no branch below can be attributed")
		}
		code, body := leave(t, c, csrf, ts.URL, inv.ID)
		if code != http.StatusConflict {
			t.Fatalf("a convener leaving their own ceremony returned %d, want %d: %s",
				code, http.StatusConflict, body)
		}
		if !strings.Contains(body, "convened this ceremony") {
			t.Errorf("the refusal is %q, and it does not tell the convener that the parties are "+
				"waiting on them — which is the only reason this is refused", body)
		}
	})

	t.Run("a ceremony this machine has already signed", func(t *testing.T) {
		ts, srv2 := startServerWith(t)
		c, csrf := authedClient(t, ts)
		me := myFingerprint(t, c, ts.URL)
		inv, rec, doc := convenedCeremony(t, 6*time.Hour, me)
		if _, err := ceremony.WriteMirror(defaultOutputDir(), rec, doc); err != nil {
			t.Fatal(err)
		}
		// **The invitation is stored, and without it this case was latently vacuous** — found by
		// the red proof, which is the only thing that could have found it. With no invitation in
		// the vault, disabling the already-signed guard let the request fall through to the
		// *no-invitation* refusal, which is also a 409: the status assertion passed against a
		// branch that has nothing to do with signing, and only the sentence assertion noticed.
		// Storing it removes that branch, so the guard under test is the only one left that can
		// refuse and its removal shows up as a leave that SUCCEEDS.
		text, eerr := inv.Encode()
		if eerr != nil {
			t.Fatal(eerr)
		}
		srv2.mu.Lock()
		v2 := srv2.vault
		srv2.mu.Unlock()
		if err := v2.AddCeremonyInvitation(inv.ID, text); err != nil {
			t.Fatal(err)
		}
		// SETUP: the record reads as OK, which is the discriminator the refusal uses. Without
		// this the refusal below could be the no-invitation arm instead.
		if st := ceremony.ReadStored(defaultOutputDir(), inv.ID, time.Now()); st.State != ceremony.LoadOK {
			t.Fatalf("setup: the mirrored record reads as %q, so the signed-already branch is "+
				"not the one this case reaches", st.State)
		}
		code, body := leave(t, c, csrf, ts.URL, inv.ID)
		if code != http.StatusConflict {
			t.Fatalf("leaving after signing returned %d, want %d: %s",
				code, http.StatusConflict, body)
		}
		if !strings.Contains(body, "already signed") {
			t.Errorf("the refusal is %q — it must say the signature is already on the document, "+
				"because the only effect of leaving now is that this party's own copy never "+
				"arrives", body)
		}
	})
}

// **A teardown answers a parked consent, and that answer must not read as a DECLINE** (P05's phase
// close, measured).
//
// `handleCeremonyLeave` reaches `stopListeningFor` → `disarmWhen`, which answers any parked consent
// so the peer's goroutine is not left on a channel nobody will write to. That release is necessary.
// What it used to send was a bare `accept: false` — and `Confirm` reads that as a person refusing:
// it prunes this party's ceremony pins and returns `(false, nil)`, which travels the wire as
// `ackDeclined`, which the convener turns into `endCeremony(StateDeclined)` — a SIGNED termination
// naming this party, plus an `ended-by` marker.
//
// That is precisely what `handleCeremonyLeave`'s own door says must not happen: *"Minting a
// termination here would put a withdrawal on the record that the user did not choose."*
// `TestLeavingWritesNoTermination` covers the quiet path; this is the path with somebody on the
// line, and it is the one where leaving is a keystroke away from an attested refusal.
func TestATeardownAnswersAConsentWithoutDecliningIt(t *testing.T) {
	se := &session{}
	cer := &ceremonyID{}
	se.arms[armInteractive] = &arm{kind: armInteractive, cer: cer}
	resp := make(chan sessionDecision, 1)
	se.pending = &pendingReq{resp: resp}

	// SETUP: there really is an arm to tear down and a consent parked on it, or the release below
	// is a channel nobody was waiting on.
	if se.arms[armInteractive] == nil || se.pending == nil {
		t.Fatal("setup: nothing is armed or nothing is parked")
	}

	if n := se.disarmWhen(func(a *arm) bool { return a.cer == cer }); n != 1 {
		t.Fatalf("the teardown released %d arm(s), want 1 — the answer below would then be nobody's", n)
	}

	select {
	case d := <-resp:
		if d.accept {
			t.Fatal("a teardown answered the consent with ACCEPT, which would sign the document")
		}
		if !d.torn {
			t.Error("a teardown answers a parked consent with a bare refusal. `Confirm` reads that " +
				"as the user declining: it prunes this party's ceremony pins and puts `ackDeclined` " +
				"on the wire, so the CONVENER mints a signed Termination naming a party who pressed " +
				"Leave and refused nothing. The two outcomes are one keystroke apart and the wire " +
				"already has a byte for each")
		}
	default:
		t.Fatal("the teardown left the parked consent unanswered — the peer's goroutine then sits " +
			"on a channel nobody will write to until the session deadline, which is the hazard the " +
			"release exists for")
	}
}
