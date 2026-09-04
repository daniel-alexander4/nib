package server

import (
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// convenedDocument returns a real convened document and the convener's identity.
func convenedDocument(t *testing.T) (pdf, cert, key []byte) {
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
			{Fingerprint: hex.EncodeToString(fpb), Label: "Alice Tenant", Signs: true},
			{Fingerprint: strings.Repeat("5e", 32), Label: "Bob Landlord", Signs: true},
		},
		Intent:         "We agree to the terms",
		Expires:        time.Now().Add(48 * time.Hour),
		HopBudget:      ceremonyHopBudget(),
		DeliveryBudget: ceremonyDeliveryLegBudget(),
		ConvenerSigns:  true,
	}, cert, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return out.Document, cert, key
}

// TestTheManualCoSignRefusesADocumentInACeremony — /pending 368, D29.
//
// **The path was real and user-reachable, not a harness one.** With no invitation supplied, `cer`
// is nil, `l3Roster()` returns the zero Roster, and until P06.S09 nothing on this path looked at
// the document at all — so a party holding a convened document open, using the ordinary co-sign
// controls, produced a signature with no roster, no L3 gate and no `checkArrival`. The far end then
// refused it on a prefix mismatch: a true sentence about a document nobody should have been able to
// make, arriving after the user had already spent a signature they cannot take back.
//
// **Driven at `buildCoSigned`, the door both co-sign routes take**, rather than at either route:
// a check at one of two doors is the ADR-009 shape this repo keeps paying for.
func TestTheManualCoSignRefusesADocumentInACeremony(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, v := unlockedServer(t)
	pdf, _, _ := convenedDocument(t)
	cert, key, err := identity(v)
	if err != nil {
		t.Fatal(err)
	}

	// SETUP: this really is a document under a ceremony, and the ordinary path really does reach
	// the door. Without the first, the refusal below could be about anything; without the second,
	// a plain PDF would prove the guard fires on everything.
	if _, cerr := ceremony.Extract(pdf); cerr != nil {
		t.Fatalf("setup: the fixture carries no ceremony record (%v), so nothing here is about a "+
			"document in a proceeding", cerr)
	}
	plain, err := testpdf.Text("an ordinary document")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if _, ok := s.buildCoSigned(rec, plain, cert, key, p2p.Attestation{Signer: "Nib User", Intent: "I agree"}, nil, l3RosterFrom(nil, "", "")); !ok {
		t.Fatalf("setup: an ORDINARY document was refused by the manual co-sign door (%d %s) — the "+
			"guard fires on everything and the refusal below says nothing", rec.Code, rec.Body.String())
	}

	// STIMULUS: the same door, the same empty roster, a document already in a proceeding.
	rec2 := httptest.NewRecorder()
	if _, ok := s.buildCoSigned(rec2, pdf, cert, key, p2p.Attestation{Signer: "Nib User", Intent: "I agree"}, nil, l3RosterFrom(nil, "", "")); ok {
		t.Fatal("the manual co-sign path signed a document that carries a ceremony record. Every " +
			"other party checks a signature against that proceeding's roster and refuses one made " +
			"outside it — so this produces a signature the user has spent and nobody will accept.")
	}
	if rec2.Code != 409 {
		t.Errorf("the refusal is %d, want 409 — this is about the STATE of a proceeding, not a "+
			"malformed request, and 409 is what every other ceremony-state refusal on this route "+
			"already answers", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "part of a signing ceremony") {
		t.Errorf("the refusal says %q, which does not name the ceremony. D29's rule is that a "+
			"refusal NAMES the proceeding, so a user can act on it rather than reading it as a bug",
			rec2.Body.String())
	}
}

// TestTheCeremonyRecordIsLabelledInTheAttachmentsList — D29's attachments clause.
//
// **The record is the one embedded file in the product that is not the user's own attachment**, and
// it is the reason every editing operation on the document is being refused. Unlabelled it sits in
// the list as an anonymous `nib-ceremony.json`.
//
// The flag is the SERVER's. Matching the filename in the client would put a second copy of
// `pdfops.CeremonyRecordName` in another language, drifting the first time it changes.
func TestTheCeremonyRecordIsLabelledInTheAttachmentsList(t *testing.T) {
	pdf, _, _ := convenedDocument(t)
	aa, err := pdfops.Attachments(pdf)
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the list is not empty. A zero-length result would satisfy "no attachment is
	// mislabelled" without ever looking at one.
	if len(aa) == 0 {
		t.Fatal("setup: a convened document listed NO attachments, so nothing below is being checked")
	}
	var marked, total int
	for _, a := range aa {
		total++
		if a.Ceremony {
			marked++
			if a.Name != pdfops.CeremonyRecordName {
				t.Errorf("attachment %q is marked as the ceremony record and is not named %q",
					a.Name, pdfops.CeremonyRecordName)
			}
		} else if a.Name == pdfops.CeremonyRecordName {
			t.Errorf("the ceremony record %q is NOT marked, so the panel cannot tell it from a "+
				"file the user attached themselves", a.Name)
		}
	}
	if marked != 1 {
		t.Errorf("%d of %d attachments are marked as the ceremony record, want exactly 1", marked, total)
	}
}

// TestAnInProgressCopyIsNotNamedAsTheFinishedDocument — D28's third clause.
//
// **Every arrival used to be `"co-signed with <peer>.pdf"`**, so at hop 3 of a nine-party ceremony
// a party's own copy was named as though the proceeding were over — and named after the one peer
// who happened to hand it over, with six signatures still to collect. The name is what a user reads
// most often, because it is the tab.
//
// **The manual case is asserted to be UNCHANGED, and that is half the test.** An ordinary two-party
// co-sign IS finished when it arrives — no roster, nobody left to wait for — so renaming it would
// trade a true label for a false one in the commoner case.
func TestAnInProgressCopyIsNotNamedAsTheFinishedDocument(t *testing.T) {
	pdf, cert, key := convenedDocument(t)
	inv := invitationFromConvened(t, pdf, cert, key)
	cer, err := ceremonyFor(inv.text, cert, key, inv.peerFP)
	if err != nil {
		t.Fatalf("setup: could not build the ceremony from its own invitation: %v", err)
	}

	// SETUP: the proceeding really is unfinished. If the fixture were already complete the
	// assertion below would be about the finished branch and would pass without the fix.
	if _, nerr := p2p.NextContributor(pdf, cer.l3Roster()); nerr != nil {
		t.Fatalf("setup: this document is not mid-proceeding (%v), so the in-progress branch is "+
			"not the one being tested", nerr)
	}

	got := arrivalDocName("Bob Landlord", cer, pdf)
	if !strings.Contains(got, "in progress") {
		t.Errorf("a hop of a live ceremony is named %q. Every signing party after this one still "+
			"has to sign, so a name that reads as the finished document tells the user the "+
			"proceeding is over — on the tab, which is where they read it.", got)
	}
	if !strings.HasSuffix(got, ".pdf") {
		t.Errorf("arrivalDocName returned %q, which is not a filename", got)
	}

	// The manual path, unchanged and asserted so.
	if manual := arrivalDocName("Bob Landlord", nil, pdf); strings.Contains(manual, "in progress") {
		t.Errorf("an ordinary two-party co-sign is named %q. It has no roster and nobody left to "+
			"wait for — it IS finished when it arrives, and calling it in-progress would trade a "+
			"true label for a false one in the commoner case.", manual)
	}
}

type convenedInvitation struct {
	text   string
	peerFP []byte
}

// invitationFromConvened re-mints the counterparty's invitation from a convened document, so a test
// can build the same `*ceremonyID` an arm would.
func invitationFromConvened(t *testing.T, pdf, cert, key []byte) convenedInvitation {
	t.Helper()
	rec, err := ceremony.Extract(pdf)
	if err != nil {
		t.Fatal(err)
	}
	fpb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	me := hex.EncodeToString(fpb)
	var other ceremony.Party
	for _, p := range rec.Roster {
		if !strings.EqualFold(p.Fingerprint, me) {
			other = p
		}
	}
	if other.Fingerprint == "" {
		t.Fatal("setup: the roster names no counterparty")
	}
	peerFP, err := hex.DecodeString(other.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	rh, err := rec.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	// `NewInvitations` is the product's own door — a hand-built invitation would be a second
	// derivation of what a real one carries, and the fixture would then agree with itself.
	invs, err := ceremony.NewInvitations(rec)
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := invs[strings.ToLower(other.Fingerprint)]
	if !ok {
		t.Fatalf("setup: no invitation was issued for the counterparty (%d issued)", len(invs))
	}
	inv.RosterHash = hex.EncodeToString(rh)
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return convenedInvitation{text: text, peerFP: peerFP}
}
