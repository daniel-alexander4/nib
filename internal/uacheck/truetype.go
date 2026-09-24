package uacheck

import (
	"encoding/binary"
	"fmt"
)

// Reading an embedded TrueType program — `PLAN-accessibility.md` P07.S04.
//
// # Why the checker parses font tables at all
//
// ua1 7.21.4.2 t2 is conditional: *"If the FontDescriptor … contains a CIDSet stream, then it shall
// identify all CIDs which are present in the font program."* nib's own output carries no `/CIDSet` —
// P04.S02 removes the one pdfcpu writes — so for nib's documents the clause has no subject. A user's
// document from another producer, or a PDF/A-1 file that REQUIRES the stream, can carry one, and
// answering `CannotCheck` for every such file would leave the report unable to settle a question that
// is in fact decidable.
//
// # What "present in the font program" means, MEASURED rather than read
//
// pdfcpu subsets a TrueType face by keeping every glyph SLOT and emptying the glyphs it did not use:
// on one authored page the program reported `maxp.numGlyphs` 3359 with only 8 non-empty `loca`
// intervals. The clause could mean either population, so both were written into the `/CIDSet` and
// handed to veraPDF: a set of the 8 non-empty glyphs FAILS 7.21.4.2 t2, and a set of all 3359 slots
// PASSES. **So the population is `numGlyphs`**, and the rule needs only `maxp` — not `glyf`/`loca`,
// which P04.S02's comment named as the cost of computing a correct set.
//
// pdfcpu's own TrueType reader (`pkg/font/install.go`) is unexported, so this is the minimum a
// checker needs: the table directory and one field of `maxp`.
//
// **It is not `readTrueType` below, deliberately, and the two read one directory by different rules**: this one is
// strict (it refuses `OTTO` and `ttcf`, and takes the FIRST `maxp`) because 7.21.4.2 t2 reads a CIDFontType2 program,
// which veraPDF opens through `CIDFontType2Program`, not `TrueTypeFontParser`; `readTrueType` is the latter, line for
// line. P07.S04 builds the CIDFontType2 reader, and that is where the two meet or are declared apart.

// trueTypeGlyphCount returns `maxp.numGlyphs` from a TrueType font program.
//
// It refuses anything it cannot vouch for rather than guessing: a collection (`ttcf`), a CFF-flavoured
// OpenType (`OTTO`), a truncated directory, or a `maxp` too short to hold the field. Each refusal
// becomes `CannotCheck` in the rule, because nib failing to read a program is not the program being
// wrong.
func trueTypeGlyphCount(prog []byte) (int, error) {
	if len(prog) < 12 {
		return 0, fmt.Errorf("the font program is %d bytes, too short for a table directory", len(prog))
	}
	switch tag := string(prog[0:4]); tag {
	case "\x00\x01\x00\x00", "true":
	case "OTTO":
		return 0, fmt.Errorf("the font program is CFF-flavoured OpenType, whose glyph set nib does not read")
	case "ttcf":
		return 0, fmt.Errorf("the font program is a TrueType collection, which nib does not read")
	default:
		return 0, fmt.Errorf("the font program has an unrecognised signature %q", tag)
	}
	tables := int(binary.BigEndian.Uint16(prog[4:6]))
	for i := 0; i < tables; i++ {
		rec := 12 + i*16
		if rec+16 > len(prog) {
			return 0, fmt.Errorf("the table directory is truncated at record %d of %d", i, tables)
		}
		if string(prog[rec:rec+4]) != "maxp" {
			continue
		}
		off := int(binary.BigEndian.Uint32(prog[rec+8:]))
		length := int(binary.BigEndian.Uint32(prog[rec+12:]))
		if off < 0 || length < 6 || off+length > len(prog) {
			return 0, fmt.Errorf("the maxp table lies outside the font program or is too short")
		}
		return int(binary.BigEndian.Uint16(prog[off+4 : off+6])), nil
	}
	return 0, fmt.Errorf("the font program has no maxp table")
}

// cidSetExact reports whether set identifies exactly the CIDs in [0, n): every one of them present,
// and no bit set at or beyond n. It returns the first CID missing, and the first CID claimed that the
// program does not hold, or -1 for either.
//
// **Both directions are the clause, measured.** A set covering every glyph slot and nothing more
// PASSES veraPDF; the same set with the unused bits of its last byte turned on FAILS, and so does one
// with an extra byte of set bits. A set claiming glyphs the program does not have misidentifies the
// CIDs exactly as one omitting glyphs does — and the first version of this function checked only
// coverage, which the law 5 check at S04's close caught as a disagreement.
//
// Bit order is PDF's: the high-order bit of the first byte is CID 0 (ISO 32000-1 §9.8.3). Zero bytes
// past the end are not a claim and are not refused.
func cidSetExact(set []byte, n int) (ok bool, missing, extra int) {
	missing, extra = -1, -1
	for cid := 0; cid < len(set)*8; cid++ {
		on := set[cid/8]&(0x80>>uint(cid%8)) != 0
		switch {
		case cid < n && !on && missing < 0:
			missing = cid
		case cid >= n && on && extra < 0:
			extra = cid
		}
	}
	if len(set)*8 < n && missing < 0 {
		missing = len(set) * 8
	}
	return missing < 0 && extra < 0, missing, extra
}

// # Opening a program the way veraPDF opens it — `PLAN-ua-coverage.md` P07.S03
//
// ua1 7.21.6 t1 and t4 have a subject only when veraPDF PARSED the program, and t2 reads the parse too, so the
// question for those clauses is not "is this a well-formed TrueType program" but "does veraPDF-parser 1.30.2's
// `TrueTypeFontParser` get through it". `readTrueType` is that parser ported line for line over a cursor with the
// semantics of veraPDF's streams, which is where every surprise was:
//
//   - a read is bounded by the PROGRAM's end, never by the table's declared length — a format 0 subtable cut short
//     and followed by other tables reads those tables' bytes and parses (measured);
//   - `skip` clamps at the end and never fails, and a block read (`readFixed`, a Pascal string) that comes up short
//     does not fail either; a single-byte read at the end does;
//   - a seek past the end fails — and for a program of 10240 bytes or more (`SeekableInputStream.MAX_BUFFER_SIZE`)
//     veraPDF reads it through a temporary file whose seek throws an `IllegalArgumentException` that nothing above
//     catches, so veraPDF reports NOTHING for the document (measured). nib cannot agree with an absent answer, so
//     that case is `ttUnknown`;
//   - `head`, `cmap`, `maxp` and `post` may be missing; `hhea` and `hmtx` may not; checksums, the version tag and
//     alignment are never read, and a later duplicate directory entry replaces an earlier one.

// veraPDFMemoryLimit is the program length from which veraPDF reads through a temporary file instead of memory.
const veraPDFMemoryLimit = 10240

// maxTrueTypeReads bounds the reads NIB spends on a document's TrueType programs, all of them together — a `cmap` of
// 65,535 format 4 subtables of 32,767 segments each is 8.6e9 reads, and past this nib says it did not finish rather
// than guess. It counts nib's reads, not veraPDF's work: format 4's glyph-index loop is checked at its extremes and
// never walked, so a program under this budget can still cost veraPDF hours to reach the same answer.
const maxTrueTypeReads = 1 << 22

// macGlyphNameCount is `TrueTypePredefined.MAC_INDEX_TO_GLYPH_NAME.length`, which a format 2.5 `post` indexes.
const macGlyphNameCount = 258

type ttState int

const (
	ttParsed  ttState = iota // veraPDF parses it
	ttFailed                 // veraPDF's parse throws: no program for 7.21.6 t1/t4, not embedded for 7.21.4.1 t1
	ttUnknown                // nib cannot say what veraPDF does
)

// trueTypeProgram is what the 7.21.6 clauses read of a program veraPDF parsed.
type trueTypeProgram struct {
	state   ttState
	why     string          // for ttFailed and ttUnknown
	nrCmaps int             // the cmap table's subtable count, duplicates included; 0 with no cmap table
	pairs   map[[2]int]bool // the (platform, encoding) pairs present
	reads   int             // what reading it cost, charged to the document's budget
}

func (p trueTypeProgram) has(platform, encoding int) bool { return p.pairs[[2]int{platform, encoding}] }

// ttReader is a cursor with veraPDF's stream semantics; the first problem stops it.
type ttReader struct {
	b     []byte
	pos   int64
	reads int
	limit int
	state ttState
	why   string
}

func (r *ttReader) ok() bool { return r.state == ttParsed }

func (r *ttReader) stop(s ttState, why string) {
	if r.ok() {
		r.state, r.why = s, why
	}
}

func (r *ttReader) seek(off int64, what string) {
	if !r.ok() {
		return
	}
	switch {
	case off < 0:
		r.stop(ttFailed, what+" lies before the program's start")
	case off > int64(len(r.b)) && len(r.b) >= veraPDFMemoryLimit:
		r.stop(ttUnknown, what+" lies past the end of a program of "+fmt.Sprint(len(r.b))+" bytes, where veraPDF's "+
			"reader throws an exception it does not handle and reports nothing for the document")
	case off > int64(len(r.b)):
		r.stop(ttFailed, what+" lies past the program's end")
	default:
		r.pos = off
	}
}

func (r *ttReader) skip(n int64) {
	if r.ok() {
		r.pos = min(r.pos+n, int64(len(r.b)))
	}
}

func (r *ttReader) byte() int {
	if !r.ok() {
		return 0
	}
	if r.reads++; r.reads > r.limit {
		r.stop(ttUnknown, fmt.Sprintf("the document's TrueType programs cost more than %d reads, where nib stops", maxTrueTypeReads))
		return 0
	}
	if r.pos >= int64(len(r.b)) {
		r.stop(ttFailed, "a read runs past the program's end")
		return 0
	}
	r.pos++
	return int(r.b[r.pos-1])
}

func (r *ttReader) u16() int   { return r.byte()<<8 | r.byte() }
func (r *ttReader) u32() int64 { return int64(r.u16())<<16 | int64(r.u16()) }

// block is Java's `read(buf, n)`: as many bytes as remain, never a failure.
func (r *ttReader) block(n int64) []byte {
	if !r.ok() {
		return nil
	}
	end := min(r.pos+n, int64(len(r.b)))
	out := r.b[r.pos:end]
	r.pos = end
	return out
}

// readTrueType is `TrueTypeFontParser.readHeader`, `readTableDirectory` and `readTables` — the tables half of
// `BaseTrueTypeProgram.parseFont`. The font-dependent half (`createCIDToNameTable`) is the caller's.
func readTrueType(prog []byte, budget int) trueTypeProgram {
	r := &ttReader{b: prog, limit: budget}
	type table struct {
		off, length int64
		present     bool
	}
	var head, hhea, hmtx, cmap, maxp, post table
	r.skip(4)
	n := r.u16()
	r.skip(6)
	for i := 0; i < n && r.ok(); i++ {
		tag, _, off, length := r.u32(), r.u32(), r.u32(), r.u32()
		t := table{off, length, true}
		switch string([]byte{byte(tag >> 24), byte(tag >> 16), byte(tag >> 8), byte(tag)}) {
		case "head":
			head = t
		case "hhea":
			hhea = t
		case "hmtx":
			hmtx = t
		case "cmap":
			cmap = t
		case "maxp":
			maxp = t
		case "post":
			post = t
		}
	}
	if head.present {
		r.seek(head.off, "the head table")
		r.skip(18)
		r.u16()
	}
	if !hhea.present {
		r.stop(ttFailed, "it has no hhea table")
	}
	r.seek(hhea.off, "the hhea table")
	r.skip(4)
	r.u16()
	r.u16()
	r.skip(26)
	nh := r.u16()
	if !hmtx.present {
		r.stop(ttFailed, "it has no hmtx table")
	}
	r.seek(hmtx.off, "the hmtx table")
	for i := 0; i < nh && r.ok(); i++ {
		r.u16()
		r.skip(2)
	}
	p := trueTypeProgram{pairs: map[[2]int]bool{}}
	if cmap.present {
		p.nrCmaps = r.readCmap(cmap.off, p.pairs)
	}
	numGlyphs := nh
	if maxp.present {
		r.seek(maxp.off, "the maxp table")
		r.skip(4)
		numGlyphs = r.u16()
	}
	if post.present {
		r.readPost(post.off, post.length, numGlyphs)
	}
	p.state, p.why, p.reads = r.state, r.why, r.reads
	return p
}

// readCmap is `TrueTypeCmapTable.readTable`: every subtable record, then each subtable's body by its format —
// 0, 4 and 6 are read, 2 and every other format stop after the format field.
func (r *ttReader) readCmap(tableOff int64, pairs map[[2]int]bool) int {
	r.seek(tableOff, "the cmap table")
	r.skip(2)
	n := r.u16()
	type rec struct {
		platform, encoding int
		off                int64
	}
	recs := make([]rec, 0, n)
	for i := 0; i < n && r.ok(); i++ {
		recs = append(recs, rec{r.u16(), r.u16(), r.u32()})
	}
	for _, s := range recs {
		if !r.ok() {
			break
		}
		pairs[[2]int{s.platform, s.encoding}] = true
		r.seek(s.off+tableOff, "a cmap subtable")
		switch r.u16() {
		case 0:
			r.skip(4)
			for i := 0; i < 256 && r.ok(); i++ {
				r.byte()
			}
		case 4:
			r.readSegmentMapping()
		case 6:
			r.skip(4)
			r.u16()
			count := r.u16()
			for i := 0; i < count && r.ok(); i++ {
				r.u16()
			}
		}
	}
	return n
}

// readSegmentMapping is format 4. Its glyph-index loop seeks once per code of every segment with a range offset;
// the offsets rise by 2 with each code, so only the first code that fails is asked for, never walked to.
func (r *ttReader) readSegmentMapping() {
	r.skip(4)
	seg := r.u16() / 2
	r.skip(6)
	read := func() []int {
		v := make([]int, 0, seg)
		for i := 0; i < seg && r.ok(); i++ {
			v = append(v, r.u16())
		}
		return v
	}
	ends := read()
	r.skip(2)
	starts, _, ranges := read(), read(), read()
	if !r.ok() {
		return
	}
	begin, size := r.pos, int64(len(r.b))
	for i := 0; i < seg; i++ {
		if ranges[i] == 0 || starts[i] == 0xFFFF || ends[i] == 0xFFFF || ends[i] < starts[i] {
			continue
		}
		first := begin + int64(ranges[i]/2+(i-seg))*2
		last := first + int64(ends[i]-starts[i])*2
		if first >= 0 && last+2 <= size {
			continue
		}
		// The first code whose read does not fit: a seek before the start, past the end, or a read over it.
		at := first
		if first >= 0 && first+2 <= size {
			at = first + ((size-first-2)/2+1)*2
		}
		if at < 0 || at > size {
			r.seek(at, "a format 4 glyph index")
		}
		r.stop(ttFailed, "a format 4 glyph index runs past the program's end")
		return
	}
}

// readPost is `TrueTypePostTable.readTable`: formats 2 and 2.5 are read, every other format stops at its header.
func (r *ttReader) readPost(off, length int64, numGlyphs int) {
	r.seek(off, "the post table")
	var f [4]byte
	copy(f[:], r.block(4))
	format := binary.BigEndian.Uint32(f[:])
	r.skip(28)
	switch format {
	case 0x00020000:
		numGlyphs = r.u16() // `setNumGlyphs` when it differs from maxp's: the post table's own count is the one read
		for i := 0; i < numGlyphs && r.ok(); i++ {
			r.u16()
		}
		for r.ok() && r.pos < off+length {
			r.block(int64(r.byte()))
		}
	case 0x00028000:
		for i := 0; i < numGlyphs && r.ok(); i++ {
			if idx := int(int8(r.byte())) + i; r.ok() && (idx < 0 || idx >= macGlyphNameCount) {
				r.stop(ttFailed, "its format 2.5 post table indexes past the Macintosh glyph names")
			}
		}
	}
}
