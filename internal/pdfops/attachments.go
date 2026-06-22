package pdfops

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// AttachmentInfo names one embedded file in a document's Names→EmbeddedFiles
// tree. (Page-level FileAttachment annotations are a separate, rarer carrier and
// are not listed here — Scan still flags them.)
type AttachmentInfo struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// Attachments lists the document-level embedded files. pdfcpu's ListAttachments
// returns stubs (name + description, no data), so this is cheap. An empty result
// (no name tree) is not an error.
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
	return out, nil
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
func ExtractAttachment(pdf []byte, name string) ([]byte, error) {
	name = attachmentName(name)
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	a, err := ctx.ExtractAttachment(model.Attachment{ID: name})
	if err != nil {
		return nil, err
	}
	if a == nil || a.Reader == nil {
		return nil, fmt.Errorf("no attachment named %q", name)
	}
	return io.ReadAll(a.Reader)
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
