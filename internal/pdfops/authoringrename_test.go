package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/mdpdf"
)

// upstreamAuthoringFaces is the set nib authored in before `/pending 560`, and the set any other producer that
// embeds Roboto writes: upstream's bytes under upstream's names.
func upstreamAuthoringFaces(t *testing.T) *mdpdf.Faces {
	t.Helper()
	read := func(name, file string) mdpdf.Font {
		bb, err := ocrFontFS.ReadFile("fonts/" + file)
		if err != nil {
			t.Fatal(err)
		}
		return mdpdf.Font{Name: name, Data: bb}
	}
	return &mdpdf.Faces{
		Body: read("Roboto-Regular", "Roboto-Regular.ttf"), Bold: read("Roboto-Bold", "Roboto-Bold.ttf"),
		Italic: read("Roboto-Italic", "Roboto-Italic.ttf"), BoldItalic: read("Roboto-BoldItalic", "Roboto-BoldItalic.ttf"),
		Code: read("LiberationMono", "LiberationMono-Regular.ttf"),
	}
}

// TestTheAuthoringFacesInstallUnderNibsNames — `/pending 560`, ADR-044's declared gap. pdfcpu names a face by the
// PostScript name INSIDE the bytes, so the table's key, the offered name and the installed `.gob` must all be the
// one name — and it must not be a name another producer writes.
func TestTheAuthoringFacesInstallUnderNibsNames(t *testing.T) {
	model.NewDefaultConfiguration()
	orig := font.UserFontDir
	dir := t.TempDir()
	font.UserFontDir = dir
	t.Cleanup(func() { font.UserFontDir = orig })

	faces := authoringFaces()
	offered := map[string]mdpdf.Font{}
	for _, f := range []mdpdf.Font{faces.Body, faces.Bold, faces.Italic, faces.BoldItalic, faces.Code} {
		offered[f.Name] = f
	}
	if len(authoringFontFiles) != 5 || len(offered) != 5 {
		t.Fatalf("authoringFontFiles has %d faces and authoringFaces offers %d distinct names, want 5 and 5", len(authoringFontFiles), len(offered))
	}
	for name, file := range authoringFontFiles {
		for _, upstream := range []string{authoringUpstreamToken, upstreamFaceToken} {
			if strings.Contains(name, upstream) {
				t.Errorf("%s still carries %q, a name another producer's document carries too", name, upstream)
			}
		}
		f, ok := offered[name]
		if !ok || len(f.Data) == 0 {
			t.Errorf("%s is in authoringFontFiles and authoringFaces does not offer it (or offers no bytes)", name)
			continue
		}
		if err := font.InstallFontFromBytes(dir, f.Name, f.Data); err != nil {
			t.Errorf("install %s: %v", name, err)
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name+".gob")); err != nil {
			t.Errorf("%s installed under some other name — the name in the bytes is not the table's: %v", name, err)
		}
		was := strings.TrimSuffix(strings.TrimPrefix(file, "fonts/"), ".ttf")
		if strings.HasPrefix(was, "LiberationMono") {
			was = "LiberationMono"
		}
		if _, err := os.Stat(filepath.Join(dir, was+".gob")); err == nil {
			t.Errorf("%s installed under upstream's name %s, which is the collision this removes", name, was)
		}
	}
	body, bold, embedded := AuthoredTextFaces()
	if !embedded || body != "NibText-Regular" || bold != "NibText-Bold" {
		t.Errorf("AuthoredTextFaces = %q, %q, embedded=%v; want NibText-Regular, NibText-Bold, true", body, bold, embedded)
	}
}

// TestAConversionIsSetInNibsFacesAndNoOneElses — what a Markdown conversion's fonts are called now.
func TestAConversionIsSetInNibsFacesAndNoOneElses(t *testing.T) {
	out, err := mdpdfRaw(t)
	if err != nil {
		t.Fatal(err)
	}
	if !carriesFace(t, out, "NibText-Regular") {
		t.Error("a Markdown conversion does not carry NibText-Regular")
	}
	for _, upstream := range []string{"Roboto-Regular", "Roboto-Bold", "LiberationMono"} {
		if carriesFace(t, out, upstream) {
			t.Errorf("a Markdown conversion still carries %s", upstream)
		}
	}
}

// TestAWatermarkOnADocumentCarryingRobotoKeepsItsEmbeddedFace — the defect itself. A document that carries its own
// `Roboto-Regular`, in a shape nib's face does not describe (LibreOffice's: a font pdfcpu will not rebuild), met
// pdfcpu's by-name match on the face nib watermarks in; nib refused the reuse and drew the stamp in Base-14, which
// is not embedded (PDF/UA 7.21.4.1). Under nib's own name there is nothing in the document to collide with.
func TestAWatermarkOnADocumentCarryingRobotoKeepsItsEmbeddedFace(t *testing.T) {
	host, err := mdpdf.ConvertWithFaces([]byte(p4Markdown), upstreamAuthoringFaces(t), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(host), model.NewDefaultConfiguration())
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
			continue
		}
		if bf := d.NameEntry("BaseFont"); bf != nil && strings.HasSuffix(*bf, "+Roboto-Regular") {
			d["Encoding"] = types.Name("WinAnsiEncoding") // not a font pdfcpu can rebuild from nib's bytes
			found = true
		}
	}
	if !found {
		t.Fatal("setup: the host carries no Roboto-Regular, so nothing in it can collide with a face of that name")
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	foreign := buf.Bytes()
	if missing := fontsNotEmbedded(t, foreign); len(missing) > 0 {
		t.Fatalf("setup: the host already draws with unembedded fonts %v", missing)
	}

	logged := captureLog(t)
	for name, stamp := range map[string]func([]byte) ([]byte, error){
		"StampWatermark":   func(b []byte) ([]byte, error) { return StampWatermark(b, "DRAFT", WatermarkStyle{}) },
		"StampPageNumbers": func(b []byte) ([]byte, error) { return StampPageNumbers(b, PageNumberStyle{OfTotal: true}) },
	} {
		logged.Reset()
		out, err := stamp(foreign)
		if err != nil {
			t.Errorf("%s on a document carrying its own Roboto: %v", name, err)
			continue
		}
		if missing := fontsNotEmbedded(t, out); len(missing) > 0 {
			t.Errorf("%s on a document carrying its own Roboto drew with unembedded fonts %v — the stamp fell "+
				"back to Base-14 because the document's font has the name of nib's (log: %q)", name, missing, logged.String())
		}
		if !carriesFace(t, out, "NibText-Regular") {
			t.Errorf("%s did not draw in NibText-Regular", name)
		}
	}
}
