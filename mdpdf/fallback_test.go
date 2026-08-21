package mdpdf

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/font"
)

// TestAFontNameCannotEscapeTheFontDirectory.
//
// mdpdf's own doc says it is a root package other projects import, so `Font.Name` is API
// surface rather than an internal constant. It is joined into a path and the result is
// **gob-decoded** — `encoding/gob` over an attacker-chosen file is an allocation hazard at
// minimum. Nib's own caller passes constants, so this closes the cost of being importable
// rather than a live break.
func TestAFontNameCannotEscapeTheFontDirectory(t *testing.T) {
	for _, bad := range []string{
		"../../../../etc/passwd",
		"..",
		"a/b",
		`a\b`,
		"",
		"name with spaces",
		"nul\x00byte",
	} {
		if safeFontName(bad) {
			t.Errorf("safeFontName(%q) = true — this becomes a path component and the "+
				"file it names is gob-decoded", bad)
		}
		// registerMetrics is the door with reach: it OPENS the path and gob-decodes it.
		// installedFallback carries the same guard and is deliberately not asserted here —
		// its os.Stat fails on a traversal path anyway, so removing its check leaves this
		// green, and a test that cannot fail is worse than an absent one. Proven by
		// mutation rather than assumed.
		if err := registerMetrics(bad); err == nil {
			t.Errorf("registerMetrics(%q) did not refuse — this path is gob-decoded", bad)
		}
	}
	// A PLANTED file, because without one every traversal case above fails with ENOENT and
	// the test passes whether the guard exists or not — proven by mutation: removing the
	// check from both doors left the whole test green. The guard is only observable when
	// the path it would have opened actually exists.
	// pdfcpu leaves font.UserFontDir EMPTY until its config is initialised, so a bare
	// `go test` joins the name against "" and the path is relative to the process working
	// directory. That makes the traversal more real, not less — and it also meant the
	// first version of this block was skipped entirely, silently, which is the
	// never-exercised-subject defect this suite exists to catch. Set explicitly.
	{
		dir := t.TempDir()
		saved := font.UserFontDir
		font.UserFontDir = filepath.Join(dir, "fonts")
		defer func() { font.UserFontDir = saved }()
		if err := os.MkdirAll(font.UserFontDir, 0o755); err != nil {
			t.Fatal(err)
		}
		planted := filepath.Join(dir, "nibtest-planted.gob")
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(font.TTFLight{}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(planted, buf.Bytes(), 0o644); err != nil {
			t.Fatalf("could not plant a file next to the font dir: %v", err)
		}

		const name = "../nibtest-planted"
		// STIMULUS: the file really is there and really is decodable, so a refusal below
		// can only come from the name check.
		if _, err := os.Stat(planted); err != nil {
			t.Fatalf("setup: the planted file is not there: %v", err)
		}
		if err := registerMetrics(name); err == nil {
			t.Error("registerMetrics gob-decoded a file OUTSIDE the font directory — the " +
				"name was used as a path component and this is the traversal")
		}
		if installedFallback(name) {
			t.Error("installedFallback found a font outside the font directory")
		}
	}

	// The control. A predicate that refuses everything means no fallback font ever loads,
	// which is a silent loss of every non-Latin script Nib supports.
	for _, ok := range []string{"NotoSansDevanagari-Regular", "Roboto", "DroidSansFallbackFull", "a.b_c-1"} {
		if !safeFontName(ok) {
			t.Errorf("safeFontName(%q) = false — this is a real vendored font name", ok)
		}
	}
}
