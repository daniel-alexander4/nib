package pdfops

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// labelReady is a Markdown conversion the way the two labelling doors produce one: tagged, titled from its
// file name, and declared in a language.
func labelReady(t *testing.T, md string) []byte {
	t.Helper()
	pdf, err := ConvertDocToPDF([]byte(md), ".md")
	if err != nil {
		t.Fatal(err)
	}
	if pdf, err = TitleFromName(pdf, "notes.md"); err != nil {
		t.Fatal(err)
	}
	if pdf, err = SetLang(pdf, "en"); err != nil {
		t.Fatal(err)
	}
	return pdf
}

func TestLabelUAWritesTheIdentificationOnAConversionThatEarnsIt(t *testing.T) {
	src := labelReady(t, "# Notes\n\nA paragraph.\n")
	if ok, _ := testpdf.ClaimsUA(src); ok {
		t.Fatal("setup: the conversion already claims PDF/UA, so writing one proves nothing")
	}
	out, err := LabelUA(src, true)
	if err != nil {
		t.Fatalf("LabelUA refused a conversion that meets every condition: %v", err)
	}
	if ok, err := testpdf.ClaimsUA(out); err != nil || !ok {
		t.Fatalf("LabelUA returned without an identification (err %v)", err)
	}
	if !packetHasTitle(catalogPacket(t, out)) {
		t.Error("labelling disturbed the packet: dc:title is gone")
	}
}

// TestLabelUARefusesEachMissingConditionByName — one refusal per cause, each asserted against its own
// sentinel so a refusal for the wrong reason cannot pass as the right one.
func TestLabelUARefusesEachMissingConditionByName(t *testing.T) {
	ready := labelReady(t, "# Notes\n\nA paragraph.\n")

	untaggedTitled, err := testpdf.Text("plain")
	if err != nil {
		t.Fatal(err)
	}
	if untaggedTitled, err = SetTitle(untaggedTitled, "Plain"); err != nil {
		t.Fatal(err)
	}
	if untaggedTitled, err = SetLang(untaggedTitled, "en"); err != nil {
		t.Fatal(err)
	}

	noLang, err := ConvertDocToPDF([]byte("# Notes\n\nA paragraph.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	if noLang, err = TitleFromName(noLang, "notes.md"); err != nil {
		t.Fatal(err)
	}

	noTitle, err := ConvertDocToPDF([]byte("# Notes\n\nA paragraph.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	if noTitle, err = SetLang(noTitle, "en"); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name     string
		pdf      []byte
		asserted bool
		want     error
	}{
		{"a pre-filled language nobody chose", ready, false, ErrUALanguageNotAsserted},
		{"an untagged document", untaggedTitled, true, ErrUAUntagged},
		{"no declared language", noLang, true, ErrUANoLanguage},
		{"no title", noTitle, true, ErrUANoTitle},
	} {
		out, err := LabelUA(c.pdf, c.asserted)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: LabelUA returned %v, want %v", c.name, err, c.want)
		}
		if out != nil {
			t.Errorf("%s: a refusal still returned a document", c.name)
		}
	}
	// Control: the ready document with the language asserted is NOT refused, or every row above could be
	// a door that refuses everything.
	if _, err := LabelUA(ready, true); err != nil {
		t.Fatalf("control: the ready document was refused too (%v)", err)
	}
}

// TestLabelUARefusesAPacketWithAnEmptyTitle — the dc:title condition on its own. The no-title row above
// has no packet at all, so it is refused before this check is reached; here the packet exists and viewers
// are asked to display a title, and the title is empty.
func TestLabelUARefusesAPacketWithAnEmptyTitle(t *testing.T) {
	ready := labelReady(t, "# Notes\n\nA paragraph.\n")
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(ready), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	ir, ok := root["Metadata"].(types.IndirectRef)
	if !ok {
		t.Fatal("setup: the conversion has no indirect /Metadata")
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(ir)
	if err != nil || sd == nil || sd.Decode() != nil {
		t.Fatalf("setup: metadata stream: %v", err)
	}
	emptied := regexp.MustCompile(`(<rdf:li[^>]*>)[^<]*(</rdf:li>)`).ReplaceAllString(string(sd.Content), "$1$2")
	if emptied == string(sd.Content) || packetHasTitle(emptied) {
		t.Fatal("setup: the title could not be emptied, so this row would test a titled packet")
	}
	sd.Content = []byte(emptied)
	if err := sd.Encode(); err != nil {
		t.Fatal(err)
	}
	entry, _ := ctx.XRefTable.FindTableEntryForIndRef(&ir)
	entry.Object = *sd
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		t.Fatal(err)
	}
	if _, err := LabelUA(buf.Bytes(), true); !errors.Is(err, ErrUANoTitle) {
		t.Errorf("a packet whose dc:title is empty was answered %v, want %v", err, ErrUANoTitle)
	}
}

// markdownForEveryNodeKind is one snippet per goldmark node kind mdpdf's renderer switches on. The guard
// below keys on mdpdf's own `case *ast.` list, so a kind added to the renderer fails here until it has a
// snippet — and then the veraPDF test measures it.
var markdownForEveryNodeKind = map[string]string{
	"Heading":         "# A heading\n\n## A subheading\n",
	"Paragraph":       "A paragraph that wraps across the measure of the page so it runs to more than one line and more than one marked-content sequence.\n",
	"TextBlock":       "- a tight list item\n",
	"List":            "- one\n  - nested\n- two\n\n1. first\n2. second\n",
	"Blockquote":      "> A quoted paragraph.\n",
	"FencedCodeBlock": "```\nfenced code\n```\n",
	"CodeBlock":       "    indented code\n",
	"ThematicBreak":   "Before the rule.\n\n---\n\nAfter the rule.\n",
	"HTMLBlock":       "<div>an html block</div>\n",
	"Text":            "Plain text.\n",
	"String":          "Smart &amp; entities.\n",
	"CodeSpan":        "Some `inline code` here.\n",
	"Emphasis":        "*emphasis*, **strong** and ***both***.\n",
	"AutoLink":        "<https://example.com>\n",
	"Image":           "![an image](nowhere.png)\n",
	"RawHTML":         "Inline <b>raw html</b> here.\n",
}

func TestTheLabelFixtureCoversEveryNodeKindMdpdfRenders(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "mdpdf", "mdpdf.go"))
	if err != nil {
		t.Fatal(err)
	}
	caseLines := strings.Join(regexp.MustCompile(`(?m)^\s*case [^\n]*`).FindAllString(string(src), -1), "\n")
	var kinds []string
	for _, m := range regexp.MustCompile(`\*ast\.([A-Za-z]+)`).FindAllStringSubmatch(caseLines, -1) {
		kinds = append(kinds, m[1])
	}
	sort.Strings(kinds)
	if len(kinds) < 10 {
		t.Fatalf("found only %d ast node kind(s) in mdpdf's switches — the scan has stopped matching", len(kinds))
	}
	seen := map[string]bool{}
	for _, k := range kinds {
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, ok := markdownForEveryNodeKind[k]; !ok {
			t.Errorf("mdpdf renders *ast.%s and the label fixture has no snippet for it — a document containing one "+
				"would be labelled PDF/UA on a construct veraPDF has never been asked about", k)
		}
	}
}

// TestNibsOwnMarkdownConversionConformsAcrossEveryConstruct — the claim the label makes, measured on every
// build that has veraPDF.
func TestNibsOwnMarkdownConversionConformsAcrossEveryConstruct(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the conformance the label claims is UNMEASURED here")
	}
	keys := make([]string, 0, len(markdownForEveryNodeKind))
	for k := range markdownForEveryNodeKind {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var md strings.Builder
	for _, k := range keys {
		md.WriteString(markdownForEveryNodeKind[k])
		md.WriteString("\n")
	}
	labelled, err := LabelUA(labelReady(t, md.String()), true)
	if err != nil {
		t.Fatalf("LabelUA refused the all-constructs conversion: %v", err)
	}
	p := filepath.Join(t.TempDir(), "constructs.pdf")
	if err := os.WriteFile(p, labelled, 0o600); err != nil {
		t.Fatal(err)
	}
	cl := ua1FailedClauses(t, vp, []string{p})
	if cl["constructs.pdf"] == nil {
		t.Fatal("veraPDF could not validate the labelled conversion")
	}
	if len(cl["constructs.pdf"]) > 0 {
		t.Errorf("nib's labelled Markdown conversion fails PDF/UA-1 on %v — the label would be a false claim", sortedClauses(cl["constructs.pdf"]))
	}
}
