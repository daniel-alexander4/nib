package pairing

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// wordlistSHA256 is the frozen list's digest.
//
// **Caveat 4, as relaxed on 2026-08-18.** The list was originally to be frozen
// *forever*, because a name decoded to a fingerprint and swapping a word would make a
// name written on paper last year decode to a different key. Pairing no longer works
// that way — nothing decodes a name to a pin — so a wordlist change is now a breaking
// *display* change rather than a silent authentication failure. The guard stays because
// a label people learn to recognise should not move quietly; what relaxed is the
// promise, from "never" to "not without a version bump and a note".
const wordlistSHA256 = "17f6dcb6e56b91b495a4d1aadf41aa0de67b8a7b4d9e813c691a3c9ccbb79446"

// TestWordlistIsFrozen is the checksum guard caveat 4 asks for.
func TestWordlistIsFrozen(t *testing.T) {
	sum := sha256.Sum256([]byte(wordlistFile))
	got := hex.EncodeToString(sum[:])
	if got != wordlistSHA256 {
		t.Fatalf(`the pairing wordlist has changed.

  was: %s
  now: %s

Every user's six-word name is derived from this list, so changing it renames every
identity in the field: a name someone wrote down, or learned to recognise, is now
somebody else's. That is a breaking user-visible change, not a refactor.

If the change is deliberate it needs a MINOR version bump and a note in the release
saying names have changed — then update this constant in the same commit. If it is
not deliberate, restore the file; gen/gen_wordlist.py regenerates it and prints the
digest of what it produced.`, wordlistSHA256, got)
	}
}

// TestWordlistProperties is the other half of the freeze, and it is the half that is
// easy to leave out.
//
// A checksum guard freezes a mistake exactly as well as it freezes a good list. On its
// own it would have pinned a duplicated word, a pair of homophones, or a 2047-entry
// file just as faithfully — for good. These are the constraints gen_wordlist.py selects
// against, re-asserted here against the artifact that actually ships, so the claim rests
// on the file rather than on the generator having been correct when it ran.
func TestWordlistProperties(t *testing.T) {
	w := allWords()

	if len(w) != 1<<bitsPerWord {
		t.Fatalf("the list has %d words, need exactly %d — the encoding packs %d bits per word "+
			"and a short list makes some indices unencodable", len(w), 1<<bitsPerWord, bitsPerWord)
	}

	seen := make(map[string]int, len(w))
	prefixes := make(map[string]int, len(w))
	for i, word := range w {
		if j, dup := seen[word]; dup {
			t.Errorf("%q appears at both %d and %d — two indices encode the same word, so the "+
				"encoding is not injective and a name no longer identifies one fingerprint", word, j, i)
		}
		seen[word] = i

		if len(word) < 3 || len(word) > 8 {
			t.Errorf("%q is %d characters; the list is 3–8", word, len(word))
		}
		for _, r := range word {
			if r < 'a' || r > 'z' {
				t.Errorf("%q contains %q — the list is lowercase ASCII, because these words are "+
					"typed, spoken and read back on machines with any keyboard", word, r)
				break
			}
		}

		p := word
		if len(word) > 4 {
			p = word[:4]
		}
		if j, clash := prefixes[p]; clash {
			t.Errorf("%q and %q share the first four characters — a half-heard word over a phone "+
				"has to resolve to one entry", w[j], word)
		}
		prefixes[p] = i
	}

	// No word may be a prefix of another, in either direction.
	//
	// The four-character rule above does NOT imply this, and assuming it did is how 57
	// ambiguous pairs shipped in the first generated list: a three-letter word's "first
	// four" is the word itself, so `all` and `allow` both satisfied it while "all…" over
	// a bad line resolves to neither.
	for _, word := range w {
		for k := 3; k < len(word); k++ {
			if j, ok := seen[word[:k]]; ok {
				t.Errorf("%q is a prefix of %q — hearing the shorter one leaves the listener "+
					"unable to tell which was said", w[j], word)
			}
		}
	}

	// Sorted, because decoding looks a word up and a reader scanning the file should be
	// able to find one. Asserted rather than assumed: the generator sorts, and a later
	// hand-edit inserting a word in the wrong place would otherwise pass silently.
	for i := 1; i < len(w); i++ {
		if w[i-1] >= w[i] {
			t.Fatalf("the list is not sorted at %d: %q then %q", i, w[i-1], w[i])
		}
	}
}

// TestWordlistCarriesNoTrailingBlank guards the parse rather than the content: a stray
// blank line would make Fields() shorter than the file looks and shift every index by
// one from that point, renaming most identities with nothing visibly wrong.
func TestWordlistCarriesNoTrailingBlank(t *testing.T) {
	lines := strings.Split(strings.TrimSuffix(wordlistFile, "\n"), "\n")
	if len(lines) != len(allWords()) {
		t.Errorf("the file has %d lines but parses to %d words — a blank or padded line shifts "+
			"every index after it", len(lines), len(allWords()))
	}
	for i, l := range lines {
		if l != strings.TrimSpace(l) {
			t.Errorf("line %d (%q) carries surrounding whitespace", i+1, l)
		}
	}
}
