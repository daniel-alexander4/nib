package pairing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// --- the encoding -----------------------------------------------------------

// TestFixedVectors pins the encoding against vectors derived BY HAND.
//
// This distinction is the whole value of the test. Vectors produced by running Name()
// and pasting the output assert only that the code equals itself — they would survive
// any change to the bit selection, the word order, or the list, which is precisely what
// they exist to catch. The two boundary fingerprints are hand-derivable and that is why
// the plan named them: all-zero takes the first word six times, all-ones takes the last
// word six times, whatever the list happens to contain.
func TestFixedVectors(t *testing.T) {
	w := allWords()
	first, last := w[0], w[len(w)-1]

	zero := make([]byte, 32)
	ones := bytes.Repeat([]byte{0xFF}, 32)

	cases := []struct {
		name string
		fp   []byte
		want string
	}{
		{"all zero", zero, strings.TrimSpace(strings.Repeat(first+" ", NameWords))},
		{"all ones", ones, strings.TrimSpace(strings.Repeat(last+" ", NameWords))},
		// A third vector from real bytes, with its indices derived BY HAND from the
		// definition — the leading 66 bits, big-endian, in six 11-bit groups:
		//
		//   0x01 23 45 67 89 ab cd ef 01
		//     = 00000001 00100011 01000101 01100111 10001001 10101011 11001101 11101111 00000001
		//
		//   regrouped by 11, most significant first:
		//     00000001001 = 9      00011010001 = 209    01011001111 = 719
		//     00010011010 = 154    10111100110 = 1510   11110111100 = 1980
		//
		// The six literals below are those indices. They are written out rather than
		// recomputed by a helper: a helper that re-implements the same bit walk would
		// go wrong in the same direction as the code it checks.
		{"0123456789abcdef", mustHex(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
			strings.Join([]string{w[9], w[209], w[719], w[154], w[1510], w[1980]}, " ")},
	}

	for _, c := range cases {
		got, err := Name(c.fp)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: Name = %q, want %q", c.name, got, c.want)
		}
		// Round-trip: the bits must come back. An encoding proved in one direction only
		// is not proved injective.
		back, err := decode(got)
		if err != nil {
			t.Fatalf("%s: decode: %v", c.name, err)
		}
		if !bytes.Equal(back, leading66(c.fp)) {
			t.Errorf("%s: round-trip lost bits: %x, want %x", c.name, back, leading66(c.fp))
		}
	}
}

func leading66(fp []byte) []byte {
	out := make([]byte, 9)
	copy(out, fp[:9])
	out[8] &= 0xC0 // keep bits 64 and 65, clear the rest
	return out
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestNameIsDerivedNotStored is D3's substance: a name is a function of the key, so
// deriving twice yields the same words and two keys do not share a name by accident.
func TestNameIsDerivedNotStored(t *testing.T) {
	fp := sha256.Sum256([]byte("identity one"))
	a, err := Name(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	b, err := Name(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("deriving twice gave %q then %q", a, b)
	}
	other := sha256.Sum256([]byte("identity two"))
	c, err := Name(other[:])
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Errorf("two different identities share the name %q", a)
	}
}

// TestNameRefusesAShortFingerprint: 66 bits cannot come from 8 bytes, and silently
// reading past the end would be a panic in production rather than an error here.
func TestNameRefusesAShortFingerprint(t *testing.T) {
	if _, err := Name(make([]byte, 8)); err == nil {
		t.Error("an 8-byte fingerprint produced a name; 66 bits need 9 bytes")
	}
}

// --- decoding -----------------------------------------------------------------

// TestDecodeRejectsWithDistinctErrors covers the acceptance clause: a wrong-length
// phrase and an out-of-list word are refused, and refused DIFFERENTLY. One error for
// both would tell a caller nothing about which mistake was made.
func TestDecodeRejectsWithDistinctErrors(t *testing.T) {
	w := allWords()
	good := strings.Join([]string{w[0], w[1], w[2], w[3], w[4], w[5]}, " ")

	if _, err := decode(good); err != nil {
		t.Fatalf("setup: a well-formed name did not decode: %v", err)
	}

	short := strings.Join([]string{w[0], w[1], w[2]}, " ")
	_, errShort := decode(short)
	if errShort == nil {
		t.Fatal("a three-word phrase decoded")
	}
	long := good + " " + w[6]
	_, errLong := decode(long)
	if errLong == nil {
		t.Fatal("a seven-word phrase decoded")
	}

	bad := strings.Join([]string{w[0], w[1], "zzzznotaword", w[3], w[4], w[5]}, " ")
	_, errBad := decode(bad)
	if errBad == nil {
		t.Fatal("a phrase with an out-of-list word decoded")
	}

	// The distinctness is the clause. Compare on the sentinels, not on message text.
	if !isWordCount(errShort) || !isWordCount(errLong) {
		t.Errorf("wrong-length phrases did not report the word-count error: %v / %v", errShort, errLong)
	}
	if !isNotInList(errBad) {
		t.Errorf("an out-of-list word reported %v, not the not-in-list error", errBad)
	}
	if isWordCount(errBad) {
		t.Error("the out-of-list error is indistinguishable from the wrong-length one")
	}
}

// Matched with errors.Is against the package's own sentinels, not against message text.
// A guard that greps prose passes when the prose survives a behaviour change — a hole
// this repo has been caught by twice.
func isWordCount(err error) bool { return errors.Is(err, errWordCount) }
func isNotInList(err error) bool { return errors.Is(err, errNotInList) }

// TestNameSurvivesRetyping: the name travels through people — pasted out of an email,
// read off a screen, retyped with a capital or a double space.
func TestNameSurvivesRetyping(t *testing.T) {
	fp := sha256.Sum256([]byte("retyped"))
	name, err := Name(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{
		name,
		strings.ToUpper(name),
		"  " + name + "  ",
		strings.ReplaceAll(name, " ", "   "),
		strings.ReplaceAll(name, " ", "\n"),
	} {
		if !Matches(fp[:], variant) {
			t.Errorf("Matches rejected a retyped form: %q", variant)
		}
	}
}

// --- the relocated transposition clause ---------------------------------------

// TestTransposedNameDoesNotMatch is P01.S01's transposition clause, driven where the
// slice grill moved it: through Matches, not through the decoder.
//
// The encoding carries no checksum — six words is 66 bits with nothing left over — so
// every six-word phrase from the list is a valid encoding and `decode` has nothing to
// reject a transposition WITH. The property is real all the same, and this is where it
// lives: swapping two words produces a different bit string, so it no longer matches the
// fingerprint it came from. A human hearing the words in the wrong order sees it too.
func TestTransposedNameDoesNotMatch(t *testing.T) {
	fp := sha256.Sum256([]byte("transposition"))
	name, err := Name(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	if !Matches(fp[:], name) {
		t.Fatal("setup: the name derived from this fingerprint does not match it")
	}

	parts := strings.Fields(name)
	swapped := 0
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == parts[i+1] {
			continue // swapping equal words is not a transposition
		}
		p := append([]string(nil), parts...)
		p[i], p[i+1] = p[i+1], p[i]
		swapped++
		if Matches(fp[:], strings.Join(p, " ")) {
			t.Errorf("swapping words %d and %d still matches: %q", i+1, i+2, strings.Join(p, " "))
		}
		// It still DECODES — that is the point of the relocation, and asserting it here
		// stops someone "fixing" the decoder to reject transpositions and spending the
		// bits this design refused to spend.
		if _, err := decode(strings.Join(p, " ")); err != nil {
			t.Errorf("a transposed name failed to decode (%v) — the decoder has grown a "+
				"checksum, which costs payload bits the design deliberately keeps", err)
		}
	}
	if swapped == 0 {
		t.Fatal("setup: no adjacent pair differed, so no transposition was tested")
	}

	// A substituted word must not match either — the same mechanism, stated separately
	// because it is a different mistake.
	w := allWords()
	sub := append([]string(nil), parts...)
	for _, cand := range w {
		if cand != sub[0] {
			sub[0] = cand
			break
		}
	}
	if Matches(fp[:], strings.Join(sub, " ")) {
		t.Error("substituting the first word still matches the fingerprint")
	}
}

// TestMatchesRejectsMalformed: Matches answers false rather than erroring, so every
// malformed shape has to be checked here or it would silently read as a match.
func TestMatchesRejectsMalformed(t *testing.T) {
	fp := sha256.Sum256([]byte("malformed"))
	for _, bad := range []string{"", "   ", "one two three", "zzzz zzzz zzzz zzzz zzzz zzzz"} {
		if Matches(fp[:], bad) {
			t.Errorf("Matches accepted %q", bad)
		}
	}
	if Matches(make([]byte, 4), "anything at all here now please") {
		t.Error("Matches accepted a short fingerprint")
	}
}
