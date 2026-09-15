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

	// pt is the resolved /ParentTree, built on first use by parentTree.
	pt map[int]types.Object
	// content is every page's classified drawing operators, built on first use by contentEvents.
	content []contentEvent
	// contentErr is why content could not be read, when it could not.
	contentErr  string
	contentDone bool
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
	return &Document{Ctx: ctx, Catalog: cat}, nil
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
	if obj == nil {
		return false
	}
	_, err := d.Ctx.XRefTable.DereferenceStringOrHexLiteral(obj, model.V10, nil)
	return err == nil
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
