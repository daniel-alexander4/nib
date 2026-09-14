package pdfops

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Seam S7 — nib's run reader against pdf.js's own extraction, on the same bytes.
// `PLAN-accessibility.md` P08.S02, `PLAN-text-reflow.md` P03's first exit criterion.
//
// ── What "agree" means here, and why it is not "the same items" ──────────────
// pdf.js cuts text into items by its own heuristics — measured, one LibreOffice `Tj` became three items
// split at a space, and every line ends with an empty end-of-line item. A run is the document's unit,
// one show operator. So the two readers are held to what both must produce however they cut: the
// page's TEXT, whitespace removed, and the set of BASELINES it sits on. A reader that dropped a run,
// decoded a code wrongly, or misplaced a line fails one of the two.
//
// The pdf.js side is `test/pdfjs/textcontent.mjs`, which loads the vendored build the app ships.

type pdfjsText struct {
	Version string `json:"version"`
	Pages   []struct {
		Page  int `json:"page"`
		Items []struct {
			Str string  `json:"str"`
			X   float64 `json:"x"`
			Y   float64 `json:"y"`
		} `json:"items"`
	} `json:"pages"`
}

func squeeze(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func baselineSet(ys []float64) map[int64]bool {
	set := map[int64]bool{}
	for _, y := range ys {
		set[int64(math.Round(y*2))] = true // half-point resolution
	}
	return set
}

func TestRunTextAndBaselinesAgreeWithPdfjs(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed, so pdf.js cannot run and seam S7 is not measured on this machine")
	}
	script := filepath.Join("..", "..", "test", "pdfjs", "textcontent.mjs")
	if _, serr := os.Stat(script); serr != nil {
		t.Fatalf("the pdf.js extraction script is missing: %v", serr)
	}
	compared := 0
	for _, doc := range runCorpus(t) {
		file := filepath.Join(t.TempDir(), "doc.pdf")
		if werr := os.WriteFile(file, doc.pdf, 0o600); werr != nil {
			t.Fatal(werr)
		}
		var stderr bytes.Buffer
		cmd := exec.Command(node, script, file)
		cmd.Stderr = &stderr
		out, rerr := cmd.Output()
		if rerr != nil {
			t.Fatalf("%s: pdf.js failed: %v\n%s", doc.name, rerr, stderr.String())
		}
		var js pdfjsText
		if jerr := json.Unmarshal(out, &js); jerr != nil {
			t.Fatalf("%s: pdf.js output is not JSON: %v", doc.name, jerr)
		}
		ctx, cerr := api.ReadValidateAndOptimize(bytes.NewReader(doc.pdf), model.NewDefaultConfiguration())
		if cerr != nil {
			t.Fatal(cerr)
		}
		if len(js.Pages) != ctx.PageCount {
			t.Errorf("%s: pdf.js read %d page(s), nib %d", doc.name, len(js.Pages), ctx.PageCount)
			continue
		}
		for _, pg := range js.Pages {
			pr, perr := readPageRuns(ctx, pg.Page)
			if perr != nil {
				t.Fatalf("%s page %d: %v", doc.name, pg.Page, perr)
			}
			var jsText strings.Builder
			var jsYs, goYs []float64
			for _, it := range pg.Items {
				if squeeze(it.Str) == "" {
					continue
				}
				jsText.WriteString(it.Str)
				jsYs = append(jsYs, it.Y)
			}
			var goText strings.Builder
			for _, r := range pr.runs {
				if squeeze(r.text) == "" {
					continue
				}
				goText.WriteString(r.text)
				goYs = append(goYs, r.y)
			}
			if g, j := squeeze(goText.String()), squeeze(jsText.String()); g != j {
				t.Errorf("%s page %d: the text disagrees\n  nib:    %q\n  pdf.js: %q", doc.name, pg.Page, g, j)
			}
			gb, jb := baselineSet(goYs), baselineSet(jsYs)
			if len(gb) != len(jb) {
				t.Errorf("%s page %d: nib finds %d baseline(s), pdf.js %d", doc.name, pg.Page, len(gb), len(jb))
			}
			for y := range jb {
				if !gb[y] {
					t.Errorf("%s page %d: pdf.js has text on the baseline at y=%.1f and nib has none", doc.name, pg.Page, float64(y)/2)
				}
			}
			if len(jb) > 0 {
				compared++
			}
		}
		t.Logf("%s: agreed with pdf.js %s", doc.name, js.Version)
	}
	if compared == 0 {
		t.Error("no page with text was compared, so the agreement means nothing")
	}
}
