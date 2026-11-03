package pdfops

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"strings"

	"golang.org/x/image/bmp"

	"nib/internal/ziputil"
)

// Pages as images, the server half (PLAN-import-export P02.S01, ADR-189). The browser renders each page to a PNG
// (pdf.js is the renderer, and the only one that sees the document as the window shows it); this is the ONE place
// those PNGs become a GIF, a BMP, a multi-page TIFF, a CBZ or a ZIP. PNG, JPEG and WebP are the browser's own
// `canvas.toBlob`: Go has no WebP encoder, and a second JPEG path would disagree with the first.

// PageImageFormat names a way to package rendered pages.
type PageImageFormat string

const (
	PageImagesZIP  PageImageFormat = "zip"
	PageImagesCBZ  PageImageFormat = "cbz"
	PageImagesTIFF PageImageFormat = "tiff"
	PageImagesGIF  PageImageFormat = "gif"
	PageImagesBMP  PageImageFormat = "bmp"
)

// ErrPageImageFormat: a format this door does not write.
var ErrPageImageFormat = errors.New("that is not a picture format Nib writes")

// ErrPageImageUnreadable: a page picture that is not a PNG Nib can decode.
var ErrPageImageUnreadable = errors.New("a page picture could not be read")

// ErrOnePageFormat: a format that holds one picture was asked for several.
var ErrOnePageFormat = errors.New("that picture format holds one page; use TIFF, CBZ or ZIP for several")

// PageImageResult is the packaged bytes with the name and media type a download carries.
type PageImageResult struct {
	Data []byte
	Name string
	Mime string
}

// ParsePageImageFormat reads a form value. The empty string is not a format.
func ParsePageImageFormat(s string) (PageImageFormat, bool) {
	switch f := PageImageFormat(strings.ToLower(s)); f {
	case PageImagesZIP, PageImagesCBZ, PageImagesTIFF, PageImagesGIF, PageImagesBMP:
		return f, true
	}
	return "", false
}

// EncodePageImages packages pngs (one per page, in order) as format. widthsPt, when it has one entry per page,
// sets a TIFF's resolution from the page's width in points; otherwise 144 dpi, the window's render scale.
// A page past the pixel caps is refused (ErrImagePixels), never shrunk, and so is a page count past importPages.
func EncodePageImages(format PageImageFormat, pngs [][]byte, widthsPt []float64) (PageImageResult, error) {
	if len(pngs) == 0 {
		return PageImageResult{}, ErrNoPages
	}
	if len(pngs) > importPages {
		return PageImageResult{}, fmt.Errorf("%w: %d pages, and Nib writes up to %d", ErrTooManyPages, len(pngs), importPages)
	}
	switch format {
	case PageImagesZIP, PageImagesCBZ:
		return packPageArchive(format, pngs)
	case PageImagesTIFF:
		return packPageTIFF(pngs, widthsPt)
	case PageImagesGIF, PageImagesBMP:
		if len(pngs) != 1 {
			return PageImageResult{}, ErrOnePageFormat
		}
		return packOnePage(format, pngs[0])
	}
	return PageImageResult{}, fmt.Errorf("%w: %q", ErrPageImageFormat, string(format))
}

// decodePage decodes one PNG after charging its declared size to the job, so a hostile or merely huge upload is
// refused before it is expanded.
func decodePage(j *rasterJob, data []byte) (image.Image, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pdfops: %w: %v", ErrPageImageUnreadable, err)
	}
	if err := j.charge(cfg.Width, cfg.Height); err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pdfops: %w: %v", ErrPageImageUnreadable, err)
	}
	return img, nil
}

func packPageArchive(format PageImageFormat, pngs [][]byte) (PageImageResult, error) {
	entries := make([]ziputil.Entry, len(pngs))
	for i, p := range pngs {
		// A picture that is not a PNG is not a page of this export: the names promise PNG.
		if _, err := png.DecodeConfig(bytes.NewReader(p)); err != nil {
			return PageImageResult{}, fmt.Errorf("pdfops: %w: %v", ErrPageImageUnreadable, err)
		}
		entries[i] = ziputil.Entry{Name: fmt.Sprintf("page-%03d.png", i+1), Data: p}
	}
	data, err := ziputil.Write(entries)
	if err != nil {
		return PageImageResult{}, err
	}
	if format == PageImagesCBZ {
		return PageImageResult{Data: data, Name: "pages.cbz", Mime: "application/vnd.comicbook+zip"}, nil
	}
	return PageImageResult{Data: data, Name: "pages.zip", Mime: "application/zip"}, nil
}

func packOnePage(format PageImageFormat, data []byte) (PageImageResult, error) {
	j, cancel := newRasterJob(context.Background())
	defer cancel()
	img, err := decodePage(j, data)
	if err != nil {
		return PageImageResult{}, err
	}
	var buf bytes.Buffer
	switch format {
	case PageImagesBMP:
		err = bmp.Encode(&buf, img)
		return finishOne(buf, err, "page.bmp", "image/bmp")
	default:
		err = gif.Encode(&buf, img, &gif.Options{NumColors: 256, Quantizer: documentQuantizer{}, Drawer: draw.FloydSteinberg})
		return finishOne(buf, err, "page.gif", "image/gif")
	}
}

func finishOne(buf bytes.Buffer, err error, name, mime string) (PageImageResult, error) {
	if err != nil {
		return PageImageResult{}, fmt.Errorf("pdfops: could not write the picture: %w", err)
	}
	return PageImageResult{Data: buf.Bytes(), Name: name, Mime: mime}, nil
}

// documentPalette is 40 greys then a 6×6×6 colour cube: a page is mostly white, black and the greys between
// them, which the stock Plan 9 palette dithers into visible noise.
var documentPalette = func() color.Palette {
	p := make(color.Palette, 0, 256)
	for i := 0; i < 40; i++ {
		v := uint8(i * 255 / 39)
		p = append(p, color.RGBA{v, v, v, 255})
	}
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				p = append(p, color.RGBA{uint8(r * 51), uint8(g * 51), uint8(b * 51), 255})
			}
		}
	}
	return p
}()

type documentQuantizer struct{}

func (documentQuantizer) Quantize(p color.Palette, _ image.Image) color.Palette {
	return append(p, documentPalette...)
}

// ---- multi-page TIFF -------------------------------------------------------------------------------------------

const tiffStripTarget = 256 << 10

func packPageTIFF(pngs [][]byte, widthsPt []float64) (PageImageResult, error) {
	j, cancel := newRasterJob(context.Background())
	defer cancel()
	le := binary.LittleEndian
	var out bytes.Buffer
	out.Write([]byte{'I', 'I', 42, 0, 0, 0, 0, 0})
	patch := 4 // where the offset of the next IFD goes
	for i, data := range pngs {
		if err := j.ctx.Err(); err != nil {
			return PageImageResult{}, fmt.Errorf("pdfops: writing the TIFF took too long: %w", err)
		}
		img, err := decodePage(j, data)
		if err != nil {
			return PageImageResult{}, err
		}
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		rowBytes := w * 3
		rows := tiffStripTarget / rowBytes
		if rows < 1 {
			rows = 1
		}
		if rows > h {
			rows = h
		}
		// Rows are flattened over white (a page is opaque; a transparent pixel must not become black) and
		// compressed strip by strip, so the decoded page is the only large buffer.
		var offsets, counts []uint32
		row := make([]byte, rowBytes)
		for y0 := 0; y0 < h; y0 += rows {
			var raw bytes.Buffer
			zw, _ := zlib.NewWriterLevel(&raw, zlib.BestSpeed) // a page is mostly white: level 1 is within a few percent and several times faster
			for y := y0; y < y0+rows && y < h; y++ {
				fillRow(row, img, b.Min.X, b.Min.Y+y, w)
				zw.Write(row)
			}
			if err := zw.Close(); err != nil {
				return PageImageResult{}, err
			}
			offsets = append(offsets, uint32(out.Len()))
			counts = append(counts, uint32(raw.Len()))
			out.Write(raw.Bytes())
			if out.Len() > maxConvertedBytes {
				return PageImageResult{}, fmt.Errorf("%w: the TIFF passed %d MiB", ErrImagePixels, maxConvertedBytes>>20)
			}
		}
		align(&out)
		bitsAt := uint32(out.Len())
		out.Write([]byte{8, 0, 8, 0, 8, 0})
		dpi := 144.0
		if len(widthsPt) == len(pngs) && widthsPt[i] > 0 {
			dpi = float64(w) * 72 / widthsPt[i]
			if dpi < 1 || dpi > 100000 || dpi != dpi {
				dpi = 144 // a width in points that makes no sense is the render scale, never garbage in the file
			}
		}
		resAt := uint32(out.Len())
		for k := 0; k < 2; k++ {
			var rat [8]byte
			le.PutUint32(rat[0:], uint32(dpi*100+0.5))
			le.PutUint32(rat[4:], 100)
			out.Write(rat[:])
		}
		offAt, cntAt := uint32(0), uint32(0)
		if len(offsets) > 1 {
			offAt = uint32(out.Len())
			for _, v := range offsets {
				binary.Write(&out, le, v)
			}
			cntAt = uint32(out.Len())
			for _, v := range counts {
				binary.Write(&out, le, v)
			}
		}
		align(&out)
		ifdAt := uint32(out.Len())
		le.PutUint32(out.Bytes()[patch:], ifdAt)
		type tag struct {
			id, typ uint16
			n, val  uint32
		}
		const short, long, rational = 3, 4, 5
		stripOff, stripCnt := offsets[0], counts[0]
		if len(offsets) > 1 {
			stripOff, stripCnt = offAt, cntAt
		}
		tags := []tag{
			{256, long, 1, uint32(w)}, {257, long, 1, uint32(h)}, {258, short, 3, bitsAt}, {259, short, 1, 8},
			{262, short, 1, 2}, {273, long, uint32(len(offsets)), stripOff}, {277, short, 1, 3},
			{278, long, 1, uint32(rows)}, {279, long, uint32(len(counts)), stripCnt},
			{282, rational, 1, resAt}, {283, rational, 1, resAt + 8}, {284, short, 1, 1}, {296, short, 1, 2},
		}
		var ifd [2]byte
		le.PutUint16(ifd[:], uint16(len(tags)))
		out.Write(ifd[:])
		for _, t := range tags {
			var e [12]byte
			le.PutUint16(e[0:], t.id)
			le.PutUint16(e[2:], t.typ)
			le.PutUint32(e[4:], t.n)
			if t.typ == short && t.n == 1 {
				le.PutUint16(e[8:], uint16(t.val))
			} else {
				le.PutUint32(e[8:], t.val)
			}
			out.Write(e[:])
		}
		patch = out.Len()
		out.Write([]byte{0, 0, 0, 0})
	}
	return PageImageResult{Data: out.Bytes(), Name: "pages.tiff", Mime: "image/tiff"}, nil
}

// fillRow writes row y of img as RGB over white. The two layouts a PNG decodes to for an opaque page (*image.RGBA,
// *image.NRGBA) are read from their pixel slices: going through image.Image.At cost 0.27 s a Letter page at 2×.
func fillRow(row []byte, img image.Image, x0, y, w int) {
	switch m := img.(type) {
	case *image.RGBA:
		o := m.PixOffset(x0, y)
		for x := 0; x < w; x++ {
			p := m.Pix[o+4*x : o+4*x+4 : o+4*x+4]
			a := uint32(p[3])
			row[3*x], row[3*x+1], row[3*x+2] = over(p[0], a), over(p[1], a), over(p[2], a)
		}
	case *image.NRGBA:
		o := m.PixOffset(x0, y)
		for x := 0; x < w; x++ {
			p := m.Pix[o+4*x : o+4*x+4 : o+4*x+4]
			a := uint32(p[3])
			row[3*x] = nover(p[0], a)
			row[3*x+1] = nover(p[1], a)
			row[3*x+2] = nover(p[2], a)
		}
	default:
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(x0+x, y).RGBA()
			row[3*x], row[3*x+1], row[3*x+2] = flatten(r, a), flatten(g, a), flatten(bl, a)
		}
	}
}

// over composites a premultiplied 8-bit channel c with alpha a over white; nover does the same for a straight one.
func over(c byte, a uint32) byte  { return byte(uint32(c) + (255 - a)) }
func nover(c byte, a uint32) byte { return byte((uint32(c)*a + 255*(255-a) + 127) / 255) }

func align(b *bytes.Buffer) {
	if b.Len()%2 == 1 {
		b.WriteByte(0)
	}
}

// flatten returns the 8-bit channel c of a premultiplied pixel with alpha a, composited over white.
func flatten(c, a uint32) byte {
	return byte((c + (0xffff - a)) >> 8)
}
