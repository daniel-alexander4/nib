package ceremony

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestEveryInvitationFieldWithARecordCounterpartIsCompared — P07.S02b, and it replaces a plan
// bullet rather than implementing it.
//
// The bullet said `MatchesRecord` should compare `intent`, `expires`, `capacity` and `label`.
// Label and capacity have been compared since v1.117.153 — `matchesRosterFields` compares the
// whole `Party` struct with `!=`, which is every field there is.
//
// **Intent joined them at v1.117.218 (P07.S07b), and the paragraph that stood here argued it
// should not.** It said intent was "not carried by the invitation at all, so there is nothing to
// compare, and adding the field would create the exposure rather than close it: `RosterHash` is
// the record's own digest copied into an UNSIGNED invitation, so an attacker editing `i.Intent`
// would leave the hash matching." Every clause of that is true of the field ALONE and it is why
// the field could not be added alone. What changed is not the risk assessment but the reason to
// take it: `internal/p2p` applies the signature, cannot import `internal/ceremony`, and reads its
// roster from the invitation — so C15's "every block carries the record's recital verbatim" is
// reachable only if the invitation carries the recital. The exposure that paragraph names is then
// closed by the comparison this test drives, which is the same trade every other carried field
// already makes. `Expires` is still uncarried and is still `/pending 247`'s.
//
// So the bullet is replaced by the rule that generalises it, which is the same trade `RosterHash`
// itself made over a per-field list: **every field of `Invitation` is either driven here, or
// carries a written reason for having no counterpart.** A field added to the struct and to
// neither list fails this test by name. A per-field list is what left `Label` uncompared for
// three phases; `/pending 247`'s `Expires` is the next field this will catch.
func TestEveryInvitationFieldWithARecordCounterpartIsCompared(t *testing.T) {
	// driven: mutating this field in the invitation must make MatchesRecord refuse.
	driven := map[string]func(*Invitation){
		"ID": func(i *Invitation) { i.ID = strings.Repeat("f", len(i.ID)) },
		"Roster": func(i *Invitation) {
			i.Roster = append([]Party(nil), i.Roster...)
			i.Roster[1].Capacity = "as Guarantor"
		},
		"Intent":              func(i *Invitation) { i.Intent = i.Intent + " (and one more thing)" },
		"RosterHash":          func(i *Invitation) { i.RosterHash = strings.Repeat("0", len(i.RosterHash)) },
		"ConvenerFingerprint": func(i *Invitation) { i.ConvenerFingerprint = i.Roster[1].Fingerprint },
	}
	// excluded: no counterpart in the record, so no comparison is possible. The reason is the
	// point — an entry here is a decision, and a field parked here without one is not.
	excluded := map[string]string{
		"Version": "the format version of the invitation, not of the ceremony. ParseInvitation " +
			"refuses a version this build does not know before MatchesRecord is reachable, and " +
			"Record.Version is a different number about a different artifact.",
		"Secret": "the channel key. It is never published and a Record has no counterpart to it " +
			"by construction — the whole point of D29 is that it does not go anywhere the " +
			"record goes.",
		"Seeds": "bootstrap addresses. Invitation.Seeds' own doc states this: it is the one " +
			"field that will never have a signed counterpart, so tampering with it can never " +
			"be made loud, and it is treated as attacker-controllable input everywhere instead.",
		"SeedsDropped": "never serialized — an observation about the invitation just parsed, " +
			"not a field of it.",
	}

	// Completeness, over the type rather than over a list somebody maintains.
	for i := 0; i < reflect.TypeOf(Invitation{}).NumField(); i++ {
		name := reflect.TypeOf(Invitation{}).Field(i).Name
		_, d := driven[name]
		_, e := excluded[name]
		if d == e { // in both, or in neither
			t.Errorf("Invitation.%s is in %d of the two lists, want exactly 1. Either mutating "+
				"it must make MatchesRecord refuse (add it to `driven`), or it has no record "+
				"counterpart and needs a written reason (add it to `excluded`). A field in "+
				"neither is a field the invitation can carry and nothing compares.",
				name, map[bool]int{true: 1, false: 0}[d]+map[bool]int{true: 1, false: 0}[e])
		}
	}
	for name, why := range excluded {
		if strings.TrimSpace(why) == "" {
			t.Errorf("Invitation.%s is excluded with no reason written", name)
		}
	}

	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	_, _, bfp := identity(t, "B")
	rec := draft(t, cfp, afp, bfp)
	rec.Roster[1].Capacity = "as Director" // so the Roster mutation below has something to move
	if err := rec.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	invites, err := NewInvitations(rec)
	if err != nil {
		t.Fatal(err)
	}
	base, ok := invites[afp]
	if !ok {
		t.Fatal("setup: no invitation was issued for party A")
	}
	// Stimulus, before anything is graded: the UNMUTATED pair matches. Without this every row
	// below could be passing because MatchesRecord refuses everything.
	if err := base.MatchesRecord(rec); err != nil {
		t.Fatalf("setup: an honest invitation does not match its own record (%v) — every "+
			"refusal below would then prove nothing", err)
	}

	for name, mutate := range driven {
		t.Run(name, func(t *testing.T) {
			bad := base
			mutate(&bad)
			if reflect.DeepEqual(bad, base) {
				t.Fatalf("the mutation for %s changed nothing, so the refusal below would be "+
					"about an unmutated invitation", name)
			}
			err := bad.MatchesRecord(rec)
			if err == nil {
				t.Fatalf("Invitation.%s was changed and MatchesRecord accepted it. The party "+
					"reads that field and the record carries its own copy; nothing compares "+
					"them, so the two can disagree with every check green.", name)
			}
			if !errors.Is(err, ErrRosterMismatch) {
				t.Errorf("Invitation.%s refused with %v, want an ErrRosterMismatch — a caller "+
					"distinguishing a tampered invitation from a broken one cannot", name, err)
			}
		})
	}
}
