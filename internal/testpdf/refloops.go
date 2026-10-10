package testpdf

import (
	"fmt"
	"strings"
)

// Documents whose references loop through an edge pdfcpu's validator follows without a guard — `/pending 764`,
// ADR-069. Each comes as a pair: Loop, whose reference returns to an object on the chain (pdfcpu recurses on it
// until the process dies), and Ends, the same document with that one reference pointing at an object that ends
// the chain. Ends validating is what shows the pair differs only in the loop.

// RefLoop is one such pair; Edge names the chain.
type RefLoop struct {
	Edge       string
	Loop, Ends []byte
}

// onePage is a one-page document with res as the page's /Resources body, annots as its /Annots body (none when
// empty), extra as more catalog entries, and objs from object 10 on.
func onePage(res, annots, extra string, objs map[int]string) []byte {
	page := "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << " + res + " >>"
	if annots != "" {
		page += " /Annots [" + annots + "]"
	}
	o := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R " + extra + " >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: page + " >>",
		4: "<< /Length 0 >>\nstream\n\nendstream",
	}
	for n, b := range objs {
		o[n] = b
	}
	return assemble(o)
}

// pair builds Loop with @X replaced by loop and Ends with @X replaced by ends, in every object body.
func pair(edge, loop, ends string, build func(x func(string) string) []byte) RefLoop {
	sub := func(v string) func(string) string {
		return func(s string) string { return strings.ReplaceAll(s, "@X", v) }
	}
	return RefLoop{Edge: edge, Loop: build(sub(loop)), Ends: build(sub(ends))}
}

func tiling(res string) string {
	return "<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 /BBox [0 0 1 1] /XStep 1 /YStep 1 " +
		"/Resources << " + res + " >> /Length 0 >>\nstream\n\nendstream"
}

func image(extra string) string {
	return "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace /DeviceGray /BitsPerComponent 8 " +
		extra + " /Length 1 >>\nstream\n\x00\nendstream"
}

const (
	fnGray = "<< /FunctionType 2 /Domain [0 1] /C0 [0] /C1 [1] /N 1 >>"
	fnRGB  = "<< /FunctionType 2 /Domain [0 1] /C0 [0 0 0] /C1 [1 1 1] /N 1 >>"
	annot  = "/Type /Annot /Rect [0 0 10 10] "
)

// RefLoops returns one pair per edge of the reference door's table.
func RefLoops() []RefLoop {
	return []RefLoop{
		pair("tiling pattern /Resources /Pattern", "10 0 R", "11 0 R", func(x func(string) string) []byte {
			return onePage("/Pattern << /P 10 0 R >>", "", "", map[int]string{
				10: x(tiling("/Pattern << /Q @X >>")), 11: tiling(""),
			})
		}),
		pair("property list /Resources /Properties", "10 0 R", "11 0 R", func(x func(string) string) []byte {
			return onePage("/Properties << /MC 10 0 R >>", "", "", map[int]string{
				10: x("<< /Resources << /Properties << /N @X >> >> >>"), 11: "<< /Metadata 12 0 R >>",
				12: "<< /Type /Metadata /Subtype /XML /Length 0 >>\nstream\n\nendstream",
			})
		}),
		pair("stitching function /Functions", "11 0 R", "12 0 R", func(x func(string) string) []byte {
			return onePage("/Shading << /Sh 10 0 R >>", "", "", map[int]string{
				10: "<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 1 1] /Function 11 0 R >>",
				11: x("<< /FunctionType 3 /Domain [0 1] /Functions [@X] /Bounds [] /Encode [0 1] >>"), 12: fnRGB,
			})
		}),
		pair("Indexed base", "10 0 R", "/DeviceRGB", func(x func(string) string) []byte {
			return onePage("/ColorSpace << /CS 10 0 R >>", "", "", map[int]string{10: x("[/Indexed @X 1 <000000FFFFFF>]")})
		}),
		pair("Pattern colour space base", "10 0 R", "/DeviceRGB", func(x func(string) string) []byte {
			return onePage("/ColorSpace << /CS 10 0 R >>", "", "", map[int]string{10: x("[/Pattern @X]")})
		}),
		pair("Separation alternate", "10 0 R", "/DeviceGray", func(x func(string) string) []byte {
			return onePage("/ColorSpace << /CS 10 0 R >>", "", "", map[int]string{
				10: x("[/Separation /Spot @X 12 0 R]"), 12: fnGray,
			})
		}),
		pair("DeviceN alternate", "10 0 R", "/DeviceGray", func(x func(string) string) []byte {
			return onePage("/ColorSpace << /CS 10 0 R >>", "", "", map[int]string{
				10: x("[/DeviceN [/A] @X 12 0 R]"), 12: fnGray,
			})
		}),
		pair("DeviceN /Colorants", "10 0 R", "/DeviceGray", func(x func(string) string) []byte {
			return onePage("/ColorSpace << /CS 10 0 R >>", "", "", map[int]string{
				10: "[/DeviceN [/A] /DeviceGray 12 0 R 11 0 R]", 11: "<< /Colorants << /A 13 0 R >> >>",
				12: fnGray, 13: x("[/Separation /A @X 12 0 R]"),
			})
		}),
		pair("DeviceN /Process /ColorSpace", "10 0 R", "/DeviceGray", func(x func(string) string) []byte {
			return onePage("/ColorSpace << /CS 10 0 R >>", "", "", map[int]string{
				10: "[/DeviceN [/A] /DeviceGray 12 0 R 11 0 R]", 11: "<< /Process 14 0 R >>",
				12: fnGray, 14: x("<< /ColorSpace @X /Components [/A] >>"),
			})
		}),
		pair("type 5 halftone", "11 0 R", "12 0 R", func(x func(string) string) []byte {
			return onePage("/ExtGState << /G 10 0 R >>", "", "", map[int]string{
				10: "<< /Type /ExtGState /HT 11 0 R >>", 11: x("<< /HalftoneType 5 /Default @X >>"),
				12: "<< /HalftoneType 1 /Frequency 60 /Angle 45 /SpotFunction /Round >>",
			})
		}),
		pair("image /Mask", "/Mask 10 0 R", "", func(x func(string) string) []byte {
			return onePage("/XObject << /Im 10 0 R >>", "", "", map[int]string{
				10: image("/Mask 11 0 R"), 11: x(image("@X")),
			})
		}),
		pair("image /Alternates", "/Mask 10 0 R", "", func(x func(string) string) []byte {
			return onePage("/XObject << /Im 10 0 R >>", "", "", map[int]string{
				10: image("/Alternates [11 0 R]"), 11: x(image("@X")),
			})
		}),
		pair("action /Next", "10 0 R", "11 0 R", func(x func(string) string) []byte {
			return onePage("", "", "/OpenAction 10 0 R", map[int]string{
				10: x("<< /S /JavaScript /JS (1) /Next @X >>"), 11: "<< /S /JavaScript /JS (2) >>",
			})
		}),
		pair("action /Next array", "10 0 R", "11 0 R", func(x func(string) string) []byte {
			return onePage("", "", "/OpenAction 10 0 R", map[int]string{
				10: x("<< /S /JavaScript /JS (1) /Next [@X] >>"), 11: "<< /S /JavaScript /JS (2) >>",
			})
		}),
		pair("GoToE target /T", "11 0 R", "<< /R /P >>", func(x func(string) string) []byte {
			return onePage("", "", "/OpenAction 10 0 R", map[int]string{
				10: "<< /S /GoToE /D (x) /T 11 0 R >>", 11: x("<< /R /C /N (a) /T @X >>"),
			})
		}),
		pair("GoTo3DView /TA through Link /A", "10 0 R", "12 0 R", func(x func(string) string) []byte {
			return onePage("", "10 0 R", "", map[int]string{
				10: "<< " + annot + "/Subtype /Link /A 11 0 R >>", 11: x("<< /S /GoTo3DView /TA @X /V /D >>"),
				12: "<< " + annot + "/Subtype /Text >>",
			})
		}),
		pair("markup /IRT", "/IRT 10 0 R", "", func(x func(string) string) []byte {
			return onePage("", "10 0 R", "", map[int]string{
				10: "<< " + annot + "/Subtype /Text /IRT 11 0 R >>", 11: x("<< " + annot + "/Subtype /Text @X >>"),
			})
		}),
		pair("Screen /A", "10 0 R", "12 0 R", func(x func(string) string) []byte {
			return onePage("", "10 0 R", "", map[int]string{
				10: "<< " + annot + "/Subtype /Screen /A 11 0 R >>", 11: x("<< /S /GoTo3DView /TA @X /V /D >>"),
				12: "<< " + annot + "/Subtype /Text >>",
			})
		}),
		pair("Screen /AA", "10 0 R", "12 0 R", func(x func(string) string) []byte {
			return onePage("", "10 0 R", "", map[int]string{
				10: "<< " + annot + "/Subtype /Screen /AA << /E 11 0 R >> >>", 11: x("<< /S /GoTo3DView /TA @X /V /D >>"),
				12: "<< " + annot + "/Subtype /Text >>",
			})
		}),
		pair("selector rendition /R", "11 0 R", "12 0 R", func(x func(string) string) []byte {
			return onePage("", "20 0 R", "/OpenAction 10 0 R", map[int]string{
				20: "<< " + annot + "/Subtype /Screen >>", 10: "<< /S /Rendition /OP 0 /AN 20 0 R /R 11 0 R >>", 11: x("<< /Type /Rendition /S /SR /R [@X] >>"),
				12: "<< /Type /Rendition /S /MR >>",
			})
		}),
		pair("media clip section /D", "13 0 R", "14 0 R", func(x func(string) string) []byte {
			return onePage("", "20 0 R", "/OpenAction 10 0 R", map[int]string{
				20: "<< " + annot + "/Subtype /Screen >>", 10: "<< /S /Rendition /OP 0 /AN 20 0 R /R 11 0 R >>", 11: "<< /Type /Rendition /S /MR /C 13 0 R >>",
				13: x("<< /Type /MediaClip /S /MCS /D @X >>"),
				14: "<< /Type /MediaClip /S /MCD /CT (video/mp4) /D << /Type /Filespec /F (x.mp4) >> >>",
			})
		}),
	}
}

// SharedPatterns is a page drawing tiling patterns each naming the next twice, depth deep: no loop, and 2^depth
// paths for a validator that walks each one.
func SharedPatterns(depth int) []byte {
	objs := map[int]string{}
	for k := 0; k < depth; k++ {
		res := ""
		if k+1 < depth {
			res = fmt.Sprintf("/Pattern << /A %d 0 R /B %d 0 R >>", 11+k, 11+k)
		}
		objs[10+k] = tiling(res)
	}
	return onePage("/Pattern << /P 10 0 R >>", "", "", objs)
}

// ChainedPatterns is a page drawing a chain of n tiling patterns, each naming the next once.
func ChainedPatterns(n int) []byte {
	objs := map[int]string{}
	for k := 0; k < n; k++ {
		res := ""
		if k+1 < n {
			res = fmt.Sprintf("/Pattern << /A %d 0 R >>", 11+k)
		}
		objs[10+k] = tiling(res)
	}
	return onePage("/Pattern << /P 10 0 R >>", "", "", objs)
}

// FanInEntries are the ways a page's /Resources reaches a function, each with @F where the function goes. pdfcpu
// validates a page's resources once per page, so a function shared by every page is walked once per page.
var FanInEntries = map[string]string{
	"ExtGState /TR":             "/ExtGState << /G << /Type /ExtGState /TR @F >> >>",
	"ExtGState /SMask /TR":      "/ExtGState << /G << /Type /ExtGState /SMask << /S /Luminosity /G 20 0 R /TR @F >> >> >>",
	"Shading /Function":         "/Shading << /Sh << /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 1 1] /Function @F >> >>",
	"shading Pattern /Shading":  "/Pattern << /P << /PatternType 2 /Shading << /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 1 1] /Function @F >> >> >>",
	"Separation tint transform": "/ColorSpace << /CS [/Separation /S /DeviceGray @F] >>",
	"DeviceN tint transform":    "/ColorSpace << /CS [/DeviceN [/A] /DeviceGray @F] >>",
	"DeviceN /Process":          "/ColorSpace << /CS [/DeviceN [/A] /DeviceGray 21 0 R << /Process << /ColorSpace [/Separation /S /DeviceGray @F] /Components [/A] >> >>] >>",
	// /pending 767: the transfer keys after /TR, and a shading pattern's own /ExtGState.
	"ExtGState /TR2":             "/ExtGState << /G << /Type /ExtGState /TR2 @F >> >>",
	"ExtGState /BG":              "/ExtGState << /G << /Type /ExtGState /BG @F >> >>",
	"ExtGState /BG2":             "/ExtGState << /G << /Type /ExtGState /BG2 @F >> >>",
	"ExtGState /UCR":             "/ExtGState << /G << /Type /ExtGState /UCR @F >> >>",
	"ExtGState /UCR2":            "/ExtGState << /G << /Type /ExtGState /UCR2 @F >> >>",
	"shading Pattern /ExtGState": "/Pattern << /P << /PatternType 2 /Shading << /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 1 1] /Function 21 0 R >> /ExtGState << /Type /ExtGState /TR @F >> >> >>",
}

// FanInShape is a document reaching one shared structure through Edge: Small once and shallowly, which reads; Large
// often enough that pdfcpu's validator, which walks the structure afresh from every naming, is past the path budget.
type FanInShape struct {
	Edge         string
	Small, Large []byte
}

// FanInShapes are the entry edges whose shared structure is not a function reached from a page's /Resources, so
// `FanIn` cannot build them — /pending 767. Each structure is a chain whose links name the next twice (2^depth
// paths); without its edge the door counts that chain once, from the chain's own shape, and passes Large.
func FanInShapes() []FanInShape {
	action := func(next int) string {
		return fmt.Sprintf("<< /S /JavaScript /JS (1) /Next [%d 0 R %d 0 R] >>", next, next)
	}
	halftone := func(next int) string {
		return fmt.Sprintf("<< /Type /Halftone /HalftoneType 5 /Default %d 0 R /Gray %d 0 R >>", next, next)
	}
	stitch := func(next int) string {
		return fmt.Sprintf("<< /FunctionType 3 /Domain [0 1] /Functions [%d 0 R %d 0 R] /Bounds [0.5] /Encode [0 1 0 1] >>", next, next)
	}
	selector := func(next int) string {
		return fmt.Sprintf("<< /Type /Rendition /S /SR /R [%d 0 R %d 0 R] >>", next, next)
	}
	// each adds naming i's objects to o and returns its page's entries, or "" when the naming is not a page.
	build := func(edge string, each func(i int, o map[int]string) string, link func(int) string, end string) FanInShape {
		doc := func(namings, depth int) []byte {
			o := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 4: "<< /Length 0 >>\nstream\n\nendstream"}
			for k := 0; k < depth; k++ {
				if k+1 < depth {
					o[30+k] = link(31 + k)
				} else {
					o[30+k] = end
				}
			}
			var kids []string
			for i := 0; i < namings; i++ {
				page := each(i, o)
				if page == "" {
					continue
				}
				kids = append(kids, fmt.Sprintf("%d 0 R", 100+i))
				o[100+i] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R " + page + " >>"
			}
			o[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))
			return assemble(o)
		}
		return FanInShape{Edge: edge, Small: doc(1, 3), Large: doc(400, 12)}
	}
	return []FanInShape{
		// Every page lists the one annotation: pdfcpu validates a page's /Annots once per page.
		build("page /Annots", func(_ int, o map[int]string) string {
			o[20] = "<< " + annot + "/Subtype /Link /A 30 0 R >>"
			return "/Annots [20 0 R]"
		}, action, "<< /S /JavaScript /JS (2) >>"),
		build("ExtGState /HT", func(int, map[int]string) string {
			return "/Resources << /ExtGState << /G << /Type /ExtGState /HT 30 0 R >> >> >>"
		}, halftone, "<< /Type /Halftone /HalftoneType 1 /Frequency 60 /Angle 45 /SpotFunction /Round >>"),
		// One page, many images: pdfcpu validates each image once, and its colour space from it.
		build("image /ColorSpace", func(i int, o map[int]string) string {
			o[1000+i] = "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace [/Separation /S /DeviceGray 30 0 R] " +
				"/BitsPerComponent 8 /Length 1 >>\nstream\n\x00\nendstream"
			o[21] = strings.TrimSuffix(o[21], ">>") + fmt.Sprintf("/I%d %d 0 R >>", i, 1000+i)
			if i > 0 {
				return ""
			}
			o[21] = "<< " + o[21]
			return "/Resources << /XObject 21 0 R >>"
		}, stitch, fnGray),
		build("Rendition action /R", func(_ int, o map[int]string) string {
			o[20] = "<< " + annot + "/Subtype /Screen >>"
			return "/Annots [20 0 R] /AA << /O << /S /Rendition /OP 0 /AN 20 0 R /R 30 0 R >> >>"
		}, selector, "<< /Type /Rendition /S /MR >>"),
	}
}

// AssembleCompressed is `Assemble` with the objects numbered in packed held in one object stream, under a
// cross-reference stream — where pdfcpu's read leaves them undecoded until something names them.
func AssembleCompressed(objs map[int]string, packed ...int) []byte {
	in := map[int]int{}
	for i, nr := range packed {
		in[nr] = i + 1
	}
	top := 0
	for nr := range objs {
		top = max(top, nr)
	}
	stm, xr := top+1, top+2
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	off := map[int]int{}
	for nr := 1; nr <= top; nr++ {
		if body, ok := objs[nr]; ok && in[nr] == 0 {
			off[nr] = b.Len()
			fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", nr, body)
		}
	}
	var head, body strings.Builder
	for _, nr := range packed {
		fmt.Fprintf(&head, "%d %d ", nr, body.Len())
		body.WriteString(objs[nr] + "\n")
	}
	off[stm] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<< /Type /ObjStm /N %d /First %d /Length %d >>\nstream\n%s%s\nendstream\nendobj\n",
		stm, len(packed), head.Len(), head.Len()+body.Len(), head.String(), body.String())
	off[xr] = b.Len()
	var x []byte
	for nr := 0; nr <= xr; nr++ {
		switch o := off[nr]; {
		case in[nr] > 0:
			i := in[nr] - 1
			x = append(x, 2, byte(stm>>24), byte(stm>>16), byte(stm>>8), byte(stm), byte(i>>8), byte(i))
		case o > 0:
			x = append(x, 1, byte(o>>24), byte(o>>16), byte(o>>8), byte(o), 0, 0)
		default:
			x = append(x, 0, 0, 0, 0, 0, 255, 255)
		}
	}
	fmt.Fprintf(&b, "%d 0 obj\n<< /Type /XRef /Size %d /W [1 4 2] /Root 1 0 R /Length %d >>\nstream\n", xr, xr+1, len(x))
	b.Write(x)
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", off[xr])
	return []byte(b.String())
}

// FanIn is pages pages whose /Resources reach, through entry, one shared stitching function whose two
// sub-functions are the same function, depth levels deep: 2^depth paths from the function, once per page.
func FanIn(entry string, pages, depth int) []byte {
	o := map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R >>",
		4:  "<< /Length 0 >>\nstream\n\nendstream",
		20: "<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] /Group << /S /Transparency /CS /DeviceGray >> /Length 0 >>\nstream\n\nendstream",
		21: fnGray,
	}
	for k := 0; k < depth; k++ {
		if k+1 < depth {
			o[30+k] = fmt.Sprintf("<< /FunctionType 3 /Domain [0 1] /Functions [%d 0 R %d 0 R] /Bounds [0.5] /Encode [0 1 0 1] >>", 31+k, 31+k)
		} else {
			o[30+k] = fnGray
		}
	}
	res := strings.ReplaceAll(entry, "@F", "30 0 R")
	var kids []string
	for p := 0; p < pages; p++ {
		n := 100 + p
		kids = append(kids, fmt.Sprintf("%d 0 R", n))
		o[n] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << " + res + " >> >>"
	}
	o[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pages)
	return assemble(o)
}

// SharedNameTree is a JavaScript name tree whose nodes each list the next twice, depth deep: no loop, and 2^depth
// paths for a validator that walks each one. pdfcpu caps a tree's depth, never its breadth.
func SharedNameTree(depth int) []byte {
	objs := map[int]string{10: "<< /Kids [11 0 R] >>", 11 + depth: "<< /Limits [(a) (a)] /Names [(a) 9 0 R] >>",
		9: "<< /S /JavaScript /JS (1) >>"}
	for k := 0; k < depth; k++ {
		objs[11+k] = fmt.Sprintf("<< /Limits [(a) (a)] /Kids [%d 0 R %d 0 R] >>", 12+k, 12+k)
	}
	return onePage("", "", "/Names << /JavaScript 10 0 R >>", objs)
}

// SelfSeparationImage is /pending 614's reported shape: an image whose /ColorSpace is a Separation naming itself
// as its alternate, with a tint transform that is not a function — found killing the process through PreparePDFA
// and through the ceremony arrival gate's record read.
func SelfSeparationImage() []byte {
	return onePage("/XObject << /Im 11 0 R >>", "", "", map[int]string{
		10: "[/Separation /X 10 0 R 0]",
		11: "<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /ColorSpace 10 0 R /BitsPerComponent 8 /Length 1 >>\nstream\n\x00\nendstream",
	})
}
