package pdfops

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The door's own table. This runs EVERYWHERE — including on a machine that has both
// converters installed, which is the point: the wording a user sees when a tool is missing was
// previously assertable only on a machine that happened to lack it, and every existing test
// simply skips there.

func TestMissingToolForClassifiesBothSentinelsAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want MissingTool
		ok   bool
	}{
		{"libreoffice", ErrLibreOfficeMissing, ToolLibreOffice, true},
		{"ghostscript", ErrGhostscriptMissing, ToolGhostscript, true},
		{"wrapped", fmt.Errorf("converting: %w", ErrLibreOfficeMissing), ToolLibreOffice, true},
		{"unsupported type", ErrUnsupportedOffice, ToolNone, false},
		{"some other error", errors.New("disk full"), ToolNone, false},
		{"nil", nil, ToolNone, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := MissingToolFor(tc.err)
			if ok != tc.ok {
				t.Fatalf("MissingToolFor(%v) ok = %v, want %v", tc.err, ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("MissingToolFor(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// The wrapped case above is not decoration: `ConvertOfficeToPDF` returns the sentinel bare
// today, but a caller that ever wraps it must not silently fall through to the generic branch
// and print a raw error where a remedy belongs. `errors.Is` gives that for free — this asserts
// the door actually uses it rather than `==`.
func TestMissingToolForUsesErrorsIsRatherThanEquality(t *testing.T) {
	wrapped := fmt.Errorf("office: %w", ErrGhostscriptMissing)
	if _, ok := MissingToolFor(wrapped); !ok {
		t.Error("a wrapped sentinel was not classified — the door compares with == rather than " +
			"errors.Is, so any caller that adds context loses the remedy")
	}
}

// Every surface builds its sentence from these, so an empty one ships an empty sentence.
func TestEveryToolNamesItselfAndItsVendor(t *testing.T) {
	for _, tool := range []MissingTool{ToolLibreOffice, ToolGhostscript} {
		if tool.Name() == "" {
			t.Errorf("tool %d has no Name(); every surface interpolates it", tool)
		}
		if v := tool.Vendor(); !strings.HasPrefix(v, "https://") {
			t.Errorf("tool %s has Vendor()=%q — the CLI prints this, so it must be a real https URL",
				tool.Name(), v)
		}
		// The claim nib is allowed to make is about the SEARCH. "is not installed" is a claim
		// about the user's machine that exec.LookPath plus a candidate list cannot support: a
		// copy installed somewhere neither knows about is invisible to both.
		nf := tool.NotFound()
		if !strings.Contains(nf, "could not find") {
			t.Errorf("NotFound() for %s is %q — it must state what nib DID (looked and did not "+
				"find), not assert that the tool is absent from the machine", tool.Name(), nf)
		}
		if strings.Contains(nf, "not installed") {
			t.Errorf("NotFound() for %s says %q; ADR-040 turns on exactly this wording, because "+
				"the stock macOS and Windows installs are off PATH and nib reported them absent",
				tool.Name(), nf)
		}
	}
	if ToolNone.Name() != "" || ToolNone.Vendor() != "" || ToolNone.NotFound() != "" {
		t.Error("ToolNone describes a tool; it is the not-a-missing-tool case and must stay empty")
	}
}
