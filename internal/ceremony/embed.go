package ceremony

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"nib/internal/p2p"
	"nib/internal/pdfops"
	"nib/internal/pdfread"
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

// ErrTwoRecords is Extract's refusal of a document carrying the record's key twice (/pending 745).
// Neither is excluded from its digest, so the document can match neither record's DocHash; and a
// reader choosing one would be naming a ceremony by the order someone else wrote a tree in. It is
// deliberately NOT ErrNoRecord: every gate treats that as "an ordinary PDF", and convening one
// would embed a third.
var ErrTwoRecords = errors.New("document carries two ceremony records, so which ceremony it " +
	"belongs to has no answer")

// ErrRecordUnreadable is Extract's answer for a document whose list of embedded files this build could not
// read in full (ADR-164, `/pending 880`). The record is an embedded file, so whether the document carries one is
// NOT KNOWN — and it is deliberately not ErrNoRecord, which every gate treats as "an ordinary PDF": a signed
// ceremony document whose record sat in a part of the list pdfcpu's validator dropped was read as an ordinary
// PDF, silently.
var ErrRecordUnreadable = errors.New("the document's embedded files could not be read in full, so whether it " +
	"carries a ceremony record is not known")

// ErrDigestVersion reports a record whose DocHash was computed under a different
// content-digest rule than this build uses.
//
// Its own sentinel, and not a shape of the hash-mismatch error, for the reason D32 exists:
// the two mean opposite things to a reader. A hash mismatch says somebody changed the
// document. This says the two builds measure the document differently and the numbers were
// never comparable — nobody has done anything wrong.
//
// **Which build is behind decides the remedy, and the error says which** (ADR-143): see
// digestRuleSkew. `errors.Is(err, ErrDigestVersion)` answers for either direction.
var ErrDigestVersion = errors.New("this ceremony's document hash was computed under a different version of Nib")

// ShortHash renders a digest for a human sentence — the same abbreviation `CheckDocument` and
// `ReadMirror` already use, exported so a caller outside this package can produce the identical
// sentence rather than a second spelling of it.
func ShortHash(h string) string { return short(h) }

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
// pdfops.ContentDigest is stable where a byte hash is not (rule 8; rule 7 varied on documents holding fonts that differ
// only in their subset tag, /pending 927): measured identical across adding
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
//
// # It is the WRITING side only (ADR-080)
//
// DocumentHash is the current rule, which is what a new record commits to (`Convene`, its one
// production caller). A record is CHECKED by DocumentHashFor, under the rule its own signed
// `DigestVersion` names — so a record written by the previous build is compared with the number that
// build computed, not with this build's.
func DocumentHash(pdf []byte) (string, error) {
	return pdfops.ContentDigest(pdf)
}

// DocumentHashFor is the value r's DocHash should hold for these bytes: the content digest under the
// rule r was written under (ADR-080). Every site that COMPARES a DocHash computes it here — a
// comparison under any other rule is a tampering sentence for a version difference.
//
// A rule this build does not compute is ErrDigestVersion (digestRuleSkew), never a number.
func DocumentHashFor(pdf []byte, r Record) (string, error) {
	if err := digestRuleComputable(r); err != nil {
		return "", err
	}
	return pdfops.ContentDigestAt(pdf, r.DigestVersion)
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
	return pdfops.EmbedCeremonyRecord(pdf, b)
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
//
// **Through `pdfops.CeremonyRecord`, the entry ContentDigest excludes, and no other** (/pending
// 745). It read the record by name, and pdfcpu resolves a name by tree key and then by the first
// filespec CALLING itself `nib-ceremony.json` — so which entry was the record had one answer here
// and another in the digest. A document with two is ErrTwoRecords, by name.
func Extract(pdf []byte) (Record, error) {
	b, err := pdfops.CeremonyRecord(pdf)
	if errors.Is(err, pdfops.ErrTwoCeremonyRecords) {
		return Record{}, ErrTwoRecords
	}
	if errors.Is(err, pdfread.ErrNameTreeUnread) {
		return Record{}, fmt.Errorf("%w: %w", ErrRecordUnreadable, err)
	}
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
	got, err := DocumentHashFor(pdf, r)
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
// does its convener signature verify, and was it written under a digest rule this build knows (the
// current one, or a retired one, which is a record and not a hash it can recompute). It does NOT
// compare the document's hash.
//
// # Why this split exists (P07.S02b)
//
// **A receiving party can never pass `CheckDocument` on an arrival that carries a signature —
// measured, not argued.** It said "at any hop" until P08.S03, and the exception is the case C04
// turned on: the first signing party of a ceremony whose convener CARRIES receives a document with
// no signature on it, and there the comparison is both answerable and the only content anchor that
// party has. `internal/server`'s arrival gate now asks it exactly there. The measurement below is
// unchanged and is why it is asked nowhere else. The
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
// `CheckDocument` keeps the hash comparison. **It has NO production callers, and this comment
// claimed three** — the convener checking their own bytes, the arrival gate on a signature-free
// arrival, and the tests. Whole-tree grep (`CheckDocument(` excluding `_test.go` and this
// declaration): **zero**. The arrival gate calls `ceremony.DocumentHash`
// (`internal/server/ceremonyid.go`) and says so at its own line; the only hits anywhere are tests
// and comments.
//
// The function is kept rather than deleted because ADR-013's last bullet already states the
// zero-caller fact correctly and treats the comparison as the shape a future caller wants. What
// is corrected here is the description: a sentence naming callers that do not exist is how a
// reader concludes a gate is enforced somewhere it is not (/pending 458, found by the 2026-09-09
// deepdive; the root `zerocaller_test.go` now names `CheckDocument` among the exports allowed
// none — /pending 445).
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
	if err := digestRuleSkew(r); err != nil {
		return r, err
	}
	return r, nil
}

// digestRuleSkew is the one door for D32's RECORD-LEVEL digest-rule question: nil when r names the rule this
// build computes (`pdfops.ContentDigestRuleReadable`) or one it retired (`ContentDigestRuleRetired`, a record
// it can read but not recompute — D1, P02.S08), else a skew error that is ErrDigestVersion to `errors.Is`.
// A site that is about to COMPARE or COMPUTE a DocHash asks `digestRuleComputable` below instead, FIRST
// (DocumentHashFor, ReadMirror), because a comparison across rules produces a tampering sentence for a
// version difference — /pending 725's bump to rule 4 would otherwise have reported every stored unsigned
// mirror as damaged.
func digestRuleSkew(r Record) error {
	if pdfops.ContentDigestRuleReadable(r.DigestVersion) || pdfops.ContentDigestRuleRetired(r.DigestVersion) {
		return nil
	}
	return digestSkew{rule: r.DigestVersion}
}

// digestRuleComputable is the door for a site that is about to COMPUTE a digest from document bytes
// (`DocumentHashFor`, and `readMirrorAt` for a mirror it is not reading to end): nil only for the current rule. A
// retired rule is the skew "older-nib" (wire code 18), never a mismatch, because rules 5 and 6 are not computable by
// this build (D1, P02.S08). `digestRuleSkew` above is the record-level question and accepts a retired rule.
func digestRuleComputable(r Record) error {
	if pdfops.ContentDigestRuleReadable(r.DigestVersion) {
		return nil
	}
	return digestSkew{rule: r.DigestVersion}
}

// RequireComputableRule is digestRuleComputable for a caller outside the package that CONTINUES a ceremony without
// necessarily computing a digest: the arrival gate checks the hash on hop 1 only, and a retired rule's record
// (CheckRecord accepts it) must not be signed further on hop 2 or later because no hash was asked for (D1, P02.S08).
func RequireComputableRule(r Record) error { return digestRuleComputable(r) }

// digestSkew is the skew, with its DIRECTION (ADR-143, /pending 691).
//
// The sentence used to end "update Nib" whichever build was behind. That is the right advice when the
// record is NEWER than this build, and exactly wrong when it is OLDER: every bump retires the oldest
// readable rule (ADR-080), so a ceremony begun two rules back meets this sentence on the up-to-date
// machine, where there is nothing to update to. The remedy there is to finish the ceremony on the Nib
// it was started with, or to convene it again.
//
// It is ErrDigestVersion to `errors.Is`, as every caller already asks, and ALSO the p2p sentinel for
// its direction — so the arrival gate's refusal crosses the wire by name (`p2p.refusalCode`) instead of
// as a bare EOF, without a second mapping at each site that raises it (ADR-009).
type digestSkew struct {
	rule int // the rule the record names; 0 is a record written before the field existed
}

// retired reports that the record's rule is one this build no longer computes, as opposed to one it
// does not compute yet.
func (e digestSkew) retired() bool { return e.rule < pdfops.ContentDigestVersion }

func (e digestSkew) Error() string {
	oldest := pdfops.ContentDigestVersion
	for pdfops.ContentDigestRuleReadable(oldest - 1) {
		oldest--
	}
	computes := fmt.Sprintf("rule %d", pdfops.ContentDigestVersion)
	if oldest != pdfops.ContentDigestVersion {
		computes = fmt.Sprintf("rule %d and rule %d", oldest, pdfops.ContentDigestVersion)
	}
	if e.retired() {
		return fmt.Sprintf("this ceremony was started with an older Nib: its document hash was computed "+
			"under content-digest rule %d and this build computes %s, so the two numbers are not "+
			"comparable — finish the ceremony on the Nib it was started with, or convene it again. This "+
			"is a version difference, not a changed document", e.rule, computes)
	}
	return fmt.Sprintf("this ceremony was started with a newer Nib: its document hash was computed "+
		"under content-digest rule %d and this build computes %s, so the two numbers are not "+
		"comparable — update Nib to take part in it. This is a version difference, not a changed "+
		"document", e.rule, computes)
}

func (e digestSkew) Is(target error) bool {
	switch target {
	case ErrDigestVersion:
		return true
	case p2p.ErrCeremonyRuleRetired:
		return e.retired()
	case p2p.ErrCeremonyRuleTooNew:
		return !e.retired()
	}
	return false
}
