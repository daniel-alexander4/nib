package fontcode

import (
	"math"
	"testing"

	"nib/internal/cmapres"
)

// TestCIDMapsAreReadTheWayVeraPDFReadsThem — `CMap.toCID`'s order: the last cid mapping read answers first, notdef
// mappings in file order after every cid mapping, a used CMap's mappings after the CMap's own, and a mapping answering
// -1 is no answer at all. Every row but one is a shape `uacheck`'s P07.S04b fixtures measured end to end on veraPDF
// 1.30.2; the negative CID is the one veraPDF throws on (no report), so its row pins only what nib hands `uacheck`.
func TestCIDMapsAreReadTheWayVeraPDFReadsThem(t *testing.T) {
	for _, c := range []struct {
		body string
		code int
		cid  int
		held bool
	}{
		{"1 begincidrange <0000> <00FF> 0 endcidrange 1 begincidchar <0021> 5 endcidchar", 0x21, 5, true},
		{"1 begincidchar <0021> 5 endcidchar 1 begincidrange <0000> <00FF> 0 endcidrange", 0x21, 0x21, true},
		{"2 begincidrange <0021> <0021> 5 <0000> <00FF> 0 endcidrange", 0x21, 0x21, true},
		{"1 begincidrange <0020> <0030> 4 endcidrange", 0x21, 5, true},
		{"1 beginnotdefrange <0020> <0030> 7 endnotdefrange", 0x25, 7, true},
		{"2 beginnotdefchar <0021> 5 <0021> 33 endnotdefchar", 0x21, 5, true},
		{"1 beginnotdefchar <0021> 33 endnotdefchar 1 begincidchar <0021> 5 endcidchar", 0x21, 5, true},
		{"/Identity-H usecmap 1 begincidchar <0021> 5 endcidchar", 0x21, 5, true},
		{"1 begincidchar <0021> 5 endcidchar /Identity-V usecmap", 0x21, 5, true},
		{"/Identity-H usecmap 1 begincidchar <0022> 5 endcidchar", 0x21, 0x21, true},
		{"1 begincidrange <0020> <0030> 4 endcidrange 1 begincidchar <0021> -1 endcidchar", 0x21, 5, true},
		{"1 begincidchar <0021> -5 endcidchar", 0x21, -5, true},
		{"1 begincidchar <0022> 5 endcidchar", 0x21, 0, false},
		{"1 begincidchar <0021> +7 endcidchar", 0x21, 7, true},
		{"1 begincidchar <0021> + endcidchar", 0x21, 0, false},
		{"/Identity#2DH usecmap 1 begincidchar <0022> 5 endcidchar", 0x21, 0, false},
		{"/Identity-H [ /X ] usecmap 1 begincidchar <0022> 5 endcidchar", 0x21, 0x21, true},
		{"/Identity-H << >> usecmap 1 begincidchar <0022> 5 endcidchar", 0x21, 0, false},
		{"99999999999999999999 begincidchar 1 begincidchar <0021> 5 endcidchar", 0x21, 0, false},
		// Long.MAX_VALUE's low 32 bits are -1, the sentinel: past a long, a CID maps nothing.
		{"1 begincidchar <0021> 99999999999999999999 endcidchar", 0x21, 0, false},
		{"/Other usecmap 1 begincidchar <0022> 5 endcidchar", 0x21, 0, false},
	} {
		m := ParseCIDMap([]byte("1 begincodespacerange <0000> <FFFF> endcodespacerange " + c.body))
		if m.Malformed {
			t.Errorf("%q: malformed", c.body)
			continue
		}
		if cid, held, _, _ := (CIDChain{m}).Lookup(c.code, 1<<30); cid != c.cid || held != c.held {
			t.Errorf("%q: code %#x is CID %d (held %v), want %d (held %v)", c.body, c.code, cid, held, c.cid, c.held)
		}
	}
}

// TestAMalformedCIDEntryEmptiesTheCMap — `checkTokenType` throws on an entry of the wrong kind, and veraPDF-parser
// discards the whole CMap (measured: a real CID).
func TestAMalformedCIDEntryEmptiesTheCMap(t *testing.T) {
	for _, body := range []string{
		"1 begincidchar <0021> 5.0 endcidchar",
		"1 begincidrange <0021> 5 endcidrange",
		"1 beginnotdefchar <0021> /x endnotdefchar",
	} {
		if m := ParseCIDMap([]byte(body)); !m.Malformed {
			t.Errorf("%q: read as well-formed", body)
		}
		if c := ParseCodespace([]byte(body)); !c.Malformed {
			t.Errorf("%q: the codespace reader reads it as well-formed, so the two readers disagree on one CMap", body)
		}
	}
}

// TestAChainAnswersEveryCIDMappingBeforeAnyNotdef — a dictionary /UseCMap chain is read in place: every CMap's CID
// mappings in chain order, then every notdef mapping, so a used CMap's CID beats the using CMap's notdef.
func TestAChainAnswersEveryCIDMappingBeforeAnyNotdef(t *testing.T) {
	own := ParseCIDMap([]byte("1 begincidchar <0021> 5 endcidchar 1 beginnotdefchar <0022> 8 endnotdefchar 1 beginnotdefchar <0023> 3 endnotdefchar"))
	used := ParseCIDMap([]byte("1 begincidchar <0021> 33 endcidchar 1 begincidchar <0022> 7 endcidchar 1 beginnotdefchar <0023> 9 endnotdefchar"))
	c := CIDChain{own, used}
	for code, want := range map[int]int{0x21: 5, 0x22: 7, 0x23: 3} {
		if cid, _, _, _ := c.Lookup(code, 1<<30); cid != want {
			t.Errorf("code %#x is CID %d, want %d", code, cid, want)
		}
	}
	if cid, held, _, _ := (CIDChain{IdentityCIDs()}).Lookup(0xFFFF, 1<<30); cid != 0xFFFF || !held {
		t.Errorf("Identity maps 0xFFFF to %d (held %v)", cid, held)
	}
}

// TestALookupStopsAtItsLimit — the walk stops where the caller's budget does and says it did not finish.
func TestALookupStopsAtItsLimit(t *testing.T) {
	m := ParseCIDMap([]byte("3 begincidchar <0022> 5 <0023> 6 <0024> 7 endcidchar"))
	if _, _, asked, done := (CIDChain{m}).Lookup(0x21, 2); asked != 2 || done {
		t.Errorf("a lookup limited to 2 asked %d and finished %v, want 2 and false", asked, done)
	}
	if _, held, asked, done := (CIDChain{m}).Lookup(0x21, 3); asked != 3 || !done || held {
		t.Errorf("a lookup limited to 3 asked %d, finished %v, held %v — want 3, true, false", asked, done, held)
	}
}

// walked is the CMap with no index: every lookup of it is the walk.
func walked(m *CIDMap) *CIDMap { return &CIDMap{cids: m.cids, notdefs: m.notdefs} }

// TestACarriedCMapsIndexAnswersAsItsWalkDoes — /pending 858. A carried CMap is searched, not walked, and the search
// is the walk's answer: both sides of every mapping's two ends and a sample of 0000-1FFFF, through every carried
// predefined CMap, gives the same CID and the same "held" either way. The -V CMaps are the ones whose lists
// overlap (their own entries stand in front of the -H CMap's they use), so one of them must be among the indexed.
func TestACarriedCMapsIndexAnswersAsItsWalkDoes(t *testing.T) {
	indexed := map[string]bool{}
	for _, name := range cmapres.Names() {
		_, m, ok := Predefined(name)
		if !ok {
			continue
		}
		indexed[name] = m.cidIndex != nil
		plain := walked(m)
		// Between two neighbouring ends the first mapping to hold a code is the same one, so the ends, the codes
		// beside them and one code between are every case; the stride over 0000-1FFFF is a second, blind sample.
		var codes []int
		for c := -1; c <= 0x1FFFF; c += 37 {
			codes = append(codes, c)
		}
		for _, list := range [][]cidMapping{m.cids, m.notdefs} {
			for _, e := range list {
				codes = append(codes, e.lo-1, e.lo, e.lo+(e.hi-e.lo)/2, e.hi, e.hi+1)
			}
		}
		differ := 0
		for _, code := range codes {
			cid, held, asked, done := (CIDChain{m}).Lookup(code, 1<<30)
			wantCID, wantHeld, _, _ := (CIDChain{plain}).Lookup(code, 1<<30)
			if cid != wantCID || held != wantHeld || !done {
				if differ++; differ <= 3 {
					t.Errorf("%s: code %#x is CID %d (held %v) by the index, and CID %d (held %v) by the walk", name, code, cid, held, wantCID, wantHeld)
				}
			}
			if m.cidIndex != nil && asked > 40 {
				t.Fatalf("%s: code %#x cost %d asks through the index — the walk's cost, not a search's", name, code, asked)
			}
		}
	}
	for _, name := range []string{"UniGB-UCS2-H", "UniJIS-UCS2-H", "UniJIS-UCS2-V", "GBK-EUC-H"} {
		if !indexed[name] {
			t.Errorf("%s is held without an index: a document showing a few hundred characters through it spends its whole CMap budget", name)
		}
	}
}

// TestAnIndexIsTheFirstMappingInListOrder — the index over lists that overlap every way: the first mapping in list
// order to hold a code answers it, a mapping holding no code answers none, and a list with a mapping that can
// answer -1 (which the walk passes by) is not indexed at all.
func TestAnIndexIsTheFirstMappingInListOrder(t *testing.T) {
	seed := uint32(1)
	next := func(n int) int {
		seed = seed*1664525 + 1013904223
		return int(seed>>8) % n
	}
	for round := 0; round < 200; round++ {
		var list []cidMapping
		for i, n := 0, indexMinimum+next(40); i < n; i++ {
			lo := next(300)
			e := cidMapping{lo: lo, hi: lo + next(60), cid: next(5000), notdef: next(4) == 0}
			if next(10) == 0 {
				e.hi = lo - 1 - next(5) // holds nothing
			}
			list = append(list, e)
		}
		idx := indexMappings(list)
		if idx == nil {
			t.Fatalf("round %d: a list of %d ordinary mappings was not indexed", round, len(list))
		}
		m := &CIDMap{cids: list, cidIndex: idx}
		for code := -2; code <= 370; code++ {
			cid, held, _, _ := (CIDChain{m}).Lookup(code, 1<<30)
			wantCID, wantHeld, _, _ := (CIDChain{walked(m)}).Lookup(code, 1<<30)
			if cid != wantCID || held != wantHeld {
				t.Fatalf("round %d: code %d is CID %d (held %v) by the index, and CID %d (held %v) by the walk", round, code, cid, held, wantCID, wantHeld)
			}
		}
	}
	long := func(e cidMapping) []cidMapping {
		list := []cidMapping{e}
		for i := 0; i < indexMinimum; i++ {
			list = append(list, cidMapping{lo: 1000 + i, hi: 1000 + i, cid: i})
		}
		return list
	}
	for name, e := range map[string]cidMapping{
		"a range reaching CID -1":         {lo: 0, hi: 9, cid: -5},
		"a notdef range of CID -1":        {lo: 0, hi: 9, cid: -1, notdef: true},
		"a range running past a Java int": {lo: 0, hi: 9, cid: math.MaxInt32 - 3},
		"a range ending at the last code": {lo: 0, hi: math.MaxInt, cid: 0, notdef: true},
	} {
		if indexMappings(long(e)) != nil {
			t.Errorf("%s was indexed: the walk passes a mapping answering -1 by, and a span cannot", name)
		}
	}
	if indexMappings(long(cidMapping{lo: 0, hi: 9, cid: 1})[:indexMinimum-1]) != nil {
		t.Error("a list shorter than the minimum was indexed")
	}
	// A search is charged whole, and refused whole where the limit will not cover it.
	m := &CIDMap{cids: long(cidMapping{lo: 0, hi: 9, cid: 1})}
	m.index()
	if _, _, asked, done := (CIDChain{m}).Lookup(5, m.cidIndex.asks-1); asked != 0 || done {
		t.Errorf("a search past its limit asked %d and finished %v, want 0 and false", asked, done)
	}
	if cid, held, asked, done := (CIDChain{m}).Lookup(5, m.cidIndex.asks); cid != 6 || !held || asked != m.cidIndex.asks || !done {
		t.Errorf("a search within its limit answered CID %d (held %v, asked %d, finished %v), want 6", cid, held, asked, done)
	}
}
