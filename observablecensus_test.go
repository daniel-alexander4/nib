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
				if readerMentions(s, r, f, sh.tag[f]) {
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

// inPackageOnlyRecorded is the population above, as it stands: **25 of 658 published fields**. It was
// 47 of 511 when `observables_test.go`'s header refused the field-level rule, and 43 when the
// verdicts below were taken.
//
// **Every entry now carries its verdict** (/pending 575). The pass read each field's uses resolved by
// TYPE (go/types over every non-test package, so `.State` of another struct is not a use of this
// one) and then the JSON tag in the window and the CLI. Of the 36 that were undecided:
//
//   - **17 had a real reader the table never named**, and left this list when `published` named the
//     one file that reads each: `ceremony.Stored` (7) and `ceremony.Receipt` (3) are drawn by the
//     window, `ceremony.Record`'s `DocHash`/`Intent`/`Roster`/`Version` and `ceremony.Invitation`'s
//     `ConvenerFingerprint`/`Secret`/`Seeds` are read by the server and the CLI.
//   - **18 stay, deliberately in-package**, each for one of three reasons written against it: the
//     field is VERIFIED where it is defined (a signature, a format number, a commitment — a reader
//     outside would re-implement the check); it is WRITTEN outside and read inside (a constructor's
//     input); or it is working state behind an accessor.
//   - **1 was read by nothing at all**, `vault.ExternalSigner.ChainPEM`, and is gone (/pending 863): the
//     field is removed, and a vault that still holds its key opens as before.
//
// **Seven are answered at SHAPE level.** `ceremony.Anchor` and `p2p.Channel` are in
// `internalShapes` with reasons — an opaque token and an inward parameter carrier — so their fields
// are in this list because the per-field question is narrower than the rule.
var inPackageOnlyRecorded = []string{
	// ceremony.Anchor  // shape declared internal, with a reason
	"ceremony.Anchor.Ceremony", // /pending 686: read by VerifyAgainst, the one door
	"ceremony.Anchor.Convener",
	"ceremony.Anchor.RosterHash",
	// ceremony.CandidateRecord — a signed DHT record. The server and `nib rendezvous` BUILD one
	// (`ceremonynet.go:191-194`, `cli/rendezvous.go:603-616`, keyed literals) and hand it to this
	// package, which signs it and, on the way back, verifies it; nothing outside reads a field back.
	"ceremony.CandidateRecord.Addrs",      // written outside, read in candidate.go
	"ceremony.CandidateRecord.CeremonyID", // written outside, read in candidate.go
	"ceremony.CandidateRecord.SPKI",       // verified where defined: candidate.go:227,258
	// `Sig` joined at /pending 808 R5: its out-of-package "reader" was `.Sig` as a PREFIX of `.Signature`.
	"ceremony.CandidateRecord.Sig",     // verified where defined: candidate.go:352,404
	"ceremony.CandidateRecord.Version", // verified where defined: candidate.go:276,335
	// ceremony.Invitation
	// `SeedsDropped` is `json:"-"` and its own doc says it: "Its only reader today is `seeds_test.go`",
	// kept because the acceptance clause wants the count observable. A declared diagnostic.
	"ceremony.Invitation.SeedsDropped",
	// Written by the two constructors (`delivery.go:1543`, `cli/rendezvous.go:532`), checked by the parse.
	"ceremony.Invitation.Version",
	// ceremony.Record — the convener's signature material and the digest's format number: verified
	// by `Record.Verify` and the embed reader (`record.go:286,512-519`, `embed.go:129,295,301`).
	"ceremony.Record.ConvenerCert",
	"ceremony.Record.ConvenerSig",
	"ceremony.Record.DigestVersion",
	// ceremony.Termination — compared with an `Anchor`'s, in-package (`termination.go:242`,
	// `mirror.go:893`): `ceremony.Anchor`'s reason in `internalShapes`, from the other side.
	"ceremony.Termination.RosterHash",
	// discovery.Announcement — written by the three announcers (`server/discover.go:545`,
	// `server/lan.go:177`, `cli/discover.go:150`), read by the package's own encoding and its
	// own-echo check (`announce.go:266`, `mcast.go:421`).
	"discovery.Announcement.Nonce",
	// instance.Record — at /pending 808 R5 the second launch (`cmd/nib/main.go`, `rec.Addr`) was named
	// as the reader, taking Addr OFF this list; Token and Handoff came ON, because the only outside
	// "reader" they had was `.Token` as a prefix of `.TokenMatches`. The second launch hands the record
	// to Probe and HandOff, which read them in-package — true, and what this list records. Token left it again
	// at /pending 630 (v1.182.18): the launch's exit removal passes its own `rec.Token` to `instance.Remove`.
	// Challenge joined at /pending 827: the launch WRITES it (`Challenge: true`) and hands the record to
	// Probe, which is its one reader and is in-package. All three: written at `cmd/nib/main.go:132`,
	// read inside.
	"instance.Record.Challenge",
	"instance.Record.Handoff",
	"instance.Record.Version",
	// p2p.Channel  // shape declared internal, with a reason
	"p2p.Channel.Export",
	"p2p.Channel.PeerFP",
	"p2p.Channel.Proto",
	"p2p.Channel.Stream",
	// rendezvous.SelfAddress — the evidence `V4` and `V6` are classified from (`selfaddr.go:162-171`);
	// the CLI prints the two classes and never the observations.
	"rendezvous.SelfAddress.Observations",
	// vault.PinnedPeer — the pin's scope set, read and written only through the vault's own scope
	// doors (`vault.go:1289-1365`); the peers route never shows it.
	"vault.PinnedPeer.Ceremonies",
	// vault.Slot — the wrapped content key: unwrapped in `vault.go:628` and not to be read elsewhere.
	"vault.Slot.Wrapped",
}
