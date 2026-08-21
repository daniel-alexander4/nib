package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// TestCloseDropsTheDocument is the transition, not the end state: an empty
// docResponse and a 404 from /api/pdf are also what the server answers at launch,
// so asserting only the post-close values would pass against a close that did
// nothing. Each check therefore runs once with the document open first.
func TestCloseDropsTheDocument(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	resp, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/pdf before close = %d, want 200", resp.StatusCode)
	}

	assertEmptyDoc(t, postDoc(t, ts, c, csrf, "/api/close"), "close response")

	resp, err = c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("/api/pdf after close = %d, want 404", resp.StatusCode)
	}
	// The same literal the thirteen already-guarded handlers emit — the client
	// reads the string, so a route inventing its own wording makes the empty
	// state ambiguous even though the status agrees.
	var e struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		t.Fatal(err)
	}
	if e.Error != "no document open" {
		t.Errorf("error = %q, want %q", e.Error, "no document open")
	}

	assertEmptyDoc(t, getDoc(t, ts, c), "/api/doc after close")
}

// TestCloseClearsBothRings asserts the rings THEMSELVES, not docResponse.CanUndo.
//
// The obvious HTTP-level check — canUndo true before the close, false after — is
// vacuous here, and that is not obvious from reading it. docResponse computes
// canUndo from len(s.undo) under the lock and then returns the ZERO STRUCT when
// doc == nil (server.go), discarding it. So canUndo is false after any close
// because the document is gone, not because the ring was cleared: the check would
// pass unchanged against a setDoc(nil) that left both rings fully populated. The
// clause it is cited as discharging is a P01 exit criterion about retained
// document bytes, so the ring is what has to be looked at.
//
// These tests are in-package, so it can be.
func TestCloseClearsBothRings(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := openTestServer(t, pdf)
	doc := s.activeDoc()

	// Populate BOTH rings: two commits fill undo, one undo moves a state across.
	s.commitMutation(doc, pdf, pdf)
	s.commitMutation(doc, pdf, pdf)
	s.handleUndo(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/undo", nil))
	if len(doc.undo) == 0 || len(doc.redo) == 0 {
		t.Fatalf("setup: want both rings non-empty, got undo=%d redo=%d", len(doc.undo), len(doc.redo))
	}

	s.handleClose(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/close", nil))

	// The rings moved onto the document in P03.S04, and this test deliberately holds
	// the closed document's pointer — which is exactly the situation a request still
	// in flight is in. Dropping the registry entry alone would leave these stacks
	// populated for as long as any such reference lives, so setDoc releases them
	// explicitly and this asserts that it did. Reading the SERVER's rings here would
	// now be unfalsifiable: there are none.
	if len(doc.undo) != 0 || len(doc.redo) != 0 {
		t.Errorf("close must clear both rings, got undo=%d redo=%d", len(doc.undo), len(doc.redo))
	}
	if s.activeDoc() != nil {
		t.Error("close must drop the document")
	}
}

// TestCloseIsIdempotent covers the only way a user reaches it — a race against a
// control the UI disables with nothing open — on the ErrNotEncrypted-passes-
// through precedent (v1.57.0): closing nothing is a success, not a 404.
func TestCloseIsIdempotent(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	assertEmptyDoc(t, postDoc(t, ts, c, csrf, "/api/close"), "close with nothing open")

	openByPath(t, ts.URL, c, csrf, path)
	postDoc(t, ts, c, csrf, "/api/close")
	assertEmptyDoc(t, postDoc(t, ts, c, csrf, "/api/close"), "second close")
}

// TestOpenAfterClose is the point of the whole slice: putting a document down is
// not quitting, so the next open must behave like any other.
func TestOpenAfterClose(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)
	postDoc(t, ts, c, csrf, "/api/close")

	dr := openByPath(t, ts.URL, c, csrf, path)
	if !dr.CanSave || dr.Path != path {
		t.Errorf("open after close = %+v, want a saveable document at %q", dr, path)
	}
	resp, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/api/pdf after re-open = %d, want 200", resp.StatusCode)
	}
}

// TestCommitAfterCloseIsRefused discharges the clause P01.S01 recorded as `not
// exercised`: a close landing while an operation is in flight makes that
// operation answer 404, not 200.
//
// It needs no timing and no goroutines. Exactly one lock acquisition decides the
// outcome — the test-and-write inside the commit helpers — so placing the close
// before it IS the in-flight case, exactly and deterministically. A hammering
// test could only ever visit that window by luck and could not prove it had.
func TestCommitAfterCloseIsRefused(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		commit func(*Server) bool
	}{
		{"commitMutation", func(s *Server) bool { return s.commitMutation(s.activeDoc(), pdf, pdf) }},
		{"commitBarrier", func(s *Server) bool { return s.commitBarrier(s.activeDoc(), pdf) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestServer(t, pdf)
			// The non-zero probe: the identical call must succeed with a
			// document open, or a helper that always returned false would pass.
			if !tc.commit(s) {
				t.Fatal("commit with a document open returned false")
			}
			// Drive the real close, not setDoc(nil) — the clause is about a
			// close landing, so the handler is the subject.
			s.handleClose(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/close", nil))
			if tc.commit(s) {
				t.Error("commit after a close returned true — a success for discarded work")
			}
		})
	}
}

// TestRedactRefusedAfterClose is the post-close exposure check at the HTTP level,
// on the irreversible route. It is distinct from the P01.S01 test, which reached
// the empty state only by never opening anything: this one reaches it by closing,
// which is the state that did not exist before this slice.
func TestRedactRefusedAfterClose(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	redactBody := func() (string, io.Reader) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(form)
		iw, _ := mw.CreateFormFile("page", "page-1.png")
		png.Encode(iw, image.NewRGBA(image.Rect(0, 0, 1224, 1584)))
		mw.WriteField("pageNum", "1")
		mw.WriteField("pageW", "612")
		mw.WriteField("pageH", "792")
		mw.Close()
		return mw.FormDataContentType(), &buf
	}

	ct, body := redactBody()
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/redact", ct, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redact with a document open = %d, want 200", resp.StatusCode)
	}

	postDoc(t, ts, c, csrf, "/api/close")

	ct, body = redactBody()
	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/redact", ct, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		got, _ := io.ReadAll(resp.Body)
		t.Fatalf("redact after close = %d, want 404 (body %s)", resp.StatusCode, got)
	}
}

// TestCloseRacesMutation exercises the handler concurrently against the state
// this slice newly makes reachable — s.doc nil at an arbitrary moment — under the
// race detector.
//
// Note what it does NOT assert, because the obvious invariant is wrong: "a 200
// never carries an empty docResponse" fails on entirely correct behaviour, since
// handlePages releases the lock between its successful commit and its
// docResponse() call (pages.go), so a close landing in that gap legitimately
// produces 200 with an empty body. Nor can it prove it visited the window at all
// — a close that wins outright and a close that lands mid-operation both produce
// a 404, so a both-outcomes rule would be a proxy for that, not a measurement of
// it. TestCommitAfterCloseIsRefused owns that property, deterministically.
//
// What this resolves: every reply is 200 or 404 — never a panic from an
// unguarded s.doc read, never a 500, never a dropped connection — and `go test
// -race` is clean.
func TestCloseRacesMutation(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)
	pdf := fetchPDF(t, ts, c)

	for i := 0; i < 20; i++ {
		openByPath(t, ts.URL, c, csrf, path)
		headStart := i%2 == 1

		// Errors travel back over channels rather than t.Fatal — these are not
		// the test goroutine, and t.Fatal there runs Goexit on the wrong one,
		// which would turn the transport error a panicked handler produces (the
		// very thing this test watches for) into a hang instead of a failure.
		var wg sync.WaitGroup
		status := make(chan int, 1)
		errc := make(chan error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
			fw.Write(pdf)
			mw.WriteField("op", "rotate")
			mw.WriteField("deg", "90")
			mw.Close()
			code, e := post(c, csrf, ts.URL+"/api/pages", mw.FormDataContentType(), &buf)
			if e != nil {
				errc <- e
				return
			}
			status <- code
		}()
		go func() {
			defer wg.Done()
			// Half the iterations give the page op a head start. Without it the
			// close wins every single time — measured, 20/20 — because closing
			// is instant while a page op spends milliseconds in pdfcpu, so the
			// 200 arm of the assertion below would never once execute and the
			// test would say nothing about the post-commit path.
			//
			// The stagger is a sampling aid, not a correctness dependency: the
			// invariant holds whichever side wins, so an iteration that lands
			// the other way still passes. It is emphatically not a claim to
			// have visited the commit window — see the doc comment above.
			if headStart {
				time.Sleep(20 * time.Millisecond)
			}
			if _, e := post(c, csrf, ts.URL+"/api/close", "", nil); e != nil {
				errc <- e
			}
		}()
		wg.Wait()
		close(errc)
		close(status)

		for e := range errc {
			t.Fatalf("iteration %d: request failed (a panicked handler drops the connection): %v", i, e)
		}
		code, ok := <-status
		if !ok {
			t.Fatalf("iteration %d: /api/pages returned no status", i)
		}
		// 409 is the close landing mid-flight, and it is the outcome this test races for.
		//
		// The list read "200 or 404" until v1.116.3, when the eight commit-failure branches
		// moved to 409 per ADR-004 — "an id naming a document the server no longer holds is
		// 409, never 404". This test drives exactly that race, so it was pinning the code
		// the ADR forbids. 404 stays in the set: `resolveDoc` still answers it for the
		// genuinely-nothing-open case, which this race can also produce.
		if code != http.StatusOK && code != http.StatusNotFound && code != http.StatusConflict {
			t.Fatalf("iteration %d: /api/pages = %d, want 200, 404 or 409", i, code)
		}
	}
}

// post issues a CSRF-bearing write and returns the status, draining and closing
// the body. Unlike the shared write() helper it reports errors instead of calling
// t.Fatal, so it is safe to call from a goroutine.
func post(c *http.Client, csrf, url, contentType string, body io.Reader) (int, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return 0, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// assertEmptyDoc checks a response is the zero docResponse field by field.
// docResponse holds two slices (Flags, Signature.Signers) so it is not
// comparable with ==, and spelling the fields out states what "the empty
// response" actually means rather than leaving it to a struct literal.
func assertEmptyDoc(t *testing.T, dr docResponse, what string) {
	t.Helper()
	if dr.Name != "" || dr.Path != "" || dr.CanSave {
		t.Errorf("%s: name=%q path=%q canSave=%v, want all empty", what, dr.Name, dr.Path, dr.CanSave)
	}
	if dr.Signature.State != "" || len(dr.Signature.Signers) != 0 || dr.Signature.AddedAfter {
		t.Errorf("%s: signature = %+v, want the zero status", what, dr.Signature)
	}
	if dr.Flags != nil {
		t.Errorf("%s: flags = %s, want none", what, dr.Flags)
	}
	if dr.CanUndo || dr.CanRedo {
		t.Errorf("%s: canUndo=%v canRedo=%v, want both false", what, dr.CanUndo, dr.CanRedo)
	}
}

// getDoc reads the current document metadata.
func getDoc(t *testing.T, ts *httptest.Server, c *http.Client) docResponse {
	t.Helper()
	resp, err := c.Get(ts.URL + "/api/doc")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var dr docResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	return dr
}
