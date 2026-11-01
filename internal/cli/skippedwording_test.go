package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheCLIAndTheWindowSayTheSameThingOfASkippedField — /pending 919. `nib fill` and the window's toast
// (`tellSkippedFields`) report the same fact — a field the data named that the form cannot take a value for — in two
// languages, and a change to one alone is how they drift. The clause they share is read out of the window's source,
// so changing either side red-lights this.
func TestTheCLIAndTheWindowSayTheSameThingOfASkippedField(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	i := strings.Index(src, "function tellSkippedFields(")
	if i < 0 {
		t.Fatal("tellSkippedFields is gone or renamed — the guard has nothing to bind to")
	}
	body := src[i : i+strings.Index(src[i:], "\n}")]
	if !strings.Contains(body, unfilledStem) {
		t.Errorf("the window's toast no longer contains %q, which `nib fill` says", unfilledStem)
	}
}
