package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// TestTermTextEscapesWhatATerminalWouldObey — /pending 727. Every control, bidi and invalid byte
// becomes a visible escape; ordinary text, accents and symbols included, passes unchanged.
func TestTermTextEscapesWhatATerminalWouldObey(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Plain title", "Plain title"},
		{"Café — § 3 ✓", "Café — § 3 ✓"},
		{"a\x1b[2Jb", `a\x1b[2Jb`},
		{"line\nvalid (3 signer(s))", `line\x0avalid (3 signer(s))`},
		{"tab\there", `tab\x09here`},
		{"del\x7f", `del\x7f`},
		{"c1\u009b31m", `c1\u009b31m`},
		{"rtl" + string(rune(0x202e)) + "gnp.exe", "rtl\\u202egnp.exe"},
		{"iso" + string(rune(0x2066)) + "x", "iso\\u2066x"},
		{"bad\xffbyte", `bad\xffbyte`},
	} {
		if got := termText(c.in); got != c.want {
			t.Errorf("termText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEveryDocumentStringReachesTheTerminalEscaped — /pending 727. A document's own strings reach
// the terminal through `termText` on every command that prints them: a bookmark title, an
// attachment's name, a ceremony party's label, and any error that wraps one. Each fixture carries a
// real escape byte and a newline; neither may arrive raw.
func TestEveryDocumentStringReachesTheTerminalEscaped(t *testing.T) {
	const hostile = "Evil\x1b]0;pwned\x07\nvalid (9 signer(s))"
	raw := func(out string) bool {
		return strings.ContainsAny(out, "\x1b\x07") || strings.Contains(out, "\nvalid (9")
	}

	dir := t.TempDir()

	// Written by hand: pdfcpu's own writer drops the control bytes, and an author's file need not.
	bm := filepath.Join(dir, "bm.pdf")
	mustWrite(t, bm, testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Outlines 5 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		4: "<< /Length 0 >>\nstream\n\nendstream",
		5: "<< /Type /Outlines /First 6 0 R /Last 6 0 R /Count 1 >>",
		6: `<< /Title (Evil\033]0;pwned\007\nvalid \(9 signer\(s\)\)) /Parent 5 0 R /Dest [3 0 R /Fit] >>`,
	}))
	out, code := captureStdout(t, func() int { return cmdOutline([]string{bm}) })
	// pdfcpu's bookmark reader drops control bytes today, so this site is held by the door only in
	// depth: the assertion is that nothing raw arrives, whichever layer removed it.
	if code != 0 || !strings.Contains(out, "Evil") {
		t.Fatalf("setup: nib outline exit %d did not print the hostile title at all: %q", code, out)
	}
	if raw(out) {
		t.Errorf("nib outline printed a bookmark title's control bytes raw: %q", out)
	}

	att := filepath.Join(dir, "att.pdf")
	mustWrite(t, att, testpdf.WithEmbedded(testpdf.Embedded{Key: hostile + ".txt", F: hostile + ".txt", UF: hostile + ".txt", Data: "x"}))
	out, code = captureStdout(t, func() int { return cmdAttachments([]string{att}) })
	if code != 0 || !strings.Contains(out, "Evil") {
		t.Fatalf("setup: nib attachments exit %d did not list the hostile name: %q", code, out)
	}
	if raw(out) {
		t.Errorf("nib attachments printed an attachment name's control bytes raw: %q", out)
	}

	saved := convenedNames[1]
	convenedNames[1] = hostile
	cer := convenedFixture(t, dir, 2, 1)
	convenedNames[1] = saved
	out, _ = captureStdout(t, func() int { return cmdVerify([]string{cer}) })
	if !strings.Contains(out, "Evil") {
		t.Fatalf("setup: nib verify did not print the hostile party label: %q", out)
	}
	if raw(out) {
		t.Errorf("nib verify printed a ceremony party's label raw: %q", out)
	}

	errOut := captureStderr(t, func() { errf("%s: %v", "file.pdf", hostile) })
	if raw(errOut) {
		t.Errorf("errf printed an error's document text raw: %q", errOut)
	}
}
