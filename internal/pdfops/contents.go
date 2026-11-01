package pdfops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A contents page inserted into the document — `/pending 929`, the second half of `/pending 910`.
//
// # Where the entries come from, and why there is no review step of its own
//
// From the document's bookmarks when it has any, otherwise from the headings the tagger reads (`OutlineFromHeadings`,
// the reading 910 reviews in the outline editor). So the review is the one 910 built: Fill from headings, correct the
// list, Save outline — and then the contents page says what the bookmarks say. A caller may also pass the list itself
// (`Items`), which is how a script reviews its own.
//
// # What it adds, and what it leaves
//
// k blank pages, sized as the neighbour page, carrying a title and one line per entry: the title at its indent, the
// page number at the right margin. They are pdfcpu text stamps — artifacts, in nib's own embedded face — so a tagged
// document gains no untagged content. The entries are NOT links in this version (declared): a link on a page of a tagged
// document must itself be tagged, and the bookmarks are the clickable form.
//
// # What shifts, and the rule for each
//
// The page numbers printed are the NEW physical numbers (an entry past the insertion point is shown k higher), found by
// laying out until k is stable. A destination names a page OBJECT and does not move. `/PageLabels` names a page INDEX and
// would silently misnumber every page after the insertion, so it is carried (`carriedPageLabels`, ADR-046): the new pages
// carry no label, every other page keeps the one it had. No number is shown from a label — the physical number is.

// ErrNoContentsEntries is the refusal of a contents page with nothing to list.
var ErrNoContentsEntries = errors.New("pdfops: there are no bookmarks and no headings to make a contents page from")

// ContentsOptions describes the page.
type ContentsOptions struct {
	Items []OutlineItem // the list to print; nil takes the bookmarks, else the headings
	After int           // insert after this page (1-based); 0 puts it before page 1
	Depth int           // levels shown, 1-3 (0 = 3)
	Title string        // the heading ("" = Contents)
}

// ContentsReport says what was made.
type ContentsReport struct {
	Entries int    `json:"entries"`
	Pages   int    `json:"pages"`  // contents pages inserted
	Source  string `json:"source"` // given | bookmarks | headings
}

const (
	contentsMargin  = 72.0 // points, all four sides
	contentsLeading = 17.0
	contentsSize    = 11
	contentsTitleSz = 22
	contentsIndent  = 18.0
	contentsMaxRows = 4000 // a document with more entries than this gets a refusal, not forty pages of list
)

type contentsLine struct {
	text   string
	number int // the page number printed; 0 for the title line
	level  int
	title  bool
}

// InsertContents inserts the contents page(s) and returns them with a report.
func InsertContents(pdf []byte, opt ContentsOptions) ([]byte, ContentsReport, error) {
	n, err := PageCount(pdf)
	if err != nil {
		return nil, ContentsReport{}, err
	}
	if opt.After < 0 || opt.After > n {
		return nil, ContentsReport{}, &PageRangeError{Page: opt.After, Pages: n}
	}
	depth := opt.Depth
	if depth < 1 || depth > 3 {
		depth = 3
	}
	title := strings.TrimSpace(opt.Title)
	if title == "" {
		title = "Contents"
	}
	rep := ContentsReport{}
	items := opt.Items
	switch {
	case items != nil:
		rep.Source = "given"
	default:
		if items, err = Outline(pdf); err != nil {
			return nil, ContentsReport{}, err
		}
		rep.Source = "bookmarks"
		if len(items) == 0 {
			if items, err = OutlineFromHeadings(pdf); err != nil {
				return nil, ContentsReport{}, err
			}
			rep.Source = "headings"
		}
	}
	var shown []OutlineItem
	for _, it := range items {
		if it.Level < depth && strings.TrimSpace(it.Title) != "" {
			if err := pageInDocument(it.Page, n); err != nil {
				return nil, ContentsReport{}, err
			}
			shown = append(shown, it)
		}
	}
	if len(shown) == 0 {
		return nil, ContentsReport{}, ErrNoContentsEntries
	}
	if len(shown) > contentsMaxRows {
		return nil, ContentsReport{}, fmt.Errorf("pdfops: %d entries is more than a contents page lists (%d); lower the depth", len(shown), contentsMaxRows)
	}
	rep.Entries = len(shown)

	// The neighbour's box: the page the contents sit before (After == 0) or after.
	ref := max(opt.After, 1)
	llx, lly, urx, ury, err := PageBox(pdf, ref)
	if err != nil {
		return nil, ContentsReport{}, err
	}
	width, height := urx-llx, ury-lly
	first := int((height - 2*contentsMargin - 2*contentsLeading) / contentsLeading)
	rest := int((height - 2*contentsMargin) / contentsLeading)
	if first < 3 || rest < 3 || width < 2*contentsMargin+120 {
		return nil, ContentsReport{}, fmt.Errorf("pdfops: that page (%.0f × %.0f pt) is too small to hold a contents page", width, height)
	}

	// Lay out until the page count is stable: the numbers printed depend on it and its length depends on them
	// (only through a digit more or fewer, which does not move a line — but the loop states the dependence).
	k := 1
	var pages [][]contentsLine
	for pass := 0; pass < 4; pass++ {
		pages = layoutContents(title, shown, opt.After, k, first, rest)
		if len(pages) == k {
			break
		}
		k = len(pages)
	}
	if len(pages) != k {
		return nil, ContentsReport{}, errors.New("pdfops: the contents page count did not settle")
	}
	rep.Pages = k

	// 1. the blank pages, in one block.
	out := pdf
	for i := 0; i < k; i++ {
		if opt.After == 0 {
			out, err = InsertBlank(out, 1, true)
		} else {
			out, err = InsertBlank(out, opt.After, false)
		}
		if err != nil {
			return nil, ContentsReport{}, err
		}
	}
	// 2. the labels, if the document has them: a page label names an index, so every one after the block moved.
	if out, err = shiftPageLabels(out, n, opt.After, k); err != nil {
		return nil, ContentsReport{}, err
	}
	// 3. the text.
	if out, err = stampContents(out, opt.After, pages, width); err != nil {
		return nil, ContentsReport{}, err
	}
	return out, rep, nil
}

// layoutContents breaks the lines into pages for a block of k pages inserted after `after`.
func layoutContents(title string, items []OutlineItem, after, k, first, rest int) [][]contentsLine {
	lines := []contentsLine{{text: title, title: true}}
	for _, it := range items {
		num := it.Page
		if num > after {
			num += k
		}
		text := strings.Join(strings.Fields(it.Title), " ")
		if limit := 76 - 6*it.Level; len([]rune(text)) > limit {
			text = strings.TrimSpace(string([]rune(text)[:limit-1])) + "…"
		}
		lines = append(lines, contentsLine{text: text, number: num, level: it.Level})
	}
	var pages [][]contentsLine
	capacity := first
	for len(lines) > 0 {
		take := min(capacity, len(lines))
		pages = append(pages, lines[:take])
		lines = lines[take:]
		capacity = rest
	}
	return pages
}

// shiftPageLabels re-expresses /PageLabels over the output, whose pages after `after` are k later and whose k new
// pages carry no label. A document without the key is returned unchanged.
func shiftPageLabels(pdf []byte, n, after, k int) ([]byte, error) {
	keep := make([]int, 0, n+k)
	for i := 1; i <= after; i++ {
		keep = append(keep, i)
	}
	for i := 0; i < k; i++ {
		keep = append(keep, 0) // a page the source's tree does not reach: the empty label
	}
	for i := after + 1; i <= n; i++ {
		keep = append(keep, i)
	}
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.OPTIMIZE
	return rewriteWithConf(pdf, conf, func(ctx *model.Context) error {
		root, err := ctx.Catalog()
		if err != nil {
			return err
		}
		if _, has := root["PageLabels"]; !has {
			return nil
		}
		if labels := carriedPageLabels(ctx.XRefTable, root, keep); labels != nil {
			root["PageLabels"] = labels
		} else {
			delete(root, "PageLabels")
		}
		return nil
	})
}

// stampContents draws the lines on the k pages after `after`, one text stamp each, in nib's own embedded face.
func stampContents(pdf []byte, after int, pages [][]contentsLine, width float64) ([]byte, error) {
	face, _, embedded := AuthoredTextFaces()
	return stampTextWatermarks(pdf, embedded, []string{face}, func(ctx *model.Context, embedded bool) error {
		name := "Helvetica"
		if embedded {
			name = face
		}
		wms := map[int][]*model.Watermark{}
		put := func(page int, desc, text string) error {
			t, err := stampText(text)
			if err != nil {
				return err
			}
			if t == "" {
				return nil
			}
			wm, err := api.TextWatermark(context.Background(), t, desc, true, false, types.POINTS, nil)
			if err != nil {
				return err
			}
			wms[page] = append(wms[page], wm)
			return nil
		}
		for i, lines := range pages {
			page := after + 1 + i
			y := contentsMargin
			for _, ln := range lines {
				size, x := contentsSize, contentsMargin+contentsIndent*float64(ln.level)
				if ln.title {
					size = contentsTitleSz
					x = contentsMargin
				}
				d := fmt.Sprintf("fontname:%s, points:%d, scalefactor:1 abs, position:tl, offset:%.1f %.1f, fillcolor:#000000, rotation:0", name, size, x, -y)
				if err := put(page, d, ln.text); err != nil {
					return err
				}
				if !ln.title {
					d = fmt.Sprintf("fontname:%s, points:%d, scalefactor:1 abs, position:tr, offset:%.1f %.1f, fillcolor:#000000, rotation:0", name, size, -contentsMargin, -y)
					if err := put(page, d, fmt.Sprint(ln.number)); err != nil {
						return err
					}
				}
				y += contentsLeading
				if ln.title {
					y += contentsLeading // a blank line under the title
				}
			}
		}
		return contentHeldUnfiltered(ctx, wms, func() error { return pdfcpu.AddWatermarksSliceMap(context.Background(), ctx, wms) })
	})
}
