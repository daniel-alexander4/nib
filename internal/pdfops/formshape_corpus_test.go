package pdfops

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// The form-shape instrument (/pending 391): is a structural fingerprint of a form STABLE across a new revision or a
// re-scan of the same form, and DISTINCT across different forms? If it is not, remembering a form's shape is not worth
// designing, and /pending 395 and 396 learn the same fact.
//
// Not a tier and not a gate, like build/accuracy.sh and build/formrt.sh, and for their reason: real forms are somebody's,
// so the corpus is a LOCAL list that is never committed (build/formshape.sh sets NIB_FORMSHAPE_LIST). One path per line;
// a BLANK LINE starts a new form, so the paths in one block are the same form (revisions, re-scans at another DPI) and
// paths in different blocks are different forms. It prints COUNTS only — never a path, a word or a name.
//
// # What is fingerprinted
//
// Only what MapPage reads, because it is the one reader whose answer is a fraction of the displayed page and so does not
// move with a page size or a scan's DPI. Three readings of one page, scored 0..1 and reported side by side so the
// numbers say which one carries the signal:
//
//   - shapes: the page's rules and boxes (kind and rectangle). Two pages score the fraction of the larger page's shapes
//     that the other page has within shapeTol of the page, symmetric.
//   - words: the set of lower-cased words the page's text runs read, Jaccard. A hidden (OCR) layer counts: a re-scan
//     that was recognised has words, one that was not has none.
//   - both: the mean of the two where both pages have that reading, otherwise the one they have.
//
// A page with no shapes and no words (a scan with no text layer, or text drawn as outlines) has NOTHING this reader can
// fingerprint. A pair with such a side is counted as "unmeasured", never scored 0: a scan's picture needs the window's
// reader (web/detect.js) and this instrument does not have it. That is the declared gap, and the unmeasured count is how
// large it is on the list.
//
// Pages are compared by position (page i with page i) for the first shapePages pages. A revision that adds a cover page
// scores low here, and that is a finding about the fingerprint, not a defect of the comparison.
const (
	shapeTol   = 0.015 // a rectangle edge within this fraction of the page is the same edge
	shapePages = 3     // pages read per document; MapPage costs seconds a page
)

// shapeReading is one page's three readings, reduced to what the comparison needs.
type shapeReading struct {
	shapes []shapeRect
	words  map[string]bool
}

type shapeRect struct {
	kind string
	r    MapRect
}

func (s shapeReading) empty() bool { return len(s.shapes) == 0 && len(s.words) == 0 }

// readingOf reduces a page map. A "white" shape is the ground a form lays under a blank and says nothing about the
// form's structure that its box does not; it is left out so a producer that paints grounds does not look different.
func readingOf(m PageMap) shapeReading {
	r := shapeReading{words: map[string]bool{}}
	for _, s := range m.Shapes {
		if s.Kind == "white" {
			continue
		}
		r.shapes = append(r.shapes, shapeRect{s.Kind, s.Rect})
	}
	for _, w := range m.Widgets {
		r.shapes = append(r.shapes, shapeRect{"widget:" + w.Kind, w.Rect})
	}
	for _, t := range m.Text {
		for _, w := range strings.FieldsFunc(strings.ToLower(t.Text), func(c rune) bool {
			return !unicode.IsLetter(c) && !unicode.IsDigit(c)
		}) {
			r.words[w] = true
		}
	}
	return r
}

func sameRect(a, b MapRect) bool {
	for i := range a {
		d := a[i] - b[i]
		if d < 0 {
			d = -d
		}
		if d > shapeTol {
			return false
		}
	}
	return true
}

// shapeScore is the symmetric fraction of shapes each side finds on the other. Both empty is not asked (the caller
// counts it unmeasured); one empty scores 0.
func shapeScore(a, b []shapeRect) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	found := func(from, in []shapeRect) int {
		n := 0
		for _, x := range from {
			for _, y := range in {
				if x.kind == y.kind && sameRect(x.r, y.r) {
					n++
					break
				}
			}
		}
		return n
	}
	f := float64(found(a, b))/float64(len(a)) + float64(found(b, a))/float64(len(b))
	return f / 2
}

func wordScore(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	in := 0
	for w := range a {
		if b[w] {
			in++
		}
	}
	union := len(a) + len(b) - in
	if union == 0 {
		return 1
	}
	return float64(in) / float64(union)
}

// pairScores returns the three scores for one pair of pages, and ok=false when either side has nothing to fingerprint.
func pairScores(a, b shapeReading) (shapes, words, both float64, ok bool) {
	if a.empty() || b.empty() {
		return 0, 0, 0, false
	}
	shapes = shapeScore(a.shapes, b.shapes)
	words = wordScore(a.words, b.words)
	switch {
	case len(a.shapes) > 0 && len(b.shapes) > 0 && len(a.words) > 0 && len(b.words) > 0:
		both = (shapes + words) / 2
	case len(a.shapes) > 0 && len(b.shapes) > 0:
		both = shapes
	default:
		both = words
	}
	return shapes, words, both, true
}

// documentScores is the mean of pairScores over the pages both documents have, up to shapePages; ok is false when no
// page pair could be measured.
func documentScores(a, b []shapeReading) (shapes, words, both float64, ok bool) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var cnt int
	for i := 0; i < n; i++ {
		s, w, c, k := pairScores(a[i], b[i])
		if !k {
			continue
		}
		shapes, words, both = shapes+s, words+w, both+c
		cnt++
	}
	if cnt == 0 {
		return 0, 0, 0, false
	}
	f := float64(cnt)
	return shapes / f, words / f, both / f, true
}

// parseShapeList reads the block list: paths, a blank line between forms, # comments.
func parseShapeList(raw string) [][]string {
	var forms [][]string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			forms = append(forms, cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "#"):
		default:
			cur = append(cur, line)
		}
	}
	flush()
	return forms
}

// separation reports, for one variant, the threshold sweep: at each threshold the same-form pairs it would reject and
// the different-form pairs it would accept. A threshold with both 0 exists exactly when the variant separates the list.
func separation(same, diff []float64) (lines []string, clean bool) {
	for _, th := range []float64{0.5, 0.6, 0.7, 0.8, 0.9} {
		rej, acc := 0, 0
		for _, s := range same {
			if s < th {
				rej++
			}
		}
		for _, d := range diff {
			if d >= th {
				acc++
			}
		}
		if rej == 0 && acc == 0 {
			clean = true
		}
		lines = append(lines, fmt.Sprintf("  at %.1f: %d of %d same-form pairs rejected, %d of %d different-form pairs accepted", th, rej, len(same), acc, len(diff)))
	}
	return lines, clean
}

func TestFormShapeCorpus(t *testing.T) {
	list := os.Getenv("NIB_FORMSHAPE_LIST")
	if list == "" {
		t.Skip("NIB_FORMSHAPE_LIST not set: this is an instrument, run build/formshape.sh")
	}
	raw, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	forms := parseShapeList(string(raw))
	type doc struct {
		form  int
		pages []shapeReading
	}
	var docs []doc
	unreadable := 0
	for fi, paths := range forms {
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				unreadable++
				continue
			}
			var pages []shapeReading
			for pg := 1; pg <= shapePages; pg++ {
				m, err := MapPage(data, pg)
				if err != nil {
					break
				}
				pages = append(pages, readingOf(m))
			}
			if len(pages) == 0 {
				unreadable++
				continue
			}
			docs = append(docs, doc{fi, pages})
		}
	}
	type variant struct {
		name       string
		same, diff []float64
	}
	vs := []*variant{{name: "shapes"}, {name: "words"}, {name: "both"}}
	unmeasuredSame, unmeasuredDiff := 0, 0
	for i := range docs {
		for j := i + 1; j < len(docs); j++ {
			s, w, c, ok := documentScores(docs[i].pages, docs[j].pages)
			sameForm := docs[i].form == docs[j].form
			if !ok {
				if sameForm {
					unmeasuredSame++
				} else {
					unmeasuredDiff++
				}
				continue
			}
			for k, x := range []float64{s, w, c} {
				if sameForm {
					vs[k].same = append(vs[k].same, x)
				} else {
					vs[k].diff = append(vs[k].diff, x)
				}
			}
		}
	}
	blank := 0
	for _, d := range docs {
		all := true
		for _, p := range d.pages {
			if !p.empty() {
				all = false
			}
		}
		if all {
			blank++
		}
	}
	fmt.Printf("FORMSHAPE forms %d, documents %d (%d unreadable, %d with nothing this reader fingerprints), pages read per document up to %d\n", len(forms), len(docs), unreadable, blank, shapePages)
	fmt.Printf("FORMSHAPE pairs unmeasured (a side has nothing to fingerprint): %d same-form, %d different-form\n", unmeasuredSame, unmeasuredDiff)
	for _, v := range vs {
		sort.Float64s(v.same)
		sort.Float64s(v.diff)
		lo, hi := "none", "none"
		if len(v.same) > 0 {
			lo = fmt.Sprintf("%.3f", v.same[0])
		}
		if len(v.diff) > 0 {
			hi = fmt.Sprintf("%.3f", v.diff[len(v.diff)-1])
		}
		lines, clean := separation(v.same, v.diff)
		fmt.Printf("FORMSHAPE %s: lowest same-form score %s, highest different-form score %s, a clean threshold exists: %v\n", v.name, lo, hi, clean)
		for _, l := range lines {
			fmt.Println("FORMSHAPE" + l)
		}
	}
	if len(forms) < 2 {
		fmt.Println("FORMSHAPE note: fewer than two forms listed, so no different-form pair exists and distinctness is not measured")
	}
}

// The scoring is the instrument's arithmetic, so it is tested without a corpus: a figure printed by a comparison that
// scored a rescaled copy low, or two different forms high, would be a number about the comparison.

func shapesOf(rs ...[5]float64) []shapeRect {
	var out []shapeRect
	for _, r := range rs {
		kind := map[float64]string{0: "h", 1: "v", 2: "box"}[r[4]]
		out = append(out, shapeRect{kind, MapRect{r[0], r[1], r[2], r[3]}})
	}
	return out
}

func TestFormShapeScoring(t *testing.T) {
	a := shapesOf([5]float64{.1, .1, .9, .11, 0}, [5]float64{.1, .1, .11, .9, 1}, [5]float64{.2, .2, .5, .3, 2})

	if got := shapeScore(a, a); got != 1 {
		t.Fatalf("a page against itself scored %v, want 1", got)
	}
	// A re-scan lands every edge a little off, inside the tolerance.
	moved := shapesOf([5]float64{.105, .098, .895, .112, 0}, [5]float64{.108, .1, .112, .9, 1}, [5]float64{.205, .195, .495, .305, 2})
	if got := shapeScore(a, moved); got != 1 {
		t.Fatalf("a page with every edge inside the tolerance scored %v, want 1", got)
	}
	// An edge past the tolerance is a different shape.
	far := shapesOf([5]float64{.15, .1, .9, .11, 0}, [5]float64{.1, .1, .11, .9, 1}, [5]float64{.2, .2, .5, .3, 2})
	if got := shapeScore(a, far); got >= 1 || got < 0.5 {
		t.Fatalf("one of three shapes moved past the tolerance scored %v, want between 0.5 and 1", got)
	}
	// A different kind at the same place is not the same shape.
	otherKind := shapesOf([5]float64{.1, .1, .9, .11, 2}, [5]float64{.1, .1, .11, .9, 2}, [5]float64{.2, .2, .5, .3, 2})
	if got := shapeScore(a, otherKind); got >= 1 {
		t.Fatalf("kind ignored: scored %v", got)
	}
	// The score is symmetric: a page that holds the other plus extra shapes is penalised from both sides.
	more := append(append([]shapeRect{}, a...), shapesOf([5]float64{.6, .6, .9, .7, 2})...)
	if shapeScore(a, more) != shapeScore(more, a) || shapeScore(a, more) >= 1 {
		t.Fatalf("subset not penalised symmetrically: %v vs %v", shapeScore(a, more), shapeScore(more, a))
	}

	w := func(ws ...string) map[string]bool {
		m := map[string]bool{}
		for _, x := range ws {
			m[x] = true
		}
		return m
	}
	if got := wordScore(w("name", "date"), w("name", "date")); got != 1 {
		t.Fatalf("identical words scored %v", got)
	}
	if got := wordScore(w("name", "date"), w("amount", "total")); got != 0 {
		t.Fatalf("disjoint words scored %v", got)
	}
	if got := wordScore(w("a", "b", "c"), w("b", "c", "d")); got != 0.5 {
		t.Fatalf("2 shared of 4 scored %v, want 0.5", got)
	}

	// A page with nothing to fingerprint is unmeasured, never a zero.
	if _, _, _, ok := pairScores(shapeReading{}, shapeReading{shapes: a, words: w("x")}); ok {
		t.Fatal("a page with nothing scored instead of being unmeasured")
	}
	// A scan that was recognised has words and no shapes: it is measured, on words alone.
	_, _, both, ok := pairScores(shapeReading{words: w("name", "date")}, shapeReading{words: w("name", "date")})
	if !ok || both != 1 {
		t.Fatalf("words-only pages: ok=%v both=%v, want measured at 1", ok, both)
	}
	// And a words-only side against a shapes-and-words side is scored on words, not dragged to 0 by the missing shapes.
	_, _, both, ok = pairScores(shapeReading{words: w("name", "date")}, shapeReading{shapes: a, words: w("name", "date")})
	if !ok || both != 1 {
		t.Fatalf("scan against born-digital: ok=%v both=%v, want measured at 1 on the words they share", ok, both)
	}
}

func TestFormShapeListAndSeparation(t *testing.T) {
	forms := parseShapeList("# a comment\n/a/1.pdf\n/a/2.pdf\r\n\n\n/b/1.pdf\n \n/c/1.pdf\n/c/2.pdf\n")
	want := [][]string{{"/a/1.pdf", "/a/2.pdf"}, {"/b/1.pdf"}, {"/c/1.pdf", "/c/2.pdf"}}
	if fmt.Sprint(forms) != fmt.Sprint(want) {
		t.Fatalf("blocks = %v, want %v", forms, want)
	}
	// A list that separates reports a clean threshold; one whose same-form pair scores under a different-form pair does not.
	if _, clean := separation([]float64{0.9, 0.95}, []float64{0.1, 0.3}); !clean {
		t.Fatal("a separated list reported no clean threshold")
	}
	if _, clean := separation([]float64{0.4, 0.95}, []float64{0.6, 0.3}); clean {
		t.Fatal("an overlapping list reported a clean threshold")
	}
}

func TestFormShapeReadingOfAPage(t *testing.T) {
	m := PageMap{
		Shapes:  []MapShape{{Rect: MapRect{.1, .1, .9, .11}, Kind: "h"}, {Rect: MapRect{0, 0, 1, 1}, Kind: "white"}},
		Widgets: []MapWidget{{Rect: MapRect{.2, .2, .5, .3}, Kind: "text"}},
		Text:    []MapText{{Text: "Full Name, DATE-of-birth 12"}, {Text: "name"}},
	}
	r := readingOf(m)
	if len(r.shapes) != 2 {
		t.Fatalf("shapes = %v, want the rule and the widget and not the white ground", r.shapes)
	}
	if r.shapes[1].kind != "widget:text" {
		t.Fatalf("a widget is read as %q, want widget:text so it never matches a drawn box", r.shapes[1].kind)
	}
	for _, w := range []string{"full", "name", "date", "of", "birth", "12"} {
		if !r.words[w] {
			t.Fatalf("word %q missing from %v", w, r.words)
		}
	}
	if len(r.words) != 6 {
		t.Fatalf("words = %v, want 6 distinct lower-cased words", r.words)
	}
}
