package pdfops

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The HEIC door (P04.S02b, D2, W2, ADR-199). Tests that need a real decoder skip by name and say so; the
// sniff, classify, container-size, probe and argument tests run on stand-in tools and need nothing installed.

func needHEIF(t *testing.T) {
	t.Helper()
	if !HEIFAvailable() {
		t.Skip("present-path test not run: no HEIF decoder (heif-dec, heif-convert, magick) is installed on this machine")
	}
}

func needShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stand-in tools are shell scripts")
	}
}

// testPNG is a w×h PNG, what a stand-in decoder "decodes" to.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// standIn writes an executable that logs its arguments, MAGICK_CONFIGURE_PATH and that variable's policy
// file to log, then runs body (with $IN and $OUT set to the input and output paths, a "heic:" / "png:"
// prefix already removed). It returns the tool.
func standIn(t *testing.T, name, log, body string) heifTool {
	t.Helper()
	needShell(t)
	dir := t.TempDir()
	script := `#!/bin/sh
echo "$@" >> '` + log + `'
echo "CFG=$MAGICK_CONFIGURE_PATH" >> '` + log + `'
if [ -n "$MAGICK_CONFIGURE_PATH" ]; then cat "$MAGICK_CONFIGURE_PATH/policy.xml" >> '` + log + `'; fi
for a in "$@"; do IN="$OUT"; OUT="$a"; done
IN="${IN#heic:}"; OUT="${OUT#png:}"
` + body + "\n"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return heifTool{name: name, path: p}
}

func TestHEICIsClassifiedByItsBytesAndItsName(t *testing.T) {
	heic := heifProbeImage
	avif := append([]byte{0, 0, 0, 0x1c}, []byte("ftypavif\x00\x00\x00\x00mif1avifmiaf")...)
	for _, c := range []struct {
		name string
		head []byte
		want string
		err  error
	}{
		{"a.heic", heic, "heic", nil},
		{"A.HEIF", heic, "heic", nil},
		// A phone or a mail client renames pictures: a picture is a picture whatever it is called.
		{"a.jpg", heic, "heic", nil},
		{"", heic, "heic", nil},
		// A different KIND of document is the contradiction.
		{"a.docx", heic, "", ErrExtensionDisagrees},
		{"a.ps", heic, "", ErrExtensionDisagrees},
		// A .heic that is not HEIC, and an AV1 picture (AVIF), which Nib does not claim.
		{"a.heic", []byte("hello"), "", ErrExtensionDisagrees},
		// W2: MVG text under a HEIC name is never handed to a tool that would draw it.
		{"a.heic", []byte("push graphic-context\nviewbox 0 0 64 64\nimage over 0,0 0,0 'label:@/etc/passwd'\npop graphic-context\n"), "", ErrExtensionDisagrees},
		{"a.avif", avif, "", ErrNotADocument},
		{"a.heic", avif, "", ErrExtensionDisagrees},
	} {
		f, err := classify(c.head, c.name)
		if c.want == "" {
			if !errors.Is(err, c.err) {
				t.Errorf("%q: err = %v, want %v", c.name, err, c.err)
			}
			continue
		}
		if err != nil || f.Key != c.want {
			t.Errorf("%q: got (%q, %v), want %q", c.name, f.Key, err, c.want)
		}
	}
}

func TestOnlyHEIFBrandsAreHEIF(t *testing.T) {
	box := func(major string, compat ...string) []byte {
		b := []byte("\x00\x00\x00\x00ftyp" + major + "\x00\x00\x00\x00")
		for _, c := range compat {
			b = append(b, c...)
		}
		binary.BigEndian.PutUint32(b[0:4], uint32(len(b)))
		return b
	}
	for _, c := range []struct {
		head []byte
		want bool
	}{
		{box("heic"), true},
		{box("mif1", "heic"), true},
		{box("isom", "mif1"), true},
		{box("isom", "mp41"), false}, // an MP4
		{box("avif", "mif1"), false},
		{box("mif1", "avis"), false},
		{box("heic", "avif"), false},
		{[]byte("ftyp"), false},
		// The minor version (bytes 12-15) is not a brand: "heic" there is a number that happens to spell it.
		{box("isom", ""), false},
	} {
		if got := isHEIF(c.head); got != c.want {
			t.Errorf("%q: isHEIF = %v, want %v", c.head, got, c.want)
		}
	}
	minor := []byte("\x00\x00\x00\x14ftypisomheic\x00\x00\x00\x00")
	if isHEIF(minor) {
		t.Error("a minor version spelling heic was read as a brand")
	}
}

func TestTheContainerSaysHowBigEachPictureIs(t *testing.T) {
	// `ispe` is the CODED size: the encoder pads to its block grid (64×48 is coded 64×64 and cropped by a
	// `clap` property), so the cap reads a size at least as large as the picture shown.
	sizes := heifDeclaredSizes(heifProbeImage)
	if len(sizes) != 1 || sizes[0] != [2]uint32{64, 64} {
		t.Fatalf("declared sizes = %v, want [[64 64]]", sizes)
	}
	if got := heifDeclaredSizes([]byte("not a container at all")); len(got) != 0 {
		t.Errorf("garbage declared %v", got)
	}
	if got := heifDeclaredSizes(nil); len(got) != 0 {
		t.Errorf("nil declared %v", got)
	}
}

// withSize returns the probe picture with its declared size replaced.
func withSize(t *testing.T, w, h uint32) []byte {
	t.Helper()
	b := append([]byte(nil), heifProbeImage...)
	i := bytes.Index(b, []byte("ispe"))
	if i < 0 {
		t.Fatal("the probe picture has no ispe")
	}
	binary.BigEndian.PutUint32(b[i+8:], w)
	binary.BigEndian.PutUint32(b[i+12:], h)
	return b
}

func TestAPictureOverThePixelCapIsRefusedBeforeAnyDecoderRuns(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	tool := standIn(t, "heif-convert", log, `exit 1`)
	for _, c := range []struct {
		w, h uint32
		ok   bool
	}{
		{20000, 10000, true},  // exactly 200 Mpx
		{20000, 10001, false}, // one row over
		{1 << 31, 1, false},   // would overflow an int on a 32-bit build
		{65535, 65535, false},
	} {
		os.Remove(log)
		_, err := convertHEICWith(context.Background(), []heifTool{tool}, withSize(t, c.w, c.h))
		if c.ok {
			// Past the cap check, the stand-in has no PNG to hand back: any outcome but the pixel refusal is fine.
			if errors.Is(err, ErrImagePixels) {
				t.Errorf("%d×%d refused as too large", c.w, c.h)
			}
			continue
		}
		if !errors.Is(err, ErrImagePixels) {
			t.Errorf("%d×%d: err = %v, want ErrImagePixels", c.w, c.h, err)
		}
		if _, serr := os.Stat(log); serr == nil {
			t.Errorf("%d×%d: a decoder was started for a picture over the cap", c.w, c.h)
		}
	}
}

func TestNoDecoderIsAMissingTool(t *testing.T) {
	_, err := convertHEICWith(context.Background(), nil, heifProbeImage)
	if !errors.Is(err, ErrHEIFMissing) {
		t.Fatalf("err = %v", err)
	}
	if tool, ok := MissingToolFor(err); !ok || tool != ToolHEIF {
		t.Errorf("MissingToolFor = %v, %v", tool, ok)
	}
	if refusalCause(err) != "missing-tool" {
		t.Errorf("cause = %q", refusalCause(err))
	}
}

// A script that decodes by copying a PNG of a given size: $PNG is set by the caller's environment file.
func copyingBody(t *testing.T, w, h int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pic.png")
	if err := os.WriteFile(p, testPNG(t, w, h), 0o644); err != nil {
		t.Fatal(err)
	}
	return `cp '` + p + `' "$OUT"`
}

func TestAToolThatCannotReadHEICIsSkippedForTheNextAndNamedWhenNoneCan(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	blind := standIn(t, "heif-dec", log, `echo "no plugin for this format" >&2; exit 1`)
	wrong := standIn(t, "heif-convert", log, copyingBody(t, 10, 10)) // answers, but not with the test picture
	good := standIn(t, "magick", log, copyingBody(t, heifProbeW, heifProbeH))

	_, err := convertHEICWith(context.Background(), []heifTool{blind, wrong}, heifProbeImage)
	if !errors.Is(err, ErrHEIFCannotRead) {
		t.Fatalf("no tool reads HEIC: err = %v, want ErrHEIFCannotRead", err)
	}
	if errors.Is(err, ErrHEIFUnreadable) || refusalCause(err) != "tool-cannot-read" {
		t.Errorf("a blind tool was reported as a bad file: cause %q", refusalCause(err))
	}
	out, err := convertHEICWith(context.Background(), []heifTool{blind, wrong, good}, heifProbeImage)
	if err != nil {
		t.Fatalf("the third tool reads HEIC: %v", err)
	}
	if cfg, perr := png.DecodeConfig(bytes.NewReader(out)); perr != nil || cfg.Width != heifProbeW {
		t.Errorf("did not get the third tool's picture back: %v %v", cfg, perr)
	}
}

func TestTheProbeIsAskedOncePerBinary(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	tool := standIn(t, "heif-convert", log, copyingBody(t, heifProbeW, heifProbeH))
	for i := 0; i < 3; i++ {
		if _, err := convertHEICWith(context.Background(), []heifTool{tool}, heifProbeImage); err != nil {
			t.Fatal(err)
		}
	}
	runs := strings.Count(readFile(t, log), "CFG=")
	if runs != 4 { // one probe, three conversions
		t.Errorf("the tool ran %d times for three conversions, want 4 (one probe and three conversions)", runs)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestImageMagickIsNamedByPrefixAndGivenAPolicyOfItsOwn(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	tool := standIn(t, "magick", log, copyingBody(t, heifProbeW, heifProbeH))
	if _, err := convertHEICWith(context.Background(), []heifTool{tool}, heifProbeImage); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, log)
	if !strings.Contains(got, "heic:") || !strings.Contains(got, "png:") {
		t.Errorf("magick was not given both coders by prefix: %q", got)
	}
	if !strings.Contains(got, "CFG=/") || !strings.Contains(got, `<policy domain="delegate" rights="none" pattern="*" />`) {
		t.Errorf("magick ran without Nib's policy.xml: %q", got)
	}
	// libheif's tools get no policy and no prefix: their arguments are the two paths.
	log2 := filepath.Join(t.TempDir(), "log")
	lh := standIn(t, "heif-dec", log2, copyingBody(t, heifProbeW, heifProbeH))
	if _, err := convertHEICWith(context.Background(), []heifTool{lh}, heifProbeImage); err != nil {
		t.Fatal(err)
	}
	if g := readFile(t, log2); strings.Contains(g, "heic:") || strings.Contains(g, "policy") || !strings.Contains(g, "CFG=\n") {
		t.Errorf("heif-dec was given magick's arguments or policy: %q", g)
	}
}

func TestThePolicyAllowsTheTwoCodersAndNothingElse(t *testing.T) {
	var doc struct {
		Policies []struct {
			Domain  string `xml:"domain,attr"`
			Rights  string `xml:"rights,attr"`
			Pattern string `xml:"pattern,attr"`
		} `xml:"policy"`
	}
	if err := xml.Unmarshal([]byte(magickPolicy), &doc); err != nil {
		t.Fatalf("the policy is not XML: %v", err)
	}
	// Later rules win: the blanket denials must come BEFORE the two allowances, or they would undo them.
	deny := map[string]int{}
	for i, p := range doc.Policies {
		if p.Rights == "none" && p.Pattern == "*" {
			deny[p.Domain] = i
		}
		if p.Rights != "none" && p.Domain == "coder" && deny["coder"] > i {
			t.Errorf("coder allowance %q comes before the blanket denial and is undone by it", p.Pattern)
		}
		if p.Domain == "coder" && p.Rights != "none" {
			for _, bad := range []string{"MVG", "SVG", "MSL", "URL", "HTTP", "TEXT", "LABEL", "CAPTION", "EPHEMERAL", "PS", "PDF", "XPS"} {
				if strings.Contains(strings.ToUpper(p.Pattern), bad) {
					t.Errorf("the policy allows the %s coder", bad)
				}
			}
		}
	}
	for _, d := range []string{"delegate", "coder", "filter", "module"} {
		if _, ok := deny[d]; !ok {
			t.Errorf("no blanket denial for %s", d)
		}
	}
	if !strings.Contains(magickPolicy, `pattern="@*"`) {
		t.Error("`@file` paths are not denied")
	}
}

func TestAFileWithSeveralPicturesIsReadAtItsFirst(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	two := testPNG(t, 7, 9)
	p := filepath.Join(t.TempDir(), "pic.png")
	os.WriteFile(p, two, 0o644)
	q := filepath.Join(t.TempDir(), "probe.png")
	os.WriteFile(q, testPNG(t, heifProbeW, heifProbeH), 0o644)
	// Like libheif: the probe picture alone is out.png, a file of two is out-1.png and out-2.png.
	body := `if [ "$(wc -c < "$IN")" -gt 2000 ]; then cp '` + p + `' "${OUT%.png}-1.png"; cp '` + p + `' "${OUT%.png}-2.png"; else cp '` + q + `' "$OUT"; fi`
	tool := standIn(t, "heif-convert", log, body)
	data := append(append([]byte(nil), heifProbeImage...), make([]byte, 4000)...)
	out, err := convertHEICWith(context.Background(), []heifTool{tool}, data)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := png.DecodeConfig(bytes.NewReader(out))
	if cfg.Width != 7 {
		t.Errorf("did not read the first of the several pictures: %dx%d", cfg.Width, cfg.Height)
	}
}

func TestADecoderThatProducesNothingIsAnUnreadablePicture(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	q := filepath.Join(t.TempDir(), "probe.png")
	os.WriteFile(q, testPNG(t, heifProbeW, heifProbeH), 0o644)
	body := `if [ "$(wc -c < "$IN")" -gt 2000 ]; then echo "corrupt picture" >&2; exit 3; else cp '` + q + `' "$OUT"; fi`
	tool := standIn(t, "heif-convert", log, body)
	data := append(append([]byte(nil), heifProbeImage...), make([]byte, 4000)...)
	_, err := convertHEICWith(context.Background(), []heifTool{tool}, data)
	if !errors.Is(err, ErrHEIFUnreadable) || errors.Is(err, ErrHEIFCannotRead) {
		t.Fatalf("err = %v, want ErrHEIFUnreadable alone", err)
	}
	if !strings.Contains(err.Error(), "corrupt picture") {
		t.Errorf("the decoder's own words are lost: %v", err)
	}
	if refusalCause(err) != "unreadable" || !isNativeRefusal(err) {
		t.Errorf("cause %q native %v", refusalCause(err), isNativeRefusal(err))
	}
}

func TestAHEICOpensAsAOnePagePDFThroughTheRealDecoder(t *testing.T) {
	needHEIF(t)
	src, size := BytesSource(heifProbeImage)
	res, err := Import(context.Background(), src, size, "IMG_0001.heic", ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := PageCount(res.PDF); err != nil || n != 1 {
		t.Fatalf("pages = %d, %v", n, err)
	}
	if !res.Converted || res.Name != "IMG_0001.pdf" {
		t.Errorf("result %+v", res)
	}
	// The same bytes under a picture's other name open too.
	src, size = BytesSource(heifProbeImage)
	if _, err := Import(context.Background(), src, size, "IMG_0001.jpg", ImportOptions{}); err != nil {
		t.Errorf("renamed picture: %v", err)
	}
}

func TestEveryDecoderOnThisMachineReadsHEIC(t *testing.T) {
	tools := heifTools()
	if len(tools) == 0 {
		t.Skip("present-path test not run: no HEIF decoder is installed on this machine")
	}
	for _, tool := range tools {
		if err := heifProbe(context.Background(), tool); err != nil {
			t.Logf("%s (%s) cannot read HEIC: %v", tool.name, tool.path, err)
		}
	}
}

func TestTheHEICRowIsInTheTableAndGatedOnItsTool(t *testing.T) {
	f, ok := FormatForExt("HEIF")
	if !ok || f.Key != "heic" || f.Tool != ToolHEIF || f.Door {
		t.Fatalf("row %+v", f)
	}
	if got := missingToolFor(f); !errors.Is(got, ErrHEIFMissing) {
		t.Errorf("missingToolFor = %v", got)
	}
	if FormatAvailable(f) != HEIFAvailable() {
		t.Error("the row's availability is not the tool's")
	}
}

// A container that declares a zero extent is not a pixel-cap refusal: the decoder judges it (the survivor of the
// first mutation pass, which turned a zero into a sizeless-image error that named no cap).
func TestAZeroDeclaredSizeIsLeftToTheDecoder(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	tool := standIn(t, "heif-convert", log, `exit 1`)
	_, err := convertHEICWith(context.Background(), []heifTool{tool}, withSize(t, 0, 0))
	if errors.Is(err, ErrImagePixels) {
		t.Fatalf("a zero extent was refused as too much picture: %v", err)
	}
	if _, serr := os.Stat(log); serr != nil {
		t.Error("the decoder was never asked about a zero-extent picture")
	}
}
