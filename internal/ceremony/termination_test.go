package ceremony

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func terminationFixture(t *testing.T) (Record, []byte, []byte) {
	t.Helper()
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	return r, cert, key
}

// TestATerminationBindsExactlyOneProceeding — P08.S04b.
//
// The object is convener-signed and says a proceeding ended. Everything that makes it safe is one
// field: `RosterHash` commits to the version, the convener, the id, the DocHash, the intent, the
// deadline and every roster entry, so a cross-ceremony replay AND /pending 318's same-id
// substitution both fall to a single comparison.
func TestATerminationBindsExactlyOneProceeding(t *testing.T) {
	rec, cert, key := terminationFixture(t)

	term, err := SignTermination(rec, StateDeclined, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	// CONTROL FIRST: an honest object verifies, or every refusal below is a function that refuses
	// everything.
	if err := term.Verify(rec); err != nil {
		t.Fatalf("an honestly minted termination did not verify: %v", err)
	}

	t.Run("a different proceeding is refused", func(t *testing.T) {
		other, _, _ := terminationFixture(t)
		if err := term.Verify(other); !errors.Is(err, ErrBadTermination) {
			t.Errorf("a termination verified against a DIFFERENT proceeding (%v). One convener's "+
				"decline would then end any ceremony they are a party to.", err)
		}
	})

	t.Run("the same id with a different roster is refused", func(t *testing.T) {
		// /pending 318's shape: an attacker who controls the id cannot also match the commitment.
		same, c2, k2 := terminationFixture(t)
		same.ID = rec.ID
		if err := same.Sign(c2, k2); err != nil {
			t.Fatal(err)
		}
		if err := term.Verify(same); !errors.Is(err, ErrBadTermination) {
			t.Errorf("a termination verified against a ceremony that merely shares its id (%v)", err)
		}
	})

	t.Run("a flipped state is refused", func(t *testing.T) {
		flipped := term
		flipped.State = StateCompleted
		if err := flipped.Verify(rec); !errors.Is(err, ErrBadTermination) {
			t.Errorf("declined was rewritten to completed and still verified (%v) — the state "+
				"is a signed chunk precisely so it cannot be", err)
		}
	})

	t.Run("a non-convener signer is refused", func(t *testing.T) {
		c2, k2, _ := identity(t, "Not the convener")
		rogue, err := SignTermination(rec, StateDeclined, c2, k2)
		if err != nil {
			t.Fatal(err)
		}
		if err := rogue.Verify(rec); !errors.Is(err, ErrBadTermination) {
			t.Errorf("a party who is not the convener ended the proceeding (%v)", err)
		}
	})

	t.Run("only two states can be attested", func(t *testing.T) {
		// Expired and abandoned have no convener to sign them — abandoned means the convener never
		// came back — so they are derived locally by every machine. A signable third state would be
		// a claim nobody is in a position to make.
		for _, bad := range []string{"expired", "abandoned", "", "Declined"} {
			if _, err := SignTermination(rec, bad, cert, key); err == nil {
				t.Errorf("%q was accepted as an attestable end state", bad)
			}
		}
	})
}

// TestTheTerminationPreimageHasNoMalleableAxis is T03, and it is what EARNS the absence of a
// `Canonical`/`IsCanonical` pair on this object.
//
// `Record` needs that machinery because two of its axes have second encodings — a timestamp has
// sub-second and timezone renderings, text has case and whitespace — and P07.S02's grill measured
// both. This object was designed with `party` and `When` deliberately removed (each was an attack
// in its own right), and the consequence is that every surviving chunk is fixed-width, a derived
// lowercase-hex fingerprint, raw bytes, or one of two closed literals.
//
// **So the test is: perturb each axis in a way a canonical form would fold, and require the
// preimage to MOVE.** If any perturbation leaves it unchanged, there is a malleable axis after all
// and the missing canonical form is a hole rather than a simplification.
func TestTheTerminationPreimageHasNoMalleableAxis(t *testing.T) {
	rec, cert, key := terminationFixture(t)
	term, err := SignTermination(rec, StateDeclined, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	base, err := term.preimage()
	if err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the preimage must be non-trivial and must carry its domain tag, or "it changed"
	// below is a statement about an empty buffer.
	if len(base) < 64 || !bytes.Contains(base, []byte(terminationDomain)) {
		t.Fatalf("setup: the preimage is %d bytes and %q is %v — this test would be comparing "+
			"nothing", len(base), terminationDomain, bytes.Contains(base, []byte(terminationDomain)))
	}

	// **The axis that must NOT move it, and this is the property rather than an exception.** An
	// upper-case rendering of the same commitment must produce the SAME signed bytes — that is
	// exactly what carrying raw bytes buys, and it is why no canonical form is needed to fold it.
	// The first cut of this test asserted the opposite and failed, which is the error it now pins:
	// case-folding a hex field is not malleability, it is malleability already handled.
	upper := term
	upper.RosterHash = strings.ToUpper(term.RosterHash)
	if got, gerr := upper.preimage(); gerr != nil || !bytes.Equal(base, got) {
		t.Errorf("an upper-case roster hash produced a DIFFERENT preimage (err=%v) — the field is "+
			"decoded to raw bytes precisely so the two renderings sign identically; if they do "+
			"not, hex case is a live malleable axis and this object needs the canonical form it "+
			"does not have", gerr)
	}

	for _, tc := range []struct {
		name string
		mut  func(*Termination)
	}{
		{"the state", func(x *Termination) { x.State = StateCompleted }},
		{"the version", func(x *Termination) { x.Version = 2 }},
		{"the roster hash's VALUE", func(x *Termination) {
			b, _ := hex.DecodeString(x.RosterHash)
			b[0] ^= 0xff
			x.RosterHash = hex.EncodeToString(b)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := term
			tc.mut(&x)
			got, gerr := x.preimage()
			if gerr != nil {
				return // an axis that cannot even be rendered is not malleable
			}
			if bytes.Equal(base, got) {
				t.Errorf("perturbing %s left the preimage byte-identical — that axis has a second "+
					"encoding, which is exactly what a Canonical form exists to fold. This object "+
					"has none, on the argument that no such axis survives; that argument is now "+
					"false and the missing canonical form is a hole.", tc.name)
			}
		})
	}

}

// TestTheTerminationDoorKeepsAbsenceApartFromDamage — T02's three outcomes, driven separately.
//
// Absence is the ORDINARY case: most ceremonies never terminate explicitly, and a convener who
// declines to mint one is indistinguishable from one who has not decided. Reading that as damage
// would put an accusation about the user's own disk under every ordinary ceremony.
func TestTheTerminationDoorKeepsAbsenceApartFromDamage(t *testing.T) {
	root := t.TempDir()
	rec, cert, key := terminationFixture(t)
	if _, err := WriteMirror(root, rec, nil); err != nil {
		t.Fatal(err)
	}

	// ABSENT — and it must not be damage.
	if _, err := ReadTermination(root, rec); !errors.Is(err, ErrNoTermination) {
		t.Errorf("an ordinary ceremony with no termination reported %v, want ErrNoTermination — "+
			"absence must never read as damage", err)
	}

	term, err := SignTermination(rec, StateDeclined, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTermination(root, term); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTermination(root, rec)
	if err != nil {
		t.Fatalf("a written termination did not read back: %v", err)
	}
	if got.State != StateDeclined {
		t.Errorf("the state did not survive: %q", got.State)
	}

	// WRITE-ONCE: the same state again is idempotent, a different one is a conflict.
	if err := WriteTermination(root, term); err != nil {
		t.Errorf("re-writing the same end state was refused: %v", err)
	}
	other, err := SignTermination(rec, StateCompleted, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTermination(root, other); !errors.Is(err, ErrTerminationConflict) {
		t.Errorf("a second termination naming a DIFFERENT state was accepted (%v) — two conveners' "+
			"worth of end state under one id is the substitution /pending 318 exists against, and "+
			"silently taking the later one picks the attacker's", err)
	}
	if after, _ := ReadTermination(root, rec); after.State != StateDeclined {
		t.Error("the original end state was clobbered by the refused write")
	}

	// PRESENT AND UNVERIFIABLE — its own state, never damage-of-the-mirror.
	dir, _ := MirrorDir(root, rec.ID)
	if err := os.WriteFile(filepath.Join(dir, terminationFile), []byte(`{"version":1,"state":"declined"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ReadTermination(root, rec)
	if !errors.Is(err, ErrBadTermination) {
		t.Errorf("a planted termination reported %v, want ErrBadTermination", err)
	}
	if errors.Is(err, ErrMirrorDamaged) {
		t.Error("a planted termination was reported as MIRROR DAMAGE — that word is about this " +
			"machine's own disk, and would tell a user to suspect their hardware for what is " +
			"far more likely a planted file")
	}
}

// TestTheMirrorsOwnDoorsAreUnchangedByATermination — the /pending 321 lesson, asserted.
//
// A fourth file in the ceremony directory is exactly the shape that broke the mirror once: a
// companion written inside `WriteMirror`'s ordered sequence, cross-checked against `document.pdf`
// whose bytes change every hop. This one is safe because it binds the RECORD — whose roster
// commitment is immutable for the ceremony's life — and because `WriteMirror` neither writes nor
// consults it. Both halves are asserted, because the second is the one a later edit would break.
func TestTheMirrorsOwnDoorsAreUnchangedByATermination(t *testing.T) {
	root := t.TempDir()
	// A REAL convened document, because ReadMirror compares it to the record's DocHash while it is
	// unsigned — a hand-written byte string fails that check for its own reason and would make
	// this test report a defect it is not looking for.
	rec, doc := convened(t)
	cert, key, _ := identity(t, "Convener")
	term, err := SignTermination(rec, StateDeclined, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteMirror(root, rec, doc); err != nil {
		t.Fatal(err)
	}
	if err := WriteTermination(root, term); err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the termination must genuinely BE on disk, or every assertion below is about a
	// directory that has three files and proves nothing about the fourth. The first cut of this
	// test dropped the write while keeping the assertions, which is exactly that vacuous shape.
	dir, derr := MirrorDir(root, rec.ID)
	if derr != nil {
		t.Fatal(derr)
	}
	if _, serr := os.Stat(filepath.Join(dir, terminationFile)); serr != nil {
		t.Fatalf("setup: no termination on disk (%v) — nothing below is about a fourth file", serr)
	}

	// A hop still in flight when the convener ends the proceeding must STILL be able to store.
	// Otherwise the receive path renders the failure as "Signed, but not saved — do not close Nib",
	// telling a user to rescue a document over what is really a race with a decline.
	if _, err := WriteMirror(root, rec, append(append([]byte{}, doc...), []byte("hop 2\n")...)); err != nil {
		t.Errorf("WriteMirror refused because a termination exists (%v) — an in-flight Store would "+
			"then print the disk-full sentence for a race with a decline", err)
	}
	// And reading the mirror is untouched by it.
	if _, pdf, err := ReadMirror(root, rec.ID, time.Now()); err != nil || len(pdf) == 0 {
		t.Errorf("ReadMirror broke in the presence of a termination: %v", err)
	}
}
