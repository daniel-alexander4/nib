package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Finding is one item of active or hidden content the scan surfaced.
type Finding struct {
	// Kind is the finding's category ("javascript", "launch", …) and is what Severity and
	// Detail are derived FROM at the append site. It is deliberately not on the wire: the
	// client renders severity, detail and page, and nothing anywhere read `kind` at the far
	// end — /pending 259 found it published and unread, hidden behind 31 matches of `f.kind`
	// on the client's own form-field objects, an unrelated shape. Kept as a field because it
	// is real internal information; dropped from the JSON because a published field nobody
	// consumes is the historyEvicted class.
	Kind     string `json:"-"`
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
	// ISO 32000-1 §12.6.4's remaining action types that run something, reach outside
	// the document, or change what is visible. Rendition is high because a rendition
	// action can carry its own JavaScript (§12.6.4.13); Hide and SetOCGState are the
	// hidden-content half of what Scan exists to report, and a document that hides
	// content on open reads as clean without them.
	"Rendition":   "high",
	"Movie":       "medium",
	"Sound":       "medium",
	"SetOCGState": "medium",
	"GoTo3DView":  "medium",
	"Hide":        "medium",
}

// eachAction walks an action and everything its /Next chains to, calling fn on each.
//
// **Without this, one benign action hides any number of risky ones.** /Next (§12.6.1) is
// a dict or an ARRAY of dicts, each of which may chain further, and both Scan and
// StripActive used to look only at the head: `<< /S /GoTo /Next << /S /JavaScript >> >>`
// scanned clean and survived the strip untouched. Depth is bounded because /Next can be
// circular in a malformed file and this runs on documents chosen for being malformed.
func eachAction(xt *model.XRefTable, act types.Dict, depth int, fn func(types.Dict)) {
	if act == nil || depth > 32 {
		return
	}
	fn(act)
	switch next := act["Next"].(type) {
	case types.Array:
		for _, a := range next {
			eachAction(xt, derefDict(xt, a), depth+1, fn)
		}
	default:
		if d := derefDict(xt, act["Next"]); d != nil {
			eachAction(xt, d, depth+1, fn)
		}
	}
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
		// The FIELD TREE, which neither this scan nor StripActive walked.
		//
		// The page walk above catches /AA and /A on widget ANNOTATIONS, which covers the
		// merged field+widget dict — the shape a single-widget field takes. It does not
		// cover a field dict that is the PARENT of its widget kids, which is what Acrobat
		// produces for a multi-widget field and is the standard home (§12.7.5.3) for the
		// /AA /K keystroke, /F format, /V validate and /C calculate scripts. So a PDF whose
		// only active content is field-level JavaScript scanned CLEAN, and — because
		// server/scan.go's residual re-scan is this same detector — StripActive then
		// reported "all active content neutralized" with the scripts still in place.
		eachFormField(xt, af, func(f types.Dict) {
			if _, ok := f.Find("AA"); ok {
				add("additionalActions", "medium",
					"Form field additional actions (keystroke, format, validate or calculate script)", 0)
			}
			eachAction(xt, derefDict(xt, f["A"]), 0, func(act types.Dict) {
				if sev, ok := riskyActions[nameVal(act, "S")]; ok {
					add("action", sev, actionDetail(nameVal(act, "S")), 0)
				}
			})
		})
		// /CO is the calculation ORDER — an array of the fields whose /AA /C scripts run
		// and in what sequence. Its presence is the tell that calculate scripts exist even
		// if a field dict is malformed enough that the walk above missed one.
		if co := derefArray(xt, af["CO"]); len(co) > 0 {
			add("additionalActions", "medium",
				"Form calculation order (fields with calculate scripts)", 0)
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

	// Document information dictionary: identifying properties (author, title, …).
	// ReadContext sets only xt.Info (the indirect ref), never the convenience
	// fields (xt.Author/Title/…, which are populated by validate, which we skip),
	// so deref the dict and read each entry as text directly.
	if xt.Info != nil {
		if info := derefDict(xt, *xt.Info); info != nil {
			for _, key := range []string{"Author", "Creator", "Title", "Subject", "Keywords"} {
				o, ok := info.Find(key)
				if !ok {
					continue
				}
				v, err := xt.DereferenceText(o)
				if err != nil {
					continue
				}
				if v = strings.TrimSpace(v); v != "" {
					add("info", "low", key+": "+clip(v, 80), 0)
				}
			}
		}
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
			eachAction(xt, derefDict(xt, annot["A"]), 0, func(act types.Dict) {
				if sev, ok := riskyActions[nameVal(act, "S")]; ok {
					add("action", sev, actionDetail(nameVal(act, "S")), nr)
				}
			})
		}
	})

	return rep, nil
}

// eachFormField calls fn for every dict in the AcroForm field tree, parents included.
//
// The tree is /AcroForm /Fields with /Kids beneath, and a node is both a field and a widget
// annotation when a field has exactly one widget — which is why the page-level annotation
// walk covers that case and only that case. Parents of multi-widget fields are reachable
// only from here.
//
// Cycles are bounded by object number, not by depth: a /Kids array that points back at an
// ancestor is a document the reader still opens, and this package has already paid once for
// a walk that recursed on one (see eachPage's own cycle guard). Direct dicts have no object
// number, so they are bounded by depth as well.
func eachFormField(xt *model.XRefTable, af types.Dict, fn func(types.Dict)) {
	seen := map[int]bool{}
	var walk func(o types.Object, depth int)
	walk = func(o types.Object, depth int) {
		if depth > 32 {
			return
		}
		if ir, ok := o.(types.IndirectRef); ok {
			n := ir.ObjectNumber.Value()
			if seen[n] {
				return
			}
			seen[n] = true
		}
		f := derefDict(xt, o)
		if f == nil {
			return
		}
		fn(f)
		for _, k := range derefArray(xt, f["Kids"]) {
			walk(k, depth+1)
		}
	}
	for _, f := range derefArray(xt, af["Fields"]) {
		walk(f, 0)
	}
}

// StripActive neutralizes all active content while preserving the document's
// visible pages, vector text, and benign internal navigation: it deletes
// document/page/annotation auto-run hooks, JavaScript, optional-content layer
// machinery, XFA, and the action on any link/widget that could run code or
// reach outside the document, and removes embedded files. The annotations
// themselves are kept. It never edits a content stream or an annotation
// appearance (/AP) stream, so a page's rendering is unchanged except that
// dropping /OCProperties reveals optional-content layers (reveal-only — content
// is never hidden or lost), the intended hidden-content reveal; removing /XFA
// has no effect in Nib's own view (pdf.js XFA rendering is off by default via
// the raw getDocument API), though an XFA-capable external viewer would collapse
// a stripped dynamic form. Because it rewrites the file, any existing signature
// is invalidated — the result is a new, unsigned PDF.
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
			// The field tree and the calculation order, for the reason Scan now walks
			// them: /AA on a PARENT field dict is not on any annotation, so the page walk
			// never saw it. Deleting /CO alone would leave the scripts and only remove the
			// order they run in.
			eachFormField(xt, af, func(f types.Dict) {
				_ = xt.DeleteDictEntry(f, "AA")
				_ = xt.DeleteDictEntry(f, "A")
			})
			_ = xt.DeleteDictEntry(af, "CO")
		}
		// The media annotations RemoveFilesAndMedia takes out. StripActive is the STRONGER
		// tier and used to leave them: a Screen or Movie annotation survived the strip
		// while the gentle option removed it, so "remove all active content" left behind
		// the annotations whose whole purpose is to play something. The hierarchy has to
		// hold in the direction users are told it does.
		if _, err := pdfcpu.RemoveAnnotations(ctx, nil, []string{"FileAttachment", "Sound", "Movie", "Screen", "3D"}, nil, false); err != nil {
			return err
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
					risky := false
					// The whole chain, not the head: a benign /GoTo whose /Next runs
					// JavaScript is a risky action wearing a safe name. Dropping /A drops
					// the chain with it, which is the only answer that cannot leave a
					// dangling /Next — keeping the head and rewriting the chain would mean
					// re-parenting actions this function has no way to validate.
					eachAction(xt, act, 0, func(a types.Dict) {
						if _, bad := riskyActions[nameVal(a, "S")]; bad {
							risky = true
						}
					})
					if risky {
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

// StripMetadata removes the document's identifying metadata: it drops the whole
// /Info dictionary (Author, Creator, Title, Subject, Keywords, …), deletes the XMP
// /Metadata stream from the catalog and every page, and regenerates the trailer
// /ID so the original permanent identifier no longer travels with the file.
//
// One residue is unavoidable through pdfcpu's writer: for PDFs older than 2.0 it
// re-stamps a generic Producer ("pdfcpu …") and fresh CreationDate/ModDate on
// write, so the output names the tool and the processing time — but the
// personally identifying fields are gone. Like the other Secure-tab removals it
// rewrites the file, so the result is a new, unsigned PDF.
func StripMetadata(pdf []byte) ([]byte, error) {
	return writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, err := xt.Catalog()
		if err != nil {
			return err
		}
		ctx.Info = nil // ensureInfoDict re-adds only Producer/dates for <PDF2.0; nothing reads the cleared fields
		ctx.ID = nil   // nil forces a fresh pair; otherwise /ID[0] is preserved as a permanent tracker
		_ = xt.DeleteDictEntry(root, "Metadata")
		eachPage(xt, root, func(page types.Dict, _ int) {
			_ = xt.DeleteDictEntry(page, "Metadata") // page-level XMP duplicates dc:title/creator too
		})
		return nil
	})
}

// ErrWrongPassword and ErrNotEncrypted classify the two expected failures of
// RemovePassword so the caller can respond specifically (reprompt vs "already
// unprotected") instead of a generic error. ErrAlreadyEncrypted is the inverse,
// reported by Encrypt: pdfcpu refuses to re-encrypt an already-protected PDF, so
// the caller can say so cleanly (e.g. skip it in a batch) rather than surface the
// raw engine error.
var (
	ErrWrongPassword    = errors.New("wrong password")
	ErrNotEncrypted     = errors.New("document is not password-protected")
	ErrAlreadyEncrypted = errors.New("document is already password-protected")
)

// Encrypt password-protects pdf with AES-256, producing a copy that needs the
// password to open. The single password is set as both the user (open) and owner
// password, so RemovePassword(password) reverses it exactly. password must be
// non-empty — an empty one is rejected rather than silently producing an
// unprotected file — and an already-encrypted input returns ErrAlreadyEncrypted
// (pdfcpu will not re-encrypt). Like RemovePassword, api.Encrypt rewrites the
// file, so any existing signature does not survive: protection is a separate,
// signature-free export, never combined with signing.
func Encrypt(pdf []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("a password is required to protect the document")
	}
	conf := model.NewAESConfiguration(password, password, 256)
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &out, conf); err != nil {
		if strings.Contains(err.Error(), "this file is encrypted") {
			return nil, ErrAlreadyEncrypted
		}
		return nil, err
	}
	return out.Bytes(), nil
}

// RemovePassword strips a PDF's encryption — both an open/user password and
// owner-password restriction flags — producing a plain, unrestricted document.
// password is the user's open or owner password; an empty string is enough to
// drop owner-only restrictions (where the open password is itself empty). It does
// NOT crack anything: only the supplied password is tried, and a wrong or missing
// one returns ErrWrongPassword. This is qpdf --decrypt-style unlocking of a
// document the local user is authorized to edit. Because api.Decrypt rewrites the
// file, any existing signature does not survive (the caller warns where it can).
func RemovePassword(pdf []byte, password string) ([]byte, error) {
	conf := model.NewDefaultConfiguration()
	// pdfcpu validates the owner password first, then the user password, so
	// supplying the typed secret as both accepts whichever one it actually is.
	conf.UserPW = password
	conf.OwnerPW = password
	var out bytes.Buffer
	if err := api.Decrypt(bytes.NewReader(pdf), &out, conf); err != nil {
		if errors.Is(err, pdfcpu.ErrWrongPassword) || strings.Contains(err.Error(), "correct password") {
			return nil, ErrWrongPassword
		}
		if strings.Contains(err.Error(), "not encrypted") {
			return nil, ErrNotEncrypted
		}
		return nil, err
	}
	return out.Bytes(), nil
}

// clip shortens s to at most max runes, appending an ellipsis when it truncates.
func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
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
	// onPath, not a global seen-set. The set was there to stop a malformed tree recursing
	// forever, and it did — but it also SKIPPED a page object referenced twice, so the
	// numbering diverged from the viewer's: pdf.js walks the tree without deduplicating and
	// shows two pages, while Scan counted one and reported every later finding a page early.
	// On a security scan that sends the user to the wrong page to look at what was found.
	//
	// Scoped to the current path instead: a reference back to an ancestor is a cycle and is
	// refused; the same object appearing twice in different places is a duplicate, is
	// counted twice, and matches what the reader renders.
	onPath := map[int]bool{}
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
			n := -1
			if ir, ok := k.(types.IndirectRef); ok {
				n = ir.ObjectNumber.Value()
				if onPath[n] {
					continue // a cycle: this node is its own ancestor
				}
				onPath[n] = true
			}
			walk(derefDict(xt, k), depth+1)
			if n >= 0 {
				delete(onPath, n)
			}
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
	case "Rendition":
		return "Plays media, and can carry its own JavaScript"
	case "Movie", "Sound":
		return "Plays embedded media"
	case "SetOCGState":
		return "Changes which optional-content layers are visible"
	case "GoTo3DView":
		return "Switches a 3D annotation's view"
	case "Hide":
		return "Hides or shows annotations"
	default:
		return s + " action"
	}
}
