package server

import (
	"encoding/hex"
	"errors"
	"net/http"
	"nib/internal/p2p"
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

// TestTheDialSideAlsoRoutesThroughTheArrivalCheck is the second door (P07.S07b).
//
// `handleSessionInitiate` reads its ceremony identity from a pasted invitation and hands
// `l3Roster()` to the L3 gate and to `buildCoSigned`, which stamps that roster's label, capacity
// and — since this slice — its RECITAL onto the signature it applies. All three are read from an
// unsigned invitation, so the same reconciliation the receiving side has always run has to run
// here, and it has to run BEFORE the local signature exists: refusing afterwards leaves the user
// signed into something this build has just refused, and a signature cannot be taken back off a
// document. That is the deadline check's own stated reasoning one line above it.
func TestTheDialSideAlsoRoutesThroughTheArrivalCheck(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	code := stripLineComments(string(src))
	i := strings.Index(code, "func (s *Server) handleSessionInitiate(")
	if i < 0 {
		t.Fatal("cannot find handleSessionInitiate — this guard is reading the wrong thing")
	}
	body := funcBodyFrom(code, i)
	if body == "" {
		t.Fatal("could not brace-match handleSessionInitiate's body")
	}
	arrival := strings.Index(body, "checkArrival(")
	if arrival < 0 {
		t.Fatal("handleSessionInitiate never reconciles its invitation against the document's " +
			"record. It reads the roster from a pasted invitation and signs a block carrying " +
			"that roster's label, capacity and recital, so an invitation naming a different " +
			"proceeding is signed into the document unchallenged — C17 at the door nobody " +
			"checked.")
	}
	build := strings.Index(body, "buildCoSigned(")
	if build < 0 {
		t.Fatal("handleSessionInitiate no longer applies the local signature; this guard is " +
			"asserting an order between things that are no longer both here")
	}
	if arrival > build {
		t.Error("handleSessionInitiate applies the local signature BEFORE reconciling the " +
			"invitation against the record. Refusing after that leaves the user signed into a " +
			"proceeding this build has just refused, and a signature cannot be taken back off a " +
			"document.")
	}
}

// TestTheConsentGateRoutesThroughTheArrivalCheck — ADR-009, asserted on the ROUTING.
//
// The rule is "a document is reconciled against its invitation BEFORE it is acted on", and it
// holds at TWO call sites — `sessionConfirmer.Confirm` here and `handleSessionInitiate` in the
// companion test below. Asserting that `checkArrival` *can* return an error says nothing about
// whether anything calls it — which is exactly the shape ADR-009 was written from, and exactly
// how this slice's C17 clause could ship dead.
//
// **This comment said "one call site" and that was true when it was written and wrong by
// P07.S07b.** The second door existed the whole time and had no check on it: a party who
// INITIATES used its invitation's roster to gate L3, and after P07.S07a to write labels and
// capacities onto its own block, having never compared that invitation to the record the
// document carries. A guard scoped to one function cannot report the absence of the other, which
// is why the companion below scans a different function rather than widening this one.
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
	return convenedExpiring(t, 48*time.Hour)
}

// convenedExpiring is convenedFor with the deadline as a parameter, so a test can hand the arrival
// gate a proceeding that is genuinely OVER (/pending 345).
//
// **It convenes in the PAST rather than convening an expired ceremony, and the difference is a
// measurement.** The obvious shape — `Expires: time.Now().Add(-time.Minute)` — is REFUSED, and not
// by the clock you would expect: `Record.Verify`'s only comparison is a future ceiling
// (`Expires.After(now + MaxCeremonyLife)`), so nothing there objects, but `Convene` reserves a hop
// budget and a delivery round and answers *"1 hop need about 29m20s to sign and about 29m20s to
// deliver afterwards — 58m40s in all"* (P08.S05b). So the ceremony is convened at a `now` three
// hours ago, where that reservation is satisfied, and its deadline still lands wherever `in` puts
// it relative to the real clock. `Convene` taking `now` as an argument is what makes this possible
// at all.
func convenedExpiring(t *testing.T, in time.Duration) (doc []byte, invitation string) {
	t.Helper()
	// Far enough back that the hop-plus-delivery reservation is satisfied for any `in` a test is
	// likely to want, and well inside MaxCeremonyLife so the future ceiling is untouched.
	convenedAt := time.Now().Add(-3 * time.Hour)
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
		Intent:         "We agree to co-sign the lease",
		Expires:        time.Now().Add(in),
		HopBudget:      ceremonyHopBudget(),
		DeliveryBudget: ceremonyDeliveryLegBudget(),
		ConvenerSigns:  true,
	}, cert, key, convenedAt)
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

// TestTheArrivalGateRefusesAnEndedProceeding — P08.S04a.
//
// # What was wrong
//
// The only `Expires`-vs-`now` comparison a signing party ran was `Record.Verify`'s
// `MaxCeremonyLife` ceiling, which refuses a deadline too far in the FUTURE and never one in the
// past. The one refusal that existed, `checkCeremonyDeadline`, had a single production caller and
// it was on the **convener's** side — so whoever convened owned the only clock and a signer could
// be collected into a proceeding D28 declares over.
//
// # The third arm is the one that matters
//
// Refusing an expired ceremony is easy; refusing it *without also refusing honest hops* is the
// whole design. The convener admits a HOP when `Expires > t0 + ceremonyHopBudget()` (29m20s) — the
// per-hop rule this arithmetic is about; since P08.S05b `Convene` additionally reserves a delivery
// leg per hop up front, which only widens the admission and leaves the margin below unchanged. And
// the signer's gate runs at worst `t0 + 22m20s`. So a hop admitted at the convener's own margin
// arrives here with as little as **seven minutes** left, and any receiver-side reservation refuses
// it. Arm 3 pins that: it is what separates this gate from a hop reservation, and no other check
// in the tree can tell the two apart.
//
// The convener is BYPASSED throughout — there is no server and no route here, only the ceremony
// identity and a document, which is exactly what the acceptance bullet asks for.
func TestTheArrivalGateRefusesAnEndedProceeding(t *testing.T) {
	doc, invText := convenedFor(t)
	inv, err := ceremony.ParseInvitation(invText)
	if err != nil {
		t.Fatal(err)
	}
	cer := &ceremonyID{inv: inv}
	rec, err := ceremony.CheckRecord(doc, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	expires := rec.Expires

	// ARM 1 — the control. A live proceeding is admitted, or every refusal below is a gate that
	// refuses everything.
	if err := cer.checkArrival(doc, expires.Add(-24*time.Hour)); err != nil {
		t.Fatalf("a live proceeding was refused (%v)", err)
	}

	// ARM 2 — the proceeding has ended.
	err = cer.checkArrival(doc, expires.Add(time.Second))
	if err == nil {
		t.Error("a contribution offered AFTER the ceremony's deadline was accepted. The convener " +
			"holds the only other clock check, so nothing else would have refused it and the " +
			"signer is collected into a proceeding D28 declares over.")
	}
	if !errors.Is(err, p2p.ErrCeremonyEnded) {
		t.Errorf("the refusal is %v, want p2p.ErrCeremonyEnded — it needs a wire code, or it "+
			"reaches the initiator as bare EOF and is rendered as a network fault", err)
	}

	// ARM 3 — an honest hop at the convener's own worst case MUST be admitted.
	//
	// The convener admitted this hop when `Expires` was one instant past `t0 + 29m20s`; the signer
	// reaches this gate at worst `t0 + 22m20s`, leaving 7m00s. A receiver that reserved even eight
	// minutes would refuse it — which is the error this arm exists to catch, and which no other
	// assertion here can see.
	worstCase := expires.Add(-7 * time.Minute).Add(time.Second)
	if err := cer.checkArrival(doc, worstCase); err != nil {
		t.Errorf("an honest hop at the convener's worst case was refused (%v). The convener "+
			"admits at hop budget %s and the worst-case lag to this gate is %s, so a hop arrives "+
			"here with as little as %s left — reserving anything at this end refuses hops nobody "+
			"is at fault for.",
			err, ceremonyHopBudget(),
			bootstrapBudget+connectDeadline+p2p.ReceiveArrivalLag(),
			ceremonyHopBudget()-(bootstrapBudget+connectDeadline+p2p.ReceiveArrivalLag()))
	}
}

// TestTheArrivalGateRoutesThroughTheDeadlineDoor is T06 — ADR-009's half, and it exists because
// **no structural guard in this tree reads inside `checkArrival`**.
//
// The two scans that look like they might (`TestEveryCeremonyDialRoutesThroughTheDeadlineDoor` and
// the consent-signers scan) brace-match `handleSessionInitiate` and `Confirm` — `checkArrival`'s
// two CALLERS — so a check added inside `checkArrival` itself is invisible to every existing
// guard. The behavioural test above proves the rule holds today; this proves the next edit cannot
// quietly route around it.
//
// It asserts the ROUTING, not the sentence: eight copies of a message checked for agreement say
// nothing about a ninth site added without one.
func TestTheArrivalGateRoutesThroughTheDeadlineDoor(t *testing.T) {
	raw, err := os.ReadFile("ceremonyid.go")
	if err != nil {
		t.Fatal(err)
	}
	// Comments stripped: `checkArrival`'s own doc names the call it makes, and a scan satisfied by
	// prose is how a freeze guard once read its own explanation as proof of coverage (v1.117.155).
	src := stripLineComments(string(raw))
	i := strings.Index(src, "func (c *ceremonyID) checkArrival(")
	if i < 0 {
		t.Fatal("setup: checkArrival not found — this guard is pinned to a function that no " +
			"longer exists, and would otherwise pass over nothing")
	}
	body := funcBodyFrom(src, i)
	// STIMULUS: the body must be non-trivial, or "it contains the call" is true of an empty string.
	if len(body) < 100 {
		t.Fatalf("setup: checkArrival's body read as %d bytes — the brace matcher is not reading "+
			"the function", len(body))
	}
	if !strings.Contains(body, "recordOutlivesBudget(") {
		t.Error("checkArrival does not route through recordOutlivesBudget. The signing party's own " +
			"deadline check is the ONE thing standing between a signer and a proceeding that has " +
			"already ended — the convener's door cannot cover it, because the convener is the " +
			"party this gate exists to bypass.")
	}
	if !strings.Contains(body, "ceremony.CheckRecord(") {
		t.Error("checkArrival no longer verifies the record it judges — an unverified Expires is " +
			"a number a stranger chose")
	}
}
