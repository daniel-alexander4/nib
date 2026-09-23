package uacheck

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// P03's phase-close review found four inputs of a few hundred bytes that no bound in the checker stopped: a
// pass-through element listing itself, a chain of shared pass-through elements doubling at every level, form
// XObjects fanning out inside one another, and many tables each asking for the largest grid one may. Each ran
// inside a rule with no deadline, reached from the accessibility report on whatever document is open. Every one
// must now finish promptly and answer CannotCheck naming why — never a verdict over what it stopped reading.

// withinSeconds fails the test when f has not returned after s seconds — from a deadline, not after f returns,
// because the defects this file pins ran forever and a check made on return would wait with them.
func withinSeconds(t *testing.T, s float64, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(time.Duration(s * float64(time.Second))):
		t.Fatalf("%s had not finished after %.0fs", what, s)
	}
}

func TestAPassThroughElementListingItselfIsCannotCheck(t *testing.T) {
	pdf := buildPDF(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		7:  "<< /Type /StructTreeRoot /K 21 0 R >>",
		21: "<< /Type /StructElem /S /Div /P 7 0 R /K [21 0 R 21 0 R] >>",
	})
	var got Result
	withinSeconds(t, 5, "a Div listing itself twice", func() { got = verdictOf(t, pdf, "7.4.4 t1") })
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "lists itself") {
		t.Fatalf("7.4.4 t1 on a Div listing itself reports %v (%s), want CannotCheck naming the loop", got.Verdict, got.Why)
	}
}

// sharedDivs is a chain of n pass-through Divs, each listing the next twice, ending at a TD: expanding it lists
// the TD 2^n times, which is what veraPDF would count and what nib may not try to.
func sharedDivs(n int) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		7: "<< /Type /StructTreeRoot /K 100 0 R >>",
	}
	for i := 0; i < n; i++ {
		objs[100+i] = fmt.Sprintf("<< /Type /StructElem /S /Div /P 7 0 R /K [%d 0 R %d 0 R] >>", 101+i, 101+i)
	}
	objs[100+n] = "<< /Type /StructElem /S /TD /P 7 0 R >>"
	return buildPDF(objs)
}

func TestAChainOfSharedPassThroughElementsIsCannotCheckNotAHang(t *testing.T) {
	// The control: a short chain is read in full, so the TD is reached and its parent clause answered.
	if got := verdictOf(t, sharedDivs(4), "7.4.4 t1"); got.Verdict != Pass {
		t.Fatalf("control: four shared Divs report %v (%s) for 7.4.4 t1, want Pass — the fixture does not read", got.Verdict, got.Why)
	}
	var got Result
	withinSeconds(t, 10, "thirty shared Divs", func() { got = verdictOf(t, sharedDivs(30), "7.4.4 t1") })
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "expand past") {
		t.Fatalf("thirty shared Divs report %v (%s) for 7.4.4 t1, want CannotCheck naming the expansion budget", got.Verdict, got.Why)
	}
}

func TestAParentLoopIsCannotCheckNeverAMissingParent(t *testing.T) {
	doc := func(p13 string) []byte {
		return buildPDF(map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
			7:  "<< /Type /StructTreeRoot /K 11 0 R >>",
			11: "<< /Type /StructElem /S /TD /P 12 0 R >>",
			12: "<< /Type /StructElem /S /Div /P 13 0 R >>",
			13: "<< /Type /StructElem /S /Div " + p13 + " >>",
		})
	}
	// The control: the same climb ending without a /P is the measured failure (7.2-9 fails a TD with no /P).
	if got := verdictOf(t, doc(""), "7.2 t9"); got.Verdict != Fail || !strings.Contains(got.Why, "no /P") {
		t.Fatalf("control: a climb ending in no /P reports %v (%s), want Fail naming the missing /P", got.Verdict, got.Why)
	}
	var got Result
	withinSeconds(t, 5, "a /P loop", func() { got = verdictOf(t, doc("/P 12 0 R"), "7.2 t9") })
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "loop") {
		t.Fatalf("a /P loop between two Divs reports %v (%s) for 7.2 t9, want CannotCheck naming the loop", got.Verdict, got.Why)
	}
}

// formFanOut is a page drawing form X0, each form Xi drawing X(i+1) ten times, the last drawing one rectangle
// (or nothing, when empty): 10^depth walks from a file of a few kilobytes.
func formFanOut(depth int, empty bool) []byte {
	stream := func(body, extra string) string {
		return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", extra, len(body), body)
	}
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X0 100 0 R >> >> /Contents 4 0 R >>",
		4: stream("/X0 Do", ""),
		7: "<< /Type /StructTreeRoot >>",
	}
	for i := 0; i < depth; i++ {
		body := strings.Repeat(fmt.Sprintf("/X%d Do ", i+1), 10)
		res := fmt.Sprintf("/Resources << /XObject << /X%d %d 0 R >> >>", i+1, 101+i)
		objs[100+i] = stream(body, "/Type /XObject /Subtype /Form /BBox [0 0 10 10] "+res)
	}
	leaf := "0 0 1 1 re f"
	if empty {
		leaf = "q Q"
	}
	objs[100+depth] = stream(leaf, "/Type /XObject /Subtype /Form /BBox [0 0 10 10]")
	return buildPDF(objs)
}

func TestFormsFanningOutAreCannotCheckNotAnOutOfMemory(t *testing.T) {
	// The control: two levels are walked in full, and the untagged rectangles they draw fail 7.1 t3.
	if got := verdictOf(t, formFanOut(2, false), "7.1 t3"); got.Verdict != Fail {
		t.Fatalf("control: two levels of forms report %v (%s) for 7.1 t3, want Fail — the fixture draws nothing", got.Verdict, got.Why)
	}
	var got Result
	withinSeconds(t, 10, "seven levels of fan-out ten", func() { got = verdictOf(t, formFanOut(7, false), "7.1 t3") })
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "enters form XObjects more than") {
		t.Fatalf("seven levels of form fan-out report %v (%s) for 7.1 t3, want CannotCheck naming the walk budget", got.Verdict, got.Why)
	}
}

// The two content budgets each hold alone: the re-review found either one removable with the suite green, the
// other catching the fan-out in its place.
func TestEachContentBudgetHoldsOnItsOwn(t *testing.T) {
	// Walks with nothing drawn: 10^7 form entries and not one event, so only the walk budget can stop it.
	var got Result
	withinSeconds(t, 10, "a fan-out that draws nothing", func() { got = verdictOf(t, formFanOut(7, true), "7.1 t3") })
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "enters form XObjects more than") {
		t.Fatalf("a fan-out drawing nothing reports %v (%s), want CannotCheck naming the walk budget", got.Verdict, got.Why)
	}
	// Events with few walks: one form of twenty rectangles drawn 60,000 times — under the walk budget, and
	// 1.2 million drawing operators, past the event budget.
	body := strings.Repeat("0 0 1 1 re f ", 20)
	page := strings.Repeat("/X0 Do ", 60000)
	pdf := buildPDF(map[int]string{
		1:   "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:   "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:   "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /X0 100 0 R >> >> /Contents 4 0 R >>",
		4:   fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(page), page),
		7:   "<< /Type /StructTreeRoot >>",
		100: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length %d >>\nstream\n%s\nendstream", len(body), body),
	})
	withinSeconds(t, 20, "1.2 million drawing operators", func() { got = verdictOf(t, pdf, "7.1 t3") })
	if got.Verdict != CannotCheck || !strings.Contains(got.Why, "drawing operators") {
		t.Fatalf("1.2 million drawing operators report %v (%s), want CannotCheck naming the event budget", got.Verdict, got.Why)
	}
}

// selfListingUnder is a document whose one Table holds a Div that lists itself — the containment door's and the
// table layout's kids, which the kid-sequence fixtures above do not reach.
func selfListingUnder() []byte {
	return buildPDF(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		7:  "<< /Type /StructTreeRoot /K 20 0 R >>",
		20: "<< /Type /StructElem /S /Table /P 7 0 R /K 21 0 R >>",
		21: "<< /Type /StructElem /S /Div /P 20 0 R /K [21 0 R 22 0 R] >>",
		22: "<< /Type /StructElem /S /TR /P 21 0 R /K 23 0 R >>",
		23: "<< /Type /StructElem /S /TD /P 22 0 R >>",
	})
}

func TestUnreadableKidsAreCannotCheckInEveryDoor(t *testing.T) {
	pdf := selfListingUnder()
	var tableKids []string
	for _, c := range containmentMatrix {
		if c.subject == "Table" && c.kids != nil {
			tableKids = append(tableKids, c.clause)
		}
	}
	if len(tableKids) == 0 {
		t.Fatal("the matrix has no Table-kids row — the fixture's subject reaches no containment clause")
	}
	for _, clause := range append(tableKids, "7.2 t15", "7.2 t43") {
		if got := verdictOf(t, pdf, clause); got.Verdict != CannotCheck || !strings.Contains(got.Why, "lists itself") {
			t.Errorf("%s over a Table whose Div lists itself = %v (%s), want CannotCheck naming the loop", clause, got.Verdict, got.Why)
		}
	}
}

// A depth refusal belongs to the walk, not to the element: a Div met 40 levels into a walk from a Table above a
// 70-Div chain answers for its OWN chain whichever was asked first (the fix pass's re-review found the first
// memo answering "deeper than 64" for a Div whose chain was 31 deep).
func TestADepthRefusalDoesNotDependOnWhichElementWasAskedFirst(t *testing.T) {
	build := func() (*Document, types.Dict, types.Dict) {
		objs := map[int]string{
			1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R >>",
			2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
			7:  "<< /Type /StructTreeRoot /K [20 0 R 140 0 R] >>",
			20: "<< /Type /StructElem /S /Table /P 7 0 R /K 100 0 R >>",
		}
		for i := 0; i < 70; i++ {
			objs[100+i] = fmt.Sprintf("<< /Type /StructElem /S /Div /P 7 0 R /K %d 0 R >>", 101+i)
		}
		objs[170] = "<< /Type /StructElem /S /TR /P 7 0 R /K 171 0 R >>"
		objs[171] = "<< /Type /StructElem /S /TD /P 170 0 R >>"
		d, err := open(buildPDF(objs))
		if err != nil {
			t.Fatal(err)
		}
		return d, d.dict(types.IndirectRef{ObjectNumber: types.Integer(20)}), d.dict(types.IndirectRef{ObjectNumber: types.Integer(140)})
	}
	d, table, div40 := build()
	if _, why := d.elementKids(table); !strings.Contains(why, "deeper than") {
		t.Fatalf("the Table above seventy Divs lists its kids (%q); want the depth refusal", why)
	}
	kids, why := d.elementKids(div40)
	if why != "" || len(kids) != 1 {
		t.Fatalf("after the Table was asked, the 40th Div's own 30-deep chain answers %d kids (%q); want its one TR", len(kids), why)
	}
	d, table, div40 = build()
	if kids, why := d.elementKids(div40); why != "" || len(kids) != 1 {
		t.Fatalf("asked first, the 40th Div answers %d kids (%q); want its one TR", len(kids), why)
	}
	if _, why := d.elementKids(table); !strings.Contains(why, "deeper than") {
		t.Fatalf("after the Div was asked, the Table lists its kids (%q); want the depth refusal", why)
	}
}

func TestTheDocumentsTablesShareOneSlotBudget(t *testing.T) {
	// Each table asks for exactly the per-table cap; five of them together pass the document's.
	table := fmt.Sprintf("Table(TR(TD!r%d))", maxTableSlots)
	doc := func(n int) *Document {
		d, err := open(treeDoc("", "Document("+strings.TrimSuffix(strings.Repeat(table+",", n), ",")+")"))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	layouts, _, _ := doc(3).tablesIn()
	for i, l := range layouts {
		if strings.Contains(l.unlayable, "together") {
			t.Fatalf("control: table %d of three is refused by the document's budget (%s)", i+1, l.unlayable)
		}
	}
	var last *tableLayout
	withinSeconds(t, 10, "five tables at the per-table cap", func() {
		layouts, _, _ = doc(5).tablesIn()
		last = layouts[len(layouts)-1]
	})
	if len(layouts) != 5 {
		t.Fatalf("five tables laid out as %d — the fixture does not carry its subject", len(layouts))
	}
	if !strings.Contains(last.unlayable, "together") {
		t.Fatalf("the fifth table at the per-table cap is laid out (%q); want it refused by the document's slot budget", last.unlayable)
	}
}
