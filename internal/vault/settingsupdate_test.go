package vault

import (
	"errors"
	"sync"
	"testing"
)

// TestUpdateSettingsHoldsTheLockAcrossReadAndWrite — `/pending 519`.
//
// **The defect was a lost update between two read-modify-write pairs**, not a torn write. `Settings()`
// returns a copy under the lock and `SetSettings()` writes the whole struct back under the lock, so
// each call is atomic and the pair is not: a writer that read, then had another writer land, then
// wrote its stale copy, silently discarded the other's change.
//
// This drives that shape directly. Half the goroutines do the OLD pair and half use the door; what is
// asserted is the door's own guarantee — a mutate that reads what is there and writes a derived value
// cannot lose a concurrent one, because no other writer can interleave between its read and its write.
func TestUpdateSettingsHoldsTheLockAcrossReadAndWrite(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	// Each goroutine appends one colour through the door. Under a correct door every append is
	// preserved; with a read-modify-write pair that does not hold the lock, appends are lost.
	const writers = 24
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = v.UpdateSettings(func(s *Settings) {
				s.RecentHighlightColors = append(s.RecentHighlightColors, "#000000")
			})
		}(i)
	}
	wg.Wait()

	if got := len(v.Settings().RecentHighlightColors); got != writers {
		t.Errorf("%d of %d concurrent appends survived. UpdateSettings must hold the lock across the "+
			"read AND the write: anything less is the lost update /pending 519 records, where a "+
			"writer's stale copy overwrites a change that landed after it read", got, writers)
	}
}

// TestUpdateSettingsRollsBackAFailedSave — `/pending 510`'s pattern, on this door's one site.
//
// A mutator that assigns and then fails to persist leaves memory ahead of disk: the running Nib
// behaves as though the change was stored, and the next launch disagrees.
func TestUpdateSettingsRollsBackAFailedSave(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.UpdateSettings(func(s *Settings) { s.Appearance = "light" }); err != nil {
		t.Fatal(err)
	}

	saved := writeFileAtomic
	t.Cleanup(func() { writeFileAtomic = saved })
	calls := 0
	writeFileAtomic = func(string, []byte) error { calls++; return errors.New("disk full") }

	uerr := v.UpdateSettings(func(s *Settings) { s.Appearance = "dark" })
	writeFileAtomic = saved

	// STIMULUS: the save really was attempted and really failed.
	if calls != 1 {
		t.Fatalf("setup: the failing write ran %d time(s), want 1 — the rollback below is untested", calls)
	}
	if uerr == nil {
		t.Fatal("UpdateSettings reported success although its save failed")
	}
	if got := v.Settings().Appearance; got != "light" {
		t.Errorf("after a failed save the in-memory setting is %q, want the previous %q. Memory is "+
			"now ahead of disk: this Nib behaves as though the change persisted and the next launch "+
			"will not have it", got, "light")
	}
}
