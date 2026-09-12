package contentstream

import "fmt"

// Tokenize splits a decoded content stream into tokens covering it **completely and without
// overlap**: `tokens[0].Start == 0`, each token's `End` is the next one's `Start`, and the last
// token's `End` is `len(src)`.
//
// That total coverage is not tidiness — it is what makes the writer's job "copy every span in
// order" and therefore what makes a byte-identical round trip structural. Whitespace is a token for
// the same reason: a tokenizer that skipped it would have to invent it again on the way out, and
// inventing `\n` where the document had `\r\n` is exactly the silent corruption this package is
// built to avoid.
//
// **It does not fail on malformed input.** A content stream that ends mid-string, or carries bytes
// no grammar accepts, still tokenizes — the unterminated construct becomes one token running to the
// end. A parser that refused would be useless on the documents this has to read, which are produced
// by every PDF writer that has ever existed; and refusing buys nothing, because a caller that
// splices nothing still gets its bytes back unchanged.
func Tokenize(src []byte) []Token {
	var out []Token
	i := 0
	for i < len(src) {
		start := i
		c := src[i]

		switch {
		case isWhite(c):
			for i < len(src) && isWhite(src[i]) {
				i++
			}
			out = append(out, Token{Whitespace, start, i})

		case c == '%':
			// A comment runs to the end of line and is lexically whitespace. Kept as Whitespace so
			// callers looking for "the gap between tokens" do not have to know about it.
			for i < len(src) && src[i] != '\n' && src[i] != '\r' {
				i++
			}
			out = append(out, Token{Whitespace, start, i})

		case c == '(':
			i = scanLiteralString(src, i)
			out = append(out, Token{LiteralString, start, i})

		case c == '<':
			if i+1 < len(src) && src[i+1] == '<' {
				i += 2
				out = append(out, Token{DictOpen, start, i})
			} else {
				i = scanHexString(src, i)
				out = append(out, Token{HexString, start, i})
			}

		case c == '>':
			if i+1 < len(src) && src[i+1] == '>' {
				i += 2
				out = append(out, Token{DictClose, start, i})
			} else {
				// A stray '>' is not legal, and is emitted as a one-byte operand rather than
				// dropped: every byte must land in some token or the round trip is not total.
				i++
				out = append(out, Token{Operand, start, i})
			}

		case c == '[':
			i++
			out = append(out, Token{ArrayOpen, start, i})

		case c == ']':
			i++
			out = append(out, Token{ArrayClose, start, i})

		case c == '/':
			// A name: '/' then regular characters. `#xx` escapes are regular characters, so they
			// need no special handling to find the END of the name — and since the span is never
			// decoded, they need none at all.
			i++
			for i < len(src) && isRegular(src[i]) {
				i++
			}
			out = append(out, Token{Operand, start, i})

		case c == ')' || c == '{' || c == '}':
			// Delimiters with no meaning in a content stream. One byte, kept as an operand so the
			// coverage stays total.
			i++
			out = append(out, Token{Operand, start, i})

		default:
			// A run of regular characters: a number, a keyword, or an operator.
			for i < len(src) && isRegular(src[i]) {
				i++
			}
			if i == start {
				// Defensive: no byte consumed would loop forever. Cannot be reached — every branch
				// above consumes, and `isRegular` is true for anything that falls through here —
				// but a tokenizer that can hang on malformed input is worse than one that emits a
				// junk token, and this is one line.
				i++
			}
			word := src[start:i]
			if isNumberish(word) {
				out = append(out, Token{Operand, start, i})
				break
			}
			if string(word) == "BI" {
				end := scanInlineImage(src, start)
				out = append(out, Token{InlineImage, start, end})
				i = end
				break
			}
			out = append(out, Token{Operator, start, i})
		}
	}
	return out
}

// isNumberish reports whether a regular-character run is a number rather than a keyword.
//
// It is deliberately loose: `+`, `-`, `.` and digits in any arrangement. The distinction it serves
// is Operand-versus-Operator, and a caller that needs a real number parses the span. Being loose in
// this direction is the safe one — `true`, `null` and every operator are all made of letters, so a
// keyword can never be mistaken for a number, while a malformed number like `--3` is classified as
// an operand rather than as an operator that does not exist.
func isNumberish(word []byte) bool {
	if len(word) == 0 {
		return false
	}
	for _, c := range word {
		if (c < '0' || c > '9') && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

// scanLiteralString returns the offset just past the `)` closing the literal string at src[i] ('(').
//
// Two rules, and both are load-bearing on nib's OWN output. Parens nest, so the scan counts depth
// rather than stopping at the first `)`. A backslash escapes the next byte whatever it is, so `\)`
// does not close the string and `\\` does not escape the byte after it.
//
// **Measured, not assumed:** a one-page Markdown conversion at v1.129.45 carries 79 NUL bytes and a
// `\\` escape inside its literal strings, because P04 made every glyph a two-byte index. Any glyph
// whose index happens to be 0x28 or 0x29 puts a paren in there too. Getting this wrong corrupts the
// documents nib produces today, not a hypothetical one.
func scanLiteralString(src []byte, i int) int {
	i++ // past '('
	depth := 1
	for i < len(src) {
		switch src[i] {
		case '\\':
			// Skip the escape AND the byte it escapes, in one step, so `\\` cannot be read as an
			// escape of the following byte.
			i += 2
			continue
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return len(src) // unterminated: the rest of the stream is the token
}

// scanHexString returns the offset just past the `>` closing the hex string at src[i] ('<').
func scanHexString(src []byte, i int) int {
	i++ // past '<'
	for i < len(src) {
		if src[i] == '>' {
			return i + 1
		}
		i++
	}
	return len(src)
}

// scanInlineImage returns the offset just past the `EI` ending the inline image beginning at
// src[i] ("BI").
//
// # Why this is the only construct that needs its own scanner
//
// Every other token ends by a rule over its own bytes. An inline image does not: after `ID` comes
// raw, unstructured binary — image samples — whose length the content stream never states. It can
// be computed from `/W`, `/H`, `/BPC` and `/CS` only when the data is unfiltered; with any filter
// applied the length is whatever the filter produced, which is knowable only by decoding it.
//
// So the rule is the one ISO 32000-1 gives: find `EI` **delimited by whitespace**, and treat that as
// the end. It is a heuristic in the specification itself, not a shortcut taken here.
//
// # Why the whitespace on BOTH sides matters
//
// Image bytes are arbitrary, so the two bytes `E` and `I` appear in them regularly — roughly once
// every 65 KB by chance alone. Requiring whitespace before and a delimiter or whitespace after cuts
// the false-positive rate by orders of magnitude without ever rejecting a real terminator, because
// a real `EI` is always preceded by the whitespace after `ID`'s data and followed by whatever comes
// next in the stream.
//
// **The residual risk is real and is declared rather than hidden**: a binary payload containing
// `\n EI ` will terminate the image early, and no rule over the bytes alone can prevent it. That is
// a property of the format. What this package guarantees is narrower and still useful — it will not
// SILENTLY reinterpret the binary as operators, because everything from `BI` to whatever it takes to
// be `EI` is one opaque token that the writer copies verbatim.
func scanInlineImage(src []byte, i int) int {
	// Find the `ID` that starts the binary payload. Before it the image's dictionary is ordinary
	// tokens, so a plain scan for the keyword is safe only if it is a keyword — hence the
	// delimiter checks on each side.
	j := i + 2
	for j+1 < len(src) {
		if src[j] == 'I' && src[j+1] == 'D' && startsAKeyword(src, j) &&
			(j+2 >= len(src) || !isRegular(src[j+2])) {
			j += 2
			break
		}
		j++
	}
	if j+1 >= len(src) {
		return len(src) // no ID: malformed, swallow the rest rather than mis-parse it
	}
	// Exactly ONE whitespace byte follows `ID` and belongs to the delimiter, not the data
	// (ISO 32000-1 8.9.7). Skipping more would eat image samples.
	if j < len(src) && isWhite(src[j]) {
		j++
	}
	for j+1 < len(src) {
		if src[j] == 'E' && src[j+1] == 'I' && j > 0 && isWhite(src[j-1]) &&
			(j+2 >= len(src) || !isRegular(src[j+2])) {
			return j + 2
		}
		j++
	}
	return len(src)
}

// startsAKeyword reports whether position j begins a bare keyword rather than continuing something
// else — specifically, that it is not the body of a NAME.
//
// **This is the `/ID` case.** The looser test — "the previous byte is not a regular character" —
// accepts `/ID`, because `/` is a delimiter; an inline-image dictionary key spelled that way would
// be read as the marker that starts the binary payload. Requiring whitespace instead would be wrong
// the other way: `/D[1 0]ID` is legal, with `]` and no space before the keyword. So the rule is what
// the grammar says: a keyword starts after whitespace, or after a delimiter that is not the one
// which introduces a name.
//
// **What this is honestly worth, measured rather than claimed.** Reverting it to the looser check
// leaves every test in this package green, and that is not a gap in the tests — it is the truth.
// The image token is OPAQUE, so believing the payload starts earlier changes the token's span only
// if a whitespace-delimited `EI` lies between the false marker and the real one, and inside a
// well-formed image dictionary of names and numbers there cannot be one. The case where it differs
// is driven in `TestAKeywordIsNotAName`, on input that is **not legal PDF** — which is where this
// rule earns its place, since a tokenizer's job includes not making malformed input worse.
func startsAKeyword(src []byte, j int) bool {
	if j == 0 {
		return true
	}
	p := src[j-1]
	return (isWhite(p) || isDelim(p)) && p != '/'
}

// Describe renders a token for a failure message: kind and a bounded excerpt.
//
// Named `Describe` rather than `String` deliberately: a method called `String` that takes an
// argument is not a `fmt.Stringer`, so `%v` on a Token would silently print the struct instead —
// a message that looks like it was formatted and was not.
func (t Token) Describe(src []byte) string {
	b := t.Bytes(src)
	if len(b) > 40 {
		return fmt.Sprintf("%s[%d:%d] %q…", t.Kind, t.Start, t.End, b[:40])
	}
	return fmt.Sprintf("%s[%d:%d] %q", t.Kind, t.Start, t.End, b)
}
