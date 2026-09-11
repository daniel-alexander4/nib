package nib

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestTheScrollModeStubMatchesTheVendoredEnum.
//
// # What this closes
//
// `web/app.js` sets `viewer.scrollMode = ScrollMode.PAGE` to make presentation mode show one page
// at a time (v1.129.6). Tier 2 stubs the vendored viewer — that IS the tier boundary, since
// rendering is where jsdom stops — so the stub carries its own copy of the enum.
//
// **A stub whose constants drift from the thing it stands for is a harness that agrees with itself
// and with nothing else.** Every tier-2 assertion about presentation would keep passing while the
// real viewer received a number that means something different, and the first sign of it would be
// a user pressing Present and getting a horizontal scroll.
//
// Cheap by construction: two small tables, compared name by name in both directions, with the
// vendored file as the oracle.
func TestTheScrollModeStubMatchesTheVendoredEnum(t *testing.T) {
	vendored := readEnum(t, "web/vendor/pdfjs/pdf_viewer.mjs", `const ScrollMode = \{([^}]*)\}`)
	stub := readEnum(t, "test/jsdom/stub-viewer.mjs", `export const ScrollMode = \{([^}]*)\}`)

	// A floor, because two empty maps are equal and this whole test would then be vacuous.
	if len(vendored) < 4 {
		t.Fatalf("only %d ScrollMode members read from the vendored viewer; the matcher has "+
			"stopped finding the enum, so nothing below is comparing anything", len(vendored))
	}
	for name, want := range vendored {
		got, ok := stub[name]
		if !ok {
			t.Errorf("the tier-2 stub has no ScrollMode.%s, which the vendored viewer defines as "+
				"%s — a test that uses it would read undefined and set the viewer's mode to nothing",
				name, want)
			continue
		}
		if got != want {
			t.Errorf("ScrollMode.%s is %s in the stub and %s in the vendored viewer. Tier 2 would "+
				"keep passing while the real viewer got a number meaning a different layout",
				name, got, want)
		}
	}
	for name := range stub {
		if _, ok := vendored[name]; !ok {
			t.Errorf("the stub defines ScrollMode.%s and the vendored viewer does not — a test "+
				"could rely on a mode this build cannot enter", name)
		}
	}
}

// readEnum pulls `NAME: value` pairs out of the one object literal a pattern names.
func readEnum(t *testing.T, path, pattern string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(pattern).FindSubmatch(b)
	if m == nil {
		t.Fatalf("%s: no object literal matched %s — the enum was renamed or reformatted, and "+
			"this guard cannot see it any more", path, pattern)
	}
	out := map[string]string{}
	for _, pair := range regexp.MustCompile(`([A-Z_]+)\s*:\s*(-?\d+)`).FindAllStringSubmatch(string(m[1]), -1) {
		out[pair[1]] = strings.TrimSpace(pair[2])
	}
	return out
}
