package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/sign"
)

// `/pending 869`: the convener's OWN turn, driven through the routes.
//
// What these tests found, stated because the item asked a question rather than named a defect:
//
//   - **The signing door works at a mid-order turn.** `buildCoSigned` — the one door that applies
//     a ceremony signature on this machine — admits a convener the roster names second once the
//     first party has signed, places their block at their roster position, leaves the first
//     signature's bytes untouched, and moves the walk on to the third party.
//   - **No route a window can call reaches that door at the convener's own turn — first, middle or
//     last.** `POST /api/ceremony/hop` resolves its target as `Order[Done]` and refuses when that
//     is this machine; `/api/cosign/sign` and `/api/session/initiate` refuse a convened document
//     because the window sends no invitation. The one caller that gets a signing convener through
//     is a harness posting the `invitation` form field to `/api/session/initiate`, where the
//     convener's own block rides on the dial to the NEXT party.
//
// So the refusals asserted here are each correct as a rule — a dial to yourself is not a hop, and
// a ceremony document is not co-signed outside its ceremony — and the gap is the route that is
// missing between them. When that route is built, the `mine` assertions below are the ones that
// change, and they should change on purpose.

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
// signing call answers, and returns nothing: every answer here is a refusal or a statement.
func ownTurnAsTheWindowSeesIt(t *testing.T, f *hopFixture, pdf []byte, peer, intent string, position, of int) {
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
}

// TestAConvenerNamedSecondAtTheirOwnTurn convenes `[Bob, me, Carla]`, puts Bob's real signature on
// the document, and does what the rail tells the convener to do.
func TestAConvenerNamedSecondAtTheirOwnTurn(t *testing.T) {
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

	// SETUP: Bob's turn, through the signing door, installed and mirrored as a hop's result is.
	w := httptest.NewRecorder()
	bobSigned, ok := f.srv.buildCoSigned(w, convened, bobCert, bobKey,
		p2p.Attestation{Signer: "Bob", AcceptedPeer: f.me, Intent: rec.Intent, When: time.Now()},
		tinyPNG(t), roster)
	if !ok {
		t.Fatalf("setup: the first party was refused at their own turn: %d %s", w.Code, w.Body.String())
	}
	install := func(data []byte) {
		t.Helper()
		if _, err := f.srv.installCeremonyResult(f.id, "form.pdf", data); err != nil {
			t.Fatal(err)
		}
		f.srv.mirrorHop(data)
	}
	install(bobSigned)

	// 1. What the window is offered: the turn is this machine's, and every signing call refuses.
	ownTurnAsTheWindowSeesIt(t, f, bobSigned, carla, rec.Intent, 2, 3)

	// 2. The signing door itself, with this machine's own identity, over the document Bob signed.
	cert, key, err := identity(f.srv.unlockedVault())
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	mine, ok := f.srv.buildCoSigned(w, bobSigned, cert, key,
		p2p.Attestation{Signer: "Nib User", AcceptedPeer: carla, Intent: rec.Intent, When: time.Now()},
		tinyPNG(t), roster)
	if !ok {
		t.Fatalf("the signing door refuses a convener at their own mid-order turn: %d %s",
			w.Code, w.Body.String())
	}
	// Bob's signature survives: the convener's is an incremental update over Bob's exact bytes.
	if !bytes.HasPrefix(mine, bobSigned) {
		t.Error("the convener's signature rewrote the document Bob signed instead of appending to it")
	}
	st := sign.Verify(mine)
	if st.State != sign.Valid || len(st.Signers) != 2 {
		t.Fatalf("after the convener signs: state %q with %d signer(s), want valid with 2",
			st.State, len(st.Signers))
	}
	atts := p2p.ReadAttestations(mine)
	if len(atts) != 2 || !strings.EqualFold(atts[1].Fingerprint, f.me) {
		t.Fatalf("the second signature is not this machine's: %+v", atts)
	}
	// It accepts its signing predecessor — Bob — and not the party the caller named.
	if !strings.EqualFold(atts[1].AcceptedPeer, bob) || !atts[1].Valid {
		t.Errorf("the convener's block accepts %q (valid=%v), want Bob, who signed before them",
			atts[1].AcceptedPeer, atts[1].Valid)
	}
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
	// The walk moves on to Carla.
	pr, perr := p2p.ContributionProgress(mine, roster)
	if perr != nil || pr.Done != 2 || pr.Complete || !strings.EqualFold(pr.Order[pr.Done].Fingerprint, carla) {
		t.Fatalf("after the convener signs the walk reads %+v (%v), want two signed and Carla next", pr, perr)
	}

	// 3. And once that signature is on the open document, the hop route calls Carla as a carry.
	install(mine)
	code, body := f.quote(t, "")
	var q ceremonyHopQuote
	if code != http.StatusOK || json.Unmarshal([]byte(body), &q) != nil {
		t.Fatalf("the quote after the convener signed: %d %s", code, body)
	}
	if q.Mine || q.Party != "Carla" || q.Contributes {
		t.Errorf("after the convener signed the quote is %s, want a call to Carla at which this "+
			"machine signs nothing", body)
	}
}

// TestAConvenerNamedFirstIsOfferedNoMoreAtTheirOwnTurn — the same answers at position one, which
// is the ceremony the setup sheet produces by default (`#cerISign` ships checked).
func TestAConvenerNamedFirstIsOfferedNoMoreAtTheirOwnTurn(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the fixture sandboxes too; the home guard reads this body
	f := newHopFixture(t, 48*time.Hour)
	rec, convened, err := ceremony.ReadMirror(defaultOutputDir(), f.id, time.Now())
	if err != nil {
		t.Fatalf("setup: the convened ceremony does not read back: %v", err)
	}
	ownTurnAsTheWindowSeesIt(t, f, convened, strings.Repeat("2b", 32), rec.Intent, 1, 3)
}
