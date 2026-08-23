package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// Every place that resolves the addressed document handles the case where it is
// not there. Two shapes count as handling it:
//
//   - resolveDoc(w, r), which writes both refusals itself — 404 for "nothing is
//     open", 409 for "not that one" — and is what thirteen handlers use;
//   - docFor(r) directly, for the six handlers whose nil answer is not a 404
//     (attestations returns an empty list; cosign/quote answers 400; undo, redo,
//     close and doc have their own shapes). Those must handle `err` themselves.
//
// **This guard replaced P03.S01's, and the replacement is deliberate.** S01's
// version matched `doc := s.activeDoc()` and asserted exactly 15 sites. S02 changed
// that idiom, so the old guard went red — and the tempting repair was to loosen its
// regex until it passed, which is how a guard stops guarding. It was rewritten
// instead, and the rewrite is *stronger*: the old one checked only the nil branch,
// this one also requires the 409 arm, which is new surface that had no guard at all.
func TestEveryDocumentResolutionIsHandled(t *testing.T) {
	direct := regexp.MustCompile(`s\.docFor\(r\)`)
	handled := regexp.MustCompile(`err != nil`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	resolveSites, directSites := 0, 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			// Comments are skipped: an earlier run of this guard's predecessor
			// flagged a doc comment quoting the idiom, which both reports a site
			// that does not exist and inflates the count its own stimulus
			// assertion depends on.
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.Contains(line, "s.resolveDoc(w, r)") && !strings.Contains(line, "func (s *Server)") {
				resolveSites++
				continue
			}
			if !direct.MatchString(line) {
				continue
			}
			// docFor's own definition and resolveDoc's call are not call sites.
			if strings.Contains(line, "func (s *Server) docFor") {
				continue
			}
			directSites++
			found := false
			for j := i; j < len(lines) && j <= i+3; j++ {
				if handled.MatchString(lines[j]) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s:%d resolves a document with docFor but does not handle the not-found error within 3 lines", f, i+1)
			}
		}
	}

	// The stimulus, before the response. A regex matching nothing would report
	// every site handled — perfect health over an empty population, forever.
	// resolveDoc's own body contains one docFor call, hence 7 rather than 6.
	// 17, not 13: the four install-only routes (outline, pages, redact, and
	// export's reload branch) gained a resolution during this slice. They never
	// read the open document — they work from posted bytes — so before the
	// registry there was nothing for them to resolve, and "the open one" was
	// unambiguous. It is not any more. The guard demanded this count change be
	// deliberate rather than absorbed, which is the whole reason it names a number.
	// 18, not 17: /api/stamps (v1.109.16) asks whether the addressed document already
	// carries a stamp layer, and it is a document question like any other — it resolves
	// the same way rather than reading whichever document happens to be active.
	// 22, not 18: the same four install-only routes gained a SECOND resolution, an early one
	// (/pending 261, v1.117.126). They work from posted bytes, so they used to parse the whole
	// multipart body and run the PDF operation before the resolve at the commit discovered the
	// document was gone. The early one is advisory and refuses before the body is read; the one
	// at the commit stays authoritative, because the document can be closed while the body is in
	// flight. Two sites per route is the point, not a duplicate to collapse.
	if resolveSites != 22 {
		t.Errorf("expected 22 resolveDoc sites, found %d — update this deliberately if intended", resolveSites)
	}
	// 8, not 7: P06.S02's handleCloseView resolves with docFor rather than resolveDoc,
	// because its not-found branch is a 409 ("that document is no longer open") and
	// resolveDoc's is a 404. Bumped deliberately, which is what the number is for — the
	// guard went red on the first run after the route landed and is the only thing in
	// the tree that would have noticed a resolution site appearing.
	if directSites != 8 {
		t.Errorf("expected 8 direct docFor sites, found %d — update this deliberately if intended", directSites)
	}
}

// ADR-001's second half: ids are monotonic for the life of the process and are
// never reused. The law reduces to an id comparison, so a recycled id defeats it
// silently — an operation pinned to a closed document passes its check against the
// document that took its number.
func TestDocumentIDsAreNeverReused(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}

	s.setDoc(&document{data: pdf})
	first := s.activeDoc().id
	// The non-zero probe: without this, "the ids differ" would also be satisfied
	// by both being the zero value.
	if !first.valid() {
		t.Fatalf("the first document got no usable id: %+v", first)
	}

	s.setDoc(nil) // close
	if s.activeDoc() != nil {
		t.Fatal("closing must leave no active document")
	}

	s.setDoc(&document{data: pdf}) // open another
	second := s.activeDoc().id

	if second == first {
		t.Fatalf("a reopened document reused the closed document's id (%s) — this defeats operation pinning silently", first)
	}
	if second.Seq <= first.Seq {
		t.Errorf("ids must strictly increase: first %d, second %d", first.Seq, second.Seq)
	}
}

// The epoch is why Seq alone is not enough: Seq restarts at 1 when the process
// does, so a client holding an id across a restart on a fixed NIB_ADDR port could
// otherwise name a document the new process has since reassigned.
func TestEachProcessGetsItsOwnEpoch(t *testing.T) {
	a := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")
	b := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")

	if a.epoch == "" || b.epoch == "" {
		t.Fatal("a server was constructed without an epoch; an id could be issued without one")
	}
	if a.epoch == b.epoch {
		t.Error("two servers share an epoch — an id from one process would be accepted by another")
	}

	// And the epoch reaches the id, rather than merely existing on the Server.
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a.setDoc(&document{data: pdf})
	if got := a.activeDoc().id.Epoch; got != a.epoch {
		t.Errorf("id epoch = %q, want the server's %q", got, a.epoch)
	}
}

// The registry holds the document and names it active. Asserted on the registry
// itself rather than through app behaviour: every existing test already proves the
// app behaves, and would go on proving it against a registry that was never
// consulted.
func TestOpenPutsTheDocumentInTheRegistry(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{epoch: "test-epoch"}

	if len(s.docs) != 0 {
		t.Fatalf("a fresh server should hold no documents, got %d", len(s.docs))
	}

	s.setDoc(&document{data: pdf})

	if len(s.docs) != 1 {
		t.Fatalf("registry holds %d documents, want 1", len(s.docs))
	}
	if s.docs[0].id != s.activeID {
		t.Errorf("the open document (%s) is not the active id (%s)", s.docs[0].id, s.activeID)
	}
	if s.activeDoc() != s.docs[0] {
		t.Error("activeDoc() does not return the registry's document")
	}

	s.setDoc(nil)
	if len(s.docs) != 0 || s.activeID.valid() {
		t.Errorf("closing must empty the registry and clear the active id, got %d docs / id %s", len(s.docs), s.activeID)
	}
}
