package pdfops

import (
	"bytes"
	"encoding/binary"
	"math"
	"strconv"
)

// Print's ink from a charstring program: CFF (`/FontFile3`) and Type 1 (`/FontFile`). The second reader fontink.go
// names, and the same answer in the same shape: how far a program's glyphs reach, in ems from the baseline.
//
// Written from Adobe's own documents — Technical Note #5176 (the CFF container), #5177 (Type 2 charstrings) and
// "Adobe Type 1 Font Format" — and from nothing else. `internal/uacheck` reads the same two containers, and it is
// deliberately NOT this reader: that one reads a program as veraPDF does, misreadings included (a charset read past
// its glyph count, an encoding supplement stored as a SID), because its question is what veraPDF will say. This
// one's question is where a renderer puts ink, and a renderer follows the specification. Two readings of one format
// held side by side, as `internal/fontcode` holds two of a CMap — sharing the parse would give one of them the
// other's answers.
//
// What a glyph's reach is: the lowest and highest point its outline NAMES — end points and control points alike. A
// curve lies inside its control points, so this is never less than the ink and may be a little more. The safe
// direction, and the only one allowed here: a box drawn to it covers the glyph.
//
// Refused, and so the font gives no ink and every run in it keeps the line's reach: anything this does not read
// exactly. An arithmetic or storage operator in a Type 2 charstring; `endchar` in its four-argument accent form; a
// `seac` naming a part outside the letters and accents; a multiple-master subroutine; an operator with the wrong
// number of arguments; a font matrix that is not a thousand units to the em, upright; a program that spends more
// than its budget. A font is vouched for whole or not at all, because one misread glyph is a letter left outside a
// redaction.
//
// Every bound below is spent by the DOCUMENT, which chooses its own fonts.
const (
	csMaxDepth   = 10        // nested subroutine calls: both specifications' own limit
	csT2Stack    = 48        // Type 2 argument stack (#5177 appendix B)
	csT1Stack    = 24        // Type 1 BuildChar operand stack
	csGlyphSteps = 100_000   // numbers and operators one glyph may spend
	csFontSteps  = 8_000_000 // and one program, in all
	csMaxGlyphs  = 65_535
	csMaxFDs     = 256
)

// csBox is one glyph's reach in glyph units; inked false is a glyph that draws nothing (a space).
type csBox struct {
	lo, hi float64
	inked  bool
}

func (b *csBox) mark(y float64) {
	if !b.inked {
		b.lo, b.hi, b.inked = y, y, true
		return
	}
	b.lo, b.hi = math.Min(b.lo, y), math.Max(b.hi, y)
}

// csPath is the pen both interpreters move. Only what a drawing operator touches is marked: a move that nothing is
// drawn from paints nothing.
type csPath struct {
	x, y float64
	box  csBox
}

func (p *csPath) move(dx, dy float64) { p.x, p.y = p.x+dx, p.y+dy }

func (p *csPath) line(dx, dy float64) {
	p.box.mark(p.y)
	p.move(dx, dy)
	p.box.mark(p.y)
}

func (p *csPath) curve(dx1, dy1, dx2, dy2, dx3, dy3 float64) {
	p.box.mark(p.y)
	p.move(dx1, dy1)
	p.box.mark(p.y)
	p.move(dx2, dy2)
	p.box.mark(p.y)
	p.move(dx3, dy3)
	p.box.mark(p.y)
}

// ---- CFF: the container (#5176) ----

// cffIdx is an INDEX: count objects in data, object i at data[off(i)-1 : off(i+1)-1].
type cffIdx struct {
	b       []byte // the whole program
	count   int
	offSize int
	offs    int // where the offset array starts
	base    int // where object data starts, less one
	end     int // the first byte after the INDEX
}

func readCFFIdx(b []byte, at int) (cffIdx, bool) {
	if at < 0 || at+2 > len(b) {
		return cffIdx{}, false
	}
	x := cffIdx{b: b, count: int(binary.BigEndian.Uint16(b[at:]))}
	if x.count == 0 {
		x.end = at + 2
		return x, true
	}
	if at+3 > len(b) {
		return cffIdx{}, false
	}
	x.offSize = int(b[at+2])
	if x.offSize < 1 || x.offSize > 4 {
		return cffIdx{}, false
	}
	x.offs = at + 3
	x.base = x.offs + (x.count+1)*x.offSize - 1
	if x.base+1 > len(b) {
		return cffIdx{}, false
	}
	last := x.off(x.count)
	if last < 1 || x.base+last > len(b) {
		return cffIdx{}, false
	}
	x.end = x.base + last
	return x, true
}

func (x cffIdx) off(i int) int {
	v := 0
	for _, c := range x.b[x.offs+i*x.offSize : x.offs+(i+1)*x.offSize] {
		v = v<<8 | int(c)
	}
	return v
}

func (x cffIdx) get(i int) ([]byte, bool) {
	if i < 0 || i >= x.count {
		return nil, false
	}
	a, z := x.off(i), x.off(i+1)
	if a < 1 || z < a || x.base+z > x.end {
		return nil, false
	}
	return x.b[x.base+a : x.base+z], true
}

// cffDict reads a DICT into its operators' operands. A two-byte operator 12 n is keyed 1200+n.
func cffDict(d []byte) (map[int][]float64, bool) {
	out := map[int][]float64{}
	var args []float64
	for i := 0; i < len(d); {
		c := d[i]
		switch {
		case c <= 21:
			op := int(c)
			i++
			if c == 12 {
				if i >= len(d) {
					return nil, false
				}
				op = 1200 + int(d[i])
				i++
			}
			out[op] = args
			args = nil
		case c == 28:
			if i+3 > len(d) {
				return nil, false
			}
			args = append(args, float64(int16(binary.BigEndian.Uint16(d[i+1:]))))
			i += 3
		case c == 29:
			if i+5 > len(d) {
				return nil, false
			}
			args = append(args, float64(int32(binary.BigEndian.Uint32(d[i+1:]))))
			i += 5
		case c == 30:
			var s []byte
			i++
			for done := false; !done; i++ {
				if i >= len(d) || len(s) > 64 {
					return nil, false
				}
				for _, n := range [2]byte{d[i] >> 4, d[i] & 15} {
					switch {
					case n <= 9:
						s = append(s, '0'+n)
					case n == 0xa:
						s = append(s, '.')
					case n == 0xb:
						s = append(s, 'E')
					case n == 0xc:
						s = append(s, 'E', '-')
					case n == 0xe:
						s = append(s, '-')
					case n == 0xf:
						done = true
					default:
						return nil, false
					}
					if done {
						break
					}
				}
			}
			v, err := strconv.ParseFloat(string(s), 64)
			if err != nil {
				return nil, false
			}
			args = append(args, v)
		case c >= 32 && c <= 246:
			args = append(args, float64(int(c)-139))
			i++
		case c >= 247 && c <= 254:
			if i+2 > len(d) {
				return nil, false
			}
			if c <= 250 {
				args = append(args, float64((int(c)-247)*256+int(d[i+1])+108))
			} else {
				args = append(args, float64(-(int(c)-251)*256-int(d[i+1])-108))
			}
			i += 2
		default:
			return nil, false
		}
		if len(args) > 48 {
			return nil, false
		}
	}
	return out, true
}

// dictInt is operator op's single whole operand.
func dictInt(d map[int][]float64, op int) (int, bool) {
	a, has := d[op]
	if !has || len(a) != 1 || a[0] != math.Trunc(a[0]) || math.Abs(a[0]) > 1<<31 {
		return 0, false
	}
	return int(a[0]), true
}

// thousandToTheEm reports the font matrix of a program whose glyph units are a thousand to the em, set upright with
// no move — the only one this reads, because the ink is handed back in ems and a matrix is where a renderer and a reader can disagree.
func thousandToTheEm(m []float64) bool {
	const eps = 1e-7
	return len(m) == 6 && math.Abs(m[0]-0.001) < eps && math.Abs(m[3]-0.001) < eps &&
		math.Abs(m[1]) < eps && math.Abs(m[2]) < eps && math.Abs(m[4]) < eps && math.Abs(m[5]) < eps
}

func unitMatrix(m []float64) bool {
	return len(m) == 6 && m[0] == 1 && m[1] == 0 && m[2] == 0 && m[3] == 1 && m[4] == 0 && m[5] == 0
}

// cffInk is a CFF program opened far enough to run any one glyph.
type cffInk struct {
	chars  cffIdx
	global cffIdx
	locals []cffIdx // one per font dictionary; a name-keyed program has one
	fdOf   []uint8  // glyph → font dictionary; nil where there is one
	cids   map[int]int
	isCID  bool
	steps  int // what is left of csFontSteps
	boxes  map[int]csBox
	failed bool
}

// openCFF reads the container. b may be the bare program or an OpenType file holding it in a `CFF ` table.
func openCFF(b []byte) *cffInk {
	if len(b) >= 12 && (string(b[:4]) == "OTTO" || binary.BigEndian.Uint32(b) == 0x00010000) {
		n := int(binary.BigEndian.Uint16(b[4:]))
		var table []byte
		for i := 0; i < n; i++ {
			e := 12 + 16*i
			if e+16 > len(b) {
				return nil
			}
			off, length := int64(binary.BigEndian.Uint32(b[e+8:])), int64(binary.BigEndian.Uint32(b[e+12:]))
			if string(b[e:e+4]) == "CFF " && off+length <= int64(len(b)) {
				table = b[off : off+length]
			}
		}
		b = table
	}
	if len(b) < 4 || b[0] != 1 {
		return nil
	}
	names, ok := readCFFIdx(b, int(b[2]))
	if !ok || names.count != 1 { // one font to a program, as a PDF embeds it
		return nil
	}
	tops, ok := readCFFIdx(b, names.end)
	if !ok || tops.count != 1 {
		return nil
	}
	strs, ok := readCFFIdx(b, tops.end)
	if !ok {
		return nil
	}
	k := &cffInk{steps: csFontSteps, boxes: map[int]csBox{}}
	if k.global, ok = readCFFIdx(b, strs.end); !ok {
		return nil
	}
	raw, _ := tops.get(0)
	top, ok := cffDict(raw)
	if !ok {
		return nil
	}
	if t, has := top[1206]; has && (len(t) != 1 || t[0] != 2) { // CharstringType
		return nil
	}
	at, ok := dictInt(top, 17)
	if !ok {
		return nil
	}
	if k.chars, ok = readCFFIdx(b, at); !ok || k.chars.count == 0 || k.chars.count > csMaxGlyphs {
		return nil
	}
	topMatrix, hasTop := top[1207]
	if hasTop && !thousandToTheEm(topMatrix) {
		return nil
	}
	// local reads the Subrs a Private DICT names; a dictionary without them has none.
	local := func(d map[int][]float64) (cffIdx, bool) {
		p, has := d[18]
		if !has {
			return cffIdx{}, true
		}
		if len(p) != 2 || p[0] < 0 || p[1] < 0 || p[0] > float64(len(b)) || p[1] > float64(len(b)) || int(p[0])+int(p[1]) > len(b) {
			return cffIdx{}, false
		}
		priv, ok := cffDict(b[int(p[1]) : int(p[1])+int(p[0])])
		if !ok {
			return cffIdx{}, false
		}
		if _, has := priv[19]; !has {
			return cffIdx{}, true
		}
		rel, ok := dictInt(priv, 19)
		if !ok {
			return cffIdx{}, false
		}
		return readCFFIdx(b, int(p[1])+rel)
	}
	if _, isCID := top[1230]; !isCID {
		l, ok := local(top)
		if !ok {
			return nil
		}
		k.locals = []cffIdx{l}
		return k
	}
	k.isCID = true
	at, ok = dictInt(top, 1236)
	if !ok {
		return nil
	}
	fds, ok := readCFFIdx(b, at)
	if !ok || fds.count == 0 || fds.count > csMaxFDs {
		return nil
	}
	for i := 0; i < fds.count; i++ {
		raw, _ := fds.get(i)
		fd, ok := cffDict(raw)
		if !ok {
			return nil
		}
		// Either the program or its font dictionaries say a thousand to the em — or neither, which is the default.
		// Both saying it is read differently by different renderers, and is not read here.
		if m, has := fd[1207]; has && !(hasTop && unitMatrix(m)) && !(!hasTop && thousandToTheEm(m)) {
			return nil
		}
		l, ok := local(fd)
		if !ok {
			return nil
		}
		k.locals = append(k.locals, l)
	}
	n := k.chars.count
	if at, ok = dictInt(top, 1237); !ok || at < 0 || at >= len(b) {
		return nil
	}
	k.fdOf = make([]uint8, n)
	switch sel := b[at:]; sel[0] {
	case 0:
		if len(sel) < 1+n {
			return nil
		}
		copy(k.fdOf, sel[1:1+n])
	case 3:
		if len(sel) < 5 {
			return nil
		}
		ranges := int(binary.BigEndian.Uint16(sel[1:]))
		if len(sel) < 3+3*ranges+2 || ranges == 0 || binary.BigEndian.Uint16(sel[3:]) != 0 {
			return nil
		}
		for r := 0; r < ranges; r++ {
			first, fd := int(binary.BigEndian.Uint16(sel[3+3*r:])), sel[5+3*r]
			next := int(binary.BigEndian.Uint16(sel[6+3*r:])) // the next range's first, or the sentinel
			if next <= first || next > n || (r == ranges-1 && next != n) {
				return nil
			}
			for g := first; g < next; g++ {
				k.fdOf[g] = fd
			}
		}
	default:
		return nil
	}
	for _, fd := range k.fdOf {
		if int(fd) >= len(k.locals) {
			return nil
		}
	}
	// The charset: which CID each glyph is. Glyph 0 is CID 0 and is not written.
	if at, ok = dictInt(top, 15); !ok || at <= 2 || at >= len(b) { // 0..2 are predefined NAME charsets: not a CID font's
		return nil
	}
	k.cids = map[int]int{0: 0}
	set := b[at:]
	put := func(g, cid int) {
		if _, taken := k.cids[cid]; !taken {
			k.cids[cid] = g
		}
	}
	switch set[0] {
	case 0:
		if len(set) < 1+2*(n-1) {
			return nil
		}
		for g := 1; g < n; g++ {
			put(g, int(binary.BigEndian.Uint16(set[1+2*(g-1):])))
		}
	case 1, 2:
		width := 3 + int(set[0]-1)
		for g, p := 1, 1; g < n; p += width {
			if p+width > len(set) {
				return nil
			}
			first, left := int(binary.BigEndian.Uint16(set[p:])), int(set[p+2])
			if set[0] == 2 {
				left = int(binary.BigEndian.Uint16(set[p+2:]))
			}
			for i := 0; i <= left && g < n; i, g = i+1, g+1 {
				put(g, first+i)
			}
		}
	default:
		return nil
	}
	return k
}

// glyph is glyph g's reach, read once. ok false is a glyph this cannot vouch for; the program is then spent.
func (k *cffInk) glyph(g int) (csBox, bool) {
	if k.failed || g < 0 || g >= k.chars.count {
		return csBox{}, false
	}
	if box, read := k.boxes[g]; read {
		return box, true
	}
	code, ok := k.chars.get(g)
	if !ok {
		k.failed = true
		return csBox{}, false
	}
	fd := 0
	if k.fdOf != nil {
		fd = int(k.fdOf[g])
	}
	run := t2Run{global: k.global, local: k.locals[fd], steps: min(csGlyphSteps, k.steps)}
	before := run.steps
	ok = run.exec(code, 0)
	k.steps -= before - run.steps
	if !ok {
		k.failed = true
		return csBox{}, false
	}
	k.boxes[g] = run.path.box
	return run.path.box, true
}

// ---- Type 2 charstrings (#5177) ----

type t2Run struct {
	global, local cffIdx
	st            [csT2Stack]float64
	n             int
	stems         int
	path          csPath
	steps         int
	done          bool
}

// t2Bias is what a subroutine number is offset by (#5177 section 4.7).
func t2Bias(count int) int {
	switch {
	case count < 1240:
		return 107
	case count < 33900:
		return 1131
	}
	return 32768
}

// exec runs one charstring or subroutine. False refuses the glyph.
func (r *t2Run) exec(c []byte, depth int) bool {
	if depth > csMaxDepth {
		return false
	}
	for i := 0; i < len(c) && !r.done; {
		if r.steps--; r.steps < 0 {
			return false
		}
		b := c[i]
		i++
		if b >= 32 || b == 28 {
			var v float64
			switch {
			case b == 28:
				if i+2 > len(c) {
					return false
				}
				v = float64(int16(binary.BigEndian.Uint16(c[i:])))
				i += 2
			case b <= 246:
				v = float64(int(b) - 139)
			case b <= 254:
				if i >= len(c) {
					return false
				}
				if b <= 250 {
					v = float64((int(b)-247)*256 + int(c[i]) + 108)
				} else {
					v = float64(-(int(b)-251)*256 - int(c[i]) - 108)
				}
				i++
			default: // 255: sixteen bits and sixteen of fraction
				if i+4 > len(c) {
					return false
				}
				v = float64(int32(binary.BigEndian.Uint32(c[i:]))) / 65536
				i += 4
			}
			if r.n >= csT2Stack {
				return false
			}
			r.st[r.n] = v
			r.n++
			continue
		}
		n, a, p := r.n, r.st[:r.n], &r.path
		switch b {
		case 1, 3, 18, 23: // the stem hints; an odd count carries the width first
			r.stems += n / 2
		case 19, 20: // hintmask, cntrmask: what is on the stack is the last vertical stems
			r.stems += n / 2
			if i += (r.stems + 7) / 8; i > len(c) {
				return false
			}
		case 21: // rmoveto; a moveto takes its arguments from the top, the width below them
			if n < 2 || n > 3 {
				return false
			}
			p.move(a[n-2], a[n-1])
		case 22: // hmoveto
			if n < 1 || n > 2 {
				return false
			}
			p.move(a[n-1], 0)
		case 4: // vmoveto
			if n < 1 || n > 2 {
				return false
			}
			p.move(0, a[n-1])
		case 5: // rlineto
			if n < 2 || n%2 != 0 {
				return false
			}
			for k := 0; k < n; k += 2 {
				p.line(a[k], a[k+1])
			}
		case 6, 7: // hlineto, vlineto: alternating
			if n < 1 {
				return false
			}
			for k, across := 0, b == 6; k < n; k, across = k+1, !across {
				if across {
					p.line(a[k], 0)
				} else {
					p.line(0, a[k])
				}
			}
		case 8: // rrcurveto
			if n < 6 || n%6 != 0 {
				return false
			}
			for k := 0; k < n; k += 6 {
				p.curve(a[k], a[k+1], a[k+2], a[k+3], a[k+4], a[k+5])
			}
		case 24: // rcurveline
			if n < 8 || (n-2)%6 != 0 {
				return false
			}
			for k := 0; k < n-2; k += 6 {
				p.curve(a[k], a[k+1], a[k+2], a[k+3], a[k+4], a[k+5])
			}
			p.line(a[n-2], a[n-1])
		case 25: // rlinecurve
			if n < 8 || (n-6)%2 != 0 {
				return false
			}
			for k := 0; k < n-6; k += 2 {
				p.line(a[k], a[k+1])
			}
			p.curve(a[n-6], a[n-5], a[n-4], a[n-3], a[n-2], a[n-1])
		case 26, 27: // vvcurveto, hhcurveto: an odd count carries the other direction's first step
			if n < 4 || n%4 > 1 {
				return false
			}
			k, off := 0, 0.0
			if n%4 == 1 {
				k, off = 1, a[0]
			}
			for ; k < n; k, off = k+4, 0 {
				if b == 26 {
					p.curve(off, a[k], a[k+1], a[k+2], 0, a[k+3])
				} else {
					p.curve(a[k], off, a[k+1], a[k+2], a[k+3], 0)
				}
			}
		case 30, 31: // vhcurveto, hvcurveto: alternating, the last curve taking one more
			if n < 4 || n%4 > 1 {
				return false
			}
			for k, across := 0, b == 31; n-k >= 4; k, across = k+4, !across {
				last := 0.0
				if n-k == 5 {
					last = a[k+4]
				}
				if across {
					p.curve(a[k], 0, a[k+1], a[k+2], last, a[k+3])
				} else {
					p.curve(0, a[k], a[k+1], a[k+2], a[k+3], last)
				}
			}
		case 10, 29: // callsubr, callgsubr
			if n < 1 {
				return false
			}
			subrs := r.local
			if b == 29 {
				subrs = r.global
			}
			v := a[n-1]
			if v != math.Trunc(v) {
				return false
			}
			sub, ok := subrs.get(int(v) + t2Bias(subrs.count))
			r.n--
			if !ok || !r.exec(sub, depth+1) {
				return false
			}
			continue // a subroutine leaves the stack to its caller
		case 11: // return
			return true
		case 14: // endchar; with four arguments it is an accented letter made of two others — not read
			if n > 1 {
				return false
			}
			r.done = true
		case 12:
			if i >= len(c) {
				return false
			}
			op := c[i]
			i++
			switch {
			case op == 34 && n == 7: // hflex
				p.curve(a[0], 0, a[1], a[2], a[3], 0)
				p.curve(a[4], 0, a[5], -a[2], a[6], 0)
			case op == 35 && n == 13: // flex
				p.curve(a[0], a[1], a[2], a[3], a[4], a[5])
				p.curve(a[6], a[7], a[8], a[9], a[10], a[11])
			case op == 36 && n == 9: // hflex1
				p.curve(a[0], a[1], a[2], a[3], a[4], 0)
				p.curve(a[5], 0, a[6], a[7], a[8], -(a[1] + a[3] + a[7]))
			case op == 37 && n == 11: // flex1
				dx, dy := a[0]+a[2]+a[4]+a[6]+a[8], a[1]+a[3]+a[5]+a[7]+a[9]
				p.curve(a[0], a[1], a[2], a[3], a[4], a[5])
				if math.Abs(dx) > math.Abs(dy) {
					p.curve(a[6], a[7], a[8], a[9], a[10], -dy)
				} else {
					p.curve(a[6], a[7], a[8], a[9], -dx, a[10])
				}
			default: // arithmetic, storage, conditionals: no font of the corpus uses one, and none is read
				return false
			}
		default:
			return false
		}
		r.n = 0
	}
	return true
}

// readCFFInk is a CFF program's ink. byCID asks for a glyph at a time by CID — a Type0 font's codes under Identity-H
// — and is answered only by a program with a CID charset, or a name-keyed one, whose glyph numbers ARE its CIDs
// (ISO 32000-1 9.7.4.2). Otherwise every glyph is read and the answer is the program's whole reach.
func readCFFInk(b []byte, byCID bool) *fontInk {
	k := openCFF(b)
	if k == nil {
		return nil
	}
	if byCID {
		return &fontInk{byCode: true, cff: k}
	}
	out := &fontInk{lo: math.Inf(1), hi: math.Inf(-1)}
	for g := 0; g < k.chars.count; g++ {
		box, ok := k.glyph(g)
		if !ok {
			return nil
		}
		if box.inked {
			out.lo, out.hi = math.Min(out.lo, box.lo/1000), math.Max(out.hi, box.hi/1000)
		}
	}
	if !(out.hi > out.lo) {
		return nil
	}
	return out
}

// ---- Type 1 ("Adobe Type 1 Font Format") ----

// t1Decrypt undoes the one cipher a Type 1 program uses twice: the private dictionary under key 55665, each
// charstring under 4330.
func t1Decrypt(b []byte, key uint16) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = c ^ byte(key>>8)
		key = (uint16(c)+key)*52845 + 22719
	}
	return out
}

func psSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == 0
}

// t1Lex cuts the decrypted private dictionary into tokens, and steps over binary by its declared length.
type t1Lex struct {
	b []byte
	p int
}

func (l *t1Lex) token() string {
	for l.p < len(l.b) && psSpace(l.b[l.p]) {
		l.p++
	}
	a := l.p
	if l.p < len(l.b) && l.b[l.p] == '/' {
		l.p++
	}
	for l.p < len(l.b) && !psSpace(l.b[l.p]) && l.b[l.p] != '/' {
		l.p++
		if c := l.b[l.p-1]; c == '[' || c == ']' || c == '{' || c == '}' {
			if l.p-1 > a {
				l.p-- // a bracket ends the token before it
			}
			break
		}
	}
	return string(l.b[a:l.p])
}

// binary reads "<length> <RD> " and the length bytes after it, the length already read.
func (l *t1Lex) binary(length string) ([]byte, bool) {
	n, err := strconv.Atoi(length)
	if err != nil || n < 0 {
		return nil, false
	}
	if l.token() == "" { // RD, -| or whatever the font called it
		return nil, false
	}
	l.p++ // exactly one separator
	if l.p+n > len(l.b) {
		return nil, false
	}
	l.p += n
	return l.b[l.p-n : l.p], true
}

// t1Ink is a Type 1 program opened far enough to run any one glyph.
type t1Ink struct {
	subrs map[int][]byte
	chars map[string][]byte
	boxes map[string]csBox
	steps int
}

func openType1(b []byte) *t1Ink {
	cut := bytes.Index(b, []byte("eexec"))
	if cut < 0 {
		return nil
	}
	// The matrix is in the clear part, as six numbers in brackets.
	clear := b[:cut]
	m := bytes.Index(clear, []byte("/FontMatrix"))
	if m < 0 {
		return nil
	}
	lx := &t1Lex{b: clear, p: m + len("/FontMatrix")}
	if t := lx.token(); t != "[" && t != "{" {
		return nil
	}
	var matrix []float64
	for len(matrix) < 6 {
		v, err := strconv.ParseFloat(lx.token(), 64)
		if err != nil {
			return nil
		}
		matrix = append(matrix, v)
	}
	if !thousandToTheEm(matrix) {
		return nil
	}
	enc := b[cut+len("eexec"):]
	for len(enc) > 0 && psSpace(enc[0]) && enc[0] != 0 {
		enc = enc[1:]
	}
	if len(enc) < 4 {
		return nil
	}
	hex := true
	for _, c := range enc[:4] {
		hex = hex && (c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F')
	}
	if hex { // a program written in hexadecimal: two digits a byte, white space between them ignored
		raw := make([]byte, 0, len(enc)/2)
		var hi, have = byte(0), false
	digits:
		for _, c := range enc {
			var v byte
			switch {
			case c >= '0' && c <= '9':
				v = c - '0'
			case c >= 'a' && c <= 'f':
				v = c - 'a' + 10
			case c >= 'A' && c <= 'F':
				v = c - 'A' + 10
			case psSpace(c):
				continue
			default: // the clear text that closes the program ("cleartomark")
				break digits
			}
			if have {
				raw = append(raw, hi<<4|v)
			} else {
				hi = v
			}
			have = !have
		}
		enc = raw
	}
	plain := t1Decrypt(enc, 55665)
	if len(plain) < 4 {
		return nil
	}
	lx = &t1Lex{b: plain, p: 4}
	k := &t1Ink{boxes: map[string]csBox{}, steps: csFontSteps}
	lenIV := 4
	var rawSubrs map[int][]byte
	var rawChars map[string][]byte
	for mode := 0; lx.p < len(plain); {
		t := lx.token()
		switch {
		case t == "":
			lx.p = len(plain)
		case t == "/lenIV":
			v, err := strconv.Atoi(lx.token())
			if err != nil || v < 0 || v > 8 {
				return nil
			}
			lenIV = v
		case t == "/Subrs":
			if rawSubrs != nil || rawChars != nil { // a second array (a hybrid font's): not read
				return nil
			}
			rawSubrs, mode = map[int][]byte{}, 1
		case t == "/CharStrings":
			if rawChars != nil {
				return nil
			}
			rawChars, mode = map[string][]byte{}, 2
			for lx.p < len(plain) && lx.token() != "begin" {
			}
		case mode == 1 && t == "dup":
			back := lx.p
			n, err := strconv.Atoi(lx.token())
			if err != nil { // not an entry of the array: "dup /CharStrings …" where the private dictionary goes on
				lx.p = back
				continue
			}
			body, ok := lx.binary(lx.token())
			if !ok || n < 0 || n > csMaxGlyphs || len(rawSubrs) >= csMaxGlyphs {
				return nil
			}
			rawSubrs[n] = body
		case mode == 2 && t == "end":
			mode = 3
			lx.p = len(plain)
		case mode == 2 && len(t) > 1 && t[0] == '/':
			body, ok := lx.binary(lx.token())
			if !ok || len(rawChars) >= csMaxGlyphs {
				return nil
			}
			if _, twice := rawChars[t[1:]]; twice {
				return nil
			}
			rawChars[t[1:]] = body
		}
	}
	if len(rawChars) == 0 {
		return nil
	}
	open := func(body []byte) ([]byte, bool) {
		if len(body) < lenIV {
			return nil, false
		}
		return t1Decrypt(body, 4330)[lenIV:], true
	}
	k.subrs, k.chars = make(map[int][]byte, len(rawSubrs)), make(map[string][]byte, len(rawChars))
	for n, body := range rawSubrs {
		c, ok := open(body)
		if !ok {
			return nil
		}
		k.subrs[n] = c
	}
	for name, body := range rawChars {
		c, ok := open(body)
		if !ok {
			return nil
		}
		k.chars[name] = c
	}
	return k
}

// t1SeacPart names the glyph StandardEncoding puts at a code, for the codes an accented letter is made from: the
// letters, the accents, and the few letters that carry one. Any other code is not read.
func t1SeacPart(code int) (string, bool) {
	if code >= 'A' && code <= 'Z' || code >= 'a' && code <= 'z' {
		return string(rune(code)), true
	}
	name, ok := map[int]string{193: "grave", 194: "acute", 195: "circumflex", 196: "tilde", 197: "macron", 198: "breve",
		199: "dotaccent", 200: "dieresis", 202: "ring", 203: "cedilla", 205: "hungarumlaut", 206: "ogonek", 207: "caron",
		225: "AE", 232: "Lslash", 233: "Oslash", 241: "ae", 245: "dotlessi", 248: "lslash", 249: "oslash"}[code]
	return name, ok
}

func (k *t1Ink) glyph(name string, depth int) (csBox, bool) {
	if box, read := k.boxes[name]; read {
		return box, true
	}
	code, has := k.chars[name]
	if !has || depth > 1 { // a part of an accented letter is not itself one: nothing here can come back to itself
		return csBox{}, false
	}
	run := t1Run{font: k, steps: min(csGlyphSteps, k.steps), depth: depth}
	before := run.steps
	ok := run.exec(code, 0)
	k.steps -= before - run.steps
	if !ok {
		return csBox{}, false
	}
	k.boxes[name] = run.path.box
	return run.path.box, true
}

type t1Run struct {
	font  *t1Ink
	st    [csT1Stack]float64
	n     int
	ps    []float64 // what an OtherSubr left for `pop`
	path  csPath
	flex  int // 0: not in a flex; else one more than the points given so far
	steps int
	depth int // how deep in accented letters
	done  bool
}

func (r *t1Run) exec(c []byte, depth int) bool {
	if depth > csMaxDepth {
		return false
	}
	for i := 0; i < len(c) && !r.done; {
		if r.steps--; r.steps < 0 {
			return false
		}
		b := c[i]
		i++
		if b >= 32 {
			var v float64
			switch {
			case b <= 246:
				v = float64(int(b) - 139)
			case b <= 254:
				if i >= len(c) {
					return false
				}
				if b <= 250 {
					v = float64((int(b)-247)*256 + int(c[i]) + 108)
				} else {
					v = float64(-(int(b)-251)*256 - int(c[i]) - 108)
				}
				i++
			default:
				if i+4 > len(c) {
					return false
				}
				v = float64(int32(binary.BigEndian.Uint32(c[i:])))
				i += 4
			}
			if r.n >= csT1Stack {
				return false
			}
			r.st[r.n] = v
			r.n++
			continue
		}
		n, a, p := r.n, r.st[:r.n], &r.path
		want := func(k int) bool { return n == k }
		switch b {
		case 1, 3: // hstem, vstem
			if !want(2) {
				return false
			}
		case 9: // closepath: the pen stays where it is
		case 13: // hsbw: the pen starts at the left side bearing, on the baseline
			if !want(2) {
				return false
			}
			p.x, p.y = a[0], 0
		case 21:
			if !want(2) {
				return false
			}
			p.move(a[0], a[1])
		case 22:
			if !want(1) {
				return false
			}
			p.move(a[0], 0)
		case 4:
			if !want(1) {
				return false
			}
			p.move(0, a[0])
		case 5:
			if !want(2) {
				return false
			}
			p.line(a[0], a[1])
		case 6:
			if !want(1) {
				return false
			}
			p.line(a[0], 0)
		case 7:
			if !want(1) {
				return false
			}
			p.line(0, a[0])
		case 8:
			if !want(6) {
				return false
			}
			p.curve(a[0], a[1], a[2], a[3], a[4], a[5])
		case 30: // vhcurveto
			if !want(4) {
				return false
			}
			p.curve(0, a[0], a[1], a[2], a[3], 0)
		case 31: // hvcurveto
			if !want(4) {
				return false
			}
			p.curve(a[0], 0, a[1], a[2], 0, a[3])
		case 10: // callsubr
			if n < 1 || a[n-1] != math.Trunc(a[n-1]) {
				return false
			}
			sub, has := r.font.subrs[int(a[n-1])]
			r.n--
			if !has || !r.exec(sub, depth+1) {
				return false
			}
			continue
		case 11:
			return true
		case 14:
			r.done = true
		case 12:
			if i >= len(c) {
				return false
			}
			op := c[i]
			i++
			switch op {
			case 0: // dotsection
			case 1, 2: // vstem3, hstem3
				if !want(6) {
					return false
				}
			case 6: // seac: asb adx ady bchar achar — a letter and an accent set ady up from where it was drawn
				if !want(5) {
					return false
				}
				base, ok1 := t1SeacPart(int(a[3]))
				accent, ok2 := t1SeacPart(int(a[4]))
				if !ok1 || !ok2 || a[3] != math.Trunc(a[3]) || a[4] != math.Trunc(a[4]) {
					return false
				}
				for _, part := range []struct {
					name string
					up   float64
				}{{base, 0}, {accent, a[2]}} {
					before := r.font.steps
					box, ok := r.font.glyph(part.name, r.depth+1)
					r.steps -= before - r.font.steps
					if !ok || r.steps < 0 {
						return false
					}
					if box.inked {
						p.box.mark(box.lo + part.up)
						p.box.mark(box.hi + part.up)
					}
				}
				r.done = true
			case 7: // sbw
				if !want(4) {
					return false
				}
				p.x, p.y = a[0], a[1]
			case 12: // div
				if n < 2 || a[n-1] == 0 {
					return false
				}
				r.st[n-2] = a[n-2] / a[n-1]
				r.n--
				continue
			case 16: // callothersubr: arg1 … argn n othersubr#
				if n < 2 || a[n-2] != math.Trunc(a[n-2]) || a[n-2] < 0 || int(a[n-2]) > n-2 {
					return false
				}
				which, count := a[n-1], int(a[n-2])
				args := a[n-2-count : n-2]
				switch {
				case which == 1 && count == 0: // a flex begins: the next seven moves are its points, not moves
					if r.flex != 0 {
						return false
					}
					p.box.mark(p.y)
					r.flex = 1
				case which == 2 && count == 0: // one point of it; the first is only the reference the rest are set from
					if r.flex == 0 || r.flex > 7 {
						return false
					}
					if r.flex > 1 {
						p.box.mark(p.y)
					}
					r.flex++
				case which == 0 && count == 3: // it ends, and hands back where the pen is
					if r.flex != 8 {
						return false
					}
					r.flex = 0
					r.ps = append(r.ps[:0], args[2], args[1])
				case which == 3 && count == 1: // hint replacement: hands the subroutine number back
					r.ps = append(r.ps[:0], args[0])
				case which == 12 || which == 13: // counter control: takes its numbers, hands nothing back
					r.ps = r.ps[:0]
				default: // multiple masters and anything a font made up
					return false
				}
				r.n = n - 2 - count
				continue
			case 17: // pop
				if len(r.ps) == 0 || n >= csT1Stack {
					return false
				}
				r.st[n] = r.ps[len(r.ps)-1]
				r.ps = r.ps[:len(r.ps)-1]
				r.n++
				continue
			case 33: // setcurrentpoint
				if !want(2) {
					return false
				}
				p.x, p.y = a[0], a[1]
			default:
				return false
			}
		default:
			return false
		}
		r.n = 0
	}
	return true
}

// readType1Ink is a Type 1 program's whole reach. Which glyph a code draws is the encoding's to say, and this does
// not read it: the answer is every glyph's, and is only ever used to tighten.
func readType1Ink(b []byte) *fontInk {
	k := openType1(b)
	if k == nil {
		return nil
	}
	out := &fontInk{lo: math.Inf(1), hi: math.Inf(-1)}
	for name := range k.chars {
		box, ok := k.glyph(name, 0)
		if !ok {
			return nil
		}
		if box.inked {
			out.lo, out.hi = math.Min(out.lo, box.lo/1000), math.Max(out.hi, box.hi/1000)
		}
	}
	if !(out.hi > out.lo) {
		return nil
	}
	return out
}
