package server

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P02.S02 — accepting arms the listener (D14).
//
// # The defect these drive, stated once
//
// A signer who accepts an invitation and never arms is, from the convener's side, exactly a signer
// who ignored it: the convener dials, nothing answers, and the proceeding stalls on a party who
// believes they have done their part. **Measured before the slice, with a probe against HEAD:** a
// successful `POST /api/ceremony/accept` left `/api/session/status` answering `{armed: false}`, and
// the only way on was for the user to find the Receive modal, pick the convener out of a peer list
// and press Arm — the second user action D14 removes.

// convenedCeremony convenes a real ceremony and hands back the parts a test needs to put a
// verifiable record on this machine's disk.
//
// **A real `Convene` and not a hand-built `Record`**, because `ReadStored` verifies before it
// returns anything and a fabricated record classifies as `LoadUnparseable` — a test built on one
// would assert the fallback while believing it asserted the deadline.
func convenedCeremony(t *testing.T, life time.Duration, otherFP string) (ceremony.Invitation, ceremony.Record, []byte) {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	fpb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ceremony.Convene(base, ceremony.ConveneRequest{
		Roster: []ceremony.Party{
			{Fingerprint: hex.EncodeToString(fpb), Label: "Convener", Signs: true},
			{Fingerprint: otherFP, Label: "The other party", Signs: true},
		},
		Intent:         "We agree to co-sign the lease",
		Expires:        time.Now().Add(life),
		HopBudget:      ceremonyHopBudget(),
		DeliveryBudget: ceremonyDeliveryLegBudget(),
		ConvenerSigns:  true,
	}, cert, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, inv := range out.Invites {
		if strings.EqualFold(inv.Party.Fingerprint, otherFP) {
			text = inv.Text
		}
	}
	if text == "" {
		t.Fatal("setup: convene issued no invitation for the counterparty")
	}
	parsed, err := ceremony.ParseInvitation(text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed, out.Record, out.Document
}

// armedWithin polls the session status until something is armed, and reports whether it happened.
//
// The arm is raised on a detached goroutine — deliberately, so it cannot delay or fail the accept
// — so a single read after the response is a race the test would lose on a slow machine.
func armedWithin(s *Server, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.sess.status().Armed {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s.sess.status().Armed
}

// TestAcceptingArmsTheListenerAndOnlyInARealNibProcess drives clauses 1 and 3 of the slice
// together, in one table, and the pairing is what makes either one mean anything.
//
// **The two cases are each other's control.** "Armed after accepting" alone could be an arm this
// test's own setup raised; "not armed without the gate" alone could be an arm that was merely
// slower than the wait. Run against one wait and one accept, with the gate as the only difference,
// each case is the other's stimulus — which is the shape a same-wait assertion needs, because the
// negative case is asserting an ABSENCE and an absence has to be timed against something.
func TestAcceptingArmsTheListenerAndOnlyInARealNibProcess(t *testing.T) {
	// **The window is generous on purpose and it is not the figure under test.** What is under
	// test is a difference between two runs of one path; a wait long enough that the positive case
	// is not flaky makes the negative case stronger, not weaker.
	const wait = 3 * time.Second

	for _, tc := range []struct {
		name      string
		realNib   bool
		wantArmed bool
	}{
		{"a real Nib process arms on accepting", true, true},
		{"a harness does not arm", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts, srv := startServerWith(t)
			if tc.realNib {
				srv.EnableDeliveryRearm()
			}
			c, csrf := authedClient(t, ts)
			me := myFingerprint(t, c, ts.URL)
			invitation, _ := inviteFor(t, me)

			// SETUP: nothing is armed before the accept. Without this the positive case is
			// satisfied by any arm at all, including one an earlier unlock raised.
			if srv.sess.status().Armed {
				t.Fatal("something was already armed before the invitation was accepted, so " +
					"this test cannot attribute an arm to accepting")
			}

			code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
				acceptRequest{Invitation: invitation})
			if code != http.StatusOK {
				t.Fatalf("setup: accept returned %d: %s", code, body)
			}

			got := armedWithin(srv, wait)
			if tc.realNib {
				defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
			}
			if got != tc.wantArmed {
				if tc.wantArmed {
					t.Errorf("accepting an invitation left this machine unarmed after %s — the "+
						"convener will dial and nothing will answer, which from their side is "+
						"indistinguishable from a party who ignored the invitation (D14)", wait)
				} else {
					t.Errorf("accepting an invitation opened a listener on a Server that never " +
						"called EnableDeliveryRearm — a test constructs one of these dozens of " +
						"times, and arming a network socket is something a Nib process does, not " +
						"something constructing a Server does")
				}
			}
		})
	}
}

// TestTheHopArmsWindowIsTheRecordsDeadlineWhereThisMachineHoldsARecord is D14's second clause, as
// amended: the bound is read from the record where one exists rather than defaulted to the ceiling.
//
// **The first assertion is the stimulus and it is the one that keeps this honest.** `hopWindowFor`
// returning the deadline could be a function that returns the deadline it was handed regardless of
// whether it read anything, so the ceiling case is asserted FIRST, on the same ceremony, with only
// the record's presence on disk changing between the two reads.
func TestTheHopArmsWindowIsTheRecordsDeadlineWhereThisMachineHoldsARecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const life = 6 * time.Hour
	inv, rec, doc := convenedCeremony(t, life, strings.Repeat("ab", 32))
	cer := &ceremonyID{inv: inv}

	// SETUP: with no record on this machine — the ordinary state of a party who has just
	// accepted — the window is the ceiling, because there is nothing else it could honestly be.
	if got := hopWindowFor(cer); got != ceremony.MaxCeremonyLife {
		t.Fatalf("with no record on disk the hop window is %s, want the %s ceiling — a party who "+
			"has accepted and is waiting holds no record, and flooring them short would take "+
			"them off the network minutes after they accepted", got, ceremony.MaxCeremonyLife)
	}

	if _, err := ceremony.WriteMirror(defaultOutputDir(), rec, doc); err != nil {
		t.Fatal(err)
	}

	got := hopWindowFor(cer)
	if got > life || got < life-time.Minute {
		t.Errorf("with the record on disk the hop window is %s, want about %s — the arm holds "+
			"this machine's one network-reachable surface open, and a 30-day bound on a "+
			"proceeding that ends in six hours is a bound in name only", got, life)
	}
}

// TestTheHopSweepLeavesTheConvenerAlone proves the OUTCOME, and says plainly that it cannot prove
// the rule.
//
// **What it proves:** the sweep does not arm for a ceremony this machine convened, with every
// later guard removed from its path — the ceremony is listed, its invitation is in the vault, and
// the convener is pinned, so the pin check that silently refused the first cut of this test cannot
// refuse it now.
//
// **What it cannot prove, found by the red proof and recorded rather than papered over:** that the
// convener BRANCH is what refuses. Disabling that branch with a mutation that compiles leaves this
// test green, because `ceremonyFor` then reaches `hopBetween`, which refuses `a == b` with *"was
// given as both ends"* — under D22's hub the convener's only possible counterparty is itself. No
// fixture can make the branch the deciding one, so no red proof for it exists and the corpus has
// none; the second half below is a source scan, which is the only instrument left, and it is
// narrow on purpose. A scan proves a line is present; nothing here proves it is load-bearing,
// because it is not.
func TestTheHopSweepLeavesTheConvenerAlone(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.EnableDeliveryRearm()
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)

	// An invitation this machine convened. `inviteForConvener` builds exactly that case, which is
	// also the one `handleCeremonyAccept` refuses at the door — so the sweep is driven directly,
	// through the same vault the route would have handed it.
	invitation, _ := inviteForConvener(t, me)
	inv, err := ceremony.ParseInvitation(invitation)
	if err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	v := srv.vault
	srv.mu.Unlock()
	if v == nil {
		t.Fatal("setup: the vault is not open, so the sweep would return before reaching any rule")
	}
	if err := v.AddCeremonyInvitation(inv.ID, invitation); err != nil {
		t.Fatal(err)
	}
	if err := ceremony.WriteMe(defaultOutputDir(), inv.ID, me); err != nil {
		t.Fatal(err)
	}
	// **The self-pin is what makes this test about the convener rule at all**, and it was missing
	// in the first cut. A machine does not pin itself, so the sweep's `pinnedLabel` check refused
	// this ceremony one step later — the assertion below passed, and passed just as well with the
	// convener rule disabled. The red proof caught it: the mutation went red by failing to
	// COMPILE, which is exactly the outcome `redproof.sh` refuses to accept as a proof, and with a
	// compiling mutation the check came back GREEN. Pinning here removes the later guard so this
	// one is the only thing that can still refuse.
	meFP, derr := hex.DecodeString(me)
	if derr != nil {
		t.Fatal(derr)
	}
	if err := v.AddCeremonyPeer(meFP, "me", inv.ID); err != nil {
		t.Fatal(err)
	}
	if _, pinned := pinnedLabel(v, meFP); !pinned {
		t.Fatal("setup: the self-pin did not take, so the sweep would refuse at the pin check " +
			"and this test would prove nothing about the convener rule")
	}

	// SETUP: the sweep can see this ceremony at all. Without it a green result is a sweep that
	// found nothing, which proves nothing about the convener rule.
	stored, err := ceremony.ListStored(defaultOutputDir(), time.Now())
	if err != nil || len(stored) != 1 || stored[0].ID != inv.ID {
		t.Fatalf("setup: the sweep's own listing does not contain this ceremony: %v %v", stored, err)
	}

	srv.rearmCeremonies(v)
	if srv.sess.status().Armed {
		defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
		t.Error("the sweep armed for a ceremony this machine convened — the convener carries the " +
			"baton to each party in turn, so an arm it holds for itself is the interactive slot " +
			"missing at the moment it reaches out")
	}

	// The scan half. It asserts the branch EXISTS in the sweep's own body, which is all that is
	// available: see the header for why nothing can assert it fires.
	src, err := os.ReadFile("ceremonyarm.go")
	if err != nil {
		t.Fatal(err)
	}
	// **`rearmCeremoniesPreferring`, not `rearmCeremonies` — and the scan caught the rename
	// itself.** Splitting the sweep into a wrapper and a body left this scan reading the wrapper's
	// one line, and it went red naming the rule as missing. That is the correct behaviour for a
	// scan whose subject moved, and it is the reason the anchor is the function that HOLDS the
	// rule rather than the one that shares its name.
	body := funcBodyFrom(string(src), strings.Index(string(src), "func (s *Server) rearmCeremoniesPreferring("))
	if !strings.Contains(body, "strings.EqualFold(me, inv.ConvenerFingerprint)") {
		t.Error("rearmCeremonies no longer compares this machine against the invitation's " +
			"convener. The outcome above is unchanged — hopBetween refuses a self-pair either " +
			"way — so this scan is the only thing that can tell you the topology rule went away")
	}
}

// TestASecondCeremonyArmIsRefusedAsAConflict covers the status mapping the extraction created, and
// it is here because nothing covered it before.
//
// **`armCeremonyHop` returns sentinels and `handleSessionArm` turns them into codes.** That mapping
// is new — the codes used to be written at the three points that produced them — and a named search
// over `internal/server/*_test.go` for `a session is already armed` and for either of the other two
// messages returned **nothing**, so the QUIC ceremony arm's refusals were asserted nowhere at all.
// An extraction that swapped two of the three would have been invisible.
//
// The 409 is the one of the three a test can reach: the other two need a socket that will not open.
// Those are recorded as `not exercised` rather than counted, in the slice's seam inventory.
func TestASecondCeremonyArmIsRefusedAsAConflict(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	arm := armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0",
		Transport: transportQUIC, Invitation: invitation,
	}
	// SETUP: the first arm succeeds, so the refusal below is a SECOND arm being refused and not
	// this build failing to arm at all — which would produce the same non-200 with a different
	// meaning.
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", arm); code != http.StatusOK {
		t.Fatalf("setup: the first QUIC ceremony arm returned %d: %s", code, body)
	}
	defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})

	code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", arm)
	if code != http.StatusConflict {
		t.Errorf("a second ceremony arm returned %d, want %d — the door reports the slot is taken "+
			"through a sentinel now, and a route that maps it to any other code tells the user "+
			"their request was malformed or that Nib broke: %d %s",
			code, http.StatusConflict, code, body)
	}
	if !strings.Contains(body, "a session is already armed") {
		t.Errorf("the conflict body is %q, and it no longer carries the sentence the branch "+
			"answered with before the extraction", body)
	}
}

// TestAnExplicitArmDisplacesTheAcceptTimeArm is a phase-close regression, found by tier 6.
//
// **D14 inverted D21's observable invariant and nothing below tier 6 noticed.** D21's rule is that
// accepting an invitation removes the manual PINNING step, and `ceremonyrepro.sh` observes it the
// only way it can — accept, then arm, and require 200. Once accepting ARMS, that arm came back
// `409 a session is already armed`: the step D21 removed returned as a conflict, and a party
// pressing Arm was told their machine was busy with something the message does not name.
//
// It is pulled down to tier 1 because tier 6 needs two processes and real HTTP, and a regression
// that only a two-process harness can see is one that comes back between runs of it.
func TestAnExplicitArmDisplacesTheAcceptTimeArm(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.EnableDeliveryRearm()
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	// SETUP: the accept-time arm is actually up. Without it this test passes on a build where D14
	// never fired, which is the one build the assertion below cannot be about.
	if !armedWithin(srv, 3*time.Second) {
		t.Fatal("setup: accepting did not arm, so there is no policy arm for an explicit request " +
			"to displace and this test proves nothing")
	}

	// The request the harness makes: a transport and a bind of the caller's choosing, which is why
	// an idempotent 200 would be wrong — the sweep armed QUIC on 0.0.0.0:0.
	code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: transportTCP,
		Invitation: invitation,
	})
	defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
	if code != http.StatusOK {
		t.Fatalf("arming after accepting returned %d, want 200: %s\n"+
			"D21's whole point is that accepting removes the manual step. A party who accepts and "+
			"then presses Arm must not be told their own machine is already busy — that is the "+
			"step D21 removed, returning as a conflict.", code, body)
	}
}

// TestAUsersOwnArmIsNeverDisplaced — the other half, and the one that keeps the fix from becoming a
// way for any request to take a live session's slot.
//
// `displacePolicyArm` yields only a `byPolicy` arm. An arm a user asked for is the incumbent and
// wins, which is the rule `setVerify` already keeps for gates and for the same reason.
func TestAUsersOwnArmIsNeverDisplaced(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); code != http.StatusOK {
		t.Fatalf("setup: accept returned %d: %s", code, body)
	}
	// No EnableDeliveryRearm, so nothing armed by policy: the first arm below is the USER's.
	arm := armRequest{Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: transportTCP,
		Invitation: invitation}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", arm); code != http.StatusOK {
		t.Fatalf("setup: the user's own arm returned %d: %s", code, body)
	}
	defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})

	code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", arm)
	if code != http.StatusConflict {
		t.Errorf("a second arm over a live USER arm returned %d, want %d — displacement is for "+
			"this machine's own accept-time arm and nothing else. A request that can take a live "+
			"session's slot is the tripwire D22 protects, not a courtesy: %s",
			code, http.StatusConflict, body)
	}
}

// TestAPolicyArmWithSomethingOnScreenIsNotDisplaced — the third of `displacePolicyArm`'s three
// conditions, and the one a mutation showed was untested.
//
// **Found by a probe, not by review.** Removing the `se.pending != nil || se.verify != nil` guard
// left every other test in this file green: the two above drive a policy arm with nothing in
// flight, so the condition never decided anything. A build without it tears down an arm whose peer
// is mid-exchange — the spoken check on screen, or a consent request waiting — which answers on the
// user's behalf, the failure `disarmIf` records for the arm-window timer.
func TestAPolicyArmWithSomethingOnScreenIsNotDisplaced(t *testing.T) {
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
		t.Fatal("setup: accepting did not arm, so there is no policy arm to protect")
	}
	defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})

	// Something on screen: the spoken check. Parked directly, because reaching it through a real
	// session needs a peer mid-handshake and this is the state under test, not the route to it.
	pv := &pendingVerify{words: "one two three four", resp: make(chan bool, 1)}
	if !srv.sess.setVerify(pv) {
		t.Fatal("setup: the spoken check would not park, so nothing is in flight to protect")
	}
	defer srv.sess.clearVerifyIf(pv)

	code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: transportTCP,
		Invitation: invitation,
	})
	if code != http.StatusConflict {
		t.Errorf("an arm request displaced a policy arm with the spoken check on screen — it "+
			"returned %d, want %d. A peer is mid-exchange and a person is looking at four words; "+
			"tearing that down answers for them: %s", code, http.StatusConflict, body)
	}
}

// TestTheSweepPrefersTheCeremonyItWasGiven — tier 4d's finding, pulled down to tier 1.
//
// `ListStored` sorts by id, so a sweep with no preference arms for whichever accepted-and-unsigned
// ceremony sorts FIRST — arbitrary from the user's point of view, and it leaves the one they just
// joined unarmed. The accept trigger passes the id it just took on.
//
// **It is driven directly, against a FREE slot, and that is the honest scope of the guarantee.**
// Two accepts in quick succession each start a sweep; making the trigger displace so the second
// could win was tried and backed out, because both then displace and whichever goroutine runs last
// decides. With one interactive slot something must lose when a machine holds two live ceremonies —
// that is `/pending 378`'s recorded residual doubt, not something this sweep can settle. What this
// row asserts is the part that IS settled: given a free slot and a named preference, the sweep arms
// for the named one.
func TestTheSweepPrefersTheCeremonyItWasGiven(t *testing.T) {
	ts, srv := startServerWith(t)
	// Deliberately NOT EnableDeliveryRearm: the accepts below must not arm, so the sweep below
	// runs against a free slot and the preference is the only thing deciding.
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)

	accept := func(invitation string) string {
		t.Helper()
		code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
			acceptRequest{Invitation: invitation})
		if code != http.StatusOK {
			t.Fatalf("setup: accept returned %d: %s", code, body)
		}
		var ar acceptResponse
		if err := json.Unmarshal([]byte(body), &ar); err != nil {
			t.Fatal(err)
		}
		return ar.Ceremony
	}

	// Two ceremonies, and the PREFERRED one must sort second — otherwise listing order already
	// picks it and a build ignoring the preference passes.
	var low, high string
	for i := 0; i < 40 && high == ""; i++ {
		invA, _ := inviteFor(t, me)
		invB, _ := inviteFor(t, me)
		a, err := ceremony.ParseInvitation(invA)
		if err != nil {
			t.Fatal(err)
		}
		b, err := ceremony.ParseInvitation(invB)
		if err != nil {
			t.Fatal(err)
		}
		if a.ID < b.ID {
			low, high = accept(invA), accept(invB)
		}
	}
	if high == "" {
		t.Skip("could not draw two ceremony ids in the needed order in 40 tries")
	}
	if !(low < high) {
		t.Fatalf("setup: %q does not sort before %q, so listing order and the preference agree "+
			"and this test cannot tell them apart", low, high)
	}
	if srv.sess.status().Armed {
		t.Fatal("setup: something is armed, so the slot is not free and the preference is not " +
			"what decides below")
	}

	srv.mu.Lock()
	v := srv.vault
	srv.mu.Unlock()
	srv.rearmCeremoniesPreferring(v, high)
	defer postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})

	srv.sess.mu.Lock()
	a := srv.sess.arms[armInteractive]
	var armedFor string
	if a != nil && a.cer != nil {
		armedFor = a.cer.inv.ID
	}
	srv.sess.mu.Unlock()

	if armedFor != high {
		t.Errorf("the sweep was given %q and armed for %q. Without the preference it takes "+
			"`ListStored`'s order, so the ceremony a user just accepted is left unarmed whenever "+
			"an older one sorts before it — which is arbitrary to them and is what tier 4d hit",
			high, armedFor)
	}
}

// TestTheAcceptTriggerNamesWhatItAccepted is a source scan, and it is one on purpose.
//
// **A probe showed nothing else can see this.** Replacing `s.rearmCeremoniesAsync(v, inv.ID)` with
// `s.rearmCeremoniesAsync(v, "")` left the package green: with ONE ceremony the preference and the
// listing order pick the same one, and with two the slot is already held by the first accept's
// sweep — so the second accept's preference cannot decide anything, and making it decide is the
// racing displacement that was tried and backed out (see `rearmCeremoniesPreferring`).
//
// So the argument is unobservable from behaviour, and the scan is what is left. It proves the id is
// passed; `TestTheSweepPrefersTheCeremonyItWasGiven` proves the sweep honours one when it gets one.
// Neither proves the pair matters on a machine holding two live ceremonies — nothing can, because
// with one interactive slot that outcome is `/pending 378`'s residual doubt rather than a rule.
func TestTheAcceptTriggerNamesWhatItAccepted(t *testing.T) {
	src, err := os.ReadFile("accept.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	body := funcBodyFrom(code, strings.Index(code, "func (s *Server) handleCeremonyAccept("))
	if body == "" {
		t.Fatal("cannot find handleCeremonyAccept — this guard is reading the wrong thing")
	}
	if !strings.Contains(body, "rearmCeremoniesAsync(v, inv.ID)") {
		t.Error("the accept trigger no longer names the ceremony it just accepted. The sweep then " +
			"takes `ListStored`'s order, so on a machine holding an older accepted ceremony the " +
			"one the user just joined is left unarmed — which is what tier 4d hit as `instance 3 " +
			"could not arm before hop 1`")
	}
}
