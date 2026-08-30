package ceremony

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/sign"
)

// AttachmentName is the embedded file the record travels in.
//
// A PDF attachment rather than metadata or a custom dictionary, and the choice was
// **measured rather than reasoned** (caveat 10, discharged 2026-08-18): an attachment added
// in the pre-signing structural pass survives three incremental signatures byte-identical,
// and attaching one *after* a signature invalidates every signature on the document.
//
// **The sentence that used to follow — "that second half is why PrepareDocument is the only
// place a record can be embedded" — named a call site that cannot exist** (found 2026-08-24,
// P07.S02). `p2p.PrepareDocument` calls `AppendReadme` and nothing else; it CANNOT embed a
// record, because `internal/p2p` cannot import this package. Nothing in production embedded a
// record at all when that sentence was written. What enforces the rule is `Embed`'s own
// signature check below, and the convene door that calls it in order.
//
// **The name itself lives in `internal/pdfops` (2026-08-24, P07.S02)**, because `ContentDigest`
// must exclude this one file from the embedded-files axis it now covers — a digest that hashed
// the record would be a fixed point, since the record contains that digest. `pdfops` cannot
// import this package (this one imports it), so one constant with two readers beats two
// constants that can drift. ADR-009.
const AttachmentName = pdfops.CeremonyRecordName

// ErrNoRecord is returned when a document carries no ceremony record. Distinct from a
// malformed one: an ordinary PDF has no record and that is not an error, while a document
// that has one which will not parse is a broken ceremony.
var ErrNoRecord = errors.New("document carries no ceremony record")

// ErrDigestVersion reports a record whose DocHash was computed under a different
// content-digest rule than this build uses.
//
// Its own sentinel, and not a shape of the hash-mismatch error, for the reason D32 exists:
// the two mean opposite things to a reader. A hash mismatch says somebody changed the
// document. This says the two builds measure the document differently and the numbers were
// never comparable — nobody has done anything wrong, and the fix is an update.
var ErrDigestVersion = errors.New("this ceremony's document hash was computed under a different version of Nib")

// DocumentHash is the value a record's DocHash field holds, and the one a later party
// recomputes to check it.
//
// **It is a content digest, not a hash of the file's bytes, and that was forced by
// measurement rather than chosen.** D20 says "SHA-256 of the prepared document", which
// reads as a byte hash — and a byte hash cannot be recomputed by anyone but the writer.
// The record contains the hash, so the document containing the record cannot be what was
// hashed; the obvious repair is "hash it with the record stripped", and that fails too,
// because pdfcpu's rewrite **is not idempotent** — normalising the same document twice
// produces two different files (measured 2026-08-19). Attach-then-detach is not an
// identity, so the convener and a party at hop four would compute different numbers from
// the same document, every time.
//
// pdfops.ContentDigest is stable where a byte hash is not: measured identical across adding
// the attachment, and across a REWRITE of the same document.
//
// # What this hash proves, and to whom — corrected 2026-08-24 (P07.S02), by measurement
//
// The three paragraphs that used to stand here were false of the code, and one of them was
// load-bearing for a plan clause. They are recorded rather than deleted, because the wrong
// version is the one a reader will have in their head:
//
//   - It said the digest was "measured identical … across three incremental signatures", and
//     that "makes the hop-4 clause buildable at all". **Measured false on the real path.**
//     ContentDigest hashes each page's /Annots, and a VISIBLE signature adds a widget annot;
//     `p2p.Contribute` supplies an appearance on every production co-sign. The measurement
//     behind the old sentence was taken with INVISIBLE signatures, and so is the guard that
//     was cited as discharging it. On an honest four-party ceremony `CheckDocument` passes at
//     hop 1 and fails from hop 2 — with a sentence accusing an honest counterparty.
//   - It said the digest does "NOT [cover] annotations, form values, attachments or
//     metadata". Annotations and form values were folded in at v1.116.18 and attachments at
//     this slice; the sentence outlived both.
//   - It said "everything else is covered by the signatures … and flip to invalid on any
//     edit". **False in the window this digest is checked in**, which is exactly the window
//     where a structural rewrite is legal: the document is UNSIGNED, so there is no signature
//     to be the fallback. Measured: an attached schedule swapped under an unchanged filename
//     left the digest unmoved and CheckDocument clean.
//
// **So, stated honestly:** this digest proves that the pages, their geometry and resources,
// their annotations and the document's embedded files are the ones the ceremony was convened
// over — **to the convener, and to anyone who sees the document before the first VISIBLE
// signature.** It is a convene-time identity. Later parties in a hop chain get a byte-prefix
// relationship rather than a recomputable commitment, and that chain is anchored only at the
// convener. Building the per-hop continuity that would replace it is NOT this slice's, and
// the mechanism the plan adopted for it — byte prefix plus `AddedAfter == false` — was
// measured at this slice's grill to PASS on a document whose first page had been blacked out
// by the last signer. See the grill record before relying on it.
func DocumentHash(pdf []byte) (string, error) {
	return pdfops.ContentDigest(pdf)
}

// Embed attaches the record to the document.
//
// It refuses an already-signed document for the same reason PrepareDocument refuses to
// append the readme to one: attaching is a structural rewrite, and a structural rewrite
// invalidates every signature already on the file. Measured, not argued.
func Embed(pdf []byte, r Record) ([]byte, error) {
	if sign.Verify(pdf).State != sign.Unsigned {
		return nil, errors.New("document is already signed; the ceremony record must be embedded before any signature")
	}
	b, err := r.Encode()
	if err != nil {
		return nil, err
	}
	return pdfops.AddAttachment(pdf, AttachmentName, b)
}

// ProceedingOf is what a document claims about the ceremony it belongs to, in the form
// `internal/p2p` can check signatures against (P07.S04).
//
// **One door for "which proceeding is this document's" (ADR-009).** `p2p` cannot import this
// package — since P07.S02a that is a production import cycle — so it takes the commitment as a
// primitive, and every caller that has a document routes through here rather than deriving it.
// Deriving it in two places is how one of them ends up comparing the signatures to each other,
// which is the defect this whole seam exists to close.
//
// **A document with no record, or one that does not verify, yields the ZERO Proceeding**, and
// that is deliberate: `markOneProceeding` treats an empty commitment as disqualifying, so a
// document whose record cannot be read can never be reported as one proceeding. Errors are not
// returned for the same reason — every one of them means the same thing to the caller, which is
// "this document cannot tell you which ceremony it belongs to".
func ProceedingOf(pdf []byte, now time.Time) p2p.Proceeding {
	r, err := CheckRecord(pdf, now)
	if err != nil {
		return p2p.Proceeding{}
	}
	h, err := r.RosterHash()
	if err != nil {
		return p2p.Proceeding{}
	}
	// The OBLIGED signers, in roster order — a `signs:false` convener is not one, which is what
	// makes C16 fall out of C18's count rather than needing a rule of its own (P07.S05a).
	var signing, members []string
	for _, party := range r.Roster {
		members = append(members, party.Fingerprint)
		if party.Signs {
			signing = append(signing, party.Fingerprint)
		}
	}
	return p2p.Proceeding{Commitment: hex.EncodeToString(h), Signing: signing, Members: members}
}

// Extract reads the record out of a document.
func Extract(pdf []byte) (Record, error) {
	b, err := pdfops.ExtractAttachment(pdf, AttachmentName)
	if err != nil || len(b) == 0 {
		return Record{}, ErrNoRecord
	}
	return Decode(b)
}

// CheckDocument is what a party runs on a document it was handed, before any pairing.
//
// `now` is threaded rather than read here for the same reason Record.Verify takes one: a
// clock read inside a validation verdict is nondeterminism reaching a decision.
//
// It answers three questions in the order they matter: is there a record, does its
// convener signature verify, and does the document it arrived in still hash to what the
// record says. The third is the one a later party in the chain can only answer for
// itself — the convener's own bytes satisfy a round-trip test without anyone recomputing
// anything.
func CheckDocument(pdf []byte, now time.Time) (Record, error) {
	r, err := CheckRecord(pdf, now)
	if err != nil {
		return r, err
	}
	got, err := DocumentHash(pdf)
	if err != nil {
		return r, err
	}
	if got != r.DocHash {
		return r, fmt.Errorf("the document does not match the ceremony record: it hashes to %s "+
			"and the record was written for %s — these are not the same document",
			short(got), short(r.DocHash))
	}
	return r, nil
}

// CheckRecord is CheckDocument's first three questions and not its fourth: is there a record,
// does its convener signature verify, and was it written under a digest rule this build can
// compare against. It does NOT compare the document's hash.
//
// # Why this split exists (P07.S02b)
//
// **A receiving party can never pass `CheckDocument`, at any hop — measured, not argued.** The
// document handed to a counterparty always carries at least the sender's co-signature, that
// signature is VISIBLE on every production path (`buildCoSigned` supplies appearance bytes), a
// visible signature adds a widget annotation, and `ContentDigest` hashes `/Annots`. Measured at
// this slice: the hop-1 receiver's copy carries one valid signature, `Extract` and `Record.Verify`
// both return nil, and `CheckDocument` returns *"these are not the same document"* — an
// accusation of tampering aimed at an honest convener.
//
// So C17's *"the party runs CheckDocument"* is not buildable as written, for hop 1 any more than
// for hop 4, and the honest split is this one: the record-level questions are answerable by
// everybody and are what the arrival gate asks; the hash comparison is a **convene-time
// identity**, answerable only before the first visible signature. `embed.go`'s own paragraph
// above says so in those words; this function is that sentence made callable.
//
// `CheckDocument` keeps the hash comparison and keeps its callers — the convener checking their
// own bytes, and the tests that measure the boundary.
func CheckRecord(pdf []byte, now time.Time) (Record, error) {
	r, err := Extract(pdf)
	if err != nil {
		return Record{}, err
	}
	if err := r.Verify(now); err != nil {
		return r, err
	}
	// The digest-rule skew, BEFORE the hash comparison — because the hash comparison is what
	// produces the wrong sentence. Two builds that hash different axes will always disagree
	// about the number; saying "these are not the same document" describes a tampered file,
	// and this is a Nib version difference. D32: a skew produces a sentence naming the
	// mismatch, never a verdict about the counterparty.
	//
	// A zero means a record written before the field existed. There are none in the field —
	// P07.S02 is the first code that ever constructs a Record — so this is treated as the
	// skew it is rather than defaulted, which would silently compare across digest rules.
	if r.DigestVersion != pdfops.ContentDigestVersion {
		return r, fmt.Errorf("%w: this ceremony's document hash was computed under Nib's "+
			"content-digest rule %d and this build uses rule %d, so the two numbers are not "+
			"comparable — update Nib rather than treating this as a changed document",
			ErrDigestVersion, r.DigestVersion, pdfops.ContentDigestVersion)
	}
	return r, nil
}
