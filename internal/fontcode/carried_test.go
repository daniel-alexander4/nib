package fontcode

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"nib/internal/cmapres"
)

// veraCMaps is veraPDF's own copy of Adobe's CMap resources (`font/cmap/<name>` in its CLI jar), or nil where
// veraPDF is not installed.
func veraCMaps(t *testing.T) map[string][]byte {
	t.Helper()
	home, _ := os.UserHomeDir()
	jars, _ := filepath.Glob(filepath.Join(home, "verapdf", "bin", "cli-*.jar"))
	if len(jars) == 0 {
		t.Log("NOTE (not a pass): veraPDF is absent, so the carried CMaps are not compared with its own in this run")
		return nil
	}
	z, err := zip.OpenReader(jars[0])
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	out := map[string][]byte{}
	for _, f := range z.File {
		if name, ok := strings.CutPrefix(f.Name, "font/cmap/"); ok && name != "" {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			if out[name], err = io.ReadAll(r); err != nil {
				t.Fatal(err)
			}
			r.Close()
		}
	}
	return out
}

var (
	codespaceListRE = regexp.MustCompile(`(?s)begincodespacerange(.*?)endcodespacerange`)
	hexRE           = regexp.MustCompile(`<([0-9A-Fa-f]*)>`)
	useRE           = regexp.MustCompile(`/(\S+)\s+usecmap`)
)

// declared is what a CMap program declares outside its mapping lists: its codespace ranges in order, the CMaps it
// uses, and whether a `usecmap` stands before the first list.
func declared(prog []byte) (ranges []string, uses []string) {
	for _, l := range codespaceListRE.FindAllSubmatch(prog, -1) {
		for _, h := range hexRE.FindAllSubmatch(l[1], -1) {
			ranges = append(ranges, strings.ToLower(string(h[1])))
		}
	}
	for _, u := range useRE.FindAllSubmatch(prog, -1) {
		uses = append(uses, string(u[1]))
	}
	return ranges, uses
}

// TestEveryCarriedCMapReadsAsVeraPDFsOwnDoes is ADR-117's ground: the tables nib carries are pdf.js's packing of
// Adobe's CMaps, veraPDF answers from ITS copy of them, and the two are different files of possibly different
// versions. So each carried CMap is read beside veraPDF's file of the same name, by the same reader, and must answer
// alike everywhere: the same codespace ranges in the same order and the same CMap used; the same CID, held or not,
// at every code where either reading's mappings begin or end (a mapping is linear between two such codes); and for
// a UCS2 CMap the same text for every CID of two bytes.
func TestEveryCarriedCMapReadsAsVeraPDFsOwnDoes(t *testing.T) {
	vera := veraCMaps(t)
	if vera == nil {
		return
	}
	names := cmapres.Names()
	if len(names) != 53 {
		t.Fatalf("%d CMaps are carried, want 49 predefined CMaps and the four UCS2 CMaps", len(names))
	}
	for _, name := range names {
		theirs, ok := vera[name]
		if !ok {
			t.Errorf("%s: veraPDF carries no CMap of that name", name)
			continue
		}
		ours, err := cmapres.Program(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		wr, wu := declared(theirs)
		gr, gu := declared(ours)
		if !reflect.DeepEqual(wr, gr) || !reflect.DeepEqual(wu, gu) {
			t.Errorf("%s: declares codespace %v using %v, veraPDF's file %v using %v", name, gr, gu, wr, wu)
			continue
		}
		if isUCS2Name(name) {
			budget := maxUCS2Blocks
			a := ParseToUnicode(theirs, &budget)
			b := UCS2(strings.Split(name, "-")[0], strings.Split(name, "-")[1])
			if b == nil || a.Malformed || a.Truncated {
				t.Errorf("%s: not read whole (ours %v, veraPDF's malformed %v truncated %v)", name, b != nil, a.Malformed, a.Truncated)
				continue
			}
			// What 7.21.7 reads of a CID: no text, text holding one of the three code points, or any other text.
			judged := func(text string, mapped bool) int {
				switch {
				case !mapped:
					return 0
				case strings.ContainsRune(text, 0) || strings.ContainsRune(text, 0xFFFE) || strings.ContainsRune(text, 0xFEFF):
					return 1
				}
				return 2
			}
			mapped, otherText := 0, 0
			var gaps [][2]int
			for cid := 0; cid <= 0xFFFF; cid++ {
				at, am, _ := a.Lookup(cid)
				bt, bm, _ := b.tu.Lookup(cid)
				if am {
					mapped++
				}
				if at != bt {
					otherText++
				}
				if judged(at, am) == judged(bt, bm) {
					continue
				}
				if n := len(gaps); n > 0 && gaps[n-1][1] == cid-1 {
					gaps[n-1][1] = cid
				} else {
					gaps = append(gaps, [2]int{cid, cid})
				}
			}
			if !reflect.DeepEqual(gaps, ucs2Gaps[name]) {
				t.Errorf("%s: the CIDs 7.21.7 would judge differently by veraPDF's file are %v, and ucs2Gaps says %v", name, gaps, ucs2Gaps[name])
			}
			for _, g := range gaps {
				for _, cid := range []int{g[0], g[1]} {
					if _, _, known := b.Lookup(cid); known {
						t.Errorf("%s: CID %d is in a gap and Lookup answers it as known", name, cid)
					}
				}
				if _, _, known := b.Lookup(g[0] - 1); !known {
					t.Errorf("%s: CID %d is outside the gap and Lookup refuses it", name, g[0]-1)
				}
			}
			if mapped < 8000 {
				t.Errorf("%s: only %d CIDs have text — the comparison compared nothing", name, mapped)
			}
			t.Logf("%s: %d CIDs with text in veraPDF's file, %d given other text by nib's, %d judged differently", name, mapped, otherText, len(gaps))
			continue
		}
		a := ParseCIDMap(theirs)
		_, b, ok := Predefined(name)
		if !ok || a.Malformed {
			t.Errorf("%s: not read whole (ours %v, veraPDF's malformed %v)", name, ok, a.Malformed)
			continue
		}
		if a.Entries() < 50 {
			t.Errorf("%s: veraPDF's file reads as %d mappings — the comparison compared nothing", name, a.Entries())
		}
		at := map[int]bool{}
		for _, m := range []*CIDMap{a, b} {
			for _, list := range [][]cidMapping{m.cids, m.notdefs} {
				for _, e := range list {
					at[e.lo-1], at[e.lo], at[e.hi], at[e.hi+1] = true, true, true, true
				}
			}
		}
		for code := range at {
			ac, ah, _, _ := CIDChain{a}.Lookup(code, 1<<30)
			bc, bh, _, _ := CIDChain{b}.Lookup(code, 1<<30)
			if ac != bc || ah != bh {
				t.Errorf("%s: code %#x is CID %d (held %v), veraPDF's file %d (held %v)", name, code, bc, bh, ac, ah)
				break
			}
		}
	}
}

// TestACarriedCMapIsReadOnceAndSharedUnwritten: the carried tables are held for the process, so a merge into one
// document's codespace must leave the shared one as it was read — it is read closed (`predefined`), and `Merge` and
// `Clone` write the closing only where it changes something. The second half bites under `-race` only: two documents
// merging the same carried codespace at once.
func TestACarriedCMapIsReadOnceAndSharedUnwritten(t *testing.T) {
	cs, cids, ok := Predefined("90ms-RKSJ-H")
	if !ok {
		t.Fatal("90ms-RKSJ-H is not carried")
	}
	again, cidsAgain, _ := Predefined("90ms-RKSJ-H")
	if cs != again || cids != cidsAgain {
		t.Error("a second ask read the CMap again")
	}
	before := *cs
	own := ParseCodespace([]byte("1 begincodespacerange <00> <80> endcodespacerange"))
	own.Merge(cs)
	cs.Clone()
	if !reflect.DeepEqual(before, *cs) {
		t.Error("merging a carried codespace into another, or cloning it, wrote to it")
	}
	// Read closed, every one: a CMap no other uses is merged first by a document, and two may do it at once.
	for _, name := range cmapres.Names() {
		if c, _, ok := Predefined(name); ok && c.tailOwn {
			t.Errorf("%s is held with its last group open: the first merge of it would write to it", name)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c := ParseCodespace([]byte("/90ms-RKSJ-H usecmap"))
				c.Merge(cs)
				cs.Clone()
			}
		}()
	}
	wg.Wait()
	for _, name := range []string{"Identity-H", "Adobe-Japan1-UCS2", "NoSuch", "../x"} {
		if _, _, ok := Predefined(name); ok {
			t.Errorf("Predefined(%q) answered a CMap", name)
		}
	}
	if UCS2("Adobe", "KR") != nil || UCS2("Adobe", "Identity") != nil || UCS2("UniJIS", "UCS2-H") != nil {
		t.Error("UCS2 answered a CMap nib does not carry")
	}
}

// TestAnEmbeddedCMapUsingACarriedOneHoldsItsMappingsBehindItsOwn: `usecmap` naming a predefined CMap appends that
// CMap's cid mappings after the program's own and merges its codespace where the operator stands — and a program
// repeating the operator holds the table once.
func TestAnEmbeddedCMapUsingACarriedOneHoldsItsMappingsBehindItsOwn(t *testing.T) {
	_, h, _ := Predefined("90ms-RKSJ-H")
	prog := []byte("/90ms-RKSJ-H usecmap 1 begincidrange <41> <41> 7 endcidrange")
	m := ParseCIDMap(prog)
	if m.Entries() != h.Entries()+1 {
		t.Fatalf("holds %d mappings, want the used CMap's %d and its own one", m.Entries(), h.Entries())
	}
	if cid, held, _, _ := (CIDChain{m}).Lookup(0x41, 1<<20); cid != 7 || !held {
		t.Errorf("its own mapping of 41 answers CID %d, want 7: the used CMap's must sit behind it", cid)
	}
	want, _, _, _ := CIDChain{h}.Lookup(0x8140, 1<<20)
	if cid, held, _, _ := (CIDChain{m}).Lookup(0x8140, 1<<20); cid != want || !held || want == 0 {
		t.Errorf("8140 answers CID %d, want the used CMap's %d", cid, want)
	}
	twice := ParseCIDMap(append(append([]byte{}, prog...), " /90ms-RKSJ-H usecmap /90ms-RKSJ-H usecmap"...))
	if twice.Entries() != m.Entries() {
		t.Errorf("a repeated usecmap holds %d mappings, want %d", twice.Entries(), m.Entries())
	}
	cs := ParseCodespace(prog)
	if len(cs.UsesCMaps) != 0 {
		t.Errorf("the codespace left %v for its caller to resolve", cs.UsesCMaps)
	}
	var cut []int
	if !cs.Codes([]byte{0x41, 0x81, 0x40}, func(_ []byte, v int) bool { cut = append(cut, v); return true }) || !reflect.DeepEqual(cut, []int{0x41, 0x8140}) {
		t.Errorf("41 8140 is cut as %x", cut)
	}
}
