package pdfops

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestTheSelfOverwriteDoorAnswersOnIdentityNotSpelling — /pending 569's door, on its own.
//
// The two surfaces' tests drive `nib split` and `/api/split-pages` and assert the user's document
// survives. They cannot distinguish the reasons a particular output was cleared, so the cases that
// separate "compares file identity" from "compares path strings" are asserted here, against the
// door directly.
func TestTheSelfOverwriteDoorAnswersOnIdentityNotSpelling(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(src, []byte("the document"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.pdf")
	if err := os.WriteFile(other, []byte("something else"), 0o644); err != nil {
		t.Fatal(err)
	}
	absent := filepath.Join(dir, "not-there-yet.pdf")

	t.Run("the source itself collides", func(t *testing.T) {
		got, ok := OutputOverwritingSource(src, []string{other, absent, src})
		if !ok || got != src {
			t.Errorf("got (%q, %v), want (%q, true) — the source was not recognised in a list "+
				"that contains it", got, ok, src)
		}
	})

	t.Run("a different spelling of the source collides", func(t *testing.T) {
		// `<dir>/./sub/../doc.pdf`, uncleaned: a string comparison sees a different path and
		// `os.Stat` sees the same inode. This is the case the door exists to get right, because
		// the spelling of an output is built by joining, not by the user.
		sub := filepath.Join(dir, "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		// Concatenated, NOT filepath.Join — which Cleans, so `Join(sub, "..", "doc.pdf")` comes
		// back byte-identical to src and the case proves nothing. The stimulus assertion below
		// caught exactly that on this test's first run.
		sep := string(os.PathSeparator)
		spelled := sub + sep + ".." + sep + "doc.pdf"
		if spelled == src {
			t.Fatal("setup: the two spellings are identical, so this case proves nothing")
		}
		if _, ok := OutputOverwritingSource(src, []string{spelled}); !ok {
			t.Errorf("%q was not recognised as the source: the check is comparing path strings, "+
				"and one file has as many spellings as it has ways of being joined", spelled)
		}
	})

	t.Run("a link to the source collides", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("SKIP (not a pass): POSIX symlink")
		}
		link := filepath.Join(dir, "link.pdf")
		if err := os.Symlink(src, link); err != nil {
			t.Skipf("SKIP (not a pass): symlinks unavailable here: %v", err)
		}
		if _, ok := OutputOverwritingSource(src, []string{link}); !ok {
			t.Error("a symlink pointing at the source was not recognised as the source — the CLI's " +
				"write door resolves a link before writing, so this one destroys the document")
		}
	})

	t.Run("an unrelated or absent output does not", func(t *testing.T) {
		if got, ok := OutputOverwritingSource(src, []string{other, absent}); ok {
			t.Errorf("reported %q as the source: the refusal has widened from 'this IS the "+
				"document' to 'this name is taken', which would break re-running a split", got)
		}
	})

	t.Run("an empty or missing source collides with nothing", func(t *testing.T) {
		// A document uploaded through the browser has no path, and `nib split -` reads stdin.
		// Both arrive as "", and both must be a clean no.
		if got, ok := OutputOverwritingSource("", []string{src, other}); ok {
			t.Errorf("an empty source matched %q — a document with no file on disk would be "+
				"refused a split into any folder holding a file", got)
		}
		if got, ok := OutputOverwritingSource(absent, []string{src, other}); ok {
			t.Errorf("a source that does not exist matched %q — an unreadable input must fail as "+
				"an unreadable input, not as a collision", got)
		}
	})

	t.Run("no outputs at all", func(t *testing.T) {
		if _, ok := OutputOverwritingSource(src, nil); ok {
			t.Error("an empty output list reported a collision")
		}
	})
}

// TestASplitPartsModeIsOneValueBothDoorsName — /pending 570.
//
// Both split doors pass this constant rather than a literal of their own, so the ONE thing that can
// still split them is the constant changing. 0644 is the decision (`SplitPartMode`'s own comment
// carries the reasoning and the declared exposure); this pins the value so a change to it is a
// change somebody made on purpose rather than one that rides along.
func TestASplitPartsModeIsOneValueBothDoorsName(t *testing.T) {
	if got := SplitPartMode; got != 0o644 {
		t.Errorf("SplitPartMode = %#o, want 0644. A split part is a user's document going into a "+
			"folder they named, not one of nib's own files — 0600 is the habit that made the GUI "+
			"write parts another account could not read while the CLI wrote parts it could", got)
	}
}
