package nib

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestTheInPackageOnlyFieldsAreTheOnesRecorded — /pending 568.
//
// # What this is, and what it deliberately is NOT
//
// `observables_test.go`'s outside-the-package arm is SHAPE-level, by a measured decision its own
// header states at length: the per-FIELD version reports **47 of 511 fields**, of which **37 are
// false orphans** — fields with real out-of-package consumers the reader table simply never named,
// like `ceremony.Party.Fingerprint`, read by seventeen files in `internal/server`. Making the rule
// per-field would force the table to name those large files, and the scan's own declared
// coincidental-match limit means a large file satisfies almost any field name: a stricter-LOOKING
// rule buying a looser table.
//
// That header records the 47 as a NUMBER and says the verdicts are *"left to a pass that reads the
// call sites"*. This is not that rule. **It is the population itself**, pinned — because until now
// the 47 existed only as three digits in a comment, with no way to see which fields they were or to
// tell whether the set had moved.
//
// # Why a census and not a rule
//
// A rule would have to decide each field, and the thing that made the field-level rule unsound —
// a bare-name match satisfied by any common word — makes an automatic verdict unsound too. A census
// asserts something a rule cannot: that the SET has not changed without anyone noticing. A field
// joining it is a shape whose readers all sit inside its own package, which is worth a look; a
// field leaving it is a reader that appeared. Either way the diff is the output.
//
// # Both automatic filters are unsound, which is why the verdicts are recorded rather than computed
//
// Established, not suspected, and re-verified for this census: a Go IMPORT filter over-rejects,
// because imports are per FILE — `internal/server/extsigner.go:33` reads `vault.ExternalSigner.CertPEM`
// and that file's import block (lines 3-11) does not name `nib/internal/vault` at all, though the
// package imports it elsewhere. And a bare-name match against `web/app.js` over-accepts, because
// `name`, `state` and `reason` appear in a twenty-thousand-line client for a hundred reasons.
//
// # Reading a failure
//
// The message prints the set and the difference. **Do not silence it by editing the list** unless
// the field really has moved: a field that gained an out-of-package reader should be removed from
// the list in the commit that gave it one, and a new arrival should be looked at before it is added.
func TestTheInPackageOnlyFieldsAreTheOnesRecorded(t *testing.T) {
	shapes := discoverObservables(t)
	got := inPackageOnlyFields(t, shapes)

	want := map[string]bool{}
	for _, f := range inPackageOnlyRecorded {
		want[f] = true
	}

	// The stimulus floor. A scan that discovered nothing would report an empty set, which is
	// indistinguishable from a tree in which every field has an out-of-package reader — and the
	// second is not a state this repo has ever been in.
	if len(shapes) < 20 {
		t.Fatalf("only %d observables were discovered, so this census is grading almost nothing",
			len(shapes))
	}

	var added, gone []string
	for _, f := range got {
		if !want[f] {
			added = append(added, f)
		}
	}
	seen := map[string]bool{}
	for _, f := range got {
		seen[f] = true
	}
	for f := range want {
		if !seen[f] {
			gone = append(gone, f)
		}
	}
	sort.Strings(added)
	sort.Strings(gone)

	if len(added) > 0 {
		t.Errorf("%d field(s) are now satisfied ONLY by readers inside their own declaring "+
			"package, and are not in the recorded set. A shape's own package always mentions its "+
			"own fields, so this is a field the reader table reports as covered while nothing "+
			"outside the package reads it. Look at each before adding it:\n  %s",
			len(added), strings.Join(added, "\n  "))
	}
	if len(gone) > 0 {
		t.Errorf("%d recorded field(s) now HAVE an out-of-package reader — delete them from "+
			"`inPackageOnlyRecorded`, or the list stops describing anything and silently re-admits "+
			"each the day its reader is removed:\n  %s", len(gone), strings.Join(gone, "\n  "))
	}
	t.Logf("%d of %d published fields are satisfied only in-package", len(got), countFields(shapes))
}

// countFields is the denominator the header's "47 of 511" quotes.
func countFields(shapes map[string]observable) int {
	n := 0
	for _, sh := range shapes {
		n += len(sh.fields)
	}
	return n
}

// inPackageOnlyFields is the per-FIELD question the shape-level arm refuses to ASK AS A RULE, asked
// here as a measurement: which fields are matched by no reader outside the shape's own package.
//
// **It reuses the main scan's matcher exactly** — `.Field`, or `.jsonTag`, or a bare-word tag match
// for a `.js`/`.sh` reader — because a census run against a different matcher would be measuring a
// different thing from the rule it sits beside, and the difference would look like a finding.
func inPackageOnlyFields(t *testing.T, shapes map[string]observable) []string {
	t.Helper()
	cache := map[string]string{}
	src := func(p string) string {
		if s, ok := cache[p]; ok {
			return s
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("reader %s named by the table does not exist: %v", p, err)
		}
		s := codeOnly(string(b))
		cache[p] = s
		return s
	}

	var out []string
	for _, name := range sortedKeys(shapes) {
		sh := shapes[name]
		if _, skip := excluded[name]; skip {
			continue
		}
		readers, ok := published[name]
		if !ok && strings.HasPrefix(name, "server.") {
			readers, ok = jsonShapeReaders, true
		}
		if !ok {
			continue // untabled: the main scan reports it, and it is not this census's finding
		}
		pkgDir := filepath.Dir(sh.file)
		for _, f := range sh.fields {
			key := name + "." + f
			if _, parked := unreadKnown[key]; parked {
				continue // a park is consulted before the reader loop; this census follows it
			}
			outside := false
			for _, r := range readers {
				if filepath.Dir(r) == pkgDir {
					continue
				}
				s := src(r)
				if strings.Contains(s, "."+f) ||
					(sh.tag[f] != "" && strings.Contains(s, "."+sh.tag[f])) ||
					(sh.tag[f] != "" && (strings.HasSuffix(r, ".js") || strings.HasSuffix(r, ".sh")) &&
						mentionsWord(s, sh.tag[f])) {
					outside = true
					break
				}
			}
			if !outside {
				out = append(out, key)
			}
		}
	}
	sort.Strings(out)
	return out
}

// inPackageOnlyRecorded is the population above, as it stands: **40 of 544 published fields**, close
// to the `47 of 511` `observables_test.go`'s header measured when it refused the field-level rule.
// It is a MEASUREMENT written down, not a list anybody chose.
//
// **The verdicts are NOT here, and that is the honest state rather than an omission.** The item
// this closes asked for the population to be made visible first — *"print the 47 with their
// declaring package and current readers, then take them in one pass"* — and this is the first
// half. Reading 34 call sites to say which are genuinely unread is the second, and it is filed
// rather than guessed: an automatic verdict is exactly what both unsound filters would give.
//
// **Six of the forty are already answered at SHAPE level.** `ceremony.Anchor` and `p2p.Channel`
// are in `internalShapes` with reasons — an opaque token and an inward parameter carrier — so
// their fields are in this list because the per-field question is narrower than the rule, not
// because anything is undecided about them.
var inPackageOnlyRecorded = []string{
	// ceremony.Anchor  // shape declared internal, with a reason
	"ceremony.Anchor.Convener",
	"ceremony.Anchor.RosterHash",
	// ceremony.CandidateRecord
	"ceremony.CandidateRecord.Addrs",
	"ceremony.CandidateRecord.CeremonyID",
	"ceremony.CandidateRecord.SPKI",
	"ceremony.CandidateRecord.Version",
	// ceremony.Invitation
	"ceremony.Invitation.ConvenerFingerprint",
	"ceremony.Invitation.Secret",
	"ceremony.Invitation.Seeds",
	"ceremony.Invitation.SeedsDropped",
	"ceremony.Invitation.Version",
	// ceremony.Receipt
	"ceremony.Receipt.Name",
	"ceremony.Receipt.ObservedAt",
	"ceremony.Receipt.State",
	// ceremony.Record
	"ceremony.Record.ConvenerCert",
	"ceremony.Record.ConvenerSig",
	"ceremony.Record.DigestVersion",
	"ceremony.Record.DocHash",
	"ceremony.Record.Intent",
	"ceremony.Record.Roster",
	"ceremony.Record.Version",
	// ceremony.Stored
	"ceremony.Stored.Ended",
	"ceremony.Stored.Joined",
	"ceremony.Stored.Me",
	"ceremony.Stored.Name",
	"ceremony.Stored.Reason",
	"ceremony.Stored.State",
	"ceremony.Stored.Verification",
	// ceremony.Termination
	"ceremony.Termination.RosterHash",
	// discovery.Announcement
	"discovery.Announcement.Nonce",
	// instance.Record
	"instance.Record.Addr",
	"instance.Record.Version",
	// p2p.Channel  // shape declared internal, with a reason
	"p2p.Channel.Export",
	"p2p.Channel.PeerFP",
	"p2p.Channel.Proto",
	"p2p.Channel.Stream",
	// rendezvous.SelfAddress
	"rendezvous.SelfAddress.Observations",
	// vault.ExternalSigner
	"vault.ExternalSigner.ChainPEM",
	// vault.PinnedPeer
	"vault.PinnedPeer.Ceremonies",
	// vault.Slot
	"vault.Slot.Wrapped",
}
