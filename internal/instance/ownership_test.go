package instance

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRemoveTakesOnlyTheRecordItWasGiven — /pending 630, 606. An exiting instance's deferred
// removal runs after its listener closed, so a launch can already have cleared that record and
// published its own; the exiting instance then deleted the NEW record, and the next launch started
// a third Nib. Remove now takes a token and leaves any other record where it is.
func TestRemoveTakesOnlyTheRecordItWasGiven(t *testing.T) {
	dir := t.TempDir()
	mine := Record{Addr: "127.0.0.1:4001", Token: "the-exiting-one", Handoff: "h1"}
	theirs := Record{Addr: "127.0.0.1:4002", Token: "the-new-primary", Handoff: "h2"}
	if err := Create(dir, theirs); err != nil {
		t.Fatal(err)
	}
	removed, err := Remove(dir, mine.Token)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, rerr := Read(dir)
	if removed || rerr != nil || got.Token != theirs.Token {
		t.Fatalf("removing by the exiting instance's token took the live one's record (removed %v, now %+v, %v)", removed, got, rerr)
	}
	// And nothing is left beside it: the moved-aside copy is dropped, not stranded.
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("the config directory holds %d entries after a refused removal, want only the record", len(ents))
	}

	// The stimulus the refusal above is measured against: the RIGHT token removes it.
	removed, err = Remove(dir, theirs.Token)
	if err != nil || !removed {
		t.Fatalf("removing by the record's own token did nothing (removed %v, %v)", removed, err)
	}
	if _, err := Read(dir); !os.IsNotExist(err) {
		t.Errorf("after removal by its own token the record still reads (%v)", err)
	}
	if removed, err := Remove(dir, theirs.Token); removed || err != nil {
		t.Errorf("removing an absent record = (%v, %v), want (false, nil)", removed, err)
	}
}

// TestADamagedRecordIsReportedAndClearedAndOnlyThen — /pending 813. An unreadable record was
// "treated as absent" and left in place, so Create (exclusive) refused every later launch's record
// and each ran alongside, for good. Read now names it, and RemoveDamaged clears only a record that
// is still unreadable.
func TestADamagedRecordIsReportedAndClearedAndOnlyThen(t *testing.T) {
	for _, body := range []string{"", "{\"addr\":", `{"addr":"127.0.0.1:9"}`} {
		dir := t.TempDir()
		if err := os.WriteFile(Path(dir), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(dir); !errors.Is(err, ErrDamaged) {
			t.Errorf("%q: Read returned %v, want ErrDamaged — a caller cannot tell it from absent", body, err)
		}
		removed, err := RemoveDamaged(dir)
		if err != nil || !removed {
			t.Errorf("%q: RemoveDamaged = (%v, %v), want it cleared", body, removed, err)
		}
		if err := Create(dir, Record{Addr: "127.0.0.1:4001", Token: "t"}); err != nil {
			t.Errorf("%q: after clearing, a launch still cannot publish: %v", body, err)
		}
	}

	// A record that reads is never "damaged", however it got there.
	dir := t.TempDir()
	if err := Create(dir, Record{Addr: "127.0.0.1:4001", Token: "live"}); err != nil {
		t.Fatal(err)
	}
	if removed, err := RemoveDamaged(dir); removed || err != nil {
		t.Errorf("RemoveDamaged took a readable record (%v, %v)", removed, err)
	}
	if got, err := Read(dir); err != nil || got.Token != "live" {
		t.Errorf("the readable record did not survive RemoveDamaged: %+v, %v", got, err)
	}
}

// TestARecordIsNeverSeenHalfWritten — /pending 606, 813. Create was an O_EXCL open followed by a
// write, so a reader between the two saw an empty file. Now that an unreadable record is CLEARED,
// a reader in that window would delete a record being published, so Create must land it whole.
func TestARecordIsNeverSeenHalfWritten(t *testing.T) {
	dir := t.TempDir()
	var damaged, seen int
	var mu sync.Mutex
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, err := Read(dir)
				mu.Lock()
				if errors.Is(err, ErrDamaged) {
					damaged++
				} else if err == nil {
					seen++
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < 3000; i++ {
		if err := Create(dir, Record{Addr: "127.0.0.1:4001", Token: "t"}); err != nil {
			t.Fatal(err)
		}
		if _, err := Remove(dir, "t"); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	if seen == 0 {
		t.Fatal("setup: the readers never saw a record, so the race was never run")
	}
	if damaged != 0 {
		t.Errorf("a reader saw a half-written record %d times in %d reads of a whole one — a launch would clear a record being published", damaged, seen)
	}
}

// TestCheckTellsATimeoutFromAGoneInstance — /pending 783. A timeout was read as "nobody there", so
// a busy Nib's record was removed and a second Nib started beside it. Gone and Unknown are now
// different answers, and only Gone licenses removal.
func TestCheckTellsATimeoutFromAGoneInstance(t *testing.T) {
	saved := probeTimeout
	probeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { probeTimeout = saved })

	release := make(chan struct{})
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer busy.Close()
	defer close(release)
	if got := Probe(Record{Addr: strings.TrimPrefix(busy.URL, "http://"), Token: "t"}); got != Unknown {
		t.Errorf("an instance that did not answer in time reads %v, want unknown — gone licenses removing a live record", got)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := ln.Addr().String()
	ln.Close()
	if got := Probe(Record{Addr: dead, Token: "t"}); got != Gone {
		t.Errorf("a refused address reads %v, want gone — the ordinary stale record would never be taken over", got)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer other.Close()
	if got := Probe(Record{Addr: strings.TrimPrefix(other.URL, "http://"), Token: "t"}); got != Gone {
		t.Errorf("something that is not this record's Nib reads %v, want gone", got)
	}
}

// TestAHandOffThatOutlastsItsWaitIsNotAFailure — /pending 783. The hand-off route opens the file
// inside the request, and the launch waited the probe's two seconds for it; on a large file it gave
// up, became the primary, and opened the document a second time. A timeout is now its own error.
func TestAHandOffThatOutlastsItsWaitIsNotAFailure(t *testing.T) {
	saved := handoffTimeout
	handoffTimeout = 200 * time.Millisecond
	t.Cleanup(func() { handoffTimeout = saved })

	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer slow.Close()
	defer close(release)
	rec := Record{Addr: strings.TrimPrefix(slow.URL, "http://"), Token: "t", Handoff: "h"}
	if _, _, _, err := HandOff(rec, "/tmp/large.pdf", "test"); !errors.Is(err, ErrHandOffUnanswered) {
		t.Errorf("a hand-off that outlasted its wait returned %v, want ErrHandOffUnanswered — the launch reads any other error as \"become the primary\"", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := ln.Addr().String()
	ln.Close()
	if _, _, _, err := HandOff(Record{Addr: dead, Token: "t", Handoff: "h"}, "/tmp/x.pdf", "test"); err == nil || errors.Is(err, ErrHandOffUnanswered) {
		t.Errorf("a refused hand-off returned %v; nothing has the request, so it must not read as unanswered", err)
	}
}
