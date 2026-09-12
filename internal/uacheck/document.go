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
