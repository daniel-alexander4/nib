package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// `/pending 870`: what a page operation whose output IS the document still took from it. Driven before the change,
// on `spliceHost` with a metadata packet added (title, an attachment, an `/ID`, a packet naming its creator):
//
//	                          files   /Info title   /ID[0]   packet
//	DuplicatePage             none    none          new      none
//	InsertPDF, SplitPage      kept    kept          kept     none      (kept since /pending 602, the packet not)
//	spliceWithoutStructure    host's none, none     new      none
//	Append, InsertBlank       kept    kept          kept     kept
//	Collect, RemovePages      none    none          new      none      — and still: see the control below

// packetOf is the catalog packet of pdf, or "" where it has none.
func packetOf(t *testing.T, pdf []byte) string {
	t.Helper()
	p, _ := packetIn(t, readCtx(t, pdf))
	return p
}

// withPacket is spliceHost carrying a metadata packet that says more than the title.
func withPacket(t *testing.T) []byte {
	t.Helper()
	out, err := writeMutated(spliceHost(t), func(ctx *model.Context) error {
		root, err := ctx.XRefTable.Catalog()
		if err != nil {
			return err
		}
		sd, err := ctx.NewStreamDictForBuf([]byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF ` +
			`xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description ` +
			`xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
			`<xmp:CreatorTool>Quill</xmp:CreatorTool><dc:title><rdf:Alt><rdf:li xml:lang="x-default">Lease` +
			`</rdf:li></rdf:Alt></dc:title></rdf:Description></rdf:RDF></x:xmpmeta>`))
		if err != nil {
			return err
		}
		sd.InsertName("Type", "Metadata")
		sd.InsertName("Subtype", "XML")
		if err := sd.Encode(); err != nil {
			return err
		}
		ir, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		root["Metadata"] = *ir
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAnOperationWhoseOutputIsTheDocumentKeepsWhatDescribesIt — a duplicated page, an inserted document, a split
// page and the splice's last resort each return the document they were given, with its title, its `/ID`, its files
// and its packet. The last resort is driven with the insertion BEFORE page 1, where its old shape made the inserted
// document the host.
func TestAnOperationWhoseOutputIsTheDocumentKeepsWhatDescribesIt(t *testing.T) {
	host := withPacket(t)
	wantFiles, wantTitle, wantID, _ := documentFactsOf(t, host)
	if len(wantFiles) != 1 || wantTitle != "Lease" || wantID == "" || !strings.Contains(packetOf(t, host), "Quill") {
		t.Fatalf("setup: the fixture carries files %v, title %q, id %q and packet %q", wantFiles, wantTitle, wantID, packetOf(t, host))
	}
	other, err := testpdf.Text("inserted")
	if err != nil {
		t.Fatal(err)
	}
	if other, err = SetTitle(other, "Theirs"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		run   func() ([]byte, error)
		pages int
	}{
		{"DuplicatePage", func() ([]byte, error) { return DuplicatePage(host, 2) }, 4},
		{"InsertPDF", func() ([]byte, error) { return InsertPDF(host, other, 2, true) }, 4},
		{"SplitPage", func() ([]byte, error) { return withoutCount(SplitPage(host, 2, 2, 1, false)) }, 4},
		{"spliceWithoutStructure before page 1", func() ([]byte, error) { return withoutWithheld(spliceWithoutStructure(host, 0, 1, 3, other)) }, 4},
	} {
		out, err := c.run()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if n, _ := PageCount(out); n != c.pages {
			t.Errorf("%s: %d pages, want %d", c.name, n, c.pages)
		}
		files, title, id, _ := documentFactsOf(t, out)
		if strings.Join(files, " ") != "record.txt" {
			t.Errorf("%s: embedded files %v, want the document's own record.txt", c.name, files)
		}
		if title != wantTitle {
			t.Errorf("%s: /Info title %q, want %q", c.name, title, wantTitle)
		}
		if id != wantID {
			t.Errorf("%s: /ID[0] %s, want the document's own %s — an edit of a file keeps its permanent identifier", c.name, id, wantID)
		}
		if p := packetOf(t, out); !strings.Contains(p, "Quill") || !strings.Contains(p, "Lease") {
			t.Errorf("%s: the metadata packet reads %q, want the document's own — /Info kept its title, and the two "+
				"describe one document", c.name, p)
		}
	}
}

// TestADuplicateThatLosesItsTreeIsStillTheDocument — `subsetCarrying`'s second attempt. The fixture is
// `TestTheOutputGateFallsBackToTheHonestLoss`'s: a page drawing a form that holds the marked content, so the
// duplicate draws it twice, the gate refuses the carry and the selection runs again with no tree. That second
// write is the same document too.
func TestADuplicateThatLosesItsTreeIsStillTheDocument(t *testing.T) {
	form := "/P <</MCID 0>> BDC\nBT /F1 18 Tf 72 700 Td (ZZINFORM) Tj ET\nEMC\n"
	src, err := SetTitle(assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 30 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
			"/Resources << /XObject << /Fm1 12 0 R >> /Font << /F1 11 0 R >> >> /Contents 4 0 R >>",
		4:  streamObj("q 1 0 0 1 0 0 cm /Fm1 Do Q\n"),
		11: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		12: fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /StructParents 0 "+
			"/Resources << /Font << /F1 11 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(form), form),
		30: "<< /Type /StructTreeRoot /K [31 0 R] /ParentTree 39 0 R >>",
		31: "<< /Type /StructElem /S /P /P 30 0 R /Pg 3 0 R /K [0] >>",
		39: "<< /Nums [0 [31 0 R]] >>",
	}), "Lease")
	if err != nil {
		t.Fatal(err)
	}
	_, _, wantID, _ := documentFactsOf(t, src)
	out, err := DuplicatePage(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s := inspectTags(out); s.tree || s.marked || !inspectTags(src).tree {
		t.Fatalf("setup: the duplicate kept its tree (tree=%v marked=%v), so the second attempt never ran", s.tree, s.marked)
	}
	if _, title, id, _ := documentFactsOf(t, out); title != "Lease" || id != wantID {
		t.Errorf("the duplicate written without its tree has title %q and /ID[0] %s, want Lease and the document's own %s",
			title, id, wantID)
	}
}

// TestASelectionPrimitiveStillWritesAnExtract — the control, and `CarryAttachments`' recorded decision: `Collect`
// and `RemovePages` are what an extract, a split and an export go through, so neither keeps the document's facts
// on its own. A new file made from some pages is a new document: no title, no files, and an `/ID` of its own.
func TestASelectionPrimitiveStillWritesAnExtract(t *testing.T) {
	host := withPacket(t)
	_, _, hostID, _ := documentFactsOf(t, host)
	for name, run := range map[string]func() ([]byte, error){
		"Collect":     func() ([]byte, error) { return Collect(host, []string{"3", "1", "2"}) },
		"RemovePages": func() ([]byte, error) { return RemovePages(host, []string{"2"}) },
	} {
		out, err := run()
		if err != nil {
			t.Fatal(err)
		}
		files, title, id, _ := documentFactsOf(t, out)
		if len(files) != 0 || title != "" || id == hostID || packetOf(t, out) != "" {
			t.Errorf("%s carried files %v, title %q, the document's /ID (%v) or its packet (%q) — whether a selection "+
				"is the document or an extract of it is its caller's to say", name, files, title, id == hostID, packetOf(t, out))
		}
	}
}

// TestAPacketPutBackNeverBringsAClaim — ADR-032. The packet a duplicate or an insert keeps is the one the document
// had, and a labelled document's said PDF/UA: the identification must be gone from what is put back, and the rest
// of the packet must not be.
func TestAPacketPutBackNeverBringsAClaim(t *testing.T) {
	labelled := labelledFixture(t)
	if !testpdf.PacketClaimsUA(packetOf(t, labelled)) {
		t.Fatal("setup: the labelled fixture's packet does not claim PDF/UA")
	}
	other, err := testpdf.Text("inserted")
	if err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]func() ([]byte, error){
		"DuplicatePage":          func() ([]byte, error) { return DuplicatePage(labelled, 1) },
		"InsertPDF":              func() ([]byte, error) { return InsertPDF(labelled, other, 1, false) },
		"spliceWithoutStructure": func() ([]byte, error) { return withoutWithheld(spliceWithoutStructure(labelled, 1, 2, 1, other)) },
	} {
		out, err := run()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		p := packetOf(t, out)
		if testpdf.PacketClaimsUA(p) {
			t.Errorf("%s: the packet put back still claims PDF/UA — any change drops the identification", name)
		}
		if !strings.Contains(p, "Census") {
			t.Errorf("%s: the packet reads %q, want the document's own title still in it", name, p)
		}
	}
	// And the last resort carries no tree, whatever the host had: that is all it is for.
	out, _, err := spliceWithoutStructure(labelled, 1, 2, 1, other)
	if err != nil {
		t.Fatal(err)
	}
	if !inspectTags(labelled).claims() || inspectTags(out).claims() {
		t.Errorf("the splice's last resort wrote a document that claims tagging (host claims: %v) — it exists to "+
			"write none", inspectTags(labelled).claims())
	}
}

// TestDeletingOrReorderingPagesIsTheSameDocument — the app's delete and reorder (`sameDocument`): the document
// with a page gone or its pages moved keeps its title, its `/ID`, its files and its packet, in one write.
func TestDeletingOrReorderingPagesIsTheSameDocument(t *testing.T) {
	host := withPacket(t)
	_, wantTitle, wantID, _ := documentFactsOf(t, host)
	for _, c := range []struct {
		name  string
		run   func() ([]byte, int, error)
		pages int
	}{
		{"DeletePages", func() ([]byte, int, error) { return DeletePages(host, []string{"2"}) }, 2},
		{"ReorderPages", func() ([]byte, int, error) { return ReorderPages(host, []string{"3", "1", "2"}) }, 3},
	} {
		out, dropped, err := c.run()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if n, _ := PageCount(out); n != c.pages || dropped != 0 {
			t.Errorf("%s: %d pages and %d file(s) dropped, want %d and 0", c.name, n, dropped, c.pages)
		}
		files, title, id, _ := documentFactsOf(t, out)
		if strings.Join(files, " ") != "record.txt" || title != wantTitle || id != wantID {
			t.Errorf("%s: files %v, title %q, /ID[0] %s — want record.txt, %q and the document's own %s",
				c.name, files, title, id, wantTitle, wantID)
		}
		if p := packetOf(t, out); !strings.Contains(p, "Quill") || !strings.Contains(p, "Lease") {
			t.Errorf("%s: the metadata packet reads %q, want the document's own", c.name, p)
		}
	}
}

// TestAFileThatReachesADeletedPageIsCarriedFromItsBytes — `documentFacts` withholds the embedded files when an
// entry reaches a page the selection dropped (putting the tree back would write the deleted page out). For the
// app's delete that must not be a silent loss — the ceremony record is an embedded file — so the door re-adds
// each file from its bytes, and the copy points at nothing but its own streams.
func TestAFileThatReachesADeletedPageIsCarriedFromItsBytes(t *testing.T) {
	host, err := writeMutated(withPacket(t), func(ctx *model.Context) error {
		xt := ctx.XRefTable
		entries, err := treeEntries(xt)
		if err != nil || len(entries) != 1 {
			return fmt.Errorf("setup: %d entries, %v", len(entries), err)
		}
		fs, err := xt.DereferenceDict(entries[0].fs)
		if err != nil || fs == nil {
			return fmt.Errorf("setup: the filespec does not resolve: %v", err)
		}
		pages := pdfread.Pages(ctx)
		if len(pages) != 3 || pages[1].Ref == nil {
			return fmt.Errorf("setup: %d pages", len(pages))
		}
		fs["NibProbe"] = *pages[1].Ref
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// SETUP: the reference survived the write, or the door below is never asked the question.
	probe := func(pdf []byte) bool {
		ctx := readCtx(t, pdf)
		e, err := treeEntries(ctx.XRefTable)
		if err != nil || len(e) != 1 {
			t.Fatalf("%d embedded-file entries, %v", len(e), err)
		}
		fs, _ := ctx.XRefTable.DereferenceDict(e[0].fs)
		return fs["NibProbe"] != nil
	}
	if !probe(host) {
		t.Fatal("setup: the filespec's reference to page 2 did not survive the write")
	}
	out, dropped, err := DeletePages(host, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	files, title, _, _ := documentFactsOf(t, out)
	if strings.Join(files, " ") != "record.txt" || dropped != 0 {
		t.Fatalf("deleting the page a file reaches left files %v with %d dropped, want record.txt carried from its bytes", files, dropped)
	}
	if title != "Lease" {
		t.Errorf("title %q, want Lease — only the files take the second way back", title)
	}
	if probe(out) {
		t.Error("the carried file still points at the deleted page, so the page was written into the output")
	}
	if got, err := ExtractAttachment(out, "record.txt"); err != nil || string(got) != "the record" {
		t.Errorf("the carried file reads %q (%v), want its own bytes", got, err)
	}
	// CONTROL: the same file through a reorder that drops no page takes the first way — the tree as it stood.
	kept, _, err := ReorderPages(host, []string{"3", "1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if !probe(kept) {
		t.Error("a reorder that drops no page rebuilt the file instead of keeping the document's own entry")
	}
}

// withoutCount drops a split's dropped-file count, for the tests that ask something else of its output.
func withoutCount(out []byte, _ int, err error) ([]byte, error) { return out, err }

// withoutWithheld drops a splice's files-withheld answer the same way.
func withoutWithheld(out []byte, _ bool, err error) ([]byte, error) { return out, err }

// fileReachingPage2 is withPacket with its one embedded file's specification holding a reference to page 2, and a
// probe that says whether a document's entry still holds it.
func fileReachingPage2(t *testing.T) (host []byte, probe func([]byte) bool) {
	t.Helper()
	host, err := writeMutated(withPacket(t), func(ctx *model.Context) error {
		xt := ctx.XRefTable
		entries, err := treeEntries(xt)
		if err != nil || len(entries) != 1 {
			return fmt.Errorf("setup: %d entries, %v", len(entries), err)
		}
		fs, err := xt.DereferenceDict(entries[0].fs)
		if err != nil || fs == nil {
			return fmt.Errorf("setup: the filespec does not resolve: %v", err)
		}
		pages := pdfread.Pages(ctx)
		if len(pages) != 3 || pages[1].Ref == nil {
			return fmt.Errorf("setup: %d pages", len(pages))
		}
		fs["NibProbe"] = *pages[1].Ref
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	probe = func(pdf []byte) bool {
		ctx := readCtx(t, pdf)
		e, err := treeEntries(ctx.XRefTable)
		if err != nil || len(e) != 1 {
			t.Fatalf("%d embedded-file entries, %v", len(e), err)
		}
		fs, _ := ctx.XRefTable.DereferenceDict(e[0].fs)
		return fs["NibProbe"] != nil
	}
	if !probe(host) {
		t.Fatal("setup: the filespec's reference to page 2 did not survive the write")
	}
	return host, probe
}

// TestAFileThatReachesASplitPageIsCarriedFromItsBytes — a split REMOVES the page it cuts, so a file reaching that
// page is withheld by the splice exactly as a delete's is. Measured before the splits took the delete's second way
// back: `SplitPage` and `SplitRegions` on the page a file reached each returned the document with NO embedded
// files and no word of it. The control splits a page the file does not reach, which keeps the document's own entry.
func TestAFileThatReachesASplitPageIsCarriedFromItsBytes(t *testing.T) {
	host, probe := fileReachingPage2(t)
	splits := map[string]func(page int) ([]byte, int, error){
		"SplitPage": func(page int) ([]byte, int, error) { return SplitPage(host, page, 2, 1, false) },
		"SplitRegions": func(page int) ([]byte, int, error) {
			return SplitRegions(host, page, [][4]float64{{0, 0, 200, 200}, {200, 200, 400, 400}})
		},
	}
	for name, split := range splits {
		out, dropped, err := split(2)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		files, title, _, _ := documentFactsOf(t, out)
		if strings.Join(files, " ") != "record.txt" || dropped != 0 {
			t.Fatalf("%s of the page a file reaches left files %v with %d dropped, want record.txt carried from its bytes", name, files, dropped)
		}
		if title != "Lease" {
			t.Errorf("%s: title %q, want Lease", name, title)
		}
		if probe(out) {
			t.Errorf("%s: the carried file still points at the page that was cut, so that page was written into the output", name)
		}
		if got, err := ExtractAttachment(out, "record.txt"); err != nil || string(got) != "the record" {
			t.Errorf("%s: the carried file reads %q (%v), want its own bytes", name, got, err)
		}
		// CONTROL: the file does not reach page 1, so splitting it keeps the tree as it stood.
		kept, dropped, err := split(1)
		if err != nil || dropped != 0 {
			t.Fatalf("%s page 1: %d dropped, %v", name, dropped, err)
		}
		if !probe(kept) {
			t.Errorf("%s of a page the file does not reach rebuilt the file instead of keeping the document's own entry", name)
		}
	}
}
