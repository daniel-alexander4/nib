package ceremony

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAReceiptThatCannotBeReadIsNotWrittenOver — /pending 584, ADR-012's write-once rule.
//
// `WriteReceipt` wrote unless the read SUCCEEDED, so every read failure was treated as "no receipt
// yet" — and its write is a rename, which replaces whatever is there. Only an absent receipt
// licenses writing one. Two failures are driven, because they reach the write by different roads:
// one the file's permissions refuse, and one that opens and will not parse.
func TestAReceiptThatCannotBeReadIsNotWrittenOver(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	first := Receipt{Ceremony: id, State: StateDeclined, ObservedAt: time.Unix(1_700_000_000, 0).UTC()}
	later := Receipt{Ceremony: id, State: StateAbandoned, ObservedAt: time.Unix(1_800_000_000, 0).UTC()}

	t.Run("no receipt is written", func(t *testing.T) {
		// The CONTROL: the refusal below must not be a door that never writes.
		root := t.TempDir()
		if err := WriteReceipt(root, id, first); err != nil {
			t.Fatalf("a first receipt was refused: %v", err)
		}
		if got, err := ReadReceipt(root, id); err != nil || got.State != StateDeclined {
			t.Fatalf("the first receipt reads back as (%q, %v)", got.State, err)
		}
	})

	t.Run("unreadable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads a 0000 file, so this failure cannot be staged")
		}
		root := t.TempDir()
		if err := WriteReceipt(root, id, first); err != nil {
			t.Fatal(err)
		}
		dir, err := EndedDir(root, id)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, receiptFile)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		werr := WriteReceipt(root, id, later)
		if cerr := os.Chmod(path, 0o600); cerr != nil {
			t.Fatal(cerr)
		}
		if werr == nil {
			t.Error("a second receipt was written over one this machine could not open")
		}
		if errors.Is(werr, ErrReceiptConflict) {
			t.Errorf("the refusal is ErrReceiptConflict (%v), which the close-out does not log — "+
				"nothing was compared, so nothing conflicted", werr)
		}
		got, rerr := ReadReceipt(root, id)
		if rerr != nil || got.State != StateDeclined || !got.ObservedAt.Equal(first.ObservedAt) {
			t.Fatalf("after the refused write the receipt reads (%q, %v, %v), want the first "+
				"observation %q untouched", got.State, got.ObservedAt, rerr, StateDeclined)
		}
	})

	t.Run("unparseable", func(t *testing.T) {
		root := t.TempDir()
		dir, err := EndedDir(root, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, receiptFile)
		damaged := []byte(`{"ceremony":"` + id + `","state":"declined","observed_at":`)
		if err := os.WriteFile(path, damaged, 0o600); err != nil {
			t.Fatal(err)
		}
		if werr := WriteReceipt(root, id, later); werr == nil {
			t.Error("a second receipt was written over one that would not parse")
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, damaged) {
			t.Fatalf("the damaged receipt was replaced: it now reads %q — what was left of the "+
				"first observation is gone", got)
		}
	})
}

// TestTheCloseOutFlushesBothFoldersTheMoveTouched — /pending 584's other half.
//
// The move is one `os.Rename`, which is atomic and not durable: the entry that appeared in
// `ended/` and the one that left `ceremonies/` both live in directories nothing flushed. An fsync
// leaves nothing on disk to assert on, so the test watches the door the flush goes through.
func TestTheCloseOutFlushesBothFoldersTheMoveTouched(t *testing.T) {
	rec, _, _ := terminationFixture(t)
	root := t.TempDir()
	if _, err := WriteMirror(root, rec, nil); err != nil {
		t.Fatal(err)
	}
	live, err := MirrorDir(root, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	ended, err := EndedDir(root, rec.ID)
	if err != nil {
		t.Fatal(err)
	}

	var flushed []string
	movedFirst := true
	real := syncDir
	syncDir = func(dir string) {
		// The flush is only worth anything AFTER the rename it is there to make durable.
		if _, serr := os.Stat(ended); serr != nil {
			movedFirst = false
		}
		flushed = append(flushed, dir)
		real(dir)
	}
	t.Cleanup(func() { syncDir = real })

	if err := CloseOutMirror(root, rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, serr := os.Stat(ended); serr != nil {
		t.Fatalf("setup: the close-out did not move the folder: %v", serr)
	}
	for _, want := range []string{filepath.Dir(ended), filepath.Dir(live)} {
		found := false
		for _, got := range flushed {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the close-out did not flush %s (it flushed %q) — the move is a change to "+
				"that directory, and unflushed it can be undone by a power loss after the vault "+
				"stores have already gone", want, flushed)
		}
	}
	if !movedFirst {
		t.Error("a directory was flushed before the folder had moved, so the flush covers nothing")
	}

	// A second pass finds nothing to move and must not claim to have flushed a move.
	flushed = nil
	if err := CloseOutMirror(root, rec.ID); err != nil {
		t.Fatal(err)
	}
	if len(flushed) != 0 {
		t.Errorf("a close-out with nothing to move flushed %q", flushed)
	}
}
