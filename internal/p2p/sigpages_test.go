package p2p

import (
	"bytes"
	"encoding/hex"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pairing"
	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestSixIsBelowTheGeometricCeiling holds D25's choice and the geometry that justifies it
// together, so neither can move without the other being read.
//
// D25 picked six over the eight that arithmetically fit, because six "leaves room for a page
// heading and a margin that is not a rounding error". That reasoning is only true while a
// heading exists and while six is genuinely below the ceiling — this asserts both halves.
func TestSixIsBelowTheGeometricCeiling(t *testing.T) {
	const a4Height = 841.89
	fits := 0
	for i := 0; i < 32; i++ {
		if stackPlacement(1, i).Rect[3] <= a4Height {
			fits++
		}
	}
	if fits == 0 {
		t.Fatal("no block fits an A4 page — stackPlacement has changed shape and this guard " +
			"is measuring nothing")
	}
	if blocksPerPage > fits {
		t.Errorf("blocksPerPage is %d and only %d blocks fit an A4 page — a ceremony would "+
			"allocate a page it then overflows", blocksPerPage, fits)
	}
	if blocksPerPage == fits {
		t.Errorf("blocksPerPage is %d, exactly the geometric ceiling — D25 chose six BELOW the "+
			"ceiling so a page heading and a margin have room. If the heading is gone, this "+
			"number should be %d and D25's reasoning needs revisiting rather than silently "+
			"outliving itself.", blocksPerPage, fits)
	}
}

// TestTodaysCoSignIsUntouchedByTheAllocator — PrepareDocument does NOT gain pages.
//
// The first draft of the allocator routed PrepareDocument through the ceremony door at
// signers=2, so that a two-party ceremony would keep today's appearance. That was solving a
// problem that does not exist — today's co-sign has no Record and never reaches the ceremony
// door — and it cost a special case that made a block's page depend on its index under two
// rules. This pins the fact that made the special case unnecessary.
func TestTodaysCoSignIsUntouchedByTheAllocator(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	basePages, _ := pdfops.PageCount(base)
	prepared, err := PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	pages, _ := pdfops.PageCount(prepared)
	if pages != basePages+1 {
		t.Errorf("PrepareDocument produced %d pages from %d — a plain co-sign gains the readme "+
			"and nothing else; anything more is a visible change to the shipped product with no "+
			"criterion asking for it", pages, basePages)
	}
	// And the blocks still sit on the readme page, which is what readmeFloor reserves room for.
	place, err := NextPlacement(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if place.Page != pages {
		t.Errorf("the first block targets page %d of %d", place.Page, pages)
	}
}

func TestSignaturePagesAllocatesFromTheRoster(t *testing.T) {
	for _, c := range []struct{ signers, want int }{
		{1, 1}, {2, 1}, {3, 1}, {6, 1}, {7, 2}, {9, 2}, {12, 2}, {13, 3}, {32, 6},
	} {
		if got := signaturePages(c.signers); got != c.want {
			t.Errorf("signaturePages(%d) = %d, want %d", c.signers, got, c.want)
		}
	}
	// D25 states the 32 case in prose ("thirty-two is six signature pages"); assert it so the
	// sentence and the code cannot drift.
	if got := signaturePages(32); got != 6 {
		t.Errorf("MaxRoster of 32 allocates %d pages; invitation.go's comment says six", got)
	}
}

// TestPreparingATwoPartyDocumentIsUNCHANGED — the shipped product's output does not move.
//
// This is the regression pin on the whole allocation change. A two-party co-sign is the only
// ceremony Nib can currently run, and its blocks sit on the readme page. If allocation moved
// them, every existing co-signed document would look different for no criterion's sake — and
// worse, ContentDigest would move, which is DocHash, which is an axis of RosterHash.
func TestPreparingATwoPartyCeremonyAllocatesItsOwnPages(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	old, err := PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	oldPages, _ := pdfops.PageCount(old)
	oldDigest, err := pdfops.ContentDigest(old)
	if err != nil {
		t.Fatal(err)
	}
	// Setup assertion: the readme really was appended, or "unchanged" below is a comparison
	// between two documents that had nothing done to them.
	basePages, _ := pdfops.PageCount(base)
	if oldPages != basePages+1 {
		t.Fatalf("setup: PrepareDocument did not append the readme (%d -> %d)", basePages, oldPages)
	}

	fp := make([]byte, 32)
	viaDoor, err := PrepareCeremonyDocument(base, CeremonyID{}, fp, 2)
	if err != nil {
		t.Fatal(err)
	}
	newPages, _ := pdfops.PageCount(viaDoor)
	newDigest, err := pdfops.ContentDigest(viaDoor)
	if err != nil {
		t.Fatal(err)
	}
	// readme + ceremony page + one signature page.
	if want := oldPages + 1 + signaturePages(2); newPages != want {
		t.Errorf("a two-party ceremony has %d pages, want %d", newPages, want)
	}
	// It MUST differ from a plain co-sign — a ceremony document is a different artifact, and
	// a digest that did not move would mean the ceremony page and signature page were never
	// appended.
	if newDigest == oldDigest {
		t.Errorf("a ceremony document hashes identically to a plain co-signed one (%s) — the "+
			"ceremony and signature pages were not appended", oldDigest[:16])
	}
}

func TestAllocatedPagesLandAfterTheReadmeAndCarryAHeading(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	var id CeremonyID
	doc, err := PrepareCeremonyDocument(base, id, make([]byte, 32), 9)
	if err != nil {
		t.Fatal(err)
	}
	pages, _ := pdfops.PageCount(doc)
	// 1 content + 1 readme + 1 ceremony page + 2 signature pages
	if want := 1 + 1 + 1 + signaturePages(9); pages != want {
		t.Fatalf("a nine-party document has %d pages, want %d", pages, want)
	}
	// The blocks now land on an ALLOCATED page, not on the readme — D25's overlap half.
	place, err := NextPlacement(doc)
	if err != nil {
		t.Fatal(err)
	}
	if place.Page != pages {
		t.Errorf("NextPlacement targets page %d of %d", place.Page, pages)
	}
	if place.Page <= 3 {
		t.Errorf("the first block landed on page %d — that is the content, the readme or the "+
			"ceremony page, so allocation did not move the stack off the prose", place.Page)
	}
	// And the heading is really there, so "six leaves room for a heading" is not a claim about
	// a page that has none. Extracted through the same unescape/WinAnsi path readme_test.go
	// established, because a phrase that arrives escaped makes a POSITIVE assertion fail
	// loudly and a negative one pass quietly.
	txt := renderedPageText(t, doc, pages)
	if !strings.Contains(txt, id.String()) {
		t.Errorf("the allocated page does not name its ceremony; extracted: %.160q", txt)
	}
	if !strings.Contains(txt, "page 2 of 2") {
		t.Errorf("the allocated page does not say which page it is; extracted: %.160q", txt)
	}
}

// renderedPageText extracts one page's visible text, undoing the PDF escaping and the
// WinAnsi single-byte encoding — the same path readme_test.go's renderedReadme uses, and
// for the same reason recorded there.
func renderedPageText(t *testing.T, pdf []byte, page int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := api.ExtractContent(bytes.NewReader(pdf), []string{strconv.Itoa(page)},
		func(r io.Reader, _ int) error { _, e := io.Copy(&buf, r); return e },
		model.NewDefaultConfiguration()); err != nil {
		t.Fatalf("extract page %d: %v", page, err)
	}
	var runs []string
	for _, m := range litRE.FindAllStringSubmatch(buf.String(), -1) {
		runs = append(runs, winAnsiToUTF8(pdfUnescape(m[1])))
	}
	flat := strings.Join(strings.Fields(strings.Join(runs, " ")), " ")
	// Setup assertion: an extractor that silently returned nothing would make every
	// substring check above pass.
	if flat == "" {
		t.Fatalf("extraction of page %d returned nothing — no assertion built on it means "+
			"anything", page)
	}
	return flat
}

// TestTheCeremonyPageNamesTheProceedingAndNoOneElse — S08's struck "name the convener"
// clause, landed, plus the property that keeps it safe.
//
// The page must say which proceeding the document belongs to and how many parties are
// obliged, so a flattened bundle is legible and an unfinished ceremony is visibly unfinished
// (C18's substance). It must NOT carry convener-typed text, because this page is vector text
// through CreateFromJSON — the path where S08's four hazards live — while a signature block
// is a rasterised PNG that never touches it.
func TestTheCeremonyPageNamesTheProceedingAndNoOneElse(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	var id CeremonyID
	for i := range id {
		id[i] = byte(i)
	}
	fp := make([]byte, 32)
	for i := range fp {
		fp[i] = byte(i)
	}
	doc, err := PrepareCeremonyDocument(base, id, fp, 9)
	if err != nil {
		t.Fatal(err)
	}
	// The ceremony page is the one after the readme: content, readme, ceremony, sig pages.
	txt := renderedPageText(t, doc, 3)

	if !strings.Contains(txt, id.String()) {
		t.Errorf("the ceremony page does not name its ceremony: %.200q", txt)
	}
	// **The whole phrase, not the digit.** `strings.Contains(txt, "9")` was unfalsifiable:
	// the fixture's ceremony id renders as 000102…0f and the fingerprint likewise, so a "9"
	// is on the page whether or not the count ever is. Found at the slice's diff review.
	if !strings.Contains(txt, "Parties obliged to sign: 9") {
		t.Errorf("the ceremony page does not say how many parties are obliged to sign, so an "+
			"abandoned ceremony reads as a complete one: %.200q", txt)
	}
	if !strings.Contains(txt, hex.EncodeToString(fp)) {
		t.Errorf("the ceremony page does not name the convener by fingerprint: %.200q", txt)
	}
	// The six-word name, DERIVED — the identity vocabulary the rest of the product uses, and
	// the reason no convener-typed label is needed here at all.
	name, err := pairing.Name(fp)
	if err != nil {
		t.Fatalf("setup: pairing.Name refused a 32-byte fingerprint: %v", err)
	}
	if !strings.Contains(txt, name) {
		t.Errorf("the ceremony page does not carry the convener's six-word name %q — a reader "+
			"cannot check it aloud against the person on the phone: %.200q", name, txt)
	}
}

// TestTheCeremonyPageTakesNoPartySuppliedBytes is the guard on the property above, and it is
// deliberately about the SIGNATURE of the door rather than about the rendered output.
//
// An output-shaped test ("no convener label appears on the page") passes trivially while a
// future caller is free to thread one in. What keeps the hazard class away is that the door
// cannot be handed free text at all: its inputs are a ceremony id, a fingerprint and a count.
func TestTheCeremonyPageTakesNoPartySuppliedBytes(t *testing.T) {
	ty := reflect.TypeOf(PrepareCeremonyDocument)
	// (pdf []byte, ceremonyID string, convenerFP []byte, signers int)
	if ty.NumIn() != 4 {
		t.Fatalf("PrepareCeremonyDocument takes %d arguments; this guard was written for 4 and "+
			"cannot tell which of them is new", ty.NumIn())
	}
	// NO string parameter at all. The first draft took the ceremony id as a string and
	// measured: it accepted "%v not an id\nsecond line" and rendered it. The id is now a
	// [16]byte this package hex-encodes itself, so the hazard is unrepresentable rather than
	// validated — and a future caller cannot reintroduce it without changing this signature
	// and reading this test.
	for i := 0; i < ty.NumIn(); i++ {
		if ty.In(i).Kind() == reflect.String {
			t.Errorf("PrepareCeremonyDocument parameter %d is a string. Every string here is a "+
				"channel for convener-typed text onto a pdfcpu-rendered page, where a `%%` is a "+
				"placeholder introducer, a WinAnsi-unencodable rune renders as a space with "+
				"err == nil, and an embedded newline positions a run upward over existing text. "+
				"The recital and every capacity belong on S07's rasterised block.", i)
		}
	}
	// The stimulus: the door really is reachable and really renders, so the loop above is not
	// passing over a function that does nothing.
	base, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareCeremonyDocument(base, CeremonyID{}, make([]byte, 32), 2); err != nil {
		t.Fatalf("setup: the door refused an ordinary ceremony: %v", err)
	}
}
