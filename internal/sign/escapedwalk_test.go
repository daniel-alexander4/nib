package sign

import (
	"bytes"
	"strings"
	"testing"
)

// TestTheBoundaryWalkReadsASignaturesNamesInEitherSpelling is /pending 862: the walk finds an earlier
// version from the file's raw bytes — every literal ByteRange ending at a revision boundary, the
// `/Contents` of the object around it, and the `/XRef` that makes an object a boundary at all — and
// all three searches read the name as written. A name may spell any character `#xx` (ISO 32000-1
// 7.3.5), so a signature written that way, whose dictionary a later revision then replaced, had no
// version the walk could see.
//
// Measured before the fix: the plain spelling returns the signed version, and each escaped spelling
// read `no-signature` — a wrong cause on the dispute surface, about a signer whose signature is in
// the file.
func TestTheBoundaryWalkReadsASignaturesNamesInEitherSpelling(t *testing.T) {
	a := newIdentity(t, "Alice")
	plain := sigDict("1", "")
	respell := func(s, from, to string) string {
		t.Helper()
		out := strings.Replace(s, from, to, 1)
		if out == s || strings.Contains(out, from) {
			t.Fatalf("STIMULUS: %s was not respelled %s", from, to)
		}
		return out
	}
	// A later revision replaces the signature's dictionary: no record of the file as it stands
	// reaches the signed version, so only the walk can.
	replaced := func(signed []byte) []byte {
		return synthRevision(t, signed, []sobj{{num: 5, body: "<</Foo 2>>"}}, 1)
	}

	for _, c := range []struct {
		name   string
		signed []byte
	}{
		{"every name plain", fillSig(t, synthRevision(t, nil, baseObjs(plain), 1), "1", nil, detached(t, a))},
		{"an escaped /ByteRange",
			fillSig(t, synthRevision(t, nil, baseObjs(respell(plain, "/ByteRange", "/Byte#52ange")), 1), "1", nil, detached(t, a))},
		{"an escaped /XRef on the signed revision's cross-reference stream",
			fillSig(t, []byte(respell(string(synthRevision(t, nil, baseObjs(plain), 1)), "/Type/XRef", "/Type/X#52ef")), "1", nil, detached(t, a))},
	} {
		if st := Verify(c.signed); st.State != Valid || len(st.Signers) != 1 || st.Signers[0].Fingerprint != a.fp {
			t.Fatalf("STIMULUS %s: the signed version reads %v, want valid with Alice's", c.name, st.State)
		}
		got := SignedRevisionFor(replaced(c.signed), a.fp)
		if !bytes.Equal(got.Prefix, c.signed) {
			t.Errorf("%s: SignedRevisionFor reads cause %q with a %d-byte version, want the %d-byte version Alice signed — "+
				"the walk did not find a signature whose name is spelled with an escape", c.name, got.Cause, len(got.Prefix), len(c.signed))
			continue
		}
		if !got.EarlierRevision {
			t.Errorf("%s: the version came from an earlier revision and the result does not say so", c.name)
		}
	}

	// An escaped `/Contents` is a signature nib's own sweep refuses (`ownsGap` reads that name as
	// written), so no version is returned for it either way. What the walk owes is the SAME cause
	// the file reads before the later revision — a signature naming the signer that did not
	// re-verify — and never "there is no signature here".
	for _, to := range []string{"/#43ontents", "/Co#6etents"} {
		signed := fillSig(t, synthRevision(t, nil, baseObjs(respell(plain, "/Contents", to)), 1), "1", nil, detached(t, a))
		if got := SignedRevisionFor(signed, a.fp); got.Cause != RevisionPrefixFailed {
			t.Fatalf("STIMULUS %s: the file as signed reads cause %q, want %q", to, got.Cause, RevisionPrefixFailed)
		}
		if got := SignedRevisionFor(replaced(signed), a.fp); got.Cause != RevisionPrefixFailed {
			t.Errorf("%s, its dictionary then replaced: cause %q, want %q — the walk reads the cause the file had "+
				"before the later revision", to, got.Cause, RevisionPrefixFailed)
		}
	}

	// The other direction: a name that only resembles one of the two is not it, in either spelling.
	for _, pdf := range []string{
		"1 0 obj\n<</Byte#52ang [0 1 2 3]>>\nendobj\n",
		"1 0 obj\n<</Byte#53ange [0 1 2 3]>>\nendobj\n",
		"1 0 obj\n<</Byte#5ange [0 1 2 3]>>\nendobj\n",
	} {
		budget := int64(1 << 20)
		if lits, ok := rawByteRangesEndingAt([]byte(pdf), []int64{5}, &budget); !ok || len(lits) != 0 {
			t.Errorf("rawByteRangesEndingAt(%q) = %v, %v — want no literal: the name is not /ByteRange", pdf, lits, ok)
		}
	}
	for _, c := range []struct {
		span, want string
		ok         bool
	}{
		{"<</Contents <30 82>>>", "30 82", true},
		{"<</#43ontents<3082>>>", "3082", true},
		{"<</Contents 7 0 R/Co#6etents\n<ab>>>", "ab", true}, // the first that is a literal, as the regexp found it
		{"<</Contents <<>>>>", "", false},
		{"<</#43ontent <3082>>>", "", false},
		{"<</Contents <30zz>>>", "", false},
	} {
		if got, ok := rawContents([]byte(c.span)); ok != c.ok || string(got) != c.want {
			t.Errorf("rawContents(%q) = %q, %v — want %q, %v", c.span, got, ok, c.want, c.ok)
		}
	}
}
