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

// TestTheLibraryPatchIsReadGoAndLexGoOnlyAsItsNoticeSays holds ADR-066's maintenance rule, as ADR-068
// widened it. nib carries a modified third-party module — `github.com/digitorus/pdf` v0.1.2, whose
// object-stream lookup `third_party/digitorus-pdf/read.go` makes linear (/pending 758) and whose
// `lex.go` readArray stops on an object's end (/pending 761) — and its NOTICE.nib says exactly what
// differs from upstream. (A second one, the signature library, has the guard below.) So:
//
//   - go.mod still REQUIRES v0.1.2 and REPLACES it with that directory: a bump of the require line
//     leaves the patch applied to a version it was not written against, and this fails until the
//     patch is re-applied to the new version and this pin moved with it;
//   - the copy differs from the module-cache original in read.go and lex.go and nothing else, and adds
//     nothing but NOTICE.nib — a third edit would be a change the notice does not declare;
//   - read.go does differ, and carries the cache, and lex.go carries the readArray refusal — a copy
//     quietly reverted to upstream would pass the two checks above and reinstate the quadratic, or the
//     endless append.
//
// The comparison needs v0.1.2 in the module cache; without it that half skips, loudly.
func TestTheLibraryPatchIsReadGoAndLexGoOnlyAsItsNoticeSays(t *testing.T) {
	const (
		mod  = "github.com/digitorus/pdf"
		ver  = "v0.1.2"
		dir  = "third_party/digitorus-pdf"
		file = "read.go"
		lex  = "lex.go"
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
	lexed, err := os.ReadFile(filepath.Join(dir, lex))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(lexed, []byte(`b.errorf("unexpected keyword %q parsing array", tok)`)) {
		t.Errorf("%s/%s's readArray does not refuse an object's end — the copy has been reverted to upstream", dir, lex)
	}
	// ADR-170: AES is decrypted, and a signature's /Contents is not.
	if !bytes.Contains(patched, []byte("func hashV5(")) || !bytes.Contains(patched, []byte("func signatureContents(")) {
		t.Errorf("%s/%s does not decrypt AES-256 or does not know a signature's /Contents — ADR-170's change has been reverted", dir, file)
	}
	if !bytes.Contains(lexed, []byte("!signatureContents(x)")) {
		t.Errorf("%s/%s's readDict decrypts a signature's /Contents — ADR-170's exemption has been reverted", dir, lex)
	}

	differ, missing, added := upstreamDiff(t, mod, ver, dir)
	// sorted, so lex.go precedes read.go
	if len(differ) != 2 || differ[0] != lex || differ[1] != file {
		t.Errorf("files differing from upstream %s@%s: %v, want exactly [%s %s]", mod, ver, differ, lex, file)
	}
	if len(missing) != 0 {
		t.Errorf("upstream files missing from %s: %v", dir, missing)
	}
	if len(added) != 1 || added[0] != "NOTICE.nib" {
		t.Errorf("files added to %s: %v, want exactly [NOTICE.nib]", dir, added)
	}
}

// upstreamDiff compares a patched copy under dir with mod@ver in the module cache and names the files
// that differ, that upstream has and the copy lacks, and that the copy adds — each sorted. Without that
// version in the module cache it skips, loudly.
func upstreamDiff(t *testing.T, mod, ver, dir string) ([]string, []string, []string) {
	t.Helper()
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
	return differ, missing, added
}

// TestTheSignatureLibraryPatchIsItsVerifyPathOnlyAsItsNoticeSays holds the same rule for nib's SECOND
// modified module (ADR-142, /pending 735): `github.com/digitorus/pdfsign`, whose `processByteRange`
// `third_party/digitorus-pdfsign/verify/signature.go` makes read a signature's ranges into one buffer,
// with `verify/verify.go` passing it the file's size. The four checks are the first guard's: the require
// pin, the replace, the patch still being there, and nothing else differing from upstream.
func TestTheSignatureLibraryPatchIsItsVerifyPathOnlyAsItsNoticeSays(t *testing.T) {
	const (
		mod = "github.com/digitorus/pdfsign"
		ver = "v0.0.0-20260407063256-85ede6424a74"
		dir = "third_party/digitorus-pdfsign"
		sig = "verify/signature.go"
		vfy = "verify/verify.go"
	)
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(mod+" "+ver) + `\s*(//.*)?$`).Match(gomod) {
		t.Errorf("go.mod no longer requires %s %s — re-apply %s's patch to the new version (NOTICE.nib, ADR-142), then move this pin", mod, ver, dir)
	}
	if !regexp.MustCompile(`(?m)^replace\s+` + regexp.QuoteMeta(mod) + `\s+=>\s+\./` + regexp.QuoteMeta(dir) + `\s*$`).Match(gomod) {
		t.Errorf("go.mod does not replace %s with ./%s — the patched library is not the one linked", mod, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "NOTICE.nib")); err != nil {
		t.Errorf("%s/NOTICE.nib: %v — the modified copy must say what was modified", dir, err)
	}
	patched, err := os.ReadFile(filepath.Join(dir, sig))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(patched, []byte("io.ReadAll(")) || !bytes.Contains(patched, []byte("func processByteRange(v pdf.Value, file io.ReaderAt, size int64, p7 *pkcs7.PKCS7) error {")) {
		t.Errorf("%s/%s reads a byte range with io.ReadAll, or takes no size — the copy has been reverted to upstream", dir, sig)
	}
	caller, err := os.ReadFile(filepath.Join(dir, vfy))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(caller, []byte("processSignature(v, file, size, options)")) {
		t.Errorf("%s/%s does not pass the file's size to processSignature", dir, vfy)
	}
	differ, missing, added := upstreamDiff(t, mod, ver, dir)
	if len(differ) != 2 || differ[0] != sig || differ[1] != vfy {
		t.Errorf("files differing from upstream %s@%s: %v, want exactly [%s %s]", mod, ver, differ, sig, vfy)
	}
	if len(missing) != 0 {
		t.Errorf("upstream files missing from %s: %v", dir, missing)
	}
	if len(added) != 1 || added[0] != "NOTICE.nib" {
		t.Errorf("files added to %s: %v, want exactly [NOTICE.nib]", dir, added)
	}
}
