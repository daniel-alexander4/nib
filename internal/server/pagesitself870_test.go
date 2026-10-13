package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfops"
	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// `/pending 870` part 5, the app's half. Deleting a page and reordering pages return the document the user has
// open, with a page gone or its pages moved. Driven through `/api/pages` before the change, on a three-page
// document with a title, an `/ID`, one embedded file and a metadata packet naming its creator:
//
//	          files   /Info title   /ID[0]   packet
//	delete    kept    none          new      none
//	reorder   kept    none          new      none
//
// The files were kept by a second read and write of both documents (`pdfops.CarryAttachments`); nothing kept the
// rest. `Append`, `InsertBlank`, `DuplicatePage`, `InsertPDF` and `SplitPage` through the same route keep all four.

// describedFacts is what describes a document whatever order its pages are in.
type describedFacts struct {
	files  string
	title  string
	id     string
	packet string
	pages  int
}

func describedFactsOf(t *testing.T, pdf []byte) describedFacts {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	var f describedFacts
	f.pages = ctx.PageCount
	att, err := pdfops.Attachments(pdf)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range att {
		f.files += a.Name + " "
	}
	f.files = strings.TrimSpace(f.files)
	if ctx.Info != nil {
		if d, derr := ctx.XRefTable.DereferenceDict(*ctx.Info); derr == nil && d != nil {
			switch s := d["Title"].(type) {
			case types.StringLiteral:
				f.title, _ = types.StringLiteralToString(s)
			case types.HexLiteral:
				f.title, _ = types.HexLiteralToString(s)
			}
		}
	}
	if len(ctx.ID) > 0 {
		f.id = ctx.ID[0].String()
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if o, ok := root["Metadata"]; ok {
		sd, _, derr := ctx.XRefTable.DereferenceStreamDict(o)
		if derr != nil || sd == nil {
			t.Fatalf("/Metadata does not resolve: %v", derr)
		}
		if derr := sd.Decode(); derr != nil {
			t.Fatal(derr)
		}
		f.packet = string(sd.Content)
	}
	return f
}

// describedFixture is a three-page document with a title, a file, an `/ID` and a packet that says more than its
// title (who made it).
func describedFixture(t *testing.T) []byte {
	t.Helper()
	doc, err := testpdf.Text("alpha", "beta", "gamma")
	if err != nil {
		t.Fatal(err)
	}
	if doc, err = pdfops.AddAttachment(doc, "record.txt", []byte("the record")); err != nil {
		t.Fatal(err)
	}
	if doc, err = pdfops.SetTitle(doc, "Lease"); err != nil {
		t.Fatal(err)
	}
	ctx, err := pdfread.ReadOptimized(doc, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	ir, ok := root["Metadata"].(types.IndirectRef)
	if !ok {
		t.Fatal("setup: a titled document has no indirect /Metadata")
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(ir)
	if err != nil || sd == nil {
		t.Fatalf("setup: /Metadata does not resolve: %v", err)
	}
	if err := sd.Decode(); err != nil {
		t.Fatal(err)
	}
	const made = `<rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/"><xmp:CreatorTool>Quill</xmp:CreatorTool></rdf:Description>`
	if !strings.Contains(string(sd.Content), "</rdf:RDF>") {
		t.Fatal("setup: the title packet has no rdf:RDF")
	}
	sd.Content = []byte(strings.Replace(string(sd.Content), "</rdf:RDF>", made+"</rdf:RDF>", 1))
	if err := sd.Encode(); err != nil {
		t.Fatal(err)
	}
	entry, found := ctx.XRefTable.FindTableEntryForIndRef(&ir)
	if !found {
		t.Fatal("setup: no xref entry for /Metadata")
	}
	entry.Object = *sd
	out, err := pdfread.Write(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestDeletingOrReorderingPagesKeepsWhatDescribesTheDocument — through the route, read off the document the
// server then holds.
func TestDeletingOrReorderingPagesKeepsWhatDescribesTheDocument(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	src := describedFixture(t)
	want := describedFactsOf(t, src)
	if want.files != "record.txt" || want.title != "Lease" || want.id == "" || !strings.Contains(want.packet, "Quill") || want.pages != 3 {
		t.Fatalf("setup: the fixture reads %+v", want)
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: writeTemp(t, src)}); code != http.StatusOK {
		t.Fatalf("setup: open returned %d: %s", code, body)
	}
	for _, op := range []struct {
		name, pages string
		left        int
	}{{"delete", "2", 2}, {"reorder", "3,1,2", 3}} {
		got := describedFactsOf(t, pagesOp(t, ts, c, csrf, src, op.name, op.pages).data)
		if got.pages != op.left {
			t.Errorf("%s: %d pages, want %d", op.name, got.pages, op.left)
		}
		if got.files != want.files {
			t.Errorf("%s: embedded files %q, want %q", op.name, got.files, want.files)
		}
		if got.title != want.title {
			t.Errorf("%s: /Info title %q, want %q — the document is the one that was open, less a page or reordered", op.name, got.title, want.title)
		}
		if got.id != want.id {
			t.Errorf("%s: /ID[0] %s, want the document's own %s — an edit of a file keeps its permanent identifier", op.name, got.id, want.id)
		}
		if !strings.Contains(got.packet, "Quill") || !strings.Contains(got.packet, "Lease") {
			t.Errorf("%s: the metadata packet reads %q, want the document's own", op.name, got.packet)
		}
	}
}

// TestDeletingOrReorderingPagesDropsAConformanceClaim — ADR-032: the packet the two operations now keep is the
// document's own, and a labelled document's said PDF/UA. The identification goes; the rest of the packet stays.
func TestDeletingOrReorderingPagesDropsAConformanceClaim(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	src, err := testpdf.WithUAIdentification(describedFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if !testpdf.PacketClaimsUA(describedFactsOf(t, src).packet) {
		t.Fatal("setup: the labelled fixture's packet does not claim PDF/UA")
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: writeTemp(t, src)}); code != http.StatusOK {
		t.Fatalf("setup: open returned %d: %s", code, body)
	}
	for _, op := range [][2]string{{"delete", "2"}, {"reorder", "3,1,2"}} {
		p := describedFactsOf(t, pagesOp(t, ts, c, csrf, src, op[0], op[1]).data).packet
		if testpdf.PacketClaimsUA(p) {
			t.Errorf("%s: the packet kept still claims PDF/UA — any change drops the identification", op[0])
		}
		if !strings.Contains(p, "Quill") {
			t.Errorf("%s: the packet reads %q, want the rest of the document's own packet kept", op[0], p)
		}
	}
}
