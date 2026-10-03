package ceremony

import (
	"errors"
	"os"
	"testing"
)

// TestALateWriteDoesNotResurrectAClosedOutCeremony — /pending 585, 807 R8.
//
// `WriteName`, `WriteMe` and `WriteTermination` each `MkdirAll`'d the LIVE folder, so a write
// landing after ADR-012's move re-created `ceremonies/<id>/` — which `ListStored` shows as a live,
// record-less ceremony — and a late termination there escaped the write-once check, because the
// moved one sits in `ended/<id>/`, and then superseded it on read.
func TestALateWriteDoesNotResurrectAClosedOutCeremony(t *testing.T) {
	rec, cert, key := terminationFixture(t)
	root := t.TempDir()
	if _, err := WriteMirror(root, rec, nil); err != nil {
		t.Fatal(err)
	}
	stopped, err := SignTermination(rec, StateStopped, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTermination(root, stopped); err != nil {
		t.Fatal(err)
	}
	if err := CloseOutMirror(root, rec.ID); err != nil {
		t.Fatal(err)
	}
	live, _ := MirrorDir(root, rec.ID)
	ended, _ := EndedDir(root, rec.ID)
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatalf("setup: the close-out left the live folder (%v)", err)
	}

	if err := WriteName(root, rec.ID, "Lease renewal"); err != nil {
		t.Errorf("WriteName after the close-out: %v", err)
	}
	if err := WriteMe(root, rec.ID, "AB"+rec.ID); err != nil {
		t.Errorf("WriteMe after the close-out: %v", err)
	}
	if err := WriteVerification(root, rec.ID, Verification{Presented: true}); err != nil {
		t.Errorf("WriteVerification after the close-out: %v", err)
	}
	completed, err := SignTermination(rec, StateCompleted, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTermination(root, completed); !errors.Is(err, ErrTerminationConflict) {
		t.Errorf("a late termination naming a different end state was accepted (%v) — the moved "+
			"one is the end state this machine already recorded", err)
	}

	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Errorf("a write after the close-out re-created %s, which the listing shows as a live "+
			"ceremony with no record (%v)", live, err)
	}
	if got := readName(ended); got != "Lease renewal" {
		t.Errorf("the name written after the close-out is %q in the ended folder, want it there", got)
	}
	if got, err := ReadTermination(root, rec); err != nil || got.State != StateStopped {
		t.Errorf("the end state reads %q (%v), want the original %q", got.State, err, StateStopped)
	}

	// And where a live folder exists again anyway (a write that resolved it an instant before the
	// move, or a late hop's `WriteMirror`), the write-once check still sees the moved end state.
	if err := os.MkdirAll(live, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteTermination(root, completed); !errors.Is(err, ErrTerminationConflict) {
		t.Errorf("beside a re-created live folder, a different end state was accepted (%v), and the "+
			"reader prefers the live copy — it would supersede the recorded one", err)
	}
	if got, err := ReadTermination(root, rec); err != nil || got.State != StateStopped {
		t.Errorf("beside a re-created live folder the end state reads %q (%v), want %q", got.State, err, StateStopped)
	}
}
