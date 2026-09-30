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
