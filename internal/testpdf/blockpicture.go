package testpdf

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strconv"
)

// Picture is one image XObject found in a PDF's bytes: its size and its decoded samples.
type Picture struct {
	W, H int
	RGB  []byte // W*H*3 samples, row by row from the top
}

var (
	imageDict   = regexp.MustCompile(`(?s)<<[^<>]*?/Subtype\s*/Image[^<>]*?>>\s*stream\r?\n`)
	imageWidth  = regexp.MustCompile(`/Width\s+(\d+)`)
	imageHeight = regexp.MustCompile(`/Height\s+(\d+)`)
	imageLength = regexp.MustCompile(`/Length\s+(\d+)`)
)

// PicturesIn returns every DeviceRGB, flate-coded image XObject written PLAINLY in pdf — which is how
// the signing library writes a visible signature's picture, in the incremental update it appends —
// in file order. It is how a test reads the signature block a document actually carries (ADR-158):
// the samples, not a rendering of them.
//
// It reads bytes rather than parsing, on purpose and within a stated limit: an image inside an
// object stream, or one with another filter or colour space, is not returned. A caller that expects
// a picture and gets none must fail, never pass.
func PicturesIn(pdf []byte) []Picture {
	var out []Picture
	for _, loc := range imageDict.FindAllIndex(pdf, -1) {
		dict := pdf[loc[0]:loc[1]]
		if !bytes.Contains(dict, []byte("/DeviceRGB")) || !bytes.Contains(dict, []byte("/FlateDecode")) ||
			bytes.Contains(dict, []byte("/DCTDecode")) {
			continue
		}
		w, h, n := number(imageWidth, dict), number(imageHeight, dict), number(imageLength, dict)
		if w <= 0 || h <= 0 || n <= 0 || loc[1]+n > len(pdf) {
			continue
		}
		zr, err := zlib.NewReader(bytes.NewReader(pdf[loc[1] : loc[1]+n]))
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(zr)
		if err != nil || len(raw) != w*h*3 {
			continue
		}
		out = append(out, Picture{W: w, H: h, RGB: raw})
	}
	return out
}

func number(re *regexp.Regexp, b []byte) int {
	m := re.FindSubmatch(b)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}
