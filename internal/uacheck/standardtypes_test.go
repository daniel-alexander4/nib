package uacheck

import "testing"

// TestTheCheckerHoldsTheSameStandardSetAsTheSpec — ISO 32000-1:2008 §14.8.4, Tables 333-340: 12 grouping, 19 block-level, 15 inline and 3 illustration types, 49 in all.
// Pinned against the table's count and a few names from each group, not against pdfops' copy: agreeing
// with the writer's list is exactly what `structure.go`'s header says the checker must not rely on.
func TestTheCheckerHoldsTheSameStandardSetAsTheSpec(t *testing.T) {
	if n := len(standardStructureTypes); n != 49 {
		t.Errorf("the standard set holds %d types, want the 49 of ISO 32000-1 §14.8.4", n)
	}
	for _, name := range []string{"Document", "Private", "H6", "LBody", "TFoot", "Annot", "WP", "Form"} {
		if !standardStructureTypes[name] {
			t.Errorf("/%s is a standard type in ISO 32000-1 §14.8.4 and the set lacks it", name)
		}
	}
	for _, name := range []string{"Artifact", "Title", "Aside", "FENote", "Sub", "Em", "Strong", "DocumentFragment"} {
		if standardStructureTypes[name] {
			t.Errorf("/%s is not a PDF 1.7 standard type (Artifact is a marked-content tag; the rest are PDF 2.0)", name)
		}
	}
}
