package cli

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

func stampFixture(t *testing.T) (dir, in, logo string) {
	t.Helper()
	dir = t.TempDir()
	base, _ := testpdf.Text("page one", "page two")
	in = filepath.Join(dir, "in.pdf")
	os.WriteFile(in, base, 0o600)
	var buf bytes.Buffer
	png.Encode(&buf, patterned())
	logo = filepath.Join(dir, "logo.png")
	os.WriteFile(logo, buf.Bytes(), 0o600)
	return
}

// TestStampManyInOneCall — RelaxTax B3. Text with a font, size and colour and a picture, each on its own page and
// rectangle, in one call; the counts say how the text fitted and never repeat it.
func TestStampManyInOneCall(t *testing.T) {
	dir, in, logo := stampFixture(t)
	list := filepath.Join(dir, "s.json")
	spec, _ := json.Marshal([]stampEntry{
		{Page: 1, Rect: [4]float64{100, 600, 300, 620}, Text: "STAMP-ONE", Font: "Courier-Bold", Size: 12, Color: "#cc0000"},
		{Page: 2, Rect: [4]float64{100, 500, 300, 520}, Text: "STAMP-TWO"},
		{Page: 2, Rect: [4]float64{400, 100, 480, 160}, Image: logo},
		{Page: 1, Rect: [4]float64{10, 10, 40, 22}, Text: "this text is far too long for a box this small and cannot shrink enough to fit it"},
	})
	os.WriteFile(list, spec, 0o600)
	out := filepath.Join(dir, "out.pdf")
	_, stderr, code := fillChild(t, "", "", "stamp", in, "--stamps", list, "-o", out, "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var rep map[string]int
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &rep); err != nil || rep["text"] != 3 || rep["images"] != 1 {
		t.Errorf("report %q (%v), want 3 text lines and 1 picture", stderr, err)
	}
	if strings.Contains(stderr, "STAMP") {
		t.Errorf("the report repeats the text: %q", stderr)
	}
	if rep["shrunk"]+rep["wrapped"]+rep["overran"] == 0 {
		t.Errorf("the long line in a small box shows no fit outcome: %q", stderr)
	}
	doc, _ := os.ReadFile(out)
	pages, err := pdfops.DocumentText(doc)
	if err != nil || len(pages) != 2 {
		t.Fatalf("%d pages, %v", len(pages), err)
	}
	found := func(page int, s string) *pdfops.TextRun {
		for _, r := range pages[page-1].Runs {
			if strings.Contains(r.Text, s) {
				return &r
			}
		}
		return nil
	}
	if r := found(1, "STAMP-ONE"); r == nil {
		t.Error("STAMP-ONE is not on page 1")
	} else if r.Size < 11 || r.Size > 13 {
		t.Errorf("STAMP-ONE is %v pt, want the 12 asked for", r.Size)
	}
	if found(2, "STAMP-TWO") == nil || found(1, "STAMP-TWO") != nil {
		t.Error("STAMP-TWO is not on page 2 alone")
	}
	imgs, err := pdfops.ExtractImageFiles(doc)
	if err != nil || len(imgs) != 1 || imgs[0].Page != 2 {
		t.Errorf("pictures %+v (%v), want one, on page 2", imgs, err)
	}
}

func TestStampManyRefusesWhatItCannotPlaceNamingTheEntry(t *testing.T) {
	dir, in, _ := stampFixture(t)
	for name, c := range map[string]struct{ spec, want string }{
		"page":    {`[{"page":9,"rect":[1,1,50,20],"text":"SECRET-1"}]`, "stamp 1 is on page 9"},
		"rect":    {`[{"page":1,"rect":[50,1,1,20],"text":"SECRET-2"}]`, "stamp 1: rect"},
		"both":    {`[{"page":1,"rect":[1,1,50,20],"text":"SECRET-3","image":"x.png"}]`, "exactly one of text and image"},
		"neither": {`[{"page":1,"rect":[1,1,50,20]}]`, "exactly one of text and image"},
		"empty":   {`[]`, "lists no stamp"},
	} {
		_, stderr, code := fillChild(t, c.spec, "", "stamp", in, "--stamps", "-", "-o", filepath.Join(dir, name+".pdf"))
		if code != 1 || !strings.Contains(stderr, c.want) {
			t.Errorf("%s: exit %d, stderr %q, want 1 containing %q", name, code, stderr, c.want)
		}
		if strings.Contains(stderr, "SECRET") {
			t.Errorf("%s: the refusal repeats the text: %q", name, stderr)
		}
		if _, err := os.Stat(filepath.Join(dir, name+".pdf")); err == nil {
			t.Errorf("%s: an output was written", name)
		}
	}
	if _, stderr, code := fillChild(t, "[]", "", "stamp", in, "--stamps", "x.json", "--text", "T", "-o", filepath.Join(dir, "m.pdf")); code != 1 || !strings.Contains(stderr, "carries its own pages") {
		t.Errorf("--stamps with --text: exit %d, %q", code, stderr)
	}
}
