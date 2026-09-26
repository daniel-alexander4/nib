package fontcode

import (
	"strings"
	"testing"
	"unicode/utf16"
)

// The P07 phase-close review's findings in this package, each held by a test that puts a control under the ceiling
// beside the case over it and counts the work done — never a clock.

// TestTextMapExpandsWithinTheSpecificationsLimits — R3-5: a 32 KB CMap of one full-plane range with a 16 KB destination
// allocated 11.4 GB, and a range of 1 KB codes 65,536 1 KB keys. A range's codes are four bytes at most and its
// incrementing destination 512, and every range is charged its bytes as well as its codes.
func TestTextMapExpandsWithinTheSpecificationsLimits(t *testing.T) {
	dst := func(n int) string { return strings.Repeat("00", n-2) + "0041" }
	for _, c := range []struct {
		name string
		src  string
		want int
	}{
		{"a 512-byte destination is expanded", "1 beginbfrange <00> <FF> <" + dst(512) + "> endbfrange", 256},
		{"a 513-byte destination is not", "1 beginbfrange <00> <FF> <" + dst(513) + "> endbfrange", 0},
		{"four-byte codes are expanded", "1 beginbfrange <00000000> <000000FF> <0041> endbfrange", 256},
		{"five-byte codes are not", "1 beginbfrange <0000000000> <00000000FF> <0041> endbfrange", 0},
		// Each 10,000-code range of 512-byte destinations is ~12.8 MB of the 16 MB: the first fits, the second does not.
		{"one range the byte budget holds", "1 beginbfrange <0000> <270F> <" + dst(512) + "> endbfrange", 10000},
		{"a second range past the byte budget", "2 beginbfrange <0000> <270F> <" + dst(512) + "> <5000> <770F> <" + dst(512) + "> endbfrange", 10000},
		// An array destination is read from the file, one per code, so its length is paid in the CMap's own bytes.
		{"an array destination", "1 beginbfrange <00> <02> [<0041> <0042> <0043>] endbfrange", 3},
	} {
		if got := len(TextMap([]byte(c.src))); got != c.want {
			t.Errorf("%s: %d codes expanded, want %d", c.name, got, c.want)
		}
	}
}

// fullValue is `ToUnicodeInterval.toUnicode` written into the destination's whole width — the reading `value` compacts.
func fullValue(r uniRange, code int64) string {
	u := make([]byte, r.length)
	n := code - r.begin + r.start
	for i := len(u) - 1; i >= 0; i-- {
		u[i] = byte(n)
		n >>= 8
	}
	for i := 0; i+1 < len(u); i++ {
		if (u[i] == 0xFF && u[i+1] == 0xFE) || (u[i] == 0xFE && u[i+1] == 0xFF) {
			return "\uFFFE"
		}
	}
	if u[0] == 0 {
		return string(rune(u[len(u)-1]))
	}
	return javaUTF16BE(u)
}

// TestARangeValueIsReadAtABoundedWidth — R3-6: a 1 MB destination was written out per lookup (1.3 s per thousand,
// measured). The compacted reading must agree with the full-width one on everything 7.21.7 reads — null, U+0000,
// U+FEFF, U+FFFE — and differ, if at all, only by U+FFFF units; up to ten bytes it is the full reading exactly.
func TestARangeValueIsReadAtABoundedWidth(t *testing.T) {
	norm := func(s string) string { return strings.ReplaceAll(s, "\uFFFF", "") }
	starts := []int64{0, 0x41, 0xFEFF, 0xFFFE, 0x7FFFFFFFFFFFFFF0, -2, -0x100, -1 << 40}
	for length := 1; length <= 24; length++ {
		for _, start := range starts {
			for _, code := range []int64{0, 1, 2, 255} {
				r := uniRange{begin: 0, end: 255, start: start, length: length}
				got, want := r.value(code), fullValue(r, code)
				if length <= 10 && got != want {
					t.Errorf("length %d start %#x code %d: %+q, want %+q exactly", length, start, code, got, want)
				}
				if norm(got) != norm(want) || strings.ContainsAny(got, "\u0000\uFEFF\uFFFE") != strings.ContainsAny(want, "\u0000\uFEFF\uFFFE") {
					t.Errorf("length %d start %#x code %d: %+q reads differently from %+q", length, start, code, got, want)
				}
			}
		}
	}
	// The stimulus: a megabyte destination whose fill is FF, so the full reading is half a million characters.
	r := uniRange{begin: 0, end: 255, start: -0x100, length: 1 << 20}
	if full := len(utf16.Encode([]rune(fullValue(r, 0)))); full < 1<<19 {
		t.Fatalf("setup: the full reading is %d units, not the half million the case needs", full)
	}
	if got := len(utf16.Encode([]rune(r.value(0)))); got > 6 {
		t.Errorf("a 1 MB destination read as %d UTF-16 units, want at most 6", got)
	}
}

// TestAUsedToUnicodeIsLinkedNotCopied — R2-2: `With` copied every entry of the used CMap into the using one, per font
// and per hop. The link answers as the copy did — the deepest entry wins, ranges after entries, the used CMap last —
// and the copy shares the receiver's own entries rather than duplicating them.
func TestAUsedToUnicodeIsLinkedNotCopied(t *testing.T) {
	deep := parseTU([]byte("2 beginbfchar <41> <0058> <43> <0059> endbfchar 1 beginbfrange <60> <61> <0070> endbfrange"))
	mid := parseTU([]byte("2 beginbfchar <41> <0000> <42> <0042> endbfchar")).With(deep)
	top := parseTU([]byte("2 beginbfchar <41> <0041> <44> <0044> endbfchar 1 beginbfrange <60> <60> <0030> endbfrange"))
	linked := top.With(mid)
	for code, want := range map[int]string{0x41: "X", 0x42: "B", 0x43: "Y", 0x44: "D", 0x60: "0", 0x61: "q"} {
		if got, ok, known := linked.Lookup(code); !ok || !known || got != want {
			t.Errorf("code %#x = %q (%v, %v), want %q", code, got, ok, known, want)
		}
	}
	if got, ok, _ := top.Lookup(0x41); !ok || got != "A" {
		t.Errorf("With wrote through the receiver: its code 0x41 now reads %q", got)
	}
	top.chars[0x7F] = "shared"
	if got, _, _ := linked.Lookup(0x7F); got != "shared" {
		t.Error("the linked CMap holds a copy of the receiver's entries rather than sharing them")
	}
	// A chain past maxUseDepth — never built by the checker, which refuses one — is unknown, not a hang.
	c := parseTU(nil)
	for i := 0; i < maxUseDepth+1; i++ {
		c = parseTU(nil).With(c)
	}
	if _, _, known := c.Lookup(1); known {
		t.Error("a chain past the depth ceiling answered")
	}
}

// TestACloneKeepsItsShortestLength — R3-7: the checker merged a font's CMaps into an EMPTY Codespace, whose shortest
// length is unset, so `<50 41>` under `<00>-<3F>` and `<8000>-<FFFF>` refused where veraPDF reads two codes 0.
func TestACloneKeepsItsShortestLength(t *testing.T) {
	own := ParseCodespace([]byte("2 begincodespacerange <00> <3F> <8000> <FFFF> endcodespacerange"))
	if got := codes(own, []byte{0x50, 0x41}); got != "50=0 41=0" {
		t.Fatalf("setup: the parsed codespace cuts %s", got)
	}
	c := own.Clone()
	c.Merge(ParseCodespace([]byte("1 begincodespacerange <C0> <C0> endcodespacerange")))
	if got := codes(c, []byte{0x50, 0x41, 0xC0}); got != "50=0 41=0 C0=192" {
		t.Errorf("a clone merged into cuts %s", got)
	}
	if got := codes(own, []byte{0xC0}); got == "C0=192" {
		t.Error("merging into the clone wrote through to the original")
	}
}

// TestStringEmptyIsStringsOwnAnswer — R2-8: the checker asks whether a string shows anything without decoding it, and
// the answer must be String's, escape for escape.
func TestStringEmptyIsStringsOwnAnswer(t *testing.T) {
	for _, raw := range []string{"()", "(a)", "(\\\n)", "(\\\r\n)", "(\\\r)", "(\\\n\\\r\n)", "(\\\na)", "(\\)", "(\\", "(\r)",
		"(\\101)", "(\\q)", "<>", "< >", "<0>", "< 4 1 >", "<zz>", "<zz0>", "", "x", "(", "(\\\\)"} {
		if got, want := StringEmpty([]byte(raw)), len(String([]byte(raw))) == 0; got != want {
			t.Errorf("%q: StringEmpty %v, String %q", raw, got, String([]byte(raw)))
		}
	}
}
