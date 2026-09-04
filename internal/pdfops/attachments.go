package pdfops

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"hash"
	"sort"
)

// AttachmentInfo names one embedded file. Two carriers are listed: the document's
// Names→EmbeddedFiles tree (the usual one, via pdfcpu's ListAttachments) and
// page-level /FileAttachment annotations (rarer, but Scan flags them so the list
// must too) — for the latter Desc records which page it hangs off.
type AttachmentInfo struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
	// Ceremony marks the one embedded file that is a signing ceremony's record (P06.S09, D29).
	//
	// **The test is the NAME, and it is made here rather than in the client.** `CeremonyRecordName`
	// is this package's constant and the client would otherwise carry a second copy of it in
	// another language, drifting the first time it changes — the shape ADR-009 refuses. The panel
	// renders a label off this flag and never matches on the string.
	//
	// It is a LABEL and not a permission: what stops the record being removed is `ceremonyFreeze`,
	// which refuses every mutating route on a document carrying one, so `POST /api/sanitize` — the
	// only path in the product that removes an attachment — never runs. This field exists so the
	// user can see what the file is before they wonder why.
	Ceremony bool `json:"ceremony,omitempty"`
}

// Attachments lists the document's embedded files: the catalog name tree plus any
// page-level FileAttachment annotations. pdfcpu's ListAttachments returns stubs
// (name + description, no data), so this is cheap. An empty result (neither
// carrier present) is not an error.
func Attachments(pdf []byte) ([]AttachmentInfo, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	aa, err := ctx.ListAttachments()
	if err != nil {
		return nil, err
	}
	out := make([]AttachmentInfo, 0, len(aa))
	for _, a := range aa {
		name := a.FileName
		if name == "" {
			name = a.ID // the add path keys the name tree on ID; FileName mirrors it
		}
		// Through attachmentName, like every other name path in this file. This one
		// was raw, and it is the one the user sees and acts on: server/attachments.go
		// hands the listed name to sendDownload, which puts it in a
		// Content-Disposition filename — and the name comes from an untrusted PDF.
		// attachmentName exists precisely because "any downstream disk write must
		// never see a dot-path"; the guard was simply not applied here.
		if clean := attachmentName(name); clean != "" {
			name = clean
		}
		out = append(out, AttachmentInfo{Name: name, Desc: a.Desc, Ceremony: name == CeremonyRecordName})
	}
	// The same RVO context that populated the name tree exposes the page tree, so
	// no second read is needed (the eachPage/derefDict helpers Scan uses work on it).
	root, err := ctx.Catalog()
	if err != nil {
		return nil, err
	}
	for _, pa := range pageFileAttachments(ctx.XRefTable, root) {
		out = append(out, AttachmentInfo{Name: pageAttachmentName(pa), Desc: fmt.Sprintf("Attached to page %d", pa.page)})
	}
	return out, nil
}

// pageFileAttachment is one page-level /FileAttachment annotation: the embedded
// file's name (from the filespec, basename only), the page it hangs off, and the
// filespec dict to read its bytes from.
type pageFileAttachment struct {
	name string
	page int
	fs   types.Dict
}

// pageAttachmentName is the stable list/extract key for a page attachment: the
// filespec name, or a page-derived fallback when the filespec carries no name.
func pageAttachmentName(pa pageFileAttachment) string {
	if pa.name != "" {
		return pa.name
	}
	return fmt.Sprintf("page-%d-attachment", pa.page)
}

// pageFileAttachments walks page /Annots for Subtype /FileAttachment, mirroring
// Scan's detection (scan.go), and returns each in page order. This is the carrier
// the catalog name tree — and thus ListAttachments — does not cover.
func pageFileAttachments(xt *model.XRefTable, root types.Dict) []pageFileAttachment {
	var out []pageFileAttachment
	eachPage(xt, root, func(page types.Dict, nr int) {
		for _, a := range derefArray(xt, page["Annots"]) {
			annot := derefDict(xt, a)
			if annot == nil || nameVal(annot, "Subtype") != "FileAttachment" {
				continue
			}
			fs := derefDict(xt, annot["FS"])
			if fs == nil {
				continue
			}
			out = append(out, pageFileAttachment{name: fileSpecName(xt, fs), page: nr, fs: fs})
		}
	})
	return out
}

// fileSpecName reads a filespec's basename (preferring /UF over /F), stripped to a
// clean basename like attachmentName. Empty if the filespec names no file.
func fileSpecName(xt *model.XRefTable, fs types.Dict) string {
	for _, key := range []string{"UF", "F"} {
		o, found := fs.Find(key)
		if !found {
			continue
		}
		if s, err := xt.DereferenceStringOrHexLiteral(o, model.V10, nil); err == nil {
			if s = attachmentName(s); s != "" {
				return s
			}
		}
	}
	return ""
}

// fileSpecBytes returns the decoded embedded-file bytes from a filespec's EF→F
// (or →UF) stream. It mirrors pdfcpu's own (unexported) decodeFileSpecStreamDict:
// an unfiltered stream's content is its raw bytes.
func fileSpecBytes(xt *model.XRefTable, fs types.Dict) ([]byte, error) {
	ef := derefDict(xt, fs["EF"])
	if ef == nil {
		return nil, fmt.Errorf("attachment has no embedded file")
	}
	o, found := ef.Find("F")
	if !found {
		o, found = ef.Find("UF")
	}
	if !found || o == nil {
		return nil, fmt.Errorf("attachment has no embedded file stream")
	}
	sd, _, err := xt.DereferenceStreamDict(o)
	if err != nil {
		return nil, err
	}
	if sd == nil {
		return nil, fmt.Errorf("attachment stream is empty")
	}
	if sd.FilterPipeline == nil {
		return sd.Raw, nil
	}
	if err := sd.Decode(); err != nil {
		return nil, err
	}
	return sd.Content, nil
}

// AddAttachment embeds data as a new attachment named name (a basename — any
// path is stripped). A name that already exists is rejected rather than letting
// pdfcpu silently store a mangled duplicate key. It mutates the document.
func AddAttachment(pdf []byte, name string, data []byte) ([]byte, error) {
	name = attachmentName(name)
	if name == "" {
		return nil, fmt.Errorf("attachment needs a file name")
	}
	return writeMutated(pdf, func(ctx *model.Context) error {
		existing, err := ctx.ListAttachments()
		if err != nil {
			return err
		}
		for _, a := range existing {
			if a.FileName == name || a.ID == name {
				return fmt.Errorf("an attachment named %q already exists", name)
			}
		}
		return ctx.AddAttachment(model.Attachment{Reader: bytes.NewReader(data), ID: name}, false)
	})
}

// ContentDigest is a SHA-256 over the page count and every page's content stream, in
// order — a projection of the document that survives operations which change its bytes.
//
// **It exists because a byte hash cannot be recomputed by anyone but the writer.** Measured
// 2026-08-19: pdfcpu's rewrite is not idempotent — normalising the same document twice
// produces two different files — so "hash the document with the attachment removed" gives
// the convener one number and every later party a different one. Attaching and then
// detaching is not an identity either.
//
// This is stable where that is not. Measured: identical across adding an attachment, and
// across a rewrite of the same document.
//
// # What it covers, and the exclusion list that outlived its own truth
//
// **Covered:** page count; per page the content stream, MediaBox/CropBox/Rotate, the page
// resources followed into font and XObject streams, and /Annots in full; and the catalog's
// embedded-files name tree, minus the ceremony record (see CeremonyRecordName).
//
// **Not covered:** document metadata, and the AcroForm structure outside page /Annots.
//
// This comment used to carry an exclusion list reading "annotations, form field values,
// attachments, and metadata", justified by "tamper-evidence for everything else is what the
// signatures are for — they cover the actual bytes, and any edit to them flips the
// verification verdict". **Three of those four are now covered, and the justification was
// refuted twice by measurement** (v1.116.18 for annotations and form values; P07.S02 for
// attachments). The argument fails for one reason both times: the window this digest is
// checked in is the PRE-SIGNATURE window — that is precisely when a structural rewrite is
// legal — so there is no signature to be the fallback it names. It also contradicted the
// body of its own function, which had folded /Annots in with a comment saying so.
//
// It is recorded rather than deleted because the same argument will be offered for the next
// axis, and it is wrong the same way. Also worth stating: "any edit flips the verdict" is
// itself false of an INCREMENTAL update, which is how every co-signature is applied.
//
// # It covers the page RESOURCES too, and until v1.116.18 it did not
//
// The content stream is only half of what is on a page. For a scanned document — Nib's
// central case, with 41 OCR languages behind it — the stream is invariant boilerplate
// (`q W 0 0 H 0 0 cm /Im0 Do Q`) and the entire visible page is the image XObject, which
// lives in `/Resources`. So the digest was blind to exactly the documents it matters most
// for: a party receiving a prepared contract before the first signature could swap the
// image bytes — clause text, amounts, the signature block — and `CheckDocument` returned
// clean, because the record was untouched and the stream was unchanged.
//
// Fonts are folded in for the same reason one step removed: the stream says which glyphs to
// draw and the font decides what they look like.
//
// **Measured, because the whole reason this function exists is that a byte hash is not
// stable here.** On a rewritten document (`Optimize` applied twice, whose raw bytes are NOT
// identical — the non-idempotence that rules out a byte hash):
//
//	resource digest, rewrite 1 == rewrite 2    stable
//	content digest,  image bytes swapped       UNCHANGED  ← the defect
//	resource digest, image bytes swapped       changes    ← the fix
//
// Decoded content is hashed where the filter decodes, and the raw stream where it does not,
// with a marker distinguishing the two — otherwise a document whose filter this build cannot
// decode would hash identically to one where the decode produced nothing.
// ContentDigestVersion identifies WHAT this build hashes, and it is bound into the digest.
//
// **Without it, improving the coverage accuses a counterparty of tampering.** v1.116.18 changed
// what ContentDigest covers without moving anything: a record written by the previous build
// passed the version gate and then failed the hash comparison with *"the document does not
// match the ceremony record… these are not the same document"*, when the cause was a Nib point
// release. Every one of the coverage gaps found since needs another change, so this has to be
// a version, not a constant that happens to be right today.
//
// Bump it whenever the set of hashed axes changes. `Record.FormatVersion` is a different
// number answering a different question (what the roster preimage binds); a digest change does
// not need to move that, but it does need to move this.
//
// **Bumped to 3 (2026-08-24, P07.S02).** The embedded-files name tree is now covered — see
// CeremonyRecordName and the attachment block in ContentDigest for what was measured.
//
// **And the constant was doing only half its job until this slice.** It was bound INTO the
// digest and carried nowhere beside it — three occurrences in the whole tree, all in this
// file — so nothing could ever compare two versions. Binding a version inside a hash changes
// the number; it cannot produce a sentence, because the reader has nothing to read. A build
// with version 3 meeting a record written under 2 therefore produced the exact accusation the
// paragraph above says this constant prevents. `Record` now carries the digest version it was
// written under, so the mismatch is reported as a skew (D32) rather than as tampering.
const ContentDigestVersion = 3

// CeremonyRecordName is the one embedded file ContentDigest must NOT hash.
//
// It lives here rather than in `internal/ceremony` because the exclusion is a property of the
// digest, and `internal/pdfops` cannot import `internal/ceremony` (that package imports this
// one). `ceremony.AttachmentName` is defined as this constant, so there is one name and not
// two that can drift — ADR-009.
//
// **Why it is excluded, and it is not a preference:** the record contains `DocHash`, which is
// this digest of the document the record is embedded in. A digest that covered the record
// would be a fixed point — the value would have to be known before it could be computed.
// Measured stable both ways at the P07.S02 grill: embedding the record leaves the digest
// byte-identical, before and after this slice widened the coverage.
const CeremonyRecordName = "nib-ceremony.json"

func ContentDigest(pdf []byte) (string, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return "", err
	}
	h := sha256.New()
	// Every field is length-prefixed through these two helpers — see hashChunk. The first
	// draft wrote the resource kind and name unprefixed, which is injective by luck rather
	// than by construction (C3).
	hashChunk(h, []byte("nib-content-digest"))
	hashUint(h, ContentDigestVersion)
	hashUint(h, uint64(ctx.PageCount))
	for i := 1; i <= ctx.PageCount; i++ {
		d, _, _, err := ctx.PageDict(i, false)
		if err != nil || d == nil {
			return "", fmt.Errorf("page %d is unreadable: %w", i, err)
		}
		c, err := ctx.PageContent(d, i)
		if err != nil && err != model.ErrNoContent {
			return "", fmt.Errorf("page %d content: %w", i, err)
		}
		hashChunk(h, c)
		// GEOMETRY, which the first draft did not cover at all. Shrinking /CropBox to
		// excise a paragraph, or setting /Rotate 90, changes what every reader displays.
		// It is per-page, and it is exactly what a general reviewer reads past.
		for _, key := range []string{"MediaBox", "CropBox", "Rotate"} {
			hashChunk(h, []byte(key))
			hashObject(ctx.XRefTable, d[key], h, 0)
		}
		hashPageResources(ctx.XRefTable, d, h)
		// ANNOTATIONS, because the exclusion's premise is false in the window this digest
		// is checked in. The argument was "everything else is covered by the signatures" —
		// but CheckDocument exists for the pre-FIRST-signature hop, where there are none,
		// and in that window Nib's OWN operations defeat a content-only digest: AddNotes
		// writes /Text annotations (visible sticky notes) and the form fill writes /V plus
		// widget /AP streams. Neither touches a content stream. For a contract, the form
		// values ARE the agreement.
		hashChunk(h, []byte("Annots"))
		hashObject(ctx.XRefTable, d["Annots"], h, 0)
	}
	// EMBEDDED FILES, v3 — and the exclusion above it was refuted the same way the
	// annotations exclusion was, one paragraph up: by asking what the argument actually
	// covers in the window this digest is checked in.
	//
	// The old sentence was "attachments are not covered; tamper-evidence for everything else
	// is what the signatures are for". **Measured at the P07.S02 grill:** an attached
	// `Schedule-A.txt` reading "rent is 1000/mo" was removed and re-added under the SAME
	// filename reading "rent is 100000/mo"; the digest did not move and `CheckDocument`
	// returned nil. The document is unsigned in that window — which is precisely when
	// `Embed` permits a structural rewrite — so there was no signature to be the fallback the
	// argument named. For a lease, the schedule IS the agreement, exactly as the form values
	// are.
	//
	// Sorted by name so the digest is a property of the document rather than of pdfcpu's
	// enumeration order; the name and the bytes are hashed as separate length-prefixed
	// chunks, so a rename and an edit cannot be made to cancel out.
	if err := hashEmbeddedFiles(ctx, h); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashEmbeddedFiles folds the catalog name tree into the digest, minus the ceremony record.
//
// Page-level /FileAttachment annotations are deliberately NOT walked here: they hang off
// `/Annots`, which the per-page loop above already hashes. Hashing them twice would be
// harmless but would state the coverage in two places.
func hashEmbeddedFiles(ctx *model.Context, h hash.Hash) error {
	aa, err := ctx.ListAttachments()
	if err != nil {
		// A document with no name tree is the ordinary case and is not an error; pdfcpu
		// reports it as one on some inputs. Hash the count as zero and carry on, so an
		// unreadable tree cannot silently look like an empty one.
		hashChunk(h, []byte("embedded-files"))
		hashUint(h, 0)
		return nil
	}
	names := make([]string, 0, len(aa))
	for _, a := range aa {
		name := a.FileName
		if name == "" {
			name = a.ID
		}
		if clean := attachmentName(name); clean != "" {
			name = clean
		}
		if name == CeremonyRecordName {
			continue // the self-reference; see CeremonyRecordName
		}
		names = append(names, name)
	}
	sort.Strings(names)
	hashChunk(h, []byte("embedded-files"))
	hashUint(h, uint64(len(names)))
	for _, name := range names {
		hashChunk(h, []byte(name))
		a, err := ctx.ExtractAttachment(model.Attachment{ID: name})
		if err != nil || a == nil || a.Reader == nil {
			// Named in the tree but unreadable. Hash a marker rather than skipping: a file
			// that cannot be read is a different document from one that is not there, and
			// silently skipping would let an attacker hide an edit behind a broken filespec.
			hashChunk(h, []byte("unreadable"))
			continue
		}
		b, err := io.ReadAll(a.Reader)
		if err != nil {
			return fmt.Errorf("attachment %q: %w", name, err)
		}
		hashChunk(h, b)
	}
	return nil
}

// hashChunk writes a length-prefixed byte string, and hashUint a length-prefixed integer.
//
// Length prefixes everywhere, so the framing is injective by construction rather than by the
// happy accident that two field values never slide across their boundary. Same discipline as
// internal/ceremony's preimageBuilder, which exists for the same reason.
func hashChunk(h hash.Hash, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	h.Write(n[:])
	h.Write(b)
}

func hashUint(h hash.Hash, v uint64) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], v)
	hashChunk(h, n[:])
}

// hashObject writes a CANONICAL encoding of o into h.
//
// Dict keys are sorted, because types.Dict is a map and pdfcpu's own PDFString() would emit
// them in Go's randomised iteration order — nondeterminism reaching a digest (C11), which is
// the one defect no single-process test can see.
//
// Indirect references are followed, bounded by depth and by an on-path object set. Object
// NUMBERS are never hashed: they change on every pdfcpu rewrite, which is the whole reason
// this function exists instead of a byte hash.
func hashObject(xt *model.XRefTable, o types.Object, h hash.Hash, depth int) {
	if depth > 16 {
		hashChunk(h, []byte("#depth"))
		return
	}
	if ir, ok := o.(types.IndirectRef); ok {
		d, err := xt.Dereference(ir)
		if err != nil {
			hashChunk(h, []byte("#unresolved"))
			return
		}
		o = d
	}
	switch v := o.(type) {
	case nil:
		hashChunk(h, []byte("#nil"))
	case types.Dict:
		hashChunk(h, []byte("#dict"))
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		hashUint(h, uint64(len(keys)))
		for _, k := range keys {
			hashChunk(h, []byte(k))
			hashObject(xt, v[k], h, depth+1)
		}
	case types.StreamDict:
		hashChunk(h, []byte("#stream"))
		hashObject(xt, v.Dict, h, depth+1)
		hashStreamBody(&v, h)
	case types.Array:
		hashChunk(h, []byte("#array"))
		hashUint(h, uint64(len(v)))
		for _, e := range v {
			hashObject(xt, e, h, depth+1)
		}
	default:
		// Names, strings, numbers, booleans. PDFString is canonical for these — the
		// ordering hazard is dicts, and those are handled above.
		hashChunk(h, []byte(o.PDFString()))
	}
}

// hashStreamBody writes a stream's bytes, decoded where the filter decodes.
//
// The marker distinguishes the two: without it a document whose filter this build cannot
// decode hashes identically to one where the decode produced nothing.
func hashStreamBody(sd *types.StreamDict, h hash.Hash) {
	body := sd.Raw
	marker := byte(0)
	if derr := sd.Decode(); derr == nil {
		body = sd.Content
		marker = 1
	}
	hashChunk(h, []byte{marker})
	hashChunk(h, body)
}

// resourceKinds are the page-visible resource categories folded into ContentDigest.
//
// XObject holds images and form XObjects — for a scan, the whole visible page. Font decides
// what the glyphs the content stream names actually look like. The others (ColorSpace,
// Pattern, Shading, ExtGState) are deliberately out: they are parameters rather than content,
// and every one of them that changes what is drawn does so through a stream these two already
// cover.
var resourceKinds = []string{"Font", "XObject"}

// hashPageResources folds a page's resources into h, in a canonical order.
//
// # The DICT, not only the stream body
//
// The first draft hashed `sd.Raw`/`sd.Content` and nothing from `sd.Dict`, which left the
// reader's *interpretation* of those bytes unhashed: `/Decode [1 0 1 0 1 0]` inverts the whole
// page image, `/ColorSpace` re-pointed at an all-white indexed palette blanks it, `/Matrix` on
// a form scales a signature block to nothing — all with an identical digest. That is the same
// attack the coverage was added to close, one dictionary key over.
//
// # And "Font" was inert
//
// `DereferenceStreamDict` type-asserts `types.StreamDict` and errors otherwise (verified in
// pdfcpu v0.13.0 model/xreftable.go:989). A page `/Font` entry is a font DICTIONARY; the font
// program is a stream several levels below, under `/FontDescriptor /FontFile2`. So the old
// loop `continue`d on every font on every page while the doc claimed fonts were folded in —
// leaving `/BaseFont`, `/Encoding`, `/Differences` and the embedded program all free to change
// every glyph on the page. `hashObject` follows the dict and reaches the program, and it
// recurses into a form XObject's own `/Resources` for the same reason.
//
// Order comes from the resource NAMES, not from object numbers — object numbers change on
// every pdfcpu rewrite and names do not, which is the same reason this hashes the stream
// rather than the file.
func hashPageResources(xt *model.XRefTable, page types.Dict, h hash.Hash) {
	res, _ := xt.DereferenceDict(page["Resources"])
	if res == nil {
		hashChunk(h, []byte("#nores"))
		return
	}
	for _, kind := range resourceKinds {
		hashChunk(h, []byte(kind))
		sub, _ := xt.DereferenceDict(res[kind])
		if sub == nil {
			hashChunk(h, []byte("#none"))
			continue
		}
		names := make([]string, 0, len(sub))
		for k := range sub {
			names = append(names, k)
		}
		sort.Strings(names)
		hashUint(h, uint64(len(names)))
		for _, name := range names {
			hashChunk(h, []byte(name))
			hashObject(xt, sub[name], h, 0)
		}
	}
}

// RemoveAttachment returns the document without the named attachment, leaving it
// unchanged when there is nothing by that name.
//
// It exists so a value can be computed over "the document apart from this attachment" —
// the Ceremony Record's docHash, which cannot include the record that contains it. A
// caller hashing the whole file would produce a number no later party could reproduce,
// because by then the record is inside.
//
// Absent is not an error: the callers that want this are asking "what does this document
// look like without X", and a document that never had X already looks like that.
func RemoveAttachment(pdf []byte, name string) ([]byte, error) {
	name = attachmentName(name)
	if name == "" {
		return nil, fmt.Errorf("attachment needs a file name")
	}
	return writeMutated(pdf, func(ctx *model.Context) error {
		existing, err := ctx.ListAttachments()
		if err != nil {
			return err
		}
		for _, a := range existing {
			if a.FileName == name || a.ID == name {
				_, err := ctx.RemoveAttachments([]string{name})
				return err
			}
		}
		return nil
	})
}

// ExtractAttachment returns the decoded bytes of the embedded file named name.
// It looks in the catalog name tree first, then falls back to page-level
// FileAttachment annotations (keyed by the same name pageFileAttachments lists).
func ExtractAttachment(pdf []byte, name string) ([]byte, error) {
	name = attachmentName(name)
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	if a, err := ctx.ExtractAttachment(model.Attachment{ID: name}); err == nil && a != nil && a.Reader != nil {
		return io.ReadAll(a.Reader)
	}
	// Not in the name tree — try page-level FileAttachment annotations.
	root, err := ctx.Catalog()
	if err != nil {
		return nil, err
	}
	for _, pa := range pageFileAttachments(ctx.XRefTable, root) {
		if pageAttachmentName(pa) == name {
			return fileSpecBytes(ctx.XRefTable, pa.fs)
		}
	}
	return nil, fmt.Errorf("no attachment named %q", name)
}

// attachmentName reduces a user-supplied name to a clean basename: any directory
// path is dropped (the name keys the embedded-files tree and lands in the
// filespec, so it must not carry separators).
func attachmentName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	// A bare "." or ".." would survive the separator strip; harmless as a PDF
	// name-tree key, but any downstream disk write must never see a dot-path.
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// SignatureWidget is one visible signature's appearance on a page.
type SignatureWidget struct {
	Page  int        // 1-based
	Rect  [4]float64 // llx, lly, urx, ury in PDF points
	HasAP bool       // an appearance stream is attached, so there is something to draw
}

// SignatureWidgets reports every signature widget annotation and where it sits.
//
// **It answers "was this block DRAWN, and where" without a rasteriser** — which is the positive
// control D25's placement clause asks for, in the only form this repo can produce. The clause is
// right that "a raster cannot distinguish 'off the page' from 'never drawn'", and the corollary is
// that a check on placement arithmetic alone cannot distinguish "placed correctly" from "not
// placed at all": both leave a valid document, and `sign.Verify` reports an INVISIBLE signature
// exactly as it reports a visible one.
//
// `HasAP` is the half that separates a widget from a drawing. An annotation with no /AP has a
// rectangle and no appearance stream, so a reader renders nothing there — the block would be
// "placed" by every geometric measure and blank on the page.
//
// This is not a rendered measurement and does not claim to be: it reads the file's structure, so
// it cannot see a block that is drawn in white on white, or one an /AP stream positions outside
// its own BBox. The rendered half needs pdf.js and belongs at tier 3.
func SignatureWidgets(pdf []byte) ([]SignatureWidget, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	root, err := ctx.Catalog()
	if err != nil {
		return nil, err
	}
	var out []SignatureWidget
	eachPage(ctx.XRefTable, root, func(page types.Dict, nr int) {
		for _, a := range derefArray(ctx.XRefTable, page["Annots"]) {
			annot := derefDict(ctx.XRefTable, a)
			if annot == nil || nameVal(annot, "Subtype") != "Widget" {
				continue
			}
			// /FT may live on the widget or be inherited from its AcroForm field parent; a
			// signature widget that carries neither is not one.
			ft := nameVal(annot, "FT")
			if ft == "" {
				if parent := derefDict(ctx.XRefTable, annot["Parent"]); parent != nil {
					ft = nameVal(parent, "FT")
				}
			}
			if ft != "Sig" {
				continue
			}
			r := derefArray(ctx.XRefTable, annot["Rect"])
			if len(r) != 4 {
				continue
			}
			var rect [4]float64
			ok := true
			for i, v := range r {
				f, isF := numeric(ctx.XRefTable, v)
				if !isF {
					ok = false
					break
				}
				rect[i] = f
			}
			if !ok {
				continue
			}
			out = append(out, SignatureWidget{
				Page:  nr,
				Rect:  rect,
				HasAP: derefDict(ctx.XRefTable, annot["AP"]) != nil,
			})
		}
	})
	return out, nil
}

// numeric dereferences an object to a float, accepting both PDF numeric types.
func numeric(xt *model.XRefTable, o types.Object) (float64, bool) {
	d, err := xt.Dereference(o)
	if err != nil {
		return 0, false
	}
	switch v := d.(type) {
	case types.Integer:
		return float64(v.Value()), true
	case types.Float:
		return v.Value(), true
	}
	return 0, false
}
