package vault

import (
	"bytes"
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
