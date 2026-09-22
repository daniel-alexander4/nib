package p2p

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
)

// `PLAN-ua-coverage.md` P02.S09 — nib's own pages are tagged as real content, so a tagged document
// stays wholly tagged through co-sign and ceremony preparation, and an untagged one is not made to
// claim tagging by nib's pages.
//
// Measured before the slice, on this test's own host: a tagged Markdown conversion came out of
// `PrepareCeremonyDocument` still claiming tagging, at tier Exact, with 35 text runs outside any
// element at two signers and 36 at eight — every co-signed tagged document shipped that shape.

func taggedHost(t *testing.T) []byte {
	t.Helper()
	pdf, err := pdfops.ConvertDocToPDF([]byte("# A contract\n\nThe parties agree to the terms below.\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	if u, err := pdfops.UnmarkedTextRuns(pdf); err != nil || u != 0 || !pdfops.Inspect(pdf).Tagged {
		t.Fatalf("setup: the host is not a wholly tagged document (%d unmarked runs, err %v)", u, err)
	}
	return pdf
}

// TestATaggedDocumentStaysWhollyTaggedThroughPreparation — the slice's point, on both preparation
// paths: the plain co-sign (the readme alone) and a ceremony at two and eight signers (the readme, the
// ceremony page and every signature page).
func TestATaggedDocumentStaysWhollyTaggedThroughPreparation(t *testing.T) {
	host := taggedHost(t)
	cases := []struct {
		name    string
		prepare func() ([]byte, error)
	}{
		{"co-sign", func() ([]byte, error) { return PrepareDocument(host) }},
		{"ceremony, 2 signers", func() ([]byte, error) {
			return PrepareCeremonyDocument(host, CeremonyID{1, 2, 3}, []byte("convener"), 2)
		}},
		{"ceremony, 8 signers", func() ([]byte, error) {
			return PrepareCeremonyDocument(host, CeremonyID{1, 2, 3}, []byte("convener"), 8)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.prepare()
			if err != nil {
				t.Fatal(err)
			}
			if n, _ := pdfops.PageCount(out); n < 2 {
				t.Fatalf("setup: preparation produced %d page(s); nib's pages were not appended", n)
			}
			if !pdfops.Inspect(out).Tagged {
				t.Fatal("the prepared document no longer claims tagging")
			}
			if u, err := pdfops.UnmarkedTextRuns(out); err != nil || u != 0 {
				t.Errorf("%d text runs are outside any structure element (err %v) — nib's pages are "+
					"undescribed under the document's claim", u, err)
			}
			if src, ok := pdfops.StructureSource(out); !ok || string(src) != "Exact" {
				t.Errorf("the tree's tier is %q (recorded %v), want the host's Exact kept", src, ok)
			}
		})
	}
}

// TestAnUntaggedDocumentIsNotMadeToClaimTagging — extend-only (ADR-048), through the real callers: nib's
// tagged pages appended to a document nobody tagged must not make it claim a structure.
func TestAnUntaggedDocumentIsNotMadeToClaimTagging(t *testing.T) {
	user, err := pdfops.CreateFromJSON([]byte(`{"pages":{"1":{"content":{"text":[` +
		`{"value":"A contract","anchor":"TopLeft","position":[72,720],` +
		`"font":{"name":"Helvetica","size":12}}]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if pdfops.Inspect(user).Tagged {
		t.Fatal("setup: the user's document already claims tagging")
	}
	out, err := PrepareCeremonyDocument(user, CeremonyID{4}, []byte("convener"), 2)
	if err != nil {
		t.Fatal(err)
	}
	if pdfops.Inspect(out).Tagged {
		t.Error("an untagged document claims tagging after nib appended its own tagged pages")
	}
}

// TestAPreparedTaggedDocumentGainsNoUA1Clause — veraPDF over the ceremony-prepared tagged document:
// nib's pages add nothing its input did not fail (5 t1 aside — ADR-032 drops the identification).
func TestAPreparedTaggedDocumentGainsNoUA1Clause(t *testing.T) {
	vp := verapdfPath()
	if vp == "" {
		t.Skip("SKIP (not a pass): veraPDF is absent, so the prepared document's ua1 delta is UNCHECKED in this run")
	}
	host := taggedHost(t)
	out, err := PrepareCeremonyDocument(host, CeremonyID{5}, []byte("convener"), 2)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "host.pdf"), filepath.Join(dir, "prepared.pdf")
	for p, d := range map[string][]byte{a: host, b: out} {
		if err := os.WriteFile(p, d, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := ua1Failures(t, vp, []string{a, b})
	for c := range got["prepared.pdf"] {
		if !got["host.pdf"][c] && c != "5 t1" {
			t.Errorf("preparation adds %s, which the tagged host does not fail", c)
		}
	}
}

// verapdfPath finds veraPDF the three ways `internal/pdfops`' tests do: NIB_VERAPDF, PATH, ~/verapdf.
func verapdfPath() string {
	if p := os.Getenv("NIB_VERAPDF"); p != "" {
		return p
	}
	if p, err := exec.LookPath("verapdf"); err == nil {
		return p
	}
	if home, _ := os.UserHomeDir(); home != "" {
		if cand := filepath.Join(home, "verapdf", "verapdf"); fileExists(cand) {
			return cand
		}
	}
	return ""
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// ua1Failures validates the files in one veraPDF run and returns each file's failed clauses ("7.1 t3").
// A file veraPDF could not validate is a test failure, not an empty set.
func ua1Failures(t *testing.T, vp string, files []string) map[string]map[string]bool {
	t.Helper()
	out, _ := exec.Command(vp, append([]string{"--flavour", "ua1"}, files...)...).Output()
	var rep struct {
		Jobs []struct {
			Item struct {
				Name string `xml:"name"`
			} `xml:"item"`
			Report struct {
				Status string `xml:"jobEndStatus,attr"`
				Rules  []struct {
					Clause string `xml:"clause,attr"`
					Test   string `xml:"testNumber,attr"`
					Status string `xml:"status,attr"`
				} `xml:"details>rule"`
			} `xml:"validationReport"`
		} `xml:"jobs>job"`
	}
	if err := xml.Unmarshal(out, &rep); err != nil {
		t.Fatalf("veraPDF report did not parse: %v\n%.600s", err, out)
	}
	got := map[string]map[string]bool{}
	for _, j := range rep.Jobs {
		if j.Report.Status != "normal" {
			t.Fatalf("veraPDF could not validate %s (%s)", j.Item.Name, j.Report.Status)
		}
		set := map[string]bool{}
		for _, r := range j.Report.Rules {
			if r.Status == "failed" {
				set[r.Clause+" t"+r.Test] = true
			}
		}
		got[filepath.Base(j.Item.Name)] = set
	}
	return got
}

// kindsOf lists the structure elements that describe text, in order, as "kind:first words".
func kindsOf(t *testing.T, pdf []byte) []string {
	t.Helper()
	tree, err := pdfops.ReadStructure(pdf)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range tree.Elements {
		if e.Text == "" {
			continue
		}
		w := strings.Fields(e.Text)
		if len(w) > 3 {
			w = w[:3]
		}
		out = append(out, e.Kind+":"+strings.Join(w, " "))
	}
	return out
}

// TestNibsPagesAreReadAsTheParagraphsTheyAre — the roles are exact, so the elements are the page's own
// structure: the readme's title and one paragraph per paragraph (not one per wrapped line, not one for
// the whole page), and the ceremony page's facts each on their own with the hand-wrapped last sentence
// whole.
func TestNibsPagesAreReadAsTheParagraphsTheyAre(t *testing.T) {
	readme, err := RenderReadme()
	if err != nil {
		t.Fatal(err)
	}
	got := kindsOf(t, readme)
	if len(got) != 1+len(readmeParagraphs) || !strings.HasPrefix(got[0], "H1:About this co-signed") {
		t.Errorf("the readme reads as %d element(s) %v; want its title as H1 and %d paragraphs",
			len(got), got, len(readmeParagraphs))
	}
	for i, k := range got[1:] {
		if !strings.HasPrefix(k, "P:") {
			t.Errorf("readme element %d is %q, want a paragraph", i+1, k)
		}
	}

	// A 32-byte fingerprint, so `pairing.Name` reads it and the "which reads as" line is DRAWN — the
	// line that must stay with the fingerprint it reads, and that a shorter fixture never draws.
	fp := make([]byte, 32)
	for i := range fp {
		fp[i] = byte(i + 1)
	}
	page, err := renderCeremonyPage(CeremonyID{7}, fp, 3)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := pdfops.ReadStructure(page)
	if err != nil {
		t.Fatal(err)
	}
	drawn := false
	for _, e := range tree.Elements {
		if strings.Contains(e.Text, "which reads as") {
			drawn = strings.HasPrefix(e.Text, "Convened by:")
		}
	}
	if !drawn {
		t.Error("the convener's name line is not read with the fingerprint it names")
	}
	cer := kindsOf(t, page)
	want := []string{"P:This document is", "P:Ceremony:", "P:Parties obliged to", "P:Convened by:", "P:Each signature below", "P:A signature block"}
	if len(cer) != len(want) {
		t.Fatalf("the ceremony page reads as %v; want %d paragraphs %v", cer, len(want), want)
	}
	for i := range want {
		if !strings.HasPrefix(cer[i], want[i]) {
			t.Errorf("ceremony element %d is %q, want it to start %q", i, cer[i], want[i])
		}
	}

	sig, err := renderSignaturePage(CeremonyID{7}, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := kindsOf(t, sig); len(got) != 1 || !strings.HasPrefix(got[0], "H1:Signatures") {
		t.Errorf("the signature page reads as %v; want its one line as a level-1 heading", got)
	}
}
