package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// P08.S01 — a ceremony survives a restart on every machine, not just the convener's.
//
// # The defect these drive, stated once
//
// Before this slice `handleCeremonyAccept` pinned the convener and threw the invitation away, on
// the argument — written at the line — that "the arm already takes the invitation text on every
// call, so nothing needs it at rest". D24 makes a ceremony span quitting Nib, and at that point the
// argument stops holding: a party who accepted an invitation and then restarted had a pin, an
// identity, and no way to rejoin, because `ceremonyFor` begins at `ParseInvitation(text)` and the
// rendezvous key, both salts and the channel binding are all HKDF over the secret inside that text.
//
// The manual step D21 removed came back one process boundary out, and nothing in the tree could see
// it: no harness restarts an instance, so nothing had ever read state written by a previous process.

// TestAcceptPersistsTheInvitationSoAReArmNeedsNoPaste is inventory row S01-1.
//
// **The assertion is on the arm, not on the vault row**, for the reason
// `TestAcceptingAnInvitationRemovesTheManualPin` gives about pins: a row that exists and does not
// satisfy the door it was created for satisfies nothing. What is checked is that an arm carrying
// only a ceremony id — no invitation text anywhere in the request — is accepted.
func TestAcceptPersistsTheInvitationSoAReArmNeedsNoPaste(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)

	inv, err := ceremony.ParseInvitation(invitation)
	if err != nil {
		t.Fatal(err)
	}

	acode, abody := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation})
	if acode != http.StatusOK {
		t.Fatalf("accept returned %d: %s", acode, abody)
	}

	// **Stimulus, and it has to come after the accept.** The pin check fires before the invitation
	// is resolved at all — measured: an id-only arm before accepting is refused with "that peer
	// isn't pinned", which says nothing about the lookup. So the discriminating probe is an arm
	// that IS pinned, naming a ceremony this machine never accepted: it isolates the one branch
	// under test, and it fails if the id is ignored and the stored invitation used regardless.
	other := "cc" + strings.Repeat("0", 30)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm",
		armRequest{Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp", Ceremony: other})
	if code != http.StatusBadRequest || !strings.Contains(body, "holds no invitation") {
		t.Fatalf("setup: arming by an unknown ceremony id returned %d %q — the lookup either does "+
			"not happen or does not discriminate, and the pass below would mean nothing", code, body)
	}

	// The real id, with no invitation text anywhere in the request.
	code, body = postForCode(t, c, csrf, ts.URL+"/api/session/arm",
		armRequest{Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp", Ceremony: inv.ID})
	if code != http.StatusOK {
		t.Fatalf("arming by ceremony id after accepting returned %d %q — the invitation was "+
			"accepted, so this machine should be able to rejoin without it being pasted again",
			code, body)
	}
}

// TestAReArmFromDiskCarriesNoInvitation is inventory row S01-2, and it is an ABSENCE assertion.
//
// **Why it is written against the marshalled request rather than against the handler**: the whole
// defect class here is a harness supplying what the product lacks — `pairrepro.sh` keeps every
// invitation in a shell variable, so a "restarted" party re-armed from the harness's own copy and
// the run stayed green over a capability that did not exist (ADR-010's configured-past-the-
// disagreement shape). An assertion that the arm *worked* is true either way. What separates them
// is that the bytes on the wire contain no invitation, so that is what is asserted.
func TestAReArmFromDiskCarriesNoInvitation(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)
	inv, err := ceremony.ParseInvitation(invitation)
	if err != nil {
		t.Fatal(err)
	}
	if acode, abody := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); acode != http.StatusOK {
		t.Fatalf("accept returned %d: %s", acode, abody)
	}

	req := armRequest{Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp", Ceremony: inv.ID}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	// The bytes that would go on the wire carry no invitation, and — the half that matters — no
	// substring of one either, so a field renamed rather than removed still fails here.
	if strings.Contains(string(body), "invitation") {
		t.Errorf("the re-arm request body mentions an invitation: %s", body)
	}
	if strings.Contains(string(body), invitation[:32]) {
		t.Errorf("the re-arm request body carries the invitation text itself: %s", body)
	}
	if code, rbody := postForCode(t, c, csrf, ts.URL+"/api/session/arm", req); code != http.StatusOK {
		t.Fatalf("the arm this test asserts is invitation-free returned %d %q", code, rbody)
	}
}

// TestAnArmNamesOneSourceForItsInvitation is inventory row S01-3.
//
// Two sources for one value, with the loser silent, is the drift this repo keeps finding. The
// refusal is the same shape `checkTransport` records for an unknown transport: refuse, never
// downgrade.
func TestAnArmNamesOneSourceForItsInvitation(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)
	inv, err := ceremony.ParseInvitation(invitation)
	if err != nil {
		t.Fatal(err)
	}
	if acode, abody := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); acode != http.StatusOK {
		t.Fatalf("accept returned %d: %s", acode, abody)
	}

	// Both supplied, and both individually VALID — so a build that silently picked either one
	// would arm successfully. That is what makes this a real refusal rather than a parse error.
	code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp",
		Ceremony: inv.ID, Invitation: invitation,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("an arm naming a stored ceremony AND carrying an invitation returned %d %q — "+
			"one of the two was chosen silently, and the caller cannot tell which", code, body)
	}
	if !strings.Contains(body, "send one or the other") {
		t.Errorf("the refusal does not say which two things collided: %q", body)
	}
}

// TestAReIssuedInvitationStillMatchesItsRecord is inventory row S01-4, and it closes a live defect.
//
// `handleCeremonyInvites` rebuilds each party's invitation from the mirror's record plus the vault's
// stored secret, field by field — and it omitted `Intent`. `MatchesRecord` compares the recital and
// refuses on a mismatch, and `Contribute` refuses a ceremony signature carrying none, so **every
// re-issued invitation parsed, armed, and was then refused at the recipient's arrival gate**, after
// the convener had been told the re-issue succeeded.
//
// Nobody had hit it because this route had exactly one reference in the whole tree — its own
// registration. No Go test, no harness clause, no caller in the web client, and it is the only
// production reader of `ReadMirror` and the convener's only disk-based recovery path (D21, gap #24).
//
// **The assertion runs `MatchesRecord` itself rather than diffing fields**, deliberately. A
// field-by-field comparison is the same shape that produced the defect: a constructor that forgets
// one field, checked by a test that forgets the same one. Running the production comparison means a
// future field added to the commitment fails here without anyone remembering to extend this test.
func TestAReIssuedInvitationStillMatchesItsRecord(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	other := strings.Repeat("2b", 32)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: other, Label: "The other party", Signs: true},
		},
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	})
	if code != http.StatusOK {
		t.Fatalf("convene: %d %s", code, body)
	}
	var out conveneResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}

	// The record as it actually landed on disk — the same bytes the route reads back.
	rec, _, err := ceremony.ReadMirror(defaultOutputDir(), out.Ceremony, time.Now())
	if err != nil {
		t.Fatalf("the ceremony this test re-issues for is not on disk: %v", err)
	}
	// Stimulus: the recital is non-empty, so "the re-issued copy carries it" is a real claim and
	// not one satisfied by two empty strings. This is the exact field the defect dropped.
	if rec.Intent == "" {
		t.Fatal("setup: the convened record carries no recital, so this test cannot see the " +
			"field it exists to check")
	}

	code, body = postForCode(t, c, csrf, ts.URL+"/api/ceremony/invites",
		ceremonyInvitesRequest{Ceremony: out.Ceremony})
	if code != http.StatusOK {
		t.Fatalf("re-issue: %d %s", code, body)
	}
	var again conveneResponse
	if err := json.Unmarshal([]byte(body), &again); err != nil {
		t.Fatal(err)
	}
	if len(again.Invites) == 0 {
		t.Fatal("the re-issue returned no invitations")
	}
	for _, iv := range again.Invites {
		inv, perr := ceremony.ParseInvitation(iv.Invitation)
		if perr != nil {
			t.Fatalf("a re-issued invitation does not parse: %v", perr)
		}
		// The production comparison, which is what the recipient's arrival gate runs.
		if merr := inv.MatchesRecord(rec); merr != nil {
			t.Errorf("the invitation re-issued to %s is refused against the record it was built "+
				"from: %v — a convener who re-issues after losing an email hands out something "+
				"every recipient rejects, and is told it worked", iv.Label, merr)
		}
	}
}

// TestTheCeremonyListingSaysWhenThisNibMustNotAct is P08.S03's route half.
//
// # Why there is no lock, and why this test is about a NOTE rather than a refusal
//
// The plan asked for an exclusive lock on `~/nib/ceremonies/`. There is no locking anywhere in this
// tree, and adding some would be a second cross-process policy contradicting the one that already
// exists: `cmd/nib/main.go` decides deliberately that a launch which loses the instance race
// *"carries on and serves"*, because *"a launch that loses twice is better off running than
// refusing to start"*.
//
// The signal is already maintained — `instanceToken` is empty exactly when this process is not the
// recorded instance — so a non-primary Nib reads and must not act. `startServer` sets no token, so
// the fixture IS the non-primary case, which is the one worth driving: it is the state a user
// reaches by double-clicking Nib twice.
func TestTheCeremonyListingSaysWhenThisNibMustNotAct(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("2b", 32), Label: "The other party", Signs: true},
		},
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	})
	if code != http.StatusOK {
		t.Fatalf("convene: %d %s", code, body)
	}
	var conv conveneResponse
	if err := json.Unmarshal([]byte(body), &conv); err != nil {
		t.Fatal(err)
	}

	res, err := c.Get(ts.URL + "/api/ceremonies")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("the listing returned %d", res.StatusCode)
	}
	var got ceremoniesResponse
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	// Stimulus: the ceremony that was just convened is actually in the answer, so the assertions
	// below are about a populated listing and not an empty one.
	var found bool
	for _, s := range got.Ceremonies {
		if s.ID == conv.Ceremony {
			found = true
			if s.State != ceremony.LoadOK {
				t.Errorf("the ceremony just convened lists as %s (%s)", s.State, s.Reason)
			}
			if s.Intent == "" || len(s.Roster) != 2 {
				t.Errorf("the entry carries intent %q and %d parties — the listing answers from "+
					"record.json, so both are available without opening the document",
					s.Intent, len(s.Roster))
			}
		}
	}
	if !found {
		t.Fatalf("the ceremony just convened (%s) is not in the listing of %d",
			conv.Ceremony, len(got.Ceremonies))
	}

	// This fixture holds no instance record, which is what a second Nib looks like.
	if got.Primary {
		t.Fatal("setup: the fixture reports itself primary, so the note below cannot be checked")
	}
	if got.Note == "" {
		t.Error("a Nib that is not this machine's recorded instance lists the ceremonies and says " +
			"nothing about it — two processes would then both resume and both prune the same " +
			"folder, with no lock between them")
	}
	if !strings.Contains(got.Note, "already running") {
		t.Errorf("the note does not say why this Nib must not act: %q", got.Note)
	}
}

// TestACeremonyArmWaitsForTheCeremonyOnBothTransports is P08.S04's C05 half.
//
// # What was wrong, and why no existing test could see it
//
// `runCeremonyReceive` has bounded a ceremony arm by `ceremony.MaxCeremonyLife` since P05.S09b.
// `runSession` — every TCP arm, ceremony or not — kept `sessionAcceptTimeout`, five minutes, with no
// ceremony in the arithmetic. So a party third in a roster armed, waited while two earlier hops ran,
// and was disarmed before the baton reached them. `lan.go` states the thirty-day window as though it
// were the arm's property generally; it was true of one of the two paths.
//
// Nothing could catch it, because the only available observable was "the ceremony completed" — and a
// loopback relay finishes in seconds, so a five-minute bound and a thirty-day bound produce the same
// outcome. The fix is to expose the FIGURE: `session.until` is now in the status, and the two bounds
// are three orders of magnitude apart, so the assertion is on which of them was chosen.
//
// **The bound is still a constant and D16's amendment asks for a ceremony-scoped one** — the
// record's `Expires`. Not buildable here: an arm holds an invitation and the invitation carries no
// deadline. That is `/pending 247`, and this test asserts the ceiling that exists today.
func TestACeremonyArmWaitsForTheCeremonyOnBothTransports(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)
	if acode, abody := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); acode != http.StatusOK {
		t.Fatalf("accept: %d %s", acode, abody)
	}

	armWindow := func(t *testing.T, req armRequest) time.Duration {
		t.Helper()
		if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm", req); code != http.StatusOK {
			t.Fatalf("arm: %d %s", code, body)
		}
		res, err := c.Get(ts.URL + "/api/session/status")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var st sessionStatus
		if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
			t.Fatal(err)
		}
		if !st.Armed {
			t.Fatal("the arm reported success and the status says nothing is armed")
		}
		if st.Until == nil {
			t.Fatal("the status carries no arm window, so the bound cannot be checked and the " +
				"only available assertion is that the ceremony finished — which is true under " +
				"either bound, because a loopback relay takes seconds")
		}
		d := time.Until(*st.Until)
		if code, body := postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{}); code != http.StatusOK {
			t.Fatalf("disarm: %d %s", code, body)
		}
		return d
	}

	// Stimulus: a MANUAL arm on the same transport, so the two figures come from one code path and
	// differ only by whether a ceremony is carried. Without it a long window could just be this
	// build's timeout for everything.
	manual := armWindow(t, armRequest{Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp"})
	if manual > 2*sessionAcceptTimeout {
		t.Fatalf("setup: a manual arm's window is %s, which is not the five-minute bound this "+
			"test contrasts against", manual)
	}

	ceremonial := armWindow(t, armRequest{
		Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp", Invitation: invitation,
	})
	if ceremonial <= 2*sessionAcceptTimeout {
		t.Errorf("a TCP CEREMONY arm's window is %s — the same manual bound as a non-ceremony "+
			"arm (%s). A party third in a roster is then disarmed while the earlier hops run, "+
			"which is C05 failing on one of the two transports", ceremonial, manual)
	}
	// Named against the ceiling that exists rather than a bare "big": the figure is the point.
	if ceremonial < ceremony.MaxCeremonyLife-time.Hour {
		t.Errorf("a ceremony arm's window is %s, want about %s", ceremonial, ceremony.MaxCeremonyLife)
	}
}

// TestConveningTwiceOnOneDocumentIsRefusedAtTheRoute is P08.S07's C13 half.
//
// # What was and was not covered
//
// `ErrAlreadyConvened` has been in `internal/ceremony` since P07.S02a and
// `TestTheAlreadyConvenedRefusalNamesTheAnswerAndItsCost` drives it — at the PACKAGE. C13's word is
// "server-side", and nothing drove the route: `conveneStatus`'s 409 mapping, and the sentence a user
// actually reads, were asserted by nothing. Tier 6's "SECOND ceremony" clause convenes on a FRESH
// document, which is the allowed case.
//
// # The direction this does NOT cover, named rather than implied
//
// C13 says "a document already under a live one". This drives the convened OUTPUT — the document in
// the tab, which now carries the record. Re-opening the ORIGINAL file and convening again is not
// refused, and cannot be keyed on the hash: `docHash` is computed over the PREPARED document
// (`internal/ceremony/convene.go:230-237`), which embeds a fresh 128-bit ceremony id, so two
// convenes of one source file produce two different prepared documents with two different hashes.
// Catching that needs the original's hash stored in the record too, which is a format bump and
// therefore its own slice. Recorded in the plan.
func TestConveningTwiceOnOneDocumentIsRefusedAtTheRoute(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	req := conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("2b", 32), Label: "The other party", Signs: true},
		},
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	}
	// Stimulus: the first convene SUCCEEDS, so the refusal below is about the second one and not
	// about a roster the route would have rejected either way.
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", req); code != http.StatusOK {
		t.Fatalf("setup: the first convene returned %d %s", code, body)
	}

	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", req)
	if code != http.StatusConflict {
		t.Fatalf("convening again on the same document returned %d, want 409 — 409 is the one "+
			"status that says this is about the DOCUMENT's state rather than a field the "+
			"convener can correct: %s", code, body)
	}
	if !strings.Contains(body, "already part of a ceremony") {
		t.Errorf("the refusal does not name what is wrong: %q", body)
	}
	// C04's own wording: the message must carry the COST, because that is the half a builder omits.
	if !strings.Contains(body, "cannot be carried") && !strings.Contains(body, "signatures already") {
		t.Errorf("the refusal does not say that signatures already collected are lost, which C04 "+
			"asks for separately precisely because it is the bad news: %q", body)
	}
}

// TestAnInvitationReIssuedMidCeremonyLeavesEveryoneElseUntouched is P08.S07's C14 half.
//
// # Why this needed a red proof to be worth anything
//
// The re-issue reads each party's secret back from the vault, so "every other party's state is
// untouched" is true BY CONSTRUCTION — the comparison would pass against any behaviour, including
// one that regenerated the secret for the party being re-issued to and thereby broke their
// invitation. The red proof `reissue-regenerates-the-secret` is what makes the comparison able to
// fail; without it this test asserts that nothing changed in a function that changes nothing.
func TestAnInvitationReIssuedMidCeremonyLeavesEveryoneElseUntouched(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	partyB, partyC := strings.Repeat("2b", 32), strings.Repeat("3c", 32)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: partyB, Label: "B", Signs: true},
			{Fingerprint: partyC, Label: "C", Signs: true},
		},
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	})
	if code != http.StatusOK {
		t.Fatalf("convene: %d %s", code, body)
	}
	var first conveneResponse
	if err := json.Unmarshal([]byte(body), &first); err != nil {
		t.Fatal(err)
	}
	// Stimulus: there really are two invitations to compare, so "the others are unchanged" is a
	// claim about a populated set.
	if len(first.Invites) != 2 {
		t.Fatalf("setup: convene issued %d invitations, want 2", len(first.Invites))
	}

	code, body = postForCode(t, c, csrf, ts.URL+"/api/ceremony/invites",
		ceremonyInvitesRequest{Ceremony: first.Ceremony})
	if code != http.StatusOK {
		t.Fatalf("re-issue: %d %s", code, body)
	}
	var again conveneResponse
	if err := json.Unmarshal([]byte(body), &again); err != nil {
		t.Fatal(err)
	}
	byFP := map[string]string{}
	for _, iv := range again.Invites {
		byFP[strings.ToLower(iv.Fingerprint)] = iv.Invitation
	}
	for _, iv := range first.Invites {
		got, ok := byFP[strings.ToLower(iv.Fingerprint)]
		if !ok {
			t.Errorf("the re-issue dropped %s entirely", iv.Label)
			continue
		}
		// Byte-identical, which is the point: a re-issue hands back the SAME invitation, so a
		// party who lost their email is not made to re-accept a different one — and every other
		// party's copy stays valid, which is what "untouched" has to mean.
		if got != iv.Invitation {
			t.Errorf("the invitation re-issued to %s differs from the one convene issued — every "+
				"party who already holds theirs would now be holding a stale one", iv.Label)
		}
	}
}
