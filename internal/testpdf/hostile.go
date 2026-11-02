package testpdf

import (
	"archive/zip"
	"bytes"
	"fmt"
	goimage "image"
	"image/png"
	"math/rand"
)

// The size-targeted inputs the import door's caps and ceilings are measured with (PLAN-import-export
// X3, `build/genpdf.go`'s shape): each generator takes the size it must reach and PANICS if its output is
// short, so a measurement is never taken on an input smaller than the one it claims to bound. Nothing
// binary is committed; every fixture is built in the test.

// PackageOfSize is a Word package of at least target bytes: one paragraph, and a stored random blob that
// nothing references. It exercises what Nib holds in memory and writes to disk for a large source,
// without making the converter lay anything out.
func PackageOfSize(target int64) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	base := MinimalDocx("a very large package")
	br, _ := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	for _, f := range br.File {
		rc, _ := f.Open()
		w, _ := zw.Create(f.Name)
		var b bytes.Buffer
		b.ReadFrom(rc)
		body := b.Bytes()
		if f.Name == "[Content_Types].xml" { // the blob's extension needs a type, or the package is invalid
			body = bytes.Replace(body, []byte(`<Default Extension="xml"`), []byte(`<Default Extension="bin" ContentType="application/octet-stream"/><Default Extension="xml"`), 1)
		}
		w.Write(body)
		rc.Close()
	}
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "word/media/blob.bin", Method: zip.Store})
	if err != nil {
		panic(err)
	}
	rng := rand.New(rand.NewSource(1))
	chunk := make([]byte, 1<<20)
	for written := int64(buf.Len()); written < target; written += int64(len(chunk)) {
		rng.Read(chunk)
		w.Write(chunk)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	if int64(buf.Len()) < target {
		panic(fmt.Sprintf("PackageOfSize: built %d bytes, asked for %d", buf.Len(), target))
	}
	return buf.Bytes()
}

// DocxWithImages is a Word document placing n distinct noise PNGs of side×side pixels, each inline, so a
// converter has to decode and lay out all of them. It panics if the document holds fewer than n images.
func DocxWithImages(n, side int) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, body []byte) {
		w, err := zw.Create(name)
		if err != nil {
			panic(err)
		}
		w.Write(body)
	}
	add("[Content_Types].xml", []byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`))
	add("_rels/.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`))
	var rels, body bytes.Buffer
	rels.WriteString(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < n; i++ {
		img := goimage.NewRGBA(goimage.Rect(0, 0, side, side))
		rng.Read(img.Pix)
		for p := 3; p < len(img.Pix); p += 4 {
			img.Pix[p] = 255
		}
		var pb bytes.Buffer
		if err := png.Encode(&pb, img); err != nil {
			panic(err)
		}
		add(fmt.Sprintf("word/media/i%d.png", i), pb.Bytes())
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/i%d.png"/>`, i+10, i)
		fmt.Fprintf(&body, `<w:p><w:r><w:drawing><wp:inline><wp:extent cx="1800000" cy="1800000"/><wp:docPr id="%d" name="p%d"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="%d" name="p%d"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rId%d"/></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1800000" cy="1800000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`, i+1, i, i+1, i, i+10)
	}
	rels.WriteString(`</Relationships>`)
	add("word/_rels/document.xml.rels", rels.Bytes())
	add("word/document.xml", []byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>`+body.String()+`</w:body></w:document>`))
	if err := zw.Close(); err != nil {
		panic(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	count := 0
	for _, f := range zr.File {
		if len(f.Name) > 11 && f.Name[:11] == "word/media/" {
			count++
		}
	}
	if count < n {
		panic(fmt.Sprintf("DocxWithImages: %d images built, asked for %d", count, n))
	}
	return buf.Bytes()
}
