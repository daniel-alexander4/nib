package server

import (
	"net/http"
	"testing"

	"nib/internal/ceremony"
)

// TestADeclineDuringAVaultReopenStillRevokesThePins — /pending 710 R4-4.
//
// `declineCeremony` returned on a nil vault and said "the sweep at the next unlock reaches this
// ceremony". It does not: `closeOutReason` refuses every ceremony that is not `LoadOK`, and a party
// declining at its own hop holds no record — so the pins, the ceremony secret and the stored
// invitation stayed for good. The vault is nil mid-session inside `handleVaultImport`'s window;
// this drives that state directly (the file on disk still opens) and asserts the reopened vault
// holds none of this ceremony's pins.
func TestADeclineDuringAVaultReopenStillRevokesThePins(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	me := myFingerprint(t, c, ts.URL)
	invitation, _ := inviteFor(t, me)
	v := srv.unlockedVault()
	if v == nil {
		t.Fatal("setup: the vault is locked, so nothing below can pin anything")
	}
	before := len(v.PinnedPeers())
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/ceremony/accept",
		acceptRequest{Invitation: invitation}); code != http.StatusOK {
		t.Fatalf("accept: %d %s", code, body)
	}
	if len(v.PinnedPeers()) != before+1 {
		t.Fatal("setup: accepting pinned nothing, so the revocation below would prove nothing")
	}
	inv, err := ceremony.ParseInvitation(invitation)
	if err != nil {
		t.Fatal(err)
	}

	// The import window: the vault file is in place and the in-memory vault is gone.
	srv.mu.Lock()
	srv.vault = nil
	srv.mu.Unlock()
	srv.declineCeremony(&ceremonyID{inv: inv})

	srv.ensureUnlocked()
	reopened := srv.unlockedVault()
	if reopened == nil {
		t.Fatal("setup: the vault on disk does not reopen, so this test cannot see what it holds")
	}
	if got := len(reopened.PinnedPeers()); got != before {
		t.Errorf("a decline that landed while the vault was being reopened left %d ceremony pin(s) "+
			"— nothing reaches them later, because the close-out sweep refuses a ceremony this "+
			"machine holds no record for", got-before)
	}
}
