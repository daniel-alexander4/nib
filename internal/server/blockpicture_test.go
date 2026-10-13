package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// The server draws the signature block (ADR-158, /pending 367). These are S-C's route-level rows:
// C1 a route signs a drawn block with no picture sent, C2 a picture sent is refused, C6 a block that
// cannot be drawn or does not fit is refused at the quote, C7 the quoted picture is the one signed.

// blockSamples is a block picture as the signing library embeds it: 8-bit RGB, row by row.
func blockSamples(t *testing.T, pngB64 string) (w, h int, rgb []byte) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(pngB64)
	if err != nil || len(raw) == 0 {
		t.Fatalf("the quote's picture is not base64 of anything (%d bytes, %v)", len(raw), err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the quote's picture is not a PNG: %v", err)
	}
	return img.Bounds().Dx(), img.Bounds().Dy(), samplesOf(img)
}

func samplesOf(img image.Image) []byte {
	b := img.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*3)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			out = append(out, byte(r>>8), byte(g>>8), byte(bl>>8))
		}
	}
	return out
}

// cosignSignFormAt is cosignSignForm with the quote's time echoed, as the window posts it.
func cosignSignFormAt(t *testing.T, fp, intent, when string, picture []byte) (*bytes.Buffer, string) {
	t.Helper()
	pdf, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	pj, _ := json.Marshal(cosignParams{Fingerprint: fp, Intent: intent, When: when})
	mw.WriteField("params", string(pj))
	if picture != nil {
		aw, _ := mw.CreateFormFile("appearance", "attestation.png")
		aw.Write(picture)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// C7 and C1 — the picture a quote shows is, sample for sample, the picture in the signed document,
// and the signing request carried no picture at all.
func TestTheQuotedPictureIsThePictureSigned(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	fp := strings.Repeat("ab", 32)
	pinPeer(t, c, csrf, ts.URL, fp)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", jsonBody(openRequest{Path: pdfPath}))
	resp.Body.Close()

	const intent = "I agree to the lease of 14 Harbour Road as amended on 3 October."
	// A time well away from now, inside the skew bound: a route that drew with its own clock would
	// draw a different last line.
	read := time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339)
	qr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/cosign/quote", "application/json",
		jsonBody(cosignParams{Fingerprint: fp, Intent: intent, When: read}))
	var q cosignQuote
	json.NewDecoder(qr.Body).Decode(&q)
	qr.Body.Close()
	if qr.StatusCode != http.StatusOK || q.When != read || len(q.Lines) < 5 {
		t.Fatalf("setup: the quote answered %d with when=%q and %d lines", qr.StatusCode, q.When, len(q.Lines))
	}
	w, h, quoted := blockSamples(t, q.Image)
	rect := p2p.NominalBlockRect()
	if w != int(rect[2]-rect[0])*3 || h != int(rect[3]-rect[1])*3 {
		t.Fatalf("the quoted picture is %dx%d, which is not the block's shape", w, h)
	}
	dark := 0
	for i := 0; i < len(quoted); i += 3 {
		if quoted[i] < 128 {
			dark++
		}
	}
	if dark < 3000 {
		t.Fatalf("the quoted picture has %d dark samples: it shows no block", dark)
	}

	body, ct := cosignSignFormAt(t, fp, intent, q.When, nil)
	sr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/cosign/sign", ct, body)
	signed, _ := io.ReadAll(sr.Body)
	sr.Body.Close()
	if sr.StatusCode != http.StatusOK {
		t.Fatalf("signing with no picture sent answered %d: %s", sr.StatusCode, signed)
	}
	if st := sign.Verify(signed); st.State != sign.Valid || len(st.Signers) != 1 {
		t.Fatalf("setup: the signed document verifies %q with %d signer(s)", st.State, len(st.Signers))
	}
	pics := testpdf.PicturesIn(signed)
	if len(pics) == 0 {
		t.Fatal("the signed document carries no picture: the signature is invisible, with no block on the page")
	}
	got := pics[len(pics)-1]
	if got.W != w || got.H != h || !bytes.Equal(got.RGB, quoted) {
		t.Errorf("the block embedded in the signed document (%dx%d) is not the picture the quote showed "+
			"(%dx%d): a party can be shown one block and sign another", got.W, got.H, w, h)
	}

	// The control: a different time is a different block, so the equality above is not two blanks.
	body2, ct2 := cosignSignFormAt(t, fp, intent, time.Now().Add(-2*time.Hour).UTC().Format(time.RFC3339), nil)
	sr2 := write(t, c, csrf, http.MethodPost, ts.URL+"/api/cosign/sign", ct2, body2)
	other, _ := io.ReadAll(sr2.Body)
	sr2.Body.Close()
	if p2 := testpdf.PicturesIn(other); len(p2) == 0 || bytes.Equal(p2[len(p2)-1].RGB, quoted) {
		t.Error("a signature made at another time carries the same picture: the comparison cannot tell two blocks apart")
	}
}

// C2 — each of the five signing routes refuses a request that carries a picture of the block, in one
// sentence, before it does anything else.
func TestEverySigningRouteRefusesAPictureOfTheBlock(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	fp := strings.Repeat("ab", 32)
	pinPeer(t, c, csrf, ts.URL, fp)
	tiny := base64.StdEncoding.EncodeToString(tinyPNG(t))

	multipartWith := func(route string) (int, string) {
		body, ct := cosignSignFormAt(t, fp, "I agree to sign this document.", "", tinyPNG(t))
		resp := write(t, c, csrf, http.MethodPost, ts.URL+route, ct, body)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	rows := []struct {
		route string
		send  func() (int, string)
	}{
		{"/api/cosign/sign", func() (int, string) { return multipartWith("/api/cosign/sign") }},
		{"/api/session/initiate", func() (int, string) { return multipartWith("/api/session/initiate") }},
		{"/api/session/respond", func() (int, string) {
			return postForCode(t, c, csrf, ts.URL+"/api/session/respond",
				map[string]any{"id": "x", "accept": true, "intent": "ok", "appearance": tiny})
		}},
		{"/api/ceremony/hop", func() (int, string) {
			return postForCode(t, c, csrf, ts.URL+"/api/ceremony/hop", ceremonyHopRequest{Ceremony: "c", Appearance: tiny})
		}},
		{"/api/ceremony/sign-turn", func() (int, string) {
			return postForCode(t, c, csrf, ts.URL+"/api/ceremony/sign-turn", ceremonySignTurnRequest{Ceremony: "c", Appearance: tiny})
		}},
	}
	for _, row := range rows {
		code, body := row.send()
		if code != http.StatusBadRequest || !strings.Contains(body, "does not accept a picture") {
			t.Errorf("%s answered a request carrying a picture of the block with %d %s — want 400 and the "+
				"sentence saying Nib draws the block itself", row.route, code, body)
		}
		if strings.HasPrefix(body, "%PDF") {
			t.Errorf("%s SIGNED a request that carried a picture", row.route)
		}
	}
	// The control: without the picture the same request is not refused for it.
	body, ct := cosignSignFormAt(t, fp, "I agree to sign this document.", "", nil)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/cosign/sign", ct, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the same co-sign with no picture answered %d: the refusal above is not about the picture", resp.StatusCode)
	}
}

// C6 and C4, at the quote — a block that does not fit in the face it is drawn in, or holds a
// character no face has, is refused where the words are typed and never reaches a signature.
func TestABlockThatCannotBeDrawnIsRefusedAtTheQuoteAndTheSignature(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	fp := strings.Repeat("ab", 32)
	pinPeer(t, c, csrf, ts.URL, fp)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", jsonBody(openRequest{Path: pdfPath}))
	resp.Body.Close()

	// 72 CJK characters: three lines by the Helvetica estimate this used to be wrapped by, and more
	// than the frame holds in the face that draws them.
	wide := strings.Repeat("私たちは株式譲渡契約に同意します。", 9)
	for _, row := range []struct{ name, intent, want string }{
		{"a recital too long in the face drawn", wide, "lines"},
		{"a character no typeface has", "I agree  to this", "U+E000"},
		{"a line break", "I agree\nto this", "line break"},
	} {
		code, body := postForCode(t, c, csrf, ts.URL+"/api/cosign/quote", cosignParams{Fingerprint: fp, Intent: row.intent})
		if code != http.StatusBadRequest || !strings.Contains(body, row.want) {
			t.Errorf("%s: the quote answered %d %s — want 400 naming %q", row.name, code, body, row.want)
		}
		form, ct := cosignSignFormAt(t, fp, row.intent, "", nil)
		sr := write(t, c, csrf, http.MethodPost, ts.URL+"/api/cosign/sign", ct, form)
		b, _ := io.ReadAll(sr.Body)
		sr.Body.Close()
		if sr.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), row.want) {
			t.Errorf("%s: the signing route answered %d %.120s — want the same refusal, and no document", row.name, sr.StatusCode, b)
		}
	}
	// The control: a recital in the same script that fits is quoted, with its picture.
	code, body := postForCode(t, c, csrf, ts.URL+"/api/cosign/quote", cosignParams{Fingerprint: fp, Intent: "私たちは同意します。"})
	if code != http.StatusOK || !strings.Contains(body, `"image":"`) {
		t.Errorf("a short CJK recital answered %d %.160s — want a quote with a picture", code, body)
	}

	// A pinned label is on every block signed with that peer, so it is asked where it is typed.
	code, body = postForCode(t, c, csrf, ts.URL+"/api/peers/pin", pinPeerRequest{Fingerprint: strings.Repeat("cd", 32), Label: "Bob"})
	if code != http.StatusBadRequest || !strings.Contains(body, "U+E000") || !strings.Contains(body, "label") {
		t.Errorf("pinning a label with a character no typeface has answered %d %s", code, body)
	}
	code, body = postForCode(t, c, csrf, ts.URL+"/api/peers/pin", pinPeerRequest{Fingerprint: strings.Repeat("cd", 32), Label: "محمد العربي"})
	if code != http.StatusOK {
		t.Errorf("pinning an Arabic label answered %d %s", code, body)
	}
}

// An acceptance signs the intent IT carries, which need not be the one a quote measured — so the
// answer is asked the block's two questions before it is handed to the exchange.
func TestAnAcceptanceWhoseBlockCannotBeDrawnIsRefusedBeforeItIsDelivered(t *testing.T) {
	ts, srv := startServerWith(t)
	authedClient(t, ts) // unlocks the vault the answer reads the identity and the pins from
	if srv.vault == nil {
		t.Fatal("setup: the vault is not unlocked")
	}
	peer := strings.Repeat("ab", 32)
	p := &pendingReq{view: pendingView{ID: "r1", Fingerprint: peer}, signs: true}
	for _, row := range []struct {
		name, intent, want string
		refused            bool
	}{
		{"an ordinary recital", "I agree to sign this document.", "", false},
		{"a character no typeface has", "I agree ", "U+E000", true},
		{"a recital past the frame", strings.Repeat("私たちは株式譲渡契約に同意します。", 9), "lines", true},
	} {
		w := httptest.NewRecorder()
		got := srv.refusedAnswerBlock(w, srv.vault, p, p2p.Roster{}, row.intent)
		if got != row.refused {
			t.Errorf("%s: refused=%v, want %v (%d %s)", row.name, got, row.refused, w.Code, w.Body.String())
		}
		if row.refused && (w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), row.want)) {
			t.Errorf("%s: answered %d %s — want 400 naming %q", row.name, w.Code, w.Body.String(), row.want)
		}
	}
}
