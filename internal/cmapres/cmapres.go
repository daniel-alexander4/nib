// Package cmapres carries the CMap resources the PDF/UA checker reads a Type 0 font with and a document does not
// hold itself: ISO 32000-1's predefined CMaps (a named `/Encoding` — its codespace and its codes' CIDs) and Adobe's
// `Adobe-<ordering>-UCS2` CMaps (a CID's Unicode, where the font has no `/ToUnicode` entry for it). ADR-117.
//
// # Where the bytes come from
//
// They are Adobe's cmap-resources in pdf.js's packed form — the `.bcmap` files the window already ships under
// `web/vendor/pdfjs/cmaps/`. `go:embed` cannot reach a parent directory and `web/` belongs to the root package, which
// this package cannot import, so the 63 files the checker needs are COPIED into `bcmap/` here, and
// `TestEveryCarriedCMapIsTheVendoredFile` holds each copy byte-identical to the vendored one: nothing is carried that
// the repository did not already carry, and a pdf.js update that moves the tables moves these with them or fails.
//
// # What it does with them
//
// It reads the packed form (`decode`) and writes it back out as a CMap PROGRAM (`Program`) — the PostScript text an
// embedded CMap stream holds. It interprets nothing: `internal/fontcode` reads that program with the same readers it
// reads a document's own CMaps with (ADR-052), so a carried CMap is cut, mapped and merged by exactly the rules an
// embedded one is.
//
// # What it does not carry
//
// `Adobe-KR-UCS2`: pdf.js ships no such table, so `Has` answers false for it and the checker says so.
// `Identity-H` and `Identity-V` are `fontcode`'s own, and need no table.
package cmapres

import (
	"bytes"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed bcmap/*.bcmap
var files embed.FS

// Names is every CMap carried, sorted.
func Names() []string {
	entries, _ := fs.ReadDir(files, "bcmap")
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".bcmap"))
	}
	return out
}

// Packed is a carried CMap's packed bytes, exactly as vendored; false for a name that is not carried (a name that
// is a path among them: an embedded file system opens no path that leaves its directory).
func Packed(name string) ([]byte, bool) {
	b, err := files.ReadFile("bcmap/" + name + ".bcmap")
	return b, err == nil
}

// Has is whether a CMap of that name is carried.
func Has(name string) bool {
	_, ok := Packed(name)
	return ok
}

// Program is a carried CMap written out as a CMap program: a `usecmap` first where it uses another, then its
// codespace ranges and its `cidchar`, `cidrange`, `notdefrange`, `bfchar` and `bfrange` lists — each record of the
// packed file in its order and of its own kind, in lists of at most a hundred, as Adobe's own files are written.
func Program(name string) ([]byte, error) {
	b, ok := Packed(name)
	if !ok {
		return nil, fmt.Errorf("cmapres: no CMap named %q is carried", name)
	}
	c, err := decode(b)
	if err != nil {
		return nil, fmt.Errorf("cmapres: %s: %w", name, err)
	}
	return write(name, c), nil
}

// The packed form's record kinds, by number.
const (
	kindCodespace = iota
	kindNotdefRange
	kindCIDChar
	kindCIDRange
	kindBFChar
	kindBFRange
	kindCount
)

// write is the program text. **The one function in the repository outside `internal/fontcode` that spells a CMap
// list's name** (`TestTheReadersRouteThroughThisDoor` names it): it WRITES the lists, for `fontcode` to read.
func write(name string, c *packed) []byte {
	lists := [kindCount]string{"codespacerange", "notdefrange", "cidchar", "cidrange", "bfchar", "bfrange"}
	var out bytes.Buffer
	out.WriteString("begincmap\n")
	if c.use != "" {
		fmt.Fprintf(&out, "/%s usecmap\n", c.use)
	}
	fmt.Fprintf(&out, "/CMapName /%s def\n/WMode %d def\n", name, c.wmode)
	for _, rec := range c.records {
		for entries := rec.entries; len(entries) > 0; {
			n := min(len(entries), 100)
			fmt.Fprintf(&out, "%d begin%s\n", n, lists[rec.kind])
			for _, e := range entries[:n] {
				fmt.Fprintf(&out, "<%s>", hex.EncodeToString(e.lo))
				if rec.kind != kindCIDChar && rec.kind != kindBFChar {
					fmt.Fprintf(&out, " <%s>", hex.EncodeToString(e.hi))
				}
				switch rec.kind {
				case kindBFChar, kindBFRange:
					fmt.Fprintf(&out, " <%s>", hex.EncodeToString(e.dst))
				case kindCIDChar, kindCIDRange, kindNotdefRange:
					fmt.Fprintf(&out, " %d", e.cid)
				}
				out.WriteByte('\n')
			}
			fmt.Fprintf(&out, "end%s\n", lists[rec.kind])
			entries = entries[n:]
		}
	}
	out.WriteString("endcmap\n")
	return out.Bytes()
}

// entry is one range of a packed CMap: a codespace range (lo, hi), a CID or notdef range (lo, hi, cid) or a
// Unicode range (lo, hi, dst — the first code's text, the last byte counting up).
type entry struct {
	lo, hi []byte
	cid    uint32
	dst    []byte
}

// record is one record of a packed CMap: entries of one kind.
type record struct {
	kind    int
	entries []entry
}

// packed is a decoded `.bcmap`.
type packed struct {
	wmode   int
	use     string
	records []record
}

// maxCode is the longest code or value a packed record may hold; pdf.js's reader has the same ceiling.
const maxCode = 16

// reader walks a packed CMap's bytes.
type reader struct {
	b   []byte
	pos int
	err error
}

var errShort = errors.New("the packed CMap ends inside a record")

func (r *reader) byte() byte {
	if r.pos >= len(r.b) {
		r.err = errShort
		return 0
	}
	r.pos++
	return r.b[r.pos-1]
}

// number is an unsigned integer, seven bits a byte, most significant first, the last byte's top bit clear.
func (r *reader) number() uint32 {
	var n uint32
	for r.err == nil {
		b := r.byte()
		n = n<<7 | uint32(b&0x7F)
		if b&0x80 == 0 {
			break
		}
	}
	return n
}

// signed is a number whose lowest bit is its sign.
func (r *reader) signed() int64 {
	n := r.number()
	if n&1 != 0 {
		return ^int64(n >> 1)
	}
	return int64(n >> 1)
}

// raw is `size` bytes as they stand.
func (r *reader) raw(size int) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = r.byte()
	}
	return out
}

// hexNumber is a `size`-byte big-endian number written seven bits a byte like `number`; bits past `size` bytes
// are dropped.
func (r *reader) hexNumber(size int) []byte {
	var groups []byte
	for r.err == nil {
		b := r.byte()
		groups = append(groups, b&0x7F)
		if b&0x80 == 0 {
			break
		}
	}
	out := make([]byte, size)
	var buf uint32
	bits := 0
	g := len(groups)
	for i := size - 1; i >= 0; i-- {
		for bits < 8 && g > 0 {
			g--
			buf |= uint32(groups[g]) << bits
			bits += 7
		}
		out[i] = byte(buf)
		buf >>= 8
		bits -= 8
	}
	return out
}

// hexSigned is a hexNumber whose lowest bit is its sign: the rest, shifted down one, and complemented when set.
func (r *reader) hexSigned(size int) []byte {
	out := r.hexNumber(size)
	var sign byte
	if out[size-1]&1 != 0 {
		sign = 0xFF
	}
	var carry byte
	for i := range out {
		v := out[i]
		out[i] = (carry<<7 | v>>1) ^ sign
		carry = v & 1
	}
	return out
}

func (r *reader) text() string {
	n := int(r.number())
	var s strings.Builder
	for i := 0; i < n && r.err == nil; i++ {
		s.WriteRune(rune(r.number()))
	}
	return s.String()
}

// add is a += b over equal-length big-endian numbers, the carry out dropped.
func add(a, b []byte) {
	var c uint
	for i := len(a) - 1; i >= 0; i-- {
		c += uint(a[i]) + uint(b[i])
		a[i] = byte(c)
		c >>= 8
	}
}

// inc is a++.
func inc(a []byte) {
	for i := len(a) - 1; i >= 0; i-- {
		a[i]++
		if a[i] != 0 {
			return
		}
	}
}

func clone(b []byte) []byte { return append([]byte(nil), b...) }

// decode reads pdf.js's packed CMap form, as its own reader does (`BinaryCMapReader`, read from the vendored
// build): a header byte whose low bit is the writing mode, then records. A record's first byte is its kind in the
// top three bits, a "sequential" flag, and its code size less one in the low four; then a count, then that many
// entries, each after the first written as its distance from the one before.
//
// Kind 7 is a comment or the name of the CMap this one uses. pdf.js skips notdef ranges (kind 1); they are read
// here, because the checker's code-to-CID mapping holds them.
func decode(b []byte) (*packed, error) {
	r := &reader{b: b}
	c := &packed{wmode: int(r.byte() & 1)}
	for r.pos < len(r.b) && r.err == nil {
		head := r.byte()
		kind := head >> 5
		if kind == 7 {
			switch head & 0x1F {
			case 0:
				r.text()
			case 1:
				c.use = r.text()
			}
			continue
		}
		sequential := head&0x10 != 0
		size := int(head&0x0F) + 1
		if size > maxCode {
			return nil, fmt.Errorf("a record of %d-byte codes", size)
		}
		count := int(r.number())
		if int(kind) >= kindCount {
			return nil, fmt.Errorf("a record of unknown kind %d", kind)
		}
		var list []entry
		switch int(kind) {
		case kindCodespace:
			lo := r.raw(size)
			hi := r.hexNumber(size)
			add(hi, lo)
			list = append(list, entry{lo: clone(lo), hi: clone(hi)})
			for i := 1; i < count && r.err == nil; i++ {
				inc(hi)
				lo = r.hexNumber(size)
				add(lo, hi)
				hi = r.hexNumber(size)
				add(hi, lo)
				list = append(list, entry{lo: clone(lo), hi: clone(hi)})
			}
		case kindNotdefRange:
			lo := r.raw(size)
			hi := r.hexNumber(size)
			add(hi, lo)
			list = append(list, entry{lo: clone(lo), hi: clone(hi), cid: r.number()})
			for i := 1; i < count && r.err == nil; i++ {
				inc(hi)
				lo = r.hexNumber(size)
				add(lo, hi)
				hi = r.hexNumber(size)
				add(hi, lo)
				list = append(list, entry{lo: clone(lo), hi: clone(hi), cid: r.number()})
			}
		case kindCIDChar:
			code := r.raw(size)
			cid := int64(r.number())
			list = append(list, entry{lo: clone(code), hi: clone(code), cid: uint32(cid)})
			for i := 1; i < count && r.err == nil; i++ {
				inc(code)
				if !sequential {
					add(code, r.hexNumber(size))
				}
				cid = r.signed() + cid + 1
				list = append(list, entry{lo: clone(code), hi: clone(code), cid: uint32(cid)})
			}
		case kindCIDRange:
			lo := r.raw(size)
			hi := r.hexNumber(size)
			add(hi, lo)
			list = append(list, entry{lo: clone(lo), hi: clone(hi), cid: r.number()})
			for i := 1; i < count && r.err == nil; i++ {
				inc(hi)
				if sequential {
					lo = clone(hi)
				} else {
					lo = r.hexNumber(size)
					add(lo, hi)
				}
				hi = r.hexNumber(size)
				add(hi, lo)
				list = append(list, entry{lo: clone(lo), hi: clone(hi), cid: r.number()})
			}
		case kindBFChar: // the code is two bytes, the text `size`
			code := r.raw(2)
			dst := r.raw(size)
			list = append(list, entry{lo: clone(code), hi: clone(code), dst: clone(dst)})
			for i := 1; i < count && r.err == nil; i++ {
				inc(code)
				if !sequential {
					add(code, r.hexNumber(2))
				}
				inc(dst)
				add(dst, r.hexSigned(size))
				list = append(list, entry{lo: clone(code), hi: clone(code), dst: clone(dst)})
			}
		case kindBFRange:
			lo := r.raw(2)
			hi := r.hexNumber(2)
			add(hi, lo)
			list = append(list, entry{lo: clone(lo), hi: clone(hi), dst: r.raw(size)})
			for i := 1; i < count && r.err == nil; i++ {
				inc(hi)
				if sequential {
					lo = clone(hi)
				} else {
					lo = r.hexNumber(2)
					add(lo, hi)
				}
				hi = r.hexNumber(2)
				add(hi, lo)
				list = append(list, entry{lo: clone(lo), hi: clone(hi), dst: r.raw(size)})
			}
		}
		c.records = append(c.records, record{kind: int(kind), entries: list})
	}
	if r.err != nil {
		return nil, r.err
	}
	return c, nil
}
