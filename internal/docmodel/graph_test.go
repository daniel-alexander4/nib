package docmodel

import (
	"os/exec"
	"strings"
	"testing"
)

// The writers cannot reach a PDF (PLAN-import-export D16, R8): the model and every `internal/docwrite/*` package may
// not import, even transitively, a package that reads one. The census is over the transitive closure (`go list -deps`),
// so a helper that re-exports a reader is caught too.
var pdfReaders = []string{"nib/internal/pdfops", "nib/internal/pdfread", "nib/internal/uacheck"}

// deps is every package pkg imports, directly or not.
func deps(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	return strings.Fields(string(out))
}

// reaching is the readers in pkg's closure.
func reaching(t *testing.T, pkg string) []string {
	var hit []string
	for _, d := range deps(t, pkg) {
		for _, r := range pdfReaders {
			if d == r {
				hit = append(hit, d)
			}
		}
	}
	return hit
}

func TestNoWriterReachesAPDFReader(t *testing.T) {
	// The positive control: the READER reaches pdfops, so the census can see an edge when there is one. Without it an
	// empty result proves nothing (the writers do not exist yet).
	if got := reaching(t, "nib/internal/docmodel/read"); len(got) == 0 {
		t.Fatal("the census cannot see docmodel/read import pdfops: it would pass anything")
	}
	out, err := exec.Command("go", "list", "nib/internal/docmodel", "nib/internal/docwrite/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	pkgs := strings.Fields(string(out))
	if len(pkgs) == 0 {
		t.Fatal("no package to census")
	}
	for _, p := range pkgs {
		if got := reaching(t, p); len(got) > 0 {
			t.Errorf("%s reaches a PDF reader: %v", p, got)
		}
	}
}
