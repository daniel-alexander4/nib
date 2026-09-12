package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// D4's tier, recorded with the tree — `PLAN-accessibility.md` P06.S03.

// TestEveryTreeNibWritesRecordsItsSource — S03's first acceptance clause.
//
// The population is every door in this package that writes a structure tree. It is two today and
// grows with P06.S05 and P06.S06; a door added without a source is a tree whose trustworthiness
// nobody can ask about, which is the whole of what D4 says the user must be told.
func TestEveryTreeNibWritesRecordsItsSource(t *testing.T) {
	for _, c := range []struct {
		door string
		make func() ([]byte, error)
		want tagSource
		why  string
	}{
		{"tagMarkdown", func() ([]byte, error) {
			return tagMarkdown([]byte(s02Markdown), authoringFaces(), markdownFallbackFonts())
		}, sourceExact, "it reads mdpdf's own goldmark AST"},
		{"TagAuthored", func() ([]byte, error) {
			out, _, err := TagAuthored(authoredPDF(t))
			return out, err
		}, sourceGeneric, "it brackets a page knowing nothing about what the content says"},
	} {
		out, err := c.make()
		if err != nil {
			t.Errorf("%s: %v", c.door, err)
			continue
		}
		got, ok := StructureSource(out)
		if !ok {
			t.Errorf("%s writes a tree and records no source — nobody can ask how far to trust it", c.door)
			continue
		}
		if got != c.want {
			t.Errorf("%s records %q, want %q — %s", c.door, got, c.want, c.why)
		}
	}
}

// TestTheSourceIsReadBackFromTheDOCUMENT, not remembered from the call — S03's second clause.
//
// It survives a round trip through `writeMutated`, which is what every later operation does to a
// document. A record that pdfcpu drops on the next rewrite is a record that exists until the first
// thing happens to the file.
func TestTheSourceIsReadBackFromTheDocument(t *testing.T) {
	tagged, err := tagMarkdown([]byte(s02Markdown), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := StructureSource(tagged); !ok || got != sourceExact {
		t.Fatalf("before the round trip: %q/%v", got, ok)
	}
	// Through a no-op rewrite, and through the operations a user would actually run.
	for _, c := range []struct {
		name string
		op   func([]byte) ([]byte, error)
	}{
		{"a no-op rewrite", func(b []byte) ([]byte, error) {
			return writeMutated(b, func(*model.Context) error { return nil })
		}},
		{"SetTitle", func(b []byte) ([]byte, error) { return SetTitle(b, "x") }},
		{"SetLang", func(b []byte) ([]byte, error) { return SetLang(b, "en") }},
		{"Rotate", func(b []byte) ([]byte, error) { return Rotate(b, nil, 90) }},
	} {
		out, oerr := c.op(tagged)
		if oerr != nil {
			t.Errorf("%s: %v", c.name, oerr)
			continue
		}
		got, ok := StructureSource(out)
		if !ok || got != sourceExact {
			t.Errorf("after %s the source is %q/%v — the record did not survive an operation a "+
				"user runs", c.name, got, ok)
		}
	}
}

// TestAnUnrecordedTreeIsNotTreatedAsExact — S03's third clause, and the one that matters most.
//
// **Every tree in the field today carries no record.** A reader that defaulted to the best tier
// would describe every one of them as the most trustworthy kind — which is the exact shape of
// ADR-031's law 1 in a new field: asserting a property the document does not have.
func TestAnUnrecordedTreeIsNotTreatedAsExact(t *testing.T) {
	for _, c := range []struct {
		name string
		pdf  []byte
	}{
		{"a tagged document nib did not tag", taggedFixture()},
		{"an untagged document", untaggedFixture()},
	} {
		got, ok := StructureSource(c.pdf)
		if ok {
			t.Errorf("%s reports source %q — it records nothing, and saying otherwise invents a "+
				"provenance", c.name, got)
		}
		if got != "" {
			t.Errorf("%s returned %q with ok=false; the value must be empty when nothing is recorded",
				c.name, got)
		}
	}
}

// TestAForeignValueIsNotReadAsATier: another producer's private key that happens to share this
// name says nothing about D4's tiers, and guessing what it meant is worse than reporting nothing.
func TestAForeignValueIsNotReadAsATier(t *testing.T) {
	tagged, err := tagMarkdown([]byte("# H\n\nBody.\n"), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := writeMutated(tagged, func(ctx *model.Context) error {
		cat, cerr := ctx.XRefTable.Catalog()
		if cerr != nil {
			return cerr
		}
		root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
		if rerr != nil || root == nil {
			return rerr
		}
		root[tagSourceKey] = types.Name("SomebodyElsesIdea")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := StructureSource(foreign); ok {
		t.Errorf("a value this code never writes was read as tier %q", got)
	}
}

// TestTheSourceKeyDoesNotBreakTheDocument: a private key in `/StructTreeRoot` must not cost
// validity. Measured — pdfcpu validates it and veraPDF raises nothing — and asserted because the
// cost of being wrong is every tagged document nib produces.
func TestTheSourceKeyDoesNotBreakTheDocument(t *testing.T) {
	tagged, err := tagMarkdown([]byte(s02Markdown), authoringFaces(), markdownFallbackFonts())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := api.ReadValidateAndOptimize(bytes.NewReader(tagged), model.NewDefaultConfiguration()); rerr != nil {
		t.Fatalf("a document carrying the source key does not validate: %v", rerr)
	}
	if _, defects := checkTree(t, tagged); len(defects) > 0 {
		t.Errorf("the tree is not self-consistent: %v", defects)
	}
}
