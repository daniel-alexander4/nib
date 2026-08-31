package ceremony

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// hopBudget is the figure internal/server's ceremonyHopBudget() produces. A LITERAL here,
// because this package cannot see two of its four terms and a test that reached for a
// derived value would be asserting against something it cannot compute.
const hopBudget = 29*time.Minute + 20*time.Second

func conveneReq(t *testing.T, parties ...string) ConveneRequest {
	t.Helper()
	r := ConveneRequest{
		Intent:         "We agree to co-sign the lease",
		Expires:        time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		HopBudget:      hopBudget,
		DeliveryBudget: hopBudget,
		ConvenerSigns:  true,
	}
	for i, fp := range parties {
		r.Roster = append(r.Roster, Party{Fingerprint: fp, Label: string(rune('A' + i)), Signs: true})
	}
	return r
}

func TestConveneProducesARecordADocumentAndOneInvitationPerParty(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	_, _, bfp := identity(t, "B")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	got, err := Convene(base, conveneReq(t, cfp, afp, bfp), cert, key, now)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Invites); n != 2 {
		t.Errorf("a three-party ceremony produced %d invitations, want 2 (the convener holds "+
			"every party's and receives none)", n)
	}
	// Roster ORDER, not map order — roster order is hop order.
	for i, inv := range got.Invites {
		if inv.Party.Fingerprint != got.Record.Roster[i+1].Fingerprint {
			t.Errorf("invite %d is for %s; roster position %d is %s — invitations must come back "+
				"in roster order, which is hop order", i, short(inv.Party.Fingerprint), i+1,
				short(got.Record.Roster[i+1].Fingerprint))
		}
		if inv.Text == "" {
			t.Errorf("invite %d has no encoded text, so the caller must re-encode it and can "+
				"discover a failure after the record is already embedded", i)
		}
		if _, perr := ParseInvitation(inv.Text); perr != nil {
			t.Errorf("invite %d does not parse: %v", i, perr)
		}
		if merr := inv.Invitation.MatchesRecord(got.Record); merr != nil {
			t.Errorf("invite %d does not match the record it was made from: %v", i, merr)
		}
	}
	// **The pages really were appended — and this assertion is here because its absence made
	// the next one vacuous.** Measured at the slice's diff review: patching
	// PrepareCeremonyDocument to `return pdf, nil` (no readme, no ceremony page, no signature
	// pages) left every convene test GREEN. CheckDocument proves hash-then-embed and cannot
	// see whether anything was ever appended, so the ordering law was guarded at one arrow
	// out of three — while Convene's own doc comment refuses dependency injection precisely
	// so "the ordering guard would not go green with the readme never appended". It did.
	basePages, err := pdfops.PageCount(base)
	if err != nil {
		t.Fatal(err)
	}
	gotPages, err := pdfops.PageCount(got.Document)
	if err != nil {
		t.Fatal(err)
	}
	signing := 0
	for _, p := range got.Record.Roster {
		if p.Signs {
			signing++
		}
	}
	if want := basePages + 2 + p2p.SignaturePagesFor(signing); gotPages != want {
		t.Errorf("the convened document has %d pages, want %d (%d content + readme + ceremony "+
			"page + %d signature page(s)) — the pre-signing pass did not append what it claims",
			gotPages, want, basePages, p2p.SignaturePagesFor(signing))
	}

	// And CheckDocument passes on the bytes the door returned, which is the hash-then-embed
	// half of the same law.
	if _, err := CheckDocument(got.Document, now); err != nil {
		t.Errorf("the convened document does not check out against its own record: %v", err)
	}
	back, err := Extract(got.Document)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != got.Record.ID || back.DocHash != got.Record.DocHash {
		t.Error("the embedded record is not the one Convene returned")
	}
	if err := back.Verify(now); err != nil {
		t.Errorf("the embedded record does not verify: %v", err)
	}
}

// TestConveneRefusesEachThingByName — six refusals, each asserted by SENTINEL and each
// asserted DISTINCT from the others.
//
// The distinctness arm is the one that matters: six rows asserting only "an error came back"
// are all satisfied by one helper returning one message, which is how a refusal set ships
// looking complete while telling a user nothing.
func TestConveneRefusesEachThingByName(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	// Setup assertion: the ordinary case SUCCEEDS, or every refusal below is equally true of
	// a door that refuses everything.
	if _, err := Convene(base, conveneReq(t, cfp, afp), cert, key, now); err != nil {
		t.Fatalf("setup: an ordinary two-party convene was refused (%v), so no refusal below "+
			"distinguishes anything", err)
	}

	seen := map[string]string{}
	for _, c := range []struct {
		name string
		want error
		mut  func(r *ConveneRequest)
	}{
		{"a roster of one", ErrRosterTooSmall, func(r *ConveneRequest) { r.Roster = r.Roster[:1] }},
		{"a duplicate party", ErrDuplicateParty, func(r *ConveneRequest) {
			r.Roster = append(r.Roster, r.Roster[1])
		}},
		{"an empty intent", ErrNoIntent, func(r *ConveneRequest) { r.Intent = "   " }},
		{"an over-long recital", ErrIntentTooLong, func(r *ConveneRequest) {
			r.Intent = strings.Repeat("We agree to grant a lease of Flat 3 Acacia Avenue ", 4)
		}},
		{"a deadline that cannot fit every hop", ErrDeadlineTooTight, func(r *ConveneRequest) {
			r.Expires = now.Add(hopBudget / 2)
		}},
		{"no hop budget", ErrNoHopBudget, func(r *ConveneRequest) { r.HopBudget = 0 }},
		// P08.S05b. **This row was missing on the first cut and a mutation found it**: deleting
		// the zero-refusal guard for DeliveryBudget left the whole suite green, because every
		// fixture supplies one and nothing asked what happens when a caller does not. That is a
		// two-arm rule covered on one arm — and a duration whose zero means "everything fits" is
		// the rule switched off, which is why HopBudget refuses its own zero rather than
		// defaulting.
		{"no delivery budget", ErrNoDeliveryBudget, func(r *ConveneRequest) { r.DeliveryBudget = 0 }},
	} {
		t.Run(c.name, func(t *testing.T) {
			req := conveneReq(t, cfp, afp)
			c.mut(&req)
			_, err := Convene(base, req, cert, key, now)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if prev, dup := seen[err.Error()]; dup {
				t.Errorf("%q produces the same sentence as %q. A convener told the same thing "+
					"for two different mistakes cannot act on either.", c.name, prev)
			}
			seen[err.Error()] = c.name
		})
	}
}

// TestConveneRefusesASecondConveneBeforeTheAttachmentLayerDoes — C04's boundary, and the
// refusal must be Nib's sentence rather than pdfcpu's.
func TestConveneRefusesASecondConveneBeforeTheAttachmentLayerDoes(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	first, err := Convene(base, conveneReq(t, cfp, afp), cert, key, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Convene(first.Document, conveneReq(t, cfp, afp), cert, key, now)
	if !errors.Is(err, ErrAlreadyConvened) {
		t.Fatalf("a second convene reported %v, want ErrAlreadyConvened", err)
	}
	if strings.Contains(err.Error(), AttachmentName) {
		t.Errorf("the refusal names a PDF internal (%q): %v — that is pdfcpu's duplicate-name "+
			"error reaching a solicitor", AttachmentName, err)
	}
}

// TestConveneInsertsTheConvenerWhenTheRosterOmitsThem — A8, and it is a fresh-install defect.
func TestConveneInsertsTheConvenerWhenTheRosterOmitsThem(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	_, _, bfp := identity(t, "B")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	req := conveneReq(t, afp, bfp) // the convener is NOT on it
	for _, p := range req.Roster {
		if p.Fingerprint == cfp {
			t.Fatal("setup: the convener is on the roster, so this test proves nothing")
		}
	}
	got, err := Convene(base, req, cert, key, now)
	if err != nil {
		t.Fatalf("a roster that omits the convener was refused: %v — on a fresh vault the "+
			"identity is minted during this call, so no client-supplied roster can contain it", err)
	}
	if got.Record.Roster[0].Fingerprint != cfp {
		t.Errorf("the convener is at position %d, want 0", indexOf(got.Record.Roster, cfp))
	}
	if _, ok := got.Record.Convener(); !ok {
		t.Error("the record's convener is not in its own roster")
	}
}

// TestConveneWarnsPastTheSittingCeilingRatherThanRefusing — D22's pin keeps 32 as the hard cap
// and ~8 as a copywriting bound. Refusing at 8 would make C03, C18 and C21 — all nine-party —
// unreachable through the only door that builds a record.
func TestConveneWarnsPastTheSittingCeilingRatherThanRefusing(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	fps := []string{cfp}
	for i := 0; i < 8; i++ {
		_, _, fp := identity(t, string(rune('A'+i)))
		fps = append(fps, fp)
	}
	req := conveneReq(t, fps...)
	req.Expires = now.Add(20 * time.Hour) // headroom for eight hops AND their delivery legs (P08.S05b)
	got, err := Convene(base, req, cert, key, now)
	if err != nil {
		t.Fatalf("a nine-party ceremony was REFUSED (%v). D22 makes ~8 a copywriting bound and "+
			"32 the hard cap; refusing here makes C03, C18 and C21 unreachable.", err)
	}
	found := false
	for _, w := range got.Warnings {
		if w.Code == WarnSittingCeiling {
			found = true
			if w.Text == "" {
				t.Error("the sitting-ceiling warning has no text for a surface to render")
			}
		}
	}
	if !found {
		t.Errorf("a nine-party ceremony carried no %q warning, so a convener is not told that "+
			"this is more than one sitting", WarnSittingCeiling)
	}
	// And a small one does NOT warn, or the warning means nothing.
	small, err := Convene(base, conveneReq(t, cfp, fps[1]), cert, key, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range small.Warnings {
		if w.Code == WarnSittingCeiling {
			t.Error("a two-party ceremony warned about the sitting ceiling")
		}
	}
}

func indexOf(roster []Party, fp string) int {
	for i, p := range roster {
		if p.Fingerprint == fp {
			return i
		}
	}
	return -1
}

// TestTheConvenedDocumentIsBUILT — the Convene -> PrepareCeremonyDocument seam, which had
// no reach at all until the slice's diff review measured it.
//
// Three mutations were green in this package before this test existed: making
// PrepareCeremonyDocument a no-op; passing len(roster) where the signing count belongs; and
// dropping the `copy(id[:], raw)` so every rendered page names ceremony 000…0 while the
// record carries the real one. CheckDocument cannot see any of them — Embed does not move
// the digest (ContentDigest excludes the record by name), so a document that gained no pages
// still hashes consistently with its own record.
//
// The roster here has a NON-SIGNING convener on purpose: with everyone signing, `signing`
// and `len(roster)` are equal and the second mutation stays invisible.
func TestTheConvenedDocumentIsBUILT(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	// **SIX signers plus a non-signing convener — the numbers must STRADDLE a page boundary.**
	//
	// ceil(n/6): six signers is ONE page, seven parties is TWO. Passing len(roster) where the
	// signing count belongs therefore changes the page count and this test sees it. A first
	// draft used seven signers and eight parties, which both round to two pages — the
	// assertion was written correctly and the fixture could not exercise it. Measured: the
	// mutation stayed green until these numbers moved.
	req := ConveneRequest{
		Intent: "We agree to co-sign the lease", Expires: now.Add(20 * time.Hour),
		HopBudget: hopBudget, DeliveryBudget: hopBudget, ConvenerSigns: false,
		Roster: []Party{{Fingerprint: cfp, Label: "Convener", Signs: false}},
	}
	for i := 0; i < 6; i++ {
		_, _, fp := identity(t, string(rune('A'+i)))
		req.Roster = append(req.Roster, Party{Fingerprint: fp, Label: string(rune('A' + i)), Signs: true})
	}
	got, err := Convene(base, req, cert, key, now)
	if err != nil {
		t.Fatal(err)
	}
	signing := 0
	for _, p := range got.Record.Roster {
		if p.Signs {
			signing++
		}
	}
	if signing != 6 || len(got.Record.Roster) != 7 {
		t.Fatalf("setup: %d signers of %d parties — this test needs them to DIFFER, or the "+
			"signing-count mutation is invisible", signing, len(got.Record.Roster))
	}
	// And they must straddle a page boundary, or they differ and the page count still cannot
	// tell them apart — which is exactly how the first draft of this fixture failed.
	if p2p.SignaturePagesFor(signing) == p2p.SignaturePagesFor(len(got.Record.Roster)) {
		t.Fatalf("setup: %d signers and %d parties both allocate %d page(s), so substituting "+
			"one for the other is invisible to the assertion below",
			signing, len(got.Record.Roster), p2p.SignaturePagesFor(signing))
	}
	basePages, _ := pdfops.PageCount(base)
	gotPages, err := pdfops.PageCount(got.Document)
	if err != nil {
		t.Fatal(err)
	}
	// Allocated from the SIGNING count, not the roster length: 7 signers -> 2 pages, and a
	// roster of 8 would also be 2, so the assertion is written against signaturePages of the
	// signing count rather than a literal.
	if want := basePages + 2 + p2p.SignaturePagesFor(signing); gotPages != want {
		t.Errorf("the convened document has %d pages, want %d — pages are allocated from the "+
			"SIGNING count (%d), not the roster length (%d)",
			gotPages, want, signing, len(got.Record.Roster))
	}

	// And the rendered pages name THIS ceremony. Without this, dropping the id copy leaves
	// every page naming ceremony 000…0 while the record carries the real one — and every
	// other assertion in the package still passes.
	txt := renderedText(t, got.Document, gotPages)
	if !strings.Contains(txt, got.Record.ID) {
		t.Errorf("the last allocated page does not name ceremony %s: %.200q", got.Record.ID, txt)
	}
}

// renderedText extracts one page's visible text, undoing the PDF escaping the way
// internal/p2p's readme tests established.
func renderedText(t *testing.T, pdf []byte, page int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := api.ExtractContent(bytes.NewReader(pdf), []string{strconv.Itoa(page)},
		func(r io.Reader, _ int) error { _, e := io.Copy(&buf, r); return e },
		model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("extract page %d: %v", page, err)
	}
	var runs []string
	for _, m := range regexp.MustCompile(`\(((?:[^()\\]|\\.)*)\)`).FindAllStringSubmatch(buf.String(), -1) {
		runs = append(runs, m[1])
	}
	flat := strings.Join(strings.Fields(strings.Join(runs, " ")), " ")
	if flat == "" {
		t.Fatalf("extraction of page %d returned nothing — no assertion on it means anything", page)
	}
	return flat
}

// TestTheReservationCoversTheDeliveryRoundAndNotJustTheHops — P08.S05b's boundary, both arms.
//
// **This test exists because the slice would otherwise have shipped vacuously green.** Nothing in
// the tree sits within twenty hours of the reservation boundary: every convene fixture sets
// `Expires` days out, and the only `ErrDeadlineTooTight` case (`TestConveneRefusesEachThingByName`)
// uses `hopBudget/2`, which is refused with the delivery term and refused without it. So the whole
// change could be reverted and every existing assertion would stay green.
//
// The middle arm is the one that carries the slice: `hops*HopBudget + 1s` was ADMITTED before this
// change and must now be REFUSED, because the round that follows the last hop had no time reserved
// for it at all.
func TestTheReservationCoversTheDeliveryRoundAndNotJustTheHops(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	// **The delivery budget is deliberately UNEQUAL to the hop budget, and that is the point.**
	// Every fixture in the tree sets them equal, which makes `hops*DeliveryBudget` and
	// `hops*HopBudget` the same number — so a reservation that read the wrong field would pass
	// every assertion here. A review found exactly that: mutating `deliverTime` to multiply
	// HopBudget left the whole suite green. Half a hop budget is enough to tell the two apart,
	// and it is also the direction P08.S05d moves in.
	const hops = 1
	deliveryBudget := hopBudget / 2
	sign := time.Duration(hops) * hopBudget
	deliver := time.Duration(hops) * deliveryBudget
	need := sign + deliver
	if sign == deliver {
		t.Fatal("setup: the sign and deliver terms are equal, so nothing below can tell a " +
			"reservation that reads HopBudget twice from one that reads both fields")
	}

	at := func(d time.Duration) error {
		req := conveneReq(t, cfp, afp)
		req.DeliveryBudget = deliveryBudget
		req.Expires = now.Add(d)
		_, err := Convene(base, req, cert, key, now)
		return err
	}

	// SETUP: comfortably past the boundary must succeed, or every refusal below is a door that
	// refuses everything and the two arms say nothing.
	if err := at(need + time.Hour); err != nil {
		t.Fatalf("setup: a deadline an hour past the reservation was refused (%v); the arms below "+
			"cannot distinguish a working boundary from a broken door", err)
	}

	// ARM 1 — one second past the whole reservation is admitted.
	if err := at(need + time.Second); err != nil {
		t.Errorf("a deadline %s away was refused, but %d hop(s) need %s to sign and %s to deliver "+
			"= %s. The reservation is larger than the terms it is built from.",
			need+time.Second, hops, sign, deliver, need)
	}

	// ARM 2 — exactly the reservation is refused, because the comparison is strict.
	if err := at(need); !errors.Is(err, ErrDeadlineTooTight) {
		t.Errorf("a deadline exactly at the reservation returned %v; want ErrDeadlineTooTight — "+
			"`After` is strict and a ceremony that ends at the instant its last delivery finishes "+
			"has no room for it", err)
	}

	// ARM 3 — THE ONE THAT CARRIES THE SLICE. Enough for the hops and nothing for the round.
	// Before P08.S05b this was admitted; a revert makes exactly this line go green again.
	if err := at(sign + time.Second); !errors.Is(err, ErrDeadlineTooTight) {
		t.Errorf("a deadline %s away — enough for %d hop(s) of signing and one second more — was "+
			"admitted (%v). The delivery round that follows the last hop needs %s and had none "+
			"reserved, so the ceremony would end before its own document could be delivered.",
			sign+time.Second, hops, err, deliver)
	}
}

// TestTheRefusalNamesBOTHTermsRatherThanOneTotal — the sentence a convener reasons from.
//
// The refusal is the only place a convener learns how much time to add. Folding the delivery
// reservation into a single "N hops need X" figure makes that sentence an arithmetic lie: X is not
// hop time, and a convener who divides it by N to size a hop gets a number twice too large.
func TestTheRefusalNamesBOTHTermsRatherThanOneTotal(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	req := conveneReq(t, cfp, afp)
	// Unequal, for the boundary test's reason: with sign == deliver the three figures collapse
	// and "each term is named" cannot be distinguished from "one term printed twice".
	req.DeliveryBudget = hopBudget / 2
	req.Expires = now.Add(time.Minute) // far inside the reservation
	_, err = Convene(base, req, cert, key, now)
	if !errors.Is(err, ErrDeadlineTooTight) {
		t.Fatalf("setup: want ErrDeadlineTooTight, got %v — nothing below reads a refusal", err)
	}
	msg := err.Error()
	for _, want := range []string{"to sign", "to deliver", "in all"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not contain %q; a convener cannot tell which half of the "+
				"time they are short of.\n  got: %s", want, msg)
		}
	}
	// **The three figures are asserted BY VALUE, not by counting one of them.** The first cut
	// checked `strings.Count(msg, hopBudget) >= 2`, which passes only because this fixture's sign
	// and deliver terms happen to be equal — it would stop meaning anything the moment P08.S05d
	// makes DeliveryBudget smaller, and it could never tell "two terms printed" from "one term
	// printed twice". Each term is named separately, and so is the total that must be their sum.
	hops := len(req.Roster) - 1
	sign := time.Duration(hops) * req.HopBudget
	deliver := time.Duration(hops) * req.DeliveryBudget
	if deliver == 0 {
		t.Fatal("setup: the delivery term is zero, so the figures collapse and the assertions " +
			"below cannot tell a folded sentence from a split one")
	}
	for label, d := range map[string]time.Duration{"sign": sign, "deliver": deliver, "total": sign + deliver} {
		if !strings.Contains(msg, d.String()) {
			t.Errorf("the refusal omits the %s figure (%s); a convener cannot size a new deadline "+
				"from a sentence missing one of its terms.\n  got: %s", label, d, msg)
		}
	}
}

// TestTheSittingWarningStaysArithmeticallyTrue — P08.S05b.
//
// The warning tells a convener how much of their attention a long ceremony costs. Folding the
// delivery reservation into that total would break the sentence's own arithmetic — it names a
// per-hop figure and a hop count, and a reader who multiplies them must get the total it prints —
// and it would miscategorise the round, which is machine time on legs the convener does not sit
// through, as "the convener's attention". So the round is a separate clause with its own figure.
func TestTheSittingWarningStaysArithmeticallyTrue(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	parties := []string{cfp}
	for i := 0; i < SittingCeiling; i++ {
		_, _, fp := identity(t, "P"+itoa(i))
		parties = append(parties, fp)
	}
	req := conveneReq(t, parties...)
	// Unequal: with the two budgets the same, `Contains(text, round)` is satisfied by the
	// attention figure and the second assertion carries no information beyond the literal word.
	req.DeliveryBudget = hopBudget / 2
	// A day inside MaxCeremonyLife rather than exactly on it — `record.go`'s ceiling is a strict
	// `After`, so an exact-30d fixture is one comparison-operator change away from red for a
	// reason that has nothing to do with this test.
	req.Expires = now.Add(29 * 24 * time.Hour)
	got, err := Convene(base, req, cert, key, now)
	if err != nil {
		t.Fatalf("setup: the over-ceiling convene was refused (%v), so there is no warning to read", err)
	}
	var text string
	for _, w := range got.Warnings {
		if w.Code == WarnSittingCeiling {
			text = w.Text
		}
	}
	if text == "" {
		t.Fatal("setup: no sitting-ceiling warning was produced, so nothing below reads a sentence")
	}

	hops := len(got.Record.Roster) - 1
	attention := time.Duration(hops) * req.HopBudget
	round := time.Duration(hops) * req.DeliveryBudget

	// The attention figure must be hops × HopBudget EXACTLY — the product of the two numbers the
	// sentence itself gives the reader.
	if !strings.Contains(text, attention.String()) {
		t.Errorf("the warning does not state %s as the convener's attention (hops %d × %s). A "+
			"reader who multiplies the two figures the sentence gives them must arrive at the "+
			"total it prints.\n  got: %s", attention, hops, req.HopBudget, text)
	}
	// And the round is named separately rather than folded in.
	if !strings.Contains(text, round.String()) || !strings.Contains(text, "Delivering") {
		t.Errorf("the warning does not name the delivery reservation (%s) as its own clause; "+
			"folded into the attention total it would make the sentence's arithmetic false.\n"+
			"  got: %s", round, text)
	}
}
