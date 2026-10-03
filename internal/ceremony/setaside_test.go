package ceremony

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"
)

// TestEveryCeremonyFolderRefusesARelativeRoot — /pending 631 and 813. `EndedDir` refused a relative root
// and `MirrorDir` did not, so a mirror (a non-convener's only copy of its own signature) could be written
// where the close-out can never move it, and `ReadMirrorFor` silently dropped `ended/`. One root rule,
// at `storeDir`, for every folder and both listings.
func TestEveryCeremonyFolderRefusesARelativeRoot(t *testing.T) {
	rec, doc := convened(t)
	t.Chdir(t.TempDir()) // a regression writes under the working directory; keep it out of the tree
	const rel = "nib"
	if _, err := MirrorDir(rel, rec.ID); !errors.Is(err, ErrRootNotAbsolute) {
		t.Errorf("MirrorDir: %v; want ErrRootNotAbsolute", err)
	}
	if _, err := WriteMirror(rel, rec, doc); !errors.Is(err, ErrRootNotAbsolute) {
		t.Errorf("WriteMirror: %v; want ErrRootNotAbsolute", err)
	}
	if _, _, err := ReadMirrorFor(rel, rec, time.Now()); !errors.Is(err, ErrRootNotAbsolute) {
		t.Errorf("ReadMirrorFor: %v; want ErrRootNotAbsolute, never an answer from the live folder alone", err)
	}
	if _, err := ListStored(rel, time.Now()); !errors.Is(err, ErrRootNotAbsolute) {
		t.Errorf("ListStored: %v; want ErrRootNotAbsolute", err)
	}
	if _, err := ListEnded(rel); !errors.Is(err, ErrRootNotAbsolute) {
		t.Errorf("ListEnded: %v; want ErrRootNotAbsolute", err)
	}
	if _, err := os.Stat(rel); !os.IsNotExist(err) {
		t.Errorf("a refused store still created %q under the working directory (%v)", rel, err)
	}
}

// TestAFolderSetAsideByTheCloseOutIsStillRead — /pending 813. Another proceeding took this id and closed
// out first, so `ended/<id>` is its; this proceeding's own close-out then sets its folder aside beside it.
// The close-out leaves nothing live, and `ReadMirrorFor` passes over the other proceeding to this one's copy.
func TestAFolderSetAsideByTheCloseOutIsStillRead(t *testing.T) {
	root := t.TempDir()
	recA, docA := convened(t)
	recB, docB := convened(t)
	recB.ID = recA.ID
	cert, key, fp := identity(t, "Attacker")
	recB.Roster[0].Fingerprint = fp
	if err := recB.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteMirror(root, recB, docB); err != nil {
		t.Fatal(err)
	}
	if err := CloseOutMirror(root, recB.ID); err != nil {
		t.Fatal(err)
	}
	live, err := WriteMirror(root, recA, docA)
	if err != nil {
		t.Fatal(err)
	}
	if err := CloseOutMirror(root, recA.ID); err != nil {
		t.Fatalf("the second close-out under a taken id: %v; want it moved aside", err)
	}
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Errorf("the live folder is still there after its close-out (%v)", err)
	}
	pdf, ended, err := ReadMirrorFor(root, recA, time.Now())
	if err != nil || !ended || !bytes.Equal(pdf, docA) {
		t.Fatalf("ended=%v err=%v %d bytes; want this proceeding's copy from the set-aside folder", ended, err, len(pdf))
	}
}
