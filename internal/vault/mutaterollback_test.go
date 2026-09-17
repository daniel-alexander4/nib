package vault

import (
	"bytes"
	"errors"
	"strconv"
	"testing"
)

// withFailingWrite makes every vault write fail until the returned function is called, which
// restores the real writer and reports how many writes were attempted. The count is the STIMULUS
// check: a rollback assertion over a save that never ran asserts nothing.
func withFailingWrite(t *testing.T) func() int {
	t.Helper()
	saved := writeFileAtomic
	calls := 0
	writeFileAtomic = func(string, []byte) error { calls++; return errors.New("disk full") }
	t.Cleanup(func() { writeFileAtomic = saved })
	return func() int {
		writeFileAtomic = saved
		return calls
	}
}

// newVault is a vault on a fresh temp dir, unlocked by a fresh key.
func newVault(t *testing.T) *Vault {
	t.Helper()
	pub, keyPath := newKey(t)
	v, err := Create(t.TempDir(), pub, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestAFailedSaveRestoresWhatTheMutationOverwroteInPlace — `/pending 510`.
//
// # Why these four sites and not four easier ones
//
// Every mutator in this package now routes through `mutateLocked`, and a rollback test written
// over a plain field — the draft string, the appearance — passes whether the door's snapshot is
// deep or shallow, because there is nothing for a shallow copy to share. These four are the sites
// where `before := v.contents` would share a backing array with the change it is supposed to undo,
// so the rollback is REAL here and vacuous there:
//
//   - DeleteImage filters `Images[:0]` in place, so the survivors are already shuffled down over
//     the array a header-only restore would hand back — the deleted image stays gone and the last
//     survivor appears twice.
//   - AddCeremonySecret upserts through `&v.contents.CeremonySecrets[i]`, having first ZEROED the
//     secret it replaces. A header restore returns the new secret and the old one is gone: the
//     invitation the convener can no longer re-issue, which is what the vault holds these for.
//   - AddCeremonyInvitation and AddPinnedPeer each assign into an element of a shared array.
//
// The assertion in each case is the same one: after a failed write, memory holds what disk holds.
func TestAFailedSaveRestoresWhatTheMutationOverwroteInPlace(t *testing.T) {
	t.Run("DeleteImage filters in place", func(t *testing.T) {
		v := newVault(t)
		var ids []string
		for _, name := range []string{"a", "b", "c"} {
			img, err := v.AddImage(name, "image/png", []byte(name))
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, img.ID)
		}

		done := withFailingWrite(t)
		derr := v.DeleteImage(ids[0])
		if writes := done(); writes != 1 {
			t.Fatalf("setup: the failing write ran %d time(s), want 1", writes)
		}
		if derr == nil {
			t.Fatal("DeleteImage reported success although its save failed")
		}

		got := v.Images()
		if len(got) != 3 {
			t.Fatalf("after a failed delete the library holds %d image(s), want the original 3 — "+
				"memory is ahead of disk, and the next launch will still have all three", len(got))
		}
		for i, want := range ids {
			if got[i].ID != want || string(got[i].Data) != string([]byte{"abc"[i]}) {
				t.Errorf("image %d is %q/%q after the rollback, want %q/%q. `Images[:0]` compacts "+
					"the SHARED backing array, so restoring a slice header puts the old length "+
					"back over elements the filter has already moved",
					i, got[i].ID, got[i].Data, want, string([]byte{"abc"[i]}))
			}
		}
	})

	t.Run("AddCeremonySecret upserts through a pointer", func(t *testing.T) {
		v := newVault(t)
		const cer = "0123456789abcdef0123456789abcdef"
		fp := bytes.Repeat([]byte{0x11}, 32)
		first := bytes.Repeat([]byte{0xAA}, 32)
		if err := v.AddCeremonySecret(cer, fp, first); err != nil {
			t.Fatal(err)
		}

		done := withFailingWrite(t)
		aerr := v.AddCeremonySecret(cer, fp, bytes.Repeat([]byte{0xBB}, 32))
		if writes := done(); writes != 1 {
			t.Fatalf("setup: the failing write ran %d time(s), want 1", writes)
		}
		if aerr == nil {
			t.Fatal("AddCeremonySecret reported success although its save failed")
		}

		got, ok := v.CeremonySecret(cer, fp)
		if !ok {
			t.Fatal("the secret is gone entirely after a failed re-issue; disk still holds it")
		}
		if !bytes.Equal(got, first) {
			t.Errorf("after a failed re-issue the secret in memory is %x…, want the stored %x…. "+
				"The upsert writes through &v.contents.CeremonySecrets[i] and zeroes the secret "+
				"it replaces, so a snapshot that shares that array restores neither",
				got[:4], first[:4])
		}
	})

	t.Run("AddCeremonyInvitation upserts into an element", func(t *testing.T) {
		v := newVault(t)
		const cer = "0123456789abcdef0123456789abcdef"
		if err := v.AddCeremonyInvitation(cer, "the-invitation-as-accepted"); err != nil {
			t.Fatal(err)
		}

		done := withFailingWrite(t)
		aerr := v.AddCeremonyInvitation(cer, "a-replacement-that-never-landed")
		if writes := done(); writes != 1 {
			t.Fatalf("setup: the failing write ran %d time(s), want 1", writes)
		}
		if aerr == nil {
			t.Fatal("AddCeremonyInvitation reported success although its save failed")
		}

		got, ok := v.CeremonyInvitationFor(cer)
		if !ok || got != "the-invitation-as-accepted" {
			t.Errorf("after a failed re-store the invitation is %q (present=%v), want the stored "+
				"one. This machine would re-arm with text the disk does not hold", got, ok)
		}
	})

	t.Run("AddPinnedPeer relabels an element", func(t *testing.T) {
		v := newVault(t)
		fp := bytes.Repeat([]byte{0x22}, 32)
		if err := v.AddPinnedPeer(fp, "the name the user chose"); err != nil {
			t.Fatal(err)
		}

		done := withFailingWrite(t)
		aerr := v.AddPinnedPeer(fp, "a rename that never landed")
		if writes := done(); writes != 1 {
			t.Fatalf("setup: the failing write ran %d time(s), want 1", writes)
		}
		if aerr == nil {
			t.Fatal("AddPinnedPeer reported success although its save failed")
		}

		pins := v.PinnedPeers()
		if len(pins) != 1 {
			t.Fatalf("the peer list holds %d pin(s) after a failed relabel, want 1", len(pins))
		}
		if pins[0].Label != "the name the user chose" {
			t.Errorf("after a failed relabel the pin reads %q, want the stored %q",
				pins[0].Label, "the name the user chose")
		}
	})
}

// TestAFailedSaveRestoresTheKeySlots — the door covers everything save() persists, and the key
// slots are the half that is not in Contents.
//
// RemoveKey shifts the slot slice IN PLACE (`append(v.ssh[:idx], v.ssh[idx+1:]...)`), so a
// snapshot that keeps the same array restores a length over slots that have already moved down —
// the removed key stays removed and the last one appears twice. A vault that believes it holds a
// slot the file does not, or has dropped one the file still has, is the enrolment equivalent of
// the same defect: the next launch disagrees about who can open it.
func TestAFailedSaveRestoresTheKeySlots(t *testing.T) {
	v := newVault(t)
	var added []string
	for i := 0; i < 2; i++ {
		pub, keyPath := newKey(t)
		if err := v.AddKey(pub, keyPath); err != nil {
			t.Fatal(err)
		}
		added = append(added, pub)
	}
	before := v.Keys()
	if len(before) != 3 {
		t.Fatalf("setup: %d slot(s) enrolled, want 3", len(before))
	}

	done := withFailingWrite(t)
	rerr := v.RemoveKey(added[0])
	if writes := done(); writes != 1 {
		t.Fatalf("setup: the failing write ran %d time(s), want 1", writes)
	}
	if rerr == nil {
		t.Fatal("RemoveKey reported success although its save failed")
	}

	after := v.Keys()
	if len(after) != len(before) {
		t.Fatalf("after a failed removal %d slot(s) are enrolled, want the original %d — this "+
			"Nib believes a key was unenrolled and the file still authorises it",
			len(after), len(before))
	}
	for i := range before {
		if after[i].PubKey != before[i].PubKey || after[i].KeyPath != before[i].KeyPath {
			t.Errorf("slot %d is %q after the rollback, want %q. The removal shifts the slice in "+
				"place, so a snapshot sharing its array restores a length and not the slots",
				i, keyID(after[i].PubKey), keyID(before[i].PubKey))
		}
	}

	// And a failed ADD must not leave a slot enrolled that the file does not carry.
	pub, keyPath := newKey(t)
	done = withFailingWrite(t)
	aerr := v.AddKey(pub, keyPath)
	if writes := done(); writes != 1 {
		t.Fatalf("setup: the failing write ran %d time(s), want 1", writes)
	}
	if aerr == nil {
		t.Fatal("AddKey reported success although its save failed")
	}
	if got := len(v.Keys()); got != len(before) {
		t.Errorf("after a failed enrolment %d slot(s) are enrolled, want %d", got, len(before))
	}
}

// TestAFailedSaveLeavesNothingBehindAtEveryRoutedMutator sweeps the rest of the door's callers.
//
// It is the census half: the four sites above are where the rollback is HARD, and these are the
// ones where it is easy and therefore where an unrouted site would hide. Each drives one mutator
// against a failing write and asserts the vault reads back exactly as it did before the call.
func TestAFailedSaveLeavesNothingBehindAtEveryRoutedMutator(t *testing.T) {
	const cer = "0123456789abcdef0123456789abcdef"
	fp := bytes.Repeat([]byte{0x33}, 32)

	cases := []struct {
		name string
		// seed puts the vault in the state the mutation starts from.
		seed func(t *testing.T, v *Vault)
		// change is the mutation whose save will fail.
		change func(v *Vault) error
		// read is the fact that must be unchanged afterwards.
		read func(v *Vault) string
	}{
		{
			name:   "AddImage",
			change: func(v *Vault) error { _, err := v.AddImage("n", "image/png", []byte("d")); return err },
			read:   func(v *Vault) string { return strconv.Itoa(len(v.Images())) },
		},
		{
			name:   "SetExternalSigner",
			change: func(v *Vault) error { return v.SetExternalSigner([]byte("p12"), []byte("c"), nil) },
			read: func(v *Vault) string {
				if _, ok := v.ExternalSigner(); ok {
					return "present"
				}
				return "absent"
			},
		},
		{
			name: "ClearExternalSigner",
			seed: func(t *testing.T, v *Vault) {
				if err := v.SetExternalSigner([]byte("p12"), []byte("c"), nil); err != nil {
					t.Fatal(err)
				}
			},
			change: func(v *Vault) error { return v.ClearExternalSigner() },
			read: func(v *Vault) string {
				if _, ok := v.ExternalSigner(); ok {
					return "present"
				}
				return "absent"
			},
		},
		{
			name:   "SetIdentityIfAbsent",
			change: func(v *Vault) error { _, _, err := v.SetIdentityIfAbsent([]byte("cert"), []byte("key")); return err },
			read: func(v *Vault) string {
				if _, _, ok := v.Identity(); ok {
					return "present"
				}
				return "absent"
			},
		},
		{
			name: "RemovePinnedPeer",
			seed: func(t *testing.T, v *Vault) {
				if err := v.AddPinnedPeer(fp, "peer"); err != nil {
					t.Fatal(err)
				}
			},
			change: func(v *Vault) error { return v.RemovePinnedPeer(fp) },
			read:   func(v *Vault) string { return strconv.Itoa(len(v.PinnedPeers())) },
		},
		{
			name: "PruneCeremonyPeers",
			seed: func(t *testing.T, v *Vault) {
				if err := v.AddCeremonyPeer(fp, "peer", cer); err != nil {
					t.Fatal(err)
				}
			},
			change: func(v *Vault) error { _, err := v.PruneCeremonyPeers(cer); return err },
			read:   func(v *Vault) string { return strconv.Itoa(len(v.PinnedPeers())) },
		},
		{
			name:   "SetProfile",
			change: func(v *Vault) error { return v.SetProfile(map[string]string{"name": "Dan"}) },
			read:   func(v *Vault) string { return strconv.Itoa(len(v.Profile())) },
		},
		{
			name:   "UpdateSettings",
			change: func(v *Vault) error { return v.UpdateSettings(func(s *Settings) { s.Appearance = "light" }) },
			read:   func(v *Vault) string { return v.Settings().Appearance },
		},
		{
			name:   "AddRecent",
			change: func(v *Vault) error { return v.AddRecent("/tmp/a.pdf") },
			read:   func(v *Vault) string { return strconv.Itoa(len(v.Recent())) },
		},
		{
			name:   "SetCeremonyDraft",
			change: func(v *Vault) error { return v.SetCeremonyDraft("a roster and a recital") },
			read: func(v *Vault) string {
				d, _ := v.CeremonyDraft()
				return d
			},
		},
		{
			name: "ClearCeremonyDraft",
			seed: func(t *testing.T, v *Vault) {
				if err := v.SetCeremonyDraft("a roster and a recital"); err != nil {
					t.Fatal(err)
				}
			},
			change: func(v *Vault) error { return v.ClearCeremonyDraft() },
			read: func(v *Vault) string {
				d, _ := v.CeremonyDraft()
				return d
			},
		},
		{
			name:   "AddCeremonySecret",
			change: func(v *Vault) error { return v.AddCeremonySecret(cer, fp, bytes.Repeat([]byte{9}, 32)) },
			read:   func(v *Vault) string { return strconv.Itoa(len(v.CeremonySecrets(cer))) },
		},
		{
			name: "PruneCeremonySecrets",
			seed: func(t *testing.T, v *Vault) {
				if err := v.AddCeremonySecret(cer, fp, bytes.Repeat([]byte{9}, 32)); err != nil {
					t.Fatal(err)
				}
			},
			change: func(v *Vault) error { _, err := v.PruneCeremonySecrets(cer); return err },
			read:   func(v *Vault) string { return strconv.Itoa(len(v.CeremonySecrets(cer))) },
		},
		{
			name:   "AddCeremonyInvitation",
			change: func(v *Vault) error { return v.AddCeremonyInvitation(cer, "an invitation") },
			read: func(v *Vault) string {
				i, _ := v.CeremonyInvitationFor(cer)
				return i
			},
		},
		{
			name: "PruneCeremonyInvitations",
			seed: func(t *testing.T, v *Vault) {
				if err := v.AddCeremonyInvitation(cer, "an invitation"); err != nil {
					t.Fatal(err)
				}
			},
			change: func(v *Vault) error { _, err := v.PruneCeremonyInvitations(cer); return err },
			read: func(v *Vault) string {
				i, _ := v.CeremonyInvitationFor(cer)
				return i
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newVault(t)
			if tc.seed != nil {
				tc.seed(t, v)
			}
			want := tc.read(v)

			done := withFailingWrite(t)
			cerr := tc.change(v)
			writes := done()

			if writes != 1 {
				t.Fatalf("setup: the failing write ran %d time(s), want 1 — the assertion below "+
					"would pass on a mutator that never tried to persist", writes)
			}
			if cerr == nil {
				t.Fatal("the mutator reported success although its save failed")
			}
			if got := tc.read(v); got != want {
				t.Errorf("after a failed save the vault reads %q, want the pre-change %q. Memory "+
					"is ahead of disk: this Nib behaves as though the change persisted and the "+
					"next launch will not have it", got, want)
			}
		})
	}
}

// TestAFailedSaveRestoresASlotsWrappedBytes — `/pending 549`, the door's snapshot over the half
// of the vault that decides who can open it.
//
// # Why it drives the door directly instead of a mutator
//
// The door's contract is over an ARBITRARY apply: "applies apply … and puts memory back exactly
// as it was when the write fails". Every mutator of `v.ssh` today appends or shifts whole slots
// — AddKey appends a fresh one and RemoveKey shifts the slice — so none of them can tell a
// one-level copy from a deep one, and TestAFailedSaveRestoresTheKeySlots above is green under
// either. Worse, it CANNOT tell: `Keys()` deliberately omits `Wrapped`, so a rollback that hands
// back the mutation's wrapped key is invisible to every assertion in this package that goes
// through the public surface.
//
// That is the whole reason this is written as a defect this package does not yet contain. A
// rewrap written through an element of `v.ssh` is the exact shape `AddCeremonySecret` already
// uses on the contents side, and the day someone writes one, a vault whose in-memory slot no
// longer matches the file's is a vault that believes it is sealed to a key the file was never
// sealed to — the enrolment half of the memory-ahead-of-disk defect, and the one with no way back.
func TestAFailedSaveRestoresASlotsWrappedBytes(t *testing.T) {
	v := newVault(t)

	v.mu.Lock()
	if len(v.ssh) == 0 {
		v.mu.Unlock()
		t.Fatal("setup: the vault holds no key slots, so there is nothing to snapshot")
	}
	original := append([]byte(nil), v.ssh[0].Wrapped...)
	v.mu.Unlock()
	if len(original) < 4 {
		t.Fatalf("setup: slot 0 carries %d wrapped byte(s) — the assertion below would compare "+
			"nothing", len(original))
	}

	done := withFailingWrite(t)
	v.mu.Lock()
	// An in-place rewrap: the bytes change, the slot header does not. A snapshot that copies
	// only the Slot structs shares this array and hands the mutation's own bytes back as the
	// "restored" ones.
	err := v.mutateLocked(func() {
		for i := range v.ssh[0].Wrapped {
			v.ssh[0].Wrapped[i] ^= 0xff
		}
	})
	after := append([]byte(nil), v.ssh[0].Wrapped...)
	v.mu.Unlock()

	if writes := done(); writes != 1 {
		t.Fatalf("setup: the failing write ran %d time(s), want 1 — a rollback assertion over a "+
			"save that never ran asserts nothing", writes)
	}
	if err == nil {
		t.Fatal("the door reported success although its save failed")
	}
	if !bytes.Equal(after, original) {
		t.Errorf("after a failed save the slot's wrapped key is %x…, want the stored %x… — the "+
			"door's snapshot shares the Wrapped array with the mutation, so the rollback handed "+
			"back the bytes it was supposed to undo. This Nib now believes the vault is sealed "+
			"to a key the file was never sealed to, and Keys() cannot show it: that accessor "+
			"omits Wrapped.", after[:4], original[:4])
	}
}
