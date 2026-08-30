package ceremony

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"nib/internal/atomicfile"
	"nib/internal/sign"
)

// The on-disk mirror: ~/nib/ceremonies/<id>/ (D20, D29).
//
// A ceremony can outlive a sitting — a party is interrupted, closes Nib, and comes back —
// so the record and the document in flight are written beside each other under the
// ceremony's own id. The id is what makes "continue" name a ceremony rather than guess at
// one.
//
// **What must never be written here is the invitation secret** (D29). The secret lives in
// the vault, which is sealed to the user's SSH key; this directory is ordinary files under
// the user's home. A test that asserts the vault *contains* the secret cannot see a copy
// left on disk beside the document, which is why the check that matters is an absence
// check over this directory.

// idPattern is what a ceremony id must look like before it is allowed near a path.
//
// The id comes out of a record, and a record can arrive from another party — so it is
// attacker-controlled input being used to build a directory name. 32 hex characters and
// nothing else: no separators, no dots, no room for traversal. Validated rather than
// escaped, because a validator has one answer and an escaper has a list of cases.
var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ErrBadID is returned for an id that cannot safely name a directory.
var ErrBadID = errors.New("ceremony id is not 32 hex characters")

// ValidID is the ONE door for the rule above (ADR-009), and it is exported because the rule has
// callers outside this package — /pending 308.
//
// `idPattern` was unexported and reached from `MirrorDir` and `ListStored` alone, so every site
// that builds a path from a ceremony id had to be inside `internal/ceremony` to be safe. P08.S05
// puts the id into a delivered filename under `~/nib/`, written from `internal/server`, which
// could not reach the predicate at all.
//
// **What this does NOT claim.** The traversal exposure was already closed: `MirrorDir` refuses
// before `filepath.Join` is ever reached, at all five path sites, and `TestTheMirrorRefusesAnUnsafeID`
// drives it. The reason the rule also belongs in `Record.Verify` is ORDERING, not path safety —
// see the call there.
func ValidID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrBadID, id)
	}
	return nil
}

// ErrMirrorDamaged reports a stored ceremony whose document does not match its own record.
//
// Its own sentinel because the alternative sentence is an accusation: without it the
// mismatch surfaces at CheckDocument as "these are not the same document", which describes a
// counterparty substituting a file. This one says the copy on THIS machine is damaged, which
// is what actually happened and what the user can act on.
var ErrMirrorDamaged = errors.New("this ceremony's stored document does not match its record")

// MirrorDir returns the directory for a ceremony, under root (normally ~/nib).
func MirrorDir(root, id string) (string, error) {
	if err := ValidID(id); err != nil {
		return "", err
	}
	return filepath.Join(root, "ceremonies", id), nil
}

// WriteMirror creates the ceremony's directory and writes the record and the document.
//
// The record is written as JSON beside the document rather than only inside it: a party
// resuming needs to know which ceremony a file belongs to before opening the PDF, and
// reading a PDF attachment to find out is a slower answer to a question the directory
// name already half-answers.
func WriteMirror(root string, r Record, pdf []byte) (string, error) {
	dir, err := MirrorDir(root, r.ID)
	if err != nil {
		return "", err
	}
	// **A different proceeding may not overwrite this one (/pending 318).**
	//
	// `ValidID` constrains the id's SHAPE and nothing constrains its VALUE: `NewID`'s 128 random
	// bits are a convention, and `Record.ID` is a plain signed field a convener running its own
	// binary sets freely. So a convener who shares a roster with the victim can mint ceremony Y
	// with `Y.ID = X.ID` — 32 valid hex, passing every check /pending 308 added — and the victim's
	// hop-time write then overwrites `~/nib/ceremonies/<X.ID>/`. For a non-convener that directory
	// is the SOLE durable copy of the document carrying their own signature (P08's C11 pin), so it
	// destroys a signature they already made, on a ceremony whose convener is somebody else.
	//
	// `RosterHash` is the discriminator because it is the only candidate that is all three of:
	// derivable from `record.json` alone, exactly what `ConvenerSig` signs, and covering every
	// axis — version, convener, id, DocHash, intent, expires and every roster entry. `ConvenerCert`
	// is shared by two ceremonies of one convener; `DocHash` and `Expires` are single axes inside
	// RosterHash that two proceedings can share. `MatchesRecord` already owns the sentence for
	// this comparison, and `TestOneInvitationMatchesExactlyOneRecord` already refuses a convener
	// running two chains under one id — this is the same rule at the storage door.
	//
	// **Placed before MkdirAll and before any write.** Lower down it would return an error with
	// `document.pdf` already clobbered and, since v1.117.271, the sidecar already unlinked.
	if err := refuseDifferentProceeding(dir, r); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	b, err := r.Encode()
	if err != nil {
		return "", err
	}
	// **The DOCUMENT first, then the record — the order is the commit point (P07.S02a).**
	//
	// C22 says the mirror is written "before the HTTP response returns", which is a
	// durability claim. Two bare os.WriteFile calls do not make it: they return before the
	// bytes reach the disk, so on the crash C22 exists for, a write that REPORTED success can
	// be absent or truncated.
	//
	// Worse was the ordering. With record.json first, a crash between the two left
	// (record, nil, nil) from ReadMirror — indistinguishable from a deliberately
	// document-less mirror, which WriteMirror itself creates when pdf is nil. A resuming
	// party could not tell "no document yet" from "the document was lost". Writing the
	// document first makes the record's presence mean "both are here": the record is the last
	// thing to land, so a torn write leaves a directory with no record, which ReadMirror
	// reports as an ordinary miss.
	if pdf != nil {
		// **The sidecar is UNLINKED BEFORE the document is written (/pending 321).**
		//
		// The document-then-record ordering below is a FIRST-WRITE argument — P08.S01's own scope
		// said so in advance: *"WriteMirror's document-then-record ordering is a first-write
		// argument that does not survive a third file"*. S02 then added the third file and the
		// warning did not travel. At hop 2 or later `record.json` and `document.sha256` both
		// already exist, so a crash between the document and the sidecar leaves a COMPLETE, VALID
		// document beside the PREVIOUS hop's checksum — and `ReadMirror`'s unconditional sidecar
		// check then reports `ErrMirrorDamaged` permanently, which is a false accusation against
		// the user's own disk with no repair path.
		//
		// Removing it first makes the torn state "no sidecar", which `ReadMirror` already tolerates
		// by design and says so at the line: mirrors written before S02 have none, and it is a
		// damage detector rather than an access control. So the window costs the detector for one
		// document and never the document.
		//
		// Best-effort: a sidecar that is already absent is the ordinary case at hop 1, and a remove
		// that fails leaves the old behaviour rather than blocking the write.
		_ = os.Remove(filepath.Join(dir, "document.sha256"))
		if err := atomicfile.WriteDurable(filepath.Join(dir, "document.pdf"), pdf, 0o600); err != nil {
			return "", err
		}
	}
	// 0600 on both: these are the documents of a signing in progress, and the directory is
	// under the user's home where the default umask would otherwise make them world-readable
	// on a shared machine.
	//
	// **Windows note, stated rather than assumed:** Go's os maps perm only to the read-only
	// attribute there and inherits ACLs, so this mode is a no-op on the platform build.sh
	// ships for. The vault survives that because it is AES-GCM sealed; the mirror has no such
	// fallback, and D29's reasoning for keeping the invitation SECRET out of this directory
	// is what carries the weight instead.
	// **The sidecar, and it is the check `DocHash` cannot be (P08.S02, C02).**
	//
	// `ReadMirror`'s hash comparison is gated on the document being UNSIGNED, and that limit is
	// correct and load-bearing: `DocHash` is a convene-time identity, a visible signature adds a
	// widget annot, and `ContentDigest` covers `/Annots` — so from the first signature the document
	// legitimately stops hashing to it. The consequence, stated at that gate, is that from hop 2
	// onward the mirror is stored **with no self-check at all**.
	//
	// That is exactly the window C02 is about. Truncating a signed document at a PRIOR `%%EOF`
	// yields a well-formed PDF one revision short: it parses, `sign.Verify` reports Valid with
	// fewer signers, and nothing notices. A byte hash of the file as written catches it, and a byte
	// hash is right here for the reason it is wrong for `DocHash` — this is a local self-check over
	// one byte stream, not an identity a remote party must recompute through pdfcpu.
	//
	// Written BETWEEN the document and the record, so `record.json` is still the commit point.
	if pdf != nil {
		sum := sha256.Sum256(pdf)
		if err := atomicfile.WriteDurable(filepath.Join(dir, "document.sha256"),
			[]byte(hex.EncodeToString(sum[:])), 0o600); err != nil {
			return "", err
		}
	}
	if err := atomicfile.WriteDurable(filepath.Join(dir, "record.json"), b, 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

// ReadMirror loads a ceremony back off disk.
func ReadMirror(root, id string, now time.Time) (Record, []byte, error) {
	dir, err := MirrorDir(root, id)
	if err != nil {
		return Record{}, nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		return Record{}, nil, err
	}
	r, err := Decode(b)
	if err != nil {
		return Record{}, nil, err
	}
	// **Verified, because this file is attacker-controlled input and the record read out of
	// it is a root of trust.** MirrorDir's own comment already says the id "comes out of a
	// record, and a record can arrive from another party"; the same is true of the record
	// itself once anything can write here — a co-tenant, a synced home, malware, and from
	// P07.S05 a record that arrived over the wire. The convene route re-mints invitations
	// from this record's roster and commitment, so an unverified one lets someone else choose
	// what the convener hands out.
	//
	// Every other production reader of a Record verifies before acting on it
	// (checkCeremonyDeadline, CheckDocument). This was the site where that rule was not
	// applied — one rule, one of two places. `now` is threaded rather than read here for the
	// reason the rest of this package threads it: a clock read inside a verdict is
	// nondeterminism reaching a decision.
	if err := r.Verify(now); err != nil {
		return Record{}, nil, fmt.Errorf("this ceremony's stored record does not verify: %w", err)
	}
	pdf, err := os.ReadFile(filepath.Join(dir, "document.pdf"))
	if err != nil && !os.IsNotExist(err) {
		return r, nil, err
	}
	// **The bytes are checked against the record that names them (P07.S02a).**
	//
	// Before this, a truncated or half-written document.pdf came back as (record, bytes, nil)
	// — success — and nothing compared them, though the record carries DocHash precisely so
	// that a later party can. A resuming party would then fail at CheckDocument with "these
	// are not the same document", which is an accusation against a counterparty about damage
	// that happened on this machine's own disk.
	//
	// Reported as a distinct error rather than by returning the bytes: a caller that gets
	// (record, damaged-bytes, nil) has no way to tell.
	// **Only while the document is UNSIGNED, and that limit is the point.**
	//
	// DocHash is a convene-time identity. Measured at P07.S02: ContentDigest covers each
	// page's /Annots, a VISIBLE signature adds a widget annot, and the production path signs
	// visibly — so from the first signature onward the document in flight legitimately does
	// NOT hash to what the record says. An unconditional check here would refuse every
	// mirror from hop 2 on, which is the resumption case the mirror exists for.
	//
	// So this catches the case it can: a convened document that was torn or truncated on the
	// way to disk, before anybody signed. Past that point the mirror is stored without a
	// self-check, and saying so is better than implying a coverage it has not got — the
	// per-hop continuity that would replace it is S05's carry route.
	// **The sidecar check runs UNCONDITIONALLY, signed or not (P08.S02, C02).** It is the half the
	// `DocHash` comparison below cannot cover, and it covers the case that matters: a mirror
	// written at hop 2 or later, which is every mirror a resumption actually reads.
	//
	// **A missing sidecar is tolerated**, and that is deliberate rather than a hole. Mirrors written
	// before this slice have none, and refusing them would strand ceremonies that are in flight the
	// day it ships. It is a damage detector and not an access control: anyone who can truncate the
	// document can delete the sidecar, and `ReadMirror`'s own distinction — damage on this machine
	// versus an accusation about a counterparty — is what it serves.
	if len(pdf) > 0 {
		if want, rerr := os.ReadFile(filepath.Join(dir, "document.sha256")); rerr == nil {
			got := sha256.Sum256(pdf)
			if hex.EncodeToString(got[:]) != strings.TrimSpace(string(want)) {
				return r, nil, fmt.Errorf("%w: its stored bytes do not match the checksum written "+
					"beside them, so the copy on this machine was damaged or truncated after it "+
					"was written", ErrMirrorDamaged)
			}
		}
	}
	if len(pdf) > 0 && sign.Verify(pdf).State == sign.Unsigned {
		got, herr := DocumentHash(pdf)
		if herr != nil {
			return r, nil, fmt.Errorf("%w: its document will not parse: %v", ErrMirrorDamaged, herr)
		}
		if got != r.DocHash {
			return r, nil, fmt.Errorf("%w: the stored document hashes to %s and this ceremony's "+
				"record was written for %s, so the copy on this machine is damaged or incomplete",
				ErrMirrorDamaged, short(got), short(r.DocHash))
		}
	}
	return r, pdf, nil
}

// RemoveMirror deletes a ceremony's directory.
//
// **It is NOT the close-out prune, though it was written to be** — corrected 2026-08-29 by the
// P08.S01 deepdive, which found the claim describing a caller that does not exist. Its only
// production caller is `unconvene`, the convene ROLLBACK (`internal/server/convene.go:272`), so
// today a ceremony that completes, declines, expires or is abandoned keeps its directory forever.
// D29's close-out prune is P08.S06's, and this function is what it will call.
//
// `os.RemoveAll`, so it takes whatever else is in the directory with it — including any leftover
// `.nib-*.tmp` from an interrupted `atomicfile` write, which nothing else sweeps.
func RemoveMirror(root, id string) error {
	dir, err := MirrorDir(root, id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// The listing (P08.S03) — what a resumed Nib can say about the ceremonies on this machine.

// LoadState classifies what happened when one stored ceremony was read.
//
// **Four outcomes, not two, and the reason is that three of them brick a ceremony while looking
// alike.** `ReadMirror` collapses every failure into one error and its only production caller then
// collapses that into one 404, so a Nib update mid-ceremony — which moves `FormatVersion` and makes
// `Verify` refuse — is indistinguishable from a forged record and from a directory the user deleted.
// Each of those wants a different sentence and a different remedy, and one of them (skew) must stay
// PRUNABLE, which a verdict of "does not verify" would forbid.
type LoadState string

const (
	// LoadOK: the record read and verified.
	LoadOK LoadState = "ok"
	// LoadAbsent: a directory with no record.json. Ordinary — `WriteMirror` writes the record
	// LAST, so this is also what a torn write leaves, and both mean "nothing usable here yet".
	LoadAbsent LoadState = "absent"
	// LoadUnparseable: record.json exists and is not a record. Local damage.
	LoadUnparseable LoadState = "unparseable"
	// LoadVersionSkew: written by a Nib whose record format this build does not know. **Not
	// damage and not forgery** — the vault says the equivalent in plain language rather than
	// accusing anyone (`checkContentsVersion`), and this is that sentence for the mirror.
	LoadVersionSkew LoadState = "version-skew"
	// LoadUnverifiable: it parsed, and its convener signature, canonical form, roster bound or
	// deadline ceiling did not hold. This is the one that IS an accusation, and it is the only
	// one of the four that should read as one.
	LoadUnverifiable LoadState = "unverifiable"
)

// Stored is one ceremony as the listing reports it.
//
// **The document is never opened to build this.** Measured at the P08.S01 deepdive: `ReadMirror`
// costs 10 ms at 100 pages, 69 ms at 500 and 195 ms at 1000 — superlinear, on text-only fixtures —
// because it runs `sign.Verify` and, while unsigned, a full `ContentDigest`. At fifty stored
// ceremonies that is seconds on a request path. So the listing answers from `record.json` alone.
//
// **Which is why it carries no signature count and no "next action".** Those need the document, and
// the honest split is that the list says which ceremonies exist and the detail comes when one is
// opened. D24's *"2 of 4 signed — waiting for Amir"* is a panel line about ONE ceremony, and the
// panel can afford to open one.
type Stored struct {
	ID    string    `json:"id"`
	State LoadState `json:"state"`
	// Reason is the sentence for a non-OK state, already written for a person.
	Reason string `json:"reason,omitempty"`
	// The rest are populated only for LoadOK.
	Intent  string    `json:"intent,omitempty"`
	Expires time.Time `json:"expires,omitempty"`
	Roster  []Party   `json:"roster,omitempty"`
}

// ReadStored loads one ceremony's record and classifies the outcome. It does NOT read the document.
func ReadStored(root, id string, now time.Time) Stored {
	s := Stored{ID: id, State: LoadOK}
	dir, err := MirrorDir(root, id)
	if err != nil {
		s.State, s.Reason = LoadUnparseable, "that is not a ceremony id"
		return s
	}
	b, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		// **An unreadable record is NOT an absent one (/pending 320).** Every read error used to
		// land on LoadAbsent, so a permission problem or an I/O fault was reported to the user as
		// "its folder may have been removed" — advice that sends them looking for something that
		// is right there. The two want opposite remedies, and the sentence for absence is the one
		// that reads as reassuring.
		if os.IsNotExist(err) {
			s.State = LoadAbsent
			s.Reason = "this ceremony has no record on this machine — its folder may have been " +
				"removed, or it was interrupted before anything was written"
			return s
		}
		s.State = LoadUnparseable
		s.Reason = "this ceremony's record is on this machine and could not be read: " + err.Error()
		return s
	}
	r, err := Decode(b)
	if err != nil {
		s.State = LoadUnparseable
		s.Reason = "this ceremony's record is on this machine but cannot be read — the file is " +
			"damaged"
		return s
	}
	if verr := r.Verify(now); verr != nil {
		// **Skew first, and it is the ordering that matters.** `Verify` checks the version before
		// anything else, so every skewed record also fails the checks below it; classifying in the
		// other order would report a newer Nib's ceremony as forged.
		if errors.Is(verr, ErrVersion) {
			s.State = LoadVersionSkew
			s.Reason = "this ceremony was created by a newer version of Nib — update Nib to " +
				"open it. It is not damaged."
			return s
		}
		s.State = LoadUnverifiable
		s.Reason = "this ceremony's record does not verify: " + verr.Error()
		return s
	}
	s.Intent, s.Expires, s.Roster = r.Intent, r.Expires, r.Roster
	return s
}

// ListStored enumerates the ceremonies on this machine, one entry each, sorted by id.
//
// **One bad entry never costs another (D34's self-healing rule).** Every directory is classified
// independently, so a hand-deleted, damaged or version-skewed ceremony degrades to its own row and
// the rest still load — which is the whole of C12, and the reason this returns a slice of outcomes
// rather than a slice plus an error.
//
// A name that is not a ceremony id is skipped silently rather than reported: the directory is under
// the user's own `~/nib`, and anything else in there is theirs, not a broken ceremony.
func ListStored(root string, now time.Time) ([]Stored, error) {
	ents, err := os.ReadDir(filepath.Join(root, "ceremonies"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no ceremonies yet is not a failure
		}
		return nil, err
	}
	var out []Stored
	for _, e := range ents {
		if !e.IsDir() || ValidID(e.Name()) != nil {
			continue
		}
		out = append(out, ReadStored(root, e.Name(), now))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ErrDifferentProceeding reports that the directory for this ceremony id already holds a DIFFERENT
// proceeding's record — two ceremonies sharing one id.
//
// Its own sentinel because the caller must not report it as a persist failure: on the receive path
// a `WriteMirror` error becomes "Signed, but not saved — do not close Nib", and that sentence is
// false here. Nothing was short of disk, saving a copy elsewhere fixes nothing, and the honest
// remedy is that one of the two ceremonies has to be convened again.
var ErrDifferentProceeding = errors.New("another ceremony is already stored under this id, and it " +
	"is not this one")

// refuseDifferentProceeding compares the stored record, if any, against the one being written.
//
// **Local damage must NOT refuse**, and that asymmetry is deliberate. If the stored record will not
// read or will not decode, the write proceeds: refusing would brick the ceremony with no repair
// path — C12's whole subject — and an attacker who can corrupt that file already has write access
// to `~/nib`, which subsumes this bug entirely. The guard exists to stop a SECOND PROCEEDING, not
// to defend a directory whose contents are already someone else's.
//
// The refusal deliberately says nothing about the stored proceeding beyond its existence: printing
// its intent or roster would turn this into a disclosure oracle for whoever provoked it.
func refuseDifferentProceeding(dir string, r Record) error {
	b, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		return nil // absent (the ordinary first write), or unreadable — see above
	}
	stored, err := Decode(b)
	if err != nil {
		return nil // local damage, not a second proceeding
	}
	want, err := stored.RosterHash()
	if err != nil {
		return nil
	}
	got, err := r.RosterHash()
	if err != nil {
		return err
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("%w: this id already holds a proceeding with a different roster "+
			"commitment, so writing here would destroy it", ErrDifferentProceeding)
	}
	return nil
}
