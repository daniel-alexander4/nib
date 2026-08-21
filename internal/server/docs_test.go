package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"nib/internal/sign"
	"nib/internal/testpdf"
	"testing"
)

// P06.S03 — GET /api/docs, the client's only way to learn what the server holds.

func TestDocsListsEveryOpenDocumentInOrderWithTheActiveOne(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)

	// The empty case first, and it is not a formality: the client uses an empty list to
	// decide "collapse to the launch state", so an empty registry answering anything
	// other than an empty list would send it the wrong way.
	empty := fetchDocs(t, ts, c)
	if len(empty.Docs) != 0 || empty.ActiveID != "" {
		t.Fatalf("a fresh server reports %d documents / active %q, want none", len(empty.Docs), empty.ActiveID)
	}

	first := openByPath(t, ts.URL, c, csrf, path)
	second := openByPath(t, ts.URL, c, csrf, path)
	third := openByPath(t, ts.URL, c, csrf, path)

	got := fetchDocs(t, ts, c)
	if len(got.Docs) != 3 {
		t.Fatalf("registry reports %d documents, want 3", len(got.Docs))
	}
	// ORDER, not just membership: the client renders tabs from this list, so a set that
	// happens to contain the right ids in the wrong order is a strip that reshuffles on
	// every reload.
	want := []string{first.ID, second.ID, third.ID}
	for i, id := range want {
		if got.Docs[i].ID != id {
			t.Errorf("document %d is %s, want %s — the list is not in registry order", i, got.Docs[i].ID, id)
		}
	}
	if got.ActiveID != third.ID {
		t.Errorf("activeId = %s, want the last-opened %s", got.ActiveID, third.ID)
	}
	// Each entry has to be usable on its own — the client re-opens documents from these,
	// so a list of bare ids would not be enough.
	if got.Docs[0].Path != first.Path {
		t.Errorf("the listed document does not carry its path (%q)", got.Docs[0].Path)
	}
}

// TestDocsFollowsACloseView — the list is what the client reconciles against, so it has
// to move when the registry does.
func TestDocsFollowsACloseView(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	first := openByPath(t, ts.URL, c, csrf, path)
	second := openByPath(t, ts.URL, c, csrf, path)
	if n := len(fetchDocs(t, ts, c).Docs); n != 2 {
		t.Fatalf("setup: %d documents, want 2", n)
	}

	resp := writeTo(t, c, csrf, ts.URL+"/api/close-view", first.ID)
	resp.Body.Close()

	got := fetchDocs(t, ts, c)
	if len(got.Docs) != 1 || got.Docs[0].ID != second.ID {
		t.Errorf("after closing one, the list is %v, want just %s", ids(got), second.ID)
	}
	if got.ActiveID != second.ID {
		t.Errorf("activeId = %s, want %s", got.ActiveID, second.ID)
	}
}

func fetchDocs(t *testing.T, ts *httptest.Server, c *http.Client) docsResponse {
	t.Helper()
	resp, err := c.Get(ts.URL + "/api/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/docs = %d, want 200", resp.StatusCode)
	}
	var dr docsResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatalf("decoding /api/docs: %v", err)
	}
	return dr
}

func ids(d docsResponse) []string {
	out := make([]string, 0, len(d.Docs))
	for _, x := range d.Docs {
		out = append(out, x.ID)
	}
	return out
}

// TestAPathlessDocumentKeepsItsNameAcrossAReload.
//
// Three routes — /api/upload, /api/combine, /api/office — set `docResponse.Name` after the
// fact and stored it nowhere. `docName(doc.path)` returns "" for a path-less document, so
// `GET /api/docs` reported `Name: ""` for them forever after; `web/app.js` only assigns
// `originalName` when `meta.name` is truthy, so a reload rendered the tab as "Untitled" and
// export defaults reverted to "document".
//
// That is the second half of the defect /api/docs was built to fix — its own comment names
// uploads, combines and office conversions as the population, and it restored their
// REACHABILITY without their identity.
func TestAPathlessDocumentKeepsItsNameAcrossAReload(t *testing.T) {
	_, s := startServerWith(t)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	doc := &document{path: "", name: "Lease Agreement.pdf", data: pdf, sig: sign.Verify(pdf)}
	installed, err := s.addDocCapped(doc)
	if err != nil {
		t.Fatal(err)
	}

	// STIMULUS: the install response names it, which is what always worked.
	if got := s.docResponse(installed).Name; got != "Lease Agreement.pdf" {
		t.Fatalf("the install response does not name the document (%q) — this test is not "+
			"measuring the reload", got)
	}

	// The reload: a fresh GET /api/docs, which is what the client rebuilds tabs from.
	rr := httptest.NewRecorder()
	s.handleDocs(rr, httptest.NewRequest(http.MethodGet, "/api/docs", nil))
	var out docsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range out.Docs {
		if d.ID != installed.id.String() {
			continue
		}
		found = true
		if d.Name != "Lease Agreement.pdf" {
			t.Errorf("/api/docs reports Name %q for an uploaded document — the client "+
				"renders that tab as Untitled and exports default to \"document\"", d.Name)
		}
	}
	if !found {
		t.Fatal("the document is not in /api/docs at all")
	}
}
