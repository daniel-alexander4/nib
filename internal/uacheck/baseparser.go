package uacheck

import (
	"math"
	"strconv"
)

// veraPDF-parser's `BaseParser` — the ONE tokenizer nib reads veraPDF's PostScript-shaped inputs with: a Type 3 glyph
// procedure's head (`Type3CharProcParser`, P07.S04b), a Type 1 program's cleartext (`Type1FontProgram` through
// `NotSeekableCOSParser` in PostScript mode) and its decrypted private part (`Type1PrivateParser`, P07.S06). It is a
// port, quirks included, because nib is graded against veraPDF's verdicts:
//
//   - a token's VALUE is one buffer that only the readers that clear it change: `[`, `]`, `{`, `}`, `)` and the end of
//     the input leave the previous token's value in place (`Type1PrivateParser` reads it after EOF);
//   - `readNumber` stops at the first byte that is neither a digit nor a dot, answers Double.MAX_VALUE where Java's parse
//     throws, and in PostScript mode reads `radix#digits` — with DIGITS only, so `16#FF` is Double.MAX_VALUE;
//   - a literal string's octal escape is decoded only outside PostScript mode (in it, `\123` is `23`);
//   - a name's `#xx` is not decoded (PostScript mode, and the private parser's own `readName`);
//   - a byte at or above 0x80 is neither a space nor a delimiter (`CharTable` is asked with a signed byte).
//
// The two input classes differ at the edges, and the difference is the source's (`bpSource`): a buffered stream
// (`ASBufferedInFilter`, the NotSeekable parsers) answers -1 reading past its end, a seekable one (`ASMemoryInStream`,
// `InternalInputStream`) throws an IOException.

// bpSource is the byte source under the tokenizer.
type bpSource interface {
	readByte() byte // Java's `readByte`: past the end, -1 or an IOException (by source)
	unread()
	isEOF() bool
	peek() int
}

// t1Abort is how the ports below leave a parse: an IOException (errIO), a runtime exception veraPDF does not catch
// (errThrow), or a shape nib does not mirror (errRefuse). Raised by panic and recovered at each reader's entry.
type t1Abort struct{ err cffErr }

func t1Fail(kind errKind, why string) { panic(t1Abort{cffErr{kind: kind, why: why}}) }

// bpBufSrc is `ASBufferedInFilter` as a buffered stream (`initialize` called): a 2048-byte window whose read position
// sits near its middle, refilled from the underlying stream as it passes three quarters. Emulated byte for byte because
// its edges are observable — `read(byte[], int)` takes a second path when the last refill brought 11 bytes or fewer, and
// `readByte` past the end answers -1 without moving.
type bpBufSrc struct {
	src    []byte
	sp     int
	buf    [bfCap]byte
	pos    int
	eod    int
	bufEnd int // `bufferEnd`; `bufferBegin` is 0 throughout this use
}

const (
	bfCap    = 2048
	bfHalf   = bfCap / 2
	bfThresh = 3 * bfCap / 4
)

func newBufSrc(b []byte) *bpBufSrc {
	s := &bpBufSrc{src: b, eod: -1}
	s.fill(bfHalf, bfHalf)
	s.pos = bfHalf
	return s
}

// srcRead is `InputStream.read(byte[], int, int)` over the decoded stream: it fills what it can, -1 at the end.
func (s *bpBufSrc) srcRead(dst []byte) int {
	if len(dst) == 0 {
		return 0
	}
	if s.sp >= len(s.src) {
		return -1
	}
	n := copy(dst, s.src[s.sp:])
	s.sp += n
	return n
}

// fill is `readFromStreamToBuffer`.
func (s *bpBufSrc) fill(off, n int) {
	r := s.srcRead(s.buf[off : off+n])
	s.bufEnd = r
	if r < n {
		if r < 0 {
			r = 0
		}
		s.eod = off + r
	}
}

func (s *bpBufSrc) shift(n int) {
	if n > bfCap {
		n = bfCap
	}
	copy(s.buf[:], s.buf[n:])
	s.pos -= n
	if s.eod != -1 {
		s.eod -= n
	}
}

func (s *bpBufSrc) feed() {
	if s.pos > bfHalf {
		n := s.pos - bfHalf
		s.shift(n)
		s.fill(bfCap-n, n)
	}
}

func (s *bpBufSrc) isEOF() bool { return s.eod != -1 && s.pos >= s.eod }

func (s *bpBufSrc) readByte() byte {
	if s.isEOF() {
		return 0xFF
	}
	if s.pos < 0 || s.pos >= bfCap {
		t1Fail(errThrow, "the buffered reader indexes outside its buffer")
	}
	c := s.buf[s.pos]
	s.pos++
	if s.pos > bfThresh && s.eod == -1 {
		s.feed()
	}
	return c
}

func (s *bpBufSrc) unread() {
	i := s.pos - 1
	if i > bfCap || i < 0 {
		t1Fail(errIO, "the buffered reader cannot unread before its buffer")
	}
	s.pos = i
}

func (s *bpBufSrc) peek() int {
	i := s.pos
	if i > bfCap || i < 0 {
		t1Fail(errIO, "the buffered reader cannot peek outside its buffer")
	}
	if s.eod != -1 && i >= s.eod {
		return -1
	}
	if i == bfCap {
		t1Fail(errThrow, "the buffered reader peeks one past its buffer")
	}
	return int(s.buf[i])
}

// copyFrom is `copyFromBuffer`.
func (s *bpBufSrc) copyFrom(off, size int, res []byte) int {
	end := bfCap
	if s.eod != -1 && s.eod < end {
		end = s.eod
	}
	n := size
	if end-off < n {
		n = end - off
	}
	if n < 0 || off < 0 || n > len(res) {
		t1Fail(errThrow, "the buffered reader copies a negative length")
	}
	copy(res[:n], s.buf[off:off+n])
	return n
}

// read is `read(byte[] buffer, int size)`.
func (s *bpBufSrc) read(dst []byte, size int) int {
	if s.isEOF() {
		return -1
	}
	if size < s.bufEnd {
		n := s.copyFrom(s.pos, size, dst)
		s.pos += n
		if s.pos > bfThresh {
			s.feed()
		}
		return n
	}
	copied := s.copyFrom(s.pos, size, dst)
	r := s.srcRead(dst[copied:size])
	if r == -1 {
		r = 0
	}
	s.shift(copied + r)
	srcBegin := max(0, copied+r-bfCap)
	destBegin := max(0, bfCap-copied-r)
	n := min(len(dst), copied+r)
	if destBegin+n > bfCap || srcBegin+n > len(dst) {
		t1Fail(errThrow, "the buffered reader copies past its buffer")
	}
	copy(s.buf[destBegin:destBegin+n], dst[srcBegin:srcBegin+n])
	s.pos = bfCap
	s.feed()
	return copied + r
}

// bpMemSrc is a seekable stream (`ASMemoryInStream`, `InternalInputStream`): `readByte` past the end throws.
type bpMemSrc struct {
	b   []byte
	off int
}

func (s *bpMemSrc) readByte() byte {
	if s.off >= len(s.b) {
		t1Fail(errIO, "End of file is reached")
	}
	s.off++
	return s.b[s.off-1]
}

// read is `read()`: -1 past the end, without moving.
func (s *bpMemSrc) read() int {
	if s.off >= len(s.b) {
		return -1
	}
	s.off++
	return int(s.b[s.off-1])
}

func (s *bpMemSrc) unread() {
	if s.off-1 < 0 {
		t1Fail(errIO, "Can't seek for a negative offset")
	}
	s.off--
}

func (s *bpMemSrc) isEOF() bool { return s.off == len(s.b) }

func (s *bpMemSrc) peek() int {
	if s.off >= len(s.b) {
		return -1
	}
	return int(s.b[s.off])
}

// bpType is `Token.Type`; bpTNull is the type a fresh token has before its first read (Java's null).
type bpType uint8

const (
	bpTNull bpType = iota
	bpTNone
	bpTKeyword
	bpTInteger
	bpTReal
	bpTLitString
	bpTHexString
	bpTName
	bpTOpenArray
	bpTCloseArray
	bpTOpenDict
	bpTCloseDict
	bpTEOF
	bpTStartProc
	bpTEndProc
)

// bpKeywords is `Token.KEYWORDS`: a keyword token's `keyword` is null for any other text.
var bpKeywords = map[string]bool{"null": true, "true": true, "false": true, "stream": true, "endstream": true,
	"obj": true, "endobj": true, "R": true, "n": true, "f": true, "xref": true, "startxref": true, "trailer": true}

// bpLexer is `BaseParser` with its `Token`.
type bpLexer struct {
	src bpSource
	ps  bool // `isPSParser`

	typ     bpType
	kw      string // the keyword a TT_KEYWORD token names, "" for Java's null
	integer int64
	real    float64
	val     []byte
	a85     bool // the last string token was ASCII85, which nib does not decode
}

// bpMaxString is `MAX_STRING_LENGTH`: a literal string stops growing past it.
const bpMaxString = 65535

func bpSpace(c byte) bool { return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32 }

func bpDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return bpSpace(c)
}

func (l *bpLexer) value() string { return string(l.val) }

// skipSpaces is `skipSpaces(skipComment)`.
func (l *bpLexer) skipSpaces(comment bool) {
	for l.skipSingleSpace(comment) {
	}
}

func (l *bpLexer) skipSingleSpace(comment bool) bool {
	if l.src.isEOF() {
		return false
	}
	c := l.src.readByte()
	if bpSpace(c) {
		return true
	}
	if c == '%' && comment {
		l.skipComment()
		return true
	}
	l.src.unread()
	return false
}

// skipComment is either parser's: to LF, or CR and an LF after it (both variants read the same bytes; the source
// decides what reading past the end does).
func (l *bpLexer) skipComment() {
	for !l.src.isEOF() {
		c := l.src.readByte()
		if c == '\n' {
			return
		}
		if c == '\r' {
			if l.src.readByte() != '\n' {
				l.src.unread()
			}
			return
		}
	}
}

// next is `nextToken`.
func (l *bpLexer) next() {
	l.skipSpaces(true)
	if l.src.isEOF() {
		l.typ = bpTEOF
		return
	}
	l.typ = bpTNone
	c := l.src.readByte()
	switch c {
	case '(':
		l.typ = bpTLitString
		l.readLitString()
	case ')':
	case '<':
		c = l.src.readByte()
		switch c {
		case '<':
			l.typ = bpTOpenDict
		case '~':
			l.typ = bpTHexString
			l.readASCII85()
		default:
			l.src.unread()
			l.typ = bpTHexString
			l.readHexString()
		}
	case '>':
		if l.src.readByte() == '>' {
			l.typ = bpTCloseDict
		} else {
			t1Fail(errIO, "Unknown symbol after '>'")
		}
	case '[':
		l.typ = bpTOpenArray
	case ']':
		l.typ = bpTCloseArray
	case '{':
		if l.ps {
			l.typ = bpTStartProc
		}
	case '}':
		if l.ps {
			l.typ = bpTEndProc
		}
	case '/':
		l.typ = bpTName
		l.readName()
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '.':
		l.src.unread()
		l.readNumber()
	case '+':
		l.readNumber()
	case '-':
		l.readNumber()
		l.integer = -l.integer
		l.real = -l.real
	default:
		l.src.unread()
		l.readToken()
		l.typ = bpTKeyword
		l.kw = ""
		if bpKeywords[l.value()] {
			l.kw = l.value()
		}
	}
}

func (l *bpLexer) readToken() {
	l.val = l.val[:0]
	for !l.src.isEOF() {
		c := l.src.readByte()
		if c < 0x80 && bpDelimiter(c) {
			l.src.unread()
			break
		}
		l.val = append(l.val, c)
	}
}

// readName is `readName` without `#` decoding — PostScript mode's, and `Type1PrivateParser`'s own override. (A Type 3
// procedure's names are read the same way: nothing there reads a name's value, and the bytes a name spans do not change.)
func (l *bpLexer) readName() { l.readToken() }

// readNumber is `readNumber`.
func (l *bpLexer) readNumber() {
	l.val = l.val[:0]
	l.typ = bpTInteger
	radix := 10
	fail := func() {
		l.integer = math.MaxInt64 // Math.round(Double.MAX_VALUE)
		l.real = math.MaxFloat64
	}
	for !l.src.isEOF() {
		c := l.src.readByte()
		if c < 0x80 && bpDelimiter(c) {
			l.src.unread()
			break
		}
		switch {
		case c >= '0' && c <= '9':
			l.val = append(l.val, c)
		case c == '.':
			l.typ = bpTReal
			l.val = append(l.val, c)
		case c == '#' && l.ps:
			if l.typ == bpTInteger {
				r, err := strconv.ParseInt(l.value(), 10, 32) // Integer.parseInt: its throw ends the read HERE
				if err != nil {
					fail()
					return
				}
				radix = int(r)
			}
			l.val = l.val[:0]
		default:
			l.src.unread()
			goto done
		}
	}
done:
	if l.typ == bpTInteger {
		if radix < 2 || radix > 36 {
			fail()
			return
		}
		v, err := strconv.ParseInt(l.value(), radix, 64)
		if err != nil {
			fail()
			return
		}
		l.integer, l.real = v, float64(v)
		return
	}
	v, err := strconv.ParseFloat(l.value(), 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			fail()
			return
		}
	}
	l.integer, l.real = javaRound(v), v
}

// readLitString is `readLitString`.
func (l *bpLexer) readLitString() {
	l.val = l.val[:0]
	l.a85 = false
	depth := 0
	add := true
	put := func(c byte) {
		if add {
			l.val = append(l.val, c)
		}
	}
	c := l.src.readByte()
	for !l.src.isEOF() {
		switch c {
		default:
			put(c)
		case '(':
			depth++
			put(c)
		case ')':
			if depth == 0 {
				return
			}
			depth--
			put(c)
		case '\\':
			c = l.src.readByte()
			switch c {
			case '(', ')':
				put(c)
			case 'n':
				put('\n')
			case 'r':
				put('\r')
			case 't':
				put('\t')
			case 'b':
				put('\b')
			case 'f':
				put('\f')
			case '0', '1', '2', '3', '4', '5', '6', '7':
				if !l.ps {
					v := c - '0'
					for i := 1; i < 3; i++ {
						c = l.src.readByte()
						if c < '0' || c > '7' {
							l.src.unread()
							break
						}
						v = v<<3 + (c - '0')
					}
					put(v)
				}
			case '\n':
			case '\r':
				if l.src.readByte() != '\n' {
					l.src.unread()
				}
			default:
				put(c)
			}
		}
		c = l.src.readByte()
		if add && len(l.val) > bpMaxString {
			add = false
		}
	}
}

func bpHexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// readHexString is `readHexString`.
func (l *bpLexer) readHexString() {
	l.val = l.val[:0]
	l.a85 = false
	uc, odd := 0, false
	for !l.src.isEOF() {
		c := l.src.readByte()
		if c == '>' {
			if odd {
				l.val = append(l.val, byte(uc<<4))
			}
			return
		}
		if bpSpace(c) {
			continue
		}
		if h := bpHexDigit(c); h >= 0 {
			if odd {
				l.val = append(l.val, byte(uc<<4+h))
				uc = 0
			} else {
				uc = h
			}
			odd = !odd
		}
	}
}

// readASCII85 spans the bytes the NotSeekable `readASCII85` does — to `~>` or the end — and marks the token: nib does
// not decode ASCII85, and a Type 1 reader that meets one refuses.
func (l *bpLexer) readASCII85() {
	l.a85 = true
	l.val = l.val[:0]
	b := l.src.readByte()
	for !l.src.isEOF() && (b != '~' || l.src.peek() != '>') {
		b = l.src.readByte()
	}
	l.src.readByte()
}

// javaRound is Java's `Math.round(double)`: ties toward positive infinity, NaN 0, saturating.
func javaRound(v float64) int64 {
	if math.IsNaN(v) {
		return 0
	}
	f := math.Floor(v)
	if v-f >= 0.5 {
		f++
	}
	return javaD2L(f)
}

// javaD2L is Java's `(long)` of a double: toward zero, NaN 0, saturating.
func javaD2L(v float64) int64 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= 9.223372036854775807e18:
		return math.MaxInt64
	case v <= -9.223372036854775808e18:
		return math.MinInt64
	}
	return int64(v)
}

// javaD2I is Java's `(int)` of a double.
func javaD2I(v float64) int32 {
	switch {
	case math.IsNaN(v):
		return 0
	case v >= math.MaxInt32:
		return math.MaxInt32
	case v <= math.MinInt32:
		return math.MinInt32
	}
	return int32(v)
}

// bpTokens is `Type3CharProcParser`'s view of the tokenizer: kinds, a keyword's text, a number's value (`token.real`).
type bpTokens struct {
	b  []byte
	lx *bpLexer
}

func (p *bpTokens) next() (t bpToken) {
	if p.lx == nil {
		p.lx = &bpLexer{src: newBufSrc(p.b)}
	}
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(t1Abort); !ok {
				panic(r)
			}
			t = bpToken{kind: bpThrow}
		}
	}()
	p.lx.next()
	switch p.lx.typ {
	case bpTKeyword:
		return bpToken{kind: bpKeyword, text: p.lx.value()}
	case bpTInteger:
		return bpToken{kind: bpInteger, value: p.lx.real}
	case bpTReal:
		return bpToken{kind: bpReal, value: p.lx.real}
	case bpTEOF:
		return bpToken{kind: bpEOF}
	}
	return bpToken{}
}
