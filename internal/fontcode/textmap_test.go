package fontcode

import "testing"

// TestTextMapReadsDestinationsAsTheViewerDoes — `/pending 730`, two divergences from pdf.js (`parseBfChar` and
// `readToUnicode` in the vendored worker), the reader nib's viewer is. `pdfops.codesFor` inverts TextMap, so a wrong
// entry here is a code typed for the wrong character AND the same wrong text read back, with nothing to flag it.
func TestTextMapReadsDestinationsAsTheViewerDoes(t *testing.T) {
	cmap := func(body string) map[string]string {
		return TextMap([]byte("/CIDInit /ProcSet findresource begin 12 dict begin begincmap " + body + " endcmap end end"))
	}
	// A NAME destination: pdf.js throws there and keeps what the block mapped before it. TextMap skipped the name and
	// paired what followed one place out of step, mapping 0x41 to "B".
	m := cmap("3 beginbfchar <40> <0040> <41> /A <42> <0042> endbfchar 1 beginbfchar <43> <0043> endbfchar")
	if got := m["\x40"]; got != "@" {
		t.Errorf("an entry before the name destination maps to %q, want %q", got, "@")
	}
	if got, ok := m["\x41"]; ok {
		t.Errorf("the code whose destination is a name maps to %q; pdf.js maps it to nothing", got)
	}
	if got, ok := m["\x42"]; ok {
		t.Errorf("an entry after the name destination in its block maps to %q; pdf.js stops the block there", got)
	}
	if got := m["\x43"]; got != "C" {
		t.Errorf("the NEXT block maps 0x43 to %q, want %q — a broken block is that block only", got, "C")
	}
	// A literal-string destination is a string to pdf.js, as a hex one is.
	if got := cmap(`1 beginbfchar <44> (\000D) endbfchar`)["\x44"]; got != "D" {
		t.Errorf("a literal destination maps to %q, want %q", got, "D")
	}
	// An odd-length destination: a zero byte in front, then UTF-16BE — one byte is its ISO-8859-1 character.
	if got := cmap("1 beginbfchar <45> <E9> endbfchar")["\x45"]; got != "é" {
		t.Errorf("a one-byte destination maps to %q, want %q", got, "é")
	}
	if got := cmap("1 beginbfchar <46> <0041E9> endbfchar")["\x46"]; got != "\u0000䇩" {
		t.Errorf("a three-byte destination maps to %q, want %q", got, "\u0000䇩")
	}
}
