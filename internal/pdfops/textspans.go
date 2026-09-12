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

// artifactSpan is one `/Artifact <<…>> BDC` marker: the byte range of the operand-plus-operator that
// opens the sequence, so a caller can REPLACE the marker while leaving what it brackets alone.
type artifactSpan struct {
	start, end int // [start, end) covers `/Artifact <<…>> BDC`
}

// watermarkArtifactSpans returns, in draw order, the marker of every marked-content sequence opened
// as a pagination watermark artifact — `PLAN-accessibility.md` P06.S06.
//
// # Why this exists, and why it is not a byte search
//
// `StampTextLayer` draws each OCR'd word through `api.TextWatermark`, and pdfcpu brackets every
// watermark it stamps:
//
//	/Artifact <</Subtype /Watermark /Type /Pagination >>BDC q <cm> /GS0 gs /Fm0 Do Q EMC
//
// An artifact is, by definition, content a conforming reader is to skip. So nib's searchable text
// layer — the only thing that makes a scan readable — is explicitly declared not to be read, and
// the page satisfies ua1 7.1 t3 for it by DISCLAIMING the content rather than describing it.
// Tagging an OCR layer is therefore a replacement of that marker, not an insertion around bare
// content.
//
// A `bytes.Index` for "/Artifact" would find the same text inside a string operand or an inline
// image's bytes. Tokenizing cannot: `Tokenize` returns a string as one token and an inline image as
// one opaque token, so only a real operand is ever considered.
//
// # Why it matches the SUBTYPE and not every artifact
//
// A document may already carry legitimate artifacts — a running header, a page number — that nib did
// not put there and must not re-describe as content. The watermark subtype is what nib's own stamp
// writes, so it is what nib may claim.
func watermarkArtifactSpans(src []byte) []artifactSpan {
	toks := contentstream.Tokenize(src)
	var out []artifactSpan

	// The marker is `/Artifact` then a property dictionary then `BDC`. `<<` and `>>` are tokens in
	// their own right in this package — nesting is the caller's business — so the dictionary is
	// walked with a depth counter rather than read as one token.
	const (
		idle   = iota // nothing seen
		named         // `/Artifact` seen, dictionary not yet open
		inDict        // inside the property dictionary
		closed        // dictionary closed, expecting BDC
	)
	state, start, depth, isWatermark := idle, 0, 0, false
	reset := func() { state, depth, isWatermark = idle, 0, false }

	for _, tk := range toks {
		if tk.Kind == contentstream.Whitespace {
			continue
		}
		switch state {
		case idle:
			if tk.Kind == contentstream.Operand && string(tk.Bytes(src)) == "/Artifact" {
				state, start, isWatermark = named, tk.Start, false
			}
		case named:
			if tk.Kind == contentstream.DictOpen {
				state, depth = inDict, 1
			} else {
				reset()
			}
		case inDict:
			switch tk.Kind {
			case contentstream.DictOpen:
				depth++
			case contentstream.DictClose:
				if depth--; depth == 0 {
					state = closed
				}
			case contentstream.Operand:
				// The subtype nib's own stamp writes. A document may already carry legitimate
				// artifacts — a running header, a page number — that nib did not put there and must
				// not re-describe as content.
				if string(tk.Bytes(src)) == "/Watermark" {
					isWatermark = true
				}
			}
		case closed:
			if tk.Kind == contentstream.Operator && string(tk.Bytes(src)) == "BDC" && isWatermark {
				out = append(out, artifactSpan{start: start, end: tk.End})
			}
			reset()
		}
	}
	return out
}
