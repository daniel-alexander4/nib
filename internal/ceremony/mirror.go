package ceremony

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// ErrMirrorDamaged reports a stored ceremony whose document does not match its own record.
//
// Its own sentinel because the alternative sentence is an accusation: without it the
// mismatch surfaces at CheckDocument as "these are not the same document", which describes a
// counterparty substituting a file. This one says the copy on THIS machine is damaged, which
// is what actually happened and what the user can act on.
var ErrMirrorDamaged = errors.New("this ceremony's stored document does not match its record")

// MirrorDir returns the directory for a ceremony, under root (normally ~/nib).
func MirrorDir(root, id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("%w: %q", ErrBadID, id)
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

// RemoveMirror deletes a ceremony's directory — the close-out prune (D29).
func RemoveMirror(root, id string) error {
	dir, err := MirrorDir(root, id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
