package pdfops

import (
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// What a command line asks of a document's pages without a window (`nib info`, `nib text`; /pending 924, 925): how big each
// page is as it is shown, and what it prints and where. The page map is the one reader of both (ADR-088), so a figure
// here is the figure the window works from.

// PageInfo is one page's geometry. Width and Height are the page as DISPLAYED, in points: the visible box (the crop box,
// else the media box) with /Rotate applied, which is what a person measures. The boxes are as the file gives them, in the
// page's own user space.
type PageInfo struct {
	Page   int        `json:"page"`
	Width  float64    `json:"width"`
	Height float64    `json:"height"`
	Rotate int        `json:"rotate"`
	Media  [4]float64 `json:"mediaBox"`
	Crop   [4]float64 `json:"cropBox"`
}

// ListPages gives each page's geometry, in page order. A document with no page lists none.
func ListPages(pdf []byte) ([]PageInfo, error) {
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	pages := pdfread.Pages(ctx)
	out := make([]PageInfo, 0, len(pages))
	for _, pg := range pages {
		sp := newDisplaySpace(pg)
		out = append(out, PageInfo{Page: pg.Nr, Width: sp.w, Height: sp.h, Rotate: pageRotation(pg), Media: mediaBoxOf(pg), Crop: visibleBoxOf(pg)})
	}
	return out, nil
}

// TextRun is one run of text a page prints. Rect is [left top right bottom] in points on the DISPLAYED page, origin at
// its top left, so no caller turns the page or flips an axis; Size is the text's height in points. A run is one show
// operator's text, as the page map reads it (ADR-088) — in the order the page DRAWS it, which is not reading order.
type TextRun struct {
	Text   string     `json:"text"`
	Rect   [4]float64 `json:"rect"`
	Size   float64    `json:"size"`
	Hidden bool       `json:"hidden,omitempty"`
}

// PageText is what one page prints.
type PageText struct {
	Page   int     `json:"page"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// NoText is a page that sets no text at all: a scan, or text drawn as outlines.
	NoText bool `json:"noText,omitempty"`
	// Unread is how many glyphs on the page have no text the document lets nib read (`PageMap.Unread`, ADR-176). Above
	// zero, Runs is not all the page prints.
	Unread int       `json:"unread,omitempty"`
	Runs   []TextRun `json:"runs"`
}

// DocumentText reads the text of every page of pdf in ONE pass: the document is validated once, its pages walked once,
// and the pages share one walk budget — a loop over `MapPage` pays the validation again for each page. A page that cannot
// be read fails the whole read, naming it: a partial document passed off as a whole is the loss /pending 918 was.
func DocumentText(pdf []byte) ([]PageText, error) {
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	pages := pdfread.Pages(ctx)
	budget := newFormWalkBudget(len(pages))
	out := make([]PageText, 0, len(pages))
	for _, pg := range pages {
		m, err := mapPageIn(ctx, pg, budget)
		if err != nil {
			return nil, err
		}
		pt := PageText{Page: m.Page, Width: m.Width, Height: m.Height, NoText: m.NoText, Unread: m.Unread, Runs: make([]TextRun, 0, len(m.Text))}
		for _, t := range m.Text {
			pt.Runs = append(pt.Runs, TextRun{
				Text:   t.Text,
				Rect:   [4]float64{t.Rect[0] * m.Width, t.Rect[1] * m.Height, t.Rect[2] * m.Width, t.Rect[3] * m.Height},
				Size:   t.Size * m.Height,
				Hidden: t.Hidden,
			})
		}
		out = append(out, pt)
	}
	return out, nil
}
