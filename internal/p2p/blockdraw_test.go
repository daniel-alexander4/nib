package p2p

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// The signature block's drawing door (ADR-158, /pending 367). The rows are S-C's, in
// `instruments/three-decisions-2026-10-12.md`: C3 a face for every character and real shaping, C4
// the refusal, C5 the wrap, C6 the height, C7 one picture for one set of lines, C8 the cost.

// blockScriptRows is the table every test here walks: one realistic line per script the shipped
// faces cover, and a mixed-direction one.
var blockScriptRows = []struct{ name, text string }{
	{"latin", "Signer: Margaret O'Hanlon-Šimůnek"},
	{"cyrillic", "Signer: Иван Сергеевич Петров"},
	{"greek", "Signer: Γεώργιος Παπαδόπουλος"},
	{"cjk", "Signer: 司馬 遼太郎(東京国際法律事務所)"},
	{"hangul", "Signer: 남궁민수 (서울국제법률사무소)"},
	{"hebrew", "Signer: אברהם בן-דוד רוזנצווייג"},
	{"arabic", "Signer: عبد الرحمن بن محمد آل سعود"},
	{"devanagari", "Signer: श्रीमती लक्ष्मीनारायणन अय्यर"},
	{"thai", "Signer: นางสาวกัญญาณัฐ ศรีสุวรรณวัฒนกุล"},
	{"mixed", "Intent: We agree — نوافق — אנו מסכימים — 同意します (12 Oct 2026)."},
}

// shapeForTest is `shapeBlockLine` under the door's own lock.
func shapeForTest(t *testing.T, s string) shapedLine {
	t.Helper()
	blockFaces.mu.Lock()
	defer blockFaces.mu.Unlock()
	if err := loadBlockFaces(); err != nil {
		t.Fatalf("the block's faces did not load: %v", err)
	}
	return shapeBlockLine(s)
}

// widthForTest measures a line from its shaped glyphs — NOT through `blockLineWidth`, which is the
// function the wrap uses: a test that measured with the wrap's own ruler would agree with any ruler.
func widthForTest(t *testing.T, s string) float64 {
	t.Helper()
	ln := shapeForTest(t, s)
	var adv float64
	for _, r := range ln.runs {
		for _, g := range r.Glyphs {
			adv += float64(g.Advance)
		}
	}
	return adv / 64 / blockShapeSize * blockTextPt
}

// C3 — every character of every row is given a face that has it, and the line is SHAPED: letters
// join, conjuncts form, marks are placed, right-to-left runs are laid out right to left.
func TestEveryScriptIsDrawnInAFaceThatHasItAndIsShaped(t *testing.T) {
	for _, row := range blockScriptRows {
		ln := shapeForTest(t, row.text)
		if len(ln.missing) != 0 {
			t.Errorf("%s: %d character(s) have no face or drew as the missing-glyph box: %q", row.name, len(ln.missing), string(ln.missing))
		}
		glyphs := 0
		for _, r := range ln.runs {
			for _, g := range r.Glyphs {
				glyphs++
				if g.GlyphID == 0 {
					t.Errorf("%s: a glyph is the font's missing-glyph box (cluster %d)", row.name, g.ClusterIndex)
				}
			}
		}
		if glyphs == 0 || ln.advance <= 0 {
			t.Errorf("%s: shaped to %d glyph(s) and an advance of %d — nothing would be drawn", row.name, glyphs, ln.advance)
		}
	}

	// The zero above is not structural: a character no shipped face has MUST count, exactly once.
	if ln := shapeForTest(t, "Signer: AB"); len(ln.missing) != 1 || ln.missing[0] != 0xE000 {
		t.Fatalf("a private-use character repeated twice counted as %q, want exactly one U+E000 — the "+
			"count above cannot see a missing face", string(ln.missing))
	}

	// Arabic JOINS: at least one letter of a word is drawn with a glyph other than its isolated one.
	ar := shapeForTest(t, "محمد")
	nominal := map[uint32]bool{}
	blockFaces.mu.Lock()
	for _, r := range "محمد" {
		if bf := faceFor(r); bf != nil {
			g, _ := bf.face.NominalGlyph(r)
			nominal[uint32(g)] = true
		}
	}
	blockFaces.mu.Unlock()
	joined := false
	for _, r := range ar.runs {
		for _, g := range r.Glyphs {
			if !nominal[uint32(g.GlyphID)] {
				joined = true
			}
		}
	}
	if !joined {
		t.Error("an Arabic word was drawn with each letter's isolated glyph: the letters are not joined, " +
			"which is the text set one rune at a time and not what the signer wrote")
	}

	// Devanagari CONJOINS: क + virama + ष is one conjunct, fewer glyphs than characters.
	if n := glyphCount(shapeForTest(t, "क्ष")); n >= 3 {
		t.Errorf("the conjunct क्ष shaped to %d glyphs for 3 characters: the virama was drawn rather than applied", n)
	}

	// Thai marks are PLACED: a tone mark over an upper vowel is lifted, not drawn on top of it.
	lifted := false
	for _, r := range shapeForTest(t, "ที่").runs {
		for _, g := range r.Glyphs {
			if g.XOffset != 0 || g.YOffset != 0 {
				lifted = true
			}
		}
	}
	if !lifted {
		t.Error("no glyph of ที่ carries an offset: the tone mark sits on its vowel instead of above it")
	}

	// Right to left: inside a Hebrew run the leftmost glyph is the LAST letter…
	he := shapeForTest(t, "x אבג y")
	found := false
	for _, r := range he.runs {
		if len(r.Glyphs) >= 3 && r.Glyphs[0].ClusterIndex > r.Glyphs[len(r.Glyphs)-1].ClusterIndex {
			found = true
		}
	}
	if !found {
		t.Error("the Hebrew run of “x אבג y” is laid out first letter leftmost: it reads backwards")
	}
	// …and two right-to-left runs side by side are in right-to-left ORDER: the Arabic word written
	// first is drawn to the right of the Hebrew word written second.
	two := shapeForTest(t, "محمد דוד")
	if len(two.runs) < 2 {
		t.Fatalf("setup: “محمد דוד” shaped to %d run(s), want the Arabic and the Hebrew apart", len(two.runs))
	}
	if first, last := two.runs[0].Runes.Offset, two.runs[len(two.runs)-1].Runes.Offset; first <= last {
		t.Errorf("the run drawn leftmost starts at character %d and the rightmost at %d: two right-to-left "+
			"words are in left-to-right order, so the line reads second word first", first, last)
	}
}

func glyphCount(ln shapedLine) int {
	n := 0
	for _, r := range ln.runs {
		n += len(r.Glyphs)
	}
	return n
}

// C4 — a character no shipped face covers is refused BY NAME before anything is drawn or signed.
func TestACharacterNoFaceHasIsRefusedByNameAndNeverDrawn(t *testing.T) {
	att := Attestation{Signer: "Alice", AcceptedPeer: strings.Repeat("ab", 32), AcceptedPeerLabel: "Bob",
		Intent: "I agree  to this", When: time.Date(2026, 10, 12, 14, 30, 0, 0, time.UTC)}
	var u *UndrawableError
	err := BlockDrawable(att)
	if !errors.As(err, &u) {
		t.Fatalf("a recital holding a character no face has was answered %v, want an UndrawableError", err)
	}
	if u.Rune != 0xE000 || u.Field != "the recital" {
		t.Errorf("the refusal names %q in %q, want U+E000 in the recital", string(u.Rune), u.Field)
	}
	if !strings.Contains(err.Error(), "U+E000") || !strings.Contains(err.Error(), "the recital") {
		t.Errorf("the sentence does not say which character or where: %q", err.Error())
	}
	// Each field is asked, and the one at fault is the one named.
	for field, a := range map[string]Attestation{
		"the signer's name": {Signer: "Alice", Intent: "x", AcceptedPeerLabel: "Bob"},
		"the capacity":      {Signer: "Alice", Capacity: "Director", Intent: "x", RosterSize: 2, Position: 1},
		"the peer's label":  {Signer: "Alice", Intent: "x", AcceptedPeerLabel: "Bb"},
	} {
		if e := BlockDrawable(a); !errors.As(e, &u) || u.Field != field {
			t.Errorf("a bad character in %s was answered %v", field, e)
		}
	}
	// A line break and a bidirectional override are not characters a block can show either.
	for _, bad := range []string{"line one\nline two", "safe ‮ desrever"} {
		if e := TextDrawable(bad); !errors.As(e, &u) {
			t.Errorf("%q was admitted (%v): the block would not show what the /Reason signs", bad, e)
		}
	}
	// The control: ordinary text in every script is admitted.
	for _, row := range blockScriptRows {
		if e := TextDrawable(row.text); e != nil {
			t.Errorf("%s was refused: %v", row.name, e)
		}
	}

	// Nothing is drawn, and nothing is signed.
	if img, e := att.BlockImage(); e == nil || img != nil {
		t.Errorf("BlockImage drew %d bytes for a block it cannot draw (err %v)", len(img), e)
	}
	if img, e := drawBlock([]string{"Nib co-signing attestation", "Intent: I agree "}); !errors.As(e, &u) || img != nil {
		t.Errorf("the drawing itself answered %v with %d bytes: a missing glyph must be refused where it "+
			"is drawn, not only where it is asked about", e, len(img))
	}
	a := l3Identity(t, "A")
	doc := l3Prepared(t)
	place, perr := NextPlacement(doc)
	if perr != nil {
		t.Fatal(perr)
	}
	if out, e := Contribute(doc, []byte(a.cert), []byte(a.key), att, place); e == nil || out != nil {
		t.Errorf("Contribute signed a document whose block it cannot draw (err %v, %d bytes)", e, len(out))
	}
}

// C5 — no line of any block is wider than the frame's inner width, measured from the glyphs drawn.
func TestNoBlockLineIsWiderThanItsFrame(t *testing.T) {
	words := map[string][]string{
		"latin":      {"Maximilian", "Hohenzollern-Sigmaringen", "WWWWMMMM", "il", "Vermögensverwaltung"},
		"cyrillic":   {"Александр", "Рождественский", "ЩЩЩЩ"},
		"cjk":        {"東京国際法律事務所", "代表取締役", "ハーバーロード・ホールディングス"},
		"hangul":     {"서울국제법률사무소", "대표이사", "주식매매계약서"},
		"hebrew":     {"אברהם", "רוזנצווייג", "המצורף"},
		"arabic":     {"عبدالرحمن", "العزيز", "للاستثمار"},
		"devanagari": {"लक्ष्मीनारायणन", "वेंकटसुब्रमण्यम", "श्रीमती"},
		"thai":       {"กัญญาณัฐ", "ศรีสุวรรณวัฒนกุล", "กรรมการผู้จัดการ"},
	}
	fp := strings.Repeat("ab12", 16)
	when := time.Date(2026, 10, 12, 14, 30, 0, 0, time.UTC)
	limit := blockTextWidth()
	checked, widest := 0, 0.0
	for script, ws := range words {
		for n := 1; n <= 40; n += 3 {
			var spaced, solid []string
			for i := 0; i < n; i++ {
				spaced = append(spaced, ws[i%len(ws)])
				solid = append(solid, ws[(i*2)%len(ws)])
			}
			for _, value := range []string{strings.Join(spaced, " "), strings.Join(solid, ""), "Dr. " + strings.Join(spaced, " / ")} {
				for _, a := range []Attestation{
					{Signer: value, AcceptedPeer: fp, AcceptedPeerLabel: value, Intent: value, When: when},
					{Signer: value, Capacity: value, Intent: value, RosterSize: 9, Position: 4, When: when},
				} {
					for _, ln := range a.AppearanceLines() {
						w := widthForTest(t, ln)
						checked++
						if w > widest {
							widest = w
						}
						if w > limit+0.01 {
							t.Fatalf("%s: a block line is %.1fpt wide and the frame holds %.1fpt: %q", script, w, limit, ln)
						}
					}
				}
			}
		}
	}
	if checked < 2000 || widest < limit*0.9 {
		t.Fatalf("only %d lines were measured and the widest is %.1fpt of %.1fpt: the names are not long "+
			"enough to wrap, so nothing here tests the wrap", checked, widest, limit)
	}
	// The fixture intents the rest of this package signs with.
	for _, intent := range []string{defaultIntent, "I agree to co-sign", "We agree to grant a lease of Flat 3 Acacia Avenue",
		strings.Repeat("We agree to grant a lease of Flat 3 Acacia Avenue ", 12)} {
		for _, ln := range (Attestation{Signer: "Nib User", Intent: intent, RosterSize: 4, Position: 2, When: when}).AppearanceLines() {
			if w := widthForTest(t, ln); w > limit+0.01 {
				t.Errorf("a fixture intent's line is %.1fpt wide (frame %.1fpt): %q", w, limit, ln)
			}
		}
	}
	// A hard break never parts a letter from the marks on it.
	for _, ln := range wrapBlockLine("Signer: ", strings.Repeat("क्षि", 60)) {
		if r := []rune(ln); len(r) > 0 && continuesCluster(0, r[0]) {
			t.Fatalf("a wrapped line begins with a combining mark (U+%04X): %q", r[0], ln)
		}
	}
}

// C6 — a block that does not fit its frame is REFUSED: by line count where the attestation is built,
// and by the drawing itself, which never shrinks past the floor and never clips.
func TestABlockThatDoesNotFitItsFrameIsRefusedNotShrunkOrClipped(t *testing.T) {
	// A CJK recital the old Helvetica estimate called three lines is, in the face drawn, far more.
	long := strings.Repeat("私たちは株式譲渡契約に同意します。", 12)
	att := Attestation{Signer: "山田 太郎", Intent: long, RosterSize: 3, Position: 1, When: time.Now()}
	if BlockFits(att) {
		t.Fatalf("a %d-character CJK recital 'fits' in %d lines: the wrap is not measuring the face drawn",
			len([]rune(long)), BlockLineCount(att))
	}
	if _, err := att.BlockImage(); !errors.Is(err, ErrBlockTooTall) {
		t.Errorf("BlockImage answered %v for a block of %d lines, want ErrBlockTooTall", err, BlockLineCount(att))
	}
	nine := make([]string, maxBlockLines+1)
	for i := range nine {
		nine[i] = fmt.Sprintf("line %d", i+1)
	}
	if img, err := drawBlock(nine); !errors.Is(err, ErrBlockTooTall) || img != nil {
		t.Errorf("drawing %d lines answered %v (%d bytes): a block past the limit must not be drawn smaller", len(nine), err, len(img))
	}
	if img, err := drawBlock(nil); err == nil || img != nil {
		t.Errorf("drawing no lines answered %v", err)
	}
	// A line handed in wider than the frame is not clipped at its edge.
	if img, err := drawBlock([]string{strings.Repeat("W", 90)}); err == nil || img != nil {
		t.Errorf("a 90-W line was drawn (%d bytes, err %v): it does not fit, so part of it was cut off", len(img), err)
	}
	// The control: the limit itself draws.
	if _, err := drawBlock(nine[:maxBlockLines]); err != nil {
		t.Errorf("a block of exactly %d lines was refused: %v", maxBlockLines, err)
	}
}

// C7 — the picture is a function of the lines alone, and the picture signed is that picture.
func TestTheSameLinesAreTheSamePictureAndItIsTheOneEmbedded(t *testing.T) {
	a := l3Identity(t, "A")
	when := time.Date(2026, 10, 12, 14, 30, 0, 0, time.UTC)
	// Latin throughout. When this was written the signing library could not write a /Reason holding
	// a character whose UTF-16 form has a parenthesis or backslash byte (most CJK, some Arabic);
	// ADR-163 closed that, and `TestAContributionInAnyScriptIsSignedAsWritten` signs the other scripts.
	att := Attestation{Signer: "Margaret O'Hanlon-Šimůnek", AcceptedPeer: strings.Repeat("ab12", 16),
		AcceptedPeerLabel: "Dieter Müller (Kanzlei Müller & Søn)", Intent: "I agree to the lease of 14 Harbour Road.", When: when}
	one, err := att.BlockImage()
	if err != nil {
		t.Fatal(err)
	}
	two, err := att.BlockImage()
	if err != nil || !bytes.Equal(one, two) {
		t.Fatalf("the same attestation drew two different pictures (err %v): a quote and a signature "+
			"could then never be the same bytes", err)
	}
	later := att
	later.When = when.Add(time.Minute)
	if other, _ := later.BlockImage(); bytes.Equal(one, other) {
		t.Fatal("setup: a different time drew the same picture, so the comparison below cannot tell blocks apart")
	}
	img, err := png.Decode(bytes.NewReader(one))
	if err != nil {
		t.Fatal(err)
	}
	rect := NominalBlockRect()
	wantW, wantH := int((rect[2]-rect[0])*blockScale), int((rect[3]-rect[1])*blockScale)
	if img.Bounds().Dx() != wantW || img.Bounds().Dy() != wantH {
		t.Fatalf("the picture is %v, want %dx%d — the block's own shape", img.Bounds(), wantW, wantH)
	}
	ink, frame := inkOf(img)
	if frame < 2*(wantW+wantH) || ink < 2000 {
		t.Fatalf("the picture has %d dark frame pixels and %d of text: it shows nothing a reader can read", frame, ink)
	}

	doc := l3Prepared(t)
	place, err := NextPlacement(doc)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := Contribute(doc, []byte(a.cert), []byte(a.key), att, place)
	if err != nil {
		t.Fatal(err)
	}
	before, after := testpdf.PicturesIn(doc), testpdf.PicturesIn(signed)
	if len(after) != len(before)+1 {
		t.Fatalf("the contribution added %d picture(s), want exactly one — the block", len(after)-len(before))
	}
	got := after[len(after)-1]
	if got.W != wantW || got.H != wantH || !bytes.Equal(got.RGB, rgbOf(img)) {
		t.Errorf("the picture embedded in the signed document (%dx%d) is not the picture BlockImage "+
			"drew for the same attestation: what a party is shown and what is signed can differ", got.W, got.H)
	}
}

// inkOf counts the dark pixels of a block picture: inside the frame, and on it.
func inkOf(img image.Image) (interior, frame int) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 > 128 || g>>8 > 128 || bl>>8 > 128 {
				continue
			}
			if x < b.Min.X+blockBorder || y < b.Min.Y+blockBorder || x >= b.Max.X-blockBorder || y >= b.Max.Y-blockBorder {
				frame++
			} else {
				interior++
			}
		}
	}
	return interior, frame
}

// rgbOf is img as the signing library embeds it: 8-bit RGB, row by row.
func rgbOf(img image.Image) []byte {
	b := img.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*3)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			out = append(out, byte(r>>8), byte(g>>8), byte(bl>>8))
		}
	}
	return out
}

// The picture's own layout, which tier 3 reads from a rendered page (`test/ui/blockcontent.test.mjs`)
// and which is asserted here on the samples: a frame, and one row of text per line, clear of the frame.
func TestTheBlockPictureHasAFrameAndOneRowOfTextPerLine(t *testing.T) {
	att := Attestation{Signer: "Nib User", AcceptedPeer: strings.Repeat("ab12", 16), AcceptedPeerLabel: "Bob",
		Intent: "I agree to sign this document.", When: time.Date(2026, 10, 12, 14, 30, 0, 0, time.UTC)}
	lines := att.AppearanceLines()
	raw, err := att.BlockImage()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	dark := func(x, y int) bool { r, _, _, _ := img.At(x, y).RGBA(); return r>>8 < 128 }
	for _, p := range [][2]int{{0, 0}, {b.Dx() - 1, 0}, {0, b.Dy() - 1}, {b.Dx() - 1, b.Dy() - 1}, {b.Dx() / 2, 0}, {0, b.Dy() / 2}} {
		if !dark(p[0], p[1]) {
			t.Fatalf("the frame is not drawn at %v", p)
		}
	}
	band := blockPadPt * blockScale / 2
	rows := make([]int, 0, b.Dy())
	for y := band; y < b.Dy()-band; y++ {
		n := 0
		for x := band; x < b.Dx()-band; x++ {
			if dark(x, y) {
				n++
			}
		}
		rows = append(rows, n)
	}
	last := -1
	for i, n := range rows {
		if n >= 3 {
			last = i
		}
	}
	for s := 0; s < len(lines); s++ {
		sum := 0
		for _, n := range rows[s*len(rows)/len(lines) : (s+1)*len(rows)/len(lines)] {
			sum += n
		}
		if sum < 200 {
			t.Errorf("strip %d of %d carries %d dark pixels: the lines are not laid out one per row (%q)", s, len(lines), sum, lines[s])
		}
	}
	if last < 0 || last >= len(rows)-4 {
		t.Errorf("the lowest ink is at row %d of %d: the last line runs into the frame", last, len(rows))
	}
}

// C8 — what a block costs to draw, measured. The figures are logged for the record (ADR-158 quotes
// them); the bound is loose on purpose, there to catch a change of order and not a slow machine.
func TestWhatDrawingABlockCosts(t *testing.T) {
	when := time.Date(2026, 10, 12, 14, 30, 0, 0, time.UTC)
	fp := strings.Repeat("ab12", 16)
	pad := func(lines []string, n int) []string {
		out := append([]string(nil), lines...)
		for len(out) < n {
			out = append(out[:len(out)-1], "Intent: and a further line of the recital, of ordinary length", out[len(out)-1])
		}
		return out
	}
	latin := Attestation{Signer: "Margaret O'Hanlon-Šimůnek", AcceptedPeer: fp, AcceptedPeerLabel: "Dieter Müller",
		Intent: "I agree to sign this document.", When: when}.AppearanceLines()
	mixed := []string{
		"Nib co-signing attestation",
		"Signer: Dr. محمد العربي (Mohammed Al-Arabi) / דוד כהן",
		"Capacity: Director / مدير — 取締役 — 대표이사",
		"Party 4 of 9",
		"Intent: We agree — نوافق — אנו מסכימים — 同意します —",
		"हम सहमत हैं — เราตกลง — Συμφωνούμε — Мы согласны —",
		"우리는 동의합니다 — 私たちは同意します (12 Oct 2026).",
		"Time: 2026-10-12 14:30 UTC",
	}
	if _, err := drawBlock(latin); err != nil { // the faces are parsed once per process; not what is timed
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		lines []string
	}{
		{"5 lines, Latin", pad(latin, 5)},
		{"7 lines, Latin", pad(latin, 7)},
		{"8 lines, Latin (the limit; a 9th is refused)", pad(latin, maxBlockLines)},
		{"8 lines, mixed scripts and directions", mixed},
	}
	for _, c := range cases {
		if len(c.lines) > maxBlockLines {
			t.Fatalf("setup: %s is %d lines", c.name, len(c.lines))
		}
		const rounds = 20
		start := time.Now()
		var size int
		for i := 0; i < rounds; i++ {
			img, err := drawBlock(c.lines)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			size = len(img)
		}
		per := time.Since(start) / rounds
		t.Logf("draw %-46s %6.2f ms a block, %d-byte PNG", c.name, float64(per.Microseconds())/1000, size)
		if per > 500*time.Millisecond {
			t.Errorf("%s took %v a block", c.name, per)
		}
	}
	// The wrap and the fit, which run where a recital is typed: convene admits 4,096 characters to
	// be measured, and the refusal bisects the longest field.
	for _, n := range []int{200, 1000, 4096} {
		recital := string([]rune(strings.Repeat("私たちは同意します agreed ", n/14+1))[:n])
		a := Attestation{Signer: "Nib User", Intent: recital, RosterSize: 3, Position: 1, When: when}
		start := time.Now()
		fits := BlockFits(a)
		wrap := time.Since(start)
		start = time.Now()
		lines, _, _, keep := BlockOverflow(a)
		over := time.Since(start)
		t.Logf("wrap a %4d-character recital: fits=%v in %6.2f ms; the refusal's bisection (%d lines, %d fit) %7.2f ms",
			n, fits, float64(wrap.Microseconds())/1000, lines, keep, float64(over.Microseconds())/1000)
		if wrap+over > 20*time.Second {
			t.Errorf("measuring a %d-character recital took %v", n, wrap+over)
		}
	}
}

// A field longer than `blockMeasureCap` is refused without being shaped: the wrap is a shaping per
// measurement now, and the manual co-sign routes bound nothing themselves.
func TestAFieldPastTheMeasuringCapIsRefusedWithoutBeingMeasured(t *testing.T) {
	// Zero-width joiners have no advance, so by WIDTH any number of them fits one line: only the cap
	// refuses this, which is what makes it the row that sees the cap.
	over := Attestation{Signer: "A", Intent: "ok" + strings.Repeat("\u200d", blockMeasureCap+1), RosterSize: 2, Position: 1}
	if BlockFits(over) {
		t.Errorf("a recital of %d characters 'fits': it was measured rather than refused for its length", blockMeasureCap+3)
	}
	if under := (Attestation{Signer: "A", Intent: "ok" + strings.Repeat("\u200d", 100), RosterSize: 2, Position: 1}); !BlockFits(under) {
		t.Fatal("setup: a short recital holding zero-width joiners does not fit, so the row above is not about the cap")
	}
	huge := strings.Repeat("word ", 200_000) // a million characters
	start := time.Now()
	lines, limit, worst, fits := BlockOverflow(Attestation{Signer: "A", Intent: huge, RosterSize: 2, Position: 1})
	label := AcceptsLabelFits(huge, strings.Repeat("ab12", 16))
	keep := MaxAcceptsLabelRunes(huge, strings.Repeat("ab12", 16))
	took := time.Since(start)
	if lines <= limit || worst != "the recital" || fits <= 0 || fits > blockMeasureCap {
		t.Errorf("the refusal for a million-character recital reads %d lines of %d, %s, %d fit", lines, limit, worst, fits)
	}
	if label || keep <= 0 || keep > blockMeasureCap {
		t.Errorf("a million-character label: fits=%v, %d characters quoted as fitting", label, keep)
	}
	if took > 10*time.Second {
		t.Errorf("answering for a million characters took %v: the whole text was shaped", took)
	}
	t.Logf("a million-character recital and label were answered in %v", took)
}
