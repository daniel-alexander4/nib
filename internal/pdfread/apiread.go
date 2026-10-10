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
// function did with the context (pkg/api, v0.13.0; the line is cited on each). `Encrypt` and `RemovePassword` are
// no longer restated here: they are `pdfops`' rewrite door under the command (`protectRewrite`), over the same
// `ReadOptimized`, because a password change must drop a conformance identification (`/pending 641`).
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

// MergeRaw is `api.MergeRaw` without a divider page (merge.go:61): the first document is the destination and each
// later one is merged into it, then the closing pass — through `Optimize`, so it is budgeted over the MERGED
// context, which no part's own budget bounds (two parts' forms of one `/Length` are compared with each other).
// `pdfops`' own merge door (`mergeOnce`, ADR-048) is the one that carries structure; this is for parts nib built
// itself or a packet's exhibits.
//
// **More than `mergeFan` parts are merged in groups, and the groups merged.** pdfcpu appends a document by
// putting the destination's page tree UNDER a new root, so each part merged in makes the tree one level
// deeper, and a hundred of them reached the depth every later read refuses: measured, 499 one-page parts
// failed with "page tree depth 101 exceeds limit 100" (`/pending 771`) — an image export or a redaction of
// that many pages. Grouped, the depth grows by the fan once per level instead of once per part. The first
// part of the first group is still the destination whose catalog is kept.
func MergeRaw(pdfs [][]byte) ([]byte, error) {
	for len(pdfs) > mergeFan {
		groups := make([][]byte, 0, len(pdfs)/mergeFan+1)
		for from := 0; from < len(pdfs); from += mergeFan {
			thru := min(from+mergeFan, len(pdfs))
			if thru-from == 1 {
				groups = append(groups, pdfs[from])
				continue
			}
			g, err := mergeRawOnce(pdfs[from:thru])
			if err != nil {
				return nil, err
			}
			groups = append(groups, g)
		}
		pdfs = groups
	}
	return mergeRawOnce(pdfs)
}

// mergeFan is how many parts one pass of MergeRaw merges into one. A pass adds at most this many levels to
// the page tree, so three passes — 13,824 parts — stay under the read limit of 100.
const mergeFan = 24

// mergeRawOnce merges pdfs in one pass, each into the first.
func mergeRawOnce(pdfs [][]byte) (out []byte, err error) {
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
		PutBackValidatorLosses(src) // before the merge renumbers its references (`write.go`)
		if err := pdfcpu.MergeXRefTables(strconv.Itoa(i), src, dest, false, false); err != nil {
			return nil, err
		}
	}
	if conf.OptimizeBeforeWriting {
		if err := Optimize(dest); err != nil {
			return nil, err
		}
	}
	return Write(dest)
}
