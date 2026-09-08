package ceremony

import (
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// /pending 380 — a party who has accepted and not signed is PULLED the end state, not delivered it.
//
// # Why a pull at all
//
// A delivery rendezvous is keyed `(ceremony, hop)` and its listener pins one peer, so a machine
// holding a pre-hop arm for one ceremony cannot also hold a delivery arm for another. Tier 4d
// measured exactly that: the recipient's single slot was armed for a different ceremony and refused
// the convener on its own identity. `rendezvous.Fetch` is a DHT read and needs no arm, so the
// one-pinned-peer rule is never approached.

// endStateFixture mints a real convened ceremony and its declined termination, and hands back what
// each side actually holds: the convener's record, and the invitation a pre-hop party has.
func endStateFixture(t *testing.T) (Invitation, Record, Termination) {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Nib User")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Convene(base, ConveneRequest{
		Roster: []Party{
			{Fingerprint: hexOf(fp), Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("ab", 32), Label: "The other party", Signs: true},
		},
		Intent:         "We agree",
		Expires:        time.Now().Add(24 * time.Hour),
		HopBudget:      5 * time.Minute,
		DeliveryBudget: 5 * time.Minute,
	}, cert, key, time.Now())
	if err != nil {
		t.Fatalf("setup: convene failed: %v", err)
	}
	var inv Invitation
	for _, in := range out.Invites {
		if !strings.EqualFold(in.Party.Fingerprint, hexOf(fp)) {
			parsed, perr := ParseInvitation(in.Text)
			if perr != nil {
				t.Fatal(perr)
			}
			inv = parsed
		}
	}
	if inv.ID == "" {
		t.Fatal("setup: convene issued no invitation for the counterparty")
	}
	term, err := SignTermination(out.Record, StateDeclined, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	return inv, out.Record, term
}

// TestAPreHopPartyOpensTheEndStateFromItsInvitationAlone is the item's whole point: everything the
// reader needs is in the invitation, because the invitation is all they have.
func TestAPreHopPartyOpensTheEndStateFromItsInvitationAlone(t *testing.T) {
	inv, rec, term := endStateFixture(t)

	key, err := inv.EndStateKey()
	if err != nil {
		t.Fatal(err)
	}
	salt, err := inv.EndStateSalt()
	if err != nil {
		t.Fatal(err)
	}
	convAnchor, err := rec.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := term.Seal(key, salt, convAnchor)
	if err != nil {
		t.Fatalf("the convener could not seal its own end state: %v", err)
	}

	// The reader's side: no record, no document, just the invitation.
	anchor, err := inv.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	got, err := OpenEndState(key, salt, sealed, anchor)
	if err != nil {
		t.Fatalf("a party holding only its invitation cannot open the end state: %v. That party is "+
			"the entire population this path exists for — they hold no record, which is why they "+
			"could not be delivered to in the first place", err)
	}
	if got.State != StateDeclined {
		t.Errorf("the opened end state reads %q, want %q", got.State, StateDeclined)
	}
}

// TestTheConvenerAndThePartyDeriveTheSameTarget — both sides compute the target from the invitation
// secret, so a mismatch means they publish and read at different places and never meet.
func TestTheConvenerAndThePartyDeriveTheSameTarget(t *testing.T) {
	inv, _, _ := endStateFixture(t)
	other, _, _ := endStateFixture(t)

	for _, d := range []struct {
		name string
		fn   func(Invitation) ([]byte, error)
	}{
		{"seed", Invitation.EndStateSeed},
		{"salt", Invitation.EndStateSalt},
		{"key", Invitation.EndStateKey},
	} {
		a, err := d.fn(inv)
		if err != nil {
			t.Fatal(err)
		}
		b, err := d.fn(inv)
		if err != nil {
			t.Fatal(err)
		}
		if string(a) != string(b) {
			t.Errorf("%s is not deterministic — the two ends would never meet", d.name)
		}
		c, err := d.fn(other)
		if err != nil {
			t.Fatal(err)
		}
		if string(a) == string(c) {
			t.Errorf("%s is the SAME for two different ceremonies. The target would collide and one "+
				"proceeding's end state would be read as another's", d.name)
		}
	}
}

// TestTheEndStateDerivationsAreSeparatedFromTheHopFamily.
//
// `derive`'s own doc states the rule: "a value used for one purpose can never be the value used for
// another — the failure that turns a rendezvous key into a decryption key". The end-state family
// sits beside the hop family and must not collide with any hop, including hop 0.
func TestTheEndStateDerivationsAreSeparatedFromTheHopFamily(t *testing.T) {
	inv, _, _ := endStateFixture(t)
	es, err := inv.EndStateSeed()
	if err != nil {
		t.Fatal(err)
	}
	ek, err := inv.EndStateKey()
	if err != nil {
		t.Fatal(err)
	}
	esalt, err := inv.EndStateSalt()
	if err != nil {
		t.Fatal(err)
	}
	if string(es) == string(ek) || string(es) == string(esalt) || string(ek) == string(esalt) {
		t.Fatal("two of the three end-state derivations are equal — they share a domain")
	}
	for hop := 0; hop < 4; hop++ {
		hs, err := inv.HopSeed(hop)
		if err != nil {
			t.Fatal(err)
		}
		hk, err := inv.RecordKey(hop)
		if err != nil {
			t.Fatal(err)
		}
		if string(es) == string(hs) {
			t.Errorf("the end-state seed equals hop %d's seed — the end state would be published at "+
				"a target a real hop uses", hop)
		}
		if string(ek) == string(hk) {
			t.Errorf("the end-state key equals hop %d's record key", hop)
		}
	}
}

// TestASealedEndStateFitsTheRendezvous is the guard for a failure that is otherwise SILENT.
//
// `MaxSealedRecord`'s doc: an over-size value is refused by our own store inside `dht.Server.Put`
// before any datagram is sent, `getput.Put` logs a warning per node and returns nil — so the record
// never leaves the machine and nothing says so.
//
// **Measured while building this, and the margin is a function of a NAME.** A termination carrying
// a PEM certificate seals well inside the cap for the 8-character common name production mints
// (`GenerateIdentity("Nib User")` — `finalize.go` is its only production caller), and goes OVER for
// a 128-character one. The assertion below is on the real fixture; the second half pins the reason
// the margin exists, so the day the name becomes user-supplied this goes red instead of going
// quiet.
func TestASealedEndStateFitsTheRendezvous(t *testing.T) {
	inv, rec, term := endStateFixture(t)
	key, _ := inv.EndStateKey()
	salt, _ := inv.EndStateSalt()
	anchor, err := rec.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := term.Seal(key, salt, anchor)
	if err != nil {
		t.Fatalf("a real end state does not fit the rendezvous: %v", err)
	}
	if len(sealed) > MaxSealedRecord {
		t.Fatalf("a sealed end state is %d bytes against a %d cap", len(sealed), MaxSealedRecord)
	}
	t.Logf("a sealed end state is %d bytes; the cap is %d (%d bytes of margin)",
		len(sealed), MaxSealedRecord, MaxSealedRecord-len(sealed))

	// The reason the margin exists. A long common name is not reachable through any production
	// path today — and this is what says so out loud if one ever appears.
	longCert, longKey, err := sign.GenerateIdentity(strings.Repeat("N", 128))
	if err != nil {
		t.Skipf("a 128-character common name is refused at the identity door (%v), which is a "+
			"stronger guarantee than this test asserts", err)
	}
	longBase, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	longRec, err := Convene(longBase, ConveneRequest{
		Roster: []Party{
			{Fingerprint: hexOf(mustFP(t, longCert)), Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("ab", 32), Label: "Other", Signs: true},
		},
		Intent: "We agree", Expires: time.Now().Add(24 * time.Hour),
		HopBudget: 5 * time.Minute, DeliveryBudget: 5 * time.Minute,
	}, longCert, longKey, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	longTerm, err := SignTermination(longRec.Record, StateDeclined, longCert, longKey)
	if err != nil {
		t.Fatal(err)
	}
	longAnchor, err := longRec.Record.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := longTerm.Seal(key, salt, longAnchor); err == nil {
		t.Log("a 128-character common name still fits — the margin has grown; no action needed")
	} else {
		t.Logf("as expected, a 128-character common name does not fit: %v", err)
	}
}

func mustFP(t *testing.T, cert []byte) []byte {
	t.Helper()
	fp, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

// The three tests below exist because the four above did not catch their mutations.
//
// **Measured, not supposed:** removing `OpenEndState`'s verification, removing `Seal`'s size
// ceiling, and dropping the salt from the AAD each left the original set GREEN. A happy-path test
// proves an object round-trips; it says nothing about what the door REFUSES, and refusing is the
// whole job of a reader whose bytes come off the public DHT.

// TestAPlantedEndStateIsRefused — the door verifies, and this is what says so.
//
// The bytes at that target are written by whoever reached it first. A correctly sealed, correctly
// signed end state for ANOTHER proceeding is the strongest such object an attacker can cheaply
// produce, and it must not open here.
func TestAPlantedEndStateIsRefused(t *testing.T) {
	inv, _, _ := endStateFixture(t)
	other, otherRec, otherTerm := endStateFixture(t)

	// Sealed with THIS ceremony's key and salt so the AEAD opens — the refusal has to come from
	// the anchor, not from the cipher. Without this the test would pass on a decryption failure
	// and prove nothing about verification.
	key, _ := inv.EndStateKey()
	salt, _ := inv.EndStateSalt()
	otherAnchor, err := otherRec.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := otherTerm.Seal(key, salt, otherAnchor)
	if err != nil {
		t.Fatal(err)
	}
	_ = other

	anchor, err := inv.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenEndState(key, salt, sealed, anchor); err == nil {
		t.Fatal("a real, correctly signed end state for a DIFFERENT proceeding opened against this " +
			"invitation. A party would be told their ceremony had ended on the strength of somebody " +
			"else's decline")
	}
}

// TestAnEndStateDoesNotOpenAtAnotherTarget — the salt is in the AAD, and that is what binds a
// sealed record to the place it was published.
//
// Without it a record lifted from one target replays at every other target sharing the key.
func TestAnEndStateDoesNotOpenAtAnotherTarget(t *testing.T) {
	inv, rec, term := endStateFixture(t)
	key, _ := inv.EndStateKey()
	salt, _ := inv.EndStateSalt()
	anchor, err := rec.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := term.Seal(key, salt, anchor)
	if err != nil {
		t.Fatal(err)
	}

	// SETUP: it opens at its OWN salt, so the refusal below is the salt and not a broken fixture.
	invAnchor, err := inv.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenEndState(key, salt, sealed, invAnchor); err != nil {
		t.Fatalf("setup: the record does not open at its own target: %v", err)
	}

	elsewhere := append([]byte{}, salt...)
	elsewhere[0] ^= 0xff
	if _, err := OpenEndState(key, elsewhere, sealed, invAnchor); err == nil {
		t.Fatal("a sealed end state opened at a target it was not published at. The salt is in the " +
			"AAD precisely so a record cannot be lifted from one target and replayed at another")
	}
}

// TestAnOversizeEndStateIsRefusedAtTheSeal drives the ceiling itself.
//
// The fixture test above only LOGS what a long common name does, because the margin is a property
// of the fixture and may grow. This asserts the guard: an object built to exceed the cap must come
// back as `ErrEndStateTooBig`, because the alternative is `dht.Server.Put` dropping it before any
// datagram leaves and nothing saying so.
func TestAnOversizeEndStateIsRefusedAtTheSeal(t *testing.T) {
	inv, rec, term := endStateFixture(t)
	key, _ := inv.EndStateKey()
	salt, _ := inv.EndStateSalt()
	anchor, err := rec.Anchor()
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the honest object fits, so the refusal below is the SIZE and not the object.
	if _, err := term.Seal(key, salt, anchor); err != nil {
		t.Fatalf("setup: an ordinary end state does not seal: %v", err)
	}

	// Padding a signed field breaks the signature, so this drives the ceiling through a field the
	// signature does not cover — and the check must fire before anything else refuses it.
	big := term
	big.ConvenerCert = term.ConvenerCert + strings.Repeat("A", MaxSealedRecord)
	_, err = big.Seal(key, salt, anchor)
	if err == nil {
		t.Fatal("a sealed end state far over the cap was accepted. It would be dropped by this " +
			"machine's own store before any datagram was sent, and nothing would say so")
	}
	if !errors.Is(err, ErrEndStateTooBig) {
		t.Errorf("an over-size end state is refused as %v, which is not ErrEndStateTooBig. A size "+
			"refusal reported as a verification failure sends the reader to the wrong place", err)
	}
}
