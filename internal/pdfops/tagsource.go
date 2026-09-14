package pdfops

import (
	"bytes"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Which of D4's sources produced a structure tree — `PLAN-accessibility.md` P06.S03.
//
// D4: *"Observed-exact → written silently. Observed-approximate → written silently, marked in the
// tag tree as OCR-derived. Inferred → proposed for review. **The user is told which of the three
// produced the tree they are looking at.**"*
//
// # Why a private key, and why on the tree root
//
// `/StructTreeRoot` has exactly six defined entries — `Type`, `K`, `IDTree`, `ParentTree`,
// `ParentTreeNextKey`, `RoleMap`, `ClassMap` — and none of them says where the tree came from. PDF
// permits private keys in any dictionary, so this is one, named for nib so it cannot collide with a
// producer's own.
//
// It goes on the tree root rather than in the XMP because it is a fact ABOUT THE TREE, and the
// place a reader of the tree already is. Metadata is where a cataloguer looks; this is for whoever
// is deciding whether to trust the structure in front of them.
const tagSourceKey = "NibStructureSource"

// tagSource is one of D4's tiers, plus the one D4 does not have a name for.
type tagSource string

const (
	// sourceExact is structure nib read from something that KNOWS it — `mdpdf`'s goldmark AST, or
	// form fields nib itself placed. D4's observed-exact.
	sourceExact tagSource = "Exact"
	// sourceApproximate is structure derived from OCR: tesseract's block/paragraph/line grouping is
	// a good guess about a scan and is not the document's own account of itself. D4's
	// observed-approximate, written silently and marked.
	sourceApproximate tagSource = "Approximate"
	// sourceInferred is the autotagger's proposal over an arbitrary PDF. D4's inferred — proposed
	// for review rather than written, and named here so a tree carrying it can be recognised.
	sourceInferred tagSource = "Inferred"
	// sourceGeneric is grouping with NO semantic claim: P05.S04's emitter, which brackets a page's
	// content as one `/Div` because it knows nothing about what the content says.
	//
	// **D4 has three tiers and this is a fourth**, added because the alternative is calling that
	// tree `Exact` — and a reader told a `/Div`-per-page tree is exact has been told something
	// false about the only thing this key exists to say.
	sourceGeneric tagSource = "Generic"
)

// valid reports whether s is one of the four. A value outside them is a document written by
// something other than this code, and reading it as a tier would be a guess.
func (s tagSource) valid() bool {
	switch s {
	case sourceExact, sourceApproximate, sourceInferred, sourceGeneric:
		return true
	}
	return false
}

// setTagSource records which tier produced the tree.
func setTagSource(ctx *model.Context, src tagSource) error {
	if !src.valid() {
		return fmt.Errorf("pdfops: %q is not one of the structure sources", src)
	}
	cat, err := ctx.XRefTable.Catalog()
	if err != nil {
		return err
	}
	st, ok := cat["StructTreeRoot"]
	if !ok {
		return fmt.Errorf("pdfops: the document has no structure tree to record a source for")
	}
	root, err := ctx.DereferenceDict(st)
	if err != nil || root == nil {
		return fmt.Errorf("pdfops: /StructTreeRoot does not resolve: %w", err)
	}
	root[tagSourceKey] = types.Name(string(src))
	return nil
}

// StructureSource reports which of D4's tiers produced a document's structure tree.
//
// **Three answers, and the distinction between two of them is the point.** `("", false)` means the
// document records nothing — either it has no tree, or it has one nib did not write, or it was
// written before this key existed. `(value, true)` means it says so.
//
// **An unrecorded tree is NOT reported as exact**, and that is the clause this exists to satisfy:
// every tree in the field today carries no record, so a reader that defaulted to the best tier
// would describe every one of them as the most trustworthy kind.
func StructureSource(pdf []byte) (tagSource, bool) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return "", false
	}
	cat, cerr := ctx.XRefTable.Catalog()
	if cerr != nil {
		return "", false
	}
	root, rerr := ctx.DereferenceDict(cat["StructTreeRoot"])
	if rerr != nil || root == nil {
		return "", false
	}
	n, ok := root[tagSourceKey].(types.Name)
	if !ok {
		return "", false
	}
	s := tagSource(n.Value())
	if !s.valid() {
		// A value this code did not write. Reporting it as a tier would be a guess about what
		// another producer meant by a key that happens to share a name.
		return "", false
	}
	return s, true
}

// DescribeStructureSource is the sentence the accessibility report shows about where a document's
// structure came from — D4's *"the user is told which of the three produced the tree they are
// looking at"*, `PLAN-accessibility.md` P07.
//
// **One door for both surfaces** (ADR-009): the UI and `nib ua` each call it, and a guard at the repo
// root checks that, so the two cannot describe one tree differently. An unrecorded tree gets its own
// sentence and never borrows the best tier's — that is `StructureSource`'s second answer carried
// through to the person reading it.
func DescribeStructureSource(pdf []byte) string {
	src, ok := StructureSource(pdf)
	if !ok {
		return "Nib has no record of where this document's structure came from — it may have none, or another program wrote it."
	}
	switch src {
	case sourceExact:
		return "Nib wrote this document's structure from what it already knew — the Markdown's headings and lists, or the form fields it placed."
	case sourceApproximate:
		return "This document's structure was read from a scan by text recognition (OCR) — a good guess, not the document's own account of itself."
	case sourceInferred:
		return "Nib inferred this document's structure from how its pages look — review it before relying on it."
	case sourceGeneric:
		return "This document's structure groups each page as one block and says nothing about what is on it."
	}
	return "Nib has no record of where this document's structure came from — it may have none, or another program wrote it."
}
