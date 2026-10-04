package uacheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheAgreementSentenceDoesNotCountARefusal — /pending 727. Every harness behind N skips a
// CannotCheck rather than scoring it, so "the same verdict on every document that exercises the rule"
// claimed agreement on documents nib refused. The sentence, and each document that restates it, says
// the agreement holds wherever nib reached a verdict.
func TestTheAgreementSentenceDoesNotCountARefusal(t *testing.T) {
	if !strings.Contains(Agreement(), "wherever nib reached one") {
		t.Errorf("the agreement sentence does not limit itself to documents nib reached a verdict on: %q", Agreement())
	}
	for _, doc := range []string{"README.md", filepath.Join("docs", "accessibility-parity.md")} {
		b, err := os.ReadFile(filepath.Join("..", "..", doc))
		if err != nil {
			t.Fatal(err)
		}
		s := strings.Join(strings.Fields(string(b)), " ")
		if !strings.Contains(s, "the same verdict on every document that exercises the rule") {
			t.Fatalf("%s no longer states the agreement claim this test reads", doc)
		}
		if !strings.Contains(s, "exercises the rule, wherever Nib reached one") {
			t.Errorf("%s states the agreement claim without limiting it to documents Nib reached a verdict on", doc)
		}
	}
}
