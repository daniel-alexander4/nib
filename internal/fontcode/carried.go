package fontcode

import (
	"strings"
	"sync"

	"nib/internal/cmapres"
)

// The CMaps a document names and does not hold (ADR-117): ISO 32000-1's predefined CMaps and Adobe's UCS2 CMaps,
// which `internal/cmapres` carries as programs. They are read HERE, by the readers an embedded CMap is read by, so a
// carried CMap's codespace, its code-to-CID lists and its Unicode entries follow the same rules — veraPDF's — and a
// `usecmap` naming one is merged where the operator stands, in an embedded program and a carried one alike.
//
// What was read is held for the process and shared by every document: nothing writes to a CMap after its parse
// (`Codespace.Merge` and `Clone` take care not to), and a caller that merges into a codespace clones it first.

// carriedCMap is one predefined CMap as read: what cuts its strings and what maps its codes.
type carriedCMap struct {
	cs   *Codespace
	cids *CIDMap
}

var carried struct {
	sync.Mutex
	cmaps map[string]*carriedCMap
	ucs2  map[string]*UCS2Map
}

// isUCS2Name is whether a carried name is one of Adobe's CID-to-Unicode CMaps rather than a predefined CMap: they
// are the carried names that begin `Adobe-`.
func isUCS2Name(name string) bool { return strings.HasPrefix(name, "Adobe-") }

// predefined is the carried predefined CMap of that name, read once; nil where none is carried (Identity-H and
// Identity-V among them: the readers hold those themselves).
func predefined(name string) *carriedCMap {
	if isUCS2Name(name) || !cmapres.Has(name) {
		return nil
	}
	carried.Lock()
	c, ok := carried.cmaps[name]
	carried.Unlock()
	if ok {
		return c
	}
	// Read outside the lock: a carried CMap that uses another reads it through this same door.
	if prog, err := cmapres.Program(name); err == nil {
		c = &carriedCMap{cs: ParseCodespace(prog), cids: ParseCIDMap(prog)}
		if c.cs.Malformed || c.cids.Malformed || len(c.cs.UsesCMaps) > 0 {
			c = nil // a table nib cannot read whole is one it does not carry
		} else {
			c.cs.tailOwn = false
		}
	}
	carried.Lock()
	defer carried.Unlock()
	if prior, ok := carried.cmaps[name]; ok {
		return prior
	}
	if carried.cmaps == nil {
		carried.cmaps = map[string]*carriedCMap{}
	}
	carried.cmaps[name] = c
	return c
}

// Predefined is a predefined CMap nib carries: the codespace its strings are cut by and its code-to-CID mapping,
// with every CMap it uses merged in. ok is false for a name nib carries no table for. The Codespace is shared:
// a caller that merges into it clones it first.
func Predefined(name string) (cs *Codespace, cids *CIDMap, ok bool) {
	c := predefined(name)
	if c == nil {
		return nil, nil, false
	}
	return c.cs, c.cids, true
}

// maxUCS2Blocks is the range-index budget one UCS2 CMap is read with: Adobe's largest spans some 31,000 CIDs,
// which is 120 blocks.
const maxUCS2Blocks = 1 << 10

// UCS2Map is one of Adobe's `Adobe-<ordering>-UCS2` CMaps as nib carries it: a CID's Unicode text.
//
// **It is not the file veraPDF reads.** nib's is the version pdf.js packs and veraPDF's is the one in its own jar;
// read side by side (`TestEveryCarriedCMapReadsAsVeraPDFsOwnDoes`) they give some 1,700 CIDs different TEXT — a
// figure dash for an en dash, a character with and without its variation selector. PDF/UA-1 7.21.7 does not read the
// text: it asks whether a glyph has any (t1) and whether it holds U+0000, U+FEFF or U+FFFE (t2), and on those two
// questions the versions part at 95 CIDs only, in three runs — `ucs2Gaps`. There the answer is `known` false, and a
// caller says it cannot tell. Everywhere else the two questions have veraPDF's answers, and `text` is nib's version's.
type UCS2Map struct {
	tu   *ToUnicode
	gaps [][2]int
}

// ucs2Gaps are the CID runs where a carried UCS2 CMap and veraPDF's own disagree on whether a CID has text: the
// Adobe-CNS1 run and the second Adobe-Japan1 run are CIDs veraPDF's newer file maps and nib's does not, the first
// Adobe-Japan1 run the reverse. Neither version maps any CID to U+0000, U+FEFF or U+FFFE. Measured, and held exact —
// no run missing, none left over — by the test wherever veraPDF is present.
var ucs2Gaps = map[string][][2]int{
	"Adobe-CNS1-UCS2":   {{19088, 19178}},
	"Adobe-Japan1-UCS2": {{23053, 23054}, {23058, 23059}},
}

// Lookup is the text of a CID and whether it has any; known is false where veraPDF's version of the CMap answers
// differently from nib's (`ucs2Gaps`).
func (m *UCS2Map) Lookup(cid int) (text string, mapped, known bool) {
	for _, g := range m.gaps {
		if cid >= g[0] && cid <= g[1] {
			return "", false, false
		}
	}
	return m.tu.Lookup(cid)
}

// UCS2 is Adobe's `<registry>-<ordering>-UCS2` CMap where nib carries it; nil where it does not (`Adobe-KR-UCS2`,
// and every collection Adobe publishes no such CMap for).
func UCS2(registry, ordering string) *UCS2Map {
	name := registry + "-" + ordering + "-UCS2"
	if !isUCS2Name(name) || !cmapres.Has(name) {
		return nil
	}
	carried.Lock()
	defer carried.Unlock()
	if u, ok := carried.ucs2[name]; ok {
		return u
	}
	var u *UCS2Map
	if prog, err := cmapres.Program(name); err == nil {
		budget := maxUCS2Blocks
		if tu := ParseToUnicode(prog, &budget); !tu.Malformed && !tu.Truncated {
			u = &UCS2Map{tu: tu, gaps: ucs2Gaps[name]}
		}
	}
	if carried.ucs2 == nil {
		carried.ucs2 = map[string]*UCS2Map{}
	}
	carried.ucs2[name] = u
	return u
}
