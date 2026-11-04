package pptxexport

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func list(t *testing.T, args ...string) []string {
	t.Helper()
	out, err := exec.Command("go", append([]string{"list"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("go list %v: %v\n%s", args, err, out)
	}
	return strings.Fields(string(out))
}

// This package is the one docwrite-shaped writer that reads a PDF (Y3): it may reach the export reader and nothing that
// signs, hands off or serves. The positive control is that it does reach pdfops, so an empty result below means something.
func TestThePowerPointWriterReachesTheExportReaderAndNothingThatSignsOrServes(t *testing.T) {
	deps := list(t, "-deps", "nib/internal/pptxexport")
	if !slices.Contains(deps, "nib/internal/pdfops") {
		t.Fatal("the census cannot see this package import pdfops: it would pass anything")
	}
	for _, bad := range []string{"nib/internal/sign", "nib/internal/ceremony", "nib/internal/server", "nib/internal/p2p", "nib/internal/vault"} {
		if slices.Contains(deps, bad) {
			t.Errorf("pptxexport reaches %s", bad)
		}
	}
}

// Nothing but the server route imports it, and no docwrite package does (they take the flowing model only, R8).
func TestOnlyTheExportRouteImportsThePowerPointWriter(t *testing.T) {
	n := 0
	raw, err := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \" \"}}", "nib/...").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		pkg, imps, _ := strings.Cut(l, "|")
		if slices.Contains(strings.Fields(imps), "nib/internal/pptxexport") {
			n++
			if pkg != "nib/internal/server" {
				t.Errorf("%s imports pptxexport; only the server route may", pkg)
			}
		}
	}
	if n != 1 {
		t.Errorf("importers = %d, want exactly 1 (the route): the census would pass an unrouted writer", n)
	}
}
