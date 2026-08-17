package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
