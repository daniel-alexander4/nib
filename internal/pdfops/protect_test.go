package pdfops

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// protectedP reads the /P a protected copy carries, opening it with its password.
func protectedP(t *testing.T, pdf []byte, pw string) int {
	t.Helper()
	conf := model.NewDefaultConfiguration()
	conf.UserPW, conf.OwnerPW = pw, pw
	ctx, err := api.ReadContext(bytes.NewReader(pdf), conf)
	if err != nil {
		t.Fatalf("the protected copy did not open with its password: %v", err)
	}
	if ctx.E == nil {
		t.Fatal("the protected copy carries no encryption dictionary")
	}
	return ctx.E.P
}

// TestProtectionControlsOpeningAndNothingElse — /pending 640. The one password opens AND owns the copy, so a
// permission bit binds nobody who can open it; the file must not claim otherwise, and it must not deny bit 10
// (extraction for accessibility). Every bit from 3 to 12 is granted.
func TestProtectionControlsOpeningAndNothingElse(t *testing.T) {
	src := threePagePDF(t)
	// Stimulus first: pdfcpu's own AES default is the restrictive one this fix replaces.
	var dflt bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(src), &dflt, model.NewAESConfiguration("pw", "pw", 256)); err != nil {
		t.Fatal(err)
	}
	if p := protectedP(t, dflt.Bytes(), "pw"); p&(1<<9) != 0 {
		t.Fatalf("setup: pdfcpu's default now grants bit 10 (/P %#x), so this test no longer tells nib's choice from it", p)
	}
	enc, err := Encrypt(src, "pw")
	if err != nil {
		t.Fatal(err)
	}
	p := protectedP(t, enc, "pw")
	if p&3 != 0 {
		t.Errorf("the protected copy sets permission bits 1-2 (/P %#x), which ISO 32000 requires to be 0", p)
	}
	for bit := 3; bit <= 12; bit++ {
		if bit == 7 || bit == 8 {
			continue // reserved
		}
		if p&(1<<(bit-1)) == 0 {
			t.Errorf("the protected copy denies permission bit %d (/P %#x) — a restriction nobody holding the password is bound by", bit, p)
		}
	}
}

// TestProtectionDoesNotReadTheUsersPdfcpuConfig — the flags nib writes are nib's, not a config file's on the
// machine. pdfcpu caches its config per process, so the hostile config is planted in a fresh one.
func TestProtectionDoesNotReadTheUsersPdfcpuConfig(t *testing.T) {
	if os.Getenv("NIB_PROTECT_CONFIG_CHILD") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProtectionDoesNotReadTheUsersPdfcpuConfig$", "-test.v")
		cmd.Env = append(os.Environ(), "NIB_PROTECT_CONFIG_CHILD="+t.TempDir())
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "--- PASS") {
			t.Fatalf("the child run under a planted pdfcpu config did not pass (%v):\n%s", err, out)
		}
		return
	}
	dir := os.Getenv("NIB_PROTECT_CONFIG_CHILD")
	if err := model.EnsureDefaultConfigAt(dir, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pdfcpu", "config.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hostile := strings.Replace(string(b), "encryptKeyLength: 256", "encryptKeyLength: 128", 1)
	// Its `permissions: 0xF0C3` — every restriction, bit 10 included — is already the hostile value.
	if hostile == string(b) || !strings.Contains(hostile, "permissions: 0xF0C3") {
		t.Fatal("setup: pdfcpu's default config no longer spells encryptKeyLength: 256 / permissions: 0xF0C3")
	}
	if err := os.WriteFile(path, []byte(hostile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := model.EnsureDefaultConfigAt(dir, false); err != nil {
		t.Fatal(err)
	}
	if c := model.NewDefaultConfiguration(); c.EncryptKeyLength != 128 || c.Permissions != 0xF0C3 {
		t.Fatal("setup: pdfcpu did not load the planted config")
	}
	enc, err := Encrypt(threePagePDF(t), "pw")
	if err != nil {
		t.Fatal(err)
	}
	conf := model.NewDefaultConfiguration()
	conf.UserPW, conf.OwnerPW = "pw", "pw"
	ctx, err := api.ReadContext(bytes.NewReader(enc), conf)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.E == nil || ctx.E.V != 5 { // V 5 is AES-256; a 128-bit key is V 4
		t.Errorf("under a config asking for 128-bit keys the protected copy is %+v, want AES-256 as nib chose", ctx.E)
	}
	if ctx.E != nil && ctx.E.P&(1<<9) == 0 {
		t.Errorf("under a planted config the protected copy denies bit 10 (/P %#x)", ctx.E.P)
	}
}
