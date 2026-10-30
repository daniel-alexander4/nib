package pdfops

import (
	"strconv"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// textOf is a page's printed text joined, in the order the map gives it.
func textOf(t *testing.T, pdf []byte) (string, PageMap) {
	t.Helper()
	m, err := MapPage(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, r := range m.Text {
		sb.WriteString(r.Text)
	}
	return sb.String(), m
}

// diffFont is a simple Type 1 font (not one pdfcpu has metrics for, with an advance for every ASCII code) whose
// /Encoding is the dictionary given and whose descriptor carries flags.
func diffFont(enc string, flags int) (string, map[int]string) {
	return "<< /Type /Font /Subtype /Type1 /BaseFont /ABCDEF+NotACoreFont /Encoding " + enc + " /FontDescriptor 6 0 R" + widths95 + " >>",
		map[int]string{6: "<< /Type /FontDescriptor /FontName /ABCDEF+NotACoreFont /Flags " + strconv.Itoa(flags) + " /FontBBox [0 0 600 800] /ItalicAngle 0 /Ascent 629 /Descent -157 /CapHeight 562 /StemV 50 >>"}
}

// TestADifferencesFontIsReadThroughTheGlyphList — /pending 918, ADR-176. i1040gi.pdf p.125 draws its text in simple Type 1C
// fonts with `/Encoding << /BaseEncoding /WinAnsiEncoding /Differences [31 /fi] >>` and no /ToUnicode, and the map held two
// runs of its 2,843 letters. The same shape, made here: a base, a code the list names as a ligature, a renamed letter, a
// name built from parts, and one the list does not carry.
func TestADifferencesFontIsReadThroughTheGlyphList(t *testing.T) {
	dict, extra := diffFont("<< /Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [31 /fi 66 /A 67 /uni0042 68 /f_f_i 69 /g123] >>", 32)
	got, m := textOf(t, coreInkPage(dict, "(\\037rst BCDE x)", mergeObjs(extra)))
	// 0x1F is the fi ligature, read as its letters; B→A, C→uniXXXX "B", D→"ffi", E names nothing readable; 'r','s','t',' ','x' by the base.
	if want := "first AB" + "ffi" + " x"; got != want {
		t.Errorf("the page reads %q, want %q", got, want)
	}
	if m.Unread != 1 {
		t.Errorf("Unread = %d, want 1 (the one code whose name the list does not carry)", m.Unread)
	}
}

// TestASymbolicFontsBaseIsNotGuessed — a font not declared nonsymbolic has, outside its /Differences, the font
// program's own encoding, which this reader does not read: those codes stay undecoded and are counted, not guessed.
func TestASymbolicFontsBaseIsNotGuessed(t *testing.T) {
	enc := "<< /Type /Encoding /Differences [97 /b] >>"
	for _, c := range []struct {
		name   string
		flags  int
		want   string
		unread int
	}{
		{"declared symbolic", 4, "b", 2},
		{"declared both, which is not nonsymbolic", 36, "b", 2},
		{"declared nonsymbolic", 32, "bxy", 0},
	} {
		dict, extra := diffFont(enc, c.flags)
		got, m := textOf(t, coreInkPage(dict, "(axy)", mergeObjs(extra)))
		if got != c.want || m.Unread != c.unread {
			t.Errorf("%s: reads %q with %d unread, want %q with %d", c.name, got, m.Unread, c.want, c.unread)
		}
	}
}

// TestAnEncodingReferenceToANameIsTheSameFont — `/Encoding 7 0 R` where object 7 is `/WinAnsiEncoding` was read as no
// encoding at all, since only a direct name was taken for one.
func TestAnEncodingReferenceToANameIsTheSameFont(t *testing.T) {
	dict, extra := diffFont("7 0 R", 32)
	extra[7] = "/WinAnsiEncoding"
	got, m := textOf(t, coreInkPage(dict, "(Hi)", mergeObjs(extra)))
	if got != "Hi" || m.Unread != 0 {
		t.Errorf("reads %q with %d unread, want \"Hi\" with 0", got, m.Unread)
	}
}

// TestDifferencesCodesOutsideAByteAreIgnored — a document writes the code numbers. -5 and 300 must not wrap to 251 and 44
// (the bytes a conversion would give them), and a long array is read no further than a font can use.
func TestDifferencesCodesOutsideAByteAreIgnored(t *testing.T) {
	items := types.Array{types.Integer(-5), types.Name("A"), types.Integer(300), types.Name("B")}
	for i := 0; i < 5000; i++ {
		items = append(items, types.Name("a"))
	}
	f := types.Dict{"Subtype": types.Name("Type1"), "Encoding": types.Dict{"Differences": items}}
	diffs, _ := encodingDictionaryOf(widthXRef(t), f)
	if _, wrapped := diffs[251]; wrapped {
		t.Error("code -5 was read as byte 251")
	}
	if got, wrapped := diffs[44]; wrapped && got == "B" {
		t.Error("code 300 was read as byte 44")
	}
	if len(diffs) > 256 {
		t.Errorf("%d codes from one font", len(diffs))
	}
}

func mergeObjs(m map[int]string) map[int]string { return m }
