package pdfops

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"nib/internal/pdfread"
)

// inspectFixture is a one-page document with the given /Info entries (raw PDF syntax, e.g. `/Author (x)`), an
// uncompressed XMP packet when xmp is not empty, and one non-embedded Helvetica as /F1.
func inspectFixture(info string, xmp []byte) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	catalog := "<< /Type /Catalog /Pages 2 0 R >>"
	if len(xmp) > 0 {
		catalog = "<< /Type /Catalog /Pages 2 0 R /Metadata 7 0 R >>"
	}
	content := "BT /F1 12 Tf 72 700 Td (hello) Tj ET"
	objs := []string{
		catalog,
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< " + info + " >>",
	}
	if len(xmp) > 0 {
		objs = append(objs, fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(xmp), xmp))
	}
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R /Info 6 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

func infoOf(t *testing.T, got Inspection, key string) InfoEntry {
	t.Helper()
	for _, e := range got.Info {
		if e.Key == key {
			return e
		}
	}
	t.Fatalf("no %s in %+v", key, got.Info)
	return InfoEntry{}
}

// The inspector shows every /Info key, not Scan's five: Producer and the dates are what a forensic reader asks first.
// A value the document chose is cleaned and the count of what was taken is said — an Author carrying U+202E would
// otherwise read reversed — and a name-valued entry (/Trapped) is shown as the name it is.
func TestInspectShowsEveryInfoKeyCleanedAndSaysWhatCleaningTook(t *testing.T) {
	// UTF-16BE with a BOM: "Jane" U+202E "eoD" — the override that makes it read "Jane Doe" backwards from there.
	author := "<FEFF004A0061006E0065202E0065006F0044>"
	pdf := inspectFixture("/Author "+author+" /Producer (Acme PDF 9.1) /CreationDate (D:20240101120000Z) /Trapped /False", nil)
	got, err := InspectDocument(pdf)
	if err != nil {
		t.Fatal(err)
	}
	a := infoOf(t, got, "Author")
	if a.Value != "JaneeoD" || a.Removed != 1 {
		t.Errorf("Author = %q (removed %d), want the override dropped and counted: \"JaneeoD\", 1", a.Value, a.Removed)
	}
	if p := infoOf(t, got, "Producer"); p.Value != "Acme PDF 9.1" {
		t.Errorf("Producer = %q, want Acme PDF 9.1 (Scan never showed it)", p.Value)
	}
	if c := infoOf(t, got, "CreationDate"); c.Value != "D:20240101120000Z" {
		t.Errorf("CreationDate = %q", c.Value)
	}
	if tr := infoOf(t, got, "Trapped"); tr.Value != "False" || tr.NotText {
		t.Errorf("Trapped = %+v, want the name shown", tr)
	}
	if len(got.Info) != 4 {
		t.Errorf("got %d info rows, want 4", len(got.Info))
	}
}

// A long value is cut at the cap and SAYS it was cut.
func TestInspectCutsALongInfoValueAndSaysSo(t *testing.T) {
	long := strings.Repeat("x", inspectMaxValue+50)
	got, err := InspectDocument(inspectFixture("/Subject ("+long+")", nil))
	if err != nil {
		t.Fatal(err)
	}
	s := infoOf(t, got, "Subject")
	if len([]rune(s.Value)) != inspectMaxValue || !s.Cut {
		t.Errorf("Subject is %d characters, cut=%v; want %d and cut", len([]rune(s.Value)), s.Cut, inspectMaxValue)
	}
}

// The XMP packet is shown as text, and one past the display cap is shown in part and said to be.
func TestInspectShowsTheXMPPacketAndCutsALargeOne(t *testing.T) {
	small := []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><dc:creator>Jane Doe</dc:creator></x:xmpmeta>`)
	got, err := InspectDocument(inspectFixture("/Title (t)", small))
	if err != nil {
		t.Fatal(err)
	}
	if !got.XMP.Present || got.XMP.Text != string(small) || got.XMP.Cut {
		t.Errorf("XMP = %+v, want the whole packet, not cut", got.XMP)
	}
	big := append([]byte("<x:xmpmeta>"), bytes.Repeat([]byte("y"), inspectMaxXMP+100)...)
	got, err = InspectDocument(inspectFixture("/Title (t)", big))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.XMP.Text) != inspectMaxXMP || !got.XMP.Cut {
		t.Errorf("a %d-byte packet showed %d bytes, cut=%v; want %d and cut", len(big), len(got.XMP.Text), got.XMP.Cut, inspectMaxXMP)
	}
	got, err = InspectDocument(inspectFixture("/Title (t)", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.XMP.Present {
		t.Errorf("a document with no packet reported one: %+v", got.XMP)
	}
}

// A packet that decodes past the decode cap is not decoded whole into memory and is said to be too large.
func TestInspectRefusesToDecodeAnOversizedPacket(t *testing.T) {
	huge := bytes.Repeat([]byte("z"), pdfread.MaxPacketBytes+10)
	got, err := InspectDocument(inspectFixture("/Title (t)", huge))
	if err != nil {
		t.Fatal(err)
	}
	if !got.XMP.TooLarge || got.XMP.Text != "" {
		t.Errorf("XMP = present %v tooLarge %v with %d bytes of text; want too large, nothing shown", got.XMP.Present, got.XMP.TooLarge, len(got.XMP.Text))
	}
}

// The fonts are listed with whether each is embedded: an unembedded Helvetica says so.
func TestInspectListsFontsAndWhetherEachIsEmbedded(t *testing.T) {
	got, err := InspectDocument(inspectFixture("/Title (t)", nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fonts) != 1 || got.Fonts[0] != (FontRow{Name: "Helvetica", Subtype: "Type1", Embedded: false}) {
		t.Errorf("fonts = %+v, want one unembedded Type1 Helvetica", got.Fonts)
	}
}
