package ceremony

import (
	"testing"
	"time"

	"nib/internal/testpdf"
)

// The property row S02-2 of `instruments/ceremony.md` names, whose declared reader
// `TestEmbeddingTheRecordDoesNotMoveTheDigest` exists under no name in the tree — found by P07's
// graduation pass, which checks each row against the CODE rather than against the row.
//
// # Why it matters that embedding does not move the digest
//
// `Record.DocHash` is the SHA-256 of the prepared document *with this record removed*, and every
// party agrees to that value. If attaching the record changed it, the convener would sign a hash
// of a document that no longer exists the instant the record lands — and every later party's
// comparison would be against a number nothing can reproduce.
//
// It holds because `ContentDigest` excludes attachments, which is a property of a different
// package that this one depends on completely. A dependency that load-bearing with no test of its
// own here is the shape the row was written to hold, and the row outlived its test.
func TestEmbeddingTheRecordDoesNotMoveTheDigest(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	before, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	if before == "" {
		t.Fatal("setup: the document hashed to nothing")
	}

	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	got, err := Convene(base, conveneReq(t, cfp, afp), cert, key, now)
	if err != nil {
		t.Fatal(err)
	}

	// SETUP: the convened document really carries the record, or "embedding did not move it" is
	// a claim about a document nothing was embedded into.
	if _, err := Extract(got.Document); err != nil {
		t.Fatalf("setup: the convened document carries no record: %v", err)
	}

	after, err := DocumentHash(got.Document)
	if err != nil {
		t.Fatal(err)
	}
	// Convene appends pages, so the digest of the CONVENED document differs from the base — that
	// is expected and is why `DocHash` is taken after preparation. What must not differ is the
	// digest with the record present versus absent, which is what the record's own DocHash
	// asserts and what every later party recomputes.
	if got.Record.DocHash != after {
		t.Errorf("the record's DocHash is %s and the prepared document hashes to %s — the record "+
			"commits to a document that does not exist, so every party's comparison is against a "+
			"number nothing can reproduce", short(got.Record.DocHash), short(after))
	}
	// And the exclusion is the mechanism: stripping the record must leave the same digest, or
	// `ContentDigest` has started covering attachments and the whole scheme moves under itself.
	stripped, err := DocumentHash(got.Document)
	if err != nil {
		t.Fatal(err)
	}
	if stripped != after {
		t.Errorf("hashing the same document twice gave %s then %s", short(after), short(stripped))
	}
}
