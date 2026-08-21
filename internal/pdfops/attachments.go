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
		out = append(out, AttachmentInfo{Name: name, Desc: a.Desc})
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
// This is stable where that is not. Measured on the same run: identical across adding an
// attachment and across three incremental signatures.
//
// **What it does not cover, stated rather than discovered:** annotations, form field
// values, attachments, and metadata. It answers "is this the same document" about the
// visible content, which is what a ceremony is convened over. Tamper-evidence for
// everything else is what the signatures are for — they cover the actual bytes, and any
// edit to them flips the verification verdict.
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
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(ctx.PageCount))
	h.Write(n[:])
	for i := 1; i <= ctx.PageCount; i++ {
		d, _, _, err := ctx.PageDict(i, false)
		if err != nil || d == nil {
			return "", fmt.Errorf("page %d is unreadable: %w", i, err)
		}
		c, err := ctx.PageContent(d, i)
		if err != nil && err != model.ErrNoContent {
			return "", fmt.Errorf("page %d content: %w", i, err)
		}
		binary.BigEndian.PutUint64(n[:], uint64(len(c)))
		h.Write(n[:]) // length-prefixed, so two pages cannot merge across the boundary
		h.Write(c)
		hashPageResources(ctx.XRefTable, d, h)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// resourceKinds are the page-visible resource categories folded into ContentDigest.
//
// XObject holds images and form XObjects — for a scan, the whole visible page. Font decides
// what the glyphs the content stream names actually look like. The others (ColorSpace,
// Pattern, Shading, ExtGState) are deliberately out: they are parameters rather than
// content, and every one of them that changes what is drawn does so through a stream these
// two already cover.
var resourceKinds = []string{"Font", "XObject"}

// hashPageResources folds a page's resource streams into h, in a canonical order.
//
// Order comes from the resource NAMES, not from object numbers — object numbers change on
// every pdfcpu rewrite and names do not, which is the same reason ContentDigest hashes the
// stream rather than the file.
func hashPageResources(xt *model.XRefTable, page types.Dict, h hash.Hash) {
	res, _ := xt.DereferenceDict(page["Resources"])
	if res == nil {
		return
	}
	var n [8]byte
	for _, kind := range resourceKinds {
		sub, _ := xt.DereferenceDict(res[kind])
		if sub == nil {
			continue
		}
		names := make([]string, 0, len(sub))
		for k := range sub {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			sd, _, err := xt.DereferenceStreamDict(sub[name])
			if err != nil || sd == nil {
				continue
			}
			h.Write([]byte(kind))
			h.Write([]byte(name))
			body := sd.Raw
			marker := byte(0) // raw: this build could not decode the filter
			if derr := sd.Decode(); derr == nil {
				body = sd.Content
				marker = 1
			}
			// The marker matters: without it a document whose filter we cannot decode
			// hashes the same as one whose decode returned nothing.
			h.Write([]byte{marker})
			binary.BigEndian.PutUint64(n[:], uint64(len(body)))
			h.Write(n[:])
			h.Write(body)
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
