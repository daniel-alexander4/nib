package pdfops

import (
	"strings"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// A word broken at a line's end rejoins when the break moves — `PLAN-text-reflow.md` P08.S07. A justifying producer
// hyphenates: a line ends "accom-" and the next opens "modate" (301 of 909 Antenna House and 106 of 436 InDesign
// multi-line paragraphs carry one). Re-wrapped as two words, the fragment lands mid-line as "accom- modate", silently. A
// broken word whose halves the edit keeps is one item to the breaker, offering its old break point: it sits whole where it
// fits, and keeps its hyphen only where it still ends a line. Nothing new is hyphenated.
//
// Whether the hyphen is the producer's or the word's ("full-time" broken after its hyphen) the glyphs do not say. The
// document does: over the real-producer corpus, of 836 such breaks, 630 are written joined elsewhere in their document,
// 22 hyphenated, 4 both and 180 neither. A soft hyphen (U+00AD) is the producer's by definition. So the word is rejoined
// without its hyphen where the joined form is written and the hyphenated one is not, with it where the hyphenated form is
// written, and refused — `hyphen-unsure` — where neither is and the break would move (law 3). The evidence is read from
// the page and two either side: there it agrees with the whole document's in 88% of Antenna House's breaks and 81% of
// InDesign's (the page alone: 76% and 66%), and where it does not, it finds nothing — a refusal, never the other answer.

// hyphenRadius is how many pages either side of the edited one are read for evidence.
const hyphenRadius = 2

// breakHyphen reports whether word w ends in a break hyphen — a hyphen after a letter, the word at least two letters
// before it — and whether that hyphen is soft.
func breakHyphen(w reflowWord) (soft, ok bool) {
	r := []rune(w.text())
	if len(r) < 3 || len(w.glyphs) < 2 || !unicode.IsLetter(r[len(r)-2]) {
		return false, false
	}
	if last := []rune(w.glyphs[len(w.glyphs)-1].text); len(last) != 1 || last[0] != r[len(r)-1] {
		return false, false // the hyphen is not a glyph of its own, and cannot be dropped alone
	}
	switch r[len(r)-1] {
	case '­':
		return true, true
	case '-', '‐':
		return false, true
	}
	return false, false
}

// startsLower reports whether a word begins with a lower-case letter — the next line continuing a word, not a new one.
func startsLower(w reflowWord) bool {
	for _, r := range w.text() {
		return unicode.IsLower(r)
	}
	return false
}

// evidenceKey is a word as the evidence compares it: lower case, without the punctuation around it, a U+2010 hyphen
// read as '-' and a soft hyphen inside it dropped — a word written "full‐time" or "accom\u00admodate" is the same
// evidence as "full-time" and "accommodate" (the review of S07, I2).
func evidenceKey(s string) string {
	s = strings.NewReplacer("\u2010", "-", "\u00ad", "").Replace(s)
	return strings.ToLower(strings.TrimFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' }))
}

// hyphenEvidence is every whole word written on page pageNr and the hyphenRadius pages either side — never half of a
// break: a line's last word when it ends in a hyphen, nor the first word of the line after it.
func hyphenEvidence(ctx *model.Context, layout pageLayout, pageNr int) map[string]bool {
	seen := map[string]bool{}
	endsInHyphen := func(w string) bool {
		return strings.HasSuffix(w, "-") || strings.HasSuffix(w, "\u2010") || strings.HasSuffix(w, "\u00ad")
	}
	add := func(l pageLayout) {
		for _, p := range l.paragraphs {
			broke := false // the line before ended in a hyphen
			for li, ln := range p.lines {
				ws := editWords(ln.text)
				for wi, w := range ws {
					if (wi == len(ws)-1 && li < len(p.lines)-1 && endsInHyphen(w)) || (wi == 0 && broke) {
						continue
					}
					seen[evidenceKey(w)] = true
				}
				broke = len(ws) > 0 && endsInHyphen(ws[len(ws)-1])
			}
		}
	}
	add(layout)
	budget := newFormWalkBudget(2 * hyphenRadius) // one budget across the pages read, as every page loop shares one
	for q := pageNr - hyphenRadius; q <= pageNr+hyphenRadius; q++ {
		if q == pageNr || q < 1 || q > ctx.PageCount {
			continue
		}
		l, err := readPageLayout(ctx, pageAt(ctx, nil, q), budget)
		if err != nil {
			continue // a page that does not read gives no evidence; the break is refused if it needs some
		}
		add(l)
	}
	return seen
}

// rejoinBroken finds each break the edit keeps both halves of — old words oi and oi+1, the first ending its line in a
// break hyphen and the second opening the next in lower case, kept as new words ni and ni+1 — and makes them one item
// with its break point. A break nothing settles, or one in a chain (a word hyphenated across three lines, whose middle
// is the tail of one break and the head of the next — the review of S07, W1), stays as its words and is returned in
// unsure, as the indices of its head and tail in the result: those may stay only where every head still ends a line with
// its tail opening the next ({-1, -1}: a break exists and cannot be followed, so any re-set refuses). words is one item
// per new word; the result may be shorter.
func rejoinBroken(ctx *model.Context, layout pageLayout, pageNr int, lines [][]reflowWord, words []emitWord, al wordAlignment,
	oldWord []reflowWord, oldAt [][2]int) (out []emitWord, unsure [][2]int) {
	// breakAt reports whether new word ni is the head of a break whose tail is new word ni+1.
	breakAt := func(ni int) (oi, wi int, soft, ok bool) {
		oi, ok1 := al.kept[ni]
		oj, ok2 := al.kept[ni+1]
		if ni < 0 || !ok1 || !ok2 || oj != oi+1 {
			return 0, 0, false, false
		}
		li, wi := oldAt[oi][0], oldAt[oi][1]
		soft, isBreak := breakHyphen(oldWord[oi])
		if !isBreak || wi != len(lines[li])-1 || oldAt[oi+1] != [2]int{li + 1, 0} || !startsLower(oldWord[oi+1]) {
			return 0, 0, false, false
		}
		return oi, wi, soft, true
	}
	if !al.ok {
		// Past the alignment's bound no word is known kept, so no break can be found by its halves: one the paragraph
		// carries is refused rather than left to land mid-line (the review of S07, I1).
		for li := 0; li+1 < len(lines); li++ {
			if _, isBreak := breakHyphen(lines[li][len(lines[li])-1]); isBreak && startsLower(lines[li+1][0]) {
				return words, [][2]int{{-1, -1}}
			}
		}
		return words, nil
	}
	var evidence map[string]bool
	for ni := 0; ni < len(words); ni++ {
		oi, wi, soft, ok := breakAt(ni)
		if !ok {
			out = append(out, words[ni])
			continue
		}
		head, tail := oldWord[oi], oldWord[oi+1]
		h, t := words[ni], words[ni+1]
		drop, keep := soft, false
		if !soft {
			if evidence == nil {
				evidence = hyphenEvidence(ctx, layout, pageNr)
			}
			r := []rune(head.text())
			a, b := evidenceKey(string(r[:len(r)-1])), evidenceKey(tail.text())
			keep = evidence[a+"-"+b]
			drop = !keep && evidence[a+b]
		}
		_, _, _, prevBreak := breakAt(ni - 1)
		_, _, _, nextBreak := breakAt(ni + 1)
		if (!drop && !keep) || prevBreak || nextBreak {
			// The tail is not taken here: the next word is examined in its turn, as a head of its own or as itself.
			out = append(out, h)
			unsure = append(unsure, [2]int{len(out) - 1, len(out)})
			continue
		}
		// Each glyph keeps its own font (P08.S06), so halves in two fonts join as well as halves in one; the word's own
		// look is its last glyph's.
		joined := emitWord{face: t.face, font: t.font, tfSize: t.tfSize}
		hcodes, hkerns, hspacing, hlooks, hwidth := h.codes, h.kerns, h.spacing, h.looks, h.width
		if drop {
			// The hyphen glyph goes, with the kern before it and its advance.
			g := head.glyphs[len(head.glyphs)-1]
			n := len(hcodes) - 1
			hcodes, hkerns, hspacing, hlooks, hwidth = hcodes[:n], hkerns[:n-1], hspacing[:n], hlooks[:n], hwidth-g.kern-g.advance
		}
		joined.looks = append(append(joined.looks, hlooks...), t.looks...)
		joined.codes = append(append(joined.codes, hcodes...), t.codes...)
		joined.kerns = append(append(append(joined.kerns, hkerns...), 0), t.kerns...)
		joined.spacing = append(append(joined.spacing, hspacing...), t.spacing...)
		joined.width = hwidth + t.width
		joined.broken = &[2]emitWord{h, t}
		joined.keepFrom = oi - wi + 1 // the oldIdx of the first word of the line the break ended
		out = append(out, joined)
		ni++
	}
	return out, unsure
}
