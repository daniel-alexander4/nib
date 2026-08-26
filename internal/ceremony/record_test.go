package ceremony

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

func identity(t *testing.T, cn string) (certPEM, keyPEM []byte, fp string) {
	t.Helper()
	c, k, err := sign.GenerateIdentity(cn)
	if err != nil {
		t.Fatal(err)
	}
	b, err := sign.Fingerprint(c)
	if err != nil {
		t.Fatal(err)
	}
	return c, k, hex.EncodeToString(b)
}

// draft builds a record for a three-party ceremony with a NON-signing convener, which is
// the shape most of these tests want: `signs:false` is the axis PLAN-2 exists to protect
// and a roster where everyone signs cannot see it.
func draft(t *testing.T, convenerFP string, others ...string) Record {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	r := Record{
		ID:      id,
		DocHash: strings.Repeat("ab", 32),
		Intent:  "We agree to co-sign the lease",
		Expires: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Roster:  []Party{{Fingerprint: convenerFP, Label: "Convener", Signs: false}},
	}
	for i, fp := range others {
		r.Roster = append(r.Roster, Party{Fingerprint: fp, Label: string(rune('A' + i)), Signs: true})
	}
	return r
}

// --- the commitment ----------------------------------------------------------

// TestRosterHashCoversEveryAxis is PLAN-2's specification turned into a check.
//
// The pin exists because "a commitment over all of the above" is a gesture: a slice would
// have decided what that meant and the decision would have been invisible. So every axis
// is varied ALONE and must move the hash. An axis short is the attack.
func TestRosterHashCoversEveryAxis(t *testing.T) {
	_, _, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base := draft(t, cfp, afp)
	want, err := base.RosterHash()
	if err != nil {
		t.Fatal(err)
	}

	_, _, other := identity(t, "Someone else")
	for _, c := range []struct {
		name string
		mut  func(r *Record)
		why  string
	}{
		{"version", func(r *Record) { r.Version = 99 },
			"two format versions could produce the same commitment, so a party running a " +
				"different version would silently agree to a record it parsed differently (D32)"},
		{"id", func(r *Record) { r.ID = "00000000000000000000000000000000" },
			"two ceremonies could share a commitment"},
		{"docHash", func(r *Record) { r.DocHash = strings.Repeat("cd", 32) },
			"the commitment would not name the document, so signatures could attest to a " +
				"proceeding about different bytes"},
		{"intent", func(r *Record) { r.Intent = "We agree to something else entirely" },
			"the thing everyone is agreeing to could be changed without breaking the commitment"},
		{"expires", func(r *Record) { r.Expires = r.Expires.Add(24 * time.Hour) },
			"the deadline is an externally-supplied security parameter (D16 clock 3)"},
		{"a fingerprint", func(r *Record) { r.Roster[1].Fingerprint = other },
			"a party could be swapped for another and every signature would still verify " +
				"against the same commitment"},
		{"the signs flag", func(r *Record) { r.Roster[0].Signs = true },
			"a convener could present one roster to the signers and another to a verifier, " +
				"differing only in who was obliged to sign, and both would hash the same — " +
				"this is the whole class PLAN-2 exists to catch"},
		{"a label", func(r *Record) { r.Roster[1].Label = "Someone Else" },
			"the displayed roster could differ from the committed one"},
		{"roster order", func(r *Record) { r.Roster[0], r.Roster[1] = r.Roster[1], r.Roster[0] },
			"the order IS the signing order (D20), so reordering must break the commitment"},
	} {
		r := base
		r.Roster = append([]Party(nil), base.Roster...)
		c.mut(&r)
		got, err := r.RosterHash()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if string(got) == string(want) {
			t.Errorf("changing %s left the commitment unchanged — %s", c.name, c.why)
		}
	}
}

// TestEveryRecordFieldIsInTheCommitment is the completeness half of the guard above, and it
// exists because that one is a HAND LIST.
//
// `TestRosterHashCoversEveryAxis` names nine axes and says why each matters — which is worth
// keeping, because the reasons are the specification. What it cannot do is notice a TENTH.
// Measured at the P07.S02 grill: the same shape one level down (`inPreimage` over `Party`)
// shipped a field outside the commitment while reading green, and this slice then added
// `Record.DigestVersion`, which the nine-axis list does not mention.
//
// So: the list documents why; this drives what. Every field of Record moves RosterHash, or it
// is named below with its reason.
func TestEveryRecordFieldIsInTheCommitment(t *testing.T) {
	// Deliberately outside the commitment, each with its reason.
	excluded := map[string]string{
		"ConvenerSig": "it IS the signature over this hash — a value cannot be inside the " +
			"preimage it signs",
	}

	certA, _, fpA := identity(t, "Convener A")
	certB, _, fpB := identity(t, "Convener B")
	base := Record{
		Version: FormatVersion, ID: "id", DocHash: strings.Repeat("ab", 32),
		DigestVersion: 3, Intent: "intent",
		Expires:      time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Roster:       []Party{{Fingerprint: fpA, Label: "Convener", Signs: false}},
		ConvenerCert: string(certA),
	}

	ty := reflect.TypeOf(Record{})
	if ty.NumField() == 0 {
		t.Fatal("Record has no fields — this guard would pass vacuously")
	}
	moved := 0
	for i := 0; i < ty.NumField(); i++ {
		f := ty.Field(i)
		if excluded[f.Name] != "" {
			continue
		}
		var mutate func(r *Record)
		switch f.Name {
		case "ConvenerCert":
			// Two REAL certs: convenerFingerprint returns "" for anything unparseable, so two
			// junk strings would both hash as empty and this row would fail for the wrong
			// reason. The roster gains B so the record stays internally coherent.
			mutate = func(r *Record) {
				r.ConvenerCert = string(certB)
				r.Roster = []Party{{Fingerprint: fpB, Label: "Convener", Signs: false}}
			}
		case "Roster":
			mutate = func(r *Record) {
				r.Roster = append(append([]Party(nil), r.Roster...),
					Party{Fingerprint: strings.Repeat("22", 32), Label: "A", Signs: true})
			}
		default:
			switch f.Type.Kind() {
			case reflect.String:
				mutate = func(r *Record) {
					reflect.ValueOf(r).Elem().FieldByName(f.Name).SetString("something else")
				}
			case reflect.Int:
				mutate = func(r *Record) {
					reflect.ValueOf(r).Elem().FieldByName(f.Name).SetInt(99)
				}
			default:
				if f.Type == reflect.TypeOf(time.Time{}) {
					mutate = func(r *Record) { r.Expires = r.Expires.Add(24 * time.Hour) }
					break
				}
				t.Fatalf("Record.%s is a %s and this guard has no mutation rule for that kind. "+
					"Add one — a field it cannot vary is a field it cannot cover.", f.Name, f.Type)
			}
		}
		want, err := base.RosterHash()
		if err != nil {
			t.Fatal(err)
		}
		r := base
		mutate(&r)
		got, err := r.RosterHash()
		if err != nil {
			t.Fatalf("Record.%s: %v", f.Name, err)
		}
		if bytes.Equal(got, want) {
			t.Errorf("Record.%s varies and RosterHash does NOT move, so the field is OUTSIDE the "+
				"commitment — two records differing in it carry one valid ConvenerSig. Add it to "+
				"rosterPreimage, or name it in `excluded` with why.", f.Name)
			continue
		}
		moved++
	}
	if moved == 0 {
		t.Fatal("no Record field moved RosterHash — the preimage is not reading the record, so " +
			"this guard measured nothing")
	}
	for f := range excluded {
		if _, ok := ty.FieldByName(f); !ok {
			t.Errorf("`excluded` names %q and Record has no such field, so the exclusion covers "+
				"nothing and is quietly weakening this guard.", f)
		}
	}
}

// TestRosterHashIsNotAmbiguousAcrossFieldBoundaries: the axes are length-prefixed, so two
// records that differ only in where a boundary falls must not collide.
//
// Without prefixes, a label ending in "x" followed by an empty next field is
// indistinguishable from an empty label followed by a field beginning "x" — the classic
// concatenation ambiguity, and it is exactly how a crafted label forges a commitment.
func TestRosterHashIsNotAmbiguousAcrossFieldBoundaries(t *testing.T) {
	_, _, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")

	one := draft(t, cfp, afp)
	one.Roster[1].Label = "AB"
	two := draft(t, cfp, afp)
	two.ID = one.ID
	two.Roster[1].Label = "A"
	two.Roster = append(two.Roster, Party{Fingerprint: afp, Label: "B", Signs: true})

	h1, err := one.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := two.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	if string(h1) == string(h2) {
		t.Error("two records differing only in where a field boundary falls hash the same — " +
			"the axes are being concatenated without length prefixes, and a crafted label can " +
			"forge a commitment")
	}
}

// TestEveryPartyFieldIsInTheCommitment — EVERY field of a roster entry is inside
// `rosterPreimage`, or it is named here with its reason.
//
// A field added to Party and NOT added to the preimage sits silently outside the commitment:
// the copy the signers read and the copy a verifier reads can differ in it and both hash the
// same. That is precisely what `Party.Name` was for three phases (see Party's own doc), and
// no test in this package could see it, because a test that asserts ONE named field's
// exclusion says nothing about the next field somebody adds.
//
// **Two rewrites, and the second is the one that matters.** It replaced an exclusion test in
// 2026-08-22 (D21) — `TestTheNameIsNotInTheCommitment`, which could not survive `Name`'s
// deletion. Then, at the **P07.S02 grill (2026-08-24), it was found to be a CLAIM rather than
// a MEASUREMENT** and rewritten again.
//
// What it was: a hand-maintained `inPreimage` map, checked against `reflect.TypeOf(Party{})`
// — never against `rosterPreimage` itself. So it compared one restatement of the preimage to
// the struct, and the function it was about was not in the loop at all. Measured on a
// pristine export at the grill: adding `Capacity` to `Party` and to `inPreimage` ONLY, with
// `rosterPreimage` untouched, shipped **green** — with `Director` and `Witness` hashing
// identically. The guard's own failure message pointed the implementer at the map ("Add it to
// rosterPreimage, OR name it in `excluded`"), which is the cheaper of the two edits.
//
// What it is now: for every field of Party, vary THAT FIELD ALONE and require RosterHash to
// move. There is no list to keep in step, because the preimage is driven rather than
// described. A field added to Party and not to rosterPreimage goes red on the field's own
// name, in the same commit that adds it.
//
// **The stimulus assertion moved with the rewrite**, and that is not bookkeeping: the old one
// asserted the struct's field set was non-empty, which is the wrong axis — it could not tell a
// preimage that reads the roster from one that ignores it. The new one requires that at least
// one field actually MOVED the hash.
//
// The `excluded` map survives for a genuine future exclusion, and it now costs something to
// use: an entry must say why, and the inverse loop below checks it still names a real field.
func TestEveryPartyFieldIsInTheCommitment(t *testing.T) {
	// Deliberately outside the commitment, each with its reason. EMPTY is the correct state:
	// `Name` is the only entry this map would ever have carried and it is not a field any
	// more. An unexplained entry is how the next one gets parked and forgotten.
	excluded := map[string]string{}

	ty := reflect.TypeOf(Party{})
	if ty.NumField() == 0 {
		t.Fatal("Party has no fields — every loop below would pass vacuously")
	}
	moved := 0
	for i := 0; i < ty.NumField(); i++ {
		f := ty.Field(i)
		if reason := excluded[f.Name]; reason != "" {
			continue
		}
		a, b := twoPartyValues(t, f)
		ra := Record{
			ID: "id", DocHash: strings.Repeat("cd", 32), Intent: "intent",
			Expires: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
			Roster:  []Party{basePartyWith(t, f.Name, a)},
		}
		rb := ra
		rb.Roster = []Party{basePartyWith(t, f.Name, b)}
		ha, err := ra.RosterHash()
		if err != nil {
			t.Fatalf("Party.%s: rosterHash over value A: %v", f.Name, err)
		}
		hb, err := rb.RosterHash()
		if err != nil {
			t.Fatalf("Party.%s: rosterHash over value B: %v", f.Name, err)
		}
		if bytes.Equal(ha, hb) {
			t.Errorf("Party.%s varies (%v vs %v) and RosterHash does NOT move, so the field is "+
				"OUTSIDE the commitment. The copy the signers read and the copy a verifier reads "+
				"can differ in it while both hash the same — which is what Party.Name was. Add it "+
				"to rosterPreimage's per-party loop, or name it in `excluded` with why.",
				f.Name, a, b)
			continue
		}
		moved++
	}
	// The stimulus assertion, and it is aimed at the right axis this time: if NOTHING moved
	// the hash, rosterPreimage is not reading the roster at all and every row above passed
	// for a reason that has nothing to do with per-field coverage.
	if moved == 0 {
		t.Fatal("no Party field moved RosterHash — rosterPreimage is not digesting the roster, " +
			"so this guard measured nothing")
	}
	for f := range excluded {
		if _, ok := ty.FieldByName(f); !ok {
			t.Errorf("`excluded` names %q and Party has no such field, so the exclusion covers "+
				"nothing and is quietly weakening this guard.", f)
		}
	}
}

// basePartyWith returns a valid Party with one field overridden.
//
// Valid matters: rosterPreimage refuses a fingerprint that is not 32 raw bytes of hex
// (record.go), so a naive reflect.Zero would make every row fail for the wrong reason.
func basePartyWith(t *testing.T, field string, v any) Party {
	t.Helper()
	p := Party{Fingerprint: strings.Repeat("11", 32), Label: "base", Signs: true}
	rv := reflect.ValueOf(&p).Elem().FieldByName(field)
	if !rv.IsValid() || !rv.CanSet() {
		t.Fatalf("Party.%s cannot be set through reflect — this guard cannot cover it", field)
	}
	rv.Set(reflect.ValueOf(v))
	return p
}

// twoPartyValues gives two distinct, VALID values for one Party field.
//
// Per-field rather than per-kind for Fingerprint, because "two distinct strings" is not two
// distinct fingerprints and the preimage would refuse them.
func twoPartyValues(t *testing.T, f reflect.StructField) (any, any) {
	t.Helper()
	if f.Name == "Fingerprint" {
		return strings.Repeat("11", 32), strings.Repeat("22", 32)
	}
	switch f.Type.Kind() {
	case reflect.String:
		return "alpha", "beta"
	case reflect.Bool:
		return false, true
	default:
		t.Fatalf("Party.%s is a %s and this guard has no two-distinct-values rule for that kind. "+
			"Add one — a field this guard cannot vary is a field it cannot cover.", f.Name, f.Type.Kind())
		return nil, nil
	}
}

// TestEveryPartyFieldIsComparedByMatchesRecord is the twin of the guard above, on the
// COMPARISON side, and it exists because the two sides were asymmetric.
//
// The commitment side had a per-field structural guard (however weak — see above). The
// comparison side has a four-row hand table (invitation_test.go) that names `fingerprint
// swapped`, `signs flipped`, `party added` and `id changed`. `Label` is inside the preimage
// and is compared by nothing; a second such field would have been the second silent one.
//
// Why an uncompared field matters: the invitation is UNSIGNED. The record is the only signed
// copy of the roster, so a field MatchesRecord skips is a field a tampered invitation can
// disagree with the record about, forever, with every check green.
func TestEveryPartyFieldIsComparedByMatchesRecord(t *testing.T) {
	// Named exclusions only, each with its reason.
	excluded := map[string]string{}

	ty := reflect.TypeOf(Party{})
	if ty.NumField() == 0 {
		t.Fatal("Party has no fields — this guard would pass vacuously")
	}
	cert, key, cfp := identity(t, "Convener")
	caught := 0
	for i := 0; i < ty.NumField(); i++ {
		f := ty.Field(i)
		if excluded[f.Name] != "" {
			continue
		}
		a, b := twoPartyValues(t, f)
		rec := draft(t, cfp, strings.Repeat("33", 32))
		rec.Roster[1] = basePartyWith(t, f.Name, a)
		if err := rec.Sign(cert, key); err != nil {
			t.Fatal(err)
		}
		invs, err := NewInvitations(rec)
		if err != nil {
			t.Fatalf("Party.%s: %v", f.Name, err)
		}
		var inv Invitation
		for _, v := range invs {
			inv = v
		}
		// The tamper: the party's row in the INVITATION says something the signed record does
		// not. Nothing signs the invitation, so this is a one-byte edit on the wire.
		inv.Roster = append([]Party(nil), inv.Roster...)
		inv.Roster[1] = basePartyWith(t, f.Name, b)
		if err := inv.MatchesRecord(rec); err == nil {
			t.Errorf("Party.%s differs between the invitation (%v) and the signed record (%v) "+
				"and MatchesRecord ACCEPTS it. The invitation is unsigned, so a field it does not "+
				"compare is one a tamperer owns. Compare it, or name it in `excluded` with why.",
				f.Name, b, a)
			continue
		}
		caught++
	}
	if caught == 0 {
		t.Fatal("MatchesRecord refused nothing for any field — the tamper is not reaching it, " +
			"so this guard measured nothing")
	}
}

// TestAVerifiedRecordIsCanonical drives the two axes the preimage NORMALISES, which are the
// two places a stored record can differ from what its own commitment binds.
//
// Both were measured at the P07.S02 grill against the tree as it then stood, and both
// produced a valid signature over a record that was not the record on disk. See Canonical's
// doc for the harm; this is the check that makes it unrepresentable.
//
// The sub-clauses are separate on purpose. A single "is it canonical" assertion would go
// green the moment ONE axis were fixed, and the case axis is the loud one — so the
// sub-second axis is exactly the one that would have been left behind.
func TestAVerifiedRecordIsCanonical(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")

	t.Run("signing canonicalises both axes", func(t *testing.T) {
		r := draft(t, cfp, afp)
		r.Roster[1].Fingerprint = strings.ToUpper(afp)
		r.Expires = time.Date(2026, 9, 1, 12, 0, 0, 500_000_000, time.FixedZone("x", 5*3600))
		if err := r.Sign(cert, key); err != nil {
			t.Fatal(err)
		}
		if got := r.Roster[1].Fingerprint; got != afp {
			t.Errorf("Sign left a non-lowercase fingerprint %q; the preimage hex-decodes it, so "+
				"the stored case is outside the commitment and two records share one signature", got)
		}
		if r.Expires.Nanosecond() != 0 {
			t.Errorf("Sign left sub-second precision (%s); the preimage renders RFC3339 to the "+
				"second, so those digits are outside the commitment", r.Expires.Format(time.RFC3339Nano))
		}
		if r.Expires.Location() != time.UTC {
			t.Errorf("Sign left location %v; the preimage renders .UTC(), so the zone is outside "+
				"the commitment", r.Expires.Location())
		}
		if err := r.Verify(time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatalf("a record Sign canonicalised must verify: %v", err)
		}

		// **The byte round trip, which is the clause as written and is stronger than the
		// field checks above.** Canonical form's whole purpose is that the STORED bytes are
		// derivable from what the commitment binds — so encoding a verified record, decoding
		// it, and encoding it again must produce identical bytes. A field-by-field check can
		// pass while some axis nobody thought to assert still moves under a round trip.
		enc1, err := r.Encode()
		if err != nil {
			t.Fatal(err)
		}
		back, err := Decode(enc1)
		if err != nil {
			t.Fatal(err)
		}
		enc2, err := back.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(enc1, enc2) {
			t.Errorf("a verified record does not survive an encode/decode/encode round trip "+
				"byte-identically:\n first: %s\nsecond: %s", enc1, enc2)
		}
		// And the decoded copy must still be canonical and still verify — otherwise the
		// round trip is what makes a record non-canonical, which is worse than not having
		// the rule.
		if !back.IsCanonical() {
			t.Error("a canonical record decoded from its own JSON is no longer canonical")
		}
		if err := back.Verify(time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Errorf("a canonical record does not verify after a JSON round trip: %v", err)
		}
	})

	// The refusal half. Sign canonicalises, so a non-canonical record can only arrive from
	// somewhere else — which is exactly the case that matters, because a record arrives from
	// another party over the wire.
	for _, c := range []struct {
		name string
		mut  func(r *Record)
		why  string
	}{
		{"an uppercase roster fingerprint", func(r *Record) {
			r.Roster[1].Fingerprint = strings.ToUpper(r.Roster[1].Fingerprint)
		}, "the preimage hex-decodes fingerprints, so case is folded and both forms carry one " +
			"valid ConvenerSig — while MatchesRecord compares the strings and refuses one of them"},
		{"a sub-second deadline", func(r *Record) {
			r.Expires = r.Expires.Add(500 * time.Millisecond)
		}, "the preimage renders RFC3339 to the second, so the fractional part is unsigned"},
		{"a non-UTC deadline", func(r *Record) {
			r.Expires = r.Expires.In(time.FixedZone("elsewhere", 5*3600))
		}, "the preimage renders .UTC(), so the stored zone is unsigned"},
	} {
		t.Run("refused: "+c.name, func(t *testing.T) {
			r := draft(t, cfp, afp)
			if err := r.Sign(cert, key); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
			// Setup assertion: it must verify BEFORE the mutation, or the refusal below could
			// be for any reason at all.
			if err := r.Verify(now); err != nil {
				t.Fatalf("setup: the signed record must verify before the mutation: %v", err)
			}
			c.mut(&r)
			// The mutation must NOT break the signature — that is the whole point. If it did,
			// the axis would already be committed and this row would be testing nothing.
			h, err := r.RosterHash()
			if err != nil {
				t.Fatal(err)
			}
			sig, err := hex.DecodeString(r.ConvenerSig)
			if err != nil {
				t.Fatal(err)
			}
			if err := sign.VerifyDigest(h, sig, []byte(r.ConvenerCert)); err != nil {
				t.Fatalf("%s broke the signature, so this axis is already inside the commitment "+
					"and this row is vacuous", c.name)
			}
			if err := r.Verify(now); !errors.Is(err, ErrNotCanonical) {
				t.Errorf("%s produced a record whose signature still verifies and Verify said %v "+
					"— want ErrNotCanonical. %s", c.name, err, c.why)
			}
		})
	}
}

// TestRosterHashGoldenVector pins the commitment to a literal.
//
// **There was no golden vector anywhere in this package, and no test pinned FormatVersion to
// a literal** (found at the P07.S02 grill, 2026-08-24) — every reference compared the
// constant to itself. So `rosterPreimage` could change shape with the whole repo green, and
// the version guarding it could fail to move with equal silence. That is the one failure a
// per-field guard cannot see: it proves each field is covered and says nothing about the
// bytes, the order, or the domain tag.
//
// When this test goes red, the preimage changed. That is either a bug or a format bump — and
// a format bump means FormatVersion moves and the vector below is re-cut in the same commit.
func TestRosterHashGoldenVector(t *testing.T) {
	// FormatVersion pinned as a LITERAL, deliberately. Comparing the constant to itself is
	// what the rest of the suite already does and it cannot fail.
	if FormatVersion != 4 {
		t.Fatalf("FormatVersion is %d and this vector was cut for 4. If the format changed on "+
			"purpose, re-cut the vector below in the same commit; if not, this is the bug.",
			FormatVersion)
	}
	r := Record{
		Version: FormatVersion,
		ID:      "0123456789abcdef0123456789abcdef",
		DocHash: strings.Repeat("ab", 32),
		// A LITERAL, not pdfops.ContentDigestVersion. The vector pins the preimage's SHAPE;
		// wiring it to the constant would let a digest-rule bump slide the vector along with
		// it and quietly stop pinning anything.
		DigestVersion: 3,
		Intent:        "We agree to co-sign the lease",
		Expires:       time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Roster: []Party{
			{Fingerprint: strings.Repeat("11", 32), Label: "Convener", Signs: false},
			{Fingerprint: strings.Repeat("22", 32), Label: "A", Signs: true},
		},
	}
	h, err := r.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	const want = "bb8ec37b8640287d871534607a23fd2e81da6508eb6b46173c97e67fe663b056"
	got := hex.EncodeToString(h)
	if got != want {
		t.Fatalf("RosterHash over the pinned record is %s and the vector says %s — the preimage "+
			"changed. Either this is the bug, or the format moved and both this vector and "+
			"FormatVersion are re-cut together.", got, want)
	}
}

// --- the convener signature ---------------------------------------------------

func TestRecordSignsAndVerifies(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if r.Version != FormatVersion {
		t.Errorf("Sign left the version at %d, want %d — the version is the first axis of the "+
			"preimage, so an unset one commits to the wrong thing", r.Version, FormatVersion)
	}
	if err := r.Verify(time.Now()); err != nil {
		t.Fatalf("a freshly signed record does not verify: %v", err)
	}
	if c, ok := r.Convener(); !ok || c.Signs {
		t.Errorf("Convener() = %+v, ok=%v — want the non-signing roster entry", c, ok)
	}
}

// TestATamperedRecordIsRefusedByName covers the acceptance clause: "a record whose convener
// signature does not verify is refused before any pairing, with a distinct message".
//
// Distinct matters. A malformed record is a broken file; a record whose signature fails is
// a record somebody changed. Reporting both as "could not read the ceremony" tells the user
// to try again in one case and to stop in the other.
func TestATamperedRecordIsRefusedByName(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if err := r.Verify(time.Now()); err != nil {
		t.Fatalf("setup: the record does not verify before tampering: %v", err)
	}

	for _, c := range []struct {
		name string
		mut  func(r *Record)
	}{
		{"the intent", func(r *Record) { r.Intent = "We agree to something nobody said" }},
		{"a fingerprint", func(r *Record) { r.Roster[1].Fingerprint = strings.Repeat("11", 32) }},
		{"the signs flag", func(r *Record) { r.Roster[0].Signs = true }},
		{"the deadline", func(r *Record) { r.Expires = r.Expires.Add(365 * 24 * time.Hour) }},
	} {
		bad := r
		bad.Roster = append([]Party(nil), r.Roster...)
		c.mut(&bad)
		err := bad.Verify(time.Now())
		if !errors.Is(err, ErrBadConvenerSignature) {
			t.Errorf("altering %s gave %v, want ErrBadConvenerSignature", c.name, err)
		}
	}
}

// TestARecordSignedByAnOutsiderIsRefusedDifferently: a signature that verifies but belongs
// to nobody in the roster is a well-formed lie, not a corruption, and says so.
func TestARecordSignedByAnOutsiderIsRefusedDifferently(t *testing.T) {
	_, _, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	outCert, outKey, _ := identity(t, "Outsider")

	r := draft(t, cfp, afp)
	if err := r.Sign(outCert, outKey); err != nil {
		t.Fatal(err)
	}
	err := r.Verify(time.Now())
	if !errors.Is(err, ErrConvenerNotInRoster) {
		t.Errorf("a record signed by someone outside its roster gave %v, want "+
			"ErrConvenerNotInRoster — it is internally consistent and describes a proceeding "+
			"its own signer is not part of, which is a different failure from tampering", err)
	}
	if errors.Is(err, ErrBadConvenerSignature) {
		t.Error("the two failures are indistinguishable, so a user cannot tell a corrupted " +
			"record from a forged one")
	}
}

// TestAnUnknownVersionIsRefused (D32): the version is the first axis, and a record from a
// future format must be refused rather than parsed optimistically.
func TestAnUnknownVersionIsRefused(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	r.Version = FormatVersion + 1
	if err := r.Verify(time.Now()); !errors.Is(err, ErrVersion) {
		t.Errorf("a record claiming version %d gave %v, want the version error",
			r.Version, err)
	}
}

// TestTheVersionIsWrittenAtCreation is the D32 pin, driven by reading it back rather than
// by trusting Sign to have set it.
//
// P07's skew bullet tests two versions meeting; it says nothing about whether the field
// existed three phases earlier — and rosterHash's preimage BEGINS with it, so a record
// written without one commits to zero and every later comparison is against the wrong
// number.
func TestTheVersionIsWrittenAtCreation(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp) // deliberately not setting Version
	if r.Version != 0 {
		t.Fatal("setup: the draft already carries a version, so Sign writing one proves nothing")
	}
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	b, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.Version != FormatVersion {
		t.Errorf("the encoded record carries version %d, want %d — read back off the bytes, "+
			"not off the struct that was signed", back.Version, FormatVersion)
	}
	if err := back.Verify(time.Now()); err != nil {
		t.Errorf("the record does not survive an encode/decode round trip: %v", err)
	}
}

// --- the document ------------------------------------------------------------

// TestARecordSurvivesIncrementalSignatures is caveat 10, driven rather than read out of
// pdfcpu's documentation — and the arc's own failure-mode #1 is a "verified" claim about a
// dependency that was never re-verified.
//
// Three incremental signatures, and the record must still be readable, still verify, and
// the document must still hash to what the record says. The last is the hop-4 clause: the
// convener's own bytes satisfy a round-trip without anyone recomputing anything, so the
// recompute has to happen after somebody else has signed.
func TestARecordSurvivesIncrementalSignatures(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	aCert, aKey, afp := identity(t, "A")
	bCert, bKey, bfp := identity(t, "B")

	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	r := draft(t, cfp, afp, bfp)
	hash, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	r.DocHash = hash
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	doc, err := Embed(base, r)
	if err != nil {
		t.Fatal(err)
	}

	// The stimulus, asserted before anything is graded: the record really is in there and
	// really checks out before a single signature is added.
	if _, err := CheckDocument(doc, time.Now()); err != nil {
		t.Fatalf("setup: the freshly embedded record does not check out: %v", err)
	}

	invisible := doc
	for i, id := range []struct{ cert, key []byte }{{aCert, aKey}, {bCert, bKey}, {aCert, aKey}} {
		invisible, err = sign.SignApproval(invisible, id.cert, id.key, sign.Options{
			Name:   "signer",
			Reason: "co-signing",
			When:   time.Now(),
		})
		if err != nil {
			t.Fatalf("signature %d: %v", i+1, err)
		}
	}

	// The stimulus for the assertion below: there really are three signatures on it now.
	st := sign.Verify(invisible)
	if n := len(st.Signers); n != 3 {
		t.Fatalf("setup: the document carries %d signatures, want 3 — the recompute below "+
			"would not be crossing any incremental updates", n)
	}
	if st.State != sign.Valid {
		t.Fatalf("setup: the signatures do not verify (%s), so this is not the case caveat 10 names", st.State)
	}

	// Caveat 10: the RECORD survives three incremental signatures — it is still there, it
	// still parses, and its convener signature still verifies.
	got, err := Extract(invisible)
	if err != nil {
		t.Fatalf("after three incremental signatures the record cannot be read: %v", err)
	}
	if err := got.Verify(time.Now()); err != nil {
		t.Fatalf("after three incremental signatures the record no longer verifies: %v", err)
	}
	if got.ID != r.ID {
		t.Errorf("the record came back with id %s, want %s", got.ID, r.ID)
	}

	// And the digest is unmoved — **by these signatures, which are INVISIBLE.**
	if _, err := CheckDocument(invisible, time.Now()); err != nil {
		t.Fatalf("three invisible signatures moved the digest: %v", err)
	}

	// ---------------------------------------------------------------------------------
	// **The limit, asserted rather than left to a comfortable green (2026-08-24, P07.S02).**
	//
	// Everything above signs with no Appearance. The production path does not: `p2p.Contribute`
	// sets one whenever the caller supplies appearance bytes, and the live consent flow always
	// does. A VISIBLE signature adds a widget annotation, ContentDigest hashes /Annots, and the
	// digest therefore moves at the FIRST such signature.
	//
	// This test was previously cited — in this file, in embed.go and in the plan — as
	// discharging D20's "hop-4 clause". It could not: it signed invisibly, so its final
	// assertion was unconditionally true and could not fail for the reason the clause is about.
	// Rather than delete the test or pretend the clause holds, the limit is now MEASURED here,
	// so the next reader finds the boundary instead of a green they will trust.
	visible, err := p2p.Contribute(doc, aCert, aKey,
		p2p.Attestation{Signer: "A", When: time.Now()}, onePixelPNG(t),
		p2p.Placement{Page: 1, Rect: [4]float64{40, 40, 320, 124}})
	if err != nil {
		t.Fatal(err)
	}
	if vst := sign.Verify(visible); vst.State != sign.Valid || len(vst.Signers) != 1 {
		t.Fatalf("setup: the visible signature did not take (%s, %d signers)", vst.State, len(vst.Signers))
	}
	if _, err := CheckDocument(visible, time.Now()); err == nil {
		t.Error("a VISIBLE signature left the digest unchanged. If that is now true, the " +
			"convene-time-only limit recorded in embed.go and in the P07.S02 grill has been " +
			"lifted and every claim resting on it should be revisited — this is good news, not " +
			"a failure, but it must not pass silently.")
	}
}

// onePixelPNG is the smallest valid appearance image: enough to make a signature VISIBLE,
// which is the only property the tests using it care about.
func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{0, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestEmbedRefusesASignedDocument — the same refusal PrepareDocument already makes about
// the readme, for the same measured reason: a structural rewrite invalidates every
// signature on the file.
func TestEmbedRefusesASignedDocument(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	aCert, aKey, afp := identity(t, "A")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.SignApproval(base, aCert, aKey, sign.Options{Name: "A", When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if sign.Verify(signed).State != sign.Valid {
		t.Fatal("setup: the document is not signed, so the refusal below proves nothing")
	}
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := Embed(signed, r); err == nil {
		t.Error("a ceremony record was embedded into an already-signed document — the rewrite " +
			"invalidates every signature already on it")
	}
}

// TestADocumentWithoutARecordSaysSo: an ordinary PDF has no record, and that is not a
// corruption.
func TestADocumentWithoutARecordSaysSo(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CheckDocument(base, time.Now()); !errors.Is(err, ErrNoRecord) {
		t.Errorf("an ordinary PDF gave %v, want ErrNoRecord", err)
	}
}

// TestASwappedDocumentIsCaught: the record travels with the document, so the check that
// matters is that it is the document the record was written for.
func TestASwappedDocumentIsCaught(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	other, err := testpdf.Text("a different document entirely")
	if err != nil {
		t.Fatal(err)
	}

	r := draft(t, cfp, afp)
	h, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}

	// The record, valid and correctly signed, moved onto a DIFFERENT document.
	swapped, err := Embed(other, r)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CheckDocument(swapped, time.Now())
	if err == nil {
		t.Fatal("a correctly-signed record was accepted on a document it was not written for")
	}
	if errors.Is(err, ErrBadConvenerSignature) {
		t.Error("reported as a signature failure — the signature is fine; the document is wrong, " +
			"and telling the user the record was tampered with sends them after the wrong thing")
	}
	if !strings.Contains(err.Error(), "not the same document") {
		t.Errorf("error = %v, want it to name the mismatch", err)
	}
}

// --- the roster token in signatures -------------------------------------------

// TestEverySignatureCarriesOneCommitment is the acceptance clause: "[NibRoster:<hash>]
// appears in each signer's /Reason and cross-binds; a document whose signers do not share
// one commitment is reported as such rather than as co-signed".
//
// Both halves are driven, and the second is the one that matters — a document where the
// signers agreed to different ceremonies must not read as a co-signed document, because
// "co-signed" is what a person acts on.
func TestEverySignatureCarriesOneCommitment(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	aCert, aKey, afp := identity(t, "A")
	bCert, bKey, bfp := identity(t, "B")

	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	r := draft(t, cfp, afp, bfp)
	h, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	tok, err := r.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	rosterHex := hex.EncodeToString(tok)

	doc, err := Embed(base, r)
	if err != nil {
		t.Fatal(err)
	}

	sameCeremony := signWithRoster(t, doc, aCert, aKey, afp, bfp, rosterHex)
	sameCeremony = signWithRoster(t, sameCeremony, bCert, bKey, bfp, afp, rosterHex)

	// **Through the door a real caller uses.** `ReadAttestations` supplies no commitment, so it
	// can never report one proceeding — which is P07.S04's fix, not a regression: agreement
	// among signers is a fact about what they wrote, and this test is about agreement with the
	// record the DOCUMENT carries.
	ats := p2p.Attestations(sign.Verify(sameCeremony), ProceedingOf(sameCeremony, time.Now()))
	if len(ats) != 2 {
		t.Fatalf("setup: %d attestations, want 2", len(ats))
	}
	for i, a := range ats {
		if a.RosterHash != rosterHex {
			t.Errorf("signature %d carries roster %q, want %q", i, a.RosterHash, rosterHex)
		}
		if !a.OneProceeding {
			t.Errorf("signature %d is not reported as part of one proceeding", i)
		}
	}

	// The half that matters: two signers who committed to DIFFERENT ceremonies.
	other := draft(t, cfp, afp, bfp)
	otherTokBytes, err := other.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	otherHex := hex.EncodeToString(otherTokBytes)
	if otherHex == rosterHex {
		t.Fatal("setup: the two ceremonies share a commitment, so the split below is not a split")
	}

	split := signWithRoster(t, doc, aCert, aKey, afp, bfp, rosterHex)
	split = signWithRoster(t, split, bCert, bKey, bfp, afp, otherHex)

	sats := p2p.Attestations(sign.Verify(split), ProceedingOf(split, time.Now()))
	if len(sats) != 2 {
		t.Fatalf("setup: %d attestations on the split document, want 2", len(sats))
	}
	for i, a := range sats {
		if a.OneProceeding {
			t.Errorf("signature %d on a document whose signers committed to DIFFERENT ceremonies "+
				"is reported as part of one proceeding — a verifier would call this co-signed, "+
				"and the two people signed up to different things", i)
		}
	}
}

// signWithRoster adds one co-signature carrying a roster commitment.
func signWithRoster(t *testing.T, pdf, certPEM, keyPEM []byte, myFP, peerFP, roster string) []byte {
	t.Helper()
	place, err := p2p.NextPlacement(pdf)
	if err != nil {
		t.Fatal(err)
	}
	att := p2p.Attestation{
		Signer:       "signer",
		AcceptedPeer: peerFP,
		Intent:       "I agree to co-sign",
		When:         time.Now(),
		RosterHash:   roster,
		// **The version travels with the hash, and `reason()` emits NEITHER without it**
		// (P07.S04). From `FormatVersion` rather than a literal: this fixture stands in for the
		// production writer that does not exist yet, and a literal here would keep passing on the
		// day the record format moves while the real thing produced an uninterpretable token.
		RosterVersion: FormatVersion,
	}
	out, err := p2p.Contribute(pdf, certPEM, keyPEM, att, nil, place)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// --- the mirror ---------------------------------------------------------------

func TestTheMirrorRoundTrips(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// **A REAL document, and the record's DocHash names it (updated 2026-08-24, P07.S02a).**
	// The fixture was []byte("%PDF-1.7\nfake"): fine while the mirror stored whatever it was
	// handed, and no longer, because ReadMirror now checks an unsigned stored document against
	// the record that names it. A round-trip test whose payload is not a document cannot tell
	// a working mirror from one that lost the bytes and put something else there.
	doc, derr := testpdf.Text("the lease")
	if derr != nil {
		t.Fatal(derr)
	}
	h, derr := DocumentHash(doc)
	if derr != nil {
		t.Fatal(derr)
	}
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	dir, err := WriteMirror(root, r, doc)
	if err != nil {
		t.Fatal(err)
	}
	back, pdf, err := ReadMirror(root, r.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != r.ID || len(pdf) != len(doc) {
		t.Errorf("mirror round-trip lost data: id=%s pdf=%d bytes", back.ID, len(pdf))
	}
	if err := back.Verify(time.Now()); err != nil {
		t.Errorf("the record does not verify after a trip through the mirror: %v", err)
	}
	if err := RemoveMirror(root, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the ceremony directory survived the prune")
	}
}

// TestTheMirrorRefusesAnUnsafeID: the id comes out of a record, and a record can arrive
// from another party — so it is attacker-controlled input being used to build a path.
func TestTheMirrorRefusesAnUnsafeID(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{
		"../../etc", "..", "", "abc", strings.Repeat("z", 32),
		"0123456789abcdef0123456789abcde/", "0123456789abcdef0123456789abcdeF",
	} {
		if _, err := MirrorDir(root, bad); err == nil {
			t.Errorf("MirrorDir accepted %q — a ceremony id names a directory, and this one "+
				"either escapes the root or is not an id at all", bad)
		}
	}
	// The positive control: a real id is accepted, or the loop above would pass against a
	// function that refused everything.
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MirrorDir(root, id); err != nil {
		t.Errorf("a freshly generated id was refused: %v", err)
	}
}

// TestTheMirrorHoldsNoSecret is D29's clause, and it is an ABSENCE check by design: a test
// that asserts the vault contains the invitation secret cannot see a copy left on disk
// beside the document.
//
// The mirror writes what it is given, so the guard is that nothing in this package ever
// writes a secret field — asserted over the bytes that actually land on disk.
func TestTheMirrorHoldsNoSecret(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	_, _, afp := identity(t, "A")
	r := draft(t, cfp, afp)
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	inv, err := oneInvitation(t, r)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir, err := WriteMirror(root, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The stimulus: the file really has content, so "no secret in it" is not true of an
	// empty file.
	if len(b) < 100 {
		t.Fatalf("setup: the mirrored record is %d bytes, which is not a record", len(b))
	}
	// The ACTUAL secret bytes of the invitation THIS ceremony issued — not the word
	// "secret", and not a freshly-minted one.
	//
	// The first draft called NewInvitation *after* writing the mirror, which mints a new
	// random secret every call: it searched the file for a value that had never existed
	// anywhere, so it could not have failed. Caught by probing, and it is the same shape as
	// the vacuity this repo keeps finding — an assertion whose subject was never present.
	if len(inv.Secret) != SecretLen {
		t.Fatal("setup: the invitation has no secret, so searching for it proves nothing")
	}
	for _, form := range [][]byte{
		inv.Secret,
		[]byte(hex.EncodeToString(inv.Secret)),
		[]byte(base64.StdEncoding.EncodeToString(inv.Secret)),
		[]byte(base64.RawURLEncoding.EncodeToString(inv.Secret)),
	} {
		if bytes.Contains(b, form) {
			t.Errorf("the invitation secret is in the mirrored record, encoded as %.12s… . The "+
				"mirror is ordinary files under the user's home; the secret belongs in the vault, "+
				"which is sealed to the user's SSH key (D29)", form)
		}
	}
	// The field-name scan too, which catches a secret stored in a form this test did not
	// think of encoding.
	for _, forbidden := range []string{"secret", "Secret", "privateKey"} {
		if strings.Contains(string(b), forbidden) {
			t.Errorf("the mirrored record contains the field name %q", forbidden)
		}
	}
}

// TestTheRosterHashBindsWhoConvened.
//
// v2 left the convener outside the roster preimage, and the omission was **unargued** —
// `RosterHash`'s exclusion list named only the six-word name and the secret, so this looked
// exactly like a decision. It was not: any roster member could re-sign an unchanged roster
// with their own key and `Verify` still passed, because it asks only that the signer appear
// SOMEWHERE in the roster. The hash was byte-identical, so `RosterToken` was too, and a
// verifier reading the finished document could not tell which of them convened.
//
// The FINGERPRINT is bound, not the certificate: a cert can be re-issued for the same key
// and the same identity, and binding the bytes would change the token for a ceremony that
// has not changed.
func TestTheRosterHashBindsWhoConvened(t *testing.T) {
	certA, keyA, fpA := identity(t, "Convener")
	certB, keyB, fpB := identity(t, "Other")
	rec := draft(t, fpA, fpB)
	if err := rec.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	first, err := rec.RosterHash()
	if err != nil {
		t.Fatal(err)
	}

	// The other roster member re-signs the IDENTICAL roster.
	forged := rec
	if err := forged.Sign(certB, keyB); err != nil {
		t.Fatal(err)
	}
	// STIMULUS: the roster itself did not change, so any difference below is the convener
	// axis and nothing else.
	if len(forged.Roster) != len(rec.Roster) {
		t.Fatal("setup: the roster changed, so this measures more than the convener")
	}
	for i := range rec.Roster {
		if forged.Roster[i] != rec.Roster[i] {
			t.Fatal("setup: a roster entry changed")
		}
	}
	// And it still VERIFIES — the forgery is not prevented, it is made visible. Saying so
	// is the point: a test that asserted refusal here would be describing a property this
	// change does not have.
	if err := forged.Verify(time.Now()); err != nil {
		t.Fatalf("setup: the re-signed record does not verify (%v), so this is not the "+
			"in-roster case the binding is about", err)
	}

	second, err := forged.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Error("the roster hash is unchanged when a different roster member convenes — " +
			"the token in a finished document names a set of parties and not who called them")
	}

	// The control: re-signing with the SAME identity must reproduce the same hash, or the
	// token stops being a stable commitment to the ceremony.
	again := rec
	if err := again.Sign(certA, keyA); err != nil {
		t.Fatal(err)
	}
	third, err := again.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, third) {
		t.Error("re-signing with the same identity changed the roster hash — the token is " +
			"not a stable commitment to the ceremony")
	}
}

// TestACeremonyDeadlineHasACeiling — the plan's fourth externally-supplied security
// parameter, and its own words are "enforced, not documented".
//
// D16 calls clock 3 "days, convener's choice", which is a parameter with no bound. It
// governs how long a listener may arm, how long invitation-scoped pins persist (D29) and how
// long a mirror lives — so a convener setting ten years was a config away, and nothing in
// the tree said otherwise. Before this, `Record.Expires` was read at exactly ONE line in the
// whole repo (the roster preimage) and never compared to a clock at all.
func TestACeremonyDeadlineHasACeiling(t *testing.T) {
	cert, key, cfp := identity(t, "convener")
	_, _, afp := identity(t, "alice")
	now := time.Now()

	sign := func(d time.Duration) Record {
		t.Helper()
		r := draft(t, cfp, afp)
		r.Expires = now.Add(d)
		if err := r.Sign(cert, key); err != nil {
			t.Fatal(err)
		}
		return r
	}

	// SETUP: a deadline inside the ceiling verifies. Without this the refusal below is
	// equally true of a Verify that refuses everything, which would make every ceremony
	// unopenable and every other test in this file fail for a reason nobody would read.
	ok := sign(MaxCeremonyLife - time.Hour)
	if err := ok.Verify(now); err != nil {
		t.Fatalf("setup: a deadline inside the ceiling was refused (%v), so the ceiling "+
			"assertion below cannot distinguish a bound from a blanket refusal", err)
	}

	over := sign(MaxCeremonyLife + time.Hour)
	err := over.Verify(now)
	if !errors.Is(err, ErrCeremonyTooLong) {
		t.Errorf("a deadline %s ahead verified as %v; want ErrCeremonyTooLong. This is an "+
			"externally-supplied security parameter and the plan's own rule for all four of "+
			"them is that they are enforced rather than documented.",
			MaxCeremonyLife+time.Hour, err)
	}

	// **An EXPIRED ceremony still verifies, and that is the half a ceiling gets wrong.**
	// An expired record is a liveness fact about the proceeding, not a validity fact about
	// the document: a verifier reading a finished PDF next year must still be able to check
	// who was on the roster and that the convener signed. A Verify that refused a past
	// deadline would make every completed ceremony unverifiable the day after it ended.
	past := sign(-90 * 24 * time.Hour)
	if err := past.Verify(now); err != nil {
		t.Errorf("a ceremony that ended three months ago no longer verifies (%v) — a signed "+
			"record must stay checkable after the proceeding it describes is over, or the "+
			"document's own evidence expires with it", err)
	}

	// And the ordering: a record that is BOTH forged and over-long is reported as forged.
	// A convener who signed a ten-year deadline is a misconfiguration and the fix is theirs;
	// one who did not sign at all is an attacker, and that is the sentence a user needs.
	forged := sign(MaxCeremonyLife + time.Hour)
	forged.Roster[1].Fingerprint = afp[:len(afp)-2] + "ff"
	if err := forged.Verify(now); errors.Is(err, ErrCeremonyTooLong) {
		t.Errorf("a record with a broken roster AND an over-long deadline was reported as "+
			"too long; the signature failure is the one that matters and must be reported "+
			"first (got %v)", err)
	}
}

// TestTheHopNumberComesFromTheSignedRoster — the definition that did not exist.
//
// Eight functions take a `hop`, including every key, salt and seed derivation, and until now
// nothing in the tree derived one or checked one: the only assignment anywhere was a literal
// `0` in a self-test. A caller's off-by-one produced a valid key and salt at a hop that does
// not exist, published into the void, and the result read as `FetchEmpty` — which is
// indistinguishable from a counterparty who has not arrived yet.
func TestTheHopNumberComesFromTheSignedRoster(t *testing.T) {
	cert, key, c := identity(t, "convener")
	_, _, a := identity(t, "alice")
	_, _, b := identity(t, "bob")
	r := draft(t, c, a, b) // roster: convener, alice, bob
	// SIGNED, because a hop is resolved through the convener and the convener is resolved
	// from the record's own certificate. That is a property worth having rather than a
	// nuisance: a hop number cannot be derived from an unsigned draft, so the topology is
	// read off an authenticated artifact or not at all.
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}

	if got := r.Hops(); got != 2 {
		t.Fatalf("setup: a three-party roster must have 2 hops, got %d — every assertion "+
			"below is about the mapping and needs the count to be right first", got)
	}

	// Adjacency IS the hop, and both directions give the same number: the two ends of one
	// hop must agree without negotiating, which is the whole reason this is read off a
	// convener-signed artifact rather than counted.
	for _, tc := range []struct {
		x, y string
		want int
		what string
	}{
		{c, a, 0, "convener→alice"},
		{a, c, 0, "alice→convener, the same hop from the other end"},
		{c, b, 1, "convener→bob"},
		{b, c, 1, "bob→convener"},
	} {
		got, err := r.Hop(tc.x, tc.y)
		if err != nil {
			t.Errorf("%s: %v", tc.what, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s is hop %d, want %d", tc.what, got, tc.want)
		}
	}

	// **Two COUNTERPARTIES share no hop — this is D22's topology, and criterion 19 made
	// structural.** The ceremony is a convener hub: "the convener writes the record,
	// prepares the document, dials each party in roster order… Every hop is exactly today's
	// two-party session." Alice and Bob never speak, so a rule that gave them a shared hop
	// key would be describing a different ceremony — and would have each of them arming for
	// a peer D22's own TRIPWIRE argument says they never accept.
	//
	// **The first version of this test asserted alice→bob was hop 1.** It read Party's "the
	// order of the roster IS the signing order" and inferred a chain. Signing order is not
	// connection topology, and D22 is explicit about which this is.
	if _, err := r.Hop(a, b); !errors.Is(err, ErrNotAHop) {
		t.Errorf("alice and bob were given hop %v; under a convener hub they never connect "+
			"to each other, so a shared hop key between them is a key for a session that "+
			"does not exist", err)
	}
	if _, err := r.Hop(a, a); !errors.Is(err, ErrNotAHop) {
		t.Error("one party was accepted as both ends of a hop")
	}
	stranger := strings.Repeat("cd", 32)
	if _, err := r.Hop(a, stranger); !errors.Is(err, ErrNotAHop) {
		t.Error("a fingerprint outside the roster was given a hop number")
	}

	// Case, because a fingerprint differing only in case is the same party — the same fact
	// ParseInvitation normalises for, and that Convener() gets wrong one function away.
	if got, err := r.Hop(strings.ToUpper(c), a); err != nil || got != 0 {
		t.Errorf("an upper-case fingerprint did not resolve to its own party (%d, %v) — the "+
			"two ends of a hop would then derive different keys for it", got, err)
	}
}

// TestBothRostersUseOneHopRule — ADR-009 applied to a rule that has two natural homes.
//
// Record and Invitation both carry a roster and both get asked for hop numbers. Two
// implementations would be worse than the usual duplicate: they would agree on every roster
// anyone tested and disagree exactly where order was subtle — a party appearing twice, a
// case-differing fingerprint, an off-by-one at the ends.
func TestBothRostersUseOneHopRule(t *testing.T) {
	cert, key, c := identity(t, "convener")
	_, _, a := identity(t, "alice")
	_, _, b := identity(t, "bob")
	r := draft(t, c, a, b)
	// Signed, because NewInvitation resolves the convener from the record's own certificate
	// — an unsigned draft has none and is refused.
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	inv, err := oneInvitation(t, r)
	if err != nil {
		t.Fatal(err)
	}

	// SETUP: both rosters really are the same, or "they agree" is trivially true of two
	// different questions.
	if len(inv.Roster) != len(r.Roster) {
		t.Fatalf("setup: the invitation's roster has %d parties and the record's %d",
			len(inv.Roster), len(r.Roster))
	}
	if inv.Hops() != r.Hops() {
		t.Errorf("hop counts differ: invitation %d, record %d", inv.Hops(), r.Hops())
	}

	for _, pair := range [][2]string{{c, a}, {a, b}, {b, a}, {c, b}, {a, a}} {
		rh, rerr := r.Hop(pair[0], pair[1])
		ih, ierr := inv.Hop(pair[0], pair[1])
		if (rerr == nil) != (ierr == nil) {
			t.Errorf("%s/%s: record says %v and invitation says %v — one rule, two answers",
				short(pair[0]), short(pair[1]), rerr, ierr)
			continue
		}
		if rerr == nil && rh != ih {
			t.Errorf("%s/%s: record says hop %d and invitation says hop %d",
				short(pair[0]), short(pair[1]), rh, ih)
		}
	}
}

// oneInvitation returns any one party's invitation, for the many tests whose subject is not
// which party holds which secret. Since P05.S04 the convener mints one per party, so a test
// that wants "an invitation for this record" has to say whose.
func oneInvitation(t *testing.T, r Record) (Invitation, error) {
	t.Helper()
	all, err := NewInvitations(r)
	if err != nil {
		return Invitation{}, err
	}
	// Deterministic: roster order, not map order, so a failure is reproducible.
	for _, p := range r.Roster {
		if inv, ok := all[p.Fingerprint]; ok {
			return inv, nil
		}
	}
	t.Fatal("no invitation was minted for any roster party")
	return Invitation{}, nil
}

// TestAgreementAmongSignersIsNotAProceeding — P07.S04's measured defect, driven.
//
// `markOneProceeding` compared the signatures' commitments **only to each other**. The token
// lives inside the signed `/Reason`, so it is a value the signer picks — and two parties who both
// write the same 64-hex value they invented, on a document carrying **no ceremony record at all**,
// were reported as one proceeding on every signature. `web/app.js` renders that as
// *"✓ One proceeding — every signature on this document commits to the same ceremony."*
//
// It was latent only because nothing populated the token. C01 is the change that populates it, so
// this is a precondition of C01 rather than an improvement beside it.
//
// The control comes FIRST and is the same one the test above drives: a REAL ceremony still reports
// one proceeding, or this fix has simply turned the verdict off.
func TestAgreementAmongSignersIsNotAProceeding(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	aCert, aKey, afp := identity(t, "A")
	bCert, bKey, bfp := identity(t, "B")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}

	// The control: a genuine ceremony, its own record, its own commitment.
	r := draft(t, cfp, afp, bfp)
	h, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	tokBytes, err := r.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	real := hex.EncodeToString(tokBytes)
	doc, err := Embed(base, r)
	if err != nil {
		t.Fatal(err)
	}
	honest := signWithRoster(t, doc, aCert, aKey, afp, bfp, real)
	honest = signWithRoster(t, honest, bCert, bKey, bfp, afp, real)
	for i, a := range p2p.Attestations(sign.Verify(honest), ProceedingOf(honest, time.Now())) {
		if !a.OneProceeding {
			t.Fatalf("signature %d of an HONEST ceremony is not reported as one proceeding — the "+
				"fix has turned the verdict off rather than basing it on the record", i)
		}
	}

	// **And the defect: no record anywhere, and a value the signers made up.**
	invented := strings.Repeat("ab", 32)
	if invented == real {
		t.Fatal("setup: the invented commitment collides with the real one")
	}
	forged := signWithRoster(t, base, aCert, aKey, afp, bfp, invented)
	forged = signWithRoster(t, forged, bCert, bKey, bfp, afp, invented)
	// Stimulus: the document really carries no record, and both signatures really carry the
	// invented token — otherwise the verdict below would be false for a different reason.
	if _, err := Extract(forged); !errors.Is(err, ErrNoRecord) {
		t.Fatalf("setup: the forged document carries a record (%v)", err)
	}
	ats := p2p.Attestations(sign.Verify(forged), ProceedingOf(forged, time.Now()))
	if len(ats) != 2 {
		t.Fatalf("setup: %d attestations, want 2", len(ats))
	}
	for i, a := range ats {
		if a.RosterHash != invented {
			t.Fatalf("setup: signature %d carries %q, want the invented token", i, a.RosterHash)
		}
		if a.OneProceeding {
			t.Errorf("signature %d of a document with NO ceremony record, whose signers both "+
				"wrote a commitment they chose themselves, is reported as part of one "+
				"proceeding. The client renders that as \"✓ One proceeding — every signature on "+
				"this document commits to the same ceremony\", over a ceremony that does not "+
				"exist.", i)
		}
	}
}

// TestAnUnsignedRecordIsNotAProceeding — the third way "✓ One proceeding" could be false, and the
// one the first two arms do not reach.
//
// `TestAgreementAmongSignersIsNotAProceeding` covers a document with NO record. This covers a
// document that HAS one, whose signatures commit to its real hash, and whose record **nobody
// signed** — so nothing binds that roster to a convener. Found by mutating `ProceedingOf` to
// ignore `CheckRecord`'s error and noticing that neither existing arm went red: both of them
// compare against a *different* hash, so a commitment lifted from an unverified record would
// still have matched.
//
// The record has to be a verifying one for the control, and the same record unsigned for the
// arm — same roster, same id, so the only difference is the signature.
func TestAnUnsignedRecordIsNotAProceeding(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	aCert, aKey, afp := identity(t, "A")
	bCert, bKey, bfp := identity(t, "B")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	h, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}

	build := func(sign bool) ([]byte, string) {
		t.Helper()
		r := draft(t, cfp, afp, bfp)
		r.DocHash = h
		if sign {
			if err := r.Sign(cert, key); err != nil {
				t.Fatal(err)
			}
		}
		tok, err := r.RosterHash()
		if err != nil {
			t.Fatal(err)
		}
		hex := hexOf(tok)
		doc, err := Embed(base, r)
		if err != nil {
			t.Fatal(err)
		}
		out := signWithRoster(t, doc, aCert, aKey, afp, bfp, hex)
		out = signWithRoster(t, out, bCert, bKey, bfp, afp, hex)
		return out, hex
	}

	// The control: signed record, and the ✓ is earned.
	signed, _ := build(true)
	for i, a := range p2p.Attestations(signVerify(signed), ProceedingOf(signed, time.Now())) {
		if !a.OneProceeding {
			t.Fatalf("signature %d of a SIGNED record's ceremony is not one proceeding — the "+
				"arm below would then prove nothing", i)
		}
	}

	// The arm: identical roster, identical commitments on both signatures, record unsigned.
	unsigned, _ := build(false)
	if _, err := Extract(unsigned); err != nil {
		t.Fatalf("setup: the unsigned-record fixture carries no record at all (%v)", err)
	}
	ats := p2p.Attestations(signVerify(unsigned), ProceedingOf(unsigned, time.Now()))
	if len(ats) != 2 {
		t.Fatalf("setup: %d attestations, want 2", len(ats))
	}
	for i, a := range ats {
		if a.RosterHash == "" {
			t.Fatalf("setup: signature %d carries no commitment, so the verdict below is not "+
				"about the record", i)
		}
		if a.OneProceeding {
			t.Errorf("signature %d commits to a record NOBODY SIGNED and is reported as part of "+
				"one proceeding. Nothing binds that roster to a convener, so the ✓ would be "+
				"vouching for a ceremony anyone could have written into the document.", i)
		}
	}
}

// hexOf is the hex spelling a commitment travels as.
func hexOf(b []byte) string { return hex.EncodeToString(b) }

// signVerify is sign.Verify, named locally so these tests read as "the already-verified status".
func signVerify(pdf []byte) sign.Status { return sign.Verify(pdf) }

// TestAnIncompleteCeremonyIsReportedIncomplete — C18, driven at five of nine, and C16 with it.
//
// C18's own words for what happens without this: a nine-party ceremony abandoned at hop five
// renders *untampered, 5 signers, every attestation matched, one proceeding* — every one of those
// true — and **no surface says four obliged parties never signed**. "Mutually co-signed" and "one
// proceeding" are both facts about the signatures that ARE there.
//
// C16 is the same count read the other way: a `signs:false` convener is not obliged, so a ceremony
// they carried to completion must read complete rather than short a signer. Both are driven here
// because they are one mechanism and a test for either alone would pass on a build that had only
// that half.
func TestAnIncompleteCeremonyIsReportedIncomplete(t *testing.T) {
	cert, key, cfp := identity(t, "Convener")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	// Nine parties: a NON-SIGNING convener plus eight obliged signers, so the roster length and
	// the obliged count differ — otherwise "obliged" and "everyone" are the same number and this
	// cannot tell them apart.
	type signer struct {
		cert, key []byte
		fp        string
	}
	var signers []signer
	r := draft(t, cfp)
	r.Roster[0].Signs = false
	for i := 0; i < 8; i++ {
		c, k, fp := identity(t, string(rune('A'+i)))
		signers = append(signers, signer{c, k, fp})
		r.Roster = append(r.Roster, Party{Fingerprint: fp, Label: string(rune('A' + i)), Signs: true})
	}
	h, err := DocumentHash(base)
	if err != nil {
		t.Fatal(err)
	}
	r.DocHash = h
	if err := r.Sign(cert, key); err != nil {
		t.Fatal(err)
	}
	tok, err := r.RosterHash()
	if err != nil {
		t.Fatal(err)
	}
	commitment := hex.EncodeToString(tok)
	doc, err := Embed(base, r)
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: nine roster entries, EIGHT obliged. A test that could not tell those apart would
	// pass on a build that counted the roster.
	proc := ProceedingOf(doc, time.Now())
	if len(r.Roster) != 9 || len(proc.Signing) != 8 {
		t.Fatalf("setup: %d roster entries and %d obliged — they must DIFFER, or the "+
			"non-signing convener is invisible to every assertion below",
			len(r.Roster), len(proc.Signing))
	}

	// Five of the eight sign, then stop.
	partial := doc
	for i := 0; i < 5; i++ {
		prev := ""
		if i > 0 {
			prev = signers[i-1].fp
		}
		partial = signWithRoster(t, partial, signers[i].cert, signers[i].key, signers[i].fp,
			prev, commitment)
	}
	ats := p2p.Attestations(sign.Verify(partial), ProceedingOf(partial, time.Now()))
	signed, obliged := p2p.Completeness(ats, ProceedingOf(partial, time.Now()))
	if obliged != 8 {
		t.Errorf("an abandoned ceremony reports %d obliged signers, want 8", obliged)
	}
	if signed != 5 {
		t.Errorf("an abandoned ceremony reports %d signed, want 5 — the client renders this as "+
			"\"5 of 8 obliged signer(s) have signed\", and without it the document reads as "+
			"untampered, five signers, one proceeding, with nothing saying three never signed",
			signed)
	}

	// And the completion, which is C16: all eight sign, the convener signs nothing, and it must
	// read COMPLETE rather than short the convener.
	full := partial
	for i := 5; i < 8; i++ {
		full = signWithRoster(t, full, signers[i].cert, signers[i].key, signers[i].fp,
			signers[i-1].fp, commitment)
	}
	fats := p2p.Attestations(sign.Verify(full), ProceedingOf(full, time.Now()))
	fsigned, fobliged := p2p.Completeness(fats, ProceedingOf(full, time.Now()))
	if fsigned != fobliged {
		t.Errorf("a COMPLETED ceremony carried by a non-signing convener reports %d of %d — it "+
			"is short its convener, who was convened not to sign. That is C16: the verifier "+
			"must not cry wolf over a party with no obligation.", fsigned, fobliged)
	}
	// The convener really did sign nothing, or "complete" above is complete for the wrong reason.
	for _, a := range fats {
		if strings.EqualFold(a.Fingerprint, cfp) {
			t.Fatal("setup: the convener signed, so C16's case is not the one being driven")
		}
	}
}

// TestCompletenessSaysNothingWithoutARoster — the third state, and the one that would misreport
// every ordinary co-sign in the product.
//
// A two-party co-sign carries no record, so there is no roster and no obligation. Reporting "0 of
// 0 signed" about one is a verdict on a proceeding that does not exist — and the client's whole
// block keys on `obliged > 0`, so a non-zero here would put a completeness line on every document
// Nib has ever signed.
func TestCompletenessSaysNothingWithoutARoster(t *testing.T) {
	aCert, aKey, afp := identity(t, "A")
	bCert, bKey, bfp := identity(t, "B")
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	plain := signWithRoster(t, base, aCert, aKey, afp, bfp, "")
	plain = signWithRoster(t, plain, bCert, bKey, bfp, afp, "")
	ats := p2p.Attestations(sign.Verify(plain), ProceedingOf(plain, time.Now()))
	// Stimulus: it really is a signed two-party document, so a zero below is about the ROSTER.
	if len(ats) != 2 {
		t.Fatalf("setup: %d attestations on the two-party fixture", len(ats))
	}
	signed, obliged := p2p.Completeness(ats, ProceedingOf(plain, time.Now()))
	if obliged != 0 || signed != 0 {
		t.Errorf("an ordinary two-party co-sign reports %d of %d obliged — it has no ceremony "+
			"record and no obligation, and the client would put a completeness line on every "+
			"document Nib has ever signed", signed, obliged)
	}
}
