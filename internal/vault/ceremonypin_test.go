package vault

import (
	"encoding/hex"
	"testing"
)

// TestACeremonyPinNeverRenamesAPinTheUserMade — a live defect this slice's first caller would
// have shipped.
//
// `AddCeremonyPeer`'s doc says an existing pin is never downgraded, and the code honoured that
// for `Ceremony` and not for `Label`: `addPinned` assigned the label unconditionally. So
// accepting an invitation would overwrite the user's own private nickname for a peer they had
// pinned themselves, with whatever label the convener published — a stranger editing this user's
// peer list by inviting them. Invisible until now because `AddCeremonyPeer` had no production
// caller at all.
func TestACeremonyPinNeverRenamesAPinTheUserMade(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	fp := make([]byte, 32)
	for i := range fp {
		fp[i] = 0x11
	}
	if err := v.AddPinnedPeer(fp, "Mum's solicitor"); err != nil {
		t.Fatal(err)
	}
	// Stimulus: the user's pin really is there, under their own name and with no ceremony.
	pins := v.PinnedPeers()
	if len(pins) != 1 || pins[0].Label != "Mum's solicitor" || len(pins[0].Ceremonies) != 0 {
		t.Fatalf("setup: %+v — the assertions below need one USER pin", pins)
	}

	if err := v.AddCeremonyPeer(fp, "Party 2", "ceremony-abc"); err != nil {
		t.Fatal(err)
	}
	got := v.PinnedPeers()
	if len(got) != 1 {
		t.Fatalf("a ceremony pin DUPLICATED an existing peer: %+v", got)
	}
	if got[0].Label != "Mum's solicitor" {
		t.Errorf("the ceremony renamed the user's peer to %q. The label is the user's own note "+
			"to themselves and the invitation's is a stranger's text; a convener must not be "+
			"able to edit this machine's peer list by inviting it.", got[0].Label)
	}
	if len(got[0].Ceremonies) != 0 {
		t.Errorf("the user's pin was downgraded to ceremonies %q — a prune would then take "+
			"away a relationship the user established", got[0].Ceremonies)
	}

	// The control, and it is what stops the fix from becoming "never update a label": the USER
	// renaming their own peer still works.
	if err := v.AddPinnedPeer(fp, "The solicitor"); err != nil {
		t.Fatal(err)
	}
	if again := v.PinnedPeers(); again[0].Label != "The solicitor" {
		t.Errorf("the user could not rename their own peer (label is %q) — the fix has "+
			"frozen labels rather than protecting them", again[0].Label)
	}

	// And an UNNAMED pin is not a name to protect: a ceremony may fill one in.
	other := make([]byte, 32)
	for i := range other {
		other[i] = 0x22
	}
	if err := v.AddPinnedPeer(other, ""); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyPeer(other, "Party 3", "ceremony-abc"); err != nil {
		t.Fatal(err)
	}
	for _, p := range v.PinnedPeers() {
		if hex.EncodeToString(p.Fingerprint) == hex.EncodeToString(other) && p.Label != "Party 3" {
			t.Errorf("an unnamed pin was left unnamed (%q) — a blank row in the peer list "+
				"reads as a bug rather than as an unnamed peer", p.Label)
		}
	}
}

// TestTwoCeremoniesCanShareAPin — measured before it was written, which is why it exists.
//
// `PinnedPeer.Ceremony` was one id. A second ceremony naming a party the first had already
// pinned left the pin scoped to the FIRST, so whichever ceremony ended first took the pin with
// it and the other's next arm failed with "that peer isn't pinned" — a peer the user had never
// unpinned, gone, with no sentence anywhere. Measured on a probe before the fix: two
// `AddCeremonyPeer` calls for one fingerprint left `ceremony-A`, and pruning A removed the pin
// outright.
//
// The same counterparty across two matters is the ordinary case for this product's user.
func TestTwoCeremoniesCanShareAPin(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	fp := make([]byte, 32)
	for i := range fp {
		fp[i] = 0x33
	}
	if err := v.AddCeremonyPeer(fp, "Bob", "ceremony-A"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyPeer(fp, "Bob", "ceremony-B"); err != nil {
		t.Fatal(err)
	}
	// Stimulus: ONE pin carrying BOTH scopes. A second pin would be a different bug (a
	// duplicate peer) and would satisfy the survival assertion below for the wrong reason.
	pins := v.PinnedPeers()
	if len(pins) != 1 {
		t.Fatalf("two ceremonies produced %d pins for one fingerprint, want 1: %+v", len(pins), pins)
	}
	if len(pins[0].Ceremonies) != 2 {
		t.Fatalf("the pin carries %q, want both ceremonies — with only one scope recorded, "+
			"whichever ceremony ends first takes the pin the other still needs",
			pins[0].Ceremonies)
	}

	n, err := v.PruneCeremonyPeers("ceremony-A")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruning ceremony A reported %d pins touched, want 1 — the count is what a "+
			"caller logs, and a scope removed is a pin touched", n)
	}
	after := v.PinnedPeers()
	if len(after) != 1 {
		t.Fatalf("ending ceremony A removed a peer ceremony B still needs — B's next arm would " +
			"refuse an unpinned peer the user never unpinned")
	}
	if len(after[0].Ceremonies) != 1 || after[0].Ceremonies[0] != "ceremony-B" {
		t.Errorf("after pruning A the pin carries %q, want [ceremony-B] — A's scope must go "+
			"even though the pin stays, or the pin outlives every ceremony that made it",
			after[0].Ceremonies)
	}

	// And the last scope going takes the pin: a revocable pin that survives its last ceremony
	// is a permanent pin, which is the whole of what D29 forbids.
	if _, err := v.PruneCeremonyPeers("ceremony-B"); err != nil {
		t.Fatal(err)
	}
	if left := v.PinnedPeers(); len(left) != 0 {
		t.Errorf("after both ceremonies ended the peer list still holds %+v", left)
	}
}

// TestAUserPinIsNeverGivenACeremonyScope — the other direction, and it is the one that would
// turn the fix above into a new defect.
//
// If `AddCeremonyPeer` added a scope to a pin the USER made, the next prune would take away a
// relationship the user established — which is exactly what `Ceremonies`' one-way-promotion rule
// exists to prevent, and a set makes the mistake easier to make than a single field did.
func TestAUserPinIsNeverGivenACeremonyScope(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	fp := make([]byte, 32)
	for i := range fp {
		fp[i] = 0x44
	}
	if err := v.AddPinnedPeer(fp, "My solicitor"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyPeer(fp, "Party 2", "ceremony-C"); err != nil {
		t.Fatal(err)
	}
	if got := v.PinnedPeers(); len(got) != 1 || len(got[0].Ceremonies) != 0 {
		t.Fatalf("a user pin was given a ceremony scope (%+v) — the next prune would unpin a "+
			"peer the user chose", got)
	}
	if _, err := v.PruneCeremonyPeers("ceremony-C"); err != nil {
		t.Fatal(err)
	}
	if got := v.PinnedPeers(); len(got) != 1 {
		t.Errorf("a ceremony's prune removed the user's own pin")
	}
}
