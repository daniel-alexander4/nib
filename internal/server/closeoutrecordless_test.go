package server

import (
	"encoding/hex"
	"os"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
)

// TestARecordlessCeremonyIsClosedOutOnlyOnAnAttestedEnd — `/pending 623` part 5.
//
// A party that accepted an invitation and never signed holds no record, and `closeOutReason` opens
// by refusing anything that is not `LoadOK` — so a proceeding the convener declined or stopped
// before the baton reached them left its ceremony secret, its stored invitation and its
// ceremony-scoped pin in the vault for good.
//
// **Three arms from one fixture, and the first two are what keep the third from being a way to
// lose a live invitation.** Silence is not an end — not even a year of it, because an invitation
// carries no deadline to conclude "abandoned" from. A termination that does not verify against the
// invitation is not an end either: anyone able to drop a file into `~/nib/ceremonies/<id>/` could
// otherwise take a waiting party's invitation away. Only the convener's own signed end state is.
func TestARecordlessCeremonyIsClosedOutOnlyOnAnAttestedEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, v := unlockedServer(t)
	srv.mu.Lock()
	srv.SetInstanceToken("this-one")
	srv.mu.Unlock()
	root := defaultOutputDir()

	myCert, _, err := identity(v)
	if err != nil {
		t.Fatal(err)
	}
	myFP, err := sign.Fingerprint(myCert)
	if err != nil {
		t.Fatal(err)
	}
	me := hex.EncodeToString(myFP)

	invitation, rec, cert, key, convenerFP := conveneRealCeremonyFor(t, me)
	inv, err := ceremony.ParseInvitation(invitation)
	if err != nil {
		t.Fatal(err)
	}
	// What accepting leaves on this machine: the invitation, the convener's pin scoped to the
	// ceremony, a secret, and a folder with no record in it.
	convFP, err := hex.DecodeString(convenerFP)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyInvitation(inv.ID, invitation); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyPeer(convFP, "Convener", inv.ID); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonySecret(inv.ID, convFP, []byte("the-secret")); err != nil {
		t.Fatal(err)
	}
	if err := ceremony.WriteMe(root, inv.ID, me); err != nil {
		t.Fatal(err)
	}
	live, err := ceremony.MirrorDir(root, inv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st := ceremony.ReadStored(root, inv.ID, time.Now()); st.State != ceremony.LoadAbsent {
		t.Fatalf("setup: the ceremony reads as %q, want a folder with no record (%q) — this test "+
			"is about the party closeOutReason cannot speak for", st.State, ceremony.LoadAbsent)
	}
	held := func() (problems []string) {
		if _, ok := v.CeremonyInvitationFor(inv.ID); !ok {
			problems = append(problems, "the stored invitation is gone")
		}
		if n := countScopedPins(v, inv.ID); n != 1 {
			problems = append(problems, "the ceremony-scoped pin is gone")
		}
		if n := len(v.CeremonySecrets(inv.ID)); n != 1 {
			problems = append(problems, "the ceremony secret is gone")
		}
		if _, serr := os.Stat(live); serr != nil {
			problems = append(problems, "the live folder is gone")
		}
		return problems
	}
	if p := held(); len(p) > 0 {
		t.Fatalf("setup: %v", p)
	}

	// ── SILENCE. No termination, and a clock a year on: nothing here says the proceeding ended.
	srv.closeOutEnded(v, time.Now().Add(365*24*time.Hour))
	if p := held(); len(p) > 0 {
		t.Fatalf("a record-less ceremony with NO end state was closed out (%v) — a party still "+
			"waiting for their turn lost their invitation to an inference", p)
	}

	// ── A termination that does not verify: signed for another proceeding, filed under this one.
	_, otherRec, otherCert, otherKey, _ := conveneRealCeremonyFor(t, me)
	foreign, err := ceremony.SignTermination(otherRec, ceremony.StateDeclined, otherCert, otherKey)
	if err != nil {
		t.Fatal(err)
	}
	foreign.Ceremony = inv.ID
	if err := ceremony.WriteTermination(root, foreign); err != nil {
		t.Fatal(err)
	}
	if _, verr := ceremony.ReadTerminationFor(root, inv); verr == nil {
		t.Fatal("setup: a termination signed for another proceeding verified against this invitation")
	}
	srv.closeOutEnded(v, time.Now())
	if p := held(); len(p) > 0 {
		t.Fatalf("a termination that does NOT verify closed a record-less ceremony out (%v) — "+
			"anyone able to drop a file into ~/nib/ceremonies/<id>/ could take a waiting party's "+
			"invitation and pins away", p)
	}

	// ── The convener's own signed decline, for THIS proceeding.
	term, err := ceremony.SignTermination(rec, ceremony.StateDeclined, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(terminationPathForTest(t, inv.ID)); err != nil {
		t.Fatal(err)
	}
	if err := ceremony.WriteTermination(root, term); err != nil {
		t.Fatal(err)
	}
	if _, verr := ceremony.ReadTerminationFor(root, inv); verr != nil {
		t.Fatalf("setup: the convener's signed decline does not verify against the invitation: %v", verr)
	}
	srv.closeOutEnded(v, time.Now())
	if _, ok := v.CeremonyInvitationFor(inv.ID); ok {
		t.Error("the stored invitation survived an attested end — it carries the ceremony secret")
	}
	if n := countScopedPins(v, inv.ID); n != 0 {
		t.Errorf("%d ceremony-scoped pin(s) survived an attested end, want 0", n)
	}
	if n := len(v.CeremonySecrets(inv.ID)); n != 0 {
		t.Errorf("%d ceremony secret(s) survived an attested end, want 0", n)
	}
	if _, serr := os.Stat(live); !os.IsNotExist(serr) {
		t.Errorf("the ended ceremony's folder is still in the live set (%v)", serr)
	}
	// Moved, not deleted (ADR-012): the signed end state is still readable, from ended/.
	if got, rerr := ceremony.ReadTerminationFor(root, inv); rerr != nil || got.State != ceremony.StateDeclined {
		t.Errorf("after the close-out the signed end state reads as %q (%v), want %q from the moved folder",
			got.State, rerr, ceremony.StateDeclined)
	}
	receipt, rerr := ceremony.ReadReceipt(root, inv.ID)
	if rerr != nil || receipt.State != ceremony.StateDeclined {
		t.Errorf("the local receipt reads %q (%v), want %q — the usual receipt, naming the attested state",
			receipt.State, rerr, ceremony.StateDeclined)
	}
}
