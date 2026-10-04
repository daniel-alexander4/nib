package pdfread

import (
	"bytes"
	"errors"
	"sort"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// pdfcpu's reader-taking `api` functions, restated over this package's reads — `/pending 716`.
//
// Each of these `api` functions reads the document ITSELF, through pdfcpu's `ReadAndValidate` and — for all but
// `NUp`, `PageDims` and `MergeRaw`'s reads — `ReadValidateAndOptimize`, whose optimize pass has no budget
// (`optimize.go`). nib used to hand them a reader from a door that only ran the reference check on a SECOND,
// unvalidated parse, so the pass ran unbounded inside them: on the 706 chain of 400 forms (68 KB), `Outline`,
// `SplitByBookmarks`, `StampTextLayer`, `StampImages`, `FillFormCSV`, `FillFormXFDF`, `ExportFormJSON`, image
// extraction, `Encrypt`, `RemovePassword` and `RedactPages` (through `MergeRaw`'s closing pass) each ran past 30 s.
// Restated, each reads once, through `Validated` and the budgeted pass, and then does exactly what the `api`
// function did with the context (pkg/api, v0.13.0; the line is cited on each).
//
// There is no longer any door that hands pdfcpu a reader of a PDF: `TestEveryValidatingReadRoutesThroughTheDoor`
// refuses a call outside this package to any `api` function that reads a PDF from a reader, wherever the reader
// was built.

// copyConf is conf, or pdfcpu's default where an `api` function would have made one. The caller's configuration is
// copied: pdfcpu writes to the one it is given.
func copyConf(conf *model.Configuration) *model.Configuration {
	if conf == nil {
		return model.NewDefaultConfiguration()
	}
	cp := *conf
	return &cp
}

// apiConf is copyConf with cmd set as the `api` function sets it.
func apiConf(conf *model.Configuration, cmd model.CommandMode) *model.Configuration {
	c := copyConf(conf)
	c.Cmd = cmd
	return c
}

func write(ctx *model.Context) ([]byte, error) {
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// PageDims is `api.PageDims` (page.go:240).
func PageDims(pdf []byte, conf *model.Configuration) (pd []types.Dim, err error) {
	defer fault.Catch(&err)
	ctx, err := Validated(pdf, copyConf(conf))
	if err != nil {
		return nil, err
	}
	if pd, err = ctx.PageDims(); err != nil {
		return nil, err
	}
	if len(pd) != ctx.PageCount {
		return nil, errors.New("pdfcpu: corrupt page dimensions")
	}
	return pd, nil
}

// Bookmarks is `api.Bookmarks` with a nil configuration (bookmark.go:35).
func Bookmarks(pdf []byte) (bms []pdfcpu.Bookmark, err error) {
	defer fault.Catch(&err)
	ctx, err := ReadOptimized(pdf, apiConf(nil, model.LISTBOOKMARKS))
	if err != nil {
		return nil, err
	}
	return pdfcpu.Bookmarks(ctx)
}

// ExportForm is `api.ExportForm` (form.go:402).
func ExportForm(pdf []byte, source string) (fg *form.FormGroup, err error) {
	defer fault.Catch(&err)
	ctx, err := ReadOptimized(pdf, apiConf(nil, model.EXPORTFORMFIELDS))
	if err != nil {
		return nil, err
	}
	fg, ok, err := form.ExportForm(ctx.XRefTable, source)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, api.ErrNoFormFieldsAffected
	}
	return fg, nil
}

// ExportFormJSON is `api.ExportFormJSON` (form.go:431).
func ExportFormJSON(pdf []byte, source string) (out []byte, err error) {
	defer fault.Catch(&err)
	ctx, err := ReadOptimized(pdf, apiConf(nil, model.EXPORTFORMFIELDS))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	ok, err := form.ExportFormJSON(ctx.XRefTable, source, &b)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, api.ErrNoFormFieldsAffected
	}
	return b.Bytes(), nil
}

// ExtractImages is `api.ExtractImages` (extract.go:149). **It refuses past the budget rather than skipping**:
// pdfcpu finds a page's images in what the pass records (`ExtractPageImages` reads `ctx.Optimize.PageImages`), so a
// skipped pass would report "no images" for a document full of them.
func ExtractImages(pdf []byte, selectedPages []string, digest func(model.Image, bool, int) error) (err error) {
	defer fault.Catch(&err)
	ctx, err := ReadOptimizedOrRefuse(pdf, apiConf(nil, model.EXTRACTIMAGES))
	if err != nil {
		return err
	}
	pages, err := api.PagesForPageSelection(ctx.PageCount, selectedPages, true, true)
	if err != nil {
		return err
	}
	var sp []int
	for i, v := range pages {
		if v {
			sp = append(sp, i)
		}
	}
	sort.Ints(sp)
	if len(sp) == 0 {
		return nil
	}
	maxPageDigits := len(strconv.Itoa(sp[len(sp)-1]))
	for _, p := range sp {
		mm, err := pdfcpu.ExtractPageImages(ctx, p, false)
		if err != nil {
			return err
		}
		for _, img := range mm {
			if err := digest(img, len(mm) == 1, maxPageDigits); err != nil {
				return err
			}
		}
	}
	return nil
}

// Encrypt is `api.Encrypt` (crypto.go:30, which is `api.Optimize` under `model.ENCRYPT`). conf carries the passwords.
func Encrypt(pdf []byte, conf *model.Configuration) ([]byte, error) {
	if conf == nil {
		return nil, errors.New("pdfcpu: missing configuration for encryption")
	}
	return rewrite(pdf, apiConf(conf, model.ENCRYPT))
}

// Decrypt is `api.Decrypt` (crypto.go:101).
func Decrypt(pdf []byte, conf *model.Configuration) ([]byte, error) {
	if conf == nil {
		return nil, errors.New("pdfcpu: missing configuration for decryption")
	}
	return rewrite(pdf, apiConf(conf, model.DECRYPT))
}

// rewrite is `api.Optimize` (optimize.go:32) without its stats file: read, optimize, write.
func rewrite(pdf []byte, conf *model.Configuration) (out []byte, err error) {
	defer fault.Catch(&err)
	ctx, err := ReadOptimized(pdf, conf)
	if err != nil {
		return nil, err
	}
	return write(ctx)
}

// MergeRaw is `api.MergeRaw` without a divider page (merge.go:61): the first document is the destination and each
// later one is merged into it, then the closing pass — through `Optimize`, so it is budgeted over the MERGED
// context, which no part's own budget bounds (two parts' forms of one `/Length` are compared with each other).
// `pdfops`' own merge door (`mergeOnce`, ADR-048) is the one that carries structure; this is for parts nib built
// itself or a packet's exhibits.
func MergeRaw(pdfs [][]byte) (out []byte, err error) {
	defer fault.Catch(&err)
	if len(pdfs) == 0 {
		return nil, errors.New("pdfcpu: MergeRaw: missing rsc")
	}
	conf := apiConf(nil, model.MERGECREATE)
	conf.ValidationMode = model.ValidationRelaxed
	conf.CreateBookmarks = false
	dest, err := Validated(pdfs[0], conf)
	if err != nil {
		return nil, err
	}
	dest.EnsureVersionForWriting()
	for i, b := range pdfs[1:] {
		src, err := Validated(b, dest.Configuration)
		if err != nil {
			return nil, err
		}
		if dest.XRefTable.Version() < model.V20 && src.XRefTable.Version() == model.V20 {
			return nil, pdfcpu.ErrUnsupportedVersion
		}
		if err := pdfcpu.MergeXRefTables(strconv.Itoa(i), src, dest, false, false); err != nil {
			return nil, err
		}
	}
	if conf.OptimizeBeforeWriting {
		if err := Optimize(dest); err != nil {
			return nil, err
		}
	}
	return write(dest)
}
