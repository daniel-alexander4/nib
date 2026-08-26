package server

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// inviteFor builds a real two-party ceremony and returns the counterparty's invitation text
// together with the convener's fingerprint.
func inviteFor(t *testing.T, otherFP string) (invitation, convenerFP string) {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	fpb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	convenerFP = hex.EncodeToString(fpb)
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	out, err := ceremony.Convene(base, ceremony.ConveneRequest{
		Roster: []ceremony.Party{
			{Fingerprint: convenerFP, Label: "Convener", Signs: true},
			{Fingerprint: otherFP, Label: "The other party", Capacity: "as Director", Signs: true},
		},
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Now().Add(48 * time.Hour),
		HopBudget:     ceremonyHopBudget(),
		ConvenerSigns: true,
	}, cert, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, inv := range out.Invites {
		if strings.EqualFold(inv.Party.Fingerprint, otherFP) {
			invitation = inv.Text
		}
	}
	if invitation == "" {
		t.Fatal("setup: convene issued no invitation for the counterparty")
	}
	return invitation, convenerFP
}

// TestAcceptingAnInvitationRemovesTheManualPin — D21, driven through the door the arm
// actually refuses at.
//
// The refusal this closes is `handleSessionArm`'s: "that peer isn't pinned — pin their
// fingerprint first". It fires BEFORE the invitation is parsed, so a party holding a perfectly
// good invitation could not arm. The assertion is therefore written as **arm fails, accept, arm
// succeeds** rather than as "a pin appeared in the vault": a pin that exists and does not satisfy
// the door it was created for satisfies nothing.
func TestAcceptingAnInvitationRemovesTheManualPin(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, convenerFP := inviteFor(t, me)

	// Stimulus, and it is the whole point: arming against the convener is REFUSED right now.
	code, body := postForCode(t, c, csrf, ts.URL+"/api/session/arm",
		armRequest{Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp", Invitation: invitation})
	if code != http.StatusBadRequest || !strings.Contains(body, "isn't pinned") {
		t.Fatalf("setup: arming against an unpinned convener returned %d %q — this test is "+
			"about removing that refusal, and it is not there to remove", code, body)
	}

	var got acceptResponse
	acode, abody := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation})
	if acode != http.StatusOK {
		t.Fatalf("accept returned %d: %s", acode, abody)
	}
	if err := json.Unmarshal([]byte(abody), &got); err != nil {
		t.Fatal(err)
	}
	// **One pin, not the roster.** D22 is a hub: this party can only ever be on a hop with the
	// convener, so pinning anybody else would pin a peer it can never dial.
	if got.Pinned != 1 {
		t.Errorf("accept established %d pins, want exactly 1 — a counterparty's only possible "+
			"hop partner is the convener (hopBetween), so any other pin is a stranger this "+
			"machine will never dial", got.Pinned)
	}
	if got.Signing != 2 {
		t.Errorf("the response says %d obliged signers, want 2", got.Signing)
	}
	var sawSelf, sawConvener bool
	for _, p := range got.Roster {
		if p.Self {
			sawSelf = true
		}
		if p.Convener {
			sawConvener = true
			if p.Name == "" {
				t.Error("the convener is returned with no six-word name — that name is the " +
					"only pre-commit check a party has on who invited them")
			}
		}
	}
	if !sawSelf || !sawConvener {
		t.Errorf("the response marks self=%v convener=%v; a reader cannot find either without "+
			"re-deriving them", sawSelf, sawConvener)
	}

	// The refusal is gone, and that is the criterion.
	code2, body2 := postForCode(t, c, csrf, ts.URL+"/api/session/arm",
		armRequest{Fingerprint: convenerFP, Bind: "127.0.0.1:0", Transport: "tcp", Invitation: invitation})
	if code2 != http.StatusOK {
		t.Fatalf("after accepting the invitation, arming still failed: %d %q — no party may "+
			"have to pin a fingerprint by hand to take part in a ceremony they were invited "+
			"to (D21)", code2, body2)
	}
	postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{})
}

// TestAcceptRefusesEachThingByName — every refusal asserted DISTINCT from the others, because
// one helper printing one sentence satisfies five rows otherwise.
func TestAcceptRefusesEachThingByName(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	good, _ := inviteFor(t, me)
	// An invitation for somebody else entirely.
	notMine, _ := inviteFor(t, strings.Repeat("aa", 32))

	seen := map[string]string{}
	for _, tc := range []struct{ name, invitation, want string }{
		{"not an invitation", "hello", "not a Nib invitation"},
		{"damaged", good[:len(good)-2] + "00", "damaged"},
		{"not one of its parties", notMine, "does not name you as one of its parties"},
	} {
		code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
			acceptRequest{Invitation: tc.invitation})
		if code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400 (%s)", tc.name, code, body)
			continue
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s: the refusal does not say %q: %s", tc.name, tc.want, body)
		}
		if prior, dup := seen[body]; dup {
			t.Errorf("%s and %s produce the SAME sentence (%q) — one message covering two "+
				"refusals means a user cannot tell which happened", tc.name, prior, body)
		}
		seen[body] = tc.name
	}

	// And the convener accepting their own invitation, which is its own fault and its own
	// sentence: it would pin nobody (the door skips self) and report success.
	convenerInv, _ := inviteForConvener(t, me)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: convenerInv})
	if code != http.StatusBadRequest || !strings.Contains(body, "you convened this ceremony") {
		t.Errorf("accepting your own invitation returned %d %q, want a named refusal", code, body)
	}
}

// inviteForConvener returns an invitation whose CONVENER field names `meFP` — the self-accept
// case, which the route refuses.
//
// Assembled rather than convened, because a real convene signs with a key this test cannot
// reach: the record is signed by a throwaway identity and the invitation's `ConvenerFingerprint`
// is then set to `meFP`. That is exactly the state the route must catch — the invitation SAYS
// this machine convened — and it is reachable in production the same way, since nothing signs
// the invitation (PLAN-1).
func inviteForConvener(t *testing.T, meFP string) (invitation, convenerFP string) {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Someone")
	if err != nil {
		t.Fatal(err)
	}
	fpb, _ := sign.Fingerprint(cert)
	id, err := ceremony.NewID()
	if err != nil {
		t.Fatal(err)
	}
	rec := ceremony.Record{
		ID:      id,
		DocHash: strings.Repeat("ab", 32),
		Intent:  "We agree",
		Expires: time.Now().Add(48 * time.Hour),
		Roster: []ceremony.Party{
			{Fingerprint: hex.EncodeToString(fpb), Label: "Someone", Signs: true},
			{Fingerprint: meFP, Label: "Me", Signs: true},
		},
	}
	if err := rec.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	invites, err := ceremony.NewInvitations(rec)
	if err != nil {
		t.Fatal(err)
	}
	inv := invites[meFP]
	inv.ConvenerFingerprint = meFP
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return text, meFP
}

// myFingerprint is this server's own signing fingerprint, read through the route that mints the
// identity on first use — so the ceremonies these tests build actually name this machine.
func myFingerprint(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	resp, err := c.Get(base + "/api/identity")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	pem, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := sign.Fingerprint(pem)
	if err != nil {
		t.Fatalf("the identity route did not return a certificate: %v", err)
	}
	return hex.EncodeToString(fp)
}

// postForCode posts JSON and returns the status and the body, because these tests are about
// WHICH refusal happened and `write` only hands back a response.
func postForCode(t *testing.T, c *http.Client, csrf, url string, v any) (int, string) {
	t.Helper()
	resp := write(t, c, csrf, http.MethodPost, url, "application/json", jsonBody(v))
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(b))
}

// TestConveningPinsItsRosterAndKeepsTheSecretOutOfTheMirror — two criteria in one drive,
// because they need the same state and it is expensive to build twice.
//
// **This is also the first Go test of ANY kind over `POST /api/ceremony/convene`.** S02a
// live-verified the route with a scratchpad script that no longer exists, so between that slice
// and this one the product's only ceremony-creating surface was exercised by nothing in the
// committed suite. Found while looking for a test to hang T07 on.
//
// The two criteria:
//
//  1. **The convener's own pins (D21, from the hub side).** The convener has to arm against each
//     party in turn, and `handleSessionArm` refuses an unpinned peer — so a convene that does
//     not pin leaves the convener typing N-1 fingerprints by hand.
//  2. **The invitation secret is never written under `~/nib/ceremonies/` (D29, P01's parked
//     criterion).** That directory is ordinary files under the user's home; the vault is sealed.
//     The walk asserts the directory exists and is NON-EMPTY before it grades anything, because
//     an absence check over a directory nothing created is green having read nothing.
func TestConveningPinsItsRosterAndKeepsTheSecretOutOfTheMirror(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	// **After startServer, not before.** It sets HOME itself, so a HOME set here is overwritten
	// and the walk below would search a directory the server never writes to — an absence check
	// pointed at the wrong place, which passes.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	partyA := strings.Repeat("1a", 32)
	partyB := strings.Repeat("2b", 32)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: partyA, Label: "A", Signs: true},
			{Fingerprint: partyB, Label: "B", Capacity: "as Director", Signs: true},
		},
		Intent:        "We agree to co-sign",
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
	if len(out.Invites) != 2 {
		t.Fatalf("convene issued %d invitations, want 2", len(out.Invites))
	}

	// (1) Both parties are pinned, and arming against one is no longer refused.
	presp, err := c.Get(ts.URL + "/api/peers")
	if err != nil {
		t.Fatal(err)
	}
	var pins peersResponse
	perr := json.NewDecoder(presp.Body).Decode(&pins)
	presp.Body.Close()
	if perr != nil {
		t.Fatal(perr)
	}
	want := map[string]bool{partyA: false, partyB: false}
	for _, p := range pins.Peers {
		if _, ok := want[strings.ToLower(p.Fingerprint)]; ok {
			want[strings.ToLower(p.Fingerprint)] = true
		}
	}
	for fp, got := range want {
		if !got {
			t.Errorf("party %s is not pinned after convening — the convener would have to pin "+
				"every party by hand before arming against them, which is D21's harm from the "+
				"hub side", short8(fp))
		}
	}

	// ARM, because the criterion is "after a ceremony is armed" — an arm opens a listener,
	// derives the rendezvous key from the secret, and is the moment a careless implementation
	// would write it beside the record.
	acode, abody := postForCode(t, c, csrf, ts.URL+"/api/session/arm", armRequest{
		Fingerprint: partyA, Bind: "127.0.0.1:0", Transport: "tcp",
		Invitation: out.Invites[0].Invitation,
	})
	if acode != http.StatusOK {
		t.Fatalf("arm after convene: %d %s — the convener could not arm against a party they "+
			"had just convened with", acode, abody)
	}
	t.Cleanup(func() { postForCode(t, c, csrf, ts.URL+"/api/session/disarm", struct{}{}) })

	// (2) The secret, in every spelling a writer might reach for, is absent from the mirror.
	root := filepath.Join(home, "nib", "ceremonies")
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Fatalf("setup: %s does not exist after a convene (%v) — the walk below would be an "+
			"absence check over nothing, which is green having read nothing", root, err)
	}
	var needles [][]byte
	for _, text := range out.Invites {
		inv, err := ceremony.ParseInvitation(text.Invitation)
		if err != nil {
			t.Fatal(err)
		}
		hexed := hex.EncodeToString(inv.Secret)
		needles = append(needles, inv.Secret, []byte(hexed), []byte(strings.ToUpper(hexed)),
			[]byte(base64.StdEncoding.EncodeToString(inv.Secret)),
			[]byte(base64.RawURLEncoding.EncodeToString(inv.Secret)))
	}
	walked := 0
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		walked++
		for _, n := range needles {
			if len(n) > 0 && strings.Contains(string(b), string(n)) {
				t.Errorf("an invitation secret appears in %s. It keys the rendezvous, the "+
					"record encryption and the channel binding, and this directory is "+
					"ordinary files under the user's home (D29)", path)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if walked == 0 {
		t.Fatalf("%s exists but holds no files, so the search read no bytes and its clean "+
			"result means nothing", root)
	}
	t.Logf("searched %d file(s) under %s for %d spellings of 2 secrets", walked, root, len(needles))
}
