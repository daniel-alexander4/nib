package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"sync"
	"testing"

	"nib/internal/testpdf"
)

// P03.S04's acceptance, and the first tests in this plan that can only be written
// because a document can describe ITSELF (docResponse takes its document).
//
// None of this is reachable through the GUI yet: the real app cannot open a second
// document until P03.S05 lands arrivals, so tier 3 cannot exercise a single clause
// here. These drive the registry directly and say so, rather than implying a
// coverage the harness does not have.

// evictionServer builds a server holding `count` documents, each with a history of
// `perDoc` bytes, under a budget small enough to run in milliseconds. The first
// document is active.
func evictionServer(t *testing.T, budget int) *Server {
	t.Helper()
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	s.maxHistoryBytes = budget
	return s
}

// pushHistory puts n entries of size bytes onto doc's undo stack directly, without
// going through commitMutation — so a test can set up an over-budget state without
// the trim it is about to assert running during setup.
func pushHistory(doc *document, n, size int) {
	for i := 0; i < n; i++ {
		doc.undo = append(doc.undo, make([]byte, size))
	}
}

// THE slice's headline clause: two documents past the budget evicts the inactive
// one, and the evicted document reports it ITSELF.
func TestBudgetEvictsInactiveDocumentWhole(t *testing.T) {
	const budget = 64 << 10
	s := evictionServer(t, budget)
	active := s.activeDoc()

	otherID := addDocument(s, []byte("second"))
	other := documentByID(s, otherID)
	if other == nil || other == active {
		t.Fatal("setup: the second document is not a distinct registry entry")
	}
	pushHistory(other, 4, budget/2) // the inactive document alone is over budget

	// The stimulus, asserted before the response. Without this, every assertion
	// below is satisfied by a document that never had a history to lose — the
	// eviction would "pass" against a no-op.
	s.mu.Lock()
	before := s.historyBytesLocked()
	s.mu.Unlock()
	if before <= budget {
		t.Fatalf("setup: %d bytes is not over the %d budget — nothing would be evicted", before, budget)
	}
	if s.historyEvictions != 0 {
		t.Fatalf("setup: %d evictions before anything was trimmed", s.historyEvictions)
	}

	// Now grow the ACTIVE document, which is what runs the budget.
	s.commitMutation(active, []byte("x"), []byte("y"))

	s.mu.Lock()
	after := s.historyBytesLocked()
	s.mu.Unlock()
	if after > budget {
		t.Errorf("history holds %d bytes, over the %d budget — eviction did not converge", after, budget)
	}
	if s.historyEvictions != 1 {
		t.Errorf("historyEvictions = %d, want 1", s.historyEvictions)
	}

	// The INACTIVE document lost its history...
	if len(other.undo) != 0 || len(other.redo) != 0 {
		t.Errorf("the inactive document kept its history: undo=%d redo=%d", len(other.undo), len(other.redo))
	}
	// ...and the ACTIVE one kept the entry it just made. Evicting the document the
	// user is looking at, to make room for one they are not, is the wrong trade and
	// this is what would catch it.
	if len(active.undo) == 0 {
		t.Error("the ACTIVE document lost its history — inactive documents must be evicted first")
	}
	if active.historyEvicted {
		t.Error("the active document was marked evicted")
	}
}

// Found by reviewing the slice's own diff, not by the tests above — which all grow
// the ACTIVE document and so cannot tell "the document that grew" apart from "the
// document the user is looking at".
//
// This phase exists precisely so an operation can be addressed to a document the
// user is not looking at. When one is, the document that grew is inactive and the
// active document is somebody else — and an eviction pass that protects the grown
// one and treats the rest as fair game throws away the history of the tab actually
// on screen. That is the acceptance clause inverted, and it would have shipped
// looking correct.
func TestEvictionSparesTheActiveDocumentWhenAnInactiveOneGrows(t *testing.T) {
	const budget = 64 << 10
	s := evictionServer(t, budget)
	active := s.activeDoc()
	pushHistory(active, 1, 1024) // the user's own work, on screen

	grownID := addDocument(s, []byte("addressed"))
	grown := documentByID(s, grownID)

	bystanderID := addDocument(s, []byte("bystander"))
	bystander := documentByID(s, bystanderID)
	pushHistory(bystander, 4, budget/2)

	// The stimulus: over budget before the operation, and the active document has a
	// history that eviction could take. Without both, "the active one survived" is
	// satisfied by a document that had nothing to lose.
	s.mu.Lock()
	before := s.historyBytesLocked()
	s.mu.Unlock()
	if before <= budget {
		t.Fatalf("setup: %d bytes is not over the %d budget", before, budget)
	}
	if len(active.undo) == 0 {
		t.Fatal("setup: the active document has no history, so sparing it would prove nothing")
	}

	// The operation is addressed to `grown`, which is NOT the active document.
	if grown == active {
		t.Fatal("setup: the grown document is the active one — the case under test is the other one")
	}
	s.commitMutation(grown, []byte("x"), []byte("y"))

	if len(active.undo) == 0 || active.historyEvicted {
		t.Error("the ACTIVE document's history was evicted while an inactive document grew — the user loses the history of the tab they are looking at, to make room for one they are not")
	}
	if len(bystander.undo) != 0 {
		t.Error("the uninvolved inactive document kept its history — it should be evicted first")
	}
	if !bystander.historyEvicted {
		t.Error("the evicted bystander is not reporting it")
	}
}

// The clause the plan-review pin added, and it needs its own test because
// `canUndo:false` is what BOTH states look like. Asserting only the effect would
// discharge the pin with the evidence the original clause already had.
func TestEvictionIsDistinguishableFromNeverHavingHadHistory(t *testing.T) {
	const budget = 64 << 10
	s := evictionServer(t, budget)
	active := s.activeDoc()

	untouchedID := addDocument(s, []byte("untouched"))
	untouched := documentByID(s, untouchedID)
	evictedID := addDocument(s, []byte("evicted"))
	evicted := documentByID(s, evictedID)
	pushHistory(evicted, 4, budget/2)

	s.commitMutation(active, []byte("x"), []byte("y"))

	// The stimulus: the eviction must actually have happened, or "the two documents
	// report differently" is being asked of two documents in the same state.
	if s.historyEvictions == 0 {
		t.Fatal("no eviction occurred — the comparison below would be between two identical documents")
	}

	never := s.docResponse(untouched)
	lost := s.docResponse(evicted)

	// Both read canUndo:false. That is the whole problem.
	if never.CanUndo || lost.CanUndo {
		t.Fatalf("setup: expected both to report canUndo false, got never=%v lost=%v", never.CanUndo, lost.CanUndo)
	}

	if never.HistoryEvicted {
		t.Error("a document that never had a history is reported as evicted")
	}
	if !lost.HistoryEvicted {
		t.Error("an evicted document is indistinguishable from one that never had a history — this is the silent eviction the plan-review pin refuses")
	}
}

// The 2× → 2N× pin: the budget covers the PAIR. Before P03.S04 only the undo stack
// counted, so bytes parked in redo were invisible to the budget and one document
// could hold ~2× the ceiling.
func TestBudgetCountsRedoBytesToo(t *testing.T) {
	const budget = 64 << 10
	s := evictionServer(t, budget)
	active := s.activeDoc()

	otherID := addDocument(s, []byte("second"))
	other := documentByID(s, otherID)
	// All of the inactive document's bytes sit in REDO. A budget that counted only
	// undo would see zero here and evict nothing.
	for i := 0; i < 4; i++ {
		other.redo = append(other.redo, make([]byte, budget/2))
	}

	s.mu.Lock()
	before := s.historyBytesLocked()
	s.mu.Unlock()
	if before <= budget {
		t.Fatalf("setup: %d bytes of redo did not exceed the %d budget", before, budget)
	}

	s.commitMutation(active, []byte("x"), []byte("y"))

	if len(other.redo) != 0 {
		t.Errorf("redo bytes were not counted toward the budget: %d entries survived — this is the 2N× ceiling the pin closes", len(other.redo))
	}
}

// The budget's one named exception, stated in undo.go's contract: the active
// document always keeps its last undo entry. A document whose single most recent
// state is larger than the whole budget stays undoable rather than being silently
// stripped of the one thing undo is for.
func TestActiveDocumentKeepsItsLastUndoEntry(t *testing.T) {
	const budget = 8 << 10
	s := evictionServer(t, budget)
	active := s.activeDoc()

	big := make([]byte, budget*4)
	s.commitMutation(active, big, []byte("result"))

	if len(active.undo) != 1 {
		t.Fatalf("undo depth = %d, want 1 (the entry is larger than the budget and must survive)", len(active.undo))
	}
	s.mu.Lock()
	over := s.historyBytesLocked()
	s.mu.Unlock()
	if over <= budget {
		t.Fatalf("setup: the entry (%d bytes) did not exceed the budget — the exception was never exercised", over)
	}
	if !s.docResponse(active).CanUndo {
		t.Error("the active document reports canUndo false while holding an undo entry")
	}
}

// The premise pin's clause: single-document behaviour is unchanged wherever the
// depth cap binds, which with ordinary PDFs is every realistic case. Asserted at the
// PRODUCTION budget, not a lowered one — the point is that the real ceiling is not
// what trims an ordinary history.
func TestSingleDocumentTrimmingIsUnchangedAtTheRealBudget(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf) // no maxHistoryBytes override: the shipping budget
	doc := s.activeDoc()

	for i := 0; i < maxUndoDepth+5; i++ {
		s.commitMutation(doc, pdf, pdf)
	}

	if len(doc.undo) != maxUndoDepth {
		t.Errorf("undo depth = %d, want %d — the depth cap, not the byte budget, must be what trims an ordinary history", len(doc.undo), maxUndoDepth)
	}
	if s.historyEvictions != 0 {
		t.Errorf("historyEvictions = %d, want 0 — %d states of an ordinary PDF must not approach the byte budget", s.historyEvictions, maxUndoDepth)
	}
	if doc.historyEvicted {
		t.Error("an ordinary edit history was reported as evicted")
	}
}

// The standing race fixture. This FAILS under -race against the code as it stood
// before P03.S04: docResponse released s.mu and then read doc.path, doc.sig and
// doc.data, racing undo.go's `doc.data = result`.
//
// It is worth keeping rather than deleting with the fix, because the shape that
// caused it is easy to reintroduce — the read is cheap-looking, and moving it back
// outside the lock would look like an optimization. It reproduces a plain user
// situation: one document, two browser panes, one polling /api/doc while the other
// runs an operation.
//
// Note it is only meaningful under -race; without it the test passes trivially.
// CONTRIBUTING.md requires `go test -race ./internal/server/` after concurrency work.
func TestDocResponseRacesMutation(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})

	const rounds = 30
	var mutations, reads int
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
			fw.Write(pdf)
			mw.WriteField("op", "rotate")
			mw.WriteField("deg", "90")
			mw.Close()
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/pages", &buf)
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("X-CSRF-Token", csrf)
			resp, err := c.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				mu.Lock()
				mutations++
				mu.Unlock()
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			resp, err := c.Get(ts.URL + "/api/doc")
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				mu.Lock()
				reads++
				mu.Unlock()
			}
		}
	}()
	wg.Wait()

	// The stimulus, and this one is load-bearing: -race only reports a race it
	// OBSERVES. If neither arm actually ran, the test is a clean green that proves
	// nothing at all — which is exactly how this defect survived until P03.S04.
	if mutations == 0 {
		t.Fatal("no mutation succeeded — the writing half of the race never ran")
	}
	if reads == 0 {
		t.Fatal("no /api/doc read succeeded — the reading half of the race never ran")
	}
}

// TestDocumentReadRoutesRaceMutation is the generalisation of the test above, and
// the reason it exists is the shape of the bug it caught.
//
// TestDocResponseRacesMutation drives /api/doc only — the one path that had been
// fixed. Twelve sibling handlers went on reading doc.data with no lock held, and
// `go test -race ./internal/server/` stayed green over all of them, because a
// detector only reports the race it OBSERVES and nothing drove those routes
// concurrently with a write. A guard whose population is one is not a guard for
// the class it appears to cover.
//
// Each read route below is driven against a stream of page ops (undo.go's
// `doc.data = result`). Every one of them tripped the detector before docBytes
// existed; /api/scan trips it on the first round.
func TestDocumentReadRoutesRaceMutation(t *testing.T) {
	readRoutes := []struct{ name, method, path string }{
		{"scan", http.MethodGet, "/api/scan"},
		{"outline", http.MethodGet, "/api/outline"},
		{"attachments", http.MethodGet, "/api/attachments"},
		{"form-data", http.MethodGet, "/api/form-data"},
		{"optimize", http.MethodPost, "/api/optimize"},
	}

	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})

	const rounds = 25
	var mu sync.Mutex
	mutations := 0
	reads := map[string]int{}

	var wg sync.WaitGroup
	wg.Add(1 + len(readRoutes))

	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
			fw.Write(pdf)
			mw.WriteField("op", "rotate")
			mw.WriteField("deg", "90")
			mw.Close()
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/pages", &buf)
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("X-CSRF-Token", csrf)
			resp, err := c.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				mu.Lock()
				mutations++
				mu.Unlock()
			}
		}
	}()

	for _, rt := range readRoutes {
		go func(name, method, path string) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				req, err := http.NewRequest(method, ts.URL+path, nil)
				if err != nil {
					continue
				}
				req.Header.Set("X-CSRF-Token", csrf)
				resp, err := c.Do(req)
				if err != nil {
					continue
				}
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					mu.Lock()
					reads[name]++
					mu.Unlock()
				}
			}
		}(rt.name, rt.method, rt.path)
	}
	wg.Wait()

	// The stimulus, per the sibling above: -race reports only what it observes, so
	// an arm that never ran turns this into a green over nothing. Asserted PER
	// ROUTE, not in total — one busy route would otherwise cover for four silent
	// ones, which is the same one-population mistake this test exists to correct.
	if mutations == 0 {
		t.Fatal("no mutation succeeded — the writing half of the race never ran")
	}
	for _, rt := range readRoutes {
		if reads[rt.name] == 0 {
			t.Errorf("no %s read succeeded — that half of the race never ran, so -race saw nothing there", rt.name)
		}
	}
}
