package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

const answers = `{"forms":[{"textfield":[{"name":"fullName","value":"Ada Lovelace"}]}]}`

// fillChild runs `nib args...` in a child (the test binary re-executed, see TestMain) with stdin
// as given, optionally under strace, and returns what it wrote to stdout and stderr.
func fillChild(t *testing.T, stdin string, trace string, args ...string) (stdout []byte, stderr string, code int) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	if trace != "" {
		cmd = exec.Command("strace", "-f", "-o", trace, "-e", "trace=openat,creat,mkdir,mkdirat,rename,renameat,renameat2,link,symlink,unlink,unlinkat", exe)
	}
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "TMPDIR="+t.TempDir(),
		helperEnv+"="+strings.Join(args, "\x1f"))
	var so, se bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(stdin), &so, &se
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	return so.Bytes(), se.String(), code
}

func fullName(t *testing.T, pdf []byte) string {
	t.Helper()
	exp, err := pdfops.ExportFormJSON(pdf)
	if err != nil {
		t.Fatalf("the output is not a readable form: %v", err)
	}
	return string(exp)
}

func TestFillTakesItsAnswersOnStdin(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(t.TempDir(), "form.pdf")
	if err := os.WriteFile(in, form, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("json to stdout", func(t *testing.T) {
		out, stderr, code := fillChild(t, answers, "", "fill", in, "--data", "-", "--format", "json", "-o", "-")
		if code != 0 || !strings.Contains(fullName(t, out), "Ada Lovelace") {
			t.Fatalf("exit %d, stderr %q, output form %s", code, stderr, fullName(t, out))
		}
	})
	t.Run("json is the default for a record with no extension", func(t *testing.T) {
		out, stderr, code := fillChild(t, answers, "", "fill", in, "--data", "-", "-o", "-")
		if code != 0 || !strings.Contains(fullName(t, out), "Ada Lovelace") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
	t.Run("xfdf", func(t *testing.T) {
		x := `<?xml version="1.0"?><xfdf xmlns="http://ns.adobe.com/xfdf/"><fields><field name="fullName"><value>Grace Hopper</value></field></fields></xfdf>`
		out, stderr, code := fillChild(t, x, "", "fill", in, "--data", "-", "--format", "xfdf", "-o", "-")
		if code != 0 || !strings.Contains(fullName(t, out), "Grace Hopper") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		// The same bytes read as JSON are refused, which is what the flag is for.
		if _, _, code := fillChild(t, x, "", "fill", in, "--data", "-", "-o", "-"); code == 0 {
			t.Error("an XFDF record on stdin with no --format was read as JSON and accepted")
		}
	})
	t.Run("the form and the record cannot both be stdin", func(t *testing.T) {
		_, stderr, code := fillChild(t, answers, "", "fill", "-", "--data", "-", "-o", "-")
		if code != 1 || !strings.Contains(stderr, "cannot both be") {
			t.Errorf("exit %d, stderr %q, want 1 and a message saying so", code, stderr)
		}
	})
	t.Run("a format that is neither", func(t *testing.T) {
		for _, f := range []string{"yaml", "csv"} {
			if _, _, code := fillChild(t, answers, "", "fill", in, "--data", "-", "--format", f, "-o", "-"); code != 1 {
				t.Errorf("--format %s exited %d, want 1", f, code)
			}
		}
		csv := filepath.Join(t.TempDir(), "d.csv")
		os.WriteFile(csv, []byte("fullName\nA\n"), 0o600)
		if _, _, code := fillChild(t, "", "", "fill", in, "--data", csv, "--format", "json", "--out-dir", t.TempDir()); code != 1 {
			t.Errorf("--format beside a .csv exited %d, want 1", code)
		}
	})
}

func TestFillFormat(t *testing.T) {
	for _, c := range []struct{ format, path, want string }{
		{"", "a.json", "json"}, {"", "a.XFDF", "xfdf"}, {"", "a.csv", "csv"}, {"", "-", "json"}, {"", "answers", "json"},
		{"xfdf", "-", "xfdf"}, {"XFDF", "a.json", "xfdf"}, {"json", "a.xfdf", "json"},
		{"csv", "-", ""}, {"json", "a.csv", ""}, {"yaml", "-", ""},
	} {
		got, ok := fillFormat(c.format, c.path)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("fillFormat(%q, %q) = %q, %v; want %q", c.format, c.path, got, ok, c.want)
		}
	}
}

// TestFillOnStdinToStdoutCreatesNothingOnDisk — the claim a caller handling a return's data relies on, judged by
// every file-creating system call of the whole process tree and not by a listing afterwards: a temp file made
// and removed leaves nothing to list. The control run, `-o FILE`, must show its `.nib-*.tmp` or the oracle is blind.
func TestFillOnStdinToStdoutCreatesNothingOnDisk(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the oracle is strace")
	}
	if _, err := exec.LookPath("strace"); err != nil {
		t.Skip("no strace on PATH")
	}
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "form.pdf")
	os.WriteFile(in, form, 0o600)
	created := func(trace string) []string {
		b, err := os.ReadFile(trace)
		if err != nil {
			t.Fatal(err)
		}
		var hits []string
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "ENOENT") || strings.Contains(l, "= -1") {
				continue
			}
			if strings.Contains(l, "O_CREAT") || strings.Contains(l, "O_WRONLY") || strings.Contains(l, "O_RDWR") ||
				strings.Contains(l, "mkdir") || strings.Contains(l, "rename") || strings.Contains(l, "link(") ||
				strings.Contains(l, "creat(") {
				hits = append(hits, l)
			}
		}
		return hits
	}
	ctl := filepath.Join(t.TempDir(), "ctl.trace")
	_, stderr, code := fillChild(t, answers, ctl, "fill", in, "--data", "-", "-o", filepath.Join(dir, "out.pdf"))
	if code != 0 {
		t.Fatalf("control exit %d: %s", code, stderr)
	}
	if hits := created(ctl); !strings.Contains(strings.Join(hits, "\n"), ".nib-") {
		t.Fatalf("the control run (-o FILE) shows no .nib-*.tmp, so the oracle cannot see a staged file:\n%s", strings.Join(hits, "\n"))
	}
	tr := filepath.Join(t.TempDir(), "run.trace")
	_, stderr, code = fillChild(t, answers, tr, "fill", in, "--data", "-", "--format", "json", "-o", "-")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if hits := created(tr); len(hits) > 0 {
		t.Errorf("stdin to stdout created or renamed files:\n%s", strings.Join(hits, "\n"))
	}
}
