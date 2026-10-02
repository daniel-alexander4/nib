package pdfops

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/mdpdf"
)

// readsAsEdit reports whether text read back from a re-set paragraph is the edit as typed, word for word — except that a
// word the producer hyphenated across a line end ("accom-" "modate") may read back rejoined, with its hyphen dropped or
// kept (P08.S07). The tests that check an edit reads back use this, not equality of the words: a rejoined word is one
// word where the edit has two. Anything else that differs is a difference.
func readsAsEdit(read, edit string) bool {
	r, e := editWords(read), editWords(edit)
	i, j := 0, 0
	for i < len(r) && j < len(e) {
		if r[i] == e[j] {
			i, j = i+1, j+1
			continue
		}
		if j+1 < len(e) {
			if head := e[j]; (strings.HasSuffix(head, "-") || strings.HasSuffix(head, "‐") || strings.HasSuffix(head, "­")) && len([]rune(head)) > 1 {
				_, n := lastRune(head)
				if r[i] == head[:len(head)-n]+e[j+1] || r[i] == head+e[j+1] {
					i, j = i+1, j+2
					continue
				}
			}
		}
		return false
	}
	return i == len(r) && j == len(e)
}

// editLetters is how much of an edit a read-back has covered: its text without white space or hyphens, which a rejoin
// removes.
func editLetters(s string) int {
	n := 0
	for _, r := range s {
		if r != '-' && r != '‐' && r != '­' && !strings.ContainsRune(" \t\n\r", r) {
			n++
		}
	}
	return n
}

func lastRune(s string) (rune, int) {
	r := []rune(s)
	l := r[len(r)-1]
	return l, len(string(l))
}

func TestReadsAsEditRejoinsOnlyABreak(t *testing.T) {
	for _, c := range []struct {
		read, edit string
		want       bool
	}{
		{"we accommodate you", "we accom- modate you", true},
		{"a full-time job", "a full- time job", true},
		{"we accom- modate you", "we accom- modate you", true},
		{"we accommodate", "we accommodate", true},
		{"we accomodate you", "we accom- modate you", false},
		{"we accommodate", "we accom- modate you", false},
		{"wellknown", "well-known", false},
		{"1990 present", "1990 - present", false}, // a lone hyphen is a word, never half of a break (the review of S07, W3)
	} {
		if got := readsAsEdit(c.read, c.edit); got != c.want {
			t.Errorf("readsAsEdit(%q, %q) = %v, want %v", c.read, c.edit, got, c.want)
		}
	}
}

// brokenPara is a justified paragraph (each line but the last stretched by `Tw` to x = 400, as a hyphenating producer
// sets it) whose first line ends in a word the producer hyphenated — head "-" / tail — over a last line; evidence, when
// given, is a separate one-line paragraph below it.
func brokenPara(head, tail, evidence string) string {
	l1 := "We will do all that we can to " + head + "-"
	l2 := tail + " the needs of each and every one of"
	tw := func(line string) string {
		return num((328 - mdpdf.CoreWidth(line, "Helvetica", 12)) / float64(strings.Count(line, " ")))
	}
	s := "BT /F1 12 Tf 14 TL 72 700 Td " + tw(l1) + " Tw (" + l1 + ") Tj T* " + tw(l2) + " Tw (" + l2 + ") Tj T* 0 Tw (our clients this year.) Tj ET"
	if evidence != "" {
		s += " BT /F1 12 Tf 72 500 Td (" + evidence + ") Tj ET"
	}
	return s
}

// readBack returns the words of paragraph 0 of a re-set page, line by line.
func readBack(t *testing.T, pdf []byte) [][]string {
	t.Helper()
	l, _ := layoutOf(t, pdf)
	var out [][]string
	for _, ln := range l.paragraphs[0].lines {
		out = append(out, editWords(ln.text))
	}
	return out
}

// TestABrokenWordRejoinsWhenItsBreakMoves — `PLAN-text-reflow.md` P08.S07: a word the producer hyphenated across a line
// end reads back as one word when the edit moves it mid-line — its hyphen dropped where the document writes the word
// joined, kept where it writes it hyphenated — and keeps its hyphen, split across the lines, where it still ends a line.
//
// **The stimulus is asserted**: the paragraph reads with the break at its first line's end, and the edit moves it.
func TestABrokenWordRejoinsWhenItsBreakMoves(t *testing.T) {
	for _, c := range []struct {
		name, head, tail, evidence, joined string
	}{
		{"joined elsewhere: the hyphen goes", "accom", "modate", "It is hard to accommodate them all here.", "accommodate"},
		{"hyphenated elsewhere: the hyphen stays", "full", "time", "We want a full-time job for all.", "full-time"},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf := helveticaPage(brokenPara(c.head, c.tail, c.evidence))
			before := readBack(t, pdf)
			if len(before) != 3 || before[0][len(before[0])-1] != c.head+"-" || before[1][0] != c.tail {
				t.Fatalf("setup: the paragraph reads %v, want three lines broken at %s-/%s", before, c.head, c.tail)
			}
			text := strings.Join(append(append(append([]string{}, before[0]...), before[1]...), before[2]...), " ")
			moved := strings.TrimPrefix(text, "We will ")
			out, cause := reflowed(t, pdf, 0, moved)
			if cause != "" {
				t.Fatalf("refused (%s)", cause)
			}
			got := readBack(t, out)
			var flat []string
			for li, ln := range got {
				for wi, w := range ln {
					if w == c.head+"-" && wi == len(ln)-1 && li+1 < len(got) {
						t.Fatalf("setup: the edit left the break at a line end: %v", got)
					}
					flat = append(flat, w)
				}
			}
			if !strings.Contains(" "+strings.Join(flat, " ")+" ", " "+c.joined+" ") {
				t.Errorf("reads back %v, want %q as one word", got, c.joined)
			}
			if !readsAsEdit(strings.Join(flat, " "), moved) {
				t.Errorf("reads back %v, not the edit %q", got, moved)
			}
			// Edited before the break on its own line, word for word the same count: the line is not the line the break
			// ended, so the whole word is tried first, and fits (the review of S07, W4 — keepBreak's content half).
			swapped := strings.Replace(text, "do all", "go all", 1)
			out, cause = reflowed(t, pdf, 0, swapped)
			if cause != "" {
				t.Fatalf("refused (%s)", cause)
			}
			if got := readBack(t, out); got[0][len(got[0])-1] == c.head+"-" {
				t.Errorf("an edit on the break's own line kept the break: %v", got)
			}
			// Edited after the break, the break stays where it was: split, with its hyphen.
			kept := strings.Replace(text, "clients", "customers", 1)
			out, cause = reflowed(t, pdf, 0, kept)
			if cause != "" {
				t.Fatalf("refused (%s)", cause)
			}
			if got := readBack(t, out); got[0][len(got[0])-1] != c.head+"-" || got[1][0] != c.tail {
				t.Errorf("an edit after the break reads back %v, want the break kept at the first line's end", got)
			}
		})
	}
}

// TestABreakNothingSettlesIsRefusedOnlyWhereItMoves — with no evidence either way, a break moved mid-line is
// `hyphen-unsure`; one that stays at its line end is re-set as before. And a hyphen inside a word on a line ("well-known")
// is never a break.
func TestABreakNothingSettlesIsRefusedOnlyWhereItMoves(t *testing.T) {
	pdf := helveticaPage(brokenPara("accom", "modate", ""))
	text := "We will do all that we can to accom- modate the needs of each and every one of our clients this year."
	if _, cause := reflowed(t, pdf, 0, strings.TrimPrefix(text, "We will ")); cause != causeHyphenUnsure {
		t.Errorf("a break with no evidence moved mid-line: cause %q, want %q", cause, causeHyphenUnsure)
	}
	if _, cause := reflowed(t, pdf, 0, strings.Replace(text, "clients", "customers", 1)); cause != "" {
		t.Errorf("a break with no evidence left at its line end refused (%s)", cause)
	}
	// A line ending in a hyphen before a capital is not a break (a range, a name): moved, it is neither rejoined nor
	// refused as unsure.
	up := helveticaPage(brokenPara("Smith", "Jones", ""))
	if _, cause := reflowed(t, up, 0, "do all that we can to Smith- Jones the needs of each and every one of our clients this year."); cause == causeHyphenUnsure {
		t.Errorf("a hyphen before a capital was taken for a break")
	}
	// Nor after a figure ("1990-" / "present"): a hyphen that breaks a word follows a letter.
	fig := helveticaPage(brokenPara("1990", "present", ""))
	if _, cause := reflowed(t, fig, 0, "do all that we can to 1990- present the needs of each and every one of our clients this year."); cause == causeHyphenUnsure {
		t.Errorf("a hyphen after a figure was taken for a break")
	}
	// "well-known" mid-line, moved by the edit: not a break, never touched.
	wk := helveticaPage("BT /F1 12 Tf 14 TL 72 700 Td (We will do all that a well-known firm can do for) Tj T* (the needs of every one of our clients.) Tj ET")
	out, cause := reflowed(t, wk, 0, "We do all that a well-known firm can do for the needs of every one of our clients.")
	if cause != "" {
		t.Fatalf("refused (%s)", cause)
	}
	if got := readBack(t, out); !strings.Contains(strings.Join(got[0], " "), "well-known") {
		t.Errorf("reads back %v, want well-known kept whole", got)
	}
}

// TestASoftHyphenIsAlwaysTheProducers — a break drawn with a soft hyphen (U+00AD) is the producer's by definition, and
// rejoins without its hyphen with no evidence read at all (rejoinBroken is given no document to read).
func TestASoftHyphenIsAlwaysTheProducers(t *testing.T) {
	g := func(text string, code byte) runGlyph { return runGlyph{code: []byte{code}, text: text, advance: 5} }
	head := reflowWord{glyphs: []runGlyph{g("a", 'a'), g("c", 'c'), g("­", 0xAD)}, width: 15}
	tail := reflowWord{glyphs: []runGlyph{g("t", 't')}, width: 5}
	lines := [][]reflowWord{{head}, {tail}}
	ew := func(w reflowWord) emitWord {
		e := emitWord{width: w.width}
		for i, gl := range w.glyphs {
			e.codes = append(e.codes, gl.code)
			e.spacing = append(e.spacing, textSpacing{th: 1})
			if i > 0 {
				e.kerns = append(e.kerns, 0)
			}
		}
		return e
	}
	if soft, ok := breakHyphen(head); !soft || !ok {
		t.Fatalf("setup: breakHyphen(ac\\u00ad) = %v, %v", soft, ok)
	}
	al := wordAlignment{ok: true, kept: map[int]int{0: 0, 1: 1}}
	got, unsure := rejoinBroken(nil, pageLayout{}, 1, lines, []emitWord{ew(head), ew(tail)}, al, []reflowWord{head, tail}, [][2]int{{0, 0}, {1, 0}})
	if len(got) != 1 || len(unsure) != 0 || got[0].broken == nil || string(bytesJoin(got[0].codes)) != "act" || got[0].width != 15 {
		t.Errorf("rejoined to %+v, want one word act, 15 wide, with its break point", got)
	}
}

func bytesJoin(cs [][]byte) []byte {
	var b []byte
	for _, c := range cs {
		b = append(b, c...)
	}
	return b
}

// TestBrokenWordsOverTheCorpus — S07 over the real-producer corpus. Every readable paragraph carrying a producer's break
// (a line ending in a break hyphen, the next opening in lower case) is edited twice: its first word removed, which moves
// the break, and its last word removed, which leaves every line before the last as it was. The first must read back as
// the edit with the break rejoined, or refuse `hyphen-unsure` (or another cause of its own); the second must keep the
// breaks before its last line where they were. The cost of reading the evidence is measured.
func TestBrokenWordsOverTheCorpus(t *testing.T) {
	corp := externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers"))
	if corp.absent != "" {
		t.Skip("SKIP (not a measurement): " + corp.absent)
	}
	causes, endCauses := map[string]int{}, map[string]int{}
	reset, rejoined, inPlace, moved, unread := 0, 0, 0, 0, 0
	var evidenceCost time.Duration
	evidenceCalls := 0
	for _, doc := range corp.docs {
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		for p := 1; p <= ctx.PageCount; p++ {
			l, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
			if err != nil {
				continue
			}
			for pi, para := range l.paragraphs {
				lines, _, cause := paragraphWords(para)
				if cause != "" || len(lines) < 2 {
					continue
				}
				breaks := 0
				for li := 0; li+1 < len(lines); li++ {
					if _, ok := breakHyphen(lines[li][len(lines[li])-1]); ok && startsLower(lines[li+1][0]) {
						breaks++
					}
				}
				if breaks == 0 {
					continue
				}
				t0 := time.Now()
				hyphenEvidence(ctx, l, p)
				evidenceCost += time.Since(t0)
				evidenceCalls++
				ws := editWords(para.text())
				for _, e := range []struct {
					edit  string
					moves bool
				}{
					{strings.Join(ws[1:], " "), true},
					{strings.Join(ws[:len(ws)-1], " "), false},
				} {
					c2, _ := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
					out, err := reflowParagraph(c2, p, pi, e.edit)
					if err != nil {
						t.Fatalf("%s p%d ¶%d: %v", doc.name, p, pi, err)
					}
					if out.content == nil {
						if e.moves {
							causes[out.cause]++
						} else {
							endCauses[out.cause]++
						}
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
					var got []string
					for i, n := 0, 0; i < len(para.lines) && n < editLetters(e.edit); i++ {
						ln, ok := lineAtBaseline(l3, paragraphBox(para), para.lines[i].y, para.lines[0].size)
						if !ok {
							break
						}
						got = append(got, ln.text)
						n += editLetters(ln.text)
					}
					if !e.moves {
						// Every break before the last line still ends the line it ended: an edit that never reached it left it
						// alone. (Lines are compared by break, not by text: the breaker setting a line at its natural spaces
						// may take a word up that the producer did not — P08.S03's measured drift, not this slice's.)
						for li := 0; li+2 < len(lines); li++ {
							head := lines[li][len(lines[li])-1]
							if _, ok := breakHyphen(head); !ok || !startsLower(lines[li+1][0]) {
								continue
							}
							if li >= len(got) {
								unread++
								continue
							}
							if ws := editWords(got[li]); len(ws) > 0 && ws[len(ws)-1] == head.text() {
								inPlace++
							} else {
								moved++
								t.Logf("%s p%d ¶%d: an edit at the end moved the break %q off line %d: %q", doc.name, p, pi, head.text(), li, got[li])
							}
						}
						continue
					}
					read := strings.Join(got, " ")
					if !readsAsEdit(read, e.edit) {
						t.Errorf("%s p%d ¶%d: wrote %q, reads back %q", doc.name, p, pi, e.edit, read)
						continue
					}
					reset++
					rejoined += len(editWords(e.edit)) - len(editWords(read))
					// No fragment of a break the edit carried is left mid-line ("accom- modate") — readsAsEdit allows the
					// split form, so this asks for it by line (the review of S07, W5).
					for _, ln := range got {
						ws := editWords(ln)
						for k := 0; k+1 < len(ws); k++ {
							if r := []rune(ws[k]); len(r) > 2 && r[len(r)-1] == '-' && unicode.IsLetter(r[len(r)-2]) && startsLowerText(ws[k+1]) &&
								strings.Contains(" "+e.edit+" ", " "+ws[k]+" "+ws[k+1]+" ") {
								t.Errorf("%s p%d ¶%d: a break left mid-line: %q %q in %q", doc.name, p, pi, ws[k], ws[k+1], ln)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("break moved: re-set %d (%d words rejoined), refused %v; breaks an edit did not reach: in place %d, moved %d, unread %d; evidence %d reads, %v each",
		reset, rejoined, causes, inPlace, moved, unread, evidenceCalls, evidenceCost/time.Duration(max(1, evidenceCalls)))
	t.Logf("an edit at the end refused: %v", endCauses)
	if float64(moved) > 0.05*float64(inPlace+moved) {
		t.Errorf("%d of %d breaks an edit did not reach moved off their line", moved, inPlace+moved)
	}
	if rejoined == 0 || inPlace == 0 {
		t.Errorf("%d words rejoined, %d paragraphs in place — the census asked nothing", rejoined, inPlace)
	}
}

func startsLowerText(s string) bool {
	for _, r := range s {
		return unicode.IsLower(r)
	}
	return false
}

// TestAWordAcrossThreeLinesIsUnsure — a word hyphenated across three lines ("con-" / "tra-" / "ry"): the middle is the
// tail of one break and the head of the next. Each break, rejoined alone, would leave the other's fragment mid-line
// ("contra- ry"), so both are left as their words and returned unsure — refused wherever they move (the review of S07,
// W1). Soft hyphens, so no evidence is read and the chain alone decides.
func TestAWordAcrossThreeLinesIsUnsure(t *testing.T) {
	g := func(text string) runGlyph { return runGlyph{code: []byte(text), text: text, advance: 5} }
	word := func(texts ...string) reflowWord {
		w := reflowWord{}
		for _, x := range texts {
			w.glyphs = append(w.glyphs, g(x))
			w.width += 5
		}
		return w
	}
	ew := func(w reflowWord) emitWord {
		e := emitWord{width: w.width}
		for i, gl := range w.glyphs {
			e.codes = append(e.codes, gl.code)
			e.spacing = append(e.spacing, textSpacing{th: 1})
			if i > 0 {
				e.kerns = append(e.kerns, 0)
			}
		}
		return e
	}
	con, tra, ry := word("c", "o", "n", "\u00ad"), word("t", "r", "a", "\u00ad"), word("r", "y")
	lines := [][]reflowWord{{con}, {tra}, {ry}}
	al := wordAlignment{ok: true, kept: map[int]int{0: 0, 1: 1, 2: 2}}
	got, unsure := rejoinBroken(nil, pageLayout{}, 1, lines, []emitWord{ew(con), ew(tra), ew(ry)}, al,
		[]reflowWord{con, tra, ry}, [][2]int{{0, 0}, {1, 0}, {2, 0}})
	if len(got) != 3 || len(unsure) != 2 || unsure[0] != [2]int{0, 1} || unsure[1] != [2]int{1, 2} {
		t.Errorf("rejoined to %d items with unsure %v, want the three words and both breaks unsure", len(got), unsure)
	}
	for _, w := range got {
		if w.broken != nil {
			t.Errorf("a break in the chain was rejoined alone")
		}
	}
	// The stimulus: either break alone, with its neighbour's fragment not a break, rejoins.
	one, u := rejoinBroken(nil, pageLayout{}, 1, [][]reflowWord{{con}, {word("t", "r", "a")}}, []emitWord{ew(con), ew(word("t", "r", "a"))},
		wordAlignment{ok: true, kept: map[int]int{0: 0, 1: 1}}, []reflowWord{con, word("t", "r", "a")}, [][2]int{{0, 0}, {1, 0}})
	if len(one) != 1 || one[0].broken == nil || len(u) != 0 {
		t.Fatalf("setup: a single soft-hyphen break did not rejoin (%d items, unsure %v)", len(one), u)
	}
}

// TestABreakPastTheAlignmentsBoundIsRefused — past alignWords' bound nothing is known kept, so a break cannot be followed
// by its halves; the paragraph carrying one refuses rather than leave it to land mid-line (the review of S07, I1).
func TestABreakPastTheAlignmentsBoundIsRefused(t *testing.T) {
	g := func(text string) runGlyph { return runGlyph{code: []byte(text), text: text, advance: 5} }
	head := reflowWord{glyphs: []runGlyph{g("a"), g("c"), g("-")}}
	tail := reflowWord{glyphs: []runGlyph{g("t")}}
	_, unsure := rejoinBroken(nil, pageLayout{}, 1, [][]reflowWord{{head}, {tail}}, nil, wordAlignment{}, nil, nil)
	if len(unsure) != 1 || unsure[0][0] >= 0 {
		t.Errorf("unsure %v, want the refusal marker", unsure)
	}
	if _, unsure := rejoinBroken(nil, pageLayout{}, 1, [][]reflowWord{{head}, {reflowWord{glyphs: []runGlyph{g("T")}}}}, nil, wordAlignment{}, nil, nil); len(unsure) != 0 {
		t.Errorf("a paragraph with no break refused past the bound: %v", unsure)
	}
}
