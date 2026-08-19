package vault

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestPinnedPeersRoundTrip(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	fpA, fpB := []byte{1, 2, 3, 4}, []byte{5, 6, 7, 8}
	if err := v.AddPinnedPeer(fpA, "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddPinnedPeer(fpB, "Bob"); err != nil {
		t.Fatal(err)
	}

	// Persisted across a reopen.
	re, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := re.PinnedPeers(); len(got) != 2 {
		t.Fatalf("after reopen peers = %d, want 2", len(got))
	}

	// Re-pinning an existing fingerprint updates the label, no duplicate.
	if err := re.AddPinnedPeer(fpA, "Alice Smith"); err != nil {
		t.Fatal(err)
	}
	peers := re.PinnedPeers()
	if len(peers) != 2 {
		t.Errorf("re-pin duplicated: peers = %d, want 2", len(peers))
	}
	for _, p := range peers {
		if bytes.Equal(p.Fingerprint, fpA) && p.Label != "Alice Smith" {
			t.Errorf("re-pin label = %q, want %q", p.Label, "Alice Smith")
		}
	}

	// Returned slice is a copy — mutating it must not affect the store.
	peers[0].Fingerprint[0] ^= 0xff
	for _, p := range re.PinnedPeers() {
		if p.Fingerprint[0] != fpA[0] && p.Fingerprint[0] != fpB[0] {
			t.Error("PinnedPeers returned an aliased slice; external mutation leaked into the store")
		}
	}

	// Remove.
	if err := re.RemovePinnedPeer(fpA); err != nil {
		t.Fatal(err)
	}
	got := re.PinnedPeers()
	if len(got) != 1 || !bytes.Equal(got[0].Fingerprint, fpB) {
		t.Errorf("after remove, peers = %+v, want only Bob", got)
	}
}

// TestAnInvitationsPinsAreGoneWhenTheCeremonyEnds is P01's D29 exit criterion, driven the
// way it asks: "a pin created by consuming an invitation is marked with its ceremony and is
// gone when the ceremony ends; a pin the user promoted survives".
//
// The case it exists for: accepting an invitation and then DECLINING the ceremony. Without
// the scope, the strangers on that roster stay in the peer list for good — the user pinned
// four people to read one document and is left pinned to them permanently.
func TestAnInvitationsPinsAreGoneWhenTheCeremonyEnds(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	mine := bytes.Repeat([]byte{0x11}, 32)   // someone the user pinned themselves
	theirs := bytes.Repeat([]byte{0x22}, 32) // a stranger from the roster
	both := bytes.Repeat([]byte{0x33}, 32)   // on the roster AND already a user pin

	if err := v.AddPinnedPeer(mine, "My colleague"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddPinnedPeer(both, "Also my colleague"); err != nil {
		t.Fatal(err)
	}
	const ceremony = "0123456789abcdef0123456789abcdef"
	for _, fp := range [][]byte{theirs, both} {
		if err := v.AddCeremonyPeer(fp, "From the invitation", ceremony); err != nil {
			t.Fatal(err)
		}
	}

	// The stimulus: all three are pinned before the prune, so "gone afterwards" is not
	// true of something that was never there.
	if n := len(v.PinnedPeers()); n != 3 {
		t.Fatalf("setup: %d pins before the prune, want 3", n)
	}

	n, err := v.PruneCeremonyPeers(ceremony)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("the prune removed %d pins, want 1 — only the stranger belongs to the ceremony", n)
	}

	left := map[string]bool{}
	for _, p := range v.PinnedPeers() {
		left[hex.EncodeToString(p.Fingerprint)] = true
	}
	if left[hex.EncodeToString(theirs)] {
		t.Error("a pin created by consuming an invitation survived the ceremony — the user is " +
			"left pinned to a stranger they met to read one document (D29)")
	}
	if !left[hex.EncodeToString(mine)] {
		t.Error("a pin the USER made was removed by a ceremony's prune")
	}
	if !left[hex.EncodeToString(both)] {
		t.Error("a peer the user had already pinned was removed because a ceremony also named " +
			"them — an invitation must not be able to take away a relationship the user made")
	}

	// An empty id must not match every user pin and delete the lot.
	if _, err := v.PruneCeremonyPeers(""); err == nil {
		t.Error("pruning with an empty ceremony id was accepted — that matches every user pin")
	}
}
