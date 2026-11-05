// Package docmodel is the one document model the editable exports share (PLAN-import-export D4, D16, ADR-192).
//
// It holds data and nothing that reads a PDF: the reader is `docmodel/read`, and the writers (`internal/docwrite/<format>`,
// P02.S04 and P03) import this package and no PDF reader — `TestNoWriterReachesAPDFReader` holds that over the transitive
// import graph, so a writer cannot grow its own reading of the page and give a fifth document of one PDF.
package docmodel

// Kind is what a Block is.
type Kind string

const (
	Heading   Kind = "heading"
	Paragraph Kind = "paragraph"
	ListItem  Kind = "listitem"
	Table     Kind = "table"
	Figure    Kind = "figure"
)

// Source says which structure a document, or one of its pages, was read from (D17).
const (
	SourceTree     = "tree"     // the author's own tagging
	SourceProposal = "proposal" // the tagger's proposal over the page's runs
	SourceMixed    = "mixed"    // a tagged document some of whose pages fell back to the proposal
)

// Span is a stretch of text in one style. Link is an http, https or mailto URI, else "" (W1).
type Span struct {
	Text   string
	Bold   bool
	Italic bool
	Link   string
}

// Cell is one table cell: its paragraphs, each a run of spans. ColSpan and RowSpan are at least 1.
type Cell struct {
	Paragraphs [][]Span
	ColSpan    int
	RowSpan    int
	Header     bool
}

// Image is a picture, or the placeholder that stands for one a writer cannot carry.
type Image struct {
	// Data is the encoded picture, nil for a placeholder; Ext is "jpg" or "png".
	Data          []byte
	Ext           string
	Width, Height int
	// Placeholder is the cause Data is nil, one of pdfops' Placeholder* words, or "".
	Placeholder string
	// Alt describes the picture: the author's, else "Scanned page N" for a page that is only a picture (D20).
	Alt string
}

// Block is one thing in the document, in reading order.
type Block struct {
	Kind Kind
	// Level is a heading's level (1-6) or a list item's nesting (1 is outermost).
	Level int
	// Marker is a list item's label as drawn; List numbers the list it belongs to.
	Marker string
	List   int
	// Page is the page the block begins on.
	Page int
	// Dir is "ltr" or "rtl", from the block's text.
	Dir string
	// Spans is the text of a heading, paragraph or list item.
	Spans []Span
	// Rows is a table's cells, row by row; HeaderRows is how many leading rows are headers.
	Rows       [][]Cell
	HeaderRows int
	// Image is a Figure's picture.
	Image *Image
	// Running says a paragraph is a running header or footer.
	Running bool
}

// Text is the block's text without style.
func (b Block) Text() string {
	var s string
	for _, sp := range b.Spans {
		s += sp.Text
	}
	return s
}

// Meta is what the document says of itself. Title is never taken from a source path; Lang is the catalog's, else "und".
type Meta struct {
	Title string
	Lang  string
	Dir   string
}

// FlaggedPage is a page the export read with a stated limit.
type FlaggedPage struct {
	Page  int
	Cause string
}

// Causes a page is flagged (D17, D19, D20).
const (
	FlagTreeIncomplete = "tree-incomplete" // the author's tree does not account for the page's visible words
	FlagLayout         = "layout"          // the grouping reported a layout it could not order (the cause is in Report.Layout)
	FlagUnread         = "unread-glyphs"   // glyphs with no text the document lets nib read
	FlagEmpty          = "nothing-read"    // no text and no picture was read from the page
)

// Report is what the export did not carry, and why — counts and enumerations only, never a name or a value, so the
// route can put it in a header (W3, W11).
type Report struct {
	// Source is SourceTree, SourceProposal or SourceMixed (SourceProposal for an untagged document).
	Source string
	// Tagged says the document has a structure tree at all.
	Tagged bool
	// TreeUnread says the tree could not be read; the proposal was used throughout.
	TreeUnread bool
	// FlaggedPages are pages read with a limit; ScannedPages are pages exported as their own picture (D20).
	FlaggedPages []FlaggedPage
	ScannedPages []int
	// Layout names, by page, the layout the grouping could not order (D19).
	Layout map[int]string
	// Placeholders counts pictures not carried, by cause. DroppedLinks counts links not carried, by cause.
	Placeholders map[string]int
	DroppedLinks map[string]int
	// Text left out because a reader could not see it, by cause (D30, X6).
	CoveredText   int
	MaybeCovered  int
	HiddenText    int
	HiddenLayer   int
	OffPage       int
	CoverUntested int
	UnreadGlyphs  int
	// ArtifactText counts runs of text the document marks as an artifact (page numbers, running headers, watermarks), left
	// out of every export (/pending 949).
	ArtifactText int
	// RunningSetAside counts running headers and footers a tagged page's tree does not hold, left out of that page.
	RunningSetAside int
	// NoTextPages counts pages that draw no glyph at all (a scan, or text drawn as outlines); SkippedImages counts pictures
	// drawn and not offered as figures, by cause ("artifact", "small").
	NoTextPages   int
	SkippedImages map[string]int
	// UnstyledLines counts lines whose style could not be cut and are one plain span.
	UnstyledLines int
	// TablesAsText counts tables past the bounds in bound.go, written as one line per row.
	TablesAsText int
	// BidiControls counts the text-direction embedding, override and isolate controls Clean removes from the text (/pending 947).
	BidiControls int
}

// Document is the model.
type Document struct {
	Meta   Meta
	Blocks []Block
	Report Report
}
