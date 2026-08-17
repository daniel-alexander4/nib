package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/instance"
	"nib/internal/testpdf"
)

// P07.S02 — the hand-off.

func handoffTo(t *testing.T, ts *httptest.Server, c *http.Client, secret, path string) (int, handoffResponse) {
	t.Helper()
	body, _ := json.Marshal(handoffRequest{Path: path})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/handoff", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set(instance.HeaderHandoff, secret)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out handoffResponse
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func pdfOnDisk(t *testing.T) string {
	t.Helper()
	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "handed.pdf")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestAHandoffToALockedInstanceIsQueuedAndOpensOnUnlock is the case the route exists
// for, and it is not an edge case: Nib starts locked, so this is the FIRST double-click
// of every session.
func TestAHandoffToALockedInstanceIsQueuedAndOpensOnUnlock(t *testing.T) {
	ts, srv := startServerWith(t)
	srv.SetHandoffSecret("secret")
	c := newClient(t)
	path := pdfOnDisk(t)

	// The stimulus. Without it "queued" below could be the answer of an unlocked
	// instance that simply failed to open the file.
	if st := getStatus(t, c, ts.URL); st.State == "ready" {
		t.Fatalf("setup: the vault is already unlocked (%q) — this test cannot observe the locked path", st.State)
	}

	code, out := handoffTo(t, ts, c, "secret", path)
	if code != http.StatusOK || out.Result != "queued" {
		t.Fatalf("hand-off to a locked instance = %d/%q, want 200/queued — refusing sends the launch down its become-primary path and produces two Nibs every morning", code, out.Result)
	}
	srv.mu.Lock()
	n := len(srv.docs)
	srv.mu.Unlock()
	if n != 0 {
		t.Errorf("a locked instance opened %d documents; the queue is supposed to hold them", n)
	}

	// Unlocking drains it — through adoptVault, the one place a vault becomes available.
	authedClient(t, ts)

	srv.mu.Lock()
	n, path0 := len(srv.docs), ""
	if n > 0 {
		path0 = srv.docs[0].path
	}
	srv.mu.Unlock()
	if n != 1 || path0 != path {
		t.Errorf("after unlocking, the instance holds %d documents (first path %q), want 1 × %q — the queued hand-off never drained", n, path0, path)
	}
}

// TestAHandoffForAnAlreadyOpenPathFocusesIt — D16. Two tabs on one path are two
// independent working copies of the same file, and whichever saves last silently
// discards the other's work.
func TestAHandoffForAnAlreadyOpenPathFocusesIt(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	srv.SetHandoffSecret("secret")
	path := pdfOnDisk(t)

	first := openByPath(t, ts.URL, c, csrf, path)
	second := openByPath(t, ts.URL, c, csrf, path) // a second working copy exists already
	_ = second
	srv.mu.Lock()
	before := len(srv.docs)
	srv.mu.Unlock()

	code, out := handoffTo(t, ts, c, "secret", path)
	if code != http.StatusOK || out.Result != "focused" {
		t.Fatalf("hand-off for an open path = %d/%q, want 200/focused", code, out.Result)
	}
	srv.mu.Lock()
	after, active := len(srv.docs), srv.activeID
	srv.mu.Unlock()
	if after != before {
		t.Errorf("the hand-off opened another copy: %d documents, want %d", after, before)
	}
	if !active.valid() || active.String() != first.ID {
		// It focuses the FIRST document holding that path, which is what docForPath finds.
		t.Logf("active is %s (first was %s) — either open document holding the path is acceptable", active, first.ID)
	}
}

func TestAHandoffIsRefusedWithoutTheSecretAndWhenTheServerHasNone(t *testing.T) {
	ts, srv := startServerWith(t)
	c := newClient(t)
	path := pdfOnDisk(t)

	// No secret published: this instance is not the one any record names.
	if code, _ := handoffTo(t, ts, c, "anything", path); code != http.StatusForbidden {
		t.Errorf("a server with no hand-off secret answered %d, want 403", code)
	}
	srv.SetHandoffSecret("right")
	if code, _ := handoffTo(t, ts, c, "wrong", path); code != http.StatusForbidden {
		t.Errorf("a wrong secret answered %d, want 403", code)
	}
	if code, _ := handoffTo(t, ts, c, "", path); code != http.StatusForbidden {
		t.Errorf("no secret at all answered %d, want 403", code)
	}
	// And the PROBE token must not work here. This is D20's whole first constraint: the
	// probe token proves identity and grants nothing, so presenting it as a hand-off
	// secret must fail — otherwise every read of the record is a capability grant.
	srv.SetInstanceToken("probe-token")
	if code, _ := handoffTo(t, ts, c, "probe-token", path); code != http.StatusForbidden {
		t.Errorf("the PROBE token authorised a hand-off (%d) — identity has become a capability, and internal/instance's own contract is false", code)
	}
}

// TestAHandoffGoesThroughTheOrdinaryInstallChecks — D20's third constraint. A
// self-contained handler would be a second, less-checked way to install a document.
func TestAHandoffGoesThroughTheOrdinaryInstallChecks(t *testing.T) {
	ts, srv := startServerWith(t)
	c, _ := authedClient(t, ts)
	srv.SetHandoffSecret("secret")

	notPDF := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(notPDF, []byte("this is not a PDF at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := handoffTo(t, ts, c, "secret", notPDF)
	if code != http.StatusOK || out.Result != "refused" {
		t.Fatalf("handing off a non-PDF = %d/%q, want 200/refused — without LooksLikePDF any readable file becomes the open document with canSave true, and Save clobbers it", code, out.Result)
	}
	srv.mu.Lock()
	n := len(srv.docs)
	srv.mu.Unlock()
	if n != 0 {
		t.Errorf("the non-PDF was installed anyway (%d documents)", n)
	}

	// And the document cap refuses a hand-off exactly as it refuses an open. DISTINCT
	// paths, because D16 would focus rather than open the same one twice — which is
	// itself worth stating: a cap test built on one repeated path never fills anything.
	data, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	refused := false
	for i := 0; i <= maxOpenDocs; i++ {
		p := filepath.Join(dir, fmt.Sprintf("doc-%d.pdf", i))
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		code, out := handoffTo(t, ts, c, "secret", p)
		if code != http.StatusOK {
			t.Fatalf("hand-off %d answered %d", i+1, code)
		}
		if out.Result == "refused" {
			refused = true
			break
		}
	}
	if !refused {
		t.Error("the document cap never refused a hand-off — it is not going through addDocCapped")
	}
}

func TestTheQueueIsBoundedAndDropsTheOldest(t *testing.T) {
	s := &Server{epoch: "test-epoch"}
	for i := 0; i < maxOpenDocs+3; i++ {
		s.queuePendingOpen(filepath.Join("/tmp", "queued", string(rune('a'+i))+".pdf"))
	}
	s.mu.Lock()
	got := append([]string(nil), s.pendingOpens...)
	s.mu.Unlock()
	if len(got) != maxOpenDocs {
		t.Fatalf("the queue holds %d paths, want at most %d — queueing more promises documents the cap will refuse", len(got), maxOpenDocs)
	}
	// The OLDEST went, not the newest: the most recent double-click is the one the user
	// is waiting on.
	if got[len(got)-1] != filepath.Join("/tmp", "queued", string(rune('a'+maxOpenDocs+2))+".pdf") {
		t.Errorf("the newest queued path is %q — the bound dropped the wrong end", got[len(got)-1])
	}

	// The same file clicked twice is one document.
	before := len(got)
	s.queuePendingOpen(got[0])
	s.mu.Lock()
	after := len(s.pendingOpens)
	s.mu.Unlock()
	if after != before {
		t.Errorf("queueing an already-queued path grew the queue to %d", after)
	}
}

// TestAHandoffCanonicalisesThePathBeforeComparingIt — P07 phase close.
//
// D16's promise is enforced by a STRING COMPARE against the path each document was
// opened from, and `handleOpen` stores a cleaned path (`server.go:337`) while this route
// stored whatever it was sent. So two spellings of one file — the thing `filepath.Clean`
// exists to collapse — opened it twice, and the harm is D16's own: two working copies,
// last save silently discarding the other.
//
// It was not reachable in production, because the only caller sends a path
// `filepath.Abs` has already cleaned. That is the finding rather than the excuse: the
// route's correctness rested on a property of one caller.
func TestAHandoffCanonicalisesThePathBeforeComparingIt(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	srv.SetHandoffSecret("secret")
	path := pdfOnDisk(t)

	openByPath(t, ts.URL, c, csrf, path)
	srv.mu.Lock()
	before := len(srv.docs)
	srv.mu.Unlock()
	// The stimulus: one document is open, so "the count did not change" below is a
	// statement about the compare and not about an empty registry.
	if before != 1 {
		t.Fatalf("setup: %d documents open, want 1", before)
	}

	// The same file, spelled with an interior "." — what a shell completion, a symlinked
	// launcher, or a file manager passing a relative-ish path produces.
	noisy := filepath.Join(filepath.Dir(path), ".", filepath.Base(path))
	noisy = filepath.Dir(noisy) + string(filepath.Separator) + "." + string(filepath.Separator) + filepath.Base(path)
	if noisy == path {
		t.Fatalf("setup: the noisy spelling %q is identical to the clean one, so this test compares nothing", noisy)
	}

	code, out := handoffTo(t, ts, c, "secret", noisy)
	if code != http.StatusOK || out.Result != "focused" {
		t.Fatalf("handing off %q for an open %q = %d/%q, want 200/focused", noisy, path, code, out.Result)
	}
	srv.mu.Lock()
	after := len(srv.docs)
	srv.mu.Unlock()
	if after != before {
		t.Errorf("a second spelling of the same path opened another copy: %d documents, want %d — D16 is a string compare and the strings were not canonicalised", after, before)
	}
}

// TestAPathIsNotQueuedOnceTheVaultIsOpen — P07 phase close.
//
// `handleHandoff` reads `unlocked` before it decodes the body, so by the time it decides
// to queue, the vault may already have been adopted — and `adoptVault` drains the queue
// exactly once, at that moment. A path appended afterwards waits for an unlock that has
// already happened and will not happen again, so the document silently never opens: the
// user double-clicks a file while typing their passphrase and nothing appears, ever.
//
// The guard is inside `queuePendingOpen` rather than at the call site because that is
// where it shares the lock `adoptVault` sets the vault under. Tested there for the same
// reason — a test through the route could only ever observe the ordinary ordering.
func TestAPathIsNotQueuedOnceTheVaultIsOpen(t *testing.T) {
	_, srv := startServerWith(t)

	// The stimulus: while locked it DOES queue, or "it did not queue" below says nothing.
	if !srv.queuePendingOpen("/tmp/while-locked.pdf") {
		t.Fatal("setup: a locked instance refused to queue, so the unlocked case below is not a contrast")
	}
	srv.mu.Lock()
	queuedWhileLocked := len(srv.pendingOpens)
	srv.mu.Unlock()
	if queuedWhileLocked != 1 {
		t.Fatalf("setup: %d paths queued while locked, want 1", queuedWhileLocked)
	}

	// A real vault, adopted through the real entry point — adoptVault is the one place a
	// vault becomes available and the one place the drain runs, so anything that set
	// srv.vault directly would step over the code under test.
	_, v := unlockedServer(t)
	srv.adoptVault(v)

	if srv.queuePendingOpen("/tmp/after-unlock.pdf") {
		t.Error("a path was queued after the vault opened — the drain has already run, so it waits for an unlock that will not come again and the document never opens")
	}
	srv.mu.Lock()
	left := append([]string(nil), srv.pendingOpens...)
	srv.mu.Unlock()
	if len(left) != 0 {
		t.Errorf("pendingOpens still holds %v after unlock; nothing will ever drain it", left)
	}
}

// TestTheVaultIsAdoptedInExactlyOnePlace — P07 phase close, promoting V80 from an
// inspection to a guard.
//
// The seam inventory carried "production sites assigning `s.vault`: 1" as a row checked
// by eye at each slice close. That is the class of row this project has learned to
// distrust: it is true until someone adds the second one, and nothing tells them.
//
// One is the pass because `adoptVault` is where the pending-open drain hangs. A second
// assignment is a second unlock route, and a queued hand-off would then open when the
// user unlocks one way and silently never open when they unlock the other — the exact
// shape of `closeDocument` carrying its own copy of `tearDownView`, and of `openBrowse`
// being `browseDir` written twice.
func TestTheVaultIsAdoptedInExactlyOnePlace(t *testing.T) {
	sites := 0
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var where []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "s.vault = ") {
				sites++
				where = append(where, fmt.Sprintf("%s:%d", name, i+1))
			}
		}
	}
	// The stimulus: zero would mean the scan is reading the wrong thing, and "no second
	// assignment" is exactly what zero looks like.
	if sites == 0 {
		t.Fatal("the scan found no assignment to s.vault at all — it is not reading what it thinks, and it would report a clean pass forever")
	}
	if sites != 1 {
		t.Errorf("s.vault is assigned in %d places (%v), want 1 — the pending-open drain hangs off adoptVault, so a second unlock route means a handed-off document opens when the user unlocks one way and silently never opens when they unlock the other", sites, where)
	}
}
