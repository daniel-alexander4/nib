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
// # What "present in the font program" means — MEASURED, and read the way veraPDF reads it
//
// pdfcpu subsets a TrueType face by keeping every glyph SLOT and emptying the glyphs it did not use: on one authored
// page the program reported `maxp.numGlyphs` 3359 with only 8 non-empty `loca` intervals. A set of the 8 non-empty
// glyphs FAILS 7.21.4.2 t2 and a set of all 3359 slots PASSES, so the population is glyph SLOTS, not glyphs drawn.
//
// **Which slots is veraPDF's `CIDFontType2Program`, and it reads two counts, neither of them the first `maxp`**
// (measured at the P07 phase close, R1-2): the CIDs the set must list are `getCIDList` — under an identity map every
// CID below the `hhea` numberOfHMetrics (the `hmtx` advances), under a /CIDToGIDMap stream every CID the map holds whose
// glyph is below `maxp.numGlyphs` — and the CIDs it may list are `containsCID`, a glyph below `maxp.numGlyphs`, where a
// later duplicate `maxp` replaces an earlier one. CID 0 is skipped both ways. Both counts are `readTrueType`'s, the one
// reader of a TrueType program (ADR-009): a strict first-`maxp` reader stood beside it here until that review measured
// it passing a set veraPDF fails (`trueTypeCIDSetResult`).

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

// maxTrueTypeReads bounds the work NIB spends on a document's TrueType programs, all of them together — a `cmap` of
// 65,535 format 4 subtables of 32,767 segments each is 8.6e9 reads, and past this nib says it did not finish rather
// than guess. Reads count one each and a code-to-glyph map entry `mapEntryCost` (its memory: a 1 KB program of
// overlapping 64K-code segments pinned 142 MiB at one read per entry, the P07.S04a review measured).
const maxTrueTypeReads = 1 << 22

// mapEntryCost is what one map entry is charged, in reads.
const mapEntryCost = 8

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
	// What the per-glyph clauses read (P07.S04), valid when state is ttParsed.
	unitsPerEm int
	advances   []int
	numGlyphs  int
	hasCmap    bool
	subtables  []*ttSubtable
	hasPost    bool
	post       map[string]int
	byPair     map[[2]int]*ttSubtable // the first subtable of each (platform, encoding)
	mapping    []*ttSubtable          // the subtables holding any entry, in record order
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
// `BaseTrueTypeProgram.parseFont`. The font-dependent half (`createCIDToNameTable`) is the caller's. `budget` is what is
// left of the document's reads.
//
// It keeps what the per-glyph clauses ask of the program (P07.S04): the units per em (2048 with no `head`), the
// `hmtx` advances, the glyph count (`maxp`, else the advance count), every cmap subtable's code-to-glyph map in record
// order, and a format 2 or 2.5 `post` table's names.
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
	p := trueTypeProgram{pairs: map[[2]int]bool{}, unitsPerEm: 2048}
	if head.present {
		r.seek(head.off, "the head table")
		r.skip(18)
		p.unitsPerEm = r.u16()
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
		p.advances = append(p.advances, r.u16())
		r.skip(2)
	}
	if cmap.present {
		p.hasCmap = true
		p.nrCmaps, p.subtables = r.readCmap(cmap.off, p.pairs)
	}
	p.numGlyphs = nh
	if maxp.present {
		r.seek(maxp.off, "the maxp table")
		r.skip(4)
		p.numGlyphs = r.u16()
	}
	if post.present {
		p.hasPost = true
		p.post = r.readPost(post.off, post.length, p.numGlyphs)
	}
	p.state, p.why, p.reads = r.state, r.why, r.reads
	if p.state != ttParsed {
		p.advances, p.subtables, p.post = nil, nil, nil // nothing reads them, and a stopped walk may hold a great deal
	}
	for _, s := range p.subtables {
		if p.byPair == nil {
			p.byPair = map[[2]int]*ttSubtable{}
		}
		if _, seen := p.byPair[[2]int{s.platform, s.encoding}]; !seen {
			p.byPair[[2]int{s.platform, s.encoding}] = s // `getCmapTable` answers the FIRST matching record
		}
		if len(s.m) > 0 {
			p.mapping = append(p.mapping, s) // only a subtable that maps something can answer `getGID`
		}
	}
	return p
}

// ttSubtable is one cmap subtable as `TrueTypeCmapSubtable` holds it: a code-to-glyph map, and the FIRST code put into
// it, which a (3,0) subtable's lookup masks by (`sampleCode`) — insertion order a Go map alone would lose.
type ttSubtable struct {
	platform, encoding int
	m                  map[int]int
	first              int
}

func (t *ttSubtable) put(code, gid int) {
	if t.first < 0 {
		t.first = code
	}
	t.m[code] = gid
}

// put is a map entry read from the program, charged what an entry costs.
func (r *ttReader) put(t *ttSubtable, code, gid int) {
	if r.charge(mapEntryCost); r.ok() {
		t.put(code, gid)
	}
}

// charge counts n units of work against the budget — a map entry costs what a read does.
func (r *ttReader) charge(n int) {
	if !r.ok() {
		return
	}
	if r.reads += n; r.reads > r.limit {
		r.stop(ttUnknown, fmt.Sprintf("the document's TrueType programs cost more than %d reads, where nib stops", maxTrueTypeReads))
	}
}

// readCmap is `TrueTypeCmapTable.readTable`: every subtable record, then each subtable's body by its format —
// 0, 4 and 6 are read, 2 and every other format stop after the format field and map nothing.
func (r *ttReader) readCmap(tableOff int64, pairs map[[2]int]bool) (int, []*ttSubtable) {
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
	var subs []*ttSubtable
	for _, s := range recs {
		if !r.ok() {
			break
		}
		pairs[[2]int{s.platform, s.encoding}] = true
		t := &ttSubtable{platform: s.platform, encoding: s.encoding, m: map[int]int{}, first: -1}
		subs = append(subs, t)
		r.seek(s.off+tableOff, "a cmap subtable")
		switch r.u16() {
		case 0:
			r.skip(4)
			for i := 0; i < 256 && r.ok(); i++ {
				r.put(t, i, r.byte())
			}
		case 4:
			r.readSegmentMapping(t)
		case 6:
			r.skip(4)
			first := r.u16()
			count := r.u16()
			for i := 0; i < count && r.ok(); i++ {
				r.put(t, first+i, r.u16())
			}
		}
	}
	return n, subs
}

// readSegmentMapping is format 4, walked as veraPDF walks it: a segment with no range offset maps EVERY code from its
// start to its end (the 0xFFFF sentinel included) to `(idDelta + code) % 65536`; one with a range offset — unless it
// starts or ends at 0xFFFF — reads each code's glyph index, and a non-zero one takes `idDelta` too. Every entry is
// charged to the budget: overlapping segments are 2^31 entries in a few kilobytes.
func (r *ttReader) readSegmentMapping(t *ttSubtable) {
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
	starts, deltas, ranges := read(), read(), read()
	if !r.ok() {
		return
	}
	begin := r.pos
	for i := 0; i < seg && r.ok(); i++ {
		if ranges[i] == 0 {
			if ends[i] >= starts[i] {
				r.charge((ends[i] - starts[i] + 1) * mapEntryCost)
			}
			for j := starts[i]; j <= ends[i] && r.ok(); j++ {
				t.put(j, (deltas[i]+j)%65536)
			}
			continue
		}
		if starts[i] == 0xFFFF || ends[i] == 0xFFFF {
			continue
		}
		for j := 0; j <= ends[i]-starts[i] && r.ok(); j++ {
			r.seek(begin+int64(ranges[i]/2+j+(i-seg))*2, "a format 4 glyph index")
			g := r.u16()
			if g != 0 {
				g = (g + deltas[i]) % 65536
			}
			if r.ok() {
				r.put(t, j+starts[i], g)
			}
		}
	}
}

// readPost is `TrueTypePostTable.readTable`: formats 2 and 2.5 are read into a name-to-glyph map (a later glyph wins a
// duplicated name), every other format stops at its header and names nothing.
func (r *ttReader) readPost(off, length int64, numGlyphs int) map[string]int {
	r.seek(off, "the post table")
	var f [4]byte
	copy(f[:], r.block(4))
	format := binary.BigEndian.Uint32(f[:])
	r.skip(28)
	names := map[string]int{}
	switch format {
	case 0x00020000:
		numGlyphs = r.u16() // `setNumGlyphs` when it differs from maxp's: the post table's own count is the one read
		index := make([]int, 0, numGlyphs)
		for i := 0; i < numGlyphs && r.ok(); i++ {
			index = append(index, r.u16())
		}
		var strs []string
		for r.ok() && r.pos < off+length {
			strs = append(strs, latin1(r.block(int64(r.byte()))))
		}
		r.charge(len(index))
		for i, idx := range index {
			if !r.ok() {
				break
			}
			if idx < macGlyphNameCount {
				names[macGlyphNames[idx]] = i
			} else if k := idx - macGlyphNameCount; k < len(strs) {
				names[strs[k]] = i
			}
		}
	case 0x00028000:
		for i := 0; i < numGlyphs && r.ok(); i++ {
			if idx := int(int8(r.byte())) + i; r.ok() && (idx < 0 || idx >= macGlyphNameCount) {
				r.stop(ttFailed, "its format 2.5 post table indexes past the Macintosh glyph names")
			} else if r.ok() {
				names[macGlyphNames[idx]] = i
			}
		}
	}
	return names
}

// openTypeHasCFFTable is `OpenTypeFontProgram.getCFFTable`'s search, which is how veraPDF opens a /FontFile3 /OpenType
// program under a Type 1 font or a CIDFontType0 (`isCFF`): skip 4 bytes, read the table count, skip 6, then read that
// many 16-byte records and take the first tagged "CFF ". A read past the end is Java's -1 masked to 0xFF, so a record
// the program cuts short can never carry the tag, and a program with no such record throws — caught where the program
// is built, so veraPDF has no program and 7.21.4.1 t1 FAILS (measured on junk, a two-byte program, and a TrueType
// program under that subtype: the P07 phase close, R2-1).
func openTypeHasCFFTable(b []byte) bool {
	at := func(i int) byte {
		if i < len(b) {
			return b[i]
		}
		return 0xFF
	}
	n := int(at(4))<<8 | int(at(5))
	for i := 0; i < n && 12+16*i+4 <= len(b); i++ {
		if string(b[12+16*i:12+16*i+4]) == "CFF " {
			return true
		}
	}
	return false
}

// latin1 is `new String(bytes, ISO_8859_1)`.
func latin1(b []byte) string {
	rs := make([]rune, len(b))
	for i, c := range b {
		rs[i] = rune(c)
	}
	return string(rs)
}
