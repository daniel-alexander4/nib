package p2p

import (
	"errors"
	"strings"
	"testing"
)

// TestEverySignatureAfterTheFirstNamesItsPredecessor — /pending 807 R7.
//
// `Matched` asks whether the accepted party signed this document at ALL, so L3 used to admit a
// chain in which a signature named its successor, or a signer two places back. The readme printed
// into every ceremony document says each names "the signer before it". Each arm moves ONE axis and
// asserts first that the old check is satisfied (`Matched`), so a refusal is about the new rule.
func TestEverySignatureAfterTheFirstNamesItsPredecessor(t *testing.T) {
	a, b, c, d := l3Identity(t, "A"), l3Identity(t, "B"), l3Identity(t, "C"), l3Identity(t, "D")
	r := l3Roster(a, b, c, d)

	// The control: each signature after the first names the one before it, and D may sign.
	good := l3Chain(t, l3Prepared(t), []l3Party{a, b, c}, []l3Party{a, a, b}, "")
	if err := AdmitContribution(good, r, d.fp); err != nil {
		t.Fatalf("control: a chain naming each predecessor was refused: %v", err)
	}

	for _, tc := range []struct {
		name    string
		accepts []l3Party
		bad     int // the signature (0-based) that names the wrong party
	}{
		{"names its successor", []l3Party{a, c, b}, 1},
		{"names a signer two places back", []l3Party{a, a, a}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := l3Chain(t, l3Prepared(t), []l3Party{a, b, c}, tc.accepts, "")
			ats := ReadAttestations(doc)
			if len(ats) != 3 || !ats[tc.bad].Valid || !ats[tc.bad].Matched {
				t.Fatalf("setup: signature %d is not a valid, cross-bound signature, so a refusal "+
					"would not be about whom it names: %+v", tc.bad+1, ats)
			}
			err := AdmitContribution(doc, r, d.fp)
			if !errors.Is(err, ErrPrefixUnproven) {
				t.Fatalf("a chain whose signature %d does not name its predecessor was admitted "+
					"(%v), want ErrPrefixUnproven — the readme says every signature after the "+
					"first names the signer before it", tc.bad+1, err)
			}
		})
	}
}

// TestAnUppercaseRosterFingerprintCrossBinds — /pending 648.
//
// A roster reaches `PredecessorOf` from JSON un-normalised and `safeHex` validates without
// lowercasing, so an uppercase roster signs an uppercase token into `/Reason`. `crossBind` compared
// it with `==` against the library's lowercase fingerprint, and L3 refused every hop after the
// first — permanently, since the token is inside signed bytes.
func TestAnUppercaseRosterFingerprintCrossBinds(t *testing.T) {
	up := func(p l3Party) l3Party { p.fp = strings.ToUpper(p.fp); return p }
	a, b, c := l3Identity(t, "A"), l3Identity(t, "B"), l3Identity(t, "C")
	A, B, C := up(a), up(b), up(c)
	r := l3Roster(A, B, C)

	doc := l3Chain(t, l3Prepared(t), []l3Party{a, b}, []l3Party{A, A}, "")
	ats := ReadAttestations(doc)
	if len(ats) != 2 || ats[1].AcceptedPeer != A.fp {
		t.Fatalf("setup: signature 2 does not carry the UPPERCASE token, so this asserts nothing "+
			"about case: %+v", ats)
	}
	if !ats[1].Matched {
		t.Errorf("signature 2 accepts A in uppercase and A signed; crossBind did not match it")
	}
	if err := AdmitContribution(doc, r, C.fp); err != nil {
		t.Fatalf("an uppercase roster's third signer was refused: %v", err)
	}
}
