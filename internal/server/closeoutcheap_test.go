package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// countCloseOutReads swaps the sweep's mirror read for one that records which ceremonies had their
// document opened, and still reads them — the sweep's behaviour is unchanged, only observed.
func countCloseOutReads(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	was := readMirrorForCloseOut
	readMirrorForCloseOut = func(root, id string, now time.Time) (ceremony.Record, []byte, error) {
		opened = append(opened, id)
		return was(root, id, now)
	}
	t.Cleanup(func() { readMirrorForCloseOut = was })
	return &opened
}

func primaryServer(t *testing.T) (*Server, func(now time.Time)) {
	t.Helper()
	srv, v := unlockedServer(t)
	srv.mu.Lock()
	srv.SetInstanceToken("this-one")
	srv.mu.Unlock()
	return srv, func(now time.Time) { srv.closeOutEnded(v, now) }
}

func liveDirOf(t *testing.T, id string) string {
	t.Helper()
	dir, err := ceremony.MirrorDir(defaultOutputDir(), id)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestTheSweepOpensADocumentOnlyForACeremonyThatMightCloseOut is /pending 360, decision B.
//
// The sweep ran `ceremony.ReadMirror` — a whole-document read, a checksum and, while the document is
// unsigned, a full content digest — over EVERY stored ceremony, on `GET /api/ceremonies` and on
// unlock, to reach a decision that for a live ceremony is "no" from `record.json` alone. Measured
// before the change on 0.8 MB unsigned documents: the sweep's cost grew with the number of stored
// ceremonies; after it, with the number that might close.
//
// **Two live ceremonies and one whose finished document has arrived, in one sweep.** The count is
// the observable, and it is asserted beside the outcome it must not change: the delivered one is
// still read through the strong door and still moved, and the live ones are exactly where they were.
func TestTheSweepOpensADocumentOnlyForACeremonyThatMightCloseOut(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, sweep := primaryServer(t)
	liveA, _, _ := ceremonyOnDisk(t)
	liveB, _, _ := ceremonyOnDisk(t)
	done, _, _ := ceremonyOnDisk(t)
	path := deliveredPathFor(done)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("the finished document"), 0o600); err != nil {
		t.Fatal(err)
	}

	opened := countCloseOutReads(t)
	sweep(time.Now())

	if len(*opened) != 1 || (*opened)[0] != done.ID {
		t.Errorf("the sweep opened the documents of %d ceremonies (%v); only %s could close out. "+
			"A live ceremony's answer is in its record.json, and reading its document costs a "+
			"content digest per ceremony on every visit to the panel", len(*opened), *opened, done.ID)
	}
	// The outcome the count must not have changed, both ways.
	if _, err := os.Stat(liveDirOf(t, done.ID)); !os.IsNotExist(err) {
		t.Errorf("the ceremony whose finished document arrived was NOT closed out (%v) — the cheap "+
			"test may only skip, and it skipped one the strong door closes", err)
	}
	for _, rec := range []ceremony.Record{liveA, liveB} {
		if _, err := os.Stat(liveDirOf(t, rec.ID)); err != nil {
			t.Errorf("a live ceremony left the live set (%v)", err)
		}
	}
}

// TestACeremonyThatMightCloseStillGoesThroughTheStrongDoor is the other half of decision B: the
// cheap test never closes anything.
//
// **A ceremony long past its grace, whose stored document was damaged after it was written.** The
// listing's row says "past" — the cheap test says yes — and `ReadMirror` then refuses the copy
// (its checksum no longer matches), so the folder stays exactly where it is, as it did before the
// cheap test existed. A sweep that closed out on the row alone moves it.
//
// **The control is the same fixture undamaged**, closed out by the same sweep at the same instant:
// without it the assertion above is satisfied by a sweep that never closes anything.
func TestACeremonyThatMightCloseStillGoesThroughTheStrongDoor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, sweep := primaryServer(t)
	damaged, _, _ := ceremonyOnDisk(t)
	whole, _, _ := ceremonyOnDisk(t)
	doc := filepath.Join(liveDirOf(t, damaged.ID), "document.pdf")
	b, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc, b[:len(b)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	later := damaged.Expires.Add(closeOutGrace + time.Hour)
	if whole.Expires.After(later) {
		later = whole.Expires.Add(closeOutGrace + time.Hour)
	}
	for _, rec := range []ceremony.Record{damaged, whole} {
		st := ceremony.ReadStored(defaultOutputDir(), rec.ID, later)
		if !mightCloseOut(st, later) {
			t.Fatalf("setup: a ceremony past its grace does not pass the cheap test (state %q, "+
				"reason %q), so nothing below reaches the strong door", st.State, st.Reason)
		}
	}

	opened := countCloseOutReads(t)
	sweep(later)

	if len(*opened) != 2 {
		t.Errorf("the strong door was asked about %d ceremonies (%v), want both — each passed the "+
			"cheap test and neither may be decided without its mirror being read", len(*opened), *opened)
	}
	if _, err := os.Stat(liveDirOf(t, damaged.ID)); err != nil {
		t.Errorf("a ceremony whose stored document no longer matches its checksum was closed out "+
			"(%v) — moved on the strength of record.json alone, which is the decision the strong "+
			"door exists to refuse", err)
	}
	if _, err := os.Stat(liveDirOf(t, whole.ID)); !os.IsNotExist(err) {
		t.Errorf("the undamaged ceremony past its grace was not closed out (%v), so the damaged "+
			"one's survival proves nothing", err)
	}
}

// TestTheCheapTestSaysNoOnlyWhereTheStrongDoorDoes pins the three ways in, one at a time.
//
// `closeOutReason` can answer yes for an attested end, an arrived document, or a deadline past its
// grace. Each alone must pass the cheap test — a way in it forgot is a ceremony stranded in the live
// set until the grace, read by nothing — and none of them is "no".
func TestTheCheapTestSaysNoOnlyWhereTheStrongDoorDoes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now()
	id := "abababababababababababababababab"
	live := ceremony.Stored{ID: id, State: ceremony.LoadOK, Intent: "We agree", Expires: now.Add(time.Hour)}
	if mightCloseOut(live, now) {
		t.Fatal("setup: a live ceremony with no end state, no delivered document and a deadline " +
			"ahead passes the cheap test, so every arm below would pass against a test that " +
			"always says yes")
	}

	ended := live
	ended.Ended = ceremony.StateDeclined
	if !mightCloseOut(ended, now) {
		t.Error("an attested end does not pass the cheap test — a declined ceremony would never " +
			"reach the strong door")
	}
	if !mightCloseOut(live, live.Expires.Add(closeOutGrace+time.Second)) {
		t.Error("a deadline past its grace does not pass the cheap test — an abandoned ceremony " +
			"would hold its folder and its pins for good")
	}
	path := deliveredPath(live.Intent, live.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("the finished document"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !mightCloseOut(live, now) {
		t.Error("an arrived document does not pass the cheap test — C11's central case would wait " +
			"for the grace and be recorded as abandoned")
	}
	// And a record that did not read is "no" whatever else is true of it.
	for _, class := range []ceremony.LoadState{
		ceremony.LoadAbsent, ceremony.LoadUnparseable, ceremony.LoadVersionSkew, ceremony.LoadUnverifiable,
	} {
		st := ended
		st.State = class
		if mightCloseOut(st, live.Expires.Add(closeOutGrace+time.Second)) {
			t.Errorf("a ceremony classified %v passes the cheap test — its deadline and its end "+
				"state are not facts", class)
		}
	}
}
