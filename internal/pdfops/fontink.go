package pdfops

import (
	"encoding/binary"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// How far a run of PRINT reaches up and down the page, from the font's own glyphs (ADR-098, /pending 851 parts 1, 2).
//
// `runBox` gives every run a full size above its baseline and a quarter below. That is a line's reach, and a
// search-redaction drawn to it took the tails of the line above for 792 of 6,490 words of the accuracy corpus
// (median 1.1pt, `build/accuracy.sh`'s vertical-reach column), while a descender deeper than a quarter of a size is
// left showing. ADR-093 gave a fitted OCR word its own ink beside that box; this is the same for print.
//
// The only thing that bounds what a glyph paints is its outline. A font descriptor does not: in the corpus `/Descent`
// is POSITIVE in some thirty descriptors, one `/CapHeight` is 1170 and one `/FontBBox` runs 205..771. A constant
// does not either: 0.85 of a size holds every word of the corpus and no capital with an accent. So the ink is read
// from the embedded program's own points, and where there is no program this reads, a run has no ink and its box
// stays exactly as it was.
//
// What is read: a TrueType program (`/FontFile2`). Two answers come out of one, and they are not equally good:
//
//   - a font whose codes ARE its glyph numbers — a Type0 font under Identity-H whose CIDFontType2 maps CIDs to glyphs
//     by identity, which is how every current writer sets text outside Latin-1 — gives each run the ink of the glyphs
//     it shows. That may be deeper than the reach (an Arabic tail), and then the box grows to it.
//   - any other TrueType font gives the reach of EVERY glyph in its program, because which glyph a code draws is
//     decided by the program's own tables and a reader that maps them differently from the renderer has the wrong
//     glyph's ink. A subset holds little more than the glyphs the document shows, so this is nearly as tight. It is
//     only ever used to TIGHTEN: where the program reaches past the line's reach, the reach stands (`printInk`,
//     pagemap.go).
//
// Not read, and so left as they were: a font that is not embedded; Type 1 and CFF programs (`/FontFile`,
// `/FontFile3` — their outlines are charstring programs, a second reader); Type 3; a glyph that scales or turns a
// component by a full matrix; a run that is stroked (its ink is wider than its outline by half the line width), turned, or hidden.

// fontInkLimit is the largest decoded program read for its glyphs. Past it the font has no ink to give.
const fontInkLimit = 32 << 20

// fontInk is where a program's glyphs reach, in ems from the baseline: lo below (negative), hi above.
type fontInk struct {
	lo, hi float64
	// glyphs is each glyph's own [lo, hi], and inked whether it paints anything; byCode says a code is a glyph number.
	glyphs [][2]float64
	inked  []bool
	byCode bool
}

// inkSource is what loadRunFont keeps so the program is read only when a reader asks — the map — and once.
type inkSource struct {
	xt     *model.XRefTable
	prog   types.Object // the /FontFile2 stream
	byCode bool
	done   bool
	ink    *fontInk
}

// inkSourceFor finds the TrueType program font dictionary d draws from, or nil.
func inkSourceFor(xt *model.XRefTable, d types.Dict) *inkSource {
	sub := nameVal(d, "Subtype")
	byCode := false
	if sub == "Type0" {
		kids, err := xt.DereferenceArray(d["DescendantFonts"])
		if err != nil || len(kids) != 1 {
			return nil
		}
		kid, err := xt.DereferenceDict(kids[0])
		if err != nil || kid == nil || nameVal(kid, "Subtype") != "CIDFontType2" {
			return nil
		}
		// A code is a glyph number only under Identity-H with no map between CID and glyph.
		m, has := kid["CIDToGIDMap"]
		name, isName := m.(types.Name)
		byCode = nameVal(d, "Encoding") == "Identity-H" && (!has || (isName && name == "Identity"))
		d = kid
	} else if sub != "TrueType" {
		return nil
	}
	fd, err := xt.DereferenceDict(d["FontDescriptor"])
	if err != nil || fd == nil {
		return nil
	}
	prog, ok := fd["FontFile2"]
	if !ok {
		return nil
	}
	return &inkSource{xt: xt, prog: prog, byCode: byCode}
}

func (s *inkSource) read() *fontInk {
	if s == nil {
		return nil
	}
	if s.done {
		return s.ink
	}
	s.done = true
	sd, _, err := s.xt.DereferenceStreamDict(s.prog)
	if err != nil || sd == nil {
		return nil
	}
	c := *sd // decode a copy: the stream in the table stays as it was read
	if err := pdfread.DecodeWithin(&c, fontInkLimit); err != nil {
		return nil
	}
	if s.ink = readGlyfInk(c.Content); s.ink != nil {
		s.ink.byCode = s.byCode
	}
	return s.ink
}

// readGlyfInk reads every glyph's vertical extent from a TrueType program's own points — not from the box each glyph
// declares for itself, which a renderer does not read. Nil where the program cannot be read whole: a font is either
// vouched for or it is not, because one misread glyph is a letter left outside a redaction.
func readGlyfInk(b []byte) *fontInk {
	if len(b) < 12 {
		return nil
	}
	tables := map[string][]byte{}
	n := int(binary.BigEndian.Uint16(b[4:]))
	for i := 0; i < n; i++ {
		e := 12 + 16*i
		if e+16 > len(b) {
			return nil
		}
		off, length := int64(binary.BigEndian.Uint32(b[e+8:])), int64(binary.BigEndian.Uint32(b[e+12:]))
		if off+length > int64(len(b)) {
			continue // a table that runs off the end is one this does not have
		}
		tables[string(b[e:e+4])] = b[off : off+length]
	}
	head, maxp, loca, glyf := tables["head"], tables["maxp"], tables["loca"], tables["glyf"]
	if len(head) < 54 || len(maxp) < 6 || loca == nil || glyf == nil {
		return nil
	}
	upem := float64(binary.BigEndian.Uint16(head[18:]))
	if upem < 16 || upem > 16384 {
		return nil
	}
	long := binary.BigEndian.Uint16(head[50:]) != 0
	count := int(binary.BigEndian.Uint16(maxp[4:]))
	at := func(i int) int {
		if long {
			if 4*i+4 > len(loca) {
				return -1
			}
			return int(binary.BigEndian.Uint32(loca[4*i:]))
		}
		if 2*i+2 > len(loca) {
			return -1
		}
		return 2 * int(binary.BigEndian.Uint16(loca[2*i:]))
	}
	g := glyfReader{glyf: glyf, at: at, count: count, lo: make([]float64, count), hi: make([]float64, count), state: make([]byte, count)}
	out := &fontInk{lo: math.Inf(1), hi: math.Inf(-1), glyphs: make([][2]float64, count), inked: make([]bool, count)}
	for i := 0; i < count; i++ {
		if !g.read(i, 0) {
			return nil
		}
		if g.state[i] == glyphInked {
			out.glyphs[i], out.inked[i] = [2]float64{g.lo[i] / upem, g.hi[i] / upem}, true
			out.lo, out.hi = math.Min(out.lo, out.glyphs[i][0]), math.Max(out.hi, out.glyphs[i][1])
		}
	}
	if !(out.hi > out.lo) {
		return nil
	}
	return out
}

const (
	glyphUnread = iota
	glyphReading
	glyphEmpty
	glyphInked
)

type glyfReader struct {
	glyf   []byte
	at     func(int) int
	count  int
	lo, hi []float64 // font units
	state  []byte
}

// read fills glyph i's extent. False means the program is not one this reader vouches for.
func (g *glyfReader) read(i, depth int) bool {
	if i < 0 || i >= g.count || depth > 8 {
		return false
	}
	switch g.state[i] {
	case glyphReading:
		return false // a glyph made of itself
	case glyphEmpty, glyphInked:
		return true
	}
	g.state[i] = glyphReading
	a, z := g.at(i), g.at(i+1)
	if a < 0 || z < a || z > len(g.glyf) {
		return false
	}
	if z == a {
		g.state[i] = glyphEmpty
		return true
	}
	d := g.glyf[a:z]
	if len(d) < 10 {
		return false
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	if contours := int(int16(binary.BigEndian.Uint16(d))); contours >= 0 {
		p := 10
		points := 0
		for c := 0; c < contours; c++ {
			if p+2 > len(d) {
				return false
			}
			points = int(binary.BigEndian.Uint16(d[p:])) + 1
			p += 2
		}
		if p+2 > len(d) {
			return false
		}
		p += 2 + int(binary.BigEndian.Uint16(d[p:])) // the instructions
		flags := make([]byte, 0, points)
		for len(flags) < points {
			if p >= len(d) {
				return false
			}
			f := d[p]
			p++
			flags = append(flags, f)
			if f&0x08 != 0 { // repeated
				if p >= len(d) {
					return false
				}
				for k := int(d[p]); k > 0 && len(flags) < points; k-- {
					flags = append(flags, f)
				}
				p++
			}
		}
		for _, f := range flags { // the x coordinates: stepped over
			if f&0x02 != 0 {
				p++
			} else if f&0x10 == 0 {
				p += 2
			}
		}
		y := 0
		for _, f := range flags {
			switch {
			case f&0x04 != 0:
				if p >= len(d) {
					return false
				}
				if f&0x20 != 0 {
					y += int(d[p])
				} else {
					y -= int(d[p])
				}
				p++
			case f&0x20 == 0:
				if p+2 > len(d) {
					return false
				}
				y += int(int16(binary.BigEndian.Uint16(d[p:])))
				p += 2
			}
			lo, hi = math.Min(lo, float64(y)), math.Max(hi, float64(y))
		}
	} else {
		for p := 10; ; {
			if p+4 > len(d) {
				return false
			}
			f, kid := binary.BigEndian.Uint16(d[p:]), int(binary.BigEndian.Uint16(d[p+2:]))
			p += 4
			// A part placed by matching points, or turned or sheared (a two-by-two), is not placed by a move and a
			// scale up the page: not read.
			if f&0x0002 == 0 || f&0x0080 != 0 {
				return false
			}
			dy, sy := 0.0, 1.0
			if f&0x0001 != 0 {
				if p+4 > len(d) {
					return false
				}
				dy = float64(int16(binary.BigEndian.Uint16(d[p+2:])))
				p += 4
			} else {
				if p+2 > len(d) {
					return false
				}
				dy = float64(int8(d[p+1]))
				p += 2
			}
			switch {
			case f&0x0008 != 0: // one scale both ways
				if p+2 > len(d) {
					return false
				}
				sy = float64(int16(binary.BigEndian.Uint16(d[p:]))) / 16384
				p += 2
			case f&0x0040 != 0: // one across, one up
				if p+4 > len(d) {
					return false
				}
				sy = float64(int16(binary.BigEndian.Uint16(d[p+2:]))) / 16384
				p += 4
			}
			if f&0x0800 != 0 {
				dy *= sy // the move is in the part's own scaled space
			}
			if !g.read(kid, depth+1) {
				return false
			}
			if g.state[kid] == glyphInked {
				a, z := g.lo[kid]*sy+dy, g.hi[kid]*sy+dy
				lo, hi = math.Min(lo, math.Min(a, z)), math.Max(hi, math.Max(a, z))
			}
			if f&0x0020 == 0 {
				break
			}
		}
	}
	if hi > lo {
		g.lo[i], g.hi[i], g.state[i] = lo, hi, glyphInked
	} else {
		g.state[i] = glyphEmpty
	}
	return true
}
