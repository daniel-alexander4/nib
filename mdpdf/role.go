package mdpdf

// Structure carried from the Markdown AST to the laid-out runs — `PLAN-accessibility.md` P06.S01.
//
// # Why this exists, and why it is not output
//
// `mdpdf` walks a real goldmark AST: at the moment it lays out a line it knows whether that line is
// a level-2 heading, the second item of a nested list, or a code block. It then **throws all of it
// away** — a `style` carries a font and a size, which is everything a renderer needs and nothing a
// tagger does.
//
// P06.S02 tags a Markdown document from its own structure rather than wrapping each page in one
// generic `/Div` (which is all P05.S04's emitter, knowing nothing, could honestly claim). That needs
// this information to survive the layout, and **only that**: nothing here changes a byte of what is
// drawn.
//
// # Why roles are positional rather than a tree
//
// The runs come out of `spec()` in order, one pdfcpu text entry each, and pdfcpu emits them in that
// order — so the Nth run is the Nth text-drawing operator in the page's content stream. A caller
// that wants to bracket a run in marked content needs to know *which* run, and a flat per-page list
// in draw order says exactly that. A tree would have to be re-flattened into this to be usable, and
// the flattening is where a mismatch would hide.
type Structure struct {
	// Pages holds one entry per page, each a list of roles in the order the runs were drawn.
	Pages [][]Role
}

// RoleKind is the structural element a run belongs to.
type RoleKind uint8

const (
	// RoleBody is ordinary paragraph text, and the zero value: a run nobody classified is body
	// text, which is what it was before this existed.
	RoleBody RoleKind = iota
	// RoleHeading is a heading; `Role.Level` is 1-6.
	RoleHeading
	// RoleListItem is text inside a list item; `Role.Level` is the nesting depth, 1 for a
	// top-level list.
	RoleListItem
	// RoleCode is a preformatted block.
	RoleCode
	// RoleQuote is block-quoted text; `Role.Level` is the quote nesting depth.
	RoleQuote
	// RoleMarker is a list bullet or number — the `•` or `2.` drawn in the gutter.
	//
	// It is its own kind because PDF/UA wants it as `/Lbl` beside the item's `/LBody`, and because
	// it is the one run whose text is not in the source document at all.
	RoleMarker
)

func (k RoleKind) String() string {
	switch k {
	case RoleBody:
		return "body"
	case RoleHeading:
		return "heading"
	case RoleListItem:
		return "listitem"
	case RoleCode:
		return "code"
	case RoleQuote:
		return "quote"
	case RoleMarker:
		return "marker"
	}
	return "?"
}

// Role is one run's structural identity.
type Role struct {
	Kind RoleKind
	// Level is the heading level (1-6) for RoleHeading, and the nesting depth for RoleListItem and
	// RoleQuote. Zero for everything else.
	Level int
	// Block is an ordinal identifying the block-level construct this run belongs to. Runs of the
	// same paragraph, the same code block or the same list item share one; every new construct gets
	// the next number.
	//
	// **Kind and Level alone cannot express what a tagger needs**, which is the finding P06.S02's
	// grill produced against P06.S01's first shape. Measured on a document with two consecutive
	// paragraphs and a two-line code block: runs 1 and 2 were both `body/0` and runs 7 and 8 were
	// both `code/0`, and those two cases need OPPOSITE treatment — the paragraphs are two elements,
	// the code lines are two MCIDs of one. Nothing in `{Kind, Level}` distinguishes them.
	//
	// It is an ordinal rather than a pointer or a nesting path because the consumer walks runs in
	// draw order and only ever asks *"is this the same block as the last one"*. A path would be a
	// second tree to keep in step with the first.
	Block int
}
