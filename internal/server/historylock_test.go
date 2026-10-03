package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"nib/internal/instance"
	"nib/internal/sign"
)

// TestUndoAndRedoVerifyOutsideTheGlobalLock — /pending 783.
//
// Both routes ran `sign.Verify` inside `s.mu`, unlike every commit door. Measured on this machine
// before the fix: 50–110 ms of hold for one signature on a 3.3–5 MB document and 140–295 ms for
// three, during which every other route — and, before the token went lock-free, the instance
// probe a second launch makes on a two-second budget — waited. The verification is observed from
// the inside: if `s.mu` is held while it runs, TryLock fails.
func TestUndoAndRedoVerifyOutsideTheGlobalLock(t *testing.T) {
	for _, route := range []string{"undo", "redo"} {
		t.Run(route, func(t *testing.T) {
			s := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")
			snapshot := []byte("%PDF-1.4 the snapshot")
			doc := s.addDoc(&document{data: []byte("%PDF-1.4 the current state")})
			if route == "undo" {
				doc.undo = [][]byte{snapshot}
			} else {
				doc.redo = [][]byte{snapshot}
			}
			calls, heldDuring := 0, 0
			saved := historyVerify
			t.Cleanup(func() { historyVerify = saved })
			historyVerify = func(b []byte) sign.Status {
				calls++
				if s.mu.TryLock() {
					s.mu.Unlock()
				} else {
					heldDuring++
				}
				return sign.Verify(b)
			}
			rr := httptest.NewRecorder()
			if route == "undo" {
				s.handleUndo(rr, httptest.NewRequest(http.MethodPost, "/api/undo", nil))
			} else {
				s.handleRedo(rr, httptest.NewRequest(http.MethodPost, "/api/redo", nil))
			}
			// The stimulus: the step happened and verified something, or "never under the
			// lock" is true of a route that never verified.
			if rr.Code != http.StatusOK || calls == 0 || !bytes.Equal(doc.data, snapshot) {
				t.Fatalf("setup: the %s did not step (code %d, %d verifications, data %q)", route, rr.Code, calls, doc.data)
			}
			if heldDuring != 0 {
				t.Errorf("the %s verified the signature while holding s.mu (%d of %d calls) — every other route waits for it, and a large signed document holds the global lock for hundreds of milliseconds", route, heldDuring, calls)
			}
		})
	}
}

// TestAHistoryStepCommitsOnlyTheSnapshotItVerified is the other half of moving the verification out
// of the lock: the stack can move while it runs. Here a concurrent edit pushes a new snapshot during
// the first verification, so the verdict in hand describes bytes that are no longer the top. The
// step must start again on the new top — committing the old verdict beside the new bytes would show
// a signature status for a document that is not the one on screen.
func TestAHistoryStepCommitsOnlyTheSnapshotItVerified(t *testing.T) {
	s := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")
	stale := []byte("%PDF-1.4 the snapshot that was on top when the verification began")
	fresh := signedFixture(t)
	doc := s.addDoc(&document{data: []byte("%PDF-1.4 the current state")})
	doc.undo = [][]byte{stale}

	saved := historyVerify
	t.Cleanup(func() { historyVerify = saved })
	moved := false
	historyVerify = func(b []byte) sign.Status {
		if !moved {
			moved = true
			s.mu.Lock()
			doc.undo = append(doc.undo, fresh)
			s.mu.Unlock()
		}
		return sign.Verify(b)
	}
	rr := httptest.NewRecorder()
	s.handleUndo(rr, httptest.NewRequest(http.MethodPost, "/api/undo", nil))
	if rr.Code != http.StatusOK || !moved {
		t.Fatalf("setup: the undo did not run its verification (code %d, moved %v)", rr.Code, moved)
	}
	if !bytes.Equal(doc.data, fresh) {
		t.Fatalf("the undo installed %d bytes, want the snapshot on top at commit time (%d bytes)", len(doc.data), len(fresh))
	}
	if doc.sig.State != sign.Verify(fresh).State || doc.sig.State == sign.Unsigned {
		t.Errorf("the installed bytes are signed but doc.sig reads %v — the verdict of the snapshot that was on top BEFORE the stack moved was committed beside the one that is", doc.sig.State)
	}
	if len(doc.undo) != 1 || !bytes.Equal(doc.undo[0], stale) {
		t.Errorf("the stack under the step is wrong: %d entries", len(doc.undo))
	}
}

// TestTheInstanceProbeAnswersWhileTheGlobalLockIsHeld — /pending 783. A probe is answered on a
// two-second budget; while it read its token under `s.mu`, any route holding the lock that long
// made a live Nib read as a dead one, and the launch cleared its record and started a second Nib.
func TestTheInstanceProbeAnswersWhileTheGlobalLockIsHeld(t *testing.T) {
	s := New(os.DirFS("."), os.DirFS("."), t.TempDir(), "test")
	s.SetInstanceToken("probe-token")
	s.SetHandoffSecret("handoff-secret")
	s.mu.Lock()
	defer s.mu.Unlock()

	answered := make(chan int, 1)
	go func() {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/instance", nil)
		req.Header.Set(instance.HeaderToken, "probe-token")
		s.handleInstance(rr, req)
		answered <- rr.Code
	}()
	select {
	case code := <-answered:
		if code != http.StatusOK {
			t.Errorf("the probe answered %d with the right token, want 200", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the instance probe waited on s.mu — a Nib busy for two seconds reads as dead to the next launch")
	}

	// The hand-off's secret check is the same read; a wrong secret is refused without the lock.
	refused := make(chan int, 1)
	go func() {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/handoff", nil)
		req.Header.Set(instance.HeaderHandoff, "wrong")
		s.handleHandoff(rr, req)
		refused <- rr.Code
	}()
	select {
	case code := <-refused:
		if code != http.StatusForbidden {
			t.Errorf("a wrong hand-off secret answered %d, want 403", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the hand-off's secret check waited on s.mu")
	}
}
