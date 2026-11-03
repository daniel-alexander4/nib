package pdfops

import (
	"regexp"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Style is read once per font object (`loadRunFont`) and handed to the page map as `MapText.Bold` / `Italic`
// (ADR-088 extended additively; PLAN-import-export D18, P02.S03a). The order is the plan's (Y7): the font
// DESCRIPTOR first (`/FontWeight` >= 700 or ForceBold; `/ItalicAngle` != 0 or the Italic flag), then the BASE-FONT
// name's tokens with a subset tag (`ABCDEF+`) stripped, and no signal means regular. A Type 0 font reads its
// descendant's descriptor. Text drawn with `Tr 2` and a stroke is bold regardless — how a producer fakes weight.
//
// A descriptor that SAYS regular (`/FontWeight 400`, no flag) is a signal and the name is not consulted, so a
// font named "Bold" with a weight of 400 is regular. The detection readers ignore both fields.

const (
	fontFlagItalic    = 1 << 6  // PDF 32000-1 Table 123, bit 7
	fontFlagForceBold = 1 << 18 // bit 19
	boldWeight        = 700
)

var styleSubsetTag = regexp.MustCompile(`^[A-Z]{6}\+`)

// fontStyle is whether the font draws bold and italic, from its dictionary alone.
func fontStyle(xt *model.XRefTable, d types.Dict) (bold, italic bool) {
	desc := d
	if nameVal(d, "Subtype") == "Type0" {
		if kids, err := xt.DereferenceArray(d["DescendantFonts"]); err == nil && len(kids) == 1 {
			if kid := derefDict(xt, kids[0]); kid != nil {
				desc = kid
			}
		}
	}
	fd := derefDict(xt, desc["FontDescriptor"])
	var boldKnown, italicKnown bool
	if fd != nil {
		if w, ok := pdfNumber(xt, fd["FontWeight"]); ok && w > 0 {
			bold, boldKnown = w >= boldWeight, true
		}
		if flags, ok := fd["Flags"].(types.Integer); ok {
			if flags.Value()&fontFlagForceBold != 0 {
				bold, boldKnown = true, true
			}
			if flags.Value()&fontFlagItalic != 0 {
				italic, italicKnown = true, true
			}
		}
		if a, ok := pdfNumber(xt, fd["ItalicAngle"]); ok && a != 0 {
			italic, italicKnown = true, true
		}
	}
	if boldKnown && italicKnown {
		return bold, italic
	}
	name := nameVal(desc, "BaseFont")
	if name == "" {
		name = nameVal(d, "BaseFont")
	}
	nb, ni := styleFromName(name)
	// A descriptor that spoke to one axis does not silence the name on the other.
	if !boldKnown {
		bold = nb
	}
	if !italicKnown {
		italic = ni
	}
	return bold, italic
}

// styleFromName reads the tokens a base-font name carries: "Arial-BoldMT", "TimesNewRoman,BoldItalic",
// "ABCDEF+Calibri-Italic". Semibold is not bold (600 is below the descriptor's own line).
func styleFromName(name string) (bold, italic bool) {
	name = styleSubsetTag.ReplaceAllString(name, "")
	l := strings.ToLower(name)
	if strings.Contains(l, "semibold") || strings.Contains(l, "demibold") {
		l = strings.NewReplacer("semibold", "", "demibold", "").Replace(l)
	}
	bold = strings.Contains(l, "bold") || strings.Contains(l, "black") || strings.Contains(l, "heavy")
	italic = strings.Contains(l, "italic") || strings.Contains(l, "oblique")
	return bold, italic
}
