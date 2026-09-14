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
// answering `CannotCheck` for every such file would make S07's identification unreachable for a
// question that is in fact decidable.
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
