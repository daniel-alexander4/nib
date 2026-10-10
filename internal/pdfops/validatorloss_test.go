package pdfops

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// A font pdfcpu's validator takes out of a document is still in what nib writes — ADR-129, `/pending 842`.

// lostFontDoc is a one-page document whose page names four fonts: /F2 a plain Helvetica, and three the relaxed
// validator takes out of the `/Font` dictionary rather than refuse — /F1 a Type 0 font whose descendant fails
// validation (`/CIDToGIDMap /NoIdentity`, the shape of veraPDF's 7.21.3.2-t01-fail-a), /F3 a font written
// directly in the dictionary, /F4 a reference to nothing. With twin, /F1's `/BaseFont` is /F2's, and the
// validator points /F1 at /F2 instead of emptying it.
func lostFontDoc(twin bool) []byte {
	base := "/Broken"
	if twin {
		base = "/Helvetica"
	}
	content := "BT /F1 12 Tf 72 700 Td <0001> Tj /F2 12 Tf (two) Tj /F3 12 Tf (three) Tj /F4 12 Tf (four) Tj ET"
	program := "the program of the font the validator loses"
	return testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << " +
			"/F1 5 0 R /F2 6 0 R /F3 << /Type /Font /Subtype /Type1 /BaseFont /Courier >> /F4 99 0 R >> >> >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type0 /BaseFont " + base + " /Encoding /Identity-H /DescendantFonts [7 0 R] >>",
		6: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7: "<< /Type /Font /Subtype /CIDFontType2 /BaseFont " + base + " /CIDSystemInfo << /Registry (Adobe) " +
			"/Ordering (Identity) /Supplement 0 >> /FontDescriptor 8 0 R /CIDToGIDMap /NoIdentity >>",
		8: "<< /Type /FontDescriptor /FontName " + base + " /Flags 4 /FontBBox [0 0 1000 1000] /ItalicAngle 0 " +
			"/Ascent 800 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 9 0 R >>",
		9: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(program), program),
	})
}

// fontsAsWritten is each font name of each page's `/Font` dictionary with everything it reaches, read WITHOUT
// pdfcpu's validator (which is what takes the fonts away), so it is what the file holds. References are followed
// and never printed, since a rewrite renumbers them; a stream is its decoded-or-raw bytes' hash.
func fontsAsWritten(t *testing.T, pdf []byte) []map[string]string {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("unvalidated read: %v", err)
	}
	var render func(o types.Object, on map[int]bool, depth int) string
	renderDict := func(d types.Dict, on map[int]bool, depth int) string {
		keys := make([]string, 0, len(d))
		for k := range d {
			if k != "Parent" && k != "Length" && k != "Filter" && k != "DecodeParms" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("<<")
		for _, k := range keys {
			b.WriteString("/" + k + " " + render(d[k], on, depth+1))
		}
		return b.String() + ">>"
	}
	render = func(o types.Object, on map[int]bool, depth int) string {
		if depth > 40 {
			return "…"
		}
		switch v := o.(type) {
		case nil:
			return "null"
		case types.IndirectRef:
			n := v.ObjectNumber.Value()
			if on[n] {
				return "loop"
			}
			r, err := ctx.Dereference(v)
			if err != nil || r == nil {
				return "nothing"
			}
			on[n] = true
			defer delete(on, n)
			return render(r, on, depth+1)
		case types.Dict:
			return renderDict(v, on, depth)
		case types.StreamDict:
			body := v.Raw
			if err := v.Decode(); err == nil && v.Content != nil {
				body = v.Content
			}
			return renderDict(v.Dict, on, depth) + fmt.Sprintf("stream %x", sha256.Sum256(body))
		case types.Array:
			var b strings.Builder
			for _, el := range v {
				b.WriteString(render(el, on, depth+1) + " ")
			}
			return "[" + b.String() + "]"
		}
		return o.PDFString()
	}
	// The page tree walked by hand: pdfcpu counts and indexes pages only as it validates.
	var pages []map[string]string
	var walk func(o types.Object, inherited types.Object, depth int)
	walk = func(o types.Object, inherited types.Object, depth int) {
		d, _ := ctx.DereferenceDict(o)
		if d == nil || depth > 40 {
			return
		}
		if r, ok := d["Resources"]; ok {
			inherited = r
		}
		if kids, _ := ctx.DereferenceArray(d["Kids"]); kids != nil {
			for _, k := range kids {
				walk(k, inherited, depth+1)
			}
			return
		}
		fonts := map[string]string{}
		if res, _ := ctx.DereferenceDict(inherited); res != nil {
			if fd, _ := ctx.DereferenceDict(res["Font"]); fd != nil {
				for name, f := range fd {
					fonts[name] = render(f, map[int]bool{}, 0)
				}
			}
		}
		pages = append(pages, fonts)
	}
	if root, err := ctx.XRefTable.Catalog(); err == nil {
		walk(root["Pages"], nil, 0)
	}
	if len(pages) == 0 {
		t.Fatal("setup: no page found in the unvalidated read")
	}
	return pages
}

// rewritesThatKeepThePage are operations after which a page still draws what it drew, each returning the output
// and which of its pages the source's first page became. `digest` marks the ones `ContentDigest` is unmoved by.
var rewritesThatKeepThePage = []struct {
	name   string
	digest bool
	run    func(pdf []byte) ([]byte, error)
}{
	{"a rewrite that changes nothing", true, func(pdf []byte) ([]byte, error) {
		return writeMutated(pdf, func(*model.Context) error { return nil })
	}},
	{"EmbedCeremonyRecord", true, func(pdf []byte) ([]byte, error) { return EmbedCeremonyRecord(pdf, []byte(`{"v":1}`)) }},
	{"AddNotes", false, func(pdf []byte) ([]byte, error) {
		return AddNotes(pdf, []Note{{Page: 1, X: 100, Y: 100, Text: "a note"}})
	}},
	{"StampWatermark", false, func(pdf []byte) ([]byte, error) {
		return StampWatermark(pdf, "DRAFT", WatermarkStyle{Color: "#808080", Opacity: 0.3, Scale: 0.5, Angle: 45})
	}},
	{"Rotate", false, func(pdf []byte) ([]byte, error) { return Rotate(pdf, []string{"1"}, 90) }},
	{"Append, as the document appended to", false, func(pdf []byte) ([]byte, error) {
		return Append(pdf, testpdf.Assemble(blankDocObjects()))
	}},
	{"Append, as the document appended", false, func(pdf []byte) ([]byte, error) {
		out, err := Append(testpdf.Assemble(blankDocObjects()), pdf)
		if err != nil {
			return nil, err
		}
		return Collect(out, []string{"2", "1"})
	}},
}

func blankDocObjects() map[int]string {
	return map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
	}
}

// TestARewriteKeepsTheFontsTheValidatorTakes — on the fixture, with and without a same-named twin, and on the
// two veraPDF corpus files `/pending 842` was found on: after each rewrite the first page's `/Font` names every
// font the source named and each reaches what it reached, and where the operation leaves the content alone,
// `ContentDigest` is the source's under BOTH rules.
func TestARewriteKeepsTheFontsTheValidatorTakes(t *testing.T) {
	docs := map[string][]byte{"fixture": lostFontDoc(false), "fixture with a twin": lostFontDoc(true)}
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, "nib", "verapdfs", "PDF_UA-1", "7.21 Fonts", "7.21.3 Composite fonts", "7.21.3.2 CIDFonts")
		for _, name := range []string{"7.21.3.2-t01-fail-a.pdf", "7.21.3.2-t01-fail-c.pdf"} {
			if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
				docs[name] = b
			} else {
				t.Logf("SKIP (not a pass): %s is not on this machine", name)
			}
		}
	}
	for name, src := range docs {
		want := fontsAsWritten(t, src)[0]
		// Stimulus: the validator does take a font out of this document, or nothing below is tested.
		ctx, err := pdfread.ReadOptimized(src, model.NewDefaultConfiguration())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := pdfread.ValidatorLosses(ctx); n == 0 {
			t.Fatalf("setup: %s: pdfcpu's validator changed no /Font entry, so the document does not show the defect", name)
		}
		if strings.HasPrefix(name, "fixture") {
			if len(want) != 4 || want["F4"] != "nothing" || !strings.Contains(want["F1"], "stream") {
				t.Fatalf("setup: %s: the fixture's fonts read as %v", name, want)
			}
		}
		for _, rw := range rewritesThatKeepThePage {
			out, err := rw.run(src)
			if err != nil {
				t.Errorf("%s: %s: %v", name, rw.name, err)
				continue
			}
			got := fontsAsWritten(t, out)[0]
			for font, reaches := range want {
				g, ok := got[font]
				switch {
				case !ok:
					t.Errorf("%s: after %s the page's /Font has no /%s — the content still names it (has %v)",
						name, rw.name, font, keysOf(got))
				case g != reaches:
					t.Errorf("%s: after %s /%s reaches\n  %s\nwhere the source's reached\n  %s", name, rw.name, font, g, reaches)
				}
			}
			if !rw.digest {
				continue
			}
			for _, rule := range []int{legacyContentDigestVersion, ContentDigestVersion} {
				before, err1 := ContentDigestAt(src, rule)
				after, err2 := ContentDigestAt(out, rule)
				if err1 != nil || err2 != nil {
					t.Errorf("%s: %s: digest under rule %d: %v / %v", name, rw.name, rule, err1, err2)
					continue
				}
				if before != after {
					t.Errorf("%s: %s moved ContentDigest under rule %d: %s, was %s", name, rw.name, rule, after, before)
				}
			}
		}
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestACutOrComposedPageKeepsTheFontTheValidatorTakes — the operations that move a page's content somewhere else
// (a cut copies it into a new context, n-up into a form): the lost Type 0 font's program is still in the output.
func TestACutOrComposedPageKeepsTheFontTheValidatorTakes(t *testing.T) {
	src := lostFontDoc(false)
	for name, run := range map[string]func() ([]byte, error){
		"SplitPage": func() ([]byte, error) { return SplitPage(src, 1, 2, 1, false) },
		"NUp":       func() ([]byte, error) { return NUp(src, 2, false) },
	} {
		out, err := run()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		ctx, err := api.ReadContext(bytes.NewReader(out), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for n, e := range ctx.XRefTable.Table {
			if e == nil || e.Free {
				continue
			}
			// Dereferenced: an object held in an object stream is not in the table until it is asked for.
			o, _ := ctx.Dereference(*types.NewIndirectRef(n, 0))
			if d, ok := o.(types.Dict); ok {
				if bf := d.NameEntry("BaseFont"); bf != nil && *bf == "Broken" && d.NameEntry("Subtype") != nil && *d.NameEntry("Subtype") == "Type0" {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s: the output holds no Type 0 font /Broken — the page's content still names it", name)
		}
	}
}

// TestAnOperationsOwnChangeToALostFontEntryStands — the write puts back only an entry still as the validator left
// it: one the operation removed stays removed, and one it rebound keeps the operation's font.
func TestAnOperationsOwnChangeToALostFontEntryStands(t *testing.T) {
	out, err := writeMutated(lostFontDoc(false), func(ctx *model.Context) error {
		for _, pg := range pdfread.Pages(ctx) {
			res, _ := ctx.DereferenceDict(pg.Dict["Resources"])
			fd, _ := ctx.DereferenceDict(res["Font"])
			if fd == nil {
				return fmt.Errorf("no /Font")
			}
			delete(fd, "F1")
			fd["F3"] = fd["F2"]
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := fontsAsWritten(t, out)[0]
	if _, ok := got["F1"]; ok {
		t.Errorf("/F1 was removed by the operation and is back: %v", keysOf(got))
	}
	if got["F3"] != got["F2"] || got["F2"] == "" {
		t.Errorf("/F3 was rebound to /F2's font by the operation and reads %q (F2 %q)", got["F3"], got["F2"])
	}
}

// TestTwoPagesKeepTheirOwnLostFontOfOneName — two pages each bind /F1 to a DIFFERENT font the validator takes, and
// the second inherits its `/Resources` from a page-tree node. pdfcpu's optimize pass gives each page a new
// dictionary, so the name alone says nothing: each page's entry is found again from the page. After a rewrite
// that changes nothing, each page's /F1 reaches the font it reached, and the digest is the source's.
func TestTwoPagesKeepTheirOwnLostFontOfOneName(t *testing.T) {
	content := "BT /F1 12 Tf 72 700 Td <0001> Tj ET"
	stream := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
	type0 := func(base string, desc int) string {
		return "<< /Type /Font /Subtype /Type0 /BaseFont /" + base + " /Encoding /Identity-H /DescendantFonts [" +
			fmt.Sprint(desc) + " 0 R] >>"
	}
	cid := func(base string) string {
		return "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /" + base + " /CIDSystemInfo << /Registry (Adobe) " +
			"/Ordering (Identity) /Supplement 0 >> /CIDToGIDMap /NoIdentity >>"
	}
	src := testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R 10 0 R] /Count 2 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		4:  stream,
		5:  type0("First", 6),
		6:  cid("First"),
		10: "<< /Type /Pages /Parent 2 0 R /Kids [11 0 R] /Count 1 /Resources << /Font << /F1 12 0 R >> >> >>",
		11: "<< /Type /Page /Parent 10 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		12: type0("Second", 13),
		13: cid("Second"),
	})
	want := fontsAsWritten(t, src)
	if len(want) != 2 || !strings.Contains(want[0]["F1"], "/First") || !strings.Contains(want[1]["F1"], "/Second") {
		t.Fatalf("setup: the fixture's pages read as %v", want)
	}
	out, err := writeMutated(src, func(*model.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	got := fontsAsWritten(t, out)
	for i := range want {
		if got[i]["F1"] != want[i]["F1"] {
			t.Errorf("page %d: /F1 reaches %q after the rewrite, was %q", i+1, got[i]["F1"], want[i]["F1"])
		}
	}
	for _, rule := range []int{legacyContentDigestVersion, ContentDigestVersion} {
		before, err1 := ContentDigestAt(src, rule)
		after, err2 := ContentDigestAt(out, rule)
		if err1 != nil || err2 != nil || before != after {
			t.Errorf("rule %d: digest %s (%v), was %s (%v)", rule, after, err2, before, err1)
		}
	}
}

// TestALostFontIsNotPutBackOntoAnotherObject — the reference is restored only while its number is still the
// cross-reference entry the document named: an operation that put another object there keeps the page from
// drawing with it.
func TestALostFontIsNotPutBackOntoAnotherObject(t *testing.T) {
	out, err := writeMutated(lostFontDoc(false), func(ctx *model.Context) error {
		ctx.XRefTable.Table[5] = &model.XRefTableEntry{Object: types.Dict{"Type": types.Name("NotTheFont")}, Generation: new(int)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := fontsAsWritten(t, out)[0]
	if f, ok := got["F1"]; ok {
		t.Errorf("/F1 was put back onto an object the operation replaced: %s", f)
	}
	if !strings.Contains(got["F3"], "/Courier") {
		t.Errorf("setup: /F3, which the operation did not touch, reads %q", got["F3"])
	}
}
