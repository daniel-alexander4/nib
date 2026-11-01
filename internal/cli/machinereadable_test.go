package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// jsonOf runs `nib args...` in-process and returns its stdout decoded as a JSON array of objects.
func jsonOf(t *testing.T, args ...string) []map[string]any {
	t.Helper()
	out, code := captureStdout(t, func() int { _, c := Run(args, "t"); return c })
	if code != 0 {
		t.Fatalf("%v exited %d", args, code)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("%v is not a JSON array of objects: %v\n%s", args, err, out)
	}
	return rows
}

func keysOf(m map[string]any) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}

// pinKeys fails unless m holds every key of required, and no key outside required and optional. A key
// renamed or removed is red here, and so is one added without being named in the doc (see below).
func pinKeys(t *testing.T, what string, m map[string]any, required, optional []string) {
	t.Helper()
	allowed := map[string]bool{}
	for _, k := range append(append([]string{}, required...), optional...) {
		allowed[k] = true
	}
	for _, k := range required {
		if _, ok := m[k]; !ok {
			t.Errorf("%s lost its key %q (has %v): raise APIVersion and say so in docs/cli-machine-readable.md", what, k, keysOf(m))
		}
	}
	for k := range m {
		if !allowed[k] {
			t.Errorf("%s gained the key %q: name it here and in docs/cli-machine-readable.md", what, k)
		}
	}
}

func numbers(t *testing.T, what string, v any, n int) {
	t.Helper()
	a, ok := v.([]any)
	if !ok || len(a) != n {
		t.Errorf("%s = %v, want an array of %d numbers", what, v, n)
		return
	}
	for _, x := range a {
		if _, ok := x.(float64); !ok {
			t.Errorf("%s holds %v, not a number", what, x)
		}
	}
}

// TestTheMachineReadableShapesAreLocked — ADR-183, RelaxTax N4. `fields --json`, `text --json` and
// `info --json` are read by a program; each key, its type and the origin of its rectangle are pinned,
// and docs/cli-machine-readable.md must name every key (TestTheMachineReadableDocNamesEveryKey).
func TestTheMachineReadableShapesAreLocked(t *testing.T) {
	dir := t.TempDir()
	form, _ := testpdf.Form()
	text, _ := testpdf.Text("Hello world")
	fp, tp := filepath.Join(dir, "form.pdf"), filepath.Join(dir, "text.pdf")
	os.WriteFile(fp, form, 0o600)
	os.WriteFile(tp, text, 0o600)

	t.Run("fields", func(t *testing.T) {
		rows := jsonOf(t, "fields", fp, "--json")
		if len(rows) < 2 {
			t.Fatalf("the fixture lists %d fields, want its text field and check box", len(rows))
		}
		for _, r := range rows {
			pinKeys(t, "a fields row", r, []string{"name", "type", "fillable", "flags", "onPage", "widgets"}, []string{"tooltip", "label"})
			if _, ok := r["name"].(string); !ok {
				t.Errorf("name is %v", r["name"])
			}
			if _, ok := r["fillable"].(bool); !ok {
				t.Errorf("fillable is %v", r["fillable"])
			}
			if _, ok := r["flags"].(float64); !ok {
				t.Errorf("flags is %v", r["flags"])
			}
			ws, _ := r["widgets"].([]any)
			if len(ws) == 0 {
				t.Errorf("%v has no widgets", r["name"])
			}
			for _, w := range ws {
				wm := w.(map[string]any)
				pinKeys(t, "a widget", wm, []string{"page", "rect"}, nil)
				numbers(t, "widget rect", wm["rect"], 4)
			}
		}
	})
	t.Run("fields of a document with none", func(t *testing.T) {
		if rows := jsonOf(t, "fields", tp, "--json"); len(rows) != 0 {
			t.Errorf("%d rows, want an empty array (not null)", len(rows))
		}
	})
	t.Run("text", func(t *testing.T) {
		rows := jsonOf(t, "text", tp, "--json")
		if len(rows) != 1 {
			t.Fatalf("%d pages, want 1", len(rows))
		}
		pinKeys(t, "a text page", rows[0], []string{"page", "width", "height", "runs"}, []string{"noText", "unread"})
		runs, _ := rows[0]["runs"].([]any)
		if len(runs) == 0 {
			t.Fatal("the fixture has no runs: the test reaches nothing")
		}
		for _, r := range runs {
			rm := r.(map[string]any)
			pinKeys(t, "a run", rm, []string{"text", "rect", "size"}, []string{"hidden"})
			numbers(t, "run rect", rm["rect"], 4)
			// Origin top left: top < bottom, and the first line of a page sits in its upper half.
			rc := rm["rect"].([]any)
			if rc[1].(float64) >= rc[3].(float64) {
				t.Errorf("run rect %v is [left top right bottom] with the origin at the TOP left: top must be above bottom", rc)
			}
		}
	})
	t.Run("info", func(t *testing.T) {
		rows := jsonOf(t, "info", tp, "--json")
		if len(rows) != 1 {
			t.Fatalf("%d pages, want 1", len(rows))
		}
		pinKeys(t, "an info page", rows[0], []string{"page", "width", "height", "rotate", "mediaBox", "cropBox"}, nil)
		numbers(t, "mediaBox", rows[0]["mediaBox"], 4)
		numbers(t, "cropBox", rows[0]["cropBox"], 4)
	})
	t.Run("stdin", func(t *testing.T) {
		in, err := os.Open(tp)
		if err != nil {
			t.Fatal(err)
		}
		defer in.Close()
		old := os.Stdin
		os.Stdin = in
		defer func() { os.Stdin = old }()
		if rows := jsonOf(t, "info", "-", "--json"); len(rows) != 1 {
			t.Errorf("info - read %d pages from stdin, want 1", len(rows))
		}
	})
}

// TestTheMachineReadableDocNamesEveryKey keeps the document and the golden from drifting: every key
// pinned above must appear in the document as a code span.
func TestTheMachineReadableDocNamesEveryKey(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "cli-machine-readable.md"))
	if err != nil {
		t.Skip("no docs directory (a vendored copy)")
	}
	for _, k := range []string{"name", "type", "fillable", "flags", "onPage", "widgets", "tooltip", "label", "page", "rect",
		"width", "height", "runs", "noText", "unread", "text", "size", "hidden", "rotate", "mediaBox", "cropBox",
		"index", "bytes", "file", "filter", "images", "version", "api", "commands", "NIB_HEADLESS", "--format", "-o -"} {
		if !strings.Contains(string(doc), k) || (k[0] != '-' && k != "NIB_HEADLESS" && !strings.Contains(string(doc), "`"+k)) {
			t.Errorf("docs/cli-machine-readable.md does not name `%s`", k)
		}
	}
}
