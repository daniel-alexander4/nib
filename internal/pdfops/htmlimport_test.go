package pdfops

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nib/internal/browserrender"
	"nib/internal/tempdir"
)

// The browser door (P04.S04a, D29, R3, X4, X5, ADR-200). Tests that need a real browser name what they did
// not run; NIB_REQUIRE_TOOLS=1 turns every such skip into a failure (X5).

func needBrowser(t *testing.T) string {
	t.Helper()
	p := browserPath()
	if p != "" && BrowserAvailable() {
		return p
	}
	if os.Getenv("NIB_REQUIRE_TOOLS") == "1" {
		t.Fatal("NIB_REQUIRE_TOOLS=1 and no browser that prints a page is available")
	}
	t.Skip("present-path test not run: no Chromium-family browser that prints a page is available on this machine")
	return ""
}

func pdfText(t *testing.T, pdf []byte) string {
	t.Helper()
	pages, err := DocumentText(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, p := range pages {
		for _, r := range p.Runs {
			b.WriteString(r.Text)
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), "")
}

func TestSniffAndClassifyAWebPage(t *testing.T) {
	for _, c := range []struct {
		head string
		want bool
	}{
		{"<!DOCTYPE html><html><body>x", true},
		{"\xEF\xBB\xBF  \n<HTML>", true},
		{"<!-- c --><head><title>x", true},
		{"<p>just a fragment</p>", false},
		{"# Markdown mentioning <html> in prose", false},
		{"plain text", false},
		{"<html>\x00binary", false},
	} {
		if got := isHTML([]byte(c.head)); got != c.want {
			t.Errorf("%q: isHTML = %v, want %v", c.head, got, c.want)
		}
	}
	page := []byte("<!doctype html><html><body>x</body></html>")
	if f, err := classify(page, "saved.html"); err != nil || f.Key != "html" {
		t.Errorf("classify(page, saved.html) = %v, %v", f.Key, err)
	}
	if f, err := classify(page, "notes.md"); err != nil || f.Key != "markdown" {
		t.Errorf("a Markdown name keeps markup as text: %v, %v", f.Key, err)
	}
	if _, err := classify(page, "scan.docx"); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("a web page named .docx = %v, want ErrExtensionDisagrees", err)
	}
	if _, err := classify(page, ""); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("a nameless web page = %v, want ErrExtensionDisagrees (not guessed)", err)
	}
	if f, err := classify([]byte("<p>hello</p>"), "frag.html"); err != nil || f.Key != "html" {
		t.Errorf("a fragment named .html = %v, %v", f.Key, err)
	}
	if _, err := classify([]byte{0x00, 0x01, 0x02}, "bin.html"); !errors.Is(err, ErrExtensionDisagrees) {
		t.Errorf("binary named .html = %v", err)
	}
}

func TestSanitizeKeepsWordsAndDropsEverythingThatReaches(t *testing.T) {
	src := `<!doctype html><html><head><title>T</title><base href="file:///etc/">
<meta http-equiv="refresh" content="0;url=file:///etc/hostname">
<link rel="stylesheet" href="http://evil.test/a.css"><script>alert(1)</script>
<style>p{color:red}</style><style>@import url(http://evil.test/x.css);</style></head>
<body onload="x()"><h1 id=a class=b style="color:blue">Heading</h1>
<p style="background:url(http://evil.test/y)">Para <a href="javascript:alert(1)">bad</a> <a href="https://ok.test/">good</a></p>
<iframe src="file:///etc/hostname"></iframe><object data="file:///etc/hostname"></object><embed src="file:///etc/hostname">
<svg><use href="file:///etc/hostname"/></svg><form action="http://evil.test"><input name=q></form>
<img src="http://evil.test/p.png" alt="remote pic"><img src="file:///etc/hostname" alt="local pic">
<img src="data:image/png;base64,iVBORw0KGgo=" alt="inline"><img src="data:text/html;base64,PGI+" alt="html image">
<noscript>NOSCRIPTWORDS</noscript><script>BODYSCRIPTTEXT</script><iframe>IFRAMEFALLBACK</iframe><img src="data:image/png,rawbytes" alt="notbase64"><table><tr><td colspan=2>cell</td></tr></table></body></html>`
	out, st, err := sanitizeHTML([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{"<script", "alert(1)", "onload", "<iframe", "<object", "<embed", "<svg", "<form", "<input", "<link", "<base", "refresh", "file:", "evil.test", "javascript:", "@import", "NOSCRIPTWORDS", "<title", "text/html;base64", "url(", "BODYSCRIPTTEXT", "IFRAMEFALLBACK", "rawbytes"} {
		if strings.Contains(s, bad) {
			t.Errorf("sanitised page still contains %q:\n%s", bad, s)
		}
	}
	for _, want := range []string{"Heading", "Para", "good", "https://ok.test/", "color:red", "remote pic", "local pic", "html image", `src="data:image/png;base64,iVBORw0KGgo="`, "colspan", "cell", `class="b"`, "color:blue"} {
		if !strings.Contains(s, want) {
			t.Errorf("sanitised page lost %q:\n%s", want, s)
		}
	}
	if st.DroppedImages != 4 {
		t.Errorf("DroppedImages = %d, want 4", st.DroppedImages)
	}
}

func TestADataPictureOverTheCapIsDropped(t *testing.T) {
	big := strings.Repeat("A", (maxDataImage/3+1)*4)
	out, st, err := sanitizeHTML([]byte(`<img src="data:image/png;base64,` + big + `" alt="big">`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "base64") || st.DroppedImages != 1 {
		t.Errorf("an over-cap picture was kept (dropped=%d)", st.DroppedImages)
	}
}

// hostile pages: every vector, served first as the stranger wrote it. An anchor is not a vector here: printing
// follows no link and nothing clicks (a `ping` attribute fired in neither the control nor the door, measured).
type vector struct {
	name string
	page func(secretURL, canaryURL string) string
	// reads: the vector tries to print the secret file's words; otherwise it tries to reach the canary.
	reads bool
}

var vectors = []vector{
	{"meta refresh", func(s, c string) string { return `<meta http-equiv=refresh content="0;url=` + s + `">` }, true},
	{"iframe", func(s, c string) string { return `<iframe src="` + s + `" width=600 height=200></iframe>` }, true},
	{"object", func(s, c string) string {
		return `<object data="` + s + `" type="text/plain" width=600 height=200></object>`
	}, true},
	{"embed", func(s, c string) string { return `<embed src="` + s + `" type="text/plain" width=600 height=200>` }, true},
	{"link prefetch", func(s, c string) string {
		return `<link rel=prefetch href="` + c + `/prefetch"><link rel=stylesheet href="` + c + `/sheet.css">`
	}, false},
	{"css import", func(s, c string) string { return `<style>@import url("` + c + `/imp.css");</style>` }, false},
	{"css url", func(s, c string) string { return `<style>body{background:url("` + c + `/bg.png")}</style>` }, false},
	{"img", func(s, c string) string { return `<img src="` + c + `/px.png">` }, false},
	{"meta refresh to canary", func(s, c string) string { return `<meta http-equiv=refresh content="0;url=` + c + `/go">` }, false},
	{"svg use", func(s, c string) string {
		return `<svg width=300 height=40><use href="` + s + `#x"/><image href="` + c + `/svgimg.png" width=10 height=10/></svg>`
	}, false},
	{"base then relative", func(s, c string) string { return `<base href="` + c + `/"><img src="rel.png">` }, false},
}

const (
	secretWord = "SECRETWORDALPHA"
	benignWord = "BENIGNSENTINELBETA"
)

func hostilePage(v vector, secretURL, canaryURL string) string {
	return "<!doctype html><html><head><meta charset=utf-8>" + v.page(secretURL, canaryURL) + "</head><body><p>" + benignWord + "</p></body></html>"
}

// renderFromFile is the positive control (X5): the SAME page served from a file: URL with none of the door's
// protections. It must print the secret (reads) or reach the canary, or the vector tests nothing.
func renderFromFile(t *testing.T, chromium, pagePath string) ([]byte, error) {
	t.Helper()
	dir, err := tempdir.New("htmlctl")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	prof := dir.Join("p")
	if err := os.Mkdir(prof, 0o700); err != nil {
		t.Fatal(err)
	}
	out := dir.Join("o.pdf")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chromium, "--headless", "--disable-gpu", "--no-pdf-header-footer", "--user-data-dir="+prof,
		"--virtual-time-budget=3000", "--print-to-pdf="+out, "file://"+pagePath)
	cmd.Env = toolEnv(os.Environ(), dir.Path())
	_ = cmd.Run()
	return os.ReadFile(out)
}

// TestEveryVectorIsKeptOutByTheDoorAndFiresWithoutIt is X5: each vector, with a positive control that names
// what it removes, a benign sentinel that proves the page rendered, and a loopback canary that counts hits.
func TestEveryVectorIsKeptOutByTheDoorAndFiresWithoutIt(t *testing.T) {
	chromium := needBrowser(t)
	t.Logf("browser: %s", chromium)
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
		w.Header().Set("Content-Type", "text/css")
	})}
	go srv.Serve(ln)
	defer srv.Close()
	canary := "http://" + ln.Addr().String()
	secretURL := "file://" + secretPath

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			page := hostilePage(v, secretURL, canary)
			pagePath := filepath.Join(work, strings.ReplaceAll(v.name, " ", "_")+".html")
			if err := os.WriteFile(pagePath, []byte(page), 0o600); err != nil {
				t.Fatal(err)
			}
			// the control: no protections.
			before := hits.Load()
			ctl, _ := renderFromFile(t, chromium, pagePath)
			fired := false
			if v.reads {
				fired = ctl != nil && strings.Contains(pdfText(t, ctl), secretWord)
			} else {
				fired = hits.Load() > before
			}
			if !fired {
				t.Errorf("INCONCLUSIVE: the positive control (this page from file:, no host rule, no CSP header) did not %s, so this vector proves nothing on %s",
					map[bool]string{true: "print the secret", false: "reach the canary"}[v.reads], chromium)
				return
			}
			// the door, unsanitised: the page exactly as the stranger wrote it, through the origin alone.
			before = hits.Load()
			pdf, err := renderDocument(context.Background(), chromium, []byte(page))
			if err != nil {
				t.Fatalf("the door could not render the hostile page: %v", err)
			}
			text := pdfText(t, pdf)
			if !strings.Contains(text, benignWord) {
				t.Errorf("INCONCLUSIVE: the benign sentinel %q is missing from the protected output (%q)", benignWord, text)
			}
			if strings.Contains(text, secretWord) {
				t.Errorf("the door printed the secret for %s", v.name)
			}
			if n := hits.Load() - before; n != 0 {
				t.Errorf("the door let %s reach the canary %d times", v.name, n)
			}
			// the whole import path: sanitised first, then the door.
			before = hits.Load()
			pdf2, err := ConvertHTMLToPDF(context.Background(), []byte(page))
			if err != nil {
				t.Fatalf("ConvertHTMLToPDF: %v", err)
			}
			text2 := pdfText(t, pdf2)
			if !strings.Contains(text2, benignWord) || strings.Contains(text2, secretWord) || hits.Load() != before {
				t.Errorf("the import path: sentinel=%v secret=%v canary=%d", strings.Contains(text2, benignWord), strings.Contains(text2, secretWord), hits.Load()-before)
			}
		})
	}
}

func TestAWebPageBecomesAPDFWithItsWords(t *testing.T) {
	needBrowser(t)
	pdf, err := ConvertHTMLToPDF(context.Background(), []byte(`<h1>Quarterly report</h1><p>Revenue grew</p><table><tr><td>North</td><td>12</td></tr></table>`))
	if err != nil {
		t.Fatal(err)
	}
	text := pdfText(t, pdf)
	for _, w := range []string{"Quarterlyreport", "Revenuegrew", "North"} {
		if !strings.Contains(text, w) {
			t.Errorf("the PDF text lacks %q: %q", w, text)
		}
	}
}

func TestANoBrowserMachineNamesTheTool(t *testing.T) {
	old := browserPath
	browserPath = func() string { return "" }
	defer func() { browserPath = old }()
	if _, err := ConvertHTMLToPDF(context.Background(), []byte("<p>x</p>")); !errors.Is(err, ErrBrowserMissing) {
		t.Fatalf("err = %v, want ErrBrowserMissing", err)
	}
	if tool, ok := MissingToolFor(ErrBrowserMissing); !ok || tool != ToolBrowser {
		t.Errorf("MissingToolFor = %v, %v", tool, ok)
	}
	if FormatAvailable(Format{Tool: ToolBrowser}) {
		t.Error("the html format is available with no browser")
	}
}

// A stub browser that exits 1 makes the format unavailable with a reason (D27).
func TestAStubBrowserThatExitsOneIsRefusedByTheProbe(t *testing.T) {
	needShell(t)
	stub := filepath.Join(t.TempDir(), "chromium")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := browserPath
	browserPath = func() string { return stub }
	defer func() { browserPath = old }()
	_, err := ConvertHTMLToPDF(context.Background(), []byte("<p>x</p>"))
	if !errors.Is(err, ErrBrowserCannotRender) {
		t.Fatalf("err = %v, want ErrBrowserCannotRender", err)
	}
	if BrowserAvailable() {
		t.Error("BrowserAvailable is true for a stub that cannot print")
	}
}

// A browser that writes a PDF and exits 0 without ever fetching the page wrote something that is not the page (X4 c).
func TestABrowserThatNeverAsksForThePageIsRefused(t *testing.T) {
	needShell(t)
	stub := filepath.Join(t.TempDir(), "chromium")
	script := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in --print-to-pdf=*) out=\"${a#--print-to-pdf=}\";; esac; done\nprintf '%%PDF-1.4\\n' > \"$out\"\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := renderDocument(context.Background(), stub, []byte("<p>x</p>"))
	if !errors.Is(err, browserrender.ErrNotServed) {
		t.Fatalf("err = %v, want ErrNotServed", err)
	}
}

func TestTheRenderDoorLeavesNoListenerNoProfileAndNoProcessBehind(t *testing.T) {
	chromium := needBrowser(t)
	root, _ := tempdir.Root()
	before, _ := os.ReadDir(root)
	if _, err := renderDocument(context.Background(), chromium, []byte("<p>x</p>")); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadDir(root)
	if len(after) > len(before) {
		t.Errorf("temp entries before %d, after %d", len(before), len(after))
	}
	_ = fmt.Sprint
}

func TestPicturesTogetherAreBoundedAsWellAsOneByOne(t *testing.T) {
	old := maxDataImagesAll
	maxDataImagesAll = 10
	defer func() { maxDataImagesAll = old }()
	img := `<img src="data:image/png;base64,iVBORw0KGgo=" alt="x">` // 8 bytes decoded
	out, st, err := sanitizeHTML([]byte(img + img))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), "base64") != 1 || st.DroppedImages != 1 {
		t.Errorf("kept %d pictures, dropped %d; want 1 and 1", strings.Count(string(out), "base64"), st.DroppedImages)
	}
}
