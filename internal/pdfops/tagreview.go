package pdfops

import (
	"errors"
	"fmt"
	"math"
	"nib/internal/pdfread"
	"sort"
	"strings"
)

// The review doors — `PLAN-accessibility.md` P08.S06b.
//
// The server and the Tags card see a proposal as plain values and hand back a review: every element,
// in the order the reviewer wants, each with the role they chose or marked ignored. `CommitTags`
// never trusts the review to describe the document — it proposes again from the bytes it is given
// and requires the review to account for exactly those elements, text for text, before anything is
// written. A document that changed between propose and commit is refused as stale rather than
// tagged by a list that described something else.
//
// # Figures (ADR-122)
//
// A Figure is proposed for an image the page draws, and it is the one element a review must ADD something to:
// kept, it needs a description of what the picture shows (`TagReview.Alt`), because a Figure with none fails
// the clause it was written for (ua1 7.3 t1) and nothing here can look at a picture. Ignored, it is left the
// artifact it would have been. **A Figure has no text, so the echo that catches a changed paragraph cannot see
// a changed picture**: an image replaced by another in the same place, by the same operator, commits under the
// description written for the first. What the commit does hold is where the image is — it proposes again, and
// the figure's operator must be the one a fresh read of the page finds.

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
	// Parent is the ID of the element this one sits under, or -1 at the top level. Only a table has children:
	// Table, then each TR and its TH/TD cells directly after it, row by row (ADR-121).
	Parent int `json:"parent"`
	// Rect is the element's extent on its page in PDF user space: left, bottom, right, top. The
	// vertical extent is ESTIMATED from baselines and size (0.85 em up, 0.25 em down) — enough to
	// point at an element, not a measurement of its glyph boxes. A table, a row and a cell carry the ruled
	// grid's own box instead, and a Figure the box its image is drawn in (ADR-122).
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
	// Alt is the description of what a Figure shows, which a kept Figure must have and no other element may
	// carry (ADR-122).
	Alt string
}

// ErrTagsStale is a review that no longer describes the document it is committed to.
var ErrTagsStale = errCommitStale

// ErrTagsReview is a review that is malformed on its own terms: an unknown element, one listed twice,
// a role nobody can choose, or nothing left to commit.
var ErrTagsReview = errors.New("pdfops: the review cannot be applied")

// reviewRoles is what a reviewer may choose for an element that is not part of a table. A cell is a TH or a
// TD (`cellRoles`); a Table, a TR and a Figure keep their type.
var reviewRoles = map[string]bool{"H1": true, "H2": true, "H3": true, "H4": true, "H5": true, "H6": true, "P": true, "LI": true}

// ProposeTags proposes a structure for pdf. It writes nothing.
func ProposeTags(pdf []byte) (TagProposal, error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return TagProposal{}, err
	}
	p, err := proposeStructure(ctx)
	if err != nil {
		return TagProposal{}, err
	}
	out := TagProposal{Elements: []TagElement{}, Unsupported: []TagPageNote{}, NoText: append([]int{}, p.noText...)}
	boxes := map[int][4]float64{}
	var walked []pdfread.Page
	if len(p.elements) > 0 {
		walked = pdfread.Pages(ctx)
	}
	for i, el := range p.elements {
		box, ok := boxes[el.page]
		if !ok {
			box = mediaBoxOf(pageAt(ctx, walked, el.page))
			boxes[el.page] = box
		}
		out.Elements = append(out.Elements, TagElement{
			ID: i, Role: el.role, Page: el.page, Text: el.text, Marker: el.marker, List: el.list, Parent: el.parent,
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
	ctx, err := inspectionRead(pdf)
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
	// A table is reviewed as one thing (ADR-121): its rows and cells stay under it, in the order proposed. They
	// were proposed consecutively, so "each directly after the element proposed before it" is the whole rule —
	// it holds the parent first, the subtree unbroken and every row and cell in place.
	//
	// **A table the reviewer says is not one is written as paragraphs** — the Table reviewed as `P`. A ruled form
	// tiles as regularly as a table does, and without this the only answers to a grid that is not a table were to
	// tag it as one or to ignore it, which marks its text as decoration. Each cell that has text becomes a
	// paragraph, row by row, and the Table and its rows are not written.
	last, ignoredTable, declined := -1, -1, false
	for _, r := range reviewed {
		if r.ID < 0 || r.ID >= len(p.elements) || seen[r.ID] {
			return nil, fmt.Errorf("%w: element %d is unknown or listed twice", ErrTagsReview, r.ID)
		}
		seen[r.ID] = true
		el := p.elements[r.ID]
		if el.text != r.Text {
			return nil, fmt.Errorf("%w (element %d)", errCommitStale, r.ID)
		}
		if el.role != figureRole && r.Alt != "" {
			return nil, fmt.Errorf("%w: only a figure takes a description of what it shows, and element %d is not one", ErrTagsReview, r.ID)
		}
		inTable := el.parent >= 0
		if inTable && last != r.ID-1 {
			return nil, fmt.Errorf("%w: a table's rows and cells stay under it in the order they were proposed (element %d) — move the whole table", ErrTagsReview, r.ID)
		}
		last = r.ID
		if inTable && r.Ignore {
			return nil, fmt.Errorf("%w: a row or a cell cannot be ignored by itself (element %d) — ignore the whole table", ErrTagsReview, r.ID)
		}
		if el.role == "Table" {
			ignoredTable, declined = -1, !r.Ignore && r.Role == "P"
			if r.Ignore {
				ignoredTable = r.ID
			}
			if declined {
				continue
			}
		}
		if r.Ignore || (inTable && ignoredTable >= 0) {
			ignoredPages = append(ignoredPages, el.page)
			continue
		}
		if inTable && declined {
			if cellRoles[el.role] && len(el.lines) > 0 {
				el.role, el.parent = "P", -1
				ordered = append(ordered, el)
			}
			continue
		}
		switch {
		case el.role == figureRole:
			// A picture is a Figure or it is ignored (ADR-122): nothing else it could be made has anything to hold.
			if r.Role != figureRole {
				return nil, fmt.Errorf("%w: a figure keeps its type — it cannot be made %q (element %d); ignore it if it is decoration", ErrTagsReview, r.Role, r.ID)
			}
			if strings.TrimSpace(r.Alt) == "" {
				return nil, fmt.Errorf("%w: a figure needs a description of what it shows, or must be ignored (element %d, page %d)", ErrTagsReview, r.ID, el.page)
			}
			el.alt = r.Alt
		case r.Role == figureRole:
			return nil, fmt.Errorf("%w: only a picture the page draws can be a Figure, and element %d is text", ErrTagsReview, r.ID)
		case cellRoles[el.role]:
			if !cellRoles[r.Role] {
				return nil, fmt.Errorf("%w: a table cell is a header cell (TH) or a data cell (TD), not %q (element %d)", ErrTagsReview, r.Role, r.ID)
			}
		case tableRoles[el.role]:
			if r.Role != el.role {
				return nil, fmt.Errorf("%w: a table and its rows keep their type (a table that is not one is reviewed as P, and its cells are written as paragraphs) — %q cannot be made %q (element %d)", ErrTagsReview, el.role, r.Role, r.ID)
			}
		case !reviewRoles[r.Role]:
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

// mediaBoxOf is the page's inherited /MediaBox, or zeros. The caller resolves the page (`pageAt`) — both callers
// ask it once per page of the document (/pending 756).
func mediaBoxOf(pg pdfread.Page) [4]float64 {
	attrs := pg.Attrs
	if pg.Err != nil || attrs == nil || attrs.MediaBox == nil {
		return [4]float64{}
	}
	mb := attrs.MediaBox
	return [4]float64{mb.LL.X, mb.LL.Y, mb.UR.X, mb.UR.Y}
}

func elementRect(el proposedElement) [4]float64 {
	if el.boxed {
		return el.box
	}
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
