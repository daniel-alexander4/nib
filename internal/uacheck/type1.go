package uacheck

import (
	"bytes"
	"fmt"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// A simple Type 1 font's embedded Type 1 program (/FontFile) as veraPDF 1.30.2 reads it — `PLAN-ua-coverage.md` P07.S06,
// ported from `Type1FontProgram`, `Type1PrivateParser`, `EexecFilterDecode`, `Type1CharStringParser`, `PSParser`,
// `PSOperator` and `COSParser`, quirks included, and measured against veraPDF on pdfLaTeX output and mutations of it
// (`type1_test.go`):
//
//   - the WHOLE decoded stream is read (/Length1-3 are not), through a PostScript interpreter that executes only the
//     operators in `OPERATORS_KEYWORDS` at the top level and IGNORES any other it cannot find in its one user dictionary
//     (so `ifelse` and `known` do nothing and their operands stay on the stack);
//   - `eexec` (the operator or the literal name) hands the bytes up to and including `cleartomark` — or to the end,
//     less its last byte — zero-padded to a multiple of 10240, to eexec decryption (r 55665, 4 bytes dropped), and the
//     result to a token-level private parser that finds /lenIV, /Subrs and /CharStrings;
//   - each charstring is decrypted (r 4330, lenIV bytes dropped) and read only to its first width: `hsbw`, `sbw`, or a
//     `callsubr` naming a subroutine whose own charstring recorded one. A glyph with no width is not in the program;
//   - a non-default /FontMatrix scales a width by its first entry (`(int)(w * m0 * 1000)`);
//   - /Encoding is an array of names (anything else "") or /StandardEncoding; codes it does not name are null.
//
// The three outcomes are `cffProgram`'s: parsed; not parsed (veraPDF's parse threw an IOException, or no charstrings
// were found — the font then counts as not embedded, and 7.21.4.2 t1 fails on it); a runtime exception veraPDF does not
// catch (the document reports nothing); and a refusal where nib does not mirror, or would have to spend more than its
// bounds allow — veraPDF has no bounds, and an attacker writes the program.

// type1Program is what the clauses read of a parsed Type 1 program.
type type1Program struct {
	state  ttState
	why    string
	throws string
	widths map[string]int32 // `glyphWidths`: glyph name → width
	enc    [256]string      // `encoding`; encSet marks the entries that are not Java's null
	encSet [256]bool
}

// containsGlyph is `containsGlyph`: in the widths, and never `.notdef`.
func (p *type1Program) containsGlyph(name string) bool {
	_, ok := p.widths[name]
	return ok && name != ".notdef"
}

// glyphName is `getGlyphName(code)`: the program encoding's entry, false for null.
func (p *type1Program) glyphName(code int) (string, bool) {
	if code < 0 || code >= 256 || !p.encSet[code] {
		return "", false
	}
	return p.enc[code], true
}

// containsCode is `containsCode(code)`: `getGlyph(code)` — past 255 `.notdef` — then `containsGlyph`.
func (p *type1Program) containsCode(code int) bool {
	if code >= 256 {
		return false // .notdef
	}
	n, ok := p.glyphName(code)
	return ok && p.containsGlyph(n)
}

// widthOfName is `getWidth(String)`: -1 for a name the widths do not hold. The Integer is widened to a float.
func (p *type1Program) widthOfName(name string) float32 {
	if w, ok := p.widths[name]; ok {
		return float32(w)
	}
	return -1
}

// widthOfCode is `getWidth(int)`.
func (p *type1Program) widthOfCode(code int) float32 {
	if code >= 256 {
		return p.widthOfName(".notdef")
	}
	n, ok := p.glyphName(code)
	if !ok {
		return -1
	}
	return p.widthOfName(n)
}

// Bounds veraPDF does not have. Past any of them nib refuses rather than hang or exhaust memory on a hostile program.
// t1MaxOps, t1MaxAlloc, t1MaxDecrypt and t1MaxSteps bound the DOCUMENT's Type 1 programs together (`t1Spend`), as
// `maxTrueTypeReads` bounds its TrueType ones: as a program's they let N programs cost N times the bound (the P07
// phase-close review measured 66 ms for an 862-byte program reaching t1MaxOps — ten thousand of them are minutes).
// The others bound one program's own state.
const (
	t1MaxBytes = 1 << 26 // the decoded /FontFile stream
	t1MaxOps   = 1 << 22 // PostScript objects executed, procedures' bodies included
	t1MaxStack = 1 << 20 // the operand stack's depth
	t1MaxAlloc = 1 << 21 // array slots created by `array`, `copy`, `roll` and the parser, over the whole program (a slot is
	// 56 bytes, so ~117 MB; veraPDF caps one array at 65,536 and a real font's cleartext builds a few hundred)
	t1MaxNest    = 256     // arrays and procedures nested while parsing, and operator-to-operator execution
	t1MaxDecrypt = 1 << 27 // charstring and subroutine bytes decrypted
	t1MaxSteps   = 1 << 24 // private-parser tokens read
)

// psKind is the COS type a PostScript object has; psMark and psEmpty have no base (`COS_UNDEFINED`, `empty()`).
type psKind uint8

const (
	psEmpty psKind = iota
	psMark
	psInt
	psReal
	psBool
	psNull
	psName
	psOp
	psStr
	psArr
	psProc
	psDict
)

// psObj is a COSObject as the interpreter holds it. An array and a procedure share their elements by reference.
type psObj struct {
	k psKind
	i int64
	r float64
	b bool
	s string
	a *psArray
}

type psArray struct{ e []psObj }

func (o psObj) isNumber() bool { return o.k == psInt || o.k == psReal }
func (o psObj) isName() bool   { return o.k == psName || o.k == psOp }
func (o psObj) isArray() bool  { return o.k == psArr || o.k == psProc }
func (o psObj) empty() bool    { return o.k == psEmpty || o.k == psMark }

func (o psObj) real() float64 {
	if o.k == psInt {
		return float64(o.i)
	}
	return o.r
}

func (o psObj) integer() int64 {
	if o.k == psInt {
		return o.i
	}
	return javaD2L(o.r)
}

// str is `getString()` for a name or a string, and whether there is one. A string is PDFDocEncoding-decoded; nib takes
// its bytes where they are printable ASCII (identical under that decoding) and refuses otherwise.
func (o psObj) str() (string, bool) {
	switch o.k {
	case psName, psOp:
		return o.s, true
	case psStr:
		for i := 0; i < len(o.s); i++ {
			if o.s[i] < 0x20 || o.s[i] > 0x7e {
				t1Fail(errRefuse, "a string it reads as a name holds bytes outside printable ASCII, which veraPDF "+
					"decodes as PDFDocEncoding or UTF-16 and nib does not")
			}
		}
		return o.s, true
	}
	return "", false
}

// t1Reader is one parse of one program: the cleartext's parser and interpreter, and the results.
type t1Reader struct {
	src *bpBufSrc
	lx  *bpLexer

	objects  []psObj // `COSParser.objects`
	integers []int64 // `COSParser.integers`
	flag     bool
	nest     int

	stack []psObj
	dict  map[string]psObj

	spend *t1Spend // the document's

	widths    map[string]int32
	hasWidths bool
}

// t1Spend is what the document's Type 1 programs have cost, all of them together.
type t1Spend struct{ ops, alloc, decrypted, steps int }

// exhausted names the document budget a Type 1 read went past, "" while none has: past one, the program being read
// and every later read that charges that counter are refused unread (RR1-6, `reportsNothing`).
func (s t1Spend) exhausted() string {
	switch {
	case s.ops > t1MaxOps:
		return fmt.Sprintf("the document's Type 1 programs execute more than %d PostScript objects", t1MaxOps)
	case s.alloc > t1MaxAlloc:
		return fmt.Sprintf("the document's Type 1 programs build more than %d array slots", t1MaxAlloc)
	case s.decrypted > t1MaxDecrypt:
		return fmt.Sprintf("the document's Type 1 programs decrypt more than %d bytes", t1MaxDecrypt)
	case s.steps > t1MaxSteps:
		return fmt.Sprintf("the document's Type 1 programs' private dictionaries read more than %d tokens", t1MaxSteps)
	}
	return ""
}

// readType1 is `Type1FontProgram.parseFont` over the decoded stream, charged to the document's budget `spend` (nil is a
// fresh one).
func readType1(data []byte, spend *t1Spend) (p type1Program) {
	if spend == nil {
		spend = &t1Spend{}
	}
	defer func() {
		if r := recover(); r != nil {
			a, ok := r.(t1Abort)
			if !ok {
				panic(r)
			}
			switch a.err.kind {
			case errIO:
				p = type1Program{state: ttFailed, why: a.err.why}
			case errThrow:
				p = type1Program{state: ttFailed, why: a.err.why, throws: "its Type 1 program makes veraPDF throw an " +
					"exception it does not handle (" + a.err.why + ")"}
			default:
				p = type1Program{state: ttUnknown, why: a.err.why}
			}
		}
	}()
	if len(data) > t1MaxBytes {
		t1Fail(errRefuse, fmt.Sprintf("its Type 1 program is %d bytes, past nib's bound of %d", len(data), t1MaxBytes))
	}
	r := &t1Reader{src: newBufSrc(data), dict: map[string]psObj{}, flag: true, spend: spend}
	r.lx = &bpLexer{src: r.src, ps: true}
	// The PFB probe: two bytes read and both given back.
	if r.src.readByte() == 0x80 {
		r.src.readByte()
		r.src.unread()
	}
	r.src.unread()
	r.lx.skipSpaces(true)
	obj := r.nextObject()
	for r.lx.typ != bpTEOF {
		r.processObject(obj)
		obj = r.nextObject()
	}
	p = type1Program{state: ttParsed}
	r.initializeEncoding(&p)
	if !r.hasWidths {
		t1Fail(errIO, "Type 1 font doesn't contain charstrings.")
	}
	p.widths = r.widths
	return p
}

// nextObject is `COSParser.nextObject` in PostScript mode, with `PSParser`'s dictionary overrides.
func (r *t1Reader) nextObject() psObj {
	if len(r.objects) > 0 {
		o := r.objects[0]
		r.objects = r.objects[1:]
		return o
	}
	if r.flag {
		r.lx.next()
	}
	r.flag = true
	lx := r.lx
	if lx.typ == bpTInteger {
		r.integers = append(r.integers, lx.integer)
		if len(r.integers) == 3 {
			v := r.integers[0]
			r.integers = r.integers[1:]
			return psObj{k: psInt, i: v}
		}
		return r.nextObject()
	}
	if lx.typ == bpTKeyword && lx.kw == "R" && len(r.integers) == 2 {
		t1Fail(errRefuse, "its cleartext holds an indirect reference, which veraPDF builds against no document and nib "+
			"does not mirror")
	}
	if len(r.integers) > 0 {
		v := r.integers[0]
		for _, n := range r.integers[1:] {
			r.objects = append(r.objects, psObj{k: psInt, i: n})
		}
		r.integers = r.integers[:0]
		r.flag = false
		return psObj{k: psInt, i: v}
	}
	if lx.a85 && (lx.typ == bpTHexString) {
		t1Fail(errRefuse, "its cleartext holds an ASCII85 string, which nib does not decode")
	}
	switch lx.typ {
	case bpTKeyword:
		switch lx.kw {
		case "":
			return psObj{k: psOp, s: lx.value()}
		case "null":
			return psObj{k: psNull}
		case "true":
			return psObj{k: psBool, b: true}
		case "false":
			return psObj{k: psBool}
		}
	case bpTReal:
		return psObj{k: psReal, r: lx.real}
	case bpTLitString, bpTHexString:
		return psObj{k: psStr, s: lx.value()}
	case bpTName:
		return psObj{k: psName, s: lx.value()}
	case bpTOpenArray:
		r.flag = false
		return r.getArray(psArr)
	case bpTOpenDict: // `getDictionary`: the flag set again, and the NAME `<<` — a literal, never the operator
		r.flag = true
		return psObj{k: psName, s: "<<"}
	case bpTCloseDict:
		return psObj{k: psName, s: ">>"}
	case bpTStartProc:
		r.flag = false
		return r.getArray(psProc)
	}
	return psObj{}
}

// getArray is `COSParser.getArray`, entered with the flag cleared on an open bracket or brace: elements to the first
// empty object — a close of either kind ends either — and an IOException unless that close was one.
func (r *t1Reader) getArray(k psKind) psObj {
	if r.flag {
		r.lx.next()
	}
	r.flag = true
	if r.nest++; r.nest > t1MaxNest {
		t1Fail(errRefuse, fmt.Sprintf("its cleartext nests arrays and procedures past nib's bound of %d", t1MaxNest))
	}
	arr := &psArray{}
	for o := r.nextObject(); !o.empty(); o = r.nextObject() {
		arr.e = append(arr.e, o)
		r.charge(0, 1)
	}
	r.nest--
	if r.lx.typ != bpTCloseArray && r.lx.typ != bpTEndProc {
		t1Fail(errIO, "Invalid PDF array")
	}
	return psObj{k: k, a: arr}
}

// chargeKey charges a user-dictionary access its key's length: veraPDF hashes and compares the whole key
// (`ASAtom.getASAtom`), so one access to a key of L bytes costs L — one object per 64 bytes, and at least one.
func (r *t1Reader) chargeKey(k string) { r.charge(len(k)/64, 0) }

// lookup is a user-dictionary read, charged its key.
func (r *t1Reader) lookup(k string) (psObj, bool) {
	r.chargeKey(k)
	v, ok := r.dict[k]
	return v, ok
}

// charge counts executed objects and allocated slots against nib's bounds.
func (r *t1Reader) charge(ops, alloc int) {
	r.spend.ops += ops
	r.spend.alloc += alloc
	if r.spend.ops > t1MaxOps {
		t1Fail(errRefuse, fmt.Sprintf("the document's Type 1 programs execute more than %d PostScript objects, nib's bound", t1MaxOps))
	}
	if r.spend.alloc > t1MaxAlloc {
		t1Fail(errRefuse, fmt.Sprintf("the document's Type 1 programs build more than %d array slots, nib's bound", t1MaxAlloc))
	}
}

// t1Keywords is `OPERATORS_KEYWORDS`: the operators the top level executes.
var t1Keywords = map[string]bool{"abs": true, "floor": true, "mod": true, "add": true, "idiv": true, "mul": true,
	"div": true, "neg": true, "sub": true, "ceiling": true, "round": true, "copy": true, "exch": true, "pop": true,
	"dup": true, "index": true, "roll": true, "clear": true, "count": true, "mark": true, "cleartomark": true,
	"counttomark": true, "dict": true, "begin": true, "length": true, "def": true, "load": true, "array": true,
	"put": true, "for": true, "StandardEncoding": true, "<<": true, ">>": true}

// processObject is `processObject`.
func (r *t1Reader) processObject(o psObj) {
	if o.isName() && o.s == "eexec" {
		r.eexec()
		return
	}
	r.toExecute(o, 0)
}

// toExecute is `toExecute(next, depth)`.
func (r *t1Reader) toExecute(o psObj, depth int) {
	if depth > 64 {
		t1Fail(errIO, "Type 1 font program exceeded toExecute recursion depth")
	}
	r.charge(1, 0)
	if o.k != psOp {
		r.execLiteral(o)
		return
	}
	if !t1Keywords[o.s] {
		if e, ok := r.lookup(o.s); ok {
			r.toExecute(e, depth+1)
		}
		return
	}
	r.execOp(o.s, 0)
}

// execLiteral is `getPSObject(obj).execute` for anything but an operator: a procedure pushes itself, an object with no
// base (an empty slot, a mark) pushes nothing, and anything else pushes itself.
func (r *t1Reader) execLiteral(o psObj) {
	if !o.empty() {
		r.push(o)
	}
}

func (r *t1Reader) push(o psObj) {
	if len(r.stack) >= t1MaxStack {
		t1Fail(errRefuse, fmt.Sprintf("its operand stack grows past nib's bound of %d", t1MaxStack))
	}
	r.stack = append(r.stack, o)
}

// pop is `Stack.pop`: an EmptyStackException veraPDF does not catch.
func (r *t1Reader) pop() psObj {
	if len(r.stack) == 0 {
		t1Fail(errThrow, "EmptyStackException")
	}
	o := r.stack[len(r.stack)-1]
	r.stack = r.stack[:len(r.stack)-1]
	return o
}

func (r *t1Reader) peek() psObj {
	if len(r.stack) == 0 {
		t1Fail(errThrow, "EmptyStackException")
	}
	return r.stack[len(r.stack)-1]
}

// popTop is `popTopObject`: a PostScriptException on an empty stack.
func (r *t1Reader) popTop() psObj {
	if len(r.stack) == 0 {
		t1Fail(errIO, "Operand stack is empty")
	}
	return r.pop()
}

func (r *t1Reader) topNumber() psObj {
	o := r.popTop()
	if !o.isNumber() {
		t1Fail(errIO, "Stack is empty or top element is not a number")
	}
	return o
}

func (r *t1Reader) topTwo(want func(psObj) bool) (a, b psObj) {
	if len(r.stack) > 1 {
		if a = r.pop(); want(a) {
			if b = r.pop(); want(b) {
				return a, b
			}
		}
	}
	t1Fail(errIO, "Stack doesn't have two elements of the kind the operator needs")
	return
}

// psCopy is `psCopyObject`: scalars copied, everything else shared.
func psCopy(o psObj) psObj {
	if o.k == psOp {
		o.k = psName
	}
	return o
}

// execProcedure is `PSProcedure.executeProcedure`: each element executed as `getPSObject(obj).execute`.
func (r *t1Reader) execProcedure(p psObj, depth int) {
	for _, o := range p.a.e {
		r.charge(1, 0)
		if o.k == psOp {
			r.execOp(o.s, depth+1)
		} else {
			r.execLiteral(o)
		}
	}
}

// modifiedExecute is `modifiedExecuteProcedure` (`if`, `ifelse`): an operator executed, anything else pushed as is.
func (r *t1Reader) modifiedExecute(p psObj, depth int) {
	for _, o := range p.a.e {
		r.charge(1, 0)
		if o.k == psOp {
			r.execOp(o.s, depth+1)
		} else {
			r.push(o)
		}
	}
}

// execOp is `PSOperator.execute`.
func (r *t1Reader) execOp(op string, depth int) {
	if depth > t1MaxNest {
		t1Fail(errRefuse, fmt.Sprintf("its procedures nest execution past nib's bound of %d", t1MaxNest))
	}
	isBool := func(o psObj) bool { return o.k == psBool }
	switch op {
	case "{", "}":
	case "if":
		if len(r.stack) < 2 {
			t1Fail(errIO, "No procedures for if operator")
		}
		p, b := r.pop(), r.pop()
		if p.k != psProc || b.k != psBool {
			t1Fail(errIO, "Can't execute if operator")
		}
		if b.b {
			r.modifiedExecute(p, depth)
		}
	case "ifelse":
		if len(r.stack) < 3 {
			t1Fail(errIO, "No procedures for ifelse operator")
		}
		f, t, b := r.pop(), r.pop(), r.pop()
		if f.k != psProc || t.k != psProc || b.k != psBool {
			t1Fail(errIO, "Can't execute ifelse operator")
		}
		if b.b {
			r.modifiedExecute(t, depth)
		} else {
			r.modifiedExecute(f, depth)
		}
	case "dup":
		r.push(psCopy(r.peek()))
	case "exch":
		a, b := r.pop(), r.pop()
		r.push(a)
		r.push(b)
	case "pop":
		r.pop()
	case "copy":
		n := r.topNumber()
		size := int64(len(r.stack))
		if size < n.integer() {
			t1Fail(errIO, "Can't execute copy operator")
		}
		from := int64(len(r.stack)) - int64(int32(n.integer()))
		if from < 0 || from > size {
			t1Fail(errThrow, "copy's subList is out of range")
		}
		r.charge(0, int(size-from))
		for _, o := range append([]psObj(nil), r.stack[from:]...) {
			r.push(psCopy(o))
		}
	case "index":
		n := int64(int32(r.topNumber().integer()))
		size := int64(len(r.stack))
		if size < n {
			t1Fail(errIO, "Can't execute index operator")
		}
		i := size - n - 1
		if i < 0 || i >= size {
			t1Fail(errThrow, "index reads outside the stack")
		}
		r.push(psCopy(r.stack[i]))
	case "roll":
		r.roll()
	case "clear":
		r.stack = r.stack[:0]
	case "count":
		r.push(psObj{k: psInt, i: int64(len(r.stack))})
	case "mark":
		r.push(psObj{k: psMark})
	case "cleartomark":
		for top := r.peek(); len(r.stack) > 0 && top.k != psMark; top = r.peek() {
			r.pop()
		}
	case "counttomark":
		var n int64
		for i := len(r.stack) - 1; i >= 0; i-- {
			r.charge(1, 0) // the scan is the operator's cost, and a stack of a million makes it one
			if r.stack[i].k == psMark || i == 0 {
				r.push(psObj{k: psInt, i: n})
				return
			}
			n++
		}
	case "and", "or", "xor":
		a, b := r.topTwo(isBool)
		v := map[string]bool{"and": b.b && a.b, "or": b.b || a.b, "xor": b.b != a.b}[op]
		r.push(psObj{k: psBool, b: v})
	case "not":
		o := r.popTop()
		if o.k != psBool {
			t1Fail(errIO, "Stack is empty or top element is not a boolean")
		}
		r.push(psObj{k: psBool, b: !o.b})
	case "true":
		r.push(psObj{k: psBool, b: true})
	case "false":
		r.push(psObj{k: psBool})
	case "bitshift", "abs", "neg", "ceiling", "floor", "round", "truncate", "sqrt", "cos", "atan", "sin", "exp", "cvi",
		"cvr", "ln", "log":
		r.push(psOneNumber(op, r.topNumber()))
	case "add", "div", "idiv", "mod", "mul", "sub", "eq", "ne", "gt", "ge", "lt", "le":
		a, b := r.topTwo(psObj.isNumber)
		r.push(psTwoNumbers(op, a, b))
	case "dict":
		r.topNumber()
		r.push(psObj{k: psDict})
	case "begin":
		if len(r.stack) > 0 {
			r.pop()
		}
	case "length":
		o := r.popTop()
		switch {
		case o.k == psDict:
			r.push(psObj{k: psInt}) // no key is ever set in a dictionary the interpreter builds
		case o.isArray():
			r.push(psObj{k: psInt, i: int64(len(o.a.e))})
		default:
			t1Fail(errIO, "Can't execute length operator")
		}
	case "def":
		if len(r.stack) > 1 {
			v, k := r.pop(), r.pop()
			if k.isName() || k.k == psStr {
				s, _ := k.str()
				r.chargeKey(s)
				r.dict[s] = v
			}
		}
	case "load":
		k := r.popTop()
		if !k.isName() && k.k != psStr {
			t1Fail(errIO, "Can't execute load operator")
		}
		s, _ := k.str()
		if v, ok := r.lookup(s); ok {
			r.push(v)
		}
	case "<<", ">>":
		t1Fail(errRefuse, "a dictionary operator is executed, which no token can name and nib does not mirror")
	case "array":
		n := int64(int32(r.topNumber().integer()))
		if n < 0 || n > 1<<16 {
			t1Fail(errIO, "Array size is out of range")
		}
		r.charge(0, int(n))
		r.push(psObj{k: psArr, a: &psArray{e: make([]psObj, n)}})
	case "put":
		if len(r.stack) < 3 {
			t1Fail(errIO, "Can't execute put operator")
		}
		v := r.pop()
		i := int64(int32(r.topNumber().integer()))
		a := r.pop()
		if !a.isArray() {
			t1Fail(errIO, "Can't execute put operator")
		}
		if i < 0 || i >= int64(len(a.a.e)) {
			t1Fail(errIO, "Index greater than array size or less than 0")
		}
		a.a.e[i] = v
	case "for":
		r.opFor(depth)
	case "StandardEncoding":
		if len(r.stack) == 0 {
			return
		}
		if top := r.peek(); top.isName() && top.s == "Encoding" {
			r.pop()
			r.dict["Encoding"] = psObj{k: psName, s: "StandardEncoding"}
		}
	default:
		if e, ok := r.lookup(op); ok {
			if e.k == psOp {
				r.execOp(e.s, depth+1)
			} else {
				r.execLiteral(e)
			}
		}
	}
}

// roll is `roll()`, int arithmetic and all.
func (r *t1Reader) roll() {
	a, b := r.topTwo(psObj.isNumber)
	j, n := int32(a.integer()), int32(b.integer())
	if n == 0 { // `% n` throws whichever way j points
		t1Fail(errThrow, "ArithmeticException: roll by zero")
	}
	if j < 0 {
		aj := j
		if aj != math.MinInt32 {
			aj = -aj
		}
		j = n - aj%n
	}
	if int32(len(r.stack)) < n {
		t1Fail(errIO, "Stack has less than n elements")
	}
	if n > 0 {
		r.charge(2*int(n), int(n)) // n popped into a new list, and n pushed back
	}
	var last []psObj
	for i := int32(0); i < n; i++ {
		last = append(last, r.pop())
	}
	get := func(i int32) psObj {
		if i < 0 || int(i) >= len(last) {
			t1Fail(errThrow, "roll reads outside the elements it took")
		}
		return last[i]
	}
	split := (j - 1) % n
	for i := split; i >= 0; i-- {
		r.push(get(i))
	}
	for i := n - 1; i > split; i-- {
		r.push(get(i))
	}
}

// opFor is `opFor`: veraPDF's own bound of 10000 iterations, over longs.
func (r *t1Reader) opFor(depth int) {
	if len(r.stack) == 0 {
		t1Fail(errIO, "Problem with stack")
	}
	p := r.pop()
	if p.k != psProc {
		t1Fail(errIO, "Object is not a procedure")
	}
	limit := r.topNumber().integer()
	inc := r.topNumber().integer()
	init := r.topNumber().integer()
	if inc == 0 || (inc > 0 && init > limit) || (inc < 0 && init < limit) {
		t1Fail(errIO, "Wrong increment value")
	}
	it := (limit - init) / inc
	if it < 0 {
		it = -it
	}
	if it+1 > 10000 {
		t1Fail(errIO, "Loop iteration count is too large")
	}
	for i := init; (inc > 0 && i <= limit) || (inc < 0 && i >= limit); i += inc {
		r.push(psObj{k: psInt, i: i})
		r.execProcedure(p, depth)
		r.charge(1, 0)
	}
}

func psOneNumber(op string, a psObj) psObj {
	x := a.real()
	switch op {
	case "abs":
		return psObj{k: psReal, r: math.Abs(x)}
	case "neg":
		return psObj{k: psReal, r: -x}
	case "ceiling":
		return psObj{k: psInt, i: javaD2L(math.Ceil(x))}
	case "floor":
		return psObj{k: psInt, i: javaD2L(math.Floor(x))}
	case "round":
		return psObj{k: psInt, i: javaRound(x)}
	case "truncate":
		return psObj{k: psInt, i: javaD2L(x)}
	case "sqrt":
		return psObj{k: psReal, r: math.Sqrt(x)}
	case "cos":
		return psObj{k: psReal, r: math.Cos(x)}
	case "atan":
		return psObj{k: psReal, r: math.Atan(x)}
	case "sin":
		return psObj{k: psReal, r: math.Sin(x)}
	case "exp":
		return psObj{k: psReal, r: math.Exp(x)}
	case "cvi":
		return psObj{k: psInt, i: int64(javaD2I(x))}
	case "cvr":
		return psObj{k: psReal, r: x}
	case "ln":
		return psObj{k: psReal, r: math.Log(x)}
	case "log":
		return psObj{k: psReal, r: math.Log10(x)}
	}
	return psObj{k: psInt, i: a.integer() >> 1} // bitshift
}

// javaDoubleEquals is `Double.equals`: bit equality, every NaN equal.
func javaDoubleEquals(x, y float64) bool {
	if math.IsNaN(x) || math.IsNaN(y) {
		return math.IsNaN(x) && math.IsNaN(y)
	}
	return math.Float64bits(x) == math.Float64bits(y)
}

// psTwoNumbers is `executeOperatorOnTopTwoNumbers`: `a` was on top, `b` beneath it.
func psTwoNumbers(op string, a, b psObj) psObj {
	x, y := b.real(), a.real()
	switch op {
	case "add":
		return psObj{k: psReal, r: x + y}
	case "div":
		return psObj{k: psReal, r: x / y}
	case "idiv", "mod":
		d := a.integer()
		if d == 0 {
			t1Fail(errThrow, "ArithmeticException: / by zero")
		}
		if op == "idiv" {
			return psObj{k: psInt, i: b.integer() / d}
		}
		return psObj{k: psInt, i: b.integer() % d}
	case "mul":
		return psObj{k: psReal, r: x * y}
	case "sub":
		return psObj{k: psReal, r: x - y}
	case "eq":
		return psObj{k: psBool, b: javaDoubleEquals(x, y)}
	case "ne":
		return psObj{k: psBool, b: !javaDoubleEquals(x, y)}
	case "gt":
		return psObj{k: psBool, b: x > y}
	case "ge":
		return psObj{k: psBool, b: x >= y}
	case "lt":
		return psObj{k: psBool, b: x < y}
	}
	return psObj{k: psBool, b: x <= y} // le
}

// t1DefaultMatrix is `DEFAULT_FONT_MATRIX`.
var t1DefaultMatrix = [6]float64{0.001, 0, 0, 0.001, 0, 0}

// fontMatrix is `getFontMatrix`: an array's (or a procedure's) numbers in order, the rest 0 — a seventh number is an
// ArrayIndexOutOfBoundsException veraPDF does not catch.
func (r *t1Reader) fontMatrix() [6]float64 {
	o, ok := r.dict["FontMatrix"]
	if !ok || !o.isArray() {
		return t1DefaultMatrix
	}
	var m [6]float64
	n := 0
	for _, e := range o.a.e {
		if e.isNumber() {
			if n == 6 {
				t1Fail(errThrow, "its /FontMatrix holds more than six numbers (ArrayIndexOutOfBoundsException)")
			}
			m[n] = e.real()
			n++
		}
	}
	return m
}

// initializeEncoding is `initializeEncoding`, over the user dictionary as the parse left it.
func (r *t1Reader) initializeEncoding(p *type1Program) {
	o, ok := r.dict["Encoding"]
	if !ok {
		return
	}
	switch {
	case o.isArray():
		for i, e := range o.a.e {
			if i >= 256 {
				break
			}
			s, _ := e.str()
			p.enc[i], p.encSet[i] = s, true
		}
	case o.isName() && o.s == "StandardEncoding":
		for i := range p.enc {
			p.enc[i], p.encSet[i] = standardEncoding[i], true
		}
	}
}

var cleartomark = []byte("cleartomark")

// eexec is `processObject`'s eexec branch.
func (r *t1Reader) eexec() {
	s := r.src
	for !s.isEOF() { // `skipSpacesExceptNullByte`
		if c := s.readByte(); c != 0 && bpSpace(c) {
			continue
		}
		s.unread()
		break
	}
	// `getStreamUntilToken`: the first eleven bytes by `read`, then a byte at a time until the last eleven read are
	// `cleartomark` or the stream ends — the byte read at the end is never kept — and the WHOLE buffer returned, its
	// unused tail zero.
	buf := make([]byte, 10240)
	n := s.read(buf, len(cleartomark))
	rb := s.readByte()
	if n != len(cleartomark) {
		t1Fail(errIO, "Stream is shorter than finishing token")
	}
	for !s.isEOF() && !bytes.Equal(buf[n-len(cleartomark):n], cleartomark) {
		buf[n] = rb
		n++
		if n == len(buf) {
			buf = append(buf, make([]byte, len(buf))...)
		}
		rb = s.readByte()
	}
	m := r.fontMatrix()
	dec := eexecDecrypt(buf, 55665)
	if len(dec) > 4 {
		dec = dec[4:]
	} else {
		dec = nil
	}
	// `SeekableInputStream.getSeekableStream`: kept in memory up to 10240 decoded bytes, else a temporary file — and the
	// two answer an out-of-range charstring differently.
	pp := &t1Private{lx: &bpLexer{src: &bpMemSrc{b: dec}}, r: r, lenIV: 4, internal: len(buf) != 10240}
	pp.defaultMatrix = true
	for i := range m {
		if math.Float64bits(m[i]) != math.Float64bits(t1DefaultMatrix[i]) {
			pp.defaultMatrix = false
		}
	}
	pp.m0 = m[0]
	pp.parse()
	r.widths, r.hasWidths = pp.widths, pp.widths != nil
}

// eexecDecrypt is `EexecFilterDecode`'s cipher over all of `b`, nothing dropped.
func eexecDecrypt(b []byte, key uint32) []byte {
	out := make([]byte, len(b))
	rr := key
	for i, c := range b {
		out[i] = c ^ byte(rr>>8)
		rr = ((uint32(c)+rr)*52845 + 22719) & 0xffff
	}
	return out
}

// t1Private is `Type1PrivateParser` over the decrypted private part.
type t1Private struct {
	lx            *bpLexer
	r             *t1Reader
	lenIV         int32
	widths        map[string]int32
	subrs         map[int32]t1Num
	csFound       bool
	defaultMatrix bool
	m0            float64
	internal      bool // the decoded part is past 10240 bytes: veraPDF reads it from a temporary file
}

func (p *t1Private) src() *bpMemSrc { return p.lx.src.(*bpMemSrc) }

func (p *t1Private) next() {
	if p.r.spend.steps++; p.r.spend.steps > t1MaxSteps {
		t1Fail(errRefuse, fmt.Sprintf("the document's Type 1 programs' private parts take more than %d tokens to read, "+
			"nib's bound", t1MaxSteps))
	}
	p.lx.next()
	if p.lx.a85 && p.lx.typ == bpTHexString {
		t1Fail(errRefuse, "its private part holds an ASCII85 string, which nib does not decode")
	}
}

// parse is `parse()`: tokens until the input ends, or — once /CharStrings was seen — one starting `closefile`.
func (p *t1Private) parse() {
	lx := p.lx
	lx.skipSpaces(true)
	for lx.typ != bpTEOF && (!p.csFound || !bytes.HasPrefix(lx.val, []byte("closefile"))) {
		p.next()
		p.processToken()
	}
}

func (p *t1Private) processToken() {
	lx := p.lx
	if lx.typ != bpTName {
		return
	}
	switch lx.value() {
	case "CharStrings":
		p.csFound = true
		p.next()
		if lx.typ != bpTInteger {
			return
		}
		n := int32(lx.integer)
		p.next() // dict
		p.next() // dup
		p.next() // begin
		for i := int32(0); i < n; i++ {
			if !p.decodeCharString() {
				break
			}
		}
	case "lenIV":
		p.next()
		if lx.typ == bpTInteger {
			p.lenIV = int32(lx.integer)
		}
	case "Subrs":
		p.next()
		if p.subrs == nil {
			p.subrs = map[int32]t1Num{}
		}
		n := int32(lx.integer)
		p.next() // array
		for i := int32(0); i < n; i++ {
			p.next() // dup
			if lx.value() != "dup" {
				break
			}
			p.next()
			num := lx.integer
			p.next()
			size := lx.integer
			p.skipRD()
			lx.skipSpaces(false)
			begin := p.src().off
			// `InputStream.skip(long)`: nothing for a length that is not positive, to the end at most.
			if rest := int64(len(p.src().b) - p.src().off); size > 0 {
				p.src().off += int(min(size, rest))
			}
			chunk := p.getStream(begin, size)
			if w := p.charStringWidth(chunk); w != nil {
				p.subrs[int32(num)] = *w
			}
			p.next() // NP
			if lx.value() == "noaccess" {
				p.next()
			}
		}
	}
}

// getStream is the source's `getStream(offset, length)`: in memory, an IOException unless the range lies inside the
// data (and past offset 0); from a temporary file, the range cut at the end — and a negative length the whole rest.
//
// In memory the range test is `startOffset + length <= bufferSize` over longs, so a length near 2^63 wraps and PASSES,
// and the substream is then built with `(int) length`: where that is negative (as for 2^63-1, the value a number too
// long to parse reads as) the first read copies a negative length and the result is an IOException — and within nib's
// bound on the program's size it always is (the length is 2^63 - m for an m below `begin`, whose low 32 bits are -m), so
// the wrapped case and the plainly-too-long one answer alike, and `size <= rest` below decides both without the sum.
func (p *t1Private) getStream(begin int, size int64) []byte {
	b := p.src().b
	rest := int64(len(b) - begin)
	if !p.internal {
		switch {
		case size < 0 && size < math.MinInt32:
			t1Fail(errRefuse, "a charstring's negative length is truncated to 32 bits by veraPDF, which nib does not follow")
		// `begin > 0` is veraPDF's test and cannot bind: a charstring's data starts after its name, length and RD tokens,
		// a subroutine's after dup, index, length and RD, so `begin` is at least 2.
		case !(begin > 0 && rest > 0 && size <= rest) || size < 0:
			t1Fail(errIO, "a charstring runs outside the private part")
		}
		return b[begin : int64(begin)+size]
	}
	if size < 0 || size > rest {
		return b[begin:]
	}
	return b[begin : int64(begin)+size]
}

// skipRD is `skipRD`: `-|` read as two bytes, anything else as a token.
func (p *t1Private) skipRD() {
	p.lx.skipSpaces(false)
	s := p.src()
	if s.read() == '-' {
		s.read()
		return
	}
	s.unread()
	p.next()
}

// decodeCharString is `decodeCharString`.
func (p *t1Private) decodeCharString() bool {
	lx := p.lx
	if p.widths == nil {
		p.widths = map[string]int32{}
	}
	p.next()
	if lx.typ != bpTName {
		if lx.typ == bpTKeyword && lx.value() == "end" {
			return false
		}
		t1Fail(errIO, "Error in parsing Private dictionary of font 1 file, expected a glyph name")
	}
	name := lx.value()
	p.next()
	if lx.typ != bpTInteger {
		t1Fail(errIO, "Error in parsing Private dictionary of font 1 file, expected a charstring length")
	}
	size := lx.integer
	p.skipRD()
	lx.skipSingleSpace(false)
	s := p.src()
	begin := s.off
	if p.internal && (size < 0 || size > math.MaxInt32) {
		t1Fail(errRefuse, "a charstring's length is negative or past 32 bits, where veraPDF's temporary-file reader "+
			"seeks in ways nib does not mirror")
	}
	// `skip((int) length)`, to the end at most — in memory a negative int moves BACK, but the range test below then
	// throws whatever the skip did, so only the temporary file's skip needs the length to fit (refused above).
	if size >= 0 && size <= math.MaxInt32 {
		s.off += int(min(size, int64(len(s.b)-s.off)))
	}
	chunk := p.getStream(begin, size)
	if w := p.charStringWidth(chunk); w != nil {
		if p.defaultMatrix {
			p.widths[name] = int32(w.i)
		} else {
			p.widths[name] = javaD2I(float64(w.r) * (p.m0 * 1000))
		}
	}
	p.next()
	return true
}

// charStringWidth decrypts one charstring (`EexecFilterDecode` with lenIV, read through an `ASMemoryInStream`) and reads
// its width.
func (p *t1Private) charStringWidth(chunk []byte) *t1Num {
	if p.r.spend.decrypted += len(chunk); p.r.spend.decrypted > t1MaxDecrypt {
		t1Fail(errRefuse, fmt.Sprintf("the document's Type 1 programs' charstrings decrypt to more than %d bytes, nib's "+
			"bound", t1MaxDecrypt))
	}
	if len(chunk) == 0 {
		return nil
	}
	iv := p.lenIV
	var data []byte
	switch {
	// The first read asks the chunk for min(lenIV + 8192, 2048) bytes: past MaxInt32 - 8192 that sum wraps, and at -8192
	// or below it asks for none or a negative count — shapes nib does not follow. Between, a positive lenIV past the
	// first read's 2048 bytes empties the charstring and a negative one leaves |lenIV| zero bytes ahead of it.
	case (iv > math.MaxInt32-8192 || iv <= -8192) && p.internal:
		t1Fail(errRefuse, fmt.Sprintf("its /lenIV is %d, where veraPDF's decryption buffer arithmetic over its "+
			"temporary-file reader is not mirrored", iv))
	case iv == -8192:
		return nil // a read of none, which ends the charstring (measured)
	case iv > math.MaxInt32-8192 || iv < -8192:
		// A read of a negative count copies a negative length: an IOException (measured past MaxInt32 - 8192).
		t1Fail(errIO, "Can't write bytes into passed buffer: too small.")
	case iv >= 0:
		// The first read takes 2048 bytes at most; fewer than lenIV of them ends the stream empty.
		if min(len(chunk), 2048) < int(iv) {
			return nil
		}
		data = eexecDecrypt(chunk, 4330)[iv:]
	default:
		// A negative lenIV discards nothing and leaves |lenIV| untouched (zero) bytes ahead of the decryption.
		data = append(make([]byte, -iv), eexecDecrypt(chunk, 4330)...)
	}
	return type1CharStringWidth(data, p.subrs)
}

// t1Num is `CFFNumber`.
type t1Num struct {
	i     int64
	r     float32
	isInt bool
}

func t1Int(v int32) t1Num { return t1Num{i: int64(v), r: float32(v), isInt: true} }

// type1CharStringWidth is `Type1CharStringParser` over a decrypted charstring: its first width, nil for none.
func type1CharStringWidth(b []byte, subrs map[int32]t1Num) *t1Num {
	var stack []t1Num
	i := 0
	// readStreams: bytes past the end read as the zeros the buffer held.
	byteAt := func() int {
		if i >= len(b) {
			i++
			return 0
		}
		i++
		return int(b[i-1])
	}
	pop := func(n int) {
		for ; n > 0 && len(stack) > 0; n-- {
			stack = stack[:len(stack)-1]
		}
	}
	top := func() t1Num {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v
	}
	for i < len(b) {
		c := int(b[i])
		i++
		if c > 31 {
			switch {
			case c < 247:
				stack = append(stack, t1Int(int32(c-139)))
			case c < 251:
				stack = append(stack, t1Int(int32((c-247)<<8+byteAt()+108)))
			case c < 255:
				stack = append(stack, t1Int(int32(-((c-251)<<8)-byteAt()-108)))
			default:
				v := uint32(byteAt())<<24 | uint32(byteAt())<<16 | uint32(byteAt())<<8 | uint32(byteAt())
				stack = append(stack, t1Int(int32(v)))
			}
			continue
		}
		if c != 12 {
			switch c {
			case 6, 22, 7, 4:
				pop(1)
			case 5, 21, 1, 3:
				pop(2)
			case 31, 30:
				pop(4)
			case 8:
				pop(6)
			case 13: // hsbw
				if len(stack) > 0 {
					w := top()
					pop(1)
					return &w
				}
				return nil
			case 10: // callsubr
				if len(stack) > 0 {
					n := top()
					if subrs != nil {
						if w, ok := subrs[int32(n.i)]; ok {
							return &w
						}
					}
				}
			}
			continue
		}
		switch byteAt() {
		case 33:
			pop(2)
		case 6:
			pop(5)
		case 2, 1:
			pop(6)
		case 17:
			pop(1)
		case 7: // sbw
			pop(1)
			if len(stack) > 0 {
				w := top()
				pop(2)
				return &w
			}
			return nil
		case 12: // div
			if len(stack) > 1 {
				n2, n1 := float64(top().r), float64(top().r)
				f := float32(n1 / n2)
				stack = append(stack, t1Num{i: javaD2L(float64(f)), r: f})
			}
		}
	}
	return nil
}

// type1Of is a simple Type 1 font's /FontFile program (`PDType1Font.getFontProgram`, which opens /FontFile before
// /FontFile3): nil and known where veraPDF has no parsed program — none, or one its parse failed — not known where nib
// refuses, and `throws` where veraPDF throws reading it, so that it reports nothing on the document. Read once per
// stream: veraPDF caches the program by the stream's key.
func (d *Document) type1Of(font types.Dict) (p *type1Program, known bool, why, throws string) {
	sd, _, err := d.Ctx.DereferenceStreamDict(d.dict(font["FontDescriptor"])["FontFile"])
	if err != nil || sd == nil {
		return nil, true, "", ""
	}
	key := dictID(sd.Dict)
	r, done := d.type1Reads[key]
	if !done {
		if why := d.decodeFontStream(sd, "its embedded Type 1 program"); why != "" {
			r = &type1Program{state: ttUnknown, why: why}
		} else {
			prog := readType1(sd.Content, &d.type1Spent)
			r = &prog
		}
		if d.type1Reads == nil {
			d.type1Reads = map[uintptr]*type1Program{}
		}
		d.type1Reads[key] = r
	}
	switch {
	case r.throws != "":
		return nil, true, "", r.throws
	case r.state == ttUnknown:
		return nil, false, r.why, ""
	case r.state == ttFailed:
		return nil, true, "", ""
	}
	return r, true, "", ""
}

// type1Metrics is `GFGlyph` over a font with a parsed Type 1 program (`PDType1Font.glyphIsPresent` /
// `getWidthFromProgram`): a code the PDF's encoding names asks the program for that NAME (present if it holds it and it
// is not `.notdef`, its width or -1); a code it does not name asks the program's own encoding. Code 0 is always present,
// and a program width of -1 is the descriptor's /MissingWidth, else 0.
func (d *Document) type1Metrics(g glyph, p *type1Program) glyphMetrics {
	font := g.font.dict
	if g.font.enc == nil {
		e := d.encodingOf(font)
		g.font.enc = &e
	}
	var present bool
	var w float32
	if name, named := g.font.enc.name(g.code); named {
		present, w = p.containsGlyph(name), p.widthOfName(name)
	} else {
		present, w = p.containsCode(g.code), p.widthOfCode(g.code)
	}
	dw, ok, why := d.simpleDictWidth(font, g.code)
	if !ok {
		return glyphMetrics{why: why}
	}
	program := float64(w)
	if w == -1 {
		program = d.missingWidth(font)
	}
	return glyphMetrics{known: true, valid: true, present: g.code == 0 || present, program: program, dictionary: dw}
}
