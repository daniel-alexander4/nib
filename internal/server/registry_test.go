package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// The registry replaced a single `s.doc` field with an ordered list plus an active
// id, and D6's whole argument for doing it behind one accessor is that the ~16
// call sites keep their guard shape — `doc := …; if doc == nil` — so the change
// stays reviewable and no handler grows a new way to be nil-unsafe.
//
// This asserts the property rather than the text. Asserting the text unchanged
// would fail on any correct rewrite; asserting "there is a guard near the read" is
// what actually protects against the failure, which is a handler that reads the
// document and then uses it without checking.
func TestEveryActiveDocReadIsGuarded(t *testing.T) {
	read := regexp.MustCompile(`doc := s\.activeDoc\(\)`)
	guard := regexp.MustCompile(`if doc == nil`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	sites := 0
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
			// Comments are skipped, and that is not housekeeping: the first run of
			// this guard flagged activeDoc()'s own doc comment, which quotes the
			// idiom as an example. A scan that matches prose reports sites that do
			// not exist and inflates its own count — the same vacuity it exists to
			// catch, one level up.
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if !read.MatchString(line) {
				continue
			}
			sites++
			// Within five lines, because that is the existing idiom's reach: the
			// guard follows the read immediately in every current site, and five
			// leaves room for a comment without admitting a distant check.
			found := false
			for j := i + 1; j < len(lines) && j <= i+5; j++ {
				if guard.MatchString(lines[j]) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s:%d reads the active document but does not guard it against nil within 5 lines", f, i+1)
			}
		}
	}

	// The stimulus, asserted before the response: a regex that matched nothing
	// would report every site guarded — perfect health over an empty population,
	// forever. 15 is the measured count of real call sites; the sixteenth document
	// read is docResponse's, which goes through activeDocLocked() under a held lock
	// with its own nil check. A change to this number is a real change to how many
	// places can be nil-unsafe, and should be seen.
	if sites != 15 {
		t.Errorf("expected 15 active-document reads, found %d — if that is intended, update this count deliberately", sites)
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
