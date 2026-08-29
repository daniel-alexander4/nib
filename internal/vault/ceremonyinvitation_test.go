package vault

import (
	"encoding/json"
	"strings"
	"testing"
)

// P08.S01 — the invitee's own invitation, stored so a restart can re-arm from it.
//
// The write and the delete ship together, and these drive both. `PruneCeremonySecrets`' own comment
// records why: `RemoveMirror` and `PruneCeremonyPeers` both shipped with zero production callers, so
// the ceremony's residue already had two owners that were never wired. A third store with no delete
// would be the same shape a third time — and this one holds a ceremony's secret inside the text.

func TestAStoredInvitationSurvivesAReopenAndIsPrunedByCeremony(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	const idA = "0123456789abcdef0123456789abcdef"
	const idB = "fedcba9876543210fedcba9876543210"

	if err := v.AddCeremonyInvitation(idA, "nibinv:alpha"); err != nil {
		t.Fatal(err)
	}
	if err := v.AddCeremonyInvitation(idB, "nibinv:beta"); err != nil {
		t.Fatal(err)
	}
	// Stimulus: both are held BEFORE the reopen, or the reopen proves nothing about persistence —
	// it would be comparing two absences. `TestCeremonySecretsSurviveAReopen`'s rule, next door.
	if got, ok := v.CeremonyInvitationFor(idA); !ok || got != "nibinv:alpha" {
		t.Fatalf("setup: %q/%v held before the reopen, want the stored text", got, ok)
	}

	reopened, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reopened.CeremonyInvitationFor(idA); !ok || got != "nibinv:alpha" {
		t.Errorf("after a reopen the invitation reads %q/%v — a restart is exactly the case this "+
			"store exists for, so losing it here loses the whole point", got, ok)
	}

	// Upsert, not accumulate: accepting the same invitation twice is idempotent at the route, and
	// a second row would be one nothing ever finds again.
	if err := reopened.AddCeremonyInvitation(idA, "nibinv:alpha-again"); err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.CeremonyInvitationFor(idA); got != "nibinv:alpha-again" {
		t.Errorf("re-accepting stored %q rather than replacing the row", got)
	}

	n, err := reopened.PruneCeremonyInvitations(idA)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("the prune reports %d, want 1", n)
	}
	if _, ok := reopened.CeremonyInvitationFor(idA); ok {
		t.Error("the pruned ceremony's invitation is still stored — it carries that ceremony's " +
			"secret, so this is key material kept for a ceremony that has ended")
	}
	// The other ceremony is untouched. One ceremony's teardown breaking another is the defect
	// `TestTwoCeremoniesCanShareAPin` exists for, one store along.
	if got, ok := reopened.CeremonyInvitationFor(idB); !ok || got != "nibinv:beta" {
		t.Errorf("pruning %s removed %s's invitation too (%q/%v) — a second live ceremony would "+
			"lose the ability to re-arm", idA, idB, got, ok)
	}

	// And it is durable: a prune that only cleared memory would leave the secret on disk with
	// nothing in the process that could re-read or re-write it.
	third, err := OpenSSH(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := third.CeremonyInvitationFor(idA); ok {
		t.Error("the pruned invitation came back after a reopen — the removal never reached disk")
	}
}

func TestAStoredInvitationNeedsBothHalves(t *testing.T) {
	dir := t.TempDir()
	pub, keyPath := newKey(t)
	v, err := Create(dir, pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	// A row filed under "" is a row no prune can name — the refusal `AddCeremonySecret` and
	// `PruneCeremonyPeers` both make, for the same reason.
	if err := v.AddCeremonyInvitation("", "nibinv:x"); err == nil {
		t.Error("a stored invitation with no ceremony id was accepted — nothing could ever prune it")
	}
	if err := v.AddCeremonyInvitation(strings.Repeat("a", 32), ""); err == nil {
		t.Error("an empty invitation was stored — a re-arm would then fail at ParseInvitation " +
			"with nothing to say why")
	}
	if _, err := v.PruneCeremonyInvitations(""); err == nil {
		t.Error("a prune with no ceremony id was accepted — it would match nothing, or everything")
	}
}

// TestAVaultWrittenBeforeStoredInvitationsStillOpens is inventory row S01-7's other half.
//
// `TestAPayloadFromANewerNibIsRefusedRatherThanSilentlyRewritten` covers the direction that
// matters for safety — a newer payload is refused rather than silently stripped — and it is written
// against `contentsVersion + 1`, so the 2→3 bump needed no edit there. What it cannot see is the
// direction this field actually travels in practice: a vault written by yesterday's Nib, opened by
// today's, which must degrade to "this machine has accepted nothing" rather than to an error.
//
// That reading is what makes the bump safe to make without a migration, so it is asserted rather
// than assumed.
func TestAVaultWrittenBeforeStoredInvitationsStillOpens(t *testing.T) {
	// A payload at the PREVIOUS version, carrying everything that version knew and nothing this
	// one added. Written out rather than derived, so a future bump does not silently retarget it.
	old, err := json.Marshal(Contents{
		Version: 2,
		Recent:  []string{"/tmp/a.pdf"},
		CeremonySecrets: []CeremonySecret{{
			Ceremony:    "0123456789abcdef0123456789abcdef",
			Fingerprint: []byte{1, 2, 3},
			Secret:      []byte{4, 5, 6},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeContents(old)
	if err != nil {
		t.Fatalf("a vault written before stored invitations existed was refused (%v) — every "+
			"vault in the world is one of those on the day this ships", err)
	}
	// Stimulus: the payload really carried its own era's data, so "no invitations" below is a
	// statement about the new field and not about an empty decode.
	if len(got.CeremonySecrets) != 1 {
		t.Fatalf("setup: the pre-bump payload decoded %d secrets, want 1 — this test is not "+
			"reading what it thinks it is", len(got.CeremonySecrets))
	}
	if len(got.CeremonyInvitations) != 0 {
		t.Errorf("a pre-bump payload decoded %d stored invitations, want 0 — the absence must "+
			"read as 'this machine has accepted nothing', which is true of it",
			len(got.CeremonyInvitations))
	}
}
