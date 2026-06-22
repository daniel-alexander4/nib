package pdfops

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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
	return strings.TrimSpace(name)
}
