package pdfops

import (
	"strings"

	"nib/internal/fontcode"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A simple font whose `/Encoding` is a dictionary — a base encoding with `/Differences` over it — says what its codes
// are by glyph NAME, and Adobe's glyph list says what a name is (ADR-176, /pending 918). Until then this reader gave
// such a font no text at all: 1,134,622 of the 10.5M glyphs of the 2025 IRS sources, every statute in them, and 11,475
// letters and digits of `i1040gi.pdf`, dropped without an error (`TestADifferencesFontIsReadThroughTheGlyphList`).

// maxDifferencesItems bounds the entries read from one `/Differences` array: a document sizes it. A font has 256 codes,
// so the legitimate array is at most 256 names and as many code numbers.
const maxDifferencesItems = 1024

// ligatureLetters reads the Latin presentation ligatures as the letters they stand for. The glyph list names `fi` U+FB01;
// the text a reader wants — to search, to group — is "fi", and it is what poppler and pdf.js give.
var ligatureLetters = strings.NewReplacer("ﬀ", "ff", "ﬁ", "fi", "ﬂ", "fl", "ﬃ", "ffi", "ﬄ", "ffl", "ﬅ", "st", "ﬆ", "st")

// encodingDictionaryOf reads the `/Encoding` of a simple font that `simpleDecoderFor` could not (it is a dictionary, or a
// reference to a name): the text of each `/Differences` name the glyph list carries, and the base a code outside them
// falls to. A name it cannot read is kept as "": its code stays undecoded, and is not read through the base.
//
// The base is `/BaseEncoding` where it names a table nib reads. Where there is none, the specification's base is
// StandardEncoding for a nonsymbolic font and the font program's own encoding for a symbolic one — which is not read
// here, so a font not declared nonsymbolic decodes only the codes it names. Both nil: the font draws nothing readable.
func encodingDictionaryOf(xt *model.XRefTable, d types.Dict) (map[byte]string, func(byte) (rune, bool)) {
	o, err := xt.Dereference(d["Encoding"])
	if err != nil {
		return nil, nil
	}
	if n, ok := o.(types.Name); ok { // `/Encoding 7 0 R` where 7 is a name: the same font as a direct one
		return nil, simpleDecoderFor(types.Dict{"Encoding": n})
	}
	enc, ok := o.(types.Dict)
	if !ok {
		return nil, nil
	}
	var base func(byte) (rune, bool)
	if bn := enc.NameEntry("BaseEncoding"); bn != nil {
		base = simpleDecoderFor(types.Dict{"Encoding": types.Name(*bn)})
	} else if nonsymbolic(xt, d) {
		base = decodeStandardPrintable
	}
	diffs := map[byte]string{}
	arr, _ := xt.DereferenceArray(enc["Differences"])
	if len(arr) > maxDifferencesItems {
		arr = arr[:maxDifferencesItems]
	}
	code := 0
	for _, e := range arr {
		e, _ = xt.Dereference(e)
		switch v := e.(type) {
		case types.Integer:
			code = v.Value()
		case types.Name:
			if code >= 0 && code <= 0xFF {
				// A code the document renames is the glyph it names: where the list cannot read that name the code is
				// recorded as unreadable (""), never left to fall to the base — the base's character is not what is drawn.
				s, _ := fontcode.GlyphText(v.Value())
				diffs[byte(code)] = ligatureLetters.Replace(s)
			}
			code++
		}
	}
	return diffs, base
}

// nonsymbolic is the font descriptor's own statement that the font is a Latin text font: `/Flags` bit 6 (Nonsymbolic)
// set and bit 3 (Symbolic) clear. No descriptor, no flags or both set is "not declared".
func nonsymbolic(xt *model.XRefTable, d types.Dict) bool {
	desc, err := xt.DereferenceDict(d["FontDescriptor"])
	if err != nil || desc == nil {
		return false
	}
	fo, _ := xt.Dereference(desc["Flags"])
	fl, ok := fo.(types.Integer)
	return ok && fl.Value()&32 != 0 && fl.Value()&4 == 0
}
