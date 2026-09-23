package uacheck

import (
	"bytes"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Document is a document opened once for every rule to read.
//
// # Why the rules share one open
//
// Each rule could open the bytes itself, and eighteen rules would then be eighteen
// `ReadValidateAndOptimize` calls over the same file. Measured elsewhere in this repo, that call is
// the expensive part of every operation in `internal/pdfops`. More importantly they would be
// eighteen possibly-different readings: `ReadValidateAndOptimize` normalises as it reads, so two
// rules could legitimately disagree about the same document and nothing would say which was right.
type Document struct {
	// Ctx is the parsed document. Rules read it; nothing mutates it — a checker that edited what it
	// was inspecting would report on a document the user does not have.
	Ctx *model.Context
	// Catalog is the root dictionary, resolved once because nearly every rule wants it.
	Catalog types.Dict

	// pt is the resolved /ParentTree, built on first use by parentTree, and ptErr is why part of it was not
	// read, when part was not.
	pt    map[int]types.Object
	ptErr string
	// nodes is every structure element reached from the root, built on first use by structNodes, and
	// nodesErr is why part of the tree was not read, when part was not.
	nodes     []structNode
	nodesErr  string
	nodesDone bool
	// roles memoises standardType's walk of the /RoleMap, keyed by every name on the path it followed —
	// the chain is followed to its end now (`/pending 507`), so a document with a long chain and many
	// elements would otherwise re-walk it once per element.
	roles map[string]roleResolution
	// tables memoises each table's layout (`rules_table.go`), keyed by the Table's dictionary (`dictID`), so an
	// inline table is laid out once, like one written as its own object.
	tables map[uintptr]*tableLayout
	// circular memoises roleMapCircular's walk, keyed by the element's own /S (P03.S01).
	circular map[string]bool
	// content is every page's classified drawing operators, built on first use by contentEvents.
	content []contentEvent
	// contentErr is why content could not be read, or not all of it, when it could not.
	contentErr  string
	contentDone bool
	// formWalks counts the form XObjects the content walk has entered, and contentOver records that it spent
	// its budget (`overBudget`).
	formWalks   int
	contentOps  int
	contentOver bool
	// kids memoises elementKids, keyed by the element's dictionary (`dictID`), and kidBudget is how many more
	// kids the document may expand before nib stops (`maxKidExpansion`).
	kids      map[uintptr]kidsResult
	kidBudget int
	// langs memoises parentLang's climb, keyed by the dictionary whose `/P` chain was followed (`dictID`), each
	// answer carrying how far above that dictionary it sits so a deeper climb is refused where a fresh one would be.
	langs map[uintptr]langClimb
	// mcLangCount is how many string `/Lang` values BDC property lists carried in the content walk, and mcLangBad
	// the first that failed the grammar (7.2 t29).
	mcLangCount int
	mcLangBad   *mcLang
	// raw is the file as given, kept for the one question the validated context cannot answer: whether pdfcpu's
	// validator dropped something (`hasInlineType3Font`). directType3 memoises that answer.
	raw         []byte
	directType3 *bool
	// xmp memoises readXMP.
	xmp     xmpFacts
	xmpDone bool
	// tableSlots is every grid slot the document's tables have asked for so far (`maxDocumentTableSlots`).
	tableSlots int64
}

// roleResolution is what one `/S` name resolves to through the role map: a standard structure type, or
// why nib could not follow the map to one. Exactly one of those two is set.
//
// **`circular` is a THIRD fact and it is independent of both** (`/pending 548`). Whether the walk revisits
// a name is not the same question as whether it can be typed, and veraPDF is what separates them: on
// `7.1 General/7.1-t06-fail-a.pdf` the role map is `<< /LI /LI >>` and veraPDF FAILS ua1 7.1-6 on the two
// `LI` elements — a self-map is a circular mapping — while the elements still type as `LI`, because a
// conforming reader recognises `LI` before it ever consults the map. Deriving one fact from the other
// either loses that failure or breaks three shipped rules over a document nothing is wrong with.
type roleResolution struct {
	standard   string
	unresolved string
}

// open parses pdf for the rules to read.
//
// **A document that cannot be opened is an error, not a report full of failures.** A checker that
// answered "fails every clause" for a file it could not parse would be telling the user their
// document is inaccessible when what happened is that nib could not read it — two different facts,
// and the second is not the user's to fix.
func open(pdf []byte) (*Document, error) {
	if len(pdf) == 0 {
		return nil, fmt.Errorf("uacheck: no document to check")
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, fmt.Errorf("uacheck: the document could not be read: %w", err)
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return nil, fmt.Errorf("uacheck: the document has no catalog: %w", cerr)
	}
	return &Document{Ctx: ctx, Catalog: cat, raw: pdf}, nil
}

// declaresLang reports whether obj is a declared `/Lang`: a string, direct or indirect, literal or hex —
// and **present counts, even when empty**.
//
// **`/pending 489`, found on veraPDF's own corpus, in two halves.**
//   - **Encodings.** Every reader here cast to a direct `types.StringLiteral`, which is what nib and
//     LibreOffice write, so law 5's oracle never met anything else. The corpus stores
//     `/Lang <FEFF0045004E002D00550053>` and `/Lang 12 0 R` too, and each read as no language.
//   - **Presence, not content.** veraPDF's 7.2 t33 test is `gContainsCatalogLang` and its 7.2 t34 test
//     is `Lang != null` — the key being there. Its three corpus files `7.2-t29-fail-n/o/p.pdf` declare an
//     EMPTY `/Lang` (on the catalog, on a structure element, on marked content), and veraPDF passes t33
//     and t34 on all three while failing 7.2 t29, whose subject is the empty value. nib does not
//     implement t29, so reading "" as undeclared filed a t29 failure under t33 or t34.
//
// This is the checker's OWN door, not `pdfops`' — structure.go says why the checker keeps its readings
// independent of the writer's.
func (d *Document) declaresLang(obj types.Object) bool {
	_, ok := d.text(obj)
	return ok
}

// boolValue resolves a boolean that may be stored indirectly — `/Marked 42 0 R` with `42 0 obj true`
// is legal, veraPDF reads it as true, and a bare cast read it as absent (`/pending 489`).
func (d *Document) boolValue(obj types.Object) (value, ok bool) {
	if obj == nil {
		return false, false
	}
	b, err := d.Ctx.XRefTable.DereferenceBoolean(obj, model.V10)
	if err != nil || b == nil {
		return false, false
	}
	return b.Value(), true
}

// # The typed-value door — `/pending 496`
//
// Any PDF value may be stored as an indirect object, and a reader that casts a dictionary entry straight to
// `types.Integer` or calls pdfcpu's `NameEntry` sees `42 0 R` as the wrong type. Six rules did, and it cut
// both ways: `/DisplayDocTitle 9 0 R` failed 7.1 t10 on a document veraPDF passes, and a Figure whose `/S`
// was indirect was not a Figure, so its missing alternate text passed 7.3 t1. Every typed read in this
// package goes through these, and `TestTheRulesReadTypedValuesOnlyThroughTheDoor` refuses one that does not.

// intValue resolves an integer that may be stored indirectly.
func (d *Document) intValue(obj types.Object) (int, bool) {
	if obj == nil {
		return 0, false
	}
	i, err := d.Ctx.XRefTable.DereferenceInteger(obj)
	if err != nil || i == nil {
		return 0, false
	}
	return i.Value(), true
}

// name resolves a name that may be stored indirectly, or "" when obj is absent or not a name.
func (d *Document) name(obj types.Object) string {
	if obj == nil {
		return ""
	}
	n, err := d.Ctx.XRefTable.DereferenceName(obj, model.V10, nil)
	if err != nil {
		return ""
	}
	return n.Value()
}

// text resolves a string — literal or hex, direct or indirect — and reports whether obj was one.
//
// **A reference to an object that is not there is NOT one** (P04.S02's review, measured). pdfcpu's
// `DereferenceStringOrHexLiteral` answers `("", nil)` for a reference to a free object, which is
// indistinguishable here from an empty string that is really present — and veraPDF reads such a value as
// absent (`COSObject.getString` over a null base). Measured on 1.30.2 with a readable file whose `/Alt` is
// `99 0 R` and whose object 99 does not exist: veraPDF has no subject for 7.2 t22 and nib FAILED it, and with
// the same reference on `/Lang` veraPDF failed the clause and nib PASSED it — a false fail and a false pass
// from one missing check, in a file pdfcpu's validator does not refuse.
func (d *Document) text(obj types.Object) (string, bool) {
	if obj == nil {
		return "", false
	}
	if r, err := d.Ctx.Dereference(obj); err != nil || r == nil {
		return "", false
	}
	s, err := d.Ctx.XRefTable.DereferenceStringOrHexLiteral(obj, model.V10, nil)
	return s, err == nil
}

// dict resolves obj to a dictionary, or nil.
func (d *Document) dict(obj types.Object) types.Dict {
	if obj == nil {
		return nil
	}
	res, err := d.Ctx.DereferenceDict(obj)
	if err != nil {
		return nil
	}
	return res
}
