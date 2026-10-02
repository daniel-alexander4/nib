package pdfops

import (
	"bytes"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// kernedPara is a paragraph that kerns by table: the pair "AV" is drawn three times at −80, "Te" twice at −40, and "Wo"
// once at −55 — a pair drawn under kernSupport times is a position, and lends nothing — over the page's table of ten.
var kernedPara = "BT /F1 12 Tf 14 TL 72 700 Td [(A) -80 (VAIL and T) -40 (ea for the S) -80 (A) -80 (V) (E of all the good people who came) ] TJ" +
	" T* [(T) -40 (exts W) -55 (ord and the C) (A) -80 (VE are here) ] TJ" + cleanPairs() + " ET"

// typedKerns reads the rewritten paragraph back and returns the kern before each glyph after the first of word w, in
// thousandths of an em under the word's own size and scaling — the number a producer would have written in its `TJ`.
func typedKerns(t *testing.T, pdf []byte, w string) []float64 {
	t.Helper()
	after, _ := layoutOf(t, pdf)
	lines, _, cause := paragraphWords(after.paragraphs[0])
	if cause != "" {
		t.Fatalf("the rewritten paragraph cannot be read (%s)", cause)
	}
	for _, l := range lines {
		for _, word := range l {
			if word.text() != w {
				continue
			}
			var out []float64
			for gi, g := range word.glyphs[1:] {
				out = append(out, -g.kern/(word.tfSize*word.spacing[gi+1].th*word.scale)*1000)
			}
			return out
		}
	}
	t.Fatalf("%q is not in the rewritten paragraph %v", w, lineTexts(lines))
	return nil
}

// TestATypedWordTakesThePagesKerning — `PLAN-text-reflow.md` P08.S05: a word the user types is drawn with the kern the
// page draws for each of its pairs — "AV" at −80, three times over — and with none for a pair the page does not draw, or
// draws fewer than kernSupport times ("Wo" once, "Te" twice); and a word the paragraph already draws keeps its own kerns,
// a one-off included.
//
// **The stimulus is asserted**: the page draws "AV" kerned, more than once, and the typed word is not one it draws.
func TestATypedWordTakesThePagesKerning(t *testing.T) {
	// Plain, and under 110% horizontal scaling: a kern is read back to the `TJ` number the producer wrote, and the
	// typed word's is written under its own scaling, so the same numbers come back.
	for _, tz := range []string{"", "110 Tz "} {
		t.Run("Tz="+tz, func(t *testing.T) { typedWordTakesThePagesKerning(t, strings.Replace(kernedPara, "BT ", "BT "+tz, 1)) })
	}
}

func typedWordTakesThePagesKerning(t *testing.T, content string) {
	pdf := helveticaPage(content)
	before, _ := layoutOf(t, pdf)
	if len(before.paragraphs) < 2 || len(before.paragraphs[0].lines) != 2 {
		t.Fatalf("setup: the kerned text reads as %d paragraphs, the first of %d lines — want one of two", len(before.paragraphs), len(before.paragraphs[0].lines))
	}
	lend := kernsOf(before)
	var av, wo, te int
	for k, v := range lend {
		switch k.a + k.b {
		case "Te":
			te++
		case "AV":
			av++
			if math.Abs(v-(-80)) > 1e-9 {
				t.Fatalf("setup: the page lends AV at %v, want -80", v)
			}
		case "Wo":
			wo++
		}
	}
	if av != 1 || wo != 0 || te != 0 {
		t.Fatalf("the page lends AV %d times, Wo %d and Te %d, want once, never and never — Te is drawn twice, under kernSupport", av, wo, te)
	}

	edit := "AVAIL and Tea for the SAVE of all the good people who came Texts Word and the CAVE are here NAVY Wok ÉAV"
	out, cause := reflowed(t, pdf, 0, edit)
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	// Typed: N-A unkerned on this page, A-V borrowed, V-Y unkerned.
	if got := typedKerns(t, out, "NAVY"); len(got) != 3 || math.Abs(got[0]) > 1e-6 || math.Abs(got[1]-(-80)) > 1e-6 || math.Abs(got[2]) > 1e-6 {
		t.Errorf("typed NAVY reads back with kerns %v, want [0 -80 0] — the page's AV, nothing else", got)
	}
	// A letter of two bytes in the text before the pair: the pair is found by code, not by the string's byte offset.
	if got := typedKerns(t, out, "ÉAV"); len(got) != 2 || math.Abs(got[0]) > 1e-6 || math.Abs(got[1]-(-80)) > 1e-6 {
		t.Errorf("typed ÉAV reads back with kerns %v, want [0 -80]", got)
	}
	// "Wo" is drawn once: a typed word lends nothing from it.
	if got := typedKerns(t, out, "Wok"); len(got) != 2 || math.Abs(got[0]) > 1e-6 {
		t.Errorf("typed Wok reads back with kerns %v, want none — Wo is drawn once", got)
	}
	// Kept: its own one-off kern, and its own AV.
	if got := typedKerns(t, out, "Word"); len(got) != 3 || math.Abs(got[0]-(-55)) > 1e-6 {
		t.Errorf("kept Word reads back with kerns %v, want its own -55 first", got)
	}
	if got := typedKerns(t, out, "CAVE"); len(got) != 3 || math.Abs(got[1]-(-80)) > 1e-6 {
		t.Errorf("kept CAVE reads back with kerns %v, want its own -80", got)
	}
}

// cleanPairs is a paragraph of its own (set well below the text before it) drawing ten pairs one way, three times each —
// a face that kerns by table: past kernTable, and enough that one pair drawn two ways stays under kernNoise.
func cleanPairs() string {
	s := " 1 0 0 1 72 400 Tm"
	for i, p := range []string{"To", "Yo", "Wa", "Va", "Ye", "Tu", "Pa", "Ly", "Fa", "Ta"} {
		if i%4 == 0 {
			if i > 0 {
				s += "] TJ T*"
			}
			s += " ["
		}
		for range 3 {
			s += "(" + p[:1] + ") -40 (" + p[1:] + " ) "
		}
	}
	return s + "] TJ"
}

// TestNoisyKerningLendsNothing — a pair the page draws more than one way lends nothing, and neither does any pair of a
// face whose kerned pairs are drawn more than one way past kernNoise — the positional noise of a producer that places
// glyphs rather than kerning them (Acrobat, Word), where a pair that happens to repeat is coincidence, not a table.
//
// **The stimulus is asserted**: where the face kerns by table (the two-way pair, the word-sized gap) it lends its other
// ten pairs and withholds only AV; where it lends nothing (the noisy face, too few pairs), AV is drawn one way three times
// and would lend but for the face.
func TestNoisyKerningLendsNothing(t *testing.T) {
	avDrawn := func(l pageLayout) []float64 {
		var av []float64
		for _, ln := range l.paragraphs[0].lines {
			for _, r := range ln.runs {
				for gi := 1; gi < len(r.glyphs); gi++ {
					if string(r.glyphs[gi-1].code)+string(r.glyphs[gi].code) == "AV" {
						av = append(av, -r.glyphs[gi].kern/12*1000)
					}
				}
			}
		}
		return av
	}
	for _, tc := range []struct {
		name, content string
		lends         int // how many pairs the page lends; AV is never one
	}{
		{"the pair is drawn two ways", "BT /F1 12 Tf 14 TL 72 700 Td [(A) -80 (VAIL and the S) (A) -20 (V) (E of all the good people who came) ] TJ" +
			" T* [(text and the C) (A) -80 (VE are here) ] TJ" + cleanPairs() + " ET", 10},
		{"the face is noisy", "BT /F1 12 Tf 14 TL 72 700 Td [(A) -80 (VAIL and T) -40 (ea for the S) -12 (A) -80 (V) (E of all the good people who came) ] TJ" +
			" T* [(T) -13 (exts and the C) (A) -80 (VE are h) -7 (ere h) -31 (ere g) -9 (o g) -27 (o) ] TJ" + cleanPairs() + " ET", 0},
		{"a word-sized gap is not a kern", "BT /F1 12 Tf 14 TL 72 700 Td [(A) -300 (VAIL and the S) (A) -300 (V) (E of all the good people who came) ] TJ" +
			" T* [(text and the C) (A) -300 (VE are here) ] TJ" + cleanPairs() + " ET", 10},
		{"too few pairs make no table", "BT /F1 12 Tf 14 TL 72 700 Td [(A) -80 (VAIL and the S) (A) -80 (V) (E of a) ] TJ" +
			" T* [(text and the C) (A) -80 (VE are here) ] TJ ET", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pdf := helveticaPage(tc.content)
			before, _ := layoutOf(t, pdf)
			lend := kernsOf(before)
			if _, ok := lend[kernKey{before.paragraphs[0].lines[0].runs[0].face, 12, "A", "V"}]; ok {
				t.Errorf("the page lends AV, drawn %v", avDrawn(before))
			}
			if len(lend) != tc.lends {
				t.Errorf("the page lends %d pairs, want %d", len(lend), tc.lends)
			}
			if tc.lends == 0 {
				if av := avDrawn(before); len(av) != 3 || av[0] != -80 || av[1] != -80 || av[2] != -80 {
					t.Fatalf("setup: AV drawn %v, want -80 three times — it would lend but for the face", av)
				}
			}
			out, cause := reflowed(t, pdf, 0, before.paragraphs[0].text()+" NAVY")
			if cause != "" {
				t.Fatalf("refused (%s)", cause)
			}
			if got := typedKerns(t, out, "NAVY"); len(got) != 3 || math.Abs(got[1]) > 1e-6 {
				t.Errorf("typed NAVY reads back with kerns %v, want none", got)
			}
		})
	}
}

// TestATypedWordBorrowsKerningOverTheCorpus — S05 over the real-producer corpus. Every page's lent table is checked
// against the page itself, leave-one-run-out: each kerned pair the page draws is predicted from what its OTHER runs lend,
// and a borrowed kern that differs from what the producer drew by more than kernTolerance is harm. The census is per
// producer: the table producers (InDesign, Antenna House) are where it lends, and the positioning producers (Acrobat,
// Word) lend almost nothing — the ceilings below are the measured figures, so a change to the rule that lends noise
// fails here. Then a typed word is re-set on pages that lend, and read back with the kerns the page lent.
func TestATypedWordBorrowsKerningOverTheCorpus(t *testing.T) {
	corp := externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers"))
	if corp.absent != "" {
		t.Skip("SKIP (not a measurement): " + corp.absent)
	}
	type tally struct{ gain, harm int }
	by := map[string]*tally{}
	retyped, kerned := 0, 0
	for _, doc := range corp.docs {
		prod := filepath.ToSlash(doc.name)
		if i := bytes.IndexByte([]byte(prod), '/'); i > 0 {
			prod = prod[:i]
		}
		if by[prod] == nil {
			by[prod] = &tally{}
		}
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		for p := 1; p <= ctx.PageCount; p++ {
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
			if err != nil {
				continue
			}
			// Leave one run out: the table built without run i predicts run i's pairs. Every run is asked whose pair the
			// OTHER runs draw often enough to lend — never only the runs the full table lends, which skips exactly the runs
			// that disagree with the rest (the review of S05, C2).
			draws := kernDraws(l)
			count := map[kernKey]int{}
			for _, d := range draws {
				count[d.key]++
			}
			byRun := map[int][]kernDraw{}
			for _, d := range draws {
				byRun[d.run] = append(byRun[d.run], d)
			}
			full := lentFrom(draws, -1)
			for run, ds := range byRun {
				own := map[kernKey]int{}
				for _, d := range ds {
					own[d.key]++
				}
				askable := false
				for _, d := range ds {
					askable = askable || count[d.key]-own[d.key] >= kernSupport
				}
				if !askable {
					continue
				}
				without := lentFrom(draws, run)
				for _, d := range ds {
					v, ok := without[d.key]
					if !ok {
						continue
					}
					if math.Abs(d.v-v) <= kernTolerance {
						by[prod].gain++
					} else {
						by[prod].harm++
					}
				}
			}
			// One typed word per page that lends: the page's first lent pair, typed as a word of its own at the end of
			// the first paragraph that reflows, and read back with the kern lent.
			if len(full) == 0 || retyped >= 40 {
				continue
			}
			for pi, para := range l.paragraphs {
				if _, _, cause := paragraphWords(para); cause != "" || len(para.lines) == 0 {
					continue
				}
				first := para.lines[0].runs[0]
				var pair kernKey
				var want float64
				for k, v := range full {
					if k.face == first.face && k.size == kernSize(first.size) && (pair.a == "" || k.a+k.b < pair.a+pair.b) {
						if ta, oka := first.face.textFor([]byte(k.a)); oka {
							if tb, okb := first.face.textFor([]byte(k.b)); okb && len([]rune(ta+tb)) == 2 && isLetters(ta+tb) {
								pair, want = k, v
							}
						}
					}
				}
				if pair.a == "" {
					continue
				}
				ta, _ := first.face.textFor([]byte(pair.a))
				tb, _ := first.face.textFor([]byte(pair.b))
				typed := ta + tb + ta + tb + ta
				c2, _ := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
				out, err := reflowParagraph(c2, p, pi, para.text()+" "+typed)
				if err != nil {
					t.Fatalf("%s p%d ¶%d: %v", doc.name, p, pi, err)
				}
				if out.content == nil {
					continue
				}
				d, _, _, _ := c2.PageDict(p, false)
				if err := setPageContent(c2, d, out.content); err != nil {
					t.Fatal(err)
				}
				var buf bytes.Buffer
				if err := api.WriteContext(c2, &buf); err != nil {
					t.Fatal(err)
				}
				c3, err := pdfread.Validated(buf.Bytes(), model.NewDefaultConfiguration())
				if err != nil {
					t.Fatalf("%s p%d ¶%d: the rewritten document does not read: %v", doc.name, p, pi, err)
				}
				l3, _ := readPageGlyphLayout(c3, pageAt(c3, nil, p))
				retyped++
				var got *reflowWord
				for _, p3 := range l3.paragraphs {
					ws, _, cause := paragraphWords(p3)
					if cause != "" {
						continue
					}
					for _, ln := range ws {
						for i := range ln {
							if ln[i].text() == typed && got == nil {
								got = &ln[i]
							}
						}
					}
				}
				if got == nil {
					t.Errorf("%s p%d ¶%d: typed %q is not on the page", doc.name, p, pi, typed)
					break
				}
				ok := true
				for _, gi := range []int{1, 3} {
					den := got.tfSize * got.spacing[gi].th * got.scale
					if v := -got.glyphs[gi].kern / den * 1000; math.Abs(v-want) > 1e-6 {
						ok = false
						t.Errorf("%s p%d ¶%d: typed %q reads back with %v between %q and %q, want the page's %v", doc.name, p, pi, typed, v, ta, tb, want)
					}
				}
				if ok {
					kerned++
				}
				break
			}
		}
	}
	// The ceilings are the census at the rule's adoption (2026-10-01): a table producer lends thousands and is almost
	// never wrong; a positioning producer lends next to nothing.
	for prod, n := range by {
		t.Logf("%-12s lent right %6d, off by more than %v: %d", prod, n.gain, kernTolerance, n.harm)
	}
	for _, prod := range []string{"indesign", "antennahouse"} {
		if n := by[prod]; n == nil || n.gain < 10000 || float64(n.harm) > 0.005*float64(n.gain+n.harm) {
			t.Errorf("%s lends %+v, want the table producer's thousands, under 0.5%% of them off", prod, n)
		}
	}
	noise := 0
	for _, prod := range []string{"acrobat", "word"} {
		if n := by[prod]; n != nil {
			noise += n.gain + n.harm
		}
	}
	if noise > 20 {
		t.Errorf("the positioning producers lend %d kerns, want next to none", noise)
	}
	t.Logf("typed words re-set %d, read back with the page's kern %d", retyped, kerned)
	if retyped == 0 || kerned != retyped {
		t.Errorf("typed words re-set %d, read back kerned %d", retyped, kerned)
	}
}

func isLetters(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// TestAStepWithinOneBaselineIsNotALeading — found by the census above: a one-line paragraph borrowed its column's median
// step, and a table row grouping cut into two "lines" a quarter of a point apart made that median 0.25 — so a typed word
// that grew the paragraph was set ON its first line and read back as one word with the last. A step under minPitchEm of
// the size is not a leading; the paragraphs set at a real one decide.
//
// **The stimulus is asserted**: counted, the jittered rows ARE the median.
func TestAStepWithinOneBaselineIsNotALeading(t *testing.T) {
	line := func(y float64) textLine { return textLine{y: y, size: 9} }
	l := pageLayout{paragraphs: []textParagraph{
		{lines: []textLine{line(700)}},
		{lines: []textLine{line(650), line(649.75)}},
		{lines: []textLine{line(600), line(599.67)}},
		{lines: []textLine{line(550), line(549.75)}},
		{lines: []textLine{line(500), line(489)}},
	}}
	var all []float64
	for _, q := range l.paragraphs[1:] {
		all = append(all, q.lines[0].y-q.lines[1].y)
	}
	if m := median(all); m > 1 {
		t.Fatalf("setup: every step counted, the median is %v — the jitter does not decide it", m)
	}
	if got := paragraphPitch(l, 0); got != 11 {
		t.Errorf("a one-line paragraph's pitch is %v, want 11 — the one real leading in its column", got)
	}
	// The first line carries one large word: its size is that word's, and the leading is still the body's.
	big := pageLayout{paragraphs: []textParagraph{{lines: []textLine{{y: 700, size: 18}, line(689), line(678)}}}}
	if got := paragraphPitch(big, 0); got != 11 {
		t.Errorf("a paragraph with an 18-point word on a 9-point line has pitch %v, want its 11", got)
	}
	if got := paragraphPitch(l, 1); got != 0 {
		t.Errorf("a two-line paragraph whose lines share a baseline has pitch %v, want 0 (no-pitch, refused)", got)
	}
}

// TestATypedWordIsMeasuredWithItsKerns — the kern a typed word borrows is part of its width: every reader of the width
// (the breaker, justification, centring) sees what is drawn. A centred heading given a typed kerned word stays on its axis
// only if it is (S05's review, W1).
//
// **The stimulus is asserted**: the page lends AV at size 18, so the typed word is narrower than its glyphs.
func TestATypedWordIsMeasuredWithItsKerns(t *testing.T) {
	pdf := helveticaPage(centredPage("Annual Report", 306) + "BT /F1 18 Tf 22 TL [(A) -80 (V A) -80 (V A) -80 (V) ] TJ" + cleanPairs() + " ET")
	before, _ := layoutOf(t, pdf)
	face := before.paragraphs[0].lines[0].runs[0].face
	if v := kernsOf(before).lend(face, 18, []byte("A"), []byte("V")); v != -80 {
		t.Fatalf("setup: the page lends AV at 18 as %v, want -80", v)
	}
	edit := "Annual AVAV Report"
	out, cause := reflowed(t, pdf, 0, edit)
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	if got := typedKerns(t, out, "AVAV"); len(got) != 3 || math.Abs(got[0]-(-80)) > 1e-6 {
		t.Fatalf("setup: typed AVAV reads back with kerns %v (at 12), want AV kerned -80 at 18", got)
	}
	for i, m := range midsOf(t, out, "Annual") {
		if math.Abs(m-306) > 1e-6 {
			t.Errorf("line %d centred on %v, not 306 — the typed word was measured without its kerns", i, m)
		}
	}
}
