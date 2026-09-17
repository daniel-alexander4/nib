package pdfops

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// The byte-identity acceptance for `ContentDigest` — /pending 488, ADR-013.
//
// # Why a golden file and not a property
//
// `ContentDigest` is not an internal convenience. Its output is `ceremony.Record.DocHash`, a value
// a convener SIGNS and every later party recomputes, and ADR-013 says in terms what a moved digest
// reads as: *"a record written by the previous build passed the version gate and then failed the
// hash comparison with 'the document does not match the ceremony record… these are not the same
// document', when the cause was a Nib point release."* So the acceptance for any change to this
// function — and it was written for a performance change, where the temptation to reason instead
// of measure is strongest — is not *"the digest is still well-formed"* or *"it still moves when
// the document moves"*. It is **these exact hex strings, over as many real documents as this
// machine can reach**.
//
// A property test cannot do that job. Every property `ContentDigest` has is preserved by a
// coverage change that quietly drops a resource kind: it is still deterministic, still injective
// over the documents the test happens to build, still stable across a rewrite. Only a stored
// output from BEFORE the change can see it.
//
// # The two corpora, and why the external one is the one that matters
//
// `generatedDigestCorpus` is committed-by-construction: every document is built here, from
// `testpdf` and this package's own doors, so a reader can see what changed. It covers the shapes
// Nib itself produces — notes, form fills, attachments, rotation, crops, n-up, watermarks — which
// is exactly the set a ceremony digest meets in production.
//
// It is also, on its own, too small to be evidence. Twenty documents built by the same generator
// share a producer's habits: one page tree shape, one font, one filter, no object streams, no
// inherited attributes, no shared objects between annots. A coverage change can be invisible
// across all twenty. `TestContentDigestIsByteIdenticalAcrossTheExternalCorpus` therefore walks
// **veraPDF's PDF/UA-1 corpus** — 297 files from fifteen producers, already on this machine
// because `/pending 489` made it a standing tier-1 input — and pins a digest per file.
//
// **It skips when the corpus is absent**, because a fresh clone does not have it, and the
// generated half still runs unaided. That is the same contract `CONTRIBUTING.md` gives tiers 2
// and 3.
//
// # Each row pins the INPUT too, where the input has stable bytes
//
// A row is `<name>\t<sha256 of the input>\t<digest>`, TAB-separated because two of the three
// fields routinely contain spaces (`7.18 Annotations/…` is a real corpus path, and a pinned
// refusal reads `ERROR:validateFontEncoding: …`). The first cut split on spaces and silently lost
// the two rows whose digest field is an error — a golden file that drops the rows it cannot parse
// is an acceptance with a hole in it, and it took the corpus reporting two files as "new" to see.
//
// Without the input hash a corpus file edited or replaced underneath the test produces a mismatch
// that reads as a regression in `ContentDigest`, and the reader has nothing to tell the two apart.
//
// **The generated half pins `-` instead, and that is the point of the function under test.**
// `testpdf.Text` goes through pdfcpu's writer, which stamps a fresh `/ID` and a creation date, so
// the same call produces different BYTES every run — which is precisely why `ContentDigest` exists
// rather than a byte hash (see its doc comment: *"pdfcpu's rewrite is not idempotent"*). Pinning
// the generator's output would fail on every run for a reason that has nothing to do with the
// digest.
const digestGoldenUpdateEnv = "NIB_UPDATE_DIGEST_GOLDENS"

// externalCorpusEnv overrides where the external corpus is read from; the default is where
// `/pending 489` put veraPDF's own PDF/UA-1 corpus.
const externalCorpusEnv = "NIB_DIGEST_CORPUS"

func defaultExternalCorpus() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "nib", "verapdfs")
}

// goldenRow is one line of a golden file.
type goldenRow struct {
	input  string // sha256 of the document bytes fed to ContentDigest
	digest string // what ContentDigest returned, or "ERROR:<text>"
	name   string
}

func readGolden(t *testing.T, path string) map[string]goldenRow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]goldenRow{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			t.Fatalf("%s: malformed row %q — every row is name\\tinput\\tdigest", path, line)
		}
		out[parts[0]] = goldenRow{name: parts[0], input: parts[1], digest: parts[2]}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func writeGolden(t *testing.T, path, header string, rows []goldenRow) {
	t.Helper()
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	var b strings.Builder
	b.WriteString(header)
	for _, r := range rows {
		if strings.ContainsAny(r.name, "\t\n") {
			t.Fatalf("a corpus name with a tab or newline in it cannot be pinned: %q", r.name)
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\n", r.name, r.input, r.digest)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d rows to %s", len(rows), path)
}

// digestOrError is what a row records. An input `ContentDigest` REFUSES is pinned too, by its
// error text: a change that turns a refusal into a digest is exactly as much of a coverage change
// as one that moves a hash, and the external corpus is full of deliberately broken files.
//
// The error text is flattened, because two of pdfcpu's carry an embedded newline and a row is one
// line. It is flattened on BOTH sides — writing and comparing — or the stored value and the
// recomputed one differ by the flattening itself, which is a test failure that says nothing.
func digestOrError(pdf []byte) string {
	d, err := ContentDigest(pdf)
	if err != nil {
		return flattenRowValue("ERROR:" + err.Error())
	}
	return d
}

func flattenRowValue(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\t", " "))
}

// unstableDigest marks a generated document whose digest legitimately moves from run to run.
//
// **Found by this harness on its first green run, and it is a fact about the OPERATION, not about
// the digest.** Any document that goes through pdfcpu's annotation creation carries a timestamp:
// `model/annotation.go:396` writes `/M` from `types.DateString(time.Now())` and `:618` a
// `/CreationDate` the same way. `ContentDigest` hashes `/Annots` in full, by design and by
// ADR-013's reasoning, so two builds of the "same" document a second apart genuinely differ.
// Measured on unmodified code: three runs of the identical generator, three different digests for
// `notes-on-every-page`.
//
// Such a row cannot be pinned, so it asserts the property it still has — **the same BYTES digest
// the same way** — and says so rather than being quietly dropped. The byte-identity load for
// annotation-bearing documents is carried by the external corpus, which is files on disk and has a
// whole `7.18 Annotations` directory.
const unstableDigest = "#unstable"

// digestUnstable names them, with the mechanism, rather than detecting them.
//
// **Detection was tried first and is itself flaky**: building each document twice and comparing
// catches an unstable generator only when the two builds straddle a second boundary, so a
// regeneration run pinned `notes-on-every-page` to whatever it saw and the next run failed on it.
// A test whose acceptance depends on where the clock was is worse than no test — the reader cannot
// tell it from the regression it is there to find. Declared instead, and the declaration is
// checked in both directions below.
var digestUnstable = map[string]string{
	"notes-on-every-page": "AddNotes goes through pdfcpu's annotation creation, which stamps /M " +
		"from time.Now() (model/annotation.go:396)",
	"watermark": "StampWatermark creates an annotation, stamped the same way",
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// generatedDigestCorpus builds the in-repo half: one document per shape this package can produce.
//
// Failures to BUILD are fatal rather than skipped — a generator that stops working silently
// shrinks the acceptance, which is the failure mode this whole file exists to prevent.
func generatedDigestCorpus(t *testing.T) []struct {
	Name string
	PDF  []byte
} {
	t.Helper()
	type doc = struct {
		Name string
		PDF  []byte
	}
	must := func(b []byte, err error) []byte {
		t.Helper()
		if err != nil {
			t.Fatalf("building the corpus: %v", err)
		}
		return b
	}

	one := must(testpdf.Text("the lease"))
	three := must(testpdf.Text("clause one", "clause two", "clause three"))
	form := must(testpdf.Form())
	tagged := taggedFixture()
	// `WithUAIdentification` refuses a document with no indirect /Metadata packet, so the title
	// goes on first; that is the generator's contract, not a digest fact.
	titled := must(SetTitle(one, "A Lease"))

	out := []doc{
		{"text-1page", one},
		{"text-3page", three},
		{"form", form},
		{"tagged-handbuilt", tagged},
		{"uaid", must(testpdf.WithUAIdentification(titled))},
	}

	// Documents carrying the shapes the digest was widened to cover: annotations, form values,
	// attachments, geometry, and resources reached through a form XObject.
	notes := make([]Note, 0, 3)
	for i := 1; i <= 3; i++ {
		notes = append(notes, Note{Page: i, X: 72, Y: 700, Text: fmt.Sprintf("note %d", i)})
	}
	out = append(out,
		doc{"notes-on-every-page", must(AddNotes(three, notes))},
		doc{"attachment", must(AddAttachment(one, "Schedule-A.txt", []byte("rent is 1000/mo")))},
		doc{"title", titled},
		doc{"rotated", must(Rotate(three, []string{"2"}, 90))},
		doc{"cropped", must(Crop(three, [4]float64{0.05, 0.05, 0.05, 0.05}, []string{"1"}))},
		doc{"nup2", must(NUp(three, 2, false))},
		doc{"watermark", must(StampWatermark(three, "DRAFT", WatermarkStyle{}))},
		doc{"optimized", must(Optimize(three))},
		doc{"filled-form", must(FillFormJSON(form,
			[]byte(`{"forms":[{"textfield":[{"name":"fullName","value":"Jane Doe"}],`+
				`"checkbox":[{"name":"agree","value":true}]}]}`)))},
	)

	// A document whose annots reach a page dict, which reaches the Pages node, which reaches every
	// page — the shared-object graph `hashObjectSeen`'s `#again` marker exists for. Without a row
	// of this shape the marker's semantics are unpinned, and the marker is where a memoisation
	// across pages goes wrong silently.
	out = append(out, doc{"notes-then-nup", must(NUp(must(AddNotes(three, notes)), 2, false))})

	return out
}

func TestContentDigestIsByteIdenticalAcrossTheGeneratedCorpus(t *testing.T) {
	path := filepath.Join("testdata", "digest-generated.golden")
	docs := generatedDigestCorpus(t)

	if os.Getenv(digestGoldenUpdateEnv) != "" {
		rows := make([]goldenRow, 0, len(docs))
		for _, d := range docs {
			got := digestOrError(d.PDF)
			if _, unstable := digestUnstable[d.Name]; unstable {
				got = unstableDigest
			}
			rows = append(rows, goldenRow{name: d.Name, input: "-", digest: got})
		}
		writeGolden(t, path, "# ContentDigest over the generated corpus — /pending 488, ADR-013.\n"+
			"# <name>\\t<input sha256, `-` where the generator is not byte-stable>\\t<ContentDigest output>\n"+
			"# Regenerate with "+digestGoldenUpdateEnv+"=1; a regeneration is a "+
			"ContentDigestVersion decision, not a test fix.\n", rows)
		return
	}

	golden := readGolden(t, path)
	if len(golden) == 0 {
		t.Fatalf("%s is missing or empty; regenerate it with %s=1 on a build whose digest is the "+
			"one being committed to", path, digestGoldenUpdateEnv)
	}
	seen := map[string]bool{}
	for _, d := range docs {
		seen[d.Name] = true
		want, ok := golden[d.Name]
		if !ok {
			t.Errorf("%s: no golden row; the corpus grew without the goldens moving, so this "+
				"document is unpinned", d.Name)
			continue
		}
		got := digestOrError(d.PDF)
		// Every document asserts this, pinned or not: the same BYTES digest the same way. It is
		// the one property the timestamped rows keep, and it is where an object-number-keyed memo
		// that leaked between calls would show up first.
		if second := digestOrError(d.PDF); second != got {
			t.Errorf("%s: the SAME bytes digested twice gave %s then %s — ContentDigest is not a "+
				"function of the document", d.Name, got, second)
		}
		if reason, unstable := digestUnstable[d.Name]; unstable {
			if want.digest != unstableDigest {
				t.Errorf("%s is declared unstable (%s) but its golden row is pinned to %s; one of "+
					"the two is stale", d.Name, reason, want.digest)
			}
			continue
		}
		if want.digest == unstableDigest {
			t.Errorf("%s is pinned as unstable but is not in digestUnstable — an unpinned row "+
				"with no declared mechanism is a hole in the acceptance, not a row", d.Name)
			continue
		}
		if got != want.digest {
			t.Errorf("%s: ContentDigest returned %s and the stored commitment is %s. ADR-013: a "+
				"moved digest reads as tampering across a point release — every ceremony record "+
				"written by an earlier build fails its comparison.", d.Name, got, want.digest)
		}
	}
	for name := range golden {
		if !seen[name] {
			t.Errorf("%s: a golden row with no document; the corpus shrank, which shrinks the "+
				"acceptance silently", name)
		}
	}
}

func TestContentDigestIsByteIdenticalAcrossTheExternalCorpus(t *testing.T) {
	root := os.Getenv(externalCorpusEnv)
	if root == "" {
		root = defaultExternalCorpus()
	}
	if root == "" {
		t.Skipf("no external corpus root; set %s", externalCorpusEnv)
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("the external corpus is not on this machine (%s); set %s to point at one. The "+
			"generated half still ran.", root, externalCorpusEnv)
	}

	var files []string
	if err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(p), ".pdf") {
			return nil
		}
		files = append(files, p)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Skipf("no PDFs under %s", root)
	}

	path := filepath.Join("testdata", "digest-external.golden")
	if os.Getenv(digestGoldenUpdateEnv) != "" {
		rows := make([]goldenRow, 0, len(files))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			rel, _ := filepath.Rel(root, f)
			rows = append(rows, goldenRow{name: filepath.ToSlash(rel), input: sha256Hex(b), digest: digestOrError(b)})
		}
		writeGolden(t, path, "# ContentDigest over veraPDF's PDF/UA-1 corpus — /pending 488, ADR-013.\n"+
			"# <path under the corpus root>\\t<sha256 of the file>\\t<ContentDigest output>\n"+
			"# Regenerate with "+digestGoldenUpdateEnv+"=1; a regeneration is a "+
			"ContentDigestVersion decision, not a test fix.\n", rows)
		return
	}

	golden := readGolden(t, path)
	if len(golden) == 0 {
		t.Fatalf("%s is missing or empty; regenerate it with %s=1", path, digestGoldenUpdateEnv)
	}
	checked, drifted := 0, 0
	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		name := filepath.ToSlash(rel)
		want, ok := golden[name]
		if !ok {
			// A corpus that has GROWN is not a failure of this change; say so rather than
			// failing, or a new veraPDF release turns into a false regression.
			t.Logf("%s: not in the goldens (the corpus on this machine has grown); unpinned", name)
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(b); got != want.input {
			drifted++
			t.Logf("%s: the corpus FILE changed underneath the goldens (%s, pinned over %s); "+
				"skipped rather than reported as a digest change", name, got[:12], want.input[:12])
			continue
		}
		checked++
		if got := digestOrError(b); got != want.digest {
			t.Errorf("%s: ContentDigest returned\n  %s\nand the stored commitment is\n  %s\n"+
				"ADR-013: the digest's OUTPUT is a commitment, so this is a coverage change, "+
				"whatever it was meant to be.", name, got, want.digest)
		}
	}
	if checked == 0 {
		t.Fatalf("every one of the %d corpus files was skipped (%d drifted); the acceptance ran "+
			"over nothing", len(files), drifted)
	}
	t.Logf("byte-identity held over %d corpus documents (%d skipped as drifted, %d files on disk)",
		checked, drifted, len(files))
}
