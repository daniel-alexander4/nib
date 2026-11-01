package pdfops

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"sort"

	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/image/draw"
)

// Shrinking the pictures in a document, and nothing else — `/pending 908`.
//
// # Why this exists beside Optimize and Compress
//
// Optimize is lossless and does nearly nothing for a scan or a photo portfolio, whose bytes ARE their
// pictures. Compress re-renders every page as one JPEG, so a 4.8 MB document that must be 1 MB becomes a
// document with no selectable text, no search and no links. What people asked for (three posts in a month, the
// entry records them) is the third thing: make the PICTURES smaller and leave the page alone.
//
// # What it touches, and what it leaves
//
// An image XObject is re-encoded as a JPEG, at a lower pixel size and quality, ONLY WHEN
//
//   - it is a JPEG (DCTDecode alone) of one or three components, or an 8-bit Flate-coded Gray/RGB/ICC image that
//     is plainly a photograph (more than 256 distinct colours in a sample — a flat-colour graphic stays exact);
//   - it has no /Decode, no colour-key /Mask, no stencil, no /SMaskInData and no soft mask carrying a /Matte,
//     because each of those ties the sample VALUES to something this does not carry across;
//   - the re-encoded stream is at least 10 % smaller than what was there. A picture that would grow, or barely
//     shrink, is kept as it is.
//
// **The colour space is never touched.** An image's colours are its samples read through the /ColorSpace entry in
// its own dictionary — an ICC profile, a Gray, a CalRGB — and this changes the samples' count and precision, not that
// entry, so the profile travels with the picture and "without changing the colours" is a property of the
// construction, not a hope. CMYK, indexed, JPEG 2000, JBIG2 and CCITT pictures are left alone (no encoder here
// keeps them honest). A soft mask keeps its own size: the specification lets a mask differ from the picture it masks.
//
// Text, vectors, links, forms and structure are never read — only the image streams are replaced — so everything
// that is not a picture is byte-for-byte what the rewrite (the same read-optimise-write Optimize performs) leaves.

// maxShrinkSamples bounds the samples one Flate picture may declare (w × h × components), in bytes: 256 MiB, about 85
// megapixels of RGB. A larger picture is left as it is rather than inflated into memory to be re-encoded.
const maxShrinkSamples = 256 << 20

// ShrinkLevel is one rung of the ladder: the longest side a picture may keep, in pixels, and the JPEG quality.
type ShrinkLevel struct {
	MaxEdge int
	Quality int
}

// ShrinkLadder runs from the gentlest to the hardest. Sizes fall monotonically down it, which is what lets
// `ShrinkToSize` find the gentlest rung that fits by bisection instead of trying every one.
var ShrinkLadder = []ShrinkLevel{
	{3200, 88}, {2400, 82}, {1800, 76}, {1400, 70}, {1000, 62}, {700, 52},
}

// ShrinkReport says what a pass did, so the surface can be honest about a result that did not move.
type ShrinkReport struct {
	Images  int            `json:"images"`         // picture streams found
	Shrunk  int            `json:"shrunk"`         // re-encoded smaller
	Kept    int            `json:"kept"`           // eligible, but the result was not at least 10 % smaller
	Skipped int            `json:"skipped"`        // not a picture this touches (a stencil, CMYK, a graphic, a mask, an unreadable one)
	Left    map[string]int `json:"left,omitempty"` // why pictures were skipped: reason -> count
	Level   int            `json:"level"`          // the ladder rung used
	Before  int            `json:"before"`         // bytes in
	After   int            `json:"after"`          // bytes out
	Met     bool           `json:"met"`            // for ShrinkToSize: the target was reached
}

// ShrinkPictures re-encodes the document's pictures at one rung of the ladder.
func ShrinkPictures(pdf []byte, level int) ([]byte, ShrinkReport, error) {
	if level < 0 || level >= len(ShrinkLadder) {
		return nil, ShrinkReport{}, fmt.Errorf("pdfops: there is no picture level %d (0-%d)", level, len(ShrinkLadder)-1)
	}
	rep := ShrinkReport{Level: level, Before: len(pdf)}
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.OPTIMIZE
	out, err := rewriteWithConf(pdf, conf, func(ctx *model.Context) error {
		shrinkImages(ctx, ShrinkLadder[level], &rep)
		return nil
	})
	if err != nil {
		return nil, ShrinkReport{}, err
	}
	// Optimize's own rewrite can grow a file whose pictures this left alone (measured: +2 % on one real document), and
	// "smaller" that is larger is a wrong answer: the bytes that came in are returned instead, unchanged.
	if len(out) >= len(pdf) {
		out, rep.Shrunk = pdf, 0
	}
	rep.After = len(out)
	return out, rep, nil
}

// ShrinkToSize finds the gentlest rung whose output is at most target bytes, by bisection over the ladder (at most
// three passes for six rungs). Where none fits it returns the hardest rung's output with Met false — the smallest
// this method can reach, which the caller says in words.
func ShrinkToSize(pdf []byte, target int) ([]byte, ShrinkReport, error) {
	if target <= 0 {
		return nil, ShrinkReport{}, fmt.Errorf("pdfops: the target size must be positive")
	}
	lo, hi := 0, len(ShrinkLadder)-1
	var best []byte
	var bestRep ShrinkReport
	found := false
	for lo <= hi {
		mid := (lo + hi) / 2
		out, rep, err := ShrinkPictures(pdf, mid)
		if err != nil {
			return nil, ShrinkReport{}, err
		}
		if len(out) <= target {
			best, bestRep, found = out, rep, true
			hi = mid - 1 // fits: try a gentler rung
		} else {
			if !found {
				best, bestRep = out, rep // the hardest tried so far, in case none fits
			}
			lo = mid + 1
		}
	}
	bestRep.Met = found
	return best, bestRep, nil
}

// shrinkImages walks the image streams in object order (stable, so two passes agree) and replaces what qualifies.
func shrinkImages(ctx *model.Context, lv ShrinkLevel, rep *ShrinkReport) {
	nrs := make([]int, 0, len(ctx.Table))
	for nr, e := range ctx.Table {
		if e == nil || e.Free {
			continue
		}
		if sd, ok := e.Object.(types.StreamDict); ok {
			if st := sd.Dict.NameEntry("Subtype"); st != nil && *st == "Image" {
				nrs = append(nrs, nr)
			}
		}
	}
	sort.Ints(nrs)
	for _, nr := range nrs {
		e := ctx.Table[nr]
		sd := e.Object.(types.StreamDict)
		rep.Images++
		out, why := shrinkOne(ctx.XRefTable, &sd, lv)
		switch why {
		case "":
			e.Object = *out
			rep.Shrunk++
		case reasonNotSmaller:
			rep.Kept++
		default:
			rep.Skipped++
			if rep.Left == nil {
				rep.Left = map[string]int{}
			}
			rep.Left[why]++
		}
	}
}

// The reasons a picture is left as it is. Counted in the report, so a pass that changed little can say why in words
// instead of leaving the user to guess whether it worked.
const (
	reasonNotSmaller = "not-smaller" // eligible; the result was not at least 10 % smaller
	reasonStencil    = "stencil"
	reasonDecode     = "decode"
	reasonMask       = "mask"
	reasonColour     = "colour"  // CMYK, indexed, separation, JPEG 2000: a colour space this does not re-encode
	reasonFormat     = "format"  // JBIG2, CCITT, LZW, a filter chain, other than 8 bits a sample
	reasonGraphic    = "graphic" // flat colours; a JPEG would blur them
	reasonSize       = "size"    // beyond the pixel cap, or unreadable
)

// shrinkOne returns the replacement for one image stream, or the reason there is none ("" when it replaced one).
func shrinkOne(xt *model.XRefTable, sd *types.StreamDict, lv ShrinkLevel) (out *types.StreamDict, why string) {
	d := sd.Dict
	// What ties the samples' VALUES to something this does not carry across.
	if b := d.BooleanEntry("ImageMask"); b != nil && *b {
		return nil, reasonStencil
	}
	if _, has := d["Decode"]; has {
		return nil, reasonDecode
	}
	if m, has := d["Mask"]; has {
		if _, isArray := m.(types.Array); isArray {
			return nil, reasonMask // a colour-key mask names sample values; a lossy re-encode moves them
		}
	}
	if intEntry(d, "SMaskInData") != 0 { // an integer, not a boolean: 1 and 2 say the alpha is inside the JPEG 2000 data
		return nil, reasonMask
	}
	if sm, has := d["SMask"]; has {
		if msd, _, _ := xt.DereferenceStreamDict(sm); msd != nil {
			if _, matte := msd.Dict["Matte"]; matte {
				return nil, reasonMask
			}
		}
	}
	comps := imageComponents(xt, d["ColorSpace"])
	if comps != 1 && comps != 3 {
		return nil, reasonColour
	}
	w, h := intEntry(d, "Width"), intEntry(d, "Height")
	if w <= 0 || h <= 0 || int64(w)*int64(h) > maxImagePixels {
		return nil, reasonSize
	}

	var src image.Image
	switch pipelineOf(sd) {
	case "DCTDecode":
		img, err := jpeg.Decode(bytes.NewReader(sd.Raw))
		if err != nil {
			return nil, reasonSize
		}
		switch img.(type) {
		case *image.YCbCr, *image.RGBA, *image.NRGBA:
			if comps != 3 {
				return nil, reasonColour
			}
		case *image.Gray:
			if comps != 1 {
				return nil, reasonColour
			}
		default: // CMYK, and anything else Go decodes that it cannot encode back
			return nil, reasonColour
		}
		src = img
	case "FlateDecode", "":
		if bpc := intEntry(d, "BitsPerComponent"); bpc != 8 {
			return nil, reasonFormat
		}
		// Inflated through the capped door (`pdfread.DecodeWithin`), to the size the dictionary DECLARES and no further: a
		// stream that claims a small picture and inflates past it is a decompression bomb, and is left as it is. The
		// declared size is itself bounded, so a header cannot ask for a gigabyte of samples.
		want := int64(w) * int64(h) * int64(comps)
		if want > maxShrinkSamples {
			return nil, reasonSize
		}
		if err := pdfread.DecodeWithin(sd, want); err != nil || int64(len(sd.Content)) != want {
			return nil, reasonFormat
		}
		img := samplesToImage(sd.Content, w, h, comps)
		if looksLikeAGraphic(img) {
			return nil, reasonGraphic
		}
		src = img
	default:
		return nil, reasonFormat
	}

	// The size: the longest side is brought down to the rung's, never up.
	nw, nh := w, h
	if longest := max(w, h); longest > lv.MaxEdge {
		nw = max(1, w*lv.MaxEdge/longest)
		nh = max(1, h*lv.MaxEdge/longest)
	}
	var dst image.Image = src
	if nw != w || nh != h {
		if comps == 1 {
			g := image.NewGray(image.Rect(0, 0, nw, nh))
			draw.CatmullRom.Scale(g, g.Bounds(), src, src.Bounds(), draw.Src, nil)
			dst = g
		} else {
			r := image.NewRGBA(image.Rect(0, 0, nw, nh))
			draw.CatmullRom.Scale(r, r.Bounds(), src, src.Bounds(), draw.Src, nil)
			dst = r
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: lv.Quality}); err != nil {
		return nil, reasonSize
	}
	was := len(sd.Raw)        // what is stored now: the JPEG, or the Flate stream (Decode leaves Raw alone)
	if buf.Len()*10 > was*9 { // not at least 10 % smaller
		return nil, reasonNotSmaller
	}

	n := *sd
	n.Raw = buf.Bytes()
	length := int64(len(n.Raw))
	n.StreamLength = &length
	n.Content = nil
	n.FilterPipeline = []types.PDFFilter{{Name: "DCTDecode"}}
	n.Dict = d.Clone().(types.Dict)
	n.Dict["Filter"] = types.Name("DCTDecode")
	delete(n.Dict, "DecodeParms")
	n.Dict["Width"] = types.Integer(nw)
	n.Dict["Height"] = types.Integer(nh)
	n.Dict["BitsPerComponent"] = types.Integer(8)
	n.Dict["Length"] = types.Integer(length)
	return &n, ""
}

// pipelineOf names the single filter an image stream carries ("" for none, "multi" for a chain).
func pipelineOf(sd *types.StreamDict) string {
	switch len(sd.FilterPipeline) {
	case 0:
		return ""
	case 1:
		return sd.FilterPipeline[0].Name
	}
	return "multi"
}

func intEntry(d types.Dict, key string) int {
	if v := d.IntEntry(key); v != nil {
		return *v
	}
	return 0
}

// imageComponents is the number of colour components a picture's /ColorSpace gives its samples: 1 for Gray (or an
// ICC profile of one), 3 for RGB (or an ICC profile of three), and 0 for anything this does not re-encode.
func imageComponents(xt *model.XRefTable, cs types.Object) int {
	cs, _ = xt.Dereference(cs)
	switch v := cs.(type) {
	case types.Name:
		switch v {
		case "DeviceGray", "CalGray":
			return 1
		case "DeviceRGB", "CalRGB":
			return 3
		}
	case types.Array:
		if len(v) == 0 {
			return 0
		}
		head, _ := v[0].(types.Name)
		switch head {
		case "CalGray":
			return 1
		case "CalRGB":
			return 3
		case "ICCBased":
			if len(v) < 2 {
				return 0
			}
			if prof, _, _ := xt.DereferenceStreamDict(v[1]); prof != nil {
				if n := prof.Dict.IntEntry("N"); n != nil && (*n == 1 || *n == 3) {
					return *n
				}
			}
		}
	}
	return 0
}

// samplesToImage reads w*h*comps interleaved 8-bit samples.
func samplesToImage(b []byte, w, h, comps int) image.Image {
	if comps == 1 {
		g := image.NewGray(image.Rect(0, 0, w, h))
		copy(g.Pix, b)
		return g
	}
	r := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		r.Pix[4*i], r.Pix[4*i+1], r.Pix[4*i+2], r.Pix[4*i+3] = b[3*i], b[3*i+1], b[3*i+2], 255
	}
	return r
}

// looksLikeAGraphic reports whether an uncompressed picture has so few distinct colours that a JPEG would only blur
// its edges and shift its flat colours: a logo, a chart, a screenshot. Sampled, so a huge picture costs the same
// as a small one; more than 256 distinct colours in the sample is a photograph.
func looksLikeAGraphic(img image.Image) bool {
	b := img.Bounds()
	step := max(1, (b.Dx()*b.Dy())/65536)
	seen := map[color.RGBA]struct{}{}
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if n++; n%step != 0 {
				continue
			}
			r, g, bl, _ := img.At(x, y).RGBA()
			seen[color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255}] = struct{}{}
			if len(seen) > 256 {
				return false
			}
		}
	}
	return true
}
