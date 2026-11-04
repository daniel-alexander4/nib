package pdfops

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"

	"nib/internal/pdfread"
	"nib/internal/ziputil"
)

// The raster producers (PLAN-import-export P01.S03a, ADR-187): GIF, BMP, a TIFF with more than one page, and a
// comic-book archive (CBZ) — every Group 1 import whose output is one page per picture. They are native: no
// outside tool, so they never wait at the gate (importdoor.go), which is why each carries its own caps and its
// own wall-clock budget.
//
// # One job, one budget
//
// A `rasterJob` is one import: it takes pictures a page at a time, charges each against a running budget
// BEFORE decoding it (Y6 — a header says what a decode would cost, and the refusal then costs the header), turns
// the page into a one-page PDF at once and lets the pixels go. Peak memory is one decoded page plus the output
// so far, not the document.
//
// A cap REFUSES. It never truncates: a 1,001-page file that opened as 1,000 would lose a page without saying.

// The caps (D14). PROVISIONAL starting values — P01.S03b measures a run for each producer and tunes them.
const (
	// importPages is the most pages a TIFF, GIF or CBZ may become.
	importPages = 1000
	// importTotalPixels is the most pixels, summed over every page of one import, Nib will decode.
	importTotalPixels = 1_000_000_000
	// importCBZEntryBytes is the most one picture inside a CBZ may be.
	importCBZEntryBytes = 64 << 20
	// cbzSniffBytes is the prefix of a CBZ member that sniffImage needs (the longest signature test is a BMP's 26).
	cbzSniffBytes = 64
	// nativeImportTimeout is a native producer's wall-clock cap, checked between pages.
	nativeImportTimeout = 2 * time.Minute
)

// The refusals. Each is a sentence a person can act on; refusalCause maps each to a counter label.
var (
	// ErrImagePixels: a picture, or the pictures of one import together, past the pixel caps.
	ErrImagePixels = errors.New("that is more picture than Nib opens")
	// ErrTooManyPages: more pages than importPages.
	ErrTooManyPages = errors.New("that has more pages than Nib opens")
	// ErrNoPages: an archive or file with nothing Nib can show.
	ErrNoPages = errors.New("that holds no pictures Nib can open")
	// ErrTIFFCompression: a TIFF page compressed in a way Nib has no decoder for (JPEG-in-TIFF, JPEG 2000).
	ErrTIFFCompression = errors.New("that TIFF is compressed in a way Nib cannot read")
)

// errTIFFLoop: a directory chain that returns to a directory it has read. Ending the walk there would open the
// file with the pages after the loop missing and say nothing.
var errTIFFLoop = errors.New("pdfops: this tiff could not be read: its page directories loop back on themselves")

// The density a file may declare. A page is px/dpi*72 points: a file saying 1 dpi makes a 72-pt-per-pixel
// page no viewer lays out, and one saying 100,000 a speck. A density outside the range is a guess a writer
// made badly, not a size anyone meant, so it reads as "unknown" and the page takes defaultDPI — the same
// answer a file with no density gets. (Clamping to the edge would invent a size just as wrong.)
const (
	minImportDPI = 10.0
	maxImportDPI = 2400.0
)

// usableDPI is dpi when it is a density Nib will size a page by, else 0 (unknown).
func usableDPI(dpi float64) float64 {
	if dpi < minImportDPI || dpi > maxImportDPI || dpi != dpi {
		return 0
	}
	return dpi
}

// rasterJob is one import in progress.
type rasterJob struct {
	ctx    context.Context
	segs   [][]byte
	out    int   // bytes of PDF so far, against maxConvertedBytes
	pixels int64 // everything charged so far — decoded pages and a GIF's held frames alike — against importTotalPixels
}

func newRasterJob(ctx context.Context) (*rasterJob, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, nativeImportTimeout)
	return &rasterJob{ctx: ctx}, cancel
}

// charge books w×h against the budget before a decode. A single picture over maxImagePixels is refused with the
// existing sentence's cap; the running total is the import's.
func (j *rasterJob) charge(w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("pdfops: this picture declares a %d×%d image, which has no size", w, h)
	}
	px := int64(w) * int64(h)
	if px > maxImagePixels {
		return fmt.Errorf("%w: it is %d×%d — %d megapixels, and Nib opens images up to %d",
			ErrImagePixels, w, h, px/1_000_000, maxImagePixels/1_000_000)
	}
	if j.pixels+px > importTotalPixels {
		return fmt.Errorf("%w: all together past %d megapixels", ErrImagePixels, importTotalPixels/1_000_000)
	}
	j.pixels += px
	return nil
}

// importPixelBytes caps one picture's DECODED size in bytes: the pixel cap (200 Mpx) at the usual 4 bytes a pixel.
// A 16-bit RGBA or CMYK picture is 8 and 4 bytes a pixel, so a pixel count alone lets one page cost twice that.
const importPixelBytes = int64(maxImagePixels) * 4

// bytesPerPixel is what the decoder allocates for each pixel of a picture in this colour model.
func bytesPerPixel(m color.Model) int64 {
	switch m {
	case color.GrayModel:
		return 1
	case color.Gray16Model:
		return 2
	case color.RGBA64Model, color.NRGBA64Model:
		return 8
	}
	return 4 // RGBA, NRGBA, CMYK, paletted-as-RGBA
}

func (j *rasterJob) chargeBytes(w, h int, bpp int64) error {
	if int64(w)*int64(h)*bpp > importPixelBytes {
		return fmt.Errorf("%w: it is %d×%d at %d bytes a pixel, and Nib opens pictures up to %d MiB decoded", ErrImagePixels, w, h, bpp, importPixelBytes>>20)
	}
	return nil
}

// page turns one picture into a page now and keeps only the page.
func (j *rasterJob) page(p RasterPage, rotate int) error {
	if err := j.ctx.Err(); err != nil {
		return err
	}
	if len(j.segs) >= importPages {
		return fmt.Errorf("%w: Nib opens up to %d", ErrTooManyPages, importPages)
	}
	seg, err := ImagesToPDF([]RasterPage{p}) // one page: the primitive returns it unmerged
	if err != nil {
		return fmt.Errorf("pdfops: a picture could not be made into a page: %w", err)
	}
	if rotate != 0 {
		if seg, err = Rotate(seg, nil, rotate); err != nil {
			return fmt.Errorf("pdfops: a picture's orientation could not be applied: %w", err)
		}
	}
	j.out += len(seg)
	if j.out > maxConvertedBytes {
		return ErrConvertedTooLarge
	}
	j.segs = append(j.segs, seg)
	return nil
}

func (j *rasterJob) finish(name string) ([]byte, error) {
	if len(j.segs) == 0 {
		return nil, ErrNoPages
	}
	var out []byte
	if len(j.segs) == 1 {
		out = j.segs[0]
	} else {
		var err error
		if out, err = pdfread.MergeRaw(j.segs); err != nil {
			return nil, err
		}
	}
	if titled, terr := TitleFromName(out, name); terr == nil {
		out = titled
	}
	return out, nil
}

// addFile adds every page one picture file holds.
func (j *rasterJob) addFile(data []byte) error {
	format, ok := sniffImage(data)
	if !ok {
		return ErrNotAnImage
	}
	switch format {
	case formatGIF:
		return j.addGIF(data)
	case formatBMP:
		return j.addBMP(data)
	case formatTIFF:
		ifds, err := walkTIFF(data)
		switch {
		case err != nil && (len(ifds) > 0 || errors.Is(err, ErrTooManyPages)):
			return err // a chain that began and broke: opening its first page would lose the rest without a word
		case err == nil && (len(ifds) > 1 || ifds[0].off != tiffFirstOffset(data)):
			// More than one page — or one real page behind a thumbnail the walk dropped: the decoder reads the FIRST
			// directory, which would be the thumbnail.
			return j.addTIFF(data, ifds)
		}
		// One directory — or a first one this walk cannot read, which the decoder then judges as it always has: the
		// walk is stricter than the decoder and must not turn a file that opened into a refusal.
	}
	return j.addPlain(data, format)
}

// addPlain is a PNG, JPEG, WebP or one-page TIFF: the bytes go to the page as they are.
func (j *rasterJob) addPlain(data []byte, format imageFormat) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("pdfops: this %s could not be read: %w", format, err)
	}
	if err := j.charge(cfg.Width, cfg.Height); err != nil {
		return err
	}
	dpi := 0.0
	switch format {
	case formatPNG:
		dpi = pngDPI(data)
	case formatJPEG:
		dpi = jpegDPI(data)
	}
	if dpi = usableDPI(dpi); dpi == 0 {
		dpi = defaultDPI
	}
	rot := 0
	if format == formatJPEG {
		if deg, expressible := rotationFor(jpegOrientation(data)); expressible {
			rot = deg
		}
	}
	return j.page(RasterPage{Image: data, W: float64(cfg.Width) / dpi * 72, H: float64(cfg.Height) / dpi * 72}, rot)
}

// emitImage encodes a decoded picture as a PNG and makes its page.
func (j *rasterJob) emitImage(img image.Image, dpi float64) error {
	if dpi = usableDPI(dpi); dpi == 0 {
		dpi = defaultDPI
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return err
	}
	b := img.Bounds()
	return j.page(RasterPage{Image: buf.Bytes(), W: float64(b.Dx()) / dpi * 72, H: float64(b.Dy()) / dpi * 72}, 0)
}

// ---- BMP ----------------------------------------------------------------------------------------------------

// looksLikeBMP: "BM", the two reserved words zero, and a DIB header size a BMP has. The signature is two
// letters, so the rest is what stops a text file that begins "BM" being called a picture.
func looksLikeBMP(data []byte) bool {
	if len(data) < 26 || data[0] != 'B' || data[1] != 'M' {
		return false
	}
	if binary.LittleEndian.Uint32(data[6:10]) != 0 {
		return false
	}
	switch binary.LittleEndian.Uint32(data[14:18]) {
	case 12, 40, 52, 56, 64, 108, 124:
		return true
	}
	return false
}

func bmpDPI(data []byte) float64 {
	if len(data) < 46 || binary.LittleEndian.Uint32(data[14:18]) < 40 {
		return 0
	}
	if ppm := int32(binary.LittleEndian.Uint32(data[38:42])); ppm > 0 {
		return float64(ppm) * 0.0254
	}
	return 0
}

func (j *rasterJob) addBMP(data []byte) error {
	cfg, err := bmp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("pdfops: this bmp could not be read: %w", err)
	}
	if err := j.charge(cfg.Width, cfg.Height); err != nil {
		return err
	}
	img, err := bmp.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("pdfops: this bmp could not be read: %w", err)
	}
	return j.emitImage(img, bmpDPI(data))
}

// ---- GIF ----------------------------------------------------------------------------------------------------

// gifShape walks a GIF's blocks WITHOUT decoding them and returns the logical screen and each frame's size, so
// the budget is charged before gif.DecodeAll holds a single pixel. A frame is an image descriptor (0x2C).
func gifShape(data []byte) (sw, sh int, frames [][2]int, err error) {
	bad := errors.New("pdfops: this gif could not be read: its blocks do not end where they should")
	if len(data) < 13 {
		return 0, 0, nil, bad
	}
	sw = int(binary.LittleEndian.Uint16(data[6:8]))
	sh = int(binary.LittleEndian.Uint16(data[8:10]))
	p := 13
	if data[10]&0x80 != 0 {
		p += 3 * (1 << (uint(data[10]&7) + 1))
	}
	skipSub := func() bool { // data sub-blocks up to the 0 terminator
		for p < len(data) {
			n := int(data[p])
			p++
			if n == 0 {
				return true
			}
			p += n
		}
		return false
	}
	for p < len(data) {
		switch data[p] {
		case 0x3B:
			return sw, sh, frames, nil
		case 0x21:
			p += 2
			if !skipSub() {
				return 0, 0, nil, bad
			}
		case 0x2C:
			if p+10 > len(data) {
				return 0, 0, nil, bad
			}
			w := int(binary.LittleEndian.Uint16(data[p+5 : p+7]))
			h := int(binary.LittleEndian.Uint16(data[p+7 : p+9]))
			flags := data[p+9]
			p += 10
			if flags&0x80 != 0 {
				p += 3 * (1 << (uint(flags&7) + 1))
			}
			p++ // LZW minimum code size
			if !skipSub() {
				return 0, 0, nil, bad
			}
			frames = append(frames, [2]int{w, h})
			if len(frames) > importPages {
				return 0, 0, nil, fmt.Errorf("%w: Nib opens up to %d frames", ErrTooManyPages, importPages)
			}
		default:
			return 0, 0, nil, bad
		}
	}
	return sw, sh, frames, nil // a missing trailer is tolerated: the decoder decides
}

func (j *rasterJob) addGIF(data []byte) error {
	sw, sh, frames, err := gifShape(data)
	if err != nil {
		return err
	}
	if len(frames) == 0 {
		return ErrNoPages
	}
	// Every frame becomes a page of the logical screen's size, and the frames' own pixels are held by
	// DecodeAll together, so both are charged before it runs.
	for _, f := range frames {
		if err := j.charge(sw, sh); err != nil {
			return err
		}
		if int64(f[0])*int64(f[1]) > maxImagePixels {
			return fmt.Errorf("%w: a frame is %d×%d", ErrImagePixels, f[0], f[1])
		}
		// The frame's own pixels DecodeAll holds are charged to the same counter, so the one cap is the whole sum.
		j.pixels += int64(f[0]) * int64(f[1])
		if j.pixels > importTotalPixels {
			return fmt.Errorf("%w: the frames together are past %d megapixels", ErrImagePixels, importTotalPixels/1_000_000)
		}
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("pdfops: this gif could not be read: %w", err)
	}
	screen := image.Rect(0, 0, sw, sh)
	if sw == 0 || sh == 0 {
		screen = g.Image[0].Bounds()
	}
	canvas := image.NewRGBA(screen)
	for i, fr := range g.Image {
		var before *image.RGBA
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			before = image.NewRGBA(screen)
			copy(before.Pix, canvas.Pix)
		}
		draw.Draw(canvas, fr.Bounds(), fr, fr.Bounds().Min, draw.Over)
		if err := j.emitImageFrame(canvas); err != nil {
			return err
		}
		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, fr.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			copy(canvas.Pix, before.Pix)
		}
	}
	return nil
}

func (j *rasterJob) emitImageFrame(img *image.RGBA) error { return j.emitImage(img, defaultDPI) }

// ---- TIFF ---------------------------------------------------------------------------------------------------

type tiffPage struct {
	off        uint32
	w, h       int
	comp       uint16
	dpi        float64
	reducedRes bool
	bytesPerPx int64
}

// walkTIFF reads the IFD chain itself: x/image/tiff decodes the first image directory only, so a multi-page TIFF
// would open as its first page and say nothing. Offsets are bounds-checked, each visited at most once (a chain
// that loops is refused, never opened short), and the walk stops at importPages.
func walkTIFF(data []byte) ([]tiffPage, error) {
	bad := errors.New("pdfops: this tiff could not be read: its directory is damaged")
	if len(data) < 8 {
		return nil, bad
	}
	var bo binary.ByteOrder
	switch {
	case data[0] == 'I' && data[1] == 'I' && data[2] == 42 && data[3] == 0:
		bo = binary.LittleEndian
	case data[0] == 'M' && data[1] == 'M' && data[2] == 0 && data[3] == 42:
		bo = binary.BigEndian
	default:
		return nil, bad
	}
	var pages []tiffPage
	visited := map[uint32]bool{}
	for off := bo.Uint32(data[4:8]); off != 0; {
		if visited[off] {
			return pages, errTIFFLoop
		}
		if off < 8 || uint64(off)+2 > uint64(len(data)) {
			return pages, bad
		}
		visited[off] = true
		n := int(bo.Uint16(data[off : off+2]))
		end := uint64(off) + 2 + uint64(n)*12 + 4
		if end > uint64(len(data)) {
			return pages, bad
		}
		pg := tiffPage{off: off}
		var xres float64
		unit := uint16(2)
		for e := 0; e < n; e++ {
			ent := data[int(off)+2+e*12 : int(off)+2+e*12+12]
			tag, typ := bo.Uint16(ent[0:2]), bo.Uint16(ent[2:4])
			cnt := bo.Uint32(ent[4:8])
			val := func() uint32 { // a single SHORT or LONG, held inline
				if typ == 3 {
					return uint32(bo.Uint16(ent[8:10]))
				}
				return bo.Uint32(ent[8:12])
			}
			switch tag {
			case 254:
				pg.reducedRes = val()&5 != 0 // reduced-resolution image or transparency mask: not a page
			case 256:
				pg.w = int(val())
			case 257:
				pg.h = int(val())
			case 259:
				pg.comp = uint16(val())
			case 282:
				if typ == 5 && cnt == 1 {
					if o := bo.Uint32(ent[8:12]); uint64(o)+8 <= uint64(len(data)) {
						if d := bo.Uint32(data[o+4 : o+8]); d != 0 {
							xres = float64(bo.Uint32(data[o:o+4])) / float64(d)
						}
					}
				}
			case 296:
				unit = uint16(val())
			}
		}
		switch unit {
		case 2:
			pg.dpi = xres
		case 3:
			pg.dpi = xres * 2.54
		}
		pages = append(pages, pg)
		if len(pages) > importPages {
			return nil, fmt.Errorf("%w: Nib opens up to %d", ErrTooManyPages, importPages)
		}
		off = bo.Uint32(data[end-4 : end])
	}
	if len(pages) == 0 {
		return nil, bad
	}
	// Reduced-resolution thumbnails and masks are not pages — unless that would leave none.
	var real []tiffPage
	for _, p := range pages {
		if !p.reducedRes {
			real = append(real, p)
		}
	}
	if len(real) > 0 {
		pages = real
	}
	return pages, nil
}

// tiffAt presents the file with its header's first-directory offset replaced, so tiff.Decode — which reads the
// FIRST directory — decodes the page whose directory is at `off`. Every other byte is the file's own.
type tiffAt struct {
	data []byte
	off  uint32
	bo   binary.ByteOrder
}

func (t tiffAt) ReadAt(p []byte, at int64) (int, error) {
	if at >= int64(len(t.data)) {
		return 0, io.EOF
	}
	n := copy(p, t.data[at:])
	if at < 8 { // overlay the synthetic offset
		var hdr [4]byte
		t.bo.PutUint32(hdr[:], t.off)
		for i := range hdr {
			if k := int64(4+i) - at; k >= 0 && k < int64(n) {
				p[k] = hdr[i]
			}
		}
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// tiffFirstOffset is the header's offset of the first directory, which tiff.Decode reads.
func tiffFirstOffset(data []byte) uint32 {
	if data[0] == 'M' {
		return binary.BigEndian.Uint32(data[4:8])
	}
	return binary.LittleEndian.Uint32(data[4:8])
}

func (j *rasterJob) addTIFF(data []byte, pages []tiffPage) error {
	var bo0 binary.ByteOrder = binary.LittleEndian
	if data[0] == 'M' {
		bo0 = binary.BigEndian
	}
	for i := range pages {
		// The size that is charged is the DECODER's own reading of the directory it will decode, not the walk's:
		// two readings of one directory (a repeated tag) must not let a small charge buy a large allocation.
		cfg, err := tiff.DecodeConfig(io.NewSectionReader(tiffAt{data: data, off: pages[i].off, bo: bo0}, 0, int64(len(data))))
		if err != nil {
			return fmt.Errorf("pdfops: a page of this tiff could not be read: %w", err)
		}
		pages[i].w, pages[i].h = cfg.Width, cfg.Height
		pages[i].bytesPerPx = bytesPerPixel(cfg.ColorModel)
	}
	for _, pg := range pages {
		switch pg.comp {
		case 0, 1, 3, 4, 5, 8, 32773, 32946: // none, G3, G4, LZW, deflate, PackBits — what the decoder has
		default:
			return fmt.Errorf("%w (compression %d)", ErrTIFFCompression, pg.comp)
		}
		if err := j.charge(pg.w, pg.h); err != nil {
			return err
		}
		if err := j.chargeBytes(pg.w, pg.h, pg.bytesPerPx); err != nil {
			return err
		}
	}
	var bo binary.ByteOrder = binary.LittleEndian
	if data[0] == 'M' {
		bo = binary.BigEndian
	}
	for _, pg := range pages {
		if err := j.ctx.Err(); err != nil {
			return err
		}
		r := io.NewSectionReader(tiffAt{data: data, off: pg.off, bo: bo}, 0, int64(len(data)))
		img, err := tiff.Decode(r)
		if err != nil {
			return fmt.Errorf("pdfops: a page of this tiff could not be read: %w", err)
		}
		if err := j.emitImage(img, pg.dpi); err != nil {
			return err
		}
	}
	return nil
}

// ---- CBZ ----------------------------------------------------------------------------------------------------

// cbzLimits are D14's archive caps for a CBZ, whose members Nib reads by name: unsafe names are refused.
var cbzLimits = ziputil.Limits{
	Entries:      importArchiveEntries,
	DecodedBytes: importDecodedBytes,
	RatioFloor:   importRatioFloor,
	RatioMax:     importRatioMax,
}

// CBZToDocument turns a comic-book archive — a zip of pictures — into a PDF with a page per picture, in natural
// name order (page2 before page10). Members that are not pictures (ComicInfo.xml, thumbnails' folders, macOS
// resource forks) are not pages; a picture is known by its bytes, never its name.
func CBZToDocument(ctx context.Context, data []byte, name string) ([]byte, error) {
	j, cancel := newRasterJob(ctx)
	defer cancel()
	a, err := ziputil.Open(bytes.NewReader(data), int64(len(data)), cbzLimits)
	if err != nil {
		if errors.Is(err, ziputil.ErrUnreadable) {
			return nil, fmt.Errorf("%w: the archive could not be read", ErrCorruptDocument)
		}
		return nil, fmt.Errorf("%w: %v", ErrArchiveRefused, err)
	}
	files := a.Files()
	order := make([]int, 0, len(files))
	for i, f := range files {
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") || strings.HasPrefix(baseOf(f.Name), ".") {
			continue
		}
		order = append(order, i)
	}
	sort.SliceStable(order, func(x, y int) bool { return naturalLess(files[order[x]].Name, files[order[y]].Name) })
	for _, i := range order {
		if err := j.ctx.Err(); err != nil {
			return nil, err
		}
		// A picture is known by its bytes, so the member's first bytes decide whether it is read at all: a text
		// file or a big ComicInfo.xml over the picture cap is not a page and is not why the archive is refused.
		head, err := a.Peek(files[i], cbzSniffBytes)
		if err != nil {
			if errors.Is(err, ziputil.ErrUnreadable) {
				return nil, fmt.Errorf("%w: a member of the archive could not be read", ErrCorruptDocument)
			}
			return nil, fmt.Errorf("%w: %v", ErrArchiveRefused, err)
		}
		if !LooksLikeImage(head) {
			continue
		}
		b, err := a.Read(files[i], importCBZEntryBytes)
		if err != nil {
			if errors.Is(err, ziputil.ErrUnreadable) {
				return nil, fmt.Errorf("%w: a picture in the archive could not be read", ErrCorruptDocument)
			}
			return nil, fmt.Errorf("%w: %v", ErrArchiveRefused, err)
		}
		if err := j.addFile(b); err != nil {
			return nil, err
		}
	}
	return j.finish(name)
}

func baseOf(n string) string {
	if i := strings.LastIndexByte(n, '/'); i >= 0 {
		return n[i+1:]
	}
	return n
}

// naturalLess orders names so that runs of digits compare as numbers and the rest case-insensitively:
// "p2.png" < "p10.png". A tie in value falls back to the plain comparison so the order is total.
func naturalLess(a, b string) bool {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, k := 0, 0
	for i < len(ra) && k < len(rb) {
		if unicode.IsDigit(ra[i]) && unicode.IsDigit(rb[k]) {
			si := i
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			sk := k
			for k < len(rb) && unicode.IsDigit(rb[k]) {
				k++
			}
			na, nb := strings.TrimLeft(string(ra[si:i]), "0"), strings.TrimLeft(string(rb[sk:k]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ra[i] != rb[k] {
			return ra[i] < rb[k]
		}
		i++
		k++
	}
	if len(ra)-i != len(rb)-k {
		return len(ra)-i < len(rb)-k
	}
	return a < b
}

func looksLikeGIF(b []byte) bool {
	return bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a"))
}
