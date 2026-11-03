package text

import (
	"strings"
	"testing"

	"nib/internal/docmodel"
	"nib/internal/docwrite/wtest"
)

func TestTextCarriesEveryWordInOrder(t *testing.T) {
	d := wtest.Hostile()
	out := string(Write(d))
	got := strings.Fields(out)
	var i int
	for _, w := range wtest.Words(d) {
		for i < len(got) && got[i] != w {
			i++
		}
		if i == len(got) {
			t.Fatalf("word %q missing or out of order in:\n%s", w, out)
		}
		i++
	}
	if strings.ContainsAny(out, "\x00\x01\x08\u0085￾") {
		t.Errorf("a control character survived: %q", out)
	}
	for _, want := range []string{"1. first", "  • nested", "[Picture: Scanned page 3", "Name\tValue | v"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestTextOfNothingIsNil(t *testing.T) {
	if Write(docmodel.Document{}) != nil {
		t.Error("an empty document wrote bytes")
	}
}
