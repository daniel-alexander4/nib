package pdfops

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var extPatterns = func() map[string]*regexp.Regexp {
	m := map[string]*regexp.Regexp{}
	for _, f := range formats {
		for _, e := range f.Exts {
			m[e] = regexp.MustCompile(`(^|[^A-Za-z0-9_])\.` + regexp.QuoteMeta(e) + `([^A-Za-z0-9_]|$)`)
		}
	}
	return m
}()

// listedTokens returns the format-table tokens (".ext" and MIME types) a string literal names.
func listedTokens(lit string) []string {
	seen := map[string]bool{}
	for _, f := range formats {
		for _, e := range f.Exts {
			if extPatterns[e].MatchString(lit) {
				seen["."+e] = true
			}
		}
		for _, m := range f.MIME {
			if strings.Contains(lit, m) {
				seen[m] = true
			}
		}
	}
	var out []string
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

var stringLiteral = regexp.MustCompile("\"[^\"\\n]*\"|'[^'\\n]*'|`[^`\\n]*`")

// secondLists scans root for three or more of the table's tokens inside the string literals of any six
// consecutive lines, or an `accept=` attribute that names any. Files are the Go sources, the page and
// the build directory; tests, vendored code, worktrees and the table itself are not scanned. A line
// carrying one of the exempt substrings is a named, different list (exemptLists).
func secondLists(root string, exempt []string) []string {
	var hits []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "third_party", "node_modules", ".git", ".claude", "test", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".js", ".html", ".desktop", ".yaml":
		default:
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || rel == "internal/pdfops/formats.go" {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		skip := func(line string) bool {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "//") || strings.HasPrefix(trim, "*") {
				return true
			}
			for _, e := range exempt {
				if strings.Contains(line, e) {
					return true
				}
			}
			return false
		}
		lit := make([]string, len(lines)) // each line's string-literal text
		for i, line := range lines {
			if skip(line) {
				continue
			}
			lit[i] = strings.Join(stringLiteral.FindAllString(line, -1), " ")
			site := rel + ":" + strconv.Itoa(i+1)
			if strings.Contains(line, "accept=") && len(listedTokens(line)) > 0 {
				hits = append(hits, site+" accept= names a table token")
			}
			if strings.HasPrefix(strings.TrimSpace(line), "MimeType=") && rel != "build/nib.desktop" {
				hits = append(hits, site+" MimeType= outside build/nib.desktop")
			}
		}
		for i := range lines {
			end := i + 6
			if end > len(lines) {
				end = len(lines)
			}
			if toks := listedTokens(strings.Join(lit[i:end], " ")); len(toks) >= 3 && lit[i] != "" {
				hits = append(hits, rel+":"+strconv.Itoa(i+1)+" "+strings.Join(toks, ","))
				break // one hit per file is enough to name it
			}
		}
		return nil
	})
	return hits
}

// exemptLists are the named sites that list some of the same tokens for a reason that is not "the
// documents Nib imports". Each is a different set (what can be EMBEDDED as a picture, what the
// CANVAS can encode), so folding it into the table would widen a surface by accident.
var exemptLists = []string{
	`id="addImageInput"`,                            // a picture to place on a page: only what a page can embed (PNG, JPEG)
	`id="pstImage"`,                                 // the stamp's picture: the same two
	`id="fillCsvInput"`,                             // data to fill a form from, not a document to import
	`".svg":  "image/svg+xml"`,                      // embed.go: the Content-Type of Nib's OWN static files, keyed by their extension
	`".md":   "text/markdown`,                       // (same map)
	`".png":  "image/png"`,                          // (same map)
	`case "image/jpeg", "image/png", "image/tiff":`, // mdpdf/packet.go: a standalone package that cannot import internal/pdfops; what it can append
	`ADR-189 writes`,                                // app.js: the pictures Pages as images WRITES (the browser's and the server's encoders), not documents imported
	`"golang.org/x/image/bmp"`,                      // pageimages.go: the encoders it imports (bmp, gif, png) — what Nib writes
	`ADR-194 writes`,                                // Document as…'s Format line and route: the files Nib WRITES (Markdown, HTML, text, Word, OpenDocument), not documents imported
	`one('xlsx'`,                                    // the table EXPORT's Format line: output spreadsheets, not documents imported
}

func TestTheFormatTableIsTheOnlyList(t *testing.T) {
	root := filepath.Join("..", "..")
	if hits := secondLists(root, exemptLists); len(hits) > 0 {
		t.Fatalf("a second list of the formats the table owns (internal/pdfops/formats.go, ADR-184):\n  %s", strings.Join(hits, "\n  "))
	}
}

// The census is only worth its green if it can go red: a planted second list in each file kind must be found.
func TestTheFormatCensusFindsAPlantedList(t *testing.T) {
	dir := t.TempDir()
	plant := map[string]string{
		"a.go":               "var x = []string{\".docx\", \".odt\", \".rtf\"}\n",
		"web/b.js":           "el.accept = '.docx,.odt,.rtf';\n",
		"web/c.html":         "<input type=\"file\" accept=\".docx\" />\n",
		"build/d.desktop":    "MimeType=application/pdf;\n",
		"build/e.yaml":       "mime: \"image/png,image/jpeg,image/tiff\"\n",
		"internal/f/mime.go": "const m = \"image/png image/jpeg image/webp\"\n",
	}
	for name, body := range plant {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hits := strings.Join(secondLists(dir, nil), "\n")
	for name := range plant {
		if !strings.Contains(hits, name) {
			t.Errorf("a planted list in %s was not found; hits:\n%s", name, hits)
		}
	}
	// ...and a literal naming two tokens, or one in a comment, is not a list.
	clean := t.TempDir()
	os.WriteFile(filepath.Join(clean, "ok.go"), []byte("// \".docx\", \".odt\", \".rtf\"\nvar y = \".png or .jpg\"\n"), 0o644)
	if h := secondLists(clean, nil); len(h) != 0 {
		t.Errorf("false positive: %v", h)
	}
}

func TestTheTableViewsAgree(t *testing.T) {
	seen := map[string]string{}
	for _, f := range Formats() {
		if f.Key == "" || len(f.Exts) == 0 || len(f.MIME) == 0 {
			t.Errorf("row %+v lacks a key, an extension or a MIME type", f)
		}
		for _, e := range f.Exts {
			if e != strings.ToLower(e) || strings.HasPrefix(e, ".") {
				t.Errorf("%s: extension %q must be lowercase with no dot", f.Key, e)
			}
			if prev, dup := seen[e]; dup {
				t.Errorf("extension %q is in rows %s and %s", e, prev, f.Key)
			}
			seen[e] = f.Key
		}
		if f.Image && !f.Door {
			t.Errorf("%s: an image the sniffer reads is opened by every route", f.Key)
		}
	}
	for _, e := range []string{"docx", ".DOCX", "xlsx", "odp"} {
		if !SupportedOfficeExt(e) || !SupportedDocExt(e) {
			t.Errorf("%q should convert through LibreOffice", e)
		}
	}
	for _, e := range []string{"txt", "csv"} {
		if SupportedOfficeExt(e) || !SupportedDocExt(e) {
			t.Errorf("%q should convert natively, not through LibreOffice (ADR-188)", e)
		}
	}
	for _, e := range []string{"md", ".Markdown"} {
		if !SupportedMarkdownExt(e) || SupportedOfficeExt(e) {
			t.Errorf("%q should be Markdown only", e)
		}
	}
	for _, e := range []string{"png", "pdf", "exe", "", "gif"} {
		if SupportedDocExt(e) {
			t.Errorf("%q is not a document Nib converts", e)
		}
	}
	if !IsDoorExt("TIFF") || IsDoorExt("docx") || IsDoorExt("md") {
		t.Errorf("the door set is the images and nothing else today")
	}
	// the table hands out copies
	Formats()[0].Exts[0] = "zzz"
	if formats[0].Exts[0] == "zzz" {
		t.Errorf("Formats() leaked the table's own slices")
	}
}

// build/nib.desktop's MimeType is the table's: PDF and every Door format. NIB_REGEN_DESKTOP=1 rewrites it.
func TestTheDesktopFileMimeTypeIsTheTable(t *testing.T) {
	want := "MimeType=application/pdf;" + strings.Join(FormatMIMEs(func(f Format) bool { return f.Door }), ";") + ";"
	path := filepath.Join("..", "..", "build", "nib.desktop")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	got := ""
	for i, l := range lines {
		if strings.HasPrefix(l, "MimeType=") {
			got = l
			if os.Getenv("NIB_REGEN_DESKTOP") == "1" {
				lines[i] = want
			}
		}
	}
	if os.Getenv("NIB_REGEN_DESKTOP") == "1" {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got != want {
		t.Fatalf("build/nib.desktop says %q; the format table says %q (NIB_REGEN_DESKTOP=1 go test ./internal/pdfops -run DesktopFile rewrites it)", got, want)
	}
}
