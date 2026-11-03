package read

import (
	"os"
	"strings"
	"testing"
)

// The page loop in Read is the one place tables meet the bound; this holds that the call is there and runs before the
// page's blocks join the document (a source scan: a hostile tagged PDF is not a cheap fixture).
func TestReadBoundsEveryPagesTablesBeforeTheyJoinTheDocument(t *testing.T) {
	src, err := os.ReadFile("read.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	call := strings.Index(s, "blocks = boundTables(blocks, rep)")
	join := strings.Index(s, "doc.Blocks = append(doc.Blocks, blocks...)")
	if call < 0 || join < 0 || call > join {
		t.Errorf("boundTables call at %d, the join at %d: tables reach the document unbounded", call, join)
	}
}
