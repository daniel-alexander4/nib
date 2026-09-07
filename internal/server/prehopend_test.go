package server

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P05.S03 — the declined end state reaches a party who never signed (D16, /pending 378).
//
// # The defect these drive
//
// A convener's declined round already delivers its signed `Termination` and already walks every
// roster party — a pre-hop one included. What refused it was the RECEIVING side:
// `checkDeliveredPayload` read this machine's mirror record and answered *"this machine cannot
// check that end state against its own record"*, and a party who has accepted and not signed has
// none by definition. So the object was sent to exactly the parties who could not accept it.

// endedCeremonyFor convenes, mints a real declined termination, and hands back what a PRE-HOP
// party holds: the invitation, and nothing else.
func endedCeremonyFor(t *testing.T, me string) (ceremony.Invitation, ceremony.Record, ceremony.Termination) {
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
			{Fingerprint: me, Label: "The other party", Signs: true},
		},
		Intent:         "We agree",
		Expires:        time.Now().Add(24 * time.Hour),
		HopBudget:      ceremonyHopBudget(),
		DeliveryBudget: ceremonyDeliveryLegBudget(),
		ConvenerSigns:  true,
	}, cert, key, time.Now())
	if err != nil {
		t.Fatalf("setup: convene failed, so nothing below ran: %v", err)
	}
	var text string
	for _, in := range out.Invites {
		if strings.EqualFold(in.Party.Fingerprint, me) {
			text = in.Text
		}
	}
	if text == "" {
		t.Fatal("setup: convene issued no invitation for the counterparty")
	}
	inv, err := ceremony.ParseInvitation(text)
	if err != nil {
		t.Fatal(err)
	}
	term, err := ceremony.SignTermination(out.Record, ceremony.StateDeclined, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	return inv, out.Record, term
}

// TestAPartyWhoNeverSignedCanCheckTheEndState is the slice's first acceptance clause.
//
// **Driven at `checkDeliveredPayload`, which is the gate that refused them.** The wire and the
// rendezvous are tier 4's; what tier 1 owns is whether the object is accepted once it arrives, and
// that is where the record dependency lived.
func TestAPartyWhoNeverSignedCanCheckTheEndState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	me := strings.Repeat("ab", 32)
	inv, _, term := endedCeremonyFor(t, me)
	payload, err := json.Marshal(term)
	if err != nil {
		t.Fatal(err)
	}
	cer := &ceremonyID{inv: inv}
	s := &Server{}

	// SETUP: this really is the pre-hop state. Without it the assertion below could be satisfied
	// by a machine that happens to hold a record, which is the case that already worked.
	if st := ceremony.ReadStored(defaultOutputDir(), inv.ID, time.Now()); st.State != ceremony.LoadAbsent {
		t.Fatalf("setup: this machine reads as %q for that ceremony, not absent", st.State)
	}

	if err := s.checkDeliveredPayload(cer, payload); err != nil {
		t.Errorf("a party who accepted and has not signed cannot check the convener's end state: "+
			"%v. That object is delivered to exactly this party by a round that already walks "+
			"them, so refusing it here means the parties who cannot tell a live proceeding from a "+
			"dead one are the ones it was sent to", err)
	}
}

// TestTheEndStateIsStillRefusedOnItsOwnTerms — the second acceptance clause. The new path must not
// be a weaker path.
func TestTheEndStateIsStillRefusedOnItsOwnTerms(t *testing.T) {
	t.Run("a termination for another proceeding", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		me := strings.Repeat("ab", 32)
		inv, _, _ := endedCeremonyFor(t, me)
		// A DIFFERENT ceremony's real, correctly signed termination, replayed into this one.
		_, _, other := endedCeremonyFor(t, me)
		payload, err := json.Marshal(other)
		if err != nil {
			t.Fatal(err)
		}
		if err := (&Server{}).checkDeliveredPayload(&ceremonyID{inv: inv}, payload); err == nil {
			t.Error("a signed end state minted for a DIFFERENT proceeding was accepted. The " +
				"roster commitment is the whole binding and it commits to the ceremony id, so " +
				"this is the comparison that refuses a cross-ceremony replay")
		}
	})

	t.Run("a termination signed by somebody who is not the convener", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		me := strings.Repeat("ab", 32)
		inv, rec, _ := endedCeremonyFor(t, me)
		// **THIS ceremony's own record, signed by a key that is not the convener's.** The same
		// record means the same roster commitment, so the binding comparison passes and the
		// SIGNER check is the only thing left that can refuse it — which is what makes this case
		// about that check rather than about the commitment.
		cert, key, err := sign.GenerateIdentity("Somebody else")
		if err != nil {
			t.Fatal(err)
		}
		forged, ferr := ceremony.SignTermination(rec, ceremony.StateDeclined, cert, key)
		if ferr != nil {
			t.Fatal(ferr)
		}
		payload, err := json.Marshal(forged)
		if err != nil {
			t.Fatal(err)
		}
		if err := (&Server{}).checkDeliveredPayload(&ceremonyID{inv: inv}, payload); err == nil {
			t.Error("an end state signed by a party who is not this ceremony's convener was " +
				"accepted. Without that check any roster member could end a proceeding they are " +
				"only a party to")
		}
	})
}

// TestTheDeliverySweepStillAdmitsOnlySignedCeremonies records a BACKED-OUT change, and it is a
// test rather than a comment because a comment cannot notice the change coming back.
//
// P05.S03 widened `rearmDeliveries` to admit `LoadAbsent` — a party who has accepted and not
// signed — so the convener's end-state round could reach them. **It caused a deterministic tier-4d
// failure at four parties** (*"a party is not reported delivered after the recovery run"*, twice,
// passing with the admission reverted) and three hypotheses failed to explain it. The measured
// evidence and the dead ends are in `/pending 380`.
//
// **This guard is not an argument that the admission is wrong** — it is almost certainly right, and
// the receiving half it exists for already shipped. It is here so the next attempt is a deliberate
// one that re-runs `pairrepro.sh -n 4`, rather than a re-introduction by somebody who reads
// `checkDeliveredPayload`'s pre-hop branch and reasonably concludes the sweep must already feed it.
func TestTheDeliverySweepStillAdmitsOnlySignedCeremonies(t *testing.T) {
	src, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	body := funcBodyFrom(code, strings.Index(code, "func (s *Server) rearmDeliveries("))
	if body == "" {
		t.Fatal("cannot find rearmDeliveries — this guard is reading the wrong thing")
	}
	if strings.Contains(body, "ceremony.LoadAbsent") {
		t.Error("rearmDeliveries admits the pre-hop class again. That is the change /pending 380 " +
			"records as backed out after a deterministic tier-4d failure — re-run " +
			"`./build/pairrepro.sh -n 4` and, if it is green, delete this guard with the evidence " +
			"rather than editing around it")
	}
}
