package sign

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/digitorus/pdfsign/verify"
	"github.com/digitorus/timestamp"

	"nib/internal/testpdf"
)

// ---------------------------------------------------------------------------------------------
// P01.S02 — `AddedAfter` over verified, well-formed revisions (ADR-059).
// ---------------------------------------------------------------------------------------------

// withDocTimeStamp is testpdf.DocTimeStamped, failing the test on a fixture error.
func withDocTimeStamp(t *testing.T, pdf []byte) []byte {
	t.Helper()
	out, err := testpdf.DocTimeStamped(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// approvalPlusDocTimeStamp is a one-signer approval signature with a document timestamp after it.
func approvalPlusDocTimeStamp(t *testing.T) (doc []byte, alice identity) {
	t.Helper()
	base, err := testpdf.Text("the lease")
	if err != nil {
		t.Fatal(err)
	}
	alice = newIdentity(t, "Alice")
	signed, err := SignApproval(base, alice.certPEM, alice.keyPEM, Options{Name: "Alice", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return withDocTimeStamp(t, signed), alice
}

// imprintMatches is the test's own oracle for a stamp's imprint — production never reads it on the
// verdict path (ADR-059): the token read from the gap's bytes, hashed over the raw /ByteRange.
func imprintMatches(t *testing.T, doc []byte, rec *Revision) bool {
	t.Helper()
	ts, err := timestamp.Parse(mustHex(t, doc[rec.gapStart+1:rec.gapEnd-1]))
	if err != nil {
		return false
	}
	h := ts.HashAlgorithm.New()
	br := rec.ByteRange
	for i := 0; i+1 < len(br); i += 2 {
		h.Write(doc[br[i] : br[i]+br[i+1]])
	}
	return bytes.Equal(h.Sum(nil), ts.HashedMessage)
}

func timestampRecord(t *testing.T, revs []Revision) *Revision {
	t.Helper()
	var at *Revision
	for i := range revs {
		if revs[i].Type == "DocTimeStamp" {
			if at != nil {
				t.Fatal("two document timestamps")
			}
			at = &revs[i]
		}
	}
	if at == nil {
		t.Fatal("no document timestamp record")
	}
	return at
}

// TestADocTimeStampNeverBoundsCoverage — ADR-059, the grill's amendment of the plan-review pin. A
// document timestamp names no signer and a public authority stamps any hash, so "its imprint
// matches" must never let it end coverage: an honest B-LTA document reads `AddedAfter=true,
// appended` — accurate, the stamp's revision WAS added after the last signature — and the stamp is
// not refused.
func TestADocTimeStampNeverBoundsCoverage(t *testing.T) {
	doc, alice := approvalPlusDocTimeStamp(t)
	revs := mustSweep(t, doc)
	dts := timestampRecord(t, revs)
	// STIMULUS: the stamp is well-formed, its imprint IS the hash of its ranges, and its coverage
	// reaches the end of the file — everything that would bound coverage if a stamp could.
	br := dts.ByteRange
	if dts.conjunct != 0 || dts.SubFilter != "ETSI.RFC3161" || !imprintMatches(t, doc, dts) || br[len(br)-2]+br[len(br)-1] != int64(len(doc)) {
		t.Fatalf("STIMULUS: stamp conjunct %d subfilter %q imprint %v byte range %v of %d",
			dts.conjunct, dts.SubFilter, imprintMatches(t, doc, dts), br, len(doc))
	}
	st := Verify(doc)
	if len(st.Signers) == 0 || st.Signers[0].Fingerprint != alice.fp {
		t.Fatalf("STIMULUS: Alice's approval is not the first signer: %+v", st.Signers)
	}
	if dts.Cause != "" {
		t.Errorf("a stamp whose imprint matches was refused: %q", dts.Cause)
	}
	if len(st.Refused) != 0 {
		t.Errorf("refused %+v, want none", st.Refused)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterAppended {
		t.Errorf("addedAfter %v cause %q, want true %q — the stamp's revision is covered by no signature", st.AddedAfter, st.AddedAfterCause, AddedAfterAppended)
	}
}

// TestAVerifiedDocTimeStampStillBoundsNothing — the library reads a stamp `ValidSignature=false`
// today, so through `Verify` the rule above is also met by `Verified`. This drives `coverage` with the
// stamp marked verified — as a library that learned to verify stamps would mark it — so the
// timestamp conjunct of `bounds` carries the rule on its own.
func TestAVerifiedDocTimeStampStillBoundsNothing(t *testing.T) {
	doc, _ := approvalPlusDocTimeStamp(t)
	revs, err := Revisions(doc)
	if err != nil {
		t.Fatal(err)
	}
	dts := timestampRecord(t, revs)
	dts.Verified = true
	// STIMULUS: the stamp is now verified, well-formed, and reaches EOF.
	if dts.Cause != "" || dts.CoverageEnd != int64(len(doc)) {
		t.Fatalf("STIMULUS: stamp cause %q end %d of %d", dts.Cause, dts.CoverageEnd, len(doc))
	}
	if trailing, saw := coverage(revs, len(doc)); !trailing || !saw {
		t.Errorf("coverage trailing %v saw %v with a verified stamp at EOF, want trailing: a document timestamp bounded coverage", trailing, saw)
	}
}

// TestADocTimeStampIsNeverRefusedForItsImprint — ADR-059 as the P01.S02 review amended it: the
// imprint is not checked on the verdict path (it cost 11% of `Verify` at 10 MB for one stamp, and
// `timestamp.Parse` refuses an honest RSASSA-PSS token), so a stamp whose imprint does NOT match its
// ranges is still a timestamp, still bounds nothing, and is not a refused record. The dispute surface
// checks the imprint on demand (D10).
func TestADocTimeStampIsNeverRefusedForItsImprint(t *testing.T) {
	doc, _ := approvalPlusDocTimeStamp(t)
	dts := timestampRecord(t, mustSweep(t, doc))
	ts, err := timestamp.Parse(mustHex(t, doc[dts.gapStart+1:dts.gapEnd-1]))
	if err != nil {
		t.Fatal(err)
	}
	// Flip one hex digit of the imprint inside the token. The token sits in the gap, so no signature's
	// ranges change; the DER stays parseable.
	at := -1
	for _, enc := range []string{hex.EncodeToString(ts.HashedMessage), strings.ToUpper(hex.EncodeToString(ts.HashedMessage))} {
		if i := bytes.Index(doc[dts.gapStart:dts.gapEnd], []byte(enc)); i >= 0 {
			at = int(dts.gapStart) + i
		}
	}
	if at < 0 {
		t.Fatal("the imprint is not in the token")
	}
	bad := append([]byte(nil), doc...)
	if bad[at] == '0' {
		bad[at] = '1'
	} else {
		bad[at] = '0'
	}
	rec := timestampRecord(t, mustSweep(t, bad))
	// STIMULUS: the tampered stamp passes the structure rule and the library reads it, and its
	// imprint is NOT the hash of its ranges.
	if rec.conjunct != 0 || rec.libPos < 0 || rec.SubFilter != "ETSI.RFC3161" || imprintMatches(t, bad, rec) {
		t.Fatalf("STIMULUS: tampered stamp conjunct %d libPos %d subfilter %q imprint matches %v",
			rec.conjunct, rec.libPos, rec.SubFilter, imprintMatches(t, bad, rec))
	}
	if !rec.Timestamp || rec.Cause != "" || rec.bounds() {
		t.Errorf("tampered stamp timestamp %v cause %q bounds %v, want a timestamp, not refused, bounding nothing", rec.Timestamp, rec.Cause, rec.bounds())
	}
	st := Verify(bad)
	if len(st.Refused) != 0 {
		t.Errorf("refused %+v, want none — the verdict path does not judge a stamp's imprint", st.Refused)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterAppended {
		t.Errorf("addedAfter %v cause %q, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterAppended)
	}
}

func mustHex(t *testing.T, b []byte) []byte {
	t.Helper()
	var clean []byte
	for _, c := range b {
		if isHexDigit(c) {
			clean = append(clean, c)
		}
	}
	if len(clean)%2 == 1 {
		clean = append(clean, '0')
	}
	out, err := hex.DecodeString(string(clean))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// kidsNested is a document whose only signature sits under a parent field's `/Kids` (which the
// `/Fields` walk once could not see, and since the P08 phase-close review does), carrying `blob` as
// its `/Contents`.
func kidsNested(t *testing.T, sig string, blob func([]byte) []byte) []byte {
	t.Helper()
	objs := baseObjs(sig)
	objs[3] = sobj{num: 4, body: "<</T(parent)/Kids[6 0 R]>>"}
	objs = append(objs, sobj{num: 6, body: "<</FT/Sig/T(child)/Parent 4 0 R/V 5 0 R>>"})
	doc := synthRevision(t, nil, objs, 1)
	if blob == nil {
		return doc
	}
	return fillSig(t, doc, "1", nil, blob)
}

// TestANestedSignatureThatFailsIsNotUnsigned — the zero-signer rule (grill pin). A `/Kids`-nested
// signature whose PKCS#7 does not parse: the library drops it, and `signatureBlobPresent` walked
// `/Fields`' top level and could not see it, so the document read `Unsigned` — "never signed" — while
// `Revisions` held its record. The sweep's non-empty `/Contents` makes it `Invalid` on its own; the
// walk now descends `/Kids` too (the P08 phase-close review), and each is asked separately here.
func TestANestedSignatureThatFailsIsNotUnsigned(t *testing.T) {
	doc := kidsNested(t, sigDict("1", ""), func([]byte) []byte { return bytes.Repeat([]byte{0x30, 0x01}, 40) })
	resp, lerr := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
	revs := mustSweep(t, doc)
	rec := recordFor(t, revs, 5)
	// STIMULUS: the library reports no signer, and the record carries a non-empty /Contents it could
	// not parse — so the sweep, independently of the /Fields walk, sees a checkable blob.
	if (lerr == nil && resp != nil && len(resp.Signers) != 0) || !rec.hasContents || rec.Cause != CauseUnparseableContents || !anyCheckableBlob(revs) {
		t.Fatalf("STIMULUS: library err %v, record contents %v cause %q, sweep blob %v", lerr, rec.hasContents, rec.Cause, anyCheckableBlob(revs))
	}
	if !signatureBlobPresent(doc) {
		t.Errorf("the /Fields walk does not see the nested blob — every gate that asks it alone reads the document unsigned")
	}
	st := Verify(doc)
	if st.State != Invalid {
		t.Errorf("state %s, want invalid: a signature nib could not parse is not an unsigned document", st.State)
	}
	if len(st.Refused) != 1 || st.Refused[0].Obj != 5 || st.Refused[0].Cause != CauseUnparseableContents {
		t.Errorf("refused %+v, want object 5 %q — the zero-signer path must still name what it refused", st.Refused, CauseUnparseableContents)
	}
}

// TestAnUnlistedSignatureThatFailsIsNotUnsigned — the zero-signer rule held by the SWEEP alone (the
// review of the P08 phase-close fix): a signature field nothing references — not in `/Fields`, not in
// any `/Kids` — is invisible to the `/Fields` walk however deep it goes, so `Invalid` can only come
// from the sweep's checkable blob. Without that disjunct the document reads `Unsigned` again.
func TestAnUnlistedSignatureThatFailsIsNotUnsigned(t *testing.T) {
	objs := baseObjs(sigDict("1", ""))
	objs[3] = sobj{num: 4, body: "<</T(parent)>>"}
	objs = append(objs, sobj{num: 6, body: "<</FT/Sig/T(child)/V 5 0 R>>"})
	doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, func([]byte) []byte { return bytes.Repeat([]byte{0x30, 0x01}, 40) })
	revs := mustSweep(t, doc)
	// STIMULUS: the walk cannot see it, the sweep can.
	if signatureBlobPresent(doc) || !anyCheckableBlob(revs) {
		t.Fatalf("STIMULUS: walk %v, sweep %v — want the walk blind and the sweep seeing it", signatureBlobPresent(doc), anyCheckableBlob(revs))
	}
	if st := Verify(doc); st.State != Invalid {
		t.Errorf("state %s, want invalid: an unlisted signature nib could not parse is not an unsigned document", st.State)
	}
}

// TestAPlaceholderSignatureStaysUnsigned — the other half of the zero-signer rule: "prepare for
// signing" leaves `/Type /Sig` with an EMPTY `/Contents`. It is a record, and the document is still
// unsigned — "any record" would have made it `Invalid`.
func TestAPlaceholderSignatureStaysUnsigned(t *testing.T) {
	doc := synthRevision(t, nil, baseObjs("<</Type/Sig/Filter/Adobe.PPKLite/ByteRange[0 0 0 0]/Contents<>>>"), 1)
	revs := mustSweep(t, doc)
	// STIMULUS: the placeholder IS a record, the document carries /ByteRange (so Verify reaches the
	// sweep rather than answering from the byte scan), and its /Contents is empty.
	if len(revs) != 1 || !bytes.Contains(doc, []byte("/Contents<>")) || !scanForSignatureBlob(doc) {
		t.Fatalf("STIMULUS: %d records, byte scan %v", len(revs), scanForSignatureBlob(doc))
	}
	st := Verify(doc)
	if st.State != Unsigned {
		t.Errorf("state %s, want unsigned: an empty placeholder is not a signature", st.State)
	}
	if st.AddedAfter {
		t.Errorf("an unsigned document warns added-after (cause %q): there is no signature to be added after", st.AddedAfterCause)
	}
	// The placeholder is refused by the sweep (its empty /Contents does not parse), and it is still
	// not published: a refused entry unhides the details button to say nothing true.
	if revs[0].Cause == "" {
		t.Fatalf("STIMULUS: the placeholder is not a refused record, so Refused below proves nothing")
	}
	if len(st.Refused) != 0 {
		t.Errorf("refused %+v, want none: a placeholder claims nothing", st.Refused)
	}
}

// TestAUsageRightsSignatureWithoutSigFlagsStaysUnsigned — the P01.S02 review. A Reader-extended
// form carries an intact `/Perms /UR3` signature and no `/SigFlags`, so the library never enumerates
// it and `/Fields` does not list it. Before S02 it read `Unsigned`; counting every non-empty
// `/Contents` made it `Invalid` with no signer and no refusal. Both shapes: no AcroForm, and an
// AcroForm without `/SigFlags`.
func TestAUsageRightsSignatureWithoutSigFlagsStaysUnsigned(t *testing.T) {
	a := newIdentity(t, "Adobe UR")
	for _, cat := range []string{
		"<</Type/Catalog/Pages 2 0 R/Perms<</UR3 5 0 R>>>>",
		"<</Type/Catalog/Pages 2 0 R/Perms<</UR3 5 0 R>>/AcroForm<</Fields[4 0 R]>>>>",
	} {
		objs := baseObjs(sigDict("1", "/Reference[<</TransformMethod/UR3>>]"))
		objs[0].body = cat
		objs[3].body = "<</FT/Tx/T(name)>>"
		doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
		rec := recordFor(t, mustSweep(t, doc), 5)
		// STIMULUS: the record carries a blob, is well-formed, and is outside the library's
		// enumeration; the /Fields walk sees nothing.
		if !rec.hasContents || rec.Cause != "" || rec.libPos != -1 || signatureBlobPresent(doc) {
			t.Fatalf("%s: STIMULUS: contents %v cause %q libPos %d blob walk %v", cat, rec.hasContents, rec.Cause, rec.libPos, signatureBlobPresent(doc))
		}
		st := Verify(doc)
		if st.State != Unsigned || len(st.Signers) != 0 || len(st.Refused) != 0 || st.AddedAfter {
			t.Errorf("%s: state %s signers %d refused %+v addedAfter %v, want unsigned as before P01.S02", cat, st.State, len(st.Signers), st.Refused, st.AddedAfter)
		}
	}
}

// TestStrippingSigFlagsDoesNotUnsignADocument — the P01.S02 re-review: exempting every record
// the library never enumerates let an appended revision drop `/SigFlags` and turn a signed document
// `Unsigned`, hiding that it was ever signed. Only a signature the catalog's `/Perms` names is exempt.
func TestStrippingSigFlagsDoesNotUnsignADocument(t *testing.T) {
	a := newIdentity(t, "Alice")
	for _, cat := range []string{
		"<</Type/Catalog/Pages 2 0 R>>",                            // no AcroForm at all
		"<</Type/Catalog/Pages 2 0 R/AcroForm<</Fields[4 0 R]>>>>", // a form with no /SigFlags
	} {
		objs := baseObjs(sigDict("1", ""))
		objs[0].body = cat
		objs[3].body = "<</FT/Tx/T(name)>>"
		doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
		rec := recordFor(t, mustSweep(t, doc), 5)
		// STIMULUS: a well-formed signature blob the library never enumerates and /Perms does not name.
		if !rec.hasContents || rec.Cause != "" || rec.libPos != -1 || rec.underPerms {
			t.Fatalf("%s: STIMULUS: contents %v cause %q libPos %d perms %v", cat, rec.hasContents, rec.Cause, rec.libPos, rec.underPerms)
		}
		if st := Verify(doc); st.State != Invalid {
			t.Errorf("%s: state %s — a signed document read as never signed once /SigFlags was gone", cat, st.State)
		}
	}
}

// TestNoBoundingSignatureOutranksARefusal — cause precedence, the P01.S02 review. Where no valid
// signature bounds coverage nothing measured an append, so the cause is `could-not-check` even with a
// refused record present; `refused-signature-present` is read (and worded) as an append.
func TestNoBoundingSignatureOutranksARefusal(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc, decoy := appendListedDecoy661(t, synthSigned(t, a), "")
	i := bytes.Index(doc, []byte("/MediaBox[0 0 200 200]"))
	doc[i+len("/MediaBox[0 0 ")] = '3' // inside the signed ranges, same length
	st := Verify(doc)
	// STIMULUS: the one library signer failed, and the decoy is refused.
	if len(st.Signers) != 1 || st.Signers[0].Valid || len(st.Refused) != 1 || st.Refused[0].Obj != uint32(decoy) {
		t.Fatalf("STIMULUS: signers %+v refused %+v", st.Signers, st.Refused)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("addedAfter %v cause %q, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterCouldNotCheck)
	}
}

// TestARefusedRecordTheSignaturesCoverDoesNotWarn — `AddedAfter` is a coverage fact: a refused
// record inside the revision a valid signature covers leaves the bit false and its cause empty, and
// `Refused` still names it.
func TestARefusedRecordTheSignaturesCoverDoesNotWarn(t *testing.T) {
	a := newIdentity(t, "Alice")
	objs := baseObjs("<</Type/Sig/Filter/Adobe.PPKLite/ByteRange[0 1 2 3]/Contents<0102030405>>>")
	objs[3].body = "<</FT/Sig/T(Signature1)/V 6 0 R>>"
	objs = append(objs, sobj{num: 6, body: sigDict("1", "")})
	doc := fillSig(t, synthRevision(t, nil, objs, 1), "1", nil, detached(t, a))
	good := recordFor(t, mustSweep(t, doc), 6)
	// STIMULUS: the valid signature's byte range reaches the end of the file.
	if br := good.ByteRange; len(br) != 4 || br[2]+br[3] != int64(len(doc)) {
		t.Fatalf("STIMULUS: the valid signature's range %v does not reach EOF %d", good.ByteRange, len(doc))
	}
	st := Verify(doc)
	if st.AddedAfter || st.AddedAfterCause != "" {
		t.Errorf("addedAfter %v cause %q, want false and no cause", st.AddedAfter, st.AddedAfterCause)
	}
	if len(st.Refused) != 1 || st.Refused[0].Obj != 5 {
		t.Errorf("refused %+v, want object 5", st.Refused)
	}
}

// TestARefusedRecordNeverBounds — `bounds` needs a well-formed record even where the library
// verified it: a copied dictionary verifies as the victim. A refused record's `CoverageEnd` is left
// zero by the sweep, so this drives `coverage` with one that claims the whole file, which is what
// the conjunct must refuse whatever the sweep leaves in the field.
func TestARefusedRecordNeverBounds(t *testing.T) {
	revs := []Revision{
		{Obj: 5, Verified: true, CoverageEnd: 100},
		{Obj: 9, Verified: true, Cause: CauseContentsElsewhere, CoverageEnd: 200},
	}
	// STIMULUS: the refused record is verified and claims the whole file; the good one does not.
	if !revs[1].Verified || revs[1].CoverageEnd != 200 || revs[0].CoverageEnd >= 200 {
		t.Fatal("STIMULUS: the records are not the shape under test")
	}
	if trailing, saw := coverage(revs, 200); !trailing || !saw {
		t.Errorf("coverage trailing %v saw %v, want trailing: a refused record bounded coverage", trailing, saw)
	}
}

// TestAJoinErrorOutranksARefusal — cause precedence: where the join disagrees, nothing the records
// say is known to be about this document, so a refused record present is not the reason given.
func TestAJoinErrorOutranksARefusal(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc, _ := appendListedDecoy661(t, synthSigned(t, a), "")
	// STIMULUS: undisturbed, the refusal is the cause.
	if st := Verify(doc); st.AddedAfterCause != AddedAfterRefusedSignature || len(st.Refused) != 1 {
		t.Fatalf("STIMULUS: undisturbed cause %q refused %d", st.AddedAfterCause, len(st.Refused))
	}
	orig := libraryVerify
	libraryVerify = func(f io.ReaderAt, size int64) (*verify.Response, error) {
		resp, err := orig(f, size)
		if err == nil && resp != nil {
			resp.Signers = append(resp.Signers, resp.Signers[0])
		}
		return resp, err
	}
	t.Cleanup(func() { libraryVerify = orig })
	if st := Verify(doc); !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("addedAfter %v cause %q after the join disagreed, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterCouldNotCheck)
	}
}

// TestASoleFailedSignatureCannotBeChecked — the library reports a signer and no record bounds:
// there is no coverage end to measure against, so the cause is `could-not-check`, never `appended`.
func TestASoleFailedSignatureCannotBeChecked(t *testing.T) {
	a := newIdentity(t, "Alice")
	doc := synthSigned(t, a)
	i := bytes.Index(doc, []byte("/MediaBox[0 0 200 200]"))
	doc[i+len("/MediaBox[0 0 ")] = '3' // inside the signed ranges, same length
	resp, err := verify.Verify(bytes.NewReader(doc), int64(len(doc)))
	// STIMULUS: the library reports exactly one signer, and it failed; nothing is refused.
	if err != nil || len(resp.Signers) != 1 || resp.Signers[0].ValidSignature {
		t.Fatalf("STIMULUS: library err %v, want one failed signer", err)
	}
	st := Verify(doc)
	if len(st.Refused) != 0 {
		t.Fatalf("STIMULUS: refused %+v", st.Refused)
	}
	if !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
		t.Errorf("addedAfter %v cause %q, want true %q", st.AddedAfter, st.AddedAfterCause, AddedAfterCouldNotCheck)
	}
}

// TestEitherMarkMakesADocumentTimestamp — a record is a timestamp when it is MARKED (`/Type
// /DocTimeStamp` OR `/SubFilter /ETSI.RFC3161`) AND its blob ENCAPSULATES content: then it bounds
// nothing even verified, and is not refused for a token that is not an RFC 3161 one (ADR-059: the
// verdict path never reads the token). A mark over a DETACHED blob is a label and nothing more, and
// an encapsulating blob with no mark is an ordinary signature (ADR-060) — each is a signer, and
// bounds when verified.
func TestEitherMarkMakesADocumentTimestamp(t *testing.T) {
	a := newIdentity(t, "Alice")
	for _, tc := range []struct {
		name, typ, sub string
		marked         bool
	}{
		{"type only", "DocTimeStamp", "adbe.pkcs7.detached", true},
		{"subfilter only", "Sig", "ETSI.RFC3161", true},
		{"neither", "Sig", "adbe.pkcs7.detached", false},
	} {
		for _, blob := range []struct {
			name        string
			sign        func([]byte) []byte
			encapsulate bool
		}{
			{"encapsulating", encapsulating(t, a), true},
			{"detached", detached(t, a), false},
		} {
			dict := "<</Type/" + tc.typ + "/Filter/Adobe.PPKLite/SubFilter/" + tc.sub + "/ByteRange[" + brSlot("1", 64) +
				"]/Contents" + contentsSlot("1") + ">>"
			doc := fillSig(t, synthRevision(t, nil, baseObjs(dict), 1), "1", nil, blob.sign)
			rec := recordFor(t, mustSweep(t, doc), 5)
			// STIMULUS: the record is well-formed, and carries exactly one mark or none as the row says.
			typ, sub := rec.Type == "DocTimeStamp", rec.SubFilter == "ETSI.RFC3161"
			if rec.conjunct != 0 || (tc.marked && typ == sub) || (!tc.marked && (typ || sub)) {
				t.Fatalf("%s/%s: STIMULUS: conjunct %d type %q subfilter %q", tc.name, blob.name, rec.conjunct, rec.Type, rec.SubFilter)
			}
			stamp := tc.marked && blob.encapsulate
			rec.Verified = true
			if rec.Timestamp != stamp || rec.Cause != "" || rec.bounds() == stamp {
				t.Errorf("%s/%s: timestamp %v cause %q bounds-when-verified %v, want timestamp %v, unrefused, bounding %v",
					tc.name, blob.name, rec.Timestamp, rec.Cause, rec.bounds(), stamp, !stamp)
			}
		}
	}
}

// TestARefusedFilterIsCappedForItsReaders — `/Filter` is attacker-typed, so the published copy is
// capped at 64 bytes.
func TestARefusedFilterIsCappedForItsReaders(t *testing.T) {
	long := strings.Repeat("A", 200)
	got := refusedOf([]Revision{{Obj: 7, Filter: long, Cause: CauseUnsupportedFilter, hasContents: true}, {Obj: 8, hasContents: true},
		{Obj: 9, Cause: CauseUnparseableContents}})
	// STIMULUS: the record's filter is longer than the cap, one record is well-formed, and one is a
	// refused placeholder with no /Contents (never published).
	if len(long) <= maxRefusedFilter {
		t.Fatal("STIMULUS: the filter is not over the cap")
	}
	if len(got) != 1 || got[0].Obj != 7 || got[0].Cause != CauseUnsupportedFilter {
		t.Fatalf("refused %+v, want object 7 only", got)
	}
	if len(got[0].Filter) != maxRefusedFilter {
		t.Errorf("filter carried %d bytes, want %d", len(got[0].Filter), maxRefusedFilter)
	}
}

// TestEveryRefusalCauseIsSaidToTheUser — the P01.S02 review, on the model of
// `internal/server`'s `TestEveryReflowCauseIsSaidToTheUser`. Every `RefusalCause` constant is a key of
// `web/app.js`'s REFUSAL_WORDS, and every key there is a constant: a cause with no sentence shows the
// user its code, and a sentence for a cause nothing returns (`timestamp-unverified`, removed in review)
// is a promise nothing keeps. The constants are read from the source, so a new one is counted without
// anyone listing it.
func TestEveryRefusalCauseIsSaidToTheUser(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "revisions.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	causes := map[string]bool{}
	for _, d := range file.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			vs := sp.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "RefusalCause" {
				continue
			}
			for _, v := range vs.Values {
				if lit, ok := v.(*ast.BasicLit); ok {
					c, _ := strconv.Unquote(lit.Value)
					causes[c] = true
				}
			}
		}
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "const REFUSAL_WORDS = {")
	if start < 0 {
		t.Fatal("web/app.js has no REFUSAL_WORDS block")
	}
	end := strings.Index(src[start:], "\n};")
	if end < 0 {
		t.Fatal("web/app.js's REFUSAL_WORDS block does not close")
	}
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*'?([a-z][a-z-]*)'?:`).FindAllStringSubmatch(src[start:start+end], -1) {
		keys[m[1]] = true
	}
	// STIMULUS: both sides were read — D7's five causes, and a block with sentences in it.
	if len(causes) < 5 || len(keys) == 0 {
		t.Fatalf("setup: %d causes and %d sentences read — the census would pass over nothing", len(causes), len(keys))
	}
	for c := range causes {
		if !keys[c] {
			t.Errorf("the refusal cause %q has no sentence in REFUSAL_WORDS — the user is shown the code", c)
		}
	}
	for k := range keys {
		if !causes[k] {
			t.Errorf("REFUSAL_WORDS has a sentence for %q, which is no RefusalCause", k)
		}
	}
}

// TestEverySignatureEnumerationIsTheSweep — D2 under ADR-009, in the guard's grilled form. An
// enumeration of a document's signatures in `internal/sign` is a use of `.Xref` (called, or taken as
// a method value), a use of pdfcpu's object table `.Table`, any method call given the argument
// "ByteRange" or "Fields" (a literal, or a constant naming one — digitorus's `.Key`, pdfcpu's `Find`,
// `ArrayEntry`…), or an index expression with that key (pdfcpu's `types.Dict` is a map, so
// `d["ByteRange"]` is the same read, /pending 752); each must be inside `sweep`, or on a line marked `//sigwalk:exempt <name>` whose name
// is declared here for that file, each exemption matching exactly one use. A second ByteRange walk —
// the `/Fields` walk P01.S02 deleted, which let /pending 661's listed decoy lend coverage no signature
// vouched for — fails here. `revisions.go` also reads file bytes by offset only: no `regexp`, and no
// `bytes`/`strings` `Index*`/`LastIndex*`/`Contains*`/`Cut*`/`Count`, because conjunct (11)'s bound
// is that nothing searches the whole file. And `addedAfterVerdict` is named exactly once outside its
// declaration, in `addedAfter`.
//
// Every declaration is walked — a package-level `var f = func…` is code too — and the P01.S02
// review's bypasses are each driven red by `TestTheSigwalkGuardSeesEveryBypass`.
func TestEverySignatureEnumerationIsTheSweep(t *testing.T) {
	for _, p := range sigwalkCensus(t, ".") {
		t.Error(p)
	}
}

// sigwalkExempt is the declared exemption list: name → the file it may sit in.
var sigwalkExempt = map[string]string{
	// Re-expressed over the sweep it would NARROW towards `Unsigned` (it answers for any FT /Sig).
	"signatureBlobPresent": "verify.go",
	// Reads /Reference for DocMDP; its /Kids blindness is declared at the site (/pending 734).
	"certifiedIn": "identity.go",
	// /pending 749's population cross-check: pdfcpu's table, and the one shape test on each entry. It
	// records nothing and can only route a verdict to could-not-check (ADR-063).
	"unseenSignatures":       "verify.go",
	"unseenSignatures-shape": "verify.go",
	// /pending 751: walks the xref for what resolving it would cost, and resolves no member.
	"libraryLookupCost": "objstm.go",
}

// sigwalkCensus is the guard over the non-test Go files of dir, returning each violation.
func sigwalkCensus(t *testing.T, dir string) (problems []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var parsed []*ast.File
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			t.Fatal(perr)
		}
		parsed = append(parsed, f)
	}
	// Package-level string constants, so `.Key(name)` is read through the name.
	consts := map[string]string{}
	for _, f := range parsed {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.CONST {
				for _, sp := range g.Specs {
					vs := sp.(*ast.ValueSpec)
					for i, n := range vs.Names {
						if i < len(vs.Values) {
							if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
								consts[n.Name], _ = strconv.Unquote(lit.Value)
							}
						}
					}
				}
			}
		}
	}
	stringOf := func(e ast.Expr) string {
		switch x := e.(type) {
		case *ast.BasicLit:
			v, _ := strconv.Unquote(x.Value)
			return v
		case *ast.Ident:
			if x.Obj != nil && x.Obj.Kind == ast.Con {
				if vs, ok := x.Obj.Decl.(*ast.ValueSpec); ok {
					for i, n := range vs.Names {
						if n.Name == x.Name && i < len(vs.Values) {
							if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
								v, _ := strconv.Unquote(lit.Value)
								return v
							}
						}
					}
				}
			}
			return consts[x.Name]
		}
		return ""
	}
	inSweep, seenExempt, verdictRefs := 0, map[string]int{}, map[string]int{}
	for _, file := range parsed {
		path := filepath.Base(fset.Position(file.Pos()).Filename)
		marks := map[int]string{}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if rest, ok := strings.CutPrefix(c.Text, "//sigwalk:exempt"); ok {
					name, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
					marks[fset.Position(c.Pos()).Line] = name
				}
			}
		}
		if path == "revisions.go" {
			for _, im := range file.Imports {
				if p, _ := strconv.Unquote(im.Path.Value); p == "regexp" {
					problems = append(problems, "revisions.go imports regexp: conjunct (11)'s scan is linear and bounded, never a regex over file bytes")
				}
			}
		}
		for _, d := range file.Decls {
			encl, declName := "(package level)", (*ast.Ident)(nil)
			if fn, ok := d.(*ast.FuncDecl); ok {
				encl, declName = fn.Name.Name, fn.Name
			}
			called := map[ast.Expr]bool{}
			enumeration := func(n ast.Node, what string) {
				line := fset.Position(n.Pos()).Line
				name, marked := marks[line]
				switch {
				case encl == "sweep" && path == "revisions.go":
					inSweep++
				case !marked:
					problems = append(problems, fmt.Sprintf("%s:%d (%s) enumerates signatures with %s outside the sweep — read the records "+
						"(`sweepRevisions`/`Revisions`), or mark a declared exemption (ADR-009)", path, line, encl, what))
				case sigwalkExempt[name] != path:
					problems = append(problems, fmt.Sprintf("%s:%d is marked //sigwalk:exempt %q, which is not an exemption declared for this file", path, line, name))
				default:
					seenExempt[name]++
				}
			}
			ast.Inspect(d, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.Ident:
					if x.Name == "addedAfterVerdict" && x != declName {
						verdictRefs[encl]++
					}
				case *ast.CallExpr:
					called[x.Fun] = true
					// Any method given a signature key, not only digitorus's `.Key`: pdfcpu's
					// `Find`, `ArrayEntry`, `IndirectRefEntry`… take the same argument (/pending 752).
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
						for _, a := range x.Args {
							if v := stringOf(a); v == "ByteRange" || v == "Fields" {
								enumeration(x, "."+sel.Sel.Name+`("`+v+`")`)
							}
						}
					}
				case *ast.IndexExpr:
					// pdfcpu's `types.Dict` is a map: `d["ByteRange"]` is the same read as `.Key`,
					// and a walk written against pdfcpu reads the key this way (/pending 752).
					if v := stringOf(x.Index); v == "ByteRange" || v == "Fields" {
						enumeration(x, `["`+v+`"]`)
					}
				case *ast.SelectorExpr:
					switch {
					case x.Sel.Name == "Xref":
						enumeration(x, ".Xref")
					case x.Sel.Name == "Table":
						// pdfcpu's object table (`ctx.XRefTable.Table`, or `ctx.Table` through the
						// embedding): pdfcpu's own enumeration of every object (/pending 752).
						enumeration(x, ".Table")
					case x.Sel.Name == "Key" && !called[x]:
						problems = append(problems, fmt.Sprintf("%s:%d (%s) takes .Key as a method value, which hides its argument from this guard", path, fset.Position(x.Pos()).Line, encl))
					}
					if pkg, ok := x.X.(*ast.Ident); ok && path == "revisions.go" && (pkg.Name == "bytes" || pkg.Name == "strings") {
						for _, pre := range []string{"Index", "LastIndex", "Contains", "Cut", "Count"} {
							if strings.HasPrefix(x.Sel.Name, pre) {
								problems = append(problems, fmt.Sprintf("revisions.go:%d uses %s.%s: nothing in the sweep searches file bytes", fset.Position(x.Pos()).Line, pkg.Name, x.Sel.Name))
							}
						}
					}
				}
				return true
			})
		}
	}
	// STIMULUS: the census sees the sweep's own enumeration (`.Xref` and `.Key("ByteRange")`), or a
	// walk that parsed nothing would pass.
	if inSweep != 2 {
		problems = append(problems, fmt.Sprintf("the census saw %d enumeration uses inside sweep, want 2 (.Xref and .Key(\"ByteRange\"))", inSweep))
	}
	for name, file := range sigwalkExempt {
		if n := seenExempt[name]; n != 1 {
			problems = append(problems, fmt.Sprintf("the exemption %q (%s) matched %d uses, want exactly 1", name, file, n))
		}
	}
	if len(verdictRefs) != 1 || verdictRefs["addedAfter"] != 1 {
		problems = append(problems, fmt.Sprintf("addedAfterVerdict is named from %v, want exactly once, from addedAfter", verdictRefs))
	}
	return problems
}

// TestTheSigwalkGuardSeesEveryBypass — the guard's own red proof, kept. Each shape is added to a
// copy of this package's non-test sources and the census must report it; the clean copy must not.
// The first six are the P01.S02 review's measured bypasses of the guard's first form.
func TestTheSigwalkGuardSeesEveryBypass(t *testing.T) {
	src, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	copyPkg := func(extraFile, extra string, appendRevisions string) string {
		dir := t.TempDir()
		for _, p := range src {
			if strings.HasSuffix(p, "_test.go") {
				continue
			}
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if p == "revisions.go" {
				b = append(b, appendRevisions...)
			}
			if err := os.WriteFile(filepath.Join(dir, p), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if extra != "" {
			if err := os.WriteFile(filepath.Join(dir, extraFile), []byte("package sign\n"+extra), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	// STIMULUS: the clean copy passes, so every red below is the shape's.
	if p := sigwalkCensus(t, copyPkg("", "", "")); len(p) != 0 {
		t.Fatalf("STIMULUS: the clean copy fails the census: %v", p)
	}
	for _, tc := range []struct{ name, file, code, rev, want string }{
		{"a method value of Xref", "zz.go", "func zzA(r *dpdf.Reader) { f := r.Xref; _ = f() }", "", ".Xref outside the sweep"},
		{"a package-level func literal", "zz.go", `var zzB = func(v dpdf.Value) int64 { return v.Key("ByteRange").Index(0).Int64() }`, "", `.Key("ByteRange") outside`},
		{"a constant argument", "zz.go", "const zzKey = \"ByteRange\"\nfunc zzC(v dpdf.Value) dpdf.Value { return v.Key(zzKey) }", "", `.Key("ByteRange") outside`},
		{"a constant declared in another file", "zz.go", "func zzC3(v dpdf.Value) dpdf.Value { return v.Key(zzOtherFileKey) }", "\nconst zzOtherFileKey = \"ByteRange\"\n", `.Key("ByteRange") outside`},
		{"a local constant argument", "zz.go", "func zzC2(v dpdf.Value) dpdf.Value { const k = \"Fields\"; return v.Key(k) }", "", `.Key("Fields") outside`},
		{"a plain second walk", "zz.go", `func zzD(v dpdf.Value) dpdf.Value { return v.Key("ByteRange") }`, "", `.Key("ByteRange") outside`},
		{"a borrowed exemption", "zz.go", `func zzE(v dpdf.Value) dpdf.Value { return v.Key("Fields") } //sigwalk:exempt signatureBlobPresent`, "", "not an exemption declared for this file"},
		{"a method value of Key", "zz.go", `func zzF(v dpdf.Value) dpdf.Value { k := v.Key; return k("ByteRange") }`, "", "method value"},
		{"a second verdict caller", "zz.go", "var zzG = addedAfterVerdict", "", "addedAfterVerdict is named from"},
		{"strings.Index in revisions.go", "", "", "\nfunc zzH(s string) int { return strings.Index(s, \"x\") }\n", "strings.Index"},
		{"strings.Contains in revisions.go", "", "", "\nfunc zzI(s string) bool { return strings.Contains(s, \"x\") }\n", "strings.Contains"},
		{"bytes.Cut in revisions.go", "", "", "\nfunc zzJ(b []byte) { _, _, _ = bytes.Cut(b, nil) }\n", "bytes.Cut"},
		{"bytes.Count in revisions.go", "", "", "\nfunc zzK(b []byte) int { return bytes.Count(b, nil) }\n", "bytes.Count"},
		{"a method value of bytes.Index", "", "", "\nvar zzL = bytes.Index\n", "bytes.Index"},
		// /pending 752: a second enumeration written against pdfcpu.
		{"pdfcpu dictionary indexing", "zz.go", "func zzM(d types.Dict) bool { return d[\"ByteRange\"] != nil }", "", `["ByteRange"] outside`},
		{"pdfcpu dictionary indexing by constant", "zz.go", "const zzN = \"Fields\"\nfunc zzN2(d types.Dict) bool { return d[zzN] != nil }", "", `["Fields"] outside`},
		{"a pdfcpu Dict method", "zz.go", "func zzO(d types.Dict) bool { _, ok := d.Find(\"ByteRange\"); return ok }", "", `.Find("ByteRange") outside`},
		{"pdfcpu's object table", "zz.go", "func zzP(ctx *model.Context) int { return len(ctx.XRefTable.Table) }", "", ".Table outside"},
		{"pdfcpu's object table through the embedding", "zz.go", "func zzQ(ctx *model.Context) int { return len(ctx.Table) }", "", ".Table outside"},
	} {
		got := strings.Join(sigwalkCensus(t, copyPkg(tc.file, tc.code, tc.rev)), "\n")
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s: the census did not report %q; it said:\n%s", tc.name, tc.want, got)
		}
	}
}
