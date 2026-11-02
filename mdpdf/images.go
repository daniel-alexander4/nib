package mdpdf

import (
	"fmt"
	"image"
	_ "image/jpeg" // the decoders image.DecodeConfig reads a source's size with
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
)

const (
	maxImageBytes  = 32 << 20  // one image file
	maxImagePixels = 100 << 20 // one image's width x height
	maxImages      = 100       // pictures in one document
	imagePtPerPx   = 0.75      // 96 dpi, as nib's own image documents (internal/pdfops/imagedoc.go)
	imageGap       = 8.0
)

// ImageRefusedError is an image the document names and the conversion will not read, with the source exactly as the
// template spelled it (template text, never a supplied value) and why.
type ImageRefusedError struct{ Src, Why string }

func (e *ImageRefusedError) Error() string { return fmt.Sprintf("image %q: %s", e.Src, e.Why) }

// ErrTooManyImages is a document naming more pictures than one conversion embeds.
var ErrTooManyImages = fmt.Errorf("a document names more than %d images", maxImages)

// figure is one picture placed on a page: where, how big, from which file, and the alternative text a Figure element
// needs ("" is decorative: an artifact, never a Figure with no description).
type figure struct {
	x, y, w, h float64 // lower-left corner and size, points
	path       string
	alt        string
	before     int // the number of text runs already drawn on its page: where it sits in reading order
}

// Figure is a picture's place in a page's reading order, as the tagger needs it: Before is how many of the page's text
// runs precede it, Alt its description ("" = decorative).
type Figure struct {
	Before int
	Alt    string
}

// resolveImage turns an image destination written in a template into a file under dir, or refuses it. Only a relative
// path that stays under dir (symlinks resolved on both sides) and names a regular PNG or JPEG file of a bounded size.
// A URL, a data: or file: reference, an absolute path and any `..` are refused by name, so a template cannot read or
// reach what its author did not put beside it.
func resolveImage(dir, src string) (path string, w, h int, err error) {
	refuse := func(why string) (string, int, int, error) { return "", 0, 0, &ImageRefusedError{src, why} }
	switch {
	case src == "":
		return refuse("it has no source")
	case strings.Contains(src, "\x00"):
		return refuse("its path is not a file name")
	case strings.Contains(src, "://") || strings.HasPrefix(src, "//") || strings.HasPrefix(strings.ToLower(src), "data:") || strings.HasPrefix(strings.ToLower(src), "file:"):
		return refuse("it is a link, and a conversion reads no network or other scheme; give a file beside the document")
	case filepath.IsAbs(src) || strings.HasPrefix(src, "/") || strings.HasPrefix(src, `\`) || (len(src) > 1 && src[1] == ':'):
		return refuse("it is an absolute path; give a path relative to the image folder")
	}
	clean := filepath.Clean(filepath.FromSlash(src))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return refuse("it climbs out of the image folder")
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return refuse("the image folder cannot be read")
	}
	full, err := filepath.EvalSymlinks(filepath.Join(root, clean))
	if err != nil {
		return refuse("there is no such file")
	}
	if rel, rerr := filepath.Rel(root, full); rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return refuse("it resolves, through a link, outside the image folder")
	}
	fi, err := os.Stat(full)
	if err != nil || !fi.Mode().IsRegular() {
		return refuse("it is not a regular file")
	}
	if fi.Size() > maxImageBytes {
		return refuse(fmt.Sprintf("it is larger than %d bytes", maxImageBytes))
	}
	f, err := os.Open(full)
	if err != nil {
		return refuse("it cannot be read")
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || (format != "png" && format != "jpeg") {
		return refuse("it is not a PNG or JPEG picture")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return refuse(fmt.Sprintf("it is larger than %d pixels", maxImagePixels))
	}
	return full, cfg.Width, cfg.Height, nil
}

// altText is an image's description: the plain text under the node, placeholders filled, whitespace collapsed.
func (r *renderer) altText(n *ast.Image) string {
	var b strings.Builder
	var walk func(ast.Node)
	walk = func(p ast.Node) {
		for c := p.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.Text:
				b.WriteString(r.sub.text(string(t.Segment.Value(r.src))))
				if t.SoftLineBreak() || t.HardLineBreak() {
					b.WriteByte(' ')
				}
			case *ast.String:
				b.WriteString(r.sub.text(string(t.Value)))
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// figures places every image under block n, each as a block of its own after the block's text, when the caller
// asked for images (Options.ImageDir). Without it they are skipped, as they always were.
func (r *renderer) figures(n ast.Node, indent float64) {
	if r.imageDir == "" || r.err != nil {
		return
	}
	var imgs []*ast.Image
	var find func(ast.Node)
	find = func(p ast.Node) {
		for c := p.FirstChild(); c != nil; c = c.NextSibling() {
			if im, ok := c.(*ast.Image); ok {
				imgs = append(imgs, im)
				continue
			}
			find(c)
		}
	}
	find(n)
	for _, im := range imgs {
		if r.nImages++; r.nImages > maxImages {
			r.err = ErrTooManyImages
			return
		}
		path, pw, ph, err := resolveImage(r.imageDir, string(im.Destination))
		if err != nil {
			r.err = err
			return
		}
		r.l.figure(path, float64(pw)*imagePtPerPx, float64(ph)*imagePtPerPx, r.altText(im), indent)
	}
}

// figure puts one picture on the page: scaled DOWN to the content width and to most of a page's height, never up, with
// its aspect kept, on a new page if it does not fit under the text above it.
func (l *layout) figure(path string, w, h float64, alt string, indent float64) {
	if maxW := l.g.contentW() - indent; w > maxW {
		h, w = h*maxW/w, maxW
	}
	if maxH := (l.g.top() - l.g.my) * 0.9; h > maxH {
		w, h = w*maxH/h, maxH
	}
	l.need(h + 2*imageGap)
	l.gap(imageGap)
	bottom := l.y - h
	pg := len(l.figs) - 1
	l.figs[pg] = append(l.figs[pg], figure{x: l.g.mx + indent, y: bottom, w: w, h: h, path: path, alt: alt, before: len(l.runs[pg])})
	l.y = bottom - imageGap
}
