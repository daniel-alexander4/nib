package pdfops

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"nib/internal/contentstream"
	"nib/internal/pdfread"
	"nib/internal/uacheck"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The object-delete instrument (PLAN-image-edit P01.S01 T07, D3's falsification): for each picture of each page of a
// local list of documents, remove its painting operator in memory, write, re-read, and say whether exactly that
// picture went and nothing else moved. Not a tier and not a gate, like build/formshape.sh: the list is local and
// never committed, so `build/objectdelete.sh` prints COUNTS only, never a path. `TestObjectDeleteOverTheHostileFixtures`
// runs this same code over committed fixtures with a floor, so a fresh clone still exercises it.

const (
	// deletedClean: one picture fewer, the survivors' digests and boxes the original's minus exactly the addressed
	// one, in order, and the text-run count unchanged.
	deletedClean = "clean"
	// The ways a deletion reads differently afterwards — each a finding that changes D3 before S02 builds on it.
	deletedCountDiffers  = "count-differs"
	deletedDigestsDiffer = "digests-differ"
	deletedBoxesDiffer   = "boxes-differ"
	deletedTextDiffers   = "text-differs"
	deletedNotWritten    = "write-failed"
	deletedUnreadable    = "unreadable-after"
	// objectDeleteFloor is the fewest pictures a corpus run must read to say anything (the population pin).
	objectDeleteFloor = 1000
)

// pageReading is one page's pictures, its text-run count and a key over every run's text, font and origin, from one
// read: "text runs equal" is the runs themselves, not only how many there are (code-review 2026-11-09, P01.S01 #3).
type pageReading struct {
	objs PageObjects
	runs int
	text string
}

func runsKey(runs []textRun) string {
	var b strings.Builder
	for _, r := range runs {
		fmt.Fprintf(&b, "%s\x00%s\x00%.2f\x00%.2f\x00%.2f\x01", r.text, r.font, r.x, r.y, r.size)
	}
	return b.String()
}

func readPageForProbe(pdf []byte, page int) (pageReading, error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return pageReading{}, err
	}
	walked := pdfread.Pages(ctx)
	if err := pageInDocument(page, len(walked)); err != nil {
		return pageReading{}, err
	}
	pg := pageAt(ctx, walked, page)
	pr, err := readPageDrawings(ctx, pg)
	if err != nil {
		return pageReading{}, err
	}
	return pageReading{objs: pageObjectsOf(page, newDisplaySpace(pg), pr), runs: len(pr.runs), text: runsKey(pr.runs)}, nil
}

// scoreDeletion is the oracle: after is what the page reads once the picture at addressed was removed from before.
func scoreDeletion(before, after pageReading, addressed int) (string, string) {
	b, a := before.objs.Objects, after.objs.Objects
	if len(a) != len(b)-1 {
		return deletedCountDiffers, fmt.Sprintf("%d pictures before, %d after", len(b), len(a))
	}
	for i, o := range a {
		j := i
		if i >= addressed {
			j = i + 1
		}
		if o.Digest != b[j].Digest {
			return deletedDigestsDiffer, fmt.Sprintf("survivor %d is not original %d", i, j)
		}
		if o.Box != b[j].Box || o.OnPage != b[j].OnPage {
			return deletedBoxesDiffer, fmt.Sprintf("survivor %d moved: %v → %v", i, b[j].Box, o.Box)
		}
	}
	if after.runs != before.runs {
		return deletedTextDiffers, fmt.Sprintf("%d text runs before, %d after", before.runs, after.runs)
	}
	if after.text != before.text {
		return deletedTextDiffers, "a text run's text, font or origin changed"
	}
	return deletedClean, ""
}

type deletion struct {
	outcome, why string
	out          []byte
}

// removeObject writes pdf with the picture at (page, ordinal, digest) removed: the page is re-read inside the write and
// the address must name the same operator there (D2), and only its span is cut — `q`, `cm` and `Q` stay (D3).
func removeObject(pdf []byte, page, ordinal int, digest string) ([]byte, error) {
	return writeMutated(pdf, func(ctx *model.Context) error {
		pg := pageAt(ctx, pdfread.Pages(ctx), page)
		pr, err := readPageDrawings(ctx, pg)
		if err != nil {
			return err
		}
		o, ok := ObjectAt(pageObjectsOf(page, newDisplaySpace(pg), pr), ordinal, digest)
		if !ok {
			return fmt.Errorf("object-changed: page %d has no picture %d with that digest", page, ordinal)
		}
		src, err := pdfread.PageContent(ctx, pg.Dict, page)
		if err != nil {
			return err
		}
		edited, err := contentstream.NewEdit(src).Replace(o.span.start, o.span.end, nil).Apply()
		if err != nil {
			return err
		}
		return setPageContent(ctx, pg.Dict, edited)
	})
}

// probeDeletion removes the picture objs lists at ordinal and scores the page read back.
func probeDeletion(pdf []byte, page int, objs PageObjects, ordinal int) deletion {
	before, err := readPageForProbe(pdf, page)
	if err != nil {
		return deletion{outcome: deletedUnreadable, why: err.Error()}
	}
	out, err := removeObject(pdf, page, ordinal, objs.Objects[ordinal].Digest)
	if err != nil {
		return deletion{outcome: deletedNotWritten, why: err.Error()}
	}
	after, err := readPageForProbe(out, page)
	if err != nil {
		return deletion{outcome: deletedUnreadable, why: err.Error(), out: out}
	}
	outcome, why := scoreDeletion(before, after, ordinal)
	return deletion{outcome: outcome, why: why, out: out}
}

// imageStreams hashes the raw bytes of every image XObject in pdf, by object number.
func imageStreams(pdf []byte) (map[int][32]byte, error) {
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	out := map[int][32]byte{}
	for nr, e := range ctx.XRefTable.Table {
		if e == nil || e.Free {
			continue
		}
		sd, ok := e.Object.(types.StreamDict)
		if !ok {
			continue
		}
		if st := sd.Dict.NameEntry("Subtype"); st != nil && *st == "Image" {
			out[nr] = sha256.Sum256(sd.Raw)
		}
	}
	return out, nil
}

// extractableObjects is the object numbers of the images pdfcpu's extraction finds on page.
func extractableObjects(pdf []byte, page int) (map[int]bool, error) {
	out := map[int]bool{}
	err := pdfread.ExtractImages(pdf, []string{fmt.Sprint(page)}, func(img model.Image, _ bool, _ int) error {
		out[img.ObjNr] = true
		return nil
	})
	return out, err
}

// checkerDefects is the set of clauses the checker fails, so a deletion that clears one clause and breaks another
// reads as changed and not as the same count (code-review 2026-11-09, P01.S01 #6).
func checkerDefects(pdf []byte) (map[string]bool, bool) {
	rep, err := uacheck.Check(pdf)
	if err != nil {
		return nil, false
	}
	failing := map[string]bool{}
	for _, r := range rep.Results {
		if r.Verdict == uacheck.Fail {
			failing[r.Clause] = true
		}
	}
	return failing, true
}

// defectChange says how the failing set moved: a clause gained (more), none gained but one lost (fewer), or the same.
func defectChange(before, after map[string]bool) string {
	for c := range after {
		if !before[c] {
			return "more"
		}
	}
	if len(after) < len(before) {
		return "fewer"
	}
	return "same"
}

// deleteTally counts what the instrument saw. Counts only: nothing here holds a path, a name or a word.
type deleteTally struct {
	docs, unreadable, notRewritable, panicked int
	pictures, attempted, clean                int
	notEditable                               map[string]int
	differs                                   map[string]int
	formsNotRead                              int
	// residue: of the deleted XObject pictures, how many are still in the file (same raw bytes) and how many pdfcpu's
	// extraction still finds on the page.
	inFile, notInFile, extractable, notExtractable, residueUnread int
	// checker: one deletion per document is checked before and after.
	checked, uncheckable, moreDefects, fewerDefects, sameDefects int
	byteChange                                                   int64
}

func (t *deleteTally) note(m *map[string]int, k string) {
	if *m == nil {
		*m = map[string]int{}
	}
	(*m)[k]++
}

// document probes up to maxPages pages of pdf and up to perPage editable pictures on each.
func (t *deleteTally) document(pdf []byte, maxPages, perPage int) {
	t.docs++
	defer func() {
		if r := recover(); r != nil {
			t.panicked++
		}
	}()
	ctx, err := inspectionRead(pdf)
	if err != nil {
		t.unreadable++
		return
	}
	pages := len(pdfread.Pages(ctx))
	if _, err := writeMutated(pdf, func(*model.Context) error { return nil }); err != nil {
		t.notRewritable++ // "not editable (rewrite fails)": the no-op control
		return
	}
	checkedThisDoc := false
	for page := 1; page <= pages && page <= maxPages; page++ {
		objs, err := ReadPageObjects(pdf, page)
		if err != nil {
			t.unreadable++
			return
		}
		t.formsNotRead += objs.FormsNotRead
		tried := 0
		for _, o := range objs.Objects {
			t.pictures++
			if !o.Editable {
				t.note(&t.notEditable, o.Why)
				continue
			}
			if tried >= perPage {
				continue
			}
			tried++
			t.attempted++
			d := probeDeletion(pdf, page, objs, o.Ordinal)
			if d.outcome != deletedClean {
				t.note(&t.differs, d.outcome)
				continue
			}
			t.clean++
			t.byteChange += int64(len(d.out) - len(pdf))
			if o.objNr != 0 {
				t.residue(pdf, d.out, page, o.objNr)
			}
			if !checkedThisDoc {
				checkedThisDoc = true
				before, okB := checkerDefects(pdf)
				after, okA := checkerDefects(d.out)
				switch {
				case !okB || !okA:
					t.uncheckable++
				case defectChange(before, after) == "more":
					t.checked++
					t.moreDefects++
				case defectChange(before, after) == "fewer":
					t.checked++
					t.fewerDefects++
				default:
					t.checked++
					t.sameDefects++
				}
			}
		}
	}
}

func (t *deleteTally) residue(pdf, out []byte, page, objNr int) {
	before, err1 := imageStreams(pdf)
	after, err2 := imageStreams(out)
	ex, err3 := extractableObjects(out, page)
	if err1 != nil || err2 != nil || err3 != nil {
		t.residueUnread++
		return
	}
	want, ok := before[objNr]
	found := false
	for _, h := range after {
		if ok && h == want {
			found = true
			break
		}
	}
	if found {
		t.inFile++
	} else {
		t.notInFile++
	}
	if ex[objNr] {
		t.extractable++
	} else {
		t.notExtractable++
	}
}

func counts(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var b strings.Builder
	for i, k := range ks {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%s=%d", k, m[k])
	}
	return b.String()
}

func (t *deleteTally) summary() string {
	return fmt.Sprintf("docs=%d unreadable=%d not-rewritable=%d panicked=%d pictures=%d forms-not-read=%d "+
		"not-editable[%s] attempted=%d clean=%d differs[%s] residue: in-file=%d gone=%d extractable=%d not-extractable=%d unread=%d "+
		"checker: checked=%d uncheckable=%d more=%d fewer=%d same=%d bytes-change=%d",
		t.docs, t.unreadable, t.notRewritable, t.panicked, t.pictures, t.formsNotRead, counts(t.notEditable), t.attempted,
		t.clean, counts(t.differs), t.inFile, t.notInFile, t.extractable, t.notExtractable, t.residueUnread,
		t.checked, t.uncheckable, t.moreDefects, t.fewerDefects, t.sameDefects, t.byteChange)
}

// verdict is the run's three-way answer: too few pictures to say anything, a finding, or clean.
func (t *deleteTally) verdict() (ok bool, msg string) {
	switch {
	case t.pictures < objectDeleteFloor:
		return false, fmt.Sprintf("OBJECTDELETE TOO-FEW: %d pictures read, fewer than the %d a run needs to say anything", t.pictures, objectDeleteFloor)
	case t.panicked > 0:
		return false, fmt.Sprintf("OBJECTDELETE FINDING: %d document(s) panicked the probe", t.panicked)
	case t.clean != t.attempted:
		return false, fmt.Sprintf("OBJECTDELETE FINDING: %d of %d deletions read differently afterwards (%s)", t.attempted-t.clean, t.attempted, counts(t.differs))
	}
	return true, ""
}

func TestObjectDeleteCorpus(t *testing.T) {
	list := os.Getenv("NIB_OBJECTDELETE_LIST")
	if list == "" {
		t.Skip("NIB_OBJECTDELETE_LIST not set: this is an instrument, run build/objectdelete.sh")
	}
	raw, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	var tally deleteTally
	missing := 0
	for _, line := range strings.Split(string(raw), "\n") {
		p := strings.TrimSpace(line)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			missing++
			continue
		}
		tally.document(data, 4, 4)
	}
	fmt.Printf("OBJECTDELETE missing=%d %s\n", missing, tally.summary())
	if ok, msg := tally.verdict(); !ok {
		t.Fatal(msg)
	}
}

// The oracle and the verdict without a corpus: a seeded wrong-ordinal edit is red, and so is every other way a page
// can read differently.
func TestTheObjectDeleteOracleSeesWhichPictureWent(t *testing.T) {
	obj := func(d string, x float64) PageObject {
		return PageObject{Digest: d, Box: MapRect{x, 0, x + 0.1, 0.1}, OnPage: true}
	}
	before := pageReading{objs: PageObjects{Objects: []PageObject{obj("a", 0), obj("b", 0.2), obj("c", 0.4)}}, runs: 3}
	after := func(runs int, os ...PageObject) pageReading {
		return pageReading{objs: PageObjects{Objects: os}, runs: runs}
	}
	for _, c := range []struct {
		name      string
		after     pageReading
		addressed int
		want      string
	}{
		{"the addressed one went", after(3, obj("a", 0), obj("c", 0.4)), 1, deletedClean},
		{"a different one went (seeded wrong ordinal)", after(3, obj("a", 0), obj("c", 0.4)), 0, deletedDigestsDiffer},
		{"nothing went", after(3, obj("a", 0), obj("b", 0.2), obj("c", 0.4)), 1, deletedCountDiffers},
		{"two went", after(3, obj("a", 0)), 1, deletedCountDiffers},
		{"a survivor moved", after(3, obj("a", 0), obj("c", 0.5)), 1, deletedBoxesDiffer},
		{"text was lost", after(2, obj("a", 0), obj("c", 0.4)), 1, deletedTextDiffers},
		{"a run changed, the count did not", pageReading{objs: PageObjects{Objects: []PageObject{obj("a", 0), obj("c", 0.4)}}, runs: 3, text: "moved"}, 1, deletedTextDiffers},
	} {
		if got, _ := scoreDeletion(before, c.after, c.addressed); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	var few deleteTally
	few.pictures, few.attempted, few.clean = objectDeleteFloor-1, 5, 5
	if ok, msg := few.verdict(); ok || !strings.Contains(msg, "TOO-FEW") {
		t.Errorf("below the floor: %v %q", ok, msg)
	}
	found := deleteTally{pictures: objectDeleteFloor, attempted: 5, clean: 4}
	if ok, msg := found.verdict(); ok || !strings.Contains(msg, "FINDING") {
		t.Errorf("a finding: %v %q", ok, msg)
	}
	if ok, msg := (&deleteTally{pictures: objectDeleteFloor, attempted: 5, clean: 5, panicked: 1}).verdict(); ok || !strings.Contains(msg, "panicked") {
		t.Errorf("a panic is not a finding: %v %q", ok, msg)
	}
	if got := defectChange(map[string]bool{"7.1 t3": true}, map[string]bool{"7.2 t1": true}); got != "more" {
		t.Errorf("one clause cleared and another broken reads %q, want more", got)
	}
	if got := defectChange(map[string]bool{"7.1 t3": true, "7.2 t1": true}, map[string]bool{"7.2 t1": true}); got != "fewer" {
		t.Errorf("a clause cleared reads %q, want fewer", got)
	}
	if ok, _ := (&deleteTally{pictures: objectDeleteFloor, attempted: 5, clean: 5}).verdict(); !ok {
		t.Error("a clean run at the floor is not clean")
	}
}
