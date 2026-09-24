// Package fontcode reads what a text-showing operator shows: the bytes of a string operand, the character
// codes a font's codespace cuts them into, and the Unicode text a `/ToUnicode` CMap gives each code.
//
// # Why it is one package (ADR-009)
//
// Two readers ask these questions. `internal/pdfops` reads text runs to place, measure and tag them, and
// `internal/uacheck` reads glyphs to judge PDF/UA-1 7.21.7 — which is a question about EXACTLY the codes a
// string draws and exactly what each maps to. They used to be one reader and a checker that did not look,
// and a rule answered in two places drifts: so the decoding lives here, once, and both call it.
//
// # What it follows
//
// The code reading and the checker's CMap lookup follow veraPDF 1.30.2 (veraPDF-parser `CMap.getCodeFromStream`,
// `CMapParser`, `CodeSpace`, `ToUnicodeInterval`), Java `int` arithmetic included, because the checker's answers
// are measured against veraPDF and a reader that cut a string differently would judge different glyphs. Where a
// shape makes veraPDF's own reader throw, this one says it could not read the shape rather than guess. Where that differs from what a lenient
// text extractor would do — a trailing odd byte, a code no range admits — the difference is veraPDF's and
// is written down at the site.
//
// # What it does not do
//
// It resolves nothing: callers hand it bytes (a decoded stream, a string token's span) and it never sees
// a pdfcpu object. It carries no predefined CMap and no Unicode table — a caller that needs one says so.
package fontcode

import "nib/internal/contentstream"

// String returns a literal or hex string token's bytes, delimiters and escapes removed.
//
// `contentstream` deliberately hands out spans, not values — *"a caller that does need one decodes the
// span itself"* — so this is the readers' one decoder.
func String(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '<' {
		return Hex(raw)
	}
	if raw[0] != '(' {
		return nil
	}
	body := raw[1:]
	if n := len(body); n > 0 && body[n-1] == ')' {
		body = body[:n-1]
	}
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\r' {
			// An unescaped end-of-line in a literal is read as a single newline, whatever its spelling.
			out = append(out, '\n')
			if i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
			continue
		}
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(body) {
			break
		}
		switch e := body[i]; e {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case '(', ')', '\\':
			out = append(out, e)
		case '\r':
			// A backslash at the end of a line continues the string; neither byte is content.
			if i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
		case '\n':
		default:
			if e >= '0' && e <= '7' {
				v := int(e - '0')
				for n := 1; n < 3 && i+1 < len(body) && body[i+1] >= '0' && body[i+1] <= '7'; n++ {
					i++
					v = v*8 + int(body[i]-'0')
				}
				out = append(out, byte(v))
				continue
			}
			// An unknown escape: the specification says the backslash is ignored.
			out = append(out, e)
		}
	}
	return out
}

// Hex returns a hex string token's bytes. Whitespace and any other non-digit are skipped, and a missing
// final digit is zero.
func Hex(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	body := raw[1:]
	if n := len(body); n > 0 && body[n-1] == '>' {
		body = body[:n-1]
	}
	nibbles := make([]byte, 0, len(body))
	for _, c := range body {
		if v, ok := hexNibble(c); ok {
			nibbles = append(nibbles, v)
		}
	}
	if len(nibbles)%2 == 1 {
		nibbles = append(nibbles, 0)
	}
	out := make([]byte, len(nibbles)/2)
	for i := range out {
		out[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return out
}

func hexNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// Name decodes a name's `#xx` escapes; b is the name without its slash.
func Name(b []byte) string {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == '#' && i+2 < len(b) {
			hi, hok := hexNibble(b[i+1])
			lo, lok := hexNibble(b[i+2])
			if hok && lok {
				out = append(out, hi<<4|lo)
				i += 2
				continue
			}
		}
		out = append(out, b[i])
	}
	return string(out)
}

// MatchingClose returns the index of the token closing the one opened at i, or len(toks) when nothing
// closes it.
//
// **It returned len(toks)-1 until P08.S04, and that panicked** when the opener was itself the last
// token: `toks[i+1:len-1]` is a reversed slice.
func MatchingClose(toks []contentstream.Token, i int, open, close contentstream.Kind) int {
	depth := 0
	for j := i; j < len(toks); j++ {
		switch toks[j].Kind {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(toks)
}

// Value is a code's big-endian integer value, for a reader that indexes by it (`pdfops`' widths). The checker
// looks a code up by veraPDF's Java `int` instead (`javaInt`), which differs past 0x7FFFFFFF.
func Value(code []byte) int { return int(numberFromBytes(code)) }
