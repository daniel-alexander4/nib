package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
)

// TestEveryNameInSignedTakesTheSlugDoor — /pending 821. Three writers name files in ~/nib/signed from
// untrusted text, the 48-byte cap was copied into two of them, and the third (`receivedName`, the
// peer's label) had none — so a long label made a name past NAME_MAX, refused by the filesystem
// after the sender had been told the document arrived.
//
// Both halves of ADR-009's guard: the behaviour (one long text, three names, one capped slug) and
// the routing (each builder calls the door), because a fourth builder added with its own cap would
// pass the first half.
func TestEveryNameInSignedTakesTheSlugDoor(t *testing.T) {
	long := strings.Repeat("Counterparty Holdings ", 20) // 440 bytes of label
	want := fileSlug(long, "x")
	if want == "x" || len(want) > fileSlugMax {
		t.Fatalf("setup: fileSlug(long) = %q, want a non-empty slug of at most %d bytes", want, fileSlugMax)
	}
	names := map[string]string{
		"receivedName":  receivedName(long, []byte{1, 2, 3, 4}, []byte("doc")),
		"deliveredName": deliveredName(ceremony.Record{ID: "0123456789abcdef0123456789abcdef", Intent: long}),
		"keptName":      keptName(long, []byte("doc"), time.Now()),
	}
	for fn, name := range names {
		if len(name) > 255 {
			t.Errorf("%s made a %d-byte name from a long label — past NAME_MAX, so the write is refused", fn, len(name))
		}
		if !strings.Contains(name, want) || strings.Contains(name, want+"-counterparty") {
			t.Errorf("%s = %q does not carry the door's capped slug %q", fn, name, want)
		}
	}

	src := map[string]string{"session.go": "receivedName", "delivery.go": "deliveredName", "kept.go": "keptName"}
	for file, fn := range src {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		body := funcBodyFrom(s, strings.Index(s, "func "+fn+"("))
		if body == "" {
			t.Fatalf("could not read %s's body in %s — the routing clause would pass over nothing", fn, file)
		}
		if !strings.Contains(body, "fileSlug(") {
			t.Errorf("%s no longer names its file through fileSlug — a second slug rule is how the cap was lost once", fn)
		}
	}
}
