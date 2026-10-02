package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"nib/internal/pdfread"
	"reflect"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
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
// scanned clean and survived the strip untouched.
//
// **Each indirect action is visited once** (the phase-close review of PLAN-returned-document P01): a depth cap
// alone bounds a cycle's length, not its fan-out, so `/Next [10 0 R 10 0 R]` on object 10 was walked 2^32 times and
// hung `Scan` — which reads with pdfcpu's unvalidated reader and so meets no `pdfread` loop check. This is
// `eachFormField`'s rule (`/pending 689`), and the depth cap stays for chains of direct dicts. A `/Next` that is a
// reference to an ARRAY is followed too; it used to fall through to a dict dereference and hide its actions.
//
// **The budget is the WHOLE scan's, shared by every call** (the phase-close review of PLAN-returned-document P02):
// `seen` is per call, so K annotations naming one chain of N actions walked it K times, and pages sharing one
// `/Annots` array multiplied that again — a 15 KB file held `Scan` for 3 min 46 s. Each action visited spends one;
// past zero the walk stops and the budget reads negative — for good — which the caller refuses
// (`errActionWalkTooLarge`). Each call first adds `actionAllowance`: the budget is sized by indirect objects, and a
// DIRECT annotation's own direct action is no object, so 1,100 of them on one page were refused (the re-review) while
// walking each costs only what the file holds. What the allowance does not cover is re-walking a SHARED graph.
func eachAction(xt *model.XRefTable, act types.Dict, depth int, budget *int, fn func(types.Dict)) {
	if *budget < 0 {
		return
	}
	*budget += actionAllowance
	// Breadth-first, so an object is first reached at its SHALLOWEST depth (the re-review of the fix above): depth-first
	// marked an action seen where a long chain reached it past the cap, unwalked, and the same action named at depth 1
	// was then skipped — `/Next [<32-action chain> 50 0 R]` scanned clean with 50 a JavaScript action, and StripActive
	// kept it. A node is cut only when every path to it is deeper than the cap.
	type item struct {
		d     types.Dict
		depth int
	}
	seen := map[int]bool{}
	queue := []item{{act, depth}}
	enqueue := func(o types.Object, depth int) {
		if ir, ok := o.(types.IndirectRef); ok {
			n := ir.ObjectNumber.Value()
			if seen[n] {
				return
			}
			seen[n] = true
		}
		queue = append(queue, item{derefDict(xt, o), depth})
	}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		if it.d == nil || it.depth > 32 {
			continue
		}
		if *budget <= 0 {
			*budget = -1
			return
		}
		*budget--
		fn(it.d)
		next, ok := it.d["Next"]
		if !ok {
			continue
		}
		if arr := derefArray(xt, next); arr != nil {
			if !firstVisit(seen, next) {
				continue
			}
			// Queuing the array is work too: entries that resolve to nothing are dropped at dequeue for free, so K
			// annotations over one action whose `/Next` names n of them cost K·n uncharged (the second re-review:
			// 44 KB, 133 s). Every entry spends one.
			if *budget -= len(arr); *budget < 0 {
				*budget = -1
				return
			}
			for _, a := range arr {
				enqueue(a, it.depth+1)
			}
			continue
		}
		enqueue(next, it.depth+1)
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
	actions := pageWalkBudget(xt)
	add := func(kind, sev, detail string, page int) {
		rep.Findings = append(rep.Findings, Finding{Kind: kind, Severity: sev, Detail: detail, Page: page})
	}

	// Document-level auto-run hooks.
	//
	// **The KEY is not the finding; the action dictionary is** (/pending 555). `/OpenAction` is one
	// key with two meanings (ISO 32000-1 §12.3.2) — an action dictionary, which runs, and a
	// destination, which only says which page to open at. Reporting the presence of the key called
	// the second one *"runs an action automatically"* at high severity, about a document that runs
	// nothing. The split is made in one place for both readers of this key; see `openActionForm`.
	if raw, ok := root.Find("OpenAction"); ok {
		if _, act := openActionForm(xt, raw); act != nil {
			add("openAction", "high", "Runs an action automatically when the document opens", 0)
		}
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
		eachFormField(xt, af, func(_ types.Object, f types.Dict) {
			if _, ok := f.Find("AA"); ok {
				add("additionalActions", "medium",
					"Form field additional actions (keystroke, format, validate or calculate script)", 0)
			}
			eachAction(xt, derefDict(xt, f["A"]), 0, &actions, func(act types.Dict) {
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
	if err := eachPage(xt, root, func(page types.Dict, nr int) {
		if _, ok := page.Find("AA"); ok {
			add("additionalActions", "medium", "Page additional actions (run on open or close)", nr)
		}
	}); err != nil {
		return ScanReport{}, err
	}
	if err := eachPageAnnot(xt, root, func(annot types.Dict, nr int) {
		if nameVal(annot, "Subtype") == "FileAttachment" {
			add("attachment", "medium", "File attached to a page", nr)
		}
		if _, ok := annot.Find("AA"); ok {
			add("additionalActions", "medium", "Annotation additional actions", nr)
		}
		eachAction(xt, derefDict(xt, annot["A"]), 0, &actions, func(act types.Dict) {
			if sev, ok := riskyActions[nameVal(act, "S")]; ok {
				add("action", sev, actionDetail(nameVal(act, "S")), nr)
			}
		})
	}); err != nil {
		return ScanReport{}, err
	}
	if actions < 0 {
		return ScanReport{}, errActionWalkTooLarge
	}

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
//
// **It is the ONE walk of the field tree** (ADR-009, `/pending 689`). `dropSignature` kept its own
// recursion with a depth cap and no visited set, so a field whose `/Kids` named one child k times was
// walked k^depth times — measured, 1,596 bytes (k=4, 12 levels) cost `RemovePages` 1.15 s and k=8 did
// not finish in 300 s. fn receives the object as the tree NAMES it as well as the dict, because a
// caller that marks what it finds by object number needs the reference. The depth cap is the larger
// of the two the walks used (50), so neither caller lost reach by the merge.
func eachFormField(xt *model.XRefTable, af types.Dict, fn func(o types.Object, f types.Dict)) {
	// Breadth-first for eachAction's reason: depth-first marked a shared field seen where a deep path reached it at
	// the cap, its /Kids cut, and the shallow path to it then skipped — so its subtree was never visited.
	type item struct {
		o     types.Object
		depth int
	}
	seen := map[int]bool{}
	var queue []item
	enqueue := func(o types.Object, depth int) {
		if depth > 50 {
			return
		}
		if ir, ok := o.(types.IndirectRef); ok {
			n := ir.ObjectNumber.Value()
			if seen[n] {
				return
			}
			seen[n] = true
		}
		queue = append(queue, item{o, depth})
	}
	for _, f := range derefArray(xt, af["Fields"]) {
		enqueue(f, 0)
	}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		f := derefDict(xt, it.o)
		if f == nil {
			continue
		}
		fn(it.o, f)
		if !firstVisit(seen, f["Kids"]) {
			continue
		}
		for _, k := range derefArray(xt, f["Kids"]) {
			enqueue(k, it.depth+1)
		}
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
//
// **It verifies its own claim** (`/pending 729`): the output is re-scanned and anything active left in
// it is an error (ErrActiveContentRemains), never a success the caller reports. See verifyStripped.
func StripActive(pdf []byte) ([]byte, error) {
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, err := xt.Catalog()
		if err != nil {
			return err
		}
		// **Deleted whole, including the destination form `Scan` no longer reports** — a named
		// exemption from the one-door rule rather than an oversight (/pending 555, ADR-009). The two
		// doors answer different questions: `Scan` says what is HERE, and mislabelling a view as an
		// auto-run hook is a false statement about somebody's document, while this door removes what
		// could run, and over-removing an opening view costs a reader one scroll. Reducing a
		// `/S /GoTo` to its destination the way `carriedOpenAction` does would be the symmetric
		// answer; it is deliberately not taken here, because it turns a delete into a rewrite at the
		// one door whose whole value is that it only ever takes things away.
		dropKey(xt, root, "OpenAction")
		dropKey(xt, root, "AA")
		dropKey(xt, root, "OCProperties")
		if names := derefDict(xt, root["Names"]); names != nil {
			dropKey(xt, names, "JavaScript")
		}
		// **And pdfcpu's parsed copy of the tree**, which its writer binds back into `/Names` on every
		// write (`ctx.BindNameTrees`, write.go:340-344) — so deleting the key alone restored it, and every
		// non-empty document-level JavaScript tree survived StripActive (`/pending 729`: found by the
		// verifier below the first time it ran; craftActivePDF's tree was empty, which is never parsed).
		delete(xt.Names, "JavaScript")
		if af := derefDict(xt, root["AcroForm"]); af != nil {
			dropKey(xt, af, "XFA")
			// The field tree and the calculation order, for the reason Scan now walks
			// them: /AA on a PARENT field dict is not on any annotation, so the page walk
			// never saw it. Deleting /CO alone would leave the scripts and only remove the
			// order they run in.
			eachFormField(xt, af, func(_ types.Object, f types.Dict) {
				dropKey(xt, f, "AA")
				dropKey(xt, f, "A")
			})
			dropKey(xt, af, "CO")
		}
		// The media annotations RemoveFilesAndMedia takes out. StripActive is the STRONGER
		// tier and used to leave them: a Screen or Movie annotation survived the strip
		// while the gentle option removed it, so "remove all active content" left behind
		// the annotations whose whole purpose is to play something. The hierarchy has to
		// hold in the direction users are told it does.
		if _, err := pdfcpu.RemoveAnnotations(ctx, nil, []string{"FileAttachment", "Sound", "Movie", "Screen", "3D"}, nil, false); err != nil {
			return err
		}
		if err := eachPage(xt, root, func(page types.Dict, _ int) {
			dropKey(xt, page, "AA")
		}); err != nil {
			return err
		}
		actions := pageWalkBudget(xt)
		if err := eachPageAnnot(xt, root, func(annot types.Dict, _ int) {
			dropKey(xt, annot, "AA")
			if act := derefDict(xt, annot["A"]); act != nil {
				risky := false
				// The whole chain, not the head: a benign /GoTo whose /Next runs
				// JavaScript is a risky action wearing a safe name. Dropping /A drops
				// the chain with it, which is the only answer that cannot leave a
				// dangling /Next — keeping the head and rewriting the chain would mean
				// re-parenting actions this function has no way to validate.
				eachAction(xt, act, 0, &actions, func(a types.Dict) {
					if _, bad := riskyActions[nameVal(a, "S")]; bad {
						risky = true
					}
				})
				if risky {
					dropKey(xt, annot, "A")
				}
			}
		}); err != nil {
			return err
		}
		if actions < 0 {
			return errActionWalkTooLarge
		}
		return removeAllAttachments(ctx)
	})
	if err != nil {
		return nil, err
	}
	if err := verifyStripped(out); err != nil {
		return nil, err
	}
	return out, nil
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
		dropKey(xt, root, "Metadata")
		if err := eachPage(xt, root, func(page types.Dict, _ int) {
			dropKey(xt, page, "Metadata") // page-level XMP duplicates dc:title/creator too
		}); err != nil {
			return err
		}
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
// password, so RemovePassword(password) reverses it exactly.
//
// **The password controls opening and nothing else, and the file says so** (/pending 640). With one
// password for both roles, whoever can open the copy authenticates as its OWNER, and an owner is
// bound by no permission bit — pdfcpu skips them (`read.go`'s owner branch) as the PDF spec says a
// reader must. So restricting printing, copying or editing would write flags that bind nobody, while
// `NewAESConfiguration`'s default denied all of them (`/P` 0xF0C3), including bit 10, "extract for
// accessibility". Every permission is granted instead, and the configuration is pinned rather than
// read from the user's `~/.config/pdfcpu/config.yml` (`protectConfig`). password must be
// non-empty — an empty one is rejected rather than silently producing an
// unprotected file — and an already-encrypted input returns ErrAlreadyEncrypted
// (pdfcpu will not re-encrypt). Like RemovePassword, api.Encrypt rewrites the
// file, so any existing signature does not survive: protection is a separate,
// signature-free export, never combined with signing.
func Encrypt(pdf []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("a password is required to protect the document")
	}
	conf := protectConfig(password)
	var out bytes.Buffer
	rs, err := pdfread.Reader(pdf, conf)
	if err != nil {
		return nil, err
	}
	if err := api.Encrypt(rs, &out, conf); err != nil {
		if strings.Contains(err.Error(), "this file is encrypted") {
			return nil, ErrAlreadyEncrypted
		}
		return nil, err
	}
	return out.Bytes(), nil
}

// protectConfig is Encrypt's configuration: what the protection promises — AES-256, both passwords, and every
// permission — is set here, never taken from the user's `~/.config/pdfcpu/config.yml`, which
// `NewDefaultConfiguration` would otherwise read (its `permissions` line decided nib's flags before /pending 640).
// `NewAESConfiguration` itself sets the cipher and key length after loading that file.
func protectConfig(password string) *model.Configuration {
	c := model.NewAESConfiguration(password, password, 256)
	// Every permission bit from 3 to 12 (and the reserved high bits) — but not bits 1-2, which ISO 32000 says must
	// be 0: `PermissionsAll` is 0xFFFF and would write `/P -1` (the review, reading pdfcpu's `newEncryptDict`).
	c.Permissions = model.PermissionsAll &^ 3
	return c
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
	rs, err := pdfread.Reader(pdf, conf)
	if err != nil {
		return nil, err
	}
	if err := api.Decrypt(rs, &out, conf); err != nil {
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
	_, err := inspectionRead(pdf)
	return err
}

// dropKey removes key from d, and it removes the KEY whatever pdfcpu says about the object behind it
// (`/pending 729`, ADR-009: every key a scrub removes goes through here).
//
// pdfcpu's `DeleteDictEntry` deletes the key only AFTER `DeleteObject` has freed the whole object graph
// beneath it, and returns early on any error there — a single reference with no xref entry (`/Foo 99 0 R`
// inside an action) is enough. Every scrub discarded that error, so a hostile action kept its key, the
// scrub reported success and `nib sanitize` wrote the file and exited 0. The key is what a viewer follows:
// an object nothing names is unreachable, so freeing the graph is hygiene and is best-effort here, while
// the key's removal is the security property and is unconditional. Objects already freed when the graph
// walk failed were reachable only through this key, which is gone.
func dropKey(xt *model.XRefTable, d types.Dict, key string) {
	o, found := d.Find(key)
	if !found {
		return
	}
	_ = xt.DeleteObject(o) // best-effort: see above; the key goes regardless
	d.Delete(key)
}

// ErrActiveContentRemains is StripActive's refusal: its own output still scans as carrying active content,
// so the claim "all active content removed" would be false. Nothing is written by a caller that gets it.
var ErrActiveContentRemains = errors.New("active content remains after stripping")

// strippedExempt is what StripActive deliberately leaves: identifying metadata, which is StripMetadata's
// remit and not active. **An exempt set, not a remit list**: a Scan kind added later that StripActive does
// not yet remove then refuses rather than passing unverified.
var strippedExempt = map[string]bool{"metadata": true, "info": true}

// verifyStripped re-scans StripActive's output with Scan — which reads through `api.ReadContext`, not the
// rewrite's `ReadOptimized`, so it is a second reader rather than the writer checking itself — and refuses
// any finding outside strippedExempt. A scan that fails is a refusal too: an unverified claim is not made.
func verifyStripped(out []byte) error {
	rep, err := Scan(out)
	if err != nil {
		return fmt.Errorf("%w: the result could not be re-scanned: %v", ErrActiveContentRemains, err)
	}
	return activeResidue(rep)
}

// activeResidue is verifyStripped's judgment over a report, apart from the scan that produced it.
func activeResidue(rep ScanReport) error {
	var left []string
	for _, f := range rep.Findings {
		if !strippedExempt[f.Kind] {
			left = append(left, f.Kind+" ("+f.Detail+")")
		}
	}
	if len(left) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrActiveContentRemains, strings.Join(left, "; "))
}

// writeMutated reads pdf into a validated, optimized context (so WriteContext
// has the consolidated structures it expects and the attachment/annotation
// caches are populated), applies fn, and writes the result back. The optimize
// pass is skipped where it would be unbounded (`pdfread.ReadOptimized`, `/pending 706`). It is the
// shared read→mutate→write shape for the surgical removals.
func writeMutated(pdf []byte, fn func(*model.Context) error) ([]byte, error) {
	return rewriteContext(pdf, model.NewDefaultConfiguration(), fn)
}

// rewriteWithConf is the shape of a pdfcpu `api` wrapper — read with ITS configuration (the `Cmd` it
// sets, the relaxed validation some set), apply one context operation, write — with nib's rules in the
// middle, which a wrapper gives no place for (`/pending 492`). `fault.Catch` because every wrapper it
// replaces has one: pdfcpu reports some failures by panicking.
func rewriteWithConf(pdf []byte, conf *model.Configuration, fn func(*model.Context) error) (out []byte, err error) {
	defer fault.Catch(&err)
	return rewriteContext(pdf, conf, fn)
}

// rewriteContext is the one read-change-write both doors share.
func rewriteContext(pdf []byte, conf *model.Configuration, fn func(*model.Context) error) ([]byte, error) {
	ctx, err := pdfread.ReadOptimized(pdf, conf)
	if err != nil {
		return nil, err
	}
	// Every change drops a PDF/UA identification nib did not verify (`/pending 492`), and it drops it
	// HERE, inside the rewrite the operation is already paying for — never as a second write that could
	// land after a signature. Before `fn`, so a door that verified its output can write one back.
	if _, err := dropUAIdentification(ctx); err != nil {
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

// errPageTreeTooLarge is eachPage's refusal of a tree whose walk would visit more nodes than the file could
// honestly hold.
var errPageTreeTooLarge = errors.New("pdfops: the page tree is deeper, or names its nodes more times, than a document of this size can hold, so it was not walked")

// eachPage invokes fn for each leaf page dict in document order (1-based),
// walking the page tree directly so it needs no validated PageCount. A depth
// cap and a per-walk visited set guard against malformed or cyclic trees.
//
// **And a visit budget bounds sharing** (the phase-close review of PLAN-returned-document P01). A node named twice
// is walked twice — see below — so a chain of `/Pages` nodes each listing the next twice is 2^depth leaves from a
// couple of kilobytes, and `Scan` reads without `pdfread`'s path budget. Past 16 visits per object in the file (plus
// a floor) the walk stops and says so: a truncated walk would be a security scan that silently skipped pages.
func eachPage(xt *model.XRefTable, root types.Dict, fn func(page types.Dict, nr int)) error {
	pages := derefDict(xt, root["Pages"])
	if pages == nil {
		return nil
	}
	nr := 0
	budget := pageWalkBudget(xt)
	truncated := false
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
		if node == nil || budget < 0 {
			return
		}
		if depth > 50 {
			truncated = true // a page tree 50 deep is not a document; skipping it silently is the failure above
			return
		}
		budget--
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
	if budget < 0 || truncated {
		return errPageTreeTooLarge
	}
	return nil
}

// actionAllowance is what every `eachAction` call adds to the shared budget before it walks: one chain of direct
// actions to the depth cap, so each walk pays for what it alone reaches and only re-walked shared actions spend it.
const actionAllowance = 33

// errActionWalkTooLarge is the refusal of a document whose annotations' action chains would take more visits than the
// file could hold (`eachAction`'s shared budget).
var errActionWalkTooLarge = errors.New("pdfops: the document's annotations name their actions more times than a document of this size can hold, so they were not walked")

// eachPageAnnot calls fn with each annotation of each page, in page order, ONCE: the one door every page-annotation
// walk goes through (ADR-009). A page object the tree names several times, an `/Annots` array several pages name by
// reference, and an annotation several slots name by reference are each walked at their first appearance only, so
// the walk is linear in what the file holds. Without it, pages × slots was the cost: 1,000 pages sharing one
// 1,000-entry `/Annots` array is a million visits from 15 KB (the phase-close review of PLAN-returned-document P02),
// and every reader below it — action chains, attachment and widget lists — multiplied it again.
func eachPageAnnot(xt *model.XRefTable, root types.Dict, fn func(annot types.Dict, nr int)) error {
	pages, arrays, annots := map[uintptr]bool{}, map[int]bool{}, map[int]bool{}
	return eachPage(xt, root, func(page types.Dict, nr int) {
		// The dereferenced dict IS the xref table's object, so a page named twice is one map.
		p := reflect.ValueOf(page).Pointer()
		if pages[p] {
			return
		}
		pages[p] = true
		if !firstVisit(arrays, page["Annots"]) {
			return
		}
		for _, a := range derefArray(xt, page["Annots"]) {
			if !firstVisit(annots, a) {
				continue
			}
			if annot := derefDict(xt, a); annot != nil {
				fn(annot, nr)
			}
		}
	})
}

// firstVisit reports whether o is a direct object or an indirect one not yet in seen, marking it. An ARRAY named by
// reference needs it as much as a dict does (the re-review of the breadth-first walks): `7 0 obj [<</Next 7 0 R>>
// <</Next 7 0 R>>]` re-expanded the array from each direct dict inside it, 2^32 visits from 645 bytes, and a queue
// holds that fan-out in memory where the old recursion only spun.
func firstVisit(seen map[int]bool, o types.Object) bool {
	ir, ok := o.(types.IndirectRef)
	if !ok {
		return true
	}
	n := ir.ObjectNumber.Value()
	if seen[n] {
		return false
	}
	seen[n] = true
	return true
}

// pageWalkBudget is eachPage's visit budget: 16 per LIVE object plus a floor. A compressed xref stream can declare a
// million free rows in a few kilobytes, and counting them handed a 40-level doubling chain ~16M visits (measured 4.9 s
// past pdfcpu's own read) before refusing.
func pageWalkBudget(xt *model.XRefTable) int {
	live := 0
	for _, e := range xt.Table {
		if e != nil && !e.Free {
			live++
		}
	}
	return 16*live + 1024
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
