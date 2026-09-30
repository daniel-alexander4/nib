package nib

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestTheLibraryPatchIsReadGoOnlyAsItsNoticeSays holds ADR-066's maintenance rule. nib carries ONE
// modified third-party module — `github.com/digitorus/pdf` v0.1.2, whose object-stream lookup
// `third_party/digitorus-pdf/read.go` makes linear (/pending 758) — and its NOTICE.nib says exactly
// what differs from upstream. So:
//
//   - go.mod still REQUIRES v0.1.2 and REPLACES it with that directory: a bump of the require line
//     leaves the patch applied to a version it was not written against, and this fails until the
//     patch is re-applied to the new version and this pin moved with it;
//   - the copy differs from the module-cache original in read.go and nothing else, and adds nothing
//     but NOTICE.nib — a second edit would be a change the notice does not declare;
//   - read.go does differ, and carries the cache — a copy quietly reverted to upstream would pass the
//     two checks above and reinstate the quadratic.
//
// The comparison needs v0.1.2 in the module cache; without it that half skips, loudly.
func TestTheLibraryPatchIsReadGoOnlyAsItsNoticeSays(t *testing.T) {
	const (
		mod  = "github.com/digitorus/pdf"
		ver  = "v0.1.2"
		dir  = "third_party/digitorus-pdf"
		file = "read.go"
	)
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(mod+" "+ver) + `\s*(//.*)?$`).Match(gomod) {
		t.Errorf("go.mod no longer requires %s %s — re-apply %s/%s's patch to the new version (NOTICE.nib, ADR-066), then move this pin", mod, ver, dir, file)
	}
	if !regexp.MustCompile(`(?m)^replace\s+` + regexp.QuoteMeta(mod) + `\s+=>\s+\./` + regexp.QuoteMeta(dir) + `\s*$`).Match(gomod) {
		t.Errorf("go.mod does not replace %s with ./%s — the patched reader is not the one linked", mod, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "NOTICE.nib")); err != nil {
		t.Errorf("%s/NOTICE.nib: %v — the modified copy must say what was modified", dir, err)
	}
	patched, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(patched, []byte("type objStmCache struct")) || !bytes.Contains(patched, []byte("c := r.objStm(strm)")) {
		t.Errorf("%s/%s carries no object-stream cache — the copy has been reverted to upstream", dir, file)
	}

	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	cache := strings.TrimSpace(string(out))
	if err != nil || cache == "" {
		t.Skipf("SKIP (not a pass) for the upstream comparison: go env GOMODCACHE: %v", err)
	}
	orig := filepath.Join(cache, mod+"@"+ver)
	if _, err := os.Stat(filepath.Join(orig, "go.mod")); err != nil {
		t.Skipf("SKIP (not a pass) for the upstream comparison: %s@%s is not in the module cache (%v)", mod, ver, err)
	}
	files := func(root string) map[string][]byte {
		m := map[string][]byte{}
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				t.Fatal(err)
			}
			if d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			m[filepath.ToSlash(rel)] = b
			return nil
		})
		return m
	}
	up, ours := files(orig), files(dir)
	var differ, missing, added []string
	for name, b := range up {
		o, ok := ours[name]
		switch {
		case !ok:
			missing = append(missing, name)
		case !bytes.Equal(o, b):
			differ = append(differ, name)
		}
	}
	for name := range ours {
		if _, ok := up[name]; !ok {
			added = append(added, name)
		}
	}
	sort.Strings(differ)
	sort.Strings(missing)
	sort.Strings(added)
	if len(differ) != 1 || differ[0] != file {
		t.Errorf("files differing from upstream %s@%s: %v, want exactly [%s]", mod, ver, differ, file)
	}
	if len(missing) != 0 {
		t.Errorf("upstream files missing from %s: %v", dir, missing)
	}
	if len(added) != 1 || added[0] != "NOTICE.nib" {
		t.Errorf("files added to %s: %v, want exactly [NOTICE.nib]", dir, added)
	}
}
