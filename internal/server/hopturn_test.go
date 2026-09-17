package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// `/pending 517` clause (i): the hop quote's signing block, and why it could never be quoted.
//
// # What the entry said, and what probing it found
//
// The entry read `handleCeremonyHopQuote` and reported the attestation block below `:265` as
// UNREACHABLE — the function returns early when the next party is this machine, and
// `pr.Order[pr.Done]` is that same next party, so the loop's `i == pr.Done` could never be true.
// That is correct, and it is one line of a bigger fact: the loop was a SECOND implementation of the
// question `carries` answers inside the dial, with a different predicate, and the two disagreed.
//
// **Where they disagreed is a ceremony that does not work at all.** `canonicalRoster` prepends a
// signing convener at position 0 only when the client did not name them, and says in its own words
// that "a caller who wants another position includes themselves in the roster". `SigningOrder` does
// no promotion. So a convener who names themselves SECOND is second — and under D22's hub they are
// at one end of every hop, so at hop 1 they are a pure carrier. The quote said
// `{"mine":false,"contributes":false}` (no block to render) and `carries` said "contribute" (a
// block is required), and the dial answered **400 "this hop needs your signature block and none was
// sent"**. Measured through these very routes before the fix. Every hop of such a ceremony failed.
//
// The fix is at `carries` — a machine dialling a hop carries, because the party it is dialling is
// the one whose turn it is — and the quote's re-derivation went with it, which is what makes the
// deleted block provably dead rather than accidentally dead.

// hopFixtureConvenerSecond convenes over the real routes with the convener SECOND in the signing
// order, which is the one shape where the two predicates differed.
func hopFixtureConvenerSecond(t *testing.T) *hopFixture {
	t.Helper()
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: pdfPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}
	me := myFingerprint(t, c, ts.URL)
	code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/convene", conveneRequest{
		// Bob first, this machine second. `canonicalRoster` prepends the convener only when the
		// roster does not already name them, so naming them here is what puts them at index 1.
		Roster: []convenePartyRequest{
			{Fingerprint: strings.Repeat("2b", 32), Label: "Bob", Signs: true},
			{Fingerprint: me, Label: "Convener", Signs: true},
			{Fingerprint: strings.Repeat("3c", 32), Label: "Carla", Signs: true},
		},
		Intent: "We agree to co-sign the lease", ConvenerSigns: true,
		Expires: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
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

// TestAPartyWhoseTurnHasNotComeCarries — the predicate, at the one door that owns it.
//
// `carries` answered `i < pr.Done` — carry only once the document already holds my signature — so a
// signer whose turn has not come YET was told to contribute. `Progress`'s own doc says
// `Order[Done]` IS whose turn it is, and that a caller wanting per-party state derives it from
// those two fields; `i != pr.Done` is that reading.
func TestAPartyWhoseTurnHasNotComeCarries(t *testing.T) {
	first := strings.Repeat("b0", 32)
	later := strings.Repeat("a1", 32)
	cer := &ceremonyID{inv: ceremony.Invitation{Roster: []ceremony.Party{
		{Fingerprint: first, Label: "Bob", Signs: true},
		{Fingerprint: later, Label: "Alice", Signs: true},
	}}}
	carries := func(fp string) bool {
		t.Helper()
		got, err := cer.carries(fp, nil)
		if err != nil {
			t.Fatalf("carries(%s…): %v", fp[:8], err)
		}
		return got
	}

	// SETUP: the party whose turn it IS still contributes. Without this, "everyone carries" would
	// satisfy the assertion below and the chain would simply never advance.
	if carries(first) {
		t.Fatal("setup: the party whose turn it is carries, so the case below cannot distinguish " +
			"a turn rule from a predicate that made everybody a carrier")
	}

	// THE CASE: an unsigned document, so Done is 0 and it is Bob's turn. Alice is a signing party
	// at index 1 and contributes at HER hop, not at his.
	if !carries(later) {
		t.Error("a signing party whose turn has NOT come reports that it contributes at this hop " +
			"— so a convener who placed themselves second in the signing order signs out of turn " +
			"at hop 1, and the hop route demands a signature block for a hop that needs none: " +
			"measured as 400 \"this hop needs your signature block and none was sent\" on every " +
			"hop of such a ceremony")
	}
}

// TestADialledHopQuotesNoBlockForThisMachine — the deleted block, asserted as a property.
//
// Driven on the convener-second ceremony because that is the shape that came CLOSEST to reaching
// the block: every other ceremony has the convener at `Order[0]`, where the quote returns on `mine`
// long before. If a re-derivation ever comes back — `i >= pr.Done`, say, matching what `carries`
// used to answer — this is where it shows up.
func TestADialledHopQuotesNoBlockForThisMachine(t *testing.T) {
	f := hopFixtureConvenerSecond(t)
	code, body := f.quote(t, "")
	if code != http.StatusOK {
		t.Fatalf("setup: the quote was refused %d: %s — nothing below is being tested", code, body)
	}
	var q ceremonyHopQuote
	if err := json.Unmarshal([]byte(body), &q); err != nil {
		t.Fatal(err)
	}
	// SETUP: this really is a hop that gets DIALLED. A quote answering `mine` would take the early
	// branch and satisfy everything below for the wrong reason.
	if q.Mine {
		t.Fatalf("setup: the quote reports this machine's own turn (%s), so this is not a dialled "+
			"hop and the assertion below is about the other branch", body)
	}
	if q.Party != "Bob" {
		t.Fatalf("setup: the quote names %q and the first signer is Bob — the roster order did "+
			"not survive convene, so the convener is not second and this fixture is not the "+
			"shape it claims: %s", q.Party, body)
	}
	if q.Contributes {
		t.Errorf("the quote says this machine SIGNS at a hop it is dialling: %s. The party being "+
			"called is the one whose turn it is, so the dialer carries — and a true answer here "+
			"sends the client to `renderAttestation` for a block the dial will not use", body)
	}
	// And no block travels with it. Asserted on the wire rather than on the struct, because the
	// defect this replaces was a route minting an attestation nobody could ever be quoted.
	for _, gone := range []string{`"lines"`, `"rect"`, `"when"`} {
		if strings.Contains(body, gone) {
			t.Errorf("the quote carries %s: %s — a dialled hop has no block to draw, and a route "+
				"that returns one has an attestation mint on a path that never signs", gone, body)
		}
	}
}
