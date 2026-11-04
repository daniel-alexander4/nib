package pdfops

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The saved-page packages (P04.S04b, W34, Y8, ADR-201).

func testPNGBytes(t *testing.T) []byte { return testPNG(t, 3, 2) }

func mhtml(t *testing.T, parts ...string) []byte {
	t.Helper()
	return []byte("From: <Saved by Blink>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/related; type=\"text/html\"; boundary=\"B\"\r\n\r\n" +
		strings.Join(parts, "") + "--B--\r\n")
}

func mpart(headers, body string) string { return "--B\r\n" + headers + "\r\n\r\n" + body + "\r\n" }

func TestAnMHTMLMessageBecomesOnePageWithItsPicturesInline(t *testing.T) {
	png := base64.StdEncoding.EncodeToString(testPNGBytes(t))
	msg := mhtml(t,
		mpart("Content-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\nContent-Location: https://x.test/", `<p>Caf=C3=A9</p><img src=3D"https://x.test/a.png" alt=3D"A"><img src=3D"cid:pic1" alt=3D"B"><img src=3D"https://x.test/missing.png" alt=3D"C">`),
		mpart("Content-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-Location: https://x.test/a.png", png),
		mpart("Content-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <pic1>", png),
	)
	page, resolve, err := mhtmlToHTML(msg)
	if err != nil {
		t.Fatal(err)
	}
	out, st, err := sanitizeHTMLWith(page, resolve)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "Café") || strings.Count(s, "data:image/png;base64,") != 2 || strings.Contains(s, "x.test") {
		t.Errorf("page = %s", s)
	}
	if st.DroppedImages != 1 {
		t.Errorf("DroppedImages = %d, want 1 (the picture the message does not hold)", st.DroppedImages)
	}
}

func TestAPartThatSaysImageButIsNotOneIsNotInlined(t *testing.T) {
	msg := mhtml(t,
		mpart("Content-Type: text/html", `<img src="https://x.test/a.png" alt="A">`),
		mpart("Content-Type: image/png\r\nContent-Location: https://x.test/a.png", "<script>alert(1)</script>"),
	)
	page, resolve, err := mhtmlToHTML(msg)
	if err != nil {
		t.Fatal(err)
	}
	out, _, _ := sanitizeHTMLWith(page, resolve)
	if strings.Contains(string(out), "data:") {
		t.Errorf("a part was inlined on its say-so: %s", out)
	}
}

func TestAnMHTMLMessageIsHeldToItsCaps(t *testing.T) {
	var many []string
	many = append(many, mpart("Content-Type: text/html", "<p>x</p>"))
	for i := 0; i < packageParts+1; i++ {
		many = append(many, mpart(fmt.Sprintf("Content-Type: text/plain\r\nContent-Location: n%d", i), "x"))
	}
	if _, _, err := mhtmlToHTML(mhtml(t, many...)); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("too many parts: %v", err)
	}
	big := strings.Repeat("A", packagePartBytes+8)
	if _, _, err := mhtmlToHTML(mhtml(t, mpart("Content-Type: text/html", big))); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("a part past the cap: %v", err)
	}
	// nesting past the depth cap
	nested := "--B\r\nContent-Type: multipart/related; boundary=\"B\"\r\n\r\n"
	if _, _, err := mhtmlToHTML(mhtml(t, strings.Repeat(nested, packageDepth+2))); err == nil {
		t.Error("deeply nested multipart was read")
	}
	if _, _, err := mhtmlToHTML(mhtml(t, mpart("Content-Type: text/plain", "no page here"))); !errors.Is(err, ErrPackageUnreadable) {
		t.Errorf("no html part: %v", err)
	}
	if _, _, err := mhtmlToHTML([]byte("From: x\r\n" + strings.Repeat("X-Pad: y\r\n", mimeHeaderBytes/8) + "\r\n\r\nbody")); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("a header block past the cap: %v", err)
	}
}

func TestSniffAnMHTMLMessageAndNotAMailMessage(t *testing.T) {
	if !isMHTML(mhtml(t)) {
		t.Error("an MHTML message is not recognised")
	}
	mail := []byte("From: a@b\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n")
	if isMHTML(mail) {
		t.Error("a mixed mail message was claimed as a saved page")
	}
	if f, err := classify(mhtml(t), "page.mht"); err != nil || f.Key != "mhtml" {
		t.Errorf("classify = %v, %v", f.Key, err)
	}
	if _, err := classify(mhtml(t), "page.docx"); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("an MHTML named .docx: %v", err)
	}
	if _, err := classify(mhtml(t), ""); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("a nameless MHTML: %v", err)
	}
}

func epubZip(t *testing.T, mutate func(files map[string]string, order *[]string)) []byte {
	t.Helper()
	files := map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`,
		"OEBPS/content.opf": `<package xmlns="http://www.idpf.org/2007/opf"><manifest>
<item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/><item id="c2" href="text/ch2.xhtml" media-type="application/xhtml+xml"/>
<item id="im" href="img/a.png" media-type="image/png"/></manifest><spine><itemref idref="c1"/><itemref idref="c2"/></spine></package>`,
		"OEBPS/ch1.xhtml":      `<html xmlns="http://www.w3.org/1999/xhtml"><head><style>p{color:red}</style></head><body><h1>Chapter One</h1><p>First words</p><img src="img/a.png" alt="A"/></body></html>`,
		"OEBPS/text/ch2.xhtml": `<html><body><h1>Chapter Two</h1><img src="../img/a.png" alt="B"/><img src="../../../etc/passwd" alt="escape"/></body></html>`,
		"OEBPS/img/a.png":      string(testPNGBytes(t)),
	}
	order := []string{"mimetype", "META-INF/container.xml", "OEBPS/content.opf", "OEBPS/ch1.xhtml", "OEBPS/text/ch2.xhtml", "OEBPS/img/a.png"}
	if mutate != nil {
		mutate(files, &order)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, n := range order {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(files[n]))
	}
	zw.Close()
	return b.Bytes()
}

func TestAnEPUBBecomesOnePageInSpineOrderWithItsPictures(t *testing.T) {
	page, err := epubToHTML(epubZip(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	i1, i2 := strings.Index(s, "Chapter One"), strings.Index(s, "Chapter Two")
	if i1 < 0 || i2 < i1 {
		t.Errorf("chapters out of order or missing: %d %d", i1, i2)
	}
	if strings.Count(s, "data:image/png;base64,") != 2 || !strings.Contains(s, "color:red") || !strings.Contains(s, "break-before:page") {
		t.Errorf("pictures, style or break missing: %s", s)
	}
	if strings.Contains(s, "passwd") && strings.Contains(s, "src=\"../") {
		t.Errorf("an escaping path survived: %s", s)
	}
	out, _, err := sanitizeHTML(page)
	if err != nil || strings.Count(string(out), "data:image/png;base64,") != 2 {
		t.Errorf("the joined page does not survive the sanitiser: %v", err)
	}
}

func TestAnEPUBWithNoSpineOrContainerIsRefused(t *testing.T) {
	for name, m := range map[string]func(map[string]string, *[]string){
		"no container": func(f map[string]string, o *[]string) { delete(f, "META-INF/container.xml") },
		"no package":   func(f map[string]string, o *[]string) { delete(f, "OEBPS/content.opf") },
		"empty spine": func(f map[string]string, o *[]string) {
			f["OEBPS/content.opf"] = `<package><manifest/><spine/></package>`
		},
		"spine names nothing": func(f map[string]string, o *[]string) {
			f["OEBPS/content.opf"] = `<package><manifest/><spine><itemref idref="zz"/></spine></package>`
		},
	} {
		if _, err := epubToHTML(epubZip(t, m)); !errors.Is(err, ErrPackageUnreadable) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAnEPUBClassifiesByItsMimetypeMember(t *testing.T) {
	good := epubZip(t, nil)
	if f, err := classify(good, "book.epub"); err != nil || f.Key != "epub" {
		t.Fatalf("classify = %v, %v", f.Key, err)
	}
	r, n := BytesSource(good)
	f, _ := FormatForExt("epub")
	if err := inspectPackage(r, n, f); err != nil {
		t.Errorf("a real EPUB: %v", err)
	}
	bad := epubZip(t, func(f map[string]string, o *[]string) { f["mimetype"] = "application/zip" })
	r, n = BytesSource(bad)
	if err := inspectPackage(r, n, f); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("a zip named .epub: %v", err)
	}
}

func TestSavedPagePackagesRenderThroughTheBrowser(t *testing.T) {
	needBrowser(t)
	png := base64.StdEncoding.EncodeToString(testPNGBytes(t))
	pdf, err := ConvertMHTMLToPDF(context.Background(), mhtml(t, mpart("Content-Type: text/html", `<h1>Saved heading</h1><img src="cid:p" alt="x">`), mpart("Content-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <p>", png)))
	if err != nil || !strings.Contains(pdfText(t, pdf), "Savedheading") {
		t.Errorf("mhtml: %v", err)
	}
	pdf, err = ConvertEPUBToPDF(context.Background(), epubZip(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	txt := pdfText(t, pdf)
	if !strings.Contains(txt, "ChapterOne") || !strings.Contains(txt, "ChapterTwo") {
		t.Errorf("epub text = %q", txt)
	}
}

func nestedMHTML(levels int) []byte {
	var b strings.Builder
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=\"b0\"\r\n\r\n")
	for i := 0; i < levels; i++ {
		fmt.Fprintf(&b, "--b%d\r\nContent-Type: multipart/related; boundary=\"b%d\"\r\n\r\n", i, i+1)
	}
	fmt.Fprintf(&b, "--b%d\r\nContent-Type: text/html\r\n\r\n<p>deep</p>\r\n", levels)
	for i := levels; i >= 0; i-- {
		fmt.Fprintf(&b, "--b%d--\r\n", i)
	}
	return []byte(b.String())
}

func TestNestedMultipartIsReadToTheDepthCapAndNoFurther(t *testing.T) {
	if _, _, err := mhtmlToHTML(nestedMHTML(2)); err != nil {
		t.Errorf("two levels: %v", err)
	}
	if _, _, err := mhtmlToHTML(nestedMHTML(packageDepth + 2)); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("past the depth cap: %v", err)
	}
}

func TestTheDecodedPartsTogetherAreBounded(t *testing.T) {
	old := packageAllBytes
	packageAllBytes = 10
	defer func() { packageAllBytes = old }()
	msg := mhtml(t, mpart("Content-Type: text/html", "<p>123456</p>"), mpart("Content-Type: text/plain", "123456"))
	if _, _, err := mhtmlToHTML(msg); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("err = %v", err)
	}
}

func TestAnEPUBIsHeldToItsChapterCap(t *testing.T) {
	old := epubChapters
	epubChapters = 1
	defer func() { epubChapters = old }()
	if _, err := epubToHTML(epubZip(t, nil)); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("err = %v", err)
	}
}

func TestAnLFOnlyMHTMLMessageOverTheHeaderCapIsRead(t *testing.T) {
	msg := "MIME-Version: 1.0\nContent-Type: text/html\n\n<p>" + strings.Repeat("word ", mimeHeaderBytes) + "</p>"
	if _, _, err := mhtmlToHTML([]byte(msg)); err != nil {
		t.Errorf("an LF-only message with a short header block: %v", err)
	}
}

func TestAnEPUBSpineThatRepeatsAChapterReadsItOnce(t *testing.T) {
	data := epubZip(t, func(f map[string]string, _ *[]string) {
		f["OEBPS/content.opf"] = strings.Replace(f["OEBPS/content.opf"], `<itemref idref="c1"/>`, strings.Repeat(`<itemref idref="c1"/>`, 40), 1)
	})
	page, err := epubToHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(page), "Chapter One"); n != 1 {
		t.Errorf("chapter one appears %d times, want 1", n)
	}
}

func TestAnEPUBsChaptersTogetherAreBounded(t *testing.T) {
	old := epubOutputBytes
	epubOutputBytes = 10
	defer func() { epubOutputBytes = old }()
	if _, err := epubToHTML(epubZip(t, nil)); !errors.Is(err, ErrPackageTooBig) {
		t.Errorf("err = %v", err)
	}
}
