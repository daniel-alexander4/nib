package cmapres

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEveryCarriedCMapIsTheVendoredFile: the tables here are COPIES — `go:embed` cannot reach `web/`, which is the
// root package's — so each must be, byte for byte, the file pdf.js is shipped with. A pdf.js update that changes a
// table fails here until the copy is refreshed (and `fontcode`'s comparison with veraPDF's own files is run again).
func TestEveryCarriedCMapIsTheVendoredFile(t *testing.T) {
	names := Names()
	if len(names) != 53 {
		t.Fatalf("%d CMaps are carried, want 53", len(names))
	}
	for _, name := range names {
		ours, ok := Packed(name)
		if !ok {
			t.Errorf("%s: listed and not readable", name)
			continue
		}
		vendored, err := os.ReadFile(filepath.Join("..", "..", "web", "vendor", "pdfjs", "cmaps", name+".bcmap"))
		if err != nil {
			t.Errorf("%s: pdf.js ships no such table: %v", name, err)
			continue
		}
		if !bytes.Equal(ours, vendored) {
			t.Errorf("%s: the carried copy (%d bytes) is not the vendored file (%d bytes)", name, len(ours), len(vendored))
		}
	}
}

// TestAPackedCMapIsWrittenOutAsItsProgram reads known entries of real tables: a header's writing mode and used CMap,
// codespace ranges of two lengths, a CID range, a notdef range (which pdf.js itself skips), a single CID, and a UCS2
// CMap's single and ranged text.
func TestAPackedCMapIsWrittenOutAsItsProgram(t *testing.T) {
	for _, c := range []struct {
		name string
		has  []string
	}{
		// Adobe's 90ms-RKSJ-H: four codespace ranges; space to right brace are CIDs 231 on; hiragana from 829F are CIDs 842 on.
		{"90ms-RKSJ-H", []string{"/WMode 0 def", "<00> <80>\n", "<8140> <9ffc>\n", "<a0> <df>\n", "<e040> <fcfc>\n",
			"<20> <7d> 231\n", "<829f> <82f1> 842\n", "1 beginnotdefrange\n<00> <1f> 231\n"}},
		// its vertical sibling declares no codespace: it uses the horizontal CMap and overrides some of its codes.
		{"90ms-RKSJ-V", []string{"begincmap\n/90ms-RKSJ-H usecmap\n", "/WMode 1 def", "begincidrange\n<8141> <8142> 7887\n", "<81a8> <81a8> 739\n", "<81ac> 8270\n<829f> 7918\n"}},
		{"ETenms-B5-V", []string{"/ETenms-B5-H usecmap", "begincidchar"}},
		// Adobe-Japan1-UCS2: CID 0 is U+FFFD, and CIDs 1 to 60 are the characters from the space on.
		{"Adobe-Japan1-UCS2", []string{"<0000> <ffff>\n", "17 beginbfchar\n<0000> <fffd>\n<003d> <00a5>\n", "beginbfrange\n<0001> <003c> <0020>\n"}},
	} {
		prog, err := Program(c.name)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		for _, want := range c.has {
			if !strings.Contains(string(prog), want) {
				t.Errorf("%s: its program does not hold %q", c.name, want)
			}
		}
		if c.name == "90ms-RKSJ-V" && strings.Contains(string(prog), "codespacerange") {
			t.Errorf("%s declares a codespace of its own", c.name)
		}
	}
	// Lists are counted, and never longer than a hundred — the count is what a reader takes entries by.
	for _, name := range Names() {
		prog, err := Program(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		lines := strings.Split(string(prog), "\n")
		lists := 0
		for i := 0; i < len(lines); i++ {
			n, kind, ok := strings.Cut(lines[i], " begin")
			if !ok {
				continue
			}
			lists++
			count, err := strconv.Atoi(n)
			if err != nil || count < 1 || count > 100 || i+count+1 >= len(lines) || lines[i+count+1] != "end"+kind {
				t.Errorf("%s: the %s list at line %d says %q entries, and that many lines on is not its end", name, kind, i+1, n)
				break
			}
			i += count + 1
		}
		if lists == 0 {
			t.Errorf("%s: its program holds no list", name)
		}
	}
}

// TestWhatIsNotCarriedIsRefused: a name with no table, and a name that is a path.
func TestWhatIsNotCarriedIsRefused(t *testing.T) {
	for _, name := range []string{"Adobe-KR-UCS2", "Identity-H", "UniJIS-UTF16-H", "", "../cmapres", "90ms-RKSJ-H.bcmap", "x/90ms-RKSJ-H"} {
		if Has(name) {
			t.Errorf("Has(%q)", name)
		}
		if _, err := Program(name); err == nil {
			t.Errorf("Program(%q) answered a program", name)
		}
	}
	if !Has("90ms-RKSJ-H") {
		t.Error("90ms-RKSJ-H is not carried")
	}
}

// TestATruncatedOrUnknownRecordIsAnError: every prefix of a real table either decodes or is refused — never a panic —
// and a table cut inside a record is refused.
func TestATruncatedOrUnknownRecordIsAnError(t *testing.T) {
	whole, _ := Packed("90ms-RKSJ-V")
	refused := 0
	for n := 0; n < len(whole); n++ {
		if _, err := decode(whole[:n]); err != nil {
			refused++
		}
	}
	if refused == 0 {
		t.Error("no truncation of 90ms-RKSJ-V was refused")
	}
	if _, err := decode([]byte{0, 6<<5 | 1, 1}); err == nil {
		t.Error("a record of kind 6 decoded")
	}
}
