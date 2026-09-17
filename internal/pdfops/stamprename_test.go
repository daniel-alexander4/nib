package pdfops

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The stamp faces are installed under nib's own names — `/pending 494`, PDF/UA 7.21.4.1.

// officeDocumentRTF is a document in the shape the item is about: an office suite's own output,
// carrying its own subset of the very face nib stamps in. RTF because it names the font explicitly
// and converts through nib's own door (`ConvertOfficeToPDF`), and generated at test time rather than
// committed, as `richTaggedFixture` is and for the same reason.
const officeDocumentRTF = `{\rtf1\ansi\deff0{\fonttbl{\f0\fswiss Liberation Sans;}}
\f0\fs24 A contract paragraph of ordinary prose, produced by an office suite.\par
Second paragraph with more text so the page has real content to edit and stamp over.\par}`

func officeDocument(t *testing.T) []byte {
	t.Helper()
	if !LibreOfficeAvailable() {
		t.Skip("SKIP (not a pass): LibreOffice is absent, so no document carries a producer's own " +
			"Liberation and /pending 494's clause is UNCHECKED in this run.")
	}
	pdf, err := ConvertOfficeToPDF([]byte(officeDocumentRTF), "rtf")
	if err != nil {
		t.Skipf("SKIP (not a pass): LibreOffice could not build the fixture: %v", err)
	}
	return pdf
}

// upstreamStampFaces is every face `stampFaceFor` draws with, under UPSTREAM's name — the names a
// document from another producer can collide with.
func upstreamStampFaces() []string {
	out := make([]string, 0, len(stampFaceUpstream))
	for _, u := range stampFaceUpstream {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

// simpleFaceProgram returns the font program of the document's own SIMPLE TrueType font — what an
// office suite writes, and the thing pdfcpu would have rewritten.
func simpleFaceProgram(t *testing.T, pdf []byte) []byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	for _, e := range ctx.Table {
		if e == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok || d.NameEntry("Type") == nil || *d.NameEntry("Type") != "Font" {
			continue
		}
		if st := d.NameEntry("Subtype"); st == nil || *st != "TrueType" {
			continue
		}
		fd, _ := ctx.DereferenceDict(d["FontDescriptor"])
		if fd == nil {
			continue
		}
		sd, _, err := ctx.DereferenceStreamDict(fd["FontFile2"])
		if err != nil || sd == nil || sd.Decode() != nil {
			continue
		}
		return sd.Content
	}
	return nil
}

// TestAStampOnAnOfficeDocumentEmbedsItsFace is the clause itself, on the ua1 oracle — `/pending 494`.
//
// **Measured before the fix**: the converted document fails `5 t1 / 7.1 t9 / 7.1 t10`, and a stamp
// over it added `7.21.4.1 t1` and logged *"the document already carries its own LiberationSans"*.
// That refusal was correct — pdfcpu would have rebuilt the producer's font from nib's TTF — and it
// is what this test now asks to have become unnecessary rather than merely safe.
func TestAStampOnAnOfficeDocumentEmbedsItsFace(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so /pending 494's clause — an edit on an " +
			"office-suite document embeds the face it is drawn in — is UNCHECKED in this run.")
	}
	src := officeDocument(t)

	// Stimulus floor: this document really does carry a font that, under upstream's name, nib would
	// have had to refuse. Without it the assertions below pass on a document with nothing to collide.
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(src), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	face, why := unreusableFace(ctx, upstreamStampFaces())
	if face == "" {
		t.Fatal("setup: the converted document carries no font nib would have had to refuse under " +
			"upstream's name, so this measures a collision that is not there")
	}
	t.Logf("stimulus: the document carries its own %s — %s", face, why)

	before := simpleFaceProgram(t, src)
	if len(before) == 0 {
		t.Fatal("setup: the converted document embeds no simple TrueType program to compare")
	}

	logged := captureLog(t)
	out, _, err := StampFields(src, []Field{{Page: 1, Rect: [4]float64{60, 600, 400, 620}, Text: "Replaced text", Font: "Helvetica", Size: 12}})
	if err != nil {
		t.Fatalf("stamping an office-suite document failed: %v", err)
	}
	if msg := logged.String(); strings.Contains(msg, "will not embed them") {
		t.Errorf("the stamp fell back to Base-14 faces on an office-suite document: %s", msg)
	}
	if missing := fontsNotEmbedded(t, out); len(missing) > 0 {
		t.Errorf("the stamp draws with fonts it does not embed: %v (PDF/UA 7.21.4.1)", missing)
	}
	// The producer's own font is still the producer's own font: not rebuilt, not renamed, not
	// re-encoded. This is the harm the refusal existed to prevent, and removing the refusal must
	// not have re-admitted it.
	if after := simpleFaceProgram(t, out); !bytes.Equal(before, after) {
		t.Errorf("the document's own font program changed under a stamp (%d → %d bytes)", len(before), len(after))
	}

	dir := t.TempDir()
	in, stamped := filepath.Join(dir, "office.pdf"), filepath.Join(dir, "stamped.pdf")
	for f, b := range map[string][]byte{in: src, stamped: out} {
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := ua1FailedClauses(t, vp, []string{in, stamped})
	was, now := got["office.pdf"], got["stamped.pdf"]
	if was == nil || now == nil {
		t.Fatalf("veraPDF could not validate one of the two (input %v, output %v)", was != nil, now != nil)
	}
	if now["7.21.4.1 t1"] {
		t.Errorf("a stamp on an office-suite document still fails PDF/UA 7.21.4.1: %v", sortedClauses(now))
	}
	var added []string
	for c := range now {
		if !was[c] {
			added = append(added, c)
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		t.Errorf("the stamp adds ua1 clause(s) the office document did not fail: %v (input %v)", added, sortedClauses(was))
	}
}

// TestEveryStampFaceInstallsUnderTheNameNibAsksFor — the rename and the map have to agree, and the
// failure when they do not is a panic from inside pdfcpu several frames from the cause (mdpdf's
// `installFallbacks` says so where it refuses). One face renamed and the map left alone means every
// stamp silently draws Base-14 again.
func TestEveryStampFaceInstallsUnderTheNameNibAsksFor(t *testing.T) {
	model.NewDefaultConfiguration()
	orig := font.UserFontDir
	dir := t.TempDir()
	font.UserFontDir = dir
	t.Cleanup(func() { font.UserFontDir = orig })

	cores := make([]string, 0, len(stampFaceUpstream))
	for c := range stampFaceUpstream {
		cores = append(cores, c)
	}
	sort.Strings(cores)
	for _, core := range cores {
		upstream, want := stampFaceUpstream[core], stampFaceFor[core]
		if want == "" {
			t.Errorf("%s has no drawn face", core)
			continue
		}
		if strings.Contains(want, upstreamFaceToken) {
			t.Errorf("%s is drawn in %q, which still carries the name another producer writes", core, want)
		}
		f := stampFaceBytes(upstream)
		if f.Name != want || len(f.Data) == 0 {
			t.Errorf("%s: stampFaceBytes(%s) offers %q (%d bytes), the map asks for %q", core, upstream, f.Name, len(f.Data), want)
			continue
		}
		// pdfcpu names the .gob by the PostScript name it reads OUT of the bytes, so this is the
		// only check that the rename and the map agree about what is inside the file.
		if err := font.InstallFontFromBytes(dir, f.Name, f.Data); err != nil {
			t.Errorf("%s: install %s: %v", core, f.Name, err)
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, want+".gob")); err != nil {
			t.Errorf("%s: %s installed under some other name: %v", core, want, err)
		}
		if _, err := os.Stat(filepath.Join(dir, upstream+".gob")); err == nil {
			t.Errorf("%s: the face installed under upstream's name %s, which is the collision this removes", core, upstream)
		}
	}
}

// TestTheRenamedStampFaceIsTheSameFace is the proof the rename owes: upstream's glyphs, widths and
// character map, under a different name. Asserted twice over — byte for byte on the tables, and
// field for field on what pdfcpu parses out of them, because the second is what every measurement
// nib makes actually reads.
func TestTheRenamedStampFaceIsTheSameFace(t *testing.T) {
	model.NewDefaultConfiguration()
	orig := font.UserFontDir
	dir := t.TempDir()
	font.UserFontDir = dir
	t.Cleanup(func() { font.UserFontDir = orig })

	for _, upstream := range upstreamStampFaces() {
		file := upstream
		if !strings.Contains(file, "-") {
			file += "-Regular"
		}
		src, err := ocrFontFS.ReadFile("fonts/" + file + ".ttf")
		if err != nil {
			t.Fatalf("%s: %v", upstream, err)
		}
		renamed, err := renamedFace(src)
		if err != nil {
			t.Fatalf("%s: %v", upstream, err)
		}

		was, _, err := sfntTables(src)
		if err != nil {
			t.Fatalf("%s: %v", upstream, err)
		}
		now, _, err := sfntTables(renamed)
		if err != nil {
			t.Fatalf("%s: %v", upstream, err)
		}
		if len(was) != len(now) {
			t.Errorf("%s: %d tables became %d", upstream, len(was), len(now))
		}
		for tag, wb := range was {
			nb, ok := now[tag]
			if !ok {
				t.Errorf("%s: table %q was dropped", upstream, tag)
				continue
			}
			switch tag {
			case "name":
				if bytes.Equal(wb, nb) {
					t.Errorf("%s: the name table did not change, so nothing was renamed", upstream)
				}
			case "head":
				// `checkSumAdjustment` is a checksum OF the file and so must move; the rest of head
				// — unitsPerEm, indexToLocFormat, the bounding box — must not.
				wz, nz := append([]byte(nil), wb...), append([]byte(nil), nb...)
				copy(wz[8:12], make([]byte, 4))
				copy(nz[8:12], make([]byte, 4))
				if !bytes.Equal(wz, nz) {
					t.Errorf("%s: head changed beyond its checkSumAdjustment", upstream)
				}
			default:
				if !bytes.Equal(wb, nb) {
					t.Errorf("%s: table %q changed (%d → %d bytes) — the rename is not confined to the name",
						upstream, tag, len(wb), len(nb))
				}
			}
		}

		// And what pdfcpu reads: install both and compare the parsed metrics field for field.
		if err := font.InstallFontFromBytes(dir, upstream, src); err != nil {
			t.Fatalf("%s: install upstream: %v", upstream, err)
		}
		if err := font.InstallFontFromBytes(dir, nibFaceName(upstream), renamed); err != nil {
			t.Fatalf("%s: install renamed: %v", upstream, err)
		}
		a, b := installedMetrics(t, dir, upstream), installedMetrics(t, dir, nibFaceName(upstream))
		if a.PostscriptName != upstream || b.PostscriptName != nibFaceName(upstream) {
			t.Errorf("%s: pdfcpu read the names as %q and %q", upstream, a.PostscriptName, b.PostscriptName)
		}
		a.PostscriptName, b.PostscriptName = "", ""
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: the renamed face's metrics differ from upstream's beyond the name "+
				"(widths equal: %v, cmap equal: %v, glyph→unicode equal: %v)", upstream,
				reflect.DeepEqual(a.GlyphWidths, b.GlyphWidths),
				reflect.DeepEqual(a.Chars, b.Chars),
				reflect.DeepEqual(a.ToUnicode, b.ToUnicode))
		}
	}
}

// installedMetrics gob-decodes what pdfcpu's installer wrote, which is where every width nib
// measures with comes from.
func installedMetrics(t *testing.T, dir, name string) font.TTFLight {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, name+".gob"))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	defer f.Close()
	var ttf font.TTFLight
	if err := gob.NewDecoder(f).Decode(&ttf); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return ttf
}

// TestTheRenameKeepsTheCopyrightTrademarkAndLicenceRecords — Liberation is OFL 1.1 with Reserved
// Font Name "Liberation" (`build/gen-notices.sh`), so §3 wants the NAME off a modified version and
// §2 wants the notice kept ON it. Those are opposite requirements over one table, and a replacement
// applied to the whole table would satisfy the first by breaking the second: name record 7 reads
// *"Liberation is a trademark of Red Hat, Inc."*, and a blanket rewrite turns it into a sentence
// about a trademark that does not exist.
func TestTheRenameKeepsTheCopyrightTrademarkAndLicenceRecords(t *testing.T) {
	src, err := ocrFontFS.ReadFile("fonts/LiberationSans-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := renamedFace(src)
	if err != nil {
		t.Fatal(err)
	}
	was, now := nameRecords(t, src), nameRecords(t, renamed)
	if len(was) != len(now) {
		t.Fatalf("%d name records became %d", len(was), len(now))
	}
	// **The two sets are written out here rather than read from `renameableNameIDs`.** That map is
	// the thing under test: a test that asked it which records are names would agree with it however
	// it was edited, and moving 7 into it would make this pass while the trademark line was rewritten.
	protected := map[uint16]string{0: "the copyright notice", 7: "the trademark line", 13: "the licence", 14: "the licence URL"}
	renamedIDs := map[uint16]string{1: "the family name", 3: "the unique id", 4: "the full name", 6: "the PostScript name"}

	for id, what := range protected {
		// Stimulus floor: a record that does not carry the token cannot show a rewrite that reached
		// it, so "unchanged" would be true of a blanket replacement too.
		if !strings.Contains(was[id], upstreamFaceToken) {
			continue
		}
		if now[id] != was[id] {
			t.Errorf("name record %d, %s, was rewritten: %q → %q", id, what, was[id], now[id])
		}
	}
	if !strings.Contains(was[7], upstreamFaceToken) {
		t.Fatalf("setup: the trademark record does not mention %q, so this protects nothing", upstreamFaceToken)
	}
	for id, what := range renamedIDs {
		if !strings.Contains(was[id], upstreamFaceToken) {
			t.Fatalf("setup: name record %d, %s, is %q and carries no name to change", id, what, was[id])
		}
		if strings.Contains(now[id], upstreamFaceToken) {
			t.Errorf("name record %d, %s, still reads %q", id, what, now[id])
		}
	}
	if got := now[6]; got != nibFaceName("LiberationSans") {
		t.Errorf("PostScript name is %q, want %q", got, nibFaceName("LiberationSans"))
	}
}

// nameRecords reads the Windows-platform name records of a TTF, keyed by name id.
func nameRecords(t *testing.T, ttf []byte) map[uint16]string {
	t.Helper()
	tables, _, err := sfntTables(ttf)
	if err != nil {
		t.Fatal(err)
	}
	nt := tables["name"]
	count := int(be16(nt[2:]))
	storage := int(be16(nt[4:]))
	out := map[uint16]string{}
	for i := 0; i < count; i++ {
		r := nt[6+i*12:]
		if be16(r) != 3 {
			continue // the Macintosh copies say the same thing in one byte per character
		}
		id := be16(r[6:])
		length, offset := int(be16(r[8:])), int(be16(r[10:]))
		s := nt[storage+offset : storage+offset+length]
		units := make([]uint16, len(s)/2)
		for j := range units {
			units[j] = be16(s[2*j:])
		}
		out[id] = string(utf16.Decode(units))
	}
	return out
}

func be16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
