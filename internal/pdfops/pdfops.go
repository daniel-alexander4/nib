// Package pdfops wraps the pdfcpu operations Nib performs server-side:
// assembling rasterized pages into a PDF (the guaranteed flatten), protecting
// output with a password, and exporting form data.
package pdfops

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for image.DecodeConfig
	_ "image/png"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/form"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// RasterPage is a rasterized page image plus the point size its PDF page should
// take. The client rasterizes at 2× for crispness, so the image's pixel
// dimensions are double the page; carrying the target size explicitly keeps the
// physical page at its true size instead of letting pdfcpu treat 1px as 1pt.
type RasterPage struct {
	Image []byte
	W, H  float64 // target page size in PDF points
}

// ImagesToPDF builds a PDF with one image per page, each page sized to its
// RasterPage's point dimensions. It is the rasterize path for flatten and image
// export: the client renders each page to a PNG, the server assembles them into a
// guaranteed-flat PDF at the original physical size.
func ImagesToPDF(pages []RasterPage) ([]byte, error) {
	segs := make([][]byte, len(pages))
	for i, p := range pages {
		b, err := imageToPage(p)
		if err != nil {
			return nil, err
		}
		segs[i] = b
	}
	if len(segs) == 1 {
		return segs[0], nil
	}
	readers := make([]io.ReadSeeker, len(segs))
	for i, b := range segs {
		readers[i] = bytes.NewReader(b)
	}
	var out bytes.Buffer
	if err := api.MergeRaw(readers, &out, false, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// imageToPage builds a one-page PDF whose page is exactly p.W×p.H points, with the
// image scaled to fill it. pdfcpu's default Full anchor sizes the page to the
// image's pixel dimensions (the 2× bug); a non-Full anchor with relative scale 1
// instead sizes the page to PageDim and fills it (the image's aspect matches the
// page, since it was rasterized from it).
func imageToPage(p RasterPage) ([]byte, error) {
	imp := &pdfcpu.Import{
		PageDim: &types.Dim{Width: p.W, Height: p.H},
		Pos:     types.Center,
		Scale:   1.0,
		InpUnit: types.POINTS,
	}
	var out bytes.Buffer
	if err := api.ImportImages(nil, &out, []io.Reader{bytes.NewReader(p.Image)}, imp, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Rotate rotates the given pages (e.g. []string{"1","3-5"}, or nil for all) by
// deg degrees (90, 180, 270, or negatives).
func Rotate(pdf []byte, pages []string, deg int) ([]byte, error) {
	var out bytes.Buffer
	if err := api.Rotate(bytes.NewReader(pdf), &out, deg, pages, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RemovePages drops the given pages from the PDF.
func RemovePages(pdf []byte, pages []string) ([]byte, error) {
	var out bytes.Buffer
	if err := api.RemovePages(bytes.NewReader(pdf), &out, pages, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Collect keeps only the given pages, in the exact order given (e.g.
// []string{"3","1","2"} or a range like []string{"2-5"}). It is the single
// page-selection primitive: reordering, deletion-by-omission, and extracting a
// subset into a new PDF all route through it.
func Collect(pdf []byte, order []string) ([]byte, error) {
	var out bytes.Buffer
	if err := api.Collect(bytes.NewReader(pdf), &out, order, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// InsertBlank inserts a blank page immediately after the given page (1-based).
// With a nil page config the blank inherits that page's MediaBox, so the inserted
// sheet matches its neighbour's size.
func InsertBlank(pdf []byte, afterPage int) ([]byte, error) {
	var out bytes.Buffer
	sel := []string{strconv.Itoa(afterPage)}
	if err := api.InsertPages(bytes.NewReader(pdf), &out, sel, false, nil, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// InsertPDF inserts the pages of other immediately BEFORE page (1-based) of pdf.
// Inserting before page 1 prepends other to the document; to add pages at the
// very end use Append. Together with Append this covers every position. There is
// no native pdfcpu positional merge, so it composes via splice (Collect + merge),
// the same split-and-glue replacePage uses.
func InsertPDF(pdf, other []byte, page int) ([]byte, error) {
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}
	if page < 1 || page > n {
		return nil, fmt.Errorf("page %d out of range (1-%d)", page, n)
	}
	return splice(pdf, page-1, page, n, other)
}

// DuplicatePage returns pdf with page (1-based) duplicated in place: the copy is
// inserted immediately after the original. Collect keeps a repeated page in the
// selection, so the selection "1-p" + "p-" emits page p twice; both ranges are
// always non-empty for a valid page, so no edge-guarding is needed.
func DuplicatePage(pdf []byte, page int) ([]byte, error) {
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}
	if page < 1 || page > n {
		return nil, fmt.Errorf("page %d out of range (1-%d)", page, n)
	}
	return Collect(pdf, []string{fmt.Sprintf("1-%d", page), fmt.Sprintf("%d-", page)})
}

// CreateFromJSON renders a brand-new PDF from a pdfcpu "create" JSON page
// description (api.Create with no input document). Nib uses it to generate pages
// from text it controls — currently the co-signing trust-explainer readme — as
// crisp vector text with no extra dependency.
func CreateFromJSON(spec []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := api.Create(nil, bytes.NewReader(spec), &out, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Append concatenates other after pdf (merge).
func Append(pdf, other []byte) ([]byte, error) {
	var out bytes.Buffer
	rs := []io.ReadSeeker{bytes.NewReader(pdf), bytes.NewReader(other)}
	if err := api.MergeRaw(rs, &out, false, nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Combine merges the given PDFs into one, in the order given — each source keeps
// its own pages and page sizes, so the result is their concatenation (which the
// caller can then reorder page by page). A single input is returned unchanged
// (api.MergeRaw needs at least two readers); zero inputs is an error.
func Combine(pdfs [][]byte) ([]byte, error) {
	if len(pdfs) == 0 {
		return nil, fmt.Errorf("no documents to combine")
	}
	if len(pdfs) == 1 {
		return pdfs[0], nil
	}
	readers := make([]io.ReadSeeker, len(pdfs))
	for i, b := range pdfs {
		readers[i] = bytes.NewReader(b)
	}
	var out bytes.Buffer
	if err := api.MergeRaw(readers, &out, false, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// RedactPages rebuilds a PDF so that each page given in raster (1-based page
// number -> a RasterPage: a PNG of that page with the redaction boxes already
// painted in, plus the page's true point size) is replaced by that flat image,
// while every other page is kept as-is. Because a
// redacted page becomes a pure image, the underlying text/content is genuinely
// gone — not merely covered. This is the guaranteed-removal redaction.
func RedactPages(original []byte, raster map[int]RasterPage) ([]byte, error) {
	n, err := PageCount(original)
	if err != nil {
		return nil, err
	}
	segments := make([]io.ReadSeeker, 0, n)
	for i := 1; i <= n; i++ {
		var seg []byte
		if page, ok := raster[i]; ok {
			seg, err = ImagesToPDF([]RasterPage{page})
		} else {
			seg, err = Collect(original, []string{strconv.Itoa(i)}) // extract page i, vector intact
		}
		if err != nil {
			return nil, err
		}
		segments = append(segments, bytes.NewReader(seg))
	}
	var out bytes.Buffer
	if err := api.MergeRaw(segments, &out, false, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// PageCount returns the number of pages in the PDF. (pdfcpu's PageCount path
// dereferences the configuration, so it must be non-nil.)
func PageCount(pdf []byte) (int, error) {
	return api.PageCount(bytes.NewReader(pdf), model.NewDefaultConfiguration())
}

// SplitPart is one output file of a multi-file split (by bookmark or by page
// span): a safe base filename (no directory, no extension) and the PDF bytes.
type SplitPart struct {
	Name string
	Data []byte
}

// SplitByBookmarks splits pdf into one PDF per top-level (level-1) bookmark, in
// page order — each part runs from its bookmark's page to the page before the
// next top-level bookmark, the last to the end of the document. Names are
// sanitize(prefix + bookmark title), deduped. Returns an empty slice (no error)
// when the PDF has no outline. Pages before the first bookmark belong to no part.
//
// Ranges are derived from the sorted PageFrom values rather than pdfcpu's
// per-bookmark PageThru: the read path neither sorts nor enforces page order, and
// the last bookmark's PageThru is left 0, so trusting it is fragile.
func SplitByBookmarks(pdf []byte, prefix string) ([]SplitPart, error) {
	bms, err := api.Bookmarks(bytes.NewReader(pdf), nil)
	if err != nil {
		return nil, err
	}
	if len(bms) == 0 {
		return nil, nil
	}
	sort.SliceStable(bms, func(i, j int) bool { return bms[i].PageFrom < bms[j].PageFrom })
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}

	parts := make([]SplitPart, 0, len(bms))
	seen := map[string]int{}
	for i, bm := range bms {
		from := bm.PageFrom
		if from < 1 {
			continue // a bookmark whose destination didn't resolve
		}
		thru := n
		if i+1 < len(bms) {
			thru = bms[i+1].PageFrom - 1
		}
		if thru < from {
			continue // two bookmarks on the same page → empty span
		}
		data, err := Collect(pdf, []string{fmt.Sprintf("%d-%d", from, thru)})
		if err != nil {
			return nil, err
		}
		parts = append(parts, SplitPart{Name: UniqueName(SanitizeFilename(prefix+bm.Title), len(parts)+1, seen), Data: data})
	}
	return parts, nil
}

// maxSplitParts caps how many files a page-span split may produce, so a tiny
// every-N on a huge document can't spew thousands of files.
const maxSplitParts = 500

// SplitBySpans splits pdf into one file per page-span selection in spans, in
// order — each span is a pdfcpu page selection (e.g. "1-3" or "5"), collected
// into its own PDF. Names are SanitizeFilename(prefix + span), deduped. This is the
// page-sequence splitter (every-N / custom ranges); the geometry splitters are
// SplitPage/SplitRegions, and the bookmark splitter is SplitByBookmarks.
func SplitBySpans(pdf []byte, spans []string, prefix string) ([]SplitPart, error) {
	if len(spans) == 0 {
		return nil, fmt.Errorf("no page ranges given")
	}
	if len(spans) > maxSplitParts {
		return nil, fmt.Errorf("too many output files (%d, max %d)", len(spans), maxSplitParts)
	}
	parts := make([]SplitPart, 0, len(spans))
	seen := map[string]int{}
	for _, span := range spans {
		data, err := Collect(pdf, []string{span})
		if err != nil {
			return nil, fmt.Errorf("range %q: %w", span, err)
		}
		parts = append(parts, SplitPart{Name: UniqueName(SanitizeFilename(prefix+span), len(parts)+1, seen), Data: data})
	}
	return parts, nil
}

// fnameUnsafe matches characters that are illegal or unsafe in a filename on any
// of Linux/macOS/Windows: path separators, control bytes, and Windows-reserved
// punctuation. winReserved matches the Windows device names that can't be files.
var (
	fnameUnsafe = regexp.MustCompile(`[/\\<>:"|?*\x00-\x1f\x7f]`)
	winReserved = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])$`)
)

// SanitizeFilename reduces an arbitrary string (a bookmark title, or a source
// file's base name) to a safe base filename: unsafe characters become spaces and
// collapse, no leading/trailing dots (so no hidden dotfiles and no Windows
// trailing-dot trimming), not a Windows reserved name, capped at a length that
// leaves room for a dedup suffix + ".pdf". Returns "" when nothing usable remains;
// the caller substitutes a fallback.
func SanitizeFilename(s string) string {
	s = fnameUnsafe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace runs, trim ends
	s = strings.Trim(s, ".")                 // no leading/trailing dots
	s = strings.TrimSpace(s)
	if winReserved.MatchString(s) {
		s = "_" + s
	}
	s = truncateBytes(s, 200)
	return strings.TrimRight(s, ". ")
}

// truncateBytes caps s at max bytes without splitting a UTF-8 rune.
func truncateBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

// UniqueName returns a collision-free base name for this batch: an empty name
// becomes "bookmark-<index>", and a repeat (case-insensitively, for macOS/Windows
// filesystems) gets a " (2)", " (3)" suffix.
func UniqueName(base string, index int, seen map[string]int) string {
	if base == "" {
		base = fmt.Sprintf("bookmark-%d", index)
	}
	key := strings.ToLower(base)
	if c := seen[key]; c > 0 {
		seen[key] = c + 1
		return fmt.Sprintf("%s (%d)", base, c+1)
	}
	seen[key] = 1
	return base
}

// NUp places n source pages onto each output sheet (2-up, 4-up, …) in reading
// order, restructuring the whole document in place — the inverse of the imposed-
// page split. n must be one of pdfcpu's NUpValues (2, 3, 4, 6, 8, 9, 12, 16).
// Each sheet keeps the document's own page size (pdfcpu falls back to page 1's
// MediaBox because no paper size is set in the description), so a Letter document
// stays Letter rather than being forced to the A4 default.
func NUp(pdf []byte, n int, border bool) ([]byte, error) {
	conf := model.NewDefaultConfiguration()
	desc := "border:off, margin:0"
	if border {
		desc = "border:on, margin:3"
	}
	nup, err := api.PDFNUpConfig(n, desc, conf)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.NUp(bytes.NewReader(pdf), &out, nil, nil, nup, conf); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// SplitPage splits page p (1-based) of pdf into a cols×rows grid of sub-pages in
// reading order (top-left to bottom-right), replacing that one page with the grid
// in place. The split is rasterization-free: pdfcpu's CutPage re-crops the
// original content stream through per-tile MediaBoxes, so vector/text content
// stays live and only the visible window narrows.
//
// When resize is true each sub-page is scaled up (uniformly, preserving aspect)
// to fill the original page footprint — useful for print/save, where a
// quarter-size MediaBox would otherwise print physically small. We do this by
// hand rather than via api.Resize: CutPage's tiles carry an offset MediaBox, and
// api.Resize scales content about the origin while leaving the box's lower-left
// unscaled, which mis-positions every tile but the origin-anchored one.
func SplitPage(pdf []byte, page, cols, rows int, resize bool) ([]byte, error) {
	if cols < 1 || rows < 1 || cols*rows < 2 {
		return nil, fmt.Errorf("split needs at least 2 sub-pages (got %d×%d)", cols, rows)
	}
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}
	if page < 1 || page > n {
		return nil, fmt.Errorf("page %d out of range (1-%d)", page, n)
	}

	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	// Uniform grid: cut fractions are 0, 1/cols, 2/cols, … (the leading 0 is
	// required when calling CutPage directly; the public api.Cut adds it for you).
	hor := make([]float64, rows)
	for i := range hor {
		hor[i] = float64(i) / float64(rows)
	}
	vert := make([]float64, cols)
	for j := range vert {
		vert[j] = float64(j) / float64(cols)
	}
	ctxDest, err := pdfcpu.CutPage(ctx, page, &model.Cut{Hor: hor, Vert: vert})
	if err != nil {
		return nil, err
	}
	var tilesBuf bytes.Buffer
	if err := api.WriteContext(ctxDest, &tilesBuf); err != nil {
		return nil, err
	}
	// CutPage prepends a full-size outline/preview page; drop it, keep the tiles.
	tiles, err := Collect(tilesBuf.Bytes(), []string{"2-"})
	if err != nil {
		return nil, err
	}
	if resize {
		s := cols
		if rows < s {
			s = rows
		}
		if tiles, err = scaleTiles(tiles, float64(s)); err != nil {
			return nil, err
		}
	}
	return replacePage(pdf, page, n, tiles)
}

// splice returns pdf with pages 1..leftEnd kept, then mid, then pages
// rightStart..n. A side whose range is empty (leftEnd < 1, or rightStart > n) is
// omitted, since api.Collect errors on an empty range. With rightStart =
// leftEnd+2 it drops the single page between the sides (replace it with mid);
// with rightStart = leftEnd+1 it keeps every original page (a pure insert of mid
// at that boundary).
func splice(pdf []byte, leftEnd, rightStart, n int, mid []byte) ([]byte, error) {
	segments := make([][]byte, 0, 3)
	if leftEnd >= 1 {
		left, err := Collect(pdf, []string{fmt.Sprintf("1-%d", leftEnd)})
		if err != nil {
			return nil, err
		}
		segments = append(segments, left)
	}
	segments = append(segments, mid)
	if rightStart <= n {
		right, err := Collect(pdf, []string{fmt.Sprintf("%d-", rightStart)})
		if err != nil {
			return nil, err
		}
		segments = append(segments, right)
	}
	result := segments[0]
	var err error
	for _, seg := range segments[1:] {
		if result, err = Append(result, seg); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// replacePage returns pdf with page p (1-based, of n total) replaced in place by
// the pages of tiles: pages[1..p-1] + tiles + pages[p+1..n].
func replacePage(pdf []byte, page, n int, tiles []byte) ([]byte, error) {
	return splice(pdf, page-1, page+1, n, tiles)
}

const (
	maxRegions  = 64    // cap on hand-drawn split regions per page
	maxRegionPt = 14400 // 200in — the page-dimension ceiling redaction also uses
)

// SplitRegions replaces page p (1-based) with one page per rectangle in rects, in
// order — each new page is the source page's content cropped to that rectangle.
// The page being split is removed (like the grid SplitPage); the other pages are
// left untouched.
//
// Every new page is standardized to ONE size — the largest region's width ×
// height — with smaller regions centred on it and padded with blank space, so the
// output pages are uniform rather than ragged.
//
// Rectangles are in PDF points (bottom-left origin) in the page's DISPLAY space —
// the same space the client (pdf.js) measures in — so the page is first
// normalized (any /Rotate flattened, CropBox resolved) before cropping. Like the
// grid split it is a rasterization-free re-crop; vector/text stays live, and the
// per-region clip keeps neighbouring content from bleeding in.
func SplitRegions(pdf []byte, page int, rects [][4]float64) ([]byte, error) {
	if len(rects) == 0 {
		return nil, fmt.Errorf("no regions selected")
	}
	if len(rects) > maxRegions {
		return nil, fmt.Errorf("too many regions (%d, max %d)", len(rects), maxRegions)
	}
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}
	if page < 1 || page > n {
		return nil, fmt.Errorf("page %d out of range (1-%d)", page, n)
	}

	pageOnly, err := Collect(pdf, []string{strconv.Itoa(page)})
	if err != nil {
		return nil, err
	}
	// Normalize so the client's display-space rectangles map straight onto the
	// content: this bakes any /Rotate into the content and resolves CropBox→
	// MediaBox, leaving a page whose coordinate space matches what pdf.js showed.
	norm, err := normalizePage(pageOnly)
	if err != nil {
		return nil, err
	}

	// Standardize the output: every region becomes a page of the same size — the
	// largest region's width × height — with smaller regions centred and padded.
	var pageW, pageH float64
	for _, r := range rects {
		if w := r[2] - r[0]; w > pageW {
			pageW = w
		}
		if h := r[3] - r[1]; h > pageH {
			pageH = h
		}
	}
	tiles := make([][]byte, 0, len(rects))
	for _, r := range rects {
		tile, err := cropToRect(norm, r, pageW, pageH)
		if err != nil {
			return nil, err
		}
		tiles = append(tiles, tile)
	}
	merged := tiles[0]
	for _, t := range tiles[1:] {
		if merged, err = Append(merged, t); err != nil {
			return nil, err
		}
	}
	return replacePage(pdf, page, n, merged)
}

// Crop reduces each target page (pages: a 1-based selection like []string{"2"};
// nil = every page) to frac — the keep-box as page FRACTIONS, top-left origin:
// [fx, fy, fw, fh] where (fx,fy) is the box's top-left corner and (fw,fh) its
// width/height, each in 0..1. Fractions (not absolute points) so that one drawn
// box maps to the same RELATIVE region on every page regardless of its size: on a
// uniform document this is identical to the old absolute behaviour, and on a
// mixed-size document each page is cropped proportionally instead of to a fixed
// physical window that could fall off a smaller page. Each target page is
// normalized (any /Rotate flattened, CropBox resolved) so the box maps straight
// onto the page pdf.js showed, then reduced to a page the exact size of the
// resolved window; pages outside the selection are kept untouched.
//
// Content outside the rectangle is clipped away — hidden, not destroyed (the
// objects remain in the file behind the new, smaller MediaBox); Flatten or Redact
// remove it permanently.
//
// It is a single O(N) pass: SplitRaw reads and splits the document once, each page
// is cropped in isolation, and the pages are merged once — there is no per-page
// re-Collect of the whole document (which would make it O(N²)).
func Crop(pdf []byte, frac [4]float64, pages []string) ([]byte, error) {
	if frac[2] <= 0 || frac[3] <= 0 {
		return nil, fmt.Errorf("crop region too small")
	}
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}
	targets, err := api.PagesForPageSelection(n, pages, true, false)
	if err != nil {
		return nil, err
	}

	spans, err := api.SplitRaw(bytes.NewReader(pdf), 1, model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	segs := make([][]byte, 0, len(spans))
	for _, ps := range spans {
		page, err := io.ReadAll(ps.Reader)
		if err != nil {
			return nil, err
		}
		if targets[ps.From] {
			norm, err := normalizePage(page)
			if err != nil {
				return nil, err
			}
			// Resolve the fraction box against THIS page's own display box, so
			// differently-sized pages each keep the same relative window.
			rect, w, h, err := resolveFracRect(norm, frac)
			if err != nil {
				return nil, err
			}
			// pageW=w, pageH=h means cropToRect centres with zero padding, so the
			// output page is exactly the crop window — no standardization to a
			// larger uniform size as SplitRegions does for ragged regions.
			if page, err = cropToRect(norm, rect, w, h); err != nil {
				return nil, err
			}
		}
		segs = append(segs, page)
	}
	if len(segs) == 1 {
		return segs[0], nil // MergeRaw needs ≥2 inputs; a one-page doc is already done
	}
	readers := make([]io.ReadSeeker, len(segs))
	for i, b := range segs {
		readers[i] = bytes.NewReader(b)
	}
	var out bytes.Buffer
	if err := api.MergeRaw(readers, &out, false, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// normalizePage flattens a single-page PDF's /Rotate into its content and
// resolves CropBox→MediaBox by running it through CutPage as a trivial 1×1 cut
// (which already does both), then dropping CutPage's outline page. The result is
// one page in display orientation, so the client's display-space coordinates map
// onto it directly.
func normalizePage(pdf []byte) ([]byte, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	ctxDest, err := pdfcpu.CutPage(ctx, 1, &model.Cut{Hor: []float64{0}, Vert: []float64{0}})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctxDest, &buf); err != nil {
		return nil, err
	}
	return Collect(buf.Bytes(), []string{"2-"}) // drop CutPage's outline page
}

// resolveFracRect converts a top-left-origin fraction box [fx, fy, fw, fh] (0..1)
// into the absolute source rectangle cropToRect expects: PDF points, bottom-left
// origin, relative to normPage's display-box origin (cropToRect adds the box's
// lower-left itself). normPage must already be rotation-flattened, so its MediaBox
// dimensions match the rotation-aware viewport the client measured the fractions
// against — making this identical to the old absolute path on a uniform document.
// It also returns the resolved window's width and height.
func resolveFracRect(normPage []byte, frac [4]float64) (rect [4]float64, w, h float64, err error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(normPage), model.NewDefaultConfiguration())
	if err != nil {
		return rect, 0, 0, err
	}
	_, _, attrs, err := ctx.PageDict(1, false)
	if err != nil {
		return rect, 0, 0, err
	}
	bw, bh := attrs.MediaBox.Width(), attrs.MediaBox.Height()
	fx, fy, fw, fh := frac[0], frac[1], frac[2], frac[3]
	// Flip the top-left-origin fractions to bottom-left points within the box.
	rect = [4]float64{fx * bw, (1 - (fy + fh)) * bh, (fx + fw) * bw, (1 - fy) * bh}
	return rect, rect[2] - rect[0], rect[3] - rect[1], nil
}

// cropToRect returns a one-page PDF: normPage cropped to rect (PDF points,
// bottom-left origin, relative to the page's display-box origin), centred at its
// native scale on a standardized pageW×pageH page (padded with blank space so
// every region shares one output size). normPage must already be rotation-
// flattened (see normalizePage).
func cropToRect(normPage []byte, rect [4]float64, pageW, pageH float64) ([]byte, error) {
	w, h := rect[2]-rect[0], rect[3]-rect[1]
	if w <= 1 || h <= 1 {
		return nil, fmt.Errorf("region too small")
	}
	if w > maxRegionPt || h > maxRegionPt {
		return nil, fmt.Errorf("region too large")
	}
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(normPage), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	d, _, attrs, err := ctx.PageDict(1, false)
	if err != nil {
		return nil, err
	}
	// The client's coordinates are relative to the display-box origin; add the
	// box's lower-left so an offset (e.g. cropped) page still maps correctly.
	// Centre the region on the standardized page.
	mb := attrs.MediaBox
	dx, dy := (pageW-w)/2, (pageH-h)/2
	if err := wrapPageToBox(ctx, d, 1, mb.LL.X+rect[0], mb.LL.Y+rect[1], w, h, 1.0, pageW, pageH, dx, dy); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// wrapPageToBox rewrites page pageNr in ctx so it becomes a pageW×pageH page
// showing only the source rectangle (x0,y0)–(x0+w,y0+h), scaled by s and placed
// with its lower-left at (dx,dy). It clips to the destination rectangle and maps
// the source sub-rectangle onto it; the explicit clip stops anything outside the
// rectangle (a neighbouring tile/region) from bleeding in, and any area of the
// page not covered by the placement stays blank (white) — the padding that lets
// differently-sized regions share one standardized page size. (x0,y0,w,h) are in
// the page's own content coordinates.
func wrapPageToBox(ctx *model.Context, d types.Dict, pageNr int, x0, y0, w, h, s, pageW, pageH, dx, dy float64) error {
	content, err := ctx.PageContent(d, pageNr)
	if err != nil && err != model.ErrNoContent {
		return err
	}
	if err == nil {
		var buf bytes.Buffer
		// Clip to the destination rect, then scale by s and translate so source
		// (x0,y0) lands at (dx,dy): point (x,y) -> (s*x + dx - s*x0, s*y + dy - s*y0).
		fmt.Fprintf(&buf, "q %.5f %.5f %.5f %.5f re W n %.5f 0 0 %.5f %.5f %.5f cm ",
			dx, dy, w*s, h*s, s, s, dx-s*x0, dy-s*y0)
		buf.Write(content)
		buf.WriteString(" Q")
		sd, err := ctx.NewStreamDictForBuf(buf.Bytes())
		if err != nil {
			return err
		}
		if err := sd.Encode(); err != nil {
			return err
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return err
		}
		d["Contents"] = *ref
	}
	box := types.NewRectangle(0, 0, pageW, pageH)
	d.Update("MediaBox", box.Array())
	d.Delete("CropBox")
	return nil
}

// scaleTiles scales every page of the tiles PDF up by factor s, preserving each
// page's visible region and normalizing it to start at the origin. Each tile's
// content is the original page's content stream behind an offset MediaBox; the
// shared wrapPageToBox re-crop maps the tile's whole MediaBox onto an s× page.
func scaleTiles(tiles []byte, s float64) ([]byte, error) {
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(tiles), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
	for i := 1; i <= ctx.PageCount; i++ {
		d, _, attrs, err := ctx.PageDict(i, false)
		if err != nil {
			return nil, err
		}
		mb := attrs.MediaBox
		w, h := mb.Width(), mb.Height()
		if err := wrapPageToBox(ctx, d, i, mb.LL.X, mb.LL.Y, w, h, s, w*s, h*s, 0, 0); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	if err := api.WriteContext(ctx, &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Field is a filled overlay field to stamp onto the page: the text (already "X"
// for a checked box) and its rectangle in PDF points (bottom-left origin). Font,
// Size and Color are optional cover-and-replace overrides; left zero they keep
// the historical auto-detected-field behaviour (Helvetica, size from the rect
// height, black), so detected form fields are unaffected.
type Field struct {
	Page  int        `json:"page"`
	Rect  [4]float64 `json:"rect"` // x0, y0, x1, y1
	Text  string     `json:"text"`
	Font  string     `json:"font,omitempty"`  // a Base-14 core font name; defaults to Helvetica
	Size  float64    `json:"size,omitempty"`  // explicit point size; 0 = derive from rect height
	Color string     `json:"color,omitempty"` // #RRGGBB; "" = black
}

// coreFonts is the Base-14 text-font allowlist StampFields will honour. The name
// is interpolated into the pdfcpu watermark description, so an unlisted value
// falls back to Helvetica rather than risk injecting extra description keys.
var coreFonts = map[string]bool{
	"Helvetica": true, "Helvetica-Bold": true, "Helvetica-Oblique": true, "Helvetica-BoldOblique": true,
	"Times-Roman": true, "Times-Bold": true, "Times-Italic": true, "Times-BoldItalic": true,
	"Courier": true, "Courier-Bold": true, "Courier-Oblique": true, "Courier-BoldOblique": true,
}

func coreFont(name string) string {
	if coreFonts[name] {
		return name
	}
	return "Helvetica"
}

// StampFields bakes the given overlay-field values onto the PDF as text, sized to
// each field's rectangle. All fields are applied in a single pass.
func StampFields(pdf []byte, fields []Field) ([]byte, error) {
	wms := map[int][]*model.Watermark{}
	for _, f := range fields {
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		// An explicit Size (cover-and-replace edit) is honoured up to 144pt; with
		// none, size is derived from the rect height as detected fields always have,
		// capped at 48 so a tall detected box doesn't balloon the text.
		pts, max := int(f.Size), 144
		if pts <= 0 {
			pts, max = int((f.Rect[3]-f.Rect[1])*0.72), 48
		}
		if pts < 6 {
			pts = 8
		}
		if pts > max {
			pts = max
		}
		color := "#000000"
		if hexColor.MatchString(f.Color) {
			color = f.Color
		}
		// Anchor the text near the field's bottom-left (small inset), in points.
		desc := fmt.Sprintf("fontname:%s, points:%d, scalefactor:1 abs, position:bl, offset:%.1f %.1f, fillcolor:%s, rotation:0",
			coreFont(f.Font), pts, f.Rect[0]+2, f.Rect[1]+2, color)
		wm, err := api.TextWatermark(f.Text, desc, true, false, types.POINTS)
		if err != nil {
			return nil, err
		}
		page := f.Page
		if page < 1 {
			page = 1
		}
		wms[page] = append(wms[page], wm)
	}
	if len(wms) == 0 {
		return pdf, nil
	}
	var out bytes.Buffer
	if err := api.AddWatermarksSliceMap(bytes.NewReader(pdf), &out, wms, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Stamp is an image to bake onto the page at a given rectangle (PDF points,
// bottom-left origin): a chosen signature image, or a drawn Y/N circle. The
// image is scaled to fill the rectangle's width; callers render the PNG at the
// rectangle's aspect ratio so it sits correctly on the line/letter.
type Stamp struct {
	Page int        `json:"page"`
	Rect [4]float64 `json:"rect"` // x0, y0, x1, y1
	PNG  []byte     `json:"-"`    // resolved image bytes (PNG or JPEG)
}

// StampImages bakes the given image stamps onto the PDF as image watermarks,
// each scaled and positioned to its rectangle. Applied in a single pass on top
// of any existing content.
func StampImages(pdf []byte, stamps []Stamp) ([]byte, error) {
	wms := map[int][]*model.Watermark{}
	for _, s := range stamps {
		if len(s.PNG) == 0 {
			continue
		}
		boxW := s.Rect[2] - s.Rect[0]
		boxH := s.Rect[3] - s.Rect[1]
		if boxW <= 0 || boxH <= 0 {
			continue
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(s.PNG))
		if err != nil || cfg.Width == 0 || cfg.Height == 0 {
			return nil, fmt.Errorf("decode stamp image: %w", err)
		}
		// pdfcpu sizes an image watermark as its pixel dimensions (treated as
		// points) times the absolute scale factor. Fit the image inside the box
		// preserving aspect (so a wide signature sits at line height rather than
		// ballooning to the full underline width); position:bl anchors the
		// image's bottom-left corner at the box's bottom-left.
		scale := boxW / float64(cfg.Width)
		if sy := boxH / float64(cfg.Height); sy < scale {
			scale = sy
		}
		desc := fmt.Sprintf("position:bl, offset:%.2f %.2f, scalefactor:%.5f abs, rotation:0", s.Rect[0], s.Rect[1], scale)
		wm, err := api.ImageWatermarkForReader(bytes.NewReader(s.PNG), desc, true, false, types.POINTS)
		if err != nil {
			return nil, err
		}
		page := s.Page
		if page < 1 {
			page = 1
		}
		wms[page] = append(wms[page], wm)
	}
	if len(wms) == 0 {
		return pdf, nil
	}
	var out bytes.Buffer
	if err := api.AddWatermarksSliceMap(bytes.NewReader(pdf), &out, wms, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// WatermarkStyle controls how StampWatermark renders the label.
type WatermarkStyle struct {
	Color   string  `json:"color"`   // #RRGGBB
	Opacity float64 `json:"opacity"` // 0..1
	Scale   float64 `json:"scale"`   // page-relative size, 0..1
	Angle   int     `json:"angle"`   // degrees
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// sanitize clamps the style to safe ranges and a valid hex colour. The colour
// and numbers are interpolated into the pdfcpu watermark description string, so
// this is what stops a crafted value from injecting extra description keys.
func (s WatermarkStyle) sanitize() WatermarkStyle {
	if !hexColor.MatchString(s.Color) {
		s.Color = "#8a8a8a"
	}
	s.Opacity = clampUnit(s.Opacity, 0.02, 1)
	s.Scale = clampUnit(s.Scale, 0.1, 1)
	switch {
	case s.Angle < -90:
		s.Angle = -90
	case s.Angle > 90:
		s.Angle = 90
	}
	return s
}

func clampUnit(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// StampWatermark draws text as a large diagonal watermark across every page,
// styled by st. Finalize applies it before signing so the certification covers
// the watermark.
func StampWatermark(pdf []byte, text string, st WatermarkStyle) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return pdf, nil
	}
	st = st.sanitize()
	desc := fmt.Sprintf("fontname:Helvetica, scalefactor:%.3f rel, fillcolor:%s, opacity:%.3f, rotation:%d, position:c",
		st.Scale, st.Color, st.Opacity, st.Angle)
	// Always drawn on top: a "behind" watermark is hidden by any page with an
	// opaque background, inconsistently per page — opacity gives the subtle look.
	wm, err := api.TextWatermark(text, desc, true, false, types.POINTS)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := api.AddWatermarks(bytes.NewReader(pdf), &out, nil, wm, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// pageNumPositions is the corner allowlist for StampPageNumbers. The code is
// interpolated into the pdfcpu watermark description, so an unlisted value must
// never reach it (same guard rationale as coreFonts).
var pageNumPositions = map[string]bool{
	"tl": true, "tc": true, "tr": true,
	"bl": true, "bc": true, "br": true,
}

// PageNumberStyle controls StampPageNumbers.
type PageNumberStyle struct {
	Position string // pdfcpu corner code: tl/tc/tr/bl/bc/br ("" = bc)
	Prefix   string // text before the number, e.g. "Page " or a Bates prefix
	Start    int    // number printed on the first page (default 1)
	Pad      int    // zero-pad width, 0 = none (e.g. 6 → 000123 for Bates)
	OfTotal  bool   // append " of N" (N = the number on the last page)
	Total    int    // override for OfTotal's N (0 = this file's last number); a
	// continuous multi-file run sets it to the set's grand-total last number
	Size  int    // point size (default 11)
	Color string // #RRGGBB ("" = black)
}

// pageNumOffset insets the number from the chosen corner so it doesn't sit flush
// against the page edge (points): horizontally for left/right, vertically for
// top/bottom; centres get no horizontal nudge.
func pageNumOffset(pos string) (x, y float64) {
	const hx, vy = 40.0, 24.0
	switch pos[1] {
	case 'l':
		x = hx
	case 'r':
		x = -hx
	}
	switch pos[0] {
	case 't':
		y = -vy
	case 'b':
		y = vy
	}
	return x, y
}

// StampPageNumbers bakes a running page number onto every page at a chosen
// corner. Each page's label is formatted here (prefix + optional zero-pad + start
// offset) and stamped as its own text watermark via the same SliceMap path as
// StampFields, so it covers Bates numbering as well as plain "Page n of N".
func StampPageNumbers(pdf []byte, st PageNumberStyle) ([]byte, error) {
	n, err := PageCount(pdf)
	if err != nil {
		return nil, err
	}
	pos := st.Position
	if !pageNumPositions[pos] {
		pos = "bc"
	}
	size := st.Size
	if size < 6 {
		size = 11
	}
	if size > 72 {
		size = 72
	}
	color := "#000000"
	if hexColor.MatchString(st.Color) {
		color = st.Color
	}
	start := st.Start
	if start < 1 {
		start = 1
	}
	pad := st.Pad
	if pad < 0 {
		pad = 0
	}
	if pad > 12 {
		pad = 12
	}
	// A '%' in the prefix would be read as a pdfcpu placeholder token (%p/%P/…),
	// so strip it — the number itself is formatted here, not by pdfcpu.
	prefix := strings.ReplaceAll(st.Prefix, "%", "")
	ox, oy := pageNumOffset(pos)

	wms := map[int][]*model.Watermark{}
	for i := 1; i <= n; i++ {
		label := fmt.Sprintf("%s%0*d", prefix, pad, start+i-1)
		if st.OfTotal {
			last := start + n - 1 // this file's last number…
			if st.Total > 0 {
				last = st.Total // …or the set's grand total in a continuous run
			}
			label += " of " + strconv.Itoa(last)
		}
		desc := fmt.Sprintf("fontname:Helvetica, points:%d, scalefactor:1 abs, position:%s, offset:%.1f %.1f, fillcolor:%s, rotation:0",
			size, pos, ox, oy, color)
		wm, err := api.TextWatermark(label, desc, true, false, types.POINTS)
		if err != nil {
			return nil, err
		}
		wms[i] = append(wms[i], wm)
	}
	var out bytes.Buffer
	if err := api.AddWatermarksSliceMap(bytes.NewReader(pdf), &out, wms, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ExportFormJSON returns the form field data of pdf as pdfcpu's JSON.
func ExportFormJSON(pdf []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := api.ExportFormJSON(bytes.NewReader(pdf), &out, "nib", nil); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ExportFormCSV returns the form field data as two columns: field name and value.
// It reuses pdfcpu's own JSON export (the single source of truth) and reshapes it.
func ExportFormCSV(pdf []byte) ([]byte, error) {
	jsonData, err := ExportFormJSON(pdf)
	if err != nil {
		return nil, err
	}
	var fg form.FormGroup
	if err := json.Unmarshal(jsonData, &fg); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"field", "value"})
	for _, f := range fg.Forms {
		for _, x := range f.TextFields {
			_ = cw.Write([]string{x.Name, x.Value})
		}
		for _, x := range f.DateFields {
			_ = cw.Write([]string{x.Name, x.Value})
		}
		for _, x := range f.CheckBoxes {
			_ = cw.Write([]string{x.Name, strconv.FormatBool(x.Value)})
		}
		for _, x := range f.RadioButtonGroups {
			_ = cw.Write([]string{x.Name, x.Value})
		}
		for _, x := range f.ComboBoxes {
			_ = cw.Write([]string{x.Name, x.Value})
		}
		for _, x := range f.ListBoxes {
			_ = cw.Write([]string{x.Name, joinValues(x.Values)})
		}
	}
	cw.Flush()
	return buf.Bytes(), cw.Error()
}

func joinValues(vs []string) string { return strings.Join(vs, "; ") }

// ExtractImagesZip extracts the embedded raster images from pdf and returns them
// bundled as a ZIP, plus the number of images written. The bytes come straight
// from pdfcpu: a non-CMYK JPEG or a JPEG-2000 stream is the original embedded
// data (.jpg / .jpx), while every other kind (Flate/LZW/CCITT/raw rasters and
// CMYK JPEGs) is re-encoded by pdfcpu to PNG or TIFF — so this is "the images as
// usable files", not always the byte-for-byte originals. Images pdfcpu can't
// render (JBIG2, exotic colorspaces) and inline images are skipped rather than
// written as empty files. An image referenced on several pages is written once
// (deduped by object number). Returns (nil, 0, nil) when there are none.
//
// pdfcpu aborts the extraction on the first image it can't render, so a single
// bad image would otherwise lose the whole archive. The fast path tries the
// document in one pass; if that fails, it re-runs page by page so one bad image
// only loses its own page's images, not the rest.
func ExtractImagesZip(pdf []byte) ([]byte, int, error) {
	if data, count, err := extractImages(pdf, false); err == nil {
		return data, count, nil
	}
	return extractImages(pdf, true)
}

// extractImages builds the image ZIP. With perPage false it runs a single
// api.ExtractImages over the whole document (fast, but one bad image aborts it).
// With perPage true it runs one pass per page, recovering each so a page whose
// image pdfcpu can't handle is skipped instead of sinking the whole archive.
func extractImages(pdf []byte, perPage bool) ([]byte, int, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	seen := map[int]bool{}
	count := 0
	digest := func(img model.Image, _ bool, _ int) error {
		if img.Reader == nil || img.FileType == "" || seen[img.ObjNr] {
			return nil // unsupported/empty render, or a repeat of one already written
		}
		seen[img.ObjNr] = true
		fw, err := zw.Create(fmt.Sprintf("image-%03d-obj%d.%s", count+1, img.ObjNr, img.FileType))
		if err != nil {
			return err
		}
		if _, err := io.Copy(fw, img); err != nil {
			return err
		}
		count++
		return nil
	}
	// run does one extraction pass over a page selection (nil = whole doc). The
	// recover is load-bearing: api.ExtractImages only catches pdfcpu's own faults
	// and re-panics generic ones (a missing /BitsPerComponent, etc.), which would
	// otherwise crash the request.
	run := func(pages []string) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("image extraction panicked: %v", r)
			}
		}()
		return api.ExtractImages(bytes.NewReader(pdf), pages, digest, model.NewDefaultConfiguration())
	}

	if !perPage {
		if err := run(nil); err != nil {
			return nil, 0, err
		}
	} else {
		n, err := PageCount(pdf)
		if err != nil {
			return nil, 0, err
		}
		for p := 1; p <= n; p++ {
			_ = run([]string{strconv.Itoa(p)}) // skip a page whose image can't be extracted
		}
	}

	if count == 0 {
		return nil, 0, nil
	}
	if err := zw.Close(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), count, nil
}

// Optimize losslessly shrinks a PDF: pdfcpu deduplicates repeated embedded fonts
// and images and repacks objects into compressed object/xref streams. It does NOT
// touch image resolution or quality — content and text render identically — so the
// win is large on bloated/redundant PDFs (duplicated resources, old-style xref)
// and ~nil on already-tight or image-dominated files. For real shrinkage of a
// scan, the caller rasterizes to JPEG instead.
func Optimize(pdf []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := api.Optimize(bytes.NewReader(pdf), &out, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
