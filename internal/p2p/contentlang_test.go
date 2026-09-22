package p2p

import (
	"bytes"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/mdpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Nib's own prose declares its own language — `PLAN-accessibility.md` P06.S04.

// pageContent returns one page's decoded content stream.
func pageContent(t *testing.T, pdf []byte, page int) []byte {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	d, _, _, derr := ctx.PageDict(page, false)
	if derr != nil || d == nil {
		t.Fatalf("page %d: %v", page, derr)
	}
	c, cerr := ctx.PageContent(d, page)
	if cerr != nil {
		t.Fatalf("content of page %d: %v", page, cerr)
	}
	return c
}

func catalogLangOf(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	root, rerr := ctx.XRefTable.Catalog()
	if rerr != nil {
		t.Fatal(rerr)
	}
	s, _ := root["Lang"].(types.StringLiteral)
	return string(s)
}

// TestNibsOwnProseDeclaresItsLanguageInsideAForeignDocument — S04's first acceptance clause, and
// the defect P03.S02 measured.
//
// The readme is English. `AppendReadme` puts it inside the user's document and `pdfops.Append` keeps
// the FIRST document's catalog, so before this the text was declared to be in whatever that
// document said.
func TestNibsOwnProseDeclaresItsLanguageInsideAForeignDocument(t *testing.T) {
	// A user's document that says it is German — the case that makes the defect visible.
	user, err := pdfops.CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[` +
		`{"value":"Ein deutscher Vertrag","anchor":"TopLeft","position":[72,720],` +
		`"font":{"name":"Helvetica","size":12}}]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	user, err = pdfops.SetLang(user, "de")
	if err != nil {
		t.Fatal(err)
	}

	composed, err := AppendReadme(user)
	if err != nil {
		t.Fatalf("AppendReadme: %v", err)
	}

	// The floor: the composed document must still declare German, or the case this guards does not
	// exist in it.
	if got := catalogLangOf(t, composed); got != "de" {
		t.Fatalf("the composed document declares /Lang %q, not the user's German — the defect this "+
			"test is about is not present in the fixture", got)
	}

	// Nib's page carries its own language on its content.
	readmePage := pageContent(t, composed, 2)
	if !bytes.Contains(readmePage, []byte("/Span")) || !bytes.Contains(readmePage, []byte("/Lang")) {
		t.Errorf("nib's readme page carries no content-language declaration, so its English text "+
			"is declared German by the document it was stapled into:\n%.200s", readmePage)
	}
	if !bytes.Contains(readmePage, []byte("(en)")) {
		t.Errorf("the declaration does not name English:\n%.200s", readmePage)
	}

	// And the USER's page is untouched — their language is not restated, overridden or removed.
	userPage := pageContent(t, composed, 1)
	if bytes.Contains(userPage, []byte("/Span")) {
		t.Errorf("nib added marked content to the USER's page:\n%.200s", userPage)
	}
}

// TestSignaturePagesDeclareTheirLanguageToo: the readme is not the only prose nib staples in.
func TestSignaturePagesDeclareTheirLanguageToo(t *testing.T) {
	page, err := renderPage([]any{map[string]any{
		"value": "Signature page", "pos": []any{62.0, 700.0},
		"font": map[string]any{"name": "$body"},
	}}, []mdpdf.Role{{Kind: mdpdf.RoleHeading, Level: 1}})
	if err != nil {
		t.Fatalf("renderPage: %v", err)
	}
	c := pageContent(t, page, 1)
	if !bytes.Contains(c, []byte("/Span")) || !bytes.Contains(c, []byte("(en)")) {
		t.Errorf("a signature page carries no content-language declaration:\n%.200s", c)
	}
}

// TestTheReadmeIsTaggedAndStillDeclaresItsLanguage — `PLAN-ua-coverage.md` P02.S09 inverted what this
// test used to assert (*"the declaration claims no tagging"*): the readme now claims tagging, over a
// tree nib built from the lines it composed. What must still hold is the other half of that old test's
// worry — the claim is over a tree that describes the page, never an orphaned one — and the content
// language survives the tagging, which it would not if the tagging ran first (`declareContentLang`
// skips a page that is already marked).
func TestTheReadmeIsTaggedAndStillDeclaresItsLanguage(t *testing.T) {
	readme, err := RenderReadme()
	if err != nil {
		t.Fatal(err)
	}
	f := pdfops.Inspect(readme)
	if !f.Tagged {
		t.Fatal("the readme does not claim tagging, so a tagged document it is appended to loses coverage")
	}
	if u, err := pdfops.UnmarkedTextRuns(readme); err != nil || u != 0 {
		t.Errorf("%d of the readme's text runs are outside any structure element (err %v)", u, err)
	}
	c := pageContent(t, readme, 1)
	if !bytes.Contains(c, []byte("/Span")) || !bytes.Contains(c, []byte("(en)")) {
		t.Errorf("the readme lost its content-language declaration to the tagging:\n%.300s", c)
	}
}

// TestTheComposedDocumentGainsNoUA1Clause — S04's third acceptance clause.
func TestTheComposedDocumentGainsNoUA1Clause(t *testing.T) {
	user, err := pdfops.CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[` +
		`{"value":"A contract","anchor":"TopLeft","position":[72,720],` +
		`"font":{"name":"Helvetica","size":12}}]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	before, err := AppendReadme(user)
	if err != nil {
		t.Fatal(err)
	}
	// The declaration is already in `before` — it is written at render time — so the comparison is
	// against a readme rendered WITHOUT it, which is what the mutation probe drives. What this
	// asserts is the weaker and still useful thing: the composed document is valid and
	// self-consistent with the declaration in it.
	if _, rerr := api.ReadValidateAndOptimize(bytes.NewReader(before), model.NewDefaultConfiguration()); rerr != nil {
		t.Errorf("the composed document does not validate: %v", rerr)
	}
	if n := strings.Count(string(pageContent(t, before, 2)), "/Span"); n != 1 {
		t.Errorf("nib's page carries %d /Span declarations, want exactly 1 — a second pass would "+
			"nest a language inside a language", n)
	}
}

// TestTheDeclarationIsNotAppliedTwice: idempotence, because `RenderReadme` is called once per
// composition and a double bracket nests a language inside itself.
func TestTheDeclarationIsNotAppliedTwice(t *testing.T) {
	readme, err := RenderReadme()
	if err != nil {
		t.Fatal(err)
	}
	again, n, err := pdfops.DeclareAuthoredProseLang(readme)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a second declaration bracketed %d page(s) of a page that already carries one", n)
	}
	if !bytes.Equal(again, readme) {
		t.Errorf("a second declaration changed the document (%d bytes against %d)", len(readme), len(again))
	}
}
