package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nib/internal/p2p"
)

// /pending 286 — the pinned-peer label had no bound, and the block clips silently.
//
// The signature block draws each line with `ctx.fillText(ln, pad, ...)` and **no
// `maxWidth`**, and nothing wraps — so a line too wide for the block is simply cut off
// mid-word in the rendered PDF, with no refusal anywhere and nothing on screen to say it
// happened. Three of the four user-supplied strings that reach a block line were already
// bounded through p2p's one door (Intent, Capacity, Signer); this one was not, and its
// only limit was the pin route's 64 KiB body read.
//
// That makes it the ordinary two-party co-sign path — the product's most-used signing
// flow — quietly producing a document whose block says something other than what the user
// typed.
//
// The wrapping half of the item is deliberately NOT here: changing AppearanceLines from
// "one entry per line" to "wrap and recompute" retroactively loosens three shipped
// refusals and changes what every existing block renders as. That is a design change and
// stays filed.

const testFP = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"

func pinLabel(t *testing.T, ts *httptest.Server, c *http.Client, csrf, label string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(pinPeerRequest{Fingerprint: testFP, Label: label})
	return write(t, c, csrf, http.MethodPost, ts.URL+"/api/peers/pin", "application/json", bytes.NewReader(body))
}

// errBody reads the server's {"error": ...} message.
func errBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	return e.Error
}

func TestAPinLabelTooWideForTheBlockIsRefused(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	// Wide, not merely long: "M" is the widest glyph at these metrics and the item's whole
	// point is that a rune COUNT is not a width. A 400-rune label of "i" might fit where
	// 200 of "M" does not.
	wide := strings.Repeat("M", 400)
	resp := pinLabel(t, ts, c, csrf, wide)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pinning a 400-M label answered %d, want 400 — the label is far wider than a block line, and the block would clip it with no refusal and nothing on screen to say so", resp.StatusCode)
	}

	// The refusal has to quote a number the user can act on, or "too long" is advice they
	// cannot follow. Same contract the convene route's label refusal already keeps.
	msg := errBody(t, resp)
	if !strings.Contains(msg, "at most") {
		t.Errorf("the refusal does not tell the user how much fits: %q", msg)
	}
}

func TestAnOrdinaryPinLabelIsStillAccepted(t *testing.T) {
	// The other direction, and it is what stops the guard above from being satisfied by a
	// route that refuses everything. A real counterparty name must still pin.
	//
	// **Measured, and the bound is TIGHT**: the block line is 272 units and the rendered
	// suffix `  [aabb ccdd eeff 0011...]` takes a large share of it, leaving 18 runes of
	// "M", 26 of a mixed-width name and 68 of "i". So a person's name fits and a firm's
	// often will not — "Wendy Okonkwo — Okonkwo & Reyes LLP" is 35 runes and is refused.
	//
	// That is the honest consequence of refusing rather than clipping, and it is why the
	// WRAPPING half of /pending 286 is the half worth having: today the alternative to
	// this refusal is not a longer label, it is a label silently cut off in the signed
	// document. The refusal at least says so at the moment the user types it.
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	resp := pinLabel(t, ts, c, csrf, "Wendy Okonkwo")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("an ordinary counterparty label was refused with %d — the bound is too tight to pin a real peer", resp.StatusCode)
	}
}

// The bound is measured against the line the block ACTUALLY renders, fingerprint included.
// Bounding the label alone would pass one that then pushes the fingerprint off the block,
// and the fingerprint is the half that identifies the counterparty.
func TestTheBoundCountsTheFingerprintBesideTheLabel(t *testing.T) {
	max := p2p.MaxAcceptsRunes(strings.Repeat("M", 400), testFP)
	if max <= 0 {
		t.Fatalf("MaxAcceptsRunes returned %d — no label of any length would fit, so the refusal is unusable", max)
	}
	if !p2p.AcceptsFitsBlock(strings.Repeat("M", max), testFP) {
		t.Errorf("the quoted maximum of %d M's does not itself fit — the number in the refusal is wrong, and a user who cuts to it is refused again", max)
	}
	if p2p.AcceptsFitsBlock(strings.Repeat("M", max+1), testFP) {
		t.Errorf("one rune past the quoted maximum of %d still fits — the bound is not tight, so it is not the number to quote", max)
	}
	// And it is genuinely tighter than the label-only rule, which is the reason this pair
	// exists at all rather than reusing LabelFitsBlock.
	if lbl := p2p.MaxLabelRunes(strings.Repeat("M", 400)); max >= lbl {
		t.Errorf("Accepts allows %d runes and Signer allows %d — Accepts renders a fingerprint after the label, so it must allow FEWER; if it does not, the suffix is not in the measurement", max, lbl)
	}
}
