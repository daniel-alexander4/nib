# Wordlist generator

`gen_wordlist.py` builds `internal/pairing/wordlist.txt` — the frozen 2048-word list
the six-word pairing name encodes into (D3/D4).

**It is not run at build time and must not be.** The list is a *frozen artifact*
(caveat 4): a checksum guard in `wordlist_test.go` pins it, because a wordlist change
alters a label people learn to recognise. This script is kept so a later reader can see
how the list was derived and reproduce it, not so the build can regenerate it. Running
it and committing a different list is a deliberate act that needs a version bump and a
note, which is exactly what caveat 4's relaxation says.

## Why the list is constructed rather than vendored

Caveat 4 asks for a list with "phonetic distinctness good enough to read over a phone".
A vendored list can only *assert* that property. This one **measures** it: every pair of
words in the output is at least two phoneme-edits apart, and no two share a
pronunciation. The constraints are re-asserted as guards in `wordlist_test.go`, so the
claim is checked on every run of the suite rather than resting on this script having
been correct once.

## Sources, and why these

- **Commonness — Project Gutenberg.** Word frequencies counted over fifteen
  public-domain English books (ebook numbers in the script). Public domain with no
  redistribution caveat, and exactly citable.

  *Rejected:* the Google Web Trillion Word Corpus lists (`google-10000-english` and
  relatives). They are LDC-derived and their own licence file says "I do not recommend
  using this data for commercial purposes without licensing it from the Linguistic Data
  Consortium" — which is not a basis for an input to an AGPLv3 product that is
  distributed.

- **Pronunciation — the CMU Pronouncing Dictionary** (`cmusphinx/cmudict`,
  BSD-2-Clause), for ARPABET phoneme sequences. Used at generation time only; no part
  of it ships in nib, and the output is a list of ordinary English words rather than a
  derivative of the dictionary.

Neither source is vendored into the repo. The script fetches them, and it prints the
digests of what it fetched so a rerun that produces a different list can say why.

## Selection

In order, from the frequency-ranked candidates:

1. 3–8 characters, lowercase ASCII only.
2. Present in CMUdict with a single recorded pronunciation.
3. One or two syllables — a longer word is harder to hear cleanly over a bad line.
4. Not on the exclusion list (offensive, distressing, or ambiguous-in-context words —
   these are read aloud by strangers confirming identity, sometimes about a legal
   document).
5. Unique first four characters, so a partly-heard word is still unambiguous.
6. No homophone already accepted, compared on phonemes rather than spelling.
7. At least two phoneme-edits from every word already accepted.

Greedy in frequency order, so the commonest words that survive the constraints are the
ones kept. The result is sorted alphabetically for the file, since decoding is a binary
search and a reader scanning the file should be able to find a word.
