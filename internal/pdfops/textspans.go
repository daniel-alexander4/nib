package pdfops

import "nib/internal/contentstream"

// opSpan is the byte range of one drawing group in a content stream.
type opSpan struct{ start, end int }

// textOperatorSpans returns the span of each top-level `q … Q` group that shows text, in draw
// order — `PLAN-accessibility.md` P06.S02.
//
// # Why groups and not the text operators themselves
//
// Marked content must BRACKET the operators that draw, and a `Tj` alone is not all of them: the
// font selection, the colour and the text matrix that place it are in the same `q … Q` group and
// belong inside the same marked-content sequence. Bracketing the `Tj` alone would produce a
// document whose marked content contains a glyph-drawing operator and none of the state that
// positions it — legal, and describing the wrong extent.
//
// `mdpdf`'s output is one group per laid-out run, measured:
//
//	q BT /F0 20.00 Tf ET <cm> BT 0 Tw <colour> <Td> 0 Tr (H) Tj ET Q
//
// # Why nesting depth rather than a scan for `q`
//
// `q` and `Q` nest, and a scan that paired them by order would close the first group at the first
// inner `Q`. Depth is the only correct reading, and it costs one integer.
//
// A group containing no text-showing operator is skipped: a rule or a box draws without text and
// has nothing for a structure element to describe. **`BI` inline images are never entered**, because
// `Tokenize` returns each as one opaque token — so image bytes that happen to spell `Q` cannot close
// a group.
func textOperatorSpans(src []byte) []opSpan {
	toks := contentstream.Tokenize(src)
	var out []opSpan
	depth, start, showsText := 0, -1, false
	for _, tk := range toks {
		if tk.Kind != contentstream.Operator {
			continue
		}
		switch string(tk.Bytes(src)) {
		case "q":
			if depth == 0 {
				start, showsText = tk.Start, false
			}
			depth++
		case "Q":
			depth--
			if depth == 0 && start >= 0 {
				if showsText {
					out = append(out, opSpan{start, tk.End})
				}
				start, showsText = -1, false
			}
			if depth < 0 {
				// Unbalanced input: refuse to guess at a structure the stream does not have.
				return nil
			}
		case "Tj", "TJ", "'", "\"":
			if depth > 0 {
				showsText = true
			}
		}
	}
	return out
}
