package pdfops

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// authoredPDF is a document made the way nib makes them: CreateFromJSON, not a fixture read off
// disk. The floor being tested is the floor of nib's OWN output, and a LibreOffice fixture already
// carries a title, so it cannot fail the clause this door exists to fix.
func authoredPDF(t *testing.T) []byte {
	t.Helper()
	spec := []byte(`{"pages":{"1":{"content":{"text":[{"value":"A nib-authored document",` +
		`"anchor":"TopLeft","position":[72,720],"font":{"name":"Helvetica","size":14}}]}}}}`)
	pdf, err := CreateFromJSON(spec)
	if err != nil {
		t.Fatalf("CreateFromJSON: %v", err)
	}
	return pdf
}

// titleParts reads back the three halves of the catalog floor from a validating re-read — the same
// read every later operation performs, so a title that only survives a raw byte scan does not count.
func titleParts(t *testing.T, pdf []byte) (xmp string, info string, displayDocTitle *bool) {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read/validate: %v", err)
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if o, ok := root["Metadata"]; ok {
		sd, _, derr := xt.DereferenceStreamDict(o)
		if derr != nil || sd == nil {
			t.Fatalf("catalog /Metadata does not resolve to a stream: %v", derr)
		}
		if n, _ := sd.Dict["Type"].(types.Name); n != "Metadata" {
			t.Errorf("/Metadata stream /Type = %q, want Metadata", n)
		}
		if n, _ := sd.Dict["Subtype"].(types.Name); n != "XML" {
			t.Errorf("/Metadata stream /Subtype = %q, want XML", n)
		}
		if err := sd.Decode(); err != nil {
			t.Fatalf("decode /Metadata: %v", err)
		}
		xmp = string(sd.Content)
	}
	if ctx.Info != nil {
		if d, _ := ctx.DereferenceDict(*ctx.Info); d != nil {
			if s, ok := d["Title"].(types.StringLiteral); ok {
				v, _ := types.StringLiteralToString(s)
				info = v
			}
		}
	}
	if ctx.ViewerPref != nil {
		displayDocTitle = ctx.ViewerPref.DisplayDocTitle
	}
	return xmp, info, displayDocTitle
}

// TestAuthoredOutputHasNoTitleUntilTheDoorIsCalled is the stimulus floor: without it, every
// assertion below could be satisfied by a primitive that already wrote a title, and the door would
// be verified while doing nothing. PDF/UA 7.1 t8/t9/t10 are the clauses this measures failing.
func TestAuthoredOutputHasNoTitleUntilTheDoorIsCalled(t *testing.T) {
	xmp, info, ddt := titleParts(t, authoredPDF(t))
	if xmp != "" {
		t.Errorf("CreateFromJSON already writes an XMP packet — the floor below is not being tested:\n%s", xmp)
	}
	if info != "" {
		t.Errorf("CreateFromJSON already writes Info /Title = %q", info)
	}
	if ddt != nil {
		t.Errorf("CreateFromJSON already writes DisplayDocTitle = %v", *ddt)
	}
}

func TestSetTitleWritesTheWholeCatalogFloor(t *testing.T) {
	const want = "Nib trust explainer"
	out, err := SetTitle(authoredPDF(t), want)
	if err != nil {
		t.Fatalf("SetTitle: %v", err)
	}
	xmp, info, ddt := titleParts(t, out)
	if !strings.Contains(xmp, want) {
		t.Errorf("XMP packet carries no dc:title %q:\n%s", want, xmp)
	}
	if !strings.Contains(xmp, "http://purl.org/dc/elements/1.1/") {
		t.Errorf("XMP packet declares no dc namespace, so its dc:title is not one:\n%s", xmp)
	}
	if !strings.Contains(xmp, "<?xpacket begin=") || !strings.Contains(xmp, "<?xpacket end=") {
		t.Errorf("XMP stream is not wrapped in a packet:\n%s", xmp)
	}
	if info != want {
		t.Errorf("Info /Title = %q, want %q — the two places a title lives disagree", info, want)
	}
	if ddt == nil || !*ddt {
		t.Errorf("ViewerPreferences /DisplayDocTitle = %v, want true — a title nothing displays "+
			"fails 7.1 t10 while satisfying t9", ddt)
	}
}

// TestSetTitleEscapesAHostileTitle: titles come from file names, and a file name may contain `&`
// or `<`. An unescaped one makes the packet malformed, which is worse than no packet — it fails the
// clause AND breaks readers that parse it. The assertion is that the packet still PARSES.
func TestSetTitleEscapesAHostileTitle(t *testing.T) {
	const want = `Smith & Jones <draft> "final" v1`
	out, err := SetTitle(authoredPDF(t), want)
	if err != nil {
		t.Fatalf("SetTitle: %v", err)
	}
	xmp, info, _ := titleParts(t, out)
	body := xmp
	if i := strings.Index(body, "<x:xmpmeta"); i >= 0 {
		body = body[i:]
	}
	if j := strings.Index(body, "<?xpacket end"); j >= 0 {
		body = body[:j]
	}
	var probe any
	if err := xml.Unmarshal([]byte(body), &probe); err != nil {
		t.Fatalf("XMP packet is not well-formed XML: %v\n%s", err, body)
	}
	if strings.Contains(body, "& Jones") {
		t.Errorf("a raw ampersand reached the packet unescaped:\n%s", body)
	}
	if info != want {
		t.Errorf("Info /Title = %q, want %q", info, want)
	}
}

// TestSetTitleRefusesAnEmptyTitle — `dc:title` present and blank passes a key check and fails the
// clause it exists for, so the door refuses rather than writing one.
func TestSetTitleRefusesAnEmptyTitle(t *testing.T) {
	if _, err := SetTitle(authoredPDF(t), ""); err == nil {
		t.Fatal("SetTitle accepted an empty title")
	}
}

// TestSetTitleClaimsNoTaggingItHasNot — ADR-031 law 1. Adding catalog metadata must not leave the
// document asserting a structure it does not have.
func TestSetTitleClaimsNoTaggingItHasNot(t *testing.T) {
	out, err := SetTitle(authoredPDF(t), "A titled but untagged document")
	if err != nil {
		t.Fatalf("SetTitle: %v", err)
	}
	if s := inspectTags(out); s.orphaned() {
		t.Errorf("SetTitle left the document claiming tagging it has not: %+v", s)
	}
}

func TestTitleFromFilename(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"report.pdf", "report"},
		{"/home/dan/Documents/Lease agreement.docx", "Lease agreement"},
		// A multipart header's filename is chosen by the client, so a Windows path must resolve on
		// a machine whose separator is "/". filepath.Base alone would return the whole string.
		{`C:\Users\dan\Q3 accounts.xlsx`, "Q3 accounts"},
		{"notes", "notes"},
		{"  spaced.md  ", "spaced"},
		{"archive.tar.gz", "archive.tar"},
		// Nothing a title can honestly be made of. The callers guard on "" and write no title.
		{"", ""},
		{".pdf", ""},
		{"/", ""},
		{"   ", ""},
	} {
		if got := TitleFromFilename(c.in); got != c.want {
			t.Errorf("TitleFromFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAppendKeepsTheFirstDocumentsCatalog is what holds `titledoor_test.go`'s two p2p fragment
// exemptions upright. Those rows say a title written on the readme or a signature page is discarded
// because `Append` keeps the first document's catalog — a claim about pdfcpu, not about nib, and the
// kind that goes stale silently when a dependency moves.
//
// **Both directions are asserted, and the second is the one that matters.** If Append ever started
// taking the SECOND document's catalog, a co-signed document would be retitled "About this
// co-signed document" — the user's own contract, renamed by a page nib stapled to the back, with
// nothing failing.
func TestAppendKeepsTheFirstDocumentsCatalog(t *testing.T) {
	frag, err := SetTitle(authoredPDF(t), "About this co-signed document")
	if err != nil {
		t.Fatal(err)
	}

	titled, err := SetTitle(authoredPDF(t), "The user's own contract")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Append(titled, frag)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, info, _ := titleParts(t, out); info != "The user's own contract" {
		t.Errorf("Append(titled, fragment) Info /Title = %q, want the FIRST document's title — a "+
			"stapled-on page has renamed the user's document", info)
	}

	// The untitled case is the common one: a user's uploaded PDF usually has no title at all.
	out, err = Append(authoredPDF(t), frag)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	xmp, info, _ := titleParts(t, out)
	if info != "" || xmp != "" {
		t.Errorf("Append(untitled, fragment) picked up the fragment's catalog: Info /Title = %q, "+
			"XMP present = %v — nib would be inventing a title for a user's document", info, xmp != "")
	}
}
