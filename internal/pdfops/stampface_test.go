package pdfops

import (
	"bytes"
	"errors"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Stamped text embeds its face — `PLAN-ua-coverage.md` P01.S03, PDF/UA 7.21.4.1.

func fontsNotEmbedded(t *testing.T, pdf []byte) []string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	return nonEmbeddedFonts(ctx.XRefTable)
}

// carriesFace reports whether pdf has a subset font of face — `/BaseFont /XXXXXX+<face>`.
func carriesFace(t *testing.T, pdf []byte, face string) bool {
	t.Helper()
	_, _, ok := faceStreams(t, pdf, face)
	return ok
}

// faceStreams returns the decoded font program and ToUnicode map of pdf's subset font of face.
func faceStreams(t *testing.T, pdf []byte, face string) (program, toUnicode []byte, ok bool) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	decoded := func(o types.Object) []byte {
		sd, _, err := ctx.DereferenceStreamDict(o)
		if err != nil || sd == nil || sd.Decode() != nil {
			return nil
		}
		return sd.Content
	}
	for _, e := range ctx.Table {
		if e == nil {
			continue
		}
		d, isDict := e.Object.(types.Dict)
		if !isDict || d.NameEntry("Type") == nil || *d.NameEntry("Type") != "Font" {
			continue
		}
		// The Type0 font, not its CID descendant, which carries the same BaseFont and neither map.
		if st := d.NameEntry("Subtype"); st == nil || *st != "Type0" {
			continue
		}
		if bf := d.NameEntry("BaseFont"); bf == nil || !strings.HasSuffix(*bf, "+"+face) {
			continue
		}
		desc := d
		if kids, kerr := ctx.DereferenceArray(d["DescendantFonts"]); kerr == nil && len(kids) > 0 {
			if kd, derr := ctx.DereferenceDict(kids[0]); derr == nil && kd != nil {
				desc = kd
			}
		}
		fd, _ := ctx.DereferenceDict(desc["FontDescriptor"])
		if fd != nil {
			program = decoded(fd["FontFile2"])
		}
		return program, decoded(d["ToUnicode"]), true
	}
	return nil, nil, false
}

// captureLog sends the standard logger to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logged bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &logged
}

// forceCoreFaces makes nib's faces uninstallable for the rest of the test — the real condition, an
// unwritable font directory, as mdpdf's own test creates it — and proves it took.
func forceCoreFaces(t *testing.T) *bytes.Buffer {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("SKIP (not a pass): running as root, which ignores the directory mode this test depends on")
	}
	model.NewDefaultConfiguration()
	orig := font.UserFontDir
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	logged := captureLog(t)
	t.Cleanup(func() { font.UserFontDir = orig; os.Chmod(dir, 0o700) })
	font.UserFontDir = dir
	if stampFacesInstalled() {
		t.Fatal("setup: the faces installed into a mode-0500 directory, so the fallback was never reached")
	}
	return logged
}

// stampDrives are the three stamping doors, each driven the way its caller does.
func stampDrives(core string) map[string]func([]byte) ([]byte, error) {
	return map[string]func([]byte) ([]byte, error){
		"StampFields": func(b []byte) ([]byte, error) {
			o, _, e := StampFields(b, []Field{{Page: 1, Rect: [4]float64{50, 400, 300, 420}, Text: "Replaced text", Font: core, Size: 12}})
			return o, e
		},
		"StampPageNumbers": func(b []byte) ([]byte, error) { return StampPageNumbers(b, PageNumberStyle{OfTotal: true}) },
		"StampWatermark":   func(b []byte) ([]byte, error) { return StampWatermark(b, "DRAFT", WatermarkStyle{}) },
	}
}

// TestEveryCoreFaceHasAnEmbeddedFace — `coreFonts` is the allowlist a field's face is coerced to and
// `stampFaceFor` is what it is drawn in; a name in one and not the other is drawn unembedded, or
// mapped for a face nothing can ask for.
func TestEveryCoreFaceHasAnEmbeddedFace(t *testing.T) {
	for c := range coreFonts {
		if stampFaceFor[c] == "" {
			t.Errorf("core face %s has no embedded face, so a field set in it is drawn unembedded", c)
		}
	}
	for c := range stampFaceFor {
		if !coreFonts[c] {
			t.Errorf("stampFaceFor maps %s, which coreFonts does not allow", c)
		}
	}
}

// TestStampedTextEmbedsEveryFaceItDrawsWith reads the answer out of the output, for every core face a
// field can ask for — the census drives only Helvetica, and a Times or Courier edit is as ordinary.
func TestStampedTextEmbedsEveryFaceItDrawsWith(t *testing.T) {
	base := labelReady(t, censusMarkdown())
	if missing := fontsNotEmbedded(t, base); len(missing) > 0 {
		t.Fatalf("setup: the base document already draws with unembedded fonts %v, so nothing below "+
			"could tell a stamp's font from the document's", missing)
	}
	// And the face itself, on a document with no fonts of its own — "nothing unembedded" alone passes
	// for a door that drew nothing, or a map that sends every face to one.
	bare := threePagePDF(t)
	body, _, _ := AuthoredTextFaces()
	cores := make([]string, 0, len(coreFonts))
	for c := range coreFonts {
		cores = append(cores, c)
	}
	sort.Strings(cores)
	for _, core := range cores {
		for name, drive := range stampDrives(core) {
			if name != "StampFields" && core != cores[0] {
				continue // the other two doors take no face from the caller
			}
			out, err := drive(base)
			if err != nil {
				t.Fatalf("%s (%s): %v", name, core, err)
			}
			if missing := fontsNotEmbedded(t, out); len(missing) > 0 {
				t.Errorf("%s with %s draws with fonts it does not embed: %v (PDF/UA 7.21.4.1)", name, core, missing)
			}
			want := body
			if name == "StampFields" {
				want = stampFaceFor[core]
			}
			if carriesFace(t, bare, want) {
				t.Fatalf("setup: the bare document already carries %s", want)
			}
			drawn, err := drive(bare)
			if err != nil {
				t.Fatalf("%s (%s) on a bare document: %v", name, core, err)
			}
			if !carriesFace(t, drawn, want) {
				t.Errorf("%s with %s does not draw in %s", name, core, want)
			}
		}
	}
}

// TestStampedTextReportsFallingBackToCoreFaces — the degrade is real, it is Base-14, and it is said.
func TestStampedTextReportsFallingBackToCoreFaces(t *testing.T) {
	base := labelReady(t, censusMarkdown())
	logged := forceCoreFaces(t)
	for name, drive := range stampDrives("Times-Roman") {
		logged.Reset()
		out, err := drive(base)
		if err != nil {
			t.Fatalf("%s: a font directory nib cannot write cost the user the stamp: %v", name, err)
		}
		// The response graded against its stimulus: it really fell back, so the instrument can see one.
		if missing := fontsNotEmbedded(t, out); len(missing) == 0 {
			t.Errorf("%s: no unembedded font in the output, so this did not exercise the fallback", name)
		}
		if msg := logged.String(); !strings.Contains(msg, "will not embed them") {
			t.Errorf("%s fell back to Base-14 faces and said nothing (log: %q)", name, msg)
		}
	}
}

// TestStampWidthMatchesEmittedBBoxInCoreFaces — the fallback's fit measurement, held to the same
// oracle as the embedded one: the width reported is the width pdfcpu emits in the face it drew.
func TestStampWidthMatchesEmittedBBoxInCoreFaces(t *testing.T) {
	pdf := threePagePDF(t)
	forceCoreFaces(t)
	for _, tc := range trapStrings {
		f := Field{Page: 1, Rect: [4]float64{50, 400, 150, 420}, Text: tc.text, Font: "Helvetica", Size: 12}
		out, fits, err := StampFields(pdf, []Field{f})
		if err != nil || len(fits) != 1 {
			t.Fatalf("%s: StampFields: %v (%d fits)", tc.name, err, len(fits))
		}
		if missing := fontsNotEmbedded(t, out); !strings.Contains(strings.Join(missing, " "), "Helvetica") {
			t.Fatalf("%s: the stamp did not draw in core Helvetica (unembedded: %v), so this measures the wrong face", tc.name, missing)
		}
		want := emittedBBoxWidth(t, pdf, f)
		if math.Abs(fits[0].WidthPt-want) > 0.01 {
			t.Errorf("%s: reported %.2fpt, pdfcpu emitted %.2fpt in core Helvetica (%s)", tc.name, fits[0].WidthPt, want, tc.why)
		}
		if got := stampWidth(tc.text, "Helvetica", 12, false); math.Abs(got-want) > 0.01 {
			t.Errorf("%s: stampWidth in core Helvetica = %.2fpt, emitted %.2fpt", tc.name, got, want)
		}
	}
}

// ownFontDocument returns a document that already carries a stamp nib drew in LiberationSans, with
// mutate applied to that font's dictionary — the stand-in for a document from another producer.
func ownFontDocument(t *testing.T, mutate func(ctx *model.Context, font types.Dict)) []byte {
	t.Helper()
	stamped, _, err := StampFields(threePagePDF(t), []Field{{Page: 1, Rect: [4]float64{50, 400, 300, 420}, Text: "them", Font: "Helvetica", Size: 12}})
	if err != nil {
		t.Fatal(err)
	}
	if mutate == nil {
		return stamped
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(stamped), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range ctx.Table {
		if e == nil {
			continue
		}
		d, isDict := e.Object.(types.Dict)
		if !isDict || d.NameEntry("Type") == nil || *d.NameEntry("Type") != "Font" {
			continue
		}
		if st := d.NameEntry("Subtype"); st == nil || *st != "Type0" {
			continue // the descendant CID font carries the same BaseFont
		}
		if bf := d.NameEntry("BaseFont"); bf != nil && strings.HasSuffix(*bf, "+LiberationSans") {
			mutate(ctx, d)
			found = true
		}
	}
	if !found {
		t.Fatal("setup: the first stamp carries no LiberationSans to mutate")
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// rawStamp is the stamp as pdfcpu's own wrapper does it, with no check — the stimulus each test below
// asserts is harmful before grading nib's answer.
func rawStamp(t *testing.T, pdf []byte, face string) ([]byte, error) {
	t.Helper()
	wm, err := api.TextWatermark("more", "fontname:"+face+", points:12, scalefactor:1 abs, position:bl, offset:60 300, rotation:0", true, false, types.POINTS)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = api.AddWatermarksSliceMap(bytes.NewReader(pdf), &out, map[int][]*model.Watermark{1: {wm}}, model.NewDefaultConfiguration())
	return out.Bytes(), err
}

func stampMore(t *testing.T, pdf []byte) []byte {
	t.Helper()
	out, _, err := StampFields(pdf, []Field{{Page: 1, Rect: [4]float64{50, 300, 300, 320}, Text: "more", Font: "Helvetica", Size: 12}})
	if err != nil {
		t.Fatalf("stamping a document that carries a LiberationSans failed — the bake answers 500: %v", err)
	}
	return out
}

// TestASecondStampKeepsEmbeddingTheFaceNibWrote — the check must not refuse nib's own fonts, or every
// second bake of a document degrades to Base-14.
func TestASecondStampKeepsEmbeddingTheFaceNibWrote(t *testing.T) {
	base := ownFontDocument(t, nil)
	if !carriesFace(t, base, "LiberationSans") {
		t.Fatal("setup: the document does not carry nib's LiberationSans, so there is nothing to reuse")
	}
	logged := captureLog(t)
	out := stampMore(t, base)
	if msg := logged.String(); strings.Contains(msg, "will not embed them") {
		t.Errorf("a second stamp refused the face nib itself wrote: %s", msg)
	}
	if missing := fontsNotEmbedded(t, out); len(missing) > 0 {
		t.Errorf("a second stamp draws unembedded fonts %v", missing)
	}
}

// TestAStampLeavesAFontItCannotVouchForAlone — the slice review's critical, in both of its shapes.
func TestAStampLeavesAFontItCannotVouchForAlone(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(ctx *model.Context, d types.Dict)
		harm   func(t *testing.T, fixture, raw []byte, rawErr error) bool
	}{
		{
			// LibreOffice's shape: a simple font, which pdfcpu refuses to rebuild.
			name:   "not an Identity-H font",
			mutate: func(_ *model.Context, d types.Dict) { d["Encoding"] = types.Name("WinAnsiEncoding") },
			harm:   func(_ *testing.T, _, _ []byte, rawErr error) bool { return rawErr != nil },
		},
		{
			// A producer whose glyph ids are not nib's: pdfcpu rebuilds it and the document's own map changes.
			name: "a glyph that is a different character",
			mutate: func(ctx *model.Context, d types.Dict) {
				sd, _, err := ctx.DereferenceStreamDict(d["ToUnicode"])
				if err != nil || sd == nil || sd.Decode() != nil {
					t.Fatal("setup: no ToUnicode to change")
				}
				changed := strings.Replace(string(sd.Content), "> <0074>", "> <0078>", 1) // t → x
				if changed == string(sd.Content) {
					t.Fatal("setup: the ToUnicode map has no 't' to change")
				}
				sd.Content = []byte(changed)
				if err := sd.Encode(); err != nil {
					t.Fatal(err)
				}
				ref := d["ToUnicode"].(types.IndirectRef)
				entry, _ := ctx.FindTableEntryForIndRef(&ref)
				entry.Object = *sd
			},
			harm: func(t *testing.T, fixture, raw []byte, rawErr error) bool {
				_, before, _ := faceStreams(t, fixture, "LiberationSans")
				_, after, _ := faceStreams(t, raw, "LiberationSans")
				return rawErr == nil && !bytes.Equal(before, after)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := ownFontDocument(t, tc.mutate)
			raw, rawErr := rawStamp(t, fixture, "LiberationSans")
			if !tc.harm(t, fixture, raw, rawErr) {
				t.Fatalf("setup: pdfcpu stamps this document without harm (err %v), so nib's check is not what is tested", rawErr)
			}
			programBefore, mapBefore, _ := faceStreams(t, fixture, "LiberationSans")
			logged := captureLog(t)
			out := stampMore(t, fixture)
			programAfter, mapAfter, ok := faceStreams(t, out, "LiberationSans")
			if !ok || !bytes.Equal(programBefore, programAfter) || !bytes.Equal(mapBefore, mapAfter) {
				t.Errorf("the document's own LiberationSans changed under a stamp (present %v)", ok)
			}
			if msg := logged.String(); !strings.Contains(msg, "already carries its own LiberationSans") {
				t.Errorf("the stamp fell back without saying why (log: %q)", msg)
			}
		})
	}
}

// TestAnUnforeseenEmbeddedFailureRetriesInCoreFaces — a failure on the embedded path that the reuse
// check did not foresee costs the stamp its face, never the save.
//
// **Driven through the door with an injected failure, because no document reaches it today.**
// Measured: every broken shape of a same-named font tried — no descendant, no descriptor, no program,
// a direct width array — makes pdfcpu build a fresh font instead of reusing, and the one it refuses
// (a direct ToUnicode stream) the check refuses first. The retry is the backstop for the shape nobody
// has found yet, so its contract is tested where it lives.
func TestAnUnforeseenEmbeddedFailureRetriesInCoreFaces(t *testing.T) {
	logged := captureLog(t)
	var attempts []bool
	out, err := stampTextWatermarks(threePagePDF(t), true, []string{"LiberationSans"}, func(ctx *model.Context, embedded bool) error {
		attempts = append(attempts, embedded)
		if embedded {
			return errors.New("pdfcpu: corrupt fontDict") // the shape the review reproduced
		}
		wm, werr := api.TextWatermark("more", "fontname:Helvetica, points:12, scalefactor:1 abs, position:bl, offset:60 300, rotation:0", true, false, types.POINTS)
		if werr != nil {
			return werr
		}
		return pdfcpu.AddWatermarksSliceMap(ctx, map[int][]*model.Watermark{1: {wm}})
	})
	// Stimulus first: the embedded attempt really ran and really failed.
	if len(attempts) == 0 || !attempts[0] {
		t.Fatalf("setup: the first attempt was not embedded (%v), so there was no embedded failure", attempts)
	}
	if err != nil {
		t.Fatalf("an embedded-face failure cost the user the stamp: %v", err)
	}
	if len(attempts) != 2 || attempts[1] {
		t.Errorf("attempts %v, want one embedded then one in core faces", attempts)
	}
	if len(out) == 0 || fontsNotEmbedded(t, out) == nil {
		t.Error("the retry produced no Base-14 stamp")
	}
	if msg := logged.String(); !strings.Contains(msg, "stamping in the embedded faces failed") {
		t.Errorf("the retry was silent (log: %q)", msg)
	}
}
