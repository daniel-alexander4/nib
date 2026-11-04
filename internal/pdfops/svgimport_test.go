package pdfops

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// The SVG door (P05.S01a, X11(c), R3, ADR-204). Present-path tests need a real browser and say so.

func TestSniffAndClassifyAnSVG(t *testing.T) {
	for _, c := range []struct {
		head string
		want bool
	}{
		{`<svg xmlns="http://www.w3.org/2000/svg"/>`, true},
		{"\xEF\xBB\xBF<?xml version=\"1.0\"?>\n<!-- c --><SVG/>", true},
		{`<?xml version="1.0"?><!DOCTYPE svg PUBLIC "x" "y"><svg/>`, true},
		{`<!doctype html><html><body><svg/></body></html>`, false},
		{`<html><body><svg/></body></html>`, false},
		{`<p>hello <svg/></p>`, false}, // a fragment that holds an inline svg is a fragment of a page
		{`<svgfoo/>`, false},
		{`<!-- c`, false},
		{`# Markdown <svg>`, false},
		{"<svg>\x00", false},
		{"", false},
	} {
		if got := isSVG([]byte(c.head)); got != c.want {
			t.Errorf("%q: isSVG = %v, want %v", c.head, got, c.want)
		}
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"/>`)
	if f, err := classify(svg, "logo.svg"); err != nil || f.Key != "svg" {
		t.Errorf("classify(svg, logo.svg) = %v, %v", f.Key, err)
	}
	if f, err := classify(svg, "notes.md"); err != nil || f.Key != "markdown" {
		t.Errorf("a Markdown name keeps it text: %v, %v", f.Key, err)
	}
	if _, err := classify(svg, "logo.png"); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("an SVG named .png = %v, want ErrExtensionDisagrees", err)
	}
	if _, err := classify(svg, ""); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("a nameless SVG = %v, want ErrExtensionDisagrees", err)
	}
	if f, err := classify([]byte("<p>a <svg/> b</p>"), "frag.html"); err != nil || f.Key != "html" {
		t.Errorf("a fragment with an inline svg named .html = %v, %v", f.Key, err)
	}
	if _, err := classify([]byte("not svg"), "logo.svg"); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("text named .svg = %v, want ErrExtensionDisagrees", err)
	}
}

func TestSanitizeSVGRebuildsFromAnAllowlist(t *testing.T) {
	const ns = `xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"`
	in := `<?xml version="1.0"?><svg ` + ns + ` width="2in" height="96" onload="x()">
<script>alert(1)</script><foreignObject><iframe src="file:///etc/hostname"/></foreignObject>
<a xlink:href="http://x/"><text onclick="y()" x="1" y="2" style="fill:url(#g)">KEEP</text></a>
<rect id="r" width="5" height="5" fill="url(#g)" style="fill:url(http://x/p.png)"/>
<use xlink:href="http://x/a.svg#b"/><use href="#r"/><use xlink:href="#r"/><use xlink:href="file:///etc/passwd"/>
<g xmlns="http://evil.example/ns"><text>EVILNS</text></g><rect data-x="javascript:alert(1)" fill="url(http://x/p.png)" style="width:expression(1)" width="1" height="1"/>
<image href="http://x/p.png"/><image href="data:image/png;base64,AAAA"/>
<style>@import url(http://x/c.css); a{fill:red}</style><style>g{fill:blue}</style>
<animate attributeName="x"/><set attributeName="href" to="javascript:1"/>
</svg>`
	out, w, h, err := sanitizeSVG([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{"script", "foreignObject", "iframe", "onload", "onclick", "http://x", "file:", "animate", "<set", "@import", "etc/", "<a ", "EVILNS", "javascript", "expression", "p.png)"} {
		if strings.Contains(s, bad) {
			t.Errorf("the rebuilt picture still holds %q:\n%s", bad, s)
		}
	}
	for _, good := range []string{"KEEP", `href="#r"`, `xlink:href="#r"`, `fill="url(#g)"`, "g{fill:blue}", `data:image/png;base64,AAAA`} {
		if !strings.Contains(s, good) {
			t.Errorf("the rebuilt picture lost %q:\n%s", good, s)
		}
	}
	if w != 192 || h != 96 || !strings.Contains(s, `width="192" height="96"`) || !strings.Contains(s, `viewBox="0 0 192 96"`) {
		t.Errorf("size = %v x %v, root = %.120s", w, h, s)
	}
}

func TestSanitizeSVGRefusals(t *testing.T) {
	const ns = `xmlns="http://www.w3.org/2000/svg"`
	big := `<svg ` + ns + ` width="1" height="1">` + strings.Repeat("<g>", svgMaxDepth+1) + strings.Repeat("</g>", svgMaxDepth+1) + `</svg>`
	for name, in := range map[string]string{
		"doctype":       `<!DOCTYPE svg [<!ENTITY a "aaaa">]><svg ` + ns + ` width="1" height="1">&a;</svg>`,
		"undefined ent": `<svg ` + ns + ` width="1" height="1">&lol;</svg>`,
		"not svg":       `<html xmlns="http://www.w3.org/1999/xhtml"/>`,
		"wrong ns":      `<svg width="1" height="1"/>`,
		"no size":       `<svg ` + ns + `/>`,
		"percent size":  `<svg ` + ns + ` width="100%" height="100%"/>`,
		"too wide":      `<svg ` + ns + ` width="15000" height="10"/>`,
		"too small":     `<svg ` + ns + ` width="0.5" height="10"/>`,
		"negative":      `<svg ` + ns + ` width="-5" height="10"/>`,
		"nan":           `<svg ` + ns + ` width="NaN" height="10"/>`,
		"unclosed":      `<svg ` + ns + ` width="1" height="1"><g>`,
		"too deep":      big,
		"empty":         ``,
		"viewbox zero":  `<svg ` + ns + ` viewBox="0 0 0 5"/>`,
		"viewbox inf":   `<svg ` + ns + ` viewBox="0 0 1e999 5"/>`,
		"not UTF-8":     "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><svg " + ns + " width=\"1\" height=\"1\"/>",
	} {
		if _, _, _, err := sanitizeSVG([]byte(in)); !errors.Is(err, ErrHTMLUnreadable) {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
	}
	pad := func(n int) []byte {
		head := `<svg ` + ns + ` width="1" height="1"/><!--`
		return []byte(head + strings.Repeat("a", n-len(head)-3) + "-->")
	}
	if _, _, _, err := sanitizeSVG(pad(svgMaxBytes + 1)); !errors.Is(err, ErrHTMLUnreadable) {
		t.Errorf("over the byte cap = %v", err)
	}
	if _, _, _, err := sanitizeSVG(pad(svgMaxBytes)); err != nil {
		t.Errorf("at the byte cap: %v", err)
	}
	// the depth cap accepts exactly svgMaxDepth levels including the root
	deep := func(n int) string {
		return `<svg ` + ns + ` width="1" height="1">` + strings.Repeat("<g>", n-1) + strings.Repeat("</g>", n-1) + `</svg>`
	}
	if _, _, _, err := sanitizeSVG([]byte(deep(svgMaxDepth))); err != nil {
		t.Errorf("at the depth cap: %v", err)
	}
	if _, _, _, err := sanitizeSVG([]byte(deep(svgMaxDepth + 1))); !errors.Is(err, ErrHTMLUnreadable) {
		t.Errorf("past the depth cap = %v", err)
	}
	// at the caps: accepted
	for name, in := range map[string]string{
		"max side":   `<svg ` + ns + ` width="14400" height="14400"/>`,
		"min side":   `<svg ` + ns + ` width="1" height="1"/>`,
		"viewBox":    `<svg ` + ns + ` viewBox="0 0 30 20"/>`,
		"one length": `<svg ` + ns + ` width="60" viewBox="0 0 30 20"/>`,
	} {
		if _, _, _, err := sanitizeSVG([]byte(in)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, w, h, _ := sanitizeSVG([]byte(`<svg ` + ns + ` width="60" viewBox="0 0 30 20"/>`)); w != 60 || h != 40 {
		t.Errorf("one length with a viewBox = %v x %v, want 60 x 40", w, h)
	}
}

func TestAnSVGPrintsAtItsOwnSizeOnOnePage(t *testing.T) {
	needBrowser(t)
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="100"><rect width="400" height="100" fill="#eef"/><text x="10" y="40" font-size="12">` + benignWord + `</text></svg>`
	pdf, err := ConvertSVGToPDF(context.Background(), []byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := PageCount(pdf); n != 1 {
		t.Fatalf("pages = %d", n)
	}
	x0, y0, x1, y1, err := PageBox(pdf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs((x1-x0)-300) > 0.6 || math.Abs((y1-y0)-75) > 0.6 {
		t.Errorf("page = %.2f x %.2f pt, want 300 x 75 (400 x 100 px)", x1-x0, y1-y0)
	}
	if !strings.Contains(pdfText(t, pdf), benignWord) {
		t.Errorf("the SVG's words are missing from the page")
	}
}

type svgVector struct {
	name  string
	body  func(secretURL, canaryURL string) string
	reads bool // tries to print the secret file; otherwise tries to reach the canary
}

var svgVectors = []svgVector{
	{"script", func(s, c string) string { return `<script>fetch("` + c + `/script")</script>` }, false},
	{"nested svg onload", func(s, c string) string { return `<svg onload="fetch('` + c + `/onload')" width="5" height="5"/>` }, false},
	{"image href", func(s, c string) string { return `<image href="` + c + `/image.png" width="10" height="10"/>` }, false},
	{"image xlink", func(s, c string) string {
		return `<image xlink:href="` + c + `/xlink.png" width="10" height="10"/>`
	}, false},
	{"style import", func(s, c string) string { return `<style>@import url("` + c + `/imp.css");</style>` }, false},
	{"style url", func(s, c string) string { return `<rect width="9" height="9" style="fill:url(` + c + `/p.svg#a)"/>` }, false},
	{"feImage", func(s, c string) string {
		return `<filter id="f"><feImage href="` + c + `/feimage.png"/></filter><rect width="9" height="9" filter="url(#f)"/>`
	}, false},
	{"foreignObject iframe", func(s, c string) string {
		return `<foreignObject width="300" height="100"><iframe xmlns="http://www.w3.org/1999/xhtml" src="` + s + `" width="300" height="100"/></foreignObject>`
	}, true},
	{"foreignObject img", func(s, c string) string {
		return `<foreignObject width="30" height="30"><img xmlns="http://www.w3.org/1999/xhtml" src="` + c + `/fo.png"/></foreignObject>`
	}, false},
}

func hostileSVG(v svgVector, s, c string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="400" height="200">` +
		`<text x="10" y="150" font-size="14">` + benignWord + `</text>` + v.body(s, c) + `</svg>`
}

// TestEverySVGVectorIsInertThroughTheDoorAndFiresWithoutIt: each vector, first as a document of its own from
// a file: URL (the positive control, which must fire), then rebuilt-not (the raw picture inside the wrapper,
// the browser's image mode alone), then through the whole import path.
func TestEverySVGVectorIsInertThroughTheDoorAndFiresWithoutIt(t *testing.T) {
	chromium := needBrowser(t)
	work := t.TempDir()
	secretPath := filepath.Join(work, "secret.txt")
	if err := os.WriteFile(secretPath, []byte(secretWord), 0o600); err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int64
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" id="a"/>`))
	})}
	go srv.Serve(ln)
	defer srv.Close()
	canary := "http://" + ln.Addr().String()
	secretURL := "file://" + secretPath

	for _, v := range svgVectors {
		t.Run(v.name, func(t *testing.T) {
			svg := hostileSVG(v, secretURL, canary)
			// the control: the picture as a page of its own with no protections.
			wrapPath := filepath.Join(work, strings.ReplaceAll(v.name, " ", "_")+".html")
			ctlPage := `<!doctype html><html><body>` + strings.TrimPrefix(svg, "") + `</body></html>`
			if err := os.WriteFile(wrapPath, []byte(ctlPage), 0o600); err != nil {
				t.Fatal(err)
			}
			before := hits.Load()
			ctl, _ := renderFromFile(t, chromium, wrapPath)
			fired := false
			if v.reads {
				fired = ctl != nil && strings.Contains(pdfText(t, ctl), secretWord)
			} else {
				fired = hits.Load() > before
			}
			if !fired {
				t.Errorf("INCONCLUSIVE: the positive control (this SVG inline in a page from file:, no image mode, no CSP) did not %s, so this vector proves nothing on %s",
					map[bool]string{true: "print the secret", false: "reach the canary"}[v.reads], chromium)
				return
			}
			// the browser's image mode and the origin alone: the raw picture, never rebuilt.
			before = hits.Load()
			raw := `<!doctype html><html><head><style>@page{size:400px 200px;margin:0}body{margin:0}</style></head><body><img alt="" src="data:image/svg+xml;base64,` +
				base64.StdEncoding.EncodeToString([]byte(svg)) + `"></body></html>`
			pdf, err := renderDocument(context.Background(), chromium, []byte(raw))
			if err != nil {
				t.Fatalf("the door could not render the raw picture: %v", err)
			}
			if txt := pdfText(t, pdf); strings.Contains(txt, secretWord) || hits.Load() != before {
				t.Errorf("raw picture in an img: secret=%v canary=%d", strings.Contains(txt, secretWord), hits.Load()-before)
			}
			// the whole import path.
			before = hits.Load()
			pdf2, err := ConvertSVGToPDF(context.Background(), []byte(svg))
			if err != nil {
				t.Fatalf("ConvertSVGToPDF: %v", err)
			}
			text := pdfText(t, pdf2)
			if !strings.Contains(text, benignWord) {
				t.Errorf("INCONCLUSIVE: the sentinel %q is missing from the import (%q)", benignWord, text)
			}
			if strings.Contains(text, secretWord) || hits.Load() != before {
				t.Errorf("the import path: secret=%v canary=%d", strings.Contains(text, secretWord), hits.Load()-before)
			}
		})
	}
}

func TestAnSVGOnAMachineWithNoBrowserNamesTheTool(t *testing.T) {
	old := browserPath
	browserPath = func() string { return "" }
	defer func() { browserPath = old }()
	_, err := ConvertSVGToPDF(context.Background(), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>`))
	if !errors.Is(err, ErrBrowserMissing) {
		t.Errorf("err = %v, want ErrBrowserMissing", err)
	}
}
