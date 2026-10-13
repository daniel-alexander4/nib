package pdfread

import (
	"errors"
	"runtime"
	"sync"
	"weak"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// An associated file the writer would delete — `/pending 655`, extending ADR-129's door.
//
// pdfcpu's writer serialises the catalog and each page dictionary whole and then writes only what a FIXED LIST of
// their keys reaches (`write.go:276-322`, `writePages.go:75-102`, v0.13.0). `/AF` — an associated file, ISO
// 32000-2 §14.13 — is on neither list: after any write a catalog's or a page's `/AF` is still there and the file
// specification it names is not, unless something the writer does follow names the same object. Measured, and
// held by `pdfops`' `TestWhatARewriteDoesToAnAssociatedFile`: a specification also entered in /Names
// /EmbeddedFiles (PDF/A-3 requires that of every embedded file) is carried whole, so is one a page's file
// attachment annotation names through `/FS`, and so is a structure element's `/AF`.
//
// **So a document that arrives holding such a file is not written at all**, and the refusal names its cause
// (`ErrAssociatedFileLost`). The loss is the writer's, and unlike a font the validator took out there is nothing
// to put back: the objects are in the context and the writer does not reach them.
//
// **Why refuse and not carry.** Entering the specification in /EmbeddedFiles for the write would carry it — and
// would do so on EVERY write, which is wrong for the two kinds of operation that take files away on purpose:
// "remove attachments" deletes that name tree and would have the file written back in behind it, and a page
// selection made to hand over (an extract, a split, a redaction) drops the tree so the source's files do not
// ride out inside the result. At the write nothing tells "the operation removed the files" from "nothing ever
// listed this one", so a carry here would resurrect what the user removed.
//
// **Judged on the document as it was READ, and that is deliberate.** An operation that removes the name tree
// from a document whose associated files were all listed leaves `/AF` naming files the user asked to be gone;
// that is not this refusal's business and is written as before. Only a document that arrived with a file `/AF`
// alone reaches is refused. The mark does not follow a context's objects into ANOTHER context: the source of a
// merge, and a page cut or imposed into a new document, lose such a file as before (declared, not held).

// ErrAssociatedFileLost is Write's refusal of a document read with an associated file only `/AF` reaches.
// AssociatedFileRefusal is its text, which the server's error door recognises to answer 422 with a cause.
var ErrAssociatedFileLost = errors.New(AssociatedFileRefusal)

// AssociatedFileRefusal is the sentence a user reads; it names the cause and what would have happened.
const AssociatedFileRefusal = "this document holds an associated file (/AF on the document or on a page) that is not in its list of attachments, and writing a changed copy would delete that file — so nothing was changed"

// afOnly holds the contexts read from a document with an associated file only `/AF` reaches. Keyed weakly, as
// `losses` is: a context nobody writes is collected with its mark.
var afOnly = struct {
	sync.Mutex
	m map[weak.Pointer[model.Context]]bool
}{m: map[weak.Pointer[model.Context]]bool{}}

// rememberAssociatedFiles marks ctx when the document it was read from holds, on its catalog or on a page, an
// `/AF` naming a file the writer will not write. It runs after validation, when every page dictionary is decoded.
//
// One pass over the objects the context already holds, asking each dictionary for one key; the rest runs only
// for a document that has the key at all. Measured on 1,000 / 10,000 / 50,000 one-object pages with no `/AF`:
// 0.37 / 1.0 / 10.5 ms against a validated read of 92 ms / 637 ms / 3.9 s.
func rememberAssociatedFiles(ctx *model.Context) {
	xt := ctx.XRefTable
	var holders []types.Dict
	if root, err := xt.Catalog(); err == nil && root != nil {
		if _, ok := root["AF"]; ok {
			holders = append(holders, root)
		}
	}
	for _, e := range xt.Table {
		if e == nil || e.Free {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if _, has := d["AF"]; !has {
			continue
		}
		if t, _ := d["Type"].(types.Name); t == "Page" {
			holders = append(holders, d)
		}
	}
	if len(holders) == 0 {
		return
	}
	written := writtenFileSpecs(ctx)
	lost := false
	for _, h := range holders {
		if associatedFileLost(xt, h["AF"], written) {
			lost = true
			break
		}
	}
	if !lost {
		return
	}
	key := weak.Make(ctx)
	afOnly.Lock()
	afOnly.m[key] = true
	afOnly.Unlock()
	runtime.AddCleanup(ctx, func(k weak.Pointer[model.Context]) {
		afOnly.Lock()
		delete(afOnly.m, k)
		afOnly.Unlock()
	}, key)
}

// writtenFileSpecs is the object number of every file specification the writer reaches by a road of its own:
// the values of the /EmbeddedFiles name tree, and each page annotation's `/FS`.
func writtenFileSpecs(ctx *model.Context) map[int]bool {
	xt := ctx.XRefTable
	out := map[int]bool{}
	if tree := xt.Names["EmbeddedFiles"]; tree != nil {
		_ = tree.Process(xt, func(_ *model.XRefTable, _ string, v *types.Object) error {
			if ref, ok := (*v).(types.IndirectRef); ok {
				out[ref.ObjectNumber.Value()] = true
			}
			return nil
		})
	}
	for _, p := range Pages(ctx) {
		annots, _ := xt.DereferenceArray(p.Dict["Annots"])
		for _, a := range annots {
			ad, _ := xt.DereferenceDict(a)
			if ref, ok := ad["FS"].(types.IndirectRef); ok {
				out[ref.ObjectNumber.Value()] = true
			}
		}
	}
	return out
}

// associatedFileLost reports whether af — the value of one `/AF` key — names anything the writer will not write.
// The array written as its own object is such a thing itself; so is a specification that is its own object and
// in neither road of writtenFileSpecs, and one written in the array that names an embedded stream (the
// dictionary is written with its holder, the stream it references is not).
func associatedFileLost(xt *model.XRefTable, af types.Object, written map[int]bool) bool {
	if _, isRef := af.(types.IndirectRef); isRef {
		return true
	}
	arr, _ := af.(types.Array)
	for _, el := range arr {
		switch v := el.(type) {
		case types.IndirectRef:
			if !written[v.ObjectNumber.Value()] {
				return true
			}
		case types.Dict:
			if _, has := v["EF"]; has {
				return true
			}
		}
	}
	return false
}

// refuseAssociatedFileLoss is Write's check: ctx was read from a document holding an associated file the write
// would delete.
func refuseAssociatedFileLoss(ctx *model.Context) error {
	afOnly.Lock()
	marked := afOnly.m[weak.Make(ctx)]
	afOnly.Unlock()
	if marked {
		return ErrAssociatedFileLost
	}
	return nil
}
