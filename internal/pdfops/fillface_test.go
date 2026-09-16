package pdfops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPdfcpuStillCannotFillAFieldSetInAnEmbeddedFace is `/pending 479`'s gate, and the half that rots.
//
// AuthorForm sets field text in Base-14 Helvetica, which fails PDF/UA 7.21.4.1, because the embedded
// face every other authoring door uses cannot be FILLED: measured, pdfcpu v0.13.0 writes the value as
// one-byte text against the Identity-H font and v0.15.0 refuses the fill. The fill tests cannot see
// this — they read the value back through the form export, which is right either way — so the
// observable here is what a reader draws, read out with poppler.
//
// Red means the gate has opened: route AuthorForm through `AuthoredTextFaces`, run the
// `embeddedFontsAreHonest` tail, and flip the text-form row in `uacheck`'s font test to Pass.
func TestPdfcpuStillCannotFillAFieldSetInAnEmbeddedFace(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("SKIP (not a pass): pdftotext (poppler) is not installed, so what a filled field " +
			"draws cannot be read and /pending 479's gate is UNWATCHED here")
	}
	face, _, embedded := AuthoredTextFaces()
	if !embedded {
		t.Skip("SKIP (not a pass): the embedded faces are unavailable here, so there is no " +
			"embedded face to author a form in")
	}
	const value = "Zoë Ñúñez"
	shown := func(face string) (string, error) {
		base, err := CreateFromJSON([]byte(`{"pages":{"1":{"content":{}}}}`))
		if err != nil {
			t.Fatal(err)
		}
		form, err := authorFormIn(base, []FormField{
			{Page: 1, Rect: [4]float64{50, 500, 250, 520}, Kind: "text", Name: "fullName", Label: "Name"},
		}, face)
		if err != nil {
			t.Fatalf("authoring in %s: %v", face, err)
		}
		filled, err := FillFormJSON(form, []byte(`{"forms":[{"textfield":[{"name":"fullName","value":"`+value+`"}]}]}`))
		if err != nil {
			return "", err
		}
		p := filepath.Join(t.TempDir(), "filled.pdf")
		if err := os.WriteFile(p, filled, 0o600); err != nil {
			t.Fatal(err)
		}
		txt, err := exec.Command("pdftotext", p, "-").Output()
		if err != nil {
			t.Fatalf("pdftotext: %v", err)
		}
		return strings.TrimSpace(string(txt)), nil
	}

	// Control: the same fill in the face AuthorForm ships draws the value, or this probe cannot see a
	// filled value at all and "still broken" below would be true of every face.
	if got, err := shown("Helvetica"); err != nil || !strings.Contains(got, value) {
		t.Fatalf("control: a Helvetica form filled with %q draws %q (err %v)", value, got, err)
	}
	got, err := shown(face)
	if err == nil && strings.Contains(got, value) {
		t.Errorf("pdfcpu now fills a field set in the embedded face %s correctly (draws %q). "+
			"/pending 479's gate has opened: AuthorForm can embed its field font and clear PDF/UA "+
			"7.21.4.1 — see the comment on AuthorForm", face, got)
	}
	t.Logf("a %s form filled with %q draws %q (fill err %v)", face, value, got, err)
}
