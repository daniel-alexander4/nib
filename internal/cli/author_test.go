package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

const authorFields = `[
 {"page":1,"rect":[50,700,250,720],"kind":"text","name":"who","label":"Your name","value":"Ada Lovelace"},
 {"page":1,"rect":[50,660,62,672],"kind":"check","name":"agree","label":"Agree","value":true},
 {"page":1,"rect":[50,620,200,640],"kind":"dropdown","name":"status","options":["single","joint"],"value":"joint"},
 {"page":1,"rect":[50,560,150,580],"kind":"radio","name":"pick","options":["a","b"],"value":"b"},
 {"page":1,"rect":[50,500,250,520],"kind":"text","name":"blank","label":"Left blank"}
]`

// TestAuthorAddsFieldsWithTheirDefaults — RelaxTax B2. The window's authoring door, with a default per field.
func TestAuthorAddsFieldsWithTheirDefaults(t *testing.T) {
	base, err := testpdf.Text("")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in, fp, out := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "f.json"), filepath.Join(dir, "out.pdf")
	os.WriteFile(in, base, 0o600)
	os.WriteFile(fp, []byte(authorFields), 0o600)
	_, stderr, code := fillChild(t, "", "", "author", in, "--fields", fp, "-o", out)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	// A radio group cannot be tagged by the door the window uses (AuthorTaggedForm), so this set is untagged.
	if !strings.Contains(stderr, "added 5 field(s), 4 with a default value; the form is untagged") {
		t.Errorf("summary %q", stderr)
	}
	doc, _ := os.ReadFile(out)
	fields, err := pdfops.ListFormFields(doc)
	if err != nil || len(fields) != 5 {
		t.Fatalf("%d fields, %v", len(fields), err)
	}
	xfdf, err := pdfops.ExportFormXFDF(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Ada Lovelace", "joint"} {
		if !strings.Contains(string(xfdf), want) {
			t.Errorf("the default %q is not in the form's data:\n%s", want, xfdf)
		}
	}
	// A stimulus for the blank one: it must not have been given a value.
	if strings.Count(string(xfdf), "<value>") > 5 {
		t.Errorf("more values than defaults:\n%s", xfdf)
	}
}

func TestAuthorTagsAFormWithoutARadioGroup(t *testing.T) {
	base, _ := testpdf.Text("")
	dir := t.TempDir()
	in, fp := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "f.json")
	os.WriteFile(in, base, 0o600)
	os.WriteFile(fp, []byte(`[{"page":1,"rect":[50,700,250,720],"kind":"text","name":"who","label":"Your name","value":"Ada"}]`), 0o600)
	_, stderr, code := fillChild(t, "", "", "author", in, "--fields", fp, "-o", filepath.Join(dir, "out.pdf"))
	if code != 0 || !strings.Contains(stderr, "the form is tagged") {
		t.Errorf("exit %d, summary %q, want a tagged form", code, stderr)
	}
}

func TestAuthorRefusesAValueThatIsNotAnOptionNamingTheFieldAndNotTheValue(t *testing.T) {
	base, _ := testpdf.Text("")
	dir := t.TempDir()
	in, fp := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "f.json")
	os.WriteFile(in, base, 0o600)
	os.WriteFile(fp, []byte(`[{"page":1,"rect":[50,620,200,640],"kind":"dropdown","name":"status","options":["single","joint"],"value":"MARKER-123-45-6789"}]`), 0o600)
	_, stderr, code := fillChild(t, "", "", "author", in, "--fields", fp, "-o", filepath.Join(dir, "out.pdf"))
	if code != 1 || !strings.Contains(stderr, `"status"`) {
		t.Errorf("exit %d, stderr %q, want 1 naming the field", code, stderr)
	}
	if strings.Contains(stderr, "MARKER") {
		t.Errorf("the refusal repeats the value: %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.pdf")); err == nil {
		t.Error("an output was written for a refused request")
	}
}

func TestAuthorTakesThePdfOnStdinAndWritesStdout(t *testing.T) {
	base, _ := testpdf.Text("")
	fp := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(fp, []byte(authorFields), 0o600)
	stdout, stderr, code := fillChild(t, string(base), "", "author", "-", "--fields", fp, "-o", "-")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if fields, err := pdfops.ListFormFields(stdout); err != nil || len(fields) != 5 {
		t.Errorf("%d fields, %v", len(fields), err)
	}
	if _, stderr, code := fillChild(t, string(base), "", "author", "-", "--fields", "-", "-o", "-"); code != 1 || !strings.Contains(stderr, "cannot both be") {
		t.Errorf("both on stdin: exit %d, %q", code, stderr)
	}
}

func TestAuthorDefaults(t *testing.T) {
	for _, c := range []struct {
		f    authorField
		fail string
	}{
		{authorField{FormField: pdfops.FormField{Kind: "check", Name: "c"}, Value: "maybe"}, `"c"`},
		{authorField{FormField: pdfops.FormField{Kind: "text", Name: "t"}, Value: 5.0}, `"t"`},
		{authorField{FormField: pdfops.FormField{Kind: "radio", Name: "r", Options: []string{"a", "b"}}, Value: "z"}, `"r"`},
		{authorField{FormField: pdfops.FormField{Kind: "text"}, Value: "x"}, "no name"},
	} {
		if _, _, err := authorDefaults([]authorField{c.f}); err == nil || !strings.Contains(err.Error(), c.fail) {
			t.Errorf("%+v: err = %v, want one mentioning %s", c.f, err, c.fail)
		}
	}
	if data, n, err := authorDefaults([]authorField{{FormField: pdfops.FormField{Kind: "text", Name: "t"}}}); data != nil || n != 0 || err != nil {
		t.Errorf("a field with no value gave %q %d %v", data, n, err)
	}
}
