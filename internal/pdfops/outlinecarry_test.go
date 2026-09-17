package pdfops

import (
	"bytes"
	"sort"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// addBookmarks is `bookmarkedPDF` over a document the caller built, for the tests that need the
// pages to be distinguishable from one another rather than merely present.
func addBookmarks(t *testing.T, pdf []byte, bms []pdfcpu.Bookmark) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := api.AddBookmarks(bytes.NewReader(pdf), &out, bms, true, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// outlineShape reads a document's outline back through the SAME door the product uses — `Outline()`,
// which is what "Jump to Section" and `SplitByBookmarks` both read — and returns it as a comparable
// list.
//
// Reading it back through the product's own reader rather than by walking the dictionaries is the
// point: a tree can be internally consistent and still be invisible to the reader, which is exactly
// how a destination-less container hides its surviving children (`outlinecarry.go`'s header).
func outlineShape(t *testing.T, pdf []byte) []OutlineItem {
	t.Helper()
	items, err := Outline(pdf)
	if err != nil {
		t.Fatalf("reading the outline back: %v", err)
	}
	return items
}

func sameOutline(t *testing.T, got []OutlineItem, want []OutlineItem) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("outline has %d items, want %d: got %v, want %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %+v, want %+v (whole outline %v)", i, got[i], want[i], got)
		}
	}
}

// TestAReorderedDocumentKeepsItsBookmarksPointingAtTheirOwnPages is `/pending 524`'s headline: the
// user types bookmarks, drags a page, and the panel keeps every bookmark AND sends each to the page
// it always meant.
//
// The selection is a full permutation with nothing dropped, so every bookmark must survive — and the
// pages it names must be the new POSITIONS of the pages it named before. `4,3,1` over a four-page
// document is chosen because it is not a rotation: page 1 moves to position 3, page 3 to position 2
// and page 4 to position 1, so an implementation that carried the outline without re-resolving
// anything — or one that resolved the old index — disagrees with all three.
func TestAReorderedDocumentKeepsItsBookmarksPointingAtTheirOwnPages(t *testing.T) {
	src := bookmarkedPDF(t, 4, []pdfcpu.Bookmark{
		{Title: "Alpha", PageFrom: 1},
		{Title: "Beta", PageFrom: 3},
		{Title: "Gamma", PageFrom: 4},
	})
	// The floor: if the fixture ever stops carrying an outline this test greens on two empty lists.
	if len(outlineShape(t, src)) != 3 {
		t.Fatalf("setup: the fixture does not carry three bookmarks (%v)", outlineShape(t, src))
	}

	out, err := Collect(src, []string{"4", "3", "1"})
	if err != nil {
		t.Fatal(err)
	}
	// Source order, new pages. The order is the outline's own and is deliberately not re-sorted;
	// see `outlinecarry.go`.
	sameOutline(t, outlineShape(t, out), []OutlineItem{
		{Title: "Alpha", Page: 3, Level: 0},
		{Title: "Beta", Page: 2, Level: 0},
		{Title: "Gamma", Page: 1, Level: 0},
	})
}

// TestABookmarkWhosePageWasDeletedIsDropped pins the other half of the rule, and pins that it is a
// PRUNE and not an all-or-nothing carry: the bookmarks whose pages survived are still there, at
// their new positions, beside the one that went.
//
// **It counts the item DICTIONARIES as well as reading the outline back, and that second assertion
// is the one a mutation found missing.** "Dropped" and "kept but unreadable" look identical through
// `Outline()`: pdfcpu skips an item whose destination does not resolve, so an implementation that
// kept every node and merely failed to give the doomed one a destination reads exactly like a
// correct prune. It is not one — the node is still in the file, still in the /Next chain, still
// counted, and still shown by every other PDF reader as a bookmark that goes nowhere.
func TestABookmarkWhosePageWasDeletedIsDropped(t *testing.T) {
	src := bookmarkedPDF(t, 4, []pdfcpu.Bookmark{
		{Title: "Alpha", PageFrom: 1},
		{Title: "Beta", PageFrom: 2},
		{Title: "Gamma", PageFrom: 4},
	})
	out, err := RemovePages(src, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	sameOutline(t, outlineShape(t, out), []OutlineItem{
		{Title: "Alpha", Page: 1, Level: 0},
		{Title: "Gamma", Page: 3, Level: 0},
	})
	if got := countOutlineKey(t, out, "Title"); got != 2 {
		t.Errorf("the outline holds %d item dictionaries, want 2: a node kept without a "+
			"destination is invisible to `Outline()` and still a dead bookmark in the file", got)
	}
	if got := countOutlineKey(t, out, "Dest"); got != 2 {
		t.Errorf("%d item(s) carry a /Dest, want 2 — every surviving node must have one", got)
	}
}

// TestADocumentThatLosesEveryBookmarkHasNoOutlineKeyAtAll: the catalog must not keep an `/Outlines`
// root with no children.
//
// The assertion is on the KEY and not on `Outline()` returning nothing, because those are different
// claims and only one of them is the defect. An empty outline root reads back as zero bookmarks too,
// so a test that only asked the reader would pass on a document whose contents pane opens empty —
// which is the shape this exists to forbid.
func TestADocumentThatLosesEveryBookmarkHasNoOutlineKeyAtAll(t *testing.T) {
	src := bookmarkedPDF(t, 3, []pdfcpu.Bookmark{
		{Title: "Alpha", PageFrom: 1},
		{Title: "Beta", PageFrom: 2},
	})
	out, err := Collect(src, []string{"3"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := readCtx(t, out)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if o, has := root["Outlines"]; has {
		t.Errorf("the catalog kept an /Outlines key (%v) after every bookmark was dropped; an "+
			"outline root with no children is a contents pane that opens empty", o)
	}
	if items := outlineShape(t, out); len(items) != 0 {
		t.Errorf("outline = %v, want none", items)
	}
}

// TestANestedOutlineKeepsItsShapeAndItsCounts drives the doubly-linked repair, which is where a
// carry goes subtly wrong rather than visibly.
//
// Four things are graded at once and each fails differently: the nesting (`Level`), the pages, the
// /Count MAGNITUDE on a parent whose children were pruned, and the /Count SIGN, which is the only
// place a node's open/closed state is recorded and is therefore the one that a rebuild-from-scratch
// silently discards.
func TestANestedOutlineKeepsItsShapeAndItsCounts(t *testing.T) {
	src := bookmarkedPDF(t, 6, []pdfcpu.Bookmark{
		{Title: "One", PageFrom: 1, Kids: []pdfcpu.Bookmark{
			{Title: "One-a", PageFrom: 2},
			{Title: "One-b", PageFrom: 3},
		}},
		{Title: "Two", PageFrom: 4, Kids: []pdfcpu.Bookmark{
			{Title: "Two-a", PageFrom: 5},
		}},
		{Title: "Three", PageFrom: 6},
	})
	if got := outlineShape(t, src); len(got) != 6 {
		t.Fatalf("setup: the fixture does not carry six bookmarks (%v)", got)
	}

	// Drop page 3 (One-b) and page 6 (Three). One keeps one child; Two is untouched; Three goes.
	out, err := Collect(src, []string{"1", "2", "4", "5"})
	if err != nil {
		t.Fatal(err)
	}
	sameOutline(t, outlineShape(t, out), []OutlineItem{
		{Title: "One", Page: 1, Level: 0},
		{Title: "One-a", Page: 2, Level: 1},
		{Title: "Two", Page: 3, Level: 0},
		{Title: "Two-a", Page: 4, Level: 1},
	})

	// /Count is not visible through `Outline()` at all, so it is read from the dictionaries. Both
	// parents are OPEN in the fixture (pdfcpu writes a positive /Count), and each now has exactly one
	// child, so each must read +1 — and the root must read 4, the number of items visible with the
	// whole tree expanded, not the number of top-level items.
	counts := outlineCounts(t, out)
	for _, c := range []struct {
		title string
		want  int
	}{{"", 4}, {"One", 1}, {"One-a", 0}, {"Two", 1}, {"Two-a", 0}} {
		got, has := counts[c.title]
		if c.want == 0 {
			if has {
				t.Errorf("leaf %q carries /Count %d; a leaf carries none, and 0 would claim "+
					"\"open with nothing in it\"", c.title, got)
			}
			continue
		}
		if !has {
			t.Errorf("%q carries no /Count, want %d", c.title, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("%q /Count = %d, want %d", c.title, got, c.want)
		}
	}
}

// TestAClosedBookmarkStaysClosedAcrossASelection isolates the SIGN of /Count from its magnitude.
//
// It is a separate test from the one above and not another row in it, because the two conditions are
// checked by one expression in the production code and a test that only ever sees open parents
// cannot tell `Count = n` from `Count = ±n`. The fixture is mutated to closed after it is built:
// pdfcpu's `AddBookmarks` only ever writes open items, so the shape cannot be produced any other way
// — and it is the shape every real document with a collapsed chapter has.
func TestAClosedBookmarkStaysClosedAcrossASelection(t *testing.T) {
	src := bookmarkedPDF(t, 4, []pdfcpu.Bookmark{
		{Title: "One", PageFrom: 1, Kids: []pdfcpu.Bookmark{
			{Title: "One-a", PageFrom: 2},
			{Title: "One-b", PageFrom: 3},
		}},
		{Title: "Two", PageFrom: 4},
	})
	src = closeOutlineItem(t, src, "One")
	if got := outlineCounts(t, src); got["One"] != -2 {
		t.Fatalf("setup: One's /Count is %d, want -2 (closed, two descendants)", got["One"])
	}
	if got := outlineCounts(t, src); got[""] != 2 {
		t.Fatalf("setup: the root's /Count is %d, want 2 — a closed item's subtree is not visible, "+
			"so this test could not tell a sign error from a counting error", got[""])
	}

	out, err := Collect(src, []string{"1", "2", "4"}) // page 3 (One-b) goes
	if err != nil {
		t.Fatal(err)
	}
	counts := outlineCounts(t, out)
	if counts["One"] != -1 {
		t.Errorf("One's /Count = %d, want -1: it kept one descendant and it was CLOSED, and the "+
			"sign is the only record of that", counts["One"])
	}
	if counts[""] != 2 {
		t.Errorf("the root's /Count = %d, want 2 (One and Two): a closed item's descendants are "+
			"not visible and must not be counted", counts[""])
	}
}

// TestABookmarkWhoseOwnPageWentInheritsItsFirstSurvivingChilds is the case reading pdfcpu's reader
// turned up: a chapter heading whose opening page is deleted while its sections survive.
//
// Keeping it as a title with no destination is what a careful implementation does and it is wrong
// here — `bookmarksForOutlineItem` skips an item whose destination does not resolve BEFORE it
// descends into /First (`bookmark.go:248-251`), so the surviving sections would disappear from the
// panel along with the heading. The assertion is therefore that the children are still READABLE,
// and that the heading now points at the first of them.
func TestABookmarkWhoseOwnPageWentInheritsItsFirstSurvivingChilds(t *testing.T) {
	src := bookmarkedPDF(t, 4, []pdfcpu.Bookmark{
		{Title: "Chapter", PageFrom: 1, Kids: []pdfcpu.Bookmark{
			{Title: "Section one", PageFrom: 2},
			{Title: "Section two", PageFrom: 3},
		}},
	})
	out, err := RemovePages(src, []string{"1"}) // the chapter's own page, not its sections'
	if err != nil {
		t.Fatal(err)
	}
	sameOutline(t, outlineShape(t, out), []OutlineItem{
		{Title: "Chapter", Page: 1, Level: 0}, // old page 2 — where its first surviving section is
		{Title: "Section one", Page: 1, Level: 1},
		{Title: "Section two", Page: 2, Level: 1},
	})
}

// TestAGoToActionBookmarkSurvivesAsAPlainDestination covers the second destination shape and the
// action surface in one document, because they are the same decision.
//
// A `/S /GoTo` item must still navigate; nothing shaped like an action may survive, whatever its
// /S says and whatever it chains to through /Next. The fixture gives one item a benign /GoTo whose
// /Next runs JavaScript — the exact trick `eachAction`'s header records catching in annotations —
// and a second item a /Launch, which is not navigation and must take the bookmark with it.
func TestAGoToActionBookmarkSurvivesAsAPlainDestination(t *testing.T) {
	src := bookmarkedPDF(t, 3, []pdfcpu.Bookmark{{Title: "Keeper", PageFrom: 1}})
	src = actionBookmarks(t, src)
	// THREE item dictionaries, of which pdfcpu's reader shows two: it skips an item whose /A is not
	// /GoTo (`bookmark.go:289-317`), so "Launcher" is invisible through `Outline()` and has to be
	// counted in the dictionaries. Asserting on the reader alone here would grade a fixture that
	// never carried the /Launch this test exists to see dropped.
	if got := countOutlineKey(t, src, "Title"); got != 3 {
		t.Fatalf("setup: the fixture has %d outline items, want three", got)
	}
	if got := countOutlineKey(t, src, "A"); got != 2 {
		t.Fatalf("setup: the fixture has %d items with an /A, want two", got)
	}

	out, err := Collect(src, []string{"3", "2", "1"})
	if err != nil {
		t.Fatal(err)
	}
	sameOutline(t, outlineShape(t, out), []OutlineItem{
		{Title: "Keeper", Page: 3, Level: 0},
		{Title: "Jumper", Page: 1, Level: 0}, // /A /GoTo at page 3 → position 1
	})
	if n := countOutlineKey(t, out, "A"); n != 0 {
		t.Errorf("%d outline item(s) still carry an /A; no action dictionary may survive a subset — "+
			"neither Scan nor StripActive walks the outline, so one that did would widen a surface "+
			"nib does not inspect", n)
	}
}

// TestAnOutlineDoesNotResurrectADroppedPage is the reachability half, and it is a different failure
// from a wrong bookmark: pdfcpu writes by reachability, so a surviving outline item still holding a
// destination into a removed page puts that page's dictionary — and its /Contents — back into the
// output. "Removed" would mean "hidden".
//
// It is asserted on the OUTPUT BYTES — through `fileCarries`, which scans raw and inflated stream
// segments and deliberately does not walk the page tree — rather than on the page count, because a
// resurrected page is reachable WITHOUT being in the page tree: the count would still read right.
//
// **The destination must be an EXPLICIT array, and that is what makes this test falsifiable.** A
// NAMED destination cannot re-anchor anything: `pruneNames` removes the name, so a bookmark wrongly
// kept beside it holds a string that reaches nothing and the page stays gone. Written with a named
// destination this test passes however the production code is broken — a vacuous green. The array
// `[page /Fit]` is the shape that names the page dictionary itself.
func TestAnOutlineDoesNotResurrectADroppedPage(t *testing.T) {
	base, err := testpdf.Text("ZZKEEPONE", "ZZSECRETTWO", "ZZKEEPTHREE")
	if err != nil {
		t.Fatal(err)
	}
	src := addBookmarks(t, base, []pdfcpu.Bookmark{{Title: "Keep", PageFrom: 1}})
	src = explicitDestBookmark(t, src, "Secret", 2)
	if raw, streams := fileCarries(src, "ZZSECRETTWO"); !raw && streams == 0 {
		t.Fatal("setup: the marker is not in the fixture's own bytes, so this test grades nothing")
	}
	if got := outlineShape(t, src); len(got) != 2 {
		t.Fatalf("setup: the fixture carries %v, want two bookmarks", got)
	}
	out, err := RemovePages(src, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if raw, streams := fileCarries(out, "ZZSECRETTWO"); raw || streams > 0 {
		t.Errorf("the removed page's content is still in the output (raw=%v, %d stream(s)): a "+
			"surviving outline item holding a destination into a dropped page re-anchors it, and "+
			"pdfcpu writes by reachability", raw, streams)
	}
	// The bookmark for the page that stayed is still there, or the assertion above is satisfied by
	// an outline that was dropped wholesale.
	sameOutline(t, outlineShape(t, out), []OutlineItem{{Title: "Keep", Page: 1, Level: 0}})
}

// TestASubsetFedToACompositionCarriesNoOutline pins the door split. `collectWithoutStructure` exists
// for redaction (the outline's titles name what the raster destroyed) and for a subset that is then
// COMPOSED (`api.MergeRaw` keeps only the FIRST document's catalog, so part one's contents page
// would be served as the whole document's). Both reasons are the outline's as much as the structure
// tree's, so the flag is shared — and a carry leaking onto that door is the regression.
func TestASubsetFedToACompositionCarriesNoOutline(t *testing.T) {
	src := bookmarkedPDF(t, 3, []pdfcpu.Bookmark{{Title: "Alpha", PageFrom: 1}})
	out, err := collectWithoutStructure(src, []string{"2", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if items := outlineShape(t, out); len(items) != 0 {
		t.Errorf("the non-carrying door carried an outline (%v)", items)
	}
	// And the carrying door over the same fixture does, or the assertion above is true of a fixture
	// that never had one.
	carrying, err := Collect(src, []string{"2", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if items := outlineShape(t, carrying); len(items) != 1 {
		t.Errorf("Collect over the same fixture carried %v, want one bookmark — without this the "+
			"test above passes on a document with no outline", items)
	}
}

// TestDroppingOneNamedDestinationKeepsTheRest is a separate defect this item ran into, and it is
// asserted with NO OUTLINE anywhere so that it grades `pruneNames` rather than the carry.
//
// pdfcpu's `Node.Remove` calls `DeleteObjectGraph` on each removed entry's VALUE, and a destination
// array's graph reaches the PAGE it names and whatever that page shares. Those object numbers go on
// the free list and `BindNameTrees` hands them straight back out to the name-tree nodes it mints
// while binding, so the SURVIVING entries end up pointing at the nodes that replaced them. Measured
// before the fix: all six destinations gone, including the four whose pages were kept.
//
// **The fixture is shaped, not arbitrary, and the shape IS the test.** pdfcpu splits a name tree
// leaf at `maxEntries = 3` (`model/nameTree.go:30`), so six names give three leaves of two. The
// object-number reuse only happens when a leaf EMPTIES: that is what calls `removeKid`, which frees
// the leaf's dictionary and, when one kid remains, the intermediate node above it — and those are
// the numbers `BindNameTrees` hands back out. So the two doomed names must share a leaf. Measured
// with one from each of two different leaves, nothing empties, nothing is freed and the bug does not
// reproduce — which is exactly how it stayed invisible.
//
// Here the leaves are {aOne,bOneA}, {cOneB,dTwo}, {eTwoA,fThree}, and dropping pages 3 and 4 takes
// the whole middle one.
func TestDroppingOneNamedDestinationKeepsTheRest(t *testing.T) {
	src := namedDestinations(t, 6, map[string]int{
		"aOne": 1, "bOneA": 2, "cOneB": 3, "dTwo": 4, "eTwoA": 5, "fThree": 6,
	})
	if got := destNames(t, src); len(got) != 6 {
		t.Fatalf("setup: the fixture carries %v, want six named destinations", got)
	}
	if got := destLeafCount(t, src); got < 2 {
		t.Fatalf("setup: the /Dests tree has %d leaves, want at least two — a single-leaf tree "+
			"never reaches the node removal this test exists to grade", got)
	}
	// Pages 3 and 4 go, so cOneB and dTwo go with them — the entire middle leaf — and four remain.
	out, err := Collect(src, []string{"1", "2", "5", "6"})
	if err != nil {
		t.Fatal(err)
	}
	got := destNames(t, out)
	want := []string{"aOne", "bOneA", "eTwoA", "fThree"}
	if len(got) != len(want) {
		t.Fatalf("named destinations = %v, want %v — removing the two that died took the "+
			"survivors with them", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("named destination %d = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
	}
	// Resolving is the other half: an entry can survive by NAME and point at a name-tree node rather
	// than a page, which is exactly what the object-number reuse produced.
	for _, name := range want {
		if page := destPage(t, out, name); page < 1 {
			t.Errorf("named destination %q no longer resolves to a page (got %d)", name, page)
		}
	}
}

// TestCarryingAnOutlineCarriesTheInstructionToShowIt: `/PageMode /UseOutlines` is "open with the
// bookmarks panel showing". Carrying the tree without it leaves the document opening exactly as it
// did when the tree was being dropped, which a user still reports as "my bookmarks are gone".
//
// The second half is the one that keeps the rule narrow: `/PageMode` is carried for that ONE value,
// so a mode naming something the subset destroyed does not ride in behind it.
func TestCarryingAnOutlineCarriesTheInstructionToShowIt(t *testing.T) {
	src := bookmarkedPDF(t, 3, []pdfcpu.Bookmark{{Title: "Alpha", PageFrom: 1}})
	src = setCatalogName(t, src, "PageMode", "UseOutlines")
	out, err := Collect(src, []string{"2", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := catalogName(t, out, "PageMode"); got != "UseOutlines" {
		t.Errorf("/PageMode = %q, want UseOutlines: the outline survived and the instruction to "+
			"show it did not", got)
	}

	// A mode that is NOT about outlines is still dropped, even though an outline survives.
	other := setCatalogName(t, bookmarkedPDF(t, 3, []pdfcpu.Bookmark{{Title: "Alpha", PageFrom: 1}}),
		"PageMode", "UseAttachments")
	out2, err := Collect(other, []string{"2", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := outlineShape(t, out2); len(got) != 1 {
		t.Fatalf("setup: the outline did not survive (%v), so the assertion below grades nothing", got)
	}
	if got := catalogName(t, out2, "PageMode"); got != "" {
		t.Errorf("/PageMode = %q, want none: a subset drops every embedded file, so an instruction "+
			"to open the attachments panel is one this operation has made false", got)
	}
}

// TestAnOutlineThatDidNotSurviveTakesUseOutlinesWithIt: the pairing holds in both directions, or
// the document opens on an empty panel.
func TestAnOutlineThatDidNotSurviveTakesUseOutlinesWithIt(t *testing.T) {
	src := bookmarkedPDF(t, 3, []pdfcpu.Bookmark{{Title: "Alpha", PageFrom: 1}})
	src = setCatalogName(t, src, "PageMode", "UseOutlines")
	out, err := Collect(src, []string{"3"}) // Alpha's page goes, so the whole outline goes
	if err != nil {
		t.Fatal(err)
	}
	if got := catalogName(t, out, "PageMode"); got != "" {
		t.Errorf("/PageMode = %q, want none: every bookmark was dropped, so this opens the "+
			"document on a bookmarks panel with nothing in it", got)
	}
}

// explicitDestBookmark appends a top-level outline item whose /Dest is an explicit `[page /Fit]`
// array — the shape that names a page DICTIONARY, and so the only one that can re-anchor a dropped
// page. pdfcpu's own `AddBookmarks` never writes this shape (it always writes a name), so it has to
// be built by hand.
func explicitDestBookmark(t *testing.T, pdf []byte, title string, page int) []byte {
	t.Helper()
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, rerr := xt.Catalog()
		if rerr != nil {
			return rerr
		}
		rootDic := derefDict(xt, root["Outlines"])
		if rootDic == nil {
			t.Fatal("the fixture has no outline to append to")
		}
		last, lerr := xt.DereferenceDict(rootDic["Last"])
		if lerr != nil || last == nil {
			t.Fatalf("the fixture's outline has no last item: %v", lerr)
		}
		pageRef, perr := ctx.PageDictIndRef(page)
		if perr != nil {
			return perr
		}
		rootRef, _ := root["Outlines"].(types.IndirectRef)
		item := types.Dict{
			"Title":  types.StringLiteral(title),
			"Parent": rootRef,
			"Prev":   rootDic["Last"],
			"Dest":   types.Array{*pageRef, types.Name("Fit")},
		}
		ref, ierr := xt.IndRefForNewObject(item)
		if ierr != nil {
			return ierr
		}
		last["Next"] = *ref
		rootDic["Last"] = *ref
		rootDic["Count"] = types.Integer(rootVisible(xt, rootDic))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// namedDestinations builds a document with named destinations and NO outline, so a test over it
// grades `pruneNames` and nothing else.
func namedDestinations(t *testing.T, pages int, on map[string]int) []byte {
	t.Helper()
	out, err := writeMutated(sizedPDF(t, pages, 200, 200), func(ctx *model.Context) error {
		if err := ctx.LocateNameTree("Dests", true); err != nil {
			return err
		}
		names := make([]string, 0, len(on))
		for name := range on {
			names = append(names, name)
		}
		sort.Strings(names) // a map's order would build a different tree shape each run
		for _, name := range names {
			ref, perr := ctx.PageDictIndRef(on[name])
			if perr != nil {
				return perr
			}
			arr, aerr := ctx.XRefTable.IndRefForNewObject(types.Array{*ref, types.Name("Fit")})
			if aerr != nil {
				return aerr
			}
			if err := ctx.Names["Dests"].Add(ctx.XRefTable, name, *arr, nil, []string{"D", "Dest"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// destNames lists a document's named destinations, sorted.
func destNames(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx := readCtx(t, pdf)
	node := ctx.XRefTable.Names["Dests"]
	if node == nil {
		return nil
	}
	var out []string
	if err := node.Process(ctx.XRefTable, func(_ *model.XRefTable, k string, _ *types.Object) error {
		out = append(out, k)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// destLeafCount counts the leaves of the /Dests name tree, so a test that needs a multi-leaf tree
// can say so rather than assume pdfcpu's splitting strategy never changes.
func destLeafCount(t *testing.T, pdf []byte) int {
	t.Helper()
	ctx := readCtx(t, pdf)
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	names := derefDict(xt, root["Names"])
	if names == nil {
		return 0
	}
	var count func(d types.Dict, depth int) int
	count = func(d types.Dict, depth int) int {
		if d == nil || depth > 20 {
			return 0
		}
		kids := derefArray(xt, d["Kids"])
		if len(kids) == 0 {
			return 1
		}
		n := 0
		for _, k := range kids {
			n += count(derefDict(xt, k), depth+1)
		}
		return n
	}
	return count(derefDict(xt, names["Dests"]), 0)
}

// destPage resolves a named destination to its 1-based page, or 0.
func destPage(t *testing.T, pdf []byte, name string) int {
	t.Helper()
	ctx := readCtx(t, pdf)
	arr, err := ctx.XRefTable.DereferenceDestArray(name)
	if err != nil || len(arr) == 0 {
		return 0
	}
	ref, ok := arr[0].(types.IndirectRef)
	if !ok {
		return 0
	}
	for p := 1; p <= ctx.PageCount; p++ {
		pr, perr := ctx.PageDictIndRef(p)
		if perr == nil && pr.ObjectNumber.Value() == ref.ObjectNumber.Value() {
			return p
		}
	}
	return 0
}

// setCatalogName writes a /Name-valued catalog entry.
func setCatalogName(t *testing.T, pdf []byte, key, value string) []byte {
	t.Helper()
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		root, rerr := ctx.XRefTable.Catalog()
		if rerr != nil {
			return rerr
		}
		root[key] = types.Name(value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// catalogName reads a /Name-valued catalog entry, or "" when absent.
func catalogName(t *testing.T, pdf []byte, key string) string {
	t.Helper()
	ctx := readCtx(t, pdf)
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	return nameVal(root, key)
}

// outlineCounts maps each outline item's title to its /Count, with the ROOT under "". A title absent
// from the map carries no /Count at all, which is what a leaf must look like.
func outlineCounts(t *testing.T, pdf []byte) map[string]int {
	t.Helper()
	ctx := readCtx(t, pdf)
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	rootDic := derefDict(xt, root["Outlines"])
	if rootDic == nil {
		return out
	}
	if c, ok := intVal(xt, rootDic["Count"]); ok {
		out[""] = c
	}
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > 20 {
			return
		}
		for cur := o; cur != nil; {
			d := derefDict(xt, cur)
			if d == nil {
				return
			}
			title := ""
			if s, serr := xt.DereferenceText(d["Title"]); serr == nil {
				title = s
			}
			if c, ok := intVal(xt, d["Count"]); ok {
				out[title] = c
			}
			walk(d["First"], depth+1)
			cur = d["Next"]
		}
	}
	walk(rootDic["First"], 0)
	return out
}

// countOutlineKey counts the outline items carrying the given key.
func countOutlineKey(t *testing.T, pdf []byte, key string) int {
	t.Helper()
	ctx := readCtx(t, pdf)
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	rootDic := derefDict(xt, root["Outlines"])
	if rootDic == nil {
		return 0
	}
	n := 0
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > 20 {
			return
		}
		for cur := o; cur != nil; {
			d := derefDict(xt, cur)
			if d == nil {
				return
			}
			if _, has := d[key]; has {
				n++
			}
			walk(d["First"], depth+1)
			cur = d["Next"]
		}
	}
	walk(rootDic["First"], 0)
	return n
}

// closeOutlineItem negates the named item's /Count, which is how the format records "collapsed".
// pdfcpu's AddBookmarks only ever writes open items, so a closed one has to be made by hand.
func closeOutlineItem(t *testing.T, pdf []byte, title string) []byte {
	t.Helper()
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, rerr := xt.Catalog()
		if rerr != nil {
			return rerr
		}
		rootDic := derefDict(xt, root["Outlines"])
		if rootDic == nil {
			t.Fatal("the fixture has no outline to close an item in")
		}
		found := false
		var walk func(o types.Object, depth int)
		walk = func(o types.Object, depth int) {
			if depth > 20 {
				return
			}
			for cur := o; cur != nil; {
				d := derefDict(xt, cur)
				if d == nil {
					return
				}
				if s, serr := xt.DereferenceText(d["Title"]); serr == nil && s == title {
					if c, ok := intVal(xt, d["Count"]); ok && c > 0 {
						d["Count"] = types.Integer(-c)
						found = true
					}
				}
				walk(d["First"], depth+1)
				cur = d["Next"]
			}
		}
		walk(rootDic["First"], 0)
		if !found {
			t.Fatalf("no open outline item titled %q to close", title)
		}
		// The root counts only what is VISIBLE, so collapsing an item takes its descendants out of
		// the root's total. Leaving the root's own count alone would hand the test a fixture that is
		// already wrong about the property it grades.
		rootDic["Count"] = types.Integer(rootVisible(xt, rootDic))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// rootVisible recomputes an outline root's visible-item count from the tree below it, so a fixture
// edited by hand stays self-consistent.
func rootVisible(xt *model.XRefTable, rootDic types.Dict) int {
	var count func(o types.Object, depth int) int
	count = func(o types.Object, depth int) int {
		if depth > 20 {
			return 0
		}
		n := 0
		for cur := o; cur != nil; {
			d := derefDict(xt, cur)
			if d == nil {
				return n
			}
			n++
			if c, ok := intVal(xt, d["Count"]); ok && c > 0 {
				n += count(d["First"], depth+1)
			}
			cur = d["Next"]
		}
		return n
	}
	return count(rootDic["First"], 0)
}

// actionBookmarks appends two items whose navigation lives in /A rather than /Dest: a /GoTo at
// page 3 that CHAINS to JavaScript, and a /Launch that is not navigation at all.
func actionBookmarks(t *testing.T, pdf []byte) []byte {
	t.Helper()
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, rerr := xt.Catalog()
		if rerr != nil {
			return rerr
		}
		rootDic := derefDict(xt, root["Outlines"])
		if rootDic == nil {
			t.Fatal("the fixture has no outline to append to")
		}
		last, lerr := xt.DereferenceDict(rootDic["Last"])
		if lerr != nil || last == nil {
			t.Fatalf("the fixture's outline has no last item: %v", lerr)
		}
		page3, perr := ctx.PageDictIndRef(3)
		if perr != nil {
			return perr
		}
		page2, perr := ctx.PageDictIndRef(2)
		if perr != nil {
			return perr
		}
		rootRef, _ := root["Outlines"].(types.IndirectRef)

		jumper := types.Dict{
			"Title":  types.StringLiteral("Jumper"),
			"Parent": rootRef,
			"A": types.Dict{
				"S": types.Name("GoTo"),
				"D": types.Array{*page3, types.Name("Fit")},
				// A benign head chaining to a risky action: the shape `eachAction` exists for.
				"Next": types.Dict{"S": types.Name("JavaScript"), "JS": types.StringLiteral("app.alert(1)")},
			},
		}
		jumperRef, jerr := xt.IndRefForNewObject(jumper)
		if jerr != nil {
			return jerr
		}
		launcher := types.Dict{
			"Title":  types.StringLiteral("Launcher"),
			"Parent": rootRef,
			"Prev":   *jumperRef,
			"A":      types.Dict{"S": types.Name("Launch"), "F": types.StringLiteral("/bin/sh")},
			// A destination it must NOT be rescued by: /Launch is not navigation, so this item goes
			// even though the page it names survives.
			"SE": *page2,
		}
		launcherRef, lerr2 := xt.IndRefForNewObject(launcher)
		if lerr2 != nil {
			return lerr2
		}
		jumper["Next"] = *launcherRef
		last["Next"] = *jumperRef
		jumper["Prev"] = rootDic["Last"]
		rootDic["Last"] = *launcherRef
		rootDic["Count"] = types.Integer(rootVisible(xt, rootDic))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
