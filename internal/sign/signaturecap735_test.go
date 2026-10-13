package sign

import (
	"errors"
	"fmt"
	"testing"
)

// manyRecords appends n signature dictionaries to an honest one-signature document, every one
// naming `[0 10 20 30]` and sharing ONE `/Contents` string by reference (~110 bytes a record).
// With `parses` the shared string is a real PKCS#7, so the library processes each record; without
// it the string is two bytes of nothing, and the library drops each before reading a range.
func manyRecords(t *testing.T, id identity, n int, parses bool) []byte {
	t.Helper()
	prev := synthSigned(t, id)
	blob := "<0000>"
	if parses {
		blob = "<" + fmt.Sprintf("%x", detached(t, id)([]byte("x"))) + ">"
	}
	objs := []sobj{{num: 900, body: blob}}
	for k := 0; k < n; k++ {
		objs = append(objs, sobj{num: 1000 + k, body: "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange [0 10 20 30] /Contents 900 0 R >>"})
	}
	return synthRevision(t, prev, objs, 1)
}

// TestADocumentWithMoreSignaturesThanNibChecksSaysSo — /pending 735, ADR-141. The library hashes
// every record's ranges, and nothing bounded how many records it was handed: 2,000 of them on a
// 2.2 MB file cost 19 s a verification, still `valid`. Past `maxLibrarySignatures` the document is
// refused before the library, and it says the count was why.
func TestADocumentWithMoreSignaturesThanNibChecksSaysSo(t *testing.T) {
	a := newIdentity(t, "Alice")

	// At the ceiling: the honest signature and 63 more records the library processes.
	at := manyRecords(t, a, maxLibrarySignatures-1, true)
	if n := libraryRecords(mustSweep(t, at)); n != maxLibrarySignatures {
		t.Fatalf("stimulus: the library would process %d records, want exactly %d", n, maxLibrarySignatures)
	}
	st := Verify(at)
	if st.Unchecked == UncheckedTooManySignatures {
		t.Errorf("a document with exactly %d signatures was refused as carrying too many", maxLibrarySignatures)
	}
	if len(st.Signers) == 0 {
		t.Errorf("a document with exactly %d signatures lists no signer: state %q, unchecked %q", maxLibrarySignatures, st.State, st.Unchecked)
	}
	if _, err := Revisions(at); errors.Is(err, errTooManySignatures) {
		t.Errorf("Revisions refused a document at the ceiling: %v", err)
	}

	// One past it.
	over := manyRecords(t, a, maxLibrarySignatures, true)
	if n := libraryRecords(mustSweep(t, over)); n != maxLibrarySignatures+1 {
		t.Fatalf("stimulus: the library would process %d records, want exactly %d", n, maxLibrarySignatures+1)
	}
	st = Verify(over)
	if st.State != Invalid || st.Unchecked != UncheckedTooManySignatures {
		t.Errorf("one signature past the ceiling: state %q, unchecked %q, want %q and %q",
			st.State, st.Unchecked, Invalid, UncheckedTooManySignatures)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("one signature past the ceiling: addedAfter=%v cause=%q, want true and %q — nothing measured the coverage",
			st.AddedAfter, st.AddedAfterCause, AddedAfterCouldNotCheck)
	}
	if len(st.Signers) != 0 {
		t.Errorf("one signature past the ceiling lists %d signer(s), and none was checked", len(st.Signers))
	}
	if _, err := Revisions(over); !errors.Is(err, errTooManySignatures) {
		t.Errorf("Revisions on a document past the ceiling: err %v, want errTooManySignatures", err)
	}

	// The count is of records the LIBRARY processes. A hundred dictionaries whose blob does not
	// parse cost it nothing, and they must not take the one honest signature's verdict away.
	inert := manyRecords(t, a, 100, false)
	// Counted here and not by `libraryRecords`, which is what is under test.
	revs, forLibrary := mustSweep(t, inert), 0
	for _, r := range revs {
		if r.libPos >= 0 {
			forLibrary++
		}
	}
	if len(revs) <= maxLibrarySignatures || forLibrary != 1 {
		t.Fatalf("stimulus: %d records swept and %d for the library, want more than %d and exactly 1",
			len(revs), forLibrary, maxLibrarySignatures)
	}
	st = Verify(inert)
	if st.Unchecked == UncheckedTooManySignatures {
		t.Errorf("records the library never processes were counted against the ceiling")
	}
	if len(st.Signers) != 1 || !st.Signers[0].Valid || st.Signers[0].Fingerprint != a.fp {
		t.Errorf("the honest signature beside 100 inert records reads %+v, want Alice's, valid", st.Signers)
	}
}
