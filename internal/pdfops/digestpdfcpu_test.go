package pdfops

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var pdfcpuRequire = regexp.MustCompile(`(?m)^\s*github\.com/pdfcpu/pdfcpu (v\S+)`)

// pdfcpuPinIn is the pdfcpu version a go.mod text requires, or "" when it names none.
func pdfcpuPinIn(gomod string) string {
	m := pdfcpuRequire.FindStringSubmatch(gomod)
	if m == nil {
		return ""
	}
	return m[1]
}

// digestRuleAgrees reports whether the rule this build computes was measured against the pdfcpu gomod requires.
func digestRuleAgrees(rule int, gomod string) bool {
	want, ok := digestRulePdfcpu[rule]
	return ok && want == pdfcpuPinIn(gomod)
}

// TestTheDigestRuleNamesThePdfcpuItWasMeasuredWith — D1's guard. The digest is taken over pdfcpu's own graph, so a
// pdfcpu bump with no rule bump moves digests on unchanged bytes and reads as tampering across the point release.
// Red when go.mod is edited alone (the version no longer matches the table), when the table is edited alone (it
// no longer matches go.mod), and when ContentDigestVersion is bumped alone (it has no entry): the three cases below
// plant each against the real go.mod text, so the guard is shown to fire rather than assumed to.
func TestTheDigestRuleNamesThePdfcpuItWasMeasuredWith(t *testing.T) {
	raw, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	gomod := string(raw)
	if pdfcpuPinIn(gomod) == "" {
		t.Fatal("setup: go.mod names no github.com/pdfcpu/pdfcpu requirement — this guard cannot see the pin")
	}
	if !digestRuleAgrees(ContentDigestVersion, gomod) {
		t.Errorf("ContentDigestVersion %d was measured against pdfcpu %q (digestRulePdfcpu) and go.mod requires %q — "+
			"re-measure the digest on the new pdfcpu, bump the rule if it moved (the comparison instrument is build/pdfcpudiff.sh), then move this table",
			ContentDigestVersion, digestRulePdfcpu[ContentDigestVersion], pdfcpuPinIn(gomod))
	}
	// nib carries pdfcpu as published (ADR-174): a `replace` of it would let this guard agree with a fork.
	if regexp.MustCompile(`(?m)^\s*(replace\s+)?github\.com/pdfcpu/pdfcpu\s+=>`).MatchString(gomod) {
		t.Error("go.mod replaces github.com/pdfcpu/pdfcpu: the digest rule was measured against the published release, not a fork")
	}
	// The guard fires on each edit made alone.
	bumped := strings.Replace(gomod, pdfcpuPinIn(gomod), "v0.99.0", 1)
	if digestRuleAgrees(ContentDigestVersion, bumped) {
		t.Error("a go.mod bump alone left the guard green")
	}
	if digestRuleAgrees(ContentDigestVersion+1, gomod) {
		t.Error("a ContentDigestVersion bump alone left the guard green")
	}
}
