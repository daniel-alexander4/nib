package cli

import (
	"bytes"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

// TestAnOutputNamingASignedInputIsRefused — /pending 777 (and 691's R9-3, the same defect).
//
// `-w` refused a signed PDF and `-o` naming the same file did not: `nib optimize a.pdf -o a.pdf`
// exited 0 and left a document whose signature no longer verified. Every `-o` writer that rewrites
// the document is driven here, by the same path, by a symlink and by a hard link, and the file on
// disk must be the signed bytes afterwards.
func TestAnOutputNamingASignedInputIsRefused(t *testing.T) {
	signed := signedTestPDF(t)
	extra := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(extra, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := writePDF(t, t.TempDir(), "other.pdf", "a")

	doors := []struct {
		name string
		run  func(in, out string) int
	}{
		{"optimize", func(in, out string) int { return cmdOptimize([]string{in, "-o", out}) }},
		{"rotate", func(in, out string) int { return cmdRotate([]string{in, "-o", out, "--deg", "90"}) }},
		{"pdfa", func(in, out string) int { return cmdPDFA([]string{in, "-o", out}) }},
		{"merge", func(in, out string) int { return cmdMerge([]string{other, in, "-o", out}) }},
		{"attachments --add", func(in, out string) int { return cmdAttachments([]string{in, "--add", extra, "-o", out}) }},
	}
	spellings := []struct {
		name string
		out  func(t *testing.T, in string) string
	}{
		{"the same path", func(t *testing.T, in string) string { return in }},
		{"a symlink to it", func(t *testing.T, in string) string {
			l := filepath.Join(filepath.Dir(in), "link.pdf")
			if err := os.Symlink(in, l); err != nil {
				t.Skipf("SKIP (not a pass): symlinks unavailable here: %v", err)
			}
			return l
		}},
		{"a hard link to it", func(t *testing.T, in string) string {
			l := filepath.Join(filepath.Dir(in), "hard.pdf")
			if err := os.Link(in, l); err != nil {
				t.Skipf("SKIP (not a pass): hard links unavailable here: %v", err)
			}
			return l
		}},
	}
	for _, d := range doors {
		for _, sp := range spellings {
			t.Run(d.name+"/"+sp.name, func(t *testing.T) {
				if runtime.GOOS == "windows" && sp.name != "the same path" {
					t.Skip("SKIP (not a pass): links need privileges on Windows")
				}
				in := filepath.Join(t.TempDir(), "signed.pdf")
				if err := os.WriteFile(in, signed, 0o644); err != nil {
					t.Fatal(err)
				}
				out := sp.out(t, in)
				var code int
				stderr := captureStderr(t, func() { code = d.run(in, out) })
				if code != 1 {
					t.Errorf("exit = %d, want 1 — the output replaces the signed input", code)
				}
				if !strings.Contains(stderr, "refusing to rewrite a signed PDF in place") {
					t.Errorf("stderr does not give the in-place refusal:\n%s", stderr)
				}
				if got := readPDF(t, in); !bytes.Equal(got, signed) {
					t.Errorf("the signed document was replaced (now verifies %q) — -o naming the input destroyed a signature -w refuses to touch", sign.Verify(got).State)
				}
			})
		}
	}

	// `pagenum --continuous --out-dir` naming the input's own folder is the same replacement.
	t.Run("pagenum --continuous --out-dir", func(t *testing.T) {
		dir := t.TempDir()
		in := filepath.Join(dir, "signed.pdf")
		if err := os.WriteFile(in, signed, 0o644); err != nil {
			t.Fatal(err)
		}
		var code int
		stderr := captureStderr(t, func() { code = cmdPagenum([]string{"--continuous", in, "--out-dir", dir}) })
		if code != 1 || !strings.Contains(stderr, "refusing to rewrite a signed PDF in place") {
			t.Errorf("exit = %d, want 1 with the in-place refusal:\n%s", code, stderr)
		}
		if got := readPDF(t, in); !bytes.Equal(got, signed) {
			t.Error("the signed document was replaced by its own stamped output")
		}
	})

	// The two things the refusal must not take: an unsigned document over itself, and a signed
	// document into a new file. A refusal of everything would pass every case above.
	t.Run("an unsigned input over itself is still written", func(t *testing.T) {
		in := writePDF(t, t.TempDir(), "plain.pdf", "a")
		if code := cmdRotate([]string{in, "-o", in, "--deg", "90"}); code != 0 {
			t.Errorf("rotate -o naming an unsigned input exit = %d, want 0", code)
		}
	})
	t.Run("a signed input into a new file is still written", func(t *testing.T) {
		dir := t.TempDir()
		in, out := filepath.Join(dir, "signed.pdf"), filepath.Join(dir, "copy.pdf")
		if err := os.WriteFile(in, signed, 0o644); err != nil {
			t.Fatal(err)
		}
		if code := cmdOptimize([]string{in, "-o", out}); code != 0 {
			t.Fatalf("optimize into a new file exit = %d, want 0", code)
		}
		if _, err := os.Stat(out); err != nil {
			t.Errorf("no copy was written: %v", err)
		}
		if got := readPDF(t, in); !bytes.Equal(got, signed) {
			t.Error("the original changed")
		}
	})
}

// TestAFileNameAfterTheTerminatorIsNotAFlag — /pending 777. `reorder` dropped `--` and handed the
// positionals back to `fs.Parse`, so the one escape for a file named `-weird.pdf` did not work.
func TestAFileNameAfterTheTerminatorIsNotAFlag(t *testing.T) {
	for _, args := range [][]string{
		{"-o", "o.pdf", "--", "-weird.pdf"},
		{"--", "-weird.pdf", "-o"},
		{"a.pdf", "-o", "o.pdf", "--", "-weird.pdf"},
	} {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		var out string
		outFlag(fs, &out)
		var code int
		var ok bool
		stderr := captureStderr(t, func() { code, ok = parse(fs, args) })
		if !ok {
			t.Errorf("%q: parse refused (code %d): %s", args, code, stderr)
			continue
		}
		if got := fs.Args(); !strings.Contains(strings.Join(got, " "), "-weird.pdf") {
			t.Errorf("%q: positionals = %q, want -weird.pdf among them", args, got)
		}
	}
	// A flag BEFORE the terminator is still a flag, and one after it is a file.
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var out string
	outFlag(fs, &out)
	if _, ok := parse(fs, []string{"a.pdf", "-o", "o.pdf", "--", "-o"}); !ok || out != "o.pdf" || strings.Join(fs.Args(), " ") != "a.pdf -o" {
		t.Errorf("out = %q, positionals = %q; want o.pdf and [a.pdf -o]", out, fs.Args())
	}
}

// TestAFileNamedTwiceIsRewrittenOnce — /pending 670 (R7 #8). `nib rotate -w a.pdf ./a.pdf --deg 90`
// turned the page 180°: `-w` never asked whether two arguments were one file.
func TestAFileNamedTwiceIsRewrittenOnce(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "a.pdf", "a")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Another spelling of the same file; filepath.Join would clean it back to the first.
	again := dir + string(filepath.Separator) + "sub" + string(filepath.Separator) + ".." + string(filepath.Separator) + "a.pdf"
	stdout, code := captureStdout(t, func() int { return cmdRotate([]string{"-w", in, again, "--deg", "90"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, stdout)
	}
	if n := strings.Count(stdout, ": rewritten"); n != 1 {
		t.Errorf("the file was rewritten %d times, want once:\n%s", n, stdout)
	}
	if !strings.Contains(stdout, "skipped (the same file as") {
		t.Errorf("the second spelling is not reported as skipped:\n%s", stdout)
	}

	// `pagenum --continuous -w` threads one counter through the files, so it refuses instead.
	before := readPDF(t, in)
	var pcode int
	stderr := captureStderr(t, func() { pcode = cmdPagenum([]string{"--continuous", "-w", in, again}) })
	if pcode != 1 || !strings.Contains(stderr, "is the same file as") {
		t.Errorf("pagenum --continuous -w over one file twice: exit = %d, want 1 naming it:\n%s", pcode, stderr)
	}
	if !bytes.Equal(readPDF(t, in), before) {
		t.Error("the file was stamped before the duplicate was refused")
	}
}

// TestVerifyNamesTheKeyThatSigned — /pending 626. One party signs; someone else signs over the result
// to the end of the file. Every signature is valid, nothing is "added after the last signature", and
// `nib verify` said `valid (2 signer(s))` and nothing about who. Each signer's key is now on its own line.
func TestVerifyNamesTheKeyThatSigned(t *testing.T) {
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, name := range []string{"First", "Stranger"} {
		certPEM, keyPEM, err := sign.GenerateIdentity(name)
		if err != nil {
			t.Fatal(err)
		}
		if pdf, err = sign.SignApproval(pdf, certPEM, keyPEM, sign.Options{Name: name, When: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)}); err != nil {
			t.Fatalf("sign as %s: %v", name, err)
		}
		fp, err := sign.Fingerprint(certPEM)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, hex.EncodeToString(fp))
	}
	// STIMULUS: the shape the item is about — two valid signers and no added-after warning.
	if st := sign.Verify(pdf); st.State != sign.Valid || len(st.Signers) != 2 || st.AddedAfter {
		t.Fatalf("setup: state %q, %d signer(s), addedAfter %v; want valid, 2, false", st.State, len(st.Signers), st.AddedAfter)
	}
	p := filepath.Join(t.TempDir(), "resigned.pdf")
	if err := os.WriteFile(p, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, code := captureStdout(t, func() int { return cmdVerify([]string{p}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, stdout)
	}
	low := strings.ToLower(stdout)
	for i, fp := range want {
		if !strings.Contains(low, "key "+strings.ToLower(fp)) {
			t.Errorf("signer %d's key %s is not named — the report says how many signed and not who:\n%s", i+1, fp, stdout)
		}
	}
	if !strings.Contains(stdout, "not whose key it is") {
		t.Errorf("the report does not say what a valid signature leaves unanswered:\n%s", stdout)
	}
}
