package sign

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pkcs7"
	"github.com/pdfcpu/pdfcpu/pkg/api"

	"nib/internal/testpdf"
)

// The P02.S01 acceptance table runs over DOCUMENT SHAPES, never over cause constants (plan-review C3): one record cause
// reaches different answers depending on whose certificate it names, so a table over the constants can only pass.

var reByteRange = regexp.MustCompile(`/ByteRange\s*\[[^\]]*\]`)

func catalogOf(t *testing.T, doc []byte) int {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatal(err)
	}
	p := r.Trailer().Key("Root").GetPtr()
	return int(p.GetID())
}

func signAs(t *testing.T, doc []byte, id identity, reason string) []byte {
	t.Helper()
	out, err := SignApproval(doc, id.certPEM, id.keyPEM, Options{Name: reason, Reason: reason, When: time.Now()})
	if err != nil {
		t.Fatalf("sign %s: %v", reason, err)
	}
	return out
}

// revisionShape is one document a counterparty could send back, asked about one fingerprint.
type revisionShape struct {
	name  string
	doc   []byte
	fp    string
	want  []byte        // the version expected back, byte for byte; nil for a refusal
	cause RevisionCause // expected on a refusal
	check func(t *testing.T, got SignedRevision)
}

func revisionShapes(t *testing.T) []revisionShape {
	t.Helper()
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	sA := signAs(t, base, a, "original")
	ce := mustSweep(t, sA)[0].CoverageEnd
	root := catalogOf(t, sA)
	vnum, vbody := victimDict(t, sA)

	spoof := signAs(t, sA, b, "stranger")
	appended := synthRevision(t, sA, []sobj{{num: 900, body: "<< /Foo 1 >>"}}, root)
	var resaved bytes.Buffer
	if err := api.Optimize(bytes.NewReader(sA), &resaved, nil); err != nil {
		t.Fatal(err)
	}
	corrupted := append([]byte(nil), sA...)
	gs := mustSweep(t, sA)[0].gapStart // the signature's own `<`, never the first `<30` in the file (the /ID can be one)
	if string(corrupted[gs:gs+3]) != "<30" {
		t.Fatalf("stimulus: the gap starts %q, not a DER SEQUENCE", corrupted[gs:gs+3])
	}
	corrupted[gs+1], corrupted[gs+2] = 'f', 'f'
	redefined := synthRevision(t, sA, []sobj{{num: vnum, body: strings.Replace(vbody, "(original)", "(rewritten)", 1)}}, root)
	redefinedBR := synthRevision(t, sA, []sobj{{num: vnum, body: reByteRange.ReplaceAllString(vbody, "/ByteRange [0 10 20 30]")}}, root)
	redefinedNonSig := synthRevision(t, sA, []sobj{{num: vnum, body: "<< /Foo 2 >>"}}, root)
	negCopy := synthRevision(t, sA, []sobj{{num: 900, body: reByteRange.ReplaceAllString(vbody, "/ByteRange [0 -5 10 10]")}}, root)
	sB := signAs(t, base, b, "bob")
	tsOnly, err := testpdf.DocTimeStamped(base)
	if err != nil {
		t.Fatal(err)
	}
	tsAfter, err := testpdf.DocTimeStamped(sA)
	if err != nil {
		t.Fatal(err)
	}
	twice := signAs(t, appended, a, "again")
	forged := forgeSignerInfo(t, sA)

	// I3: an unrelated erroring record inside B's signed version, which the latest revision then redefines away.
	sA2 := signAs(t, base, a, "original")
	_, vb2 := victimDict(t, sA2)
	hostile := synthRevision(t, sA2, []sobj{{num: 900, body: reByteRange.ReplaceAllString(vb2, "/ByteRange [0 -5 10 10]")}}, catalogOf(t, sA2))
	hB := signAs(t, hostile, b, "bob-over-hostile")
	i3 := synthRevision(t, hB, []sobj{{num: 900, body: "<< /Foo 3 >>"}}, catalogOf(t, hB))

	// A copy of A's dictionary whose ByteRange adds an empty pair ending where B's signature ends: the library verifies
	// it as A (the extra pair adds no bytes), so it proposes the whole spoof as "A's version".
	endB := int64(len(spoof))
	emptyPair := synthRevision(t, spoof, []sobj{{num: 901, body: reByteRange.ReplaceAllStringFunc(vbody, func(br string) string {
		return strings.TrimSuffix(br, "]") + fmt.Sprintf(" %d 0]", endB)
	})}}, catalogOf(t, spoof))
	// The P01 attack (A's record refused) plus a copy of A's dictionary whose extra empty pair ends at the file's end:
	// two refused names, the copy first, so the version is the SECOND verify.
	redefinedPlusCopy := synthRevision(t, redefined, []sobj{{num: 902, body: reByteRange.ReplaceAllStringFunc(vbody, func(br string) string {
		return strings.TrimSuffix(br, "]") + fmt.Sprintf(" %d 0]", len(redefined))
	})}}, catalogOf(t, redefined))
	forgedThenB := signAs(t, forged, b, "bob-over-forged")
	// A later revision redefines the AcroForm without /SigFlags: the library then enumerates nothing and the whole file
	// "verifies" with no error, so no record is Verified — which is not the same as failing.
	afNum := acroFormOf(t, sA)
	afBody := objectBody(t, sA, afNum)
	noSigFlags := synthRevision(t, sA, []sobj{{num: afNum, body: strings.TrimSpace(regexp.MustCompile(`/SigFlags\s+\d+`).ReplaceAllString(afBody, ""))}}, root)
	if !strings.Contains(afBody, "/SigFlags") {
		t.Fatal("stimulus: the AcroForm carries no /SigFlags to drop")
	}
	// The P01 attack plus ONE copy whose ByteRange lists the whole file sixty-four times.
	lr := len(redefined)
	oneCopy := synthRevision(t, redefined, []sobj{{num: 903, body: reByteRange.ReplaceAllString(vbody,
		"/ByteRange ["+strings.TrimSpace(strings.Repeat(fmt.Sprintf("0 %d ", lr), 64))+"]")}}, catalogOf(t, redefined))
	// /SigFlags dropped AND copies of A's blob claiming later ends: the whole file "verifies", nothing is checked, and
	// only the screen keeps the copies out of the verify slots.
	// It rides on the P01 attack, so A's own record is refused too and sorts with the copies, newest first.
	redefinedNoFlags := synthRevision(t, redefined, []sobj{{num: afNum, body: strings.TrimSpace(regexp.MustCompile(`/SigFlags\s+\d+`).ReplaceAllString(afBody, ""))}}, catalogOf(t, redefined))
	var flagCopies []sobj
	for i := 0; i < 2*maxRevisionCandidates; i++ {
		flagCopies = append(flagCopies, sobj{num: 950 + i, body: reByteRange.ReplaceAllString(vbody, fmt.Sprintf("/ByteRange [0 10 %d 10]", len(redefinedNoFlags)+7*i))})
	}
	noSigFlagsCopies := synthRevision(t, redefinedNoFlags, flagCopies, catalogOf(t, redefinedNoFlags))
	if _, revs, err := verifyIndexed(noSigFlagsCopies); err != nil || len(revs) == 0 || revs[0].libPos >= 0 {
		t.Fatalf("stimulus: err %v — the library still enumerates the signatures, so nothing is unchecked", err)
	}
	garbageXref := append(append([]byte(nil), sA...), "\nstartxref\n99999999\n%%EOF\n"...)
	brokenXref := append(append([]byte(nil), sA...), fmt.Sprintf("\nxref\n0 1\nZZZZ\ntrailer\n<< /Size 1 /Prev 0 >>\nstartxref\n%d\n%%%%EOF\n", len(sA)+1)...)
	var twiceNums []sobj
	for _, r := range mustSweep(t, twice) {
		twiceNums = append(twiceNums, sobj{num: int(r.Obj), body: "<< /Foo 4 >>"})
	}
	twiceReplaced := synthRevision(t, twice, twiceNums, catalogOf(t, twice))
	var twiceLast uint32
	for _, r := range mustSweep(t, twice) {
		if r.CoverageEnd == int64(len(twice)) {
			twiceLast = r.Obj
		}
	}
	// The review of P02.S02 — W3: only the LATER of two signatures replaced, so S01 alone returns the earlier one.
	twiceSecondReplaced := synthRevision(t, twice, []sobj{{num: int(twiceLast), body: "<< /Foo 5 >>"}}, catalogOf(t, twice))
	// C3: a copy of A's dictionary whose ranges end where A's version ends, screened out — it must not hide that end.
	endsAtVersion := reByteRange.ReplaceAllString(vbody, fmt.Sprintf("/ByteRange [0 10 20 %d]", len(sA)-20))
	copyHidesErr := synthRevision(t, synthRevision(t, redefinedNonSig, []sobj{{num: 900, body: reByteRange.ReplaceAllString(vbody, "/ByteRange [0 -5 10 10]")}}, root),
		[]sobj{{num: 903, body: endsAtVersion}}, root)
	noFlagsNonSig := synthRevision(t, redefinedNonSig, []sobj{{num: afNum, body: strings.TrimSpace(regexp.MustCompile(`/SigFlags\s+\d+`).ReplaceAllString(afBody, ""))}}, root)
	copyHidesNoFlags := synthRevision(t, noFlagsNonSig, []sobj{{num: 903, body: endsAtVersion}}, root)
	// W: a forged signature naming A, /SigFlags dropped, and copies whose ranges each select nearly the whole file —
	// they spend the screen budget, so nib did not look at everything and must not answer `resaved`.
	forgedNoFlags := synthRevision(t, forged, []sobj{{num: afNum, body: strings.TrimSpace(regexp.MustCompile(`/SigFlags\s+\d+`).ReplaceAllString(afBody, ""))}}, root)
	drain := func(size int) []byte {
		var objs []sobj
		for i := 0; i < screenBudgetFactor+4; i++ {
			objs = append(objs, sobj{num: 960 + i, body: reByteRange.ReplaceAllString(vbody, fmt.Sprintf("/ByteRange [0 10 %07d %07d]", 20+i, size-40-i))})
		}
		return synthRevision(t, forgedNoFlags, objs, root)
	}
	budgetSpent := drain(len(drain(0)))
	// The same drain with no signature and no boundary a ByteRange ends at, so the walk has nothing to look at and the
	// spent budget is S01's own to report.
	baseNoFlags := base
	if bytes.Contains(base, []byte("/SigFlags")) {
		t.Fatal("stimulus: the base form carries /SigFlags, so the copies would be dropped as failed, not screened")
	}
	copiesOnly := func(size int) []byte {
		var objs []sobj
		for i := 0; i < screenBudgetFactor+4; i++ {
			objs = append(objs, sobj{num: 960 + i, body: reByteRange.ReplaceAllString(vbody, fmt.Sprintf("/ByteRange [0 10 %07d %07d]", 20+i, size-40-i))})
		}
		return synthRevision(t, baseNoFlags, objs, catalogOf(t, baseNoFlags))
	}
	copiesOnlyDoc := copiesOnly(len(copiesOnly(0)))
	// The re-review of P02.S02: the second signature's dictionary replaced AND more fake xref sections than the screen
	// cap after it, each with a ByteRange ending at its own marker — the first version is genuine, but the search for
	// the last was cut, and the result must say so.
	fakesAfter := func(prev []byte) []byte {
		k := maxBoundaryScreens + 2
		build := func(heads, ends []int64) []byte {
			var sb strings.Builder
			for i := 0; i < k; i++ {
				fmt.Fprintf(&sb, "%06d 0 obj\n<</Type/XRef>>/ByteRange [0 10 20 %012d]\nstartxref\n%012d\n%%%%EOF\n", 3000+i, ends[i]-20, heads[i])
			}
			return synthRevision(t, prev, []sobj{{num: 991, body: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", sb.Len(), sb.String())}}, catalogOf(t, prev))
		}
		heads, ends := make([]int64, k), make([]int64, k)
		h := build(heads, ends)
		for i, at := 0, len(prev); i < k; i++ {
			hp := bytes.Index(h[at:], []byte(fmt.Sprintf("%06d 0 obj", 3000+i))) + at
			ep := bytes.Index(h[hp:], []byte("%%EOF\n")) + hp
			heads[i], ends[i] = int64(hp), int64(ep+6)
			at = ep + 6
		}
		return build(heads, ends)
	}
	hiddenLast := fakesAfter(twiceSecondReplaced)
	var tsaFP string
	for _, r := range mustSweep(t, tsOnly) {
		if r.Timestamp {
			tsaFP = r.named
		}
	}
	if tsaFP == "" {
		t.Fatal("stimulus: the timestamp names no certificate, so asking as its authority tests nothing")
	}

	return []revisionShape{
		{name: "signed, returned untouched", doc: sA, fp: a.fp, want: sA},
		{name: "a stranger co-signed to EOF (the spoof)", doc: spoof, fp: a.fp, want: sA[:ce]},
		{name: "the spoof, asked as the stranger", doc: spoof, fp: b.fp, want: spoof},
		{name: "an unsigned revision appended", doc: appended, fp: a.fp, want: sA[:ce]},
		{name: "re-saved wholesale by pdfcpu", doc: resaved.Bytes(), fp: a.fp, cause: RevisionResaved,
			check: func(t *testing.T, got SignedRevision) {
				if !got.Attributed {
					t.Error("a re-save keeps the SignerInfo intact, so the name should check against the key")
				}
			}},
		{name: "the signature blob corrupted", doc: corrupted, fp: a.fp, cause: RevisionNotYours,
			check: func(t *testing.T, got SignedRevision) {
				if len(got.Refused) == 0 {
					t.Error("the unreadable signature is not reported: \"not yours\" must carry what nib could not read")
				}
			}},
		{name: "a later revision rewrote the signer's /Reason (P01's attack)", doc: redefined, fp: a.fp, want: sA[:ce],
			check: func(t *testing.T, got SignedRevision) {
				if got.RedefinedObj != uint32(vnum) {
					t.Errorf("RedefinedObj %d, want %d: the rewrite is evidence and must be named", got.RedefinedObj, vnum)
				}
			}},
		// P02.S02: no record in the file as it stands reaches these versions; the boundary walk does (W6, W7).
		{name: "a later revision rewrote the signer's ByteRange", doc: redefinedBR, fp: a.fp, want: sA, check: foundEarlier(vnum)},
		{name: "a later revision replaced the signature with a non-signature", doc: redefinedNonSig, fp: a.fp, want: sA, check: foundEarlier(vnum)},
		{name: "a garbage startxref appended (pdfcpu refuses the whole file)", doc: garbageXref, fp: a.fp, want: sA, check: foundEarlier(0)},
		{name: "a broken xref section appended (pdfcpu refuses the whole file)", doc: brokenXref, fp: a.fp, want: sA, check: foundEarlier(0)},
		{name: "signed twice, then only the second dictionary replaced (W3)", doc: twiceSecondReplaced, fp: a.fp, want: twice,
			check: func(t *testing.T, got SignedRevision) {
				foundEarlier(int(twiceLast))(t, got)
				if len(got.Earlier) != 1 || got.Earlier[0] != ce {
					t.Errorf("Earlier %v, want [%d]", got.Earlier, ce)
				}
			}},
		{name: "the last version hidden behind more fake xref sections than the screen cap", doc: hiddenLast, fp: a.fp, want: sA,
			check: func(t *testing.T, got SignedRevision) {
				if !got.LaterUnchecked {
					t.Error("the search for a later version was cut and the result reads as the signer's last")
				}
			}},
		{name: "signed twice, nothing hidden: the last version, and the search finished", doc: twice, fp: a.fp, want: twice,
			check: func(t *testing.T, got SignedRevision) {
				if got.LaterUnchecked {
					t.Error("LaterUnchecked on a document with nothing to search past the version")
				}
			}},
		{name: "a screened-out copy ends where the version ends, whole file unreadable (C3)", doc: copyHidesErr, fp: a.fp, want: sA, check: foundEarlier(0)},
		{name: "a screened-out copy ends where the version ends, /SigFlags dropped (C3)", doc: copyHidesNoFlags, fp: a.fp, want: sA, check: foundEarlier(vnum)},
		{name: "copies spend the screen budget around a forged signature (W)", doc: budgetSpent, fp: a.fp, cause: RevisionCouldNotCheck},
		{name: "copies of a signer's dictionary in a document they never signed spend the budget", doc: copiesOnlyDoc, fp: a.fp, cause: RevisionCouldNotCheck},
		{name: "signed twice, then both dictionaries replaced", doc: twiceReplaced, fp: a.fp, want: twice,
			check: func(t *testing.T, got SignedRevision) {
				foundEarlier(int(twiceLast))(t, got)
				if len(got.Earlier) != 1 || got.Earlier[0] != ce {
					t.Errorf("Earlier %v, want [%d]: the first signature is read from the recovered version", got.Earlier, ce)
				}
			}},
		{name: "a negative-length copy appended (C2)", doc: negCopy, fp: a.fp, want: sA[:ce]},
		{name: "a negative-length copy, asked as a stranger", doc: negCopy, fp: b.fp, cause: RevisionCouldNotCheck},
		{name: "unsigned", doc: base, fp: a.fp, cause: RevisionNoSignature},
		{name: "signed by someone else only", doc: sB, fp: a.fp, cause: RevisionNotYours},
		{name: "a document timestamp only", doc: tsOnly, fp: a.fp, cause: RevisionNoSignature},
		{name: "signed, then a document timestamp", doc: tsAfter, fp: a.fp, want: sA[:ce]},
		{name: "signed twice by the same signer", doc: twice, fp: a.fp, want: twice,
			check: func(t *testing.T, got SignedRevision) {
				if len(got.Earlier) != 1 || got.Earlier[0] != ce {
					t.Errorf("Earlier %v, want [%d]: the first signature is listed beside the last", got.Earlier, ce)
				}
			}},
		{name: "a forged SignerInfo naming the signer", doc: forged, fp: a.fp, cause: RevisionResaved,
			check: func(t *testing.T, got SignedRevision) {
				if got.Attributed {
					t.Error("a SignerInfo whose signature does not check is attributed to the certificate it names")
				}
			}},
		{name: "an erroring record inside the signed version (I3)", doc: i3, fp: b.fp, cause: RevisionPrefixFailed},
		{name: "an empty fingerprint matches nothing", doc: sA, fp: "", cause: RevisionNotYours},
		{name: "a copy of the signer's dictionary claims a stranger's later end", doc: emptyPair, fp: a.fp, want: sA[:ce],
			check: func(t *testing.T, got SignedRevision) {
				if len(got.Later) == 0 {
					t.Error("the copy claiming a later end is not reported in Later")
				}
			}},
		{name: "a document timestamp, asked as its authority", doc: tsOnly, fp: tsaFP, cause: RevisionNoSignature},
		{name: "an unreadable signature, asked with an empty fingerprint", doc: corrupted, fp: "", cause: RevisionNotYours},
		{name: "the P01 attack, plus a copy claiming a stranger's later end", doc: redefinedPlusCopy, fp: a.fp, want: sA[:ce]},
		{name: "a failing signature naming the signer, then another signer", doc: forgedThenB, fp: a.fp, cause: RevisionResaved},
		{name: "a later revision dropped /SigFlags", doc: noSigFlags, fp: a.fp, want: sA[:ce]},
		{name: "the P01 attack, /SigFlags dropped, plus copies claiming later ends", doc: noSigFlagsCopies, fp: a.fp, want: sA[:ce]},
		{name: "the P01 attack, plus one copy listing the whole file 64 times", doc: oneCopy, fp: a.fp, want: sA[:ce]},
	}
}

// forgeSignerInfo flips one byte of the SignerInfo's encrypted digest: the name and the certificate are untouched.
func forgeSignerInfo(t *testing.T, doc []byte) []byte {
	t.Helper()
	sw := mustSweep(t, doc)
	gs, ge := sw[0].gapStart, sw[0].gapEnd
	var rv asn1.RawValue
	if _, err := asn1.Unmarshal(mustHex(t, doc[gs+1:ge-1]), &rv); err != nil {
		t.Fatal(err)
	}
	out := append([]byte(nil), doc...)
	pos := int(gs) + 1 + 2*(len(rv.FullBytes)-1) + 1 // the last DER byte, inside EncryptedDigest
	if out[pos] == '0' {
		out[pos] = '1'
	} else {
		out[pos] = '0'
	}
	return out
}

func TestTheSignedVersionAcrossDocumentShapes(t *testing.T) {
	for _, sh := range revisionShapes(t) {
		t.Run(sh.name, func(t *testing.T) {
			got := SignedRevisionFor(sh.doc, sh.fp)
			if sh.want != nil {
				if got.Cause != "" {
					t.Fatalf("refused %q (refused records %+v); want the signed version back", got.Cause, got.Refused)
				}
				if !bytes.Equal(got.Prefix, sh.want) {
					t.Fatalf("returned %d bytes, want the %d-byte signed version byte for byte", len(got.Prefix), len(sh.want))
				}
				if got.End != int64(len(sh.want)) {
					t.Errorf("End %d, want %d", got.End, len(sh.want))
				}
			} else {
				if got.Prefix != nil {
					t.Fatalf("returned %d bytes; want refusal %q", len(got.Prefix), sh.cause)
				}
				if got.Cause != sh.cause {
					t.Fatalf("cause %q, want %q", got.Cause, sh.cause)
				}
			}
			if sh.check != nil {
				sh.check(t, got)
			}
		})
	}
}

// TestTheSignedVersionIsCappedAtItsOwnLength — an append to the returned prefix must not write into the caller's file.
func TestTheSignedVersionIsCappedAtItsOwnLength(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a, b := newIdentity(t, "Alice"), newIdentity(t, "Bob")
	sA := signAs(t, base, a, "original")
	spoof := signAs(t, sA, b, "stranger")
	got := SignedRevisionFor(spoof, a.fp)
	if got.Prefix == nil || cap(got.Prefix) != len(got.Prefix) {
		t.Fatalf("prefix len %d cap %d: an append would overwrite the stranger's revision in the caller's buffer", len(got.Prefix), cap(got.Prefix))
	}
}

// TestCopiesOfTheSignersBlobCannotCrowdOutTheirVersion — when the whole file does not verify, every record naming the
// signer is a candidate. Measured 2026-10-02 before the screen: 40 copies of the signer's own blob claiming ends past
// the real one filled every verify slot ahead of it, and the version that was in the file came back `could-not-check`.
func TestCopiesOfTheSignersBlobCannotCrowdOutTheirVersion(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	sA := signAs(t, base, a, "original")
	ce := mustSweep(t, sA)[0].CoverageEnd
	_, vbody := victimDict(t, sA)
	var objs []sobj
	for i := 0; i < 3*maxRevisionCandidates; i++ {
		objs = append(objs, sobj{num: 900 + i, body: reByteRange.ReplaceAllString(vbody, fmt.Sprintf("/ByteRange [0 10 %d 10]", len(sA)+7*i))})
	}
	objs = append(objs, sobj{num: 999, body: reByteRange.ReplaceAllString(vbody, "/ByteRange [0 -5 10 10]")})
	doc := synthRevision(t, sA, objs, catalogOf(t, sA))
	_, revs, verr := verifyIndexed(doc)
	if verr == nil {
		t.Fatal("stimulus: the whole file verified, so the naming copies were never candidates")
	}
	cands := revisionCandidates(doc, revs, verr, a.fp)
	ahead := 0
	for _, c := range cands {
		if c.end > ce {
			ahead++
		}
	}
	if ahead <= maxRevisionCandidates {
		t.Fatalf("stimulus: %d candidates ahead of the signer's version, not more than the %d verify slots", ahead, maxRevisionCandidates)
	}
	got := SignedRevisionFor(doc, a.fp)
	if !bytes.Equal(got.Prefix, sA[:ce]) {
		t.Fatalf("cause %q, %d bytes: copies of the signer's own blob crowded out the version that is in the file", got.Cause, len(got.Prefix))
	}
}

// TestAttributedNeedsTheSignersOwnSignature — the check reads the SignerInfo, not the certificate beside it.
func TestAttributedNeedsTheSignersOwnSignature(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	sA := signAs(t, base, a, "original")
	sw := mustSweep(t, sA)
	p7, err := pkcs7.Parse(mustHex(t, sA[sw[0].gapStart+1:sw[0].gapEnd-1]))
	if err != nil {
		t.Fatal(err)
	}
	if !proofOf(p7).attributed() {
		t.Fatal("stimulus: the honest signature does not check, so a false below proves nothing")
	}
	p7.Signers[0].EncryptedDigest = append([]byte(nil), p7.Signers[0].EncryptedDigest...)
	p7.Signers[0].EncryptedDigest[len(p7.Signers[0].EncryptedDigest)-1] ^= 1
	if proofOf(p7).attributed() {
		t.Error("a SignerInfo with a broken signature is attributed")
	}
}

// TestEverySweepRunsBehindThePdfcpuGate — P02.S01's grill (T10): the sweep opens digitorus's reader, so every function
// calling `sweepRevisions` must have run pdfcpu's read before it (ADR-041). S02's screen would be the first caller to
// sweep a prefix pdfcpu never read; this fails it unless it gates too. Census by function, and position within it.
func TestEverySweepRunsBehindThePdfcpuGate(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	callers := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			gate, sweep := token.NoPos, token.NoPos
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				switch id.Name {
				case "pdfcpuRead", "pdfcpuCanRead":
					if gate == token.NoPos || call.Pos() < gate {
						gate = call.Pos()
					}
				case "sweepRevisions":
					if sweep == token.NoPos || call.Pos() < sweep {
						sweep = call.Pos()
					}
				}
				return true
			})
			if sweep == token.NoPos {
				continue
			}
			callers++
			if gate == token.NoPos || gate > sweep {
				t.Errorf("%s: %s sweeps without pdfcpu's read before it — the digitorus reader would run on a file "+
					"ADR-041's gate never saw", fset.Position(sweep), fd.Name.Name)
			}
		}
	}
	// STIMULUS: verifyIndexed and signedAsIntended both sweep, so a clean result is not a blind walk.
	if callers < 2 {
		t.Fatalf("found %d function(s) calling sweepRevisions, fewer than the 2 known — the scan is not seeing them", callers)
	}
}

// TestWellFormedVersionsAreTriedBeforeRefusedCopies — the hashing budget is finite, so the order matters: refused
// copies each claiming to cover nearly the whole file would spend it before the signer's own well-formed record.
func TestWellFormedVersionsAreTriedBeforeRefusedCopies(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	sA := signAs(t, base, a, "original")
	ce := mustSweep(t, sA)[0].CoverageEnd
	_, vbody := victimDict(t, sA)
	const copies = 40
	build := func(span int) []byte {
		var objs []sobj
		for i := 0; i < copies; i++ {
			objs = append(objs, sobj{num: 900 + i, body: reByteRange.ReplaceAllString(vbody, fmt.Sprintf("/ByteRange [0 10 %07d %07d]", 20+i, span))})
		}
		objs = append(objs, sobj{num: 999, body: reByteRange.ReplaceAllString(vbody, "/ByteRange [0 -5 10 10]")})
		return synthRevision(t, sA, objs, catalogOf(t, sA))
	}
	doc := build(0)
	doc = build(len(doc) - 20 - copies) // fixed-width numbers: the length does not move
	_, revs, verr := verifyIndexed(doc)
	if verr == nil {
		t.Fatal("stimulus: the whole file verified, so the copies were never candidates")
	}
	var hashed int64
	for _, c := range revisionCandidates(doc, revs, verr, a.fp) {
		if c.onlyRefused {
			for range c.proposers {
				hashed += int64(len(doc) - 20 - copies)
			}
		}
	}
	if hashed <= int64(screenBudgetFactor)*int64(len(doc)) {
		t.Fatalf("stimulus: the copies hash %d bytes, inside the %d-byte budget, so their order cannot matter", hashed, screenBudgetFactor*len(doc))
	}
	got := SignedRevisionFor(doc, a.fp)
	if !bytes.Equal(got.Prefix, sA[:ce]) {
		t.Fatalf("cause %q: refused copies spent the budget before the signer's own version was tried", got.Cause)
	}
}

// TestTheHelpersKeepTheirOwnContracts — each holds its rule on inputs the callers today never build, because S02 adds
// candidates that come from no record at all.
func TestTheHelpersKeepTheirOwnContracts(t *testing.T) {
	for _, c := range []struct {
		br   []int64
		size int
		ok   bool
	}{
		{[]int64{0, 10, 20, 30}, 50, true},
		{[]int64{0, 10, 20, 30}, 49, false}, // past the file
		{[]int64{0, 10, 40, -5}, 50, false}, // a negative length
		{[]int64{0, 10, -5, 40}, 50, false}, // a negative offset
		{[]int64{0, 10, 20}, 50, false},     // odd
		{[]int64{0, 0}, 50, false},          // ends at zero
	} {
		if _, ok := lastPairEnd(c.br, c.size); ok != c.ok {
			t.Errorf("lastPairEnd(%v, %d) ok=%v, want %v", c.br, c.size, ok, c.ok)
		}
	}
	for _, c := range []struct {
		br   []int64
		cost int64
		ok   bool
	}{
		{[]int64{0, 10, 20, 30}, 40, true},
		{[]int64{0, -5, 10, 10}, 0, false}, // a negative length would slice backwards
		{[]int64{-5, 10, 20, 10}, 0, false},
		{[]int64{0, 10, 45, 10}, 0, false}, // past the file
		{[]int64{0, 10, 20}, 0, false},
		{[]int64{0, 50, 0, 50}, 0, false},  // the same bytes twice: one record could charge any multiple of the file
		{[]int64{20, 10, 0, 10}, 0, false}, // descending
		{[]int64{0, 10, 10, 10}, 20, true}, // abutting is fine
	} {
		if cost, ok := rangeCost(c.br, 50); ok != c.ok || cost != c.cost {
			t.Errorf("rangeCost(%v, 50) = %d, %v; want %d, %v", c.br, cost, ok, c.cost, c.ok)
		}
	}
	good := Revision{Obj: 7, Fingerprint: "ab", Verified: true, CoverageEnd: 100}
	if obj, ok := holder([]Revision{good}, "ab", 100); !ok || obj != 7 {
		t.Fatal("stimulus: a verified, well-formed record does not hold")
	}
	for name, r := range map[string]Revision{
		"unverified":   {Fingerprint: "ab", CoverageEnd: 100},
		"refused":      {Fingerprint: "ab", Verified: true, Cause: CauseContentsElsewhere, CoverageEnd: 100},
		"a timestamp":  {Fingerprint: "ab", Verified: true, Timestamp: true, CoverageEnd: 100},
		"short":        {Fingerprint: "ab", Verified: true, CoverageEnd: 99},
		"someone else": {Fingerprint: "cd", Verified: true, CoverageEnd: 100},
	} {
		if _, ok := holder([]Revision{r}, "ab", 100); ok {
			t.Errorf("holder accepted a record that is %s", name)
		}
	}
	unnamed := []Revision{{named: "", ByteRange: []int64{0, 1, 2, 3}, Verified: true}}
	if c := revisionCandidates(make([]byte, 10), []Revision{{named: "ab", ByteRange: []int64{0, 1, 2, 3}, Verified: true}}, nil, "ab"); len(c) != 1 {
		t.Fatalf("stimulus: a verified record naming the fingerprint proposed %d candidates, not 1", len(c))
	}
	if c := revisionCandidates(make([]byte, 10), unnamed, nil, ""); c != nil {
		t.Errorf("an empty fingerprint proposed %d candidates from an unnamed record", len(c))
	}
}

// TestTheScreenNeedsTheSignersOwnSignature — a SignerInfo with a broken signature whose ranges are right is screened
// out, so it never costs a full re-verify.
func TestTheScreenNeedsTheSignersOwnSignature(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	sA := signAs(t, base, a, "original")
	rec := mustSweep(t, sA)[0]
	p7, err := pkcs7.Parse(mustHex(t, sA[rec.gapStart+1:rec.gapEnd-1]))
	if err != nil {
		t.Fatal(err)
	}
	rec.proof = proofOf(p7)
	budget := int64(1 << 40)
	if !(candidate{end: rec.CoverageEnd, proposers: []*Revision{&rec}}).screen(sA, &budget) {
		t.Fatal("stimulus: the honest record does not pass the screen")
	}
	if proofOf(p7).digestMatches(sA, []int64{0, 10, 20, 30}) {
		t.Error("ranges selecting other bytes hash to the signed digest")
	}
	p7.Signers[0].EncryptedDigest = append([]byte(nil), p7.Signers[0].EncryptedDigest...)
	p7.Signers[0].EncryptedDigest[len(p7.Signers[0].EncryptedDigest)-1] ^= 1
	rec.proof = proofOf(p7)
	if (candidate{end: rec.CoverageEnd, proposers: []*Revision{&rec}}).screen(sA, &budget) {
		t.Error("a broken SignerInfo over the right ranges passed the screen")
	}
}

// redefinedWithCopies is the P01 attack (the signer's dictionary redefined, so the genuine record is refused) with
// copies of the signer's own blob appended ahead of it and one negative-length copy failing the whole file, so every
// candidate is a refused NAME and the copies sort first. ranges gives copy i's ByteRange text.
func redefinedWithCopies(t *testing.T, n int, ranges func(i, size int) string) (doc, want []byte, vnum int, fp string) {
	t.Helper()
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	sA := signAs(t, base, a, "original")
	ce := mustSweep(t, sA)[0].CoverageEnd
	vnum, vbody := victimDict(t, sA)
	redefined := synthRevision(t, sA, []sobj{{num: vnum, body: strings.Replace(vbody, "(original)", "(rewritten)", 1)}}, catalogOf(t, sA))
	build := func(size int) []byte {
		var objs []sobj
		for i := 0; i < n; i++ {
			objs = append(objs, sobj{num: 900 + i, body: reByteRange.ReplaceAllString(vbody, ranges(i, size))})
		}
		objs = append(objs, sobj{num: 999, body: reByteRange.ReplaceAllString(vbody, "/ByteRange [0 -5 10 10]")})
		return synthRevision(t, redefined, objs, catalogOf(t, redefined))
	}
	doc = build(0)
	doc = build(len(doc)) // every number is fixed-width, so the length does not move
	_, revs, verr := verifyIndexed(doc)
	if verr == nil {
		t.Fatal("stimulus: the whole file verified, so the copies were never candidates")
	}
	for _, r := range revs {
		if r.named == a.fp && r.Cause == "" {
			t.Fatalf("stimulus: object %d names the signer and is well-formed, so the genuine record does not sort behind the copies", r.Obj)
		}
	}
	return doc, sA[:ce], vnum, a.fp
}

// TestTheScreenKeepsCopiesFromCrowdingOutAVersion — copies of the signer's blob claiming later ends sort ahead of the
// genuine (refused) record and outnumber the verify slots; only the screen lets the version through.
func TestTheScreenKeepsCopiesFromCrowdingOutAVersion(t *testing.T) {
	doc, want, vnum, fp := redefinedWithCopies(t, 2*maxRevisionCandidates, func(i, size int) string {
		return fmt.Sprintf("/ByteRange [0 10 %07d 10]", size-20-7*i)
	})
	got := SignedRevisionFor(doc, fp)
	if !bytes.Equal(got.Prefix, want) {
		t.Fatalf("cause %q: copies ahead of the genuine record filled the verify slots", got.Cause)
	}
	if got.RedefinedObj != uint32(vnum) {
		t.Errorf("RedefinedObj %d, want %d", got.RedefinedObj, vnum)
	}
}

// TestASpentBudgetIsCouldNotCheck — the declared residual, pinned: copies whose ranges each select nearly the whole file
// spend the hashing budget before the genuine record is reached, and the answer is the honest one.
func TestASpentBudgetIsCouldNotCheck(t *testing.T) {
	n := screenBudgetFactor + 4
	doc, _, _, fp := redefinedWithCopies(t, n, func(i, size int) string {
		return fmt.Sprintf("/ByteRange [0 10 %07d %07d]", 20+i, size-40-i)
	})
	got := SignedRevisionFor(doc, fp)
	if got.Prefix != nil || got.Cause != RevisionCouldNotCheck {
		t.Fatalf("cause %q, %d bytes: the budget did not bound the screen, or its end was not reported as could-not-check", got.Cause, len(got.Prefix))
	}
}

// TestAWellFormedProposerClearsRefusedOnly — an end proposed by a refused record AND a well-formed one is not a
// redefinition, whichever comes first in xref order.
func TestAWellFormedProposerClearsRefusedOnly(t *testing.T) {
	refused := Revision{Obj: 3, named: "ab", ByteRange: []int64{0, 1, 2, 3}, Verified: true, Cause: CauseContentsElsewhere}
	good := Revision{Obj: 4, named: "ab", ByteRange: []int64{0, 1, 2, 3}, Verified: true}
	for _, order := range [][]Revision{{refused, good}, {good, refused}} {
		c := revisionCandidates(make([]byte, 10), order, nil, "ab")
		if len(c) != 1 || c[0].onlyRefused || len(c[0].proposers) != 2 {
			t.Errorf("order %d,%d: %+v, want one candidate, not refused-only, two proposers", order[0].Obj, order[1].Obj, c)
		}
	}
}

// TestLaterNamesOnlyWhatReachesPast — a record ending exactly at the version is not later; one past it, or past the
// file, is.
func TestLaterNamesOnlyWhatReachesPast(t *testing.T) {
	revs := []Revision{
		{Obj: 1, named: "ab", ByteRange: []int64{0, 10, 20, 30}},  // ends at 50, the version
		{Obj: 2, named: "ab", ByteRange: []int64{0, 10, 20, 40}},  // 60, later
		{Obj: 3, named: "ab", ByteRange: []int64{0, 10, 20, 999}}, // past the file
		{Obj: 4, named: "cd", ByteRange: []int64{0, 10, 20, 40}},  // someone else
		{Obj: 5, named: "ab", ByteRange: []int64{0, 10, 20, 40}, Timestamp: true},
		{Obj: 6, named: "ab", ByteRange: []int64{0, 10, 30, -5}}, // a negative length: no end a slice could take
	}
	got := laterNaming(make([]byte, 100), revs, "ab", 50)
	if fmt.Sprint(got) != "[2 3 6]" {
		t.Errorf("Later %v, want [2 3 6]", got)
	}
}

// TestTheDigestScreenKnowsWhatItCannotScreen — RSA and SHA-384 are compared, and the shapes the screen cannot read
// (encapsulated content, no messageDigest) pass to the full re-verify rather than being thrown away.
func TestTheDigestScreenKnowsWhatItCannotScreen(t *testing.T) {
	content := bytes.Repeat([]byte("signed bytes "), 50)
	whole := []int64{0, int64(len(content))}
	other := []int64{0, 10}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(key any, pub any, digest asn1.ObjectIdentifier, detach bool) *pkcs7.PKCS7 {
		t.Helper()
		tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "t"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
		if err != nil {
			t.Fatal(err)
		}
		cert, _ := x509.ParseCertificate(der)
		sd, err := pkcs7.NewSignedData(content)
		if err != nil {
			t.Fatal(err)
		}
		sd.SetDigestAlgorithm(digest)
		if err := sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
			t.Fatal(err)
		}
		if detach {
			sd.Detach()
		}
		raw, err := sd.Finish()
		if err != nil {
			t.Fatal(err)
		}
		p7, err := pkcs7.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return p7
	}
	for name, p7 := range map[string]*pkcs7.PKCS7{
		"ECDSA SHA-384": sign(ec, &ec.PublicKey, pkcs7.OIDDigestAlgorithmSHA384, true),
		"RSA SHA-256":   sign(rk, &rk.PublicKey, pkcs7.OIDDigestAlgorithmSHA256, true),
		"RSA SHA-512":   sign(rk, &rk.PublicKey, pkcs7.OIDDigestAlgorithmSHA512, true),
		// x509's CheckSignature refuses SHA-1 outright; third-party PDFs still carry it (pdfsign testfile30).
		"RSA SHA-1":   sign(rk, &rk.PublicKey, pkcs7.OIDDigestAlgorithmSHA1, true),
		"ECDSA SHA-1": sign(ec, &ec.PublicKey, pkcs7.OIDDigestAlgorithmSHA1, true),
	} {
		if !proofOf(p7).attributed() {
			t.Errorf("%s: the honest SignerInfo is not attributed", name)
		}
		if !proofOf(p7).digestMatches(content, whole) {
			t.Errorf("%s: the signed ranges do not match their own digest", name)
		}
		if proofOf(p7).digestMatches(content, other) {
			t.Errorf("%s: other ranges match the digest — it is passed through, not compared", name)
		}
	}
	for name, p7 := range map[string]*pkcs7.PKCS7{
		"RSA":   sign(rk, &rk.PublicKey, pkcs7.OIDDigestAlgorithmSHA256, true),
		"ECDSA": sign(ec, &ec.PublicKey, pkcs7.OIDDigestAlgorithmSHA256, true),
	} {
		p7.Signers[0].EncryptedDigest[len(p7.Signers[0].EncryptedDigest)/2] ^= 1
		if proofOf(p7).attributed() {
			t.Errorf("%s: a SignerInfo with a broken signature is attributed", name)
		}
	}
	enc := sign(ec, &ec.PublicKey, pkcs7.OIDDigestAlgorithmSHA256, false)
	if len(enc.Content) == 0 {
		t.Fatal("stimulus: the encapsulating SignedData carries no content")
	}
	if !proofOf(enc).digestMatches(content, other) {
		t.Error("an encapsulating SignerInfo was screened out; it cannot be screened by ranges and must reach the re-verify")
	}
	bare := sign(ec, &ec.PublicKey, pkcs7.OIDDigestAlgorithmSHA256, true)
	attrs := bare.Signers[0].AuthenticatedAttributes[:0]
	for _, a := range bare.Signers[0].AuthenticatedAttributes {
		if !a.Type.Equal(oidMessageDigest) {
			attrs = append(attrs, a)
		}
	}
	bare.Signers[0].AuthenticatedAttributes = attrs
	if !proofOf(bare).digestMatches(content, other) {
		t.Error("a SignerInfo with no messageDigest was screened out; it cannot be screened and must reach the re-verify")
	}
}

// TestTheScreenChargesBeforeItHashes — a record that would overrun the budget is not hashed: the budget is spent at
// once, and every later screen refuses without hashing.
func TestTheScreenChargesBeforeItHashes(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	sA := signAs(t, base, a, "original")
	rec := mustSweep(t, sA)[0]
	p7, err := pkcs7.Parse(mustHex(t, sA[rec.gapStart+1:rec.gapEnd-1]))
	if err != nil {
		t.Fatal(err)
	}
	var proposers []*Revision
	for i := 0; i < 5; i++ { // attributed, but their ranges (40 bytes each) hash to something else
		proposers = append(proposers, &Revision{proof: proofOf(p7), ByteRange: []int64{0, 10, 20, 30}})
	}
	c := candidate{end: 50, proposers: proposers}
	budget := int64(100)
	if c.screen(sA, &budget) {
		t.Fatal("stimulus: ranges selecting other bytes passed the screen")
	}
	if budget != -1 {
		t.Fatalf("budget %d after the screen, want -1: the third record (80+40 > 100) was hashed before it was charged", budget)
	}
	rec.proof = proofOf(p7)
	if (candidate{end: rec.CoverageEnd, proposers: []*Revision{&rec}}).screen(sA, &budget) {
		t.Error("a spent budget still let the genuine record be hashed")
	}
}

func acroFormOf(t *testing.T, doc []byte) int {
	t.Helper()
	r, err := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatal(err)
	}
	p := r.Trailer().Key("Root").Key("AcroForm").GetPtr()
	if p.GetID() == 0 {
		t.Fatal("stimulus: the AcroForm is direct, so it cannot be redefined alone")
	}
	return int(p.GetID())
}

func objectBody(t *testing.T, doc []byte, num int) string {
	t.Helper()
	hdr := []byte(fmt.Sprintf("\n%d 0 obj", num))
	i := bytes.LastIndex(doc, hdr)
	if i < 0 {
		t.Fatalf("no object %d", num)
	}
	j := bytes.Index(doc[i:], []byte("endobj"))
	return string(doc[i+len(hdr) : i+j])
}

// TestARecordHoldsNoCopyOfItsBlob — the sweep runs on every install, mutation and undo. Records sharing one indirect
// `/Contents` each held its decoded blob: +201 MB at 100 dictionaries over a 2 MB blob (P02.S01's review).
func TestARecordHoldsNoCopyOfItsBlob(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	sd, err := pkcs7.NewSignedData(bytes.Repeat([]byte{'x'}, 2<<20))
	if err != nil {
		t.Fatal(err)
	}
	if err := sd.AddSigner(a.cert, a.signer, pkcs7.SignerInfoConfig{}); err != nil {
		t.Fatal(err)
	}
	der, err := sd.Finish()
	if err != nil {
		t.Fatal(err)
	}
	objs := []sobj{{num: 900, body: "<" + fmt.Sprintf("%x", der) + ">"}}
	const m = 100
	for k := 0; k < m; k++ {
		objs = append(objs, sobj{num: 1000 + k, body: "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange [0 10 20 30] /Contents 900 0 R >>"})
	}
	doc := synthRevision(t, base, objs, catalogOf(t, base))
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	revs, err := sweepRevisions(doc)
	runtime.GC()
	runtime.ReadMemStats(&after)
	withProof := 0
	for i := range revs {
		if revs[i].proof != nil {
			withProof++
		}
	}
	if withProof < m {
		t.Fatalf("stimulus: %d of %d records parsed a signer (sweep err %v), so nothing was retained to measure", withProof, m, err)
	}
	if grew := int64(after.HeapAlloc) - int64(before.HeapAlloc); grew > 32<<20 {
		t.Errorf("records over one shared 2 MB blob keep %d MB live: each holds a copy of it", grew>>20)
	}
	runtime.KeepAlive(revs)
}

// TestTheScreenAgreesWithTheLibraryOnEveryShapeItVerifies — a shape the library verifies and the screen refuses throws
// a genuine version away on the screened path (P02.S01's re-review): Ed25519, and signers with no signed attributes,
// which sign the content itself and are checked over the bytes their ranges select.
func TestTheScreenAgreesWithTheLibraryOnEveryShapeItVerifies(t *testing.T) {
	content := bytes.Repeat([]byte("signed bytes "), 50)
	pdf := append(append([]byte(nil), content...), "trailing"...)
	whole := []int64{0, int64(len(content))}
	other := []int64{0, 10}
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, ek, _ := ed25519.GenerateKey(rand.Reader)
	type keyed struct {
		key any
		pub any
	}
	keys := map[string]keyed{"ECDSA": {ec, &ec.PublicKey}, "RSA": {rk, &rk.PublicKey}, "Ed25519": {ek, ek.Public()}}
	for name, k := range keys {
		for _, bare := range []bool{false, true} {
			tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "t"},
				NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
			der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, k.pub, k.key)
			if err != nil {
				t.Fatal(err)
			}
			cert, _ := x509.ParseCertificate(der)
			sd, err := pkcs7.NewSignedData(content)
			if err != nil {
				t.Fatal(err)
			}
			if bare {
				err = sd.SignWithoutAttr(cert, k.key, pkcs7.SignerInfoConfig{})
			} else {
				err = sd.AddSigner(cert, k.key, pkcs7.SignerInfoConfig{})
			}
			if err != nil {
				t.Logf("%s bare=%v: the library cannot make this shape (%v) — nothing to agree with", name, bare, err)
				continue
			}
			sd.Detach()
			raw, err := sd.Finish()
			if err != nil {
				t.Fatal(err)
			}
			p7, err := pkcs7.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			p7.Content = content
			if err := p7.Verify(); err != nil {
				t.Logf("%s bare=%v: the library does not verify its own shape (%v) — nothing to agree with", name, bare, err)
				continue
			}
			p7.Content = nil
			rec := Revision{proof: proofOf(p7), ByteRange: whole}
			budget := int64(1 << 30)
			if !(candidate{proposers: []*Revision{&rec}}).screen(pdf, &budget) {
				t.Errorf("%s bare=%v: the library verifies it and the screen refuses it", name, bare)
			}
			rec.ByteRange = other
			if (candidate{proposers: []*Revision{&rec}}).screen(pdf, &budget) {
				t.Errorf("%s bare=%v: the screen passes ranges selecting other bytes", name, bare)
			}
		}
	}
}

// TestTheScreenResolvesAlgorithmsAsTheLibraryDoes — `rsaEncryption`, `sha256WithRSA` and the curve OIDs all take their
// hash from the digest algorithm in the library (`pkcs7` `getSignatureAlgorithm`), so `sha256WithRSA` over a SHA-384
// digest verifies there and must here; and the screen agrees with the library's own Verify on each.
func TestTheScreenResolvesAlgorithmsAsTheLibraryDoes(t *testing.T) {
	content := bytes.Repeat([]byte("signed bytes "), 50)
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	for name, k := range map[string]struct {
		key, pub any
		generic  asn1.ObjectIdentifier
	}{
		"rsaEncryption": {rk, &rk.PublicKey, pkcs7.OIDEncryptionAlgorithmRSA},
		"sha256WithRSA": {rk, &rk.PublicKey, pkcs7.OIDEncryptionAlgorithmRSASHA256},
		"curve P-256":   {ec, &ec.PublicKey, pkcs7.OIDEncryptionAlgorithmECDSAP256},
	} {
		for _, digest := range []asn1.ObjectIdentifier{pkcs7.OIDDigestAlgorithmSHA256, pkcs7.OIDDigestAlgorithmSHA384} {
			tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "t"},
				NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
			der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, k.pub, k.key)
			if err != nil {
				t.Fatal(err)
			}
			cert, _ := x509.ParseCertificate(der)
			sd, _ := pkcs7.NewSignedData(content)
			sd.SetDigestAlgorithm(digest)
			if err := sd.AddSigner(cert, k.key, pkcs7.SignerInfoConfig{}); err != nil {
				t.Fatal(err)
			}
			sd.Detach()
			raw, _ := sd.Finish()
			p7, err := pkcs7.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			p7.Signers[0].DigestEncryptionAlgorithm.Algorithm = k.generic
			p7.Content = content
			lib := p7.Verify() == nil
			p7.Content = nil
			if !lib {
				t.Fatalf("stimulus: the library does not verify %s over digest %v, so there is nothing to agree with", name, digest)
			}
			if !proofOf(p7).attributed() {
				t.Errorf("%s over digest %v: the library verifies it and the screen does not attribute it", name, digest)
			}
		}
	}
}

// foundEarlier checks a version the boundary walk found: it says so, and names the redefined object when the whole file
// was read — and names none (redef 0) when the file could not be read, since nothing is then known of what it redefined.
func foundEarlier(redef int) func(t *testing.T, got SignedRevision) {
	return func(t *testing.T, got SignedRevision) {
		t.Helper()
		if !got.EarlierRevision {
			t.Error("the version came from an earlier revision and the result does not say so")
		}
		if got.RedefinedObj != uint32(redef) {
			t.Errorf("RedefinedObj %d, want %d: the later revision's rewrite is evidence and must be named", got.RedefinedObj, redef)
		}
	}
}

// TestRevisionBoundariesAreWhatTheXrefSays — the walk's population, on bytes built to each rule.
func TestRevisionBoundariesAreWhatTheXrefSays(t *testing.T) {
	body := "%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n"
	xrefAt := len(body)
	doc := body + "xref\n0 1\n0000000000 65535 f \ntrailer\n<< /Root 1 0 R >>\nstartxref\n" + strconv.Itoa(xrefAt) + "\n%%EOF\n"
	first := len(doc)
	streamAt := len(doc) + 1 // pdfsign's offset points one byte early, at the newline before the header
	doc += "\n7 0 obj\n<< /Type /XRef /Size 8 >>\nstream\nx\nendstream\nendobj\nstartxref\n" + strconv.Itoa(streamAt-1) + "\n%%EOF"
	second := len(doc)
	doc += "\n9 0 obj\n<< /Foo 1 >>\nstream\nstartxref\n" + strconv.Itoa(len(doc)+1) + "\n%%EOF\nendstream\nendobj\n" // names a non-xref object
	doc += "startxref\n" + strconv.Itoa(xrefAt) + "\n%%EOF\r\n"                                                       // a second marker naming the first xref: collapses to the first
	doc += "startxref\n0\n%%EOF\n"                                                                                    // a linearized first page names nothing
	got := revisionBoundaries([]byte(doc))
	has := map[int64]bool{}
	for _, e := range got {
		has[e] = true
	}
	for name, e := range map[string]int64{"an xref stream reached past white space, bare": int64(second),
		"a classic xref, through its \\n": int64(first)} {
		if !has[e] {
			t.Errorf("%s: end %d missing from %v", name, e, got)
		}
	}
	if len(got) > 0 && got[0] < got[len(got)-1] {
		t.Errorf("boundaries %v are not newest first", got)
	}
	for _, e := range got {
		if e > int64(second)+2 {
			t.Errorf("end %d: a marker naming a non-xref object, a repeat of the first xref, or startxref 0 was kept", e)
		}
	}
	ends := rawByteRangeEnds([]byte("/ByteRange [0 10 20 30] /ByteRange[0 1 2 x] /ByteRange [0 10 20 -3] " +
		"/ByteRange [0 1 + 2 4] /ByteRange [ 0 5 7 8 ]" + strings.Repeat(" ", 60)))
	if !ends[50] || !ends[15] || len(ends) != 2 {
		t.Errorf("rawByteRangeEnds %v, want {50, 15}: only whole, non-negative, all-integer literal arrays end anywhere", ends)
	}
}

// TestTheBoundaryWalkIsBounded — markers a hostile file appends, measured by what the walk does with them.
func TestTheBoundaryWalkIsBounded(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	a := newIdentity(t, "Alice")
	sA := signAs(t, base, a, "original")
	vnum, _ := victimDict(t, sA)
	doc := synthRevision(t, sA, []sobj{{num: vnum, body: "<< /Foo 2 >>"}}, catalogOf(t, sA))
	xo := bytes.LastIndex(sA, []byte("startxref"))
	xref, err := strconv.Atoi(string(bytes.TrimSpace(sA[xo+9 : bytes.LastIndex(sA, []byte("%%EOF"))])))
	if err != nil {
		t.Fatal(err)
	}
	// Each hostile marker sits inside a stream with a raw ByteRange literal ending exactly at it, so it survives both
	// filters. build(end) is laid out twice so the numbers can name the final offsets (fixed-width).
	hostile := func(k int, distinct bool) []byte {
		build := func(ends []int64) []byte {
			var sb strings.Builder
			for i := 0; i < k; i++ {
				target := xref
				if distinct {
					fmt.Fprintf(&sb, "%06d 0 obj\n<</Type/XRef>>", 2000+i)
					target = 0 // patched below to the header's own offset
				}
				fmt.Fprintf(&sb, "/ByteRange [0 10 20 %012d]\nstartxref\n%012d\n%%%%EOF\n", ends[i]-20, target)
			}
			out := synthRevision(t, doc, []sobj{{num: 990, body: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", sb.Len(), sb.String())}}, catalogOf(t, doc))
			if distinct { // point each marker at its own fake header
				for i, at := 0, 0; i < k; i++ {
					h := bytes.Index(out[at:], []byte(fmt.Sprintf("%06d 0 obj", 2000+i))) + at
					sx := bytes.Index(out[h:], []byte("startxref\n")) + h + 10
					copy(out[sx:sx+12], fmt.Sprintf("%012d", h))
					at = sx
				}
			}
			return out
		}
		first := build(make([]int64, k))
		var ends []int64
		for i := len(doc); len(ends) < k; {
			j := bytes.Index(first[i:], []byte("%%EOF\n")) + i
			ends = append(ends, int64(j+6))
			i = j + 6
		}
		return build(ends)
	}
	same := hostile(5000, false)
	if n := len(rawByteRangeEnds(same)); n < 5000 {
		t.Fatalf("stimulus: only %d literal ByteRanges end in the hostile file, so the markers never reach the walk", n)
	}
	got := SignedRevisionFor(same, a.fp)
	if !bytes.Equal(got.Prefix, sA) {
		t.Errorf("5,000 markers naming the genuine xref: cause %q — they queued ahead of the version", got.Cause)
	}
	verifies, budget := 0, int64(1<<40)
	if _, _, screens := walkBoundariesCounted(same, a.fp, &verifies, &budget); screens > 1 {
		t.Errorf("5,000 markers naming one xref cost %d screens; they collapse to the earliest, so one", screens)
	}
	distinct := hostile(maxBoundaryScreens+4, true)
	got = SignedRevisionFor(distinct, a.fp)
	if got.Prefix != nil || got.Cause != RevisionCouldNotCheck {
		t.Errorf("more fake xref sections than the screen cap ahead of the version: cause %q, %d bytes; want could-not-check", got.Cause, len(got.Prefix))
	}
	verifies, budget = 0, int64(1<<40)
	if _, cut, screens := walkBoundariesCounted(distinct, a.fp, &verifies, &budget); screens != maxBoundaryScreens || !cut {
		t.Errorf("%d screens, cut %v; want exactly the cap, %d, and cut", screens, cut, maxBoundaryScreens)
	}
	verifies, budget = 0, int64(1<<40)
	if _, _, screens := walkBoundariesCounted(sA, a.fp, &verifies, &budget); screens != 0 {
		t.Errorf("an honest, untouched document cost %d screens; with no earlier ByteRange-ended boundary it costs none", screens)
	}
}

// walkBoundariesCounted runs the walk the way SignedRevisionFor does, from nothing tried, and reports its screens.
func walkBoundariesCounted(pdf []byte, fp string, verifies *int, budget *int64) (*SignedRevision, bool, int) {
	return walkBoundaries(pdf, fp, map[int64]bool{}, 0, verifies, budget)
}

// TestTheBoundaryScanIsLinear — one long white-space run named by every marker cost each marker the whole run (P02.S02's
// review: 33.8 s for 2,000 markers over 4 MiB, before any cap).
func TestTheBoundaryScanIsLinear(t *testing.T) {
	doc := "%PDF-1.7\n" + strings.Repeat(" ", 4<<20) + strings.Repeat("startxref\n9\n%%EOF\n", 2000)
	done := make(chan time.Duration, 1)
	go func() {
		t0 := time.Now()
		revisionBoundaries([]byte(doc))
		done <- time.Since(t0)
	}()
	select {
	case d := <-done:
		if d > 2*time.Second {
			t.Errorf("2,000 markers naming one 4 MiB white-space run took %v; each marker re-walked the run", d)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the boundary scan did not finish in 20 s")
	}
}

// TestABoundaryEndsThroughEachLineEnding — a signer covers through whichever end-of-line its producer wrote.
func TestABoundaryEndsThroughEachLineEnding(t *testing.T) {
	head := "%PDF-1.7\n1 0 obj\n<<>>\nendobj\n"
	at := len(head)
	for name, eol := range map[string]string{"bare": "", "\\r": "\r", "\\n": "\n", "\\r\\n": "\r\n"} {
		doc := head + "xref\n0 1\n0000000000 65535 f \ntrailer\n<<>>\nstartxref\n" + strconv.Itoa(at) + "\n%%EOF" + eol + "x"
		want := int64(len(doc) - 1)
		found := false
		for _, e := range revisionBoundaries([]byte(doc)) {
			if e == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the end through the line ending, %d, is not offered", name, want)
		}
	}
}
