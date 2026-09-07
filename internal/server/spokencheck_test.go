package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// P02.S04 — the spoken check records that it was presented (D5).
//
// # The defect these drive
//
// D5: *"The record notes whether the check was presented, so a later reader can tell a confirmed
// ceremony from one where the modal never appeared."* Nothing recorded it. `autoVerifier` confirms
// without a human and `ConfirmVerification` can refuse with `errVerifyBusy` before anything reaches
// the screen — and both leave a machine that looks, afterwards, exactly like one whose user read
// the four words and said yes.
//
// # Why the record is local and unattested, in one line
//
// Two of the three states leave NO SIGNATURE: `p2p.Receive` runs the spoken check before it reads a
// document byte, so a refused or unanswered check produces nothing for a signed field to ride on.

// TestTheSpokenCheckRecordsAllThreeOutcomes is the acceptance clause, driven through the one
// function that produces all three.
//
// **Driven through `noteVerification` rather than through a live session**, because the three
// outcomes are three exits of `ConfirmVerification` and only one of them (`errVerifyBusy`) is
// reachable without a peer mid-handshake. The routing — that each exit calls this — is asserted
// separately by the source scan below, which is the same split `recitalFor` uses one field over.
func TestTheSpokenCheckRecordsAllThreeOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		presented, confirmed bool
		wantSentence         string
	}{
		{"confirmed", true, true, "the user read the words and said they matched"},
		{"presented and not confirmed", true, false, "nobody answered, or they said no"},
		{"never presented", false, false, "the words reached nobody"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			id, err := ceremony.NewID()
			if err != nil {
				t.Fatal(err)
			}
			// The directory is the caller's — `WriteVerification` deliberately does not create
			// one. `WriteMe` is what an accept runs, and it is the honest way to get the state a
			// real hop is in.
			if err := ceremony.WriteMe(defaultOutputDir(), id, "ab"); err != nil {
				t.Fatal(err)
			}

			// SETUP: absence first, on the same ceremony. Without it the assertions below are
			// satisfied by a reader that returns a fixed answer — and absence is a DISTINCT
			// fourth state (unknown), not a fourth outcome.
			if st := ceremony.ReadStored(defaultOutputDir(), id, time.Now()); st.Verification != nil {
				t.Fatalf("a ceremony with no note reports one (%+v) — absence is UNKNOWN and a "+
					"reader that invents an answer here would accuse a party of skipping a check "+
					"they performed", st.Verification)
			}

			sv := sessionVerifier{cer: &ceremonyID{inv: ceremony.Invitation{ID: id}}}
			sv.noteVerification(tc.presented, tc.confirmed)

			st := ceremony.ReadStored(defaultOutputDir(), id, time.Now())
			if st.Verification == nil {
				t.Fatalf("no note was recorded for %q — %s, and afterwards this machine looks "+
					"exactly like one whose user confirmed", tc.name, tc.wantSentence)
			}
			if st.Verification.Presented != tc.presented || st.Verification.Confirmed != tc.confirmed {
				t.Errorf("the note reads presented=%v confirmed=%v, want %v/%v — %s",
					st.Verification.Presented, st.Verification.Confirmed,
					tc.presented, tc.confirmed, tc.wantSentence)
			}
			if st.Verification.At.IsZero() {
				t.Error("the note carries no time, so a later reader cannot tell which of several " +
					"attempts on this machine it describes — which is what makes last-write-wins " +
					"honest rather than lossy")
			}
		})
	}
}

// TestTheNoteSurvivesAnUnreadableRecord — the population is a HOP, and most hops a reader asks
// about have not completed.
//
// A party who has accepted and not yet signed holds no `record.json` at all, so `ReadStored`
// returns at `LoadAbsent`. Reading the note after those branches would hide it for exactly the
// population D5 is about.
func TestTheNoteSurvivesAnUnreadableRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id, err := ceremony.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := ceremony.WriteMe(defaultOutputDir(), id, "ab"); err != nil {
		t.Fatal(err)
	}
	sv := sessionVerifier{cer: &ceremonyID{inv: ceremony.Invitation{ID: id}}}
	sv.noteVerification(true, true)

	st := ceremony.ReadStored(defaultOutputDir(), id, time.Now())
	// SETUP: this really is the pre-record state, which is the branch under test.
	if st.State != ceremony.LoadAbsent {
		t.Fatalf("setup: the ceremony reads as %q, not the pre-hop state this test is about", st.State)
	}
	if st.Verification == nil {
		t.Error("a party who has accepted and not yet signed carries no spoken-check note — and " +
			"they are the population a reader most wants to ask about, because their hop is the " +
			"one that has not happened yet")
	}
}

// TestAManualCoSignRecordsNothing — there is no ceremony for a later reader to ask about, so a
// note would be a file keyed on nothing.
func TestAManualCoSignRecordsNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// No panic and no write: `cer` is nil on the manual path, which is the ordinary case for every
	// two-party co-sign this app has ever done.
	sessionVerifier{}.noteVerification(true, true)
}

// TestEveryExitOfTheSpokenCheckRecordsItsOutcome is the routing half, and it asserts a POPULATION
// rather than comparing three known call sites for agreement.
//
// **Three exits, three notes, and the count is the assertion.** `ConfirmVerification` returns from
// exactly three places — the busy refusal, the user's answer, and the timeout — and each is a
// different one of D5's three states. A new exit added without a note would be a fourth state that
// records as an absence, and absence means UNKNOWN: the machine would then look like one running a
// build too old to write the file, which is the reading `Verification`'s doc requires and the one
// that makes a silent gap invisible.
//
// Comparing the three that exist says nothing about a fourth, which is the shape ADR-009 names.
func TestEveryExitOfTheSpokenCheckRecordsItsOutcome(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	body := funcBodyFrom(code, strings.Index(code, "func (sv sessionVerifier) ConfirmVerification("))
	if body == "" {
		t.Fatal("cannot find ConfirmVerification — this guard is reading the wrong thing")
	}

	returns := strings.Count(body, "return ")
	notes := strings.Count(body, "sv.noteVerification(")
	// A floor, or a scan that matches nothing reports every exit compliant.
	if returns < 3 {
		t.Fatalf("this scan found %d return(s) in ConfirmVerification and the function has three "+
			"— it is reading the wrong body, so its green means nothing", returns)
	}
	if notes != returns {
		t.Errorf("ConfirmVerification has %d exit(s) and %d record(s) the outcome. Every exit is "+
			"one of D5's three states, and one that records nothing is indistinguishable from a "+
			"build too old to write the file — absence means UNKNOWN, so a missing note is not a "+
			"quieter answer, it is a different one", returns, notes)
	}
}

// TestTheAnswerRecordedIsTheAnswERGiven drives `ConfirmVerification` itself, and it exists because
// the direct tests above cannot see the call site.
//
// **A mutation proved they cannot.** Changing `sv.noteVerification(true, ok)` to
// `sv.noteVerification(true, true)` — every check recorded as confirmed, including a refusal —
// left every other test in this file green: they call `noteVerification` with their own arguments,
// and the exit-population scan counts CALLS and not what is passed to them. So a machine could
// record "you confirmed the spoken words" for a user who said the words did not match, and nothing
// would have failed.
func TestTheAnswerRecordedIsTheAnswerGiven(t *testing.T) {
	for _, answer := range []bool{true, false} {
		name := "refused"
		if answer {
			name = "confirmed"
		}
		t.Run(name, func(t *testing.T) {
			ts, srv := startServerWith(t)
			_ = ts
			t.Setenv("HOME", t.TempDir())
			id, err := ceremony.NewID()
			if err != nil {
				t.Fatal(err)
			}
			if err := ceremony.WriteMe(defaultOutputDir(), id, "ab"); err != nil {
				t.Fatal(err)
			}
			sv := sessionVerifier{s: srv, saw: &reached{}, cer: &ceremonyID{inv: ceremony.Invitation{ID: id}}}

			done := make(chan bool, 1)
			go func() {
				ok, _ := sv.ConfirmVerification("one two three four")
				done <- ok
			}()
			// The gate has to be parked before it can be answered; `respondVerify` returns false
			// when nothing is waiting, which is the signal to keep waiting rather than a failure.
			deadline := time.Now().Add(3 * time.Second)
			for !srv.sess.respondVerify(answer) {
				if time.Now().After(deadline) {
					t.Fatal("setup: the spoken check never parked, so no answer could be given")
				}
				time.Sleep(10 * time.Millisecond)
			}
			select {
			case got := <-done:
				if got != answer {
					t.Fatalf("setup: ConfirmVerification returned %v for an answer of %v", got, answer)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("setup: ConfirmVerification never returned")
			}

			st := ceremony.ReadStored(defaultOutputDir(), id, time.Now())
			if st.Verification == nil {
				t.Fatal("answering the spoken check recorded nothing")
			}
			if !st.Verification.Presented {
				t.Error("the note says the words were never presented, and a user just answered them")
			}
			if st.Verification.Confirmed != answer {
				t.Errorf("the user answered %v and the note records confirmed=%v — a machine that "+
					"records a refusal as a confirmation is asserting, durably, that somebody "+
					"vouched for an identity they refused", answer, st.Verification.Confirmed)
			}
		})
	}
}
