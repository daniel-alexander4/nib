package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// streamed is one picture read back from `nib images --out-dir -`.
type streamed struct {
	head map[string]any
	data []byte
}

// readImageStream parses the stream of ADR-225 strictly: the first line, then exactly count pictures, then end
// of file. Anything else (a short picture, a missing one, a byte past the last) fails the test.
func readImageStream(t *testing.T, out string) (map[string]any, []streamed) {
	t.Helper()
	r := bufio.NewReader(strings.NewReader(out))
	readLine := func(what string) map[string]any {
		b, err := r.ReadBytes('\n')
		if err != nil {
			t.Fatalf("%s: no whole line (%v): %q", what, err, b)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s is not one JSON object: %v: %q", what, err, b)
		}
		return m
	}
	head := readLine("the stream's first line")
	pinKeys(t, "the stream's first line", head, []string{"stream", "version", "count"}, nil)
	if head["stream"] != "nib-images" || head["version"] != 1.0 {
		t.Fatalf("first line %v, want stream nib-images version 1", head)
	}
	n, _ := head["count"].(float64)
	var got []streamed
	for i := 0; i < int(n); i++ {
		h := readLine("a picture's header")
		pinKeys(t, "a picture's header", h, []string{"page", "index", "width", "height", "filter", "format", "bytes"}, nil)
		size, _ := h["bytes"].(float64)
		data := make([]byte, int(size))
		if _, err := io.ReadFull(r, data); err != nil {
			t.Fatalf("picture %d: %v bytes promised, %v", i+1, size, err)
		}
		got = append(got, streamed{h, data})
	}
	if rest, _ := io.ReadAll(r); len(rest) != 0 {
		t.Fatalf("%d bytes after the last picture, want none", len(rest))
	}
	return head, got
}

// TestImagesOnStdoutIsOneFramedStreamOfTheSamePictures — /pending 983, RelaxTax 78, ADR-225. `--out-dir -` writes
// what `--out-dir DIR` writes, byte for byte and in the same order, as one stream a program can parse strictly,
// and makes no folder.
func TestImagesOnStdoutIsOneFramedStreamOfTheSamePictures(t *testing.T) {
	dir := t.TempDir()
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, patterned()); err != nil {
		t.Fatal(err)
	}
	pdf := pdfOfImage(t, dir, "scan.png", pngBytes.Bytes())
	palette := filepath.Join(dir, "palette.pdf")
	if err := os.WriteFile(palette, onePicture("/Width 4 /Height 2 /BitsPerComponent 8 /ColorSpace [/Indexed /DeviceRGB 1 <000000FFFFFF>] /Filter /ASCIIHexDecode", "0001010000010100>"), 0o600); err != nil {
		t.Fatal(err)
	}
	both := filepath.Join(dir, "both.pdf")
	if _, code := captureStdout(t, func() int { _, c := Run([]string{"merge", pdf, palette, "-o", both}, "t"); return c }); code != 0 {
		t.Fatalf("merge exited %d", code)
	}

	folder := filepath.Join(dir, "folder")
	rows, code := listImages(t, "images", both, "--out-dir", folder, "--json")
	if code != 0 || len(rows) != 2 {
		t.Fatalf("the folder run: exit %d, %d rows; want 0 and two pictures", code, len(rows))
	}
	cwd, _ := os.Getwd()
	out, code := captureStdout(t, func() int { _, c := Run([]string{"images", both, "--out-dir", "-"}, "t"); return c })
	if code != 0 {
		t.Fatalf("--out-dir - exited %d", code)
	}
	if _, err := os.Lstat(filepath.Join(cwd, "-")); err == nil {
		t.Fatal("--out-dir - made a folder named -")
	}
	head, pics := readImageStream(t, out)
	if head["count"] != 2.0 || len(pics) != 2 {
		t.Fatalf("count %v, %d pictures; want 2", head["count"], len(pics))
	}
	for i, p := range pics {
		r := rows[i]
		for _, k := range []string{"page", "index", "width", "height", "filter", "bytes"} {
			if p.head[k] != r[k] {
				t.Errorf("picture %d: %s = %v in the stream, %v in the folder's row", i+1, k, p.head[k], r[k])
			}
		}
		name := r["file"].(string)
		if want := strings.TrimPrefix(filepath.Ext(name), "."); p.head["format"] != want {
			t.Errorf("picture %d: format %v, want %q (the folder wrote %s)", i+1, p.head["format"], want, name)
		}
		b, err := os.ReadFile(filepath.Join(folder, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p.data, b) {
			t.Errorf("picture %d: the stream's %d bytes are not the folder's %d", i+1, len(p.data), len(b))
		}
	}

	t.Run("from stdin", func(t *testing.T) {
		in, err := os.Open(both)
		if err != nil {
			t.Fatal(err)
		}
		defer in.Close()
		old := os.Stdin
		os.Stdin = in
		defer func() { os.Stdin = old }()
		out, code := captureStdout(t, func() int { _, c := Run([]string{"images", "-", "--out-dir", "-"}, "t"); return c })
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		if _, pics := readImageStream(t, out); len(pics) != 2 {
			t.Errorf("%d pictures from stdin, want 2", len(pics))
		}
	})
	t.Run("a document with none is the first line alone", func(t *testing.T) {
		out, code := captureStdout(t, func() int {
			_, c := Run([]string{"images", writePDF(t, dir, "text.pdf", "no pictures"), "--out-dir", "-"}, "t")
			return c
		})
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		if head, pics := readImageStream(t, out); head["count"] != 0.0 || len(pics) != 0 {
			t.Errorf("count %v, %d pictures; want 0", head["count"], len(pics))
		}
	})
	t.Run("a refusal writes nothing to stdout", func(t *testing.T) {
		bad := filepath.Join(dir, "bad.pdf")
		if err := os.WriteFile(bad, []byte("%PDF-1.7\nnot a document"), 0o600); err != nil {
			t.Fatal(err)
		}
		var out string
		stderr := captureStderr(t, func() {
			var code int
			out, code = captureStdout(t, func() int { _, c := Run([]string{"images", bad, "--out-dir", "-"}, "t"); return c })
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
		})
		if out != "" || stderr == "" {
			t.Errorf("stdout %q, stderr %q; want nothing on stdout and the cause on stderr", out, stderr)
		}
	})
	t.Run("a terminal as stdout is refused", func(t *testing.T) {
		// A character device is what stdoutIsTerminal reads as a terminal; /dev/null is one, so it stands in.
		dev, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			t.Skip(err)
		}
		defer dev.Close()
		old := os.Stdout
		os.Stdout = dev
		var code int
		stderr := captureStderr(t, func() { _, code = Run([]string{"images", both, "--out-dir", "-"}, "t") })
		os.Stdout = old
		if code != 1 || !strings.Contains(stderr, "terminal") {
			t.Errorf("exit %d, stderr %q; want 1 and a refusal naming the terminal", code, stderr)
		}
	})
	t.Run("--json with --out-dir - is refused", func(t *testing.T) {
		var out string
		stderr := captureStderr(t, func() {
			var code int
			out, code = captureStdout(t, func() int { _, c := Run([]string{"images", both, "--out-dir", "-", "--json"}, "t"); return c })
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
		})
		if out != "" || !strings.Contains(stderr, "--json") {
			t.Errorf("stdout %q, stderr %q; want nothing on stdout and a refusal naming --json", out, stderr)
		}
	})
}

// failingWriter accepts n bytes and then refuses, as a pipe whose reader went away does.
type failingWriter struct{ n int }

func (w *failingWriter) Write(p []byte) (int, error) {
	if len(p) > w.n {
		k := w.n
		w.n = 0
		return k, errors.New("broken pipe")
	}
	w.n -= len(p)
	return len(p), nil
}

// TestAStreamWhoseReaderWentAwayIsAFailure — ADR-225 §2: a write that does not land is an error, never exit 0.
func TestAStreamWhoseReaderWentAwayIsAFailure(t *testing.T) {
	files, err := extractOnePicture(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeImageStream(&failingWriter{n: 10}, files); err == nil {
		t.Fatal("a stream cut off after 10 bytes reported success")
	}
	var whole bytes.Buffer
	if err := writeImageStream(&whole, files); err != nil {
		t.Fatal(err)
	}
	if err := writeImageStream(&failingWriter{n: whole.Len()}, files); err != nil {
		t.Fatalf("a writer that took every byte failed: %v", err)
	}
}
