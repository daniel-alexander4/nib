package contentstream

import (
	"bytes"
	"fmt"
)

// WriteTokens emits the tokens back as bytes.
//
// **Named `WriteTokens` and not `Write` so that a guard can see it.** `zerocaller_test.go` counts
// every occurrence of an identifier to decide whether a function has a caller, so an exported
// function called `Write` is indistinguishable from the thousand `buf.Write` calls in the tree and
// can never be reported as uncalled — it would be permanently exempt from the one check that finds
// dead exported code. The name also says what it writes.
//
// **For tokens that came from `Tokenize(src)` and were not edited, this returns `src` exactly** —
// not an equivalent stream, the same bytes. That is the package's central promise and it holds by
// construction rather than by care: every token is a span of `src` and this copies spans.
//
// It is `src`-relative, so a caller cannot hand it tokens from one stream and the bytes of another
// and get something plausible-looking back: the offsets would address the wrong document, so the
// bounds check below refuses rather than producing a corrupted stream from a caller's mistake.
//
// **Declared limit: the bounds check catches tokens that reach PAST `src`, not tokens that stop
// short of it.** Writing a list that covers only part of a stream returns only that part, silently,
// because that is also what writing a deliberately-chosen subrange would look like and this cannot
// tell the two apart. Every caller in the tree writes the whole token list from `Tokenize`, and the
// round-trip tests assert total coverage separately — which is where a short list is caught.
func WriteTokens(src []byte, tokens []Token) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(src))
	for i, t := range tokens {
		if t.Start < 0 || t.End > len(src) || t.Start > t.End {
			return nil, fmt.Errorf("contentstream: token %d is %s, outside the %d-byte stream it "+
				"is being written against — these tokens came from a different stream",
				i, t.Kind, len(src))
		}
		buf.Write(src[t.Start:t.End])
	}
	return buf.Bytes(), nil
}

// Edit accumulates insertions against one stream and applies them in a single pass.
//
// # Why insertions are collected rather than applied
//
// Every caller of this package inserts at more than one place — S04 brackets page content with
// `BDC` before and `EMC` after, in one edit. Applying insertions one at a time means every offset
// after the first is stale, and the caller has to adjust them: the classic off-by-one factory, and
// one whose failures are silent because a stream spliced at the wrong offset is usually still
// syntactically valid.
//
// Collecting them keeps every offset in the ORIGINAL stream's coordinates, which is the only
// coordinate system a caller has reason to think in.
//
// # What it does not do
//
// It does not delete or replace. `PLAN-text-reflow.md`'s P05 needs replacement and will add it; it
// is not built here on the "one caller, one feature" rule, because an unused replace is an untested
// replace and this package's whole value is that its output is trustworthy.
type Edit struct {
	src     []byte
	inserts []insertion
}

type insertion struct {
	at   int
	text []byte
	seq  int // preserves the caller's order for two insertions at the same offset
}

// NewEdit starts an edit against src.
func NewEdit(src []byte) *Edit { return &Edit{src: src} }

// InsertBefore queues text to be written immediately before the byte at offset `at`, which is in
// the ORIGINAL stream's coordinates however many insertions precede it.
//
// Two insertions at the same offset are emitted in call order, so a caller bracketing a span writes
// the opener and the closer in the order they should appear.
func (e *Edit) InsertBefore(at int, text []byte) *Edit {
	e.inserts = append(e.inserts, insertion{at: at, text: append([]byte(nil), text...), seq: len(e.inserts)})
	return e
}

// Apply produces the edited stream.
//
// **With no insertions it returns the original bytes**, which is the property the round-trip law
// rests on: an operation that decides it has nothing to change costs the document nothing, rather
// than costing it a re-serialisation that happens to look the same today.
func (e *Edit) Apply() ([]byte, error) {
	if len(e.inserts) == 0 {
		return e.src, nil
	}
	for _, in := range e.inserts {
		if in.at < 0 || in.at > len(e.src) {
			return nil, fmt.Errorf("contentstream: insertion at %d is outside the %d-byte stream",
				in.at, len(e.src))
		}
	}
	// Stable sort by offset, then by call order. Written as an insertion sort over what is always a
	// handful of entries rather than pulling in sort.SliceStable for two elements.
	ins := append([]insertion(nil), e.inserts...)
	for i := 1; i < len(ins); i++ {
		for j := i; j > 0 && (ins[j-1].at > ins[j].at ||
			(ins[j-1].at == ins[j].at && ins[j-1].seq > ins[j].seq)); j-- {
			ins[j-1], ins[j] = ins[j], ins[j-1]
		}
	}
	var buf bytes.Buffer
	buf.Grow(len(e.src) + 64*len(ins))
	prev := 0
	for _, in := range ins {
		buf.Write(e.src[prev:in.at])
		buf.Write(in.text)
		prev = in.at
	}
	buf.Write(e.src[prev:])
	return buf.Bytes(), nil
}
