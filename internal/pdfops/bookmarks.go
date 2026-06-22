package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
)

// OutlineItem is one bookmark in the document outline, flattened: a title, the
// page it jumps to (1-based), and a nesting level (0 = top). The flat, page-
// ordered, leveled form is what the editor works with; the nested tree pdfcpu
// reads and writes is reconstructed from the levels.
type OutlineItem struct {
	Title string `json:"title"`
	Page  int    `json:"page"`
	Level int    `json:"level"`
}

// Outline reads the document outline as a flat, depth-first leveled list. An empty
// result (no outline) is not an error.
func Outline(pdf []byte) ([]OutlineItem, error) {
	bms, err := api.Bookmarks(bytes.NewReader(pdf), nil)
	if err != nil {
		return nil, err
	}
	var items []OutlineItem
	var walk func(bs []pdfcpu.Bookmark, level int)
	walk = func(bs []pdfcpu.Bookmark, level int) {
		for _, b := range bs {
			items = append(items, OutlineItem{Title: b.Title, Page: b.PageFrom, Level: level})
			if len(b.Kids) > 0 {
				walk(b.Kids, level+1)
			}
		}
	}
	walk(bms, 0)
	return items, nil
}

// SetOutline replaces the document outline with items (a flat, page-ordered,
// leveled list); an empty list clears the outline. Titles must be non-empty and
// unique (pdfcpu keys each bookmark's destination by its title, so duplicates
// would collide), pages must be in range, and nesting may only deepen one level
// at a time. The caller is expected to keep items sorted by page so the rebuilt
// tree satisfies pdfcpu's "siblings ascending, child ≥ parent" rule.
func SetOutline(pdf []byte, items []OutlineItem) ([]byte, error) {
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	prevLevel := -1
	for _, it := range items {
		title := strings.TrimSpace(it.Title)
		if title == "" {
			return nil, fmt.Errorf("every bookmark needs a title")
		}
		if seen[title] {
			return nil, fmt.Errorf("bookmark titles must be unique: %q appears more than once", title)
		}
		seen[title] = true
		if it.Page < 1 || it.Page > n {
			return nil, fmt.Errorf("bookmark %q points to page %d, out of range (1-%d)", title, it.Page, n)
		}
		if it.Level < 0 || it.Level > prevLevel+1 {
			return nil, fmt.Errorf("bookmark %q is nested too deep for its position", title)
		}
		prevLevel = it.Level
	}

	if len(items) == 0 {
		var out bytes.Buffer
		if err := api.RemoveBookmarks(bytes.NewReader(pdf), &out, nil); err != nil {
			if errors.Is(err, api.ErrNoOutlines) {
				return pdf, nil // nothing to clear
			}
			return nil, err
		}
		return out.Bytes(), nil
	}

	var out bytes.Buffer
	if err := api.AddBookmarks(bytes.NewReader(pdf), &out, buildBookmarkTree(items), true, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// buildBookmarkTree reconstructs the nested bookmark tree from a flat, leveled
// list (validated: levels deepen by at most one). Each item becomes a Bookmark;
// the consecutive items one level deeper become its Kids.
func buildBookmarkTree(items []OutlineItem) []pdfcpu.Bookmark {
	i := 0
	var build func(level int) []pdfcpu.Bookmark
	build = func(level int) []pdfcpu.Bookmark {
		var out []pdfcpu.Bookmark
		for i < len(items) && items[i].Level == level {
			b := pdfcpu.Bookmark{Title: strings.TrimSpace(items[i].Title), PageFrom: items[i].Page}
			i++
			if i < len(items) && items[i].Level == level+1 {
				b.Kids = build(level + 1)
			}
			out = append(out, b)
		}
		return out
	}
	return build(0)
}
