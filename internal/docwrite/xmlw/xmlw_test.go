package xmlw

import (
	"testing"

	"nib/internal/docmodel"
)

func TestEscIsTheOneDoorForXML(t *testing.T) {
	if got, want := Esc("a<b>&\"'\x00\x08z"), "a&lt;b&gt;&amp;&quot;&apos;z"; got != want {
		t.Errorf("Esc = %q, want %q", got, want)
	}
}

func TestGridPlacesCoveredPositions(t *testing.T) {
	c := func(cs, rs int) docmodel.Cell { return docmodel.Cell{ColSpan: cs, RowSpan: rs} }
	g := Grid([][]docmodel.Cell{{c(1, 3), c(2, 1)}, {c(2, 1)}, {c(1, 1), c(1, 1)}})
	// row 1: [restart(1,3)] [c(2)]; row 2: [covered] [c(2)]; row 3: [covered] [c] [c]
	if len(g[1]) != 2 || !g[1][0].Covered || g[1][1].Covered {
		t.Fatalf("row 2 = %+v", g[1])
	}
	if len(g[2]) != 3 || !g[2][0].Covered {
		t.Fatalf("row 3 = %+v", g[2])
	}
	if !g[0][0].Restart || g[0][1].Restart {
		t.Errorf("restart flags wrong: %+v", g[0])
	}
}

func TestLinksAndLanguage(t *testing.T) {
	for u, ok := range map[string]bool{"https://a/b": true, "mailto:x@y": true, "javascript:alert(1)": false, "file:///etc/passwd": false, "%": false} {
		if SafeLink(u) != ok {
			t.Errorf("SafeLink(%q) = %v", u, !ok)
		}
	}
	if LangTag(`en"><x`) != "und" || LangTag("pt-BR") != "pt-BR" || LangTag("") != "und" {
		t.Error("LangTag")
	}
}

func TestGridHoldsARowSpanThroughAShortRowAndCapsSpans(t *testing.T) {
	c := func(cs, rs int) docmodel.Cell { return docmodel.Cell{ColSpan: cs, RowSpan: rs} }
	g := Grid([][]docmodel.Cell{{c(1, 1), c(1, 1), c(1, 2)}, {c(1, 1)}})
	// the second row ends before column 3: it is padded so the covered position sits at column 3
	if len(g[1]) != 3 || g[1][0].Covered || g[1][1].Covered || !g[1][2].Covered {
		t.Errorf("row 2 = %+v", g[1])
	}
	h := Grid([][]docmodel.Cell{{c(2000000000, 2000000000)}})
	if h[0][0].ColSpan != MaxSpan || h[0][0].RowSpan != MaxSpan {
		t.Errorf("spans not capped: %+v", h[0][0])
	}
}
