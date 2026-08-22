package nib

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestNoticesUpToDate guards against THIRD-PARTY-NOTICES.md drifting from what
// build/gen-notices.sh would produce — the failure that let the pdfcpu version
// (and jsdiff attribution) rot for months. It regenerates the notices to a temp
// file and fails if they differ from the committed file, forcing a regenerate to
// land in the same commit as any dependency or vendored-asset change.
//
// The generator shells out to bash + go, so the test skips cleanly where those
// aren't available (and under -short) rather than failing spuriously; in Nib's
// normal `go test ./...` it runs.
func TestNoticesUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping notices freshness guard under -short")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available; skipping notices freshness guard")
	}
	// The generator runs `go list -deps ./cmd/nib`, whose module set is GOOS- and
	// build-tag-dependent, and the comparison below is byte-for-byte. The committed
	// file is generated on Linux, so on any other platform this fails on a file the
	// contributor did not touch and cannot fix — a spurious red that teaches people to
	// ignore the guard. Gated rather than made platform-agnostic: the file has to be
	// generated on SOME platform, and pinning that platform is the honest version of
	// "the committed artifact is the Linux one".
	if runtime.GOOS != "linux" {
		t.Skipf("THIRD-PARTY-NOTICES.md is generated on linux and `go list -deps` varies by GOOS; skipping on %s", runtime.GOOS)
	}

	got := filepath.Join(t.TempDir(), "notices.md")
	cmd := exec.Command("bash", "build/gen-notices.sh", got)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("gen-notices.sh failed: %v\n%s", err, stderr.String())
	}

	generated, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("THIRD-PARTY-NOTICES.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, committed) {
		t.Errorf("THIRD-PARTY-NOTICES.md is out of date (differs at %s) — run `make notices` and commit the result",
			firstDiff(committed, generated))
	}
}

// firstDiff returns a human-readable location of the first line that differs
// between the committed and generated notices, to point the fix at the change.
func firstDiff(a, b []byte) string {
	la, lb := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	for i := 0; i < len(la) && i < len(lb); i++ {
		if !bytes.Equal(la[i], lb[i]) {
			return "line " + strconv.Itoa(i+1)
		}
	}
	if len(la) != len(lb) {
		return "line " + strconv.Itoa(min(len(la), len(lb))+1) + " (length differs)"
	}
	return "an unknown line"
}

// TestEveryVendoredThingIsInTheNotices.
//
// `THIRD-PARTY-NOTICES.md` has two halves and only one of them is reconciled. The Go half
// is machine-enumerated (`go list -deps ./cmd/nib`) and cannot drift. The vendored half —
// four directories under `web/vendor/` and the faces under `internal/pdfops/fonts/` — is
// **hand-authored**, and `TestNoticesUpToDate` cannot see a gap in it: it compares the
// generator's output against the committed file, and both derive from the same hand-written
// list. A fifth vendored library, or a font under a different licence, would be silently
// absent from a licence file on an AGPLv3 project that distributes third-party code.
//
// This is the missing external reference: the DIRECTORIES, which are what actually ships.
func TestEveryVendoredThingIsInTheNotices(t *testing.T) {
	notices, err := os.ReadFile("THIRD-PARTY-NOTICES.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(notices)

	vendored, err := os.ReadDir("web/vendor")
	if err != nil {
		t.Fatal(err)
	}
	var dirs int
	for _, e := range vendored {
		if !e.IsDir() {
			continue
		}
		dirs++
		if !strings.Contains(body, "web/vendor/"+e.Name()) {
			t.Errorf("web/vendor/%s ships in the binary and THIRD-PARTY-NOTICES.md does "+
				"not name it — build/gen-notices.sh's vendored half is hand-authored and "+
				"reconciled against nothing, so the generator and the committed file agree "+
				"with each other while both omit it", e.Name())
		}
	}
	// The floor. Zero directories means this walked the wrong path and asserted nothing.
	if dirs < 4 {
		t.Fatalf("found %d vendored web directories; expected at least 4 — the scan is "+
			"not reading web/vendor/", dirs)
	}

	// The fonts, by family name. Each file is <Family>-<Style>.ttf; the notices credit
	// families, which is the right granularity for an OFL/Apache attribution.
	fonts, err := os.ReadDir("internal/pdfops/fonts")
	if err != nil {
		t.Fatal(err)
	}
	// The Noto section: its heading to the next top-level heading. Isolated BEFORE the loop
	// so a document that has lost the section entirely fails once and loudly, rather than
	// once per face with a sentence about that face.
	notoSection := body
	if i := strings.Index(body, "## Noto Sans"); i >= 0 {
		notoSection = body[i:]
		if j := strings.Index(notoSection[3:], "\n## "); j >= 0 {
			notoSection = notoSection[:j+3]
		}
	} else {
		t.Fatal("THIRD-PARTY-NOTICES.md has no `## Noto Sans` heading — every Noto face " +
			"below would be checked against an empty section, or against the whole file, " +
			"where the Tesseract language list already names each script")
	}

	var faces int
	for _, e := range fonts {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ttf") {
			continue
		}
		faces++
		family := strings.TrimSuffix(e.Name(), ".ttf")
		if i := strings.Index(family, "-"); i > 0 {
			family = family[:i]
		}
		// The SCRIPT token, not the full family phrase.
		//
		// The Noto faces are credited under one combined heading — "Noto Sans Thai,
		// Devanagari, Bengali, Tamil, …" — because they share one OFL grant, which is the
		// right granularity for the attribution. A check for the literal "Noto Sans
		// Bengali" fails against a file that does credit it, which is this guard crying
		// wolf rather than a gap. What must not be missable is a face whose SCRIPT nobody
		// mentioned, since that is the one likely to arrive under a different licence.
		//
		// **Scoped to the Noto section, and non-empty.** Two holes, both of which made this
		// check pass over a face nobody credited:
		//
		//   - `strings.Contains(body, "")` is ALWAYS true, so a file named exactly
		//     `NotoSans-Regular.ttf` produced an empty token and was waved through.
		//   - the Tesseract entry lists its trained languages by name — "the Arabic,
		//     Belarusian, Bengali, Bulgarian, …" — so every script token this loop looks
		//     for already appears in the file for an unrelated reason. Dropping a script
		//     from the Noto heading would leave the search satisfied by an OCR model's
		//     language list, which is a different grant from a different project.
		//
		// The Noto faces are credited under one combined heading sharing one OFL grant,
		// which is the right granularity for the attribution — so the section is the unit
		// to search, not the whole document and not a per-face phrase.
		haystack, what := body, "THIRD-PARTY-NOTICES.md"
		token := family
		if rest, isNoto := strings.CutPrefix(family, "NotoSans"); isNoto {
			token = rest
			haystack, what = notoSection, "the Noto section of THIRD-PARTY-NOTICES.md"
		}
		if token == "" {
			t.Errorf("internal/pdfops/fonts/%s yields an empty script token, and "+
				"strings.Contains(body, \"\") is always true — this face would be credited "+
				"by nothing and pass", e.Name())
			continue
		}
		spaced := regexp.MustCompile(`([a-z])([A-Z])`).ReplaceAllString(token, "$1 $2")
		if !strings.Contains(haystack, token) && !strings.Contains(haystack, spaced) {
			_ = what
			t.Errorf("internal/pdfops/fonts/%s is embedded in the binary and the notices "+
				"mention neither %q nor %q — the vendored half of gen-notices.sh is "+
				"hand-authored and reconciled against nothing, so the generator and the "+
				"committed file agree with each other while both omit it",
				e.Name(), token, spaced)
		}
	}
	if faces < 10 {
		t.Fatalf("found %d embedded font face(s); expected at least 10 — the scan is not "+
			"reading internal/pdfops/fonts/", faces)
	}
	t.Logf("%d vendored dir(s), %d font face(s), all credited", dirs, faces)
}

// TestTheNoticesPreambleNamesNoLicenseClass — the preamble may not summarise the set by
// naming license classes, because that summary cannot stay true.
//
// It said "Each is under a permissive license (BSD, MIT, or Apache-2.0)" and that was
// false of the set it introduced: `github.com/anacrolix/dht/v2` is MPL-2.0 (compatible
// through MPL 2.0 §3.3, not through being permissive), and the vendored web assets add
// SIL OFL and ISC. The file's own note said as much — **3,100 lines below the preamble**,
// in a section a reader taking the preamble as the summary has no reason to reach. This is
// a distribution-facing legal document, so a wrong summary at the top is the part that
// gets relied on.
//
// The guard is on the CONSTRUCTION, not on the specific stale names. Replacing three
// license names with five would go stale the same way on the next dependency — P05's
// caveat 6 leaves the port-mapping library's license open, so the next one may well not be
// permissive either. Nib's own license is exempt: naming AGPLv3 in the first line is the
// document saying what Nib is, which is a fact about this repo and not a claim about the
// components.
//
// It reads the COMMITTED file rather than the generator, so a hand-edit is caught too —
// even though the generator's header forbids one and TestNoticesUpToDate would fail it,
// because a guard that checks the door and not the artifact is a guard that assumes the
// other guard ran.
func TestTheNoticesPreambleNamesNoLicenseClass(t *testing.T) {
	b, err := os.ReadFile("THIRD-PARTY-NOTICES.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	i := strings.Index(body, "\n---\n")
	if i < 0 {
		t.Fatal("setup: THIRD-PARTY-NOTICES.md has no `---` rule, so the preamble cannot " +
			"be delimited and this guard would scan the whole file — including 3,400 lines " +
			"of license text, which names every class there is")
	}
	preamble := body[:i]
	// SETUP: the delimiter found the preamble and not, say, byte 0 — otherwise an empty
	// string trivially contains no license name and this test passes on nothing.
	if !strings.Contains(preamble, "incorporates the third-party software listed below") {
		t.Fatalf("setup: the text before the first `---` does not look like the preamble:\n%s", preamble)
	}
	for _, name := range []string{
		"BSD", "MIT", "Apache", "MPL", "Mozilla Public License",
		"ISC", "OFL", "SIL", "Unlicense", "zlib",
	} {
		if strings.Contains(preamble, name) {
			t.Errorf("the preamble names the license class %q. An enumeration here is a claim "+
				"about the WHOLE set that goes stale silently as dependencies change — it "+
				"already did, and the correction sat 3,100 lines below it. Say that each "+
				"component's license is AGPLv3-compatible and that its text follows; let the "+
				"per-component sections and the end-of-file note carry the specifics.\n\n%s",
				name, preamble)
		}
	}
}
