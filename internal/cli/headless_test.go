package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// helperEnv names the child run: the test binary re-executes itself so a verb builds its FIRST
// pdfcpu configuration in a process whose cache no earlier test has filled.
const helperEnv = "NIB_TEST_CLI_ARGS"

func TestMain(m *testing.M) {
	if a := os.Getenv(helperEnv); a != "" {
		if os.Getenv("NIB_TEST_OPEN_CONFIG") == "1" { // the control: build a configuration before any verb closes it
			model.NewDefaultConfiguration()
		}
		_, code := Run(strings.Split(a, "\x1f"), "test")
		Cleanup()
		os.Exit(code)
	}
	code := m.Run()
	Cleanup()
	os.Exit(code)
}

// childRun runs `nib args...` in a child with HOME, XDG_CONFIG_HOME and TMPDIR in three empty
// directories and returns them with the exit code, stdout and stderr.
func childRun(t *testing.T, env []string, args ...string) (dirs [3]string, code int, stdout, stderr string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for i := range dirs {
		dirs[i] = t.TempDir()
	}
	cmd := exec.Command(exe)
	cmd.Env = append([]string{
		"HOME=" + dirs[0], "XDG_CONFIG_HOME=" + dirs[1], "TMPDIR=" + dirs[2],
		"APPDATA=" + dirs[1], helperEnv + "=" + strings.Join(args, "\x1f"),
	}, env...)
	var so, se strings.Builder
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return dirs, code, so.String(), se.String()
}

func empty(t *testing.T, dir string) {
	t.Helper()
	var found []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && p != dir {
			found = append(found, p)
		}
		return nil
	})
	if len(found) > 0 {
		t.Errorf("%s is not empty: %v", dir, found)
	}
}

// TestHeadlessRefusesEveryNonVerb — ADR-183: with NIB_HEADLESS=1 nothing but a verb runs, so the
// desktop app (a loopback server and a browser) can never start from a program's call. Without
// it the old fall-through stands, which is what opens a file by double-click.
func TestHeadlessRefusesEveryNonVerb(t *testing.T) {
	t.Setenv("NIB_HEADLESS", "1")
	for _, args := range [][]string{{"bogus"}, {"some.pdf"}, {}, {"bogus", "--out", "x"}, {"--replace"}} {
		stderr := captureStderr(t, func() {
			handled, code := Run(args, "t")
			if !handled || code != 1 {
				t.Errorf("Run(%q) = handled %v, code %d; want handled, 1", args, handled, code)
			}
		})
		if !strings.Contains(stderr, "nib:") {
			t.Errorf("Run(%q) said %q, want a nib: refusal", args, stderr)
		}
	}
	if stderr := captureStderr(t, func() { Run([]string{"bogus"}, "t") }); !strings.Contains(stderr, "unknown command") {
		t.Errorf("an unknown verb under NIB_HEADLESS said %q, want \"unknown command\"", stderr)
	}
	out, code := captureStdout(t, func() int { _, c := Run([]string{"version"}, "9.9.9"); return c })
	if code != 0 || !strings.Contains(out, "9.9.9") {
		t.Errorf("version under NIB_HEADLESS = %d %q, want it to answer", code, out)
	}
}

func TestWithoutHeadlessANonVerbFallsThrough(t *testing.T) {
	t.Setenv("NIB_HEADLESS", "")
	for _, args := range [][]string{{"bogus"}, {"some.pdf"}, {}} {
		if handled, _ := Run(args, "t"); handled {
			t.Errorf("Run(%q) handled a non-verb with NIB_HEADLESS unset: the double-click would stop opening files", args)
		}
	}
	t.Setenv("NIB_HEADLESS", "true")
	if handled, _ := Run([]string{"some.pdf"}, "t"); handled {
		t.Error("only the value 1 is the switch")
	}
}

// TestNoVerbWritesThePdfcpuConfiguration — ADR-183. Each verb runs in a child whose HOME,
// XDG_CONFIG_HOME and TMPDIR are empty; all three must still be empty after.
func TestNoVerbWritesThePdfcpuConfiguration(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	in := filepath.Join(work, "form.pdf")
	data := filepath.Join(work, "a.json")
	if err := os.WriteFile(in, form, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Ada"}]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(work, "note.md")
	if err := os.WriteFile(md, []byte("# Title\n\nSome text.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"fill":        {"fill", in, "--data", data, "-o", filepath.Join(work, "f.pdf")},
		"fields":      {"fields", in, "--json"},
		"text":        {"text", in, "--json"},
		"info":        {"info", in, "--json"},
		"export-xfdf": {"export-xfdf", in, "-o", filepath.Join(work, "e.xfdf")},
		"merge":       {"merge", in, in, "-o", filepath.Join(work, "m.pdf")},
		"split":       {"split", in, "--every", "1", "--out-dir", filepath.Join(work, "s")},
		"stamp":       {"stamp", in, "--text", "X", "-o", filepath.Join(work, "st.pdf")},
		"pagenum":     {"pagenum", in, "-o", filepath.Join(work, "pn.pdf")},
		"office-md":   {"office", md, "-o", filepath.Join(work, "md.pdf")},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			dirs, code, _, stderr := childRun(t, nil, args...)
			if code != 0 {
				t.Fatalf("%s exited %d: %s", name, code, stderr)
			}
			for _, d := range dirs {
				empty(t, d)
			}
			// The private configuration must still carry fonts: with the configuration merely disabled, stamped text,
			// page numbers and a Markdown conversion all fell back to unembedded Base-14 faces and said so here.
			if strings.Contains(stderr, "embedded faces are unavailable") || strings.Contains(stderr, "stamping in the embedded faces failed") {
				t.Errorf("%s lost its embedded fonts: %s", name, stderr)
			}
		})
	}
	// The stimulus: a configuration built before any verb closes it DOES write the directory, so
	// the emptiness above is the oracle seeing writes and not an unreachable HOME.
	t.Run("stimulus", func(t *testing.T) {
		dirs, code, _, stderr := childRun(t, []string{"NIB_TEST_OPEN_CONFIG=1"}, "fields", in, "--json")
		if code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(dirs[1], "pdfcpu", "config.yml")); err != nil {
			t.Fatalf("the control run wrote no configuration (%v): the oracle is blind here", err)
		}
	})
}

// TestAFillOfNonLatinTextStillGetsItsFont — ADR-183. A value pdfcpu draws in Roboto needs the configuration directory
// Roboto lives in; with the configuration merely disabled a fill of "Иван" failed outright ("font Roboto-Regular not
// available"). The stateless fill upgrades itself to a private root for non-ASCII data, and leaves nothing behind.
func TestAFillOfNonLatinTextStillGetsItsFont(t *testing.T) {
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	in, data, out := filepath.Join(work, "form.pdf"), filepath.Join(work, "a.json"), filepath.Join(work, "f.pdf")
	os.WriteFile(in, form, 0o600)
	os.WriteFile(data, []byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Иван Петров"}]}]}`), 0o600)
	dirs, code, _, stderr := childRun(t, nil, "fill", in, "--data", data, "-o", out)
	if code != 0 {
		t.Fatalf("a Cyrillic fill exited %d: %s", code, stderr)
	}
	for _, d := range dirs {
		empty(t, d)
	}
	filled, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if x, _ := pdfops.ExportFormXFDF(filled); !strings.Contains(string(x), "Иван Петров") {
		t.Errorf("the value is not in the filled form:\n%s", x)
	}
}
