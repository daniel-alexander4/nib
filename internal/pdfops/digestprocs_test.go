package pdfops

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// F09 (/pending 920): S08's pin (2) asked for the moved files digested in separate processes with identical output.
// A digest that depends on something fixed once per process — the order a Go map happens to be ranged in the first
// time, a registry filled in init order, pdfcpu's duplicate-font merge taking the first of two equal candidates from a
// map-built list — agrees with itself inside one run and with its golden only by luck of the seed, and a ceremony
// is signed by one process and checked by another. The parent starts digestProcessCount children of this test
// binary; each prints a digest per document; all of them must print the same.

const (
	digestChildEnv     = "NIB_DIGEST_CHILD"
	digestProcessCount = 8
)

// duplicateFontDocuments are the shapes where pdfcpu's optimize pass chooses among equal fonts: the same font
// written once per page, a subset tag shared by fonts whose programs differ, and both at once.
func duplicateFontDocuments() []struct {
	Name string
	PDF  []byte
} {
	page := func(n, content, font int) string {
		return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>", content, font)
	}
	stream := func(body string) string {
		return fmt.Sprintf("<< /Length %d /Length1 %d >>\nstream\n%s\nendstream", len(body), len(body), body)
	}
	const text = "BT /F1 12 Tf 20 100 Td (hello) Tj ET"
	build := func(pages int, fontDict func(i int) string, extra func(i int) map[int]string) []byte {
		objs := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>"}
		var kids []string
		for i := 0; i < pages; i++ {
			pg, ct, ft := 10+i*10, 11+i*10, 12+i*10
			kids = append(kids, fmt.Sprintf("%d 0 R", pg))
			objs[pg] = page(pg, ct, ft)
			objs[ct] = stream(text)
			objs[ft] = fontDict(i)
			for n, o := range extra(i) {
				objs[n] = o
			}
		}
		objs[2] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pages)
		return assembleFixture(objs)
	}
	none := func(int) map[int]string { return nil }
	subset := func(i int) string {
		return fmt.Sprintf("<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+Dup /FirstChar 32 /LastChar 32 /Widths [250] /FontDescriptor %d 0 R >>", 13+i*10)
	}
	descriptor := func(i int) map[int]string {
		return map[int]string{
			13 + i*10: fmt.Sprintf("<< /Type /FontDescriptor /FontName /ABCDEF+Dup /Flags 32 /FontBBox [0 0 500 500] /ItalicAngle 0 /Ascent 500 /Descent 0 /CapHeight 500 /StemV 80 /FontFile2 %d 0 R >>", 14+i*10),
			14 + i*10: stream(fmt.Sprintf("program-%d", i%3)), // three programs among six fonts of one subset tag
		}
	}
	return []struct {
		Name string
		PDF  []byte
	}{
		{"the same font on six pages", build(6, func(int) string { return "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>" }, none)},
		{"one subset tag, three programs", build(6, subset, descriptor)},
		{"one subset tag, one program", build(6, subset, func(i int) map[int]string {
			m := descriptor(i)
			m[14+i*10] = stream("program-0")
			return m
		})},
	}
}

// processDocuments are the documents every process digests: the generated corpus (some of whose generators are
// not byte-stable, so each process must be handed the SAME bytes, not asked to build its own) and the duplicate-font
// shapes.
func processDocuments(t *testing.T) []struct {
	Name string
	PDF  []byte
} {
	t.Helper()
	return append(generatedDigestCorpus(t), duplicateFontDocuments()...)
}

// digestRows is one line per file in dir, digested under the current rule. A document that is refused is printed as
// its refusal — a refusal must agree too.
func digestRows(t *testing.T, dir string) []string {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, f.Name()+"\t"+digestOrError(b, 7))
	}
	sort.Strings(rows)
	return rows
}

// TestDigestProcessChild is the other half of the test below: it prints, and does nothing unless a parent asked.
func TestDigestProcessChild(t *testing.T) {
	dir := os.Getenv(digestChildEnv)
	if dir == "" {
		t.Skip("run by TestADigestIsTheSameInEverySeparateProcess")
	}
	for _, r := range digestRows(t, dir) {
		fmt.Println("DIGESTROW\t" + r)
	}
}

func TestADigestIsTheSameInEverySeparateProcess(t *testing.T) {
	if os.Getenv(digestChildEnv) != "" {
		t.Skip("a child does not start children")
	}
	dir := t.TempDir()
	dupes := map[string]bool{}
	for _, d := range duplicateFontDocuments() {
		dupes[d.Name] = true
	}
	docs := processDocuments(t)
	for i, d := range docs {
		name := fmt.Sprintf("%02d-%s.pdf", i, strings.NewReplacer(" ", "-", ",", "").Replace(d.Name))
		if err := os.WriteFile(filepath.Join(dir, name), d.PDF, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want := digestRows(t, dir)
	if len(want) < 15 {
		t.Fatalf("stimulus: only %d documents are digested — the comparison reads almost nothing", len(want))
	}
	for i, d := range docs {
		if dupes[d.Name] {
			if r := want[indexOfRow(want, fmt.Sprintf("%02d-", i))]; strings.Contains(r, "ERROR:") {
				t.Fatalf("stimulus: a duplicate-font fixture is refused (%s), so it exercises nothing", r)
			}
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < digestProcessCount; i++ {
		cmd := exec.Command(exe, "-test.run=^TestDigestProcessChild$", "-test.v")
		cmd.Env = append(os.Environ(), digestChildEnv+"="+dir)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("process %d: %v\n%s%s", i, err, out.String(), errb.String())
		}
		var got []string
		for _, line := range strings.Split(out.String(), "\n") {
			if r, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "DIGESTROW\t"); ok {
				got = append(got, r)
			}
		}
		sort.Strings(got)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("process %d digests differently from this one:\n%s", i, firstDifference(want, got))
		}
	}
}

// indexOfRow is the position of the row beginning with prefix.
func indexOfRow(rows []string, prefix string) int {
	for i, r := range rows {
		if strings.HasPrefix(r, prefix) {
			return i
		}
	}
	return 0
}

// firstDifference names the first row two sorted lists disagree on.
func firstDifference(want, got []string) string {
	for i := 0; i < len(want) || i < len(got); i++ {
		var w, g string
		if i < len(want) {
			w = want[i]
		}
		if i < len(got) {
			g = got[i]
		}
		if w != g {
			return fmt.Sprintf("row %d: this process %q, the other %q", i, w, g)
		}
	}
	return "no difference"
}
