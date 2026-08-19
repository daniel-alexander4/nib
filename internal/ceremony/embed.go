package ceremony

import (
	"errors"
	"fmt"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// AttachmentName is the embedded file the record travels in.
//
// A PDF attachment rather than metadata or a custom dictionary, and the choice was
// **measured rather than reasoned** (caveat 10, discharged 2026-08-18): an attachment added
// in the pre-signing structural pass survives three incremental signatures byte-identical,
// and attaching one *after* a signature invalidates every signature on the document. That
// second half is why PrepareDocument is the only place a record can be embedded.
const AttachmentName = "nib-ceremony.json"

// ErrNoRecord is returned when a document carries no ceremony record. Distinct from a
// malformed one: an ordinary PDF has no record and that is not an error, while a document
// that has one which will not parse is a broken ceremony.
var ErrNoRecord = errors.New("document carries no ceremony record")

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
// pdfops.ContentDigest is stable where that is not: measured identical across adding the
// attachment and across three incremental signatures. That is what makes the hop-4 clause
// buildable at all.
//
// **The narrowing, stated rather than discovered:** it covers the page count and every
// page's content stream, and NOT annotations, form values, attachments or metadata. It
// answers "is this the same document" about what is on the pages, which is what a ceremony
// is convened over; everything else is covered by the signatures, which hash the real bytes
// and flip to invalid on any edit. See the plan pin on D20.
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
// It answers three questions in the order they matter: is there a record, does its
// convener signature verify, and does the document it arrived in still hash to what the
// record says. The third is the one a later party in the chain can only answer for
// itself — the convener's own bytes satisfy a round-trip test without anyone recomputing
// anything.
func CheckDocument(pdf []byte) (Record, error) {
	r, err := Extract(pdf)
	if err != nil {
		return Record{}, err
	}
	if err := r.Verify(); err != nil {
		return r, err
	}
	got, err := DocumentHash(pdf)
	if err != nil {
		return r, err
	}
	if got != r.DocHash {
		return r, fmt.Errorf("the document does not match the ceremony record: it hashes to %s "+
			"and the record was written for %s — these are not the same document", got[:16], r.DocHash[:16])
	}
	return r, nil
}
