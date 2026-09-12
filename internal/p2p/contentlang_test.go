package p2p

import (
	"bytes"
	"strings"
	"testing"

	"nib/internal/pdfops"

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
	}})
	if err != nil {
		t.Fatalf("renderPage: %v", err)
	}
	c := pageContent(t, page, 1)
	if !bytes.Contains(c, []byte("/Span")) || !bytes.Contains(c, []byte("(en)")) {
		t.Errorf("a signature page carries no content-language declaration:\n%.200s", c)
	}
}

// TestTheDeclarationClaimsNoTagging — ADR-031's law 1 is not engaged by marked content, and this is
// the assertion that keeps it that way.
//
// `/Span <</Lang (en)>> BDC … EMC` is marked content, not a structure tree. A future change that
// reached for a structure element here would start claiming tagging over a fragment with no tree —
// which is `orphaned()`, the state P01.S06 built a door to prevent.
func TestTheDeclarationClaimsNoTagging(t *testing.T) {
	readme, err := RenderReadme()
	if err != nil {
		t.Fatal(err)
	}
	if pdfops.ClaimsTagging(readme) {
		t.Error("the readme now claims tagging — declaring a content language must not assert a " +
			"structure the fragment has not got")
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
