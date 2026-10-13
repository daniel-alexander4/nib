package pdfops

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"nib/internal/pdfread"
)

// Print's ink from a charstring program (fontinkcs.go): CFF and Type 1.
//
// What these can reach: each operator against the arithmetic its specification gives, on programs written out here
// byte by byte; every bound, each beside a control one step inside it; the container's tables; the path from a page
// to `MapText.Ink`; and, where this machine has OpenType fonts installed, the reader against x/image's reading of
// the same programs. What they cannot: whether the ink is where a renderer paints it — `build/accuracy.sh`'s
// vertical-reach column, against poppler's pixels — and a real Type 1 program against a second reader in Go (there
// is none; the reader was compared with fontTools while it was written, and the figures are in the ADR).

// op is a charstring operator; cs2 writes a Type 2 charstring from numbers, operators and raw bytes.
type op byte

func cs2(items ...any) []byte {
	var out []byte
	for _, it := range items {
		switch v := it.(type) {
		case int:
			out = append(out, 28, byte(v>>8), byte(v))
		case op:
			out = append(out, byte(v))
		case []byte:
			out = append(out, v...)
		default:
			panic(fmt.Sprintf("cs2: %T", it))
		}
	}
	return out
}

const (
	hstem, vstem, vmoveto, rlineto, hlineto, vlineto, rrcurveto     op = 1, 3, 4, 5, 6, 7, 8
	closepath, callsubr, ret, esc, hsbw, endchar                    op = 9, 10, 11, 12, 13, 14
	hstemhm, hintmask, rmoveto, hmoveto, rcurveline, rlinecurve     op = 18, 19, 21, 22, 24, 25
	vvcurveto, hhcurveto, callgsubr, vhcurveto, hvcurveto, cntrmask op = 26, 27, 29, 30, 31, 20
)

func dictNum(v int) []byte { return []byte{29, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }

func cffIndexBytes(objs [][]byte) []byte {
	out := []byte{byte(len(objs) >> 8), byte(len(objs))}
	if len(objs) == 0 {
		return out
	}
	out = append(out, 4)
	off := 1
	for i := 0; i <= len(objs); i++ {
		out = binary.BigEndian.AppendUint32(out, uint32(off))
		if i < len(objs) {
			off += len(objs[i])
		}
	}
	for _, o := range objs {
		out = append(out, o...)
	}
	return out
}

// cffSpec is a CFF program to write: name-keyed, or CID-keyed when fds is set.
type cffSpec struct {
	chars  [][]byte
	global [][]byte
	local  [][]byte
	top    []byte // extra Top DICT bytes (a FontMatrix, a CharstringType)
	// CID-keyed:
	fds      [][][]byte // each font dictionary's local subroutines
	fdExtra  []byte     // extra bytes in every font dictionary
	fdSelect []byte     // written as given
	charset  []byte     // written as given
}

var stdMatrixDict = []byte{30, 0xa0, 0x01, 0xff, 139, 139, 30, 0xa0, 0x01, 0xff, 139, 139, 12, 7} // .001 0 0 .001 0 0 FontMatrix

var twoThousandMatrixDict = bytes.ReplaceAll(stdMatrixDict, []byte{0xa0, 0x01, 0xff}, []byte{0xa0, 0x02, 0xff})

func (s cffSpec) build() []byte {
	var out []byte
	for pass, at := 0, map[string]int{}; pass < 2; pass++ {
		top := append([]byte{}, s.top...)
		if s.fds != nil {
			top = append(append(top, 139, 139, 139), 12, 30) // ROS
			top = append(append(top, dictNum(at["charset"])...), 15)
			top = append(append(top, dictNum(at["fdselect"])...), 12, 37)
			top = append(append(top, dictNum(at["fdarray"])...), 12, 36)
		} else {
			top = append(append(append(top, dictNum(at["privsize"])...), dictNum(at["priv"])...), 18)
		}
		top = append(append(top, dictNum(at["chars"])...), 17)
		out = []byte{1, 0, 4, 1}
		out = append(out, cffIndexBytes([][]byte{[]byte("f")})...)
		out = append(out, cffIndexBytes([][]byte{top})...)
		out = append(out, cffIndexBytes(nil)...)
		out = append(out, cffIndexBytes(s.global)...)
		at["chars"] = len(out)
		out = append(out, cffIndexBytes(s.chars)...)
		private := func(subrs [][]byte) (size, where int) {
			where = len(out)
			if subrs == nil {
				return 0, where
			}
			d := append(dictNum(6), 19) // Subrs, six bytes on: straight after this dictionary
			out = append(append(out, d...), cffIndexBytes(subrs)...)
			return len(d), where
		}
		if s.fds == nil {
			at["privsize"], at["priv"] = private(s.local)
			continue
		}
		var dicts [][]byte
		for _, subrs := range s.fds {
			size, where := private(subrs)
			dicts = append(dicts, append(append(append(dictNum(size), dictNum(where)...), 18), s.fdExtra...))
		}
		at["fdarray"] = len(out)
		out = append(out, cffIndexBytes(dicts)...)
		at["fdselect"] = len(out)
		out = append(out, s.fdSelect...)
		at["charset"] = len(out)
		out = append(out, s.charset...)
	}
	return out
}

// oneGlyph is a name-keyed program whose glyph 1 is c, between an empty glyph 0 and a short stroke — so the program
// has ink whatever c draws, and a refusal is c's.
func oneGlyph(c []byte) cffSpec {
	return cffSpec{chars: [][]byte{cs2(endchar), c, cs2(0, 1, rlineto, endchar)}}
}

func glyphBox(t *testing.T, b []byte, g int) (csBox, bool) {
	t.Helper()
	k := openCFF(b)
	if k == nil {
		return csBox{}, false
	}
	return k.glyph(g)
}

// TestAType2CharstringReachesWhereItsOperatorsPutIt: every path operator, by the arithmetic of Technical Note #5177.
// The pen starts at the origin; a number is the lowest and highest y any point of the path is given.
func TestAType2CharstringReachesWhereItsOperatorsPutIt(t *testing.T) {
	for name, c := range map[string]struct {
		code   []byte
		lo, hi float64
	}{
		"rlineto":                          {cs2(0, 100, rmoveto, 50, 200, rlineto, endchar), 100, 300},
		"a width before the move":          {cs2(500, 0, 100, rmoveto, 0, -150, rlineto, endchar), -50, 100},
		"a width before hmoveto":           {cs2(500, 30, hmoveto, 9, 60, rlineto, endchar), 0, 60},
		"a move nothing draws from":        {cs2(0, 100, rmoveto, 10, 10, rlineto, 0, 800, rmoveto, endchar), 100, 110},
		"hlineto, alternating":             {cs2(10, vmoveto, 5, 20, 5, -40, hlineto, endchar), -10, 30},
		"vlineto, alternating":             {cs2(20, 5, -40, vlineto, endchar), -20, 20},
		"rrcurveto, by its control points": {cs2(1, 2, 3, 400, 5, -600, rrcurveto, endchar), -198, 402},
		"rcurveline":                       {cs2(0, 10, 0, 10, 0, 10, 0, 70, rcurveline, endchar), 0, 100},
		"rlinecurve":                       {cs2(0, -5, 0, 10, 0, 10, 0, 10, rlinecurve, endchar), -5, 25},
		"vvcurveto":                        {cs2(10, 1, 20, 30, vvcurveto, endchar), 0, 60},
		"vvcurveto, a step across first":   {cs2(9, 10, 1, 20, 30, vvcurveto, endchar), 0, 60},
		"hhcurveto":                        {cs2(10, 1, 20, 30, hhcurveto, endchar), 0, 20},
		"hhcurveto, a step up first":       {cs2(-7, 10, 1, 20, 30, hhcurveto, endchar), -7, 13},
		"vhcurveto":                        {cs2(10, 1, 20, 30, vhcurveto, endchar), 0, 30},
		"vhcurveto, one more":              {cs2(10, 1, 20, 30, -100, vhcurveto, endchar), -70, 30},
		"vhcurveto, two curves":            {cs2(10, 1, 20, 30, 5, 1, -90, 7, vhcurveto, endchar), -60, 30},
		"hvcurveto":                        {cs2(10, 1, 20, 30, hvcurveto, endchar), 0, 50},
		"hvcurveto, two curves":            {cs2(10, 1, 20, 30, 40, 1, 2, 3, hvcurveto, endchar), 0, 92},
		"hflex":                            {cs2(1, 2, 30, 4, 5, 6, 7, esc, []byte{34}, endchar), 0, 30},
		"flex":                             {cs2(0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 0, -40, 50, esc, []byte{35}, endchar), -25, 15},
		"hflex1":                           {cs2(1, 10, 2, 20, 3, 4, 5, -70, 6, esc, []byte{36}, endchar), -40, 30},
		"flex1, wider than tall":           {cs2(100, 1, 100, 2, 100, 3, 100, 4, 100, 5, 9, esc, []byte{37}, endchar), 0, 15},
		"flex1, taller than wide":          {cs2(1, 10, 1, 10, 1, 10, 1, 10, 1, 10, 9, esc, []byte{37}, endchar), 0, 59},
		// One horizontal stem and one vertical left on the stack: two stems, one byte of mask. The mask is 0x0e —
		// `endchar`, were it read as an operator.
		"a hint mask is stepped over":                            {cs2(10, 20, hstemhm, 30, 40, hintmask, []byte{14}, 0, 5, rmoveto, 0, 5, rlineto, endchar), 5, 10},
		"eight stems and one left for the mask: two bytes of it": {cs2(0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, hstemhm, 0, 1, cntrmask, []byte{14, 14}, 0, 5, rlineto, endchar), 0, 5},
		"a number with a fraction":                               {cs2([]byte{255, 0, 1, 0x80, 0}, vmoveto, 0, 2, rlineto, endchar), 1.5, 3.5},
		"the short numbers":                                      {cs2([]byte{139 + 100, 247, 0, 251, 0}, vlineto, endchar), -8, 100},
		"no endchar":                                             {cs2(0, 44, rlineto), 0, 44},
	} {
		box, ok := glyphBox(t, oneGlyph(c.code).build(), 1)
		if !ok || !box.inked || box.lo != c.lo || box.hi != c.hi {
			t.Errorf("%s: reaches %v..%v (inked=%v ok=%v), want %v..%v", name, box.lo, box.hi, box.inked, ok, c.lo, c.hi)
		}
	}
	// A glyph that draws nothing is read, and is not ink.
	if box, ok := glyphBox(t, oneGlyph(cs2(0, 900, rmoveto, endchar)).build(), 1); !ok || box.inked {
		t.Errorf("a glyph that only moves: inked=%v ok=%v, want read and empty", box.inked, ok)
	}
}

// TestASubroutineIsCalledByItsBiasedNumber: local and global, 107 below the number for a short list — and a
// CID-keyed program takes a glyph's local subroutines from the font dictionary FDSelect names for it.
func TestASubroutineIsCalledByItsBiasedNumber(t *testing.T) {
	spec := cffSpec{
		chars:  [][]byte{cs2(endchar), cs2(-107, callsubr, endchar), cs2(-106, callgsubr, endchar), cs2(-107, callsubr, -107, callgsubr, endchar)},
		local:  [][]byte{cs2(0, 77, rlineto, ret)},
		global: [][]byte{cs2(0, -9, rlineto, ret), cs2(0, 33, rlineto, ret)},
	}
	b := spec.build()
	for g, want := range map[int][2]float64{1: {0, 77}, 2: {0, 33}, 3: {0, 77}} {
		if box, ok := glyphBox(t, b, g); !ok || box.lo != want[0] || box.hi != want[1] {
			t.Errorf("glyph %d reaches %v..%v (ok=%v), want %v", g, box.lo, box.hi, ok, want)
		}
	}
	if w := readCFFInk(b, false); w == nil || w.lo != 0 || w.hi != 0.077 {
		t.Errorf("the whole program reaches %+v, want 0..0.077 of an em", w)
	}

	// CID-keyed: glyphs 1 and 2 run the same charstring through two font dictionaries.
	call := cs2(-107, callsubr, endchar)
	cid := cffSpec{
		chars:    [][]byte{cs2(endchar), call, call, call},
		fds:      [][][]byte{{cs2(0, 100, rlineto, ret)}, {cs2(0, 200, rlineto, ret)}},
		fdSelect: []byte{0, 0, 0, 1, 1},
		charset:  []byte{0, 0, 9, 1, 244, 1, 245}, // format 0: glyph 1 is CID 9, glyph 2 CID 500, glyph 3 CID 501
	}
	for name, tables := range map[string][2][]byte{
		"FDSelect 0, charset 0": {cid.fdSelect, cid.charset},
		"FDSelect 3":            {{3, 0, 2, 0, 0, 0, 0, 2, 1, 0, 4}, cid.charset},
		"charset 1":             {cid.fdSelect, {1, 0, 9, 0, 1, 244, 1}}, // a range of one, then 500 and the one after it
		"charset 2":             {cid.fdSelect, {2, 0, 9, 0, 0, 1, 244, 0, 1}},
	} {
		cid.fdSelect, cid.charset = tables[0], tables[1]
		ink := readCFFInk(cid.build(), true)
		if ink == nil || !ink.byCode {
			t.Errorf("%s: not read by CID: %+v", name, ink)
			continue
		}
		for c, want := range map[int]float64{9: 0.1, 500: 0.2, 501: 0.2} {
			if _, hi, inked, ok := ink.glyph(c); !ok || !inked || hi != want {
				t.Errorf("%s: CID %d reaches up %v (inked=%v ok=%v), want %v", name, c, hi, inked, ok, want)
			}
		}
		// A CID the charset does not hold is drawn as glyph 0, which here is empty.
		if _, _, inked, ok := ink.glyph(3); !ok || inked {
			t.Errorf("%s: CID 3, which the charset does not hold: inked=%v ok=%v, want glyph 0's nothing", name, inked, ok)
		}
	}
	// A name-keyed program under a CID font numbers its glyphs by CID; past the last, glyph 0.
	ink := readCFFInk(b, true)
	if _, hi, _, ok := ink.glyph(2); !ok || hi != 0.033 {
		t.Errorf("a name-keyed program by CID: CID 2 reaches up %v (ok=%v), want glyph 2's 0.033", hi, ok)
	}
	if _, _, inked, ok := ink.glyph(40); !ok || inked {
		t.Errorf("a CID past the program's last glyph: inked=%v ok=%v, want glyph 0's nothing", inked, ok)
	}
}

// TestARunShowingAGlyphThatIsNotReadHasNoInk: a font read a glyph at a time finds a glyph it cannot vouch for only
// when a run shows it. That run has no ink — it keeps its line's reach — and neither has any run after it.
func TestARunShowingAGlyphThatIsNotReadHasNoInk(t *testing.T) {
	b := cffSpec{chars: [][]byte{cs2(endchar), cs2(0, 300, rlineto, endchar), cs2(1, 2, esc, []byte{10}, 0, 900, rlineto, endchar)}}.build()
	face := &runFont{ink: &inkSource{done: true, ink: readCFFInk(b, true)}}
	run := func(codes ...int) textRun {
		r := textRun{text: "x", size: 10, x: 50, y: 100, width: 5, face: face}
		for _, c := range codes {
			r.glyphs = append(r.glyphs, runGlyph{code: []byte{byte(c >> 8), byte(c)}, advance: 1})
		}
		return r
	}
	if bottom, top, ok := printInk(run(1)); !ok || bottom != 100 || top != 103 {
		t.Fatalf("the glyph that reads: ink %v..%v (ok=%v), want 100..103", bottom, top, ok)
	}
	if _, top, ok := printInk(run(1, 2)); ok {
		t.Errorf("a run showing a glyph that is not read was given ink, up to %v", top)
	}
	if _, _, ok := printInk(run(1)); ok {
		t.Error("a program with a glyph that is not read still gives ink")
	}
}

func repeated(n int, items ...any) []any {
	var out []any
	for i := 0; i < n; i++ {
		out = append(out, items...)
	}
	return out
}

// TestACFFProgramPastABoundGivesNothing: each bound, with the program one step inside it read and the program one
// step past it refused. A refusal is no ink for the font — the run keeps its line's reach — and never a panic.
func TestACFFProgramPastABoundGivesNothing(t *testing.T) {
	// chain is n subroutines, each calling the next, the last drawing.
	chain := func(n int) cffSpec {
		s := cffSpec{chars: [][]byte{cs2(endchar), cs2(-107, callsubr, endchar)}}
		for i := 0; i < n; i++ {
			if i == n-1 {
				s.local = append(s.local, cs2(0, 50, rlineto, ret))
			} else {
				s.local = append(s.local, cs2(i+1-107, callsubr, ret))
			}
		}
		return s
	}
	long := func(lines int) []byte { return cs2(append(repeated(lines, 0, 1, rlineto), endchar)...) }
	// many is n glyphs, each spending 90,003 steps in one shared subroutine.
	many := func(n int) cffSpec {
		s := cffSpec{chars: [][]byte{cs2(endchar)}, global: [][]byte{cs2(append(repeated(30_000, 0, 1, rlineto), ret)...)}}
		for i := 0; i < n; i++ {
			s.chars = append(s.chars, cs2(-107, callgsubr, endchar))
		}
		return s
	}
	with := func(s cffSpec, change func(*cffSpec)) cffSpec { change(&s); return s }
	plain := oneGlyph(cs2(0, 50, rlineto, endchar))
	cid := cffSpec{chars: plain.chars, fds: [][][]byte{nil}, fdSelect: []byte{0, 0, 0, 0}, charset: []byte{0, 0, 5, 0, 6}}
	for name, c := range map[string]struct {
		spec cffSpec
		read bool
	}{
		"ten calls deep":                                    {chain(10), true},
		"eleven calls deep":                                 {chain(11), false},
		"48 numbers on the stack":                           {oneGlyph(cs2(append(repeated(48, 1), hlineto, endchar)...)), true},
		"49 numbers on the stack":                           {oneGlyph(cs2(append(repeated(49, 1), hlineto, endchar)...)), false},
		"a glyph of 90,001 steps":                           {oneGlyph(long(30_000)), true},
		"a glyph of 120,001 steps":                          {oneGlyph(long(40_000)), false},
		"80 glyphs of 90,003 steps":                         {many(80), true},
		"100 glyphs of 90,003 steps":                        {many(100), false},
		"a subroutine the program does not have":            {oneGlyph(cs2(-107, callsubr, endchar)), false},
		"an arithmetic operator":                            {oneGlyph(cs2(1, 2, esc, []byte{10}, 0, 5, rlineto, endchar)), false},
		"endchar as an accented letter":                     {oneGlyph(cs2(0, 0, 65, 66, endchar)), false},
		"rlineto with one number":                           {oneGlyph(cs2(5, rlineto, endchar)), false},
		"rmoveto with none":                                 {oneGlyph(cs2(rmoveto, endchar)), false},
		"a reserved operator":                               {oneGlyph(cs2(0, 5, rlineto, op(15), endchar)), false},
		"a hint mask that runs off the end":                 {oneGlyph(cs2(1, 2, hstemhm, hintmask)), false},
		"a matrix of a thousand to the em":                  {with(plain, func(s *cffSpec) { s.top = stdMatrixDict }), true},
		"a matrix of two thousand":                          {with(plain, func(s *cffSpec) { s.top = twoThousandMatrixDict }), false},
		"Type 1 charstrings in a CFF":                       {with(plain, func(s *cffSpec) { s.top = []byte{140, 12, 6} }), false},
		"a CID program":                                     {cid, true},
		"a CID program whose dictionaries carry the matrix": {with(cid, func(s *cffSpec) { s.fdExtra = stdMatrixDict }), true},
		"a CID program with the matrix twice":               {with(cid, func(s *cffSpec) { s.top, s.fdExtra = stdMatrixDict, stdMatrixDict }), false},
		"FDSelect naming a dictionary it lacks":             {with(cid, func(s *cffSpec) { s.fdSelect = []byte{0, 0, 1, 0} }), false},
		"FDSelect 3 that stops short":                       {with(cid, func(s *cffSpec) { s.fdSelect = []byte{3, 0, 1, 0, 0, 0, 0, 1} }), false},
		"a charset that stops short":                        {with(cid, func(s *cffSpec) { s.charset = []byte{0, 0} }), false},
	} {
		if got := readCFFInk(c.spec.build(), false) != nil; got != c.read {
			t.Errorf("%s: read=%v, want %v", name, got, c.read)
		}
	}
}

// ---- Type 1 ----

func t1Encrypt(b []byte, key uint16) []byte {
	out := make([]byte, len(b))
	for i, p := range b {
		c := p ^ byte(key>>8)
		out[i] = c
		key = (uint16(c)+key)*52845 + 22719
	}
	return out
}

// cs1 writes a Type 1 charstring; a number is the five-byte form.
func cs1(items ...any) []byte {
	var out []byte
	for _, it := range items {
		switch v := it.(type) {
		case int:
			out = binary.BigEndian.AppendUint32(append(out, 255), uint32(int32(v)))
		case op:
			out = append(out, byte(v))
		case []byte:
			out = append(out, v...)
		default:
			panic(fmt.Sprintf("cs1: %T", it))
		}
	}
	return out
}

var (
	t1div           = []byte{12, 12}
	t1callothersubr = []byte{12, 16}
	t1pop           = []byte{12, 17}
	t1setcurrentpt  = []byte{12, 33}
	t1seac          = []byte{12, 6}
	t1sbw           = []byte{12, 7}
	t1dotsection    = []byte{12, 0}
	t1hstem3        = []byte{12, 2}
)

type t1Spec struct {
	matrix string
	lenIV  int
	subrs  [][]byte
	names  []string
	chars  [][]byte
	hex    bool
	twice  bool // a second /Subrs array
	says   *int // the /lenIV written, where it is not the one the charstrings were sealed with
}

func (s t1Spec) build() []byte {
	if s.matrix == "" {
		s.matrix = "[0.001 0 0 0.001 0 0]"
	}
	sealed := func(c []byte) []byte { return t1Encrypt(append(make([]byte, s.lenIV), c...), 4330) }
	var p bytes.Buffer
	p.WriteString("zzzzdup /Private 8 dict dup begin\n/RD {string currentfile exch readstring pop} executeonly def\n")
	says := s.lenIV
	if s.says != nil {
		says = *s.says
	}
	fmt.Fprintf(&p, "/lenIV %d def\n/OtherSubrs[{}{}{}{}] noaccess def\n", says)
	for n := 0; n < 1+map[bool]int{true: 1}[s.twice]; n++ {
		fmt.Fprintf(&p, "/Subrs %d array\n", len(s.subrs))
		for i, c := range s.subrs {
			body := sealed(c)
			fmt.Fprintf(&p, "dup %d %d RD %s NP\n", i, len(body), body)
		}
		p.WriteString("ND\n")
	}
	fmt.Fprintf(&p, "end noaccess put\ndup /CharStrings %d dict dup begin\n", len(s.chars))
	for i, c := range s.chars {
		body := sealed(c)
		fmt.Fprintf(&p, "/%s %d RD %s ND\n", s.names[i], len(body), body)
	}
	p.WriteString("end\nend\nreadonly put\nnoaccess put\ndup /FontName get exch definefont pop\nmark currentfile closefile\n")
	enc := t1Encrypt(p.Bytes(), 55665)
	if s.hex {
		enc = []byte(hex.EncodeToString(enc[:40]) + "\n" + strings.ToUpper(hex.EncodeToString(enc[40:])))
	}
	out := []byte("%!PS-AdobeFont-1.0: T 001.001\n11 dict begin\n/FontName /T def\n/FontMatrix " + s.matrix + " readonly def\n/Encoding StandardEncoding def\ncurrentdict end\ncurrentfile eexec\n")
	out = append(out, enc...)
	return append(out, []byte("\n"+strings.Repeat("0", 64)+"\ncleartomark\n")...)
}

// t1Glyph is a program whose glyph `a` is c — with what subroutines it needs — beside an empty glyph and a short stroke.
func t1Glyph(c []byte, subrs ...[]byte) t1Spec {
	return t1Spec{lenIV: 4, names: []string{".notdef", "a", "b"}, subrs: subrs,
		chars: [][]byte{cs1(0, 500, hsbw, endchar), c, cs1(0, 500, hsbw, 0, 1, rlineto, endchar)}}
}

// The standard flex subroutines of a Type 1 program: 0 ends a flex, 1 begins one, 2 gives a point, 3 is hint replacement.
var t1FlexSubrs = [][]byte{
	cs1(3, 0, t1callothersubr, t1pop, t1pop, t1setcurrentpt, ret),
	cs1(0, 1, t1callothersubr, ret),
	cs1(0, 2, t1callothersubr, ret),
	cs1(ret),
}

// TestAType1CharstringReachesWhereItsOperatorsPutIt: by the arithmetic of "Adobe Type 1 Font Format". The pen starts
// on the baseline at the side bearing.
func TestAType1CharstringReachesWhereItsOperatorsPutIt(t *testing.T) {
	flex := cs1(50, 500, hsbw, 0, 100, rmoveto, 0, 10, rlineto, 1, callsubr,
		0, 900, rmoveto, 2, callsubr, // the reference point: 900 up, and not part of the outline
		10, -890, rmoveto, 2, callsubr, 10, 5, rmoveto, 2, callsubr, 10, 5, rmoveto, 2, callsubr,
		10, -5, rmoveto, 2, callsubr, 10, -5, rmoveto, 2, callsubr, 10, -10, rmoveto, 2, callsubr,
		50, 110, 110, 0, callsubr, 0, -60, rlineto, closepath, endchar)
	for name, c := range map[string]struct {
		spec   t1Spec
		lo, hi float64
	}{
		"rlineto from the baseline":        {t1Glyph(cs1(50, 500, hsbw, 0, 100, rmoveto, 5, 200, rlineto, closepath, endchar)), 100, 300},
		"sbw starts off the baseline":      {t1Glyph(cs1(50, -30, 500, 0, t1sbw, 0, 10, rlineto, endchar)), -30, -20},
		"hlineto, vlineto":                 {t1Glyph(cs1(0, 500, hsbw, 7, vmoveto, 9, hlineto, -20, vlineto, 3, hmoveto, 1, vlineto, endchar)), -13, 7},
		"rrcurveto, by its control points": {t1Glyph(cs1(0, 500, hsbw, 1, 2, 3, 400, 5, -600, rrcurveto, endchar)), -198, 402},
		"vhcurveto":                        {t1Glyph(cs1(0, 500, hsbw, 10, 1, 20, 30, vhcurveto, endchar)), 0, 30},
		"hvcurveto":                        {t1Glyph(cs1(0, 500, hsbw, 10, 1, 20, 30, hvcurveto, endchar)), 0, 50},
		"div":                              {t1Glyph(cs1(0, 500, hsbw, 0, 301, 2, t1div, rlineto, endchar)), 0, 150.5},
		"hints":                            {t1Glyph(cs1(0, 500, hsbw, 0, 20, hstem, 1, 2, 3, 4, 5, 6, t1hstem3, t1dotsection, 0, 8, rlineto, endchar)), 0, 8},
		"a subroutine":                     {t1Glyph(cs1(0, 500, hsbw, 1, callsubr, endchar), cs1(ret), cs1(0, 66, rlineto, ret)), 0, 66},
		"hint replacement":                 {t1Glyph(cs1(0, 500, hsbw, 3, 1, 3, t1callothersubr, t1pop, callsubr, 0, 21, rlineto, endchar), t1FlexSubrs...), 0, 21},
		"a flex":                           {t1Glyph(flex, t1FlexSubrs...), 50, 130},
		"the short numbers":                {t1Glyph(cs1(0, 500, hsbw, []byte{139 + 100}, vlineto, []byte{251, 0}, vlineto, endchar)), -8, 100},
	} {
		for _, form := range []string{"binary", "hexadecimal", "no lenIV bytes"} {
			spec := c.spec
			spec.hex = form == "hexadecimal"
			if form == "no lenIV bytes" {
				spec.lenIV = 0
			}
			k := openType1(spec.build())
			if k == nil {
				t.Errorf("%s (%s): the program did not open", name, form)
				continue
			}
			if box, ok := k.glyph("a", 0); !ok || !box.inked || box.lo != c.lo || box.hi != c.hi {
				t.Errorf("%s (%s): reaches %v..%v (inked=%v ok=%v), want %v..%v", name, form, box.lo, box.hi, box.inked, ok, c.lo, c.hi)
			}
		}
	}
}

// TestAnAccentedLetterIsItsLetterAndItsAccentSetUp: `seac` draws two other glyphs, the accent moved by ady. The
// parts are named by StandardEncoding's codes: 97 is a, 194 acute.
func TestAnAccentedLetterIsItsLetterAndItsAccentSetUp(t *testing.T) {
	spec := t1Spec{lenIV: 4, names: []string{".notdef", "a", "acute", "aacute", "odd", "self", "o"}, chars: [][]byte{
		cs1(0, 500, hsbw, endchar),
		cs1(0, 500, hsbw, 0, -10, rmoveto, 0, 460, rlineto, endchar), // -10..450
		cs1(0, 300, hsbw, 0, 500, rmoveto, 0, 180, rlineto, endchar), // 500..680
		cs1(0, 500, hsbw, 0, 100, 120, 97, 194, t1seac),              // a, and acute 120 higher: -10..800
		cs1(0, 500, hsbw, 0, 100, 120, 97, 36, t1seac),               // 36 is not a letter or an accent
		cs1(0, 500, hsbw, 0, 100, 120, 111, 194, t1seac),             // o and acute: an accented letter in its own right
		cs1(0, 500, hsbw, 0, 400, rlineto, endchar),
	}}
	k := openType1(spec.build())
	if k == nil {
		t.Fatal("the program did not open")
	}
	if box, ok := k.glyph("aacute", 0); !ok || box.lo != -10 || box.hi != 800 {
		t.Errorf("aacute reaches %v..%v (ok=%v), want -10..800", box.lo, box.hi, ok)
	}
	if _, ok := k.glyph("odd", 0); ok {
		t.Error("an accented letter made from a code that is not a letter or an accent was read")
	}
	// An accented letter whose letter is itself an accented letter is not followed: here a is o with an acute.
	spec.names[1], spec.names[5] = "self", "a"
	if k = openType1(spec.build()); k == nil {
		t.Fatal("the program did not open")
	}
	if _, ok := k.glyph("aacute", 0); ok {
		t.Error("an accented letter whose letter is itself an accented letter was read")
	}
	if readType1Ink(spec.build()) != nil {
		t.Error("a program with a glyph that is not read was given ink")
	}
}

// TestAType1ProgramPastABoundGivesNothing: as for CFF — one step inside read, one step past refused, never a panic.
func TestAType1ProgramPastABoundGivesNothing(t *testing.T) {
	chain := func(n int) t1Spec {
		s := t1Glyph(cs1(0, 500, hsbw, 0, callsubr, endchar))
		for i := 0; i < n; i++ {
			if i == n-1 {
				s.subrs = append(s.subrs, cs1(0, 50, rlineto, ret))
			} else {
				s.subrs = append(s.subrs, cs1(i+1, callsubr, ret))
			}
		}
		return s
	}
	draw := cs1(0, 500, hsbw, 0, 50, rlineto, endchar)
	stack := func(n int) []byte {
		return cs1(append(append([]any{0, 500, hsbw}, repeated(n, 1)...), []byte{12, 1}, endchar)...)
	}
	long := func(lines int) []byte {
		return cs1(append(append([]any{0, 500, hsbw}, repeated(lines, 0, 1, rlineto)...), endchar)...)
	}
	many := func(n int) t1Spec {
		s := t1Spec{lenIV: 4, subrs: [][]byte{cs1(append(repeated(30_000, 0, 1, rlineto), ret)...)}}
		for i := 0; i < n; i++ {
			s.names, s.chars = append(s.names, fmt.Sprintf("g%d", i)), append(s.chars, cs1(0, 500, hsbw, 0, callsubr, endchar))
		}
		return s
	}
	with := func(s t1Spec, change func(*t1Spec)) t1Spec { change(&s); return s }
	for name, c := range map[string]struct {
		spec t1Spec
		read bool
	}{
		"plain":                           {t1Glyph(draw), true},
		"ten calls deep":                  {chain(10), true},
		"eleven calls deep":               {chain(11), false},
		"a glyph of 90,004 steps":         {t1Glyph(long(30_000)), true},
		"a glyph of 120,004 steps":        {t1Glyph(long(40_000)), false},
		"80 glyphs of 90,006 steps":       {many(80), true},
		"100 glyphs of 90,006 steps":      {many(100), false},
		"24 numbers on the stack":         {t1Glyph(cs1(append(append([]any{0, 500, hsbw}, repeated(24, 1)...), endchar)...)), true},
		"25 numbers on the stack":         {t1Glyph(cs1(append(append([]any{0, 500, hsbw}, repeated(25, 1)...), endchar)...)), false},
		"vstem3 with six numbers":         {t1Glyph(stack(6)), true},
		"vstem3 with five":                {t1Glyph(stack(5)), false},
		"a subroutine it does not have":   {t1Glyph(cs1(0, 500, hsbw, 4, callsubr, endchar)), false},
		"an OtherSubr it does not know":   {t1Glyph(cs1(0, 500, hsbw, 0, 14, t1callothersubr, 0, 5, rlineto, endchar)), false},
		"a flex ended after three points": {t1Glyph(cs1(0, 500, hsbw, 1, callsubr, 0, 1, rmoveto, 2, callsubr, 0, 1, rmoveto, 2, callsubr, 0, 1, rmoveto, 2, callsubr, 50, 0, 3, 0, callsubr, endchar), t1FlexSubrs...), false},
		"pop with nothing to pop":         {t1Glyph(cs1(0, 500, hsbw, t1pop, endchar)), false},
		"division by nothing":             {t1Glyph(cs1(0, 500, hsbw, 0, 5, 0, t1div, rlineto, endchar)), false},
		"a Type 2 operator":               {t1Glyph(cs1(0, 500, hsbw, 0, 5, rlineto, 1, 2, 3, 4, hhcurveto, endchar)), false},
		"a matrix of two thousand":        {with(t1Glyph(draw), func(s *t1Spec) { s.matrix = "[0.0005 0 0 0.0005 0 0]" }), false},
		"a slanted matrix":                {with(t1Glyph(draw), func(s *t1Spec) { s.matrix = "[0.001 0 0.0002 0.001 0 0]" }), false},
		"a matrix in braces":              {with(t1Glyph(draw), func(s *t1Spec) { s.matrix = "{0.001 0 0 0.001 0 0}" }), true},
		"two Subrs arrays":                {with(t1Glyph(draw), func(s *t1Spec) { s.twice = true }), false},
		"eight lenIV bytes":               {with(t1Glyph(draw), func(s *t1Spec) { s.lenIV = 8 }), true},
	} {
		if got := readType1Ink(c.spec.build()) != nil; got != c.read {
			t.Errorf("%s: read=%v, want %v", name, got, c.read)
		}
	}
	// A charstring must be at least as long as the random bytes its program says lead it.
	for says, opens := range map[int]bool{1: true, 2: false} {
		short := t1Spec{names: []string{"a"}, chars: [][]byte{cs1(endchar)}, says: &says}
		if got := openType1(short.build()) != nil; got != opens {
			t.Errorf("a one-byte charstring under /lenIV %d: opened=%v, want %v", says, got, opens)
		}
	}
}

// TestACharstringProgramCutOrBentAnywhereNeverPanics: cut at every byte, and with every byte changed three ways, a
// program gives an answer inside the whole one's or none. The answer may not GROW past the whole program's for a
// cut — a reach is only ever what some glyph's points say.
func TestACharstringProgramCutOrBentAnywhereNeverPanics(t *testing.T) {
	cff := cffSpec{
		chars:  [][]byte{cs2(endchar), cs2(-107, callsubr, 1, 2, 3, 400, 5, -600, rrcurveto, endchar), cs2(-107, callgsubr, endchar)},
		local:  [][]byte{cs2(0, 77, rlineto, ret)},
		global: [][]byte{cs2(1, 2, hstemhm, hintmask, []byte{0x80}, 0, -9, rlineto, ret)},
		top:    stdMatrixDict,
	}.build()
	cid := cffSpec{chars: [][]byte{cs2(endchar), cs2(-107, callsubr, endchar), cs2(0, 5, rlineto, endchar)},
		fds: [][][]byte{{cs2(0, 100, rlineto, ret)}, nil}, fdSelect: []byte{3, 0, 2, 0, 0, 0, 0, 2, 1, 0, 3}, charset: []byte{2, 0, 9, 0, 1}}.build()
	t1 := t1Glyph(cs1(50, 500, hsbw, 0, 100, rmoveto, 1, callsubr, 3, 1, 3, t1callothersubr, t1pop, callsubr, endchar), t1FlexSubrs[0], cs1(0, 66, rlineto, ret), t1FlexSubrs[2], t1FlexSubrs[3]).build()
	for name, c := range map[string]struct {
		b    []byte
		read func([]byte) *fontInk
	}{
		"CFF":     {cff, func(b []byte) *fontInk { return readCFFInk(b, false) }},
		"CID CFF": {cid, func(b []byte) *fontInk { return readCFFInk(b, false) }},
		"Type 1":  {t1, readType1Ink},
	} {
		whole := c.read(c.b)
		if whole == nil {
			t.Errorf("%s: the control did not read", name)
			continue
		}
		refused := 0
		for n := 0; n < len(c.b); n++ {
			got := c.read(c.b[:n])
			if got == nil {
				refused++
			} else if got.lo < whole.lo-1e-9 || got.hi > whole.hi+1e-9 {
				t.Errorf("%s cut at %d of %d: reads %v..%v, outside the whole program's %v..%v", name, n, len(c.b), got.lo, got.hi, whole.lo, whole.hi)
			}
			bent := append([]byte{}, c.b...)
			for _, x := range []byte{0xff, 0x01, 0x80} {
				bent[n] = c.b[n] ^ x
				if k := readCFFInk(bent, true); k != nil { // by CID too: every glyph asked for, and one it lacks
					for g := 0; g < 12; g++ {
						k.glyph(g)
					}
				}
				c.read(bent)
			}
		}
		if refused < len(c.b)/2 {
			t.Errorf("%s: only %d of %d cuts were refused", name, refused, len(c.b))
		}
	}
}

// ---- which font gives which ink, and the page ----

// TestWhichCharstringFontsAreRead: a Type1 font from /FontFile or /FontFile3, a CIDFontType0 from /FontFile3 — by
// CID under Identity-H alone. Never a program under a key its font's subtype does not draw from.
func TestWhichCharstringFontsAreRead(t *testing.T) {
	xt := &model.XRefTable{}
	prog := types.StreamDict{Dict: types.Dict{}}
	desc := func(key string) types.Dict { return types.Dict{key: prog} }
	simple := func(sub, key string) types.Dict {
		return types.Dict{"Subtype": types.Name(sub), "FontDescriptor": desc(key)}
	}
	type0 := func(enc, kid, key string) types.Dict {
		return types.Dict{"Subtype": types.Name("Type0"), "Encoding": types.Name(enc),
			"DescendantFonts": types.Array{types.Dict{"Subtype": types.Name(kid), "FontDescriptor": desc(key)}}}
	}
	for name, c := range map[string]struct {
		d      types.Dict
		kind   int // -1: no source
		byCode bool
	}{
		"Type1, /FontFile":                {simple("Type1", "FontFile"), inkType1, false},
		"Type1, /FontFile3":               {simple("Type1", "FontFile3"), inkCFF, false},
		"Type1, /FontFile2":               {simple("Type1", "FontFile2"), -1, false},
		"MMType1":                         {simple("MMType1", "FontFile"), -1, false},
		"Type3":                           {simple("Type3", "FontFile3"), -1, false},
		"TrueType, /FontFile3":            {simple("TrueType", "FontFile3"), -1, false},
		"TrueType, /FontFile2":            {simple("TrueType", "FontFile2"), inkGlyf, false},
		"CIDFontType0 under Identity-H":   {type0("Identity-H", "CIDFontType0", "FontFile3"), inkCFF, true},
		"CIDFontType0 under Identity-V":   {type0("Identity-V", "CIDFontType0", "FontFile3"), inkCFF, false},
		"CIDFontType0 under another CMap": {type0("UniJIS-UCS2-H", "CIDFontType0", "FontFile3"), inkCFF, false},
		"CIDFontType0, /FontFile2":        {type0("Identity-H", "CIDFontType0", "FontFile2"), -1, false},
		"CIDFontType2, /FontFile3":        {type0("Identity-H", "CIDFontType2", "FontFile3"), -1, false},
	} {
		s := inkSourceFor(xt, c.d)
		if (s == nil) != (c.kind < 0) || (s != nil && (s.kind != c.kind || s.byCode != c.byCode)) {
			t.Errorf("%s: source %+v, want kind %d byCode=%v", name, s, c.kind, c.byCode)
		}
	}
}

// programStream renders a stream object with a dictionary of its own: a font program, a CMap, a page.
func programStream(dict string, body []byte) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(body), body)
}

// TestAPagesPrintInACharstringFontCarriesItsInk: the whole path, on a page written by hand in three fonts — a Type1
// font over a CFF program, one over a Type 1 program, and a Type0 font over a CID-keyed CFF program.
//
// The two simple fonts answer for their whole program (0.7 up, 0.2 down — tighter than the line's 1 and 0.25); the
// Type0 font answers for the glyphs each run shows, and CID 7's tail, half a size down, takes the box PAST the reach.
func TestAPagesPrintInACharstringFontCarriesItsInk(t *testing.T) {
	cff := cffSpec{chars: [][]byte{cs2(endchar), cs2(0, 700, rlineto, endchar), cs2(0, -200, rlineto, endchar)}}.build()
	t1 := t1Spec{lenIV: 4, names: []string{".notdef", "A", "B"}, chars: [][]byte{cs1(0, 500, hsbw, endchar),
		cs1(0, 500, hsbw, 0, 700, rlineto, endchar), cs1(0, 500, hsbw, 0, -200, rlineto, endchar)}}.build()
	cid := cffSpec{chars: [][]byte{cs2(endchar), cs2(0, 300, rlineto, endchar), cs2(0, -500, rlineto, 0, 600, rlineto, endchar)},
		fds: [][][]byte{nil}, fdSelect: []byte{0, 0, 0, 0}, charset: []byte{0, 0, 5, 0, 7}}.build()
	toUni := "/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapName /T def /CMapType 2 def\n" +
		"1 begincodespacerange <0000> <FFFF> endcodespacerange\n2 beginbfchar <0005> <0078> <0007> <0079> endbfchar\nendcmap end end"
	descriptor := func(name, key string, n int) string {
		return fmt.Sprintf("<< /Type /FontDescriptor /FontName /%s /Flags 4 /FontBBox [0 -500 500 700] /ItalicAngle 0 /Ascent 700 /Descent -200 /CapHeight 700 /StemV 80 /%s %d 0 R >>", name, key, n)
	}
	content := "BT /F1 10 Tf 20 150 Td (AB) Tj ET BT /F2 10 Tf 20 120 Td (AB) Tj ET BT /F3 10 Tf 20 80 Td <0005> Tj ET BT /F3 10 Tf 20 40 Td <00050007> Tj ET"
	pdf := assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R /F2 8 0 R /F3 11 0 R >> >> >>",
		4:  programStream("", []byte(content)),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /AAAAAA+One /FirstChar 65 /LastChar 66 /Widths [500 500] /Encoding /WinAnsiEncoding /FontDescriptor 6 0 R >>",
		6:  descriptor("AAAAAA+One", "FontFile3", 7),
		7:  programStream("/Subtype /Type1C", cff),
		8:  "<< /Type /Font /Subtype /Type1 /BaseFont /AAAAAB+Two /FirstChar 65 /LastChar 66 /Widths [500 500] /Encoding /WinAnsiEncoding /FontDescriptor 9 0 R >>",
		9:  descriptor("AAAAAB+Two", "FontFile", 10),
		10: programStream(fmt.Sprintf("/Length1 %d /Length2 %d /Length3 0", bytes.Index(t1, []byte("eexec\n"))+6, len(t1)-bytes.Index(t1, []byte("eexec\n"))-6), t1),
		11: "<< /Type /Font /Subtype /Type0 /BaseFont /AAAAAC+Three /Encoding /Identity-H /DescendantFonts [12 0 R] /ToUnicode 15 0 R >>",
		12: "<< /Type /Font /Subtype /CIDFontType0 /BaseFont /AAAAAC+Three /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /DW 500 /FontDescriptor 13 0 R >>",
		13: descriptor("AAAAAC+Three", "FontFile3", 14),
		14: programStream("/Subtype /CIDFontType0C", cid),
		15: programStream("", []byte(toUni)),
	})
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageShapes(ctx, pageAt(ctx, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	want := map[float64][2]float64{150: {148, 157}, 120: {118, 127}, 80: {80, 83}, 40: {35, 43}}
	seen := 0
	for _, r := range pr.runs {
		w, known := want[r.y]
		if !known {
			t.Fatalf("a run at %v the page was not written with: %+v", r.y, r)
		}
		seen++
		bottom, top, ok := printInk(r)
		if !ok || math.Abs(bottom-w[0]) > 1e-9 || math.Abs(top-w[1]) > 1e-9 {
			t.Errorf("the run at %v in %s: ink %v..%v (ok=%v), want %v", r.y, r.baseFont, bottom, top, ok, w)
		}
	}
	if seen != len(want) {
		t.Fatalf("%d runs read, want %d", seen, len(want))
	}
	m, err := MapPage(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Text) != len(want) {
		t.Fatalf("the map has %d runs of text, want %d", len(m.Text), len(want))
	}
	for _, mt := range m.Text {
		if len(mt.Ink) != 2 || !(mt.Ink[0] < mt.Ink[1]) {
			t.Errorf("%q has no ink on the map: %+v", mt.Text, mt)
		}
	}
}

// TestInstalledCFFProgramsReachWhereAnotherReaderSaysTheyDo: every glyph of the OpenType-CFF fonts this machine has,
// against x/image's reading of the same charstrings — the two share nothing but the file. Skips where none is
// installed; the operators' own arithmetic is held above without it.
func TestInstalledCFFProgramsReachWhereAnotherReaderSaysTheyDo(t *testing.T) {
	var files []string
	for _, g := range []string{"/usr/share/fonts/opentype/*/*.otf", "/usr/share/fonts/OTF/*.otf", "/Library/Fonts/*.otf"} {
		found, _ := filepath.Glob(g)
		files = append(files, found...)
	}
	if len(files) == 0 {
		t.Skip("no OpenType-CFF font is installed on this machine")
	}
	if len(files) > 24 {
		files = files[:24]
	}
	fonts, checked := 0, 0
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		face, err := sfnt.Parse(b)
		k := openCFF(b)
		if err != nil || k == nil || face.UnitsPerEm() != 1000 {
			continue
		}
		fonts++
		var buf sfnt.Buffer
		for g := 0; g < face.NumGlyphs() && g < k.chars.count; g++ {
			segs, err := face.LoadGlyph(&buf, sfnt.GlyphIndex(g), fixed.I(1000), nil)
			if err != nil {
				continue // a glyph x/image does not read says nothing about this reader
			}
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, s := range segs {
				n := map[sfnt.SegmentOp]int{sfnt.SegmentOpMoveTo: 1, sfnt.SegmentOpLineTo: 1, sfnt.SegmentOpQuadTo: 2, sfnt.SegmentOpCubeTo: 3}[s.Op]
				for _, p := range s.Args[:n] {
					lo, hi = math.Min(lo, -float64(p.Y)/64), math.Max(hi, -float64(p.Y)/64)
				}
			}
			box, ok := k.glyph(g)
			if !ok {
				t.Fatalf("%s glyph %d: not read", filepath.Base(path), g)
			}
			if box.inked != (len(segs) > 0) {
				t.Errorf("%s glyph %d: inked=%v, x/image draws %d segments", filepath.Base(path), g, box.inked, len(segs))
				continue
			}
			if box.inked && (math.Abs(box.lo-lo) > 0.02 || math.Abs(box.hi-hi) > 0.02) {
				t.Errorf("%s glyph %d: reaches %v..%v, x/image %v..%v", filepath.Base(path), g, box.lo, box.hi, lo, hi)
			}
			checked++
		}
	}
	if fonts == 0 || checked == 0 {
		t.Skipf("none of %d installed fonts is one both readers read", len(files))
	}
	t.Logf("%d glyphs of %d fonts", checked, fonts)
}
