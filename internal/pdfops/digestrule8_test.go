package pdfops

import (
	"fmt"
	"strings"
	"testing"
)

// /pending 927: pdfcpu's optimize pass merges fonts that differ only in their subset tag and keeps whichever its map
// iteration meets first, so rule 7 hashed a different tag on different reads of the same bytes. Rule 8 leaves the
// tag out of a font dictionary's BaseFont, FontName and Name.

// twoTagsOnePage is one page whose /Font holds two TrueType fonts equal but for the subset tag of their /BaseFont,
// over one shared descriptor (a /FontDescriptor is compared strictly, so two descriptors would keep them apart).
func twoTagsOnePage(tagA, tagB string) []byte {
	stream := func(body string) string {
		return fmt.Sprintf("<< /Length %d /Length1 %d >>\nstream\n%s\nendstream", len(body), len(body), body)
	}
	font := func(tag string) string {
		return fmt.Sprintf("<< /Type /Font /Subtype /TrueType /BaseFont /%s+Dup /FirstChar 32 /LastChar 32 /Widths [250] /FontDescriptor 20 0 R >>", tag)
	}
	return assembleFixture(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [10 0 R] /Count 1 >>",
		10: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 11 0 R /Resources << /Font << /F1 12 0 R /F2 13 0 R >> >> >>",
		11: stream("BT /F1 12 Tf 20 100 Td (hi) Tj /F2 12 Tf (yo) Tj ET"),
		12: font(tagA),
		13: font(tagB),
		20: "<< /Type /FontDescriptor /FontName /AAAAAA+Dup /Flags 32 /FontBBox [0 0 500 500] /ItalicAngle 0 /Ascent 500 /Descent 0 /CapHeight 500 /StemV 80 /FontFile2 21 0 R >>",
		21: stream("program-0"),
	})
}

func digestsOver(t *testing.T, pdf []byte, version, reads int) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	for i := 0; i < reads; i++ {
		d, err := ContentDigestAt(pdf, version)
		if err != nil {
			t.Fatalf("rule %d: %v", version, err)
		}
		got[d] = true
	}
	return got
}

func TestRule8IsTheSameOnEveryReadOfFontsThatDifferOnlyInTheirTag(t *testing.T) {
	pdf := twoTagsOnePage("AAAAAA", "BBBBBB")
	if n := len(digestsOver(t, pdf, 7, 60)); n < 2 {
		t.Fatalf("stimulus: rule 7 gave %d distinct digest(s) in 60 reads, so the fixture does not reach the merge", n)
	}
	if n := len(digestsOver(t, pdf, 8, 60)); n != 1 {
		t.Fatalf("rule 8 gave %d distinct digests in 60 reads of the same bytes, want 1", n)
	}
}

func TestRule8IgnoresTheTagAndNothingElseOfAFontName(t *testing.T) {
	a := digestsOver(t, twoTagsOnePage("AAAAAA", "AAAAAA"), 8, 1)
	b := digestsOver(t, twoTagsOnePage("ZZZZZZ", "ZZZZZZ"), 8, 1)
	for d := range a {
		if !b[d] {
			t.Fatal("rule 8 moved with a subset tag alone")
		}
	}
	c := digestsOver(t, []byte(string(twoTagsOnePage("AAAAAA", "AAAAAA"))), 7, 1)
	d := digestsOver(t, twoTagsOnePage("ZZZZZZ", "ZZZZZZ"), 7, 1)
	for x := range c {
		if d[x] {
			t.Fatal("stimulus: rule 7 did not move with the tag, so rule 8's insensitivity shows nothing")
		}
	}
}

func TestRule7StaysComputableAndRule8IsCurrent(t *testing.T) {
	for v, want := range map[int]bool{4: false, 5: false, 6: false, 7: true, 8: true, 9: false} {
		if got := ContentDigestRuleReadable(v); got != want {
			t.Errorf("ContentDigestRuleReadable(%d) = %v, want %v", v, got, want)
		}
	}
	for v, want := range map[int]bool{4: false, 5: true, 6: true, 7: false, 8: false} {
		if got := ContentDigestRuleRetired(v); got != want {
			t.Errorf("ContentDigestRuleRetired(%d) = %v, want %v", v, got, want)
		}
	}
	if ContentDigestVersion != 8 {
		t.Errorf("ContentDigestVersion = %d, want 8", ContentDigestVersion)
	}
}

// A /FontDescriptor is compared strictly by pdfcpu, so its /FontName keeps its tag in rule 8.
func TestRule8KeepsTheTagOfAFontDescriptor(t *testing.T) {
	a := twoTagsOnePage("AAAAAA", "AAAAAA")
	b := []byte(strings.Replace(string(a), "/FontName /AAAAAA+Dup", "/FontName /ZZZZZZ+Dup", 1))
	if string(a) == string(b) {
		t.Fatal("stimulus: the descriptor's name was not changed")
	}
	da, db := digestsOver(t, a, 8, 1), digestsOver(t, b, 8, 1)
	for d := range da {
		if db[d] {
			t.Fatal("rule 8 stripped the tag of a /FontDescriptor's /FontName")
		}
	}
}

func TestTheSubsetTagIsEverythingBeforeAPlusThatIsNotFirst(t *testing.T) {
	for in, want := range map[string]string{"ABCDEF+Foo": "Foo", "Foo": "Foo", "+Foo": "+Foo", "A+B+C": "B+C", "": "", "ABCDEF+": ""} {
		if got := withoutSubsetTag(in); got != want {
			t.Errorf("withoutSubsetTag(%q) = %q, want %q", in, got, want)
		}
	}
}
