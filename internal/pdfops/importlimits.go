package pdfops

// The import door's limits (PLAN-import-export D14, L7, ADR-185). Every cap is a named constant with a
// test that refuses at cap+1 and accepts at cap, and a cap REFUSES — it never truncates.
//
// **These are PROVISIONAL starting values** (R5): P01.S03 measures a run for each producer and tunes
// them. The first three are existing figures and are not guesses; the rest are.
const (
	// MaxImportBytes is the input size cap — the server's `maxPDFBytes` (ADR-008), restated here because an
	// import may be handed a path or a stream that never passed the server's reader. A test holds them equal.
	MaxImportBytes = 200 << 20

	// maxImagePixels (imagedoc.go) is the decoded pixel cap, 200 Mpx.

	// importArchiveEntries is the most members a document package (DOCX, ODT, …) may declare.
	importArchiveEntries = 2000
	// importDecodedBytes is the most an archive may expand to, summed over its declared sizes.
	importDecodedBytes = 1 << 30
	// importRatioFloor and importRatioMax: an archive that expands past importRatioFloor bytes is refused when
	// it expands by more than importRatioMax to one. A small package cannot be a bomb whatever its ratio.
	importRatioFloor = 64 << 20
	importRatioMax   = 200
	// importTextBytes caps a Markdown, text or CSV source.
	importTextBytes = 20 << 20
	// importSniffBytes is how much of the head the sniffer reads.
	importSniffBytes = 8 << 10

	// importTextLines caps a plain-text source's lines, taken before layout (P01.S03b; provisional until measured).
	importTextLines = 40000
	// importCSVRows caps a CSV's records, taken before layout (provisional until measured).
	importCSVRows = 10000
	// importCSVCells caps a CSV's cells (D14).
	importCSVCells = 1000000
	// csvMaxColumns is mdpdf's table width limit (maxTableCols), restated so a wide CSV is refused by name
	// before a table is built; a test holds them equal.
	csvMaxColumns = 24
	// csvRowsPerTable is how many body rows one table carries: mdpdf refuses a table past 5000 rows in all.
	csvRowsPerTable = 4000
)
