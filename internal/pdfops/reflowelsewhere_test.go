package pdfops

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
)

// lineKeys names every line of l outside paragraph skip (-1 for none) by where it is and what it reads.
func lineKeys(l pageLayout, skip int) []string {
	var out []string
	for pi, p := range l.paragraphs {
		if pi == skip {
			continue
		}
		for _, ln := range p.lines {
			out = append(out, fmt.Sprintf("%.2f..%.2f at %.2f %q", ln.x0, ln.x1, ln.y, ln.text))
		}
	}
	return out
}

// TestAReflowLeavesEveryOtherLineAsItRead — /pending 790, over the real-producer corpus. A paragraph re-set in place
// draws its lines at another point of the page's content, and the grouping must not read the REST of the page
// differently for it: every line outside the edited paragraph reads back where it was, as what it was. Before the
// lines were joined left to right (`lineSegments`), `antennahouse/irs-p15.pdf` p16 read its left column's re-set line
// as part of the right column's bullet 4pt above it, and the next edit's paragraph list was wrong for both columns.
//
// The edit removes the paragraph's last word, so nothing grows and nothing below moves. **A deterministic SAMPLE, and
// it says so**: every multi-line paragraph the rewrite can read, on every page — the page is re-read from the edited
// document in memory and the edit then taken back, and the first accepted edit of each document is also written out and
// read from the written bytes, so the in-memory reading is checked against the one a user's next edit would get.
//
// **The stimulus is asserted**: the named paragraph must be among those re-set, or the census never reached the page
// that showed the defect.
func TestAReflowLeavesEveryOtherLineAsItRead(t *testing.T) {
	corp := externalPDFs(t, "real producers", "NIB_UA_PRODUCERS", filepath.Join(homeDir(), "nib", "producers"))
	if corp.absent != "" {
		t.Skip("SKIP (not a measurement): " + corp.absent)
	}
	reset, moved, movedLines, written, named := 0, 0, 0, 0, false
	causes := map[string]int{}
	for _, doc := range corp.docs {
		ctx, err := pdfread.Validated(doc.pdf, model.NewDefaultConfiguration())
		if err != nil {
			continue
		}
		roundTripped := false
		for p := 1; p <= ctx.PageCount; p++ {
			pg := pageAt(ctx, nil, p)
			l, err := readPageGlyphLayout(ctx, pg)
			if err != nil || pg.Dict == nil {
				continue
			}
			original := pg.Dict["Contents"]
			for pi, para := range l.paragraphs {
				ws := editWords(para.text())
				if len(para.lines) < 2 || len(ws) < 2 {
					continue
				}
				out, err := reflowParagraphIn(ctx, l, pg, pi, strings.Join(ws[:len(ws)-1], " "))
				if err != nil {
					t.Fatalf("%s p%d ¶%d: %v", doc.name, p, pi, err)
				}
				if out.content == nil {
					causes[out.cause]++
					continue
				}
				if err := setPageContent(ctx, pg.Dict, out.content); err != nil {
					t.Fatal(err)
				}
				after, err := readPageGlyphLayout(ctx, pageAt(ctx, nil, p))
				if err != nil {
					t.Fatalf("%s p%d ¶%d: the re-set page does not read: %v", doc.name, p, pi, err)
				}
				if !roundTripped {
					roundTripped = true
					var buf bytes.Buffer
					if err := api.WriteContext(context.Background(), ctx, &buf); err != nil {
						t.Fatal(err)
					}
					c2, err := pdfread.Validated(buf.Bytes(), model.NewDefaultConfiguration())
					if err != nil {
						t.Fatalf("%s p%d ¶%d: the rewritten document does not read: %v", doc.name, p, pi, err)
					}
					l2, err := readPageGlyphLayout(c2, pageAt(c2, nil, p))
					if err != nil {
						t.Fatal(err)
					}
					if a, b := strings.Join(lineKeys(after, -1), "\n"), strings.Join(lineKeys(l2, -1), "\n"); a != b {
						t.Errorf("%s p%d ¶%d: the edited page reads one way in memory and another from the written document", doc.name, p, pi)
					}
					written++
				}
				pg.Dict["Contents"] = original
				reset++
				named = named || (doc.name == "antennahouse/irs-p15.pdf" && p == 16 && strings.Contains(para.text(), "qualified joint venture won’t in-"))
				have := map[string]int{}
				for _, k := range lineKeys(after, -1) {
					have[k]++
				}
				gone := 0
				for _, k := range lineKeys(l, pi) {
					if have[k] == 0 {
						gone++
						if gone == 1 {
							t.Errorf("%s p%d ¶%d: a line the edit did not set no longer reads as it did: %s", doc.name, p, pi, k)
						}
						continue
					}
					have[k]--
				}
				if gone > 0 {
					moved++
					movedLines += gone
				}
			}
		}
	}
	t.Logf("%d paragraphs re-set with their last word removed (%d also read from the written document); %d left a line elsewhere on the page reading differently (%d lines); refused by cause %v",
		reset, written, moved, movedLines, causes)
	if reset == 0 || written == 0 {
		t.Errorf("%d paragraphs re-set, %d read from a written document — the census asked nothing", reset, written)
	}
	if !named && hasDoc(corp, "antennahouse/irs-p15.pdf") {
		t.Error("antennahouse/irs-p15.pdf p16's paragraph was never re-set — the census did not reach the page that showed /pending 790")
	}
}

func hasDoc(c lawOneCorpus, name string) bool {
	for _, d := range c.docs {
		if d.name == name {
			return true
		}
	}
	return false
}
