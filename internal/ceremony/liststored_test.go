package ceremony

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// P08.S03 — the listing, and the four ways a stored ceremony fails to load.
//
// # Why four and not two
//
// `ReadMirror` collapses every failure into one error and its only production caller collapses that
// into one 404, so a Nib update mid-ceremony — which moves `FormatVersion` and makes `Verify` refuse
// — reads identically to a forged record and to a folder the user deleted. Three of those brick a
// ceremony while looking alike, and one of them, skew, must stay PRUNABLE, which a verdict of "does
// not verify" would forbid.

// storedFixture writes one verifiable ceremony and returns (root, id).
func storedFixture(t *testing.T) (string, string) {
	t.Helper()
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := WriteMirror(root, r, nil); err != nil {
		t.Fatal(err)
	}
	return root, r.ID
}

func TestOneUnloadableCeremonyDoesNotCostTheOthers(t *testing.T) {
	root, good := storedFixture(t)
	// Two more, so the middle one can be broken and the others watched. Distinct ids, written
	// through the same door the production path uses.
	cert, key, cfp := identity(t, "Convener2")
	_, _, afp := identity(t, "B")
	broken := draft(t, cfp, afp)
	if err := broken.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteMirror(root, broken, nil); err != nil {
		t.Fatal(err)
	}

	// Stimulus: all of them load cleanly BEFORE anything is broken. Without this the assertions
	// below are comparing one absence against another.
	before, err := ListStored(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 {
		t.Fatalf("setup: %d ceremonies stored, want 2", len(before))
	}
	for _, s := range before {
		if s.State != LoadOK {
			t.Fatalf("setup: %s reads as %s (%s) before anything was broken", s.ID, s.State, s.Reason)
		}
	}

	dir, err := MirrorDir(root, broken.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "record.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	after, err := ListStored(root, time.Now())
	if err != nil {
		t.Fatalf("one damaged ceremony made the whole listing fail: %v — that is the defect C12 "+
			"exists to prevent, since the user's remedy would then be to find and delete it by hand", err)
	}
	if len(after) != 2 {
		t.Fatalf("the listing dropped an entry: %d, want 2 — a ceremony that cannot be read must "+
			"still be NAMED, or the user cannot act on it", len(after))
	}
	var sawGood, sawBroken bool
	for _, s := range after {
		switch s.ID {
		case good:
			sawGood = s.State == LoadOK
		case broken.ID:
			sawBroken = s.State == LoadUnparseable
			if !strings.Contains(s.Reason, "damaged") {
				t.Errorf("the unparseable entry's sentence does not say it is damaged: %q", s.Reason)
			}
		}
	}
	if !sawGood {
		t.Error("the intact ceremony stopped loading because a different one was damaged")
	}
	if !sawBroken {
		t.Error("the damaged ceremony is not reported as unparseable")
	}
}

func TestAVersionSkewIsNotReportedAsDamageOrForgery(t *testing.T) {
	root, id := storedFixture(t)
	dir, err := MirrorDir(root, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	// Stimulus: it loads cleanly at this build's version first, so the change below is the only
	// thing that moved.
	if got := ReadStored(root, id, time.Now()); got.State != LoadOK {
		t.Fatalf("setup: the fixture reads as %s (%s) before its version was bumped", got.State, got.Reason)
	}
	m["version"] = FormatVersion + 1
	bumped, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "record.json"), bumped, 0o600); err != nil {
		t.Fatal(err)
	}

	got := ReadStored(root, id, time.Now())
	if got.State != LoadVersionSkew {
		t.Fatalf("a record from a newer Nib reads as %s, want %s — %q", got.State, LoadVersionSkew, got.Reason)
	}
	// The sentence matters as much as the class. This is the one failure of the four that is
	// nobody's fault and has a remedy the user can act on.
	if !strings.Contains(got.Reason, "newer version of Nib") {
		t.Errorf("the skew sentence does not name the cause: %q", got.Reason)
	}
	if !strings.Contains(got.Reason, "update Nib") {
		t.Errorf("the skew sentence names no remedy: %q — this is the one failure of the four "+
			"that is nobody's fault and that the user can actually fix", got.Reason)
	}
	// **Against the accusatory sentence specifically, not against a word.** The first draft of
	// this check rejected any reason containing "damaged" and failed on *"It is not damaged"* —
	// a substring test that could not tell a denial from an accusation. What must not appear is
	// the phrase `LoadUnverifiable` uses, because that is the one that blames somebody.
	if strings.Contains(got.Reason, "does not verify") {
		t.Errorf("a version skew is reported in the vocabulary of forgery: %q — the vault says "+
			"the equivalent in plain language (checkContentsVersion) and this is that sentence "+
			"for the mirror", got.Reason)
	}
}

func TestAnAbsentRecordIsNotAnError(t *testing.T) {
	root := t.TempDir()
	// A ceremony directory that exists with nothing in it — what a torn write leaves, because
	// `WriteMirror` writes the record LAST.
	id := strings.Repeat("ab", 16)
	dir, err := MirrorDir(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got := ReadStored(root, id, time.Now())
	if got.State != LoadAbsent {
		t.Fatalf("an empty ceremony directory reads as %s, want %s", got.State, LoadAbsent)
	}
	list, err := ListStored(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("the listing found %d entries, want 1 — a directory with no record must still "+
			"be named, or a torn write is invisible", len(list))
	}
}

func TestTheListingDoesNotOpenTheDocument(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := WriteMirror(root, r, nil); err != nil {
		t.Fatal(err)
	}
	dir, err := MirrorDir(root, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	// **The stimulus IS the assertion here.** A `document.pdf` that is not a PDF at all: anything
	// that parsed it — `sign.Verify`, `ContentDigest`, `ReadMirror` — would fail or spend real time
	// on it. The listing must not notice.
	if err := os.WriteFile(filepath.Join(dir, "document.pdf"),
		[]byte("this is not a PDF, and the listing must never find out"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ReadStored(root, r.ID, time.Now())
	if got.State != LoadOK {
		t.Fatalf("the listing reads as %s (%s) — it opened document.pdf, which at 1000 pages "+
			"costs 195ms per ceremony on a request path", got.State, got.Reason)
	}
	if got.Intent != r.Intent {
		t.Errorf("the listing lost the recital: %q want %q", got.Intent, r.Intent)
	}
	if len(got.Roster) != len(r.Roster) {
		t.Errorf("the listing reports %d parties, want %d", len(got.Roster), len(r.Roster))
	}
}
