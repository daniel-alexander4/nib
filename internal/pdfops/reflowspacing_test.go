package pdfops

import (
	"math"
	"strings"
	"testing"
)

// mixedSpacingPara draws one paragraph under five spacings: the word "quick" across two runs of different `Tc` (Acrobat
// writes a `Tc` per run, mid-word), a line under `Tw`, `Tz` and stroke mode — kerned inside "over", so a kern is converted
// under 110% scaling — with a word raised by `Ts`, and a line shown by
// `"`, which sets word and character spacing as it shows. The signature line after it is drawn under whatever the
// paragraph left in force, so it moves or re-spaces if the rewrite does not restore all of it.
const mixedSpacingPara = "BT /F1 12 Tf 14 TL 72 700 Td 0.5 Tc (The qu) Tj 1.5 Tc (ick brown fox jumps) Tj" +
	" T* 0 Tc 2 Tw 110 Tz 1 Tr [(ov) -80 (er the lazy)] TJ 1 Ts ( dog and runs) Tj 0 Ts" +
	" 0 1 (away from here.) \"" +
	" T* T* T* (Signature line) Tj ET"

// TestEveryGlyphKeepsTheSpacingItWasDrawnWith — `PLAN-text-reflow.md` P08.S01: a paragraph whose runs differ in `Tc`,
// `Tw`, `Tz`, `Ts` and `Tr` — with state operators between its shows, and a word drawn across two of them — is re-set,
// and every glyph of every word it kept reads back under the spacing it was drawn with and at the advance it had, each
// occurrence its own; every space is the one the paragraph drew after a word in that spacing; a typed word takes the
// paragraph's usual style and the fit of the glyph before it; and the text after the paragraph is drawn where, and as, it
// was — the edit ENDS on a word drawn under other spacing than the paragraph's last show, so nothing but the restore can
// put the state back.
//
// **The stimulus is asserted**: the paragraph must read as one, under several spacings, with a word spanning two.
func TestEveryGlyphKeepsTheSpacingItWasDrawnWith(t *testing.T) {
	pdf := helveticaPage(mixedSpacingPara)
	before, beforeRuns := layoutOf(t, pdf)
	if len(before.paragraphs) < 2 {
		t.Fatalf("setup: %d paragraphs, want the mixed paragraph and the signature line", len(before.paragraphs))
	}
	lines, median, cause := paragraphWords(before.paragraphs[0])
	if cause != "" {
		t.Fatalf("setup: the paragraph cannot be read (%s)", cause)
	}
	// The spaces it drew, by the spacing of the glyph before each — read here off the positions, not through the
	// rewrite's own spacer, which would grade the rewrite against itself.
	drew := map[textSpacing]float64{}
	for _, l := range lines {
		for i := 1; i < len(l); i++ {
			drew[lastSpacing(l[i-1].spacing)] = l[i].startX - l[i-1].startX - l[i-1].width
		}
	}
	if len(drew) < 3 {
		t.Fatalf("setup: spaces drawn in %d spacings, want several", len(drew))
	}
	_ = median
	kept := map[string][]reflowWord{}
	spacings, spanning := map[textSpacing]bool{}, false
	for _, l := range lines {
		for _, w := range l {
			kept[w.text()] = append(kept[w.text()], w)
			for _, s := range w.spacing {
				spacings[s] = true
				spanning = spanning || s != w.spacing[0]
			}
		}
	}
	if len(spacings) < 4 || !spanning || kept["quick"] == nil {
		t.Fatalf("setup: %d spacings, a word spanning two %v, words %v — the fixture does not mix spacing", len(spacings), spanning, lineTexts(lines))
	}
	lastShow := beforeRuns[0]
	for _, r := range beforeRuns {
		if r.text != "Signature line" && r.span.start > lastShow.span.start {
			lastShow = r
		}
	}
	if lastSpacing(kept["brown"][0].spacing) == spacingOf(lastShow.state) {
		t.Fatal("setup: the edit's last word is drawn as the paragraph's last show, so the restore is never asked")
	}

	edit := "The quick fox jumps over the lax dog and runs now away from here. brown"
	out, cause := reflowed(t, pdf, 0, edit)
	if cause != "" {
		t.Fatalf("refused (%s), want the paragraph re-set under its own spacings", cause)
	}
	after, afterRuns := layoutOf(t, out)
	got, _, cause := paragraphWords(after.paragraphs[0])
	if cause != "" {
		t.Fatalf("the rewritten paragraph cannot be read (%s)", cause)
	}
	var words []reflowWord
	for _, l := range got {
		words = append(words, l...)
		for i := 1; i < len(l); i++ {
			prev := l[i-1]
			want, ok := drew[lastSpacing(prev.spacing)]
			if !ok {
				continue // a spacing the paragraph drew no space in: the median, which only the corpus asks
			}
			if gap := l[i].startX - prev.startX - prev.width; math.Abs(gap-want) > 1e-6 {
				t.Errorf("the space after %q is %v, want %v — the space the paragraph drew in that spacing", prev.text(), gap, want)
			}
		}
	}
	if strings.Join(lineTexts([][]reflowWord{words}), "") != edit {
		t.Fatalf("reads back %q, want %q", lineTexts(got), edit)
	}
	usual := kept["away"][0].spacing[0].style()
	seen := map[string]int{}
	for i, w := range words {
		if w.text() == "lax" || w.text() == "now" {
			// Typed: the usual style, never the raised "runs" before "now"; the fit of the glyph before it.
			before := lastSpacing(words[i-1].spacing)
			for gi, s := range w.spacing {
				if s.style() != usual || s.tc != before.tc || s.tw != before.tw {
					t.Errorf("typed word %q glyph %d drawn under %+v, want style %+v and the fit of %+v", w.text(), gi, s, usual, before)
				}
			}
			continue
		}
		occ := kept[w.text()]
		if occ == nil {
			t.Fatalf("word %q was never drawn", w.text())
		}
		k := occ[min(seen[w.text()], len(occ)-1)]
		seen[w.text()]++
		if len(k.glyphs) != len(w.glyphs) {
			t.Fatalf("%q: %d glyphs, was %d", w.text(), len(w.glyphs), len(k.glyphs))
		}
		for gi := range w.glyphs {
			if w.spacing[gi] != k.spacing[gi] {
				t.Errorf("%q glyph %d drawn under %+v, was %+v", w.text(), gi, w.spacing[gi], k.spacing[gi])
			}
			if math.Abs(w.glyphs[gi].advance-k.glyphs[gi].advance) > 1e-6 {
				t.Errorf("%q glyph %d advances %v, was %v", w.text(), gi, w.glyphs[gi].advance, k.glyphs[gi].advance)
			}
			if gi > 0 && math.Abs(w.glyphs[gi].kern-k.glyphs[gi].kern) > 1e-6 {
				t.Errorf("%q glyph %d kerned %v, was %v", w.text(), gi, w.glyphs[gi].kern, k.glyphs[gi].kern)
			}
		}
	}

	tail := func(runs []textRun) textRun {
		for _, r := range runs {
			if r.text == "Signature line" {
				return r
			}
		}
		t.Fatal("no signature line")
		return textRun{}
	}
	a, b := tail(beforeRuns), tail(afterRuns)
	if math.Abs(a.x-b.x) > 1e-6 || math.Abs(a.y-b.y) > 1e-6 || math.Abs(a.width-b.width) > 1e-6 {
		t.Errorf("the line after the paragraph moved: (%v, %v) width %v, was (%v, %v) width %v", b.x, b.y, b.width, a.x, a.y, a.width)
	}
	if spacingOf(a.state) != spacingOf(b.state) || a.state.tfSize != b.state.tfSize {
		t.Errorf("the line after the paragraph is drawn under %+v, was %+v", spacingOf(b.state), spacingOf(a.state))
	}
}

// TestAWordDrawnInTwoLooksKeepsEachOrRefuses — a word drawn twice in different styles (here "Note" stroked as a
// synthetic bold, then plain) keeps each occurrence's look wherever the edit keeps it in place, and refuses where a copy
// was moved, since nothing then says which look was meant (P08.S01's review, C1 and R2-1).
func TestAWordDrawnInTwoLooksKeepsEachOrRefuses(t *testing.T) {
	pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td 2 Tr (Note) Tj 0 Tr ( this is about the Note and more of) Tj" +
		" T* (the words that wrap onto a line here.) Tj ET")
	trs := func(pdf []byte) []int {
		l, _ := layoutOf(t, pdf)
		lines, _, cause := paragraphWords(l.paragraphs[0])
		if cause != "" {
			t.Fatalf("cannot be read (%s)", cause)
		}
		var out []int
		for _, ln := range lines {
			for _, w := range ln {
				if w.text() == "Note" {
					out = append(out, w.spacing[0].tr)
				}
			}
		}
		return out
	}
	if got := trs(pdf); len(got) != 2 || got[0] != 2 || got[1] != 0 {
		t.Fatalf("setup: Note drawn in modes %v, want [2 0]", got)
	}
	out, cause := reflowed(t, pdf, 0, "Note this is all about the Note and more of the words that wrap onto a line here.")
	if cause != "" {
		t.Fatalf("both kept: refused (%s)", cause)
	}
	if got := trs(out); len(got) != 2 || got[0] != 2 || got[1] != 0 {
		t.Errorf("both kept: Note drawn in modes %v, want each its own [2 0]", got)
	}
	// "the", drawn twice in one look, may lose one: whichever went, the survivor looks the same.
	if _, cause := reflowed(t, pdf, 0, "Note this is about Note and more of the words that wrap onto a line here."); cause != "" {
		t.Errorf("one of two alike dropped: refused (%s)", cause)
	}
	// One dropped: the alignment knows which survived, and it keeps its own look.
	out, cause = reflowed(t, pdf, 0, "This is about the Note and more of the words that wrap onto a line here.")
	if cause != "" {
		t.Fatalf("one dropped: refused (%s)", cause)
	}
	if got := trs(out); len(got) != 1 || got[0] != 0 {
		t.Errorf("one dropped: the surviving Note drawn in modes %v, want its own [0]", got)
	}
	// Two alignments explain it equally — "Note this is about the " deleted, or " this is about the Note" — so which
	// look survived is a guess (the review's R3-1, whose probe read the stroked one back).
	if _, cause := reflowed(t, pdf, 0, "Note and more of the words that wrap onto a line here."); cause != causeAmbiguousStyle {
		t.Errorf("two alignments: %q, want %q — the look cannot be known", cause, causeAmbiguousStyle)
	}
	// "the Note and" → "the and Note": either the plain Note was kept and "and" moved, or "and" was kept and a Note typed
	// after it — one candidate, but no longest alignment forces it, so a typed copy's look is still a guess.
	if _, cause := reflowed(t, pdf, 0, "Note this is about the and Note more of the words that wrap onto a line here."); cause != causeAmbiguousStyle {
		t.Errorf("kept or typed: %q, want %q", cause, causeAmbiguousStyle)
	}
	// Moved — deleted in one place, typed in another: which look was meant is a guess (the review's R2-1, whose probe read
	// the plain one back stroked).
	if _, cause := reflowed(t, pdf, 0, "This is about the Note and more of the words that wrap onto a line here. Note"); cause != causeAmbiguousStyle {
		t.Errorf("moved: %q, want %q — the look cannot be known", cause, causeAmbiguousStyle)
	}
}

// TestATypedWordOnAnEvenCountIsPlain — when the paragraph draws as many glyphs raised as plain, a typed word is plain:
// the tie goes to the plainer style, never to the raised one before it (P08.S01's review, R2-3).
func TestATypedWordOnAnEvenCountIsPlain(t *testing.T) {
	pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td (aa bb aa bb aa) Tj T* 2 Ts (cc dd cc dd cc) Tj 0 Ts ET")
	l, _ := layoutOf(t, pdf)
	if len(l.paragraphs) != 1 {
		t.Fatalf("setup: %d paragraphs, want the two lines read as one", len(l.paragraphs))
	}
	out, cause := reflowed(t, pdf, 0, "aa bb aa bb aa cc dd cc dd cc be")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	l, _ = layoutOf(t, out)
	lines, _, _ := paragraphWords(l.paragraphs[0])
	for _, ln := range lines {
		for _, w := range ln {
			if w.text() == "be" && w.spacing[0].ts != 0 {
				t.Errorf("the typed word is raised (Ts %v) on an even count", w.spacing[0].ts)
			}
		}
	}
}

// TestATypedWordsStyleNeverRestsOnMapOrder — two styles tied on count AND on every distance from plain (raised 2 and
// lowered 2) still choose one, the same one every time (P08.S01's review, R3-2). Twenty runs: a choice resting on a map's
// order differs between them.
func TestATypedWordsStyleNeverRestsOnMapOrder(t *testing.T) {
	pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td 2 Ts (aa bb aa bb aa) Tj T* -2 Ts (cc dd cc dd cc) Tj 0 Ts ET")
	l, _ := layoutOf(t, pdf)
	if len(l.paragraphs) != 1 {
		t.Fatalf("setup: %d paragraphs, want the two lines read as one", len(l.paragraphs))
	}
	seen := map[float64]bool{}
	for i := 0; i < 20; i++ {
		out, cause := reflowed(t, pdf, 0, "aa bb aa bb aa cc dd cc dd cc be")
		if cause != "" {
			t.Fatalf("refused (%s)", cause)
		}
		l, _ := layoutOf(t, out)
		lines, _, _ := paragraphWords(l.paragraphs[0])
		for _, ln := range lines {
			for _, w := range ln {
				if w.text() == "be" {
					seen[w.spacing[0].ts] = true
				}
			}
		}
	}
	if len(seen) != 1 {
		t.Errorf("the typed word's rise varied between runs: %v", seen)
	}
}

// TestOneWideGapSetsNoOtherSpace — a paragraph that draws one wide gap (here a 12pt `TJ` jump after "Total", as a form's
// label-to-value gap) must not set every space in that spacing to it: the space after "due", drawn ordinary, stays
// ordinary when an unrelated word is edited (P08.S01's review, R2-2 — a median over a spacing's own two gaps is the wider).
func TestOneWideGapSetsNoOtherSpace(t *testing.T) {
	pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td 0.3 Tc [(Total) -1000 ( due)] TJ 0 Tc ( is here and more words) Tj" +
		" T* (to wrap onto a second line of text) Tj ET")
	gapAfter := func(pdf []byte, word string) float64 {
		l, _ := layoutOf(t, pdf)
		lines, _, cause := paragraphWords(l.paragraphs[0])
		if cause != "" {
			t.Fatalf("cannot be read (%s)", cause)
		}
		for _, ln := range lines {
			for i := 1; i < len(ln); i++ {
				if ln[i-1].text() == word {
					return ln[i].startX - ln[i-1].startX - ln[i-1].width
				}
			}
		}
		t.Fatalf("no gap after %q", word)
		return 0
	}
	wide, ordinary := gapAfter(pdf, "Total"), gapAfter(pdf, "due")
	if wide < ordinary+10 {
		t.Fatalf("setup: the gap after Total is %v and after due %v — not one wide gap", wide, ordinary)
	}
	out, cause := reflowed(t, pdf, 0, "Total due is here and more words to wrap onto a second line of the text")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	// Re-set, the space is drawn under the spacing of the glyph before it — "due"'s `0.3 Tc`, where the original drew its
	// space under `0 Tc` — so it is the drawn space and 0.3 more, and nothing of the 12pt jump.
	if got := gapAfter(out, "due"); math.Abs(got-(ordinary+0.3)) > 1e-6 {
		t.Errorf("the space after \"due\" is %v, want %v (drawn %v, under 0.3 Tc) — one wide gap set it", got, ordinary+0.3, ordinary)
	}
}

// TestAnEditIsAlignedToItsParagraphWordByWord — `alignWords` keeps the longest common subsequence, each kept word of the
// edit mapped to the paragraph word it is, and a word deleted in one place and typed in another left unkept.
// Expectations are literals, worked by hand.
func TestAnEditIsAlignedToItsParagraphWordByWord(t *testing.T) {
	// What EVERY longest alignment says: "Note Z" → "Z Note" is explained as well by keeping Z as by keeping Note, so the
	// edit's Note may be the paragraph's or typed; in "a b" → "a b" each is forced.
	if olds, unkept := alignWords([]string{"Note", "Z"}, []string{"Z", "Note"}).choices(1); len(olds) != 1 || olds[0] != 0 || !unkept {
		t.Errorf("Note Z → Z Note: Note may be %v, unkept %v; want [0] and unkept", olds, unkept)
	}
	if olds, unkept := alignWords([]string{"a", "b"}, []string{"a", "b"}).choices(1); len(olds) != 1 || olds[0] != 1 || unkept {
		t.Errorf("a b → a b: b may be %v, unkept %v; want [1] and forced", olds, unkept)
	}
	for _, c := range []struct {
		old, edited string
		want        map[int]int
	}{
		{"a b c", "a b c", map[int]int{0: 0, 1: 1, 2: 2}},
		{"a b c d", "a x c d", map[int]int{0: 0, 2: 2, 3: 3}},
		// The middle: "Note" moved from the front to the end is not kept; "the" is.
		{"Note this the Note x", "this the Note x Note", map[int]int{0: 1, 1: 2, 2: 3, 3: 4}},
		{"p q r s t", "x q y", map[int]int{1: 1}},
		{"p q r s t", "p r t", map[int]int{0: 0, 1: 2, 2: 4}},
		{"a b", "", map[int]int{}},
	} {
		got := alignWords(strings.Fields(c.old), strings.Fields(c.edited)).kept
		if len(got) != len(c.want) {
			t.Errorf("%q → %q: kept %v, want %v", c.old, c.edited, got, c.want)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%q → %q: kept %v, want %v", c.old, c.edited, got, c.want)
				break
			}
		}
	}
}

// TestATypedWordTakesTheFitOfTheGlyphJustBeforeIt — typed after a word whose spacing changes part-way through it ("quick",
// `0.5 Tc` then `1.5 Tc`), a word is fitted as the LAST glyph before it was: 1.5, not the word's first 0.5 (the blind
// mutation pass's survivor).
func TestATypedWordTakesTheFitOfTheGlyphJustBeforeIt(t *testing.T) {
	pdf := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td 0.5 Tc (The qu) Tj 1.5 Tc (ick brown fox jumps over the) Tj T* (lazy dog and runs away from here.) Tj ET")
	out, cause := reflowed(t, pdf, 0, "The quick zap brown fox jumps over the lazy dog and runs away from here.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	l, _ := layoutOf(t, out)
	lines, _, _ := paragraphWords(l.paragraphs[0])
	found := false
	for _, ln := range lines {
		for _, w := range ln {
			if w.text() == "zap" {
				found = true
				if w.spacing[0].tc != 1.5 {
					t.Errorf("the typed word is fitted at Tc %v, want 1.5 — the glyph just before it", w.spacing[0].tc)
				}
			}
		}
	}
	if !found {
		t.Fatal("the typed word is not on the page")
	}
}
