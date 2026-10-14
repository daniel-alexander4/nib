package pdfops

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// `/pending 888`: the scan gained a kind — `offPageFields`, v1.215.0 — and StripActive, which re-scans its own
// output and refuses any finding it has not named as left, refused every document holding one.

// TestStrippingLeavesFormFieldsOnNoPageAndDoesNotRefuseOverThem — the behaviour, on a form that has them.
func TestStrippingLeavesFormFieldsOnNoPageAndDoesNotRefuseOverThem(t *testing.T) {
	pdf := formWithFieldsOffPage(true)
	has := func(b []byte) bool {
		t.Helper()
		rep, err := Scan(b)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range rep.Findings {
			if f.Kind == "offPageFields" {
				return true
			}
		}
		return false
	}
	if !has(pdf) {
		t.Fatal("setup: the fixture has no form field on no page")
	}
	out, err := StripActive(pdf)
	if err != nil {
		t.Fatalf("the strip refused a document over its form fields on no page: %v", err)
	}
	if !has(out) {
		t.Error("the strip removed form fields nobody asked it to: that removal is RemoveOffPageFields', and is asked for (ADR-166)")
	}
}

// TestEveryScanKindIsStrippedOrNamedAsLeft — the list. Every kind `scan.go` can report is one StripActive
// removes or one `strippedExempt` names; a kind in neither is the next document the strip refuses.
func TestEveryScanKindIsStrippedOrNamedAsLeft(t *testing.T) {
	removed := map[string]bool{
		"action": true, "additionalActions": true, "attachment": true, "hiddenLayer": true, "javascript": true,
		"media": true, "openAction": true, "slideshow": true, "xfa": true,
	}
	src, err := os.ReadFile("scan.go")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, m := range regexp.MustCompile(`\badd\("([A-Za-z]+)"|Kind:\s*"([A-Za-z]+)"`).FindAllStringSubmatch(string(src), -1) {
		found[m[1]+m[2]] = true
	}
	// A floor: the matcher reading nothing would make every claim below true of nothing.
	if len(found) < 10 {
		t.Fatalf("only %d finding kinds read from scan.go — the matcher has stopped finding them", len(found))
	}
	var unnamed []string
	for k := range found {
		if removed[k] == strippedExempt[k] {
			unnamed = append(unnamed, k)
		}
	}
	sort.Strings(unnamed)
	if len(unnamed) > 0 {
		t.Errorf("scan kinds that are neither removed by StripActive nor named in strippedExempt (or are in both): %s — "+
			"StripActive refuses a document holding one", strings.Join(unnamed, ", "))
	}
	for k := range removed {
		if !found[k] {
			t.Errorf("this test lists %q as a kind StripActive removes and scan.go reports no such kind", k)
		}
	}
	for k := range strippedExempt {
		if !found[k] {
			t.Errorf("strippedExempt names %q and scan.go reports no such kind", k)
		}
	}
}
