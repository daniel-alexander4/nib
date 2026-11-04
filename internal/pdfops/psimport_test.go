package pdfops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"nib/internal/tempdir"
)

// The PostScript door (P04.S02a, X11(b), ADR-198). Tests that need Ghostscript skip by name when it is
// absent and say how many; the sniff, classify and version tests need nothing installed.

const psHello = "%!PS\n/Helvetica findfont 24 scalefont setfont 72 700 moveto (hello nib) show showpage\n"

const epsHello = "%!PS-Adobe-3.0 EPSF-3.0\n%%BoundingBox: 0 0 200 100\n/Helvetica findfont 24 scalefont setfont 10 40 moveto (eps nib) show\n"

func needGS(t *testing.T) {
	t.Helper()
	if !GhostscriptAvailable() {
		t.Skip("present-path test not run: Ghostscript is not installed on this machine")
	}
}

func TestPostScriptIsClassifiedByItsBytesAndItsName(t *testing.T) {
	for _, c := range []struct {
		name string
		head string
		want string // table key, or "" for a refusal
		err  error
	}{
		{"a.ps", psHello, "ps", nil},
		{"a.eps", epsHello, "eps", nil},
		{"a.eps", "\xC5\xD0\xD3\xC6rest", "eps", nil},
		{"A.PS", psHello, "ps", nil},
		// A text file that begins `%!` is text and is never run.
		{"notes.txt", "%!PS looks like a program but is notes\n", "txt", nil},
		// PostScript under another document's name, or no name, is a contradiction, not a guess.
		{"a.docx", psHello, "", ErrExtensionDisagrees},
		{"", psHello, "", ErrExtensionDisagrees},
		{"a.png", psHello, "", ErrExtensionDisagrees},
		// A `.ps` that is not PostScript.
		{"a.ps", "hello, not a program", "", ErrExtensionDisagrees},
		{"a.eps", "PK\x03\x04", "", ErrExtensionDisagrees},
	} {
		f, err := classify([]byte(c.head), c.name)
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

func TestTheGhostscriptVersionFloor(t *testing.T) {
	for in, want := range map[string]bool{"10.02.1\n": true, "9.50": true, "9.56.1": true, "9.27": false, "9.49.9": false, "8.71": false, "10.0": true} {
		v, ok := parseGSVersion(in)
		if !ok {
			t.Errorf("%q did not parse", in)
			continue
		}
		if got := !v.less(gsFloor); got != want {
			t.Errorf("%q: meets the floor = %v, want %v", in, got, want)
		}
	}
	for _, bad := range []string{"", "gs", "10", "x.y", "-1.2"} {
		if _, ok := parseGSVersion(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

// The flags are the door: SAFER present, nothing that widens it, the page bound, and the input after "--".
func TestThePostScriptArgumentsAllowNothing(t *testing.T) {
	for _, eps := range []bool{false, true} {
		args := psArgs("/d/in.ps", "/d/out.pdf", eps)
		joined := " " + strings.Join(args, " ") + " "
		for _, want := range []string{" -dSAFER ", " -sDEVICE=pdfwrite ", fmt.Sprintf(" -dLastPage=%d ", importPages+1), " -sOutputFile=/d/out.pdf ", " -- /d/in.ps "} {
			if !strings.Contains(joined, want) {
				t.Errorf("eps=%v: arguments lack %q: %v", eps, want, args)
			}
		}
		for _, banned := range []string{"NOSAFER", "DELAYSAFER", "permit-file", "-sDEVICE=x11", "-c"} {
			if strings.Contains(joined, banned) {
				t.Errorf("eps=%v: arguments carry %q: %v", eps, banned, args)
			}
		}
		if got := strings.Contains(joined, " -dEPSCrop "); got != eps {
			t.Errorf("eps=%v: -dEPSCrop present = %v", eps, got)
		}
	}
}

func TestPostScriptAndEPSBecomeOnePage(t *testing.T) {
	needGS(t)
	for name, src := range map[string]string{"a.ps": psHello, "a.eps": epsHello} {
		res, err := Import(context.Background(), bytes.NewReader([]byte(src)), int64(len(src)), name, ImportOptions{Door: "test"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !res.Converted || res.Name != strings.TrimSuffix(name, filepath.Ext(name))+".pdf" {
			t.Errorf("%s: result %+v", name, res)
		}
		if n, err := PageCount(res.PDF); err != nil || n != 1 {
			t.Errorf("%s: pages = %d, %v", name, n, err)
		}
	}
}

// An EPS page is its bounding box (-dEPSCrop), not a Letter sheet.
func TestAnEPSPageIsItsBoundingBox(t *testing.T) {
	needGS(t)
	pdf, err := ConvertPostScriptToPDF(context.Background(), []byte(epsHello), true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/MediaBox[0 0 200 100]")) && !bytes.Contains(pdf, []byte("/MediaBox [0 0 200 100]")) {
		i := bytes.Index(pdf, []byte("/MediaBox"))
		t.Fatalf("media box is not the bounding box: %.60q", pdf[max(i, 0):])
	}
	plain, err := ConvertPostScriptToPDF(context.Background(), []byte(epsHello), false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(plain, []byte("/MediaBox[0 0 200 100]")) || bytes.Contains(plain, []byte("/MediaBox [0 0 200 100]")) {
		t.Error("without the crop the page should not be the bounding box (the control for the crop test)")
	}
}

// leakProgram reads a file and starts a command. Under -dNOSAFER it does both (the positive control); the
// door must do neither.
func leakProgram(secretPath, ranPath string) string {
	return "%!PS\n" +
		"{ (" + psEscape(secretPath) + ") (r) file closefile (LEAK) print } stopped { (HELD) print } if\n" +
		"{ (%pipe%touch " + psEscape(ranPath) + ") (r) file closefile (LEAK) print } stopped { (HELD) print } if\n" +
		"/Helvetica findfont 24 scalefont setfont 72 700 moveto (page) show showpage\n"
}

// The fixture is a leak with the confinement removed — otherwise "nothing leaked" proves nothing.
func TestTheLeakFixtureLeaksWithoutTheConfinement(t *testing.T) {
	needGS(t)
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	ran := filepath.Join(dir, "ran")
	mustWrite(t, secret, "s3cret")
	mustWrite(t, filepath.Join(dir, "in.ps"), leakProgram(secret, ran))
	cmd := exec.Command(ghostscriptPath(), "-dNOSAFER", "-dBATCH", "-dNOPAUSE", "-dQUIET", "-sDEVICE=nullpage", filepath.Join(dir, "in.ps"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("control run: %v", err)
	}
	if string(out) != "LEAKLEAK" {
		t.Fatalf("control printed %q: the fixture does not leak, so the next test proves nothing", out)
	}
	if _, err := os.Stat(ran); err != nil {
		t.Fatalf("control did not start its command: %v", err)
	}
}

func TestAPostScriptProgramCannotReadAFileOrStartACommand(t *testing.T) {
	needGS(t)
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	ran := filepath.Join(dir, "ran")
	mustWrite(t, secret, "s3cret")
	// Print what the program saw into the PAGE, so the output is the observable: a leak shows as text.
	src := "%!PS\n/Courier findfont 12 scalefont setfont 20 700 moveto\n" +
		"{ (" + psEscape(secret) + ") (r) file 100 string readline pop show } stopped { (HELD-READ) show } if\n" +
		"0 -20 rmoveto\n" +
		"{ (%pipe%touch " + psEscape(ran) + ") (r) file closefile (RAN) show } stopped { (HELD-PIPE) show } if\n" +
		"showpage\n"
	res, err := Import(context.Background(), strings.NewReader(src), int64(len(src)), "leak.ps", ImportOptions{Door: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("the program started a command")
	}
	text := pdfTextOf(t, res.PDF)
	for _, bad := range []string{"s3cret", "RAN"} {
		if strings.Contains(text, bad) {
			t.Errorf("the page carries %q: %q", bad, text)
		}
	}
	for _, want := range []string{"HELD-READ", "HELD-PIPE"} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q (the program did not run, or ran differently): %q", want, text)
		}
	}
}

func TestTheConfinementProbeHoldsHereAndRefusesAnUnconfinedGhostscript(t *testing.T) {
	needGS(t)
	if err := probeGhostscript(context.Background(), ghostscriptPath()); err != nil {
		t.Fatalf("the probe refused this machine's Ghostscript: %v", err)
	}
	// A wrapper that drops -dSAFER is an unconfined Ghostscript: the probe must refuse it.
	dir := t.TempDir()
	wrap := filepath.Join(dir, "gs")
	mustWrite(t, wrap, "#!/bin/sh\nargs=\"\"\nfor a in \"$@\"; do case \"$a\" in -dSAFER) a=-dNOSAFER;; esac; args=\"$args '$a'\"; done\neval exec "+ghostscriptPath()+" $args\n")
	if err := os.Chmod(wrap, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := probeGhostscript(context.Background(), wrap); !errors.Is(err, ErrGhostscriptNotConfined) {
		t.Fatalf("an unconfined Ghostscript passed the probe: %v", err)
	}
}

func TestAGhostscriptBelowTheFloorIsRefusedByName(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "gs")
	mustWrite(t, old, "#!/bin/sh\necho 9.27\n")
	if err := os.Chmod(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := probeGhostscript(context.Background(), old); !errors.Is(err, ErrGhostscriptTooOld) {
		t.Fatalf("err = %v, want ErrGhostscriptTooOld", err)
	}
	broken := filepath.Join(dir, "gs2")
	mustWrite(t, broken, "#!/bin/sh\necho not-a-version\n")
	if err := os.Chmod(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := probeGhostscript(context.Background(), broken); !errors.Is(err, ErrGhostscriptNotConfined) {
		t.Fatalf("an unparsable version should refuse, got %v", err)
	}
}

func TestAPostScriptThatRunsForeverIsEndedAndLeavesNoProcess(t *testing.T) {
	needGS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	src := "%!PS\n{ } loop\n"
	start := time.Now()
	_, err := Import(ctx, strings.NewReader(src), int64(len(src)), "loop.ps", ImportOptions{Door: "test"})
	if err == nil {
		t.Fatal("an endless program was reported as converted")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("the program was ended after %v", time.Since(start))
	}
	// The process tree is gone: no gs is still running our program.
	if out, _ := exec.Command("pgrep", "-f", `[d]LastPage=[0-9]+ .*/in/in\.ps`).Output(); strings.TrimSpace(string(out)) != "" {
		t.Fatalf("a Ghostscript is still running the program: %s", out)
	}
}

func TestAPostScriptPastThePageCapIsRefusedByName(t *testing.T) {
	needGS(t)
	src := fmt.Sprintf("%%!PS\n/Helvetica findfont 12 scalefont setfont 1 1 %d { pop 20 20 moveto (p) show showpage } for\n", importPages+5)
	_, err := Import(context.Background(), strings.NewReader(src), int64(len(src)), "many.ps", ImportOptions{Door: "test"})
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("err = %v, want ErrTooManyPages", err)
	}
}

// A program Ghostscript cannot run is a conversion that failed (a sentence from Ghostscript), not a crash.
func TestAPostScriptWithASyntaxErrorIsAFailedConversion(t *testing.T) {
	needGS(t)
	src := "%!PS\n( unterminated string\n"
	_, err := Import(context.Background(), strings.NewReader(src), int64(len(src)), "bad.ps", ImportOptions{Door: "test"})
	var conv *ConversionError
	if !errors.As(err, &conv) {
		t.Fatalf("err = %v, want a ConversionError", err)
	}
}

// A Ghostscript that exits cleanly and writes something that is not a PDF is ErrPostScriptUnreadable.
func TestOutputThatIsNotAPDFIsNamed(t *testing.T) {
	needGS(t)
	dir := t.TempDir()
	wrap := filepath.Join(dir, "gs")
	// The probe's own invocation runs the real Ghostscript; every other one writes junk to its -sOutputFile.
	mustWrite(t, wrap, "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in *probe.ps) exec "+ghostscriptPath()+" \"$@\";; -sOutputFile=*) out=\"${a#-sOutputFile=}\";; esac; done\n"+
		"case \"$1\" in --version) exec "+ghostscriptPath()+" --version;; esac\nprintf 'junk' > \"$out\"\n")
	if err := os.Chmod(wrap, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := convertPostScriptWith(context.Background(), wrap, []byte(psHello), false)
	if !errors.Is(err, ErrPostScriptUnreadable) {
		t.Fatalf("err = %v, want ErrPostScriptUnreadable", err)
	}
}

// The Ghostscript lookup caches a hit for the process (toolpath.go), so the absent case is a fresh process with
// an empty PATH — the same test binary run again, which asks the door the question with nothing to find.
func TestPostScriptWithoutGhostscriptNamesTheTool(t *testing.T) {
	if os.Getenv("NIB_TEST_NO_GS") == "1" {
		_, err := Import(context.Background(), strings.NewReader(psHello), int64(len(psHello)), "a.ps", ImportOptions{Door: "test"})
		if tool, ok := MissingToolFor(err); ok && tool == ToolGhostscript && ghostscriptPath() == "" && !psRowAvailable() {
			fmt.Println("MISSING-TOOL-NAMED")
		} else {
			fmt.Printf("WRONG: err=%v\n", err)
		}
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("PATH-less lookup still finds a stock install on Windows")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPostScriptWithoutGhostscriptNamesTheTool$")
	cmd.Env = []string{"NIB_TEST_NO_GS=1", "PATH=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "MISSING-TOOL-NAMED") {
		t.Fatalf("a machine without Ghostscript: %v\n%s", err, out)
	}
}

// The source is never written, and the converter's directory is gone after it, success or refusal.
func TestPostScriptLeavesNoDirectoryBehind(t *testing.T) {
	needGS(t)
	before := nibTempDirs(t)
	for _, src := range []string{psHello, "%!PS\n"} {
		_, _ = Import(context.Background(), strings.NewReader(src), int64(len(src)), "a.ps", ImportOptions{Door: "test"})
	}
	if after := nibTempDirs(t); len(after) > len(before) {
		t.Fatalf("directories left behind: before %v after %v", before, after)
	}
}

func mustWrite(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o700); err != nil {
		t.Fatal(err)
	}
}

func nibTempDirs(t *testing.T) []string {
	t.Helper()
	root, err := tempdir.Root()
	if err != nil {
		t.Fatal(err)
	}
	es, _ := os.ReadDir(root)
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func psRowAvailable() bool {
	f, ok := FormatForExt("ps")
	return ok && FormatAvailable(f)
}

func TestThePostScriptRefusalsAreCountedAndWorded(t *testing.T) {
	for err, want := range map[error]string{
		ErrGhostscriptTooOld:      "tool-unsafe",
		ErrGhostscriptNotConfined: "tool-unsafe",
		ErrPostScriptUnreadable:   "unreadable",
		ErrTooManyPages:           "cap:pages",
	} {
		if got := refusalCause(fmt.Errorf("wrapped: %w", err)); got != want {
			t.Errorf("%v: cause %q, want %q", err, got, want)
		}
		if !isNativeRefusal(fmt.Errorf("wrapped: %w", err)) {
			t.Errorf("%v is not a native refusal, so it would be reported as a failed conversion", err)
		}
	}
}

// fakeGS writes a stand-in Ghostscript: `--version` answers 10.02.1 and the rest of the script is body.
func fakeGS(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gs")
	mustWrite(t, p, "#!/bin/sh\ncase \"$1\" in --version) echo 10.02.1; exit 0;; esac\n"+body+"\n")
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// Each half of the probe's verdict is tested alone: the standard output (a program that read the file) and the
// marker (a program that started the command), so neither is carried by the other.
func TestTheProbeRefusesOnEitherSignalAlone(t *testing.T) {
	readOnly := fakeGS(t, "printf LEAKHELD")
	if err := probeGhostscript(context.Background(), readOnly); !errors.Is(err, ErrGhostscriptNotConfined) {
		t.Errorf("a program that read the file passed the probe: %v", err)
	}
	// Says HELDHELD but ran the command: the path is in the probe's own program.
	ranOnly := fakeGS(t, `for a in "$@"; do in="$a"; done
p=$(sed -n 's/.*%pipe%touch \(.*\)) (r) file.*/\1/p' "$in")
touch "$p"
printf HELDHELD`)
	if err := probeGhostscript(context.Background(), ranOnly); !errors.Is(err, ErrGhostscriptNotConfined) {
		t.Errorf("a program that started the command passed the probe: %v", err)
	}
	clean := fakeGS(t, "printf HELDHELD")
	if err := probeGhostscript(context.Background(), clean); err != nil {
		t.Errorf("a Ghostscript that held both was refused: %v", err)
	}
}

func TestADocumentWithNoPagesIsNamed(t *testing.T) {
	empty := "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[]/Count 0>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n"
	gs := fakeGS(t, `for a in "$@"; do case "$a" in -sOutputFile=*) out="${a#-sOutputFile=}";; esac; done
case "$*" in *probe.ps*) printf HELDHELD; exit 0;; esac
printf '%s' '`+strings.ReplaceAll(empty, "'", "")+`' > "$out"`)
	_, err := convertPostScriptWith(context.Background(), gs, []byte(psHello), false)
	if !errors.Is(err, ErrPostScriptUnreadable) {
		t.Fatalf("err = %v, want ErrPostScriptUnreadable", err)
	}
}
