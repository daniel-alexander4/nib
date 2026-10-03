package pdfops

import (
	"bytes"
	"fmt"
	"nib/internal/pdfread"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 706`: pdfcpu's optimize pass has no budget, and every page operation reads through it. These
// fixtures are the three hostile shapes measured against it (see `internal/pdfread/optimize.go`); each must now finish
// promptly, keep every page's content, and still optimize an ordinary document.

// finishesWithin fails when f has not returned after s seconds — from a deadline, not on return, because
// the unbounded pass ran for minutes and a check made after it would wait with it.
func finishesWithin(t *testing.T, s int, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(time.Duration(s) * time.Second):
		t.Fatalf("%s had not finished after %ds", what, s)
	}
}

func optStream(body, extra string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", extra, len(body), body)
}

// twoPages is a page drawing nothing of its own plus an empty second page, so `RemovePages("2")` is a page
// operation that leaves page 1's resources to be read and written.
func twoPages(page1Res, content string, objs map[int]string) []byte {
	objs[1] = "<< /Type /Catalog /Pages 2 0 R >>"
	objs[2] = "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources " + page1Res + " /Contents 4 0 R >>"
	objs[4] = optStream(content, "")
	objs[5] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>"
	return assembleFixture(objs)
}

// formChain is depth forms of one /Length, each drawing the next under its own /Resources — the comparison
// shape: every pair is compared, each comparison descending the chain.
func formChain(depth int) []byte {
	objs := map[int]string{}
	for i := 0; i < depth; i++ {
		objs[100+i] = optStream("/X Do", fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 10 10] "+
			"/Resources << /XObject << /X %d 0 R >> >>", 101+i))
	}
	objs[100+depth] = optStream("0 0 m", "/Type /XObject /Subtype /Form /BBox [0 0 10 10]")
	return twoPages("<< /XObject << /X 100 0 R >> >>", "/X Do", objs)
}

// formDiamonds is depth levels of two forms sharing one indirect /Resources that names the next level's
// two, and draws them — the walk shape: pdfcpu walks it once per path, 2^depth.
func formDiamonds(depth int) []byte {
	objs := map[int]string{}
	for i := 0; i < depth; i++ {
		objs[1000+i] = fmt.Sprintf("<< /XObject << /X %d 0 R /Y %d 0 R >> >>", 2000+2*i, 2001+2*i)
		draw := "/X Do /Y Do "
		if i == depth-1 {
			draw = ""
		}
		for j, tag := range []string{"a", "b"} {
			objs[2000+2*i+j] = optStream(fmt.Sprintf("%s%% %s%04d", draw, tag, i),
				fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources %d 0 R", 1001+i))
		}
	}
	objs[1000+depth] = "<< >>"
	return twoPages("1000 0 R", "/X Do /Y Do", objs)
}

// formsNamingTheirDictionary is n forms whose direct /Resources all name the one /XObject dictionary that
// holds them, the page drawing each — pdfcpu's visited list cuts only on indirect entries, so it walks the
// orderings.
func formsNamingTheirDictionary(n int) []byte {
	objs := map[int]string{}
	var names, draws strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&names, "/F%d %d 0 R ", i, 100+i)
		fmt.Fprintf(&draws, "/F%d Do ", i)
		objs[100+i] = optStream(fmt.Sprintf("%% f%06d", i),
			"/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources << /XObject 6 0 R >>")
	}
	objs[6] = "<< " + names.String() + ">>"
	return twoPages("<< /XObject 6 0 R >>", draws.String(), objs)
}

// formObjects counts the form XObjects a written document holds.
func formObjects(t *testing.T, pdf []byte) int {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ctx.Table {
		if e == nil || e.Free {
			continue
		}
		if sd, ok := e.Object.(types.StreamDict); ok {
			if st := sd.Dict.NameEntry("Subtype"); st != nil && *st == "Form" {
				n++
			}
		}
	}
	return n
}

func TestAPageOperationOnHostileFormsFinishesAndKeepsEveryForm(t *testing.T) {
	cases := []struct {
		name   string
		pdf    []byte
		forms  int
		within int // seconds
	}{
		// Measured unbounded: 90 s. Bounded: ~16 ms.
		{"a chain of 400 forms of one length", formChain(400), 401, 20},
		// Measured unbounded: doubling per level, 318 ms at 18. Bounded: ~80 ms.
		{"thirty levels of forms sharing their resources", formDiamonds(30), 60, 20},
		// Measured unbounded: twenty took 2.0 s, forty did not finish in four minutes. Bounded: ~16 ms.
		{"forty forms naming their own dictionary", formsNamingTheirDictionary(40), 40, 20},
		// The estimate's own cost, so the tightest deadline: charging only the entries entered let it run past
		// a minute here, and charging each examined entry without the visited list it scans ~10 s. Bounded:
		// ~1 s.
		{"two thousand forms naming their own dictionary", formsNamingTheirDictionary(2000), 2000, 8},
	}
	for _, c := range cases {
		// Stimulus first: the fixture really carries the forms it is named for.
		if got := formObjects(t, c.pdf); got != c.forms {
			t.Fatalf("setup: %s holds %d form XObjects, want %d", c.name, got, c.forms)
		}
		var out []byte
		var err error
		finishesWithin(t, c.within, "RemovePages over "+c.name, func() { out, err = RemovePages(c.pdf, []string{"2"}) })
		if err != nil {
			t.Fatalf("RemovePages over %s: %v", c.name, err)
		}
		// Degraded, not lossy: the skipped pass only deduplicates, so every form is still written.
		if got := formObjects(t, out); got != c.forms {
			t.Errorf("RemovePages over %s wrote %d form XObjects, want all %d", c.name, got, c.forms)
		}
	}
}

func TestOptimizeRefusesADocumentItsPassCannotAfford(t *testing.T) {
	var err error
	finishesWithin(t, 20, "Optimize over a chain of 400 forms", func() { _, err = Optimize(formChain(400)) })
	if err == nil || !strings.Contains(err.Error(), "will not optimize") {
		t.Fatalf("Optimize over a chain of 400 forms returned %v, want a refusal — a skipped pass there "+
			"reports success for a file it never optimised", err)
	}
}

func TestAnOrdinaryDocumentIsStillOptimized(t *testing.T) {
	// Two identical forms on one page: the pass merges them, and a skipped pass would not.
	pdf := twoPages("<< /XObject << /X 100 0 R /Y 101 0 R >> >>", "/X Do /Y Do", map[int]string{
		100: optStream("0 0 1 1 re f", "/Type /XObject /Subtype /Form /BBox [0 0 10 10]"),
		101: optStream("0 0 1 1 re f", "/Type /XObject /Subtype /Form /BBox [0 0 10 10]"),
	})
	if got := formObjects(t, pdf); got != 2 {
		t.Fatalf("setup: the fixture holds %d form XObjects, want 2", got)
	}
	out, err := RemovePages(pdf, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if got := formObjects(t, out); got != 1 {
		t.Errorf("RemovePages over two identical forms wrote %d form XObjects, want 1 — the optimize pass "+
			"did not run on an ordinary document", got)
	}
}

// The routing guard that lived here — nothing reaches pdfcpu's pass around the budget — is module-wide now,
// with the budget itself in `internal/pdfread` (`/pending 714`): `TestEveryValidatingReadRoutesThroughTheDoor`.

// TestEveryOperationOverAnApiReadFinishesOnHostileForms — `/pending 716`. Each of these operations handed a pdfcpu
// `api` function a reader, and the function ran its own read-validate-OPTIMIZE with no budget: measured on the
// 400-form chain (68 KB) before the restatement, every one but NUp and NormalizePageSizes (whose `api` reads run no
// pass) was still running at 30 s. Each now reads through `pdfread` (`apiread.go`), so the pass is skipped past the
// budget — or, for image extraction, refused.
func TestEveryOperationOverAnApiReadFinishesOnHostileForms(t *testing.T) {
	pdf := formChain(400)
	conf := model.NewAESConfiguration("pw", "pw", 256)
	conf.Optimize = false // the fixture's own encryption must not run the unbudgeted pass this test is about
	var enc bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &enc, conf); err != nil {
		t.Fatalf("setup: encrypting the fixture: %v", err)
	}
	png := onePixelPNG()
	ops := []struct {
		name string
		run  func() error
	}{
		{"Outline", func() error { _, err := Outline(pdf); return err }},
		{"SplitByBookmarks", func() error { _, err := SplitByBookmarks(pdf, "x"); return err }},
		{"StampTextLayer", func() error {
			_, err := StampTextLayer(pdf, []Word{{Page: 1, Rect: [4]float64{10, 10, 50, 20}, Text: "hello"}}, "eng")
			return err
		}},
		{"StampImages", func() error {
			_, err := StampImages(pdf, []Stamp{{Page: 1, Rect: [4]float64{10, 10, 50, 50}, PNG: png}})
			return err
		}},
		{"FillFormCSV", func() error { _, err := FillFormCSV(pdf, []byte("a\n1\n"), ""); return err }},
		{"ExportFormJSON", func() error { _, err := ExportFormJSON(pdf); return err }},
		{"ExtractImagesZip", func() error { _, _, err := ExtractImagesZip(pdf); return err }},
		{"Encrypt", func() error { _, err := Encrypt(pdf, "pw"); return err }},
		{"RemovePassword", func() error { _, err := RemovePassword(enc.Bytes(), "pw"); return err }},
		{"RedactPages", func() error {
			_, err := RedactPages(pdf, map[int]RasterPage{2: {Image: png, W: 612, H: 792}})
			return err
		}},
		{"NUp", func() error { _, err := NUp(pdf, 2, false); return err }},
		{"NormalizePageSizes", func() error { _, err := NormalizePageSizes(pdf); return err }},
	}
	for _, op := range ops {
		var err error
		st := time.Now()
		finishesWithin(t, 20, op.name+" over a chain of 400 forms", func() { err = op.run() })
		// What each answers is its own business (the chain has no fields, outline or images); that it ANSWERS is
		// this test's. The answer is logged so a changed one is visible.
		t.Logf("%s: %v (%v)", op.name, err, time.Since(st).Round(time.Millisecond))
	}
	// The ones whose answer is not "nothing to do": the document's bytes come back, every form still in them.
	for name, run := range map[string]func() ([]byte, error){
		"Encrypt":        func() ([]byte, error) { return Encrypt(pdf, "pw") },
		"RemovePassword": func() ([]byte, error) { return RemovePassword(enc.Bytes(), "pw") },
		"StampImages": func() ([]byte, error) {
			return StampImages(pdf, []Stamp{{Page: 1, Rect: [4]float64{10, 10, 50, 50}, PNG: png}})
		},
	} {
		out, err := run()
		if err != nil {
			t.Errorf("%s over the chain: %v", name, err)
			continue
		}
		if name == "Encrypt" {
			if out, err = RemovePassword(out, "pw"); err != nil {
				t.Errorf("the chain Encrypt wrote does not decrypt: %v", err)
				continue
			}
		}
		if got := formObjects(t, out); got < 401 { // a watermark adds its own form
			t.Errorf("%s over the chain wrote %d form XObjects, want all 401 kept", name, got)
		}
	}
}
