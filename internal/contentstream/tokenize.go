package contentstream

import (
	"bytes"
	"fmt"
)

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
// # Two exact answers come first, and neither is believed until the bytes agree
//
// Where the payload states its own end — an ASCII filter's `>` or `~>` — and where the dictionary states
// its length — PDF 2.0's `/L` — the scan need not guess at all. Both are tried before the rule below, and
// both are CORROBORATED: each says where the image ends, and is taken only if a real `EI` stands there
// (`imageEndsAt`). Anything else and the whitespace rule decides, exactly as it did before either existed.
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
	idAt := -1
	for j+1 < len(src) {
		if src[j] == 'I' && src[j+1] == 'D' && startsAKeyword(src, j) &&
			(j+2 >= len(src) || !isRegular(src[j+2])) {
			idAt = j
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
	if end, ok := asciiEncodedImageEnd(src, i+2, idAt, j); ok {
		return end
	}
	if end, ok := declaredLengthImageEnd(src, i+2, idAt, j); ok {
		return end
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

// asciiEncodedImageEnd finds the end of an inline image whose OUTERMOST filter is an ASCII encoding, where
// the payload states its own end — `>` for ASCIIHexDecode, `~>` for ASCII85Decode — and `EI` may follow
// it with no whitespace between (`/pending 503`: `ID 00>EI Q BT … Tj ET` swallowed everything after the
// image, so that text vanished from every reader built on this package).
//
// **It is exact, not a heuristic**, which is why it runs before the whitespace rule rather than instead of
// it: an ASCIIHex payload holds only hex digits and whitespace, and an ASCII85 payload holds `!`–`u`, `z`
// and whitespace, so the first `>` or `~>` after `ID` IS the end-of-data marker and nothing in the data
// can imitate it. Any other filter, or none, leaves ok false and the caller's whitespace rule decides.
//
// dictFrom and idAt bound the image dictionary; data is the first payload byte.
func asciiEncodedImageEnd(src []byte, dictFrom, idAt, data int) (int, bool) {
	if idAt < dictFrom {
		return 0, false
	}
	dict := src[dictFrom:idAt]
	filter, ok := inlineDictValue(dict, "/F", "/Filter")
	if !ok {
		return 0, false // no filter key, or a value shape this cannot read
	}
	var eod []byte
	switch string(filter.Bytes(dict)) {
	case "/AHx", "/ASCIIHexDecode":
		eod = []byte(">")
	case "/A85", "/ASCII85Decode":
		eod = []byte("~>")
	default:
		return 0, false
	}
	at := bytes.Index(src[data:], eod)
	if at < 0 {
		return 0, false
	}
	return imageEndsAt(src, data+at+len(eod))
}

// declaredLengthImageEnd ends an inline image where its own dictionary says the data stops: `/L`, or
// `/Length` written out — the PDF 2.0 key that exists precisely so a reader need not guess. `/pending 516`.
//
// **The declared length is a CANDIDATE, and the bytes remain the authority.** It is taken only when a real
// `EI` stands where it says the data ends, and otherwise this returns false and the caller's whitespace
// rule decides exactly as it did before the key existed. The asymmetry is the whole design. A `/L` believed
// on sight can end the token in the MIDDLE of the binary, and every byte after it would then be read as
// operators — the precise corruption this scanner exists to prevent — while a `/L` that is merely
// disbelieved costs nothing but the scan that was going to happen anyway. A new key may not introduce a
// failure the old rule did not have; it may only remove one, which here is a payload containing
// `\n EI ` and ending the image early.
//
// **What could NOT be established, declared rather than guessed.** ISO 32000-2 adds `/L` to the inline
// image dictionary and nothing in this tree states which bytes it counts: pdfcpu v0.13.0 does not
// implement the key at all — `model/parseContent.go`'s `lookupEI` scans for `EI` exactly as the rule below
// does — and no copy of the specification is in the repo. The reading taken is the one that makes the key
// useful: the length of the image DATA, counted from the first payload byte, which is the byte after the
// single whitespace following `ID`. If that reading is wrong the corroboration simply fails and nothing
// changes, and that is what made it safe to take a reading at all.
//
// dictFrom and idAt bound the image dictionary; data is the first payload byte.
func declaredLengthImageEnd(src []byte, dictFrom, idAt, data int) (int, bool) {
	if idAt < dictFrom {
		return 0, false
	}
	dict := src[dictFrom:idAt]
	v, ok := inlineDictValue(dict, "/L", "/Length")
	if !ok {
		return 0, false
	}
	n, ok := declaredLength(v.Bytes(dict))
	if !ok || n > len(src)-data {
		return 0, false
	}
	return imageEndsAt(src, data+n)
}

// declaredLength reads a `/L` value: a plain non-negative integer and nothing else. `+8` and `8.0` are
// legal PDF numbers and are not byte counts this acts on — declining costs only the fallback — and the
// length bound keeps the accumulation inside an int on every platform.
func declaredLength(b []byte) (int, bool) {
	if len(b) == 0 || len(b) > 9 {
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// imageEndsAt reports the end of an inline image whose data is believed to stop at `after`: whitespace may
// stand between the data and `EI`, and `EI` must be a keyword rather than the first two bytes of a longer
// one.
//
// **This is the corroboration both exact rules rest on** (ADR-009: one door, called by each). Neither an
// end-of-data marker nor a declared length is believed on its own — each proposes an end, and an end with
// no `EI` at it is not an end.
func imageEndsAt(src []byte, after int) (int, bool) {
	m := after
	for m < len(src) && isWhite(src[m]) {
		m++
	}
	if m+1 < len(src) && src[m] == 'E' && src[m+1] == 'I' && (m+2 >= len(src) || !isRegular(src[m+2])) {
		return m + 2, true
	}
	return 0, false
}

// inlineDictValue returns the value token of an inline image dictionary's `abbrev` or `full` key — the one
// door both exact end-of-image rules ask their question through (ADR-009), because two walks of the same
// dictionary are two answers to "what does this dictionary say".
//
// It walks the dictionary as the key/value pairs it is rather than scanning for the name, so a value that
// happens to SPELL the key is not mistaken for one: `/CS /L` names a colourspace resource, not a length.
// An array value yields its first element, which is what the filter rule wants — the outermost filter of
// `/Filter [/A85 /Fl]` — and a shape no length is written in.
//
// **A name is matched as it is SPELLED.** `#` escapes are legal in a name and are not decoded here, for the
// reason the package gives everywhere else: a span is never decoded. `/#4C` is `/L` to a conforming reader
// and is not one to this, which declines and falls back — the safe direction, and the residual is declared
// rather than hidden.
//
// The dictionary is tokenized rather than scanned by hand because this package already knows the grammar
// it is written in, and an image dictionary is a handful of tokens.
func inlineDictValue(dict []byte, abbrev, full string) (Token, bool) {
	toks := Tokenize(dict)
	for k := nextMeaningful(toks, 0); k < len(toks); {
		b := toks[k].Bytes(dict)
		if toks[k].Kind != Operand || len(b) == 0 || b[0] != '/' {
			return Token{}, false // not a key where a key must be: not a dictionary this can read
		}
		v := nextMeaningful(toks, k+1)
		if v >= len(toks) {
			return Token{}, false // a key with no value
		}
		if key := string(b); key == abbrev || key == full {
			if toks[v].Kind == ArrayOpen {
				if v = nextMeaningful(toks, v+1); v >= len(toks) {
					return Token{}, false
				}
			}
			return toks[v], true
		}
		// Step over the value so the next key is read as a key, brackets and all.
		if toks[v].Kind == ArrayOpen || toks[v].Kind == DictOpen {
			k = nextMeaningful(toks, closeOf(toks, v))
			continue
		}
		k = nextMeaningful(toks, v+1)
	}
	return Token{}, false
}

// nextMeaningful is the index of the first token at or after i that is not whitespace.
func nextMeaningful(toks []Token, i int) int {
	for i < len(toks) && toks[i].Kind == Whitespace {
		i++
	}
	return i
}

// closeOf is the index just past the bracketed value opening at i, or len(toks) if it never closes — an
// unclosed array in an image dictionary is malformed input, and swallowing the rest of the dictionary is
// the reading that cannot mistake its contents for keys.
func closeOf(toks []Token, i int) int {
	depth := 0
	for ; i < len(toks); i++ {
		switch toks[i].Kind {
		case ArrayOpen, DictOpen:
			depth++
		case ArrayClose, DictClose:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return i
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
