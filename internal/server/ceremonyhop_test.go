package server

import (
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

// hopFixture convenes a real 3-party ceremony over a real open document, through the real routes.
//
// **Driven over HTTP rather than by calling handlers, and that is deliberate.** The refusals this
// file asserts are about the DOOR — the pinning header, the entitlement, the deadline — and a
// hand-called handler skips the mux, the CSRF check and `requireUnlocked`. This slice's whole
// finding was a route whose gates were green while the product could not reach it, so the tests go
// through the same front door the client does.
type hopFixture struct {
	c       *http.Client
	csrf    string
	base    string
	id      string
	me      string
	pdfPath string
}

func newHopFixture(t *testing.T, expiresIn time.Duration) *hopFixture {
	t.Helper()
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("2b", 32), Label: "Bob", Signs: true},
			{Fingerprint: strings.Repeat("3c", 32), Label: "Carla", Signs: true},
		},
		Intent: "We agree to co-sign the lease", ConvenerSigns: true,
		Expires: time.Now().Add(expiresIn).UTC().Format(time.RFC3339),
	})
	if code != http.StatusOK {
		t.Fatalf("convene: %d %s", code, body)
	}
	var out conveneResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return &hopFixture{c: c, csrf: csrf, base: ts.URL, id: out.Ceremony, me: me, pdfPath: pdfPath}
}

// quote calls the GET, optionally pinned to a document other than the active one.
func (f *hopFixture) quote(t *testing.T, doc string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.base+"/api/ceremony/hop?ceremony="+f.id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc != "" {
		req.Header.Set("X-Nib-Doc", doc)
	}
	res, err := f.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// TestTheHopRouteRefusesBeforeItDials — P01.S02b's refusals, each driven separately.
//
// **One fault per request.** A request that is wrong in four ways proves only that something
// refused it, and tier 4's own L3 clause was that shape once — two faults true at once, credited to
// whichever fired first.
func TestTheHopRouteRefusesBeforeItDials(t *testing.T) {
	t.Run("a ceremony with room is NOT refused", func(t *testing.T) {
		// The control, and it is what stops every case below asserting a route that refuses
		// everything — a guard that refuses everyone satisfies every negative test there is.
		f := newHopFixture(t, 48*time.Hour)
		code, body := f.quote(t, "")
		if code != http.StatusOK {
			t.Fatalf("a ceremony with room was refused %d: %s", code, body)
		}
	})

	t.Run("the open document belongs to ANOTHER ceremony", func(t *testing.T) {
		// ADR-001/ADR-004. The route picks WHICH PARTY to dial from the document's signatures, so a
		// fallback to whichever tab is active would let a tab switch decide who gets called and
		// whose invitation gets minted.
		//
		// **Driven with a SECOND REAL CEREMONY, and the first version of this case was inert.** It
		// pinned to `"nib-doc-that-is-not-open"` and asserted a 409 — which arrived, from
		// `resolveDoc`'s "that document is no longer open", a completely different gate. Deleting
		// the id comparison left it green; the probe found that, not review. A case that asserts a
		// status code without asserting which check produced it is satisfied by any refusal.
		f := newHopFixture(t, 48*time.Hour)
		other := f.secondCeremony(t)
		code, body := f.quote(t, other)
		if code != http.StatusConflict {
			t.Fatalf("a hop over another ceremony's document answered %d, want 409: %s", code, body)
		}
		if !strings.Contains(body, "not the one this ceremony is running over") {
			t.Errorf("the refusal came from a different gate: %s", body)
		}
	})

	t.Run("this machine holds the ceremony but did not convene it", func(t *testing.T) {
		// **Every party writes a mirror**, so holding a record proves nothing about entitlement —
		// which is the whole reason the mint's rules moved inside one door. Without this the route
		// would reach `convenerInvitationFor` and be stopped only by the ABSENCE of a secret, a 410
		// whose sentence is about storage rather than about permission.
		//
		// Built by convening under a FOREIGN identity and dropping the result into this machine's
		// own mirror and open document — which is exactly the position a party is in after
		// accepting an invitation and signing.
		f := newHopFixture(t, 48*time.Hour)
		id, path := foreignCeremony(t)
		if code, body := postForCode(t, f.c, f.csrf, f.base+"/api/open", openRequest{Path: path}); code != http.StatusOK {
			t.Fatalf("open the foreign ceremony's document: %d %s", code, body)
		}
		f.id = id
		code, body := f.quote(t, "")
		if code != http.StatusForbidden {
			t.Fatalf("a party who did not convene answered %d, want 403: %s", code, body)
		}
		if !strings.Contains(body, "only the convener") {
			t.Errorf("the refusal came from a different gate: %s", body)
		}
	})

	t.Run("the open document is in no ceremony at all", func(t *testing.T) {
		f := newHopFixture(t, 48*time.Hour)
		plain := f.plainDoc(t)
		code, body := f.quote(t, plain)
		if code != http.StatusConflict {
			t.Fatalf("a hop over a plain document answered %d, want 409: %s", code, body)
		}
		if !strings.Contains(body, "not part of a signing ceremony") {
			t.Errorf("the refusal came from a different gate: %s", body)
		}
	})

	t.Run("a ceremony this machine does not hold", func(t *testing.T) {
		f := newHopFixture(t, 48*time.Hour)
		f.id = "0123456789abcdef0123456789abcdef"
		code, body := f.quote(t, "")
		if code != http.StatusConflict {
			t.Fatalf("an unknown ceremony answered %d, want 409: %s", code, body)
		}
	})

	t.Run("no ceremony named", func(t *testing.T) {
		f := newHopFixture(t, 48*time.Hour)
		f.id = ""
		code, _ := f.quote(t, "")
		if code != http.StatusBadRequest {
			t.Fatalf("a hop with no ceremony answered %d, want 400", code)
		}
	})
}

// TestTheConvenersOwnTurnIsNotADial — a signing convener is FIRST, so their turn has nobody to call.
//
// This is the ordinary state of a ceremony that has just been convened with `convenerSigns`, which
// `#cerISign` makes the default. Saying so is the honest answer; offering a call would be an action
// that does not exist behind a label — the exact defect P01.S02's block was set for.
func TestTheConvenersOwnTurnIsNotADial(t *testing.T) {
	f := newHopFixture(t, 48*time.Hour)
	code, body := f.quote(t, "")
	if code != http.StatusOK {
		t.Fatalf("the quote was refused %d: %s", code, body)
	}
	var q ceremonyHopQuote
	if err := json.Unmarshal([]byte(body), &q); err != nil {
		t.Fatal(err)
	}
	if !q.Mine {
		t.Fatalf("the quote does not report the convener's own turn: %s", body)
	}
	// **`mine` and `contributes` are different questions and both are true here.** The first cut
	// returned before computing `contributes`, so a signing convener's own turn published
	// `{"mine":true,"contributes":false}` — not a short-circuit but a wrong answer, and the suite
	// did not notice because nothing asserted the field. Found by reading a tier-6 response body.
	if !q.Contributes {
		t.Errorf("the quote says a SIGNING convener does not contribute at their own turn: %s — "+
			"`mine` is 'is the next signer me?' and `contributes` is 'do I sign at this hop?', and "+
			"a convener with convenerSigns is both", body)
	}
	// And the POST refuses rather than dialling the machine it runs on.
	code, body = postForCode(t, f.c, f.csrf, f.base+"/api/ceremony/hop",
		ceremonyHopRequest{Ceremony: f.id})
	if code != http.StatusConflict {
		t.Fatalf("a hop to this machine's own turn answered %d, want 409: %s", code, body)
	}
	if !strings.Contains(body, "nobody to call") {
		t.Errorf("the refusal does not say why: %s", body)
	}
}

// TestTheClientNamesNoPartyOnAHop — D1, made structural rather than promised.
//
// P01.S02's acceptance asks that "the enabled action equals `next`'s answer; a red proof shows a
// client-side guess diverging". Under this design a guess cannot diverge in EFFECT, because the
// request carries no party: the server resolves the turn for the quote and again for the dial, and
// the client sends a ceremony id and a PNG. So the property asserted is the one that makes the
// clause unfalsifiable in the right direction — there is nothing in the request shape to guess with.
func TestTheClientNamesNoPartyOnAHop(t *testing.T) {
	var shaped ceremonyHopRequest
	raw := `{"ceremony":"c","fingerprint":"` + strings.Repeat("aa", 32) + `","party":"Bob"}`
	if err := json.Unmarshal([]byte(raw), &shaped); err != nil {
		t.Fatal(err)
	}
	if shaped.Ceremony != "c" {
		t.Fatalf("the ceremony id did not decode: %+v", shaped)
	}
	// The stimulus floor: a field the shape DOES carry decodes, so this is not asserting over a
	// type that ignores everything.
	var withPNG ceremonyHopRequest
	if err := json.Unmarshal([]byte(`{"ceremony":"c","appearance":"AAAA"}`), &withPNG); err != nil {
		t.Fatal(err)
	}
	if withPNG.Appearance != "AAAA" {
		t.Fatal("the appearance field did not decode, so the negative below proves nothing")
	}
	b, err := json.Marshal(shaped)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"fingerprint", "party"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("the hop request carries %q — the client can name a party again, and D1's "+
				"rule is that the rail renders the server's answer rather than holding one", banned)
		}
	}
}

// TestOneDialInFlightPerParty — the guard, contended.
//
// A hop costs a fresh socket, a DHT bootstrap and a race bounded by `connectDeadline`, all of it
// off-link under ADR-011. Two presses would fan out two of everything at the same party.
func TestOneDialInFlightPerParty(t *testing.T) {
	s := &Server{epoch: "t"}
	party := strings.Repeat("5e", 32)
	release, held := s.holdHop("cid", party)
	if !held {
		t.Fatal("the first hold was refused")
	}
	if _, second := s.holdHop("cid", party); second {
		t.Error("two dials for the same party of the same ceremony are in flight at once")
	}
	// A DIFFERENT party of the same ceremony is not in contention, which is why the key is a pair —
	// `legKey`'s own corrected mistake was keying on the ceremony alone, unique only because the
	// walk happened to be serial.
	other, ok := s.holdHop("cid", strings.Repeat("7a", 32))
	if !ok {
		t.Error("a different party of the same ceremony was refused")
	}
	other()
	release()
	if _, again := s.holdHop("cid", party); !again {
		t.Error("the slot was not released")
	}
}

// TestAHopNeedsRoomForAWholeHop — the budget rule, tested where it can be reached.
//
// **The route-level case cannot be built through the front door, and that is a fact about the
// product rather than a gap in the test.** `Convene` refuses a deadline that does not leave room for
// every hop AND every delivery leg — measured here: a five-minute deadline is rejected at convene
// with *"2 hops need about 58m40s to sign and about 39m40s to deliver afterwards"*. So no ceremony
// can exist whose deadline was too short when it was created; the case arises only from the passage
// of time, which a unit test cannot produce.
//
// So the rule is driven at `checkMintAllowed`, which is where it lives, with a record built to sit
// inside the budget. The distinction being asserted is the one that matters: NOT "past the
// deadline" but "less than one whole hop from it" — the weaker check let a far party's consent land
// thirteen minutes after the proceeding had ended.
func TestAHopNeedsRoomForAWholeHop(t *testing.T) {
	now := time.Now()
	inside := ceremony.Record{Expires: now.Add(ceremonyHopBudget() / 2)}
	if err := recordOutlivesBudget(inside, now, ceremonyHopBudget()); err == nil {
		t.Error("a ceremony ending inside one hop budget is admitted — somebody would be asked to " +
			"consent to a signature on a proceeding that has ended by the time it completes")
	} else if !strings.Contains(err.Error(), "less than one hop") {
		t.Errorf("the refusal is not the budget one: %v", err)
	}
	// **The control, and it is the half that stops this asserting a rule that refuses everything.**
	beyond := ceremony.Record{Expires: now.Add(ceremonyHopBudget() * 2)}
	if err := recordOutlivesBudget(beyond, now, ceremonyHopBudget()); err != nil {
		t.Errorf("a ceremony with room for two hops was refused: %v", err)
	}
	// And the ZERO budget is a different question — "not past the deadline" — which is what the
	// re-issue route wants and what a hop must not settle for.
	stillOpen := ceremony.Record{Expires: now.Add(time.Minute)}
	if err := recordOutlivesBudget(stillOpen, now, 0); err != nil {
		t.Errorf("a ceremony a minute from its deadline cannot re-issue an invitation: %v", err)
	}
	if err := recordOutlivesBudget(stillOpen, now, ceremonyHopBudget()); err == nil {
		t.Error("a ceremony a minute from its deadline admits a whole hop — the two budgets are " +
			"not distinguishable, which is the collapse mintInvitationFor's parameter exists to stop")
	}
}

// secondCeremony opens a second document, convenes over it, and returns ITS document id.
//
// The point is a document that is genuinely open and genuinely in a ceremony — just not this one.
// A closed id, or a plain file, is refused by an earlier gate and proves nothing about the
// comparison this case exists for.
func (f *hopFixture) secondCeremony(t *testing.T) string {
	t.Helper()
	path := f.copyOfTheFixturePDF(t)
	if code, body := postForCode(t, f.c, f.csrf, f.base+"/api/open", openRequest{Path: path}); code != http.StatusOK {
		t.Fatalf("open second: %d %s", code, body)
	}
	code, body := postForCode(t, f.c, f.csrf, f.base+"/api/ceremony/convene", conveneRequest{
		Roster: []convenePartyRequest{
			{Fingerprint: f.me, Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("4d", 32), Label: "Dana", Signs: true},
		},
		Intent: "A second matter", ConvenerSigns: true,
		Expires: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
	})
	if code != http.StatusOK {
		t.Fatalf("convene second: %d %s", code, body)
	}
	var out conveneResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Doc.ID == "" {
		t.Fatal("the second convene returned no document id")
	}
	return out.Doc.ID
}

// plainDoc opens a document that is in no ceremony.
func (f *hopFixture) plainDoc(t *testing.T) string {
	t.Helper()
	path := f.copyOfTheFixturePDF(t)
	code, body := postForCode(t, f.c, f.csrf, f.base+"/api/open", openRequest{Path: path})
	if code != http.StatusOK {
		t.Fatalf("open plain: %d %s", code, body)
	}
	var d docResponse
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	return d.ID
}

// copyOfTheFixturePDF makes a second file so a second document can be opened.
//
// `/api/open` keys by path, so opening the same path twice returns the SAME document — which would
// make both cases above pin to the ceremony's own document and pass for the wrong reason.
func (f *hopFixture) copyOfTheFixturePDF(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(f.pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "another.pdf")
	if err := os.WriteFile(dst, src, 0o600); err != nil {
		t.Fatal(err)
	}
	return dst
}

// foreignCeremony convenes under an identity that is NOT this machine's, writes the mirror this
// machine would hold as a party, and returns the ceremony id and a path to its document.
func foreignCeremony(t *testing.T) (string, string) {
	t.Helper()
	cert, key, err := sign.GenerateIdentity("Somebody Else")
	if err != nil {
		t.Fatal(err)
	}
	fpb, err := sign.Fingerprint(cert)
	if err != nil {
		t.Fatal(err)
	}
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	out, err := ceremony.Convene(base, ceremony.ConveneRequest{
		Roster: []ceremony.Party{
			{Fingerprint: hex.EncodeToString(fpb), Label: "Their Convener", Signs: true},
			{Fingerprint: strings.Repeat("6f", 32), Label: "Someone", Signs: true},
		},
		Intent: "Their matter", Expires: time.Now().Add(48 * time.Hour),
		HopBudget: ceremonyHopBudget(), DeliveryBudget: ceremonyDeliveryLegBudget(),
		ConvenerSigns: true,
	}, cert, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, werr := ceremony.WriteMirror(defaultOutputDir(), out.Record, out.Document); werr != nil {
		t.Fatal(werr)
	}
	path := filepath.Join(t.TempDir(), "theirs.pdf")
	if werr := os.WriteFile(path, out.Document, 0o600); werr != nil {
		t.Fatal(werr)
	}
	return out.Record.ID, path
}
