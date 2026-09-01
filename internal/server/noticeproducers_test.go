package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/testpdf"
)

// breakHome makes ~/nib a regular FILE, so every MkdirAll under it refuses deterministically on
// every platform and as any user. `receivedwrite_test.go` established the fixture and the reason: a
// chmod-based one is a no-op for root and on Windows.
func breakHome(t *testing.T, home string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(home, "nib")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "nib"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestAFailedHopMirrorTellsTheSigner is `hop-not-mirrored`'s producer-side reader (/pending 346).
//
// # Why this one and not the other two
//
// Of the three `noteFailure` kinds, this is the one where the user **has signed and kept nothing**
// — the state D24 exists to prevent. Its sibling `received-not-saved` got a real producer-side
// reader at P08.S05a (`TestAFailedReceivedWriteIsNotAnAcknowledgement`); this one did not. What
// existed was a surface test calling `noteFailure` directly (`rearm_test.go`) plus `l3_test.go`
// driving `mirrorHop`'s SUCCESS path, so nothing ever observed the producer reaching its failure
// branch — the notice was asserted about a call the product might never make.
//
// # The three arms
//
// The writable arm is the control: without it the failure arm is satisfied by a `mirrorHop` that
// refuses everything. The no-record arm is the one that would bite a user hardest if it regressed —
// every ordinary two-party co-sign goes through here, and alarming on each would make the sticky
// notice noise the moment it started firing.
func TestAFailedHopMirrorTellsTheSigner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	final, _ := convenedFor(t)
	rec, err := ceremony.Extract(final)
	if err != nil {
		t.Fatalf("setup: the convened document carries no readable record (%v) — mirrorHop would "+
			"take its ErrNoRecord early return and every arm below would be about nothing", err)
	}

	// CONTROL: a writable ~/nib mirrors and says nothing.
	okServer := &Server{}
	okServer.mirrorHop(final)
	if st := okServer.sess.status(); st.Notice != nil {
		t.Fatalf("setup: a SUCCESSFUL mirror left notice %+v — the failure arm below could not "+
			"then be distinguished from a path that always complains", st.Notice)
	}
	path := filepath.Join(home, "nib", "ceremonies", rec.ID, "document.pdf")
	if _, serr := os.Stat(path); serr != nil {
		t.Fatalf("setup: a successful mirrorHop wrote nothing to %s (%v), so the failure arm "+
			"below is not a failure of anything", path, serr)
	}

	// THE FAILURE. The signature is on the document the peer holds either way; what is gone is
	// this machine's own copy, which is what lets Nib continue the ceremony after a restart.
	breakHome(t, home)
	badServer := &Server{}
	badServer.mirrorHop(final)
	st := badServer.sess.status()
	if st.Notice == nil {
		t.Fatal("the hop could not be mirrored and the user was told nothing. The log line this " +
			"replaced went to a stderr a double-clicked launch sends nowhere — which is the " +
			"argument cmd/nib/main.go already makes about its own hand-off notice — and this " +
			"is the failure where the user most needs to know: they have signed, and this " +
			"machine kept no copy")
	}
	if st.Notice.What != "hop-not-mirrored" {
		t.Errorf("the failed mirror left notice kind %q, want \"hop-not-mirrored\"; `what` is the "+
			"stable key the surface branches on, and reflectNotice offers the rescue button on "+
			"the two kinds that still have a document to save", st.Notice.What)
	}
	if !strings.Contains(st.Notice.Detail, "~/nib/ceremonies") {
		t.Errorf("the notice detail is %q and does not say where the missing copy should have "+
			"been; a user told only that something failed cannot act on it", st.Notice.Detail)
	}

	// AND THE ORDINARY CO-SIGN, which is the majority of arrivals: no record, no ceremony,
	// nothing to mirror, and NOT a failure to report.
	plain, perr := testpdf.Text("an ordinary two-party co-sign")
	if perr != nil {
		t.Fatal(perr)
	}
	quiet := &Server{}
	quiet.mirrorHop(plain)
	if n := quiet.sess.status().Notice; n != nil {
		t.Errorf("a document with no ceremony record raised %+v. Every ordinary co-sign would "+
			"then plant a sticky failure notice on a machine where nothing failed", n)
	}
}

// TestARefusedArrivalTellsTheLocalUser is /pending 345's half: the `ceremony-ended` kind an
// inventory row claimed and nothing produced.
//
// # What the row said, and why it was owed rather than wrong
//
// P08.S04a's seam inventory carries a row P5 — `refusal → noteFailure → sticky sessionStatus`,
// observable `notice.what == "ceremony-ended"`, zero-meaning **"the peer is told and the local user
// is not"**. A named search (`grep -rn '"ceremony-ended"' --include=*.go --include=*.js .`)
// returned nothing: only three kinds existed. So the row asserted coverage the tree did not have,
// which is the one failure the instrument exists to prevent.
//
// It was owed, and the zero-meaning is what settles it. The gate has two callers.
// `handleSessionInitiate` is an HTTP handler and answers 409, so its user is told. `Confirm` runs
// on the RECEIVING side inside a background arm goroutine with no response to write into — the
// exact case P08.S08's sticky notice exists for. The peer got a named wire code at S04a; the local
// user got silence, and stayed armed waiting for a proceeding their own machine had just declared
// over.
//
// # Two kinds, not one and not three
//
// `ceremony-ended` says the PROCEEDING is over — the user should stop waiting. Every other refusal
// is about THIS arrival and not the proceeding, so the arm is still worth holding. That is a real
// behavioural difference and it is what `what` is for; splitting further would be keys nothing
// branches on.
func TestARefusedArrivalTellsTheLocalUser(t *testing.T) {
	doc, invText := convenedFor(t)
	inv, err := ceremony.ParseInvitation(invText)
	if err != nil {
		t.Fatal(err)
	}

	// refuseAt drives the REAL receiving-side gate: an armed session, the production confirmer,
	// and a document handed to it at `now`. `checkArrival` runs before `setPending`, so a refusal
	// returns without any consent being asked and the notice is whatever the product wrote.
	refuseAt := func(t *testing.T, d []byte, now time.Time) (*Server, error) {
		t.Helper()
		s := &Server{}
		ln := &stubListener{}
		cer := &ceremonyID{inv: inv}
		if !s.sess.arm(ln, cer) {
			t.Fatal("setup: the session refused to arm")
		}
		t.Cleanup(s.sess.disarm)
		// The gate reads the wall clock, so an expired ceremony is expressed by expiring the
		// RECORD rather than by moving `now` — which is what a real signer's clock would see.
		sc := sessionConfirmer{s: s, saw: &reached{}, anchor: consentAnchor{ln: ln}, cer: cer}
		_, _, _, cerr := sc.Confirm(p2p.SignerAttestation{}, d)
		return s, cerr
	}

	// CONTROL FIRST, or a gate that refuses everything satisfies both arms below. The document
	// this invitation was made for gets PAST the gate and is parked for consent; the user declines
	// so the call returns instead of sitting out the five-minute consent window. The tell is that
	// no arrival notice was written — a decline is the user's answer, not a refusal by the gate.
	okServer := &Server{}
	okLn := &stubListener{}
	okCer := &ceremonyID{inv: inv}
	if !okServer.sess.arm(okLn, okCer) {
		t.Fatal("setup: the session refused to arm")
	}
	t.Cleanup(okServer.sess.disarm)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if okServer.sess.pendingPDF() != nil {
				okServer.sess.respond(sessionDecision{accept: false})
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	okSC := sessionConfirmer{s: okServer, saw: &reached{}, anchor: consentAnchor{ln: okLn}, cer: okCer}
	okAccepted, _, _, _ := okSC.Confirm(p2p.SignerAttestation{}, doc)
	if okAccepted {
		t.Fatal("setup: the control accepted, so the decline path this fixture depends on did " +
			"not run and the call did not return for the reason assumed")
	}
	if n := okServer.sess.status().Notice; n != nil {
		t.Fatalf("setup: the document this invitation was made for was refused at the gate "+
			"(notice %+v), so every refusal below would be a gate that refuses everything", n)
	}

	// ARM 1 — the proceeding is over. `checkArrival` runs `recordOutlivesBudget` with a budget of
	// zero, so a record whose `Expires` has passed is the ended case and nothing else.
	expired, expText := convenedExpiring(t, -time.Minute)
	expInv, perr := ceremony.ParseInvitation(expText)
	if perr != nil {
		t.Fatal(perr)
	}
	endedServer := &Server{}
	endedLn := &stubListener{}
	endedCer := &ceremonyID{inv: expInv}
	if !endedServer.sess.arm(endedLn, endedCer) {
		t.Fatal("setup: the session refused to arm")
	}
	t.Cleanup(endedServer.sess.disarm)
	endedSC := sessionConfirmer{s: endedServer, saw: &reached{},
		anchor: consentAnchor{ln: endedLn}, cer: endedCer}
	_, _, _, endedErr := endedSC.Confirm(p2p.SignerAttestation{}, expired)
	if endedErr == nil {
		t.Fatal("setup: an arrival for a ceremony that ended a minute ago was admitted, so " +
			"there is no refusal for the notice to be about")
	}
	st := endedServer.sess.status()
	if st.Notice == nil || st.Notice.What != "ceremony-ended" {
		t.Errorf("a refusal past the deadline left notice %+v; want a sticky \"ceremony-ended\". "+
			"The peer gets wire code 13 and the local user, whose arm is a background goroutine "+
			"with no response to write into, gets nothing — and keeps waiting for a proceeding "+
			"this machine has already declared over", st.Notice)
	}

	// ARM 2 — a valid record for a DIFFERENT proceeding. Refused too, and it is NOT the ended
	// kind: this arrival is wrong, the proceeding is not over, and the arm is worth holding.
	other, _ := convenedFor(t)
	mismatchServer, mismatchErr := refuseAt(t, other, time.Now())
	if mismatchErr == nil {
		t.Fatal("setup: a document carrying a different ceremony's record was admitted")
	}
	st = mismatchServer.sess.status()
	if st.Notice == nil || st.Notice.What != "arrival-refused" {
		t.Errorf("a roster mismatch left notice %+v; want \"arrival-refused\". Reporting it as "+
			"\"ceremony-ended\" tells the user their proceeding is over when it is running, and "+
			"the honest answer is that somebody handed them the wrong document", st.Notice)
	}
	// And the two are genuinely distinguishable, or one key would do.
	if st.Notice != nil && st.Notice.Summary == "" {
		t.Error("the refusal notice carries no summary, so reflectNotice renders nothing")
	}
}

// TestTheArrivalRefusalReachesTheLocalUserAtEveryGate is the ADR-009 half of the test above.
//
// It asserts ROUTING, not the sentence each site prints: the defect this prevents is a refusal path
// added later that answers the peer and drops the local half, which is precisely the state
// `Confirm` was in between P08.S04a and /pending 345. Comparing the known sites for agreement would
// say nothing about a new one.
func TestTheArrivalRefusalReachesTheLocalUserAtEveryGate(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	i := strings.Index(code, "func (sc sessionConfirmer) Confirm(")
	if i < 0 {
		t.Fatal("cannot find sessionConfirmer.Confirm — this guard is pinned to a function that " +
			"no longer exists under that name, so a clean result means nothing")
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("Confirm: the brace matcher read an empty body")
	}
	gate := strings.Index(body, "checkArrival(")
	if gate < 0 {
		t.Fatal("Confirm does not call checkArrival — TestTheConsentGateRoutesThroughTheArrivalCheck " +
			"owns that rule and this guard is about what happens when it refuses")
	}
	if !strings.Contains(body, "noteArrivalRefusal(") {
		t.Error("Confirm refuses an arrival without telling the user on THIS machine. It runs in " +
			"a background arm goroutine with no response to write into: the peer gets a named " +
			"wire code and the local user gets silence, and goes on waiting for a proceeding " +
			"their own machine has refused. That is P08.S04a's inventory row P5, whose " +
			"zero-meaning is 'the peer is told and the local user is not'")
	}
	// The ORDER: the notice is recorded before the refusal is returned, or the return unwinds
	// past the only place that knows what happened.
	if note := strings.Index(body, "noteArrivalRefusal("); note < gate {
		t.Error("Confirm records the refusal notice BEFORE it has asked the gate, so the notice " +
			"cannot be about the gate's answer")
	}
}
