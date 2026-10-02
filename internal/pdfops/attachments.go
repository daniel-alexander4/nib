package pdfops

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"nib/internal/pdfread"
	"strings"

	"hash"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// AttachmentInfo names one embedded file. Two carriers are listed: the document's
// Names→EmbeddedFiles tree (the usual one) and page-level /FileAttachment annotations (rarer, but
// Scan flags them so the list must too) — for the latter Desc records which page it hangs off.
type AttachmentInfo struct {
	// ID is WHERE the file is, and it is what every extraction is addressed by (/pending 745).
	//
	// **Not the name, because a name does not pick one entry.** The name a reader shows is the
	// filespec's /UF (or /F), which the document's author writes: two entries may share one, and an
	// entry keyed `a.txt` may call itself `b.txt`. Addressed by name, a download returned the first
	// match's bytes under the name the user clicked, and a page reorder rewrote one file's bytes
	// under another's name. For a name-tree entry the ID is its tree KEY, unique in any tree pdfcpu
	// will write; for a page-level annotation it is `page:<page>:<n>`, the n-th FileAttachment on
	// that page. An ID more than one entry answers to is refused, never resolved to the first.
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
	// Ceremony marks the one embedded file that is a signing ceremony's record (P06.S09, D29).
	//
	// **The test is `ceremonyRecordEntry`, and it is made here rather than in the client.** It
	// used to be the display NAME, so any entry calling itself `nib-ceremony.json` was labelled
	// the record; now it is the entry ContentDigest leaves out and `CeremonyRecord` reads (/pending
	// 745), so the label, the digest and the record agree on which entry that is. The client would
	// otherwise carry a second copy of the rule in another language — the shape ADR-009 refuses.
	// The panel renders a label off this flag and never matches on the string.
	//
	// It is a LABEL and not a permission: what stops the record being removed is `ceremonyFreeze`,
	// which refuses every mutating route on a document carrying one, so `POST /api/sanitize` — the
	// only path in the product that removes an attachment — never runs. This field exists so the
	// user can see what the file is before they wonder why.
	Ceremony bool `json:"ceremony,omitempty"`
}

// embeddedFile is one embedded file as this package reads it: what it is called and where it is
// (info), and the filespec its bytes come from.
type embeddedFile struct {
	info AttachmentInfo
	fs   types.Dict
}

// embeddedFiles lists both carriers, each file with its own filespec — the one door every reader
// of an embedded file goes through (/pending 745, ADR-009; see treeEntries). A tree entry whose
// filespec is not a dictionary is still listed, with a nil fs: it is in the document, and reading
// it fails by name rather than the whole listing failing (pdfcpu's `ListAttachments` did).
func embeddedFiles(ctx *model.Context) ([]embeddedFile, error) {
	xt := ctx.XRefTable
	entries, err := treeEntries(xt)
	if err != nil {
		return nil, err
	}
	record, _ := ceremonyRecordEntry(xt, entries)
	out := make([]embeddedFile, 0, len(entries))
	for i := range entries {
		e := entries[i]
		fs, _ := xt.DereferenceDict(e.fs)
		// The name a reader shows (/UF, then /F), cleaned like every other name path in this
		// file: server/attachments.go puts it in a Content-Disposition filename, and it comes
		// from an untrusted PDF — "any downstream disk write must never see a dot-path".
		name := ""
		if fs != nil {
			name = fileSpecName(xt, fs)
		}
		if name == "" {
			name = attachmentName(e.key)
		}
		if name == "" {
			name = e.key
		}
		desc := ""
		if fs != nil {
			if o, ok := fs.Find("Desc"); ok {
				desc, _ = xt.DereferenceStringOrHexLiteral(o, model.V10, nil)
			}
		}
		out = append(out, embeddedFile{
			info: AttachmentInfo{ID: e.key, Name: name, Desc: desc, Ceremony: i == record},
			fs:   fs,
		})
	}
	root, err := ctx.Catalog()
	if err != nil {
		return nil, err
	}
	nth := map[int]int{}
	pas, err := pageFileAttachments(xt, root)
	if err != nil {
		return nil, err
	}
	for _, pa := range pas {
		n := nth[pa.page]
		nth[pa.page]++
		out = append(out, embeddedFile{
			info: AttachmentInfo{
				ID:   fmt.Sprintf("page:%d:%d", pa.page, n),
				Name: pageAttachmentName(pa),
				Desc: fmt.Sprintf("Attached to page %d", pa.page),
			},
			fs: pa.fs,
		})
	}
	return out, nil
}

// Attachments lists the document's embedded files: the catalog name tree plus any page-level
// FileAttachment annotations. It reads no file's bytes, so it is cheap. An empty result (neither
// carrier present) is not an error.
func Attachments(pdf []byte) ([]AttachmentInfo, error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return nil, err
	}
	files, err := embeddedFiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AttachmentInfo, 0, len(files))
	for _, f := range files {
		out = append(out, f.info)
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
func pageFileAttachments(xt *model.XRefTable, root types.Dict) ([]pageFileAttachment, error) {
	var out []pageFileAttachment
	if err := eachPageAnnot(xt, root, func(annot types.Dict, nr int) {
		if nameVal(annot, "Subtype") != "FileAttachment" {
			return
		}
		fs := derefDict(xt, annot["FS"])
		if fs == nil {
			return
		}
		out = append(out, pageFileAttachment{name: fileSpecName(xt, fs), page: nr, fs: fs})
	}); err != nil {
		return nil, err
	}
	return out, nil
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
	return embeddedStreamBytes(xt, o)
}

// embeddedStreamBytes is the decoded content of one /EF value — the stream `fileSpecBytes` picks,
// or, for `CarryAttachments`, each of /F and /UF in turn (/pending 750).
func embeddedStreamBytes(xt *model.XRefTable, o types.Object) ([]byte, error) {
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
//
// **Bumped to 4 (2026-09-29, /pending 725).** The axes are the same; what one of them MEASURES
// changed, and that moves every hash exactly as a new axis does. v3 fetched each embedded file's
// bytes by NAME, so two entries whose /UF collided hashed the first one's bytes twice, a /UF
// carrying a path hashed a constant, and any entry naming itself `nib-ceremony.json` was skipped as
// the record. v4 hashes each entry from its own filespec and excludes the record only in the exact
// shape nib writes — see hashEmbeddedFiles and ceremonyRecordEntry.
const ContentDigestVersion = 4

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
//
// **The name alone does not exclude anything (v4, /pending 725).** Until then any entry whose
// filespec called itself this dropped out of the digest. The exclusion is now the entry nib itself
// writes — this key, with /F and /UF both this name, and only one such key — see
// ceremonyRecordEntry.
const CeremonyRecordName = "nib-ceremony.json"

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
// embedded-files name tree entry by entry — each key with its own filespec and streams —
// minus the ceremony record (see CeremonyRecordName).
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
func ContentDigest(pdf []byte) (string, error) {
	d, _, err := contentDigest(pdf)
	return d, err
}

// digestStats is what one ContentDigest call DID, as counters rather than as elapsed time.
//
// **It exists because the two costs this function was rewritten to remove are both quadratic in
// pages, and a clock cannot assert that they are gone.** A timing test on a machine with other
// work on it fails for reasons that have nothing to do with the digest, and it passes a
// reintroduced O(pages²) loop whenever the box happens to be quiet. These counters do not move
// with the load:
//
//   - `decodes` counts flate (and friends) decodes. A document whose pages share one embedded font
//     must not decode it once per page — that was 74.6% of the profile.
//   - `fastPath` says the one-pass page-tree walk was used rather than `PageDict` per page. False
//     is CORRECT behaviour on a tree the two walks read differently (see digestPageDicts); it is a
//     finding only when a document that should take it does not.
//
// Nothing outside this package reads them, and nothing branches on them.
type digestStats struct {
	decodes  int
	fastPath bool
}

func contentDigest(pdf []byte) (string, *digestStats, error) {
	st := &digestStats{}
	ctx, err := pdfread.ReadOptimized(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return "", st, err
	}
	d, err := digestReadContext(ctx, st)
	return d, st, err
}

func digestReadContext(ctx *model.Context, st *digestStats) (string, error) {
	return digestWithMemo(ctx, newStreamMemo(st), st)
}

// digestWithMemo takes the memo as an argument so a test can hand it one that is already at its
// budget — the "stop storing" arm no real corpus document reaches.
func digestWithMemo(ctx *model.Context, sc *streamMemo, st *digestStats) (string, error) {
	h := sha256.New()
	// Every field is length-prefixed through these two helpers — see hashChunk. The first
	// draft wrote the resource kind and name unprefixed, which is injective by luck rather
	// than by construction (C3).
	hashChunk(h, []byte("nib-content-digest"))
	hashUint(h, ContentDigestVersion)
	hashUint(h, uint64(ctx.PageCount))
	pages, err := digestPageDicts(ctx)
	if err != nil {
		return "", err
	}
	st.fastPath = pages != nil
	for i := 1; i <= ctx.PageCount; i++ {
		// `pages` is nil whenever the one-pass walk was not usable, and a nil SLICE must not be
		// indexed — the first cut wrote `pages[i-1]` unguarded, which panics on exactly the
		// malformed documents the fallback exists for, turning a safety valve into a crash.
		var d types.Dict
		if i-1 < len(pages) {
			d = pages[i-1]
		}
		if d == nil {
			var err error
			d, _, _, err = ctx.PageDict(i, false)
			if err != nil {
				return "", fmt.Errorf("page %d is unreadable: %w", i, err)
			}
			if d == nil {
				// pdfcpu answers nil with NO error where its walk finds nothing at that number —
				// a /Page carrying /Kids, for one. `%w` of that nil printed `%!w(<nil>)`.
				return "", fmt.Errorf("page %d is unreadable: the page tree has no page at that "+
					"number, though it counts %d", i, ctx.PageCount)
			}
		}
		// **Exempt from `pdfread.PageContent` (ADR-056), by name.** The digest hashes pdfcpu's bare join, and what it
		// covers is a format (ADR-013): hashing the token-boundary join instead would move `DocHash` on a divided
		// page and read as tampering, so it is a `ContentDigestVersion` bump or nothing. /pending 718.
		c, err := ctx.PageContent(d, i) //pagecontent:exempt ContentDigest
		if err != nil && err != model.ErrNoContent {
			return "", fmt.Errorf("page %d content: %w", i, err)
		}
		hashChunk(h, c)
		// GEOMETRY, which the first draft did not cover at all. Shrinking /CropBox to
		// excise a paragraph, or setting /Rotate 90, changes what every reader displays.
		// It is per-page, and it is exactly what a general reviewer reads past.
		for _, key := range []string{"MediaBox", "CropBox", "Rotate"} {
			hashChunk(h, []byte(key))
			hashObject(ctx.XRefTable, d[key], h, 0, sc)
		}
		hashPageResources(ctx.XRefTable, d, h, sc)
		// ANNOTATIONS, because the exclusion's premise is false in the window this digest
		// is checked in. The argument was "everything else is covered by the signatures" —
		// but CheckDocument exists for the pre-FIRST-signature hop, where there are none,
		// and in that window Nib's OWN operations defeat a content-only digest: AddNotes
		// writes /Text annotations (visible sticky notes) and the form fill writes /V plus
		// widget /AP streams. Neither touches a content stream. For a contract, the form
		// values ARE the agreement.
		hashChunk(h, []byte("Annots"))
		hashObject(ctx.XRefTable, d["Annots"], h, 0, sc)
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
	// Entry by entry, each from its own filespec, since v4 — v3 resolved each entry's bytes by
	// NAME and a colliding /UF hashed one file twice (/pending 725; see hashEmbeddedFiles).
	// Sorted so the digest is a property of the document rather than of the tree's layout;
	// key and filespec are hashed as separate length-prefixed chunks, so a rename and an edit
	// cannot be made to cancel out.
	hashEmbeddedFiles(ctx, h, sc)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ErrPageTreeAmbiguous is ContentDigest's refusal of a page tree that two readings order differently.
//
// The digest is `ceremony.Record.DocHash` (ADR-013), a commitment to one page order. On a malformed tree
// `collectLeaves` and pdfcpu's `PageDict` (through `pdfread.Pages`, ADR-065) can agree on how MANY pages there
// are and still name different objects at a position, so the hash would cover an order no reader shows — or a
// different order from the one the next party's reader shows. There is no one right answer to commit to, so the
// document is refused and the user is told how to get a well-formed copy. /pending 755.
var ErrPageTreeAmbiguous = errors.New("this document's page tree is malformed: two readings of it disagree " +
	"about its pages, so there is no one order to commit to — re-save it (print to PDF, or Save As in another " +
	"application) before it is used in a ceremony")

// CheckPageOrder is ContentDigest's page-order check on its own: ErrPageTreeAmbiguous when the
// document's page tree is one two readings order differently, nil otherwise. It is the SAME
// predicate the digest applies (`digestPageDicts`, ADR-009) — one door, two callers — so a
// document it passes is one the digest will not refuse on this ground.
//
// `ceremony.Convene` calls it on the ORIGINAL document, because the digest there is taken over the
// prepared copy, whose merge rewrites the tree and settles the order silently; this is the only
// place the convener can be told. A document that will not read at all returns nil: that refusal is
// the reader's, and belongs to the door that reads it next.
//
// **It reads with validation and WITHOUT pdfcpu's Optimize**, which the digest's read runs. Measured
// on tier 4's 7.3 MB interrupt fixture: the optimized read is ~11 s and the validated one ~1.6 s,
// with the comparison itself ~20 ms. Validation is what rewrites a page tree (and what leaves shape
// (d)'s indirect empty /Kids alone); Optimize dedupes resources and does not touch /Kids. Measured
// agreement: the verdict matches ContentDigest's on the four traced shapes, the agreeing shapes and
// the 330 corpus documents the digest reads (`TestCheckPageOrderAgreesWithTheDigestOnTheCorpora`).
func CheckPageOrder(pdf []byte) error {
	ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
	if err != nil {
		return nil
	}
	_, err = digestPageDicts(ctx)
	return err
}

// pageTreeAmbiguous names the first page the two readings disagree on, wrapping ErrPageTreeAmbiguous.
func pageTreeAmbiguous(page int) error {
	return fmt.Errorf("%w (they first part at page %d)", ErrPageTreeAmbiguous, page)
}

// digestPageDicts returns the page dicts in document order, from ONE walk of the page tree.
//
// # Why not `ctx.PageDict(i)` per page, which is what this loop used to do
//
// `PageDict` walks from the page-tree root on every call, so a per-page loop is O(pages²). That is
// already this package's measured cost — `collectLeaves`' own doc comment cites it — and it is the
// second of the three terms `/pending 488` profiled. **There is a ONE-PASS walk in this package
// already** (`collectLeaves`, `pageselect.go`), written for exactly this reason, and ADR-009 says a
// rule reaching more than one call site is written once and every site calls it. A third page-tree
// walk here would be the thing that ADR forbids, and `collectLeaves`' own comment already carries
// the obligation: *"The two walks must agree."* This makes it three, through one door.
//
// # It is not authoritative, so it is checked against pdfcpu's reading (/pending 755)
//
// `ContentDigest`'s output is `ceremony.Record.DocHash` (ADR-013): a value a convener signs and
// every later party recomputes, where a moved byte reads as tampering across a point release. So
// this is an OPTIMISATION and never a re-decision about what a page is. A matching COUNT is not
// enough: traced, four malformed shapes give `collectLeaves` exactly `ctx.PageCount` leaves while
// pdfcpu's `PageDict` answers a different object at some position —
//
//   - a subtree whose `/Count` is too small while the root's total is right (pdfcpu skips by the count
//     and lands on the wrong kid, so it answers one page twice and another never);
//   - a `/Type /Page` carrying a DIRECT `/Kids`;
//   - a `/Type /Page` carrying an INDIRECT `/Kids` (pdfcpu's `ArrayEntry` is direct-only, so it answers the
//     node itself as the page, while `derefArray` descends);
//   - an empty `/Pages` whose `/Kids` is an INDIRECT empty array (validation returns before rewriting it,
//     and pdfcpu answers the `/Pages` dict itself as a page, where `collectLeaves` walks past it).
//
// So whenever the fast path would be taken, every leaf's reference is compared with `pdfread.Pages`' answer
// at the same position, and that answer must carry no error and a dict. **Any mismatch is
// `ErrPageTreeAmbiguous`**: hashing either reading would commit the convener to an order the other need not
// show. Where they agree the digest is the value it always was (the dicts are the same objects), so no
// `ContentDigestVersion` moves.
//
// **A nil slice means "ask pdfcpu for every page"**: a count disagreement, or a tree `collectLeaves` refuses
// (a cycle, depth past 50), hashes exactly as before this function existed, at the old cost.
func digestPageDicts(ctx *model.Context) ([]types.Dict, error) {
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		return nil, nil
	}
	leaves, _, err := collectLeaves(ctx.XRefTable, root)
	if err != nil || len(leaves) != ctx.PageCount {
		return nil, nil
	}
	pdfcpu := pdfread.Pages(ctx)
	if len(pdfcpu) != len(leaves) {
		return nil, pageTreeAmbiguous(min(len(pdfcpu), len(leaves)) + 1)
	}
	out := make([]types.Dict, 0, len(leaves))
	for i, l := range leaves {
		p := pdfcpu[i]
		if p.Err != nil || p.Dict == nil || p.Ref == nil || *p.Ref != l.ref {
			return nil, pageTreeAmbiguous(i + 1)
		}
		out = append(out, l.dic)
	}
	return out, nil
}

// embeddedEntry is one key/value pair of the catalog's /Names /EmbeddedFiles tree, as the tree
// holds it: the key, and the entry's own filespec object (usually an indirect reference).
type embeddedEntry struct {
	key string
	fs  types.Object
}

// hashEmbeddedFiles folds the catalog name tree into the digest, ENTRY BY ENTRY, minus the
// ceremony record.
//
// # Each entry is hashed from its own filespec, never resolved by name (v4, /pending 725)
//
// v3 enumerated the tree, took each entry's cleaned file NAME, and fetched the bytes back through
// pdfcpu's `ExtractAttachment` — which resolves a name by tree key and then by the first filespec
// whose /UF, /F or /Desc matches. So which entry's bytes were hashed was decided by strings the
// document's author writes. Measured: keys `a.txt` and `b.txt` both carrying `/UF (a.txt)` hashed
// a.txt's bytes twice, and b.txt could say anything; a /UF carrying a path cleaned to a basename
// nothing resolved, so a constant marker was hashed; and any entry whose filespec named itself
// `nib-ceremony.json` was skipped as the record. And `ListAttachments` fails the WHOLE tree on one
// filespec it cannot name, which v3 then hashed as an empty tree.
//
// Now each entry contributes its key and `hashFileSpec` of its own filespec: /F, /UF, /Desc and
// the /EF streams, decoded where the filter decodes (a stream that will not decode, or a reference
// that will not resolve, hashes a marker — never nothing, so a broken filespec cannot hide an
// edit). The per-entry sub-digests are sorted, by key and then by sub-digest, so the result
// is a property of the document rather than of the tree's layout, and a malformed tree carrying one
// key twice has both entries covered.
//
// Page-level /FileAttachment annotations are deliberately NOT walked here: they hang off
// `/Annots`, which the per-page loop above already hashes. Hashing them twice would be
// harmless but would state the coverage in two places.
func hashEmbeddedFiles(ctx *model.Context, h hash.Hash, sc *streamMemo) {
	xt := ctx.XRefTable
	hashChunk(h, []byte("embedded-files"))
	entries, err := treeEntries(xt)
	if err != nil {
		// A tree that is present and unreadable is a different document from one with no tree.
		hashChunk(h, []byte("#unreadable-tree"))
		return
	}
	skip, _ := ceremonyRecordEntry(xt, entries)
	type summed struct {
		key string
		sum []byte
	}
	sums := make([]summed, 0, len(entries))
	for i, e := range entries {
		if i == skip {
			continue // the self-reference; see CeremonyRecordName
		}
		eh := sha256.New()
		hashChunk(eh, []byte(e.key))
		hashFileSpec(xt, e.fs, eh, sc)
		sums = append(sums, summed{key: e.key, sum: eh.Sum(nil)})
	}
	sort.Slice(sums, func(i, j int) bool {
		if sums[i].key != sums[j].key {
			return sums[i].key < sums[j].key
		}
		return bytes.Compare(sums[i].sum, sums[j].sum) < 0
	})
	hashUint(h, uint64(len(sums)))
	for _, s := range sums {
		hashChunk(h, []byte(s.key))
		hashChunk(h, s.sum)
	}
}

// hashFileSpec writes what an embedded-files entry SHOWS a reader: the filespec's /F, /UF and
// /Desc, and the bodies of its /EF /F and /EF /UF streams — decoded where the filter decodes.
//
// **A projection, not `hashObject` of the whole filespec, and measured rather than chosen.** The
// first cut hashed the dict entire, and the generated corpus's `attachment` row came out different
// on every run: `AddAttachment` stamps the stream's `/Params /ModDate` with the wall clock. That is
// stored once and so is stable within one document, but it moves the digest on every re-embed of
// identical bytes — `CarryAttachments` re-adds each file — which reads as tampering over a copy
// that changed nothing a party agreed to. The bytes and the names are the agreement.
//
// Each stream is reached through its OWN filespec: `/EF /F` and `/EF /UF` are both hashed because
// the format lets them differ and a reader may show either. A filespec that is not a dict (the
// string form) is hashed as an object behind a marker; a stream that will not resolve hashes
// `#unreadable` — never nothing, so a broken filespec cannot hide an edit.
func hashFileSpec(xt *model.XRefTable, o types.Object, h hash.Hash, sc *streamMemo) {
	fs, err := xt.DereferenceDict(o)
	if err != nil || fs == nil {
		hashChunk(h, []byte("#filespec-object"))
		hashObject(xt, o, h, 0, sc)
		return
	}
	for _, k := range []string{"F", "UF", "Desc"} {
		hashChunk(h, []byte(k))
		hashObject(xt, fs[k], h, 0, sc)
	}
	hashChunk(h, []byte("EF"))
	ef := derefDict(xt, fs["EF"])
	if ef == nil {
		hashChunk(h, []byte("#none"))
		return
	}
	for _, k := range []string{"F", "UF"} {
		hashChunk(h, []byte(k))
		so, found := ef.Find(k)
		if !found || so == nil {
			hashChunk(h, []byte("#nil"))
			continue
		}
		num := 0
		if ir, ok := so.(types.IndirectRef); ok {
			num = ir.ObjectNumber.Value()
		}
		sd, _, err := xt.DereferenceStreamDict(so)
		if err != nil || sd == nil {
			hashChunk(h, []byte("#unreadable"))
			continue
		}
		hashStreamBody(sd, h, sc, num)
	}
}

// ceremonyRecordEntry returns the index of the one entry ContentDigest excludes as the ceremony
// record, or -1 when none is.
//
// **The exclusion is the SHAPE nib writes, not a name an author can type.** `ceremony.Embed` goes
// through `AddAttachment`, which keys the tree `nib-ceremony.json` and writes the filespec's /F and
// /UF as that same string (pdfcpu's `NewFileSpecDict(id, id, …)`). An entry is the record only if
// all three agree; an ordinary entry that merely CALLS itself the record — by its /UF, by its /F,
// or by its key alone — is hashed like any other, so the name buys an attacker no hiding place.
//
// **And only when exactly one entry has the record's key.** pdfcpu refuses a duplicate key on Add
// but its reader keeps every entry of a malformed tree, and `Extract` reads the FIRST. Excluding
// either of two would leave the other unbound; excluding neither makes the digest cover a record,
// so the document's DocHash cannot match and every gate that compares it refuses — which is the
// right answer for a document carrying two ceremony records.
//
// **It is the one answer to "which entry is the record", and not only the digest's** (/pending
// 745). `CeremonyRecord` (what `ceremony.Extract` reads) and the `Ceremony` label `Attachments`
// shows both ask it, so the entry the digest leaves out is exactly the entry read as the record and
// labelled as one. `twice` reports the malformed tree with the record's key more than once, which
// every caller refuses rather than choosing.
func ceremonyRecordEntry(xt *model.XRefTable, entries []embeddedEntry) (idx int, twice bool) {
	found := -1
	for i, e := range entries {
		if e.key != CeremonyRecordName {
			continue
		}
		if found >= 0 {
			return -1, true
		}
		found = i
	}
	if found < 0 {
		return -1, false
	}
	fs, err := xt.DereferenceDict(entries[found].fs)
	if err != nil || fs == nil {
		return -1, false
	}
	for _, k := range []string{"F", "UF"} {
		o, ok := fs.Find(k)
		if !ok {
			return -1, false
		}
		s, err := xt.DereferenceStringOrHexLiteral(o, model.V10, nil)
		if err != nil || s != CeremonyRecordName {
			return -1, false
		}
	}
	return found, false
}

// treeEntries walks the catalog's /Names /EmbeddedFiles tree and returns every key/value pair as
// the tree holds it, in the tree's order — a malformed tree carrying one key twice yields both.
// No tree is (nil, nil); a tree that is present and cannot be located is an error.
//
// **The one enumeration of the tree** (/pending 745, ADR-009). ContentDigest, the listing, every
// extraction and `CarryAttachments` read entries through here, each entry from its OWN filespec.
// pdfcpu's `ExtractAttachment` resolves a name by tree key and then by the first filespec whose
// /UF, /F or /Desc matches, so which entry answered was chosen by strings a document's author
// writes; nothing in this package may reach an embedded file that way again.
func treeEntries(xt *model.XRefTable) ([]embeddedEntry, error) {
	if xt.Names["EmbeddedFiles"] == nil && !xt.Valid {
		if err := xt.LocateNameTree("EmbeddedFiles", false); err != nil {
			return nil, err
		}
	}
	var entries []embeddedEntry
	if root := xt.Names["EmbeddedFiles"]; root != nil {
		_ = root.Process(xt, func(_ *model.XRefTable, k string, v *types.Object) error {
			entries = append(entries, embeddedEntry{key: k, fs: *v})
			return nil
		})
	}
	return entries, nil
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
// Indirect references are followed, bounded by depth AND by a first-visit set scoped to this
// top-level call. Object NUMBERS are never hashed: they change on every pdfcpu rewrite, which is
// the whole reason this function exists instead of a byte hash.
//
// # The set was claimed here and did not exist (/pending 454)
//
// This comment read *"bounded by depth and by an on-path object set"* from the commit that
// introduced the function (`67015a7`, v1.117.0) — it described the fix rather than the code, and
// there was no set of any kind. A shared object graph was therefore re-walked once per PATH.
//
// **It is not a corner case; two shipped features compose into it.** A `/Link` annotation's
// `/Dest`, and equally a widget annot's `/P`, dereferences to a PAGE dict, whose `/Parent` is a
// Pages node whose `/Kids` is every page in the document — so from one page's `/Annots` the walk
// reaches every other page, their annots, and back again. Measured on documents built entirely
// through Nib's own doors (`testpdf.Text(N)` + `AddNotes`, one sticky note per page):
//
//	2 pages     6 ms        8 pages    1.44 s
//	4 pages    83 ms       10 pages    3.61 s
//	6 pages   385 ms       12 pages   42.58 s
//
// That is ~N⁴. A twenty-page contract with a note on each page takes about a minute; `convene` has
// no page cap and `cmd/nib` sets no `ReadTimeout` or `WriteTimeout`, and `ContentDigest` takes no
// context, so nothing can cancel it. On one real 172-page statute PDF it did not finish in 29m50s
// for a SINGLE page. It is reachable on three production doors — convene, `ReadMirror` and
// `checkArrival` — and 19 of 320 ordinary user documents on this machine are in the same class.
//
// # Why a first-visit INDEX and not the object number
//
// A repeat emits `#again` plus the position at which that object was first met in THIS walk. The
// object number cannot be hashed — see the rule above, it changes on every rewrite — but the
// first-visit position is stable, because the walk order is deterministic: dict keys are sorted and
// arrays are in order, so the Nth distinct object reached is the same object whatever pdfcpu
// numbered it.
//
// # Why the scope is one top-level call, and why there is no version bump
//
// Per-DOCUMENT scope is 8× faster again and it is WRONG: it changes the digest of ordinary
// documents, measured on two controls immediately. Per-call preserves it — on all 197 live
// ceremonies in `~/nib/ceremonies/` the stored `DocHash` still reproduces, 197 of 197.
//
// A `ContentDigestVersion` bump would be the *unsafe* option here, which is the opposite of how it
// looks: `ReadMirror` compares a stored digest through `Record.Verify`, and `Verify` never reads
// `DigestVersion` at all — so a bumped build reading an in-flight ceremony reports "the copy on this
// machine is damaged or incomplete", which is the false accusation that constant exists to prevent.
func hashObject(xt *model.XRefTable, o types.Object, h hash.Hash, depth int, sc *streamMemo) {
	hashObjectSeen(xt, o, h, depth, map[int]int{}, sc, 0)
}

// hashObjectSeen is `hashObject`'s recursion, carrying the visited set and the memo.
//
// `num` is the object number `o` was dereferenced from, or 0 for an object reached inline. It is
// carried for the stream memo and for nothing else — it is NEVER hashed, which is the rule this
// whole function exists to keep (see `hashObject`'s doc above).
func hashObjectSeen(xt *model.XRefTable, o types.Object, h hash.Hash, depth int, seen map[int]int, sc *streamMemo, num int) {
	if depth > 16 {
		hashChunk(h, []byte("#depth"))
		return
	}
	if ir, ok := o.(types.IndirectRef); ok {
		num = ir.ObjectNumber.Value()
		if idx, met := seen[num]; met {
			// Met already in this walk: record THAT it recurred and where it first appeared, so
			// "the same object again" stays distinguishable from "a different object here" without
			// re-expanding it. Recorded before dereferencing, so a cycle terminates here too.
			hashChunk(h, []byte("#again"))
			hashUint(h, uint64(idx))
			return
		}
		seen[num] = len(seen)
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
			hashObjectSeen(xt, v[k], h, depth+1, seen, sc, 0)
		}
	case types.StreamDict:
		hashChunk(h, []byte("#stream"))
		hashObjectSeen(xt, v.Dict, h, depth+1, seen, sc, 0)
		hashStreamBody(&v, h, sc, num)
	case types.Array:
		hashChunk(h, []byte("#array"))
		hashUint(h, uint64(len(v)))
		for _, e := range v {
			hashObjectSeen(xt, e, h, depth+1, seen, sc, 0)
		}
	default:
		// Names, strings, numbers, booleans. PDFString is canonical for these — the
		// ordering hazard is dicts, and those are handled above.
		hashChunk(h, []byte(o.PDFString()))
	}
}

// streamMemo remembers the DECODED bytes of a stream object across one ContentDigest call.
//
// # What it changes, and what it must not
//
// `hashObject` starts a fresh `seen` map per top-level call, and `ContentDigest` makes one such
// call per geometry key, per `/Annots` and **per resource name per page**. A font shared by a
// thousand pages is therefore reached a thousand times, and each arrival flate-decodes the
// embedded program again. Measured on a 1,000-page tagged document (`TestZZ`-style fixture,
// 8,000 Markdown clauses): `hashPageResources` was 74.6% of `ContentDigest` and 94% of THAT was
// `filter.flate.Decode`, under `hashStreamBody`.
//
// **This memo changes how OFTEN a stream is decoded and nothing else.** The bytes written into
// the hash, the marker in front of them, the order they arrive in, and every `#again` index are
// identical, because the decode is a pure function of `sd.Raw` and `sd.FilterPipeline` and
// neither moves within one `model.Context`. The `seen` map is untouched: it stays per-call, which
// ADR-013's territory requires — the doc comment on `hashObject` records that per-DOCUMENT `seen`
// scope was measured to CHANGE the digest of ordinary documents, and it is not what this is.
// `TestContentDigestIsByteIdenticalAcrossTheExternalCorpus` is the acceptance.
//
// # Why the second sighting and not the first
//
// The memo stores an object only when it is asked for a SECOND time. A one-page-one-image scan
// reaches each image exactly once, so caching on first sight would hold every decoded image in
// the document at once — hundreds of megabytes for a document whose whole point is that it is
// large — and buy nothing, because nothing asks again. Deferring to the second sighting costs one
// extra decode for each object that IS shared (two rather than one, against the thousand it
// replaces) and spends memory only where there is reuse.
//
// # And it is bounded, because a memo that cannot be capped is a second byte cap
//
// ADR-005 bounds open documents at 512 MiB of `doc.data`; a digest that could allocate without a
// ceiling would be a refusal a user cannot act on, arriving from a different door. Past
// streamMemoBudget the memo simply stops storing — every later lookup re-decodes, which is what
// the code did before this existed, so exceeding the budget costs speed and never correctness.
type streamMemo struct {
	met   map[int]bool          // object numbers reached once
	body  map[int]decodedStream // object numbers reached twice or more, with their decode
	spent int64
	st    *digestStats
}

type decodedStream struct {
	marker byte
	body   []byte
}

// streamMemoBudget is how many decoded bytes one ContentDigest call may hold.
//
// 64 MiB, which is an eighth of ADR-005's per-document ceiling and comfortably more than the
// embedded font programs of any document seen here (a full CJK face is ~20 MB, and a document
// carrying three of them still fits). It is a budget on what is RETAINED, not on what is decoded.
const streamMemoBudget = 64 << 20

func newStreamMemo(st *digestStats) *streamMemo {
	return &streamMemo{met: map[int]bool{}, body: map[int]decodedStream{}, st: st}
}

// hashStreamBody writes a stream's bytes, decoded where the filter decodes.
//
// The marker distinguishes the two: without it a document whose filter this build cannot
// decode hashes identically to one where the decode produced nothing.
func hashStreamBody(sd *types.StreamDict, h hash.Hash, sc *streamMemo, num int) {
	d := decodeStream(sd, sc, num)
	hashChunk(h, []byte{d.marker})
	hashChunk(h, d.body)
}

// decodeStream is hashStreamBody's decode, through the memo when there is one.
//
// num == 0 means the stream was not reached through an indirect reference and so has no stable
// key; it is decoded unmemoised. Object numbers are 1-based, so 0 cannot collide with a real one.
func decodeStream(sd *types.StreamDict, sc *streamMemo, num int) decodedStream {
	if sc != nil && num != 0 {
		if d, ok := sc.body[num]; ok {
			return d
		}
	}
	if sc != nil && sc.st != nil {
		sc.st.decodes++
	}
	d := decodedStream{marker: 0, body: sd.Raw}
	if derr := sd.Decode(); derr == nil {
		d = decodedStream{marker: 1, body: sd.Content}
	}
	if sc == nil || num == 0 {
		return d
	}
	if !sc.met[num] {
		// First sighting: record that it happened and keep nothing. Most objects are reached
		// exactly once and storing them is pure waste — see the type's doc comment.
		sc.met[num] = true
		return d
	}
	if n := int64(len(d.body)); sc.spent+n <= streamMemoBudget {
		sc.spent += n
		sc.body[num] = d
	}
	return d
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
func hashPageResources(xt *model.XRefTable, page types.Dict, h hash.Hash, sc *streamMemo) {
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
			hashObject(xt, sub[name], h, 0, sc)
		}
	}
}

// ExtractAttachment returns the decoded bytes of the one embedded file `ref` names — see
// ReadAttachment, which it is.
func ExtractAttachment(pdf []byte, ref string) ([]byte, error) {
	_, data, err := ReadAttachment(pdf, ref)
	return data, err
}

// ReadAttachment returns the one embedded file `ref` addresses, and its decoded bytes, read from
// that entry's OWN filespec (/pending 745).
//
// `ref` is an `AttachmentInfo.ID` first: when exactly one file has that ID, it is the answer. Only
// when none does is it taken as a displayed name — so a person typing the name the listing showed
// still reaches the file — and then exactly one file may carry that name. Anything else is
// refused, naming how many answered: a ref two entries answer to is the question "which one", and
// the answer is never "the first", which is how a download served one file's bytes under another's
// name.
func ReadAttachment(pdf []byte, ref string) (AttachmentInfo, []byte, error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return AttachmentInfo{}, nil, err
	}
	files, err := embeddedFiles(ctx)
	if err != nil {
		return AttachmentInfo{}, nil, err
	}
	pick := func(match func(AttachmentInfo) bool) []embeddedFile {
		var hit []embeddedFile
		for _, f := range files {
			if match(f.info) {
				hit = append(hit, f)
			}
		}
		return hit
	}
	hit := pick(func(a AttachmentInfo) bool { return a.ID == ref })
	if len(hit) == 0 {
		name := attachmentName(ref)
		hit = pick(func(a AttachmentInfo) bool { return name != "" && a.Name == name })
	}
	switch len(hit) {
	case 0:
		return AttachmentInfo{}, nil, fmt.Errorf("no attachment named %q", ref)
	case 1:
	default:
		return AttachmentInfo{}, nil, fmt.Errorf("%d attachments answer to %q, so which one is meant has "+
			"no answer; name one by its id (`nib attachments --json` lists them)", len(hit), ref)
	}
	if hit[0].fs == nil {
		return AttachmentInfo{}, nil, fmt.Errorf("attachment %q has no file specification", ref)
	}
	data, err := fileSpecBytes(ctx.XRefTable, hit[0].fs)
	if err != nil {
		return AttachmentInfo{}, nil, fmt.Errorf("attachment %q: %w", ref, err)
	}
	return hit[0].info, data, nil
}

// ErrTwoCeremonyRecords is CeremonyRecord's refusal of a tree carrying the record's key more than
// once. pdfcpu will not write one; its reader keeps every entry of one someone else wrote.
var ErrTwoCeremonyRecords = errors.New("the document carries two ceremony records, so which " +
	"ceremony it belongs to has no answer")

// CeremonyRecord returns the bytes of the ceremony record — the entry `ceremonyRecordEntry` names,
// which is the entry ContentDigest leaves out, and no other (/pending 745). It is (nil, nil) when
// there is none, and ErrTwoCeremonyRecords when the tree carries the record's key twice: the digest
// excludes neither, and a reader choosing one would name a ceremony the document cannot match.
//
// **Not by name.** `ceremony.Extract` read the record through the name lookup, which took the tree
// key and then the first filespec calling itself `nib-ceremony.json` — so "which entry is the
// record" had one answer in the digest and another in the reader. An entry that merely CALLS itself
// the record is an ordinary attachment to both now.
func CeremonyRecord(pdf []byte) ([]byte, error) {
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return nil, err
	}
	xt := ctx.XRefTable
	entries, err := treeEntries(xt)
	if err != nil {
		return nil, err
	}
	i, twice := ceremonyRecordEntry(xt, entries)
	if twice {
		return nil, ErrTwoCeremonyRecords
	}
	if i < 0 {
		return nil, nil
	}
	fs, err := xt.DereferenceDict(entries[i].fs)
	if err != nil || fs == nil {
		return nil, fmt.Errorf("the ceremony record has no file specification")
	}
	return fileSpecBytes(xt, fs)
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
	ctx, err := inspectionRead(pdf)
	if err != nil {
		return nil, err
	}
	root, err := ctx.Catalog()
	if err != nil {
		return nil, err
	}
	var out []SignatureWidget
	if err := eachPageAnnot(ctx.XRefTable, root, func(annot types.Dict, nr int) {
		if nameVal(annot, "Subtype") != "Widget" {
			return
		}
		// /FT is inheritable from ANY ancestor field (ISO 32000-1 Table 220), not only the
		// widget's direct parent — `inheritedFieldType` is the one reading of it.
		if inheritedFieldType(ctx.XRefTable, annot) != "Sig" {
			return
		}
		r := derefArray(ctx.XRefTable, annot["Rect"])
		if len(r) != 4 {
			return
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
			return
		}
		out = append(out, SignatureWidget{
			Page:  nr,
			Rect:  rect,
			HasAP: derefDict(ctx.XRefTable, annot["AP"]) != nil,
		})
	}); err != nil {
		return nil, err
	}
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
