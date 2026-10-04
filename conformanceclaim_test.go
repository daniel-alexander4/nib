package nib

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Law 2's NEGATIVE half — `PLAN-ua-coverage.md`'s "106 rules is 'agrees with veraPDF', never 'PDF/UA
// conformant' … Nothing in the docs or the report may say more" (/pending 713).
//
// `internal/uacheck/door_test.go`'s `TestPassingEveryClauseNibChecksIsNotConformanceAndTheDocsSaySo`
// holds the POSITIVE half: sentences that must be present. Nothing held the other half — a later edit
// could write "your document is PDF/UA compliant" into a toast and every test would stay green, because
// the checks were for text present, never for text absent. P08's exit criterion was "MET by reading".
//
// **What is scanned is what a person reads**, and only that: the README, the user-facing docs, the
// page's markup (HTML comments stripped), `web/app.js`'s string literals (comments stripped, so the
// many code comments that DISCUSS conformance are not claims), and every string literal in non-test Go
// under `internal/` and `cmd/` — the CLI's help and output, the server's responses, and the refusal
// and `Why` text the checker hands both. Struct tags are skipped: a wire key is read by code, never
// printed (the report's own flag is `allCheckedPass`, renamed from `conformant` by /pending 765 part 4).
// ADRs, plans and `docs/red-proofs.md` are not scanned: they are where the rule is argued, so they must
// be free to name what it forbids.
//
// **Any hit is a violation unless an allow-list row names it**, with its reason. The row is a fragment
// that must SPAN the hit, never a file or a paragraph, so a new claim written beside an allowed
// sentence is still caught. Every allowed row must still match something — a row whose sentence was rewritten is
// stale, and a stale row is an exemption waiting for a sentence to grow back into it.
func TestNothingAPersonReadsClaimsConformance(t *testing.T) {
	hits := conformanceClaimHits(t)
	used := make([]bool, len(conformanceAllowed))
	var bad []string
	for _, h := range hits {
		ok := false
		for i, a := range conformanceAllowed {
			if a.file == h.file && covers(h.text, a.fragment, h.at) {
				used[i] = true
				ok = true
			}
		}
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: %q in %q", h.file, h.match, clip(h.text)))
		}
	}
	for _, b := range bad {
		t.Errorf("claims conformance nib has not verified (law 2 — say \"agrees with veraPDF on N rules\", or allow-list it with a reason): %s", b)
	}
	for i, a := range conformanceAllowed {
		if !used[i] {
			t.Errorf("allow-list row %s %q matches nothing — stale; delete it", a.file, a.fragment)
		}
	}
	// A scan that read nothing passes everything.
	if len(hits) < len(conformanceAllowed) {
		t.Errorf("only %d hits across the surfaces — the scanner stopped reading", len(hits))
	}
}

// conformanceClaim is the forbidden vocabulary: every way found of saying a document IS PDF/UA (or
// accessible) rather than that nib's checks agree with veraPDF's. Case-insensitive.
//
// **"conformance" is in the list though most of its uses are negations** ("not a conformance
// certificate"), because the positive forms ("confirms conformance", "PDF/UA conformance verified")
// cannot be told from the negations by any pattern short of parsing English. The negations are few and
// each is allow-listed by name. "certificate" alone is NOT in the list: it is the signing feature's word
// (40+ uses), and the accessibility uses of it all sit in a sentence carrying one of the words below.
var conformanceClaim = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\bconform(s|ant|ance|ing|ed)?\b`,
	`\bcomplian(t|ce)\b`,
	`\bcompl(y|ies)\s+with\b`,
	`\bpdf/ua(-1)?\s+(valid|validated|certified|verified|approved|ready)\b`,
	`\b(valid|certified)\s+pdf/ua\b`,
	`\b(is|are|passes|meets|satisfies)\s+(the\s+)?pdf/ua\b`,
	`\b(fully|100%)\s+accessible\b`,
	`\bcertified\s+accessible\b`,
	`\baccessible\s+pdf\b`,
}, "|"))

// conformanceAllowed names every legitimate use: a negation, a quotation of someone else's claim, or a
// statement about what a claim IS rather than a claim. `fragment` is a substring of the hit's text.
var conformanceAllowed = []struct{ file, fragment, why string }{
	{"README.md", "for a conformance verdict, use veraPDF",
		"points the reader AWAY from nib for the verdict"},
	{"README.md", "Markdown renderer produces as conformant, not on Nib's checker",
		"ADR-033: the one label nib writes, stating that it rests on veraPDF's measurement, not on nib"},
	{"README.md", "claims PDF/UA conformance, the claim is removed",
		"ADR-032: describes the claim nib DROPS"},
	{"README.md", "Nib cannot tell whether an edit kept the document conformant",
		"ADR-032: a refusal to claim, stated as one"},
	{"internal/testpdf/uaid.go", "<pdfaid:conformance>B</pdfaid:conformance>",
		"ADR-083: the test fixture's PDF/A claim, written so the drop of it can be tested"},
	{"README.md", "falsely claims conformance",
		"PDF/A: the refusal to produce a file that would claim it"},
	{"docs/accessibility-parity.md", "Agreement is the claim; conformance is not.",
		"law 2 itself, stated to the reader"},
	{"docs/accessibility-parity.md", "where veraPDF measures every construct Nib renders conformant",
		"ADR-033: the one label nib writes, and the verdict it rests on is veraPDF's"},
	{"docs/accessibility-parity.md", "verifies whether the document conforms to accessibility standards",
		"a quotation of Acrobat's own documentation [A1], attributed"},
	{"web/index.html", "For a conformance verdict use veraPDF",
		"points the reader AWAY from nib for the verdict"},
	{"web/index.html", "conformance itself, so this is a candidate, not a certified file",
		"PDF/A: says nib cannot validate conformance"},
	{"web/app.js", "so this is not a conformance certificate",
		"the clean summary's own disclaimer (P08's phase-open reading)"},
	{"README.md", "Nib can't certify conformance itself",
		"`nib pdfa`'s row: says nib cannot certify PDF/A conformance"},
	{"internal/pdfops/labelua.go", "a conformance claim cannot rest on one",
		"ADR-033's refusal: the label is withheld"},
	{"internal/pdfops/pdfa.go", "<pdfaid:conformance>B</pdfaid:conformance>",
		"the PDF/A XMP schema's own property name, written into the file, never shown as prose"},
	{"internal/uacheck/door.go", "nothing can be said to conform",
		"the NotRun refusal: a negation"},
	{"internal/uacheck/rules_catalog.go", "the PDF/UA version and conformance level shall be specified",
		"quotes ISO 14289-1's requirement (clause 5), not a verdict"},
	{"internal/uacheck/rules_catalog.go", "the International Standard to which the file conforms",
		"quotes ISO 14289-1's requirement (clause 5), not a verdict"},
	{"internal/uacheck/rules_catalog.go", "which part of PDF/UA it claims to conform to",
		"a failure's Why: what the DOCUMENT claims, reported as missing"},
	{"internal/uacheck/rules_catalog.go", "conformance without saying to which part of the standard",
		"a failure's Why: what the DOCUMENT claims, reported as malformed"},
	{"internal/uacheck/rules_catalog.go", "is present but empty, so it claims conformance",
		"a failure's Why: what the DOCUMENT claims, reported as malformed"},
	{"internal/uacheck/rules_file.go", "a conforming file shall not contain any reference XObjects",
		"quotes ISO 14289-1's requirement, not a verdict"},
	{"internal/uacheck/rules_structure.go", "because a conforming reader recognises that type",
		"\"conforming reader\" is ISO 32000's term for a PDF viewer, not a claim about the document"},
	{"internal/uacheck/rules_truetype.go", "so they are not shown to be Unicode-compliant",
		"a CannotCheck's Why: a negation"},
}

type claimHit struct {
	file, match, text string
	at                int // the match's offset in text
}

// covers reports whether some occurrence of fragment in text spans the match at offset at — so an
// allowed sentence exempts its own word and never a second claim written into the same paragraph.
func covers(text, fragment string, at int) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], fragment)
		if j < 0 {
			return false
		}
		if i+j <= at && at < i+j+len(fragment) {
			return true
		}
		i += j + 1
	}
}

func conformanceClaimHits(t *testing.T) []claimHit {
	t.Helper()
	var hits []claimHit
	scan := func(file, text string) {
		for _, m := range conformanceClaim.FindAllStringIndex(text, -1) {
			hits = append(hits, claimHit{file, text[m[0]:m[1]], text, m[0]})
		}
	}
	read := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	htmlComment := regexp.MustCompile(`(?s)<!--.*?-->`)
	// Prose is scanned per paragraph, so the text an allow-list fragment matches is the sentence's
	// neighbourhood and a line-wrap cannot split a phrase from the scan.
	prose := func(path string) {
		for _, para := range regexp.MustCompile(`\n\s*\n`).Split(htmlComment.ReplaceAllString(read(path), ""), -1) {
			scan(filepath.ToSlash(path), strings.Join(strings.Fields(para), " "))
		}
	}
	prose("README.md")
	prose("docs/accessibility-parity.md")
	prose(filepath.Join("web", "index.html"))
	for _, lit := range jsStringLiterals(stripJSComments(read(filepath.Join("web", "app.js")))) {
		scan("web/app.js", strings.Join(strings.Fields(lit), " "))
	}
	fset := token.NewFileSet()
	for _, root := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.Field:
					// A field's tag is a wire key, not prose; its type holds no user-facing literal.
					return false
				case *ast.BasicLit:
					if n.Kind == token.STRING {
						scan(filepath.ToSlash(path), n.Value)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].file < hits[b].file })
	return hits
}

// jsStringLiterals returns the bodies of every quoted and template literal in comment-stripped source.
func jsStringLiterals(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '"' && c != '\'' && c != '`' {
			continue
		}
		j := skipJSString(s, i)
		out = append(out, s[i+1:j])
		i = j
	}
	return out
}

func clip(s string) string {
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}
