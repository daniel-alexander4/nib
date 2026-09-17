package pdfops

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder for image.DecodeConfig
	_ "image/png"  // and PNG
)

// Opening an image as a document — `/pending 400`.
//
// # Why a door rather than a branch in each open route
//
// Four routes admit bytes — path open, upload/drag-drop, the OS hand-off and File's Open — and each
// one refuses a non-PDF today at `LooksLikePDF`. A branch per route is four places to add a format,
// four places to get the page size wrong, and four places for the next reader to find only three of.
// `ImageToDocument` is the one door: it decides whether the bytes are an image at all, and if they
// are, it returns a PDF the rest of nib treats exactly like any other.
//
// # The size rule, and what it rejects
//
// A page is the image's PHYSICAL size: pixels ÷ dpi × 72. The dpi comes from the file's own metadata
// — PNG's `pHYs` chunk or JPEG's JFIF density — and where there is none it is **96**, which is what
// a screenshot is authored at. A 1920×1080 screenshot then opens at about 1440×810 pt, close to the
// size it occupied on screen.
//
// Rejected: fitting to Letter or A4, which either changes the aspect or pads a screenshot with
// margins it never had; and assuming 72 dpi, which makes every screenshot a third larger than it
// looks.
//
// # EXIF orientation is applied as a page rotation, not by rewriting pixels
//
// **pdfcpu never reads EXIF** — a named search over its Go source (`pdfcpu@v0.13.0/pkg/**.go`,
// case-insensitive `exif`) returns nothing — so a phone photo carrying orientation 6 would open
// sideways unless nib does something. Rotating the pixels means decoding and re-encoding: lossy for
// a JPEG, and hugely larger for a PNG. A page `/Rotate` is lossless, is one dictionary key, and is
// what a viewer already honours.
//
// The four unmirrored orientations map onto it exactly. **The mirrored four (2, 4, 5, 7) cannot be
// expressed as a rotation and are a declared gap**: the image opens unmirrored, which is what every
// viewer that ignores EXIF already does, and no claim is made that it was corrected.

// ErrNotAnImage says the bytes are not a format this door opens. Callers use it to tell "this is an
// image nib cannot read" from "this is not an image at all", because only the second should fall
// through to the existing not-a-PDF refusal.
var ErrNotAnImage = errors.New("pdfops: not an image this build opens")

// defaultDPI is what an image with no density metadata is assumed to be authored at. Screenshots
// carry none and are the common case.
const defaultDPI = 96.0

// maxImagePixels caps the DECODED pixel count, which the byte cap cannot: a PNG of a few hundred KB
// can decode to gigabytes, and `image.DecodeConfig` reports the dimensions without allocating them,
// so the refusal costs the header and not the image.
//
// 200 megapixels. Measured against the largest thing a user plausibly opens: a 100 MP phone photo is
// ~11,600×8,700, and an 8K screenshot is 33 MP. At 4 bytes per pixel a 200 MP image decodes to about
// 800 MB, which is the point at which refusing beats trying.
const maxImagePixels = 200 * 1000 * 1000

// imageFormat is what the magic bytes said, which is never what the extension said.
type imageFormat string

const (
	formatPNG  imageFormat = "png"
	formatJPEG imageFormat = "jpeg"
)

// sniffImage reports the format from the leading bytes.
//
// **By magic bytes, never by extension.** A text file renamed `.png` must be refused with the
// ordinary not-a-PDF sentence rather than reaching a decoder, and an image whose extension is wrong
// — which is ordinary, since a download names a file whatever the server said — must still open.
func sniffImage(data []byte) (imageFormat, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return formatPNG, true
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return formatJPEG, true
	}
	return "", false
}

// LooksLikeImage reports whether these bytes are an image `ImageToDocument` opens. It is the cheap
// test an open route makes before committing to a conversion.
func LooksLikeImage(data []byte) bool {
	_, ok := sniffImage(data)
	return ok
}

// pngDPI reads the `pHYs` chunk's horizontal density, which PNG stores in pixels per METRE.
//
// Returns 0 when the chunk is absent or declares a unit other than metres, which is the "unknown"
// the caller turns into `defaultDPI` — never a guess dressed as a reading.
func pngDPI(data []byte) float64 {
	// 8-byte signature, then chunks of: 4-byte length, 4-byte type, payload, 4-byte CRC.
	for off := 8; off+12 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[off : off+4]))
		if n < 0 || off+12+n > len(data) {
			return 0
		}
		typ := string(data[off+4 : off+8])
		if typ == "pHYs" && n >= 9 {
			body := data[off+8 : off+8+n]
			ppuX := binary.BigEndian.Uint32(body[0:4])
			if body[8] != 1 || ppuX == 0 { // unit 1 is metres; 0 means "aspect only, no real size"
				return 0
			}
			return float64(ppuX) * 0.0254
		}
		if typ == "IDAT" || typ == "IEND" {
			return 0 // pHYs precedes the image data; past it there is none
		}
		off += 12 + n
	}
	return 0
}

// jpegSegments walks a JPEG's marker segments, handing each one's marker and body to fn until fn
// returns false or the scan data begins. `jpegDPI` and `jpegOrientation` both need the same walk,
// so they share this one rather than each re-deriving where a segment ends.
func jpegSegments(data []byte, fn func(marker byte, body []byte) bool) {
	for off := 2; off+4 <= len(data); {
		if data[off] != 0xFF {
			return
		}
		marker := data[off+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			off += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan / end: no more headers
			return
		}
		n := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
		if n < 2 || off+2+n > len(data) {
			return
		}
		if !fn(marker, data[off+4:off+2+n]) {
			return
		}
		off += 2 + n
	}
}

func jpegDPI(data []byte) float64 {
	dpi := 0.0
	jpegSegments(data, func(marker byte, body []byte) bool {
		if marker != 0xE0 || !bytes.HasPrefix(body, []byte("JFIF\x00")) || len(body) < 12 {
			return true
		}
		unit := body[7]
		x := binary.BigEndian.Uint16(body[8:10])
		if x == 0 {
			return false
		}
		switch unit {
		case 1: // dots per inch
			dpi = float64(x)
		case 2: // dots per centimetre
			dpi = float64(x) * 2.54
		}
		return false
	})
	return dpi
}

// jpegOrientation returns the EXIF orientation tag (1-8), or 0 when there is none.
func jpegOrientation(data []byte) int {
	found := 0
	jpegSegments(data, func(marker byte, body []byte) bool {
		if marker != 0xE1 || !bytes.HasPrefix(body, []byte("Exif\x00\x00")) {
			return true
		}
		tiff := body[6:]
		if len(tiff) < 8 {
			return false
		}
		var bo binary.ByteOrder
		switch string(tiff[0:2]) {
		case "II":
			bo = binary.LittleEndian
		case "MM":
			bo = binary.BigEndian
		default:
			return false
		}
		ifd := int(bo.Uint32(tiff[4:8]))
		if ifd+2 > len(tiff) {
			return false
		}
		count := int(bo.Uint16(tiff[ifd : ifd+2]))
		for i := 0; i < count; i++ {
			e := ifd + 2 + i*12
			if e+12 > len(tiff) {
				return false
			}
			if bo.Uint16(tiff[e:e+2]) == 0x0112 { // Orientation
				found = int(bo.Uint16(tiff[e+8 : e+10]))
				return false
			}
		}
		return false
	})
	return found
}

// rotationFor maps an EXIF orientation onto a clockwise page rotation.
//
// The mirrored orientations return (0, false): a mirror is not a rotation, and reporting one as
// though it were would present the image confidently backwards.
func rotationFor(orientation int) (deg int, expressible bool) {
	switch orientation {
	case 0, 1:
		return 0, true
	case 3:
		return 180, true
	case 6:
		return 90, true
	case 8:
		return 270, true
	}
	return 0, false
}

// ImageToDocument turns an image into a one-page PDF at the image's physical size, titled from name.
//
// **name is the source file's name and the document is titled from it**, because a document nib
// authors owes a title: `TestEveryAuthoredDocumentGetsATitle` enumerates the doors that produce a
// PDF and refuses one that reaches neither `TitleFromName` nor `SetTitle`. A screenshot opened from
// `receipt.png` should say "receipt" in a reader's title bar, not nothing. An empty name is allowed
// and simply leaves the title unset — `TitleFromName` returns the document untouched.
//
// It returns ErrNotAnImage when the bytes are not an image at all, so a caller can tell that from a
// conversion that failed on an image it did recognise.
func ImageToDocument(data []byte, name string) ([]byte, error) {
	format, ok := sniffImage(data)
	if !ok {
		return nil, ErrNotAnImage
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pdfops: this %s could not be read: %w", format, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("pdfops: this %s declares a %d×%d image, which has no size", format, cfg.Width, cfg.Height)
	}
	if px := int64(cfg.Width) * int64(cfg.Height); px > maxImagePixels {
		return nil, fmt.Errorf("pdfops: this image is %d×%d — %d megapixels, and nib opens images up to %d",
			cfg.Width, cfg.Height, px/1_000_000, maxImagePixels/1_000_000)
	}

	dpi := 0.0
	switch format {
	case formatPNG:
		dpi = pngDPI(data)
	case formatJPEG:
		dpi = jpegDPI(data)
	}
	if dpi <= 0 {
		dpi = defaultDPI
	}

	page := RasterPage{
		Image: data,
		W:     float64(cfg.Width) / dpi * 72,
		H:     float64(cfg.Height) / dpi * 72,
	}
	out, err := ImagesToPDF([]RasterPage{page})
	if err != nil {
		return nil, fmt.Errorf("pdfops: this %s could not be made into a page: %w", format, err)
	}

	if titled, terr := TitleFromName(out, name); terr == nil {
		out = titled // never costs the caller the document: on failure the untitled one stands
	}

	if format == formatJPEG {
		if deg, expressible := rotationFor(jpegOrientation(data)); expressible && deg != 0 {
			rotated, rerr := Rotate(out, nil, deg)
			if rerr != nil {
				return nil, fmt.Errorf("pdfops: this JPEG's orientation could not be applied: %w", rerr)
			}
			out = rotated
		}
	}
	return out, nil
}
