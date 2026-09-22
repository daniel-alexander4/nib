package uacheck

// standardStructureTypes are the standard structure types of ISO 32000-1:2008 §14.8.4 — the set a
// conforming reader RECOGNISES, and so the set at which following a role map stops (§14.7.3, Note 2).
//
// **The checker holds its own copy, and `pdfops.standardStructTypes` is not borrowed** — the reason is
// `structure.go`'s header: a checker that read documents through the writer's model would agree with
// the writer by construction. Two copies of one ISO table is the price of that independence, and
// `TestTheCheckerHoldsTheSameStandardSetAsTheSpec` pins this one against the table rather than
// against pdfops' (P03.S01).
var standardStructureTypes = map[string]bool{
	// Grouping (Table 333)
	"Document": true, "Part": true, "Art": true, "Sect": true, "Div": true, "BlockQuote": true,
	"Caption": true, "TOC": true, "TOCI": true, "Index": true, "NonStruct": true, "Private": true,
	// Block-level: paragraphlike, list, table (Tables 334-337)
	"P": true, "H": true, "H1": true, "H2": true, "H3": true, "H4": true, "H5": true, "H6": true,
	"L": true, "LI": true, "Lbl": true, "LBody": true,
	"Table": true, "TR": true, "TH": true, "TD": true, "THead": true, "TBody": true, "TFoot": true,
	// Inline-level (Tables 338-339)
	"Span": true, "Quote": true, "Note": true, "Reference": true, "BibEntry": true, "Code": true,
	"Link": true, "Annot": true, "Ruby": true, "RB": true, "RT": true, "RP": true,
	"Warichu": true, "WT": true, "WP": true,
	// Illustration (Table 340)
	"Figure": true, "Formula": true, "Form": true,
}
