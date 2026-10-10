package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/scaling"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// /pending 818: one leaf's `/Count -1` or `/Kids []` took `Pages` off its one walk and onto `PageDict` page by page,
// quadratic on a flat tree (20,000 pages: 1m40-2m per call). The tolerant walk (pagesim.go) replays pdfcpu's own
// numbering with the work shared between pages. These hold it to pdfcpu's answer on every page of every odd tree —
// the order and the set, duplicates included, because other code indexes by pdfcpu's page number — and to linear work.

// sameAsPageDict fails unless got is, page by page, exactly what PageDict(p, false) answers.
func sameAsPageDict(t *testing.T, name string, ctx *model.Context, got []pdfread.Page) {
	t.Helper()
	if len(got) != max(ctx.PageCount, 0) {
		t.Fatalf("%s: %d pages answered for a %d-page document", name, len(got), ctx.PageCount)
	}
	for i, g := range got {
		p := i + 1
		d, ref, attrs, err := ctx.PageDict(p, false)
		switch {
		case g.Nr != p:
			t.Fatalf("%s: page %d reported as number %d", name, p, g.Nr)
		case (err == nil) != (g.Err == nil):
			t.Fatalf("%s page %d: PageDict err %v, tolerant walk err %v", name, p, err, g.Err)
		case err != nil && err.Error() != g.Err.Error():
			t.Fatalf("%s page %d: PageDict err %q, tolerant walk err %q", name, p, err, g.Err)
		case reflect.ValueOf(d).Pointer() != reflect.ValueOf(g.Dict).Pointer():
			t.Fatalf("%s page %d: a different dictionary from PageDict's (ref %v, PageDict's %v)", name, p, g.Ref, ref)
		case !reflect.DeepEqual(ref, g.Ref):
			t.Fatalf("%s page %d: reference %v, PageDict's %v", name, p, g.Ref, ref)
		case !reflect.DeepEqual(attrs, g.Attrs):
			t.Fatalf("%s page %d: inherited attributes %+v, PageDict's %+v", name, p, g.Attrs, attrs)
		}
	}
}

// readUnvalidated reads a document the way a relaxed, non-validating read does, so a tree validation would refuse
// can still be compared — the replay must agree with pdfcpu on any tree, not only the ones validation admits.
func readUnvalidated(t *testing.T, b []byte, mode int) *model.Context {
	t.Helper()
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = mode
	ctx, err := api.ReadContext(bytes.NewReader(b), conf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		t.Fatalf("page count: %v", err)
	}
	return ctx
}

// flatWith is n pages under one /Pages node, the pages at the given positions carrying extra.
func flatWith(n int, extra string, at ...int) []byte {
	odd := map[int]bool{}
	for _, i := range at {
		odd[i] = true
	}
	objs := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"}
	var kids strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&kids, "%d 0 R ", 3+i)
		e := fmt.Sprintf("/Rotate %d", 90*(i%4))
		if odd[i] {
			e += " " + extra
		}
		objs[3+i] = page(2, e)
	}
	objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>", kids.String(), n)
	return testpdf.Assemble(objs)
}

// The reviewer's shapes, and their neighbours: every one is answered as pdfcpu answers it, page by page.
func TestTheTolerantWalkAnswersWhatPageDictAnswersOnOddLeaves(t *testing.T) {
	// A leaf's non-negative count changes nothing pdfcpu decides, so the one walk keeps it.
	oneWalk := map[string]bool{"a leaf counting 7": true}
	cases := map[string][]byte{
		"a leaf counting -1":              flatWith(40, "/Count -1", 17),
		"a leaf counting -3":              flatWith(40, "/Count -3", 17),
		"the last leaf counting -1":       flatWith(40, "/Count -1", 39),
		"a leaf with empty kids":          flatWith(40, "/Kids [] /MediaBox [0 0 1 1]", 17),
		"two leaves, -1 and empty kids":   flatWith(40, "/Count -1", 5),
		"every leaf counting -1":          flatWith(40, "/Count -1", seq(40)...),
		"every leaf with empty kids":      flatWith(40, "/Kids [] /Rotate 270", seq(40)...),
		"a leaf whose kids name a leaf":   flatWith(40, "/Kids [5 0 R]", 17),
		"a leaf counting 7":               flatWith(40, "/Count 7", 3),
		"a leaf whose kids name the root": flatWith(40, "/Kids [2 0 R]", 30),
	}
	for name, b := range cases {
		for _, mode := range []int{model.ValidationRelaxed, model.ValidationStrict} {
			ctx := readUnvalidated(t, b, mode)
			got, _ := pdfread.SimulatePages(ctx)
			sameAsPageDict(t, name, ctx, got)
			if v, err := pdfread.Validated(b, model.NewDefaultConfiguration()); err == nil {
				pages, walked := pdfread.PagesWalked(v)
				if walked != oneWalk[name] {
					t.Errorf("%s: took the one walk = %v, want %v", name, walked, oneWalk[name])
				}
				sameAsPageDict(t, name+" (validated)", v, pages)
			}
		}
	}
	// pdfcpu's numbering, written out once: a leaf counting -1 at position k is never page k; the leaf after it
	// answers both k and k+1. The replay must reproduce that, not "repair" it.
	ctx := readUnvalidated(t, flatWith(6, "/Count -1", 2), model.ValidationRelaxed)
	got, _ := pdfread.SimulatePages(ctx)
	var objs []int
	for _, g := range got {
		objs = append(objs, g.Ref.ObjectNumber.Value())
	}
	if want := []int{3, 4, 6, 6, 7, 8}; !reflect.DeepEqual(objs, want) {
		t.Fatalf("pages answered by objects %v, want pdfcpu's %v", objs, want)
	}
}

func seq(n int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i
	}
	return s
}

// Random trees with every oddity pdfcpu's walk decides on — counts absent, wrong, negative; leaves with kids, empty
// kids or counts; duplicate and cyclic kids; attributes at every level — each compared page by page with PageDict.
func TestTheTolerantWalkAnswersWhatPageDictAnswersOnRandomTrees(t *testing.T) {
	r := rand.New(rand.NewSource(818))
	var answered, errored, empty, duplicated, oddTrees int
	for trial := 0; trial < 400; trial++ {
		b := randomTree(r)
		for _, mode := range []int{model.ValidationRelaxed, model.ValidationStrict} {
			ctx := readUnvalidated(t, b, mode)
			if !pdfread.WalkedInOnePass(ctx) {
				oddTrees++
			}
			got, _ := pdfread.SimulatePages(ctx)
			sameAsPageDict(t, fmt.Sprintf("trial %d mode %d", trial, mode), ctx, got)
			seen := map[int]bool{}
			for _, g := range got {
				switch {
				case g.Err != nil:
					errored++
				case g.Dict == nil:
					empty++
				default:
					answered++
					if seen[g.Ref.ObjectNumber.Value()] {
						duplicated++
					}
					seen[g.Ref.ObjectNumber.Value()] = true
				}
			}
		}
	}
	// The population, so a generator that drifted into producing only refusals cannot pass vacuously.
	t.Logf("%d odd trees of 800; pages: %d answered (%d a repeat of an earlier page), %d errors, %d empty",
		oddTrees, answered, duplicated, errored, empty)
	if oddTrees < 200 || answered < 500 || duplicated == 0 || errored == 0 || empty == 0 {
		t.Fatalf("the random trees no longer reach every kind of answer")
	}
}

func randomTree(r *rand.Rand) []byte {
	objs := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"}
	next := 3
	var nodes []int
	pick := func(opts ...string) string { return opts[r.Intn(len(opts))] }
	attrs := func() string {
		return pick("", "", "/MediaBox [0 0 10 20]", "/Rotate 90", "/CropBox [1 1 5 5] /Rotate 180", "/Resources << >>")
	}
	var build func(nr, depth int) int // returns the true page count beneath
	build = func(nr, depth int) int {
		nodes = append(nodes, nr)
		nk := r.Intn(5)
		var kids []string
		held := 0
		for i := 0; i < nk; i++ {
			kid := next
			next++
			if depth < 3 && r.Intn(3) == 0 {
				held += build(kid, depth+1)
			} else {
				held++
				extra := attrs() + " " + pick("", "", "", "/Count -1", "/Count -2", "/Count 0", "/Count 3", "/Kids []")
				if r.Intn(10) == 0 && len(nodes) > 0 {
					extra = fmt.Sprintf("/Kids [%d 0 R]", nodes[r.Intn(len(nodes))])
				}
				objs[kid] = page(nr, extra)
			}
			kids = append(kids, fmt.Sprintf("%d 0 R", kid))
			if r.Intn(12) == 0 && len(nodes) > 1 { // a duplicate or cyclic /Pages kid
				kids = append(kids, fmt.Sprintf("%d 0 R", nodes[r.Intn(len(nodes))]))
			}
		}
		count := pick(fmt.Sprintf("/Count %d", held), fmt.Sprintf("/Count %d", held), "", fmt.Sprintf("/Count %d", held+r.Intn(5)-2))
		typ := pick("/Type /Pages", "/Type /Pages", "/Type /Pages", "")
		if nr == 2 {
			typ, count = "/Type /Pages", fmt.Sprintf("/Count %d", held+r.Intn(3)-1)
		}
		objs[nr] = fmt.Sprintf("<< %s /Kids [%s] %s %s >>", typ, strings.Join(kids, " "), count, attrs())
		return held
	}
	build(2, 0)
	return testpdf.Assemble(objs)
}

// The reviewer's cost: a flat tree with one odd leaf is walked in work linear in its pages. Counted in steps, so a
// loaded machine cannot flake it; replaying from the root per page — the old fallback's shape — costs about n²/2
// steps. The steps are the simulation's, so the door itself (`PagesWalked`) is timed as well — by SCALING, not
// against a ceiling (/pending 841; it was 20 s, which measures the machine): four times the pages cost the tolerant
// walk ×4 and the old page-by-page fallback ×16 (1m40-2m at 20,000 pages); ×8 is the midpoint on a log scale.
func TestATolerantWalkIsLinear(t *testing.T) {
	for _, extra := range []string{"/Count -1", "/Kids []"} {
		ctxs := map[int]*model.Context{}
		for _, n := range []int{5000, 20000} {
			ctx, err := pdfread.Validated(flatWith(n, extra, n/2), model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("%s n=%d: validation refused the reviewer's shape (%v) — the test no longer reaches it", extra, n, err)
			}
			ctxs[n] = ctx
			pages, walked := pdfread.PagesWalked(ctx)
			if walked {
				t.Fatalf("%s n=%d: the odd leaf took the one walk — this test no longer reaches the tolerant one", extra, n)
			}
			got, steps := pdfread.SimulatePages(ctx)
			for i := range got {
				if !reflect.DeepEqual(pages[i].Ref, got[i].Ref) { // nil for a `/Kids []` leaf's own number: pdfcpu answers nothing
					t.Fatalf("%s n=%d page %d: Pages and the tolerant walk disagree", extra, n, i+1)
				}
			}
			if len(got) != n {
				t.Fatalf("%s n=%d: %d pages", extra, n, len(got))
			}
			for _, g := range got {
				if g.Err != nil {
					t.Fatalf("%s n=%d page %d: %v", extra, n, g.Nr, g.Err)
				}
			}
			t.Logf("%s, %d pages: %d steps (%.2f per page)", extra, n, steps, float64(steps)/float64(n))
			if steps > 4*n {
				t.Fatalf("%s, %d pages: %d steps, more than 4 per page — the walk is not sharing work between pages",
					extra, n, steps)
			}
		}
		scaling.GrowsLinearly(t, extra+": Pages through the tolerant walk", 5000, 20000, 8, func(n int) time.Duration {
			return scaling.TimeOnce(func() { pdfread.PagesWalked(ctxs[n]) })
		})
	}
}

// A tree hostile to the sharing itself — every leaf counting -1, so pdfcpu's own walk for each page runs to the end
// of the tree — is bounded by the budget and refused past it, never paid in full.
func TestATreeHostileToTheTolerantWalkIsRefusedWithinItsBudget(t *testing.T) {
	const n = 8000
	ctx := readUnvalidated(t, flatWith(n, "/Count -1", seq(n)...), model.ValidationRelaxed)
	got, steps := pdfread.SimulatePages(ctx)
	budget := pdfread.SimBudget(ctx.XRefTable)
	if steps > budget+n+1 {
		t.Fatalf("%d steps against a budget of %d", steps, budget)
	}
	refused := 0
	for _, g := range got {
		if errors.Is(g.Err, pdfread.ErrPageTreeTooIrregular) {
			refused++
		} else if g.Err != nil || g.Dict != nil {
			t.Fatalf("page %d: %v / %v — pdfcpu answers every page of this tree with nothing", g.Nr, g.Err, g.Dict)
		}
	}
	t.Logf("%d steps, budget %d; %d of %d pages refused", steps, budget, refused, n)
	if refused == 0 {
		t.Fatalf("no page refused: the budget did not bind on a tree whose full replay is ~n²/2 = %d steps", n*n/2)
	}
}
