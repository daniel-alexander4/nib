package pdfops

import (
	"bytes"
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Finding is one item of active or hidden content the scan surfaced.
type Finding struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`       // "high" | "medium" | "low"
	Detail   string `json:"detail"`         // human-readable, honest about what it is
	Page     int    `json:"page,omitempty"` // 1-based; 0 = document-level
}

// ScanReport is the result of Scan: everything hidden or active that was found.
// An empty Findings slice means nothing was detected.
type ScanReport struct {
	Findings []Finding `json:"findings"`
}

// riskyActions are PDF action types that can run code or reach outside the
// document, mapped to a severity. Plain GoTo (internal page navigation) is
// deliberately absent — it is benign and StripActive preserves it.
var riskyActions = map[string]string{
	"JavaScript": "high",
	"Launch":     "high",
	"SubmitForm": "high",
	"ImportData": "high",
	"GoToR":      "high",
	"GoToE":      "high",
	"URI":        "medium",
}

// Scan reads the PDF and reports active or hidden content: auto-run hooks
// (OpenAction / additional actions), JavaScript, risky annotation actions,
// embedded files, optional-content layers, and metadata. It is strictly
// read-only — it opens the document without validation (so it tolerates the
// malformed files this is most useful on) and never writes anything back.
func Scan(pdf []byte) (ScanReport, error) {
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return ScanReport{}, err
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		return ScanReport{}, err
	}

	var rep ScanReport
	add := func(kind, sev, detail string, page int) {
		rep.Findings = append(rep.Findings, Finding{Kind: kind, Severity: sev, Detail: detail, Page: page})
	}

	// Document-level auto-run hooks.
	if _, ok := root.Find("OpenAction"); ok {
		add("openAction", "high", "Runs an action automatically when the document opens", 0)
	}
	if _, ok := root.Find("AA"); ok {
		add("additionalActions", "medium", "Document-level additional actions (run on print, save, or close)", 0)
	}

	// Catalog name trees: JavaScript and embedded files.
	if names := derefDict(xt, root["Names"]); names != nil {
		if _, ok := names.Find("JavaScript"); ok {
			add("javascript", "high", "Document-level JavaScript", 0)
		}
		if _, ok := names.Find("EmbeddedFiles"); ok {
			add("attachment", "medium", "Embedded files attached to the document", 0)
		}
	}

	// XFA forms can carry their own scripts.
	if af := derefDict(xt, root["AcroForm"]); af != nil {
		if _, ok := af.Find("XFA"); ok {
			add("xfa", "medium", "XFA form (can contain its own scripts)", 0)
		}
	}

	// Optional content (layers); flag any hidden by default.
	if ocp := derefDict(xt, root["OCProperties"]); ocp != nil {
		hidden := 0
		if d := derefDict(xt, ocp["D"]); d != nil {
			hidden = len(derefArray(xt, d["OFF"]))
		}
		if hidden > 0 {
			add("hiddenLayer", "medium", fmt.Sprintf("%d optional-content layer(s) hidden by default", hidden), 0)
		} else {
			add("hiddenLayer", "low", "Optional-content layers (none hidden by default)", 0)
		}
	}

	// XMP metadata stream.
	if _, ok := root.Find("Metadata"); ok {
		add("metadata", "low", "XMP metadata stream", 0)
	}

	// Page-level additional actions, attachment annotations, and link/widget actions.
	eachPage(xt, root, func(page types.Dict, nr int) {
		if _, ok := page.Find("AA"); ok {
			add("additionalActions", "medium", "Page additional actions (run on open or close)", nr)
		}
		for _, a := range derefArray(xt, page["Annots"]) {
			annot := derefDict(xt, a)
			if annot == nil {
				continue
			}
			if nameVal(annot, "Subtype") == "FileAttachment" {
				add("attachment", "medium", "File attached to a page", nr)
			}
			if _, ok := annot.Find("AA"); ok {
				add("additionalActions", "medium", "Annotation additional actions", nr)
			}
			if act := derefDict(xt, annot["A"]); act != nil {
				if sev, ok := riskyActions[nameVal(act, "S")]; ok {
					add("action", sev, actionDetail(nameVal(act, "S")), nr)
				}
			}
		}
	})

	return rep, nil
}

// StripActive neutralizes all active content while preserving the document's
// visible pages, vector text, and benign internal navigation: it deletes
// document/page/annotation auto-run hooks, JavaScript, optional-content layer
// machinery, XFA, and the action on any link/widget that could run code or
// reach outside the document, and removes embedded files. The annotations
// themselves are kept. Because it rewrites the file, any existing signature is
// invalidated — the result is a new, unsigned PDF.
func StripActive(pdf []byte) ([]byte, error) {
	return writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, err := xt.Catalog()
		if err != nil {
			return err
		}
		_ = xt.DeleteDictEntry(root, "OpenAction")
		_ = xt.DeleteDictEntry(root, "AA")
		_ = xt.DeleteDictEntry(root, "OCProperties")
		if names := derefDict(xt, root["Names"]); names != nil {
			_ = xt.DeleteDictEntry(names, "JavaScript")
		}
		if af := derefDict(xt, root["AcroForm"]); af != nil {
			_ = xt.DeleteDictEntry(af, "XFA")
		}
		eachPage(xt, root, func(page types.Dict, _ int) {
			_ = xt.DeleteDictEntry(page, "AA")
			for _, a := range derefArray(xt, page["Annots"]) {
				annot := derefDict(xt, a)
				if annot == nil {
					continue
				}
				_ = xt.DeleteDictEntry(annot, "AA")
				if act := derefDict(xt, annot["A"]); act != nil {
					if _, risky := riskyActions[nameVal(act, "S")]; risky {
						_ = xt.DeleteDictEntry(annot, "A")
					}
				}
			}
		})
		return removeAllAttachments(ctx)
	})
}

// RemoveFilesAndMedia removes embedded files and dangerous media annotations
// (file attachments, sound, movie, screen, 3D) through pdfcpu's own removal
// APIs, leaving everything else — including interactivity and any active code —
// untouched. It cannot corrupt page content, so it is the gentle middle option
// between StripActive (removes all active content) and the guaranteed flatten.
func RemoveFilesAndMedia(pdf []byte) ([]byte, error) {
	return writeMutated(pdf, func(ctx *model.Context) error {
		if err := removeAllAttachments(ctx); err != nil {
			return err
		}
		mediaTypes := []string{"FileAttachment", "Sound", "Movie", "Screen", "3D"}
		_, err := pdfcpu.RemoveAnnotations(ctx, nil, mediaTypes, nil, false)
		return err
	})
}

// Validate reports whether pdf parses as a structurally sound PDF. It is the
// gate after a strip: success means the surgical removal produced a valid
// document; an error means the UI should recommend stepping down to flatten.
func Validate(pdf []byte) error {
	_, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	return err
}

// writeMutated reads pdf into a validated, optimized context (so WriteContext
// has the consolidated structures it expects and the attachment/annotation
// caches are populated), applies fn, and writes the result back. It is the
// shared read→mutate→write shape for the surgical removals.
func writeMutated(pdf []byte, fn func(*model.Context) error) ([]byte, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	if err := fn(ctx); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// removeAllAttachments deletes every embedded file, treating "no attachments" as
// success rather than an error (RemoveAttachments errors when the name tree is
// absent).
func removeAllAttachments(ctx *model.Context) error {
	aa, err := ctx.ListAttachments()
	if err != nil || len(aa) == 0 {
		return nil
	}
	_, err = ctx.RemoveAttachments(nil)
	return err
}

// eachPage invokes fn for each leaf page dict in document order (1-based),
// walking the page tree directly so it needs no validated PageCount. A depth
// cap and a per-walk visited set guard against malformed or cyclic trees.
func eachPage(xt *model.XRefTable, root types.Dict, fn func(page types.Dict, nr int)) {
	pages := derefDict(xt, root["Pages"])
	if pages == nil {
		return
	}
	nr := 0
	seen := map[int]bool{}
	var walk func(node types.Dict, depth int)
	walk = func(node types.Dict, depth int) {
		if node == nil || depth > 50 {
			return
		}
		kids := derefArray(xt, node["Kids"])
		if len(kids) == 0 {
			nr++
			fn(node, nr)
			return
		}
		for _, k := range kids {
			if ir, ok := k.(types.IndirectRef); ok {
				n := ir.ObjectNumber.Value()
				if seen[n] {
					continue
				}
				seen[n] = true
			}
			walk(derefDict(xt, k), depth+1)
		}
	}
	walk(pages, 0)
}

func derefDict(xt *model.XRefTable, o types.Object) types.Dict {
	if o == nil {
		return nil
	}
	d, _ := xt.DereferenceDict(o)
	return d
}

func derefArray(xt *model.XRefTable, o types.Object) types.Array {
	if o == nil {
		return nil
	}
	a, _ := xt.DereferenceArray(o)
	return a
}

func nameVal(d types.Dict, key string) string {
	if n := d.NameEntry(key); n != nil {
		return *n
	}
	return ""
}

// actionDetail describes a risky action type in plain English.
func actionDetail(s string) string {
	switch s {
	case "JavaScript":
		return "Runs JavaScript"
	case "Launch":
		return "Launches an external program or file"
	case "SubmitForm":
		return "Submits form data to a URL"
	case "ImportData":
		return "Imports form data from a file"
	case "GoToR", "GoToE":
		return "Opens another document"
	case "URI":
		return "Opens a web URL"
	default:
		return s + " action"
	}
}
