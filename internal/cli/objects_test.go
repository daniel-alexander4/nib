package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// `nib objects` (PLAN-image-edit P01.S02): the same door, the same causes, the same words as the route.

func listObjectsJSON(t *testing.T, path string) pdfops.PageObjects {
	t.Helper()
	out, code := captureStdout(t, func() int { return cmdObjects([]string{path, "--page", "1", "--json"}) })
	if code != 0 {
		t.Fatalf("nib objects exited %d", code)
	}
	var objs pdfops.PageObjects
	if err := json.Unmarshal([]byte(out), &objs); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	return objs
}

func TestNibObjectsListsAndRemovesAPicture(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, testpdf.TwoPictures(), 0o600); err != nil {
		t.Fatal(err)
	}
	objs := listObjectsJSON(t, in)
	if len(objs.Objects) != 2 {
		t.Fatalf("listed %+v", objs.Objects)
	}
	plain, code := captureStdout(t, func() int { return cmdObjects([]string{in, "--page", "1"}) })
	if code != 0 || !strings.Contains(plain, "picture 1  image  "+objs.Objects[0].Digest) || !strings.Contains(plain, "removable") {
		t.Fatalf("plain listing %d %q", code, plain)
	}
	out := filepath.Join(dir, "out.pdf")
	if _, code := captureStdout(t, func() int {
		return cmdObjects([]string{"remove", in, "--page", "1", "--picture", "1", "--digest", objs.Objects[0].Digest, "-o", out})
	}); code != 0 {
		t.Fatalf("remove exited %d", code)
	}
	after := listObjectsJSON(t, out)
	if len(after.Objects) != 1 || after.Objects[0].Digest != objs.Objects[1].Digest {
		t.Fatalf("after: %+v", after.Objects)
	}
	b, _ := os.ReadFile(out)
	if bytes.Contains(b, []byte("NIBPLANTEDIMAGEA")) {
		t.Fatal("the removed picture's bytes are in the output")
	}
	// The digest is the precondition: the same command again names a picture the file no longer draws there.
	var code2 int
	msg := captureStderr(t, func() {
		code2 = cmdObjects([]string{"remove", out, "--page", "1", "--picture", "1", "--digest", objs.Objects[0].Digest, "-w"})
	})
	if code2 != 1 || !strings.Contains(msg, pdfops.ObjectCauseWords(pdfops.CauseObjectChanged)) {
		t.Fatalf("a stale address exited %d saying %q", code2, msg)
	}
}

func TestNibObjectsRefusesASignedDocumentInTheRoutesWords(t *testing.T) {
	certPEM, keyPEM, err := sign.GenerateIdentity("Nib Test")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.Sign(testpdf.TwoPictures(), certPEM, keyPEM, sign.Options{Name: "Nib Test", When: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(t.TempDir(), "signed.pdf")
	if err := os.WriteFile(in, signed, 0o600); err != nil {
		t.Fatal(err)
	}
	d := listObjectsJSON(t, in).Objects[0].Digest
	var code int
	msg := captureStderr(t, func() {
		code = cmdObjects([]string{"remove", in, "--page", "1", "--picture", "1", "--digest", d, "-w"})
	})
	if code != 1 || !strings.Contains(msg, pdfops.ObjectCauseWords(pdfops.CauseSigned)) {
		t.Fatalf("exited %d saying %q", code, msg)
	}
	if b, _ := os.ReadFile(in); !bytes.Equal(b, signed) {
		t.Fatal("the signed file was rewritten")
	}
}

func TestNibObjectsWantsItsFlags(t *testing.T) {
	for _, args := range [][]string{{"x.pdf"}, {"remove", "x.pdf", "--page", "1"}} {
		var code int
		captureStderr(t, func() { code = cmdObjects(args) })
		if code != 2 {
			t.Errorf("nib objects %v exited %d, want 2", args, code)
		}
	}
}
