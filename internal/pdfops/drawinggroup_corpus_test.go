package pdfops

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"nib/internal/pdfread"
)

// The drawing-grouping instrument (PLAN-image-edit P03.S03 T02): how the ONE grouping of painted paths
// (`groupDrawnPaths`) reads real pages, before the window offers a drawing as one object. Not a tier and not a gate,
// like build/objectdelete.sh: the corpus is a LOCAL list, never committed, and `build/drawinggroup.sh` prints COUNTS
// only, never a path.
//
// Its oracle is fixed before its numbers (the plan-review pin of 2026-11-10): a hand-labelled sample of pages, in a
// local labels file beside the corpus (never committed). A label line is
//
//	<first 12 hex of the document's SHA-256> TAB <page> TAB <drawings a person sees on it> TAB <group>;<group>;...
//
// where each group is a drawing the grouping found that a person reads as exactly one whole drawing, written as its
// members' keys (`pathKey`). A labelled drawing is matched when today's grouping still has a group with exactly those
// members; the metric is matched ÷ seen, and the threshold `drawingGroupThreshold`. The dump mode writes the sample
// pages rendered with each group outlined and numbered, which is what the labels are read from.

const (
	// drawingGroupThreshold is the share of the labelled drawings a group must match exactly (same members) before the
	// window offers drawings (P03.S03 slice-grill pin 6, fixed before the instrument ran).
	drawingGroupThreshold = 0.9
	// drawingGroupFloor is the fewest groups a corpus run must read to say anything.
	drawingGroupFloor = 1000
	// drawingSamplePages is how many pages the dump writes by default.
	drawingSamplePages = 20
)

// pathKey names one painted path for a label: the stream that draws it and where its first operand starts. Within one
// document's bytes it names the same path at every read.
func pathKey(p drawnPath) string { return fmt.Sprintf("%d:%d", p.stm, p.span.start) }

// groupKey names a group by its members' keys, sorted, comma-joined.
func groupKey(paths []drawnPath, g drawnGroup) string {
	keys := make([]string, len(g.members))
	for i, m := range g.members {
		keys[i] = pathKey(paths[m])
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// groupDrawnPathsBeforeT01 is the grouping as `untaggedPieces` wrote it before T01, kept here only to count the pages
// where the lift changed `ReadUntagged`'s answer: marked paths dropped, standing compared as bare/decoration and
// page/form only.
func groupDrawnPathsBeforeT01(painted []drawnPath) int {
	var paths []drawnPath
	for _, p := range painted {
		if !p.marked {
			paths = append(paths, p)
		}
	}
	sort.SliceStable(paths, func(i, j int) bool { return paths[i].box[0] < paths[j].box[0] })
	group := make([]int, len(paths))
	for i := range group {
		group[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for group[i] != i {
			group[i] = group[group[i]]
			i = group[i]
		}
		return i
	}
	spent := 0
	for i := range paths {
		for j := i + 1; j < len(paths) && paths[j].box[0] <= paths[i].box[2] && spent < clusterBudget; j++ {
			spent++
			a, b := paths[i], paths[j]
			if a.artifact != b.artifact || a.inForm != b.inForm || !touches(a.box, b.box) {
				continue
			}
			if (plainBoxPath(a) && covers(a.box, b.box)) || (plainBoxPath(b) && covers(b.box, a.box)) {
				continue
			}
			group[find(j)] = find(i)
		}
	}
	roots := map[int]bool{}
	for i := range paths {
		roots[find(i)] = true
	}
	return len(roots)
}

// drawingTally is what a run counts.
type drawingTally struct {
	docs, unreadable, pages, budgetPages, paths, groups, multi, largest int
	kinds                                                               map[string]int
	// Groups holding at least one path of each kind the door will refuse by.
	marked, form, clips, patterned, mixedCTM int
	// changedPages counts pages whose unmarked paths the pre-T01 rule grouped into a different number of drawings.
	changedPages int
	// What the objects listing says of the drawings: removable and movable, and each refusal by cause.
	removable, movable       int
	notRemovable, notMovable map[string]int
	// The labelled sample: drawings seen, and matched, in all and in groups of two paths or more.
	labelPages, seen, matched, seenMulti, matchedMulti, labelMissing int
}

// drawingPage is one page's grouping.
type drawingPage struct {
	paths  []drawnPath // as the walk kept them; a group's members index it
	groups []drawnGroup
	spent  bool
	sp     displaySpace
}

func readDrawingPage(pdf []byte, page int) (drawingPage, int, error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return drawingPage{}, 0, err
	}
	walked := pdfread.Pages(ctx)
	if page > len(walked) {
		return drawingPage{}, len(walked), nil
	}
	pg := pageAt(ctx, walked, page)
	pr, err := readPageDrawings(ctx, pg)
	if err != nil {
		return drawingPage{}, len(walked), err
	}
	groups, spent := groupDrawnPaths(pr.paths)
	return drawingPage{paths: pr.paths, groups: groups, spent: spent, sp: newDisplaySpace(pg)}, len(walked), nil
}

func (t *drawingTally) page(dp drawingPage, before int) {
	t.pages++
	if dp.spent {
		t.budgetPages++
		return // its own column: never offered, so its groups are not counted as drawings
	}
	t.paths += len(dp.paths)
	unmarked := 0
	for _, g := range dp.groups {
		t.groups++
		first := dp.paths[g.members[0]]
		if !first.marked {
			unmarked++
		}
		if len(g.members) > 1 {
			t.multi++
		}
		if len(g.members) > t.largest {
			t.largest = len(g.members)
		}
		t.kinds[drawingKind(first, len(g.members))]++
		var clips, pat, mixed bool
		for _, m := range g.members {
			p := dp.paths[m]
			clips = clips || p.clips
			pat = pat || p.patterned
			mixed = mixed || p.ctm != first.ctm
		}
		if first.marked {
			t.marked++
		}
		if first.inForm {
			t.form++
		}
		if clips {
			t.clips++
		}
		if pat {
			t.patterned++
		}
		if mixed {
			t.mixedCTM++
		}
	}
	if unmarked != before {
		t.changedPages++
	}
	listed, _ := drawingsOf(1, dp.sp, dp.paths)
	for _, d := range listed {
		if d.Editable {
			t.removable++
		} else {
			t.notRemovable[d.Why]++
		}
		if d.Movable {
			t.movable++
		} else {
			t.notMovable[d.MoveWhy]++
		}
	}
}

func (t *drawingTally) summary() string {
	share := "n/a"
	if t.seen > 0 {
		share = fmt.Sprintf("%.3f", float64(t.matched)/float64(t.seen))
	}
	shareMulti := "n/a"
	if t.seenMulti > 0 {
		shareMulti = fmt.Sprintf("%.3f", float64(t.matchedMulti)/float64(t.seenMulti))
	}
	return fmt.Sprintf("DRAWINGGROUP docs=%d unreadable=%d pages=%d budget-spent-pages=%d paths=%d groups=%d multi=%d largest=%d "+
		"kinds[drawing=%d rule=%d box=%d] with[marked=%d form=%d clips=%d patterned=%d mixed-ctm=%d] changed-from-pre-T01-pages=%d\n"+
		"DRAWINGOBJECTS removable=%d movable=%d not-removable[%s] not-movable[%s]\n"+
		"DRAWINGLABELS pages=%d seen=%d matched=%d share=%s multi-seen=%d multi-matched=%d multi-share=%s labelled-pages-missing=%d threshold=%.2f",
		t.docs, t.unreadable, t.pages, t.budgetPages, t.paths, t.groups, t.multi, t.largest,
		t.kinds["drawing"], t.kinds["rule"], t.kinds["box"], t.marked, t.form, t.clips, t.patterned, t.mixedCTM, t.changedPages,
		t.removable, t.movable, counts(t.notRemovable), counts(t.notMovable),
		t.labelPages, t.seen, t.matched, share, t.seenMulti, t.matchedMulti, shareMulti, t.labelMissing, drawingGroupThreshold)
}

// drawingLabel is one labelled page.
type drawingLabel struct {
	page, seen int
	groups     []string
}

func readDrawingLabels(path string) (map[string][]drawingLabel, error) {
	out := map[string][]drawingLabel{}
	if path == "" {
		return out, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for n, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			return nil, fmt.Errorf("labels line %d has %d fields, want 4", n+1, len(f))
		}
		page, perr := strconv.Atoi(f[1])
		seen, serr := strconv.Atoi(f[2])
		if perr != nil || serr != nil || page < 1 || seen < 0 {
			return nil, fmt.Errorf("labels line %d: page and count are not numbers", n+1)
		}
		l := drawingLabel{page: page, seen: seen}
		for _, g := range strings.Split(f[3], ";") {
			if g = strings.TrimSpace(g); g != "" {
				l.groups = append(l.groups, g)
			}
		}
		if len(l.groups) > seen {
			return nil, fmt.Errorf("labels line %d accepts %d groups of %d drawings seen", n+1, len(l.groups), seen)
		}
		out[f[0]] = append(out[f[0]], l)
	}
	return out, nil
}

// score holds one labelled page against today's grouping: a drawing a person saw is matched when a group with exactly
// its members still exists. A drawing seen and not accepted at labelling (the grouping split or merged it) counts as
// seen and not matched.
func (t *drawingTally) score(dp drawingPage, l drawingLabel) {
	t.labelPages++
	now := map[string]bool{}
	for _, g := range dp.groups {
		now[groupKey(dp.paths, g)] = true
	}
	t.seen += l.seen
	for _, g := range l.groups {
		multi := strings.Contains(g, ",")
		if multi {
			t.seenMulti++
		}
		if now[g] {
			t.matched++
			if multi {
				t.matchedMulti++
			}
		}
	}
}

func (t *drawingTally) verdict() (bool, string) {
	switch {
	case t.groups < drawingGroupFloor:
		return false, fmt.Sprintf("DRAWINGGROUP TOO-FEW: %d groups read, the floor is %d", t.groups, drawingGroupFloor)
	case t.labelPages == 0:
		return false, "DRAWINGLABELS NONE: no labelled page was read, so the threshold was not asked"
	case float64(t.matched) < drawingGroupThreshold*float64(t.seen):
		return false, fmt.Sprintf("DRAWINGLABELS BELOW: %d of %d labelled drawings matched, under %.2f", t.matched, t.seen, drawingGroupThreshold)
	}
	return true, ""
}

func TestDrawingGroupCorpus(t *testing.T) {
	list := os.Getenv("NIB_DRAWINGGROUP_LIST")
	if list == "" {
		t.Skip("NIB_DRAWINGGROUP_LIST not set: this is an instrument, run build/drawinggroup.sh")
	}
	labels, err := readDrawingLabels(os.Getenv("NIB_DRAWINGGROUP_LABELS"))
	if err != nil {
		t.Fatal(err)
	}
	dump := os.Getenv("NIB_DRAWINGGROUP_DUMP")
	// skip passes over the first skip sampleable pages: a second, fresh sample for judging a change made after the
	// first was read.
	skip := 0
	if s := os.Getenv("NIB_DRAWINGGROUP_SKIP"); s != "" {
		if skip, err = strconv.Atoi(s); err != nil {
			t.Fatal(err)
		}
	}
	want := drawingSamplePages
	if s := os.Getenv("NIB_DRAWINGGROUP_SAMPLE"); s != "" {
		if want, err = strconv.Atoi(s); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	type candidate struct {
		sha  string
		page int
		data []byte
		dp   drawingPage
	}
	var sample []candidate
	tally := drawingTally{kinds: map[string]int{}, notRemovable: map[string]int{}, notMovable: map[string]int{}}
	labelled := map[string]bool{}
	read := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		p := strings.TrimSpace(line)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			continue
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])[:12]
		if read[sha] {
			continue // the same bytes listed twice are one document: counted, scored and sampled once
		}
		read[sha] = true
		tally.docs++
		picked := false
		for page := 1; page <= 4; page++ {
			dp, n, perr := readDrawingPage(data, page)
			if perr != nil {
				tally.unreadable++
				break
			}
			if page > n {
				break
			}
			tally.page(dp, groupDrawnPathsBeforeT01(dp.paths))
			for _, l := range labels[sha] {
				if l.page == page && !dp.spent {
					tally.score(dp, l)
					labelled[fmt.Sprintf("%s:%d", sha, page)] = true
				}
			}
			if dump != "" && !picked && sampleable(dp) {
				sample = append(sample, candidate{sha, page, data, dp})
				picked = true
			}
		}
	}
	for sha, ls := range labels {
		for _, l := range ls {
			if !labelled[fmt.Sprintf("%s:%d", sha, l.page)] {
				tally.labelMissing++
			}
		}
	}
	fmt.Println(tally.summary())
	if dump != "" {
		sort.Slice(sample, func(i, j int) bool { return sample[i].sha < sample[j].sha })
		if skip > len(sample) {
			skip = len(sample)
		}
		sample = sample[skip:]
		if len(sample) > want {
			sample = sample[:want]
		}
		for _, c := range sample {
			if err := dumpDrawingPage(dump, c.sha, c.page, c.data, c.dp); err != nil {
				t.Fatal(err)
			}
		}
		fmt.Printf("DRAWINGDUMP pages=%d\n", len(sample))
		return
	}
	if ok, msg := tally.verdict(); !ok {
		t.Fatal(msg)
	}
}

// sampleable is a page a person can label in one look: the budget not spent, between 2 and 25 unmarked groups, and
// at least one of them more than one path. Declared: the sample is biased to such pages, so the share speaks for
// pages like them and not for a page of 300 rules.
func sampleable(dp drawingPage) bool {
	if dp.spent {
		return false
	}
	n, multi := 0, false
	for _, g := range dp.groups {
		if dp.paths[g.members[0]].marked {
			continue
		}
		n++
		multi = multi || len(g.members) > 1
	}
	return n >= 2 && n <= 25 && multi
}

// dumpDrawingPage writes the page rendered with each unmarked group outlined and numbered (<sha>-p<page>.png) and a
// list of the groups by number with their keys (<sha>-p<page>.txt), for a person to label.
func dumpDrawingPage(dir, sha string, page int, data []byte, dp drawingPage) error {
	tmp, err := os.MkdirTemp("", "drawingdump")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	in := filepath.Join(tmp, "in.pdf")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return err
	}
	pg := strconv.Itoa(page)
	if out, err := exec.Command("pdftoppm", "-r", "72", "-f", pg, "-l", pg, "-png", "-singlefile", in, filepath.Join(tmp, "p")).CombinedOutput(); err != nil {
		return fmt.Errorf("pdftoppm: %v: %s", err, out)
	}
	f, err := os.Open(filepath.Join(tmp, "p.png"))
	if err != nil {
		return err
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	var list strings.Builder
	n := 0
	for _, g := range dp.groups {
		first := dp.paths[g.members[0]]
		if first.marked {
			continue
		}
		n++
		r, ok := pieceRect(dp.sp, g.box)
		c := dumpColours[(n-1)%len(dumpColours)]
		fmt.Fprintf(&list, "%d\t%s\tpaths=%d\tbox=%.3f,%.3f,%.3f,%.3f\tform=%v\tdecoration=%v\t%s\n", n, drawingKind(first, len(g.members)),
			len(g.members), r[0], r[1], r[2], r[3], first.inForm, first.artifact, groupKey(dp.paths, g))
		if !ok {
			continue
		}
		x0, y0, x1, y1 := int(r[0]*float64(w)), int(r[1]*float64(h)), int(r[2]*float64(w)), int(r[3]*float64(h))
		outline(img, x0-2, y0-2, x1+2, y1+2, c)
		digits(img, x0, y0-12, n, c)
	}
	base := filepath.Join(dir, fmt.Sprintf("%s-p%d", sha, page))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	if err := os.WriteFile(base+".png", buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.WriteFile(base+".txt", []byte(list.String()), 0o600)
}

var dumpColours = []color.RGBA{{230, 25, 75, 255}, {60, 180, 75, 255}, {0, 130, 200, 255}, {245, 130, 48, 255},
	{145, 30, 180, 255}, {70, 200, 200, 255}, {240, 50, 230, 255}, {128, 128, 0, 255}}

func outline(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for t := 0; t < 2; t++ {
		for x := x0; x <= x1; x++ {
			img.Set(x, y0+t, c)
			img.Set(x, y1-t, c)
		}
		for y := y0; y <= y1; y++ {
			img.Set(x0+t, y, c)
			img.Set(x1-t, y, c)
		}
	}
}

// digitGlyphs is a 3×5 bitmap of each digit, row by row.
var digitGlyphs = [10][5]string{
	{"###", "#.#", "#.#", "#.#", "###"}, {".#.", "##.", ".#.", ".#.", "###"}, {"###", "..#", "###", "#..", "###"},
	{"###", "..#", "###", "..#", "###"}, {"#.#", "#.#", "###", "..#", "..#"}, {"###", "#..", "###", "..#", "###"},
	{"###", "#..", "###", "#.#", "###"}, {"###", "..#", "..#", "..#", "..#"}, {"###", "#.#", "###", "#.#", "###"},
	{"###", "#.#", "###", "..#", "###"},
}

// digits writes n at (x, y) in 2-pixel cells on a white ground.
func digits(img *image.RGBA, x, y, n int, c color.RGBA) {
	s := strconv.Itoa(n)
	draw.Draw(img, image.Rect(x-1, y-1, x+len(s)*8+1, y+11), image.NewUniform(color.White), image.Point{}, draw.Src)
	for i, ch := range s {
		g := digitGlyphs[ch-'0']
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if g[row][col] == '#' {
					draw.Draw(img, image.Rect(x+i*8+col*2, y+row*2, x+i*8+col*2+2, y+row*2+2), image.NewUniform(c), image.Point{}, draw.Src)
				}
			}
		}
	}
}

// The instrument's oracle and verdict without a corpus: a label still matched, one the grouping no longer makes, a
// drawing seen and never accepted, and each verdict.
func TestTheDrawingGroupOracleMatchesExactMembers(t *testing.T) {
	a := drawnPath{box: [4]float64{0, 0, 10, 10}, curved: true, ops: 2, span: opSpan{0, 1}, mcid: -1}
	b, c := a, a
	b.box, b.span = [4]float64{5, 5, 15, 15}, opSpan{20, 21}
	c.box, c.span = [4]float64{100, 100, 110, 110}, opSpan{40, 41}
	paths := []drawnPath{a, b, c}
	groups, _ := groupDrawnPaths(paths)
	dp := drawingPage{paths: paths, groups: groups}
	var tl drawingTally
	tl.score(dp, drawingLabel{page: 1, seen: 3, groups: []string{"0:0,0:20", "0:40", "0:0"}})
	if tl.seen != 3 || tl.matched != 2 || tl.seenMulti != 1 || tl.matchedMulti != 1 {
		t.Errorf("scored seen=%d matched=%d multi %d/%d, want 3 seen, 2 matched (the pair and the lone curve; the lone first curve is not a group), multi 1/1",
			tl.seen, tl.matched, tl.matchedMulti, tl.seenMulti)
	}
	tl = drawingTally{groups: drawingGroupFloor, labelPages: 1, seen: 10, matched: 9}
	if ok, msg := tl.verdict(); !ok {
		t.Errorf("9 of 10 at a 0.9 threshold: %q", msg)
	}
	tl.matched = 8
	if ok, msg := tl.verdict(); ok || !strings.Contains(msg, "BELOW") {
		t.Errorf("8 of 10: %v %q", ok, msg)
	}
	if ok, msg := (&drawingTally{groups: drawingGroupFloor - 1, labelPages: 1}).verdict(); ok || !strings.Contains(msg, "TOO-FEW") {
		t.Errorf("below the floor: %v %q", ok, msg)
	}
	if ok, msg := (&drawingTally{groups: drawingGroupFloor}).verdict(); ok || !strings.Contains(msg, "NONE") {
		t.Errorf("no labelled page: %v %q", ok, msg)
	}
	if _, err := readDrawingLabels(writeTemp(t, "abc\t1\t1\tx;y\n")); err == nil {
		t.Error("a line accepting two groups of one drawing seen was read")
	}
}

func writeTemp(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "labels.tsv")
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
