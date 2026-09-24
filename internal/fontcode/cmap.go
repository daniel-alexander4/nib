package fontcode

import (
	"strconv"
	"unicode/utf16"

	"nib/internal/contentstream"
)

// # Two readings of a /ToUnicode CMap, and why both live here
//
// `ToUnicode` is veraPDF's reading, and the checker must use it: PDF/UA-1 7.21.7 is measured against
// veraPDF, and veraPDF-parser reads a CMap in ways a text extractor should not copy — it maps exactly the
// COUNT of entries a `beginbfchar` announces, cuts a `bfrange` at the last byte of its begin code, and
// discards the whole CMap on one malformed entry (`CMapFactory.getCMap` returns an empty CMap on any
// `IOException`). `TextMap` is the reading `internal/pdfops` extracts text with, which follows the
// specification leniently so a sloppy producer's text still reads. They share the tokenizer and every
// decoder; what differs is written down at each site, which is ADR-009's rule for a deliberate split.

// cmapTokens are a CMap program's tokens, whitespace and comments dropped.
type cmapTokens struct {
	src  []byte
	toks []contentstream.Token
	i    int
}

func newCMapTokens(src []byte) *cmapTokens {
	t := &cmapTokens{src: src}
	for _, tk := range contentstream.Tokenize(src) {
		if tk.Kind != contentstream.Whitespace {
			t.toks = append(t.toks, tk)
		}
	}
	return t
}

// next returns the next token, or ok false at the end.
func (t *cmapTokens) next() (contentstream.Token, bool) {
	if t.i >= len(t.toks) {
		return contentstream.Token{}, false
	}
	tk := t.toks[t.i]
	t.i++
	return tk, true
}

func (t *cmapTokens) text(tk contentstream.Token) string { return string(tk.Bytes(t.src)) }

// hex reads the next token as a hex string, or reports that it was something else — which is where
// veraPDF-parser's `checkTokenType` throws.
func (t *cmapTokens) hex() ([]byte, bool) {
	tk, ok := t.next()
	if !ok || tk.Kind != contentstream.HexString {
		return nil, false
	}
	return Hex(tk.Bytes(t.src)), true
}

// integer reads the next token as an integer.
func (t *cmapTokens) integer() bool {
	tk, ok := t.next()
	if !ok || tk.Kind != contentstream.Operand {
		return false
	}
	_, err := strconv.Atoi(t.text(tk))
	return err == nil
}

// maxCMapEntries bounds the entries one CMap may announce in total. The count is attacker-written and
// each entry is read from real tokens, so a count past the tokens simply runs out — but it is also the
// loop bound, and a stated count should never be one.
const maxCMapEntries = 1 << 20

// lists walks a CMap program the way veraPDF-parser's `CMapParser.processObject` does: an INTEGER
// immediately followed by a `begin<key>` keyword opens a list of exactly that many entries, and
// `each(key)` reads one. Anything else is PostScript veraPDF executes and nib does not need. A
// `/Name usecmap` pair is reported through `use`.
//
// It returns false where veraPDF would throw — an entry of the wrong kind — which empties the CMap.
func (t *cmapTokens) lists(each func(key string) bool, use func(name string)) bool {
	lastName := ""
	budget := maxCMapEntries
	for {
		tk, ok := t.next()
		if !ok {
			return true
		}
		switch tk.Kind {
		case contentstream.Operand:
			b := tk.Bytes(t.src)
			if len(b) > 0 && b[0] == '/' {
				lastName = Name(b[1:])
				continue
			}
			v, err := strconv.ParseInt(string(b), 10, 64)
			if err != nil {
				continue
			}
			// **An integer CONSUMES the object after it** (`CMapParser.processObject`): it is a list only when that
			// object is a `begin…` keyword `processList` accepts, and otherwise both are executed as PostScript — so in
			// `9 1 beginbfchar` the `1` is spent and the keyword opens nothing (the P07.S02 re-review).
			kw, ok := t.next()
			if !ok {
				return true
			}
			key := t.text(kw)
			// A name consumed here is executed, not remembered: veraPDF sets `lastCOSName` only in its own name branch.
			if kw.Kind != contentstream.Operator || len(key) < 5 || key[:5] != "begin" {
				continue
			}
			// The count is a Java int. `processList` loops it: zero or negative reads nothing and ACCEPTS whatever the
			// key; a positive count of a key it does not know fails on the first entry and is executed instead.
			n := javaInt(v)
			if n > 0 && !cmapListKey[key[5:]] {
				continue
			}
			if n > budget {
				return false
			}
			if n > 0 {
				budget -= n
			}
			for k := 0; k < n; k++ {
				if !each(key[5:]) {
					return false
				}
			}
			// veraPDF reads one token after the list and only LOGS when it is not the matching `end`.
			t.next()
		case contentstream.Operator:
			if t.text(tk) == "usecmap" && lastName != "" && use != nil {
				use(lastName)
			}
		}
	}
}

// cmapListKey is the set `CMapParser.processList` recognises; any other `begin…` returns false there and
// the pair is executed as ordinary PostScript.
var cmapListKey = map[string]bool{
	"codespacerange": true, "cidrange": true, "notdefrange": true, "cidchar": true,
	"notdefchar": true, "bfchar": true, "bfrange": true,
}

// skipEntry consumes one entry of a list the reader does not keep, with veraPDF's type checks.
func (t *cmapTokens) skipEntry(key string) bool {
	switch key {
	case "codespacerange":
		_, a := t.hex()
		_, b := t.hex()
		return a && b
	case "cidrange", "notdefrange":
		_, a := t.hex()
		_, b := t.hex()
		return a && b && t.integer()
	case "cidchar", "notdefchar":
		_, a := t.hex()
		return a && t.integer()
	case "bfchar":
		_, a := t.hex()
		return a && t.unicodeToken()
	case "bfrange":
		return t.skipBFRange()
	}
	return false
}

func (t *cmapTokens) unicodeToken() bool {
	_, ok := t.unicodeValue()
	return ok
}

func (t *cmapTokens) skipBFRange() bool {
	lo, a := t.hex()
	hi, b := t.hex()
	if !a || !b {
		return false
	}
	return t.bfRangeTail(lo, hi, nil, nil)
}

// unicodeValue reads a `bfchar` destination as `CMapParser.readStringFromUnicodeSequenceToken` does: a
// name is its own text, a one-byte string is ISO-8859-1, `<FFFE>` is U+FFFE, and anything else is decoded
// as UTF-16BE. Anything that is neither a name nor a hex string throws.
func (t *cmapTokens) unicodeValue() (string, bool) {
	tk, ok := t.next()
	if !ok {
		return "", false
	}
	b := tk.Bytes(t.src)
	switch {
	case tk.Kind == contentstream.Operand && len(b) > 0 && b[0] == '/':
		return Name(b[1:]), true
	case tk.Kind == contentstream.HexString:
		return javaUnicodeString(Hex(b)), true
	}
	return "", false
}

// javaUnicodeString is `readStringFromUnicodeSequenceToken`'s hex branch.
func javaUnicodeString(v []byte) string {
	if len(v) == 1 {
		return string(rune(v[0]))
	}
	if len(v) == 2 && v[0] == 0xFF && v[1] == 0xFE {
		return "￾"
	}
	return javaUTF16BE(v)
}

// javaUTF16BE is Java's `new String(bytes, UTF_16BE)`: an unpaired surrogate and a trailing odd byte each
// decode to U+FFFD, and a byte-order mark is NOT stripped — it is a character like any other.
func javaUTF16BE(v []byte) string {
	u := make([]uint16, len(v)/2)
	for i := range u {
		u[i] = uint16(v[2*i])<<8 | uint16(v[2*i+1])
	}
	s := string(utf16.Decode(u))
	if len(v)%2 == 1 {
		s += "�"
	}
	return s
}

// bfRangeTail reads a `bfrange` entry's destination after its two codes. With `chars` it records an
// array destination's values into it and an incrementing one into `*ranges`; with neither it only
// consumes.
func (t *cmapTokens) bfRangeTail(lo, hi []byte, chars map[int]string, ranges *[]uniRange) bool {
	if len(lo) == 0 && len(hi) == 0 {
		// `getBfrangeEndFromBytes` indexes an empty array and throws an exception `CMapFactory` does not catch — no
		// CMap veraPDF reads. (It panicked here too, and a panic mid-walk left the content population half built.)
		return false
	}
	begin, end := numberFromBytes(lo), bfRangeEnd(hi, lo)
	tk, ok := t.next()
	if !ok {
		return false
	}
	if tk.Kind == contentstream.ArrayOpen {
		// veraPDF reads ONE destination per code in the range, then skips one token for the `]`. An array
		// holding a different number of destinations leaves its parser out of step with the file, which
		// nib does not model: it reports the CMap as one it cannot read, never as a reading.
		// A range veraPDF reads as empty (end below begin) reads ZERO destinations and then skips one token, which is
		// clean only when that token is the `]` of an empty array.
		count := int64(0)
		if end >= begin {
			count = end - begin + 1
		}
		closeAt := MatchingClose(t.toks, t.i-1, contentstream.ArrayOpen, contentstream.ArrayClose)
		if closeAt >= len(t.toks) || int64(closeAt-t.i) != count {
			return false
		}
		for c := begin; c <= end; c++ {
			v, ok := t.unicodeValue()
			if !ok {
				return false
			}
			if chars != nil {
				chars[javaInt(c)] = v
			}
		}
		t.next()
		return true
	}
	if tk.Kind != contentstream.HexString {
		return false
	}
	dst := Hex(tk.Bytes(t.src))
	if len(dst) == 0 {
		return true // veraPDF logs "string is empty" and adds nothing
	}
	if ranges != nil {
		*ranges = append(*ranges, uniRange{begin: begin, end: end, start: numberFromBytes(dst), length: len(dst)})
	}
	return true
}

// numberFromBytes is `CMapParser.numberFromBytes`: a Java `long` built by shifting, so a value past eight
// bytes wraps exactly as it does there.
func numberFromBytes(b []byte) int64 {
	var v int64
	for i, c := range b {
		v += int64(c) << (uint((len(b)-i-1)*8) & 63)
	}
	return v
}

// javaInt is Java's `(int)` cast of a `long`: the low 32 bits, sign-extended. **Every code veraPDF looks up is
// one** (`CMap.getCodeFromStream`, `readSingleToUnicodeMapping`), so a four-byte code at or above 0x80000000 is
// NEGATIVE there — which no range, whose bounds are positive longs, can hold.
func javaInt(v int64) int { return int(int32(v)) }

// bfRangeEnd is `CMapParser.getBfrangeEndFromBytes`: the end code is the BEGIN code's leading bytes with
// the END code's last byte. So `<0000> <FFFF>` is 256 codes to veraPDF, not 65,536 — it logs that the
// range "contains more than 256 code" and reads it as the specification says a range must be written.
//
// Each term is a Java `int` shift (the count masked to five bits) added into a `long`, so a four-byte begin
// code at or above 0x80000000 gives a NEGATIVE end and the range holds nothing — GB18030's four-byte ranges
// map nothing in veraPDF, measured by the P07.S02 review.
func bfRangeEnd(end, begin []byte) int64 {
	n := len(begin)
	if len(end) > n {
		n = len(end)
	}
	b := make([]byte, n)
	copy(b[n-len(begin):], begin)
	e := make([]byte, n)
	copy(e[n-len(end):], end)
	term := func(c byte, shift int) int64 { return int64(int32(uint32(c) << (uint(shift) & 31))) }
	var v int64
	for i := 0; i < n-1; i++ {
		v += term(b[i], (n-i-1)*8)
	}
	return v + term(e[n-1], 0)
}

// uniRange is veraPDF's `ToUnicodeInterval`.
type uniRange struct {
	begin, end, start int64
	length            int
}

// value is `ToUnicodeInterval.toUnicode`: the destination's integer advanced by the code's offset, written
// back into the destination's width, then read by `getUnicodeNameFromLong` — which answers U+FFFE for
// EITHER adjacent byte pair FF FE or FE FF anywhere in it, a single character of the last byte when the
// first byte is zero, and UTF-16BE otherwise.
func (r uniRange) value(code int64) string {
	u := make([]byte, r.length)
	n := code - r.begin + r.start
	for i := len(u) - 1; i >= 0; i-- {
		u[i] = byte(n)
		n >>= 8
	}
	for i := 0; i+1 < len(u); i++ {
		if (u[i] == 0xFF && u[i+1] == 0xFE) || (u[i] == 0xFE && u[i+1] == 0xFF) {
			return "￾"
		}
	}
	if u[0] == 0 {
		return string(rune(u[len(u)-1]))
	}
	return javaUTF16BE(u)
}

// ToUnicode is a `/ToUnicode` CMap as veraPDF 1.30.2 reads it.
type ToUnicode struct {
	chars  map[int]string
	ranges []uniRange
	// blocks index the ranges: veraPDF's range end shares its begin code's leading bytes (`bfRangeEnd`), so a
	// range lies inside one 256-code block, and each slot of a block holds the FIRST range covering it — the
	// one `CMap.getUnicode`'s walk in file order would reach. It is what makes a lookup constant-time: a linear
	// walk over a million attacker-written ranges cost ~2.25 ms per code (the P07.S02 review, measured).
	blocks map[int64]*rangeBlock
	// Malformed is an entry of the wrong kind, where veraPDF-parser throws and uses an EMPTY CMap instead.
	// nib's tokenizer is not veraPDF's PostScript interpreter, so a caller treats it as a CMap it could not
	// read — never as one mapping nothing.
	Malformed bool
	// Truncated is a CMap whose ranges span more blocks than `maxRangeBlocks`; a code none of the indexed ones
	// maps is then unknown, not unmapped.
	Truncated bool
	// UseCMapName is the CMap a `/Name usecmap` in the PROGRAM names. veraPDF merges it at that point in the
	// parse — its entries overwriting what came before — and only when the name is a CMap it carries.
	UseCMapName string
	// used is the CMap a `/UseCMap` in the stream's DICTIONARY names (`Use`), consulted on a miss.
	used *ToUnicode
}

// rangeBlock is one 256-code block's index: bit c of filled says slot c is taken, idx[c] by which range.
type rangeBlock struct {
	filled [4]uint64
	idx    [256]int32
}

// maxRangeBlocks bounds the blocks a CMap's ranges — or every CMap a caller reads, through one ParseToUnicode budget —
// may index: 32,768 blocks is eight million codes and 34 MB, and a real font's ToUnicode fills a few hundred.
const maxRangeBlocks = 1 << 15

// ParseToUnicode reads a `/ToUnicode` CMap program the way veraPDF does, spending range blocks from a budget the
// caller shares across every CMap it reads: each block holds 1 KB, so a per-CMap ceiling alone let a document of many
// fonts hold 37 MB apiece (the P07.S02 re-review measured 1.56 GB peak at 32 fonts). A CMap that finds the budget
// spent is Truncated.
func ParseToUnicode(src []byte, blockBudget *int) *ToUnicode {
	t := newCMapTokens(src)
	u := &ToUnicode{chars: map[int]string{}}
	ok := t.lists(func(key string) bool {
		switch key {
		case "bfchar":
			code, ok := t.hex()
			if !ok {
				return false
			}
			v, ok := t.unicodeValue()
			if !ok {
				return false
			}
			u.chars[javaInt(numberFromBytes(code))] = v
			return true
		case "bfrange":
			lo, a := t.hex()
			hi, b := t.hex()
			if a && b && len(lo) > 4 {
				// Past four bytes Java's shifts wrap, and a range is no longer inside one block; nib does not
				// model that, and no font writes it.
				return false
			}
			return a && b && t.bfRangeTail(lo, hi, u.chars, &u.ranges)
		}
		return t.skipEntry(key)
	}, func(name string) { u.UseCMapName = name })
	u.Malformed = !ok
	u.index(blockBudget)
	return u
}

// index builds `blocks` from `ranges`, first range first. A range that adds no slot costs a few word operations,
// and a block's slots are filled once, so the cost is bounded by the ranges plus 256 per block.
func (u *ToUnicode) index(budget *int) {
	u.blocks = map[int64]*rangeBlock{}
	for ri, r := range u.ranges {
		if r.end < r.begin {
			continue
		}
		key := r.begin >> 8
		blk := u.blocks[key]
		if blk == nil {
			if *budget <= 0 {
				u.Truncated = true
				continue
			}
			*budget--
			blk = &rangeBlock{}
			u.blocks[key] = blk
		}
		lo, hi := int(r.begin&255), int(r.end&255)
		for c := lo; c <= hi; c++ {
			w, bit := c>>6, uint64(1)<<(c&63)
			if blk.filled[w]&bit != 0 {
				// Skip a filled run a word at a time.
				if blk.filled[w] == ^uint64(0) {
					c |= 63
				}
				continue
			}
			blk.filled[w] |= bit
			blk.idx[c] = int32(ri)
		}
	}
}

// Use is `PDCMap.getCMapFile`'s merge of a `/UseCMap` named in the stream's dictionary: the used CMap's
// `bfchar` entries OVERWRITE this one's (`CMap.useCMap` is a `putAll`), and the used CMap answers a code this
// one's entries and ranges do not.
func (u *ToUnicode) Use(o *ToUnicode) {
	if o.Malformed {
		// veraPDF would merge an EMPTY CMap here; nib does not claim to know that it would.
		u.Malformed = true
		return
	}
	for k, v := range o.chars {
		u.chars[k] = v
	}
	u.used = o
}

// With is Use on a copy: the receiver, which a caller may have cached and shared, is left as parsed.
func (u *ToUnicode) With(o *ToUnicode) *ToUnicode {
	c := *u
	c.chars = make(map[int]string, len(u.chars)+len(o.chars))
	for k, v := range u.chars {
		c.chars[k] = v
	}
	c.Use(o)
	return &c
}

// Lookup returns the Unicode text veraPDF gives a code, and whether it gives one: an entry first, then the
// first incrementing range holding it in file order (`CMap.getUnicode`), then the CMap its dictionary uses
// (`PDCMap.toUnicode`). `known` is false where nib cannot say — a malformed or truncated CMap.
func (u *ToUnicode) Lookup(code int) (text string, mapped, known bool) {
	if u == nil {
		return "", false, true
	}
	if u.Malformed {
		return "", false, false
	}
	if s, ok := u.chars[code]; ok {
		return s, true, true
	}
	c := int64(code)
	if blk := u.blocks[c>>8]; blk != nil {
		if slot := c & 255; blk.filled[slot>>6]&(uint64(1)<<(slot&63)) != 0 {
			r := u.ranges[blk.idx[slot]]
			if c >= r.begin && c <= r.end {
				return r.value(c), true, true
			}
		}
	}
	if u.Truncated {
		return "", false, false
	}
	if u.used != nil {
		return u.used.Lookup(code)
	}
	return "", false, true
}

// maxToUnicodeRange bounds one `bfrange` in TextMap: a 16-bit code space has 65,536 codes.
const maxToUnicodeRange = 1 << 16

// maxToUnicodeExpansion bounds the codes one CMap's ranges may expand to in TOTAL in TextMap: two full
// 16-bit code spaces. A font pdfops can split has at most one (a simple font 256 codes, Identity-H 65,536),
// so the second is headroom for ranges that overlap, not for more distinct codes.
const maxToUnicodeExpansion = 2 << 16

// TextMap reads a `/ToUnicode` CMap's `bfchar` and `bfrange` blocks — both range forms, the incrementing
// destination and the array of destinations — into code bytes → text, leniently: every entry up to the
// `end` keyword whatever the stated count, a range across its whole begin–end span, and a destination read
// as the specification's UTF-16BE (a ligature maps to two characters, a surrogate pair to one).
//
// # A total budget, not only a per-range one (`/pending 503`)
//
// `maxToUnicodeRange` bounds ONE range, and one range is not the cost: a 2.2 KB CMap of a hundred
// overlapping full-plane ranges expanded 6.5 million codes into 65,536 entries, 5.1 s and 110 MB, reached
// from `ProposeTags` and `CommitTags` on any page drawn in that font. So every range spends from
// `maxToUnicodeExpansion`, and a range that does not fit what is left is not expanded — its codes read as
// undecoded, which the run already reports, rather than as a stall.
func TextMap(src []byte) map[string]string {
	out := map[string]string{}
	budget := maxToUnicodeExpansion
	toks := contentstream.Tokenize(src)
	mode := ""
	var items []cmapItem
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.Kind {
		case contentstream.Whitespace:
			continue
		case contentstream.HexString:
			if mode != "" {
				items = append(items, cmapItem{b: Hex(t.Bytes(src))})
			}
			continue
		case contentstream.ArrayOpen:
			end := MatchingClose(toks, i, contentstream.ArrayOpen, contentstream.ArrayClose)
			if mode != "" {
				var arr [][]byte
				for _, at := range toks[i+1 : end] {
					if at.Kind == contentstream.HexString {
						arr = append(arr, Hex(at.Bytes(src)))
					}
				}
				items = append(items, cmapItem{arr: arr, isArr: true})
			}
			i = end
			continue
		}
		switch string(t.Bytes(src)) {
		case "beginbfchar":
			mode, items = "char", nil
		case "beginbfrange":
			mode, items = "range", nil
		case "endbfchar":
			for j := 0; j+1 < len(items); j += 2 {
				if !items[j].isArr && !items[j+1].isArr {
					out[string(items[j].b)] = utf16Text(items[j+1].b)
				}
			}
			mode, items = "", nil
		case "endbfrange":
			for j := 0; j+2 < len(items); j += 3 {
				lo, hi := items[j], items[j+1]
				if lo.isArr || hi.isArr || len(lo.b) != len(hi.b) {
					continue
				}
				expandBFRange(out, lo.b, hi.b, items[j+2], &budget)
			}
			mode, items = "", nil
		}
	}
	return out
}

// cmapItem is one operand inside a `bfchar`/`bfrange` block.
type cmapItem struct {
	b     []byte
	arr   [][]byte
	isArr bool
}

func expandBFRange(out map[string]string, lo, hi []byte, dst cmapItem, budget *int) {
	l, h := Value(lo), Value(hi)
	if h < l || h-l >= maxToUnicodeRange || h-l+1 > *budget {
		return
	}
	*budget -= h - l + 1
	for k := 0; k <= h-l; k++ {
		code := make([]byte, len(lo))
		v := l + k
		for b := len(code) - 1; b >= 0; b-- {
			code[b] = byte(v)
			v >>= 8
		}
		if dst.isArr {
			if k < len(dst.arr) {
				out[string(code)] = utf16Text(dst.arr[k])
			}
			continue
		}
		d := append([]byte(nil), dst.b...)
		switch {
		case len(d) >= 2:
			u := int(d[len(d)-2])<<8 | int(d[len(d)-1])
			u += k
			d[len(d)-2], d[len(d)-1] = byte(u>>8), byte(u)
		case len(d) == 1:
			d[0] += byte(k)
		}
		out[string(code)] = utf16Text(d)
	}
}

// utf16Text decodes a CMap destination, which is UTF-16BE and may carry surrogate pairs or several
// characters (a ligature maps to two).
func utf16Text(b []byte) string {
	if len(b)%2 == 1 {
		return string(b)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return string(utf16.Decode(u))
}
