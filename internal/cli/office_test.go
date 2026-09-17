package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheUnprintableWarningIsAskedOfMarkdownOnly — `/pending 491`.
//
// `nib office vertrag.odt -o out.pdf` printed *"warning: 163 character(s) cannot be printed and
// will render as blanks: � (U+FFFD), ծ (U+056E), ц (U+0446) …"* for a one-line German ODT that
// converted perfectly: `cmdOffice` ran the Markdown scan on the raw input before the conversion
// picked its path, and an ODT is a ZIP. Every office conversion through the CLI printed that noise,
// which is how the real warning — the Markdown document that genuinely loses characters — becomes
// the one people have learned to scroll past.
//
// Three inputs, because each answers something the others cannot: the ZIP bytes with no LibreOffice
// needed (the guard itself, which runs everywhere), a document LibreOffice really converts (the
// guard does not cost a conversion), and a Markdown file that still warns (the warning was guarded,
// not deleted).
func TestTheUnprintableWarningIsAskedOfMarkdownOnly(t *testing.T) {
	dir := t.TempDir()

	// A DOCX that is only bytes. The conversion fails without LibreOffice and the scan runs BEFORE
	// it either way, so this case exercises the guard on every machine. The bytes are not valid
	// UTF-8, so a scan that ran would have something to report.
	binary := []byte("PK\x03\x04")
	for b := 0x80; b <= 0xff; b++ {
		binary = append(binary, byte(b))
	}
	docx := filepath.Join(dir, "vertrag.docx")
	mustWrite(t, docx, binary)
	out := filepath.Join(dir, "vertrag.pdf")
	if e := captureStderr(t, func() { cmdOffice([]string{docx, "-o", out}) }); strings.Contains(e, "cannot be printed") {
		t.Errorf("nib office on a .docx warned about the container's own bytes — the Markdown scan "+
			"ran on a ZIP:\n%s", e)
	}

	// A real ODT, so the guard is shown not to have cost the conversion it sits in front of. It is
	// produced here rather than committed, which needs LibreOffice — and LibreOffice is what
	// converts it back, so there is nothing to assert without it.
	soffice, lerr := exec.LookPath("soffice")
	if lerr != nil {
		t.Log("SKIP (not a pass): LibreOffice is absent, so the real-document half of this test did " +
			"not run — only the byte-level guard above and the Markdown control below did.")
		return
	}
	src := filepath.Join(dir, "vertrag.txt")
	mustWrite(t, src, []byte("Der Vertrag über die Lieferung von Büromöbeln.\n"))
	cmd := exec.Command(soffice, "-env:UserInstallation=file://"+filepath.Join(dir, "prof"),
		"--headless", "--convert-to", "odt", src, "--outdir", dir)
	if o, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("SKIP (not a pass): LibreOffice could not produce the fixture: %v\n%s", err, o)
	}
	odt := filepath.Join(dir, "vertrag.odt")
	if _, err := os.Stat(odt); err != nil {
		t.Skipf("SKIP (not a pass): no converted fixture: %v", err)
	}
	var code int
	odtOut := filepath.Join(dir, "odt.pdf")
	stderr := captureStderr(t, func() { code = cmdOffice([]string{odt, "-o", odtOut}) })
	if code != 0 {
		t.Fatalf("nib office on a real ODT exited %d: %s", code, stderr)
	}
	if strings.Contains(stderr, "cannot be printed") {
		t.Errorf("a real ODT that converted perfectly still warned about unprintable characters:\n%s", stderr)
	}

	// The control: the warning is guarded, not removed. U+FFFD is the rune the ODT's compressed
	// bytes decoded to, and nothing in Nib's font pool prints it.
	md := filepath.Join(dir, "notes.md")
	mustWrite(t, md, []byte("# Titel\n\nEin besch�digtes Zeichen.\n"))
	mdOut := filepath.Join(dir, "notes.pdf")
	stderr = captureStderr(t, func() { code = cmdOffice([]string{md, "-o", mdOut}) })
	if code != 0 {
		t.Fatalf("nib office on Markdown exited %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "cannot be printed") {
		t.Errorf("a Markdown file with an unprintable rune produced no warning — the guard swallowed "+
			"the case the warning exists for:\n%s", stderr)
	}
}
