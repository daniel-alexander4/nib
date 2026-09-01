package server

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestAConvenedDocumentCanBeBroughtIntoLineWithItsOwnFile is /pending 341's design half.
//
// # The state it fixes
//
// The commit doors mutate memory only, so a convened document's bytes reach disk at exactly ONE
// place: `~/nib/ceremonies/<id>/document.pdf`. The file in the user's own matter folder stays the
// PRE-CEREMONY draft — unsigned, carrying no record — and `nib verify` on that file reports
// "unsigned" about a document under a live ceremony. Saving to bring the two into line answered 409
// from `ceremonyFreeze`, and did so **even with no divergence at all**.
//
// # Why the exemption is byte-identity and not something looser
//
// The freeze's own sentence is "Nib will not write DIFFERENT bytes over it". Bytes equal to the
// document under ceremony are definitionally not different, so this is the refusal's own wording
// read literally rather than a new permission. Everything else the freeze protects is untouched:
// the two mutation doors never reach this branch, and a save carrying so much as one changed byte
// is refused exactly as before.
//
// # The two arms, and the second is the one that matters
//
// A test that only checked the exemption would pass against a freeze deleted outright.
func TestAConvenedDocumentCanBeBroughtIntoLineWithItsOwnFile(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	convened, _ := convenedFor(t)

	// The user's matter folder holds the PRE-CEREMONY draft, which is what makes this item's
	// complaint real: that file is not the document under ceremony and never becomes it.
	dir := t.TempDir()
	path := filepath.Join(dir, "lease.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\nthe pre-ceremony draft"), 0o600); err != nil {
		t.Fatal(err)
	}

	doc := &document{name: "lease.pdf", path: path, data: convened}
	srv.addDoc(doc)
	id := doc.id.String()

	save := func(t *testing.T, body []byte) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/save?overwrite=1", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-CSRF-Token", csrf)
		req.Header.Set("X-Nib-Doc", id)
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}

	// ARM 1 — the same bytes. This is what the client sends when the user presses Save on a
	// document they have not edited: `bakedBytes` calls pdf.js `getData()`, the raw loaded bytes,
	// whenever `annotationStorage` is empty.
	if code := save(t, convened); code != http.StatusOK {
		t.Errorf("saving a convened document's OWN bytes to its own path answered %d, want 200. "+
			"The freeze refuses to write 'different bytes', and these are not different — so a "+
			"convened document can never be brought into line with its own file, and the user is "+
			"left with a draft on disk while `nib verify` calls it unsigned", code)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, convened) {
		t.Errorf("the save answered 200 and the file on disk is %d bytes, not the convened "+
			"document's %d — a success reply for a write that did not happen", len(got), len(convened))
	}

	// ARM 2 — ONE byte different, which is every real edit. Still refused, or the exemption has
	// deleted the freeze rather than narrowed it.
	altered := append(append([]byte{}, convened...), '\n')
	if code := save(t, altered); code != http.StatusConflict {
		t.Errorf("saving ALTERED bytes over a convened document answered %d, want 409. The "+
			"exemption is byte-identity; anything else changes the document every other party "+
			"was invited to sign, and their copies stop matching", code)
	}
	// And the file is untouched by the refusal.
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, convened) {
		t.Error("the refused save still changed the file on disk")
	}
}
