package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// inlineToUnicodeFont is a DIRECT font dictionary — no object number to key a cache by — whose
// /ToUnicode carries `ranges` one-code bfranges mapping code c to U+4E00+c.
func inlineToUnicodeFont(baseFont string, ranges int) types.Dict {
	var cm strings.Builder
	fmt.Fprintf(&cm, "/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n"+
		"1 begincodespacerange <00> <FF> endcodespacerange\n%d beginbfrange\n", ranges)
	for c := 0; c < ranges; c++ {
		fmt.Fprintf(&cm, "<%02X> <%02X> <%04X>\n", c, c, 0x4E00+c)
	}
	cm.WriteString("endbfrange\nendcmap CMapName currentdict /CMap defineresource pop end end\n")
	body := []byte(cm.String())
	return types.Dict{"Type": types.Name("Font"), "Subtype": types.Name("Type1"),
		"BaseFont": types.Name(baseFont), "Encoding": types.Name("WinAnsiEncoding"),
		"ToUnicode": types.StreamDict{Dict: types.Dict{"Length": types.Integer(len(body))}, Raw: body, Content: body}}
}

// TestAnInlineFontIsLoadedOncePerWalk — `/pending 723`. `fontFor` cached only indirect fonts, so a
// direct font dictionary was reloaded — its /ToUnicode re-parsed, its /W re-read — at every `Tf`:
// 1,600 × `/F1 1 Tf` over a 100-range CMap took 17.7 s, and nothing bounds the `Tf` count.
func TestAnInlineFontIsLoadedOncePerWalk(t *testing.T) {
	res := types.Dict{"Font": types.Dict{"F1": inlineToUnicodeFont("Inline", 100)}}
	var content strings.Builder
	content.WriteString("BT ")
	for i := 0; i < 1600; i++ {
		content.WriteString("/F1 1 Tf ")
	}
	content.WriteString("(A) Tj ET")

	w := newRunWalker(widthXRef(t))
	w.walk([]byte(content.String()), res, newRunGState(), 0, map[int]bool{})
	if w.fontLoads != 1 {
		t.Errorf("1,600 Tf selecting one direct font loaded it %d times, want 1: each load re-parses its "+
			"/ToUnicode, so the cost is Tf count × CMap size with nothing bounding the first", w.fontLoads)
	}
	// STIMULUS: the font that was loaded once is the one whose CMap was read, or the count proves nothing.
	if len(w.runs) != 1 || w.runs[0].text != "乁" {
		t.Fatalf("the run reads %+v, want one run of U+4E41 from the inline /ToUnicode", w.runs)
	}
}

// TestTheFontCacheKeepsScopesApart — the cache must not key by resource NAME: a form's own /Resources
// may bind /F1 to a different font than the page does, and both walks share one walker.
func TestTheFontCacheKeepsScopesApart(t *testing.T) {
	w := newRunWalker(widthXRef(t))
	for _, bf := range []string{"PageFont", "FormFont"} {
		res := types.Dict{"Font": types.Dict{"F1": inlineToUnicodeFont(bf, 100)}}
		w.walk([]byte("BT /F1 1 Tf (A) Tj ET"), res, newRunGState(), 0, map[int]bool{})
	}
	if len(w.runs) != 2 || w.runs[0].baseFont != "PageFont" || w.runs[1].baseFont != "FormFont" {
		var got []string
		for _, r := range w.runs {
			got = append(got, r.baseFont)
		}
		t.Errorf("two scopes binding /F1 to different fonts read as %v, want [PageFont FormFont]", got)
	}
}
