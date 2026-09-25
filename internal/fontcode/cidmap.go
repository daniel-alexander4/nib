package fontcode

import (
	"math"

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
// in-stream `/Identity-H usecmap` (or -V) appends the identity mapping after the CMap's own. Any other predefined name
// loads a CMap nib does not carry, and the codespace reader (`ParseCodespace`'s `UsesCMaps`) is where a caller refuses
// it; a name that is not predefined loads nothing in veraPDF either.
func ParseCIDMap(src []byte) *CIDMap {
	t := newCMapTokens(src)
	m := &CIDMap{}
	var own []cidMapping // this CMap's cid mappings, in file order; they precede every used one, last first
	var used []cidMapping
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
// and a caller charges the walk to a budget it must be able to stop.
func (c CIDChain) Lookup(code, limit int) (cid int, held bool, asked int, done bool) {
	for pass := 0; pass < 2; pass++ {
		for _, m := range c {
			list := m.cids
			if pass == 1 {
				list = m.notdefs
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
