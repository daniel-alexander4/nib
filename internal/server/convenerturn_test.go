package server

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// `/pending 869`: the convener's OWN turn, driven through the routes.
//
// What these tests found, stated because the item asked a question rather than named a defect:
//
//   - **The signing door works at a mid-order turn.** `buildCoSigned` — the one door that applies
//     a ceremony signature on this machine — admits a convener the roster names second once the
//     first party has signed, places their block at their roster position, leaves the first
//     signature's bytes untouched, and moves the walk on to the third party.
//   - **No route a window could call reached that door at the convener's own turn — first, middle
//     or last.** `POST /api/ceremony/hop` resolves its target as `Order[Done]` and refuses when that
//     is this machine; `/api/cosign/sign` and `/api/session/initiate` refuse a convened document
//     because the window sends no invitation. The one caller that got a signing convener through
//     was a harness posting the `invitation` form field to `/api/session/initiate`, where the
//     convener's own block rides on the dial to the NEXT party.
//
// So the refusals asserted here are each correct as a rule — a dial to yourself is not a hop, and
// a ceremony document is not co-signed outside its ceremony — and the gap was the route missing
// between them. **`POST /api/ceremony/sign-turn` is that route (ADR-133)**, and the tests below
// drive it at every position a convener can hold: first (the default), middle, and last.

// outsideCeremonyDoors posts a ceremony's document to the two signing calls the window makes that
// are not the hop route, with the field set the window sends (no invitation).
func outsideCeremonyDoors(t *testing.T, f *hopFixture, pdf []byte, peer, intent string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, route := range []string{"/api/cosign/sign", "/api/session/initiate"} {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(pdf)
		pj, _ := json.Marshal(cosignParams{Fingerprint: peer, Intent: intent})
		mw.WriteField("params", string(pj))
		aw, _ := mw.CreateFormFile("appearance", "att.png")
		aw.Write(tinyPNG(t))
		// An address nothing listens on: a request that got as far as dialling fails as
		// unreachable, which is a different answer from the refusal asserted below.
		mw.WriteField("address", "127.0.0.1:1")
		mw.WriteField("transport", "tcp")
		mw.Close()
		resp := write(t, f.c, f.csrf, http.MethodPost, f.base+route, mw.FormDataContentType(), &buf)
		// Bounded: a door that wrongly signs answers with a whole PDF.
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		resp.Body.Close()
		out[route] = http.StatusText(resp.StatusCode) + ": " + strings.TrimSpace(string(b))
	}
	return out
}

// ownTurnAsTheWindowSeesIt asserts what the rail is told at the convener's own turn and what each
// signing call but the own-turn route answers, and returns the quote the window draws its block
// from.
func ownTurnAsTheWindowSeesIt(t *testing.T, f *hopFixture, pdf []byte, peer, intent string, position, of int) ceremonyHopQuote {
	t.Helper()
	resp, err := f.c.Get(f.base + "/api/ceremony/next?ceremony=" + f.id)
	if err != nil {
		t.Fatal(err)
	}
	var next ceremonyNextResponse
	derr := json.NewDecoder(resp.Body).Decode(&next)
	resp.Body.Close()
	if derr != nil {
		t.Fatal(derr)
	}
	if !next.IsMe || next.Position != position || next.Of != of {
		t.Fatalf("setup: the rail is told %+v, want this machine's turn at %d of %d", next, position, of)
	}
	code, body := f.quote(t, "")
	var q ceremonyHopQuote
	if code != http.StatusOK || json.Unmarshal([]byte(body), &q) != nil {
		t.Fatalf("the quote at the convener's own turn: %d %s", code, body)
	}
	if !q.Mine || !q.Contributes || q.Party != "Convener" {
		t.Errorf("the quote at the convener's own turn is %s, want the convener named, mine and "+
			"contributing", body)
	}
	// The block to draw is quoted with the turn: the party's own roster label, their position among
	// the signers, and a pinned time for the signing route to echo.
	lines := strings.Join(q.Lines, "\n")
	want := "Party " + strconv.Itoa(position) + " of " + strconv.Itoa(of)
	if q.When == "" || !strings.Contains(lines, want) || !strings.Contains(lines, "Signer: Convener") {
		t.Errorf("the quote at the convener's own turn carries lines %q and when %q, want the block "+
			"to draw: the convener by their roster label, as %q, with a pinned time", q.Lines, q.When, want)
	}
	code, body = postForCode(t, f.c, f.csrf, f.base+"/api/ceremony/hop", ceremonyHopRequest{Ceremony: f.id})
	if code != http.StatusConflict || !strings.Contains(body, "nobody to call") {
		t.Errorf("a hop at the convener's own turn answered %d %s, want 409 naming that there is "+
			"nobody to call", code, body)
	}
	for route, got := range outsideCeremonyDoors(t, f, pdf, peer, intent) {
		if !strings.HasPrefix(got, "Conflict: ") || !strings.Contains(got, "Use the ceremony to sign it") {
			t.Errorf("%s took a ceremony's document with no invitation: %s — a signature made "+
				"outside the ceremony names no roster and every other party refuses it", route, got)
		}
	}
	return q
}

// convenedAs convenes over the real routes with the roster `parties` builds around this machine.
func convenedAs(t *testing.T, parties func(me string) []convenePartyRequest) *hopFixture {
	t.Helper()
	ts, srv := startServerWith(t)
	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(t.TempDir(), "form.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		Roster: parties(me), Intent: "We agree to co-sign the lease", ConvenerSigns: true,
		Expires: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
	})
	if code != http.StatusOK {
		t.Fatalf("convene: %d %s", code, body)
	}
	var out conveneResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return &hopFixture{c: c, csrf: csrf, base: ts.URL, id: out.Ceremony, me: me, pdfPath: pdfPath, srv: srv}
}

// signTurn presses the own-turn route as the window does: the quote's time, and a drawn block.
func (f *hopFixture) signTurn(t *testing.T, when string) (int, string) {
	t.Helper()
	return postForCode(t, f.c, f.csrf, f.base+"/api/ceremony/sign-turn", ceremonySignTurnRequest{
		Ceremony: f.id, Appearance: base64.StdEncoding.EncodeToString(tinyPNG(t)), When: when})
}

// openBytes is every open document's bytes, in registry order.
func (f *hopFixture) openBytes() [][]byte {
	f.srv.mu.Lock()
	defer f.srv.mu.Unlock()
	var out [][]byte
	for _, d := range f.srv.docs {
		out = append(out, d.data)
	}
	return out
}

// theOneOpenDocument is the open document's bytes, failing unless exactly one is open — a signed
// result that opened BESIDE the document it was made over is the accumulation the relay door stops.
func (f *hopFixture) theOneOpenDocument(t *testing.T) []byte {
	t.Helper()
	open := f.openBytes()
	if len(open) != 1 {
		t.Fatalf("%d documents are open, want the ceremony's one", len(open))
	}
	return open[0]
}

// signedAsAHopsResultIs asserts the own-turn route left `signed` installed and mirrored as a hop
// leaves its result: the open document and the mirror are the same bytes, an incremental update
// over `before`, carrying `signers` valid signatures the last of which is this machine's.
func signedAsAHopsResultIs(t *testing.T, f *hopFixture, before []byte, signers int, accepts string) []byte {
	t.Helper()
	signed := f.theOneOpenDocument(t)
	if bytes.Equal(signed, before) {
		t.Fatal("the route answered 200 and the open document is unchanged: nothing was signed")
	}
	// ADR-013: the signatures are what bind a party to bytes, so every earlier one must survive —
	// which an incremental update over the exact previous bytes guarantees and a rewrite does not.
	if !bytes.HasPrefix(signed, before) {
		t.Error("the convener's signature rewrote the document instead of appending to it")
	}
	st := sign.Verify(signed)
	if st.State != sign.Valid || len(st.Signers) != signers {
		t.Fatalf("after the convener signs: state %q with %d signer(s), want valid with %d",
			st.State, len(st.Signers), signers)
	}
	atts := p2p.ReadAttestations(signed)
	if len(atts) != signers || !strings.EqualFold(atts[signers-1].Fingerprint, f.me) || !atts[signers-1].Valid {
		t.Fatalf("the last signature is not this machine's valid one: %+v", atts)
	}
	for i, a := range atts {
		if !a.Valid {
			t.Errorf("signature %d of %d no longer verifies after the convener signed", i+1, signers)
		}
	}
	// What it accepts is the ROSTER's answer: the signing predecessor, or nobody for the first.
	if !strings.EqualFold(atts[signers-1].AcceptedPeer, accepts) {
		t.Errorf("the convener's block accepts %q, want %q — the party who signed before them",
			atts[signers-1].AcceptedPeer, accepts)
	}
	// ADR-012: the mirror is this machine's durable copy of the signature it just made.
	_, mirrored, err := ceremony.ReadMirror(defaultOutputDir(), f.id, time.Now())
	if err != nil {
		t.Fatalf("the mirror does not read back after the convener signed: %v", err)
	}
	if !bytes.Equal(mirrored, signed) {
		t.Error("the mirror does not hold the document the convener just signed — a restart would " +
			"come back to a ceremony at which they had not")
	}
	return signed
}

// quoteNames asserts the hop quote now calls `party`, as a hop at which this machine signs nothing.
func quoteNames(t *testing.T, f *hopFixture, party string) {
	t.Helper()
	code, body := f.quote(t, "")
	var q ceremonyHopQuote
	if code != http.StatusOK || json.Unmarshal([]byte(body), &q) != nil {
		t.Fatalf("the quote after the convener signed: %d %s", code, body)
	}
	if q.Mine || q.Party != party || q.Contributes {
		t.Errorf("after the convener signed the quote is %s, want a call to %s at which this "+
			"machine signs nothing", body, party)
	}
}

// TestAConvenerNamedSecondSignsTheirOwnTurn convenes `[Bob, me, Carla]`, puts Bob's real signature
// on the document, and does what the rail tells the convener to do.
func TestAConvenerNamedSecondSignsTheirOwnTurn(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the fixture sandboxes too; the home guard reads this body
	bobCert, bobKey, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	bobFP, err := sign.Fingerprint(bobCert)
	if err != nil {
		t.Fatal(err)
	}
	bob, carla := hex.EncodeToString(bobFP), strings.Repeat("3c", 32)
	f := hopFixtureConvenerSecondAfter(t, bob)
	rec, convened, err := ceremony.ReadMirror(defaultOutputDir(), f.id, time.Now())
	if err != nil {
		t.Fatalf("setup: the convened ceremony does not read back: %v", err)
	}
	roster := l3RosterFrom(rec.Roster, rosterHashHex(rec), rec.Intent)

	// BEFORE Bob has signed, the convener is out of turn: refused by name, nothing signed.
	code, body := f.signTurn(t, "")
	if code != http.StatusConflict || !strings.Contains(body, "Bob’s turn") {
		t.Errorf("the convener signing ahead of Bob answered %d %s, want 409 naming Bob's turn", code, body)
	}
	if got := f.theOneOpenDocument(t); !bytes.Equal(got, convened) {
		t.Fatal("an out-of-turn press changed the document")
	}

	// SETUP: Bob's turn, through the signing door, installed and mirrored as a hop's result is.
	w := httptest.NewRecorder()
	bobSigned, ok := f.srv.buildCoSigned(w, convened, bobCert, bobKey,
		p2p.Attestation{Signer: "Bob", AcceptedPeer: f.me, Intent: rec.Intent, When: time.Now()},
		tinyPNG(t), roster)
	if !ok {
		t.Fatalf("setup: the first party was refused at their own turn: %d %s", w.Code, w.Body.String())
	}
	if _, err := f.srv.installCeremonyResult(f.id, "form.pdf", bobSigned); err != nil {
		t.Fatal(err)
	}
	f.srv.mirrorHop(bobSigned)

	// 1. What the window is offered: the turn is this machine's, the hop and the outside doors
	//    refuse, and the quote carries the block to draw.
	q := ownTurnAsTheWindowSeesIt(t, f, bobSigned, carla, rec.Intent, 2, 3)

	// 2. A press with no block is refused BEFORE anything is signed: an empty appearance is an
	//    invisible signature, and a signature cannot be taken back off.
	code, body = postForCode(t, f.c, f.csrf, f.base+"/api/ceremony/sign-turn",
		ceremonySignTurnRequest{Ceremony: f.id, When: q.When})
	if code != http.StatusBadRequest || !strings.Contains(body, "invisible signature") {
		t.Errorf("a press with no signature block answered %d %s, want 400 naming the invisible signature", code, body)
	}
	if got := f.theOneOpenDocument(t); !bytes.Equal(got, bobSigned) {
		t.Fatal("a press with no signature block changed the document")
	}

	// 3. The route. The time echoed is one the convener read the block at three hours ago — inside
	//    the skew bound, and far enough from now that a route ignoring it is seen.
	if q.When == "" {
		t.Fatal("the quote pinned no time")
	}
	read := time.Now().Add(-3 * time.Hour).UTC()
	if code, body = f.signTurn(t, read.Format(time.RFC3339)); code != http.StatusOK {
		t.Fatalf("the convener's own turn, second of three: %d %s", code, body)
	}
	mine := signedAsAHopsResultIs(t, f, bobSigned, 2, bob)
	// The block is where the ROSTER puts the second signer: Bob's page, the slot above Bob's.
	pb, berr := p2p.PlacementFor(convened, roster, bob)
	pm, merr := p2p.PlacementFor(bobSigned, roster, f.me)
	if berr != nil || merr != nil {
		t.Fatalf("placement: %v, %v", berr, merr)
	}
	if pm.Page != pb.Page || pm.Rect[1] < pb.Rect[3] {
		t.Errorf("the convener's block is placed at %+v and Bob's at %+v — want the same signature "+
			"page and the slot above Bob's", pm, pb)
	}
	// The signature carries the quoted time: the block read is the block signed.
	if atts := p2p.ReadAttestations(mine); atts[1].When != read.Format("2006-01-02 15:04 MST") {
		t.Errorf("the signature is dated %s and the block was read at %s", atts[1].When,
			read.Format("2006-01-02 15:04 MST"))
	}
	// The walk moves on to Carla, and the hop route now calls her as a carry.
	pr, perr := p2p.ContributionProgress(mine, roster)
	if perr != nil || pr.Done != 2 || pr.Complete || !strings.EqualFold(pr.Order[pr.Done].Fingerprint, carla) {
		t.Fatalf("after the convener signs the walk reads %+v (%v), want two signed and Carla next", pr, perr)
	}
	quoteNames(t, f, "Carla")

	// 4. A second press signs nothing more: it is Carla's turn now.
	code, body = f.signTurn(t, "")
	if code != http.StatusConflict || !strings.Contains(body, "Carla’s turn") {
		t.Errorf("a second press answered %d %s, want 409 naming Carla's turn", code, body)
	}
	if got := f.theOneOpenDocument(t); !bytes.Equal(got, mine) {
		t.Error("a second press changed the document")
	}
}

// TestAConvenerNamedFirstSignsTheirOwnTurn — position one, which is the ceremony the setup sheet
// produces by default (`#cerISign` ships checked).
func TestAConvenerNamedFirstSignsTheirOwnTurn(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the fixture sandboxes too; the home guard reads this body
	f := newHopFixture(t, 48*time.Hour)
	rec, convened, err := ceremony.ReadMirror(defaultOutputDir(), f.id, time.Now())
	if err != nil {
		t.Fatalf("setup: the convened ceremony does not read back: %v", err)
	}
	q := ownTurnAsTheWindowSeesIt(t, f, convened, strings.Repeat("2b", 32), rec.Intent, 1, 3)
	if code, body := f.signTurn(t, q.When); code != http.StatusOK {
		t.Fatalf("the convener's own turn, first of three: %d %s", code, body)
	}
	signedAsAHopsResultIs(t, f, convened, 1, "")
	quoteNames(t, f, "Bob")
}

// TestAConvenerNamedFirstSignsADocumentOpenedCold — the same turn after a restart: the document is
// open and nothing marks it as the ceremony's (`document.ceremony` is written at convene and by an
// install, never by an open). The signed result must replace the tab the convener pressed the
// button on, not open beside it.
func TestAConvenerNamedFirstSignsADocumentOpenedCold(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the fixture sandboxes too; the home guard reads this body
	f := newHopFixture(t, 48*time.Hour)
	f.srv.mu.Lock()
	for _, d := range f.srv.docs {
		d.ceremony = ""
	}
	f.srv.mu.Unlock()
	convened := f.theOneOpenDocument(t)
	if code, body := f.signTurn(t, ""); code != http.StatusOK {
		t.Fatalf("the convener's own turn on a document opened cold: %d %s", code, body)
	}
	signedAsAHopsResultIs(t, f, convened, 1, "")
}

// TestTheOwnTurnRouteRefusesADifferentDocumentUnderTheSameRecord — the hop's arrival gate, asked
// before this machine signs: a document that carries the ceremony's record, byte for byte, and is
// not the document the record was written for. While it is unsigned that is the one anchor there is.
func TestTheOwnTurnRouteRefusesADifferentDocumentUnderTheSameRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the fixture sandboxes too; the home guard reads this body
	f := newHopFixture(t, 48*time.Hour)
	rec, _, err := ceremony.ReadMirror(defaultOutputDir(), f.id, time.Now())
	if err != nil {
		t.Fatalf("setup: the convened ceremony does not read back: %v", err)
	}
	other, err := testpdf.Text("a different lease, on worse terms")
	if err != nil {
		t.Fatal(err)
	}
	decoy, err := ceremony.Embed(other, rec)
	if err != nil {
		t.Fatal(err)
	}
	f.srv.mu.Lock()
	for _, d := range f.srv.docs {
		d.data = decoy
	}
	f.srv.mu.Unlock()
	code, body := f.signTurn(t, "")
	if code != http.StatusConflict || !strings.Contains(body, "not the same document") {
		t.Errorf("a different document under the ceremony's record answered %d %s, want 409 "+
			"saying the bytes are not the record's", code, body)
	}
	if got := f.theOneOpenDocument(t); !bytes.Equal(got, decoy) {
		t.Error("the refused press changed the document — it was signed before it was checked")
	}
}

// TestAConvenerNamedLastSignsAndFinishesTheCeremony — `[Bob, me]`: the convener signs last, there
// is nobody to call afterwards, and the proceeding reads as finished.
func TestAConvenerNamedLastSignsAndFinishesTheCeremony(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the fixture sandboxes too; the home guard reads this body
	bobCert, bobKey, err := sign.GenerateIdentity("Bob")
	if err != nil {
		t.Fatal(err)
	}
	bobFP, err := sign.Fingerprint(bobCert)
	if err != nil {
		t.Fatal(err)
	}
	bob := hex.EncodeToString(bobFP)
	f := convenedAs(t, func(me string) []convenePartyRequest {
		return []convenePartyRequest{
			{Fingerprint: bob, Label: "Bob", Signs: true},
			{Fingerprint: me, Label: "Convener", Signs: true},
		}
	})
	rec, convened, err := ceremony.ReadMirror(defaultOutputDir(), f.id, time.Now())
	if err != nil {
		t.Fatalf("setup: the convened ceremony does not read back: %v", err)
	}
	roster := l3RosterFrom(rec.Roster, rosterHashHex(rec), rec.Intent)
	w := httptest.NewRecorder()
	bobSigned, ok := f.srv.buildCoSigned(w, convened, bobCert, bobKey,
		p2p.Attestation{Signer: "Bob", AcceptedPeer: f.me, Intent: rec.Intent, When: time.Now()},
		tinyPNG(t), roster)
	if !ok {
		t.Fatalf("setup: the first party was refused at their own turn: %d %s", w.Code, w.Body.String())
	}
	if _, err := f.srv.installCeremonyResult(f.id, "form.pdf", bobSigned); err != nil {
		t.Fatal(err)
	}
	f.srv.mirrorHop(bobSigned)

	q := ownTurnAsTheWindowSeesIt(t, f, bobSigned, bob, rec.Intent, 2, 2)
	if code, body := f.signTurn(t, q.When); code != http.StatusOK {
		t.Fatalf("the convener's own turn, last of two: %d %s", code, body)
	}
	mine := signedAsAHopsResultIs(t, f, bobSigned, 2, bob)
	if pr, perr := p2p.ContributionProgress(mine, roster); perr != nil || !pr.Complete {
		t.Fatalf("after the last signer signs the walk reads %+v (%v), want complete", pr, perr)
	}
	// Nobody is left to call, and the rail is told the ceremony is finished.
	if code, body := f.quote(t, ""); code != http.StatusConflict || !strings.Contains(body, "nobody left to call") {
		t.Errorf("the quote after the last signature: %d %s, want 409 naming that nobody is left", code, body)
	}
	resp, err := f.c.Get(f.base + "/api/ceremony/next?ceremony=" + f.id)
	if err != nil {
		t.Fatal(err)
	}
	var next ceremonyNextResponse
	derr := json.NewDecoder(resp.Body).Decode(&next)
	resp.Body.Close()
	if derr != nil || (next.State != "complete" && next.State != "ended") {
		t.Errorf("after the last signature the rail is told %+v (%v), want the ceremony finished", next, derr)
	}
}

// TestOnlyTheConvenerSignsThroughTheOwnTurnRoute — a party who did not convene waits to be called
// (D22): holding the ceremony's mirror and its open document entitles them to nothing here.
func TestOnlyTheConvenerSignsThroughTheOwnTurnRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the fixture sandboxes too; the home guard reads this body
	f := newHopFixture(t, 48*time.Hour)
	id, path := foreignCeremony(t)
	if code, body := postForCode(t, f.c, f.csrf, f.base+"/api/open", openRequest{Path: path}); code != http.StatusOK {
		t.Fatalf("open the foreign ceremony's document: %d %s", code, body)
	}
	before := f.openBytes()
	f.id = id
	code, body := f.signTurn(t, "")
	if code != http.StatusForbidden || !strings.Contains(body, "you sign when the convener calls you") {
		t.Errorf("a party who did not convene answered %d %s, want 403 telling them they are called", code, body)
	}
	after := f.openBytes()
	if len(after) != len(before) {
		t.Fatalf("the refused press changed how many documents are open: %d, was %d", len(after), len(before))
	}
	for i := range before {
		if !bytes.Equal(before[i], after[i]) {
			t.Errorf("the refused press changed open document %d", i+1)
		}
	}
}
