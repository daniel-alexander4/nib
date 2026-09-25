package fontcode

import "testing"

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
