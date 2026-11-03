package read

import (
	"testing"

	"nib/internal/docmodel"
)

func TestBoundTablesCountsEachTableItWritesAsTextAndLeavesOthersAlone(t *testing.T) {
	var wide []docmodel.Cell
	for i := 0; i < docmodel.MaxTableColumns+1; i++ {
		wide = append(wide, docmodel.Cell{ColSpan: 1, RowSpan: 1, Paragraphs: [][]docmodel.Span{{{Text: "w"}}}})
	}
	rep := &docmodel.Report{}
	para := docmodel.Block{Kind: docmodel.Paragraph, Spans: []docmodel.Span{{Text: "p"}}}
	out := boundTables([]docmodel.Block{para, {Kind: docmodel.Table, Rows: [][]docmodel.Cell{wide}}, {Kind: docmodel.Table, Rows: [][]docmodel.Cell{wide[:2]}}}, rep)
	if rep.TablesAsText != 1 || len(out) != 3 || out[1].Kind != docmodel.Paragraph || out[2].Kind != docmodel.Table {
		t.Errorf("TablesAsText=%d, %d blocks", rep.TablesAsText, len(out))
	}
}
