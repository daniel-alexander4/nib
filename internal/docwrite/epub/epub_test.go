package epub

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"nib/internal/docmodel"
	"nib/internal/docwrite/wtest"
)

// read is what a reader of content.xhtml finds, from the XML alone (R1's second oracle).
type read struct {
	text     string
	headings int
	lists    int
	items    int
	tables   int
	links    []string
	images   []string
	ids      map[string]bool
	scripts  int
	rtl      int
}

func parse(t *testing.T, data []byte) read {
	t.Helper()
	r := read{ids: map[string]bool{}}
	d := xml.NewDecoder(bytes.NewReader(data))
	var text strings.Builder
	inBody, inStyle := false, false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			attr := func(n string) string {
				for _, a := range e.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			if id := attr("id"); id != "" {
				r.ids[id] = true
			}
			switch e.Name.Local {
			case "body":
				inBody = true
			case "style":
				inStyle = true
			case "script", "iframe", "object", "embed", "link", "form":
				r.scripts++
			case "h1", "h2", "h3", "h4", "h5", "h6":
				r.headings++
				text.WriteString(" ")
			case "ul", "ol":
				r.lists++
			case "li":
				r.items++
				text.WriteString(" ")
			case "table":
				r.tables++
			case "a":
				r.links = append(r.links, attr("href"))
			case "img":
				r.images = append(r.images, attr("src"))
			case "p", "td", "th", "figure", "br":
				text.WriteString(" ")
			}
			if attr("dir") == "rtl" {
				r.rtl++
			}
		case xml.EndElement:
			if e.Name.Local == "body" {
				inBody = false
			}
			if e.Name.Local == "style" {
				inStyle = false
			}
		case xml.CharData:
			if inBody && !inStyle {
				text.Write(e)
			}
		}
	}
	r.text = text.String()
	return r
}

func write(t *testing.T, d docmodel.Document) wtest.Package {
	t.Helper()
	out, err := Write(context.Background(), d)
	if err != nil || out == nil {
		t.Fatalf("Write: %v, %d bytes", err, len(out))
	}
	return wtest.Open(t, out)
}

func TestEPUBReadsBackAsTheModel(t *testing.T) {
	d := wtest.Hostile()
	pkg := write(t, d)
	pkg.WellFormed(t)
	r := parse(t, pkg.Files["OEBPS/content.xhtml"])
	// Extra: the six words of the placeholder line "[Picture: Scanned page 3 "quoted" <b>]".
	extra, ok := wtest.ReadBack(r.text, wtest.Words(d))
	if !ok || !wtest.SameWords(extra, hostileExtras) {
		t.Errorf("the XHTML does not hold the model's words in order (ok=%v, extra %q, want %q):\n%s", ok, extra, hostileExtras, r.text)
	}
	if r.headings != 1 || r.lists != 2 || r.items != 3 || r.tables != 1 {
		t.Errorf("structure: %d headings, %d lists, %d items, %d tables; want 1, 2, 3, 1", r.headings, r.lists, r.items, r.tables)
	}
	if r.scripts != 0 {
		t.Errorf("%d script-like elements in the content", r.scripts)
	}
	for _, l := range r.links {
		if strings.HasPrefix(strings.ToLower(l), "javascript:") {
			t.Errorf("a javascript link survived: %q", l)
		}
	}
	if len(r.links) != 1 || !strings.HasPrefix(r.links[0], "https://example.com/") {
		t.Errorf("links = %q, want the one https link", r.links)
	}
	if r.rtl == 0 {
		t.Error("the right-to-left paragraph lost its direction")
	}
}

func TestEPUBMimetypeIsFirstStoredAndAtByte38(t *testing.T) {
	out, _ := Write(context.Background(), wtest.Hostile())
	if string(out[:2]) != "PK" || out[8] != 0 || out[9] != 0 {
		t.Errorf("first member is not stored: % x", out[:10])
	}
	if got := string(out[38 : 38+len(mime)]); got != mime {
		t.Errorf("byte 38 holds %q, want the media type", got)
	}
	if pkg := wtest.Open(t, out); pkg.Order[0] != "mimetype" || string(pkg.Files["mimetype"]) != mime {
		t.Errorf("first entry %q", pkg.Order[0])
	}
}

// opf is the package document's parts, read from its XML.
type opf struct {
	title, lang, id, modified string
	manifest                  map[string]string // href -> media-type
	navs                      []string          // hrefs with properties="nav"
	spine                     []string          // idrefs
	ids                       map[string]string // item id -> href
	unique, version           string
	rootfile                  string
}

func readOPF(t *testing.T, pkg wtest.Package) opf {
	t.Helper()
	o := opf{manifest: map[string]string{}, ids: map[string]string{}}
	d := xml.NewDecoder(bytes.NewReader(pkg.Files["OEBPS/content.opf"]))
	var cur string
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			attr := func(n string) string {
				for _, a := range e.Attr {
					if a.Name.Local == n {
						return a.Value
					}
				}
				return ""
			}
			cur = e.Name.Local
			switch cur {
			case "package":
				o.unique, o.version = attr("unique-identifier"), attr("version")
			case "item":
				o.manifest[attr("href")] = attr("media-type")
				o.ids[attr("id")] = attr("href")
				if attr("properties") == "nav" {
					o.navs = append(o.navs, attr("href"))
				}
			case "itemref":
				o.spine = append(o.spine, attr("idref"))
			}
		case xml.CharData:
			switch cur {
			case "title":
				o.title += string(e)
			case "language":
				o.lang += string(e)
			case "identifier":
				o.id += string(e)
			case "meta":
				o.modified += string(e)
			}
		case xml.EndElement:
			cur = ""
		}
	}
	c := string(pkg.Files["META-INF/container.xml"])
	if i := strings.Index(c, `full-path="`); i >= 0 {
		o.rootfile = c[i+len(`full-path="`) : i+len(`full-path="`)+strings.Index(c[i+len(`full-path="`):], `"`)]
	}
	return o
}

func TestEPUBIsAWellFormedPublication(t *testing.T) {
	d := wtest.Hostile()
	pkg := write(t, d)
	o := readOPF(t, pkg)
	if o.rootfile != "OEBPS/content.opf" || pkg.Files[o.rootfile] == nil {
		t.Fatalf("container.xml names %q, not a member", o.rootfile)
	}
	if o.version != "3.0" || o.unique != "uid" || !strings.HasPrefix(o.id, "urn:uuid:") || len(o.id) != 45 {
		t.Errorf("package: version %q, unique %q, id %q", o.version, o.unique, o.id)
	}
	if o.title != `</title><script>alert(1)</script>` {
		t.Errorf("dc:title = %q, want the title as text", o.title)
	}
	if o.lang != "und" {
		t.Errorf("dc:language = %q, want und for a hostile language", o.lang)
	}
	if o.modified != "2000-01-01T00:00:00Z" {
		t.Errorf("dcterms:modified = %q", o.modified)
	}
	if len(o.navs) != 1 || o.navs[0] != "nav.xhtml" {
		t.Errorf("nav items = %q", o.navs)
	}
	if len(o.spine) != 1 || o.ids[o.spine[0]] != "content.xhtml" {
		t.Errorf("spine = %q", o.spine)
	}
	// Every manifest item is a member, and every member but the three package files is in the manifest.
	inMan := map[string]bool{}
	for href := range o.manifest {
		inMan["OEBPS/"+href] = true
		if pkg.Files["OEBPS/"+href] == nil {
			t.Errorf("the manifest names %q, which is not a member", href)
		}
	}
	for _, n := range pkg.Order {
		if n != "mimetype" && n != "META-INF/container.xml" && n != "OEBPS/content.opf" && !inMan[n] {
			t.Errorf("member %q is not in the manifest", n)
		}
	}
	if o.manifest["images/img-1.png"] != "image/png" {
		t.Errorf("the picture's media type = %q", o.manifest["images/img-1.png"])
	}
	// The body names its picture by a member, and no remote or data reference.
	r := parse(t, pkg.Files["OEBPS/content.xhtml"])
	if len(r.images) != 1 || r.images[0] != "images/img-1.png" {
		t.Errorf("img src = %q", r.images)
	}
	// The navigation links every heading, and each target exists in the content.
	nav := string(pkg.Files["OEBPS/nav.xhtml"])
	if !strings.Contains(nav, `epub:type="toc"`) || !strings.Contains(nav, `href="content.xhtml#h-1"`) {
		t.Errorf("nav.xhtml:\n%s", nav)
	}
	if !r.ids["h-1"] {
		t.Error("the heading the navigation names has no id in the content")
	}
}

func TestEPUBHoldsOnlyItsNamedPartsAndNoClaims(t *testing.T) {
	out, _ := Write(context.Background(), wtest.Hostile())
	pkg := wtest.Open(t, out)
	allowed := map[string]bool{"OEBPS/images/img-1.png": true}
	for _, p := range Parts {
		allowed[p] = true
	}
	for _, n := range pkg.Order {
		if !allowed[n] {
			t.Errorf("unexpected part %q", n)
		}
	}
	for _, n := range pkg.Order {
		s := string(pkg.Files[n])
		for _, bad := range []string{"conformsTo", "pdfuaid", "accessibilityFeature", "dc:creator", "dc:contributor", "dc:source", "<script", "file://"} {
			if n != "OEBPS/images/img-1.png" && strings.Contains(s, bad) {
				t.Errorf("%s holds %q", n, bad)
			}
		}
	}
}

func TestEPUBIsTheSameBytesTwiceAndNamesNothingOfTheMachine(t *testing.T) {
	a, _ := Write(context.Background(), wtest.Hostile())
	b, _ := Write(context.Background(), wtest.Hostile())
	if !bytes.Equal(a, b) {
		t.Error("the same model wrote two different packages")
	}
	d := wtest.Hostile()
	d.Blocks[0].Spans[0].Text = "Another document"
	c, _ := Write(context.Background(), d)
	if oa, oc := readOPF(t, wtest.Open(t, a)).id, readOPF(t, wtest.Open(t, c)).id; oa == oc {
		t.Errorf("two different documents share the identifier %s", oa)
	}
}

func TestEPUBNavigationNeedsAnEntryAndAHeadinglessDocumentGetsItsTitle(t *testing.T) {
	d := docmodel.Document{Meta: docmodel.Meta{Title: "Notes", Lang: "fr"}, Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Page: 1, Spans: []docmodel.Span{{Text: "just a line"}}}}}
	pkg := write(t, d)
	pkg.WellFormed(t)
	nav := string(pkg.Files["OEBPS/nav.xhtml"])
	if !strings.Contains(nav, `<li><a href="content.xhtml">Notes</a></li>`) {
		t.Errorf("nav.xhtml has no entry:\n%s", nav)
	}
	if o := readOPF(t, pkg); o.lang != "fr" {
		t.Errorf("dc:language = %q, want fr", o.lang)
	}
	// A heading with no text still gets a labelled entry.
	d.Blocks = append(d.Blocks, docmodel.Block{Kind: docmodel.Heading, Level: 2, Page: 1, Spans: []docmodel.Span{{Text: " "}}})
	if n := string(write(t, d).Files["OEBPS/nav.xhtml"]); !strings.Contains(n, ">Section 1</a>") {
		t.Errorf("an empty heading has no label:\n%s", n)
	}
}

func TestEPUBRightToLeftSetsTheSpineDirection(t *testing.T) {
	d := docmodel.Document{Meta: docmodel.Meta{Title: "ר", Lang: "he", Dir: "rtl"}, Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Page: 1, Dir: "rtl", Spans: []docmodel.Span{{Text: "שלום"}}}}}
	pkg := write(t, d)
	if !strings.Contains(string(pkg.Files["OEBPS/content.opf"]), `page-progression-direction="rtl"`) {
		t.Error("the spine does not say right to left")
	}
}

func TestEPUBNothingWrittenIsNil(t *testing.T) {
	for name, d := range map[string]docmodel.Document{
		"no blocks":         {},
		"a blank paragraph": {Blocks: []docmodel.Block{{Kind: docmodel.Paragraph, Page: 1, Spans: []docmodel.Span{{Text: " "}}}}},
	} {
		if out, err := Write(context.Background(), d); err != nil || out != nil {
			t.Errorf("%s: %d bytes, %v; want nil", name, len(out), err)
		}
	}
}

func TestEPUBContentOpensInLibreOffice(t *testing.T) {
	// LibreOffice 24.2 has no EPUB import, so the confirmatory run is the content document alone: the body is HTML to
	// it, and the model's words must come back in order. epubcheck is the package-level confirmation, and is not run here.
	d := wtest.Hostile()
	pkg := write(t, d)
	got := wtest.LibreOfficeText(t, "content.xhtml", pkg.Files["OEBPS/content.xhtml"])
	if _, ok := wtest.ReadBack(got, wtest.Words(d)); !ok { // the extras differ with the engine; the words and their order are what is judged here
		t.Errorf("LibreOffice does not read the model's words in order:\n%s", got)
	}
}

func TestEPUBNavigationNamesEachHeadingByItsOwnTarget(t *testing.T) {
	h := func(l int, t string) docmodel.Block {
		return docmodel.Block{Kind: docmodel.Heading, Level: l, Page: 1, Spans: []docmodel.Span{{Text: t}}}
	}
	pkg := write(t, docmodel.Document{Blocks: []docmodel.Block{h(1, "One"), h(2, "Two"), h(2, "Three & <four>")}})
	pkg.WellFormed(t)
	nav := string(pkg.Files["OEBPS/nav.xhtml"])
	for i, want := range []string{`<a href="content.xhtml#h-1">One</a>`, `<a href="content.xhtml#h-2">Two</a>`, `<a href="content.xhtml#h-3">Three &amp; &lt;four&gt;</a>`} {
		if !strings.Contains(nav, want) {
			t.Errorf("entry %d: %s missing from\n%s", i+1, want, nav)
		}
	}
	r := parse(t, pkg.Files["OEBPS/content.xhtml"])
	for _, id := range []string{"h-1", "h-2", "h-3"} {
		if !r.ids[id] {
			t.Errorf("the content has no %s", id)
		}
	}
}

func TestEPUBPassesEpubcheck(t *testing.T) {
	out, _ := Write(context.Background(), wtest.Hostile())
	wtest.EpubCheck(t, out)
}

func TestEPUBCarriesAPictureAReaderCanDecode(t *testing.T) {
	write(t, wtest.Hostile()).DecodesAsPNG(t, "OEBPS/images/img-1.png")
}
