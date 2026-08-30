package p2p

import (
	"bytes"

	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// P07.S06 — a block goes on the signature page its party's ROSTER POSITION allocates.
//
// The defect these pin was named by the code that created it. `sigpages.go` says of
// PrepareCeremonyDocument: "It allocates pages; it does not place blocks on them. stackPlacement
// puts every block on the page it is handed, indexed by the global signer count, so a ceremony of
// nine still has block 8 off the page." Measured on the shipped arithmetic — y0 = 40 + 96i,
// height 84 — block 8 topped out at 892 on an 842 pt A4 page, and did so silently.

// ceremonyFixture builds a real convened document for n signing parties and returns it with the
// roster, so these tests measure placement on the pages the allocator actually produced rather
// than on a page count somebody typed.
func ceremonyFixture(t *testing.T, n int) ([]byte, Roster) {
	t.Helper()
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	var r Roster
	for i := 0; i < n; i++ {
		p := l3Identity(t, fmt.Sprintf("Party %d", i))
		r.Entries = append(r.Entries, RosterEntry{Fingerprint: p.fp, Signs: true})
	}
	conv := l3Identity(t, "Convener")
	doc, err := PrepareCeremonyDocument(base, CeremonyID{1, 2, 3}, []byte(conv.fp[:8]), n)
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the allocator really produced more than one signature page at the sizes that matter,
	// or "the blocks are spread across pages" is true of a document with one page to spread over.
	if n > blocksPerPage && SignaturePagesFor(n) < 2 {
		t.Fatalf("setup: %d signers allocated %d signature page(s); this fixture cannot show a "+
			"block landing on a page other than the last", n, SignaturePagesFor(n))
	}
	return doc, r
}

// TestEveryBlockLandsOnItsOwnAllocatedPageAndInsideIt is the slice's whole point, driven at N=9.
//
// Two assertions and they fail differently on purpose: the PAGE assertion catches every block
// piling onto the last signature page (signature page 1 receiving nothing), and the BOX assertion
// catches the block that climbs past the media box. Before this slice the first was wrong for
// eight of nine parties and the second for one of them.
func TestEveryBlockLandsOnItsOwnAllocatedPageAndInsideIt(t *testing.T) {
	const n = 9
	doc, r := ceremonyFixture(t, n)
	total, err := pdfops.PageCount(doc)
	if err != nil {
		t.Fatal(err)
	}
	pages := SignaturePagesFor(n)
	first := total - pages + 1

	seen := map[int]int{}
	for i, e := range SigningOrder(r) {
		p, err := PlacementFor(doc, r, e.Fingerprint)
		if err != nil {
			t.Fatalf("party %d of %d has no placement at all: %v — a ceremony that allocated %d "+
				"signature page(s) must have somewhere to put every obliged signer's block",
				i, n, err, pages)
		}
		wantPage := first + i/blocksPerPage
		if p.Page != wantPage {
			t.Errorf("party %d's block is on page %d, want %d: the index is the party's ROSTER "+
				"POSITION on the page it allocates, not a running count on the last page",
				i, p.Page, wantPage)
		}
		seen[p.Page]++

		llx, lly, urx, ury, err := pdfops.PageBox(doc, p.Page)
		if err != nil {
			t.Fatal(err)
		}
		if p.Rect[0] < llx || p.Rect[1] < lly || p.Rect[2] > urx || p.Rect[3] > ury {
			t.Errorf("party %d's block is (%.0f,%.0f)-(%.0f,%.0f) on a page boxed "+
				"(%.0f,%.0f)-(%.0f,%.0f): it is off the page, and pdfcpu would CLAMP it rather "+
				"than say so (D25)", i, p.Rect[0], p.Rect[1], p.Rect[2], p.Rect[3],
				llx, lly, urx, ury)
		}
	}

	// **Every allocated page carries blocks.** Without this, placing all nine on one page and
	// allocating two still satisfies "each block is inside its page" — the second page is simply
	// blank, which is the appended-blank-page harm the allocator's own clause names.
	for pg := first; pg <= total; pg++ {
		if seen[pg] == 0 {
			t.Errorf("signature page %d received no block at all, while %v did: the allocator "+
				"reserved a page nothing was placed on", pg, seen)
		}
		if seen[pg] > blocksPerPage {
			t.Errorf("signature page %d carries %d blocks, more than D25's %d per page",
				pg, seen[pg], blocksPerPage)
		}
	}
}

// TestTheBlockIndexIsTheRosterPositionNotASignatureCount — the plan names three ways the count is
// wrong, and this pins the one that is checkable without forging a signature: a NON-SIGNING party
// holds no position, so the parties after them do not shift.
//
// A count of signatures on the file would give the same answer as a roster walk right up until
// something on the file differs from the ceremony — which is exactly when it matters.
func TestTheBlockIndexIsTheRosterPositionNotASignatureCount(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	// A non-signing convener FIRST, so roster order and signing order differ. Without that the
	// two rules agree and this cannot discriminate — the same setup `l3_test` uses for
	// PredecessorOf, and for the same reason.
	conv := l3Identity(t, "Convener")
	a, b := l3Identity(t, "A"), l3Identity(t, "B")
	r := Roster{Entries: []RosterEntry{
		{Fingerprint: conv.fp, Signs: false},
		{Fingerprint: a.fp, Signs: true},
		{Fingerprint: b.fp, Signs: true},
	}}
	// SETUP: the roster really has a non-signing entry before a signing one.
	if r.Entries[0].Signs || len(SigningOrder(r)) != 2 {
		t.Fatal("setup: the roster does not straddle signing and non-signing as intended")
	}
	doc, err := PrepareCeremonyDocument(base, CeremonyID{9}, []byte(conv.fp[:8]), len(SigningOrder(r)))
	if err != nil {
		t.Fatal(err)
	}

	pa, err := PlacementFor(doc, r, a.fp)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := PlacementFor(doc, r, b.fp)
	if err != nil {
		t.Fatal(err)
	}
	if pa.Rect == pb.Rect && pa.Page == pb.Page {
		t.Fatal("A and B got the same block: the two signing parties overlap exactly")
	}
	// A is the FIRST signing party, so its block is at index 0 — not index 1, which is where a
	// roster-order walk that counted the non-signing convener would put it.
	if want := stackPlacement(pa.Page, 0).Rect[1]; pa.Rect[1] != want {
		t.Errorf("the first SIGNING party's block sits at y=%.0f, want %.0f: a non-signing "+
			"convener is burning a block slot, so every party's block is one position out and "+
			"the last one runs off the page sooner than the roster says", pa.Rect[1], want)
	}

	// And a party who never signs has no block at all — asking is an error, not slot zero.
	if _, err := PlacementFor(doc, r, conv.fp); !errors.Is(err, ErrNotInRoster) {
		t.Errorf("a NON-SIGNING party was given a placement (%v): a block is a visible claim to "+
			"have signed, and there is nothing for it to describe", err)
	}
}

// TestAPlacementOnAnOffsetPageIsRefusedNotClamped is T02, and its fixture is Nib's own output.
//
// **`stackPlacement` reads no geometry, and `bottom = 40.0` is a distance from the COORDINATE
// ORIGIN rather than from the page's lower edge.** On an A4 page boxed (0,0)-(595,842) those are
// the same point, which is why the distinction never surfaced. They are not the same on a tile:
// measured, `pdfops.SplitPage(base, 1, 2, 2, false)` produces pages boxed
// (0,0)-(297.5,421), (297.5,421)-(595,842) and so on — the offset the clause names, produced by
// the product rather than by a fixture that asserts its own premise.
//
// A 280×84 block with a 40 pt margin does not fit a 297.5 pt tile. The point of the test is that
// saying so is a NAMED error: pdfcpu clamps overflow silently, so a clamp turns "the block is off
// the page" into "the block is a different size", and only one of those is visible to somebody
// reading the finished document.
func TestAPlacementOnAnOffsetPageIsRefusedNotClamped(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	tiles, err := pdfops.SplitPage(base, 1, 2, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the split really did produce an OFFSET box, or this measures an ordinary page and
	// says nothing about the origin at all. This is the clause's own premise and it is checked,
	// not assumed.
	llx, lly, urx, ury, err := pdfops.PageBox(tiles, 2)
	if err != nil {
		t.Fatal(err)
	}
	if llx == 0 && lly == 0 {
		t.Fatalf("setup: tile 2 is boxed (%.1f,%.1f)-(%.1f,%.1f), which starts at the origin — "+
			"the split path no longer produces offset tiles and this fixture cannot show the "+
			"difference between measuring from the origin and from the page's lower edge",
			llx, lly, urx, ury)
	}

	if _, err := fitToPage(tiles, stackPlacement(2, 0)); !errors.Is(err, ErrBlockOffThePage) {
		t.Fatalf("a 280x84 block was placed on a %.0fx%.0f tile and the answer was %v, want "+
			"ErrBlockOffThePage — pdfcpu CLAMPS an overflowing rect rather than refusing it, so "+
			"a placement that does not refuse produces a silently resized block (D25)",
			urx-llx, ury-lly, err)
	}

	// And the translation itself: on a page whose box starts at the origin the rect is unchanged,
	// so every existing document places exactly where it always did. Without this arm the fix
	// could be "refuse everything", which passes the assertion above.
	p, err := fitToPage(base, stackPlacement(1, 0))
	if err != nil {
		t.Fatalf("the first block on an ordinary A4 page was refused: %v", err)
	}
	if p.Rect != stackPlacement(1, 0).Rect {
		t.Errorf("on a page boxed from the origin the rect moved from %v to %v: the translation "+
			"is not a no-op there, so every already-signed document disagrees with this one",
			stackPlacement(1, 0).Rect, p.Rect)
	}
}

// TestAllocationFollowsTheSIGNINGCountNotTheRosterLength is the driver clause 1 owes.
//
// Its build half was met at P07.S02 — `convene.go` counts `p.Signs` and hands that to
// `PrepareCeremonyDocument` — but nothing drove the case the clause actually names: a roster whose
// LENGTH and SIGNING COUNT fall on opposite sides of a page boundary. Seven entries with six
// signing needs one page; counting the length would allocate two, and the second is a blank page
// in a signed document.
func TestAllocationFollowsTheSIGNINGCountNotTheRosterLength(t *testing.T) {
	var r Roster
	for i := 0; i < blocksPerPage; i++ {
		r.Entries = append(r.Entries, RosterEntry{
			Fingerprint: l3Identity(t, fmt.Sprintf("Signer %d", i)).fp, Signs: true})
	}
	r.Entries = append(r.Entries, RosterEntry{
		Fingerprint: l3Identity(t, "Convener").fp, Signs: false})

	// SETUP: the two counts really straddle the boundary, or the assertion below is true of any
	// roster and the test discriminates nothing.
	signing := len(SigningOrder(r))
	if SignaturePagesFor(signing) == SignaturePagesFor(len(r.Entries)) {
		t.Fatalf("setup: %d entries and %d signing both allocate %d page(s); this roster does not "+
			"straddle a page boundary", len(r.Entries), signing, SignaturePagesFor(signing))
	}
	if want := 1; SignaturePagesFor(signing) != want {
		t.Fatalf("setup: %d signing parties allocate %d pages, want %d", signing,
			SignaturePagesFor(signing), want)
	}

	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := PrepareCeremonyDocument(base, CeremonyID{7}, []byte("convener"), signing)
	if err != nil {
		t.Fatal(err)
	}
	total, err := pdfops.PageCount(doc)
	if err != nil {
		t.Fatal(err)
	}
	// source + readme + ceremony + one signature page.
	basePages, _ := pdfops.PageCount(base)
	if want := basePages + 3; total != want {
		t.Errorf("a roster of %d entries with %d signing produced %d pages, want %d — an appended "+
			"page for a party who never signs is a blank page in a signed document",
			len(r.Entries), signing, total, want)
	}
	// Every signing party's block lands on that single page, and none of them off it.
	for i, e := range SigningOrder(r) {
		p, err := PlacementFor(doc, r, e.Fingerprint)
		if err != nil {
			t.Fatalf("signing party %d has no placement: %v", i, err)
		}
		if p.Page != total {
			t.Errorf("party %d's block is on page %d, want the single signature page %d",
				i, p.Page, total)
		}
	}
}

// TestABlockIsActuallyDRAWNWhereItWasPlaced is the positive control D25's clause demands, in the
// only form this repo can produce without a rasteriser.
//
// **The clause's reasoning generalises past rasters.** It says "a raster cannot distinguish 'off
// the page' from 'never drawn', and absence must not pass as compliance". The same holds one level
// up: a check on placement ARITHMETIC cannot distinguish "placed correctly" from "not placed at
// all", because both leave a valid document and `sign.Verify` reports an invisible signature
// exactly as it reports a visible one. Every other test in this file would stay green if
// `Contribute` silently dropped the appearance.
//
// So this signs for real and reads the widget back out of the bytes: the annotation exists, it is
// on the page the placement named, its rect is the rect the placement named, and it carries an
// appearance stream — a widget with no /AP has a rectangle and draws nothing.
//
// What it does NOT claim: this is structural, not rendered. It cannot see a block drawn white on
// white, or an /AP stream that positions content outside its own BBox. That half needs pdf.js and
// is recorded as owed rather than implied.
func TestABlockIsActuallyDRAWNWhereItWasPlaced(t *testing.T) {
	// **A TABLE, and the second row is the whole of /pending 305's Go half.**
	//
	// At n=4 there is one signature page and it IS the last page, so `Appearance{Page}` was
	// exercised at exactly one value for as long as this test has existed — and `blockink`'s own
	// head argues that a roster "changes only the VALUES of Page and Rect", which is true and had
	// never been checked at any value but the last. n=9 allocates two signature pages, so the
	// first block lands on a page that is not the last and the vendored writer's page-index
	// resolution is finally on the hook.
	//
	// The arithmetic minimum is SEVEN signers (`blocksPerPage` = 6), not the nine /pending 305
	// assumed. n=9 is kept because it is the count the rest of this file drives.
	for _, n := range []int{4, 9} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) { drawnWherePlaced(t, n) })
	}
}

func drawnWherePlaced(t *testing.T, n int) {
	doc, r := ceremonyFixture(t, n)

	// SETUP: nothing is on the document yet, or "there is a widget" is true of the fixture.
	before, err := pdfops.SignatureWidgets(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("setup: the convened document already carries %d signature widget(s); this "+
			"cannot show that the contribution drew one", len(before))
	}

	// **EVERY party signs, not just the first.** Signing one party only ever exercises the first
	// block's page, which at both counts is the first allocated signature page; the block that
	// reaches a *middle* page is a later one.
	signed := doc
	placed := make([]Placement, 0, n)
	for i, party := range SigningOrder(r) {
		place, perr := PlacementFor(signed, r, party.Fingerprint)
		if perr != nil {
			t.Fatalf("placing party %d: %v", i, perr)
		}
		placed = append(placed, place)
		signer := l3Identity(t, fmt.Sprintf("Signer %d", i))
		// A real appearance: 1x1 opaque PNG is enough — the question is whether an /AP stream
		// reaches the page at the placed rect, not what the picture shows.
		next, cerr := Contribute(signed, []byte(signer.cert), []byte(signer.key),
			Attestation{Signer: fmt.Sprintf("Party %d", i), When: time.Now()}, onePixelPNG(), place)
		if cerr != nil {
			t.Fatalf("contributing with an appearance failed at party %d: %v", i, cerr)
		}
		signed = next
	}

	got, err := pdfops.SignatureWidgets(signed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("the signed document carries %d signature widget(s), want %d — a block was "+
			"placed by arithmetic and never drawn, which every other test here would call a pass",
			len(got), n)
	}

	total, _ := pdfops.PageCount(signed)
	// **THE STIMULUS for the whole point of the table.** At n=9 at least one block must land on a
	// page that is not the last, or this row is a second copy of the n=4 row and the page index
	// is still exercised at one value. It is asserted rather than assumed because
	// `SignaturePagesFor` is arithmetic that can change.
	if n > blocksPerPage {
		middle := false
		for _, p := range placed {
			if p.Page != total {
				middle = true
				break
			}
		}
		if !middle {
			t.Fatalf("setup: every one of %d blocks was placed on the last page (%d) — this row "+
				"exists to exercise a page index that is NOT the last, and it does not", n, total)
		}
	}

	for i, w := range got {
		place := placed[i]
		if w.Page != place.Page {
			t.Errorf("party %d's block was drawn on page %d and placed on page %d (of %d)",
				i, w.Page, place.Page, total)
		}
		for j := range w.Rect {
			if diff := w.Rect[j] - place.Rect[j]; diff > 0.5 || diff < -0.5 {
				t.Errorf("party %d's drawn rect is %v and the placement said %v: what the "+
					"document carries is not what was computed", i, w.Rect, place.Rect)
				break
			}
		}
		if !w.HasAP {
			t.Errorf("party %d's widget carries no appearance stream: it has a rectangle and "+
				"draws nothing, so the block is blank on the page while every geometric check "+
				"calls it placed", i)
		}
	}

	// **The differential half, and the fix makes it structural rather than a raster question.**
	// D25's overlap was blocks painted over the readme's prose. Blocks now live on dedicated
	// signature pages, so "no readme ink under a block" is not a scan any more — it is that no
	// block is on that page at all. The readme is the page before the ceremony page.
	readme := total - SignaturePagesFor(n) - 1
	for _, w := range got {
		if w.Page == readme {
			t.Errorf("a block was drawn on the readme page (%d of %d): that is D25's overlap, and "+
				"on a page carrying prose it is invisible to anything that only checks the rect "+
				"fits the box", readme, total)
		}
	}
}

// onePixelPNG is an opaque 1x1 image — enough to make Contribute attach an appearance stream. The
// question this fixture serves is whether an /AP reaches the page at the placed rect, never what
// the picture shows.
func onePixelPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{0, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
