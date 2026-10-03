package contentstream

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// directImageEnd is scanInlineImage as it was before `/pending 817`: every end rule asks its question of the
// stream afresh — `bytes.Index` for the ASCII filters' end-of-data marker, and a byte walk for the whitespace
// before `EI`. It is the oracle the memoised scanner must agree with on every input.
func directImageEnd(src []byte, i int) int {
	j := i + 2
	idAt := -1
	for j+1 < len(src) {
		if src[j] == 'I' && src[j+1] == 'D' && startsAKeyword(src, j) &&
			(j+2 >= len(src) || !isRegular(src[j+2])) {
			idAt = j
			j += 2
			break
		}
		j++
	}
	if j+1 >= len(src) {
		return len(src)
	}
	if j < len(src) && isWhite(src[j]) {
		j++
	}
	endsAt := func(after int) (int, bool) {
		m := after
		for m < len(src) && isWhite(src[m]) {
			m++
		}
		if m+1 < len(src) && src[m] == 'E' && src[m+1] == 'I' && (m+2 >= len(src) || !isRegular(src[m+2])) {
			return m + 2, true
		}
		return 0, false
	}
	if idAt >= i+2 {
		dict := src[i+2 : idAt]
		if f, ok := inlineDictValue(dict, "/F", "/Filter"); ok {
			var eod []byte
			switch string(f.Bytes(dict)) {
			case "/AHx", "/ASCIIHexDecode":
				eod = []byte(">")
			case "/A85", "/ASCII85Decode":
				eod = []byte("~>")
			}
			if eod != nil {
				if at := bytes.Index(src[j:], eod); at >= 0 {
					if end, ok := endsAt(j + at + len(eod)); ok {
						return end
					}
				}
			}
		}
		if v, ok := inlineDictValue(dict, "/L", "/Length"); ok {
			if n, ok := declaredLength(v.Bytes(dict)); ok && n <= len(src)-j {
				if end, ok := endsAt(j + n); ok {
					return end
				}
			}
		}
	}
	for j+1 < len(src) {
		if src[j] == 'E' && src[j+1] == 'I' && j > 0 && isWhite(src[j-1]) &&
			(j+2 >= len(src) || !isRegular(src[j+2])) {
			return j + 2
		}
		j++
	}
	return len(src)
}

// assertImageEndsAgree tokenizes src and checks every inline image ends where the direct search ends it.
// Comparing at each token's own start is enough: the rest of Tokenize is untouched, so the first image the
// two disagree on is the first token that differs.
func assertImageEndsAgree(t *testing.T, name string, src []byte) {
	t.Helper()
	for _, tk := range Tokenize(src) {
		if tk.Kind != InlineImage {
			continue
		}
		if want := directImageEnd(src, tk.Start); tk.End != want {
			t.Errorf("%s: the image at %d ends at %d; the direct search ends it at %d", name, tk.Start, tk.End, want)
			return
		}
	}
}

// TestTheMemoisedImageEndsAgreeWithTheDirectSearch — `/pending 817` changed what finding an inline image's end
// COSTS, and must not have changed where any image ends. Named cases for each memo's edge, then generated
// streams mixing every end rule: end-of-data present, absent and far; `>` against `~>`; images back to back;
// declared lengths right and wrong; whitespace runs on both sides of the index threshold.
func TestTheMemoisedImageEndsAgreeWithTheDirectSearch(t *testing.T) {
	long := strings.Repeat(" ", longRun)
	named := map[string]string{
		"hex, EOD present":           "BI /F /AHx ID 00> EI Q",
		"hex, EOD absent":            "BI /F /AHx ID 00 EI Q",
		"a85, EOD present":           "BI /F /A85 ID zz~>EI Q",
		"a85, a bare > is not ~>":    "BI /F /A85 ID zz> EI q BI /F /A85 ID zz~> EI Q",
		"back to back, one far EOD":  "BI /F /AHx ID 0 EI BI /F /AHx ID 1 EI BI /F /AHx ID 2 EI 0>EI",
		"EOD between the images":     "BI /F /AHx ID 0 EI BI /F /AHx ID 1> EI BI /F /AHx ID 2 EI q",
		"memo passed, then rescans":  "BI /F /AHx ID 0> EI BI /F /AHx ID 1 EI BI /F /AHx ID 2> EI",
		"hex and a85 interleaved":    "BI /F /A85 ID a EI BI /F /AHx ID 1 EI BI /F /A85 ID b~> EI BI /F /AHx ID 2> EI",
		"EOD then a long run, no EI": "BI /F /AHx ID 0 EI BI /F /AHx ID 1 EI >" + long + long + "x",
		"EOD then a long run, EI":    "BI /F /AHx ID 0 EI BI /F /AHx ID 1 EI >" + long + long + "EI Q",
		"run exactly the threshold":  "BI /L 1 ID 0" + long + "EI q",
		"run one short":              "BI /L 1 ID 0" + long[1:] + "EI q",
		"/L into a long run":         "BI /L 40 ID 0 EI BI /L 30 ID 0 EI x" + long + long + long + "EI",
		"/L to the run's last byte":  "BI /L 5 ID 0 EI x" + long + "EI",
		"/L past the end":            "BI /L 999 ID 0 EI",
		"a long run at the very end": "BI /L 3 ID 0 EI" + long + long,
		"long run, image at its end": long + "BI /L 1 ID 0" + long + "EI",
	}
	for name, s := range named {
		assertImageEndsAgree(t, name, []byte(s))
	}

	parts := []string{
		"BI /F /AHx ID 0a EI\n", "BI /F /AHx ID 0a> EI\n", "BI /F /AHx ID 0a>EI ", "BI /Filter /ASCIIHexDecode ID 1 EI ",
		"BI /F /A85 ID zz EI\n", "BI /F /A85 ID zz~>EI\n", "BI /F [/A85 /Fl] ID zz~> EI ", "BI /F /A85 ID zz> EI ",
		"BI /W 1 /H 1 ID \x00\xff EI\n", "BI /L 3 ID abc EI\n", "BI /L 9 ID abc EI\n", "BI /L 2 ID abc EI\n",
		"BI /L 70 ID a EI\n", "BI /L 200 ID a EI\n", "BI /CS /L ID x EI\n",
		">", "~>", "~", " ", "\n", "EI ", " EI", "q ", "Q\n", "BT (a>b) Tj ET\n", "<00> Tj ", "0 0 m ",
		long, long[:longRun-1], long + long, "\r\n" + long + "\t",
	}
	r := rand.New(rand.NewSource(817))
	for n := 0; n < 3000; n++ {
		var b strings.Builder
		for k := r.Intn(24); k >= 0; k-- {
			b.WriteString(parts[r.Intn(len(parts))])
		}
		assertImageEndsAgree(t, fmt.Sprintf("generated #%d", n), []byte(b.String()))
	}
}

// TestTheInlineImageEndSearchesAreLinear — `/pending 817`. Each stream makes every inline image ask a question
// whose answer is far away or absent; asked afresh per image, each walked the rest of the stream. Measured on
// 3 MB before the memo: no `>` ahead 4.8 s (0.1 s unfiltered); one `>` then 1.5 MB of whitespace and no `EI`,
// 2 m 20 s; a `/L` per image into one far whitespace run, 3 m 18 s.
//
// Asserted as a WORK count rather than a wall clock: the bytes the searches examined, bounded by a small
// multiple of the stream plus longRun per image. The direct search examines about images × stream / 2.
func TestTheInlineImageEndSearchesAreLinear(t *testing.T) {
	const size = 200_000
	half := size / 2
	ahx := "BI /W 1 /H 1 /F /AHx ID 00 EI\n"
	a85 := "BI /W 1 /H 1 /F /A85 ID zz EI\n"
	cases := map[string]string{
		"hex, no EOD":         strings.Repeat(ahx, size/len(ahx)),
		"a85, no EOD":         strings.Repeat(a85, size/len(a85)),
		"hex and a85, no EOD": strings.Repeat(ahx+a85, size/len(ahx+a85)),
		"hex, one far EOD":    strings.Repeat(ahx, size/len(ahx)) + ">",
		"a85, one far EOD":    strings.Repeat(a85, size/len(a85)) + "~>",
		"far EOD, then a run": strings.Repeat(ahx, half/len(ahx)) + ">" + strings.Repeat(" ", half) + "x",
		"/L into one far run": lIntoRuns(half, half),
		"/L into three runs":  lIntoRuns(half, half/3, half/3, half/3),
	}
	for name, s := range cases {
		src := []byte(s)
		toks, sc := tokenize(src)
		images := 0
		for _, tk := range toks {
			if tk.Kind == InlineImage {
				images++
			}
		}
		if images < 1000 {
			t.Fatalf("%s: fixture has %d inline images, want thousands", name, images)
		}
		if budget := 4*len(src) + longRun*images; sc.searched > budget {
			t.Errorf("%s: finding %d images' ends examined %d bytes of a %d-byte stream (budget %d): "+
				"an image is searching the rest of the stream again", name, images, sc.searched, len(src), budget)
		}
	}
}

// lIntoRuns is n bytes of `BI /L … ID 0 EI` images followed by whitespace runs of the given lengths, each
// closed by `x` and none by `EI`. Image k declares a length landing inside run k mod len(runs), each image at
// a different offset — so a cache holding only the last run answered would miss on every image.
func lIntoRuns(n int, runs ...int) string {
	const per = len("BI /L 000000000 ID 0 EI\n")
	count := n / per
	starts := make([]int, len(runs))
	at := count * per
	for r, l := range runs {
		starts[r] = at
		at += l + 1
	}
	var b strings.Builder
	for k := 0; k < count; k++ {
		data := k*per + len("BI /L 000000000 ID ")
		r := k % len(runs)
		fmt.Fprintf(&b, "BI /L %09d ID 0 EI\n", starts[r]+(k*7)%runs[r]-data)
	}
	for _, l := range runs {
		b.WriteString(strings.Repeat(" ", l) + "x")
	}
	return b.String()
}
