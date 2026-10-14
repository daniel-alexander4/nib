package ceremony

import (
	"errors"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// TestADocumentWhoseFilesCouldNotBeReadIsNotAnOrdinaryPDF is ADR-164 at the ceremony's readers (`/pending 880`).
// The record is an embedded file. Where pdfcpu's validator dropped the tree it sits in, `Extract` answered
// ErrNoRecord — which every gate reads as "an ordinary PDF" — for a document that carries one.
func TestADocumentWhoseFilesCouldNotBeReadIsNotAnOrdinaryPDF(t *testing.T) {
	doc := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: 3, Loop: true, Key: AttachmentName})
	_, err := Extract(doc)
	if errors.Is(err, ErrNoRecord) || !errors.Is(err, ErrRecordUnreadable) {
		t.Errorf("Extract answered %v — want ErrRecordUnreadable, and never ErrNoRecord", err)
	}
	if _, err := CheckRecord(doc, time.Now()); errors.Is(err, ErrNoRecord) || !errors.Is(err, ErrRecordUnreadable) {
		t.Errorf("CheckRecord answered %v — want ErrRecordUnreadable, and never ErrNoRecord", err)
	}
	// A record at the bottom of a tree deeper than the validator follows is FOUND: it is read as a record (and
	// refused as one that does not decode), not passed over.
	deep := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: 150, Key: AttachmentName})
	if _, err := Extract(deep); err == nil || errors.Is(err, ErrNoRecord) || errors.Is(err, ErrRecordUnreadable) {
		t.Errorf("Extract of a record in a deep tree answered %v — want the record read and refused as malformed", err)
	}
}
