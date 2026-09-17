package pdfops

import (
	"fmt"
	"testing"
)

// `/pending 529`'s readers — a duplicated page carries its OWN grouping elements, and the tree reads
// page then page.
//
// # What the entry supposed, and what the measurement found
//
// The entry read the defect as lost SEQUENCE: *"a screen reader on a duplicated page hears the
// original's paragraph, then the copy's, rather than the whole page and then its copy"*, with
// *"every element anchored, every MCID resolving and nothing describing content that is not there"*.
// Measured on nib's own Markdown conversion and on veraPDF's `7.5 Tables/7.5-t01-pass-a.pdf`, it is
// worse than that on both counts, because a `/ParentTree` row names LEAVES and the copy started
// there:
//
//   - the source's one `/L` and its three `/LI` were **not copied at all**. The copies of the `/Lbl`
//     and `/LBody` leaves went under the ORIGINAL page's `/LI`, so each original `/LI` came out with
//     two `/Lbl` and two `/LBody` — its text reading `"••first item of the list…"`, two bullets —
//     and page 2 had no list structure whatsoever.
//   - the table is the same shape and louder: the header `/TR` of a five-column table came out with
//     **ten** cells, reading `"Index Index Failure Condition Failure Condition Section Section …"`,
//     and page 2 had no `/Table` and no `/TR`, so a reader on the duplicate hears no table at all.
//
// So the original page's grouping elements described content on ANOTHER page, and the duplicate was
// not a list or a table. Both outputs were `carried` with zero completeness defects and zero orphan
// pages — nothing in this package compares a row's width to its table's, and nothing asks whether a
// `/LI` has more than one `/Lbl`.

const listAndCloseMarkdown = "# Heading One\n\nA first paragraph of prose that sits under the heading.\n\n" +
	"- first item of the list\n- second item of the list\n- third item of the list\n\n" +
	"A closing paragraph after the list.\n"

// kindsOf counts a structure tree's elements by `/S`, per page.
func kindsOf(st StructureTree) map[string]map[int]int {
	out := map[string]map[int]int{}
	for _, e := range st.Elements {
		if out[e.Kind] == nil {
			out[e.Kind] = map[int]int{}
		}
		out[e.Kind][e.Page]++
	}
	return out
}

// TestADuplicatedPageCARRIESItsOwnGroupingElements is the central reader: the copy is a list, and
// the original is still one.
//
// Two assertions, and they fail for different reasons. The FIRST is that page 2 has an `/L` of its
// own — it goes red where nothing climbs above the row's leaves. The SECOND is that no `/LI`
// anywhere holds more than one `/Lbl` or more than one `/LBody` — it goes red where the copies land
// under the original's items, which is a different defect: the first is the duplicate missing
// structure, the second is the ORIGINAL acquiring structure that describes another page.
func TestADuplicatedPageCARRIESItsOwnGroupingElements(t *testing.T) {
	src, err := tagMarkdown([]byte(listAndCloseMarkdown), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	before, err := ReadStructure(src)
	if err != nil {
		t.Fatal(err)
	}
	// The stimulus has to exist before anything about it can be graded.
	if n := len(kindsOf(before)["LI"]); n == 0 || kindsOf(before)["LI"][1] != 3 {
		t.Fatalf("setup: the source has %v list items on page 1, and this needs three — the "+
			"grouping elements are what the defect is about", kindsOf(before)["LI"])
	}
	out, err := DuplicatePage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v, defects, orphans, _ := carryOf(t, out); v != "carried" || len(defects) > 0 || len(orphans) > 0 {
		t.Fatalf("the duplicate is %s with %d defect(s) and %d orphan page(s): %v %v. Nothing below "+
			"grades a tree that was not carried", v, len(defects), len(orphans), defects, orphans)
	}
	st, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	kinds := kindsOf(st)
	if kinds["L"][2] != 1 || kinds["LI"][2] != 3 {
		t.Errorf("the duplicated page has %d /L and %d /LI of its own, and it needs one and three. "+
			"A `/ParentTree` row names leaves, so copying only what the row names leaves the copy "+
			"with no list structure at all and hangs its items under the ORIGINAL page's /LI",
			kinds["L"][2], kinds["LI"][2])
	}
	for _, e := range st.Elements {
		if e.Kind != "LI" {
			continue
		}
		lbl, body := 0, 0
		for _, ki := range e.Kids {
			switch st.Elements[ki].Kind {
			case "Lbl":
				lbl++
			case "LBody":
				body++
			}
		}
		if lbl > 1 || body > 1 {
			t.Errorf("/LI %d (page %d) holds %d /Lbl and %d /LBody, reading %q. A list item has at "+
				"most one of each; more than one means the duplicate's leaves were attached to the "+
				"original's item, so an item on page %d now describes content on another page",
				e.ID, e.Page, lbl, body, e.Text, e.Page)
		}
	}
}

// TestADuplicatedPagesStructureReadsPageThenPage is the sequencing half, and it is the one assertion
// the entry actually predicted.
//
// It is stated as *the tree's reading order never goes back to an earlier page*, which is one
// condition over the whole walk rather than a list of expected kinds: the interleaved order
// `H1(p1) H1(p2) P(p1) P(p2)` violates it at the third element whatever the document holds.
//
// **Three repeats are driven as well as one**, because they fail differently. Each copy going
// immediately after its ORIGINAL rather than after the previous COPY put the repeats in reverse —
// measured on `Collect(["1","1","1"])`, `/K=[(32) (66) (64) (65) (67)]` — which a two-page case
// cannot see.
func TestADuplicatedPagesStructureReadsPageThenPage(t *testing.T) {
	src, err := tagMarkdown([]byte(listAndCloseMarkdown), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		build func() ([]byte, error)
		pages int
	}{
		{"DuplicatePage", func() ([]byte, error) { return DuplicatePage(src, 1) }, 2},
		{"Collect 1,1,1", func() ([]byte, error) { return Collect(src, []string{"1", "1", "1"}) }, 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, berr := c.build()
			if berr != nil {
				t.Fatal(berr)
			}
			st, rerr := ReadStructure(out)
			if rerr != nil {
				t.Fatal(rerr)
			}
			seen, high, prev := map[int]bool{}, 0, 0
			var order []string
			for _, e := range st.Elements {
				if e.Page == 0 {
					continue // a grouping element that names no page of its own says nothing here
				}
				seen[e.Page] = true
				if e.Page > high {
					high = e.Page
				}
				order = append(order, fmt.Sprintf("%s(p%d)", e.Kind, e.Page))
				if e.Page < prev {
					t.Errorf("element %d (/%s) is on page %d and the element before it was on page "+
						"%d, so the tree goes BACK a page mid-walk. A reader hears the original's "+
						"element and then its copy rather than the whole page and then its copy.\n"+
						"order: %v", e.ID, e.Kind, e.Page, prev, order)
					return
				}
				prev = e.Page
			}
			if len(seen) != c.pages || high != c.pages {
				t.Fatalf("the walk saw pages %v (highest %d) and this drives %d, so the ordering "+
					"condition held over fewer pages than it was written for", seen, high, c.pages)
			}
		})
	}
}

// tableFixtureObjects is one page holding a two-by-two `/Table` under a `/Document`, hand-written so
// every byte is readable — `corpus_test.go`'s rule. The corpus file the defect was first measured on
// (`7.5 Tables/7.5-t01-pass-a.pdf`) is not committed and is not on a fresh clone.
func tableFixtureObjects() map[int]string {
	content := ""
	for i := 0; i < 4; i++ {
		content += fmt.Sprintf("/TD <</MCID %d>> BDC\nBT /F1 12 Tf 72 %d Td (ZZCELL%d) Tj ET\nEMC\n",
			i, 700-20*i, i)
	}
	return map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> " +
			"/Contents 4 0 R /StructParents 0 >>",
		4:  streamObj(content),
		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		30: "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R /ParentTreeNextKey 1 >>",
		31: "<< /Type /StructElem /S /Document /P 30 0 R /K [50 0 R] >>",
		39: "<< /Nums [0 [53 0 R 54 0 R 55 0 R 56 0 R]] >>",
		50: "<< /Type /StructElem /S /Table /P 31 0 R /Pg 3 0 R /K [51 0 R 52 0 R] >>",
		51: "<< /Type /StructElem /S /TR /P 50 0 R /Pg 3 0 R /K [53 0 R 54 0 R] >>",
		52: "<< /Type /StructElem /S /TR /P 50 0 R /Pg 3 0 R /K [55 0 R 56 0 R] >>",
		53: "<< /Type /StructElem /S /TH /P 51 0 R /Pg 3 0 R /K [0] >>",
		54: "<< /Type /StructElem /S /TH /P 51 0 R /Pg 3 0 R /K [1] >>",
		55: "<< /Type /StructElem /S /TD /P 52 0 R /Pg 3 0 R /K [2] >>",
		56: "<< /Type /StructElem /S /TD /P 52 0 R /Pg 3 0 R /K [3] >>",
	}
}

// TestADuplicatedTableIsATableAndTheOriginalKeepsItsWIDTH is the table half of the central reader,
// and it grades one thing the list case cannot: the `/Document` is NOT duplicated with it.
//
// A `/Document` is *"a complete document"* (ISO 32000-1 table 333). The climb stops below it, so a
// file in which the user repeated a page still holds one document rather than two — while the
// `/Table`, which is a division of content, is repeated along with the content.
func TestADuplicatedTableIsATableAndTheOriginalKeepsItsWIDTH(t *testing.T) {
	src := assembleFixture(tableFixtureObjects())
	if v, d, _, _ := carryOf(t, src); v != "carried" || len(d) > 0 {
		t.Fatalf("setup: the fixture is %s with %d defect(s): %v", v, len(d), d)
	}
	out, err := DuplicatePage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v, defects, orphans, _ := carryOf(t, out); v != "carried" || len(defects) > 0 || len(orphans) > 0 {
		t.Fatalf("the duplicate is %s with %d defect(s) and %d orphan page(s): %v %v",
			v, len(defects), len(orphans), defects, orphans)
	}
	st, err := ReadStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	kinds := kindsOf(st)
	if kinds["Table"][1] != 1 || kinds["Table"][2] != 1 {
		t.Errorf("the output holds %v /Table, and it needs one per page. A duplicate with no /Table "+
			"of its own is a page on which a reader hears no table", kinds["Table"])
	}
	if n := kinds["Document"][1] + kinds["Document"][2]; n != 1 {
		t.Errorf("the output holds %d /Document elements and a file holds one. Duplicating a page "+
			"repeats a division of content, not the document the content is in", n)
	}
	for _, e := range st.Elements {
		if e.Kind != "TR" {
			continue
		}
		if len(e.Kids) != 2 {
			t.Errorf("/TR %d (page %d) holds %d cells and the table has two columns, reading %q. A "+
				"row wider than its table destroys every header-to-cell association a reader "+
				"computes, and no gate in this package compares the two", e.ID, e.Page, len(e.Kids), e.Text)
		}
	}
	if kinds["TR"][1] != 2 || kinds["TR"][2] != 2 {
		t.Errorf("the output holds %v rows, and it needs two per page", kinds["TR"])
	}
}

// spanningTableFixtureObjects is the case `/pending 529` said the alternative fix would break: a
// `/Table` whose rows are on two pages, so no ancestor of a row is wholly on the duplicated one.
func spanningTableFixtureObjects() map[int]string {
	row := func(a, b int) string {
		return fmt.Sprintf("/TD <</MCID 0>> BDC\nBT /F1 12 Tf 72 700 Td (ZZCELL%d) Tj ET\nEMC\n"+
			"/TD <</MCID 1>> BDC\nBT /F1 12 Tf 72 680 Td (ZZCELL%d) Tj ET\nEMC\n", a, b)
	}
	return map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R /Lang (en-GB) >>",
		2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> " +
			"/Contents 4 0 R /StructParents 0 >>",
		4: streamObj(row(0, 1)),
		5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 11 0 R >> >> " +
			"/Contents 6 0 R /StructParents 1 >>",
		6:  streamObj(row(2, 3)),
		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		30: "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R /ParentTreeNextKey 2 >>",
		31: "<< /Type /StructElem /S /Document /P 30 0 R /K [50 0 R] >>",
		39: "<< /Nums [0 [53 0 R 54 0 R] 1 [55 0 R 56 0 R]] >>",
		// The `/Table` deliberately has no `/Pg`: its rows are on two different pages.
		50: "<< /Type /StructElem /S /Table /P 31 0 R /K [51 0 R 52 0 R] >>",
		51: "<< /Type /StructElem /S /TR /P 50 0 R /Pg 3 0 R /K [53 0 R 54 0 R] >>",
		52: "<< /Type /StructElem /S /TR /P 50 0 R /Pg 5 0 R /K [55 0 R 56 0 R] >>",
		53: "<< /Type /StructElem /S /TH /P 51 0 R /Pg 3 0 R /K [0] >>",
		54: "<< /Type /StructElem /S /TH /P 51 0 R /Pg 3 0 R /K [1] >>",
		55: "<< /Type /StructElem /S /TD /P 52 0 R /Pg 5 0 R /K [0] >>",
		56: "<< /Type /StructElem /S /TD /P 52 0 R /Pg 5 0 R /K [1] >>",
	}
}

// TestATableSPANNINGThePageCarriesItsRowsRatherThanRefusing is the case that decides the design.
//
// `/pending 529`'s alternative was *"clone the highest element whose subtree lies wholly within the
// duplicated page, and REFUSE the carry where no such element exists (a table spanning the page)"* —
// which is this document, and refusing it means a document with one spanning table stops carrying
// structure at all. The climb needs no refusal: it is monotone and floored at the leaf, so here it
// stops at the `/TR` that IS wholly on the duplicated page. The copy is a third row of the one
// table, in page order, and every row still holds its two cells.
func TestATableSPANNINGThePageCarriesItsRowsRatherThanRefusing(t *testing.T) {
	src := assembleFixture(spanningTableFixtureObjects())
	if v, d, _, _ := carryOf(t, src); v != "carried" || len(d) > 0 {
		t.Fatalf("setup: the fixture is %s with %d defect(s): %v", v, len(d), d)
	}
	out, err := DuplicatePage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, defects, orphans, _ := carryOf(t, out)
	if v != "carried" || len(defects) > 0 || len(orphans) > 0 {
		t.Fatalf("a document whose table spans the duplicated page came out %s with %d defect(s) "+
			"and %d orphan page(s): %v %v. Refusing here is what the entry's alternative would "+
			"have done, and it costs the whole tree", v, len(defects), len(orphans), defects, orphans)
	}
	st, rerr := ReadStructure(out)
	if rerr != nil {
		t.Fatal(rerr)
	}
	kinds := kindsOf(st)
	if n := kinds["Table"][0] + kinds["Table"][1] + kinds["Table"][2] + kinds["Table"][3]; n != 1 {
		t.Errorf("the output holds %d /Table and it needs one: the climb may not copy an ancestor "+
			"whose subtree reaches a page the repeat is not making", n)
	}
	var rows []int
	for _, e := range st.Elements {
		if e.Kind != "TR" {
			continue
		}
		rows = append(rows, e.Page)
		if len(e.Kids) != 2 {
			t.Errorf("/TR %d (page %d) holds %d cells and the table has two columns, reading %q",
				e.ID, e.Page, len(e.Kids), e.Text)
		}
	}
	// Pages 1 and 2 are the original and its copy; page 3 is the table's continuation. The copy's
	// row belongs between them, which is where the block lands when it goes after the last row the
	// repeat is copying rather than at the end of the table.
	if fmt.Sprint(rows) != fmt.Sprint([]int{1, 2, 3}) {
		t.Errorf("the table's rows read %v and they should read [1 2 3]: the duplicate's row belongs "+
			"between the page it copies and the page that continues the table", rows)
	}
}
