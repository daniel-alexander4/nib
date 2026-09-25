package uacheck

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A CFF font program as veraPDF-parser 1.30.2 reads it — `PLAN-ua-coverage.md` P07.S05a. The reader mirrors
// `CFFFontProgram` → `CFFType1FontProgram` (and `CFFFileBaseParser`, `CFFFontBaseParser`, `CFFIndex`,
// `CharStringsWidths`, `CFFCharStringsHandler`, `Type2CharStringParser`) line for line, because 7.21.5 t1, 7.21.4.1 t2
// and 7.21.4.2 t1 are judged against what veraPDF computes, not against what the CFF specification says: its Top DICT
// reads byte 28 unsigned, its encoding supplements store a SID where a glyph index belongs, its format 1/2 charsets
// read past the glyph count, and a charstring error still has nominalWidthX added. Each is written at its site.
//
// **Three outcomes, kept apart.** A program veraPDF's parse throws an `IOException` on is NOT PARSED (`ttFailed`): its
// glyphs' metrics stay null and pass, and 7.21.4.1 t1 calls it not embedded. A program on which veraPDF throws an
// exception its validator does not handle makes it report nothing on the document (`throws`, which the door
// `reportsNothing` turns into a refusal of every clause). Where nib does not mirror veraPDF the answer is `ttUnknown`,
// a refusal naming why.

// cffProgram is one simple (non-CID) CFF program's reading.
type cffProgram struct {
	state  ttState
	why    string // for ttUnknown and ttFailed
	throws string // veraPDF throws an exception it does not handle: it reports nothing on the document
	cid    bool   // CID-keyed: P07.S05b's

	charSet     map[string]int // glyph name → GID, a later duplicate winning
	inverseSize int            // `inverseCharSet.size()`: GIDs 0..inverseSize-1 are named
	inverse     map[int]string
	std, expert bool     // the program's built-in encoding is Standard or Expert (offset 0 or 1)
	encoding    [256]int // otherwise code → GID-1 (`CFFType1FontProgram.encoding`)

	charStrings cffIndex
	amount      int // charstrings: `getWidthsAmount`
	widths      *cffWidths
}

// errKind is how veraPDF's reader fails at a site: an IOException (the parse did not succeed), a runtime exception
// its validator does not catch (it reports nothing), or a shape nib does not mirror (a refusal).
type errKind int

const (
	errIO errKind = iota + 1
	errThrow
	errRefuse
)

type cffErr struct {
	kind errKind
	why  string
}

func (e *cffErr) Error() string { return e.why }

func ioErr(format string, a ...any) *cffErr    { return &cffErr{errIO, fmt.Sprintf(format, a...)} }
func throwErr(format string, a ...any) *cffErr { return &cffErr{errThrow, fmt.Sprintf(format, a...)} }
func refuse(format string, a ...any) *cffErr   { return &cffErr{errRefuse, fmt.Sprintf(format, a...)} }

// cffMaxAlloc bounds an INDEX's data length nib takes as a plain short read: veraPDF allocates `offset[count]-1` bytes
// BEFORE reading them, and past a size its heap cannot hold that is an OutOfMemoryError, not an IOException — which
// size that is depends on the JVM, so nib refuses rather than guess.
const cffMaxAlloc = 1 << 26

// cffBuffered is `SeekableInputStream`'s MAX_BUFFER_SIZE: a program shorter than this is read into memory, where a seek
// past the end throws an IOException; one this long or longer is file-backed, where it throws an
// IllegalArgumentException nothing catches (P07.S03 measured the same boundary on TrueType programs).
const cffBuffered = 10240

// cffSrc is the program's bytes with veraPDF's stream semantics.
type cffSrc struct {
	b      []byte
	pos    int
	padded int // bytes `get` has zero-padded past an INDEX's data, over the whole program
}

func (s *cffSrc) readByte() (byte, *cffErr) {
	if s.pos >= len(s.b) {
		return 0, ioErr("the program ends before a byte it reads")
	}
	c := s.b[s.pos]
	s.pos++
	return c, nil
}

// peek is `peek()`: -1 at the end.
func (s *cffSrc) peek() int {
	if s.pos >= len(s.b) {
		return -1
	}
	return int(s.b[s.pos])
}

func (s *cffSrc) seek(off int64) *cffErr {
	switch {
	case off < 0:
		return ioErr("a seek to the negative offset %d", off)
	case off > int64(len(s.b)) && len(s.b) >= cffBuffered:
		return throwErr("a seek to offset %d past the program's end (%d bytes), where veraPDF's file-backed stream throws "+
			"an IllegalArgumentException it does not handle", off, len(s.b))
	case off > int64(len(s.b)):
		return ioErr("a seek to offset %d past the program's end (%d bytes)", off, len(s.b))
	}
	s.pos = int(off)
	return nil
}

func (s *cffSrc) card8() (int, *cffErr) {
	c, err := s.readByte()
	return int(c), err
}

func (s *cffSrc) card16() (int, *cffErr) {
	hi, err := s.readByte()
	if err != nil {
		return 0, err
	}
	lo, err := s.readByte()
	return int(hi)<<8 | int(lo), err
}

// cffIndex is `CFFIndex`: offsets as Java ints, data as read.
type cffIndex struct {
	count   int
	shift   int // `offsetShift`: 3 + offSize × (count+1)
	offsets []int
	data    []byte
	padded  *int // the program's budget for zero-padded copies (`cffMaxAlloc` in all)
}

// readIndex is `CFFFileBaseParser.readIndex`.
func (s *cffSrc) readIndex() (cffIndex, *cffErr) {
	count, err := s.card16()
	if err != nil || count == 0 {
		return cffIndex{}, err
	}
	offSize, err := s.card8()
	if err != nil {
		return cffIndex{}, err
	}
	if offSize == 0 {
		return cffIndex{}, ioErr("an INDEX with offset size 0")
	}
	if int64(count+1)*int64(offSize) > int64(len(s.b)-s.pos) {
		return cffIndex{}, ioErr("an INDEX whose offsets run past the program's end")
	}
	offsets := make([]int, count+1)
	for i := range offsets {
		var v int64
		for k := 0; k < offSize; k++ {
			c, _ := s.readByte()
			v = v<<8 | int64(c)
		}
		offsets[i] = int(int32(v)) // `(int) readOffset(offSize)`
	}
	if offsets[count] < 1 {
		return cffIndex{}, ioErr("an INDEX whose last offset is below 1")
	}
	n := offsets[count] - 1
	if n > len(s.b)-s.pos {
		if n > cffMaxAlloc {
			return cffIndex{}, refuse("an INDEX of %d bytes, which veraPDF allocates before reading and nib does not "+
				"know whether its heap holds", n)
		}
		return cffIndex{}, ioErr("an INDEX whose data runs past the program's end")
	}
	data := s.b[s.pos : s.pos+n]
	s.pos += n
	return cffIndex{count: count, shift: 3 + offSize*(count+1), offsets: offsets, data: data, padded: &s.padded}, nil
}

// get is `CFFIndex.get`: an out-of-range entry or a non-positive offset is an ArrayIndexOutOfBoundsException; then
// `Arrays.copyOfRange`, which refuses a start after the end FIRST (IllegalArgumentException), then a start past the
// data (ArrayIndexOutOfBoundsException), and zero-pads an end past the data. The caller decides which its catch absorbs.
//
// **Only the last offset is bounded by the data**, so a middle one can ask for a copy of up to 2 GiB of zeros — which
// veraPDF allocates (an OutOfMemoryError past its heap) and nib will not: past `cffMaxAlloc` of padding over the whole
// program, nib refuses.
func (x cffIndex) get(n int) ([]byte, *cffErr) {
	if n < 0 || n >= x.count || x.offsets[n] <= 0 || x.offsets[n+1] <= 0 {
		return nil, &cffErr{errIO, "aioobe"}
	}
	from, to := x.offsets[n]-1, x.offsets[n+1]-1
	switch {
	case from > to:
		return nil, throwErr("an INDEX whose offsets run backwards, where veraPDF throws an IllegalArgumentException it " +
			"does not handle")
	case from > len(x.data):
		return nil, &cffErr{errIO, "aioobe"}
	}
	if pad := to - len(x.data); pad > 0 && x.padded != nil {
		if *x.padded += pad; *x.padded > cffMaxAlloc {
			return nil, refuse("its INDEX offsets ask for %d bytes past the data, which veraPDF allocates and nib will not", *x.padded)
		}
	}
	out := make([]byte, to-from)
	copy(out, x.data[from:min(to, len(x.data))])
	return out, nil
}

// isAIOOBE is an ArrayIndexOutOfBoundsException from `get`, which some callers catch.
func isAIOOBE(e *cffErr) bool { return e != nil && e.kind == errIO && e.why == "aioobe" }

// cffNumber is `CFFNumber`.
type cffNumber struct {
	integer int64
	real    float32
	isInt   bool
}

func cffInt(v int) cffNumber          { return cffNumber{integer: int64(v), real: float32(v), isInt: true} }
func cffReal(v float32) cffNumber     { return cffNumber{integer: javaFloatToLong(v), real: v} }
func (n cffNumber) getInteger() int64 { return n.integer }

// javaFloatToLong is Java's `(long) f`: truncation, NaN to 0, and saturation at the long's range.
func javaFloatToLong(f float32) int64 {
	switch {
	case f != f:
		return 0
	case float64(f) >= math.MaxInt64:
		return math.MaxInt64
	case float64(f) <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// readNumber is `CFFFileBaseParser.readNumber` for a DICT operand. **Byte 28 is read UNSIGNED** (`readCard16`), where
// the specification makes it a signed 16-bit integer.
func (s *cffSrc) readNumber() (cffNumber, *cffErr) {
	first, err := s.readByte()
	if err != nil {
		return cffNumber{}, err
	}
	if first == 0x1E {
		f, err := s.readReal()
		return cffReal(f), err
	}
	b := int(first)
	switch {
	case b > 31 && b < 247:
		return cffInt(b - 139), nil
	case b > 246 && b < 251:
		c, err := s.card8()
		return cffInt((b-247)<<8 + c + 108), err
	case b > 250 && b < 255:
		c, err := s.card8()
		return cffInt(-((b - 251) << 8) - c - 108), err
	case b == 28:
		v, err := s.card16()
		return cffInt(v), err
	case b == 29:
		hi, err := s.card16()
		if err != nil {
			return cffNumber{}, err
		}
		lo, err := s.card16()
		return cffInt(int(int32(uint32(hi)<<16 | uint32(lo)))), err
	}
	return cffNumber{}, ioErr("a DICT number that is not one")
}

// readReal is `readReal`: nibbles into a string, then `Float.parseFloat`, whose NumberFormatException is an IOException.
func (s *cffSrc) readReal() (float32, *cffErr) {
	var sb strings.Builder
	for {
		c, err := s.readByte()
		if err != nil {
			return 0, err
		}
		for _, h := range [2]byte{c >> 4, c & 0x0F} {
			switch {
			case h < 10:
				sb.WriteByte('0' + h)
			case h == 0x0A:
				sb.WriteByte('.')
			case h == 0x0B:
				sb.WriteByte('E')
			case h == 0x0C:
				sb.WriteString("E-")
			case h == 0x0E:
				sb.WriteByte('-')
			default: // 0x0F ends it; 0x0D "can not be reached" and ends it too
				return javaParseFloat(sb.String())
			}
		}
	}
}

// javaParseFloat is `Float.parseFloat` over the strings `readReal` can build (digits, `.`, `E`, `E-`, `-`): the syntax
// Go and Java refuse agrees there, and past the range Java answers Infinity (or 0) where Go reports ErrRange.
func javaParseFloat(str string) (float32, *cffErr) {
	v, err := strconv.ParseFloat(str, 32)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			return 0, ioErr("a DICT real %q that is not a number", str)
		}
	}
	return float32(v), nil
}

// cffParser holds `CFFFontBaseParser`'s and `CFFType1FontProgram`'s fields.
type cffParser struct {
	src              *cffSrc
	names            cffIndex // String INDEX (`definedNames`)
	gsubrs           cffIndex
	stack            []cffNumber
	topBegin, topEnd int64
	subset           bool

	privOff, privSize int64
	fontMatrix        [6]float32
	charStringsOff    int64
	charSetOff        int64
	encodingOff       int64
	charStringType    int
	defaultWidthX     int
	nominalWidthX     int
	subrsOff          int64
	containsROS       bool
	scanOnly          bool // a plain `CFFFontBaseParser` (the ROS scan): operator 16 is not Encoding there
	lsubrs            cffIndex
	bias              int
}

// stringBySID is `getStringBySID`: a standard string, else the String INDEX's entry, an ArrayIndexOutOfBoundsException
// there being an IOException (and an IllegalArgumentException escaping).
func (p *cffParser) stringBySID(sid int) (string, *cffErr) {
	if sid >= 0 && sid < len(cffStandardStrings) {
		return cffStandardStrings[sid], nil
	}
	if sid < 0 {
		return "", ioErr("the negative SID %d", sid)
	}
	b, err := p.names.get(sid - len(cffStandardStrings))
	if isAIOOBE(err) {
		return "", ioErr("the SID %d names no string", sid)
	}
	return string(b), err // ISO-8859-1: one rune per byte is what a Go string of the bytes compares as
}

// topOf returns the element `back` from the stack's top, or the IndexOutOfBoundsException `readTopDictUnit` turns into
// an IOException.
func (p *cffParser) topOf(back int) (cffNumber, *cffErr) {
	if len(p.stack) < back {
		return cffNumber{}, ioErr("a Top DICT operator with too few operands")
	}
	return p.stack[len(p.stack)-back], nil
}

// readTopDictUnit is `CFFFontBaseParser.readTopDictUnit` with `CFFType1FontProgram`'s one-byte operator 16 (Encoding).
func (p *cffParser) readTopDictUnit() *cffErr {
	next := p.src.peek()
	if (next > 27 && next < 31) || (next > 31 && next < 255) {
		n, err := p.src.readNumber()
		if err != nil {
			return err
		}
		p.stack = append(p.stack, n)
		return nil
	}
	if _, err := p.src.readByte(); err != nil {
		return err
	}
	if next >= 22 {
		return nil // 22-27, 31 and 255 are consumed and ignored, the stack kept
	}
	switch next {
	case 4: // Weight: its SID is resolved and dropped
		n, err := p.topOf(1)
		if err != nil {
			return err
		}
		if _, err := p.stringBySID(int(int32(n.getInteger()))); err != nil {
			return err
		}
	case 15:
		n, err := p.topOf(1)
		if err != nil {
			return err
		}
		p.charSetOff = n.getInteger()
	case 17:
		n, err := p.topOf(1)
		if err != nil {
			return err
		}
		p.charStringsOff = n.getInteger()
	case 18:
		size, err := p.topOf(2)
		if err != nil {
			return err
		}
		off, _ := p.topOf(1)
		p.privSize, p.privOff = size.getInteger(), off.getInteger()
	case 16:
		if p.scanOnly {
			break // `readTopDictOneByteOps` of the base parser: clear the stack
		}
		n, err := p.topOf(1)
		if err != nil {
			return err
		}
		p.encodingOff = n.getInteger()
	case 12:
		c, err := p.src.readByte()
		if err != nil {
			return err
		}
		switch c {
		case 7: // FontMatrix reads the BOTTOM six of the stack
			if len(p.stack) < 6 {
				return ioErr("a FontMatrix with fewer than six operands")
			}
			for i := range p.fontMatrix {
				p.fontMatrix[i] = p.stack[i].real
			}
		case 6:
			n, err := p.topOf(1)
			if err != nil {
				return err
			}
			p.charStringType = int(int32(n.getInteger()))
		case 30:
			p.containsROS = true
			return nil // ROS does not clear the stack
		}
	}
	p.stack = p.stack[:0]
	return nil
}

// readPrivateDictUnit is `readPrivateDictUnit` — with its quirk: an escape byte 12 clears the stack WITHOUT consuming
// its second byte, which the next unit then reads as a number or an operator.
func (p *cffParser) readPrivateDictUnit() *cffErr {
	next := p.src.peek()
	if (next > 27 && next < 31) || (next > 31 && next < 255) {
		n, err := p.src.readNumber()
		if err != nil {
			return err
		}
		p.stack = append(p.stack, n)
		return nil
	}
	if _, err := p.src.readByte(); err != nil {
		return err
	}
	if next >= 22 {
		return nil
	}
	switch next {
	case 20, 21, 19:
		if len(p.stack) == 0 {
			return nil // only logged
		}
		v := p.stack[len(p.stack)-1].getInteger()
		switch next {
		case 20:
			p.defaultWidthX = int(int32(v))
		case 21:
			p.nominalWidthX = int(int32(v))
		case 19:
			p.subrsOff = v + p.privOff
		}
	}
	p.stack = p.stack[:0]
	return nil
}

// cffBias is the subroutine bias by count (`readLocalSubrsAndBias`, `getGlobalBias`).
func cffBias(n int) int {
	switch {
	case n < 1240:
		return 107
	case n < 33900:
		return 1131
	}
	return 32768
}

// readCFF reads a CFF program for a simple font, `subset` being `PDFont.isSubset` of the font that opens it.
func readCFF(b []byte, subset bool) (prog cffProgram) {
	src := &cffSrc{b: b}
	p := &cffParser{src: src, subset: subset, charStringType: 2, subrsOff: -1,
		fontMatrix: [6]float32{0.001, 0, 0, 0.001, 0, 0}}
	defer func() {
		if prog.state == ttUnknown && prog.why == "" && prog.throws == "" {
			prog.why = "the program could not be read"
		}
	}()
	fail := func(err *cffErr) cffProgram {
		switch err.kind {
		case errThrow:
			return cffProgram{state: ttUnknown, throws: err.why}
		case errRefuse:
			return cffProgram{state: ttUnknown, why: err.why}
		}
		return cffProgram{state: ttFailed, why: err.why}
	}
	// `CFFFontProgram.parseFont`: header, Name, Top DICT, String and Global Subr INDEXes.
	src.pos = 2
	hdr, err := src.card8()
	if err == nil {
		err = src.seek(int64(hdr))
	}
	if err != nil {
		return fail(err)
	}
	if _, err := src.readIndex(); err != nil {
		return fail(err)
	}
	topOffset := int64(src.pos)
	top, err := src.readIndex()
	if err != nil {
		return fail(err)
	}
	if top.count == 0 {
		return fail(ioErr("the Top DICT INDEX is empty"))
	}
	if p.names, err = src.readIndex(); err != nil {
		return fail(err)
	}
	if p.gsubrs, err = src.readIndex(); err != nil {
		return fail(err)
	}
	p.topBegin = topOffset + int64(top.offsets[0]) - 1 + int64(top.shift)
	p.topEnd = topOffset + int64(top.offsets[1]) - 1 + int64(top.shift)
	cid, err := p.isCIDFont(top)
	if err != nil {
		return fail(err)
	}
	if cid {
		return cffProgram{state: ttUnknown, cid: true, why: "its CFF program is CID-keyed, which nib reads at P07.S05b"}
	}
	if err := p.parseType1(&prog); err != nil {
		return fail(err)
	}
	prog.state = ttParsed
	return prog
}

// isCIDFont is `CFFFontProgram.isCIDFont`: ROS as the first operator (by the shape of its third operand), else anywhere
// in a Top DICT read to its end by a fresh parser whose IOException means "no".
func (p *cffParser) isCIDFont(top cffIndex) (bool, *cffErr) {
	dict, err := top.get(0)
	if err != nil && !isAIOOBE(err) {
		return false, err
	}
	if err == nil && len(dict) > 4 {
		ros := -1
		switch f := int(dict[4]); {
		case f < 247 && f > 31:
			ros = 5
		case f > 246 && f < 255:
			ros = 6
		case f == 28:
			ros = 7
		case f == 29:
			ros = 9
		}
		if ros > 0 && ros+1 < len(dict) && dict[ros] == 12 && dict[ros+1] == 30 {
			return true, nil
		}
	}
	scan := &cffParser{src: &cffSrc{b: p.src.b}, names: p.names, charStringType: 2, subrsOff: -1, scanOnly: true}
	if err := scan.src.seek(p.topBegin); err != nil {
		if err.kind != errIO {
			return false, err
		}
		return false, nil
	}
	for int64(scan.src.pos) < p.topEnd {
		if err := scan.readTopDictUnit(); err != nil {
			if err.kind != errIO { // only an IOException is "no ROS"; a throw or a refusal is not an answer
				return false, err
			}
			return false, nil
		}
	}
	return scan.containsROS, nil
}

// parseType1 is `CFFType1FontProgram.parseFont`.
func (p *cffParser) parseType1(prog *cffProgram) *cffErr {
	src := p.src
	if err := src.seek(p.topBegin); err != nil {
		return err
	}
	for int64(src.pos) < p.topEnd {
		if err := p.readTopDictUnit(); err != nil {
			return err
		}
	}
	p.stack = p.stack[:0]
	if err := src.seek(p.privOff); err != nil {
		return err
	}
	for int64(src.pos) < p.privOff+p.privSize {
		if err := p.readPrivateDictUnit(); err != nil {
			return err
		}
	}
	if p.subrsOff != -1 {
		at := src.pos
		if err := src.seek(p.subrsOff); err != nil {
			return err
		}
		var err *cffErr
		if p.lsubrs, err = src.readIndex(); err != nil {
			return err
		}
		if err := src.seek(int64(at)); err != nil {
			return err
		}
		if p.charStringType == 1 {
			p.bias = 0
		} else {
			p.bias = cffBias(p.lsubrs.count)
		}
	}
	if err := src.seek(p.charStringsOff); err != nil {
		return err
	}
	cs, err := src.readIndex()
	if err != nil {
		return err
	}
	prog.charStrings, prog.amount = cs, cs.count
	if err := src.seek(p.encodingOff); err != nil {
		return err
	}
	if err := p.readEncoding(prog); err != nil {
		return err
	}
	if err := src.seek(p.charSetOff); err != nil {
		return err
	}
	if err := p.readCharSet(prog); err != nil {
		return err
	}
	w, err := newCFFWidths(p, cs)
	if err != nil {
		return err
	}
	prog.widths = w
	return nil
}

// readEncoding is `readEncoding`. **A supplement stores the SID where the glyph index belongs** (`encoding[code] =
// glyph`), which the lookup then reads as GID SID+1 — veraPDF's, mirrored.
func (p *cffParser) readEncoding(prog *cffProgram) *cffErr {
	switch p.encodingOff {
	case 0:
		prog.std = true
		return nil
	case 1:
		prog.expert = true
		return nil
	}
	src := p.src
	format, err := src.card8()
	if err != nil {
		return err
	}
	supplements := false
	switch format {
	case 0, 128:
		n, err := src.card8()
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			code, err := src.card8()
			if err != nil {
				return err
			}
			prog.encoding[code] = i
		}
		supplements = format == 128
	case 1, 129:
		n, err := src.card8()
		if err != nil {
			return err
		}
		ptr := 0
		for i := 0; i < n; i++ {
			first, err := src.card8()
			if err != nil {
				return err
			}
			left, err := src.card8()
			if err != nil {
				return err
			}
			for j := 0; j <= left; j++ {
				if first+j < len(prog.encoding) {
					prog.encoding[first+j] = ptr
					ptr++
				}
			}
		}
		supplements = format == 129
	}
	if supplements {
		n, err := src.card8()
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			code, err := src.card8()
			if err != nil {
				return err
			}
			sid, err := src.card16()
			if err != nil {
				return err
			}
			prog.encoding[code] = sid
		}
	}
	return nil
}

// readCharSet is `readCharSet`: GID 0 is `.notdef` (SID 0); a predefined charset is indexed up to the glyph count with no
// bound (past its table an ArrayIndexOutOfBoundsException nothing catches); format 0 reads nGlyphs-1 SIDs; formats 1 and
// 2 read ranges until the count is reached, the last range running PAST it; a later duplicate name wins.
func (p *cffParser) readCharSet(prog *cffProgram) *cffErr {
	prog.charSet = map[string]int{}
	prog.inverse = map[int]string{}
	put := func(name string, gid int) {
		prog.charSet[name] = gid
		prog.inverse[gid] = name
	}
	notdef, _ := p.stringBySID(0)
	put(notdef, 0)
	n := prog.amount
	var table []string
	switch p.charSetOff {
	case 0:
		table = cffISOAdobeCharset[:]
	case 1:
		table = cffExpertCharset[:]
	case 2:
		table = cffExpertSubsetCharset[:]
	}
	if table != nil {
		if n > len(table) {
			return throwErr("its CFF names the predefined charset %d but holds %d glyphs, past the table's %d, where "+
				"veraPDF throws an ArrayIndexOutOfBoundsException it does not handle", p.charSetOff, n, len(table))
		}
		for i := 0; i < n; i++ {
			put(table[i], i)
		}
	} else {
		src := p.src
		format, err := src.card8()
		if err != nil {
			return err
		}
		switch format {
		case 0:
			for i := 1; i < n; i++ {
				sid, err := src.card16()
				if err != nil {
					return err
				}
				name, err := p.stringBySID(sid)
				if err != nil {
					return err
				}
				put(name, i)
			}
		case 1, 2:
			for ptr := 1; ptr < n; {
				first, err := src.card16()
				if err != nil {
					return err
				}
				var left int
				if format == 1 {
					left, err = src.card8()
				} else {
					left, err = src.card16()
				}
				if err != nil {
					return err
				}
				for i := 0; i <= left; i++ {
					name, err := p.stringBySID(first + i)
					if err != nil {
						return err
					}
					put(name, ptr)
					ptr++
				}
			}
		default:
			return ioErr("a charset of format %d", format)
		}
	}
	prog.inverseSize = len(prog.inverse)
	return nil
}

// cffWidths is `CharStringsWidths` for a single-dictionary (non-CID) font: a subset font's widths all computed at parse,
// a full font's per glyph when asked.
type cffWidths struct {
	p      *cffParser
	cs     cffIndex
	bigCS  bool // the CharStrings data is ≥ 10240 bytes: read from the stream by computed offsets (`CFFCharStringsHandler`)
	csBase int64
	subset []float32
	full   map[int]float32
	spent  int            // charstring bytes read across every width, against `cffMaxProgramBytes`
	errs   map[int]string // a glyph whose reading nib refuses, or on which veraPDF throws
	isDef  bool
}

func newCFFWidths(p *cffParser, cs cffIndex) (*cffWidths, *cffErr) {
	w := &cffWidths{p: p, cs: cs, bigCS: len(cs.data) >= cffBuffered, csBase: p.charStringsOff + int64(cs.shift) - 1,
		full: map[int]float32{}, errs: map[int]string{}}
	def := [6]float32{0.001, 0, 0, 0.001, 0, 0}
	w.isDef = true
	for i := range def { // `Arrays.equals(float[], float[])`: by bits, so -0.0 is not 0.0
		if math.Float32bits(p.fontMatrix[i]) != math.Float32bits(def[i]) {
			w.isDef = false
		}
	}
	if p.subset {
		w.subset = make([]float32, cs.count)
		for i := range w.subset {
			v, err := w.compute(i)
			switch {
			case err != nil && err.kind == errThrow:
				return nil, err // veraPDF throws at parse: it reports nothing
			case err != nil:
				w.errs[i] = err.why // a refusal is ONE glyph's, never the program's
			}
			w.subset[i] = v
		}
	}
	return w, nil
}

// width is `CharStringsWidths.getWidth`: a subset font's precomputed width (-1 for a GID it does not hold), a full
// font's computed on first ask for ANY GID — one it does not hold reads an empty charstring and takes defaultWidthX.
func (w *cffWidths) width(gid int) (float32, *cffErr) {
	if why, refused := w.errs[gid]; refused {
		return 0, refuse("%s", why)
	}
	if w.p.charStringType == 1 {
		// Type 1 charstrings inside a CFF go through veraPDF's `Type1CharStringParser`, which nib does not mirror; the
		// program still parses, so presence and the charset answer and only a width refuses (measured). The charstring
		// is FETCHED first, as veraPDF fetches it, so a fetch that throws still reaches the door.
		if _, err := w.charString(gid); err != nil && err.kind == errThrow {
			return 0, err
		}
		return 0, refuse("its CFF charstrings are of type 1, which nib does not read for widths")
	}
	if w.subset != nil {
		if gid >= 0 && gid < len(w.subset) {
			return w.subset[gid], nil
		}
		return -1, nil
	}
	if v, ok := w.full[gid]; ok {
		return v, nil
	}
	v, err := w.compute(gid)
	if err != nil {
		if err.kind == errRefuse {
			w.errs[gid] = err.why // asked again, it refuses again without spending the budget again
		}
		return 0, err
	}
	w.full[gid] = v
	return v, nil
}

// compute is `getWidthFromCharstring` then `getActualWidth`: an IOException or ArrayIndexOutOfBoundsException while
// reading the charstring is the width -1 — to which nominalWidthX is STILL added, and the matrix scale applied.
func (w *cffWidths) compute(gid int) (float32, *cffErr) {
	var res float32
	width, found, err := w.charstringWidth(gid)
	switch {
	case err != nil && err.kind != errIO:
		return 0, err
	case err != nil:
		res = -1 + float32(w.p.nominalWidthX)
	case !found:
		res = float32(w.p.defaultWidthX)
	case width.isInt:
		res = float32(width.integer) + float32(w.p.nominalWidthX)
	default:
		res = width.real + float32(w.p.nominalWidthX)
	}
	if !w.isDef {
		res *= w.p.fontMatrix[0] * 1000
	}
	return res, nil
}

// charString is `CFFCharStringsHandler.getCharString`: past the count an empty charstring; a small CharStrings INDEX
// read through `CFFIndex.get`; a large one from the stream at computed offsets — a negative length there a
// NegativeArraySizeException nothing catches, a short read zero-padded.
func (w *cffWidths) charString(gid int) ([]byte, *cffErr) {
	if gid < 0 || gid >= w.cs.count {
		return nil, nil
	}
	if !w.bigCS {
		return w.cs.get(gid)
	}
	from := w.csBase + int64(w.cs.offsets[gid])
	to := w.csBase + int64(w.cs.offsets[gid+1])
	src := &cffSrc{b: w.p.src.b}
	if err := src.seek(from); err != nil { // the seek comes first in `getCharString`
		return nil, err
	}
	n := int(int32(to - from))
	switch {
	case n < 0:
		return nil, throwErr("a charstring of negative length, where veraPDF throws a NegativeArraySizeException it does not handle")
	case n > cffMaxAlloc:
		return nil, refuse("a charstring of %d bytes, which veraPDF allocates and nib will not", n)
	}
	if pad := n - (len(src.b) - src.pos); pad > 0 { // the zero-padded tail counts against the program's budget too
		if w.p.src.padded += pad; w.p.src.padded > cffMaxAlloc {
			return nil, refuse("its charstrings ask for %d bytes past the program, which veraPDF allocates and nib will not", w.p.src.padded)
		}
	}
	out := make([]byte, n)
	copy(out, src.b[src.pos:min(src.pos+n, len(src.b))])
	return out, nil
}

// cffMaxSubrDepth bounds nested subroutine calls. veraPDF has none — a subroutine calling itself grows its stream
// stack until the JVM runs out of memory — so past this nib refuses rather than answer for veraPDF.
const cffMaxSubrDepth = 64

// cffMaxCharstringBytes bounds the bytes one width reads across its subroutines, for the same reason, and
// cffMaxProgramBytes the bytes every width of one program reads together — a subset font computes them all at parse,
// and a tree of subroutines costs ~24 ms a glyph (measured by the P07.S05a review), so 65,535 glyphs are minutes.
const (
	cffMaxCharstringBytes = 1 << 20
	cffMaxProgramBytes    = 1 << 25
)

// charstringWidth is `Type2CharStringParser`: numbers pushed, the FIRST operator other than a subroutine call or return
// ends the parse, and only a stem, mask, move or endchar with the right operand count sets the width, as stack[0].
func (w *cffWidths) charstringWidth(gid int) (cffNumber, bool, *cffErr) {
	body, err := w.charString(gid) // fetched FIRST, whatever the type: its exceptions are veraPDF's too
	if err != nil {
		return cffNumber{}, false, err
	}
	switch w.p.charStringType {
	case 1:
		return cffNumber{}, false, nil // `width` refuses before a type-1 width is ever used
	case 2:
	default: // "Can't process CharString of type N": an IOException, caught — the width -1
		return cffNumber{}, false, ioErr("charstrings of type %d", w.p.charStringType)
	}
	streams := [][]byte{body}
	var stack []cffNumber
	read := 0
	next := func() (byte, bool) {
		for len(streams) > 0 {
			top := streams[len(streams)-1]
			if len(top) > 0 {
				streams[len(streams)-1] = top[1:]
				read++
				return top[0], true
			}
			streams = streams[:len(streams)-1]
		}
		return 0, false
	}
	// `readStreams(buf, n)`: bytes from the stream stack, a short read leaving zeros.
	bytesN := func(n int) []byte {
		out := make([]byte, n)
		for i := range out {
			c, ok := next()
			if !ok {
				break
			}
			out[i] = c
		}
		return out
	}
	subr := func(subrs cffIndex, bias int) *cffErr {
		if len(stack) == 0 {
			return nil
		}
		num := int(int32(stack[len(stack)-1].getInteger()))
		stack = stack[:len(stack)-1]
		idx := num + bias
		if subrs.count > max(idx, 0) {
			b, err := subrs.get(idx)
			if err != nil {
				return err
			}
			if len(streams) >= cffMaxSubrDepth {
				return refuse("its charstrings nest subroutines deeper than %d, where veraPDF has no bound", cffMaxSubrDepth)
			}
			streams = append(streams, b)
		}
		return nil
	}
	defer func() { w.spent += read }()
	for {
		if read > cffMaxCharstringBytes {
			return cffNumber{}, false, refuse("a charstring reads more than %d bytes through its subroutines", cffMaxCharstringBytes)
		}
		if w.spent+read > cffMaxProgramBytes {
			return cffNumber{}, false, refuse("its charstrings read more than %d bytes through their subroutines", cffMaxProgramBytes)
		}
		c, ok := next()
		if !ok {
			return cffNumber{}, false, nil
		}
		b := int(c)
		switch {
		case b > 31 && b < 247:
			stack = append(stack, cffInt(b-139))
			continue
		case b > 246 && b < 251:
			stack = append(stack, cffInt((b-247)<<8+int(bytesN(1)[0])+108))
			continue
		case b > 250 && b < 255:
			stack = append(stack, cffInt(-((b-251)<<8)-int(bytesN(1)[0])-108))
			continue
		case b == 255:
			x := bytesN(4)
			v := int32(uint32(x[0])<<24 | uint32(x[1])<<16 | uint32(x[2])<<8 | uint32(x[3]))
			stack = append(stack, cffReal(float32(v)/65536))
			continue
		case b == 28:
			x := bytesN(2)
			stack = append(stack, cffInt(int(int16(uint16(x[0])<<8|uint16(x[1])))))
			continue
		case b == 10:
			if err := subr(w.p.lsubrs, w.p.bias); err != nil {
				return cffNumber{}, false, err
			}
			continue
		case b == 29:
			if err := subr(w.p.gsubrs, cffBias(w.p.gsubrs.count)); err != nil {
				return cffNumber{}, false, err
			}
			continue
		case b == 11:
			continue
		}
		n := len(stack)
		set := false
		switch b {
		case 19, 20, 1, 3, 18, 23:
			set = n%2 == 1
		case 14:
			set = n != 0 && n != 4
		case 4, 22:
			set = n > 1
		case 21:
			set = n > 2
		}
		if set {
			return stack[0], true, nil
		}
		return cffNumber{}, false, nil
	}
}

// subsetFont is `PDFont.isSubset`: the BaseFont's part before its first `+` is six characters long — `split("\\+")`, so a
// name of only `+` signs splits into nothing and the index throws, which `parseType1FontProgram` catches as "no program".
func subsetFont(base string) (subset, ok bool) {
	if !utf8.ValidString(base) {
		return false, false // decided by the caller, which refuses: Java's decoder and Go's replace bad bytes differently
	}
	if base != "" && strings.Trim(base, "+") == "" {
		return false, false
	}
	first, _, _ := strings.Cut(base, "+")
	return javaLength(first) == 6, true
}

// javaLength is a Java String's `length()` of the UTF-8 `getNameKeyUnicodeValue` decodes: UTF-16 units, a supplementary
// character counting two and an invalid byte one (its replacement character).
func javaLength(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// glyphName is `CFFType1FontProgram.getGlyphName` — under the Standard or Expert encoding the encoding's own name,
// otherwise the charset's name for GID encoding[code]+1, `.notdef` past it.
func (c *cffProgram) glyphName(code int) string {
	switch {
	case code < 0:
		return ".notdef"
	case c.std:
		if code < len(cffStandardEncoding) {
			return cffStandardStrings[cffStandardEncoding[code]]
		}
	case c.expert:
		if code < len(cffExpertEncoding) {
			return cffStandardStrings[cffExpertEncoding[code]]
		}
	default:
		if code < 256 {
			if gid := c.encoding[code] + 1; gid < c.inverseSize {
				return c.inverse[gid]
			}
		}
	}
	return ".notdef"
}

// containsCode is `containsCode`: the charset holds the name `glyphName` gives.
func (c *cffProgram) containsCode(code int) bool {
	_, ok := c.charSet[c.glyphName(code)]
	return ok
}

// widthByGID is `getWidthByGID`: a GID outside the charstrings is GID 0's width.
func (c *cffProgram) widthByGID(gid int, held bool) (float32, *cffErr) {
	if !held || gid < 0 || gid >= c.amount {
		return c.widths.width(0)
	}
	return c.widths.width(gid)
}

// widthOfName is `getWidth(String)`.
func (c *cffProgram) widthOfName(name string) (float32, *cffErr) {
	gid, held := c.charSet[name]
	return c.widthByGID(gid, held)
}

// widthOfCode is `getWidth(int)` for a simple font: a custom encoding's GID, else through the encoding's name.
func (c *cffProgram) widthOfCode(code int) (float32, *cffErr) {
	if !c.std && !c.expert && code >= 0 && code < 256 {
		return c.widthByGID(c.encoding[code]+1, true)
	}
	return c.widthOfName(c.glyphName(code))
}
