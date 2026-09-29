// Package sign handles PDF digital-signature verification (and, from M5,
// creation). Every opened document is checked so the UI can show an
// untampered / modified / unsigned badge plus per-signer detail.
//
// Tamper-evidence is purely cryptographic: pdfsign's verifier recomputes the
// signed byte-range hash, and an edit inside a signature's ranges flips that
// signature's ValidSignature to false. An edit in a revision APPENDED after it
// leaves the signature valid — its ranges never covered those bytes — and is what
// `Status.AddedAfter` reports, measured over the verified, well-formed records of
// the one revision sweep (`revisions.go`, ADR-058/059).
// Whether the signer's certificate chains to a trusted CA (TrustedIssuer) is a
// separate identity question we deliberately ignore here — Nib cares about
// integrity, not third-party trust. We report every signer, not just the
// first, and how much weight each signing time carries (see TimeBacking).
package sign

import (
	"bytes"
	"fmt"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/verify"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// State is the integrity verdict for a document.
type State string

const (
	// Unsigned: the document carries no signature. Not an error — most PDFs.
	Unsigned State = "unsigned"
	// Valid: a signature is present and the document is unmodified since signing.
	Valid State = "valid"
	// Invalid: a signature is present but the document was modified since signing
	// (or the signature itself fails to verify) — i.e. tamper-evident.
	Invalid State = "invalid"
)

// TimeBacking says how much a signer's signing time can be trusted.
type TimeBacking string

const (
	// NoTime: the signature records no signing time at all.
	NoTime TimeBacking = "none"
	// SelfAsserted: a time is present but stated by the signer — it proves
	// nothing on its own, since the signer chose the value.
	SelfAsserted TimeBacking = "self-asserted"
	// TSA: the time is fixed by an RFC3161 timestamp token whose authority's certificate chains, as
	// a timestamping certificate, to a root this machine trusts — independent of the signer.
	TSA TimeBacking = "tsa"
	// TSAUnverified: a timestamp token is present, and its authority could NOT be verified — a
	// self-signed "authority", an unknown root, no certificate at all. The token proves only that
	// whoever signed it chose that time, and whoever made the signature can mint one with any date
	// (`/pending 708`: a backdated 2001 token from "Totally Independent TSA" read as independent).
	TSAUnverified TimeBacking = "tsa-unverified"
)

// SignerInfo is the per-signer detail surfaced to the UI.
type SignerInfo struct {
	// Name is the signature dictionary's /Name — what the signer TYPED, not the certificate's
	// subject. It is display text and nothing more: it is chosen by whoever made the signature,
	// so it is no evidence of identity. Fingerprint is the identity. (This field's comment read
	// "certificate subject common name" until ADR-051; the library has always taken it from the
	// PDF, and a reader who trusted the comment would have been trusting attacker-typed text.)
	Name        string      `json:"name,omitempty"`
	Valid       bool        `json:"valid"`            // this signer's byte-range hash checks out
	When        string      `json:"when,omitempty"`   // signing time (display string), when present
	TimeBacking TimeBacking `json:"timeBacking"`      // none / self-asserted / tsa / tsa-unverified
	Reason      string      `json:"reason,omitempty"` // signature /Reason; for co-signing, carries the attestation
	// Fingerprint is the hex SHA-256 SPKI of the certificate that SIGNED — the one this
	// signature's SignerInfo names by issuer and serial, never whichever certificate happens to
	// lead the bag (ADR-051) — read from the signature's `Revision` (ADR-058). Empty means nib
	// could not establish who signed; a consumer must treat that as unrecognised and never as a
	// match.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Status is the verification result surfaced to the UI.
type Status struct {
	// State is the verdict over Signers: `Valid` when every one verifies, `Invalid` when any fails
	// or when there is none and a signature blob is present (ADR-060).
	State State `json:"state"`
	// Signers are the records that count as signers (ADR-060): well-formed and not a document
	// timestamp. A refused copy of a real signature and a document timestamp are never here —
	// they are in Refused and Timestamps — so, WHEN THE JOIN SUCCEEDS, `len(Signers)` counts people
	// who signed, and a reader that places the next signature or matches a roster by position may
	// rely on it. Under a join error it does not hold: nothing is excluded (a refused copy or a
	// timestamp may be listed as a signer), no signer has a fingerprint, and `AddedAfter` warns
	// `could-not-check` — a reader that needs the count must not trust it without that warning clear.
	Signers []SignerInfo `json:"signers,omitempty"`
	// AddedAfter is true when the document carries content past the coverage of
	// its last VALID, well-formed signature — added after signing, covered by no
	// valid signature — or when nib could not confirm it does not. It does NOT make
	// the existing signatures invalid (each still proves its own content intact);
	// it warns that the final document is not wholly signed. In multi-party
	// signing only content after the LAST signature is flagged — content added
	// between signatures is expected. A document timestamp never counts as
	// coverage (ADR-059), and **this is never "unchanged since you signed"** —
	// that answer is a fingerprint-selected prefix, not this bit.
	AddedAfter bool `json:"addedAfter,omitempty"`
	// AddedAfterCause says which fact set AddedAfter, and is empty when it is
	// false: `appended`, `refused-signature-present` or `could-not-check`.
	AddedAfterCause AddedAfterCause `json:"addedAfterCause,omitempty"`
	// Refused lists every signature-shaped dictionary nib refused that carries a
	// non-empty /Contents, by object number, so a reader can say "object 31 claims
	// to be a signature and is not one". An empty /Contents is a prepare-for-signing
	// placeholder, not a claim, and is never listed. Its /Filter is the document's
	// own text, capped at 64 bytes.
	Refused []RefusedSignature `json:"refused,omitempty"`
	// Timestamps lists the object numbers of the document timestamps the document carries
	// (`/Type /DocTimeStamp` or `/SubFilter /ETSI.RFC3161`, non-empty `/Contents`, not refused).
	// Nib does not check a timestamp: it names no signer, anyone can obtain one over any bytes, and
	// it is neither a signer nor coverage (ADR-059, ADR-060). It is listed so a reader can say the
	// document carries one rather than say nothing about an object it saw.
	Timestamps []uint32 `json:"timestamps,omitempty"`
}

// AddedAfterCause names which fact set `Status.AddedAfter` (ADR-059).
type AddedAfterCause string

const (
	// AddedAfterAppended: every signature-shaped dictionary is well-formed, and bytes follow the
	// coverage of the last valid signature.
	AddedAfterAppended AddedAfterCause = "appended"
	// AddedAfterRefusedSignature: at least one valid signature bounds coverage, bytes follow it —
	// measured — AND the document carries a signature-shaped dictionary nib refused
	// (`Status.Refused`). Both facts, so every reader names both. The refused one may claim to reach
	// the end — /pending 661's decoys did — and a refusal is exactly a claim nib will not take.
	AddedAfterRefusedSignature AddedAfterCause = "refused-signature-present"
	// AddedAfterCouldNotCheck: the sweep or the join could not be trusted, or no valid, well-formed
	// record bounds coverage — nothing was measured, whatever else is refused.
	AddedAfterCouldNotCheck AddedAfterCause = "could-not-check"
)

// RefusedSignature is one refused record as a reader is shown it.
type RefusedSignature struct {
	Obj uint32 `json:"obj"`
	// Filter is the dictionary's `/Filter`, attacker-typed and capped at maxRefusedFilter bytes;
	// every reader escapes it (the CLI prints it `%q`, the page sets textContent).
	Filter string       `json:"filter,omitempty"`
	Cause  RefusalCause `json:"cause"`
}

// maxRefusedFilter caps the attacker-typed `/Filter` a refused record carries to a reader.
const maxRefusedFilter = 64

// refusedOf is the reader's view of every refused record, in xref order.
func refusedOf(revs []Revision) []RefusedSignature {
	var out []RefusedSignature
	for i := range revs {
		// A placeholder (empty `/Contents`) claims nothing, so it is not published as refused: it
		// would unhide the details button on an unsigned document to say nothing true.
		if revs[i].Cause == "" || !revs[i].hasContents {
			continue
		}
		f := revs[i].Filter
		if len(f) > maxRefusedFilter {
			f = f[:maxRefusedFilter]
		}
		out = append(out, RefusedSignature{Obj: revs[i].Obj, Filter: f, Cause: revs[i].Cause})
	}
	return out
}

// timestampsOf is the reader's view of every document timestamp, in xref order: the records that
// are timestamps and are not refused (a refused one is in `refusedOf`), and not placeholders.
func timestampsOf(revs []Revision) []uint32 {
	var out []uint32
	for i := range revs {
		if revs[i].Timestamp && revs[i].Cause == "" && revs[i].hasContents {
			out = append(out, revs[i].Obj)
		}
	}
	return out
}

// Verify reports whether data is unsigned, signed-and-untampered, or
// signed-and-modified, with per-signer detail. It never returns an error for
// the ordinary unsigned case — a PDF without signatures simply reports Unsigned.
func Verify(data []byte) Status {
	st, _, _ := verifyIndexed(data)
	return st
}

// libraryVerify is the one call into `digitorus/pdfsign/verify` (signing goes through
// `pdfsign/sign`, `identity.go`), as a variable so a test can prove a
// document never reached it (the K-pair gate below).
var libraryVerify = verify.Verify

// verifyIndexed is Verify's door, and `Revisions`': the byte scan, the readability gate, the sweep,
// the library and the join, in that order and in one place (ADR-009). The records it returns carry
// the join's `Verified`; its error is anything that makes them untrustworthy.
func verifyIndexed(data []byte) (Status, []Revision, error) {
	// **A document with no signature never reaches the third-party parser** (/pending 453(a)).
	//
	// `digitorus/pdf` has an unbounded read that no `recover` can contain: `readByte` returns `'\n'`
	// at EOF forever (`lex.go:71-82`) and `readLiteralString` loops appending with no EOF check
	// (`:229`), so an unterminated literal string allocates until the process dies. That is a
	// `fatal error: out of memory`, not a panic — the library's own `recover` cannot catch it,
	// `internal/safe.Recover` cannot catch it, and Go has no per-goroutine allocation cap. It is an
	// upstream defect and it cannot be fixed here.
	//
	// What CAN be done is to stop handing it documents it has no reason to see. Every signature
	// dictionary carries `/ByteRange`; a document without those bytes has no signature to verify,
	// so it is `Unsigned` by inspection and the parser is never entered. That removes the crash
	// surface for every unsigned document — which is what an upload usually is — and leaves it only
	// for documents that genuinely claim a signature.
	//
	// **It was a reduction and not a fix**, and the residue was filed (/pending 502, then 509). A
	// malformed document that DOES carry `/ByteRange` still reached the parser and could still take
	// the process. Measured: 12 of 7,438 single-bit flips of a signed fixture, every one a damaged
	// `/Filter` key on the object stream. The runaway needs a buffer with `allowEOF` set, and in the
	// verify path only the object-stream lexer sets it (`read.go:890`), so it is object-stream
	// content that is lexed past its end. v0.2.0 of the library bounds `readLiteralString` but
	// cannot be adopted: pdfsign calls `Reader.Resolve`, which v0.2.0 removed — and its
	// `readHexString` still spins at EOF.
	//
	// **The second gate below closes that residue** (ADR-041). Read the two together: the byte scan
	// keeps unsigned documents out of the parser, and `pdfcpuCanRead` keeps unreadable ones out.
	if !scanForSignatureBlob(data) {
		return Status{State: Unsigned}, nil, nil
	}
	// **Nothing reaches `digitorus/pdfsign` that `pdfcpu` cannot read** (ADR-041).
	//
	// This is the ONE door, and it is here because `verify.Verify` has exactly one call site in
	// this repo — so a rule enforced at the library's entrance is enforced for all 27 callers of
	// `Verify`, which is what a receive-path-only gate could not do.
	//
	// The verdict is `Invalid` rather than `Unsigned` for the reason `scanForSignatureBlob`'s own
	// comment gives: a document carrying `/ByteRange` that no parser can read is "something is
	// wrong with a signed document", and calling it *never signed* is the unsafe direction.
	// `p2p.ContributionProgress` already renders exactly that — "this document carries a signature
	// that cannot be read".
	if err := pdfcpuCanRead(data); err != nil {
		return Status{State: Invalid}, nil, err
	}
	// **The sweep runs before the library, and so does the K-pair gate** (P01.S01). The library
	// copies every pair a `/ByteRange` names into memory, so `[0 S 0 S … ×K]` allocates K×S and a
	// Go out-of-memory is not recoverable; a document whose ranges would read more than it holds,
	// name a negative length, or sit in an indirect array (re-parsed three times per pair) is
	// `Invalid` without the library ever seeing it — on upload, install and undo alike. Shapes that
	// read nothing are the structure rule's refused records, not this gate's (`libraryWouldOverread`).
	//
	// **A sweep that cannot finish is `Invalid` before the library too**, and it warns: the gate
	// above has nothing to judge, and a library call on a document the sweep could not read copies
	// whatever ranges it finds (measured: an indirect `/SubFilter` to a mis-headed object made the
	// sweep fail and the library copy 25 MB). No honest producer's output errors here
	// (`TestNoProducerSignatureIsRefused` asserts it per source).
	revs, sweepErr := sweepRevisions(data)
	if sweepErr != nil {
		return Status{State: Invalid, AddedAfter: true, AddedAfterCause: AddedAfterCouldNotCheck}, nil, sweepErr
	}
	if libraryWouldOverread(revs, len(data)) {
		return Status{State: Invalid, AddedAfter: true, AddedAfterCause: AddedAfterCouldNotCheck, Refused: refusedOf(revs), Timestamps: timestampsOf(revs)}, revs, errLibraryWouldOverread
	}
	resp, err := libraryVerify(bytes.NewReader(data), int64(len(data)))
	if err != nil || resp == nil || len(resp.Signers) == 0 {
		// Zero parseable signers covers two very different documents: one that is
		// genuinely unsigned, and one whose signature blob is present but fails to
		// parse (the library drops that signer with no top-level error). The latter
		// is tamper-evident — silently downgrading it to Unsigned would hide a
		// corrupted signature — so cross-check the PDF for an actual signature blob
		// before calling it unsigned.
		// **The `err == nil` conjunct was here and it defeated the cross-check exactly when it was
		// needed** (/pending 453). The comment above states the rule — a signature blob that fails
		// to parse must not be "silently downgraded to Unsigned" — and the guard skipped the blob
		// question whenever the library returned an error, which is the commonest way a tampered
		// document arrives. Measured over 75 single-byte flips inside the pre-signature revision:
		// 40 reported `unsigned`, and 39 of those still had a signature blob present. `web/app.js`
		// renders that as "Unsigned", which a reader takes as *never signed*.
		//
		// `signatureBlobPresent` no longer panics and answers through a bounded scan when the parse
		// fails, so it is safe to ask on the error path — which is the only path where the answer
		// changes anything.
		//
		// **And the sweep answers the same question where `/Fields` cannot see** (P01.S02): a record
		// with a non-empty `/Contents` is a signature blob wherever it sits — under `/Kids`, or listed
		// nowhere — so a nested signature whose PKCS#7 fails no longer reads `Unsigned` while
		// `Revisions` holds its record. Non-empty, never "any record": a prepare-for-signing
		// placeholder (`/Type /Sig`, empty `/Contents`) is a record and stays `Unsigned`, which is
		// `signatureBlobPresent`'s own contract. And only a record the library would have processed,
		// or one the sweep refused (`checkableBlob`): a Reader-extended form's intact `/Perms /UR3`
		// signature with no `/SigFlags` is a blob the library never looks at, and it read `Unsigned`
		// before this rule — counting it made such a form `Invalid` with nothing to show why.
		st := Status{State: Unsigned, Refused: refusedOf(revs), Timestamps: timestampsOf(revs)}
		if signatureBlobPresent(data) || anyCheckableBlob(revs) {
			st.State = Invalid
		}
		// No library signer: every record is unverified, and a record the library should have
		// reported makes the join disagree — which is what `Revisions` must say.
		_, jerr := joinLibrary(revs, nil)
		if jerr != nil {
			// A signature the library should have reported and did not is one nobody checked: the
			// document is `Invalid` and the warning is raised — fail-closed, as every other `errJoin`
			// route is (P01.S01's claims pass found this branch returning the error with no warning).
			st.State = Invalid
		}
		st.AddedAfter, st.AddedAfterCause = addedAfter(revs, len(data), jerr, false)
		return st, revs, jerr
	}

	// A document is untampered only if every signer's byte-range hash checks out.
	//
	// **Who signed is a separate question from whether the bytes are intact, and the library
	// answers only the second** (ADR-051). The sweep re-read each signature's PKCS#7 for the
	// certificate its SignerInfo names; the join lines the library's signers up with those records
	// by position and cross-checks the bag (ADR-058). A join that disagrees names nobody, and the
	// same disagreement makes `AddedAfter` warn below.
	//
	// **A library signer is reported only if its record counts as a signer** (ADR-060, P01.S03). The
	// library enumerates a refused copy of the victim's dictionary as a second valid signer under the
	// victim's name (/pending 687) and a document timestamp as a failed one (/pending 737), so taking
	// its list as the signers let a copy halt a ceremony, move the next signature's placement, and a
	// B-LTA document read `Invalid`. A refused record's failed verdict does not set `State` either:
	// every byte inside a counted signer's coverage is hash-bound, so what a refused record can hide
	// is a change past the last counted signer — which is exactly what `AddedAfter` reports.
	//
	// **Under a join error nothing is excluded** (declared): the positions are not known to line up,
	// so which record a library signer is cannot be said. Every library signer is listed with no
	// fingerprint, as before, and `AddedAfter` warns `could-not-check`.
	at, joinErr := joinLibrary(revs, resp.Signers)
	st := Status{State: Valid, Refused: refusedOf(revs), Timestamps: timestampsOf(revs)}
	for i := range resp.Signers {
		fp := ""
		if joinErr == nil {
			if !revs[at[i]].countsAsSigner() {
				continue
			}
			fp = revs[at[i]].Fingerprint
		}
		si := signerInfo(&resp.Signers[i], fp)
		if !si.Valid {
			st.State = Invalid
		}
		st.Signers = append(st.Signers, si)
	}
	// **No counted signer is not `Valid`** — nothing verified — and it is not `Unsigned` while a
	// signature blob is present (ADR-060): a timestamp-only document, and a lone signature refused
	// or relabelled `/SubFilter /ETSI.RFC3161` in place (its own tamper), must not read "never
	// signed". The same door the zero-library-signer path asks.
	if len(st.Signers) == 0 {
		st.State = Unsigned
		if anyCheckableBlob(revs) {
			st.State = Invalid
		}
	}
	// Flag content appended after the most-recent VALID signature, read from the records the join
	// just marked (P01.S02 deleted the `/Fields` ByteRange walk that did this on its own, and with
	// it the "two enumerations" the old comment here worried over: /pending 661's decoy, listed in
	// `/Fields`, lent that walk a coverage end no signature vouched for).
	//
	// **This must not change the integrity VERDICT** — a signature that hashed correctly is still
	// valid over its own byte range whatever coverage says, so `State` is untouched.
	//
	// **A join that disagrees is a "could not confirm"** (P01.S01): the records and the library then
	// describe different documents, and nothing either says about coverage is known to be about this
	// one. Fail-closed — `AddedAfter` warns, and the cause says it could not check.
	//
	// **The fourth argument is the LIBRARY's count, never `len(st.Signers)`** (P01.S03): the rule it
	// feeds is "the library saw a signature and none bounds", and after ADR-060 a timestamp-only or
	// refused-only document has library signers and no counted one — following the new count would
	// read it "nothing added" about bytes no valid signature covers.
	st.AddedAfter, st.AddedAfterCause = addedAfter(revs, len(data), joinErr, len(resp.Signers) > 0)
	return st, revs, joinErr
}

// anyCheckableBlob reports whether any record carries a non-empty `/Contents` — a signature blob seen
// by the xref sweep, which `signatureBlobPresent`'s `/Fields` walk cannot see under `/Kids` — EXCEPT
// a well-formed one the catalog's `/Perms` names and the library never enumerated: a `/Perms /UR3`
// usage-rights signature on a form with no `/SigFlags`, which the zero-signer path would otherwise
// call `Invalid` with no signer and no refusal to name.
//
// **The exemption is `/Perms`, never "the library did not enumerate it"** (the P01.S02 re-review):
// `/SigFlags` is a catalog key an appended revision can drop, so exempting every record outside the
// library's enumeration let anyone strip it and turn a signed document `Unsigned` — hiding that it was
// ever signed. Measured on four shapes before this rule; each now reads `Invalid`.
func anyCheckableBlob(revs []Revision) bool {
	for i := range revs {
		exempt := revs[i].underPerms && revs[i].Cause == "" && revs[i].libPos < 0
		if revs[i].hasContents && !exempt {
			return true
		}
	}
	return false
}

// coverage is the one measurement of "how far": the largest `CoverageEnd` over the records that
// `bounds` — verified by the library, well-formed, and not a document timestamp — against the
// file's length. `sawSignature` is false when no record bounds at all, which is not the same fact as
// "nothing was appended", and `addedAfterVerdict` needs both.
func coverage(revs []Revision, size int) (trailing, sawSignature bool) {
	var maxEnd int64
	for i := range revs {
		if !revs[i].bounds() {
			continue
		}
		sawSignature = true
		maxEnd = max(maxEnd, revs[i].CoverageEnd)
	}
	return sawSignature && int64(size) > maxEnd, sawSignature
}

// addedAfter is `addedAfterVerdict`'s one caller (the guard counts it): it measures coverage over
// the records, asks the verdict, and names the cause. The verdict's body is kept as it was so its
// replayed red proof (`added-after-fails-closed`) still mutates the rule it was written for.
//
// Cause precedence, first match wins: an error (sweep or join) → `could-not-check`; no record
// bounds at all → `could-not-check`, even with a refused record present, because nothing measured
// an append and `refused-signature-present` is read as one; any refused record →
// `refused-signature-present`; otherwise `appended`. (The slice grill's order put the refusal
// before the bounding check; the P01.S02 review found it then claimed an append nobody measured.)
// A refused record present with the valid signatures reaching EOF does not warn — the bit is a
// coverage fact, and `Refused` reports the record either way.
func addedAfter(revs []Revision, size int, err error, librarySawSigners bool) (bool, AddedAfterCause) {
	trailing, saw := coverage(revs, size)
	if !addedAfterVerdict(trailing, saw, err, librarySawSigners) {
		return false, ""
	}
	switch {
	case err != nil, !saw:
		return true, AddedAfterCouldNotCheck
	case len(refusedOf(revs)) > 0:
		return true, AddedAfterRefusedSignature
	}
	return true, AddedAfterAppended
}

// addedAfterVerdict combines the coverage measurement with its error under one rule: content
// found OR the check could not run means "warn". It is a named function and not an inline
// `a || err != nil` because the fail-closed direction is the whole point of it — an inline
// expression is one careless refactor away from `a` alone, which is the silent-clean behaviour
// this replaced, and nothing would fail. This is what the test binds; `addedAfter` is its one
// caller.
func addedAfterVerdict(trailing, sawSignature bool, err error, librarySawSigners bool) bool {
	// **The library reporting a signer that no valid, well-formed record stands for is itself a
	// "could not confirm".** Until P01.S02 this rule caught the library's xref walk and a
	// `/Fields` walk disagreeing (/pending 270); the `/Fields` walk is gone, and what reaches here
	// now is a document whose every library signer failed, or was refused, or is a document
	// timestamp — there is then no coverage end to measure against, and "nothing trailing" would
	// report a document as wholly signed that no valid signature covers at all.
	if librarySawSigners && !sawSignature {
		return true
	}
	return trailing || err != nil
}

// HasSignatureBlob reports whether the PDF carries a signature field with contents, independently
// of whether any library can PARSE those contents.
//
// **Exported because `Verify`'s `Unsigned` is not the same question, and a caller that needs the
// stricter one was silently getting the looser (P08.S03).** When this was written `Verify`
// downgraded to `Unsigned` whenever `verify.Verify` returned an error — the cross-check was reached
// only on the `err == nil` path (closed by /pending 453, and widened over the revision sweep by
// P01.S02) — so a document whose signature blob was present but which that library could not
// parse read as unsigned. That was the right answer for "is there a valid signature"; it is the
// WRONG answer for "may I compare this document's content digest against a convene-time hash",
// where treating a signed document as unsigned produces a tampering accusation for a library
// divergence.
func HasSignatureBlob(pdf []byte) bool { return signatureBlobPresent(pdf) }

// signatureBlobPresent reports whether the document has an AcroForm signature
// field carrying a non-empty /Contents — i.e. a real PKCS#7 blob the verifier
// should have been able to parse. It's the discriminator between a genuinely
// unsigned document (no such field, or an empty placeholder field left by another
// tool's "prepare for signing") and one whose sole signature failed to parse.
//
// # It DID panic, on 115 of 300 single-byte flips (/pending 453)
//
// The sentence here used to end *"best-effort, never panics"*, and that was false against
// attacker-supplied bytes: `digitorus/pdf` panics out of `applyFilter`, `readXref` and the object
// parser on ordinary corruption. Measured on a 3,715-byte signed document — one bit flipped per
// run, 300 offsets — **115 panicked**. Its only production caller is `ceremonyid.go`'s arrival
// gate, reached from `sessionConfirmer.Confirm` on the **p2p arm**, where `net/http`'s per-request
// recover does not apply: the panic takes the process, not the request.
//
// # Why a recover alone would have been the wrong fix
//
// Returning `false` on failure is this function's documented contract, and for a *parse error* it
// is right. But `Verify` uses the answer to tell "genuinely unsigned" from "signed and unreadable",
// so answering `false` for a document that panicked mid-parse reports a tampered signed document as
// **Unsigned** — which `web/app.js` renders as "Unsigned", i.e. *never signed*. That is the unsafe
// direction, and it is exactly the defect the cross-check exists to prevent.
//
// So the fallback is not `false`, it is a **bounded byte scan**: every signed PDF's signature
// dictionary carries `/ByteRange`, the scan allocates nothing, parses nothing, and cannot panic or
// grow. A document that will not parse but contains `/ByteRange` is reported as carrying a blob,
// which routes it to `Invalid` — "something is wrong with a signed document" — rather than to a
// claim that it was never signed.
//
// **A named exemption from the one-enumeration guard** (`TestEverySignatureEnumerationIsTheSweep`,
// P01.S02): it walks `/Fields`, and re-expressing it over the sweep would NARROW it — the sweep keeps
// only signature-shaped dictionaries, and this answers for any `FT /Sig` field with contents — which
// moves a document towards `Unsigned`, the unsafe direction. `verifyIndexed` asks the sweep the same
// question beside it (`anyCheckableBlob`), so a `/Kids`-nested blob this walk cannot see is still seen.
func signatureBlobPresent(pdf []byte) (present bool) {
	// The recover is positional: it must cover the whole parse below, including the lazy
	// dereferences inside the Key/Index walk, which is where most of the 115 panics landed.
	defer func() {
		if r := recover(); r != nil {
			present = scanForSignatureBlob(pdf)
		}
	}()
	// **A file the library cannot read the way it is meant is not read** (/pending 733). digitorus/pdf
	// never follows a hybrid-reference trailer's `/XRefStm` (ISO 32000-1 7.5.8.4; its `readXrefTable`
	// follows `/Prev` only), so every object that stream lists is invisible to it — the catalog, a
	// field, the signature dictionary itself — while pdfcpu reads them all and admits the file to the
	// library. A walk that then finds no field has not found no signature, so the answer is the byte
	// scan's. An UNSIGNED hybrid file carries no `/ByteRange` and still answers false.
	if bytes.Contains(pdf, []byte("/XRefStm")) {
		return scanForSignatureBlob(pdf)
	}
	r, err := dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return scanForSignatureBlob(pdf)
	}
	// **A parse that found no catalog is not a parse that found no signatures** (/pending 733): a
	// trailer `/Root` the library resolves to null, or to something that is neither typed `/Catalog`
	// nor carries the catalog's required `/Pages`, is a document it did not read.
	root := r.Trailer().Key("Root")
	if root.Key("Type").Name() != "Catalog" && root.Key("Pages").IsNull() {
		return scanForSignatureBlob(pdf)
	}
	acro := root.Key("AcroForm")
	if acro.IsNull() {
		return false
	}
	fields := acro.Key("Fields") //sigwalk:exempt signatureBlobPresent
	for i := 0; i < fields.Len(); i++ {
		f := fields.Index(i)
		if f.Key("FT").Name() != "Sig" {
			continue
		}
		if len(f.Key("V").Key("Contents").RawString()) > 0 {
			return true
		}
	}
	return false
}

// pdfcpuCanRead reports whether the parser nib uses for everything else can build a context from
// pdf — i.e. whether this is a document nib can read at all. It is the guard on the one door into
// `digitorus/pdfsign` (ADR-041).
//
// # Why another parser rather than a hand-rolled check
//
// The crash it prevents is `fatal error: out of memory`, which **no `recover` catches** — not the
// library's own, not `internal/safe.Recover` — because Go gives no per-goroutine allocation cap.
// So the only containment is to not hand the library the bytes. The three ways to decide which
// bytes were a patched fork of the library, a subprocess under a memory cap, and a nib-side lexer
// that re-implements enough of the object-stream reader to spot the shape. The first is a standing
// fork to re-apply on every bump; the second changes nib's process model on three platforms; the
// third is a second implementation that only helps while it agrees with the first. pdfcpu is
// already in this binary, already reads every document nib opens, and is a genuine independent
// implementation rather than a re-derivation of the one it guards.
//
// # Measured, because a guard that disagrees with what it guards is the whole risk
//
// Over all 7,438 single-bit flips of a signed fixture (every offset, both `0x01` and `0x80`):
// pdfcpu returned a verdict on **7,438 of 7,438** — it neither crashed nor hung on the corpus that
// kills the library it guards; digitorus died on **12**, and pdfcpu refused **all 12**. The cell
// that would matter — pdfcpu refusing a document digitorus reports **valid** — was **empty (0)**.
// The only behaviour change on that corpus was 10 flips moving `unsigned` → `invalid`, which is the
// direction `scanForSignatureBlob` already argues for.
//
// Both of the library's unbounded paths are covered, and the second was not in the original
// finding: a same-length payload of `(aaa…` in an object stream whose `/Filter` name is damaged
// reaches `readLiteralString` and OOMs, and `<444…` reaches `readHexString`, which **spins** — a
// hang, not a crash, killed by a watchdog at 90 s. pdfcpu refuses both with
// `decodeObjectStreamObjects: problem decoding object stream 7`.
//
// # Cost
//
// Measured 0.15–0.31× the cost of the `Verify` call it precedes, over 3.7 KB (1 page) to 94 KB
// (400 pages) — that is the measured range, and it is not extrapolated past it. It is **zero** for
// an unsigned document, because `scanForSignatureBlob` answers first and returns.
//
// # What it does not cover, stated rather than implied
//
//   - **A document pdfcpu refuses and digitorus would have verified is badged `Invalid`.** Zero
//     such cases in the corpus above, and encryption — the obvious candidate, since the two
//     libraries support different schemes — was measured separately across six shapes (AES-256,
//     AES-128 and RC4-128/40, owner-only and with a user password): pdfcpu is at least as
//     permissive as digitorus in **every** one, and the only shape digitorus reads (RC4-40,
//     owner-only) pdfcpu reads too. Residue: a real-world signed document from another tool that
//     pdfcpu alone refuses. That is a false "modified" badge, not a crash.
//   - **A bump of either library can move the line.** The guard below is what fails if it does.
//
// The `recover` is defence in depth: pdfcpu did not panic once in 7,438 flips, but a panic here
// would become the crash this exists to prevent.
func pdfcpuCanRead(pdf []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("read pdf: %v", r)
		}
	}()
	// **Relaxed is pinned rather than inherited.** `model.NewDefaultConfiguration()` returns the
	// user's `config.yml` when one exists, so a machine configured for strict validation would
	// silently give this gate a stricter rule than the one measured — and a gate that is stricter
	// than measured refuses documents nib can actually read. The loosest read pdfcpu offers is the
	// only one that belongs here: the question is "can this be parsed at all", never "is it valid".
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	_, err = api.ReadContext(bytes.NewReader(pdf), conf)
	return err
}

// scanForSignatureBlob answers the narrow question without a parser.
//
// **It exists because the parser is the thing that fails** (/pending 453). Every signed PDF carries
// a signature dictionary with a `/ByteRange` array — `pdfsign` writes one, and it is required by
// the spec for any signature the verifier could have checked. A byte scan for that token allocates
// nothing, follows no references and cannot panic, so it is a safe answer on exactly the documents
// where the full read is not.
//
// **It over-reports rather than under-reports, deliberately.** A file containing the literal bytes
// `/ByteRange` in some unrelated place is called "carries a blob", routing it to `Invalid` instead
// of `Unsigned`. That is the safe direction: telling a user a broken document may carry a signature
// costs them a look, where telling them a tampered signed document was never signed costs them the
// thing they were relying on.
func scanForSignatureBlob(pdf []byte) bool {
	return bytes.Contains(pdf, []byte("/ByteRange"))
}

// signerInfo projects a pdfsign verify.Signer onto the integrity-focused subset
// Nib surfaces. Time backing is derived from which time the signature actually
// carries — an independent RFC3161 timestamp token (TSA) versus a signer-
// supplied /M time (self-asserted) — NOT from the library's TimeSource field:
// under our default (secure) verify options that field reports "current_time"
// whenever no timestamp token is present, because the library refuses to trust
// signer-supplied time.
//
// **Token presence is NOT the signal for "independent", and was read as one until /pending 708.**
// `timestamp.Parse` checks a token only against the certificates inside it, so anyone can mint a
// token with any date. `TimestampTrusted` is the library's answer to whether the token's authority
// chains, as a timestamping certificate, to a system-trusted root (our options leave
// `ValidateTimestampCertificates` on and `AllowUntrustedRoots` off); only that is TSA.
func signerInfo(s *verify.Signer, fingerprint string) SignerInfo {
	const layout = "2006-01-02 15:04 MST"
	si := SignerInfo{Name: s.Name, Valid: s.ValidSignature, Reason: s.Reason}
	// **The fingerprint comes from the certificate the SignerInfo NAMES, never from the bag's
	// order** (ADR-051, /pending 613), and it is the record's at this signer's position — the
	// record is its one home (ADR-058). This read `s.Certificates[0]` once, under a comment
	// reasoning that "Nib identities are self-signed single certs, so element 0 is the signer" —
	// true of documents nib produced, and a statement about nothing at all for a document that
	// arrived from a peer. An empty answer means nib could not establish who signed; every
	// consumer treats that as unrecognised.
	si.Fingerprint = fingerprint
	switch {
	case s.TimeStamp != nil && !s.TimeStamp.Time.IsZero():
		si.TimeBacking = TSAUnverified
		if s.TimestampTrusted {
			si.TimeBacking = TSA
		}
		si.When = s.TimeStamp.Time.UTC().Format(layout)
	case s.SignatureTime != nil:
		si.TimeBacking = SelfAsserted
		si.When = s.SignatureTime.UTC().Format(layout)
	default:
		si.TimeBacking = NoTime
	}
	return si
}
