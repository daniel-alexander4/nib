package server

import (
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestDecliningACeremonyRevokesItsPins — P01's parked criterion, driven at last (D29).
//
// The pin is established through the real accept route and revoked through the real decline
// path's helper, and the assertion is the criterion's own words: the peer list is byte-identical
// to what it was before.
func TestDecliningACeremonyRevokesItsPins(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)

	v := srv.unlockedVault()
	if v == nil {
		t.Fatal("setup: the vault is locked, so nothing below can pin anything")
	}
	before := v.PinnedPeers()

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); code != http.StatusOK {
		t.Fatalf("accept: %d %s", code, body)
	}
	// Stimulus: the pin really is there, and it is CEREMONY-scoped. A prune of a user pin would
	// be a different bug and this assertion is what keeps the two apart.
	after := v.PinnedPeers()
	if len(after) != len(before)+1 {
		t.Fatalf("accepting added %d pins, want 1 — the revocation below would prove nothing",
			len(after)-len(before))
	}
	var scoped bool
	for _, p := range after {
		if hex.EncodeToString(p.Fingerprint) == convenerFP {
			scoped = len(p.Ceremonies) > 0
		}
	}
	if !scoped {
		t.Fatal("the convener was pinned WITHOUT a ceremony scope, so PruneCeremonyPeers " +
			"cannot take it away and D29's revocable pin is revocable in name only")
	}

	inv, err := ceremony.ParseInvitation(invitation)
	if err != nil {
		t.Fatal(err)
	}
	srv.declineCeremony(&ceremonyID{inv: inv})

	got := v.PinnedPeers()
	if len(got) != len(before) {
		t.Fatalf("after declining, the peer list holds %d pins and held %d before the ceremony "+
			"— a revocable pin that outlives the thing it was for is a permanent pin with "+
			"extra steps", len(got), len(before))
	}
	for i := range got {
		if hex.EncodeToString(got[i].Fingerprint) != hex.EncodeToString(before[i].Fingerprint) ||
			got[i].Label != before[i].Label || len(got[i].Ceremonies) != len(before[i].Ceremonies) {
			t.Errorf("pin %d differs from before the ceremony: %+v vs %+v", i, got[i], before[i])
		}
	}
}

// TestTheConsentGateRoutesThroughTheArrivalCheck — ADR-009, asserted on the ROUTING.
//
// The rule is "a received document is reconciled against its invitation BEFORE the user sees
// it", and it holds at one call site: `sessionConfirmer.Confirm`. Asserting that `checkArrival`
// *can* return an error says nothing about whether anything calls it — which is exactly the
// shape ADR-009 was written from, and exactly how this slice's C17 clause could ship dead.
//
// Three properties, and the ORDER is one of them:
//
//  1. `Confirm` calls `checkArrival`.
//  2. It calls it BEFORE `setPending` — a document parked for the user is a document they read.
//  3. It calls it BEFORE `saw.mark()`. **This one is defensive and says so:** `p2p.Receive` runs
//     the spoken check before reading any document byte and `ConfirmVerification` marks there, so
//     on every co-sign today the arm is already spent before `Confirm` is entered. The ordering
//     is asserted because it is the one that stays correct if a path ever reaches `Confirm`
//     without a spoken check — not because it changes anything now.
//
// Comments are stripped, because a scan satisfied by prose that merely NAMES the call is how
// `handleSave`'s freeze guard read its own explanation as proof of coverage (v1.117.155).
func TestTheConsentGateRoutesThroughTheArrivalCheck(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	i := strings.Index(code, "func (sc sessionConfirmer) Confirm(")
	if i < 0 {
		t.Fatal("cannot find sessionConfirmer.Confirm — this guard is reading the wrong thing")
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("could not brace-match Confirm's body")
	}
	at := func(needle string) int {
		n := strings.Index(body, needle)
		if n < 0 {
			t.Errorf("Confirm does not contain %q", needle)
		}
		return n
	}
	arrival := at("checkArrival(")
	pending := at("setPending(")
	mark := at(".mark()")
	if arrival < 0 || pending < 0 || mark < 0 {
		t.FailNow()
	}
	if arrival > pending {
		t.Error("Confirm parks the document for the user BEFORE reconciling it against the " +
			"invitation. By the time the check runs the user has already read it, and C17's " +
			"whole clause is the order.")
	}
	if arrival > mark {
		t.Error("Confirm marks the connection as having reached the user BEFORE the arrival " +
			"check. Harmless today — the spoken check has already marked by then — and wrong " +
			"the moment a path reaches Confirm without one, because a document the gate " +
			"refused was shown to nobody and must not spend the arm.")
	}

	// And the decline half (D29): a decline prunes, and a TIMEOUT does not.
	if !strings.Contains(body, "declineCeremony(") {
		t.Error("Confirm does not revoke the ceremony's pins on a decline — D29's revocable " +
			"pin then outlives the ceremony it was taken on for")
	}
	tail := body[at("ErrConsentTimedOut"):]
	if strings.Contains(tail, "declineCeremony(") {
		t.Error("the consent TIMEOUT path prunes the ceremony's pins. Nobody was at the " +
			"machine and the user has decided nothing; unpinning on their behalf makes " +
			"stepping away from the desk revoke a relationship.")
	}
	// The stimulus for the two absence checks above: the token really does appear somewhere in
	// this file, so "not found in the tail" is a fact about position rather than about spelling.
	if !regexp.MustCompile(`func \(s \*Server\) declineCeremony\(`).MatchString(string(src)) {
		t.Error("declineCeremony is not defined in this file, so the scans above are looking " +
			"for a name that could not appear and their clean result means nothing")
	}
}

// TestTheArrivalCheckRefusesADocumentTheInvitationDoesNotDescribe — C17's substance, next to the
// routing guard above.
//
// Two arms and they fail for different reasons, which is the point: a document with no ceremony
// record at all (somebody handing you an unrelated file under a ceremony arm), and a document
// carrying a perfectly valid record for a DIFFERENT ceremony (the convener running two chains).
//
// And a control, because a gate that refuses everything satisfies both arms: the document this
// invitation was actually made for passes.
func TestTheArrivalCheckRefusesADocumentTheInvitationDoesNotDescribe(t *testing.T) {
	doc, invText := convenedFor(t)
	inv, err := ceremony.ParseInvitation(invText)
	if err != nil {
		t.Fatal(err)
	}
	cer := &ceremonyID{inv: inv}

	// The control FIRST: without it the two refusals below say nothing.
	if err := cer.checkArrival(doc, time.Now()); err != nil {
		t.Fatalf("the document this invitation was made for was refused (%v) — every refusal "+
			"below would then be a gate that refuses everything", err)
	}

	plain, err := testpdf.Text("an unrelated page")
	if err != nil {
		t.Fatal(err)
	}
	if err := cer.checkArrival(plain, time.Now()); err == nil {
		t.Error("a document with no ceremony record at all was accepted under a ceremony arm")
	}

	// A second, honestly-convened ceremony: a valid record, a valid convener signature, and not
	// the one this invitation commits to.
	other, _ := convenedFor(t)
	err = cer.checkArrival(other, time.Now())
	if err == nil {
		t.Fatal("a document carrying a DIFFERENT ceremony's record was accepted. One " +
			"invitation would then authorise any number of proceedings, which is what " +
			"RosterHash exists to make impossible.")
	}
	if !errors.Is(err, ceremony.ErrRosterMismatch) {
		t.Errorf("the refusal is %v, want an ErrRosterMismatch so a caller can tell a tampered "+
			"or substituted ceremony from an unreadable document", err)
	}
}

// convenedFor builds a fresh two-party ceremony and returns the convened document together with
// the counterparty's invitation — the two artifacts that are supposed to agree.
func convenedFor(t *testing.T) (doc []byte, invitation string) {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	fpb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	other := strings.Repeat("3c", 32)
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ceremony.Convene(base, ceremony.ConveneRequest{
		Roster: []ceremony.Party{
			{Fingerprint: hex.EncodeToString(fpb), Label: "Convener", Signs: true},
			{Fingerprint: other, Label: "A", Signs: true},
		},
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Now().Add(48 * time.Hour),
		HopBudget:     ceremonyHopBudget(),
		ConvenerSigns: true,
	}, cert, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range out.Invites {
		if strings.EqualFold(i.Party.Fingerprint, other) {
			invitation = i.Text
		}
	}
	if invitation == "" {
		t.Fatal("setup: no invitation for the counterparty")
	}
	return out.Document, invitation
}

// TestEveryCeremonySessionGetsItsCeremony — the finding this slice's deepdive turned up, guarded
// where it can actually be guarded.
//
// `consentAnchor` carries a ceremony on the QUIC coordinator path and NOT on the accept loop's,
// and `runSession` was never handed one although `handleSessionArm` had stored it. So a TCP
// ceremony hop — a shape `handleSessionArm`'s own comment says exists — ran with no ceremony:
// `rd` was nil, and `ReDeliverer`'s contract says nil means "the manual/LAN path, which has no
// ceremony hop to key on", which was false there. A reconnect after a lost channel therefore
// re-signed instead of re-delivering, stacking a second, different block.
//
// **Structural, and the behavioural gap is stated rather than implied.** Driving a TCP ceremony
// through a rendezvous, a lost channel and a reconnect is a tier-4 shape, not a package test; the
// re-delivery test that exists (`TestCeremonyReDeliversAfterReconnect`) runs the QUIC path, where
// `anchor.cer` is set, so it stays green against the exact regression this prevents — measured.
// What is checkable here is that neither consumer reads the ceremony off the anchor, and that
// both call sites pass one.
func TestEveryCeremonySessionGetsItsCeremony(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))

	if !strings.Contains(code, "func (s *Server) serveOneSession(anchor consentAnchor, cer *ceremonyID,") {
		t.Fatal("serveOneSession no longer takes an explicit ceremony. Reading it off the " +
			"anchor makes every TCP ceremony hop look like a manual transfer.")
	}
	body := funcBodyFrom(code, strings.Index(code, "func (s *Server) serveOneSession("))
	if body == "" {
		t.Fatal("could not brace-match serveOneSession")
	}
	if strings.Contains(body, "anchor.cer") {
		t.Error("serveOneSession reads `anchor.cer`. The anchor is empty on the accept loop's " +
			"path, so anything derived from it is nil for every TCP ceremony — and filling the " +
			"anchor in instead would re-point consentAnchor.current, which is what " +
			"`stale-consent-on-new-session` guards.")
	}

	// Both call sites hand one over. A site that passes nil is the defect returning at a door
	// nobody looked at — the shape ADR-009 is written from.
	sites := regexp.MustCompile(`s\.serveOneSession\(([^)]*)\)`).FindAllStringSubmatch(code, -1)
	if len(sites) != 2 {
		t.Fatalf("found %d serveOneSession call sites, want 2 — the scan is not reading the "+
			"file it thinks it is", len(sites))
	}
	for _, m := range sites {
		if strings.Contains(m[1], "nil,") {
			t.Errorf("a serveOneSession call site passes a nil ceremony: %s", m[0])
		}
	}

	// And runSession, which is where the ceremony was dropped in the first place.
	if !strings.Contains(code, "func (s *Server) runSession(ln p2p.Listener, cer *ceremonyID,") {
		t.Error("runSession no longer takes the ceremony. handleSessionArm stores one on the " +
			"session and then starts this goroutine; a signature without it is how the TCP " +
			"ceremony path lost its ceremony the first time.")
	}
	rs := regexp.MustCompile(`go s\.runSession\(([^)]*)\)`).FindStringSubmatch(code)
	if rs == nil {
		t.Fatal("cannot find the runSession call site")
	}
	if !strings.Contains(rs[1], "cer") {
		t.Errorf("the runSession call site does not pass the ceremony: %s", rs[0])
	}
}
