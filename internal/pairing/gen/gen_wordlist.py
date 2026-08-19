#!/usr/bin/env python3
"""Build internal/pairing/wordlist.txt — the frozen 2048-word pairing list.

Not run at build time. See README.md in this directory for why, for the sources,
and for the selection rules this implements.

Usage:  python3 gen_wordlist.py [--out ../wordlist.txt]
"""

import argparse
import hashlib
import re
import sys
import urllib.request
from collections import Counter

# Fifteen public-domain books, by Project Gutenberg ebook number. A spread of
# authors and centuries rather than one writer's vocabulary — a list drawn from a
# single book inherits its idiom, and these words are meant to be ordinary.
PG_BOOKS = [
    (1342, "Pride and Prejudice"), (11, "Alice's Adventures in Wonderland"),
    (84, "Frankenstein"), (1661, "The Adventures of Sherlock Holmes"),
    (2701, "Moby Dick"), (98, "A Tale of Two Cities"),
    (1400, "Great Expectations"), (174, "The Picture of Dorian Gray"),
    (345, "Dracula"), (76, "Adventures of Huckleberry Finn"),
    (5200, "Metamorphosis"), (2542, "A Doll's House"),
    (16, "Peter Pan"), (43, "Dr. Jekyll and Mr. Hyde"),
    (1080, "A Modest Proposal"), (2591, "Grimms' Fairy Tales"),
    (1260, "Jane Eyre"), (768, "Wuthering Heights"),
    (158, "Emma"), (161, "Sense and Sensibility"),
    (141, "Mansfield Park"), (121, "Northanger Abbey"),
    (105, "Persuasion"), (205, "Walden"),
    (2814, "Dubliners"), (219, "Heart of Darkness"),
    (120, "Treasure Island"), (35, "The Time Machine"),
    (36, "The War of the Worlds"), (5230, "The Invisible Man"),
    (159, "The Island of Doctor Moreau"), (74, "The Adventures of Tom Sawyer"),
    (1184, "The Count of Monte Cristo"), (2852, "The Hound of the Baskervilles"),
    (244, "A Study in Scarlet"), (215, "The Call of the Wild"),
    (1232, "The Prince"), (46, "A Christmas Carol"),
    (730, "Oliver Twist"), (766, "David Copperfield"),
    (600, "Notes from the Underground"), (1497, "The Republic"),
    (2148, "The Works of Edgar Allan Poe, Volume 2"), (996, "Don Quixote"),
    (1998, "Thus Spake Zarathustra"), (4517, "Ethan Frome"),
    (514, "Little Women"), (271, "Black Beauty"),
    (55, "The Wonderful Wizard of Oz"), (289, "The Wind in the Willows"),
]


CMUDICT_URL = "https://raw.githubusercontent.com/cmusphinx/cmudict/master/cmudict.dict"

TARGET = 2048
# A word must appear in at least this many of the source books.
#
# **This is the constraint that keeps the books' own vocabulary out**, and it was added
# after reading the first output rather than reasoned in advance: a spread of fifteen
# authors does not stop each book's proper nouns entering, because a character name is
# extremely frequent *within* its book. `krogstad`, `adler`, `lanyon` and `lakeman` all
# ranked high on total frequency and appear in exactly one text. Requiring presence
# across the corpus is what "common English" actually means, and it removes dialect
# spellings (`lemme`) and one-book archaisms in the same motion.
# A word must appear in this fraction of the corpus.
#
# The job of this constraint is exactly one thing: keep any single book's own vocabulary
# out — character names, dialect spellings, an author's pet archaism. A word present in
# ten books by ten authors is common English by any reading, and one-book vocabulary
# fails that by a mile, so 0.20 does the job with room to spare. It was 0.53 and then
# 0.30 first; both were picked by hand rather than derived, and both starved the list.
# Recorded because the number looks arbitrary and the REASON is not: raise it and you
# are not buying better words, you are buying fewer of them.
MIN_BOOKS_FRACTION = 0.20
# Above this share of capitalised occurrences a word is treated as a proper noun.
# 0.30 leaves generous room for sentence-start capitalisation of an ordinary word.
PROPER_NOUN_RATIO = 0.30
MIN_LEN, MAX_LEN = 3, 8
PREFIX_LEN = 4
MIN_PHONEME_DISTANCE = 2
# Three, not two. Two was the first attempt and it starved the list: 965 words survived,
# against 2048 needed. The generator REFUSED rather than quietly loosening — which is the
# behaviour wanted — and widening the corpus from fifteen books to fifty was the first
# response, per its own message. Three syllables inside eight characters is `animal`,
# `family`, `evening`: still ordinary, still sayable down a bad line.
MAX_SYLLABLES = 3

# Words refused regardless of frequency.
#
# These are read ALOUD, by strangers, to confirm an identity — often about a document
# with legal weight. A word that is offensive, distressing, or that changes the apparent
# meaning of the sentence it lands in is a defect even though it is a perfectly good
# English word. The list is deliberately conservative and deliberately visible: a hidden
# filter is one nobody can review.
#
# Kept short on purpose. The syllable and length constraints already remove most of the
# problem, and a sprawling denylist becomes a thing nobody reads.
EXCLUDE = {
    # Death, violence, illness.
    "dead", "death", "die", "died", "dying", "kill", "killed", "murder", "corpse",
    "blood", "bloody", "wound", "wounded", "gun", "shot", "shoot", "knife", "war",
    "sick", "ill", "pain", "hurt", "grave", "tomb", "bury", "buried", "hell",
    "devil", "damn", "curse", "cursed", "hang", "hanged", "drown", "drowned",
    "cancer", "plague", "fever", "poison", "victim", "corpse", "coffin",
    # Words that read as instructions or verdicts in the middle of a ceremony.
    "no", "not", "stop", "cancel", "abort", "reject", "refuse", "deny", "denied",
    "fail", "failed", "error", "invalid", "void", "null", "false", "wrong",
    "fake", "forged", "fraud", "steal", "stolen", "theft", "cheat", "lie", "lied",
    # Legal terms that could be misheard as being about the document itself.
    "sign", "signed", "sue", "sued", "court", "judge", "guilty", "crime", "jail",
    "prison", "arrest", "law", "legal", "claim", "debt", "owe", "owed",
    # Ambiguous single letters or spelled-out confusion.
    "a", "i", "o", "u", "b", "c", "d", "e", "g", "p", "t", "v", "x", "y", "z",
    # Found by READING the generated list rather than by reasoning about the set — the
    # third pass produced every one of these. Insults, illness, and words with a
    # distressing register; all perfectly good English and none of them something to
    # read down the phone to a stranger about their legal document.
    "idiot", "fool", "stupid", "mad", "insane", "lunatic", "savage", "brute",
    "wretch", "wretched", "abhorred", "accursed", "infernal", "inhuman", "inflict",
    "illness", "disease", "agony", "misery", "despair", "terror", "horror",
    "dread", "doom", "ghost", "demon", "witch", "torture", "cruel", "vile",
    "filthy", "beggar", "slave", "drunk", "corrupt", "shame", "disgrace",
    "naked", "nude", "warlike", "captive", "hostage", "ransom", "bribe",
}


def _with_inflections(words):
    """Expand the exclusion set over ordinary English inflections.

    A set keyed to exact words does not exclude what it appears to: `wound` was on the
    list and `wounds` came through anyway, as did `wretched` behind `wretch`. Found by
    reading the generated list, not by reasoning about the set.
    """
    out = set(words)
    for w in list(words):
        out.update({w + "s", w + "es", w + "ed", w + "d", w + "ing",
                    w + "er", w + "ers", w + "y", w + "ly", w + "ish"})
        if w.endswith("e"):
            out.update({w[:-1] + "ing", w[:-1] + "ed"})
        if w.endswith("y"):
            out.update({w[:-1] + "ies", w[:-1] + "ied"})
    return out


EXCLUDE = _with_inflections(EXCLUDE)


def fetch(url):
    with urllib.request.urlopen(url, timeout=120) as r:
        return r.read()


def load_frequencies():
    """Count word frequencies across the Project Gutenberg set."""
    counts = Counter()
    books_containing = Counter()
    upper_case = Counter()
    total_case = Counter()
    digests = []
    skipped = []
    for num, title in PG_BOOKS:
        raw = None
        for url in (
            f"https://www.gutenberg.org/files/{num}/{num}-0.txt",
            f"https://www.gutenberg.org/cache/epub/{num}/pg{num}.txt",
        ):
            try:
                raw = fetch(url)
                break
            except Exception:
                continue
        if raw is None:
            # Skipped rather than fatal, but LOUDLY: a corpus that silently shrank would
            # change the output with nothing in the provenance saying why.
            print(f"  WARNING: could not fetch PG#{num} ({title}) — skipped",
                  file=sys.stderr)
            skipped.append((num, title))
            continue
        digests.append((num, title, hashlib.sha256(raw).hexdigest()[:16]))
        original = raw.decode("utf-8", errors="replace")
        text = original.lower()
        # Strip the PG header and footer so boilerplate ("gutenberg", "license",
        # "donations") does not enter the frequency table as ordinary English.
        start = text.find("*** start of")
        end = text.find("*** end of")
        if start != -1:
            text = text[text.find("\n", start) + 1:]
        if end != -1:
            text = text[:end]
        # The same window on the original casing, so the capitalisation ratio is measured
        # over the book and not over Project Gutenberg's boilerplate.
        ostart = original.lower().find("*** start of")
        oend = original.lower().find("*** end of")
        if ostart != -1:
            original = original[original.find("\n", ostart) + 1:]
        if oend != -1:
            original = original[:oend]
        words = re.findall(r"[a-z]+", text)
        counts.update(words)
        books_containing.update(set(words))
        # Capitalisation, measured on the ORIGINAL casing, is how proper nouns are found.
        #
        # A denylist cannot do this job: `indian`, `york`, `john`, `london` are ordinary
        # lowercase strings and every one of them came through the first three passes. A
        # proper noun is capitalised nearly everywhere it appears; an ordinary word is
        # capitalised only at the start of a sentence, which is a small fraction of its
        # thousands of occurrences. So the ratio separates them and no hand-maintained
        # list is needed. Found by reading the output, not by reasoning about the corpus.
        for m in re.findall(r"[A-Za-z]+", original):
            low = m.lower()
            total_case[low] += 1
            if m[0].isupper():
                upper_case[low] += 1
    return counts, books_containing, digests, skipped, upper_case, total_case


def load_cmudict():
    """word -> list of ARPABET phoneme sequences (stress markers stripped)."""
    raw = fetch(CMUDICT_URL)
    digest = hashlib.sha256(raw).hexdigest()[:16]
    prons = {}
    for line in raw.decode("utf-8", errors="replace").splitlines():
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        parts = line.split()
        word, phones = parts[0], parts[1:]
        # "word(2)" marks an alternate pronunciation; it makes the word a homograph
        # with two readings, which is exactly what we do not want to read aloud.
        base = re.sub(r"\(\d+\)$", "", word)
        stripped = tuple(re.sub(r"\d$", "", p) for p in phones)
        prons.setdefault(base, []).append(stripped)
    return prons, digest


def syllables(phones):
    """ARPABET vowels carry the syllable count."""
    return sum(1 for p in phones if p[0] in "AEIOU")


def edit_distance(a, b, cap):
    """Levenshtein over phoneme sequences, abandoning once it exceeds cap."""
    if abs(len(a) - len(b)) > cap:
        return cap + 1
    prev = list(range(len(b) + 1))
    for i, x in enumerate(a, 1):
        cur = [i]
        for j, y in enumerate(b, 1):
            cur.append(min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (x != y)))
        if min(cur) > cap:
            return cap + 1
        prev = cur
    return prev[-1]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="../wordlist.txt")
    args = ap.parse_args()

    print("fetching Project Gutenberg texts…", file=sys.stderr)
    counts, books_containing, book_digests, skipped, upper_case, total_case = load_frequencies()
    min_books = max(2, int(len(book_digests) * MIN_BOOKS_FRACTION))
    print(f"corpus: {len(book_digests)} books fetched, {len(skipped)} skipped; "
          f"a word must appear in >= {min_books} of them", file=sys.stderr)
    print("fetching CMU Pronouncing Dictionary…", file=sys.stderr)
    prons, cmu_digest = load_cmudict()

    chosen = []            # words, in acceptance order
    chosen_phones = []     # their phoneme tuples, index-aligned
    prefixes = set()       # first PREFIX_LEN characters of each accepted word
    accepted = set()       # accepted words, for the prefix-of check
    prefix_index = set()   # every prefix (len >= MIN_LEN) of every accepted word
    seen_prons = set()

    for word, _freq in counts.most_common():
        if len(chosen) >= TARGET:
            break
        if not (MIN_LEN <= len(word) <= MAX_LEN):
            continue
        if books_containing[word] < min_books:
            continue
        # Predominantly capitalised => a proper noun (a place, a nationality, a name).
        if total_case[word] and upper_case[word] / total_case[word] > PROPER_NOUN_RATIO:
            continue
        if word in EXCLUDE:
            continue
        p = prons.get(word)
        # Exactly one recorded pronunciation: a word people say two ways cannot be
        # confirmed over a phone by comparing what was said.
        if not p or len(p) != 1:
            continue
        phones = p[0]
        if syllables(phones) > MAX_SYLLABLES or syllables(phones) < 1:
            continue
        # Prefix ambiguity, in BOTH directions.
        #
        # A unique first-four is not enough on its own: it lets `all` and `allow` both in,
        # because a three-letter word's "first four" is itself. Over a bad line "all…" is
        # then ambiguous, which is the exact thing this constraint exists to stop. Found
        # by testing the generated list for prefix-of pairs — there were 57 — rather than
        # by reasoning about the rule.
        if word[:PREFIX_LEN] in prefixes:
            continue
        if word in prefix_index:  # an accepted word begins with this candidate
            continue
        if any(word[:k] in accepted for k in range(MIN_LEN, len(word))):
            continue  # this candidate begins with an accepted word
        if phones in seen_prons:
            continue
        if any(edit_distance(phones, q, MIN_PHONEME_DISTANCE - 1) < MIN_PHONEME_DISTANCE
               for q in chosen_phones):
            continue
        chosen.append(word)
        chosen_phones.append(phones)
        prefixes.add(word[:PREFIX_LEN])
        accepted.add(word)
        for k in range(MIN_LEN, len(word)):
            prefix_index.add(word[:k])
        seen_prons.add(phones)

    if len(chosen) < TARGET:
        sys.exit(f"only {len(chosen)} words survived the constraints; need {TARGET}. "
                 f"Widen the corpus rather than loosening a constraint.")

    chosen.sort()
    with open(args.out, "w") as f:
        f.write("\n".join(chosen) + "\n")

    body = open(args.out, "rb").read()
    print(f"wrote {args.out}: {len(chosen)} words, sha256={hashlib.sha256(body).hexdigest()}")
    print("\nprovenance:")
    if skipped:
        for num, title in skipped:
            print(f"  SKIPPED PG#{num:<5}              {title}")
    for num, title, d in book_digests:
        print(f"  PG#{num:<5} {d}  {title}")
    print(f"  cmudict    {cmu_digest}")


if __name__ == "__main__":
    main()
