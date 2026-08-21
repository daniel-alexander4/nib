package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// P06.S01 — Open ADDS a document rather than replacing the registry, and the
// count half of D9's cap bounds the growth that creates.
//
// The two properties are tested together because they were introduced together and
// for that reason: before this slice Open replaced, so a user's exposure was one
// document however many times they opened one. Making Open add is what turns twenty
// large scans into a keyboard shortcut, and the cap is what stops it.

// TestOpenAddsRatherThanReplacing is the slice's central claim, and it asserts the
// OLD document as well as the new one.
//
// Asserting only "the registry holds 2" would pass against an implementation that
// appended a second entry while corrupting or emptying the first — which is the
// failure this replaces, one layer over: the previous code emptied the whole
// registry while the client re-pointed only the active view, and the second view
// went on rendering a document the server no longer held.
func TestOpenAddsRatherThanReplacing(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	first := openByPath(t, ts.URL, c, csrf, path)
	if first.ID == "" {
		t.Fatal("the first open returned no document id — nothing below can be addressed")
	}
	second := openByPath(t, ts.URL, c, csrf, path)
	if second.ID == "" {
		t.Fatal("the second open returned no document id")
	}
	if second.ID == first.ID {
		t.Fatalf("both opens report the same id %s — ADR-001 ids are never reused, so this is one document, not two", first.ID)
	}

	// The new one is active…
	active := docByID(t, ts, c, "")
	if active.ID != second.ID {
		t.Errorf("active document is %s, want the just-opened %s", active.ID, second.ID)
	}
	// …and the first is STILL OPEN AND ADDRESSABLE, which is the whole point.
	kept := docByID(t, ts, c, first.ID)
	if kept.ID != first.ID {
		t.Errorf("the first document is no longer addressable (got id %q) — opening a second document orphaned it", kept.ID)
	}
	if kept.Path != first.Path {
		t.Errorf("the first document came back describing something else: path %q, want %q", kept.Path, first.Path)
	}
}

// TestOpenLeavesTheOtherDocumentsHistoryAlone replaces TestOpenClearsHistory, whose
// name stopped being true of the code this slice ships.
//
// That test opened the same file twice and asserted the second response reported no
// undo. Under setDoc that was a real claim — the open REPLACED, and the assertion
// caught a failure to clear. Under addDoc the second open is a different document
// that never had a history, so the assertion cannot fail for the reason its name
// gives: ask what it would have missed if opening did nothing to history at all, and
// the answer is everything. The property worth having under add-semantics is the
// opposite one — an open must not reach INTO a document it is not.
func TestOpenLeavesTheOtherDocumentsHistoryAlone(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	first := openByPath(t, ts.URL, c, csrf, path)
	rotateDoc(t, ts, c, csrf, fetchPDF(t, ts, c))
	// The stimulus. Without it the assertion below reads an empty history and passes
	// against an open that wiped everything.
	if before := docByID(t, ts, c, first.ID); !before.CanUndo {
		t.Fatal("setup: the first document has no undo history, so nothing below can observe it being lost")
	}

	second := openByPath(t, ts.URL, c, csrf, path)

	if second.CanUndo || second.CanRedo {
		t.Errorf("a freshly opened document must start with an empty history, got canUndo=%v canRedo=%v", second.CanUndo, second.CanRedo)
	}
	after := docByID(t, ts, c, first.ID)
	if !after.CanUndo {
		t.Error("opening a second document cleared the FIRST document's undo history — an open reached into a document it is not")
	}
	if after.HistoryEvicted {
		t.Error("the first document was reported as history-evicted by an unrelated open")
	}
}

// TestTheNinthOpenIsRefusedAndTheEighthIsNot is the cap, with its positive control.
//
// The refusal alone proves nothing: a route that refused EVERY open would pass it,
// and so would one that refused on the second. The eighth succeeding is what makes
// the ninth's refusal mean "the cap bound" rather than "opening is broken".
func TestTheNinthOpenIsRefusedAndTheEighthIsNot(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	for i := 1; i <= maxOpenDocs; i++ {
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", jsonBody(openRequest{Path: path}))
		body := readBody(t, resp)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("open %d of %d was refused (%d: %s) — the cap binds before it should", i, maxOpenDocs, resp.StatusCode, body)
		}
	}

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", jsonBody(openRequest{Path: path}))
	body := readBody(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("open %d status = %d, want 409 — the cap did not bind", maxOpenDocs+1, resp.StatusCode)
	}
	// The message has to tell the user what to DO, or the refusal is a bare "no". And
	// what it tells them has to be possible in the app as it ships — which is why this
	// asserts the instruction rather than the exact sentence: S01 said "use Close, then
	// reopen the ones you need" because Close was still close-ALL, and S02 reworded it to
	// "close one first" the moment Close view existed. The assertion survived both.
	if !strings.Contains(strings.ToLower(body), "close") {
		t.Errorf("the refusal does not tell the user what to do: %q", body)
	}
	if !strings.Contains(body, strconv.Itoa(maxOpenDocs)) {
		t.Errorf("the refusal does not name the limit, so the user cannot tell how far over they are: %q", body)
	}
}

// TestAnArrivalIsNeverRefusedByTheCap is the reason the check sits on the routes
// rather than inside addDoc.
//
// A co-signature is work a counterparty has already performed, and the session path
// installs the signed bytes straight into the registry — nothing writes them to
// disk. Refusing one at the cap does not decline a request, it destroys a document
// that has no other home. A user at the cap can close a tab and open again; a peer
// who has already signed cannot.
func TestAnArrivalIsNeverRefusedByTheCap(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}
	for i := 0; i < maxOpenDocs; i++ {
		addDocument(s, pdf)
	}
	// The stimulus: the registry really is at the cap, so the assertion below is
	// about the exemption and not about an empty registry.
	if _, err := s.addDocCapped(&document{data: pdf}); err != ErrTooManyOpen {
		t.Fatalf("setup: a capped add at %d documents returned %v, want ErrTooManyOpen", maxOpenDocs, err)
	}

	arrival := s.addDoc(&document{data: pdf, sig: sign.Verify(pdf)})
	if arrival == nil {
		t.Fatal("an arrival was refused at the cap — a signed document with no other home was destroyed")
	}
	s.mu.Lock()
	n, active := len(s.docs), s.activeID
	s.mu.Unlock()
	if n != maxOpenDocs+1 {
		t.Errorf("registry holds %d documents, want %d — the arrival did not land", n, maxOpenDocs+1)
	}
	if active != arrival.id {
		t.Errorf("the arrival is not active (active %s, arrival %s)", active, arrival.id)
	}
}

// readBody reads a response body as a string for assertions about what the user is
// told. The status code says a request was refused; only the body says why.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return string(b)
}

// docByID fetches /api/doc, pinned to id when one is given and unpinned (i.e. "the
// active document") when it is empty.
func docByID(t *testing.T, ts *httptest.Server, c *http.Client, id string) docResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/doc", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		req.Header.Set("X-Nib-Doc", id)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var dr docResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatalf("decoding /api/doc: %v", err)
	}
	return dr
}

// writeTo posts to url pinned to a document id — the shape every close-view test needs
// and none of them should re-derive.
func writeTo(t *testing.T, c *http.Client, csrf, url, docID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-CSRF-Token", csrf)
	if docID != "" {
		req.Header.Set("X-Nib-Doc", docID)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// decodeDoc reads a docResponse off a response and closes the body.
func decodeDoc(t *testing.T, resp *http.Response) docResponse {
	t.Helper()
	defer resp.Body.Close()
	var dr docResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatalf("decoding docResponse: %v", err)
	}
	return dr
}

// P06.S04 — the byte half of D9's cap.
//
// The count and the bytes bind independently, and a test that only exercised one would
// leave the other's message unreachable: they say different things to a user, and
// telling someone at the byte ceiling to "close one first" points them at the wrong
// remedy when the one they close is small.
func TestTheByteCeilingRefusesAndSaysWhichBoundBound(t *testing.T) {
	s := &Server{epoch: "test-epoch"}
	// Two documents just under the ceiling between them, so the registry is nowhere near
	// the COUNT cap and any refusal below is unambiguously the byte one.
	big := make([]byte, maxOpenBytes/2-1)
	if _, err := s.addDocCapped(&document{data: big}); err != nil {
		t.Fatalf("setup: the first large document was refused: %v", err)
	}
	if _, err := s.addDocCapped(&document{data: big}); err != nil {
		t.Fatalf("setup: the second large document was refused: %v — the ceiling binds too early", err)
	}
	s.mu.Lock()
	n := len(s.docs)
	s.mu.Unlock()
	if n != 2 || n >= maxOpenDocs {
		t.Fatalf("setup: %d documents, want 2 and well under the count cap of %d", n, maxOpenDocs)
	}

	// One more byte over.
	_, err := s.addDocCapped(&document{data: make([]byte, 4096)})
	if err != ErrTooManyBytes {
		t.Fatalf("an open past the byte ceiling returned %v, want ErrTooManyBytes", err)
	}
	if !strings.Contains(err.Error(), "MiB") {
		t.Errorf("the refusal does not name the ceiling, so the user cannot tell how far over they are: %q", err)
	}

	// And a SMALL document is still refused at the ceiling — the bound is the aggregate,
	// not the size of the document being opened. Without this the row passes against an
	// implementation that only rejected large individual documents.
	if _, err := s.addDocCapped(&document{data: []byte("tiny")}); err != ErrTooManyBytes {
		t.Errorf("a tiny document was accepted past the aggregate ceiling (%v) — the cap is measuring the wrong thing", err)
	}
}

// TestTheCountCapStillBindsBelowTheByteCeiling — the other direction, and the reason
// D9 says "whichever binds first". Eight small documents are nowhere near 512 MiB.
func TestTheCountCapStillBindsBelowTheByteCeiling(t *testing.T) {
	s := &Server{epoch: "test-epoch"}
	for i := 0; i < maxOpenDocs; i++ {
		if _, err := s.addDocCapped(&document{data: []byte("small")}); err != nil {
			t.Fatalf("setup: document %d was refused: %v", i+1, err)
		}
	}
	_, err := s.addDocCapped(&document{data: []byte("small")})
	if err != ErrTooManyOpen {
		t.Errorf("nine tiny documents returned %v, want ErrTooManyOpen — the count cap stopped binding when the byte one landed", err)
	}
}

// TestGrowingAnOpenDocumentIsBoundedByTheSameCeiling drives ADR-005's byte half through the
// door it did not reach: an operation that grows a document ALREADY open.
//
// The count half has always bound both doors (a ninth document is refused wherever it comes
// from), but the byte half lived only in addDocCapped — at OPEN. Every in-place growth path
// went past it: an OCR text layer, an N-up, a scan import, an attachment. The ADR's sentence
// was true of one door and read as true of the cap.
func TestGrowingAnOpenDocumentIsBoundedByTheSameCeiling(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	doc := s.activeDoc()
	// Kilobytes rather than half a gigabyte. The first draft of this test used the real
	// 512 MiB ceiling and spent over ten minutes allocating and copying before its first
	// assertion — see maxDocBytes.
	s.mu.Lock()
	s.maxDocBytes = 64 << 10
	s.mu.Unlock()

	// A second document holding just over half the budget, so ONE more half-budget document
	// crosses it. Opened through the capped door, which must accept it — otherwise the
	// refusal below is the count cap or the open cap and not the growth cap.
	half := make([]byte, 33<<10)
	copy(half, pdf)
	if _, err := s.addDocCapped(&document{name: "half.pdf", data: half}); err != nil {
		t.Fatalf("the setup document was itself refused (%v) — the growth refusal below "+
			"would then be measuring the wrong ceiling", err)
	}

	// STIMULUS: a commit that does NOT grow past the ceiling still lands. Without this, a
	// commitMutation that had simply started refusing everything would pass the assertion.
	if err := s.commitMutation(doc, pdf, pdf); err != nil {
		t.Fatalf("an ordinary commit was refused: %v", err)
	}

	// The growth: the active document swells to half the budget, which with half.pdf
	// already open crosses it.
	grown := make([]byte, 33<<10)
	copy(grown, pdf)
	if err := s.commitMutation(doc, pdf, grown); !errors.Is(err, ErrTooManyBytes) {
		t.Fatalf("growing an open document past the aggregate ceiling returned %v, want "+
			"ErrTooManyBytes — ADR-005 bounds the open documents' bytes, not the bytes that "+
			"arrived through one particular door", err)
	}
	// And the barrier door, which redaction and export use, is the same fact.
	if err := s.commitBarrier(doc, grown); !errors.Is(err, ErrTooManyBytes) {
		t.Fatalf("commitBarrier accepted the same growth: %v", err)
	}
	// The refusal must not have half-applied: the document still holds its old bytes.
	if len(s.docBytes(doc)) != len(pdf) {
		t.Fatalf("the refused commit changed the document anyway (%d bytes, was %d)",
			len(s.docBytes(doc)), len(pdf))
	}
	// Shrinking is never refused — the check is on the total after the write, not the delta.
	small := append([]byte(nil), pdf...)
	if err := s.commitMutation(doc, grown, small); err != nil {
		t.Fatalf("a commit that makes the document SMALLER was refused: %v", err)
	}
}
