package fontcode

import (
	"container/heap"
	"math"
	"math/bits"
	"sort"

	"nib/internal/contentstream"
)

// CIDMap is a CMap's code-to-CID mappings as veraPDF-parser 1.30.2's `CMap` holds them, for `toCID` and
// `containsCode` (`PLAN-ua-coverage.md` P07.S04b, every rule measured on veraPDF before it was written):
//
//   - `cidrange` and `cidchar` entries are inserted at the FRONT of one list (`CMap.addCidInterval`, `add(0, …)`),
//     so the LAST one read answers first — within one list and across lists alike;
//   - `notdefrange` and `notdefchar` entries are appended to a second list, consulted only when no cid mapping holds
//     the code, in file order;
//   - a used CMap's lists are APPENDED to both (`CMap.useCMap`, `addAll`) — for an in-stream `usecmap` and a
//     dictionary `/UseCMap` alike — so a CMap's own CID entries answer before the ones it uses (its notdef entries
//     read AFTER an in-stream `usecmap` would follow the used CMap's; the only CMap nib merges in-stream is Identity,
//     which has none, so that ordering never arises);
//   - a mapping answering -1 is the no-match sentinel (`CMap.toCID`'s `res != -1`), so a range reaching CID -1 is
//     skipped as if it held nothing;
//   - a code nothing holds is CID 0.
type CIDMap struct {
	cids    []cidMapping
	notdefs []cidMapping
	// cidIndex and notdefIndex answer a carried CMap's two lists by search rather than by walk (`index`); nil is
	// a list that is walked.
	cidIndex, notdefIndex *cidIndex
	// Malformed is a CMap veraPDF-parser throws on, and so reads as holding no mapping at all.
	Malformed bool
}

// Entries is how many mappings the CMap holds — a caller's measure of what reading it cost.
func (m *CIDMap) Entries() int { return len(m.cids) + len(m.notdefs) }

// cidMapping is a `CIDInterval`, a `NotDefInterval` (notdef: every code is its one CID) or a `SingleCIDMapping` (a
// range of one code, either kind). Bounds and CID are Java `int`s.
type cidMapping struct {
	lo, hi, cid int
	notdef      bool
}

// of is `CIDMappable.getCID`: -1 outside the range, and `startingCID + character - intervalStart` in Java `int`
// arithmetic inside it.
func (m cidMapping) of(code int) int {
	if code < m.lo || code > m.hi {
		return -1
	}
	if m.notdef {
		return m.cid
	}
	return int(int32(int64(m.cid) + int64(code) - int64(m.lo)))
}

// identityMapping is Identity-H's and Identity-V's mappings: veraPDF's resource file writes them as 256 ranges of
// 256 codes each starting at its own first code, which answers every code of 0000-FFFF as itself.
var identityMapping = cidMapping{lo: 0, hi: 0xFFFF, cid: 0}

// IdentityCIDs is Identity-H's and Identity-V's code-to-CID mapping.
func IdentityCIDs() *CIDMap { return &CIDMap{cids: []cidMapping{identityMapping}} }

// ParseCIDMap reads an embedded CMap program's cid and notdef lists, with veraPDF's count and type rules. An
// in-stream `/Identity-H usecmap` (or -V) appends the identity mapping after the CMap's own, and one naming a
// predefined CMap nib carries appends that CMap's mappings (`predefined`). A predefined name nib does NOT carry is
// for the codespace reader's caller to refuse (`ParseCodespace`'s `UsesCMaps`); a name that is not predefined loads
// nothing in veraPDF either.
func ParseCIDMap(src []byte) *CIDMap {
	t := newCMapTokens(src)
	m := &CIDMap{}
	var own []cidMapping // this CMap's cid mappings, in file order; they precede every used one, last first
	var used []cidMapping
	merged := map[string]bool{}
	ok := t.lists(func(key string) bool {
		var e cidMapping
		switch key {
		case "cidrange", "notdefrange":
			lo, a := t.hex()
			hi, b := t.hex()
			cid, c := t.javaInteger()
			if !a || !b || !c {
				return false
			}
			e = cidMapping{lo: javaInt(numberFromBytes(lo)), hi: javaInt(numberFromBytes(hi)), cid: cid, notdef: key == "notdefrange"}
		case "cidchar", "notdefchar":
			code, a := t.hex()
			cid, c := t.javaInteger()
			if !a || !c {
				return false
			}
			n := javaInt(numberFromBytes(code))
			e = cidMapping{lo: n, hi: n, cid: cid, notdef: key == "notdefchar"}
		default:
			return t.skipEntry(key)
		}
		if e.notdef {
			m.notdefs = append(m.notdefs, e)
		} else {
			own = append(own, e)
		}
		return true
	}, func(name string) {
		if name == "Identity-H" || name == "Identity-V" {
			used = append(used, identityMapping)
			return
		}
		// A predefined CMap nib carries (ADR-117): its cid mappings after this CMap's own, its notdef mappings where
		// the operator stands. **Once per name**: a second copy of either list sits behind the first and can answer
		// nothing the first has not, and a program repeating the operator would otherwise copy the table each time.
		if pre := predefined(name); pre != nil && !merged[name] {
			merged[name] = true
			used = append(used, pre.cids.cids...)
			m.notdefs = append(m.notdefs, pre.cids.notdefs...)
		}
	})
	if !ok {
		return &CIDMap{Malformed: true}
	}
	for i := len(own) - 1; i >= 0; i-- {
		m.cids = append(m.cids, own[i])
	}
	m.cids = append(m.cids, used...)
	return m
}

// CIDChain is a Type 0 font's code-to-CID mapping: its /Encoding CMap, then each CMap down its /UseCMap chain
// (`PDCMap.getCMapFile` appends each used CMap's lists with `useCMap`). It is read in place — every CID mapping of
// every CMap in chain order, then every notdef mapping — so a font shares the CMaps it names rather than copying them.
type CIDChain []*CIDMap

// Lookup is `CMap.toCID` and `CMap.containsCode` at once: the CID, whether any mapping holds the code, how many
// mappings were asked, and whether the answer was reached within `limit` asks — a CMap may hold a million entries,
// and a caller charges the walk to a budget it must be able to stop. A list that has an index (a carried CMap's)
// is searched instead, to the same answer, and charged the search's comparisons.
func (c CIDChain) Lookup(code, limit int) (cid int, held bool, asked int, done bool) {
	for pass := 0; pass < 2; pass++ {
		for _, m := range c {
			list, idx := m.cids, m.cidIndex
			if pass == 1 {
				list, idx = m.notdefs, m.notdefIndex
			}
			if idx != nil {
				if asked+idx.asks > limit {
					return 0, false, asked, false
				}
				asked += idx.asks
				if v, ok := idx.of(code); ok {
					return v, true, asked, true
				}
				continue
			}
			for _, e := range list {
				if asked >= limit {
					return 0, false, asked, false
				}
				asked++
				if v := e.of(code); v != -1 {
					return v, true, asked, true
				}
			}
		}
	}
	return 0, false, asked, true
}

// javaInteger reads the next token as an INTEGER entry — `checkTokenType(TT_INTEGER)` — with `javaIntegerValue`'s
// value; anything else is where veraPDF throws.
func (t *cmapTokens) javaInteger() (int, bool) {
	tk, ok := t.next()
	if !ok || tk.Kind != contentstream.Operand {
		return 0, false
	}
	return javaIntegerValue(tk.Bytes(t.src))
}

// javaIntegerValue is `BaseParser.readNumber` for an INTEGER token, then the `(int)` cast: an optional sign, then
// digits. No digits at all, or more than a Java `long` holds, is `Math.round(Double.MAX_VALUE)` — Long.MAX_VALUE,
// whose low 32 bits are -1. A token with a `.` is a REAL, and anything else is not a number. (A token veraPDF's
// reader would SPLIT — `5a` — is not an integer here: nib does not mirror where the next object would then start.)
func javaIntegerValue(b []byte) (int, bool) {
	neg := false
	if len(b) > 0 && (b[0] == '+' || b[0] == '-') {
		neg = b[0] == '-'
		b = b[1:]
	}
	var v int64
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	for _, c := range b {
		if v > (math.MaxInt64-int64(c-'0'))/10 {
			v = math.MaxInt64
			break
		}
		v = v*10 + int64(c-'0')
	}
	if len(b) == 0 {
		v = math.MaxInt64
	}
	if neg {
		v = -v
	}
	return javaInt(v), true
}

// cidIndex is one list of mappings cut into stretches of codes that do not overlap, in code order, each naming
// the mapping the walk would have answered it with — the FIRST in list order to hold the code. A search of it
// is the walk's answer for every code (/pending 858).
//
// It exists for the predefined CMaps nib carries. Measured through UniGB-UCS2-H (13,826 mappings), the walk cost
// 3.8 million asks for 500 distinct common characters and 28.5 million for GB 2312's 3,755 — against a document's
// budget of 4.2 million — so one page of ordinary Chinese or Japanese through a UCS2 CMap was refused.
type cidIndex struct {
	spans []cidSpan
	asks  int // what one search is charged: the comparisons it makes
}

// cidSpan is the codes lo..hi and the mapping that answers them.
type cidSpan struct {
	lo, hi int
	by     cidMapping
}

// indexMinimum is the shortest list worth a search: under it the walk costs no more.
const indexMinimum = 16

// indexMappings is the index of a list, or nil for a list that is walked: a short one, and one holding a mapping
// that can answer -1 — the walk passes such a mapping by and asks the next, which no span can say. A mapping
// answers -1 only from a negative CID or past the end of a Java `int`, and no carried CMap has either.
func indexMappings(list []cidMapping) *cidIndex {
	if len(list) < indexMinimum {
		return nil
	}
	var edges []int
	var byLo []int // the mappings that hold any code, by first code
	for i, e := range list {
		if e.lo > e.hi {
			continue // holds no code, and answers none in the walk
		}
		if e.cid < 0 || (!e.notdef && int64(e.cid)+int64(e.hi)-int64(e.lo) > math.MaxInt32) || e.hi == math.MaxInt {
			return nil
		}
		edges = append(edges, e.lo, e.hi+1)
		byLo = append(byLo, i)
	}
	sort.Ints(edges)
	sort.SliceStable(byLo, func(a, b int) bool { return list[byLo[a]].lo < list[byLo[b]].lo })
	idx := &cidIndex{}
	open := &firstInOrder{} // the mappings holding the stretch being cut, the first in list order on top
	next := 0
	for k := 0; k+1 < len(edges); k++ {
		from, to := edges[k], edges[k+1]-1
		if from > to {
			continue // the same edge twice
		}
		for next < len(byLo) && list[byLo[next]].lo <= from {
			heap.Push(open, byLo[next])
			next++
		}
		for open.Len() > 0 && list[(*open)[0]].hi < from {
			heap.Pop(open)
		}
		if open.Len() == 0 {
			continue // codes no mapping holds
		}
		by := list[(*open)[0]]
		if n := len(idx.spans); n > 0 && idx.spans[n-1].hi == from-1 && idx.spans[n-1].by == by {
			idx.spans[n-1].hi = to
			continue
		}
		idx.spans = append(idx.spans, cidSpan{lo: from, hi: to, by: by})
	}
	idx.asks = bits.Len(uint(len(idx.spans))) + 1
	return idx
}

// of is the CID of a code and whether any mapping of the list holds it.
func (x *cidIndex) of(code int) (int, bool) {
	i := sort.Search(len(x.spans), func(i int) bool { return x.spans[i].hi >= code })
	if i == len(x.spans) || x.spans[i].lo > code {
		return 0, false
	}
	return x.spans[i].by.of(code), true
}

// index gives the CMap's two lists their indexes. It is for a CMap held for the process — a carried one, before
// anything else can read it: a document's own CMap is read once and its walk is charged to that document.
func (m *CIDMap) index() {
	m.cidIndex, m.notdefIndex = indexMappings(m.cids), indexMappings(m.notdefs)
}

// firstInOrder is a heap of positions in a list of mappings, the lowest on top.
type firstInOrder []int

func (h firstInOrder) Len() int           { return len(h) }
func (h firstInOrder) Less(a, b int) bool { return h[a] < h[b] }
func (h firstInOrder) Swap(a, b int)      { h[a], h[b] = h[b], h[a] }
func (h *firstInOrder) Push(v any)        { *h = append(*h, v.(int)) }
func (h *firstInOrder) Pop() any {
	old := *h
	v := old[len(old)-1]
	*h = old[:len(old)-1]
	return v
}
