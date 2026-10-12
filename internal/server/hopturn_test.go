package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/p2p"
	"nib/internal/testpdf"
)

// `/pending 517` (i): a convener who names themselves somewhere other than first in the roster.
//
// `canonicalRoster` prepends the convener only when the client did not name them, and
// `p2p.SigningOrder` does no promotion, so `[Bob, me, Carla]` puts the convener SECOND. Under D22's
// hub the convener still dials every hop, so at hop 1 they are a pure carrier — and `carries` used
// to answer "contribute" for any signer whose signature was not on the document yet, so the hop
// route demanded a signature block for a hop at which this machine signs nothing.

// hopFixtureConvenerSecond convenes over the real routes with this machine second of three.
func hopFixtureConvenerSecond(t *testing.T) *hopFixture {
	t.Helper()
	return hopFixtureConvenerSecondAfter(t, strings.Repeat("2b", 32))
}

// hopFixtureConvenerSecondAfter is the same ceremony with the FIRST party's fingerprint supplied,
// so a test that holds that party's key can put their real signature on the document.
func hopFixtureConvenerSecondAfter(t *testing.T, bob string) *hopFixture {
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
		Roster: []convenePartyRequest{
			{Fingerprint: bob, Label: "Bob", Signs: true},
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
	return &hopFixture{c: c, csrf: csrf, base: ts.URL, id: out.Ceremony, me: me, pdfPath: pdfPath, srv: srv}
}

// TestAConvenerNamedSecondCarriesTheFirstHop drives the real quote route and the dial's own door.
func TestAConvenerNamedSecondCarriesTheFirstHop(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the fixture sandboxes too; the home guard reads this body
	f := hopFixtureConvenerSecond(t)
	code, body := f.quote(t, "")
	if code != http.StatusOK {
		t.Fatalf("setup: the quote was refused %d: %s", code, body)
	}
	var q ceremonyHopQuote
	if err := json.Unmarshal([]byte(body), &q); err != nil {
		t.Fatal(err)
	}
	// SETUP: the roster order survived convene, so this is a hop that gets dialled, to Bob.
	if q.Mine || q.Party != "Bob" {
		t.Fatalf("setup: want a dialled hop to Bob, got %s — the convener is not second and "+
			"nothing below is about the shape it claims", body)
	}
	if q.Contributes {
		t.Errorf("the quote says this machine signs at a hop it is dialling for Bob: %s", body)
	}

	// THE DIAL'S ANSWER, asked of the door `runHopDial` asks, over the record this machine holds
	// and the document the convene produced. The route itself cannot be driven past this decision
	// in one process: a carry goes on to dial a party who is not there.
	rec, doc, err := ceremony.ReadMirror(defaultOutputDir(), f.id, time.Now())
	if err != nil {
		t.Fatalf("setup: the convened ceremony does not read back: %v", err)
	}
	cer := &ceremonyID{inv: ceremony.Invitation{
		ID: rec.ID, Roster: rec.Roster, Intent: rec.Intent, RosterHash: rosterHashHex(rec),
		ConvenerFingerprint: f.me,
	}}
	carrying, cerr := cer.carries(f.me, doc)
	if cerr != nil {
		t.Fatalf("carries: %v", cerr)
	}
	if !carrying {
		t.Errorf("a convener second in the signing order does not carry the first hop — the dial " +
			"demands their signature block for Bob's turn: 400 \"this hop needs your signature " +
			"block and none was sent\" on every hop of such a ceremony")
	}
	// The party being dialled contributes, or nobody signs at this hop at all.
	bob := strings.Repeat("2b", 32)
	if got, berr := cer.carries(bob, doc); berr != nil || got {
		t.Errorf("the party whose turn it is carries (%v, %v)", got, berr)
	}
	// **A party who is early and is NOT the convener still takes the contribution path**, where
	// the turn rule's one door refuses them by name. Tier 4 -n 4 drives exactly this (instance 3
	// initiating over the unsigned convened document) and reads the refusal; answering "carry"
	// for every party whose turn has not come is the remedy that tier refuted.
	carla := strings.Repeat("3c", 32)
	if got, kerr := cer.carries(carla, doc); kerr != nil || got {
		t.Errorf("a party third in the order, who is not the convener, carries an unsigned "+
			"document (%v, %v) — it would be sent on with no signature and no refusal", got, kerr)
	}
	if aerr := p2p.AdmitContribution(doc, cer.l3Roster(), carla); !errors.Is(aerr, p2p.ErrNotYourTurn) {
		t.Errorf("the contribution path admits a party who is early: %v, want %v", aerr, p2p.ErrNotYourTurn)
	}
}

// TestWhoContributesAtEveryHopOfAFourPartyOrder walks every hop of a four-party signing order
// with the convener in each position, and states who signs at it.
//
// The rule, as D22 and `Progress` document it: the convener is at one end of every hop and
// `Order[Done]` is whose turn it is. So at the hop for `Order[d]` exactly ONE block is
// contributed, by `Order[d]`; the convener carries unless that party is the convener themselves;
// a party whose signature is already on the document carries; and a party who is early and is not
// the convener does NOT carry — they are sent down the contribution path, which refuses them.
func TestWhoContributesAtEveryHopOfAFourPartyOrder(t *testing.T) {
	fps := []string{strings.Repeat("a1", 32), strings.Repeat("b2", 32), strings.Repeat("c3", 32), strings.Repeat("d4", 32)}
	order := make([]p2p.RosterEntry, len(fps))
	for i, fp := range fps {
		order[i] = p2p.RosterEntry{Fingerprint: fp, Signs: true}
	}
	for conv := range fps {
		for done := range fps {
			var contributors []int
			for i, fp := range fps {
				carries := hopCarries(order, done, fp, i == conv)
				want := i < done || (i == conv && i > done)
				if carries != want {
					t.Errorf("convener at %d, hop for party %d: party %d carries=%v, want %v",
						conv, done, i, carries, want)
				}
				// The two ends of this hop: the convener and the party whose turn it is.
				if (i == conv || i == done) && !carries {
					contributors = append(contributors, i)
				}
			}
			if len(contributors) != 1 || contributors[0] != done {
				t.Errorf("convener at %d, hop for party %d: the hop's two ends contribute %v, "+
					"want exactly [%d] — one block per hop, from the party whose turn it is",
					conv, done, contributors, done)
			}
		}
	}
	// A fingerprint the order does not name is not a carrier by this rule (the roster test in
	// `carryDecision` answers for non-members and non-signers before it is asked).
	if hopCarries(order, 1, strings.Repeat("ee", 32), true) {
		t.Error("a party the signing order does not name carries")
	}
}
