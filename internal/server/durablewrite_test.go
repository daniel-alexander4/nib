package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/ceremony"
)

// TestTheContributionReachesDiskInsideStore is P08.S02's central property, driven at runtime.
//
// # Why it exists
//
// S02's whole point is that a hop's co-signed document reaches DISK before it reaches the WIRE, and
// until this test nothing in the tree observed the first half at all (/pending 343). The named
// searches that established it: `grep -rn '\.Store(' --include=*_test.go internal/` returned one
// hit and it was a `sync/atomic` in `internal/rendezvous`; `grep -rn 'nib/ceremonies\|ceremonies/'
// build/*.sh` returned nothing, so no harness on any machine inspects the mirror either.
//
// So the property was held by ONE source scan (`l3_test.go`, asserting `Store`'s body contains
// `persistContribution(`), and a scan cannot tell a call that writes a file from a call that
// returns early, spawns a goroutine, or writes somewhere else. This test asks the filesystem.
//
// # The three arms
//
// The success arm is the reader the inventory was missing. The failure arm is the reason the
// interface was WIDENED — `Store` returns an error so D24's "signed but not saved" is a sentence
// the signer sees, where before it was a `log.Printf` into a stderr a double-clicked launch sends
// nowhere. And the cache-survives-the-failure arm is the clause that keeps a failed local write
// from turning into a SECOND signature on the reconnect, which is what D24 forbids.
func TestTheContributionReachesDiskInsideStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A real convened document, so `persistContribution` takes its mirror branch rather than the
	// `ErrNoRecord` early return every ordinary two-party co-sign takes.
	final, _ := convenedFor(t)
	rec, err := ceremony.Extract(final)
	if err != nil {
		t.Fatalf("setup: the convened document carries no readable record (%v) — Store would "+
			"then return nil having written nothing, and every assertion below would be "+
			"asserting the early return", err)
	}
	inbound := []byte("%PDF-1.4\nthe document as it arrived at this hop")

	cer := &ceremonyID{}
	if err := cer.Store(inbound, final); err != nil {
		t.Fatalf("Store on a writable ~/nib returned %v; want nil", err)
	}

	// THE DURABLE HALF. Named against the exact path, because "something was written under ~/nib"
	// is also true of the received-document path and of a save.
	path := filepath.Join(home, "nib", "ceremonies", rec.ID, "document.pdf")
	got, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatalf("after Store, %s does not exist (%v). The hop's signature is nowhere on this "+
			"machine, so a crash between signing and delivering loses it — which is the one "+
			"thing D24 exists to prevent, and the ordering guard in l3_test.go cannot see it "+
			"because a call that writes nothing still contains the call", path, rerr)
	}
	if !bytes.Equal(got, final) {
		t.Errorf("the mirrored document is %d bytes and the co-signed document is %d — the file "+
			"on disk is not the bytes this hop signed", len(got), len(final))
	}

	// THE IN-MEMORY HALF, read directly rather than through `Cached`, which reads THROUGH to disk
	// (P08.S02, C01) and would therefore report a hit off the file the arm above just checked.
	cer.mu.Lock()
	hit := cer.reDelivery[reDeliverKey(inbound)]
	cer.mu.Unlock()
	if !bytes.Equal(hit, final) {
		t.Error("Store wrote the mirror and did not record the in-process re-delivery entry; a " +
			"reconnect inside this process would then re-sign rather than re-deliver")
	}

	// THE FAILURE ARM. ~/nib becomes a regular file, which makes MkdirAll refuse deterministically
	// on every platform and as any user — the fixture `receivedwrite_test.go` established, because
	// a chmod-based one is a no-op for root and on Windows.
	if err := os.RemoveAll(filepath.Join(home, "nib")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "nib"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := &ceremonyID{}
	err = broken.Store(inbound, final)
	if err == nil {
		t.Fatal("Store reported success while its durable write had failed. The error is the " +
			"only channel D24's 'signed but not saved' has — `Receive` turns it into a " +
			"persistError and the signer is told — so swallowing it here is the failure " +
			"reaching nobody, silently, on the machine that just signed")
	}
	if !bytes.Contains([]byte(err.Error()), []byte(rec.ID)) {
		t.Errorf("the persist failure reads %q and does not name the ceremony it lost; the "+
			"user is told something failed and not which proceeding", err)
	}
	// And the peer is still owed the signature: a failed local write must not cost the cache,
	// or the reconnect re-signs and stacks a second block from one identity (D24/D25).
	broken.mu.Lock()
	hit = broken.reDelivery[reDeliverKey(inbound)]
	broken.mu.Unlock()
	if !bytes.Equal(hit, final) {
		t.Error("a failed durable write also dropped the in-memory re-delivery entry, so the " +
			"initiator's reconnect would be served a fresh Contribute — a second signature " +
			"from one identity on one document")
	}
}
