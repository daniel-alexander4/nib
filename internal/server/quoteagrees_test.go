package server

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
)

// P06.S06 — the block a party is shown is the block their key signs (/pending 317).

// TestTheQuotedBlockIsTheBlockThatGetsSigned is the criterion, per line.
//
// **`StampCommitment` overwrites six fields inside a ceremony** — the roster hash and its version,
// the recital (the RECORD's; *"whatever the caller put here is discarded"*), the position, the
// roster size and the capacity — plus the signer's label. It was called at both signing points and
// at neither quote, so a party read one block and signed another.
//
// **Compared per line, and the count is the finding.** A whole-object comparison says the two
// disagree; it cannot say which line moved, and "five of six" was the item's own estimate of a
// number nothing had measured. Per line, a regression names the field.
func TestTheQuotedBlockIsTheBlockThatGetsSigned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, v := unlockedServer(t)

	cert, _, err := identity(v)
	if err != nil {
		t.Fatal(err)
	}
	myFPb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	me := hex.EncodeToString(myFPb)
	other := strings.Repeat("6f", 32)
	if err := v.AddCeremonyPeer(mustHex(t, other), "Bob Landlord", ""); err != nil {
		t.Fatal(err)
	}

	roster := p2p.Roster{
		Commitment: strings.Repeat("ab", 32), CommitmentVersion: 3,
		Intent: "We agree to the lease of 14 Elm Row",
		Entries: []p2p.RosterEntry{
			{Fingerprint: other, Signs: true, Label: "Bob Landlord"},
			{Fingerprint: me, Signs: true, Label: "Alice Tenant", Capacity: "as attorney-in-fact"},
		},
	}

	w := httptest.NewRecorder()
	p := cosignParams{Fingerprint: other, Intent: "typed into the box", When: time.Now().UTC().Format(time.RFC3339)}

	// SETUP: without a roster the block is the manual one — and it must DIFFER from the stamped
	// one, or this test cannot tell a stamp from a no-op.
	plain, ok := srv.cosignAttestation(w, v, p, p2p.Roster{})
	if !ok {
		t.Fatalf("setup: the unstamped attestation was refused: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	quoted, ok := srv.cosignAttestation(w, v, p, roster)
	if !ok {
		t.Fatalf("the stamped attestation was refused: %d %s", w.Code, w.Body.String())
	}
	if strings.Join(plain.AppearanceLines(), "\n") == strings.Join(quoted.AppearanceLines(), "\n") {
		t.Fatalf("setup: the roster changed nothing, so this fixture cannot tell a stamp from a "+
			"no-op:\n%v", quoted.AppearanceLines())
	}

	// What the SIGNING path produces from the same inputs: the attestation as it reaches
	// `Contribute`, which is `cosignAttestation` plus `StampCommitment` — the call `buildCoSigned`
	// and `coSignExchange` both make.
	signed := plain
	signed.AcceptedPeer = p2p.PredecessorOf(roster, me)
	p2p.StampCommitment(&signed, roster, me)

	q, sgn := quoted.AppearanceLines(), signed.AppearanceLines()
	if len(q) != len(sgn) {
		t.Fatalf("the quote renders %d lines and the signature %d:\nquote:  %v\nsigned: %v",
			len(q), len(sgn), q, sgn)
	}
	for i := range q {
		if q[i] != sgn[i] {
			t.Errorf("line %d differs.\n  quoted: %q\n  signed: %q\n"+
				"The party read the first and their key signed the second — which is the whole of "+
				"/pending 317, and the field that moved is named right here", i+1, q[i], sgn[i])
		}
	}
}

// TestTheFitIsAskedOfTheStampedBlock.
//
// **The fit check ran BEFORE the stamp and stamping ADDS LINES.** A stamped block carries a
// capacity and a "Party k of n" line that an unstamped one does not, so a ceremony block could pass
// the quote's height check and overflow at the signature — which becomes `ErrBlockOffThePage`, and
// P07.S08 records that pdfcpu CLAMPS overflow silently, making an instrument built to see it blind.
//
// Driven with a recital long enough that the extra lines are what tips it: the unstamped block
// fits, the stamped one does not, and the route must refuse.
func TestTheFitIsAskedOfTheStampedBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, v := unlockedServer(t)
	cert, _, _ := identity(v)
	myFPb, _ := sign.Fingerprint(cert)
	me := hex.EncodeToString(myFPb)
	other := strings.Repeat("6f", 32)
	if err := v.AddCeremonyPeer(mustHex(t, other), "Bob Landlord", ""); err != nil {
		t.Fatal(err)
	}

	long := strings.Repeat("the parties agree to the terms set out in the schedule annexed. ", 6)
	roster := p2p.Roster{
		Commitment: strings.Repeat("ab", 32), CommitmentVersion: 3, Intent: long,
		Entries: []p2p.RosterEntry{
			{Fingerprint: other, Signs: true, Label: "Bob Landlord"},
			{Fingerprint: me, Signs: true, Label: "Alice Tenant", Capacity: "as attorney-in-fact for Acme Holdings Limited"},
		},
	}
	p := cosignParams{Fingerprint: other, Intent: "short"}

	// SETUP: the UNSTAMPED block fits. Without this the refusal below could be about a recital
	// that was too long either way, and the stamp would prove nothing.
	w := httptest.NewRecorder()
	if _, ok := srv.cosignAttestation(w, v, p, p2p.Roster{}); !ok {
		t.Skipf("this fixture's unstamped block already overflows (%s), so the stamped refusal "+
			"below would not be about the stamp — recorded as a skip rather than a pass",
			strings.TrimSpace(w.Body.String()))
	}

	w = httptest.NewRecorder()
	att, ok := srv.cosignAttestation(w, v, p, roster)
	if ok {
		if p2p.BlockFits(att) {
			t.Skipf("the stamped block still fits (%d lines), so this fixture does not reach the "+
				"case — the assertion is about a block that overflows only once stamped",
				p2p.BlockLineCount(att))
		}
		t.Errorf("a block that does NOT fit was accepted (%d lines). The height check ran before "+
			"the stamp, so the quote answered for a block smaller than the one that will be "+
			"signed — and pdfcpu clamps the overflow silently", p2p.BlockLineCount(att))
	}
}

// TestTheResponderQuotePinsAndEchoesItsTime.
//
// The initiating side has always minted `when` at its quote and posted it back within
// `maxWhenSkew`. The responder's quote returned no time at all and `coSignExchange` took
// `time.Now()` at contribution — so the block a party consented to and the block signed differed by
// however long they spent reading the document, on top of the six stamped fields.
func TestTheResponderQuotePinsAndEchoesItsTime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv, v := unlockedServer(t)
	other := strings.Repeat("6f", 32)
	if err := v.AddCeremonyPeer(mustHex(t, other), "Bob Landlord", ""); err != nil {
		t.Fatal(err)
	}
	// Set directly rather than through `setPending`, which refuses an anchor that names no live
	// arm — correct there, and beside the point here: this test is about what the QUOTE route
	// answers for a pending request, not about how one comes to be pending.
	srv.sess.mu.Lock()
	srv.sess.pending = &pendingReq{
		view: pendingView{Fingerprint: other}, resp: make(chan sessionDecision, 1),
	}
	srv.sess.mu.Unlock()

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/session/quote", strings.NewReader(`{"intent":"ok"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", srv.csrf)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/session/quote returned %d", resp.StatusCode)
	}
	var q cosignQuote
	if err := json.NewDecoder(resp.Body).Decode(&q); err != nil {
		t.Fatal(err)
	}
	if q.When == "" {
		t.Fatal("the responder's quote returns no time. The block a party consents to must be the " +
			"block that is signed, and without a pinned time the signature carries the moment the " +
			"bytes were signed rather than the moment they were asked")
	}
	when, perr := time.Parse(time.RFC3339, q.When)
	if perr != nil {
		t.Fatalf("the quoted time %q is not RFC3339: %v", q.When, perr)
	}
	if d := time.Since(when); d < -time.Minute || d > time.Minute {
		t.Errorf("the quoted time is %v from now", d)
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
