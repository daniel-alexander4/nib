package cli

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"nib/internal/testpdf"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
)

const template = "# Letter to {{name}}\n\nDear {{ name }}, your reference is {{ref}}.\n"

// TestOfficeFillsAMarkdownTemplateAndSetsThePage — RelaxTax B4/B5 (the part PLAN-import-export P01.S03 pins that does
// not need the import door): placeholders from --set and --set-file, the page and its margin, stdin with --type, and
// `nib import` as the same command.
func TestOfficeFillsAMarkdownTemplateAndSetsThePage(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "t.md")
	os.WriteFile(md, []byte(template), 0o600)
	vals := filepath.Join(dir, "v.json")
	os.WriteFile(vals, []byte(`{"name":"Ada Lovelace","ref":"FROM-FILE"}`), 0o600)

	text := func(pdf []byte) string {
		pages, err := pdfops.DocumentText(pdf)
		if err != nil {
			t.Fatalf("unreadable output: %v", err)
		}
		var b strings.Builder
		for _, p := range pages {
			for _, r := range p.Runs {
				b.WriteString(r.Text + " ")
			}
		}
		return b.String()
	}
	for _, verb := range []string{"office", "import"} {
		out, stderr, code := fillChild(t, "", "", verb, md, "--set-file", vals, "--set", "ref=FROM-SET", "--page-size", "a4", "--margin", "54", "-o", "-")
		if code != 0 {
			t.Fatalf("%s exited %d: %s", verb, code, stderr)
		}
		got := text(out)
		if !strings.Contains(got, "Ada") || !strings.Contains(got, "FROM-SET") || strings.Contains(got, "FROM-FILE") || strings.Contains(got, "{{") {
			t.Errorf("%s: page text %q, want the name, the --set value overriding the file, and no braces", verb, got)
		}
		pages, _ := pdfops.DocumentText(out)
		if w := int(pages[0].Width); w != 595 {
			t.Errorf("%s: page width %d, want A4's 595", verb, w)
		}
	}
	// stdin needs --type, and gets it.
	if _, stderr, code := fillChild(t, template, "", "office", "-", "--set", "name=A", "--set", "ref=B", "-o", "-"); code != 1 || !strings.Contains(stderr, "needs --type md") {
		t.Errorf("stdin without --type: exit %d, %q", code, stderr)
	}
	out, stderr, code := fillChild(t, template, "", "office", "-", "--type", "md", "--set", "name=Grace", "--set", "ref=R1", "-o", "-")
	if code != 0 || !strings.Contains(text(out), "Grace") {
		t.Errorf("stdin with --type md: exit %d, %q", code, stderr)
	}
}

func TestOfficeRefusesWhatItCannotFillNamingTheKeyAndNotTheValue(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "t.md")
	os.WriteFile(md, []byte(template), 0o600)
	o := filepath.Join(dir, "o.pdf")
	for name, c := range map[string]struct {
		args []string
		want string
	}{
		"missing key": {[]string{"--set", "name=SECRET-VALUE-77"}, `"ref"`},
		"no equals":   {[]string{"--set", "SECRET-VALUE-77"}, "has no ="},
		"bad key":     {[]string{"--set", "na me=SECRET-VALUE-77"}, "is not letters"},
		"page size":   {[]string{"--set", "name=a", "--set", "ref=b", "--page-size", "tabloid"}, "tabloid"},
		"margin":      {[]string{"--set", "name=a", "--set", "ref=b", "--margin", "400"}, "margin"},
		"bad type":    {[]string{"--type", "docx"}, "md or markdown"},
	} {
		_, stderr, code := fillChild(t, "", "", append([]string{"office", md, "-o", o}, c.args...)...)
		if code != 1 || !strings.Contains(stderr, c.want) {
			t.Errorf("%s: exit %d, stderr %q, want 1 containing %q", name, code, stderr, c.want)
		}
		if strings.Contains(stderr, "SECRET") {
			t.Errorf("%s: the refusal repeats a value: %q", name, stderr)
		}
	}
	if _, err := os.Stat(o); err == nil {
		t.Error("an output was written for a refused request")
	}
	// A document LibreOffice converts takes none of these: refused before it is even looked for.
	docx := filepath.Join(dir, "x.docx")
	os.WriteFile(docx, testpdf.MinimalDocx("x"), 0o600)
	_, stderr, code := fillChild(t, "", "", "office", docx, "-o", o, "--page-size", "a4")
	if code != 1 || !strings.Contains(stderr, "apply to Markdown only") {
		t.Errorf("an option on a .docx: exit %d, %q", code, stderr)
	}
}

func TestPlaceholderValuesRefuseWhatTheyCannotCarry(t *testing.T) {
	big := strings.Repeat("x", maxPlaceholderValue+1)
	if _, err := placeholderValues("", []string{"k=" + big}); err == nil || strings.Contains(err.Error(), "xxxx") {
		t.Errorf("an over-long value: %v", err)
	}
	f := filepath.Join(t.TempDir(), "v.json")
	os.WriteFile(f, []byte(`["not","an","object"]`), 0o600)
	if _, err := placeholderValues(f, nil); err == nil {
		t.Error("a JSON array was accepted as the value table")
	}
	var m map[string]string
	b, _ := json.Marshal(map[string]string{"a": "1"})
	json.Unmarshal(b, &m)
	if v, err := placeholderValues("", []string{"a=1", "a=2"}); err != nil || v["a"] != "2" {
		t.Errorf("a later --set must win: %v %v", v, err)
	}
}

// TestOfficeEmbedsLocalImagesOnlyFromTheFolderItIsGiven — I3. --image-dir turns local images on; without it they are
// skipped as before; a source outside the folder, or a URL, is refused by name.
func TestOfficeEmbedsLocalImagesOnlyFromTheFolderItIsGiven(t *testing.T) {
	dir := t.TempDir()
	im := image.NewRGBA(image.Rect(0, 0, 120, 60))
	var b bytes.Buffer
	png.Encode(&b, im)
	os.WriteFile(filepath.Join(dir, "logo.png"), b.Bytes(), 0o600)
	md := filepath.Join(dir, "t.md")
	os.WriteFile(md, []byte("# T\n\n![Our logo](logo.png)\n\nText.\n"), 0o600)
	out := filepath.Join(dir, "o.pdf")

	count := func(args ...string) (int, string, int) {
		_, stderr, code := fillChild(t, "", "", append([]string{"office", md, "-o", out}, args...)...)
		if code != 0 {
			return 0, stderr, code
		}
		pdf, _ := os.ReadFile(out)
		files, err := pdfops.ExtractImageFiles(pdf)
		if err != nil {
			t.Fatal(err)
		}
		return len(files), stderr, code
	}
	if n, stderr, code := count("--image-dir", dir); code != 0 || n != 1 {
		t.Fatalf("with --image-dir: %d picture(s), exit %d, %s", n, code, stderr)
	}
	if n, _, code := count(); code != 0 || n != 0 {
		t.Errorf("without --image-dir: %d picture(s), exit %d, want none (skipped, as before)", n, code)
	}
	os.WriteFile(md, []byte("![x](https://example.com/a.png)\n"), 0o600)
	if _, stderr, code := count("--image-dir", dir); code != 1 || !strings.Contains(stderr, "https://example.com/a.png") || !strings.Contains(stderr, "link") {
		t.Errorf("a URL: exit %d, %q, want 1 naming the source", code, stderr)
	}
	os.WriteFile(md, []byte("![x](../t.png)\n"), 0o600)
	if _, stderr, code := count("--image-dir", dir); code != 1 || !strings.Contains(stderr, "climbs") {
		t.Errorf("a .. path: exit %d, %q", code, stderr)
	}
}
