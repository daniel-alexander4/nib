package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// /pending 377 — the rail told an invitee who had just accepted that their folder may be gone.
//
// # The defect these drive
//
// A successful accept ends at `ceremony.WriteMe`: the directory and the `me` marker exist and
// `record.json` does not, because an invitee holds no record until the document reaches their hop.
// `ReadStored` therefore lands on `LoadAbsent` and `handleCeremonyNext` answered `unavailable` with
// the sentence written for a directory the user had DELETED — for the single commonest invitee
// state there is. The advice sent them looking for something that had never been there.
//
// # Why the state and the sentence are two assertions and not one
//
// `unavailable` means *"the document or record could not be read well enough to say"*, by this
// route's own field doc. Here Nib read everything there is and the answer is definite, so the state
// is a wrong CLAIM independently of how the sentence reads — fixing only the wording would leave a
// surface that cannot tell a party waiting for the baton from a ceremony it failed to open.
//
// # Driven through the real accept route
//
// Not by calling `WriteMe`. The fact under test is what an accept LEAVES on disk, and a fixture
// that wrote the marker itself would assert the shape this test was written from rather than the
// shape the product produces — the argument `ceremonyme_test.go` already makes for its own driver.

// TestAPartyWhoJustAcceptedIsNotToldTheirFolderMayBeGone is the item's own case.
func TestAPartyWhoJustAcceptedIsNotToldTheirFolderMayBeGone(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, _ := inviteFor(t, me)

	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation})
	if code != http.StatusOK {
		t.Fatalf("setup: the accept failed (%d %s), so this machine is not in the state the "+
			"item is about", code, body)
	}
	var acc acceptResponse
	if err := json.Unmarshal([]byte(body), &acc); err != nil {
		t.Fatal(err)
	}

	// SETUP, asserted rather than assumed: this really is a marker with no record. Without it
	// every assertion below could be satisfied by a machine that holds a record, which is the
	// case that already answered correctly.
	st := ceremony.ReadStored(defaultOutputDir(), acc.Ceremony, time.Now())
	if st.State != ceremony.LoadAbsent {
		t.Fatalf("setup: an accepted ceremony reads as %q, not absent — the accept wrote a "+
			"record, so the state this item is about does not exist", st.State)
	}

	got := askNext(t, ts, acc.Ceremony)
	if got.State != "accepted" {
		t.Errorf("a party who has just accepted is reported as %q, want \"accepted\". "+
			"%q means Nib could not read enough to say, and here it read everything there is: "+
			"the directory holds this machine's own marker and no record, which is what waiting "+
			"for the baton looks like", got.State, got.State)
	}
	if strings.Contains(got.Reason, "may have been removed") {
		t.Errorf("a party who has just accepted is told %q — that sentence was written for a "+
			"folder the user deleted, and it sends them looking for something that was never "+
			"there", got.Reason)
	}
	if !strings.Contains(got.Reason, "nothing has arrived") {
		t.Errorf("the sentence for an accepted-and-waiting party does not say that nothing has "+
			"arrived yet: %q", got.Reason)
	}
}

// TestAGenuinelyEmptyCeremonyFolderStillSaysSo is the other arm, and it is what keeps the fix from
// being a blanket rewording.
//
// A directory with neither a record NOR a marker is the case the old sentence was written for — a
// folder that was removed, or an accept interrupted before anything was written — and it must keep
// both the sentence and the `unavailable` state. A discriminator that answered "accepted" for every
// absent directory would satisfy the test above and destroy the distinction it exists to draw.
func TestAGenuinelyEmptyCeremonyFolderStillSaysSo(t *testing.T) {
	ts, _ := startServer(t)
	id, err := ceremony.NewID()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ceremony.MirrorDir(defaultOutputDir(), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	got := askNext(t, ts, id)
	if got.State != "unavailable" {
		t.Errorf("an empty ceremony folder is reported as %q, want \"unavailable\" — nothing on "+
			"this machine says the user is a party to it, so Nib genuinely cannot say", got.State)
	}
	if !strings.Contains(got.Reason, "may have been removed") {
		t.Errorf("an empty ceremony folder no longer gets the sentence written for it: %q", got.Reason)
	}
}

// TestThePreHopPartyStillClassifiesAsAbsent pins the invariant the fix above rests on.
//
// **The item offered a fifth `LoadState` as the alternative shape, and the code refuses it.** Two
// production sites test `st.State == ceremony.LoadAbsent` to mean *exactly* this party —
// `deliveryWindowFor` (`session.go`), which gives their delivery arm the same bound as their hop
// arm rather than the five-minute floor, and `checkDeliveredPayload` (`delivery.go`), which
// verifies a convener's end state against the INVITATION because there is no record to anchor it
// to. A new state carved out of `LoadAbsent` for the accepted case would make both stop matching
// in the one case each was written for, and neither would fail loudly: the first would silently
// shorten the window and the second would fall through to "cannot check against its own record".
//
// So the discriminator is a FIELD (`Stored.Me`) and the classification is unchanged — the argument
// `Stored.Ended`'s own doc already makes for itself. This test is what makes that checkable rather
// than merely written down, and it is the only coverage `deliveryWindowFor`'s absent arm has.
func TestThePreHopPartyStillClassifiesAsAbsent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	me := strings.Repeat("ab", 32)
	inv, _, _ := endedCeremonyFor(t, me)

	// What an accept leaves: the marker, and no record.
	if err := ceremony.WriteMe(defaultOutputDir(), inv.ID, me); err != nil {
		t.Fatal(err)
	}

	st := ceremony.ReadStored(defaultOutputDir(), inv.ID, time.Now())
	if st.State != ceremony.LoadAbsent {
		t.Fatalf("a party who has accepted and not signed classifies as %q, not %q. "+
			"deliveryWindowFor and checkDeliveredPayload both key on absent to mean exactly this "+
			"party — the first would silently drop their delivery arm to the five-minute floor "+
			"and the second would refuse the convener's end state for want of a record",
			st.State, ceremony.LoadAbsent)
	}
	if !st.Joined {
		t.Errorf("a machine holding its own marker for this ceremony does not read as joined — " +
			"the accepted-and-waiting sentence and the route's \"accepted\" state both come from " +
			"that field, so a false one puts both back on the folder-may-have-been-removed path")
	}
	// **And the position stays empty, which is a different rule and not this one.**
	// `TestNoDegradedClassReportsAPosition` holds `Me` empty on every degraded class because a
	// position points into a roster this read did not verify. The first cut of /pending 377 used
	// `Me` as the discriminator and broke that guard; this line is where the two facts are held
	// apart at the one fixture that has both.
	if st.Me != "" {
		t.Errorf("a pre-hop party reports a position (%q) against a ceremony with no record. "+
			"Joined is the marker's presence and Me is the roster position it names — the second "+
			"means nothing without a roster that checked out", st.Me)
	}

	cer := &ceremonyID{inv: inv}
	if got, want := deliveryWindowFor(cer), hopWindowFor(cer); got != want {
		t.Errorf("a pre-hop party's delivery arm gets %v and their hop arm gets %v. For them a "+
			"missing record is the ORDINARY state, and the floor would close the arm before any "+
			"convener could reach it", got, want)
	}
}
