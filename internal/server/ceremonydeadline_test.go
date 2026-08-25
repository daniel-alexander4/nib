package server

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// docWithRecord attaches a ceremony record whose deadline is `in` from now. When `tamper` is
// true the record is corrupted AFTER signing, so it is structurally a record and does not
// verify — which is the only way to test that its deadline is not believed.
func docWithRecord(t *testing.T, in time.Duration, tamper bool) []byte {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("convener")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := sign.GenerateIdentity("alice")
	if err != nil {
		t.Fatal(err)
	}
	ofp, err := sign.Fingerprint(other)
	if err != nil {
		t.Fatal(err)
	}
	id, err := ceremony.NewID()
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := testpdf.Text("a page")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := ceremony.DocumentHash(pdf)
	if err != nil {
		t.Fatal(err)
	}
	rec := ceremony.Record{
		ID: id, DocHash: hash, Intent: "co-sign the lease",
		Expires: time.Now().Add(in),
		Roster: []ceremony.Party{
			{Fingerprint: hex.EncodeToString(fp), Label: "Convener", Signs: true},
			{Fingerprint: hex.EncodeToString(ofp), Label: "Alice", Signs: true},
		},
	}
	if err := rec.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if tamper {
		// A roster label is inside the commitment preimage, so changing it after signing
		// leaves a record that parses and does not verify — exactly the shape a stranger
		// hands you.
		rec.Roster[1].Label = "Mallory"
	}
	out, err := ceremony.Embed(pdf, rec)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAHopDoesNotStartAfterTheCeremonyCanOutliveIt — D16's Stage 6 pin, and the line where
// clock 3 stops being a field nobody reads.
//
// `Record.Expires` is labelled "the ceremony deadline (D16's clock 3)" and was read at exactly
// ONE line in the whole repo — the roster preimage — and never compared to a clock. So the
// deadline the convener sets, and which governs how long a listener may arm and how long
// invitation-scoped pins persist, bound nothing at all.
//
// **The rule is a NESTING rule, not a comparison.** A hop admitted one second before the
// deadline still gets `exchangeDeadline`'s six minutes, so the ceremony outlives its own expiry
// by up to that much — and the party at that hop is asked to consent to a signature on a
// proceeding that has already ended. The outer clock has to reserve the inner one's worst case.
func TestAHopDoesNotStartAfterTheCeremonyCanOutliveIt(t *testing.T) {
	now := time.Now()

	// **The expectation is a LITERAL, not the call under test (fixed 2026-08-24, P07.S02a).**
	//
	// This read `budget := p2p.ExchangeBudget()` and then asserted against a check that
	// computed its threshold from the same call — so the two agreed by construction and the
	// test could not fail for the reservation being WRONG. It was: 6 minutes reserved against
	// a hop that can spend 29m20s, which is the defect this test names in its own doc comment
	// and could not see.
	//
	// A literal means a deliberate constant change edits this line and re-reads the sum in
	// ceremonyHopBudget; an accidental one goes red.
	const budget = 29*time.Minute + 20*time.Second
	if got := ceremonyHopBudget(); got != budget {
		t.Fatalf("ceremonyHopBudget() is %s and this test was written for %s. If the clocks "+
			"moved on purpose, move this literal too and re-read C20 in the plan, which quotes "+
			"a per-hop figure; if not, this is the bug.", got, budget)
	}

	// SETUP: a deadline comfortably past one hop is ACCEPTED. Without this the refusal below
	// is equally true of a check that refuses every ceremony, which would break the feature
	// outright while looking like a safety property.
	roomy := docWithRecord(t, budget+time.Hour, false)
	if err := checkCeremonyDeadline(roomy, now); err != nil {
		t.Fatalf("setup: a ceremony ending %s from now was refused (%v), so the refusal "+
			"below cannot distinguish the nesting rule from a blanket refusal", budget+time.Hour, err)
	}

	// **Inside the budget — the case the pin is about.** Not expired: there is still time on
	// the clock, which is exactly why a naive `Expires.After(now)` would let it through.
	tight := docWithRecord(t, budget/2, false)
	err := checkCeremonyDeadline(tight, now)
	if err == nil {
		t.Errorf("a hop was allowed to start with %s left on a ceremony that gives every hop "+
			"%s. It is not expired — which is why comparing against `now` alone passes it — "+
			"but it cannot finish, and the far party is asked to consent to a signature on a "+
			"proceeding that ends first.", budget/2, budget)
	} else if !strings.Contains(err.Error(), "one hop") {
		t.Errorf("refused with %q, which does not say why — a user told only that their "+
			"ceremony is over cannot tell it from one that never started", err)
	}

	// **The regression this whole change is about**, asserted directly: a deadline that clears
	// one PHASE budget but not one HOP must be refused. Under the old reservation this was the
	// passing case, and the far party's consent landed after the ceremony had ended.
	betweenPhaseAndHop := docWithRecord(t, p2p.ExchangeBudget()+time.Minute, false)
	if err := checkCeremonyDeadline(betweenPhaseAndHop, now); err == nil {
		t.Errorf("a hop was allowed to start with %s left — more than one exchange budget (%s) "+
			"but far less than one hop (%s). Reserving the phase budget for a whole session is "+
			"exactly what exchangeDeadline's own doc forbids.",
			p2p.ExchangeBudget()+time.Minute, p2p.ExchangeBudget(), budget)
	}

	// A document with NO record is the ordinary two-party co-sign. It has no deadline to
	// honour, and refusing it would break every co-sign in the product.
	plain, perr := testpdf.Text("a page")
	if perr != nil {
		t.Fatal(perr)
	}
	if _, xerr := ceremony.Extract(plain); !errors.Is(xerr, ceremony.ErrNoRecord) {
		t.Fatalf("setup: the plain fixture unexpectedly carries a record (%v)", xerr)
	}
	if err := checkCeremonyDeadline(plain, now); err != nil {
		t.Errorf("a document with no ceremony record was refused (%v) — that is every "+
			"ordinary co-sign in the product", err)
	}

	// **And the record must VERIFY before its deadline is believed.** An unverified `Expires`
	// is a number a stranger chose, so a forged record could otherwise buy itself unlimited
	// time by claiming a deadline in the next century.
	//
	// The first version of this block built the forgery with a second `AddAttachment` under
	// the same name — which the API REFUSES — so `berr != nil`, the whole assertion sat inside
	// an `if berr == nil` that never ran, and it was dead code. Found by its red proof coming
	// back green. The tamper now happens before embedding, where it works.
	forged := docWithRecord(t, budget+time.Hour, true)
	// SETUP: the tampering really produced a record that parses and does not verify. Without
	// this, a fixture that silently failed to corrupt anything passes the assertion below by
	// having nothing wrong with it.
	fr, ferr := ceremony.Extract(forged)
	if ferr != nil {
		t.Fatalf("setup: the tampered document carries no readable record (%v)", ferr)
	}
	if fr.Verify(now) == nil {
		t.Fatal("setup: the tampered record still verifies, so this asserts nothing about " +
			"believing an unverified deadline")
	}
	if err := checkCeremonyDeadline(forged, now); err == nil {
		t.Error("a ceremony record that does not verify was believed about its own " +
			"deadline; an unverified Expires is a number a stranger chose")
	}
}
