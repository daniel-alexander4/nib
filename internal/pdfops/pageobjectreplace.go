package pdfops

import (
	"crypto/sha256"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// Replacing a picture (PLAN-image-edit P03.S01, D9). The replacement arrives as `Import`'s one-page PDF of one
// raster (the caller imports it, under `Only: RasterFormatKeys()`), so the image to lift is always one pdfcpu's
// `ImportImages` built: one image stream with direct values and at most an `/SMask` stream. The old picture's
// painting operator becomes a `Do` of the lifted image under a fresh name in the page's own `/Resources`; its `q`,
// any Figure bracket and, where it cannot be moved, its `cm` are kept. The old image is pruned as a removal prunes it.

// ObjectEditReplace swaps a picture for the image in `ObjectEdit.With`.
const ObjectEditReplace = "replace"

// ReplaceMaxPixels is a replacement picture's own ceiling: every phone and most camera photos, a quarter of what an
// import opens. Measured through the producer (P03.S01): a PNG of 12 / 24 / 48 MP costs 2.7 / 4.1 / 6.5 s and
// allocates 317 MB / 794 MB / 1.28 GB; a JPEG of the same sizes 3-7 ms, its bytes carried as they came.
const ReplaceMaxPixels = 50_000_000

// ReplacementImport is how the route and `nib objects replace` import a replacement: only a raster Nib reads itself,
// one page, under ReplaceMaxPixels. One home, so the two cannot admit different pictures.
func ReplacementImport(door string) ImportOptions {
	return ImportOptions{Door: door, Only: RasterFormatKeys(), Single: true, MaxPixels: ReplaceMaxPixels}
}

// The causes only a replace gives.
const (
	// WhyImageMask: the picture is a stencil painted in the fill colour; a picture in its place would mean something
	// else.
	WhyImageMask = "image-mask"
	// WhyObjectReferenced: the image object itself is referred to from the structure (`/StructParent`), which a
	// removed object would leave pointing at nothing.
	WhyObjectReferenced = "object-referenced"
	// WhyOrientation: the replacement is a photo turned by its EXIF orientation and this picture's placement cannot
	// be rewritten to turn it back.
	WhyOrientation = "orientation"
	// CauseAltUnanswered: the picture's tag describes it, and the edit says neither a new description nor to keep it.
	CauseAltUnanswered = "alt-unanswered"
	// CauseNotOneImage: the replacement is not one picture.
	CauseNotOneImage = "not-one-image"
)

// ReplacedObject is what a replace did: the address before it, the box the new picture reads in, whether it was
// fitted inside the old box keeping its proportion or stretched to it, the quarter turns undone, and the
// description the picture's tag now holds.
type ReplacedObject struct {
	Page      int     `json:"page"`
	Ordinal   int     `json:"ordinal"`
	Box       MapRect `json:"box"`
	Fitted    bool    `json:"fitted"`
	Stretched bool    `json:"stretched"`
	Turned    int     `json:"turned"`
	Alt       string  `json:"alt"`
}

// replacedWant is what the after-check holds a replaced slot to: its box, a `Do` of the new name, and that name
// resolving to the lifted stream's bytes.
type replacedWant struct {
	box     [4]float64
	painter [32]byte
	raw     [32]byte
}

// liftedImage is the replacement copied into the edited document.
type liftedImage struct {
	ref    types.IndirectRef
	w, h   int
	rotate int
	raw    [32]byte
}

// replaceImageKeys are the image dictionary entries a lift carries; the rest are dropped. Only `/SMask` may be a
// reference, and only on the picture itself.
var replaceImageKeys = map[string]bool{
	"Type": true, "Subtype": true, "Width": true, "Height": true, "ColorSpace": true, "BitsPerComponent": true,
	"Filter": true, "DecodeParms": true, "Decode": true, "Interpolate": true,
}

// liftImage reads with — Import's one-page PDF — and copies its one image into host as new objects.
func liftImage(host *model.Context, with []byte) (liftedImage, error) {
	notOne := func(why string) (liftedImage, error) {
		return liftedImage{}, &ObjectRefusal{Cause: CauseNotOneImage, Ordinal: -1, Detail: why}
	}
	src, err := pdfread.ReadOptimized(with, model.NewDefaultConfiguration())
	if err != nil {
		return notOne("It does not read as a document.")
	}
	pages := pdfread.Pages(src)
	if len(pages) != 1 || pages[0].Dict == nil {
		return notOne(fmt.Sprintf("It has %d pages.", len(pages)))
	}
	pg := pages[0]
	res := pg.Dict["Resources"]
	rotate := 0
	if pg.Attrs != nil {
		if res == nil {
			res = pg.Attrs.Resources
		}
		rotate = ((pg.Attrs.Rotate % 360) + 360) % 360
	}
	rd, err := src.DereferenceDict(res)
	if err != nil || rd == nil {
		return notOne("Its page names no picture.")
	}
	xo, err := src.DereferenceDict(rd["XObject"])
	if err != nil || len(xo) != 1 {
		return notOne(fmt.Sprintf("Its page names %d pictures.", len(xo)))
	}
	var only types.Object
	for _, v := range xo {
		only = v
	}
	sd, ref, err := copyImageStream(src, host, only, true)
	if err != nil {
		return notOne(err.Error())
	}
	w, _ := sd.Dict["Width"].(types.Integer)
	h, _ := sd.Dict["Height"].(types.Integer)
	if w <= 0 || h <= 0 {
		return notOne("It has no size.")
	}
	return liftedImage{ref: ref, w: int(w), h: int(h), rotate: rotate, raw: sha256.Sum256(sd.Raw)}, nil
}

// copyImageStream copies the image stream o of src into host: the kept entries, the raw (still encoded) bytes, and
// — where mask is true — its `/SMask`, copied the same way and itself holding none. Any other reference is refused.
func copyImageStream(src, host *model.Context, o types.Object, mask bool) (types.StreamDict, types.IndirectRef, error) {
	sd, _, err := src.DereferenceStreamDict(o)
	if err != nil || sd == nil {
		return types.StreamDict{}, types.IndirectRef{}, fmt.Errorf("its picture is not a stream")
	}
	if st, ok := sd.Dict["Subtype"].(types.Name); !ok || st.Value() != "Image" {
		return types.StreamDict{}, types.IndirectRef{}, fmt.Errorf("it names a %v, not a picture", sd.Dict["Subtype"])
	}
	if sd.Raw == nil {
		return types.StreamDict{}, types.IndirectRef{}, fmt.Errorf("its picture's bytes could not be read")
	}
	kept := types.Dict{}
	for k, v := range sd.Dict {
		if !replaceImageKeys[k] {
			continue
		}
		if holdsReference(v, 0) {
			return types.StreamDict{}, types.IndirectRef{}, fmt.Errorf("its picture's %s is held elsewhere", k)
		}
		kept[k] = v
	}
	if m, ok := sd.Dict["SMask"]; ok {
		if !mask {
			return types.StreamDict{}, types.IndirectRef{}, fmt.Errorf("its mask has a mask")
		}
		_, mref, err := copyImageStream(src, host, m, false)
		if err != nil {
			return types.StreamDict{}, types.IndirectRef{}, fmt.Errorf("its mask: %v", err)
		}
		kept["SMask"] = mref
	}
	kept["Length"] = types.Integer(len(sd.Raw))
	c := *sd
	c.Dict = kept
	n := int64(len(sd.Raw))
	c.StreamLength = &n
	c.StreamLengthObjNr = nil
	nr, err := host.XRefTable.InsertObject(c)
	if err != nil {
		return types.StreamDict{}, types.IndirectRef{}, err
	}
	return c, *types.NewIndirectRef(nr, 0), nil
}

// holdsReference says o is, or holds at any depth, an indirect reference: a copied value naming an object of the
// source would name an unrelated object of the host.
func holdsReference(o types.Object, depth int) bool {
	if depth > 16 {
		return true
	}
	switch v := o.(type) {
	case types.IndirectRef:
		return true
	case types.Dict:
		for _, e := range v {
			if holdsReference(e, depth+1) {
				return true
			}
		}
	case types.Array:
		for _, e := range v {
			if holdsReference(e, depth+1) {
				return true
			}
		}
	}
	return false
}

// replaceBlocked is the replace-only cause against picture o, or "": a stencil, or an image the structure refers to.
func replaceBlocked(ctx *model.Context, o PageObject, src []byte) string {
	if o.objNr == 0 {
		if o.span.start >= 0 && o.span.end <= len(src) && inlineMask.Match(src[o.span.start:o.span.end]) {
			return WhyImageMask
		}
		return ""
	}
	sd, _, err := ctx.DereferenceStreamDict(*types.NewIndirectRef(o.objNr, 0))
	if err != nil || sd == nil {
		return WhyUnreadableBox
	}
	if b, ok := sd.Dict["ImageMask"].(types.Boolean); ok && bool(b) {
		return WhyImageMask
	}
	if _, ok := sd.Dict["StructParent"]; ok {
		return WhyObjectReferenced
	}
	return ""
}

// inlineMask finds an inline image's stencil flag in its dictionary (`/IM true` or `/ImageMask true`).
var inlineMask = regexp.MustCompile(`/(IM|ImageMask)\s+true\b`)

// replacementPlacement is where the new picture goes, and the quarter turn it is drawn with. Where the picture can be
// moved it is fitted inside the old box keeping its proportion, turned so it reads upright AS DISPLAYED: the EXIF turn
// Import put on its page's /Rotate, less the turn this page's /Rotate gives it on screen (a scan stored sideways under
// /Rotate 90 is the common case). Otherwise the old `cm` is kept — it already turns its slot the way the page needs —
// the answer says stretched when the drawn proportion is not the picture's, and a photo needing a turn is refused.
func replacementPlacement(im drawnImage, o PageObject, l liftedImage, pageRot int) (box [4]float64, cm []byte, turn int, fitted, stretched bool, why string) {
	pw, ph := float64(l.w), float64(l.h)
	b := im.box
	bw, bh := b[2]-b[0], b[3]-b[1]
	if o.Movable {
		turn = ((l.rotate-pageRot)%360 + 360) % 360
		aw, ah := pw, ph
		if turn == 90 || turn == 270 {
			aw, ah = ah, aw
		}
		a := aw / ah
		tw, th := bw, bh
		if bw/bh > a {
			tw = bh * a
		} else {
			th = bw / a
		}
		x, y := b[0]+(bw-tw)/2, b[1]+(bh-th)/2
		target := [4]float64{x, y, x + tw, y + th}
		c, ok := placementTurned(target, im.outer, turn)
		if !ok {
			return b, nil, 0, false, false, WhyUnreadableBox
		}
		return target, c, turn, true, false, ""
	}
	if l.rotate != 0 {
		return b, nil, 0, false, false, WhyOrientation
	}
	// The drawn proportion is the unit square's two sides under the matrix, whatever way it turns them; a bounding box
	// inverts it for a quarter-turned matrix.
	dw, dh := math.Hypot(im.ctm[0], im.ctm[1]), math.Hypot(im.ctm[2], im.ctm[3])
	if dw <= 0 || dh <= 0 || bw <= 0 || bh <= 0 {
		return b, nil, 0, false, false, WhyUnreadableBox
	}
	return b, nil, 0, false, math.Abs((dw/dh)/(pw/ph)-1) > 0.01, ""
}

// unitTurn is the quarter turn of the unit square, clockwise by deg as /Rotate turns a page for display: a picture
// imported with its EXIF turn on the page's /Rotate is drawn upright under it.
func unitTurn(deg int) runMatrix {
	switch deg {
	case 90:
		return runMatrix{0, -1, 1, 0, 0, 1}
	case 180:
		return runMatrix{-1, 0, 0, -1, 1, 1}
	case 270:
		return runMatrix{0, 1, -1, 0, 1, 0}
	}
	return runMatrix{1, 0, 0, 1, 0, 0}
}

// freshXObjectName is a name no XObject in the page's resources — its own or inherited — has, and none in taken.
func freshXObjectName(ctx *model.Context, pg pdfread.Page, taken map[string]bool) string {
	have := map[string]bool{}
	res := pg.Dict["Resources"]
	if res == nil && pg.Attrs != nil {
		res = pg.Attrs.Resources
	}
	if rd, err := ctx.DereferenceDict(res); err == nil && rd != nil {
		if xo, err := ctx.DereferenceDict(rd["XObject"]); err == nil {
			for k := range xo {
				have[k] = true
			}
		}
	}
	for i := 1; ; i++ {
		n := fmt.Sprintf("NibR%d", i)
		if !have[n] && !taken[n] {
			taken[n] = true
			return n
		}
	}
}

// addXObjectName enters name → ref in the page's `/Resources /XObject`, copying both dictionaries first: the page's
// resources may still be a dictionary it shares — the prune leaves them as found when it cannot read what a page draws
// (`keepUnreadable`), and an inherited one is its parent's — so the entry is written only into copies the page owns.
func addXObjectName(ctx *model.Context, page types.Dict, name string, ref types.IndirectRef) error {
	res := types.Dict{}
	if page["Resources"] != nil {
		d, err := ctx.DereferenceDict(page["Resources"])
		if err != nil || d == nil {
			return fmt.Errorf("the page's resources cannot be read")
		}
		for k, v := range d {
			res[k] = v
		}
	}
	xo := types.Dict{}
	if res["XObject"] != nil {
		d, err := ctx.DereferenceDict(res["XObject"])
		if err != nil || d == nil {
			return fmt.Errorf("the page's pictures cannot be read")
		}
		for k, v := range d {
			xo[k] = v
		}
	}
	xo[name] = ref
	res["XObject"] = xo
	page["Resources"] = res
	return nil
}

// carryLayer gives the lifted image the old image XObject's optional-content membership (`/OC`): a picture in a layer
// — a watermark, a print-only stamp, one switched off — is replaced inside that layer, never made shown for good. The
// value names the host's own group, so it is carried as it is. An inline image's layer is its marked-content
// bracket, which a replace leaves where it is.
func carryLayer(ctx *model.Context, o PageObject, lifted types.IndirectRef) error {
	if o.objNr == 0 {
		return nil
	}
	old, _, err := ctx.DereferenceStreamDict(*types.NewIndirectRef(o.objNr, 0))
	if err != nil || old == nil {
		return err
	}
	oc, ok := old.Dict["OC"]
	if !ok {
		return nil
	}
	e, ok := ctx.XRefTable.FindTableEntryForIndRef(&lifted)
	if !ok || e == nil {
		return fmt.Errorf("the new picture is not in the document")
	}
	sd, ok := e.Object.(types.StreamDict)
	if !ok {
		return fmt.Errorf("the new picture is not a stream")
	}
	sd.Dict["OC"] = oc
	e.Object = sd
	return nil
}

// doOperator is the content a replaced picture is drawn by.
func doOperator(name string) []byte { return []byte("/" + name + " Do") }

// escapedAlt is s as the string a tag's `/Alt` holds.
func escapedAlt(s string) (types.Object, error) {
	esc, err := types.EscapedUTF16String(strings.TrimSpace(s))
	if err != nil {
		return nil, err
	}
	return types.StringLiteral(*esc), nil
}
