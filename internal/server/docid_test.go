package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/testpdf"
)

// THE two-document probe — P03's pinned exit criterion, and the first test in this
// plan that can fail for the reason the whole phase exists.
//
// Every test before it had one document open, where "acted on the right document"
// and "acted on the only document" are indistinguishable. This one addresses an
// operation to the INACTIVE document and asserts two things that must both hold:
// the addressed document changed, and the active one did not.
//
// The second assertion is the point, and it is made by comparing BYTES rather than
// by re-reading a status. A status would be coarser than the clause — it would go
// on saying 200 while the active document's contents were replaced underneath.
//
// Writing this is what found the defect it now guards: the mutating routes used to
// commit through helpers that resolved the ACTIVE document internally, so an
// operation addressed to B installed its result into A. That is ADR-001's
// corruption arriving through a helper rather than a forgotten header.
func TestOperationAddressedToInactiveDocumentLeavesActiveAlone(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	three, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	// A is active; B is a second document reachable only by naming its id.
	srv.setDoc(&document{data: three})
	activeID := srv.activeDoc().id
	activeBefore := append([]byte(nil), srv.activeDoc().data...)

	inactiveID := addDocument(srv, append([]byte(nil), three...))
	if inactiveID == activeID {
		t.Fatal("setup: the two documents share an id")
	}

	// Rotate, addressed to the INACTIVE document.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(three)
	mw.WriteField("op", "rotate")
	mw.WriteField("deg", "90")
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/pages", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("X-Nib-Doc", inactiveID.String())
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("addressed rotate = %d, want 200 (body %s)", resp.StatusCode, body)
	}

	// The stimulus, asserted before the response: if the addressed document did not
	// change, "the active one is unchanged" is satisfied by an operation that did
	// nothing at all — a green over an absence.
	inactiveAfter := documentByID(srv, inactiveID)
	if inactiveAfter == nil {
		t.Fatal("the addressed document vanished from the registry")
	}
	if bytes.Equal(inactiveAfter.data, three) {
		t.Fatal("the addressed document did not change — the operation reached nothing, so the assertion below proves nothing")
	}

	// And the clause itself.
	if !bytes.Equal(srv.activeDoc().data, activeBefore) {
		t.Errorf("an operation addressed to the inactive document changed the ACTIVE one (%d bytes -> %d) — this is the corruption operation pinning exists to prevent",
			len(activeBefore), len(srv.activeDoc().data))
	}
	if srv.activeDoc().id != activeID {
		t.Error("the active document changed identity")
	}
}

func documentByID(s *Server, id docID) *document {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.docs {
		if d.id == id {
			return d
		}
	}
	return nil
}

// 404 and 409 are different facts and the client branches on them: 404 blanks the
// app, 409 drops one stale tab and leaves the rest. So they differ in body as well
// as status — a shared message would make them one fact to whoever reads it.
func TestUnknownDocumentIs409AndSaysSomethingElse(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	// The non-zero probe: with nothing open, the same route answers 404 with the
	// long-standing message. Without this, a 409 test passes against a server that
	// answers 409 to everything.
	resp := write(t, c, csrf, http.MethodGet, ts.URL+"/api/attachments", "", nil)
	got404 := decodeError(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("with nothing open: status %d, want 404", resp.StatusCode)
	}
	if got404 != "no document open" {
		t.Errorf("404 body = %q, want %q", got404, "no document open")
	}

	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})

	// A well-formed id this server does not hold.
	stale := docID{Epoch: srv.epoch, Seq: 9999}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/attachments", nil)
	req.Header.Set("X-Nib-Doc", stale.String())
	resp2, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got409 := decodeError(t, resp2)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("unknown id: status %d, want 409", resp2.StatusCode)
	}
	if got409 == got404 {
		t.Errorf("409 and 404 share the message %q — a client cannot tell 'that tab is gone' from 'nothing is open'", got409)
	}
	if got409 != "that document is no longer open" {
		t.Errorf("409 body = %q, want %q", got409, "that document is no longer open")
	}
}

// An id from a previous process carries that process's Seq. Without the epoch, a
// client holding an id across a restart on a fixed NIB_ADDR port would name a
// document the new process has since assigned that number to.
func TestIDFromAnotherEpochIsRefused(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})
	live := srv.activeDoc().id

	// The probe: the live id is accepted, so a refusal below is about the epoch and
	// not about the route rejecting every id it is given.
	if resp := getWithDoc(t, c, ts.URL+"/api/attachments", live.String()); resp != http.StatusOK {
		t.Fatalf("the live id was refused (%d) — the refusal below would prove nothing", resp)
	}

	// Same Seq, different process. This is exactly the collision the epoch exists
	// for: after a restart the counter starts again at 1.
	other := docID{Epoch: "some-other-process", Seq: live.Seq}
	if resp := getWithDoc(t, c, ts.URL+"/api/attachments", other.String()); resp != http.StatusConflict {
		t.Errorf("an id from another process got %d, want 409 — a stale client could address a document it never opened", resp)
	}
	_ = csrf
}

// D15's exception: pdf.js owns the /api/pdf fetch, so its id travels as a query
// parameter rather than a header.
func TestPDFTakesItsIDAsAQueryParameter(t *testing.T) {
	ts, srv := startServerWith(t)
	c, _ := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})
	id := srv.activeDoc().id

	resp, err := c.Get(ts.URL + "/api/pdf?doc=" + id.String())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/pdf?doc=<live> = %d, want 200", resp.StatusCode)
	}

	stale := docID{Epoch: srv.epoch, Seq: 9999}
	resp2, err := c.Get(ts.URL + "/api/pdf?doc=" + stale.String())
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("/api/pdf?doc=<unknown> = %d, want 409", resp2.StatusCode)
	}
}

// Undo and redo stay BODYLESS and carry the id in the header — the thing D15 chose
// a header to make possible, and which an older exit criterion had wrong.
func TestUndoAndRedoStayBodylessAndAddressable(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})
	id := srv.activeDoc().id

	for _, route := range []string{"/api/undo", "/api/redo"} {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+route, nil) // no body, deliberately
		req.Header.Set("X-CSRF-Token", csrf)
		req.Header.Set("X-Nib-Doc", id.String())
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s with a header and no body = %d, want 200", route, resp.StatusCode)
		}

		req2, _ := http.NewRequest(http.MethodPost, ts.URL+route, nil)
		req2.Header.Set("X-CSRF-Token", csrf)
		req2.Header.Set("X-Nib-Doc", docID{Epoch: srv.epoch, Seq: 9999}.String())
		resp2, err := c.Do(req2)
		if err != nil {
			t.Fatal(err)
		}
		resp2.Body.Close()
		if resp2.StatusCode != http.StatusConflict {
			t.Errorf("%s addressed to an unknown document = %d, want 409", route, resp2.StatusCode)
		}
	}
}

// The compatibility path D15 keeps open: no id means the active document, which is
// what ~20 existing tests and every CLI verb rely on.
func TestNoIDMeansTheActiveDocument(t *testing.T) {
	ts, srv := startServerWith(t)
	c, _ := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})

	resp, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/api/pdf with no id = %d, want 200", resp.StatusCode)
	}
}

func getWithDoc(t *testing.T, c *http.Client, url, id string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Nib-Doc", id)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func decodeError(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var e struct {
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&e)
	return e.Error
}
