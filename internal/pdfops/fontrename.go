package pdfops

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
)

// Renaming a face so pdfcpu cannot mistake a stranger's font for nib's — `/pending 494`.
//
// # Why a face nib draws with must not be named like anyone else's
//
// pdfcpu matches a watermark's font to a font the document already carries BY NAME and by nothing
// else (`pdfcpu/stamp.go` `createFontResForWM`, the `fo.FontName == wm.FontName && fo.Prefix != ""`
// loop) and then rebuilds that font's program from nib's TTF (`font/fontDict.go` `UpdateUserfont`).
// A document an office suite produced carries `BAAAAA+LiberationSans`, which is the same name as the
// face nib stamps an edit in, so every such document collided. `stampTextWatermarks` refuses the
// collision and draws the stamp in Base-14 instead — correct, and PDF/UA 7.21.4.1 fails on the
// result, because a Base-14 face is not embedded.
//
// The name is the whole of pdfcpu's match, so a name no other producer writes removes the collision
// rather than handling it. pdfcpu names a face by the PostScript name INSIDE the TTF
// (`installTrueTypeRep` writes `<PostscriptName>.gob`, and `font.Read` fetches the program by that
// same file name), so the name has to be changed in the font bytes; an alias in the in-memory
// registry is not enough, because the program is read from the `.gob` the name picks.
//
// # It renames a copy, and the vendored bytes are untouched
//
// The rename is applied to the embedded bytes on the way into pdfcpu's font directory. Nothing
// rewrites `internal/pdfops/fonts/*.ttf`, so what nib SHIPS is upstream's font, byte for byte, and
// `THIRD-PARTY-NOTICES.md`'s description of it stays true of the artifact.
//
// # The licence makes this the compliant shape, not a risk
//
// Liberation is under the SIL Open Font License 1.1 **with Reserved Font Name "Liberation"**
// (`build/gen-notices.sh`). OFL §3 says no Modified Version may use a Reserved Font Name. pdfcpu
// already embeds a MODIFIED version of the face in every PDF it stamps — `font.Subset` prunes `glyf`
// and `loca` to the used glyphs and re-emits the file — and until this change it embedded that
// modified version still called `LiberationSans`. Renaming is what §3 asks for; the copyright,
// trademark and licence strings (name IDs 0, 7, 13 and 14) are left exactly as they are, which is
// what §2 asks for.
//
// # What is renamed, and what must not be
//
// Only the NAME-bearing records: family (1), unique id (3), full name (4), PostScript name (6) and
// the typographic/WWS/variations names (16, 17, 18, 20, 21, 22, 25). Record 0 is the copyright
// notice, 7 the trademark line — *"Liberation is a trademark of Red Hat, Inc."* — and 13/14 the
// licence and its URL. Rewriting any of those four would be a licence violation dressed as a fix.
const nibFaceToken = "Nib"

// upstreamFaceToken is the token replaced by `nibFaceToken` in the name records `renamedFace`
// rewrites. It is the Reserved Font Name itself, so one replacement covers `LiberationSans`,
// `Liberation Sans` and `Ascender - Liberation Sans` alike.
const upstreamFaceToken = "Liberation"

// renameableNameIDs are the name records that carry the face's NAME. Everything else — and in
// particular 0 (copyright), 7 (trademark), 13 (licence) and 14 (licence URL) — is carried through
// byte for byte.
var renameableNameIDs = map[uint16]bool{
	1: true, 3: true, 4: true, 6: true,
	16: true, 17: true, 18: true, 20: true, 21: true, 22: true, 25: true,
}

var errNotATrueTypeFile = errors.New("pdfops: not a TrueType file this can rename")

// nibFaceName is the name nib installs `face` under — `LiberationSans-Bold` → `NibSans-Bold`. A name
// that does not start with the upstream token comes back unchanged, so a face that is already nib's
// (or never was Liberation's) is its own answer.
func nibFaceName(face string) string {
	if !strings.HasPrefix(face, upstreamFaceToken) {
		return face
	}
	return nibFaceToken + strings.TrimPrefix(face, upstreamFaceToken)
}

// renamedFace returns ttf with its name records rewritten so pdfcpu installs and names it
// `nibFaceName(...)`. Every other table is carried through unchanged — the glyphs, the widths, the
// character map and the hinting are upstream's bytes, which is what makes the rename safe to
// measure text against.
func renamedFace(ttf []byte) ([]byte, error) {
	tables, order, err := sfntTables(ttf)
	if err != nil {
		return nil, err
	}
	nt, ok := tables["name"]
	if !ok {
		return nil, fmt.Errorf("%w: no name table", errNotATrueTypeFile)
	}
	renamed, err := renamedNameTable(nt)
	if err != nil {
		return nil, err
	}
	tables["name"] = renamed
	return sfntBytes(ttf[:12], tables, order)
}

// sfntTables splits a TrueType file into its tables, keyed by tag, and reports the tags in the order
// the directory listed them. It is deliberately as strict as pdfcpu's own reader
// (`font/install.go` `headerAndTables`, `ttfTables`) — a file one refuses the other refuses.
func sfntTables(b []byte) (map[string][]byte, []string, error) {
	if len(b) < 12 {
		return nil, nil, errNotATrueTypeFile
	}
	switch string(b[:4]) {
	case "\x00\x01\x00\x00", "true":
	default:
		return nil, nil, fmt.Errorf("%w: %q is not a TrueType sfnt version", errNotATrueTypeFile, b[:4])
	}
	n := int(binary.BigEndian.Uint16(b[4:]))
	if 12+n*16 > len(b) {
		return nil, nil, fmt.Errorf("%w: %d tables do not fit", errNotATrueTypeFile, n)
	}
	out := map[string][]byte{}
	order := make([]string, 0, n)
	for i := 0; i < n; i++ {
		rec := b[12+i*16 : 12+i*16+16]
		tag := string(rec[:4])
		off := binary.BigEndian.Uint32(rec[8:])
		l := binary.BigEndian.Uint32(rec[12:])
		if int(off)+int(l) > len(b) {
			return nil, nil, fmt.Errorf("%w: table %q runs past the end", errNotATrueTypeFile, tag)
		}
		out[tag] = b[off : off+l]
		order = append(order, tag)
	}
	return out, order, nil
}

// sfntBytes reassembles a TrueType file from its tables, laying them out exactly as pdfcpu's
// `createTTF` does — directory sorted by tag, each table 4-byte aligned — so a file this writes and
// a file pdfcpu writes differ only in what was asked for. Each table's checksum is recomputed, and
// `head`'s `checkSumAdjustment` last, over the finished file.
func sfntBytes(header []byte, tables map[string][]byte, _ []string) ([]byte, error) {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	var body bytes.Buffer
	offs := map[string]uint32{}
	off := uint32(12 + len(tags)*16)
	for _, tag := range tags {
		offs[tag] = off + uint32(body.Len())
		body.Write(tables[tag])
		for body.Len()%4 != 0 {
			body.WriteByte(0)
		}
	}

	var out bytes.Buffer
	out.Write(header[:12])
	for _, tag := range tags {
		out.WriteString(tag)
		_ = binary.Write(&out, binary.BigEndian, sfntChecksum(tables[tag]))
		_ = binary.Write(&out, binary.BigEndian, offs[tag])
		_ = binary.Write(&out, binary.BigEndian, uint32(len(tables[tag])))
	}
	out.Write(body.Bytes())

	b := out.Bytes()
	if head, ok := offs["head"]; ok && int(head)+12 <= len(b) {
		// Zero the field, checksum the file, then write 0xB1B0AFBA minus that sum (OpenType, head).
		binary.BigEndian.PutUint32(b[head+8:], 0)
		binary.BigEndian.PutUint32(b[head+8:], 0xB1B0AFBA-sfntChecksum(b))
	}
	return b, nil
}

// sfntChecksum is the OpenType table checksum: the sum of the data as big-endian uint32s, zero
// padded to a multiple of four.
func sfntChecksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		sum += binary.BigEndian.Uint32(w[:])
	}
	return sum
}

// renamedNameTable rewrites a format-0 naming table's name-bearing records, leaving the copyright,
// trademark and licence records alone. The storage area is rebuilt rather than patched in place,
// because a replacement is a different length and every later record's offset would move.
func renamedNameTable(t []byte) ([]byte, error) {
	if len(t) < 6 {
		return nil, fmt.Errorf("%w: name table is %d bytes", errNotATrueTypeFile, len(t))
	}
	if f := binary.BigEndian.Uint16(t); f != 0 {
		// Format 1 adds language-tag records after the name records; nothing nib vendors uses it,
		// and guessing at a layout that is not there is how a font comes out silently wrong.
		return nil, fmt.Errorf("%w: name table format %d, want 0", errNotATrueTypeFile, f)
	}
	count := int(binary.BigEndian.Uint16(t[2:]))
	storage := int(binary.BigEndian.Uint16(t[4:]))
	if 6+count*12 > len(t) || storage > len(t) {
		return nil, fmt.Errorf("%w: name table's %d records do not fit", errNotATrueTypeFile, count)
	}

	type record struct {
		platform, encoding, language, id uint16
		value                            []byte
	}
	recs := make([]record, 0, count)
	for i := 0; i < count; i++ {
		r := t[6+i*12 : 6+i*12+12]
		length := int(binary.BigEndian.Uint16(r[8:]))
		offset := int(binary.BigEndian.Uint16(r[10:]))
		if storage+offset+length > len(t) {
			return nil, fmt.Errorf("%w: name record %d runs past the storage area", errNotATrueTypeFile, i)
		}
		rec := record{
			platform: binary.BigEndian.Uint16(r),
			encoding: binary.BigEndian.Uint16(r[2:]),
			language: binary.BigEndian.Uint16(r[4:]),
			id:       binary.BigEndian.Uint16(r[6:]),
			value:    t[storage+offset : storage+offset+length],
		}
		if renameableNameIDs[rec.id] {
			renamedValue, err := renameEncoded(rec.platform, rec.value)
			if err != nil {
				return nil, err
			}
			rec.value = renamedValue
		}
		recs = append(recs, rec)
	}

	var head, store bytes.Buffer
	_ = binary.Write(&head, binary.BigEndian, uint16(0))
	_ = binary.Write(&head, binary.BigEndian, uint16(len(recs)))
	_ = binary.Write(&head, binary.BigEndian, uint16(6+len(recs)*12))
	for _, r := range recs {
		if store.Len() > 0xFFFF || len(r.value) > 0xFFFF {
			// Offsets and lengths are uint16 in this table. The vendored faces are nowhere near it,
			// and a silent wrap would produce a font whose names point at other names' bytes.
			return nil, fmt.Errorf("%w: renamed name table would overflow its 16-bit offsets", errNotATrueTypeFile)
		}
		_ = binary.Write(&head, binary.BigEndian, r.platform)
		_ = binary.Write(&head, binary.BigEndian, r.encoding)
		_ = binary.Write(&head, binary.BigEndian, r.language)
		_ = binary.Write(&head, binary.BigEndian, r.id)
		_ = binary.Write(&head, binary.BigEndian, uint16(len(r.value)))
		_ = binary.Write(&head, binary.BigEndian, uint16(store.Len()))
		store.Write(r.value)
	}
	return append(head.Bytes(), store.Bytes()...), nil
}

// renameEncoded replaces the reserved token in one name record, in that record's own encoding —
// UTF-16BE on the Windows platform (3) and one byte per character on Macintosh (1).
func renameEncoded(platform uint16, value []byte) ([]byte, error) {
	if platform == 3 {
		if len(value)%2 != 0 {
			return nil, fmt.Errorf("%w: a UTF-16BE name record of %d bytes", errNotATrueTypeFile, len(value))
		}
		units := make([]uint16, len(value)/2)
		for i := range units {
			units[i] = binary.BigEndian.Uint16(value[2*i:])
		}
		s := strings.ReplaceAll(string(utf16.Decode(units)), upstreamFaceToken, nibFaceToken)
		out := make([]byte, 0, len(s)*2)
		for _, u := range utf16.Encode([]rune(s)) {
			out = binary.BigEndian.AppendUint16(out, u)
		}
		return out, nil
	}
	return []byte(strings.ReplaceAll(string(value), upstreamFaceToken, nibFaceToken)), nil
}
