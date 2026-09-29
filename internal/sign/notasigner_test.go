package sign

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdfsign/verify"

	"nib/internal/testpdf"
)

// P01.S03 (ADR-060): a signer is a record that is well-formed AND not a document timestamp, and
// `State` is theirs. Every case below asserts its STIMULUS through the library alone first — what
// the library enumerates is what `Signers` used to be — so an outcome that differs from it is this
// slice's work and not a fixture that never carried the shape.

// libraryCount is the library's own enumeration of doc: how many signers, and how many it verified.
func libraryCount(t *testing.T, doc []byte) (n, valid int) {
	t.Helper()
	resp, err := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("STIMULUS: the library could not read the fixture: %v", err)
	}
	for _, s := range resp.Signers {
		if s.ValidSignature {
			valid++
		}
	}
	return len(resp.Signers), valid
}

func aliceSigned(t *testing.T) ([]byte, identity) {
	t.Helper()
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	signed, err := SignApproval(base, a.certPEM, a.keyPEM, Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return signed, a
}

// TestACopyTheLibraryFailsDoesNotMakeTheDocumentInvalid — the grill's third row. A copied dictionary
// whose ranges the library hashes and FAILS read `invalid` with two signers: the victim's real,
// verifying signature was reported as a tampered document by a record nib had already refused.
func TestACopyTheLibraryFailsDoesNotMakeTheDocumentInvalid(t *testing.T) {
	signed, a := aliceSigned(t)
	doc, obj, err := testpdf.CopiedSignatureDictionary(signed, testpdf.CopyLibraryFails)
	if err != nil {
		t.Fatal(err)
	}
	if n, valid := libraryCount(t, doc); n != 2 || valid != 1 {
		t.Fatalf("STIMULUS: the library reports %d signer(s), %d valid; want 2 with the copy failing", n, valid)
	}
	st := Verify(doc)
	if st.State != Valid {
		t.Errorf("state %s, want valid — the failing signature is a refused copy, not a signer", st.State)
	}
	if len(st.Signers) != 1 || st.Signers[0].Fingerprint != a.fp {
		t.Errorf("signers %+v, want exactly Alice (%s)", st.Signers, a.fp)
	}
	if len(st.Refused) != 1 || st.Refused[0].Obj != uint32(obj) {
		t.Errorf("refused %+v, want object %d", st.Refused, obj)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterRefusedSignature {
		t.Errorf("addedAfter %v %q, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterRefusedSignature)
	}
}

// TestARefusedLaterSignerIsDroppedAndTheAppendIsReported — Alice then Bob, Bob's `/ByteRange` made
// odd in place (same length). The library fails Bob; the sweep refuses his record. He is not a
// signer, so the document is Alice's and valid — and what Bob's revision added is reported as an
// append nothing valid covers, with the refusal named. That is carry (a): a refused record can hide
// only a change past the last counted signer, and `AddedAfter` reports exactly that.
func TestARefusedLaterSignerIsDroppedAndTheAppendIsReported(t *testing.T) {
	signed, a := aliceSigned(t)
	b := newIdentity(t, "Bob")
	two, err := SignApproval(signed, b.certPEM, b.keyPEM, Options{Name: "Bob", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	brRe := regexp.MustCompile(`/ByteRange\s*\[([^\]]*)\]`)
	locs := brRe.FindAllSubmatchIndex(two, -1)
	l := locs[len(locs)-1]
	inner := string(two[l[2]:l[3]])
	f := strings.Fields(inner)
	last := f[len(f)-1]
	if len(last) < 3 {
		t.Fatalf("setup: Bob's last length %q is too short to split", last)
	}
	odd := strings.Replace(inner, " "+last, " "+last[:1]+" "+last[2:], 1)
	if len(odd) != len(inner) {
		t.Fatal("setup: the rewrite changed the array's length")
	}
	doc := append([]byte{}, two...)
	copy(doc[l[2]:l[3]], odd)
	if n, valid := libraryCount(t, doc); n != 2 || valid != 1 {
		t.Fatalf("STIMULUS: the library reports %d signer(s), %d valid; want 2 with Bob failing", n, valid)
	}
	st := Verify(doc)
	if st.State != Valid || len(st.Signers) != 1 || st.Signers[0].Fingerprint != a.fp {
		t.Errorf("state %s signers %+v, want valid with exactly Alice", st.State, st.Signers)
	}
	if len(st.Refused) != 1 || st.Refused[0].Cause != CauseMalformedByteRange {
		t.Errorf("refused %+v, want Bob's record, malformed-byterange", st.Refused)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterRefusedSignature {
		t.Errorf("addedAfter %v %q, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterRefusedSignature)
	}
}

// TestADocumentTimestampIsNotASigner — /pending 737. The library enumerates a document timestamp as
// a signer and fails it, so every B-LTA document read `invalid` with two signers. The stamp is
// listed by object in `Timestamps`, the signer is Alice, the document is valid, and the stamp's
// revision is still an append no signature covers (ADR-059).
func TestADocumentTimestampIsNotASigner(t *testing.T) {
	doc, alice := approvalPlusDocTimeStamp(t)
	if n, valid := libraryCount(t, doc); n != 2 || valid != 1 {
		t.Fatalf("STIMULUS: the library reports %d signer(s), %d valid; want 2 with the stamp failing", n, valid)
	}
	dts := timestampRecord(t, mustSweep(t, doc))
	st := Verify(doc)
	if st.State != Valid {
		t.Errorf("state %s, want valid — a B-LTA document's timestamp is not a failed signature", st.State)
	}
	if len(st.Signers) != 1 || st.Signers[0].Fingerprint != alice.fp {
		t.Errorf("signers %+v, want exactly Alice", st.Signers)
	}
	if len(st.Timestamps) != 1 || st.Timestamps[0] != dts.Obj {
		t.Errorf("timestamps %v, want [%d]", st.Timestamps, dts.Obj)
	}
	if len(st.Refused) != 0 {
		t.Errorf("refused %+v, want none — a timestamp is not refused", st.Refused)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterAppended {
		t.Errorf("addedAfter %v %q, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterAppended)
	}
}

// TestATimestampOnlyDocumentStaysInvalid — the grill's Default D1 and T04. No record counts as a
// signer, and a signature blob is present, so the document is `Invalid` (never `Unsigned`), the stamp
// is named, and `AddedAfter` warns `could-not-check`: the library saw a signature and nothing
// valid bounds coverage. Following the COUNTED signers into `addedAfter`'s fourth argument reads
// this "nothing added".
func TestATimestampOnlyDocumentStaysInvalid(t *testing.T) {
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	doc := withDocTimeStamp(t, base)
	if n, _ := libraryCount(t, doc); n != 1 {
		t.Fatalf("STIMULUS: the library reports %d signer(s), want the stamp as 1", n)
	}
	dts := timestampRecord(t, mustSweep(t, doc))
	st := Verify(doc)
	if st.State != Invalid || len(st.Signers) != 0 {
		t.Errorf("state %s signers %+v, want invalid with none", st.State, st.Signers)
	}
	if len(st.Timestamps) != 1 || st.Timestamps[0] != dts.Obj {
		t.Errorf("timestamps %v, want [%d]", st.Timestamps, dts.Obj)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("addedAfter %v %q, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterCouldNotCheck)
	}
}

// relabelTimestamp rewrites the LAST `/adbe.pkcs7.detached` in doc to `/ETSI.RFC3161` in place — 13
// bytes inside that signature's own coverage, so the signature it labels now fails its hash.
func relabelTimestamp(t *testing.T, doc []byte) []byte {
	t.Helper()
	const from, to = "/adbe.pkcs7.detached", "/ETSI.RFC3161"
	i := bytes.LastIndex(doc, []byte(from))
	if i < 0 {
		t.Fatalf("setup: no %s", from)
	}
	out := append([]byte{}, doc...)
	copy(out[i:], to+strings.Repeat(" ", len(from)-len(to)))
	return out
}

// TestASoleSignatureRelabelledATimestampIsNotUnsigned — T02's red proof. The only signature's
// `/SubFilter` rewritten in place to `/ETSI.RFC3161` is a label and nothing more: the blob is a
// detached signature and encapsulates no TSTInfo, so the record is NOT a timestamp — it is a signer,
// its hash fails on its own rewrite, and the document is `Invalid` with it listed as failed. "Never
// signed" is the unsafe answer, and so is "a document timestamp is present".
func TestASoleSignatureRelabelledATimestampIsNotUnsigned(t *testing.T) {
	signed, _ := aliceSigned(t)
	doc := relabelTimestamp(t, signed)
	if n, _ := libraryCount(t, doc); n != 1 {
		t.Fatalf("STIMULUS: the library reports %d signer(s), want 1", n)
	}
	revs := mustSweep(t, doc)
	if len(revs) != 1 || revs[0].SubFilter != "ETSI.RFC3161" || revs[0].Cause != "" {
		t.Fatalf("STIMULUS: records %+v, want one well-formed record labelled ETSI.RFC3161", revs)
	}
	if revs[0].Timestamp {
		t.Errorf("record read as a timestamp by its label alone — its blob encapsulates nothing")
	}
	st := Verify(doc)
	if st.State != Invalid || len(st.Signers) != 1 || st.Signers[0].Valid || st.Signers[0].Name != "Alice" {
		t.Errorf("state %s signers %+v, want invalid with Alice listed as failed", st.State, st.Signers)
	}
	if len(st.Timestamps) != 0 {
		t.Errorf("timestamps %v, want none — a relabelled signature is not a timestamp", st.Timestamps)
	}
}

// TestARelabelledFailedSignatureStillFailsTheDocument — the P01.S03 review's relabel shape. Bob's
// signature tampered inside its own coverage reads `invalid`; relabelling it `/ETSI.RFC3161` as well
// (13 more bytes inside the same coverage) read `valid`, signers [Alice], and "a document timestamp is
// present" — the relabel hid the failure. A label is not evidence: the blob encapsulates nothing, so
// Bob is still a signer and still failed.
func TestARelabelledFailedSignatureStillFailsTheDocument(t *testing.T) {
	signed, a := aliceSigned(t)
	b := newIdentity(t, "Bob")
	two, err := SignApproval(signed, b.certPEM, b.keyPEM, Options{Name: "Bob", Reason: "I agree to pay 100", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	tam := bytes.Replace(two, []byte("pay 100"), []byte("pay 900"), 1)
	if bytes.Equal(tam, two) {
		t.Fatal("setup: no reason text to tamper")
	}
	doc := relabelTimestamp(t, tam)
	if n, valid := libraryCount(t, doc); n != 2 || valid != 1 {
		t.Fatalf("STIMULUS: the library reports %d signer(s), %d valid; want 2 with Bob failing", n, valid)
	}
	st := Verify(doc)
	if st.State != Invalid {
		t.Errorf("state %s, want invalid — relabelling a failed signature hid its failure", st.State)
	}
	if len(st.Signers) != 2 || st.Signers[0].Fingerprint != a.fp || !st.Signers[0].Valid ||
		st.Signers[1].Name != "Bob" || st.Signers[1].Valid {
		t.Errorf("signers %+v, want Alice valid and Bob listed as failed", st.Signers)
	}
	if len(st.Timestamps) != 0 {
		t.Errorf("timestamps %v, want none", st.Timestamps)
	}
}

// TestARefusedOnlyDocumentWarnsItCouldNotCheck — `addedAfter`'s fourth argument is the LIBRARY's
// count (ADR-060). A sole signature whose `/ByteRange` is made odd in place is refused, so no record
// counts and none bounds; following the counted signers into that argument reads this "nothing
// added" — a document no valid signature covers at all, reported as wholly signed.
func TestARefusedOnlyDocumentWarnsItCouldNotCheck(t *testing.T) {
	signed, _ := aliceSigned(t)
	brRe := regexp.MustCompile(`/ByteRange\s*\[([^\]]*)\]`)
	l := brRe.FindSubmatchIndex(signed)
	if l == nil {
		t.Fatal("setup: no /ByteRange")
	}
	inner := string(signed[l[2]:l[3]])
	f := strings.Fields(inner)
	last := f[len(f)-1]
	if len(last) < 3 {
		t.Fatalf("setup: last length %q is too short to split", last)
	}
	odd := strings.Replace(inner, " "+last, " "+last[:1]+" "+last[2:], 1)
	if len(odd) != len(inner) {
		t.Fatal("setup: the rewrite changed the array's length")
	}
	doc := append([]byte{}, signed...)
	copy(doc[l[2]:l[3]], odd)
	if n, _ := libraryCount(t, doc); n != 1 {
		t.Fatalf("STIMULUS: the library reports %d signer(s), want 1", n)
	}
	st := Verify(doc)
	if len(st.Signers) != 0 || len(st.Refused) != 1 || st.Refused[0].Cause != CauseMalformedByteRange {
		t.Fatalf("STIMULUS: signers %+v refused %+v, want none counted and the one record refused", st.Signers, st.Refused)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("addedAfter %v %q, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterCouldNotCheck)
	}
}

// TestTimestampsListsOnlyUnrefusedClaims — `Status.Timestamps` names a stamp once and only where it
// claims something: a refused one is already in `Refused` (listing it twice would count one object
// as two facts), and a placeholder with an empty `/Contents` claims nothing, as `refusedOf` holds.
func TestTimestampsListsOnlyUnrefusedClaims(t *testing.T) {
	revs := []Revision{
		{Obj: 1, Timestamp: true, hasContents: true},
		{Obj: 2, Timestamp: true, hasContents: true, Cause: CauseMalformedByteRange},
		{Obj: 3, Timestamp: true},
		{Obj: 4, hasContents: true},
	}
	// STIMULUS: the refused stamp IS published as refused, so a second listing would be a duplicate.
	if r := refusedOf(revs); len(r) != 1 || r[0].Obj != 2 {
		t.Fatalf("STIMULUS: refused %+v, want object 2", r)
	}
	if got := timestampsOf(revs); len(got) != 1 || got[0] != 1 {
		t.Errorf("timestamps %v, want [1] — not the refused stamp, the placeholder or the signature", got)
	}
}
