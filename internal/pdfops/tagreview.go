package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// The review doors — `PLAN-accessibility.md` P08.S06b.
//
// The server and the Tags card see a proposal as plain values and hand back a review: every element,
// in the order the reviewer wants, each with the role they chose or marked ignored. `CommitTags`
// never trusts the review to describe the document — it proposes again from the bytes it is given
// and requires the review to account for exactly those elements, text for text, before anything is
// written. A document that changed between propose and commit is refused as stale rather than
// tagged by a list that described something else.

// TagElement is one proposed element as a reviewer sees it.
// The JSON tags are the proposal route's field names (`internal/server/tags.go`), held equal by a server
// test, so `nib tag propose --json` prints what the Tags card reads (P10.S01).
type TagElement struct {
	ID     int    `json:"id"`
	Role   string `json:"role"`
	Page   int    `json:"page"`
	Text   string `json:"text"`
	Marker string `json:"marker,omitempty"`
	List   int    `json:"list"`
	// Rect is the element's extent on its page in PDF user space: left, bottom, right, top. The
	// vertical extent is ESTIMATED from baselines and size (0.85 em up, 0.25 em down) — enough to
	// point at an element, not a measurement of its glyph boxes.
	Rect [4]float64 `json:"rect"`
	// PageBox is the page's MediaBox — llx, lly, urx, ury — so a client can place Rect on the page it
	// renders. Page rotation and CropBox are not applied.
	PageBox [4]float64 `json:"pageBox"`
}

// TagPageNote is a page whose layout the grouping reported, and why.
type TagPageNote struct {
	Page   int    `json:"page"`
	Reason string `json:"reason"`
}

// TagProposal is a document's proposed structure, as reviewable values.
type TagProposal struct {
	Elements    []TagElement  `json:"elements"`
	Unsupported []TagPageNote `json:"unsupported"`
	NoText      []int         `json:"noText"`
}

// TagReview is one element as the reviewer left it.
type TagReview struct {
	ID     int
	Role   string
	Ignore bool
	// Text is the element's text as it was proposed, echoed back, so a commit can tell the review
	// still describes the document.
	Text string
}

// ErrTagsStale is a review that no longer describes the document it is committed to.
var ErrTagsStale = errCommitStale

// ErrTagsReview is a review that is malformed on its own terms: an unknown element, one listed twice,
// a role nobody can choose, or nothing left to commit.
var ErrTagsReview = errors.New("pdfops: the review cannot be applied")

// reviewRoles is what a reviewer may choose.
var reviewRoles = map[string]bool{"H1": true, "H2": true, "H3": true, "H4": true, "H5": true, "H6": true, "P": true, "LI": true}

// ProposeTags proposes a structure for pdf. It writes nothing.
func ProposeTags(pdf []byte) (TagProposal, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return TagProposal{}, err
	}
	p, err := proposeStructure(ctx)
	if err != nil {
		return TagProposal{}, err
	}
	out := TagProposal{Elements: []TagElement{}, Unsupported: []TagPageNote{}, NoText: append([]int{}, p.noText...)}
	boxes := map[int][4]float64{}
	for i, el := range p.elements {
		box, ok := boxes[el.page]
		if !ok {
			box = mediaBoxOf(ctx, el.page)
			boxes[el.page] = box
		}
		out.Elements = append(out.Elements, TagElement{
			ID: i, Role: el.role, Page: el.page, Text: el.text, Marker: el.marker, List: el.list,
			Rect: elementRect(el), PageBox: box,
		})
	}
	pages := make([]int, 0, len(p.unsupported))
	for pg := range p.unsupported {
		pages = append(pages, pg)
	}
	sort.Ints(pages)
	for _, pg := range pages {
		out.Unsupported = append(out.Unsupported, TagPageNote{Page: pg, Reason: p.unsupported[pg]})
	}
	return out, nil
}

// CommitTags writes a reviewed proposal into pdf.
func CommitTags(pdf []byte, reviewed []TagReview) ([]byte, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	p, err := proposeStructure(ctx)
	if err != nil {
		return nil, err
	}
	if len(reviewed) != len(p.elements) {
		return nil, fmt.Errorf("%w: the review lists %d element(s) and the document proposes %d", errCommitStale, len(reviewed), len(p.elements))
	}
	seen := map[int]bool{}
	var ordered []proposedElement
	var ignoredPages []int // a page whose elements are all ignored is still committed — see commitProposal
	for _, r := range reviewed {
		if r.ID < 0 || r.ID >= len(p.elements) || seen[r.ID] {
			return nil, fmt.Errorf("%w: element %d is unknown or listed twice", ErrTagsReview, r.ID)
		}
		seen[r.ID] = true
		el := p.elements[r.ID]
		if el.text != r.Text {
			return nil, fmt.Errorf("%w (element %d)", errCommitStale, r.ID)
		}
		if r.Ignore {
			ignoredPages = append(ignoredPages, el.page)
			continue
		}
		if !reviewRoles[r.Role] {
			return nil, fmt.Errorf("%w: %q is not a role a reviewer can choose", ErrTagsReview, r.Role)
		}
		el.role = r.Role
		ordered = append(ordered, el)
	}
	if len(ordered) == 0 {
		return nil, fmt.Errorf("%w: every element was ignored, so there is nothing to commit", ErrTagsReview)
	}
	// Lists follow the REVIEWED order: consecutive list items are one list, whatever the proposal
	// grouped — a reviewer who retypes a paragraph between two lists into an item has joined them.
	lists, inList := 0, false
	for i := range ordered {
		if ordered[i].role != "LI" {
			ordered[i].list, inList = -1, false
			continue
		}
		if !inList {
			lists++
			inList = true
		}
		ordered[i].list = lists - 1
	}
	return commitProposal(pdf, ordered, ignoredPages...)
}

func mediaBoxOf(ctx *model.Context, pageNr int) [4]float64 {
	_, _, attrs, err := ctx.PageDict(pageNr, false)
	if err != nil || attrs == nil || attrs.MediaBox == nil {
		return [4]float64{}
	}
	mb := attrs.MediaBox
	return [4]float64{mb.LL.X, mb.LL.Y, mb.UR.X, mb.UR.Y}
}

func elementRect(el proposedElement) [4]float64 {
	if len(el.lines) == 0 {
		return [4]float64{}
	}
	x0, bottom := math.Inf(1), math.Inf(1)
	x1, top := math.Inf(-1), math.Inf(-1)
	for _, ln := range el.lines {
		x0, x1 = math.Min(x0, ln.x0), math.Max(x1, ln.x1)
		bottom, top = math.Min(bottom, ln.y-0.25*ln.size), math.Max(top, ln.y+0.85*ln.size)
	}
	return [4]float64{x0, bottom, x1, top}
}
