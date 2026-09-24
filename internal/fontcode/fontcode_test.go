package fontcode

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAPDFStringDecodesEveryEscapeForm — the reader's half of what `contentstream` leaves as spans.
func TestAPDFStringDecodesEveryEscapeForm(t *testing.T) {
	for _, c := range []struct{ raw, want string }{
		{`(plain)`, "plain"},
		{`(a\(b\)c)`, "a(b)c"},
		{`(back\\slash)`, `back\slash`},
		{`(\101\102)`, "AB"},
		{`(\0053)`, "\x053"},
		{`(\7)`, "\x07"},
		{`(\n\r\t\b\f)`, "\n\r\t\b\f"},
		{"(x\\\ny)", "xy"},
		{"(x\\\r\ny)", "xy"},
		{"(a\r\nb)", "a\nb"},
		{`(\q)`, "q"},
		{`<48656C6C6F>`, "Hello"},
		{`<48 65 6c>`, "Hel"},
		{`<486>`, "H`"},
		{`<>`, ""},
		{`()`, ""},
	} {
		if got := string(String([]byte(c.raw))); got != c.want {
			t.Errorf("%s decodes to %q, want %q", c.raw, got, c.want)
		}
	}
}

// TestToUnicodeReadsCharsRangesAndSurrogates — both producers the corpus measured write `bfchar`
// only; the specification's two `bfrange` forms are driven here, where no generated document reaches.
func TestToUnicodeReadsCharsRangesAndSurrogates(t *testing.T) {
	cm := TextMap([]byte(`/CIDInit /ProcSet findresource begin
12 dict begin begincmap
1 begincodespacerange <0000> <FFFF> endcodespacerange
3 beginbfchar
<0001> <005A>
<0002> <00660069>
<0003> <D83DDE00>
endbfchar
2 beginbfrange
<0010> <0012> <0041>
<0020> <0021> [<0061> <0062>]
endbfrange
endcmap`))
	for code, want := range map[string]string{
		"\x00\x01": "Z", "\x00\x02": "fi", "\x00\x03": "\U0001F600",
		"\x00\x10": "A", "\x00\x11": "B", "\x00\x12": "C",
		"\x00\x20": "a", "\x00\x21": "b",
	} {
		if got, ok := cm[code]; !ok || got != want {
			t.Errorf("code % X maps to %q (present %v), want %q", code, got, ok, want)
		}
	}
	if _, ok := cm["\x00\x13"]; ok {
		t.Error("a code past the incrementing range's end was mapped")
	}
	huge := TextMap([]byte("1 beginbfrange <000000> <FFFFFF> <0041> endbfrange"))
	if len(huge) != 0 {
		t.Errorf("a range of 2^24 codes expanded to %d entries — one line in a document allocating without limit", len(huge))
	}
}

// TestACMapHasATotalExpansionBudget — `/pending 503`. Each range was bounded and their sum was not: a
// hundred overlapping full-plane ranges in 2.2 KB cost 5.1 s and 110 MB. Asserted on WHICH ranges
// expanded, not on a clock: range n maps code 0000 to the letter n, so the surviving value names the last
// range that was expanded.
func TestACMapHasATotalExpansionBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("100 beginbfrange\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "<0000> <FFFF> <%04X>\n", 0x41+i)
	}
	b.WriteString("endbfrange\n")
	got := TextMap([]byte(b.String()))["\x00\x00"]
	if got != "B" {
		t.Errorf("code 0000 maps to %q: ranges past the second full plane were still expanded (want %q, "+
			"the second range's value — %q would be the hundredth)", got, "B", string(rune(0x41+99)))
	}
	// The floor under the budget: ONE full plane is a real Identity-H font's whole ToUnicode.
	if plane := TextMap([]byte("1 beginbfrange <0000> <FFFF> <0041> endbfrange")); len(plane) != 1<<16 {
		t.Errorf("one full-plane range expanded to %d entries, want %d — the budget refuses a real font", len(plane), 1<<16)
	}
}

// TestToUnicodeIsReadTheWayVeraPDFReadsIt — the checker's reading, each row one place veraPDF-parser's
// `CMapParser`/`ToUnicodeInterval` differ from the specification's lenient one (read from source, 1.30.2).
func TestToUnicodeIsReadTheWayVeraPDFReadsIt(t *testing.T) {
	for _, c := range []struct {
		name, cmap string
		code       int
		want       string
		mapped     bool
	}{
		{"a bfchar destination", "1 beginbfchar <0001> <005A> endbfchar", 1, "Z", true},
		{"the key is the INTEGER, not the bytes", "1 beginbfchar <0041> <005A> endbfchar", 0x41, "Z", true},
		{"a <0000> destination is a character, not an absence", "1 beginbfchar <01> <0000> endbfchar", 1, "\x00", true},
		{"a one-byte destination is ISO-8859-1", "1 beginbfchar <01> <E9> endbfchar", 1, "é", true},
		{"a byte-order mark is kept", "1 beginbfchar <01> <FEFF0041> endbfchar", 1, "\uFEFFA", true},
		{"an odd trailing byte is U+FFFD", "1 beginbfchar <01> <004100> endbfchar", 1, "A\uFFFD", true},
		{"a name destination is its text", "1 beginbfchar <01> /space endbfchar", 1, "space", true},
		{"only the COUNT is read", "1 beginbfchar <01> <0041> <02> <0042> endbfchar", 2, "", false},
		{"a list with no count is not a list", "beginbfchar <01> <0041> endbfchar", 1, "", false},
		{"a range", "1 beginbfrange <0010> <0012> <0041> endbfrange", 0x12, "C", true},
		{"a range ends at its begin code's leading bytes", "1 beginbfrange <0000> <FFFF> <0041> endbfrange", 0x0100, "", false},
		{"...and holds the last byte's span", "1 beginbfrange <0000> <FFFF> <0041> endbfrange", 0x00FF, string(rune(0x41 + 0xFF)), true},
		{"a range whose first destination byte is zero reads its LAST byte", "1 beginbfrange <01> <01> <00660069> endbfrange", 1, "i", true},
		{"a range reaching FE FF answers U+FFFE", "1 beginbfrange <01> <02> <FEFE> endbfrange", 2, "\uFFFE", true},
		{"an array range", "1 beginbfrange <20> <21> [<0061> <0062>] endbfrange", 0x21, "b", true},
		{"a bfchar wins over a range", "1 beginbfrange <01> <02> <0041> endbfrange 1 beginbfchar <02> <005A> endbfchar", 2, "Z", true},
		{"the first range wins", "2 beginbfrange <01> <02> <0041> <01> <02> <0061> endbfrange", 2, "B", true},
		{"an empty range destination adds nothing", "1 beginbfrange <01> <02> <> endbfrange", 1, "", false},
		// Java `int` (the P07.S02 review, read from `CMapParser.java:245,309`): a four-byte code at or above
		// 0x80000000 is negative, so a bfchar key — cast the same way — maps it and a range never can.
		{"a GB18030 bfchar maps its negative code", "1 beginbfchar <81308131> <4E01> endbfchar", int(int32(-2127527631)), "丁", true},
		{"a GB18030 range maps nothing", "1 beginbfrange <81308130> <813081FF> <4E00> endbfrange", int(int32(-2127527631)), "", false},
		{"a negative count reads nothing", "-1 beginbfchar <01> <0041> endbfchar", 1, "", false},
		{"an empty array under an empty range reads cleanly", "1 beginbfrange <02> <01> [] endbfrange 1 beginbfchar <03> <0043> endbfchar", 3, "C", true},
	} {
		u := parseTU([]byte(c.cmap))
		if u.Malformed {
			t.Errorf("%s: parsed as malformed", c.name)
			continue
		}
		got, ok, known := u.Lookup(c.code)
		if !known || ok != c.mapped || got != c.want {
			t.Errorf("%s: code %#x = %q (mapped %v), want %q (mapped %v)", c.name, c.code, got, ok, c.want, c.mapped)
		}
	}
}

// TestAMalformedToUnicodeIsNotAReading — veraPDF discards a CMap it throws on (`CMapFactory.getCMap`), and
// nib's tokenizer is not its PostScript interpreter, so every shape that throws is reported, never read.
func TestAMalformedToUnicodeIsNotAReading(t *testing.T) {
	for name, cmap := range map[string]string{
		"a count past the entries":       "2 beginbfchar <01> <0041> endbfchar",
		"a destination that is a number": "1 beginbfchar <01> 65 endbfchar",
		"a short array":                  "1 beginbfrange <01> <03> [<0041> <0042>] endbfrange",
		"a long array":                   "1 beginbfrange <01> <02> [<0041> <0042> <0043>] endbfrange",
		"a code that is a literal":       "1 beginbfchar (A) <0041> endbfchar",
	} {
		u := parseTU([]byte(cmap))
		if !u.Malformed {
			t.Errorf("%s: read as a CMap", name)
		}
		if _, ok, known := u.Lookup(1); ok || known {
			t.Errorf("%s: a malformed CMap still answered for a code", name)
		}
	}
	// An empty range code is an exception veraPDF does not catch, and it panicked here (the P07.S02 re-review).
	for _, src := range []string{"1 beginbfrange <> <> <0041> endbfrange", "1 begincodespacerange <00> <FF> endcodespacerange 1 beginbfrange <> <> <0041> endbfrange"} {
		if u := parseTU([]byte(src)); !u.Malformed {
			t.Errorf("%q: an empty range code read as a CMap", src)
		}
		if c := ParseCodespace([]byte(src)); !c.Malformed {
			t.Errorf("%q: an empty range code read as a codespace", src)
		}
	}
	// An integer consumes the object after it, and a non-positive count accepts any begin keyword: none of these
	// opens the bfchar list (`CMapParser.processObject`, `processList`).
	for _, src := range []string{"9 1 beginbfchar <41> <0042> endbfchar", "0 beginfoo 1 beginbfchar <41> <0042> endbfchar",
		"0 begin 1 beginbfchar <41> <0042> endbfchar"} {
		if _, ok, _ := parseTU([]byte(src)).Lookup(0x41); ok {
			t.Errorf("%q mapped 0x41 — veraPDF reads no list there", src)
		}
	}
	if u := parseTU([]byte("/Foo usecmap 1 beginbfchar <01> <0041> endbfchar")); u.UseCMapName != "Foo" || u.Malformed {
		t.Errorf("a usecmap was not reported: %+v", u)
	}
}

// TestAUsedToUnicodeOverwritesAndAnswersMisses — `PDCMap.getCMapFile`'s merge of a dictionary `/UseCMap`: the used
// CMap's entries overwrite this one's, its ranges are consulted only on a miss, and a malformed one is not read.
func TestAUsedToUnicodeOverwritesAndAnswersMisses(t *testing.T) {
	own := parseTU([]byte("2 beginbfchar <41> <0041> <42> <0042> endbfchar"))
	own.Use(parseTU([]byte("1 beginbfchar <41> <0000> endbfchar 1 beginbfrange <50> <51> <0070> endbfrange")))
	for code, want := range map[int]string{0x41: "\x00", 0x42: "B", 0x51: "q"} {
		if got, ok, known := own.Lookup(code); !ok || !known || got != want {
			t.Errorf("code %#x = %q (%v, %v), want %q", code, got, ok, known, want)
		}
	}
	bad := parseTU([]byte("1 beginbfchar <41> <0041> endbfchar"))
	bad.Use(parseTU([]byte("2 beginbfchar <41> <0000> endbfchar")))
	if _, _, known := bad.Lookup(0x41); known {
		t.Error("a CMap using a malformed one still answered")
	}
}

// TestAMillionRangesAreIndexedNotWalked — the P07.S02 review measured ~2.25 ms per unmapped lookup over 2^20
// attacker-written ranges. The index answers from one block; asserted on the work, not a clock: a million lookups
// over a million ranges finish, and the first range covering a slot is the one that answers.
func TestAMillionRangesAreIndexedNotWalked(t *testing.T) {
	var b strings.Builder
	const n = 1 << 20
	fmt.Fprintf(&b, "%d beginbfrange\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "<%06X> <%06X> <%04X>\n", (i%4096)<<8, (i%4096)<<8|0xFF, 0x41+i%26)
	}
	b.WriteString("endbfrange\n")
	u := parseTU([]byte(b.String()))
	if u.Malformed || u.Truncated {
		t.Fatalf("setup: malformed %v, truncated %v", u.Malformed, u.Truncated)
	}
	if got, ok, _ := u.Lookup(0x0101 << 8); !ok || got != string(rune(0x41+0x101%26)) {
		t.Fatalf("the first range over block 0x101 did not answer: %q %v", got, ok)
	}
	for i := 0; i < n; i++ {
		if _, ok, _ := u.Lookup(0x1000000 + i); ok {
			t.Fatal("a code outside every range mapped")
		}
	}
	// Past the block ceiling, a miss is unknown rather than unmapped.
	var w strings.Builder
	fmt.Fprintf(&w, "%d beginbfrange\n", maxRangeBlocks+1)
	for i := 0; i <= maxRangeBlocks; i++ {
		fmt.Fprintf(&w, "<%06X> <%06X> <0041>\n", i<<8, i<<8)
	}
	w.WriteString("endbfrange\n")
	if tr := parseTU([]byte(w.String())); !tr.Truncated {
		t.Error("a CMap past the block ceiling was not marked truncated")
	} else if _, _, known := tr.Lookup(maxRangeBlocks<<8 | 1); known {
		t.Error("a miss past the ceiling read as unmapped")
	}
}

func codes(c *Codespace, b []byte) string {
	var out []string
	if !c.Codes(b, func(code []byte, v int) bool {
		out = append(out, fmt.Sprintf("%X=%d", code, v))
		return true
	}) {
		out = append(out, "UNREAD")
	}
	return strings.Join(out, " ")
}

// TestCodesAreCutTheWayVeraPDFCutsThem — `CMap.getCodeFromStream`, row by row.
func TestCodesAreCutTheWayVeraPDFCutsThem(t *testing.T) {
	mixed := ParseCodespace([]byte("2 begincodespacerange <00> <80> <8140> <9FFC> endcodespacerange"))
	for _, c := range []struct {
		name string
		cs   *Codespace
		in   []byte
		want string
	}{
		{"Identity, two bytes each", Identity(), []byte{0, 1, 0, 2}, "0001=1 0002=2"},
		{"a string ending inside a code is completed with 0xFF", Identity(), []byte{0, 0x41, 0}, "0041=65 00FF=255"},
		{"one- and two-byte ranges", mixed, []byte{0x41, 0x81, 0x40}, "41=65 8140=33088"},
		// 0xA0 matches no range at its first byte: skip (shortest - 0 - 1) = 0 more bytes, code 0.
		{"a byte no range admits is code 0", mixed, []byte{0xA0, 0x41}, "A0=0 41=65"},
		// 0x81 partially matches the two-byte range, 0x20 then fails it: skip (2 - 1 - 1) = 0, code 0.
		{"a partial match that fails is code 0", mixed, []byte{0x81, 0x20, 0x41}, "8120=0 41=65"},
	} {
		if got := codes(c.cs, c.in); got != c.want {
			t.Errorf("%s: % X → %s, want %s", c.name, c.in, got, c.want)
		}
	}
	if mixed.Invalid || mixed.Malformed {
		t.Errorf("a valid codespace was flagged: %+v", mixed)
	}
	overlap := ParseCodespace([]byte("2 begincodespacerange <0000> <FFFF> <00> <FF> endcodespacerange"))
	if got := codes(overlap, []byte{0x41, 0x42}); got != "4142=16706" {
		t.Errorf("an overlapping range was not dropped: %s", got)
	}
	if bad := ParseCodespace([]byte("1 begincodespacerange <80> <00> endcodespacerange")); !bad.Invalid {
		t.Error("a range whose begin is above its end was not flagged")
	}
	if use := ParseCodespace([]byte("/GB-EUC-H usecmap")); use.UsesCMap != "GB-EUC-H" {
		t.Errorf("usecmap read as %q", use.UsesCMap)
	}
	// The P07.S02 review: Identity merged AT the operator drops the program's later one-byte range, so `41 42` is
	// ONE code; merged after the parse it was two.
	if got := codes(ParseCodespace([]byte("/Identity-H usecmap 1 begincodespacerange <00> <7F> endcodespacerange")), []byte{0x41, 0x42}); got != "4142=16706" {
		t.Errorf("usecmap before the ranges: %s", got)
	}
	// A merge does not move the shortest length: own <8140>-<9FFC> (shortest 2) with a used <00>-<7F> skips the byte
	// after a no-match as veraPDF does.
	own := ParseCodespace([]byte("1 begincodespacerange <8140> <9FFC> endcodespacerange"))
	own.Merge(ParseCodespace([]byte("1 begincodespacerange <00> <7F> endcodespacerange")))
	if got := codes(own, []byte{0xA0, 0x41, 0x42}); got != "A0=0 42=66" {
		t.Errorf("after a merge: %s", got)
	}
	// Where veraPDF's reader throws, nib does not guess: the code FFFFFFFF, and a no-match with no range of its own.
	if got := codes(ParseCodespace([]byte("1 begincodespacerange <00000000> <FFFFFFFF> endcodespacerange")), []byte{0xFF, 0xFF, 0xFF, 0xFF}); got != "UNREAD" {
		t.Errorf("FFFFFFFF: %s", got)
	}
	none := &Codespace{}
	none.Merge(ParseCodespace([]byte("1 begincodespacerange <8140> <9FFC> endcodespacerange")))
	if got := codes(none, []byte{0x20}); got != "UNREAD" {
		t.Errorf("no range of its own: %s", got)
	}
}

// TestANameIsDecoded — `#xx` escapes, the one spelling rule a font resource name has.
func TestANameIsDecoded(t *testing.T) {
	if got := Name([]byte("F#31#2")); got != "F1#2" {
		t.Errorf("got %q", got)
	}
}

// TestTheReadersRouteThroughThisDoor is ADR-009's half: the checker and the text reader both read shown bytes
// and /ToUnicode CMaps, and a second copy of either is how the two came apart before (`/pending 657`).
//
// It asserts ROUTING over every package under internal/: no function outside this one holds a string literal
// naming a CMap list (`bfchar`, `bfrange`, `codespacerange`, `cidrange`, in any spelling), and none declares one of
// the decoders this package replaced. **One reader is exempt, by name, with its reason**: pdfops' stamp writer
// mirrors PDFCPU's strict ToUnicode parser to predict what pdfcpu will refuse — a third reading, of a different
// program, which ADR-052 names. A new exemption is added here, with its reason, or the reader routes through here.
// The first cut scanned two packages for three exact spellings and missed that reader entirely (the P07.S02 review).
func TestTheReadersRouteThroughThisDoor(t *testing.T) {
	exempt := map[string]string{
		"pdfops.pdfcpuToUnicode": "mirrors pdfcpu's usedGIDsFromCMap, to refuse what pdfcpu would refuse",
		"pdfops.recountBFChar":   "repairs pdfcpu's own bfchar counts in the shape pdfcpu writes",
	}
	// Case-sensitive, as a CMap program spells them — `/UseCMap`, the dictionary key, is not the `usecmap` operator.
	listWords := []string{"bfchar", "bfrange", "codespacerange", "cidrange", "cidchar", "notdefrange", "notdefchar", "usecmap"}
	bannedFuncs := map[string]bool{"decodePDFString": true, "decodeHexString": true, "hexNibble": true,
		"parseToUnicode": true, "matchingClose": true}
	var dirs []string
	for _, pattern := range []string{"../*", "../../cmd/*"} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, m...)
	}
	scanned, hits := 0, map[string]bool{}
	for _, dir := range dirs {
		if filepath.Base(dir) == "fontcode" {
			continue
		}
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
		if err != nil {
			continue
		}
		for pkgName, pkg := range pkgs {
			for _, f := range pkg.Files {
				scanned++
				// EVERY literal in the file, a package-level constant's included — the first cut read function bodies
				// only, and `const kw = "beginbfrange"` read from a function stayed green (the P07.S02 re-review). Each
				// top-level declaration is its own owner, so a constant inside a function belongs to that function.
				for _, decl := range f.Decls {
					owner := pkgName + ".(package level)"
					if fn, ok := decl.(*ast.FuncDecl); ok {
						owner = pkgName + "." + fn.Name.Name
						if bannedFuncs[fn.Name.Name] {
							t.Errorf("%s declares %s — string and CMap decoding is internal/fontcode's (ADR-009, ADR-052)", fset.Position(fn.Pos()), owner)
						}
					}
					ast.Inspect(decl, func(n ast.Node) bool {
						lit, ok := n.(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							return true
						}
						for _, w := range listWords {
							if strings.Contains(lit.Value, w) {
								if _, ok := exempt[owner]; ok {
									hits[owner] = true
									return true
								}
								t.Errorf("%s: %s names a CMap list (%s) — read CMaps through internal/fontcode, or name the "+
									"exemption and its reason here and in ADR-052", fset.Position(lit.Pos()), owner, lit.Value)
							}
						}
						return true
					})
				}
			}
		}
	}
	if scanned < 100 {
		t.Fatalf("setup: only %d Go files scanned under internal/ and cmd/ — the guard is reading the wrong place", scanned)
	}
	// Each exemption must still exist and still be the reader it was granted for, or it exempts nothing.
	for id := range exempt {
		if !hits[id] {
			t.Errorf("exemption %s matched no CMap literal — remove it", id)
		}
	}
	for _, dir := range []string{"../pdfops", "../uacheck"} {
		imports := false
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if b, err := os.ReadFile(f); err == nil && !strings.HasSuffix(f, "_test.go") && strings.Contains(string(b), `"nib/internal/fontcode"`) {
				imports = true
			}
		}
		if !imports {
			t.Errorf("%s does not import internal/fontcode, so it reads shown bytes some other way", dir)
		}
	}
}

// parseTU is ParseToUnicode with a budget of its own, as a test reading one CMap wants.
func parseTU(src []byte) *ToUnicode {
	budget := maxRangeBlocks
	return ParseToUnicode(src, &budget)
}
