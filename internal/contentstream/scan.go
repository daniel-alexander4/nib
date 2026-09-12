// Package contentstream tokenizes a decoded PDF content stream and writes it back.
//
// # What it is for
//
// `PLAN-accessibility.md` P05 needs to bracket page content in `BDC`/`EMC` so a structure tree has
// marked content to point at, and `PLAN-text-reflow.md` P05 needs to replace one text run's bytes.
// Both are edits to a content stream, and nothing in this repo — or in pdfcpu v0.13.0, which has no
// content-stream tokenizer and no text extraction at all — could read one.
//
// # The one design decision, and everything follows from it
//
// **A token is a SPAN of the original bytes, never a parsed value.**
//
// The alternative — parse each operand into a Go value and re-serialise it on the way out — makes a
// byte-identical round trip a battle over every lexical form a PDF may legally use: `1.0` against
// `1.` against `+1`, the spacing inside `<< /A /B >>`, `#20` escapes in a name, which of five
// escape spellings a string used. Each one lost is a silent corruption of somebody's document, and
// the loss is invisible until a signature stops verifying.
//
// Holding spans makes identity structural: the writer copies bytes it never interpreted, so a
// construct this package does not understand cannot be damaged by it. The round-trip test then
// checks that **nothing re-serialises**, which is one property, rather than that everything
// re-serialises correctly, which is an open-ended list.
//
// It is also what the callers want. Bracketing content needs offsets; replacing a run needs a
// splice. Neither needs a decoded value — and a caller that does need one decodes the span itself,
// where the cost is paid by whoever asked.
//
// # What it does not do
//
// It does not interpret operators, track graphics state, or resolve resources. It answers one
// question — *where does each token start and end* — and leaves the rest to callers.
package contentstream

import "fmt"

// Kind classifies a token by what its bytes ARE, which is all a splice needs to know.
//
// The distinctions that exist are the ones that change where a token ENDS: a literal string ends at
// a balanced `)`, a hex string at `>`, an inline image at `EI`. Everything whose end is "the next
// delimiter or whitespace" shares one kind rather than being split into numbers, booleans and
// nulls, because nothing here needs to tell those apart and a caller that does can read the span.
type Kind int

const (
	// Whitespace is any run of PDF whitespace (including a comment, which is lexically whitespace).
	Whitespace Kind = iota
	// Operand is a number, name, boolean, null, or any other self-delimiting token that is not an
	// operator. Callers tell operands from operators by POSITION — operands precede their operator
	// — which is what the grammar actually says.
	Operand
	// Operator is a bare keyword in operator position (`Tj`, `BDC`, `re`, `q`).
	Operator
	// LiteralString is `( … )`, with balanced unescaped parens and backslash escapes.
	LiteralString
	// HexString is `< … >`.
	HexString
	// ArrayOpen, ArrayClose, DictOpen and DictClose are `[`, `]`, `<<` and `>>`. They are tokens in
	// their own right rather than a parsed structure: nesting is the caller's business, and a
	// walker that built trees would have to re-emit them.
	ArrayOpen
	ArrayClose
	DictOpen
	DictClose
	// InlineImage is a whole `BI … ID … EI` sequence as ONE token, binary payload included. See
	// scanInlineImage for why it cannot be anything else.
	InlineImage
)

func (k Kind) String() string {
	switch k {
	case Whitespace:
		return "whitespace"
	case Operand:
		return "operand"
	case Operator:
		return "operator"
	case LiteralString:
		return "string"
	case HexString:
		return "hexstring"
	case ArrayOpen:
		return "["
	case ArrayClose:
		return "]"
	case DictOpen:
		return "<<"
	case DictClose:
		return ">>"
	case InlineImage:
		return "inlineimage"
	}
	return fmt.Sprintf("kind(%d)", int(k))
}

// Token is one lexical unit, addressed as a half-open byte range [Start, End) of the stream it came
// from. It deliberately carries no decoded value: see the package comment.
type Token struct {
	Kind  Kind
	Start int
	End   int
}

// Bytes returns the token's raw bytes from the stream it was scanned out of.
//
// It takes the stream rather than holding a slice so a Token stays a plain comparable value and a
// caller cannot accidentally mutate the source through one.
func (t Token) Bytes(src []byte) []byte { return src[t.Start:t.End] }

// isWhite reports PDF whitespace: ISO 32000-1 table 1. **NUL is whitespace in PDF**, which matters
// here rather than being trivia — nib's own authored output carries NUL bytes inside literal
// strings (79 of them in a one-page Markdown conversion, measured), and treating one as a token
// boundary outside a string would be wrong in the other direction.
func isWhite(c byte) bool {
	return c == 0x00 || c == 0x09 || c == 0x0A || c == 0x0C || c == 0x0D || c == 0x20
}

// isDelim reports a PDF delimiter: the characters that end a token without being part of it.
func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// isRegular is everything that is neither whitespace nor a delimiter — the characters a number,
// name body, keyword or operator is made of.
func isRegular(c byte) bool { return !isWhite(c) && !isDelim(c) }
